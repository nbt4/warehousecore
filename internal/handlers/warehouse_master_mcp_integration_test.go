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

	_ "github.com/lib/pq"
	"warehousecore/internal/middleware"
	"warehousecore/internal/models"
	"warehousecore/internal/repository"
)

func TestWarehouseMCPStandaloneManufacturerAndBrand(t *testing.T) {
	dsn := os.Getenv("WAREHOUSE_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set WAREHOUSE_TEST_DATABASE_URL to a disposable _test database")
	}
	parsed, err := url.Parse(dsn)
	if err != nil || !strings.HasSuffix(strings.TrimPrefix(parsed.Path, "/"), "_test") {
		t.Fatal("integration test requires a dedicated _test database")
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	const schema = "warehouse_mcp_master_create_test"
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
		`CREATE TABLE manufacturer(manufacturerid SERIAL PRIMARY KEY,name VARCHAR(255),website VARCHAR(255))`,
		`CREATE TABLE brands(brandid SERIAL PRIMARY KEY,name VARCHAR(255),manufacturerid INT REFERENCES manufacturer(manufacturerid))`,
		`CREATE TABLE audit_log(id BIGSERIAL PRIMARY KEY,user_id BIGINT,action TEXT,entity_type TEXT,entity_id TEXT,old_values JSONB,new_values JSONB,ip_address TEXT,user_agent TEXT)`,
		`CREATE TABLE warehouse_product_mutation_receipts(id BIGSERIAL PRIMARY KEY,user_id BIGINT NOT NULL,operation VARCHAR(80) NOT NULL,key_hash CHAR(64) NOT NULL,request_hash CHAR(64) NOT NULL,response JSONB NOT NULL DEFAULT '{}'::jsonb,status_code INTEGER NOT NULL DEFAULT 200,created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,UNIQUE(user_id,operation,key_hash))`,
	} {
		if _, err := db.Exec(statement); err != nil {
			t.Fatal(err)
		}
	}
	previousDB := repository.DB
	repository.DB = db
	defer func() { repository.DB = previousDB }()
	request := func(path, key string, body map[string]any) *httptest.ResponseRecorder {
		encoded, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		r := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(encoded))
		r = r.WithContext(context.WithValue(r.Context(), middleware.UserContextKey, &models.User{UserID: 11, Username: "tester", IsAdmin: true}))
		r.Header.Set("X-Cores-Origin", "MCP/AI")
		r.Header.Set("Idempotency-Key", key)
		w := httptest.NewRecorder()
		if path == "/api/v1/admin/manufacturers" {
			CreateManufacturer(w, r)
		} else {
			CreateBrand(w, r)
		}
		return w
	}
	manufacturer := map[string]any{"name": "MA Lighting", "website": "www.malighting.com"}
	first := request("/api/v1/admin/manufacturers", "manufacturer-create-1", manufacturer)
	if first.Code != http.StatusCreated {
		t.Fatalf("manufacturer create: %d %s", first.Code, first.Body.String())
	}
	var created struct {
		ManufacturerID int    `json:"manufacturer_id"`
		Website        string `json:"website"`
	}
	if err := json.Unmarshal(first.Body.Bytes(), &created); err != nil || created.ManufacturerID <= 0 || created.Website != "https://www.malighting.com" {
		t.Fatalf("manufacturer: %s %v", first.Body.String(), err)
	}
	if replay := request("/api/v1/admin/manufacturers", "manufacturer-create-1", manufacturer); replay.Code != http.StatusCreated {
		t.Fatalf("manufacturer replay: %d %s", replay.Code, replay.Body.String())
	}
	if duplicate := request("/api/v1/admin/manufacturers", "manufacturer-create-2", map[string]any{"name": "ma lighting"}); duplicate.Code != http.StatusConflict {
		t.Fatalf("duplicate manufacturer accepted: %d %s", duplicate.Code, duplicate.Body.String())
	}
	brand := map[string]any{"name": "grandMA3", "manufacturer_id": created.ManufacturerID}
	second := request("/api/v1/admin/brands", "brand-create-1", brand)
	if second.Code != http.StatusCreated {
		t.Fatalf("brand create: %d %s", second.Code, second.Body.String())
	}
	if replay := request("/api/v1/admin/brands", "brand-create-1", brand); replay.Code != http.StatusCreated {
		t.Fatalf("brand replay: %d %s", replay.Code, replay.Body.String())
	}
	if duplicate := request("/api/v1/admin/brands", "brand-create-2", brand); duplicate.Code != http.StatusConflict {
		t.Fatalf("duplicate brand accepted: %d %s", duplicate.Code, duplicate.Body.String())
	}
	var brandID, audits, receipts int
	if err := db.QueryRow(`SELECT brandid FROM brands WHERE name='grandMA3' AND manufacturerid=$1`, created.ManufacturerID).Scan(&brandID); err != nil || brandID <= 0 {
		t.Fatalf("created brand: %d %v", brandID, err)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM audit_log WHERE new_values->>'origin'='MCP/AI'`).Scan(&audits); err != nil || audits != 2 {
		t.Fatalf("audits: %d %v", audits, err)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM warehouse_product_mutation_receipts`).Scan(&receipts); err != nil || receipts != 2 {
		t.Fatalf("receipts: %d %v", receipts, err)
	}
}
