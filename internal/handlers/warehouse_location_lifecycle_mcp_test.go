package handlers

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"github.com/gorilla/mux"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strconv"
	"strings"
	"testing"
	"warehousecore/internal/middleware"
	"warehousecore/internal/models"
	"warehousecore/internal/repository"
)

func TestWarehouseLocationMCPLifecycle(t *testing.T) {
	dsn := os.Getenv("WAREHOUSE_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("dedicated _test database required")
	}
	parsed, err := url.Parse(dsn)
	if err != nil || !strings.HasSuffix(strings.TrimPrefix(parsed.Path, "/"), "_test") {
		t.Fatal("dedicated _test database required")
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	const schema = "warehouse_location_lifecycle_test"
	if _, err = db.Exec("DROP SCHEMA IF EXISTS " + schema + " CASCADE;CREATE SCHEMA " + schema + ";SET search_path TO " + schema); err != nil {
		t.Fatal(err)
	}
	defer db.Exec("DROP SCHEMA IF EXISTS " + schema + " CASCADE")
	exec := func(q string, args ...any) {
		t.Helper()
		if _, err := db.Exec(q, args...); err != nil {
			t.Fatal(err)
		}
	}
	exec(`CREATE TABLE warehouse_schema_migrations(version TEXT PRIMARY KEY);
CREATE TABLE storage_zones(zone_id SERIAL PRIMARY KEY,code TEXT,barcode TEXT,name TEXT,type TEXT,description TEXT,parent_zone_id INT,capacity NUMERIC,is_active BOOLEAN DEFAULT true,location_kind TEXT DEFAULT 'area',process_role TEXT DEFAULT 'storage',operational_status TEXT DEFAULT 'available',is_storable BOOLEAN DEFAULT true,pick_sequence INT,capacity_mode TEXT DEFAULT 'item_count',max_weight_kg NUMERIC,max_volume_m3 NUMERIC,inventory_frequency_days INT,next_count_at TIMESTAMP,last_counted_at TIMESTAMP,profile_id BIGINT,updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP);
CREATE TABLE devices(zone_id INT,lifecycle_status TEXT,status TEXT);
CREATE TABLE cases(zone_id INT,home_zone_id INT);
CREATE TABLE product_locations(zone_id INT,quantity NUMERIC);
CREATE TABLE warehouse_tasks(from_zone_id INT,to_zone_id INT,status TEXT);
CREATE TABLE inventory_counts(zone_id INT,status TEXT);
CREATE TABLE audit_log(id BIGSERIAL PRIMARY KEY,user_id BIGINT,action TEXT,entity_type TEXT,entity_id TEXT,old_values JSONB,new_values JSONB,user_agent TEXT,timestamp TIMESTAMP DEFAULT CURRENT_TIMESTAMP);
CREATE TABLE warehouse_product_mutation_receipts(id BIGSERIAL PRIMARY KEY,user_id BIGINT NOT NULL,operation VARCHAR(80) NOT NULL,key_hash CHAR(64) NOT NULL,request_hash CHAR(64) NOT NULL,response JSONB NOT NULL DEFAULT '{}'::jsonb,status_code INTEGER NOT NULL DEFAULT 200,created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,UNIQUE(user_id,operation,key_hash));
INSERT INTO storage_zones(code,barcode,name,type,description,inventory_frequency_days,last_counted_at,next_count_at,profile_id) VALUES('LIFE','LOC-LIFE','Lifecycle Fixture','shelf','private description',14,'2026-09-01','2026-09-15',1);
`)
	oldDB := repository.DB
	repository.DB = db
	defer func() { repository.DB = oldDB }()
	if err = EnsureWarehouseLocationVersionSchema(); err != nil {
		t.Fatal(err)
	}
	request := func(op, key string, body map[string]any, admin bool) *httptest.ResponseRecorder {
		t.Helper()
		raw, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		r := httptest.NewRequest(http.MethodPost, "/location", bytes.NewReader(raw))
		r = mux.SetURLVars(r, map[string]string{"id": "1"})
		r = r.WithContext(context.WithValue(r.Context(), middleware.UserContextKey, &models.User{UserID: 11, IsAdmin: admin}))
		r.Header.Set("X-Cores-Origin", "MCP/AI")
		if key != "" {
			key = "location-life-" + key
		}
		r.Header.Set("Idempotency-Key", key)
		w := httptest.NewRecorder()
		if op == "restore" {
			RestoreWarehouseLocationMCP(w, r)
		} else if op == "public" {
			ArchiveWarehouseLocation(w, r)
		} else {
			ArchiveWarehouseLocationMCP(w, r)
		}
		return w
	}
	status := func(w *httptest.ResponseRecorder, want int) {
		t.Helper()
		if w.Code != want {
			t.Fatalf("status %d want %d: %s", w.Code, want, w.Body.String())
		}
	}
	version := func() string {
		t.Helper()
		var v string
		if err := db.QueryRow(`SELECT to_char(updated_at,'YYYY-MM-DD"T"HH24:MI:SS.US"Z"') FROM storage_zones WHERE zone_id=1`).Scan(&v); err != nil {
			t.Fatal(err)
		}
		return v
	}
	body := func(op string) map[string]any {
		return map[string]any{"expected_updated_at": version(), "confirm_lifecycle": true, "confirmation_text": strings.ToUpper(op) + " WAREHOUSE LOCATION 1"}
	}
	count := func(q string) int {
		t.Helper()
		var n int
		if err := db.QueryRow(q).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	status(request("archive", "no-admin", body("archive"), false), 403)
	status(request("archive", "", body("archive"), true), 428)
	status(request("archive", "no-confirm", map[string]any{"expected_updated_at": version()}, true), 428)
	status(request("public", "public-admin", body("archive"), false), 403)
	stale := body("archive")
	stale["expected_updated_at"] = "stale"
	status(request("archive", "stale", stale, true), 409)
	// Every reference is checked again by the owner, including unknown statuses and cancelling sums.
	for i, dependency := range []struct{ insert, clear string }{
		{`INSERT INTO devices VALUES(1,'active','rented')`, `DELETE FROM devices`},
		{`INSERT INTO cases VALUES(1,NULL)`, `DELETE FROM cases`},
		{`INSERT INTO cases VALUES(NULL,1)`, `DELETE FROM cases`},
		{`INSERT INTO product_locations VALUES(1,1),(1,-1)`, `DELETE FROM product_locations`},
		{`INSERT INTO warehouse_tasks VALUES(1,NULL,'open')`, `DELETE FROM warehouse_tasks`},
		{`INSERT INTO warehouse_tasks VALUES(NULL,1,NULL)`, `DELETE FROM warehouse_tasks`},
		{`INSERT INTO inventory_counts VALUES(1,'review')`, `DELETE FROM inventory_counts`},
		{`INSERT INTO inventory_counts VALUES(1,NULL)`, `DELETE FROM inventory_counts`},
		{`INSERT INTO storage_zones(zone_id,code,barcode,name,type,parent_zone_id,is_active) VALUES(2,'P','P','Parent','rack',1,false),(3,'C','C','Child','shelf',2,true)`, `DELETE FROM storage_zones WHERE zone_id IN (2,3)`},
	} {
		exec(dependency.insert)
		status(request("archive", "dep-"+strconv.Itoa(i), body("archive"), true), 409)
		exec(dependency.clear)
	}
	exec(`UPDATE storage_zones SET operational_status='counting' WHERE zone_id=1`)
	status(request("archive", "counting", body("archive"), true), 409)
	exec(`UPDATE storage_zones SET operational_status='blocked' WHERE zone_id=1;INSERT INTO devices VALUES(1,'archived','rented');INSERT INTO product_locations VALUES(1,0);INSERT INTO warehouse_tasks VALUES(1,NULL,'done');INSERT INTO inventory_counts VALUES(1,'approved')`)
	original := body("archive")
	status(request("archive", "archive", original, true), 200)
	status(request("archive", "archive", original, true), 200)
	if count(`SELECT count(*) FROM audit_log WHERE action='storage_zone.archive'`) != 1 {
		t.Fatal("duplicate audit")
	}
	status(request("archive", "same-state", body("archive"), true), 409)
	// Inactive parent, missing parent, cycle and identity conflict block restoring.
	exec(`INSERT INTO storage_zones(zone_id,code,barcode,name,type,is_active) VALUES(2,'P','P','Parent','rack',false);UPDATE storage_zones SET parent_zone_id=2 WHERE zone_id=1`)
	status(request("restore", "parent-inactive", body("restore"), true), 409)
	exec(`UPDATE storage_zones SET parent_zone_id=99 WHERE zone_id=1`)
	status(request("restore", "parent-missing", body("restore"), true), 409)
	exec(`UPDATE storage_zones SET parent_zone_id=1 WHERE zone_id=1`)
	status(request("restore", "cycle", body("restore"), true), 409)
	exec(`UPDATE storage_zones SET parent_zone_id=NULL WHERE zone_id=1;UPDATE storage_zones SET barcode='LOC-LIFE' WHERE zone_id=2`)
	status(request("restore", "barcode", body("restore"), true), 409)
	exec(`UPDATE storage_zones SET barcode='P',is_active=true,parent_zone_id=3 WHERE zone_id=2;INSERT INTO storage_zones(zone_id,code,barcode,name,type,parent_zone_id,is_active) VALUES(3,'C','C','Cycle','rack',2,true);UPDATE storage_zones SET parent_zone_id=2 WHERE zone_id=1`)
	status(request("restore", "ancestor-cycle", body("restore"), true), 409)
	exec(`UPDATE storage_zones SET parent_zone_id=99 WHERE zone_id=2`)
	status(request("restore", "ancestor-missing", body("restore"), true), 409)
	exec(`UPDATE storage_zones SET parent_zone_id=NULL WHERE zone_id=1;DELETE FROM storage_zones WHERE zone_id=3`)
	exec(`DELETE FROM storage_zones WHERE zone_id=2;INSERT INTO warehouse_tasks VALUES(1,NULL,'open')`)
	status(request("restore", "restore-dep", body("restore"), true), 409)
	exec(`DELETE FROM warehouse_tasks WHERE status='open'`)
	restore := body("restore")
	status(request("restore", "restore", restore, true), 200)
	status(request("restore", "restore", restore, true), 200)
	var active bool
	var operational string
	db.QueryRow(`SELECT is_active,operational_status FROM storage_zones WHERE zone_id=1`).Scan(&active, &operational)
	if !active || operational != "blocked" {
		t.Fatal("modified archive must restore blocked")
	}
	// An unchanged audit preserves available, blocked and maintenance; legacy archive remains blocked.
	for _, state := range []string{"available", "blocked", "maintenance"} {
		exec(`UPDATE storage_zones SET operational_status=$1 WHERE zone_id=1`, state)
		status(request("archive", "archive-"+state, body("archive"), true), 200)
		status(request("restore", "restore-"+state, body("restore"), true), 200)
		db.QueryRow(`SELECT operational_status FROM storage_zones WHERE zone_id=1`).Scan(&operational)
		if operational != state {
			t.Fatal("restore lost state", state, operational)
		}
	}
	exec(`UPDATE storage_zones SET is_active=false,operational_status='archived' WHERE zone_id=1`)
	status(request("restore", "legacy", body("restore"), true), 200)
	db.QueryRow(`SELECT operational_status FROM storage_zones WHERE zone_id=1`).Scan(&operational)
	if operational != "blocked" {
		t.Fatal("legacy restored available")
	}
	if count(`SELECT count(*) FROM storage_zones WHERE zone_id=1 AND code='LIFE' AND barcode='LOC-LIFE' AND description='private description' AND inventory_frequency_days=14 AND last_counted_at='2026-09-01' AND next_count_at='2026-09-15' AND profile_id=1`) != 1 || count(`SELECT count(*) FROM inventory_counts`) != 1 {
		t.Fatal("metadata or history lost")
	}
	exec(`CREATE FUNCTION reject_location_audit() RETURNS TRIGGER LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'audit unavailable';END $$;CREATE TRIGGER reject_location_audit BEFORE INSERT ON audit_log FOR EACH ROW EXECUTE FUNCTION reject_location_audit()`)
	v, receipts := version(), count(`SELECT count(*) FROM warehouse_product_mutation_receipts`)
	status(request("archive", "rollback", body("archive"), true), 500)
	if version() != v || count(`SELECT count(*) FROM warehouse_product_mutation_receipts`) != receipts {
		t.Fatal("rollback leaked data")
	}
	exec(`DROP TRIGGER reject_location_audit ON audit_log`)
	status(request("archive", "before-restore-rollback", body("archive"), true), 200)
	exec(`CREATE TRIGGER reject_location_audit BEFORE INSERT ON audit_log FOR EACH ROW EXECUTE FUNCTION reject_location_audit()`)
	v, receipts = version(), count(`SELECT count(*) FROM warehouse_product_mutation_receipts`)
	status(request("restore", "restore-rollback", body("restore"), true), 500)
	if version() != v || count(`SELECT count(*) FROM warehouse_product_mutation_receipts`) != receipts {
		t.Fatal("restore rollback leaked data")
	}
}
