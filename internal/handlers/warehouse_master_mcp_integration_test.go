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

func TestWarehouseMCPStandaloneMasterCreation(t *testing.T) {
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
		`CREATE TABLE categories(categoryid SERIAL PRIMARY KEY,name VARCHAR(100),abbreviation VARCHAR(10))`,
		`CREATE TABLE subcategories(subcategoryid VARCHAR(50) PRIMARY KEY,name VARCHAR(100),abbreviation VARCHAR(10),categoryid INT REFERENCES categories(categoryid))`,
		`CREATE TABLE subbiercategories(subbiercategoryid VARCHAR(50) PRIMARY KEY,name VARCHAR(100),abbreviation VARCHAR(10),subcategoryid VARCHAR(50) REFERENCES subcategories(subcategoryid))`,
		`CREATE TABLE storage_zones(zone_id SERIAL PRIMARY KEY,code VARCHAR(50) UNIQUE,barcode VARCHAR(255),name VARCHAR(100),type TEXT,description TEXT,parent_zone_id INT REFERENCES storage_zones(zone_id),capacity NUMERIC,is_active BOOLEAN,location_kind TEXT,process_role TEXT,operational_status TEXT,is_storable BOOLEAN,pick_sequence INT,capacity_mode TEXT,max_weight_kg NUMERIC,max_volume_m3 NUMERIC,inventory_frequency_days INT,next_count_at TIMESTAMP)`,
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
		switch path {
		case "/api/v1/admin/manufacturers":
			CreateManufacturer(w, r)
		case "/api/v1/admin/brands":
			CreateBrand(w, r)
		case "/api/v1/admin/categories":
			CreateCategory(w, r)
		case "/api/v1/admin/subcategories":
			CreateSubcategory(w, r)
		case "/api/v1/admin/subbiercategories":
			CreateSubbiercategory(w, r)
		case "/api/v1/admin/warehouse/locations":
			CreateWarehouseLocation(w, r)
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
	category := request("/api/v1/admin/categories", "category-create-1", map[string]any{"name": "Lighting", "abbreviation": "LT"})
	if category.Code != http.StatusCreated {
		t.Fatalf("category create: %d %s", category.Code, category.Body.String())
	}
	var top struct {
		ID int `json:"category_id"`
	}
	if err := json.Unmarshal(category.Body.Bytes(), &top); err != nil || top.ID <= 0 {
		t.Fatalf("category response: %s %v", category.Body.String(), err)
	}
	if duplicate := request("/api/v1/admin/categories", "category-create-2", map[string]any{"name": "lighting", "abbreviation": "LT"}); duplicate.Code != http.StatusConflict {
		t.Fatalf("duplicate category accepted: %d %s", duplicate.Code, duplicate.Body.String())
	}
	sub := request("/api/v1/admin/subcategories", "subcategory-create-1", map[string]any{"name": "Control", "category_id": top.ID})
	if sub.Code != http.StatusCreated {
		t.Fatalf("subcategory create: %d %s", sub.Code, sub.Body.String())
	}
	var secondLevel struct {
		ID string `json:"subcategory_id"`
	}
	if err := json.Unmarshal(sub.Body.Bytes(), &secondLevel); err != nil || secondLevel.ID == "" {
		t.Fatalf("subcategory response: %s %v", sub.Body.String(), err)
	}
	if replay := request("/api/v1/admin/subcategories", "subcategory-create-1", map[string]any{"name": "Control", "category_id": top.ID}); replay.Code != http.StatusCreated {
		t.Fatalf("subcategory replay: %d %s", replay.Code, replay.Body.String())
	}
	if conflict := request("/api/v1/admin/subcategories", "subcategory-create-1", map[string]any{"name": "Different", "category_id": top.ID}); conflict.Code != http.StatusConflict {
		t.Fatalf("subcategory idempotency conflict: %d %s", conflict.Code, conflict.Body.String())
	}
	if missingParent := request("/api/v1/admin/subcategories", "subcategory-create-2", map[string]any{"name": "Missing", "category_id": 999999}); missingParent.Code != http.StatusNotFound {
		t.Fatalf("subcategory missing parent: %d %s", missingParent.Code, missingParent.Body.String())
	}
	third := request("/api/v1/admin/subbiercategories", "third-category-create-1", map[string]any{"name": "Network", "subcategory_id": secondLevel.ID})
	if third.Code != http.StatusCreated {
		t.Fatalf("third category create: %d %s", third.Code, third.Body.String())
	}
	var thirdLevel struct {
		ID string `json:"subbiercategory_id"`
	}
	if err := json.Unmarshal(third.Body.Bytes(), &thirdLevel); err != nil || thirdLevel.ID == "" {
		t.Fatalf("third category response: %s %v", third.Body.String(), err)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM audit_log WHERE new_values->>'origin'='MCP/AI'`).Scan(&audits); err != nil || audits != 5 {
		t.Fatalf("master audits: %d %v", audits, err)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM warehouse_product_mutation_receipts`).Scan(&receipts); err != nil || receipts != 5 {
		t.Fatalf("master receipts: %d %v", receipts, err)
	}
	location := request("/api/v1/admin/warehouse/locations", "location-create-1", map[string]any{"code": "MAIN", "name": "Main Warehouse", "type": "warehouse", "is_storable": true})
	if location.Code != http.StatusCreated {
		t.Fatalf("location create: %d %s", location.Code, location.Body.String())
	}
	var zone struct {
		ID int `json:"zone_id"`
	}
	if err := json.Unmarshal(location.Body.Bytes(), &zone); err != nil || zone.ID <= 0 {
		t.Fatalf("location response: %s %v", location.Body.String(), err)
	}
	if replay := request("/api/v1/admin/warehouse/locations", "location-create-1", map[string]any{"code": "MAIN", "name": "Main Warehouse", "type": "warehouse", "is_storable": true}); replay.Code != http.StatusCreated {
		t.Fatalf("location replay: %d %s", replay.Code, replay.Body.String())
	}
	if duplicate := request("/api/v1/admin/warehouse/locations", "location-create-2", map[string]any{"code": "main", "name": "Other", "is_storable": true}); duplicate.Code != http.StatusConflict {
		t.Fatalf("duplicate location accepted: %d %s", duplicate.Code, duplicate.Body.String())
	}
	child := request("/api/v1/admin/warehouse/locations", "location-create-3", map[string]any{"code": "SHELF-A", "name": "Shelf A", "parent_zone_id": zone.ID, "is_storable": true})
	if child.Code != http.StatusCreated {
		t.Fatalf("child location create: %d %s", child.Code, child.Body.String())
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM audit_log WHERE new_values->>'origin'='MCP/AI'`).Scan(&audits); err != nil || audits != 7 {
		t.Fatalf("location audits: %d %v", audits, err)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM warehouse_product_mutation_receipts`).Scan(&receipts); err != nil || receipts != 7 {
		t.Fatalf("location receipts: %d %v", receipts, err)
	}
}
