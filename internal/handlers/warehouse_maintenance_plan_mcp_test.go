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

func TestWarehouseMaintenancePlansAtomicWorkflows(t *testing.T) {
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
	exec("DROP SCHEMA IF EXISTS warehouse_maintenance_plan_mcp_test CASCADE; CREATE SCHEMA warehouse_maintenance_plan_mcp_test; SET search_path TO warehouse_maintenance_plan_mcp_test")
	defer db.Exec("DROP SCHEMA warehouse_maintenance_plan_mcp_test CASCADE")
	exec(warehouseDeviceFixtureSQL)
	exec(`DROP TABLE maintenance_orders; DROP TABLE maintenance_plans; CREATE TABLE users(userid BIGINT PRIMARY KEY); INSERT INTO users VALUES(11); ALTER TABLE products ADD COLUMN maintenanceinterval INT; INSERT INTO warehouse_schema_migrations VALUES('042_maintenance_work_management'); INSERT INTO devices(productid,serialnumber,status,condition_status) VALUES(1,'PLAN-DEVICE','location_unknown','available')`)
	old := repository.DB
	repository.DB = db
	defer func() { repository.DB = old }()
	for i := 0; i < 2; i++ {
		if err := EnsureMaintenanceSchema(); err != nil {
			t.Fatal(err)
		}
		if err := EnsureWarehouseDeviceVersionSchema(); err != nil {
			t.Fatal(err)
		}
		if err := EnsureWarehouseMaintenanceVersionSchema(); err != nil {
			t.Fatal(err)
		}
	}
	deviceID := "DEV-00000001"
	deviceVersion := func() string {
		t.Helper()
		var v string
		if err := db.QueryRow(`SELECT to_char(updated_at,'YYYY-MM-DD"T"HH24:MI:SS.US"Z"') FROM devices WHERE deviceid=$1`, deviceID).Scan(&v); err != nil {
			t.Fatal(err)
		}
		return v
	}
	planVersion := func(id int64) string {
		t.Helper()
		var v string
		if err := db.QueryRow(`SELECT to_char(updated_at,'YYYY-MM-DD"T"HH24:MI:SS.US"Z"') FROM maintenance_plans WHERE plan_id=$1`, id).Scan(&v); err != nil {
			t.Fatal(err)
		}
		return v
	}
	call := func(op, key string, body map[string]any, admin bool, want int) map[string]any {
		t.Helper()
		raw, _ := json.Marshal(body)
		r := httptest.NewRequest(http.MethodPost, "/plan", bytes.NewReader(raw))
		r = mux.SetURLVars(r, map[string]string{"operation": op})
		r = r.WithContext(context.WithValue(r.Context(), middleware.UserContextKey, &models.User{UserID: 11, IsAdmin: admin}))
		r.Header.Set("X-Cores-Origin", "MCP/AI")
		r.Header.Set("Idempotency-Key", key)
		w := httptest.NewRecorder()
		WarehouseMaintenancePlanMCP(w, r)
		if w.Code != want {
			t.Fatalf("%s status %d want %d: %s", op, w.Code, want, w.Body.String())
		}
		var out map[string]any
		if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
			t.Fatal(err)
		}
		return out
	}
	counts := func(plans, orders, audits, receipts int) {
		t.Helper()
		for table, want := range map[string]int{"maintenance_plans": plans, "maintenance_orders": orders, "audit_log": audits, "warehouse_product_mutation_receipts": receipts} {
			var n int
			db.QueryRow("SELECT count(*) FROM " + table).Scan(&n)
			if n != want {
				t.Fatalf("%s count %d want %d", table, n, want)
			}
		}
	}
	create := map[string]any{"device_id": deviceID, "name": "Inspection A", "interval_days": 365, "lead_time_days": 14, "next_due_at": "2099-01-01", "instructions": "private work instruction"}
	call("create", "", create, false, 403)
	p := call("create", "", create, true, 200)
	if p["ready_to_execute"] != true || p["effects"].(map[string]any)["generate_planned_order"] != false {
		t.Fatal("invalid future plan preview", p)
	}
	counts(0, 0, 0, 0)
	create["confirm_change"] = true
	call("create", "plan-missing-version", create, true, 428)
	create["expected_device_updated_at"] = deviceVersion()
	create["preview"] = true
	call("create", "", create, true, 200)
	counts(0, 0, 0, 0)
	delete(create, "preview")
	call("create", "", create, true, 428)
	c := call("create", "plan-create-once", create, true, 200)
	id := int64(c["plan"].(map[string]any)["plan_id"].(float64))
	counts(1, 0, 2, 1)
	if !reflect.DeepEqual(c, call("create", "plan-create-once", create, true, 200)) {
		t.Fatal("durable create replay changed")
	}
	counts(1, 0, 2, 1)
	if err := EnsureMaintenanceSchema(); err != nil {
		t.Fatal(err)
	}
	counts(1, 0, 2, 1) // no implicit duplicate default plan on restart
	var next string
	db.QueryRow("SELECT to_char(nextmaintenance,'YYYY-MM-DD') FROM devices WHERE deviceid=$1", deviceID).Scan(&next)
	if next != "2099-01-01" {
		t.Fatal("next date not synchronized", next)
	}
	update := map[string]any{"plan_id": id, "instructions": "Revised work", "expected_updated_at": planVersion(id), "expected_device_updated_at": deviceVersion(), "confirm_change": true}
	exec("UPDATE devices SET notes='Concurrent UI edit'")
	p = call("update", "plan-stale-device", update, true, 200)
	if p["ready_to_execute"] != false || !strings.Contains(fmt.Sprint(p["required_fields"]), "expected_device_updated_at") {
		t.Fatal("stale device accepted", p)
	}
	counts(1, 0, 2, 1)
	update["expected_device_updated_at"] = deviceVersion()
	exec("UPDATE maintenance_plans SET lead_time_days=7")
	p = call("update", "plan-stale-plan", update, true, 200)
	if p["ready_to_execute"] != false || !strings.Contains(fmt.Sprint(p["required_fields"]), "expected_updated_at") {
		t.Fatal("stale plan accepted", p)
	}
	update["expected_updated_at"] = planVersion(id)
	call("update", "plan-update-once", update, true, 200)
	counts(1, 0, 4, 2)
	lifecycle := func(op string) map[string]any {
		return map[string]any{"plan_id": id, "expected_updated_at": planVersion(id), "expected_device_updated_at": deviceVersion(), "confirm_change": true, "confirmation_text": fmt.Sprintf("%s WAREHOUSE MAINTENANCE PLAN %d", strings.ToUpper(op), id)}
	}
	a := lifecycle("archive")
	a["confirmation_text"] = "WRONG"
	call("archive", "plan-wrong-phrase", a, true, 428)
	a = lifecycle("archive")
	archived := call("archive", "plan-archive-once", a, true, 200)
	counts(1, 0, 6, 3)
	if archived["plan"].(map[string]any)["is_active"] != false {
		t.Fatal("plan still active")
	}
	var nextDate sql.NullTime
	db.QueryRow("SELECT nextmaintenance FROM devices WHERE deviceid=$1", deviceID).Scan(&nextDate)
	if nextDate.Valid {
		t.Fatal("archived plan still schedules device")
	}
	call("restore", "plan-restore-once", lifecycle("restore"), true, 200)
	counts(1, 0, 8, 4)
	// Due work order, event, device synchronization and every audit roll back together.
	exec(`CREATE FUNCTION reject_plan_audit() RETURNS TRIGGER AS $$ BEGIN IF NEW.entity_type='maintenance_plan' AND NEW.action='maintenance_plan.update' THEN RAISE EXCEPTION 'audit failed'; END IF; RETURN NEW; END $$ LANGUAGE plpgsql; CREATE TRIGGER reject_plan_audit BEFORE INSERT ON audit_log FOR EACH ROW EXECUTE FUNCTION reject_plan_audit()`)
	due := map[string]any{"plan_id": id, "next_due_at": "2000-01-01", "expected_updated_at": planVersion(id), "expected_device_updated_at": deviceVersion(), "confirm_change": true}
	beforeDevice, beforePlan := deviceVersion(), planVersion(id)
	call("update", "plan-due-audit-fail", due, true, 500)
	counts(1, 0, 8, 4)
	if deviceVersion() != beforeDevice || planVersion(id) != beforePlan {
		t.Fatal("failed audit leaked versions")
	}
	var events int
	db.QueryRow("SELECT count(*) FROM maintenance_order_events").Scan(&events)
	if events != 0 {
		t.Fatal("failed audit leaked work events")
	}
	exec("DROP TRIGGER reject_plan_audit ON audit_log")
	completed := call("update", "plan-due-update", due, true, 200)
	counts(1, 1, 11, 5)
	if completed["generated_order"].(map[string]any)["order_id"] == nil {
		t.Fatal("due order not returned")
	}
	p = call("archive", "plan-active-order-blocked", lifecycle("archive"), true, 200)
	if p["ready_to_execute"] != false || !strings.Contains(fmt.Sprint(p["required_fields"]), "open_maintenance_orders") {
		t.Fatal("open work did not block archive", p)
	}
	counts(1, 1, 11, 5)
	exec("UPDATE maintenance_orders SET status='completed'")
	call("archive", "plan-archive-completed", lifecycle("archive"), true, 200)
	counts(1, 1, 13, 6)
	exec("UPDATE devices SET condition_status='retired'")
	p = call("restore", "plan-retired-device", lifecycle("restore"), true, 200)
	if p["ready_to_execute"] != false {
		t.Fatal("retired device restored plan")
	}
	counts(1, 1, 13, 6)
}
