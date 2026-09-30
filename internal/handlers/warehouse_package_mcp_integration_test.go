package handlers

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/gorilla/mux"
	"warehousecore/internal/middleware"
	"warehousecore/internal/models"
	"warehousecore/internal/repository"
)

func TestWarehousePackageMCPAtomicMutation(t *testing.T) {
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
	const schema = "warehouse_package_mcp_test"
	if _, err = db.Exec("DROP SCHEMA IF EXISTS " + schema + " CASCADE; CREATE SCHEMA " + schema + "; SET search_path TO " + schema); err != nil {
		t.Fatal(err)
	}
	defer db.Exec("DROP SCHEMA IF EXISTS " + schema + " CASCADE")
	exec := func(query string, args ...any) {
		t.Helper()
		if _, err := db.Exec(query, args...); err != nil {
			t.Fatal(err)
		}
	}
	for _, statement := range []string{
		`CREATE TABLE warehouse_schema_migrations(version TEXT PRIMARY KEY)`,
		`CREATE TABLE products(productid SERIAL PRIMARY KEY,name TEXT,lifecycle_status TEXT DEFAULT 'active',website_thumbnail TEXT,website_images_json TEXT)`,
		`INSERT INTO products(name,lifecycle_status) VALUES('Alpha','active'),('Beta','active'),('Retired','archived')`,
		`CREATE TABLE job_packages(package_id INT,job_id INT)`,
		`CREATE TABLE audit_log(id BIGSERIAL PRIMARY KEY,user_id BIGINT,action TEXT,entity_type TEXT,entity_id TEXT,old_values JSONB,new_values JSONB,user_agent TEXT)`,
		`CREATE TABLE warehouse_product_mutation_receipts(id BIGSERIAL PRIMARY KEY,user_id BIGINT NOT NULL,operation VARCHAR(80) NOT NULL,key_hash CHAR(64) NOT NULL,request_hash CHAR(64) NOT NULL,response JSONB NOT NULL DEFAULT '{}'::jsonb,status_code INTEGER NOT NULL DEFAULT 200,created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,UNIQUE(user_id,operation,key_hash))`,
	} {
		exec(statement)
	}
	oldDB, oldReady := repository.DB, packageStorageReady
	repository.DB = db
	packageStorageReady = false
	defer func() { repository.DB = oldDB; packageStorageReady = oldReady }()
	for i := 0; i < 2; i++ {
		if err = EnsureWarehousePackageVersionSchema(); err != nil {
			t.Fatal(err)
		}
	}
	request := func(id int64, key string, body map[string]any, admin bool) *httptest.ResponseRecorder {
		raw, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		method := http.MethodPost
		if id > 0 {
			method = http.MethodPut
		}
		r := httptest.NewRequest(method, "/package", bytes.NewReader(raw))
		r = mux.SetURLVars(r, map[string]string{"id": strconv.FormatInt(id, 10)})
		r = r.WithContext(context.WithValue(r.Context(), middleware.UserContextKey, &models.User{UserID: 11, IsAdmin: admin, Username: "tester"}))
		r.Header.Set("X-Cores-Origin", "MCP/AI")
		r.Header.Set("Idempotency-Key", key)
		w := httptest.NewRecorder()
		if id == 0 {
			CreateProductPackage(w, r)
		} else {
			UpdateProductPackage(w, r)
		}
		return w
	}
	status := func(w *httptest.ResponseRecorder, want int) {
		t.Helper()
		if w.Code != want {
			t.Fatalf("status %d want %d: %s", w.Code, want, w.Body.String())
		}
	}
	body := map[string]any{"name": "Fixture Package", "description": "Details", "price": 12.34, "category": "Sound", "website_visible": false, "aliases": []string{" Mix ", "mix", ""}, "items": []map[string]any{{"product_id": 2, "quantity": 3, "is_optional": true}, {"product_id": 1, "quantity": 2, "is_optional": false}}}
	clone := func(in map[string]any) map[string]any {
		out := map[string]any{}
		for k, v := range in {
			out[k] = v
		}
		return out
	}
	status(request(0, "package-non-admin", body, false), http.StatusForbidden)
	status(request(0, "", body, true), http.StatusPreconditionRequired)
	for _, tc := range []struct {
		name, field string
		value       any
	}{
		{"price", "price", -1.0}, {"precision", "price", 1.234}, {"empty", "items", []any{}}, {"duplicate", "items", []map[string]any{{"product_id": 1, "quantity": 1}, {"product_id": 1, "quantity": 2}}}, {"zero", "items", []map[string]any{{"product_id": 1, "quantity": 0}}},
	} {
		bad := clone(body)
		bad[tc.field] = tc.value
		status(request(0, "package-invalid-"+tc.name, bad, true), http.StatusBadRequest)
	}
	bad := clone(body)
	bad["items"] = []map[string]any{{"product_id": 3, "quantity": 1}}
	status(request(0, "package-archived", bad, true), http.StatusConflict)
	bad["items"] = []map[string]any{{"product_id": 999, "quantity": 1}}
	status(request(0, "package-missing", bad, true), http.StatusNotFound)
	created := request(0, "package-create-fixture", body, true)
	status(created, http.StatusCreated)
	var result struct {
		ID      int64                  `json:"package_id"`
		Code    string                 `json:"package_code"`
		Version string                 `json:"updated_at"`
		Package warehousePackageFields `json:"package"`
	}
	if err = json.Unmarshal(created.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.ID <= 0 || result.Code == "" || result.Version == "" || len(result.Package.Items) != 2 || !result.Package.Items[1].IsOptional || len(result.Package.Aliases) != 1 {
		t.Fatalf("incomplete create: %s", created.Body.String())
	}
	replay := request(0, "package-create-fixture", body, true)
	status(replay, http.StatusCreated)
	var replayData, createdData any
	json.Unmarshal(replay.Body.Bytes(), &replayData)
	json.Unmarshal(created.Body.Bytes(), &createdData)
	if !reflectJSONEqual(replayData, createdData) {
		t.Fatalf("create replay mismatch: %s", replay.Body.String())
	}
	bad = clone(body)
	bad["name"] = "different"
	status(request(0, "package-create-fixture", bad, true), http.StatusConflict)
	status(request(0, "package-create-duplicate", body, true), http.StatusConflict)
	version := func(id int64) string {
		var v string
		if err := db.QueryRow(`SELECT to_char(updated_at,'YYYY-MM-DD"T"HH24:MI:SS.US"Z"') FROM product_packages WHERE id=$1`, id).Scan(&v); err != nil {
			t.Fatal(err)
		}
		return v
	}
	count := func(query string) int {
		var n int
		if err := db.QueryRow(query).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	update := clone(body)
	update["name"] = "Renamed Fixture"
	update["expected_updated_at"] = result.Version
	status(request(result.ID, "package-update-fixture", update, true), http.StatusOK)
	v := version(result.ID)
	if v == result.Version {
		t.Fatal("update did not change version")
	}
	status(request(result.ID, "package-update-fixture", update, true), http.StatusOK)
	if count(`SELECT count(*) FROM audit_log WHERE action='package.update'`) != 1 {
		t.Fatal("replay duplicated audit")
	}
	status(request(result.ID, "package-update-stale", update, true), http.StatusConflict)
	// Separate item edits, even from legacy writers, invalidate both parent versions.
	exec(`UPDATE product_package_items SET quantity=quantity+1 WHERE package_id=$1 AND product_id=1`, result.ID)
	if version(result.ID) == v {
		t.Fatal("content edit did not version parent")
	}
	update["expected_updated_at"] = v
	status(request(result.ID, "package-update-items-stale", update, true), http.StatusConflict)
	// A job reference created after preview is rechecked by the owning Core.
	update["expected_updated_at"] = version(result.ID)
	exec(`INSERT INTO job_packages VALUES($1,42)`, result.ID)
	status(request(result.ID, "package-update-job-content", update, true), http.StatusConflict)
	current, _, _, _, err := func() (warehousePackageFields, string, string, bool, error) {
		tx, e := db.Begin()
		if e != nil {
			return warehousePackageFields{}, "", "", false, e
		}
		defer tx.Rollback()
		return loadWarehousePackage(tx, result.ID)
	}()
	if err != nil {
		t.Fatal(err)
	}
	encoded, _ := json.Marshal(current)
	var metadata map[string]any
	json.Unmarshal(encoded, &metadata)
	metadata["name"] = "Job Package Metadata"
	metadata["description"] = nil
	metadata["category"] = nil
	metadata["aliases"] = []string{}
	metadata["expected_updated_at"] = version(result.ID)
	var lineIDsBefore, lineIDsAfter string
	db.QueryRow(`SELECT string_agg(id::text,',' ORDER BY id) FROM product_package_items WHERE package_id=$1`, result.ID).Scan(&lineIDsBefore)
	status(request(result.ID, "package-job-metadata", metadata, true), http.StatusOK)
	db.QueryRow(`SELECT string_agg(id::text,',' ORDER BY id) FROM product_package_items WHERE package_id=$1`, result.ID).Scan(&lineIDsAfter)
	if lineIDsBefore != lineIDsAfter {
		t.Fatal("metadata update replaced content line IDs")
	}
	metadata["price"] = 23.45
	metadata["expected_updated_at"] = version(result.ID)
	status(request(result.ID, "package-job-price", metadata, true), http.StatusConflict)
	// Audit failure rolls back metadata, content and the receipt.
	exec(`DELETE FROM job_packages`)
	exec(`CREATE FUNCTION reject_package_audit() RETURNS TRIGGER LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'audit unavailable'; END $$; CREATE TRIGGER reject_package_audit BEFORE INSERT ON audit_log FOR EACH ROW EXECUTE FUNCTION reject_package_audit()`)
	beforeCount := count(`SELECT count(*) FROM warehouse_product_mutation_receipts`)
	beforeVersion := version(result.ID)
	metadata["expected_updated_at"] = beforeVersion
	status(request(result.ID, "package-rollback-update", metadata, true), http.StatusInternalServerError)
	if version(result.ID) != beforeVersion || count(`SELECT count(*) FROM warehouse_product_mutation_receipts`) != beforeCount {
		t.Fatal("failed audit did not roll back")
	}
	newBody := clone(body)
	newBody["name"] = "Rollback New Package"
	status(request(0, "package-rollback-create", newBody, true), http.StatusInternalServerError)
	if count(`SELECT count(*) FROM product_packages`) != 1 || count(`SELECT count(*) FROM product_package_items`) != 2 || count(`SELECT count(*) FROM warehouse_product_mutation_receipts`) != beforeCount {
		t.Fatal("failed creation left partial package")
	}
	exec(`DROP TRIGGER reject_package_audit ON audit_log`)
	// A line moved to another package bumps both versions, and deleting it bumps its owner.
	var otherID int64
	db.QueryRow(`INSERT INTO product_packages(name) VALUES('Other') RETURNING id`).Scan(&otherID)
	oldA, oldB := version(result.ID), version(otherID)
	exec(`UPDATE product_package_items SET package_id=$1 WHERE package_id=$2 AND product_id=2`, otherID, result.ID)
	if version(result.ID) == oldA || version(otherID) == oldB {
		t.Fatal("line move must version both packages")
	}
	oldB = version(otherID)
	exec(`DELETE FROM product_package_items WHERE package_id=$1`, otherID)
	if version(otherID) == oldB {
		t.Fatal("line delete did not version package")
	}
}

func reflectJSONEqual(a, b any) bool {
	x, _ := json.Marshal(a)
	y, _ := json.Marshal(b)
	return bytes.Equal(x, y)
}
