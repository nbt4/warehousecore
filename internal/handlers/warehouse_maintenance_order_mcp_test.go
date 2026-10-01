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

func TestWarehouseMaintenanceOrdersAtomicWorkflows(t *testing.T) {
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
	exec("DROP SCHEMA IF EXISTS warehouse_maintenance_order_mcp_test CASCADE; CREATE SCHEMA warehouse_maintenance_order_mcp_test; SET search_path TO warehouse_maintenance_order_mcp_test")
	defer db.Exec("DROP SCHEMA warehouse_maintenance_order_mcp_test CASCADE")
	exec(warehouseDeviceFixtureSQL)
	exec(`DROP TABLE maintenance_orders; DROP TABLE maintenance_plans; DROP TABLE defect_reports;
 CREATE TABLE users(userid BIGINT PRIMARY KEY,is_active BOOLEAN); INSERT INTO users VALUES(11,true),(12,false);
 CREATE TABLE defect_reports(defect_id BIGSERIAL PRIMARY KEY,device_id TEXT,description TEXT,severity TEXT,status TEXT,created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,resolved_at TIMESTAMP,resolution TEXT);
 ALTER TABLE products ADD COLUMN maintenanceinterval INT; INSERT INTO warehouse_schema_migrations VALUES('042_maintenance_work_management');
 INSERT INTO devices(productid,serialnumber,status,condition_status) VALUES(1,'WORK-DEVICE','location_unknown','available')`)
	old := repository.DB
	repository.DB = db
	defer func() { repository.DB = old }()
	for i := 0; i < 2; i++ {
		for _, ensure := range []func() error{EnsureMaintenanceSchema, EnsureWarehouseDeviceVersionSchema, EnsureWarehouseMaintenanceVersionSchema, EnsureWarehouseMaintenanceLifecycleSchema} {
			if err := ensure(); err != nil {
				t.Fatal(err)
			}
		}
	}
	device := "DEV-00000001"
	version := func(table, column string, id any) string {
		t.Helper()
		var v string
		if err := db.QueryRow("SELECT to_char(updated_at,'YYYY-MM-DD\"T\"HH24:MI:SS.US\"Z\"') FROM "+table+" WHERE "+column+"=$1", id).Scan(&v); err != nil {
			t.Fatal(err)
		}
		return v
	}
	count := func(table string) int {
		t.Helper()
		var n int
		if err := db.QueryRow("SELECT count(*) FROM " + table).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	call := func(entity, op, key string, body map[string]any, admin bool, want int) map[string]any {
		t.Helper()
		raw, _ := json.Marshal(body)
		r := httptest.NewRequest(http.MethodPost, "/work", bytes.NewReader(raw))
		r = mux.SetURLVars(r, map[string]string{"entity": entity, "operation": op})
		r = r.WithContext(context.WithValue(r.Context(), middleware.UserContextKey, &models.User{UserID: 11, IsAdmin: admin}))
		r.Header.Set("X-Cores-Origin", "MCP/AI")
		r.Header.Set("Idempotency-Key", key)
		w := httptest.NewRecorder()
		WarehouseMaintenanceOrderMCP(w, r)
		if w.Code != want {
			t.Fatalf("%s %s status %d want %d: %s", entity, op, w.Code, want, w.Body.String())
		}
		out := map[string]any{}
		if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
			t.Fatal(err)
		}
		return out
	}
	controls := func(id int64, op string) map[string]any {
		return map[string]any{"order_id": id, "expected_updated_at": version("maintenance_orders", "order_id", id), "expected_device_updated_at": version("devices", "deviceid", device), "confirm_change": true, "confirmation_text": fmt.Sprintf("%s WAREHOUSE MAINTENANCE ORDER %d", strings.ToUpper(op), id)}
	}
	blocked := func(out map[string]any, field string) {
		t.Helper()
		if out["ready_to_execute"] != false || !strings.Contains(fmt.Sprint(out["required_fields"]), field) {
			t.Fatalf("missing blocker %s: %#v", field, out)
		}
	}
	create := map[string]any{"device_id": device, "title": "Repair A", "description": "Private business defect", "priority": "high", "scheduled_at": "2099-01-01T10:00:00+02:00"}
	call("defects", "create", "", create, false, 403)
	p := call("defects", "create", "", create, true, 200)
	if p["ready_to_execute"] != true || p["draft"].(map[string]any)["order_type"] != "defect" || p["effects"].(map[string]any)["condition_status"].(map[string]any)["after"] != "defective" {
		t.Fatal("wrong defect preview", p)
	}
	if count("maintenance_orders") != 0 || count("audit_log") != 0 || count("maintenance_order_events") != 0 || count("warehouse_product_mutation_receipts") != 0 {
		t.Fatal("preview wrote")
	}
	create["confirm_change"] = true
	call("defects", "create", "missing-version", create, true, 428)
	create["expected_device_updated_at"] = version("devices", "deviceid", device)
	create["preview"] = true
	call("defects", "create", "", create, true, 200)
	if count("maintenance_orders") != 0 {
		t.Fatal("dryrun wrote")
	}
	delete(create, "preview")
	call("defects", "create", "", create, true, 428)
	c := call("defects", "create", "defect-create-once", create, true, 200)
	id := int64(c["order"].(map[string]any)["order_id"].(float64))
	if !reflect.DeepEqual(c, call("defects", "create", "defect-create-once", create, true, 200)) || count("maintenance_orders") != 1 || count("maintenance_order_events") != 1 {
		t.Fatal("durable create replay duplicated")
	}
	if c["device"].(map[string]any)["condition_status"] != "defective" || c["order"].(map[string]any)["scheduled_at"] != "2099-01-01T08:00:00.000000Z" {
		t.Fatal("condition/schedule incorrect", c)
	}
	create["title"] = "Changed key payload"
	call("defects", "create", "defect-create-once", create, true, 409)
	update := controls(id, "update")
	update["title"] = "Repair A revised"
	exec(fmt.Sprintf("INSERT INTO maintenance_order_events(order_id,event_type) VALUES(%d,'ui_note')", id))
	blocked(call("defects", "update", "stale-event-version", update, true, 200), "expected_updated_at")
	update = controls(id, "update")
	update["title"] = "Repair A revised"
	update["assigned_to"] = int64(12)
	blocked(call("defects", "update", "inactive-assignee", update, true, 200), "active_assignee_required")
	update["assigned_to"] = int64(11)
	update["clear_fields"] = []string{"description", "scheduled_at"}
	call("defects", "update", "defect-update-once", update, true, 200)
	done := controls(id, "complete")
	done["outcome"] = "repaired"
	done["resolution"] = "Completed inspection"
	blocked(call("defects", "complete", "not-started-yet", done, true, 200), "start_work_before_completion")
	start := controls(id, "transition")
	start["status"] = "in_progress"
	started := call("defects", "transition", "defect-start", start, true, 200)
	if started["device"].(map[string]any)["condition_status"] != "maintenance" {
		t.Fatal("work did not start", started)
	}
	done = controls(id, "complete")
	done["outcome"] = "repaired"
	done["resolution"] = "Completed inspection"
	done["confirmation_text"] = "WRONG"
	call("defects", "complete", "wrong-completion-phrase", done, true, 428)
	done["confirmation_text"] = fmt.Sprintf("COMPLETE WAREHOUSE MAINTENANCE ORDER %d", id)
	// Full rollback includes order, event, device, audit and durable receipt.
	exec(`CREATE FUNCTION reject_work_audit() RETURNS TRIGGER AS $$ BEGIN IF NEW.entity_type='maintenance_order' AND NEW.action='maintenance_order.complete' THEN RAISE EXCEPTION 'audit failed'; END IF; RETURN NEW; END $$ LANGUAGE plpgsql; CREATE TRIGGER reject_work_audit BEFORE INSERT ON audit_log FOR EACH ROW EXECUTE FUNCTION reject_work_audit()`)
	beforeOrder, beforeDevice := version("maintenance_orders", "order_id", id), version("devices", "deviceid", device)
	events, audits, receipts := count("maintenance_order_events"), count("audit_log"), count("warehouse_product_mutation_receipts")
	call("defects", "complete", "work-audit-fail", done, true, 500)
	if version("maintenance_orders", "order_id", id) != beforeOrder || version("devices", "deviceid", device) != beforeDevice || count("maintenance_order_events") != events || count("audit_log") != audits || count("warehouse_product_mutation_receipts") != receipts {
		t.Fatal("failed audit leaked")
	}
	exec("DROP TRIGGER reject_work_audit ON audit_log")
	completed := call("defects", "complete", "defect-complete-once", done, true, 200)
	if completed["device"].(map[string]any)["condition_status"] != "available" || completed["order"].(map[string]any)["status"] != "completed" {
		t.Fatal("repair did not release device", completed)
	}
	if !reflect.DeepEqual(completed, call("defects", "complete", "defect-complete-once", done, true, 200)) {
		t.Fatal("completion replay changed")
	}
	archived := call("defects", "archive", "defect-archive", controls(id, "archive"), true, 200)
	if archived["order"].(map[string]any)["is_archived"] != true {
		t.Fatal("not archived")
	}
	if _, err = db.Exec("UPDATE maintenance_orders SET title='Forbidden UI edit' WHERE order_id=$1", id); err == nil {
		t.Fatal("archived metadata write allowed")
	}
	blocked(call("defects", "reopen", "archived-reopen", controls(id, "reopen"), true, 200), "restore_before_change")
	call("defects", "restore", "defect-restore", controls(id, "restore"), true, 200)
	reopen := controls(id, "reopen")
	reopen["notes"] = "Repair requires follow-up"
	reopened := call("defects", "reopen", "defect-reopen", reopen, true, 200)
	if reopened["order"].(map[string]any)["completed_at"] != nil || reopened["device"].(map[string]any)["condition_status"] != "defective" {
		t.Fatal("reopen did not preserve effects", reopened)
	}
	blocked(call("defects", "archive", "active-archive", controls(id, "archive"), true, 200), "terminal_order_required")
	cancel := controls(id, "cancel")
	cancel["notes"] = "Duplicate resolved externally"
	call("defects", "cancel", "defect-cancel", cancel, true, 200)
	// The old root defect schema is retained and synchronized in the same tx.
	exec(fmt.Sprintf(`INSERT INTO defect_reports(device_id,description,severity,status) VALUES('%s','Legacy defect','major','open'); INSERT INTO maintenance_orders(legacy_defect_id,device_id,order_type,status,title) VALUES(1,'%s','defect','in_progress','Migrated defect')`, device, device))
	var legacyOrder int64
	db.QueryRow("SELECT order_id FROM maintenance_orders WHERE legacy_defect_id=1").Scan(&legacyOrder)
	legacyDone := controls(legacyOrder, "complete")
	legacyDone["outcome"] = "failed"
	legacyDone["resolution"] = "Remains unsafe"
	exec("UPDATE devices SET condition_status='blocked'")
	legacyDone["expected_device_updated_at"] = version("devices", "deviceid", device)
	beforeLegacy := version("maintenance_orders", "order_id", legacyOrder)
	exec("UPDATE defect_reports SET description='Edited through legacy UI' WHERE defect_id=1")
	if version("maintenance_orders", "order_id", legacyOrder) == beforeLegacy {
		t.Fatal("legacy edit did not invalidate order")
	}
	blocked(call("defects", "complete", "stale-legacy-edit", legacyDone, true, 200), "expected_updated_at")
	legacyDone = controls(legacyOrder, "complete")
	legacyDone["outcome"] = "failed"
	legacyDone["resolution"] = "Remains unsafe"
	exec("CREATE TRIGGER reject_work_audit BEFORE INSERT ON audit_log FOR EACH ROW EXECUTE FUNCTION reject_work_audit()")
	beforeOrder, beforeDevice = version("maintenance_orders", "order_id", legacyOrder), version("devices", "deviceid", device)
	events, audits, receipts = count("maintenance_order_events"), count("audit_log"), count("warehouse_product_mutation_receipts")
	call("defects", "complete", "legacy-audit-fail", legacyDone, true, 500)
	var unchangedLegacy string
	db.QueryRow("SELECT status FROM defect_reports WHERE defect_id=1").Scan(&unchangedLegacy)
	if unchangedLegacy != "open" || version("maintenance_orders", "order_id", legacyOrder) != beforeOrder || version("devices", "deviceid", device) != beforeDevice || count("maintenance_order_events") != events || count("audit_log") != audits || count("warehouse_product_mutation_receipts") != receipts {
		t.Fatal("legacy audit failure leaked changes")
	}
	exec("DROP TRIGGER reject_work_audit ON audit_log")
	legacyResult := call("defects", "complete", "legacy-complete", legacyDone, true, 200)
	var legacyState string
	db.QueryRow("SELECT status FROM defect_reports WHERE defect_id=1").Scan(&legacyState)
	if legacyState != "closed" || legacyResult["device"].(map[string]any)["condition_status"] != "blocked" {
		t.Fatal("legacy cleanup/manual block wrong", legacyResult, legacyState)
	}
	// Linked recurring work requires the full current plan version, updates both
	// schedule entities atomically, and respects competing open orders.
	exec(`UPDATE devices SET condition_status='available'; INSERT INTO maintenance_plans(device_id,name,maintenance_type,interval_days,lead_time_days,next_due_at) VALUES('DEV-00000001','Recurring inspection','inspection',30,0,'2000-01-01')`)
	planVersion := func() string { return version("maintenance_plans", "plan_id", int64(1)) }
	planned := map[string]any{"device_id": device, "plan_id": int64(1), "order_type": "inspection", "title": "Recurring work", "confirm_change": true, "expected_device_updated_at": version("devices", "deviceid", device)}
	call("maintenance-orders", "create", "plan-version-required", planned, true, 428)
	planned["expected_plan_updated_at"] = planVersion()
	created := call("maintenance-orders", "create", "linked-work-create", planned, true, 200)
	linkedID := int64(created["order"].(map[string]any)["order_id"].(float64))
	start = controls(linkedID, "transition")
	start["status"] = "in_progress"
	start["expected_plan_updated_at"] = planVersion()
	call("maintenance-orders", "transition", "linked-start", start, true, 200)
	done = controls(linkedID, "complete")
	done["expected_plan_updated_at"] = planVersion()
	done["outcome"] = "passed"
	done["resolution"] = "Safe for service"
	done["next_due_at"] = "2099-05-01"
	exec("CREATE TRIGGER reject_work_audit BEFORE INSERT ON audit_log FOR EACH ROW EXECUTE FUNCTION reject_work_audit()")
	beforePlan := planVersion()
	beforeOrder, beforeDevice = version("maintenance_orders", "order_id", linkedID), version("devices", "deviceid", device)
	events, audits, receipts = count("maintenance_order_events"), count("audit_log"), count("warehouse_product_mutation_receipts")
	call("maintenance-orders", "complete", "linked-audit-fail", done, true, 500)
	if planVersion() != beforePlan || version("maintenance_orders", "order_id", linkedID) != beforeOrder || version("devices", "deviceid", device) != beforeDevice || count("maintenance_order_events") != events || count("audit_log") != audits || count("warehouse_product_mutation_receipts") != receipts {
		t.Fatal("linked audit failure leaked plan changes")
	}
	exec("DROP TRIGGER reject_work_audit ON audit_log")
	linkedDone := call("maintenance-orders", "complete", "linked-complete", done, true, 200)
	if linkedDone["plan"].(map[string]any)["next_due_at"] != "2099-05-01" || linkedDone["device"].(map[string]any)["next_maintenance"] != "2099-05-01" {
		t.Fatal("linked plan/device next date mismatch", linkedDone)
	}
}
