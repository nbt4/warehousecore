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
	"strings"
	"testing"
	"time"

	"warehousecore/internal/middleware"
	"warehousecore/internal/models"
	"warehousecore/internal/repository"

	jwt "github.com/golang-jwt/jwt/v5"
	"github.com/gorilla/mux"
)

func TestWarehouseCaseWorkflowAtomicTreeSchedulingInspectionAndRetention(t *testing.T) {
	dsn := os.Getenv("WAREHOUSE_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("owned _test PostgreSQL required")
	}
	parsed, err := url.Parse(dsn)
	if err != nil || !strings.HasSuffix(parsed.Path, "_test") {
		t.Fatal("owned database required")
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
	exec(`DROP SCHEMA IF EXISTS warehouse_case_workflow_test CASCADE;CREATE SCHEMA warehouse_case_workflow_test;SET search_path TO warehouse_case_workflow_test`)
	defer db.Exec(`DROP SCHEMA warehouse_case_workflow_test CASCADE`)
	exec(warehouseDeviceFixtureSQL)
	exec(inventoryMCPFixtureSQL)
	exec(caseContentFixtureSQL)
	exec(caseWorkflowFixtureSQL)
	// Fresh suite migration must create events before the operations bootstrap.
	exec(`DROP TABLE case_events`)
	old := repository.DB
	repository.DB = db
	defer func() { repository.DB = old }()
	for _, ensure := range []func() error{EnsureWarehouseDeviceVersionSchema, EnsureWarehouseCaseVersionSchema, EnsureWarehouseCaseTemplateLifecycleSchema, EnsureWarehouseCaseContentSafetySchema, EnsureWarehouseCaseWorkflowRetentionSchema} {
		if err = ensure(); err != nil {
			t.Fatal(err)
		}
	}
	// Stock sync applies to both quantity locations and packed contents.
	exec(`CREATE TRIGGER product_locations_sync_stock AFTER INSERT OR UPDATE OR DELETE ON product_locations FOR EACH ROW EXECUTE FUNCTION sync_product_stock_from_locations()`)
	t.Setenv("CORES_JWT_SECRET", strings.Repeat("k", 48))
	call := func(op, key, scope string, in map[string]any, want int) map[string]any {
		t.Helper()
		raw, _ := json.Marshal(in)
		r := httptest.NewRequest("POST", "/contents", bytes.NewReader(raw))
		r = mux.SetURLVars(r, map[string]string{"operation": op})
		r = r.WithContext(context.WithValue(r.Context(), middleware.UserContextKey, &models.User{UserID: 11, IsActive: true, IsAdmin: true}))
		r.Header.Set("X-Cores-Origin", "MCP/AI")
		r.Header.Set("Idempotency-Key", key)
		token, e := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{"uid": 11, "mcp_scope": scope, "exp": time.Now().Add(time.Hour).Unix()}).SignedString([]byte(strings.Repeat("k", 48)))
		if e != nil {
			t.Fatal(e)
		}
		r.AddCookie(&http.Cookie{Name: "cores_token", Value: token})
		w := httptest.NewRecorder()
		CaseWorkflowMCP(w, r)
		var out map[string]any
		if json.Unmarshal(w.Body.Bytes(), &out) != nil || w.Code != want {
			t.Fatalf("%s status%d want%d: %s", op, w.Code, want, w.Body.String())
		}
		return out
	}
	run := func(op, key string, in map[string]any, want int) map[string]any {
		return call(op, key, "cores:warehouse:update", in, want)
	}
	snapshot := func() string {
		t.Helper()
		var value string
		err := db.QueryRow(`SELECT jsonb_build_array((SELECT jsonb_agg(to_jsonb(t) ORDER BY caseid) FROM cases t),(SELECT jsonb_agg(to_jsonb(t) ORDER BY deviceid) FROM devices t),(SELECT jsonb_agg(to_jsonb(t) ORDER BY productid) FROM products t),(SELECT jsonb_agg(to_jsonb(t) ORDER BY caseid,deviceid) FROM devicescases t),(SELECT jsonb_agg(to_jsonb(t) ORDER BY case_id,product_id) FROM case_product_contents t),(SELECT jsonb_agg(to_jsonb(t) ORDER BY parent_case_id,child_case_id) FROM case_child_contents t),(SELECT jsonb_agg(to_jsonb(t) ORDER BY product_id,zone_id) FROM product_locations t),(SELECT count(*) FROM audit_log),(SELECT count(*) FROM case_events),(SELECT count(*) FROM device_movements),(SELECT jsonb_agg(to_jsonb(t) ORDER BY jobid,deviceid) FROM job_devices t),(SELECT jsonb_agg(to_jsonb(t) ORDER BY jobid) FROM jobs t),(SELECT count(*) FROM job_history),(SELECT count(*) FROM warehouse_product_mutation_receipts))::text`).Scan(&value)
		if err != nil {
			t.Fatal(err)
		}
		return value
	}
	confirm := func(op string, in map[string]any) map[string]any {
		t.Helper()
		in["preview"] = true
		in["confirm_change"] = false
		delete(in, "expected_context")
		delete(in, "expected_updated_at")
		delete(in, "confirmation_text")
		before := snapshot()
		out := run(op, "", in, 200)
		if out["ready_to_execute"] != true || snapshot() != before {
			t.Fatalf("preview failed or mutated: %#v", out)
		}
		in["expected_context"] = out["expected_context"]
		in["expected_updated_at"] = out["expected_updated_at"]
		in["confirmation_text"] = out["confirmation_text_required"]
		in["confirm_change"] = true
		in["preview"] = false
		return in
	}
	count := func(q string) float64 {
		t.Helper()
		var n float64
		if err := db.QueryRow(q).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}

	exec(`INSERT INTO devicescases(caseid,deviceid) VALUES(1,'DEV-00000001'),(2,'DEV-00000003');INSERT INTO case_child_contents(parent_case_id,child_case_id) VALUES(1,2);INSERT INTO case_product_contents(case_id,product_id,quantity) VALUES(2,3,1.5);UPDATE cases SET zone_id=NULL WHERE caseid=2;UPDATE cases SET case_type='fixed' WHERE caseid=1;INSERT INTO case_content_templates(case_id,product_id,expected_quantity) VALUES(1,1,2)`)
	exec(`UPDATE cases SET workflow_status='maintenance' WHERE caseid=2`)
	if run("seal", "", map[string]any{"case_id": 1, "accept_incomplete_template": true}, 200)["ready_to_execute"] != false {
		t.Fatal("maintenance child accepted")
	}
	exec(`UPDATE cases SET workflow_status='empty' WHERE caseid=2;UPDATE devices SET current_case_id=NULL WHERE deviceid='DEV-00000001'`)
	if run("seal", "", map[string]any{"case_id": 1, "accept_incomplete_template": true}, 200)["ready_to_execute"] != false {
		t.Fatal("missing native membership accepted")
	}
	exec(`UPDATE devices SET current_case_id=1 WHERE deviceid='DEV-00000001'`)
	before := snapshot()
	incomplete := run("seal", "", map[string]any{"case_id": 1}, 200)
	if incomplete["ready_to_execute"] != false || snapshot() != before {
		t.Fatal("implicit incomplete seal", incomplete)
	}
	seal := confirm("seal", map[string]any{"case_id": 1, "accept_incomplete_template": true})
	call("seal", "bad-scope", "cores:warehouse:create", seal, 403)
	run("seal", "", seal, 428)
	bad := caseContentCopy(seal)
	bad["confirmation_text"] = "SEAL"
	run("seal", "bad-phrase", bad, 428)
	exec(`UPDATE products SET weight=weight+1 WHERE productid=1`)
	if run("seal", "stale-weight", seal, 200)["ready_to_execute"] != false {
		t.Fatal("stale product ignored")
	}
	seal = confirm("seal", map[string]any{"case_id": 1, "accept_incomplete_template": true})
	done := run("seal", "seal-once", seal, 200)
	if done["operation_status"] != "executed" || count(`SELECT count(*) FROM cases WHERE caseid=1 AND workflow_status='sealed' AND sealed_at IS NOT NULL`) != 1 {
		t.Fatal(done)
	}
	after := snapshot()
	if run("seal", "seal-once", seal, 200)["audit_id"] != done["audit_id"] || snapshot() != after {
		t.Fatal("replay changed state")
	}
	exec(`UPDATE users SET is_admin=false`)
	run("seal", "seal-once", seal, 403)
	exec(`UPDATE users SET is_admin=true`)
	for i := 0; i < 2; i++ {
		if err := EnsureWarehouseCaseWorkflowRetentionSchema(); err != nil {
			t.Fatal(err)
		}
		if snapshot() != after {
			t.Fatal("restart changed state")
		}
	}
	unseal := confirm("unseal", map[string]any{"case_id": 1})
	run("unseal", "open-once", unseal, 200)
	move := confirm("move", map[string]any{"case_id": 1, "destination_zone_id": 3})
	run("move", "move-once", move, 200)
	if count(`SELECT zone_id FROM cases WHERE caseid=1`) != 3 || count(`SELECT count(*) FROM devices WHERE current_case_id IS NOT NULL AND zone_id IS NULL`) != 2 {
		t.Fatal("whole case location inconsistent")
	}
	seal = confirm("seal", map[string]any{"case_id": 1, "accept_incomplete_template": true})
	run("seal", "seal-second", seal, 200)
	// Overlapping position/package/device reservations block dispatch. A changed
	// existing reservation after preview also invalidates the final fingerprint.
	exec(`INSERT INTO job_devices(deviceid,jobid,pack_status) VALUES('DEV-00000001',2,'reserved')`)
	if run("dispatch", "", map[string]any{"case_id": 1, "job_id": 1}, 200)["ready_to_execute"] != false {
		t.Fatal("overlap accepted")
	}
	exec(`UPDATE jobs SET startdate='2027-01-01',enddate='2027-01-02' WHERE jobid=2`)
	dispatch := confirm("dispatch", map[string]any{"case_id": 1, "job_id": 1})
	exec(`INSERT INTO job_edit_sessions VALUES(1,12,CURRENT_TIMESTAMP)`)
	if run("dispatch", "stale-editor", dispatch, 200)["ready_to_execute"] != false {
		t.Fatal("editor ignored")
	}
	exec(`DELETE FROM job_edit_sessions`)
	exec(`INSERT INTO device_components VALUES('DEV-00000001','DEV-00000002')`)
	if run("dispatch", "", map[string]any{"case_id": 1, "job_id": 1}, 200)["ready_to_execute"] != false {
		t.Fatal("split component accepted")
	}
	exec(`DELETE FROM device_components`)
	dispatch = confirm("dispatch", map[string]any{"case_id": 1, "job_id": 1})
	before = snapshot()
	for _, table := range []string{"case_events", "device_movements", "job_history", "audit_log", "warehouse_product_mutation_receipts"} {
		event := "INSERT"
		if table == "warehouse_product_mutation_receipts" {
			event = "UPDATE"
		}
		exec(fmt.Sprintf(`CREATE FUNCTION reject_workflow_%s() RETURNS TRIGGER LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'injected final failure';END $$;CREATE TRIGGER reject_workflow BEFORE %s ON %s FOR EACH ROW EXECUTE FUNCTION reject_workflow_%s()`, table, event, table, table))
		run("dispatch", "dispatch-retry", dispatch, 500)
		if snapshot() != before {
			t.Fatal("partial dispatch after", table)
		}
		exec("DROP TRIGGER reject_workflow ON " + table)
	}
	run("dispatch", "dispatch-retry", dispatch, 200)
	if count(`SELECT count(*) FROM cases WHERE caseid IN(1,2) AND workflow_status='on_job' AND current_job_id=1 AND zone_id IS NULL`) != 2 || count(`SELECT count(*) FROM job_devices WHERE jobid=1 AND pack_status='issued'`) != 2 {
		t.Fatal("incomplete dispatch")
	}
	ret := confirm("return", map[string]any{"case_id": 1, "destination_zone_id": 3, "return_mode": "inspect"})
	run("return", "return-inspect", ret, 200)
	if count(`SELECT count(*) FROM devices WHERE current_case_id IS NOT NULL AND status='return_pending'`) != 2 || count(`SELECT count(*) FROM job_devices WHERE jobid=1 AND pack_status='returned'`) != 2 {
		t.Fatal("return lost assignment")
	}
	if run("inspect_return", "", map[string]any{"case_id": 1}, 200)["ready_to_execute"] != false {
		t.Fatal("implicit inspection accepted")
	}
	exec(`INSERT INTO defect_reports VALUES('DEV-00000003','open')`)
	if run("inspect_return", "", map[string]any{"case_id": 1, "inspection_passed": true}, 200)["ready_to_execute"] != false {
		t.Fatal("unresolved defect accepted")
	}
	exec(`UPDATE defect_reports SET status='closed'`)
	inspect := confirm("inspect_return", map[string]any{"case_id": 1, "inspection_passed": true})
	run("inspect_return", "inspect-once", inspect, 200)
	if count(`SELECT count(*) FROM devices WHERE current_case_id IS NOT NULL AND status='in_storage' AND condition_status='available'`) != 2 {
		t.Fatal("inspection failed")
	}
	seal = confirm("seal", map[string]any{"case_id": 1, "accept_incomplete_template": true})
	run("seal", "seal-third", seal, 200)
	dispatch = confirm("dispatch", map[string]any{"case_id": 1, "job_id": 1})
	run("dispatch", "dispatch-second", dispatch, 200)
	ret = confirm("return", map[string]any{"case_id": 1, "destination_zone_id": 3, "return_mode": "sealed"})
	run("return", "return-sealed", ret, 200)
	if count(`SELECT count(*) FROM cases WHERE caseid=1 AND workflow_status='sealed' AND current_job_id IS NULL`) != 1 || count(`SELECT count(*) FROM devices WHERE current_case_id IS NOT NULL AND status='in_storage'`) != 2 || count(`SELECT stock_quantity FROM products WHERE productid=3`) != 7 {
		t.Fatal("sealed return or stock conservation")
	}
	// Retention denies history rewriting and identity deletion independently of API.
	for _, q := range []string{`DELETE FROM cases WHERE caseid=4`, `DELETE FROM case_events`, `UPDATE case_events SET event_type='changed'`} {
		if _, err := db.Exec(q); err == nil {
			t.Fatal("retention bypass", q)
		}
	}
	native := func(want int) {
		t.Helper()
		r := httptest.NewRequest("DELETE", "/cases/4", nil)
		r = mux.SetURLVars(r, map[string]string{"id": "4"})
		r = r.WithContext(context.WithValue(r.Context(), middleware.UserContextKey, &models.User{UserID: 11, IsActive: true}))
		w := httptest.NewRecorder()
		DeleteCase(w, r)
		if w.Code != want {
			t.Fatalf("native archive %d: %s", w.Code, w.Body.String())
		}
	}
	before = snapshot()
	exec(`CREATE FUNCTION reject_native_archive() RETURNS TRIGGER LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'injected archive audit failure';END $$;CREATE TRIGGER reject_native_archive BEFORE INSERT ON audit_log FOR EACH ROW EXECUTE FUNCTION reject_native_archive()`)
	native(500)
	if snapshot() != before {
		t.Fatal("native archive partial")
	}
	exec(`DROP TRIGGER reject_native_archive ON audit_log`)
	native(200)
	after = snapshot()
	native(200)
	if snapshot() != after {
		t.Fatal("native repeat changed history")
	}
	if count(`SELECT count(*) FROM cases WHERE caseid=4 AND lifecycle_status='archived'`) != 1 || count(`SELECT count(*) FROM inventory_identifiers WHERE entity_type='case' AND entity_key='4' AND active`) != 0 {
		t.Fatal("identity archive failed")
	}
}

