package handlers

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/gorilla/mux"
	"warehousecore/internal/middleware"
	"warehousecore/internal/models"
	"warehousecore/internal/repository"
)

func TestWarehouseTasksAtomicLifecycleAndReferences(t *testing.T) {
	dsn := os.Getenv("WAREHOUSE_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("disposable _test database required")
	}
	parsed, err := url.Parse(dsn)
	if err != nil || !strings.HasSuffix(parsed.Path, "_test") {
		t.Fatal("dedicated test database required")
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	exec := func(q string) {
		t.Helper()
		if _, err := db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	exec("DROP SCHEMA IF EXISTS warehouse_task_mcp_test CASCADE; CREATE SCHEMA warehouse_task_mcp_test; SET search_path TO warehouse_task_mcp_test")
	defer db.Exec("DROP SCHEMA warehouse_task_mcp_test CASCADE")
	exec(warehouseDeviceFixtureSQL)
	exec(`DROP TABLE warehouse_tasks;
 CREATE TABLE warehouse_tasks(task_id BIGSERIAL PRIMARY KEY,task_type TEXT,status TEXT DEFAULT 'open',priority INT DEFAULT 50,from_zone_id BIGINT,to_zone_id BIGINT,case_id BIGINT,device_id TEXT,product_id BIGINT,quantity NUMERIC(12,3),job_id BIGINT,assigned_to BIGINT,due_at TIMESTAMP,notes TEXT,created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,completed_at TIMESTAMP);
 ALTER TABLE products ADD COLUMN updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP;
 ALTER TABLE storage_zones ADD COLUMN updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP;
 ALTER TABLE cases ADD COLUMN name TEXT; ALTER TABLE cases ADD COLUMN lifecycle_status TEXT; ALTER TABLE cases ADD COLUMN updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP; INSERT INTO cases(caseid,name,zone_id,lifecycle_status) VALUES(1,'Work case',1,'active');
 ALTER TABLE jobs ADD COLUMN updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP;
 CREATE TABLE users(userid BIGINT PRIMARY KEY,username TEXT,is_active BOOLEAN,updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP); INSERT INTO users(userid,username,is_active) VALUES(11,'worker',true),(12,'inactive',false);
 CREATE FUNCTION task_test_touch_reference() RETURNS TRIGGER AS $$ BEGIN NEW.updated_at:=GREATEST(clock_timestamp() AT TIME ZONE 'UTC',OLD.updated_at+INTERVAL '1 microsecond'); RETURN NEW; END $$ LANGUAGE plpgsql;
 CREATE TRIGGER task_test_product_version BEFORE UPDATE ON products FOR EACH ROW EXECUTE FUNCTION task_test_touch_reference();
 CREATE TRIGGER task_test_case_version BEFORE UPDATE ON cases FOR EACH ROW EXECUTE FUNCTION task_test_touch_reference();
 CREATE TRIGGER task_test_zone_version BEFORE UPDATE ON storage_zones FOR EACH ROW EXECUTE FUNCTION task_test_touch_reference();
 INSERT INTO devices(productid,serialnumber,status,condition_status) VALUES(1,'TASK-DEVICE','location_unknown','available')`)
	old := repository.DB
	repository.DB = db
	defer func() { repository.DB = old }()
	for i := 0; i < 2; i++ {
		for _, ensure := range []func() error{EnsureWarehouseDeviceVersionSchema, EnsureWarehouseTaskLifecycleSchema} {
			if err := ensure(); err != nil {
				t.Fatal(err)
			}
		}
	}
	count := func(table string) int {
		t.Helper()
		var n int
		if err := db.QueryRow("SELECT count(*) FROM " + table).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	version := func(table, column string, id any) string {
		t.Helper()
		var v string
		if err := db.QueryRow("SELECT to_char(updated_at,'YYYY-MM-DD\"T\"HH24:MI:SS.US\"Z\"') FROM "+table+" WHERE "+column+"=$1", id).Scan(&v); err != nil {
			t.Fatal(err)
		}
		return v
	}
	call := func(op, key string, body map[string]any, admin bool, want int) map[string]any {
		t.Helper()
		raw, _ := json.Marshal(body)
		r := httptest.NewRequest(http.MethodPost, "/task", bytes.NewReader(raw))
		r = mux.SetURLVars(r, map[string]string{"operation": op})
		r = r.WithContext(context.WithValue(r.Context(), middleware.UserContextKey, &models.User{UserID: 11, IsAdmin: admin}))
		r.Header.Set("X-Cores-Origin", "MCP/AI")
		r.Header.Set("Idempotency-Key", key)
		w := httptest.NewRecorder()
		WarehouseTaskMCP(w, r)
		if w.Code != want {
			t.Fatalf("%s status %d want %d: %s", op, w.Code, want, w.Body.String())
		}
		out := map[string]any{}
		if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
			t.Fatal(err)
		}
		return out
	}
	blocked := func(out map[string]any, field string) {
		t.Helper()
		if out["ready_to_execute"] != false || !strings.Contains(fmt.Sprint(out["required_fields"]), field) {
			t.Fatalf("missing blocker %s: %#v", field, out)
		}
	}
	device := "DEV-00000001"
	create := map[string]any{"task_type": "move", "priority": 0, "from_zone_id": int64(1), "to_zone_id": int64(3), "case_id": int64(1), "device_id": device, "product_id": int64(1), "quantity": 1, "job_id": int64(1), "assigned_to": int64(11), "due_at": "2099-01-01T10:00:00+02:00", "notes": "private warehouse work"}
	call("create", "", create, false, 403)
	p := call("create", "", create, true, 200)
	if p["ready_to_create"] != true || len(p["expected_references"].(map[string]any)) != 7 || p["effects"].(map[string]any)["inventory_movement"] != false {
		t.Fatal("wrong task preview", p)
	}
	if count("warehouse_tasks") != 0 || count("audit_log") != 0 || count("warehouse_task_events") != 0 || count("warehouse_product_mutation_receipts") != 0 {
		t.Fatal("preview wrote")
	}
	create["confirm_creation"] = true
	call("create", "task-reference-required", create, true, 428)
	create["expected_references"] = p["expected_references"]
	create["preview"] = true
	call("create", "", create, true, 200)
	if count("warehouse_tasks") != 0 {
		t.Fatal("dryrun wrote")
	}
	delete(create, "preview")
	call("create", "", create, true, 428)
	// A reference change after preview invalidates creation, even if the task
	// does not yet have a version of its own.
	exec("UPDATE storage_zones SET name='Concurrent location change' WHERE zone_id=1")
	blocked(call("create", "task-stale-reference", create, true, 200), "expected_references")
	delete(create, "confirm_creation")
	delete(create, "expected_references")
	p = call("create", "", create, true, 200)
	create["confirm_creation"] = true
	create["expected_references"] = p["expected_references"]
	c := call("create", "task-create-once", create, true, 200)
	id := int64(c["warehouse_task"].(map[string]any)["task_id"].(float64))
	if !reflect.DeepEqual(c, call("create", "task-create-once", create, true, 200)) || count("warehouse_tasks") != 1 || count("warehouse_task_events") != 1 {
		t.Fatal("durable creation duplicated")
	}
	if c["warehouse_task"].(map[string]any)["priority"] != float64(0) || c["warehouse_task"].(map[string]any)["due_at"] != "2099-01-01T08:00:00.000000Z" {
		t.Fatal("metadata lost", c)
	}
	controls := func(op string, extra map[string]any) map[string]any {
		t.Helper()
		a := map[string]any{"task_id": id}
		for k, v := range extra {
			a[k] = v
		}
		p := call(op, "", a, true, 200)
		a["expected_updated_at"] = p["expected_updated_at"]
		a["expected_references"] = p["expected_references"]
		a["confirm_change"] = true
		if phrase := p["required_confirmation_text"]; phrase != nil {
			a["confirmation_text"] = phrase
		}
		return a
	}
	u := controls("update", map[string]any{"priority": 80, "clear_fields": []string{"case_id", "assigned_to", "notes"}})
	if len(u["expected_references"].(map[string]any)) != 7 {
		t.Fatal("replaced references missing from full preview")
	}
	exec(fmt.Sprintf("INSERT INTO warehouse_task_events(task_id,event_type) VALUES(%d,'ui_note')", id))
	blocked(call("update", "task-stale-event", u, true, 200), "expected_updated_at")
	u = controls("update", map[string]any{"priority": 80, "clear_fields": []string{"case_id", "assigned_to", "notes"}})
	updated := call("update", "task-update-once", u, true, 200)
	if updated["warehouse_task"].(map[string]any)["task_type"] != "move" || updated["warehouse_task"].(map[string]any)["case_id"] != nil || updated["warehouse_task"].(map[string]any)["notes"] != "" {
		t.Fatal("partial update/clear lost fields", updated)
	}
	beforeTask, beforeDevice := version("warehouse_tasks", "task_id", id), version("devices", "deviceid", device)
	events, audits, receipts := count("warehouse_task_events"), count("audit_log"), count("warehouse_product_mutation_receipts")
	exec(`CREATE FUNCTION reject_task_audit() RETURNS TRIGGER AS $$ BEGIN IF NEW.entity_type='warehouse_task' AND NEW.action='warehouse_task.start' THEN RAISE EXCEPTION 'audit failed';END IF;RETURN NEW;END $$ LANGUAGE plpgsql;CREATE TRIGGER reject_task_audit BEFORE INSERT ON audit_log FOR EACH ROW EXECUTE FUNCTION reject_task_audit()`)
	start := controls("start", nil)
	call("start", "task-start-audit-fail", start, true, 500)
	if version("warehouse_tasks", "task_id", id) != beforeTask || version("devices", "deviceid", device) != beforeDevice || count("warehouse_task_events") != events || count("audit_log") != audits || count("warehouse_product_mutation_receipts") != receipts {
		t.Fatal("failed audit leaked task/event/reference changes")
	}
	exec("DROP TRIGGER reject_task_audit ON audit_log")
	started := call("start", "task-start", start, true, 200)
	if started["warehouse_task"].(map[string]any)["started_at"] == nil {
		t.Fatal("start not recorded")
	}
	done := controls("complete", nil)
	done["confirmation_text"] = "WRONG"
	call("complete", "task-wrong-phrase", done, true, 428)
	done["confirmation_text"] = fmt.Sprintf("COMPLETE WAREHOUSE TASK %d", id)
	completed := call("complete", "task-complete", done, true, 200)
	if completed["warehouse_task"].(map[string]any)["completed_at"] == nil {
		t.Fatal("completion not recorded")
	}
	if !reflect.DeepEqual(completed, call("complete", "task-complete", done, true, 200)) {
		t.Fatal("completion replay changed")
	}
	var physical, condition string
	db.QueryRow("SELECT status,condition_status FROM devices WHERE deviceid=$1", device).Scan(&physical, &condition)
	if physical != "location_unknown" || condition != "available" || count("device_movements") != 0 {
		t.Fatal("work-item completion moved inventory")
	}
	archived := call("archive", "task-archive", controls("archive", nil), true, 200)
	if archived["warehouse_task"].(map[string]any)["is_archived"] != true {
		t.Fatal("not archived")
	}
	if _, err = db.Exec("UPDATE warehouse_tasks SET notes='Forbidden archive edit' WHERE task_id=$1", id); err == nil {
		t.Fatal("archived metadata write allowed")
	}
	call("restore", "task-restore", controls("restore", nil), true, 200)
	reopened := call("reopen", "task-reopen", controls("reopen", map[string]any{"reason": "Follow-up work"}), true, 200)
	if reopened["warehouse_task"].(map[string]any)["status"] != "open" || reopened["warehouse_task"].(map[string]any)["completed_at"] != nil {
		t.Fatal("reopening failed")
	}
	blocked(call("archive", "task-active-archive", controls("archive", nil), true, 200), "terminal_task_required")
	call("cancel", "task-cancel", controls("cancel", map[string]any{"reason": "Cancelled duplicate work"}), true, 200)
	exec("UPDATE products SET lifecycle_status='archived' WHERE productid=1")
	blocked(call("reopen", "task-inactive-product", controls("reopen", map[string]any{"reason": "Reopen request"}), true, 200), "active_reference")
	call("archive", "task-history-archive", controls("archive", nil), true, 200)
	// Restore terminal history remains possible after referenced masters archive.
	restored := call("restore", "task-history-restore", controls("restore", nil), true, 200)
	if restored["warehouse_task"].(map[string]any)["status"] != "cancelled" {
		t.Fatal("terminal restore changed state")
	}
}
