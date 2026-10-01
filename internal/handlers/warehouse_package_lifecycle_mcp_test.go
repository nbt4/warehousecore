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
	"strconv"
	"strings"
	"testing"

	"github.com/gorilla/mux"
	"warehousecore/internal/middleware"
	"warehousecore/internal/models"
	"warehousecore/internal/repository"
)

func TestWarehousePackageMCPLifecycle(t *testing.T) {
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
	const schema = "warehouse_package_lifecycle_test"
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
	exec(`ALTER TABLE job_packages ADD COLUMN job_package_id SERIAL; CREATE TABLE jobs(jobid INT PRIMARY KEY,statusid INT,deleted_at TIMESTAMP);CREATE TABLE status(statusid INT PRIMARY KEY,status TEXT); INSERT INTO status VALUES(1,'open'),(2,'closed');INSERT INTO jobs VALUES(1,1,NULL),(2,2,NULL),(3,NULL,NULL); CREATE TABLE job_package_reservations(job_package_id INT,reservation_status VARCHAR(20)); CREATE FUNCTION warehouse_job_status_is_closed(TEXT) RETURNS BOOLEAN LANGUAGE SQL IMMUTABLE AS $$ SELECT COALESCE($1,'')='closed' $$; ALTER TABLE audit_log ADD COLUMN timestamp TIMESTAMP DEFAULT CURRENT_TIMESTAMP`)
	request := func(op string, id int64, key string, body map[string]any, admin bool) *httptest.ResponseRecorder {
		t.Helper()
		raw, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		r := httptest.NewRequest(http.MethodPost, "/package", bytes.NewReader(raw))
		r = mux.SetURLVars(r, map[string]string{"id": strconv.FormatInt(id, 10)})
		r = r.WithContext(context.WithValue(r.Context(), middleware.UserContextKey, &models.User{UserID: 11, IsAdmin: admin, Username: "tester"}))
		r.Header.Set("X-Cores-Origin", "MCP/AI")
		if key != "" {
			key = "lifecycle-" + key
		}
		r.Header.Set("Idempotency-Key", key)
		w := httptest.NewRecorder()
		switch op {
		case "create":
			CreateProductPackage(w, r)
		case "archive":
			ArchiveProductPackageMCP(w, r)
		case "restore":
			RestoreProductPackageMCP(w, r)
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
	base := map[string]any{"name": "Lifecycle Fixture", "description": "private description", "price": 12.34, "website_visible": true, "aliases": []string{"Set"}, "items": []map[string]any{{"product_id": 1, "quantity": 2, "is_optional": true}, {"product_id": 2, "quantity": 3}}}
	created := request("create", 0, "create", base, true)
	status(created, 201)
	var result map[string]any
	json.Unmarshal(created.Body.Bytes(), &result)
	id := int64(result["package_id"].(float64))
	version := func() string {
		t.Helper()
		var v string
		if err := db.QueryRow(`SELECT to_char(updated_at,'YYYY-MM-DD"T"HH24:MI:SS.US"Z"') FROM product_packages WHERE id=$1`, id).Scan(&v); err != nil {
			t.Fatal(err)
		}
		return v
	}
	body := func(op string) map[string]any {
		return map[string]any{"expected_updated_at": version(), "confirm_lifecycle": true, "confirmation_text": fmt.Sprintf("%s WAREHOUSE PACKAGE %d", strings.ToUpper(op), id)}
	}
	count := func(q string) int {
		t.Helper()
		var n int
		if err := db.QueryRow(q).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	var lineIDs string
	db.QueryRow(`SELECT string_agg(id::text,',' ORDER BY id) FROM product_package_items WHERE package_id=$1`, id).Scan(&lineIDs)
	status(request("archive", id, "admin", body("archive"), false), 403)
	status(request("archive", id, "", body("archive"), true), 428)
	status(request("archive", id, "confirm", map[string]any{"expected_updated_at": version()}, true), 428)
	bad := body("archive")
	bad["expected_updated_at"] = "stale"
	status(request("archive", id, "stale", bad, true), 409)
	// Open and unknown-status jobs both block; historical closed jobs preserve composition.
	for _, job := range []int{1, 3} {
		exec(`INSERT INTO job_packages(package_id,job_id) VALUES($1,$2)`, id, job)
		status(request("archive", id, fmt.Sprint("job-", job), body("archive"), true), 409)
		exec(`DELETE FROM job_packages`)
	}
	exec(`INSERT INTO job_packages(package_id,job_id) VALUES($1,2)`, id)
	exec(`INSERT INTO job_package_reservations SELECT job_package_id,'assigned' FROM job_packages`)
	status(request("archive", id, "reservation", body("archive"), true), 409)
	exec(`UPDATE job_package_reservations SET reservation_status='released'`)
	arc := body("archive")
	status(request("archive", id, "archive", arc, true), 200)
	status(request("archive", id, "archive", arc, true), 200)
	if count(`SELECT count(*) FROM audit_log WHERE action='package.archive'`) != 1 {
		t.Fatal("duplicate archive audit")
	}
	status(request("archive", id, "again", body("archive"), true), 409)
	var active, visible bool
	db.QueryRow(`SELECT is_active,website_visible FROM product_packages WHERE id=$1`, id).Scan(&active, &visible)
	if active || visible {
		t.Fatal("archive left package public")
	}
	// Item writes invalidate restore previews and archived/missing products block restore.
	restore := body("restore")
	exec(`UPDATE product_package_items SET quantity=quantity+1 WHERE package_id=$1 AND product_id=1`, id)
	status(request("restore", id, "item-stale", restore, true), 409)
	exec(`UPDATE products SET lifecycle_status='archived' WHERE productid=1`)
	status(request("restore", id, "product-archived", body("restore"), true), 409)
	exec(`UPDATE products SET lifecycle_status='active' WHERE productid=1`)
	// Simulate a legacy database without the package-product foreign key.
	exec(`DO $$ DECLARE c TEXT; BEGIN FOR c IN SELECT conname FROM pg_constraint WHERE conrelid='product_package_items'::regclass AND confrelid='products'::regclass LOOP EXECUTE 'ALTER TABLE product_package_items DROP CONSTRAINT '||quote_ident(c); END LOOP; END $$`)
	exec(`DELETE FROM products WHERE productid=2`)
	status(request("restore", id, "product-missing", body("restore"), true), 409)
	exec(`INSERT INTO products(productid,name,lifecycle_status) VALUES(2,'Beta','active')`)
	exec(`UPDATE jobs SET statusid=1 WHERE jobid=2`)
	status(request("restore", id, "restore-job", body("restore"), true), 409)
	exec(`UPDATE jobs SET statusid=2 WHERE jobid=2`)
	// Even legacy inactive/public packages restore as private.
	exec(`UPDATE product_packages SET website_visible=true WHERE id=$1`, id)
	restore = body("restore")
	status(request("restore", id, "restore", restore, true), 200)
	status(request("restore", id, "restore", restore, true), 200)
	db.QueryRow(`SELECT is_active,website_visible FROM product_packages WHERE id=$1`, id).Scan(&active, &visible)
	if !active || visible {
		t.Fatal("restore republished")
	}
	var afterIDs string
	db.QueryRow(`SELECT string_agg(id::text,',' ORDER BY id) FROM product_package_items WHERE package_id=$1`, id).Scan(&afterIDs)
	if lineIDs != afterIDs || count("SELECT count(*) FROM job_packages") != 1 {
		t.Fatal("lifecycle replaced contents or history")
	}
	// Audit errors roll back lifecycle, visibility and the receipt.
	exec(`CREATE FUNCTION reject_lifecycle_audit() RETURNS TRIGGER LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'audit unavailable'; END $$; CREATE TRIGGER reject_lifecycle_audit BEFORE INSERT ON audit_log FOR EACH ROW EXECUTE FUNCTION reject_lifecycle_audit()`)
	v := version()
	receipts := count("SELECT count(*) FROM warehouse_product_mutation_receipts")
	status(request("archive", id, "rollback-archive", body("archive"), true), 500)
	if version() != v || count("SELECT count(*) FROM warehouse_product_mutation_receipts") != receipts {
		t.Fatal("archive rollback leaked data")
	}
	exec(`DROP TRIGGER reject_lifecycle_audit ON audit_log`)
	status(request("archive", id, "final-archive", body("archive"), true), 200)
	exec(`CREATE TRIGGER reject_lifecycle_audit BEFORE INSERT ON audit_log FOR EACH ROW EXECUTE FUNCTION reject_lifecycle_audit()`)
	v = version()
	receipts = count("SELECT count(*) FROM warehouse_product_mutation_receipts")
	status(request("restore", id, "rollback-restore", body("restore"), true), 500)
	if version() != v || count("SELECT count(*) FROM warehouse_product_mutation_receipts") != receipts {
		t.Fatal("restore rollback leaked data")
	}
}
