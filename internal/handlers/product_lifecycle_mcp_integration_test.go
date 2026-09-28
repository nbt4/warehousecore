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
	"strings"
	"testing"

	"github.com/gorilla/mux"
	_ "github.com/lib/pq"
	"warehousecore/internal/middleware"
	"warehousecore/internal/models"
	"warehousecore/internal/repository"
)

func TestWarehouseProductMCPLifecycleDependenciesVersionAndReplay(t *testing.T) {
	dsn := os.Getenv("WAREHOUSE_TEST_DATABASE_URL")
	parsed, err := url.Parse(dsn)
	if dsn == "" {
		t.Skip("set WAREHOUSE_TEST_DATABASE_URL to a disposable _test database")
	}
	if err != nil || !strings.HasSuffix(strings.TrimPrefix(parsed.Path, "/"), "_test") {
		t.Fatal("integration test requires a dedicated _test database")
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	const schema = "warehouse_product_mcp_lifecycle_test"
	if _, err := db.Exec("DROP SCHEMA IF EXISTS " + schema + " CASCADE"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("CREATE SCHEMA " + schema); err != nil {
		t.Fatal(err)
	}
	defer db.Exec("DROP SCHEMA IF EXISTS " + schema + " CASCADE")
	if _, err := db.Exec("SET search_path TO " + schema); err != nil {
		t.Fatal(err)
	}
	for _, statement := range []string{
		`CREATE TABLE products (productid INT PRIMARY KEY,name TEXT,lifecycle_status TEXT,website_visible BOOL,website_featured BOOL,updated_at TIMESTAMP)`,
		`CREATE TABLE devices (deviceid TEXT PRIMARY KEY,productid INT,lifecycle_status TEXT,archived_at TIMESTAMP,archived_by_product BOOL,updated_at TIMESTAMP)`,
		`CREATE TABLE inventory_identifiers (entity_type TEXT,entity_key TEXT,active BOOL)`,
		`CREATE TABLE status (statusid INT PRIMARY KEY,status TEXT)`,
		`CREATE TABLE jobs (jobid INT PRIMARY KEY,statusid INT,deleted_at TIMESTAMP)`,
		`CREATE TABLE job_product_requirements (job_id INT,product_id INT)`,
		`CREATE TABLE job_devices (jobid INT,deviceid TEXT,pack_status TEXT)`,
		`CREATE TABLE audit_log (id BIGSERIAL PRIMARY KEY,user_id BIGINT,action TEXT,entity_type TEXT,entity_id TEXT,old_values JSONB,new_values JSONB,ip_address TEXT,user_agent TEXT)`,
		`CREATE TABLE warehouse_product_mutation_receipts (id BIGSERIAL PRIMARY KEY,user_id BIGINT NOT NULL,operation VARCHAR(80) NOT NULL,key_hash CHAR(64) NOT NULL,request_hash CHAR(64) NOT NULL,response JSONB NOT NULL DEFAULT '{}'::jsonb,status_code INTEGER NOT NULL DEFAULT 200,created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,UNIQUE(user_id,operation,key_hash))`,
		`INSERT INTO products VALUES (1,'Mixer','active',TRUE,TRUE,'2026-09-24T08:00:00.123456')`,
		`INSERT INTO devices(deviceid,productid,lifecycle_status,archived_by_product) VALUES ('D-1',1,'active',FALSE)`,
		`INSERT INTO inventory_identifiers VALUES ('product','1',TRUE),('device','D-1',TRUE)`,
		`INSERT INTO status VALUES (1,'Planung'),(4,'Abgeschlossen')`,
		`INSERT INTO jobs VALUES (10,1,NULL)`,
		`INSERT INTO job_product_requirements VALUES (10,1)`,
	} {
		if _, err := db.Exec(statement); err != nil {
			t.Fatal(err)
		}
	}
	previousDB := repository.DB
	repository.DB = db
	defer func() { repository.DB = previousDB }()
	request := func(method, path, key, version string) *httptest.ResponseRecorder {
		body, err := json.Marshal(map[string]string{"expectedUpdatedAt": version})
		if err != nil {
			t.Fatal(err)
		}
		r := httptest.NewRequest(method, path, bytes.NewReader(body))
		r = mux.SetURLVars(r, map[string]string{"id": "1"})
		r = r.WithContext(context.WithValue(r.Context(), middleware.UserContextKey, &models.User{UserID: 11, Username: "tester", IsAdmin: true}))
		r.Header.Set("X-Cores-Origin", "MCP/AI")
		r.Header.Set("Idempotency-Key", key)
		w := httptest.NewRecorder()
		if method == http.MethodDelete {
			DeleteProduct(w, r)
		} else {
			RestoreProduct(w, r)
		}
		return w
	}
	const initial = "2026-09-24T08:00:00.123456Z"
	archive := func(key, version string) *httptest.ResponseRecorder {
		return request(http.MethodDelete, "/api/v1/admin/products/1", key, version)
	}
	if got := archive("lifecycle-blocked-req", initial); got.Code != http.StatusConflict {
		t.Fatalf("open requirement accepted: %d %s", got.Code, got.Body.String())
	}
	if _, err := db.Exec(`DELETE FROM job_product_requirements`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO job_devices VALUES (10,'D-1','issued')`); err != nil {
		t.Fatal(err)
	}
	if got := archive("lifecycle-blocked-device", initial); got.Code != http.StatusConflict {
		t.Fatalf("issued device accepted: %d %s", got.Code, got.Body.String())
	}
	if _, err := db.Exec(`DELETE FROM job_devices`); err != nil {
		t.Fatal(err)
	}
	if got := archive("lifecycle-no-version", ""); got.Code != http.StatusPreconditionRequired {
		t.Fatalf("missing version accepted: %d %s", got.Code, got.Body.String())
	}
	first := archive("lifecycle-archive-1", initial)
	if first.Code != http.StatusOK {
		t.Fatalf("archive: %d %s", first.Code, first.Body.String())
	}
	if replay := archive("lifecycle-archive-1", initial); replay.Code != http.StatusOK {
		t.Fatalf("archive replay: %d %s", replay.Code, replay.Body.String())
	} else {
		var original, repeated map[string]any
		if err := json.Unmarshal(first.Body.Bytes(), &original); err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(replay.Body.Bytes(), &repeated); err != nil || original["updated_at"] != repeated["updated_at"] || original["archived_devices"] != repeated["archived_devices"] {
			t.Fatalf("archive replay differed: %#v %#v %v", original, repeated, err)
		}
	}
	if stale := request(http.MethodPut, "/api/v1/admin/products/1/restore", "lifecycle-restore-stale", initial); stale.Code != http.StatusConflict {
		t.Fatalf("stale restore accepted: %d %s", stale.Code, stale.Body.String())
	}
	var archived struct {
		UpdatedAt string `json:"updated_at"`
	}
	if err := json.Unmarshal(first.Body.Bytes(), &archived); err != nil || archived.UpdatedAt == "" {
		t.Fatalf("archive response: %s %v", first.Body.String(), err)
	}
	if restored := request(http.MethodPut, "/api/v1/admin/products/1/restore", "lifecycle-restore-1", archived.UpdatedAt); restored.Code != http.StatusOK {
		t.Fatalf("restore: %d %s", restored.Code, restored.Body.String())
	}
	var active int
	if err := db.QueryRow(`SELECT COUNT(*) FROM devices WHERE productid=1 AND lifecycle_status='active' AND archived_by_product=FALSE`).Scan(&active); err != nil || active != 1 {
		t.Fatalf("restored devices: %d %v", active, err)
	}
	var audits, receipts int
	if err := db.QueryRow(`SELECT COUNT(*) FROM audit_log WHERE new_values->>'origin'='MCP/AI'`).Scan(&audits); err != nil || audits != 2 {
		t.Fatalf("audits: %d %v", audits, err)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM warehouse_product_mutation_receipts`).Scan(&receipts); err != nil || receipts != 2 {
		t.Fatalf("receipts: %d %v", receipts, err)
	}
}
