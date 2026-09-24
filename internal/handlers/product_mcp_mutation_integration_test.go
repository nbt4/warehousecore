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
	_ "github.com/lib/pq"
	"warehousecore/internal/middleware"
	"warehousecore/internal/models"
	"warehousecore/internal/repository"
)

func TestWarehouseProductMCPUpdateVersionReceiptAndRollback(t *testing.T) {
	dsn := os.Getenv("WAREHOUSE_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set WAREHOUSE_TEST_DATABASE_URL to a disposable PostgreSQL database ending in _test")
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
	const schema = "warehouse_product_mcp_update_test"
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
		`CREATE TABLE products (
			productid SERIAL PRIMARY KEY, name TEXT NOT NULL, categoryid INT, subcategoryid TEXT, subbiercategoryid TEXT,
			manufacturerid INT, brandid INT, description TEXT, maintenanceinterval INT, itemcostperday FLOAT8,
			weight FLOAT8, height FLOAT8, width FLOAT8, depth FLOAT8, powerconsumption FLOAT8, pos_in_category INT,
			is_accessory BOOLEAN, is_consumable BOOLEAN, count_type_id INT, stock_quantity FLOAT8, min_stock_level FLOAT8,
			generic_barcode TEXT, price_per_unit FLOAT8, product_type TEXT, tracking_mode TEXT, lifecycle_status TEXT,
			product_kind TEXT, model_number TEXT, manufacturer_part_number TEXT, ean TEXT, attributes JSONB,
			updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP)`,
		`CREATE TABLE audit_log (id BIGSERIAL PRIMARY KEY, user_id BIGINT, action TEXT, entity_type TEXT, entity_id TEXT, old_values JSONB, new_values JSONB, ip_address TEXT, user_agent TEXT)`,
		`CREATE TABLE warehouse_product_mutation_receipts (id BIGSERIAL PRIMARY KEY, user_id BIGINT NOT NULL, operation VARCHAR(80) NOT NULL, key_hash CHAR(64) NOT NULL, request_hash CHAR(64) NOT NULL, response JSONB NOT NULL DEFAULT '{}'::jsonb, status_code INTEGER NOT NULL DEFAULT 200, created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP, UNIQUE(user_id,operation,key_hash))`,
		`INSERT INTO products(name,product_type,tracking_mode,lifecycle_status,product_kind,generic_barcode,attributes,updated_at) VALUES ('Mixer','equipment','individual','active','standard','MIX-001','{"ports":4}','2026-09-24T08:00:00.123456')`,
	} {
		if _, err := db.Exec(statement); err != nil {
			t.Fatal(err)
		}
	}
	previousDB := repository.DB
	repository.DB = db
	defer func() { repository.DB = previousDB }()
	request := func(name, barcode, key, version string) *httptest.ResponseRecorder {
		payload := map[string]any{"name": name, "product_type": "equipment", "tracking_mode": "individual", "product_kind": "standard", "generic_barcode": barcode, "attributes": map[string]any{"ports": 4}, "expectedUpdatedAt": version}
		body, err := json.Marshal(payload)
		if err != nil {
			t.Fatal(err)
		}
		r := httptest.NewRequest(http.MethodPut, "/api/v1/admin/products/1", bytes.NewReader(body))
		r = mux.SetURLVars(r, map[string]string{"id": "1"})
		r = r.WithContext(context.WithValue(r.Context(), middleware.UserContextKey, &models.User{UserID: 11, Username: "tester", IsAdmin: true}))
		r.Header.Set("X-Cores-Origin", "MCP/AI")
		r.Header.Set("Idempotency-Key", key)
		w := httptest.NewRecorder()
		UpdateProduct(w, r)
		return w
	}
	const initialVersion = "2026-09-24T08:00:00.123456Z"
	if response := request("Renamed Mixer", "MIX-001", "product-update-no-version", ""); response.Code != http.StatusPreconditionRequired {
		t.Fatalf("missing version accepted: %d %s", response.Code, response.Body.String())
	}
	first := request("Renamed Mixer", "MIX-001", "product-update-test-001", initialVersion)
	if first.Code != http.StatusOK {
		t.Fatalf("update: %d %s", first.Code, first.Body.String())
	}
	var updated struct {
		ProductID int    `json:"product_id"`
		Version   string `json:"updated_at"`
	}
	if err := json.Unmarshal(first.Body.Bytes(), &updated); err != nil || updated.ProductID != 1 || updated.Version == "" || updated.Version == initialVersion {
		t.Fatalf("updated version: %#v, %v", updated, err)
	}
	if replay := request("Renamed Mixer", "MIX-001", "product-update-test-001", initialVersion); replay.Code != http.StatusOK {
		t.Fatalf("idempotent replay: %d %s", replay.Code, replay.Body.String())
	} else {
		var repeated map[string]any
		if err := json.Unmarshal(replay.Body.Bytes(), &repeated); err != nil || repeated["product_id"] != float64(1) || repeated["updated_at"] != updated.Version {
			t.Fatalf("replay differed: %#v, %v", repeated, err)
		}
	}
	if conflict := request("Different Mixer", "MIX-001", "product-update-test-001", initialVersion); conflict.Code != http.StatusConflict {
		t.Fatalf("reused key with changed payload: %d", conflict.Code)
	}
	if stale := request("Stale Mixer", "MIX-001", "product-update-test-002", initialVersion); stale.Code != http.StatusConflict {
		t.Fatalf("stale version accepted: %d %s", stale.Code, stale.Body.String())
	}
	second := request("Cleared Barcode", "", "product-update-test-003", updated.Version)
	if second.Code != http.StatusOK {
		t.Fatalf("barcode clear: %d %s", second.Code, second.Body.String())
	}
	var barcode sql.NullString
	if err := db.QueryRow(`SELECT generic_barcode FROM products WHERE productid=1`).Scan(&barcode); err != nil || barcode.Valid {
		t.Fatalf("barcode was not cleared: %#v, %v", barcode, err)
	}
	for table, want := range map[string]int{"audit_log": 2, "warehouse_product_mutation_receipts": 2} {
		var count int
		if err := db.QueryRow("SELECT COUNT(*) FROM " + table).Scan(&count); err != nil || count != want {
			t.Fatalf("%s count=%d want=%d: %v", table, count, want, err)
		}
	}
	if _, err := db.Exec("DROP TABLE audit_log"); err != nil {
		t.Fatal(err)
	}
	var currentVersion string
	if err := db.QueryRow(`SELECT to_char(updated_at,'YYYY-MM-DD"T"HH24:MI:SS.US"Z"') FROM products WHERE productid=1`).Scan(&currentVersion); err != nil {
		t.Fatal(err)
	}
	if failure := request("Lost Audit", "", "product-update-test-004", currentVersion); failure.Code != http.StatusInternalServerError {
		t.Fatalf("missing audit must roll back: %d %s", failure.Code, failure.Body.String())
	}
	var name string
	if err := db.QueryRow("SELECT name FROM products WHERE productid=$1", strconv.Itoa(1)).Scan(&name); err != nil || name != "Cleared Barcode" {
		t.Fatalf("failed update changed product: %q, %v", name, err)
	}
}
