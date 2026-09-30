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

func TestWarehouseMCPCategoryUpdatesAndDeletion(t *testing.T) {
	dsn := os.Getenv("WAREHOUSE_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set WAREHOUSE_TEST_DATABASE_URL to a disposable _test database")
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
	const schema = "warehouse_category_mcp_test"
	if _, err := db.Exec("DROP SCHEMA IF EXISTS " + schema + " CASCADE; CREATE SCHEMA " + schema + "; SET search_path TO " + schema); err != nil {
		t.Fatal(err)
	}
	defer db.Exec("DROP SCHEMA IF EXISTS " + schema + " CASCADE")
	for _, statement := range []string{
		`CREATE TABLE warehouse_schema_migrations(version TEXT PRIMARY KEY)`,
		`CREATE TABLE categories(categoryid SERIAL PRIMARY KEY,name VARCHAR(100) NOT NULL,abbreviation VARCHAR(10))`,
		`CREATE TABLE subcategories(subcategoryid VARCHAR(50) PRIMARY KEY,name VARCHAR(100) NOT NULL,abbreviation VARCHAR(10),categoryid INT REFERENCES categories ON DELETE SET NULL)`,
		`CREATE TABLE subbiercategories(subbiercategoryid VARCHAR(50) PRIMARY KEY,name VARCHAR(100) NOT NULL,abbreviation VARCHAR(10),subcategoryid VARCHAR(50) REFERENCES subcategories ON DELETE SET NULL)`,
		`CREATE TABLE products(productid SERIAL PRIMARY KEY,name TEXT,categoryid INT REFERENCES categories ON DELETE SET NULL,subcategoryid VARCHAR(50) REFERENCES subcategories ON DELETE SET NULL,subbiercategoryid VARCHAR(50) REFERENCES subbiercategories ON DELETE SET NULL)`,
		`CREATE TABLE audit_log(id BIGSERIAL PRIMARY KEY,user_id BIGINT,action TEXT,entity_type TEXT,entity_id TEXT,old_values JSONB,new_values JSONB,user_agent TEXT)`,
		`CREATE TABLE warehouse_product_mutation_receipts(id BIGSERIAL PRIMARY KEY,user_id BIGINT NOT NULL,operation VARCHAR(80) NOT NULL,key_hash CHAR(64) NOT NULL,request_hash CHAR(64) NOT NULL,response JSONB NOT NULL DEFAULT '{}'::jsonb,status_code INTEGER NOT NULL DEFAULT 200,created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,UNIQUE(user_id,operation,key_hash))`,
		`CREATE UNIQUE INDEX uq_category ON categories(lower(trim(name)))`,
		`CREATE UNIQUE INDEX uq_subcategory ON subcategories(categoryid,lower(trim(name)))`,
		`CREATE UNIQUE INDEX uq_third ON subbiercategories(subcategoryid,lower(trim(name)))`,
		`INSERT INTO categories(name,abbreviation) VALUES('Alpha','A'),('Beta','B')`,
		`INSERT INTO subcategories VALUES('sub-a','Sub Alpha',NULL,1),('sub-b','Sub Beta','SB',2)`,
		`INSERT INTO subbiercategories VALUES('third-a','Third Alpha',NULL,'sub-a'),('third-b','Third Beta','TB','sub-b')`,
	} {
		if _, err := db.Exec(statement); err != nil {
			t.Fatal(err)
		}
	}
	oldDB := repository.DB
	repository.DB = db
	defer func() { repository.DB = oldDB }()
	for i := 0; i < 2; i++ {
		if err := EnsureWarehouseCategoryVersionSchema(); err != nil {
			t.Fatal(err)
		}
	}
	version := func(kind, id string) string {
		table, column := "categories", "categoryid"
		if kind == "subcategory" {
			table, column = "subcategories", "subcategoryid"
		} else if kind == "third_category" {
			table, column = "subbiercategories", "subbiercategoryid"
		}
		var value string
		if err := db.QueryRow(fmt.Sprintf(`SELECT to_char(updated_at,'YYYY-MM-DD"T"HH24:MI:SS.US"Z"') FROM %s WHERE %s=$1`, table, column), id).Scan(&value); err != nil {
			t.Fatal(err)
		}
		return value
	}
	request := func(method, kind, id, key string, body map[string]any, admin bool) *httptest.ResponseRecorder {
		raw, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		r := httptest.NewRequest(method, "/category/"+id, bytes.NewReader(raw))
		r = mux.SetURLVars(r, map[string]string{"id": id})
		r = r.WithContext(context.WithValue(r.Context(), middleware.UserContextKey, &models.User{UserID: 11, IsAdmin: admin, Username: "tester"}))
		r.Header.Set("X-Cores-Origin", "MCP/AI")
		r.Header.Set("Idempotency-Key", key)
		w := httptest.NewRecorder()
		if method == http.MethodPut {
			switch kind {
			case "category":
				UpdateCategory(w, r)
			case "subcategory":
				UpdateSubcategory(w, r)
			case "third_category":
				UpdateSubbiercategory(w, r)
			}
		} else {
			switch kind {
			case "category":
				DeleteCategory(w, r)
			case "subcategory":
				DeleteSubcategory(w, r)
			case "third_category":
				DeleteSubbiercategory(w, r)
			}
		}
		return w
	}
	status := func(w *httptest.ResponseRecorder, want int) {
		t.Helper()
		if w.Code != want {
			t.Fatalf("status %d want %d: %s", w.Code, want, w.Body.String())
		}
	}
	for _, target := range []struct {
		kind, id string
		body     map[string]any
	}{
		{"category", "1", map[string]any{"name": "Updated Alpha", "abbreviation": "A"}},
		{"subcategory", "sub-a", map[string]any{"name": "Updated Sub Alpha", "abbreviation": nil, "category_id": 1}},
		{"third_category", "third-a", map[string]any{"name": "Updated Third Alpha", "abbreviation": nil, "subcategory_id": "sub-a"}},
	} {
		body := target.body
		body["expected_updated_at"] = version(target.kind, target.id)
		call := func(key string, admin bool) *httptest.ResponseRecorder {
			return request(http.MethodPut, target.kind, target.id, target.kind+"-"+key, body, admin)
		}
		status(call("no-admin", false), http.StatusForbidden)
		oldVersion := body["expected_updated_at"]
		body["expected_updated_at"] = ""
		status(call("missing-version", true), http.StatusPreconditionRequired)
		body["expected_updated_at"] = oldVersion
		first := call("update-once", true)
		status(first, http.StatusOK)
		if version(target.kind, target.id) == oldVersion {
			t.Fatal("category version did not advance")
		}
		replay := call("update-once", true)
		status(replay, http.StatusOK)
		var a, b map[string]any
		json.Unmarshal(first.Body.Bytes(), &a)
		json.Unmarshal(replay.Body.Bytes(), &b)
		if !reflect.DeepEqual(a, b) {
			t.Fatal("update replay changed response")
		}
		status(call("stale", true), http.StatusConflict)
		body["name"] = "Different"
		status(call("update-once", true), http.StatusConflict)
		body["expected_updated_at"] = version(target.kind, target.id)
		body["name"] = "Audit Failure " + target.kind
		if _, err := db.Exec(`CREATE FUNCTION fail_category_audit() RETURNS TRIGGER AS $$ BEGIN RAISE EXCEPTION 'forced audit failure'; END $$ LANGUAGE plpgsql;CREATE TRIGGER fail_audit BEFORE INSERT ON audit_log FOR EACH ROW EXECUTE FUNCTION fail_category_audit()`); err != nil {
			t.Fatal(err)
		}
		status(call("audit-failure", true), http.StatusInternalServerError)
		if version(target.kind, target.id) != body["expected_updated_at"] {
			t.Fatal("failed audit did not roll back update")
		}
		if _, err := db.Exec(`DROP TRIGGER fail_audit ON audit_log; DROP FUNCTION fail_category_audit()`); err != nil {
			t.Fatal(err)
		}
		var count int
		if err := db.QueryRow(`SELECT count(*) FROM audit_log WHERE action=$1 AND old_values->>'name' IS NOT NULL AND new_values->>'origin'='MCP/AI'`, target.kind+".update").Scan(&count); err != nil || count != 1 {
			t.Fatalf("update audits %d %v", count, err)
		}
	}
	sub := map[string]any{"name": "Updated Sub Alpha", "abbreviation": nil, "category_id": 2, "expected_updated_at": version("subcategory", "sub-a")}
	third := map[string]any{"name": "Updated Third Alpha", "abbreviation": nil, "subcategory_id": "sub-b", "expected_updated_at": version("third_category", "third-a")}
	if _, err := db.Exec(`INSERT INTO products(name,categoryid,subcategoryid,subbiercategoryid) VALUES('Linked fixture',1,'sub-a','third-a')`); err != nil {
		t.Fatal(err)
	}
	status(request(http.MethodPut, "subcategory", "sub-a", "move-sub-used", sub, true), http.StatusConflict)
	status(request(http.MethodPut, "third_category", "third-a", "move-third-used", third, true), http.StatusConflict)
	sub["category_id"] = 999
	status(request(http.MethodPut, "subcategory", "sub-a", "move-sub-missing", sub, true), http.StatusNotFound)
	sub["category_id"] = 2
	third["subcategory_id"] = "missing"
	status(request(http.MethodPut, "third_category", "third-a", "move-third-missing", third, true), http.StatusNotFound)
	third["subcategory_id"] = "sub-b"
	for _, target := range []struct{ kind, id string }{{"category", "1"}, {"subcategory", "sub-a"}, {"third_category", "third-a"}} {
		body := map[string]any{"expected_updated_at": version(target.kind, target.id), "confirm_delete": true, "confirmation_text": warehouseCategoryDeletionPhrase(target.kind, target.id)}
		status(request(http.MethodDelete, target.kind, target.id, "delete-used-"+target.kind, body, true), http.StatusConflict)
	}
	if _, err := db.Exec(`DELETE FROM products`); err != nil {
		t.Fatal(err)
	}
	status(request(http.MethodPut, "subcategory", "sub-a", "move-sub-unused", sub, true), http.StatusOK)
	status(request(http.MethodPut, "third_category", "third-a", "move-third-unused", third, true), http.StatusOK)
	sub["expected_updated_at"] = version("subcategory", "sub-a")
	status(request(http.MethodPut, "subcategory", "sub-a", "no-op-sub", sub, true), http.StatusConflict)
	sub["name"] = "Sub Beta"
	status(request(http.MethodPut, "subcategory", "sub-a", "duplicate-sub", sub, true), http.StatusConflict)
	third["expected_updated_at"] = version("third_category", "third-a")
	third["name"] = "Third Beta"
	status(request(http.MethodPut, "third_category", "third-a", "duplicate-third", third, true), http.StatusConflict)
	category := map[string]any{"name": "Beta", "abbreviation": "A", "expected_updated_at": version("category", "1")}
	status(request(http.MethodPut, "category", "1", "duplicate-top", category, true), http.StatusConflict)
	category["name"] = "Valid Name"
	category["abbreviation"] = ""
	status(request(http.MethodPut, "category", "1", "empty-top-abbreviation", category, true), http.StatusBadRequest)
	category["abbreviation"] = "ABCDEFGHIJK"
	status(request(http.MethodPut, "category", "1", "long-top-abbreviation", category, true), http.StatusBadRequest)
	category["abbreviation"] = "A"
	if _, err := db.Exec(`UPDATE categories SET name='UI Category Edit' WHERE categoryid=1`); err != nil {
		t.Fatal(err)
	}
	status(request(http.MethodPut, "category", "1", "stale-ui-top", category, true), http.StatusConflict)
	if _, err := db.Exec(`INSERT INTO categories(name,abbreviation) VALUES('Unused Delete Fixture','DEL');INSERT INTO subcategories VALUES('delete-sub','Unused Sub',NULL,2);INSERT INTO subbiercategories VALUES('delete-third','Unused Third',NULL,'sub-b')`); err != nil {
		t.Fatal(err)
	}
	var unusedID string
	if err := db.QueryRow(`SELECT categoryid::text FROM categories WHERE name='Unused Delete Fixture'`).Scan(&unusedID); err != nil {
		t.Fatal(err)
	}
	for _, target := range []struct{ kind, id string }{{"category", unusedID}, {"subcategory", "delete-sub"}, {"third_category", "delete-third"}} {
		body := map[string]any{"expected_updated_at": version(target.kind, target.id), "confirm_delete": true, "confirmation_text": warehouseCategoryDeletionPhrase(target.kind, target.id)}
		call := func(key string, admin bool) *httptest.ResponseRecorder {
			return request(http.MethodDelete, target.kind, target.id, target.kind+"-"+key, body, admin)
		}
		status(call("delete-no-admin", false), http.StatusForbidden)
		body["confirm_delete"] = false
		status(call("delete-no-confirm", true), http.StatusPreconditionRequired)
		body["confirm_delete"] = true
		body["confirmation_text"] = "DELETE WRONG RECORD"
		status(call("delete-wrong-phrase", true), http.StatusPreconditionRequired)
		body["confirmation_text"] = warehouseCategoryDeletionPhrase(target.kind, target.id)
		exact := body["expected_updated_at"]
		body["expected_updated_at"] = "old-version"
		status(call("delete-stale", true), http.StatusConflict)
		body["expected_updated_at"] = exact
		if _, err := db.Exec(`CREATE FUNCTION fail_delete_audit() RETURNS TRIGGER AS $$ BEGIN RAISE EXCEPTION 'forced delete audit failure'; END $$ LANGUAGE plpgsql;CREATE TRIGGER fail_audit BEFORE INSERT ON audit_log FOR EACH ROW EXECUTE FUNCTION fail_delete_audit()`); err != nil {
			t.Fatal(err)
		}
		status(call("delete-audit-fail", true), http.StatusInternalServerError)
		if version(target.kind, target.id) != exact {
			t.Fatal("failed delete audit lost record")
		}
		if _, err := db.Exec(`DROP TRIGGER fail_audit ON audit_log;DROP FUNCTION fail_delete_audit()`); err != nil {
			t.Fatal(err)
		}
		first := call("delete-once", true)
		status(first, http.StatusOK)
		replay := call("delete-once", true)
		status(replay, http.StatusOK)
		var a, b map[string]any
		json.Unmarshal(first.Body.Bytes(), &a)
		json.Unmarshal(replay.Body.Bytes(), &b)
		if !reflect.DeepEqual(a, b) {
			t.Fatal("durable delete replay changed response")
		}
		body["expected_updated_at"] = "different"
		status(call("delete-once", true), http.StatusConflict)
	}
	var deletions, receipts int
	if err := db.QueryRow(`SELECT count(*) FROM audit_log WHERE action LIKE '%.delete' AND old_values->>'name' IS NOT NULL AND new_values->>'origin'='MCP/AI'`).Scan(&deletions); err != nil || deletions != 3 {
		t.Fatalf("delete audits %d %v", deletions, err)
	}
	if err := db.QueryRow(`SELECT count(*) FROM warehouse_product_mutation_receipts WHERE operation LIKE '%.delete'`).Scan(&receipts); err != nil || receipts != 3 {
		t.Fatalf("delete receipts %d %v", receipts, err)
	}
	if _, err := db.Exec(`CREATE TABLE extension_category_refs(category_id INT REFERENCES categories ON DELETE SET NULL)`); err != nil {
		t.Fatal(err)
	}
	body := map[string]any{"expected_updated_at": version("category", "1"), "confirm_delete": true, "confirmation_text": warehouseCategoryDeletionPhrase("category", "1")}
	status(request(http.MethodDelete, "category", "1", "extension-reference-review", body, true), http.StatusConflict)
}