const caseWorkflowFixtureSQL = `
ALTER TABLE storage_zones ADD COLUMN process_role TEXT DEFAULT 'storage';UPDATE storage_zones SET process_role='inspection' WHERE zone_id=3;
ALTER TABLE job_devices ADD COLUMN pack_ts TIMESTAMP;ALTER TABLE job_devices ADD CONSTRAINT workflow_job_device UNIQUE(deviceid,jobid);
ALTER TABLE job_package_reservations ADD COLUMN reservation_id BIGSERIAL;ALTER TABLE job_package_reservations ADD COLUMN quantity NUMERIC DEFAULT 1;ALTER TABLE job_package_reservations ADD COLUMN reserved_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP;ALTER TABLE job_package_reservations ADD COLUMN assigned_at TIMESTAMP;ALTER TABLE job_package_reservations ADD COLUMN released_at TIMESTAMP;
ALTER TABLE case_events ADD COLUMN job_id BIGINT;ALTER TABLE case_events ADD COLUMN created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP;
CREATE TABLE job_edit_sessions(job_id INT,user_id INT,last_seen TIMESTAMP);
CREATE TABLE job_history(history_id BIGSERIAL PRIMARY KEY,job_id INT,user_id INT,change_type TEXT,field_name TEXT,old_value TEXT,new_value TEXT,description TEXT,user_agent TEXT);
UPDATE status SET status='confirmed' WHERE statusid=2;UPDATE jobs SET statusid=2,job_code='JOB-'||jobid,startdate='2026-10-04',enddate='2026-10-05';
UPDATE cases SET sealed_at=CURRENT_TIMESTAMP WHERE caseid=3;
`
