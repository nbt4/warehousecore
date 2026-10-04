package handlers

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"github.com/gorilla/mux"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"warehousecore/internal/middleware"
	"warehousecore/internal/models"
	"warehousecore/internal/repository"
)

const warehouseCaseFixtureSQL = `
CREATE TABLE warehouse_schema_migrations(version TEXT PRIMARY KEY);
CREATE TABLE cases(caseid SERIAL PRIMARY KEY,name TEXT,description TEXT,case_type TEXT DEFAULT 'dynamic',case_model_id INT,width NUMERIC,height NUMERIC,depth NUMERIC,weight NUMERIC,max_weight_kg NUMERIC,zone_id INT,home_zone_id INT,barcode TEXT,rfid_tag TEXT,status TEXT DEFAULT 'free',workflow_status TEXT DEFAULT 'empty',current_job_id INT,sealed_at TIMESTAMP,updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP);
CREATE FUNCTION generate_test_case_barcode() RETURNS TRIGGER LANGUAGE plpgsql AS $$ BEGIN IF NEW.barcode IS NULL THEN NEW.barcode:='CAS-'||NEW.caseid::text; END IF; RETURN NEW; END; $$;
CREATE TRIGGER cases_generate_barcode BEFORE INSERT ON cases FOR EACH ROW EXECUTE FUNCTION generate_test_case_barcode();
CREATE TABLE case_models(model_id INT PRIMARY KEY,name TEXT);
INSERT INTO case_models VALUES(1,'Road Case');
CREATE TABLE location_profiles(profile_id INT PRIMARY KEY,allow_cases BOOL);
INSERT INTO location_profiles VALUES(1,true),(2,false);
CREATE TABLE storage_zones(zone_id INT PRIMARY KEY,parent_zone_id INT,is_active BOOL,is_storable BOOL,operational_status TEXT,capacity NUMERIC,profile_id INT);
INSERT INTO storage_zones VALUES(1,NULL,true,true,'available',1,1),(2,NULL,true,true,'blocked',NULL,1),(3,NULL,true,true,'available',NULL,2),(4,99,true,true,'available',NULL,1),(5,6,true,true,'available',NULL,1),(6,5,true,true,'available',NULL,1);
CREATE TABLE inventory_identifiers(entity_type TEXT,entity_key TEXT,identifier_kind TEXT,code TEXT,active BOOL,UNIQUE(entity_type,entity_key,identifier_kind));
CREATE UNIQUE INDEX active_codes ON inventory_identifiers(lower(trim(code))) WHERE active;
CREATE TABLE devices(deviceid TEXT,barcode TEXT,qr_code TEXT,current_case_id INT,zone_id INT,status TEXT,lifecycle_status TEXT);
CREATE TABLE devicescases(deviceid TEXT,caseid INT);
CREATE TABLE case_product_contents(case_id INT,product_id INT,quantity NUMERIC);
CREATE TABLE case_child_contents(parent_case_id INT,child_case_id INT);
CREATE TABLE case_content_templates(case_id INT,product_id INT,expected_quantity NUMERIC,lifecycle_status TEXT DEFAULT 'active');
CREATE TABLE warehouse_tasks(case_id INT,status TEXT);
CREATE TABLE jobs(jobid INT,statusid INT,deleted_at TIMESTAMP);
CREATE TABLE status(statusid INT,status TEXT);
INSERT INTO status VALUES(1,'open'),(2,'closed');
INSERT INTO jobs VALUES(1,1,NULL),(2,2,NULL);
CREATE FUNCTION warehouse_job_status_is_closed(TEXT) RETURNS BOOLEAN LANGUAGE SQL AS $$ SELECT $1='closed' $$;
CREATE TABLE product_locations(zone_id INT,quantity NUMERIC);
CREATE TABLE products(productid INT,lifecycle_status TEXT);
INSERT INTO products VALUES(1,'active'),(2,'archived');
CREATE TABLE audit_log(id BIGSERIAL PRIMARY KEY,user_id BIGINT,action TEXT,entity_type TEXT,entity_id TEXT,old_values JSONB,new_values JSONB,user_agent TEXT,timestamp TIMESTAMP DEFAULT CURRENT_TIMESTAMP);
CREATE TABLE warehouse_product_mutation_receipts(id BIGSERIAL PRIMARY KEY,user_id BIGINT,operation TEXT,key_hash TEXT,request_hash TEXT,response JSONB DEFAULT '{}'::jsonb,status_code INT DEFAULT 200,UNIQUE(user_id,operation,key_hash));
`

