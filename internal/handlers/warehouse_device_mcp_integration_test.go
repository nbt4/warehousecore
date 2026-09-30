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

	"github.com/gorilla/mux"
	"warehousecore/internal/middleware"
	"warehousecore/internal/models"
	"warehousecore/internal/repository"
)

func TestWarehouseDeviceMCPAtomicWorkflows(t *testing.T) {
	dsn := os.Getenv("WAREHOUSE_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("disposable _test database required")
	}
	parsed, err := url.Parse(dsn)
	if err != nil || !strings.HasSuffix(parsed.Path, "_test") {
		t.Fatal("dedicated _test database required")
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	exec := func(q string, args ...any) {
		t.Helper()
		if _, err := db.Exec(q, args...); err != nil {
			t.Fatal(err)
		}
	}
	exec("DROP SCHEMA IF EXISTS warehouse_device_mcp_test CASCADE; CREATE SCHEMA warehouse_device_mcp_test; SET search_path TO warehouse_device_mcp_test")
	defer db.Exec("DROP SCHEMA warehouse_device_mcp_test CASCADE")
	exec(warehouseDeviceFixtureSQL)
	old := repository.DB
	repository.DB = db
	defer func() { repository.DB = old }()
	for i := 0; i < 2; i++ {
		if err := EnsureWarehouseDeviceVersionSchema(); err != nil {
			t.Fatal(err)
		}
	}
	request := func(op, id, key string, body map[string]any, user uint, admin bool) *httptest.ResponseRecorder {
		t.Helper()
		raw, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		r := httptest.NewRequest(http.MethodPost, "/device", bytes.NewReader(raw))
		r = mux.SetURLVars(r, map[string]string{"id": id})
		r = r.WithContext(context.WithValue(r.Context(), middleware.UserContextKey, &models.User{UserID: user, IsAdmin: admin, Username: "tester"}))
		r.Header.Set("X-Cores-Origin", "MCP/AI")
		if key != "" {
			key = "test-device-" + key
		}
		r.Header.Set("Idempotency-Key", key)
		w := httptest.NewRecorder()
		switch op {
		case "create":
			CreateDevice(w, r)
		case "update":
			UpdateDevice(w, r)
		case "archive":
			DeleteDevice(w, r)
		case "restore":
			RestoreDevice(w, r)
		case "revert_update":
			RevertDeviceUpdateMCP(w, r)
		default:
			t.Fatal(op)
		}
		return w
	}
	status := func(w *httptest.ResponseRecorder, want int) {
		t.Helper()
		if w.Code != want {
			t.Fatalf("status %d want %d: %s", w.Code, want, w.Body.String())
		}
	}
	result := func(w *httptest.ResponseRecorder) map[string]any {
		t.Helper()
		var out map[string]any
		if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
			t.Fatal(err)
		}
		return out
	}
	count := func(q string) int {
		t.Helper()
		var n int
		if err := db.QueryRow(q).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	version := func(id string) string {
		t.Helper()
		var v string
		if err := db.QueryRow(`SELECT to_char(updated_at,'YYYY-MM-DD"T"HH24:MI:SS.US"Z"') FROM devices WHERE deviceid=$1`, id).Scan(&v); err != nil {
			t.Fatal(err)
		}
		return v
	}
	clone := func(in map[string]any) map[string]any {
		out := map[string]any{}
		for k, v := range in {
			out[k] = v
		}
		return out
	}
	base := map[string]any{"product_id": 1, "serial_number": " Serial-A ", "condition_rating": 4.5, "usage_hours": 12.34, "purchase_date": "2026-01-10", "last_maintenance": "2026-09-01", "next_maintenance": "2027-01-01", "notes": "private notes", "zone_id": 1, "condition_status": "defective"}
	status(request("create", "", "denied", base, 11, false), 403)
	status(request("create", "", "", base, 11, true), 428)
	for _, tc := range []struct {
		field string
		value any
		want  int
	}{{"product_id", 3, 409}, {"product_id", 4, 409}, {"product_id", 999, 404}, {"condition_rating", 4.55, 400}, {"usage_hours", 1.234, 400}, {"purchase_date", "2026-02-30", 400}, {"next_maintenance", "2020-01-01", 400}, {"zone_id", 2, 409}, {"zone_id", 3, 409}, {"condition_status", "on_job", 400}} {
		bad := clone(base)
		bad[tc.field] = tc.value
		status(request("create", "", "invalid-"+tc.field, bad, 11, true), tc.want)
	}
	created := request("create", "", "device-create", base, 11, true)
	status(created, 201)
	c := result(created)
	id := c["device_id"].(string)
	if c["physical_status"] != "in_storage" || c["condition_status"] != "defective" || id == "" {
		t.Fatalf("bad create: %#v", c)
	}
	status(request("create", "", "device-create", base, 11, true), 201)
	if count("SELECT count(*) FROM devices") != 1 || count("SELECT count(*) FROM audit_log") != 1 {
		t.Fatal("duplicate replay")
	}
	bad := clone(base)
	bad["serial_number"] = "Another"
	status(request("create", "", "capacity", bad, 11, true), 409)
	fields := c["fields"].(map[string]any)
	update := clone(fields)
	update["notes"] = "changed private notes"
	update["expected_updated_at"] = version(id)
	status(request("update", id, "device-update", update, 11, true), 200)
	status(request("update", id, "device-update", update, 11, true), 200)
	if count("SELECT count(*) FROM audit_log WHERE action='device.update'") != 1 {
		t.Fatal("update replay duplicated audit")
	}
	status(request("update", id, "stale", update, 11, true), 409)
	var auditID int64
	db.QueryRow("SELECT max(id) FROM audit_log").Scan(&auditID)
	revert := func() map[string]any {
		return map[string]any{"audit_id": auditID, "expected_updated_at": version(id), "confirm_revert": true, "confirmation_text": deviceRevertPhrase(id, auditID)}
	}
	status(request("revert_update", id, "wrong-actor", revert(), 12, true), 409)
	exec(`UPDATE audit_log SET new_values=jsonb_set(new_values,'{origin}','"UI"') WHERE id=$1`, auditID)
	status(request("revert_update", id, "wrong-origin", revert(), 11, true), 409)
	exec(`UPDATE audit_log SET new_values=jsonb_set(new_values,'{origin}','"MCP/AI"') WHERE id=$1`, auditID)
	status(request("revert_update", id, "device-revert", revert(), 11, true), 200)
	var notes string
	db.QueryRow("SELECT notes FROM devices WHERE deviceid=$1", id).Scan(&notes)
	if notes != "private notes" {
		t.Fatal("revert failed")
	}
	status(request("revert_update", id, "already-reverted", revert(), 11, true), 409)
	lifecycle := func(op string) map[string]any {
		return map[string]any{"expected_updated_at": version(id), "confirm_lifecycle": true, "confirmation_text": strings.ToUpper(op) + " WAREHOUSE DEVICE " + id}
	}
	// Every active relation independently prevents archival.
	for i, tc := range []struct{ table, insert string }{{"job_devices", `INSERT INTO job_devices VALUES($1,1,'pending')`}, {"job_position_devices", `INSERT INTO job_position_devices VALUES($1,1)`}, {"job_package_reservations", `INSERT INTO job_package_reservations VALUES($1,'reserved')`}, {"devicescases", `INSERT INTO devicescases VALUES($1)`}, {"device_components", `INSERT INTO device_components VALUES($1,'other')`}, {"warehouse_tasks", `INSERT INTO warehouse_tasks VALUES($1,'open')`}, {"maintenance_orders", `INSERT INTO maintenance_orders VALUES($1,'scheduled')`}, {"maintenance_plans", `INSERT INTO maintenance_plans VALUES($1,true)`}, {"defect_reports", `INSERT INTO defect_reports VALUES($1,'open')`}} {
		exec(tc.insert, id)
		status(request("archive", id, fmt.Sprint("dependency-", i), lifecycle("archive"), 11, true), 409)
		identity := clone(fields)
		identity["serial_number"] = "IdentityChange"
		identity["expected_updated_at"] = version(id)
		status(request("update", id, fmt.Sprint("identity-dependency-", i), identity, 11, true), 409)
		exec("DELETE FROM " + tc.table)
	}
	exec(`INSERT INTO job_devices VALUES($1,2,'issued')`, id)
	status(request("archive", id, "closed-issued", lifecycle("archive"), 11, true), 409)
	exec(`UPDATE job_devices SET pack_status='returned'`)
	product := clone(fields)
	product["product_id"] = 2
	product["expected_updated_at"] = version(id)
	status(request("update", id, "history", product, 11, true), 409)
	exec(`INSERT INTO inventory_identifiers VALUES('device',$1,'alias','CustomAlias',true)`, id)
	archive := lifecycle("archive")
	status(request("archive", id, "missing-phrase", map[string]any{"expected_updated_at": version(id)}, 11, true), 428)
	status(request("archive", id, "device-archive", archive, 11, true), 200)
	status(request("archive", id, "device-archive", archive, 11, true), 200)
	if count("SELECT count(*) FROM inventory_identifiers WHERE active") != 0 {
		t.Fatal("archive kept scans active")
	}
	bad = clone(base)
	delete(bad, "zone_id")
	bad["serial_number"] = "serial-a"
	status(request("create", "", "archived-serial", bad, 11, true), 409)
	bad["serial_number"] = "Serial-B"
	bad["barcode"] = id
	status(request("create", "", "archived-code", bad, 11, true), 409)
	exec(`INSERT INTO inventory_identifiers VALUES('case','other','canonical','customalias',true)`)
	status(request("restore", id, "restore-alias-conflict", lifecycle("restore"), 11, true), 409)
	exec(`DELETE FROM inventory_identifiers WHERE entity_type='case'`)
	exec(`INSERT INTO cases VALUES(1,1)`)
	status(request("restore", id, "restore-full", lifecycle("restore"), 11, true), 409)
	exec(`DELETE FROM cases`)
	exec(`UPDATE products SET lifecycle_status='archived' WHERE productid=1`)
	status(request("restore", id, "restore-product", lifecycle("restore"), 11, true), 409)
	exec(`UPDATE products SET lifecycle_status='active' WHERE productid=1`)
	restore := lifecycle("restore")
	status(request("restore", id, "device-restore", restore, 11, true), 200)
	status(request("restore", id, "device-restore", restore, 11, true), 200)
	if count("SELECT count(*) FROM inventory_identifiers WHERE active") != 2 {
		t.Fatal("restore missing scans")
	}
	var condition, physical string
	db.QueryRow("SELECT condition_status,status FROM devices WHERE deviceid=$1", id).Scan(&condition, &physical)
	if condition != "defective" || physical != "in_storage" {
		t.Fatal("lifecycle changed condition/location")
	}
	// Legacy writers invalidate preview and eligibility even without an audit.
	update = clone(fields)
	update["notes"] = "latest"
	update["expected_updated_at"] = version(id)
	status(request("update", id, "second-update", update, 11, true), 200)
	db.QueryRow("SELECT max(id) FROM audit_log").Scan(&auditID)
	oldVersion := version(id)
	exec(`UPDATE devices SET usage_hours=usage_hours+1 WHERE deviceid=$1`, id)
	if oldVersion == version(id) {
		t.Fatal("legacy write did not bump version")
	}
	status(request("revert_update", id, "legacy-block", revert(), 11, true), 409)
	// Audit failure rolls back create/update/archive/restore/revert and receipts.
	exec(`CREATE FUNCTION reject_device_audit() RETURNS TRIGGER LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'audit unavailable'; END $$; CREATE TRIGGER reject_device_audit BEFORE INSERT ON audit_log FOR EACH ROW EXECUTE FUNCTION reject_device_audit()`)
	receipts := count("SELECT count(*) FROM warehouse_product_mutation_receipts")
	devices := count("SELECT count(*) FROM devices")
	oldVersion = version(id)
	fresh := map[string]any{"product_id": 1, "serial_number": "rollback"}
	status(request("create", "", "rollback-create", fresh, 11, true), 500)
	update = clone(fields)
	update["notes"] = "rollback"
	update["expected_updated_at"] = version(id)
	status(request("update", id, "rollback-update", update, 11, true), 500)
	status(request("archive", id, "rollback-archive", lifecycle("archive"), 11, true), 500)
	if version(id) != oldVersion || count("SELECT count(*) FROM devices") != devices || count("SELECT count(*) FROM warehouse_product_mutation_receipts") != receipts {
		t.Fatal("failed audit leaked data or receipts")
	}
	exec(`DROP TRIGGER reject_device_audit ON audit_log`)
	// Changing scan code and then reverting to the ID must not collide with own legacy alias.
	update = clone(fields)
	update["barcode"] = "ChangedBarcode"
	update["expected_updated_at"] = version(id)
	status(request("update", id, "code-change", update, 11, true), 200)
	db.QueryRow("SELECT max(id) FROM audit_log").Scan(&auditID)
	v := version(id)
	exec(`CREATE TRIGGER reject_device_audit BEFORE INSERT ON audit_log FOR EACH ROW EXECUTE FUNCTION reject_device_audit()`)
	status(request("revert_update", id, "rollback-revert", revert(), 11, true), 500)
	if version(id) != v {
		t.Fatal("failed revert changed device")
	}
	exec(`DROP TRIGGER reject_device_audit ON audit_log`)
	rr := revert()
	status(request("revert_update", id, "code-revert", rr, 11, true), 200)
	status(request("revert_update", id, "code-revert", rr, 11, true), 200)
	status(request("archive", id, "final-archive", lifecycle("archive"), 11, true), 200)
	v = version(id)
	exec(`CREATE TRIGGER reject_device_audit BEFORE INSERT ON audit_log FOR EACH ROW EXECUTE FUNCTION reject_device_audit()`)
	status(request("restore", id, "rollback-restore", lifecycle("restore"), 11, true), 500)
	if version(id) != v || count("SELECT count(*) FROM inventory_identifiers WHERE active") != 0 {
		t.Fatal("failed restore leaked state")
	}
}
