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

func TestWarehouseMasterLifecycleAtomicity(t *testing.T) {
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
	exec("DROP SCHEMA IF EXISTS warehouse_master_lifecycle_test CASCADE; CREATE SCHEMA warehouse_master_lifecycle_test; SET search_path TO warehouse_master_lifecycle_test")
	defer db.Exec("DROP SCHEMA warehouse_master_lifecycle_test CASCADE")
	exec(`CREATE TABLE warehouse_schema_migrations(version TEXT PRIMARY KEY);
 CREATE TABLE manufacturer(manufacturerid SERIAL PRIMARY KEY,name TEXT,website TEXT);
 CREATE TABLE brands(brandid SERIAL PRIMARY KEY,name TEXT,manufacturerid INT REFERENCES manufacturer(manufacturerid));
 CREATE TABLE products(productid SERIAL PRIMARY KEY,name TEXT,manufacturerid INT,brandid INT,lifecycle_status TEXT DEFAULT 'active');
 CREATE TABLE audit_log(id BIGSERIAL PRIMARY KEY,user_id BIGINT,action TEXT,entity_type TEXT,entity_id TEXT,old_values JSONB,new_values JSONB,user_agent TEXT,timestamp TIMESTAMP DEFAULT CURRENT_TIMESTAMP);
 CREATE TABLE warehouse_product_mutation_receipts(id BIGSERIAL PRIMARY KEY,user_id BIGINT,operation TEXT,key_hash TEXT,request_hash TEXT,response JSONB DEFAULT '{}',status_code INT,UNIQUE(user_id,operation,key_hash));
 INSERT INTO manufacturer(name,website) VALUES('Manufacturer A','https://example.test'); INSERT INTO brands(name,manufacturerid) VALUES('Brand A',1); INSERT INTO products(name,manufacturerid,brandid) VALUES('Product A',1,1)`)
	old := repository.DB
	repository.DB = db
	defer func() { repository.DB = old }()
	for i := 0; i < 2; i++ {
		if err := EnsureWarehouseMasterVersionSchema(); err != nil {
			t.Fatal(err)
		}
	}
	version := func(entity string) string {
		t.Helper()
		table, key := "manufacturer", "manufacturerid"
		if entity == "brand" {
			table, key = "brands", "brandid"
		}
		var v string
		if err := db.QueryRow(`SELECT to_char(updated_at,'YYYY-MM-DD"T"HH24:MI:SS.US"Z"') FROM ` + table + ` WHERE ` + key + `=1`).Scan(&v); err != nil {
			t.Fatal(err)
		}
		return v
	}
	call := func(entity, op, key string, body map[string]any, admin bool, want int) map[string]any {
		t.Helper()
		raw, _ := json.Marshal(body)
		r := httptest.NewRequest(http.MethodPost, "/lifecycle", bytes.NewReader(raw))
		r = mux.SetURLVars(r, map[string]string{"entity": entity, "operation": op})
		r = r.WithContext(context.WithValue(r.Context(), middleware.UserContextKey, &models.User{UserID: 11, IsAdmin: admin}))
		r.Header.Set("X-Cores-Origin", "MCP/AI")
		r.Header.Set("Idempotency-Key", key)
		w := httptest.NewRecorder()
		WarehouseMasterLifecycleMCP(w, r)
		if w.Code != want {
			t.Fatalf("%s.%s status %d want %d: %s", entity, op, w.Code, want, w.Body.String())
		}
		var out map[string]any
		if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
			t.Fatal(err)
		}
		return out
	}
	confirmed := func(entity, op string) map[string]any {
		return map[string]any{"id": 1, "expected_updated_at": version(entity), "confirm_lifecycle": true, "confirmation_text": fmt.Sprintf("%s WAREHOUSE %s 1", strings.ToUpper(op), strings.ToUpper(entity))}
	}
	counts := func(audits, receipts int) {
		t.Helper()
		for table, want := range map[string]int{"audit_log": audits, "warehouse_product_mutation_receipts": receipts} {
			var n int
			db.QueryRow("SELECT count(*) FROM " + table).Scan(&n)
			if n != want {
				t.Fatalf("%s count %d want %d", table, n, want)
			}
		}
	}
	call("manufacturer", "archive", "", map[string]any{"id": 1}, false, 403)
	for _, entity := range []string{"manufacturer", "brand"} {
		p := call(entity, "archive", "", map[string]any{"id": 1}, true, 200)
		if p["ready_to_execute"] != false {
			t.Fatal("active product did not block archive", p)
		}
	}
	counts(0, 0)
	exec("UPDATE products SET lifecycle_status='archived' WHERE productid=1")
	if p := call("manufacturer", "archive", "", map[string]any{"id": 1}, true, 200); p["ready_to_execute"] != false {
		t.Fatal("active brand did not block manufacturer")
	}
	args := confirmed("brand", "archive")
	args["preview"] = true
	call("brand", "archive", "", args, true, 200)
	counts(0, 0)
	delete(args, "preview")
	call("brand", "archive", "", args, true, 428)
	wrong := confirmed("brand", "archive")
	wrong["confirmation_text"] = "ARCHIVE WAREHOUSE BRAND 2"
	call("brand", "archive", "bad-phrase", wrong, true, 428)
	created := call("brand", "archive", "master-brand-archive", args, true, 200)
	counts(1, 1)
	if created["brand"].(map[string]any)["archived_at"] == nil {
		t.Fatal("archive timestamp missing")
	}
	replay := call("brand", "archive", "master-brand-archive", args, true, 200)
	if !reflect.DeepEqual(created, replay) {
		t.Fatal("replay changed")
	}
	counts(1, 1)
	if _, err := db.Exec("UPDATE brands SET name='Changed' WHERE brandid=1"); err == nil {
		t.Fatal("ordinary edit to archived brand allowed")
	}
	ma := confirmed("manufacturer", "archive")
	call("manufacturer", "archive", "master-maker-archive", ma, true, 200)
	counts(2, 2)
	if _, err := db.Exec("INSERT INTO products(name,manufacturerid) VALUES('Illegal active product',1)"); err == nil {
		t.Fatal("active product attached to archived manufacturer")
	}
	if _, err := db.Exec("INSERT INTO products(name,brandid) VALUES('Illegal active brand product',1)"); err == nil {
		t.Fatal("active product attached to archived brand")
	}
	if _, err := db.Exec("INSERT INTO brands(name,manufacturerid) VALUES('Illegal active brand',1)"); err == nil {
		t.Fatal("active brand attached to archived manufacturer")
	}
	if p := call("brand", "restore", "", map[string]any{"id": 1}, true, 200); p["ready_to_execute"] != false {
		t.Fatal("brand restore accepted archived parent")
	}
	call("brand", "restore", "master-brand-blocked", confirmed("brand", "restore"), true, 200)
	counts(2, 2)
	// Audit failure must retain the archived state, version and previous receipts.
	exec(`CREATE FUNCTION reject_master_audit() RETURNS TRIGGER AS $$ BEGIN IF NEW.action='manufacturer.restore' THEN RAISE EXCEPTION 'audit failed'; END IF; RETURN NEW; END $$ LANGUAGE plpgsql; CREATE TRIGGER reject_master_audit BEFORE INSERT ON audit_log FOR EACH ROW EXECUTE FUNCTION reject_master_audit()`)
	restore := confirmed("manufacturer", "restore")
	before := version("manufacturer")
	call("manufacturer", "restore", "master-maker-audit-fail", restore, true, 500)
	counts(2, 2)
	if version("manufacturer") != before {
		t.Fatal("failed audit leaked version")
	}
	exec("DROP TRIGGER reject_master_audit ON audit_log")
	call("manufacturer", "restore", "master-maker-restore", restore, true, 200)
	call("brand", "restore", "master-brand-restore", confirmed("brand", "restore"), true, 200)
	counts(4, 4)
	// Every ordinary UI update invalidates an earlier preview.
	stale := confirmed("manufacturer", "archive")
	exec("UPDATE manufacturer SET website='https://updated.test' WHERE manufacturerid=1")
	p := call("manufacturer", "archive", "master-stale-preview", stale, true, 200)
	if p["ready_to_execute"] != false || !strings.Contains(fmt.Sprint(p["required_fields"]), "expected_updated_at") {
		t.Fatal("stale preview accepted")
	}
	counts(4, 4)
	// Historical products are retained throughout the lifecycle.
	var n int
	db.QueryRow("SELECT count(*) FROM products WHERE manufacturerid=1 AND brandid=1 AND lifecycle_status='archived'").Scan(&n)
	if n != 1 {
		t.Fatal("historical relationships changed")
	}
}