func TestWarehouseCaseMCPAtomicLifecycle(t *testing.T) {
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
	exec(`DROP SCHEMA IF EXISTS warehouse_case_mcp_test CASCADE;CREATE SCHEMA warehouse_case_mcp_test;SET search_path TO warehouse_case_mcp_test`)
	defer db.Exec(`DROP SCHEMA warehouse_case_mcp_test CASCADE`)
	exec(warehouseCaseFixtureSQL)
	old := repository.DB
	repository.DB = db
	defer func() { repository.DB = old }()
	for i := 0; i < 2; i++ {
		if err := EnsureWarehouseCaseVersionSchema(); err != nil {
			t.Fatal(err)
		}
	}
	run := func(op, key string, in map[string]any, admin bool) (int, map[string]any) {
		t.Helper()
		raw, _ := json.Marshal(in)
		r := httptest.NewRequest(http.MethodPost, "/case", bytes.NewReader(raw))
		r = mux.SetURLVars(r, map[string]string{"operation": op})
		r = r.WithContext(context.WithValue(r.Context(), middleware.UserContextKey, &models.User{UserID: 11, IsAdmin: admin}))
		r.Header.Set("X-Cores-Origin", "MCP/AI")
		r.Header.Set("Idempotency-Key", key)
		w := httptest.NewRecorder()
		CaseMCP(w, r)
		var out map[string]any
		if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
			t.Fatal(err)
		}
		return w.Code, out
	}
	want := func(code int, out map[string]any, status int) {
		t.Helper()
		if code != status {
			t.Fatalf("status %d want %d: %#v", code, status, out)
		}
	}
	call := func(op, key string, in map[string]any, status int) map[string]any {
		t.Helper()
		code, out := run(op, key, in, true)
		want(code, out, status)
		return out
	}
	count := func(table string) int {
		t.Helper()
		var n int
		if err := db.QueryRow(`SELECT count(*) FROM ` + table).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	version := func() string {
		t.Helper()
		var v string
		if err := db.QueryRow(`SELECT to_char(updated_at,'YYYY-MM-DD"T"HH24:MI:SS.US"Z"') FROM cases WHERE caseid=1`).Scan(&v); err != nil {
			t.Fatal(err)
		}
		return v
	}
	body := func(op string) map[string]any {
		return map[string]any{"case_id": 1, "expected_updated_at": version(), "confirm_change": true, "confirmation_text": strings.ToUpper(op) + " WAREHOUSE CASE 1"}
	}
	code, out := run("create", "case-no-admin", map[string]any{"name": "Road case"}, false)
	want(code, out, 403)
	preview := call("create", "", map[string]any{"name": "Road case", "case_model_id": 1, "zone_id": 1, "preview": true, "confirm_change": true}, 200)
	if preview["ready_to_execute"] != true || count("cases") != 0 || count("audit_log") != 0 || count("warehouse_product_mutation_receipts") != 0 {
		t.Fatal(preview)
	}
	call("create", "", map[string]any{"name": "Road case", "confirm_change": true}, 428)
	for _, zone := range []int{2, 3, 4, 5} {
		out := call("create", fmt.Sprintf("case-zone-%d", zone), map[string]any{"name": "Road case", "zone_id": zone, "confirm_change": true}, 200)
		if out["ready_to_execute"] != false || count("cases") != 0 {
			t.Fatal(out)
		}
	}
	initial := map[string]any{"name": "Road case", "case_model_id": 1, "zone_id": 1, "width": 80, "weight": 20, "max_weight_kg": 100, "description": "private note", "confirm_change": true}
	created := call("create", "case-create-1", initial, 200)
	if created["operation_status"] != "executed" || count("cases") != 1 || count("audit_log") != 1 {
		t.Fatal(created)
	}
	replay := call("create", "case-create-1", initial, 200)
	if replay["audit_id"] != created["audit_id"] || count("cases") != 1 {
		t.Fatal("durable replay failed", replay)
	}
	initial["name"] = "Changed payload"
	call("create", "case-create-1", initial, 409)
	stale := body("update")
	stale["name"] = "Road renamed"
	exec(`UPDATE cases SET description='UI update' WHERE caseid=1`)
	out = call("update", "case-stale", stale, 200)
	if out["ready_to_execute"] != false {
		t.Fatal("UI version ignored")
	}
	change := body("update")
	change["name"] = "Road renamed"
	change["clear_fields"] = []string{"description"}
	changed := call("update", "case-update-1", change, 200)
	fields := changed["case"].(map[string]any)
	if fields["description"] != nil || fields["width"] != float64(80) || fields["barcode"] != "CAS-1" {
		t.Fatal("nullable patch failed", fields)
	}
	out = call("update", "case-clear-code", map[string]any{"case_id": 1, "expected_updated_at": version(), "barcode": "", "confirm_change": true}, 200)
	if out["ready_to_execute"] != false {
		t.Fatal("barcode cleared")
	}
	// Every kind of newly-created active dependency blocks a later commit.
	for i, dep := range []struct{ insert, clear string }{
		{`INSERT INTO devicescases VALUES('D',1)`, `DELETE FROM devicescases`},
		{`INSERT INTO devices VALUES('D',NULL,NULL,1,NULL,NULL,NULL)`, `DELETE FROM devices`},
		{`INSERT INTO case_product_contents VALUES(1,1,1)`, `DELETE FROM case_product_contents`},
		{`INSERT INTO cases(name) VALUES('Nested'); INSERT INTO case_child_contents VALUES(1,2)`, `DELETE FROM case_child_contents;DELETE FROM cases WHERE caseid=2`},
		{`INSERT INTO cases(caseid,name) VALUES(2,'Parent'); INSERT INTO case_child_contents VALUES(2,1)`, `DELETE FROM case_child_contents;DELETE FROM cases WHERE caseid=2`},
		{`INSERT INTO warehouse_tasks VALUES(1,NULL)`, `DELETE FROM warehouse_tasks`},
		{`UPDATE cases SET current_job_id=1 WHERE caseid=1`, `UPDATE cases SET current_job_id=NULL WHERE caseid=1`},
		{`UPDATE cases SET workflow_status='packing' WHERE caseid=1`, `UPDATE cases SET workflow_status='empty' WHERE caseid=1`},
	} {
		exec(dep.insert)
		out = call("archive", fmt.Sprintf("case-dependency-%d", i), body("archive"), 200)
		if out["ready_to_execute"] != false {
			t.Fatal("dependency accepted", i, out)
		}
		exec(dep.clear)
	}
	// Templates remain but restore must validate their references.
	exec(`INSERT INTO case_content_templates(case_id,product_id,expected_quantity) VALUES(1,1,2)`)
	archive := body("archive")
	noPhrase := body("archive")
	noPhrase["confirmation_text"] = "wrong"
	call("archive", "case-wrong-phrase", noPhrase, 428)
	archived := call("archive", "case-archive-1", archive, 200)
	if archived["case"].(map[string]any)["lifecycle_status"] != "archived" {
		t.Fatal(archived)
	}
	if _, err := db.Exec(`INSERT INTO devicescases VALUES('illegal',1)`); err == nil {
		t.Fatal("archived case packed via UI")
	}
	var active bool
	if err := db.QueryRow(`SELECT active FROM inventory_identifiers WHERE entity_type='case' AND entity_key='1'`).Scan(&active); err != nil || active {
		t.Fatal("archived scanner identity still active", err)
	}
	exec(`UPDATE products SET lifecycle_status='archived' WHERE productid=1`)
	out = call("restore", "case-bad-template", body("restore"), 200)
	if out["ready_to_execute"] != false {
		t.Fatal("invalid template restored")
	}
	exec(`UPDATE products SET lifecycle_status='active' WHERE productid=1`)
	restore := body("restore")
	restored := call("restore", "case-restore-1", restore, 200)
	if restored["case"].(map[string]any)["lifecycle_status"] != "active" || count("case_content_templates") != 1 {
		t.Fatal(restored)
	}
	call("archive", "case-archive-1", archive, 200)
	call("restore", "case-restore-1", restore, 200)
	if err := EnsureWarehouseCaseVersionSchema(); err != nil {
		t.Fatal("restart failed", err)
	}
	// Audit failure rolls back the complete mutation and its durable receipt.
	before := version()
	exec(`CREATE FUNCTION reject_case_audit() RETURNS TRIGGER LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'test audit failure'; END; $$;CREATE TRIGGER reject_case_audit BEFORE INSERT ON audit_log FOR EACH ROW EXECUTE FUNCTION reject_case_audit()`)
	change = body("update")
	change["name"] = "Must rollback"
	call("update", "case-audit-failure", change, 500)
	if version() != before {
		t.Fatal("audit failure changed case")
	}
	var n int
	if err := db.QueryRow(`SELECT count(*) FROM warehouse_product_mutation_receipts WHERE operation='case.update'`).Scan(&n); err != nil || n != 1 {
		t.Fatal("failed receipt committed", n, err)
	}
}
