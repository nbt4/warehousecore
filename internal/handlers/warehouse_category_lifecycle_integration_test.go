package handlers

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
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

func TestWarehouseCategoryLifecycleAtomicHistoryAndGuards(t *testing.T) {
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
	const schema = "warehouse_category_lifecycle_test"
	if _, err = db.Exec(`DROP SCHEMA IF EXISTS ` + schema + ` CASCADE;CREATE SCHEMA ` + schema + `;SET search_path TO ` + schema); err != nil {
		t.Fatal(err)
	}
	defer db.Exec(`DROP SCHEMA IF EXISTS ` + schema + ` CASCADE`)
	fixture := `CREATE TABLE warehouse_schema_migrations(version TEXT PRIMARY KEY);
 CREATE TABLE categories(categoryid SERIAL PRIMARY KEY,name VARCHAR(100) NOT NULL,abbreviation VARCHAR(10));
 CREATE TABLE subcategories(subcategoryid VARCHAR(50) PRIMARY KEY,name VARCHAR(100) NOT NULL,abbreviation VARCHAR(10),categoryid INT REFERENCES categories ON DELETE SET NULL);
 CREATE TABLE subbiercategories(subbiercategoryid VARCHAR(50) PRIMARY KEY,name VARCHAR(100) NOT NULL,abbreviation VARCHAR(10),subcategoryid VARCHAR(50) REFERENCES subcategories ON DELETE SET NULL);
 CREATE TABLE products(productid SERIAL PRIMARY KEY,name TEXT,categoryid INT REFERENCES categories ON DELETE SET NULL,subcategoryid VARCHAR(50) REFERENCES subcategories ON DELETE SET NULL,subbiercategoryid VARCHAR(50) REFERENCES subbiercategories ON DELETE SET NULL,lifecycle_status TEXT NOT NULL DEFAULT 'active',updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP);
 CREATE TABLE audit_log(id BIGSERIAL PRIMARY KEY,user_id BIGINT,action TEXT,entity_type TEXT,entity_id TEXT,old_values JSONB,new_values JSONB,user_agent TEXT);
 CREATE TABLE warehouse_product_mutation_receipts(id BIGSERIAL PRIMARY KEY,user_id BIGINT NOT NULL,operation VARCHAR(80) NOT NULL,key_hash CHAR(64) NOT NULL,request_hash CHAR(64) NOT NULL,response JSONB NOT NULL DEFAULT '{}'::jsonb,status_code INTEGER NOT NULL DEFAULT 200,created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,UNIQUE(user_id,operation,key_hash));
 INSERT INTO categories(name,abbreviation) VALUES('Retained Top','RT'),('Other','O');
 INSERT INTO subcategories VALUES('legacy sub id','Retained Sub',NULL,1);
 INSERT INTO subbiercategories VALUES('legacy-third','Retained Third','TH','legacy sub id');
 INSERT INTO products(name,categoryid,subcategoryid,subbiercategoryid) VALUES('Historical product',1,'legacy sub id','legacy-third');`
	if _, err = db.Exec(fixture); err != nil {
		t.Fatal(err)
	}
	oldDB := repository.DB
	repository.DB = db
	defer func() { repository.DB = oldDB }()
	for i := 0; i < 2; i++ {
		if err = EnsureWarehouseCategoryVersionSchema(); err != nil {
			t.Fatal(err)
		}
		if err = EnsureWarehouseCategoryLifecycleSchema(); err != nil {
			t.Fatal(err)
		}
	}
	request := func(kind, op, key string, body map[string]any, admin bool) (map[string]any, int) {
		t.Helper()
		raw, _ := json.Marshal(body)
		r := httptest.NewRequest("POST", "/mcp/"+kind+"/"+op, bytes.NewReader(raw))
		r = mux.SetURLVars(r, map[string]string{"entity": kind, "operation": op})
		r = r.WithContext(context.WithValue(r.Context(), middleware.UserContextKey, &models.User{UserID: 11, IsAdmin: admin, Username: "tester"}))
		r.Header.Set("X-Cores-Origin", "MCP/AI")
		r.Header.Set("Idempotency-Key", key)
		w := httptest.NewRecorder()
		WarehouseCategoryLifecycleMCP(w, r)
		result := map[string]any{}
		if err = json.Unmarshal(w.Body.Bytes(), &result); err != nil {
			t.Fatal(w.Body.String())
		}
		return result, w.Code
	}
	preview := func(kind, op, id string) map[string]any {
		t.Helper()
		out, status := request(kind, op, "", map[string]any{"id": id}, true)
		if status != 200 {
			t.Fatalf("preview %d %v", status, out)
		}
		return out
	}
	contains := func(out map[string]any, field string) bool {
		fields, _ := out["required_fields"].([]any)
		for _, f := range fields {
			if f == field {
				return true
			}
		}
		return false
	}
	prepared := func(kind, op, id string) map[string]any {
		t.Helper()
		p := preview(kind, op, id)
		if p["ready_to_execute"] != true {
			t.Fatalf("not ready: %v", p)
		}
		return map[string]any{"id": id, "expected_updated_at": p["expected_updated_at"], "expected_dependencies": p["expected_dependencies"], "confirmation_text": p["required_confirmation_text"], "confirm_lifecycle": true}
	}
	mustSQL := func(query string) {
		t.Helper()
		if _, err = db.Exec(query); err != nil {
			t.Fatal(err)
		}
	}
	blockedSQL := func(query string) {
		t.Helper()
		if _, err = db.Exec(query); err == nil {
			t.Fatal("unchecked writer accepted: " + query)
		}
	}
	targets := []struct{ kind, id string }{{"third_category", "legacy-third"}, {"subcategory", "legacy sub id"}, {"category", "1"}}
	for _, target := range targets {
		p := preview(target.kind, "archive", target.id)
		if !contains(p, "active_dependencies") {
			t.Fatalf("active references not blocked %v", p)
		}
	}
	if _, status := request("category", "archive", "", map[string]any{"id": "1"}, false); status != 403 {
		t.Fatal("nonadmin accepted")
	}
	for _, id := range []string{"01", "-1", "2147483648", " 1", "1\n"} {
		if _, status := request("category", "archive", "", map[string]any{"id": id}, true); status != 400 {
			t.Fatal("noncanonical ID", id, status)
		}
	}
	blockedSQL(`UPDATE subbiercategories SET lifecycle_status='archived'`)
	mustSQL(`UPDATE products SET lifecycle_status='archived'`)
	// The reference fingerprint includes history, even when it no longer blocks archive.
	stale := prepared("third_category", "archive", "legacy-third")
	mustSQL(`UPDATE products SET name='Changed history',updated_at=clock_timestamp()`)
	if out, status := request("third_category", "archive", "category-stale-deps", stale, true); status != 200 || !contains(out, "expected_dependencies") {
		t.Fatalf("changed reference accepted: %d %v", status, out)
	}
	for _, target := range targets {
		in := prepared(target.kind, "archive", target.id)
		invalid := map[string]any{}
		for k, v := range in {
			invalid[k] = v
		}
		invalid["confirmation_text"] = "ARCHIVE WAREHOUSE CATEGORY 2"
		if _, status := request(target.kind, "archive", "wrong-phrase", invalid, true); status != 428 {
			t.Fatal("wrong record phrase accepted")
		}
		mustSQL(`CREATE OR REPLACE FUNCTION reject_lifecycle_audit() RETURNS TRIGGER AS $$ BEGIN RAISE EXCEPTION 'forced final audit rollback'; END $$ LANGUAGE plpgsql;CREATE TRIGGER reject_audit BEFORE INSERT ON audit_log FOR EACH ROW EXECUTE FUNCTION reject_lifecycle_audit()`)
		if out, status := request(target.kind, "archive", target.kind+"-retry", in, true); status != 500 {
			t.Fatalf("expected rollback: %d %v", status, out)
		}
		now := preview(target.kind, "archive", target.id)
		if now["expected_updated_at"] != in["expected_updated_at"] || now["expected_dependencies"] != in["expected_dependencies"] {
			t.Fatal("audit failure changed records or versions")
		}
		mustSQL(`DROP TRIGGER reject_audit ON audit_log;DROP FUNCTION reject_lifecycle_audit()`)
		out, status := request(target.kind, "archive", target.kind+"-retry", in, true)
		if status != 200 || out["operation_status"] != "archived" {
			t.Fatalf("retry: %d %v", status, out)
		}
		replay, status := request(target.kind, "archive", target.kind+"-retry", in, true)
		if status != 200 || !reflect.DeepEqual(out, replay) {
			t.Fatal("durable replay differs")
		}
		spec, _ := warehouseCategoryLifecycleSpec(target.kind)
		blockedSQL(fmt.Sprintf(`UPDATE %s SET name='Unchecked edit' WHERE %s='%s'`, spec.table, spec.key, target.id))
		blockedSQL(fmt.Sprintf(`DELETE FROM %s WHERE %s='%s'`, spec.table, spec.key, target.id))
		// Restoration cannot hide a metadata change inside a lifecycle update.
		blockedSQL(fmt.Sprintf(`UPDATE %s SET lifecycle_status='active',name='Unchecked restore edit' WHERE %s='%s'`, spec.table, spec.key, target.id))
	}
	blockedSQL(`INSERT INTO subcategories(subcategoryid,name,categoryid) VALUES('new','New',1)`)
	blockedSQL(`INSERT INTO subbiercategories(subbiercategoryid,name,subcategoryid) VALUES('new','New','legacy sub id')`)
	blockedSQL(`UPDATE products SET lifecycle_status='active'`)
	if p := preview("third_category", "restore", "legacy-third"); !contains(p, "active_parent_ancestry") {
		t.Fatal("inactive parent accepted", p)
	}
	if p := preview("subcategory", "restore", "legacy sub id"); !contains(p, "active_parent_ancestry") {
		t.Fatal("inactive grandparent accepted", p)
	}
	for i := len(targets) - 1; i >= 0; i-- {
		target := targets[i]
		in := prepared(target.kind, "restore", target.id)
		// Parent changes after preview require a fresh preview, while the target itself is unchanged.
		if target.kind == "subcategory" {
			mustSQL(`UPDATE categories SET abbreviation='NEW' WHERE categoryid=1`)
			if out, status := request(target.kind, "restore", "stale-parent", in, true); status != 200 || !contains(out, "expected_dependencies") {
				t.Fatalf("changed parent accepted: %d %v", status, out)
			}
			in = prepared(target.kind, "restore", target.id)
		}
		out, status := request(target.kind, "restore", target.kind+"-restore", in, true)
		if status != 200 || out["operation_status"] != "restored" {
			t.Fatalf("restore: %d %v", status, out)
		}
	}
	mustSQL(`UPDATE products SET lifecycle_status='active'`)
	blockedSQL(`UPDATE products SET categoryid=2`)
	var retained, receipts, audits int
	if err = db.QueryRow(`SELECT count(*) FROM products WHERE categoryid=1 AND subcategoryid='legacy sub id' AND subbiercategoryid='legacy-third'`).Scan(&retained); err != nil || retained != 1 {
		t.Fatal("history detached", err)
	}
	db.QueryRow(`SELECT count(*) FROM warehouse_product_mutation_receipts`).Scan(&receipts)
	db.QueryRow(`SELECT count(*) FROM audit_log`).Scan(&audits)
	if receipts != 6 || audits != 6 {
		t.Fatal("rollback/replay duplicated receipt or audit", receipts, audits)
	}
	// Unique retained identity is checked on restore even for legacy databases without indexes.
	mustSQL(`INSERT INTO categories(name,abbreviation) VALUES('Duplicate','D');UPDATE categories SET lifecycle_status='archived' WHERE name='Duplicate';INSERT INTO categories(name,abbreviation) VALUES('Duplicate','D')`)
	if p := preview("category", "restore", "3"); !contains(p, "duplicate_retained_identity") {
		t.Fatal("duplicate restore accepted", p)
	}
}
