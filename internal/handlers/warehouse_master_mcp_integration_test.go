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
		`CREATE TABLE products(productid SERIAL PRIMARY KEY,name TEXT,manufacturerid INT,brandid INT)`,
		`CREATE TABLE categories(categoryid SERIAL PRIMARY KEY,name VARCHAR(100),abbreviation VARCHAR(10))`,
		`CREATE TABLE subcategories(subcategoryid VARCHAR(50) PRIMARY KEY,name VARCHAR(100),abbreviation VARCHAR(10),categoryid INT REFERENCES categories(categoryid))`,
		`CREATE TABLE subbiercategories(subbiercategoryid VARCHAR(50) PRIMARY KEY,name VARCHAR(100),abbreviation VARCHAR(10),subcategoryid VARCHAR(50) REFERENCES subcategories(subcategoryid))`,
		`CREATE TABLE storage_zones(zone_id SERIAL PRIMARY KEY,code VARCHAR(50) UNIQUE,barcode VARCHAR(255),name VARCHAR(100),type TEXT,description TEXT,parent_zone_id INT REFERENCES storage_zones(zone_id),capacity NUMERIC,is_active BOOLEAN,location_kind TEXT,process_role TEXT,operational_status TEXT,is_storable BOOLEAN,pick_sequence INT,capacity_mode TEXT,max_weight_kg NUMERIC,max_volume_m3 NUMERIC,inventory_frequency_days INT,next_count_at TIMESTAMP,last_counted_at TIMESTAMP,updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP)`,
		`CREATE TABLE warehouse_schema_migrations(version TEXT PRIMARY KEY)`,
		`CREATE TABLE devices(deviceid TEXT PRIMARY KEY,zone_id INT,lifecycle_status TEXT,status TEXT)`,
		`CREATE TABLE cases(caseid INT PRIMARY KEY,zone_id INT)`,
		`CREATE TABLE product_locations(product_id INT,zone_id INT,quantity NUMERIC)`,
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
	for i := 0; i < 2; i++ {
		if err := EnsureWarehouseLocationVersionSchema(); err != nil {
			t.Fatal(err)
		}
	}
	version := func() string {
		var v string
		if err := db.QueryRow(`SELECT to_char(updated_at,'YYYY-MM-DD"T"HH24:MI:SS.US"Z"') FROM storage_zones WHERE zone_id=$1`, zone.ID).Scan(&v); err != nil {
			t.Fatal(err)
		}
		return v
	}
	put := func(key string, body map[string]any, admin bool) *httptest.ResponseRecorder {
		encoded, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		r := httptest.NewRequest(http.MethodPut, "/api/v1/admin/warehouse/locations/1", bytes.NewReader(encoded))
		r = mux.SetURLVars(r, map[string]string{"id": fmt.Sprint(zone.ID)})
		r = r.WithContext(context.WithValue(r.Context(), middleware.UserContextKey, &models.User{UserID: 11, Username: "tester", IsAdmin: admin}))
		r.Header.Set("X-Cores-Origin", "MCP/AI")
		r.Header.Set("Idempotency-Key", key)
		w := httptest.NewRecorder()
		UpdateWarehouseLocation(w, r)
		return w
	}
	body := map[string]any{"code": "MAIN", "barcode": "LOC-MAIN", "name": "Main Warehouse Updated", "type": "warehouse", "location_kind": "area", "process_role": "storage", "operational_status": "available", "capacity_mode": "item_count", "is_storable": true, "expected_updated_at": version()}
	assertStatus := func(w *httptest.ResponseRecorder, want int) {
		t.Helper()
		if w.Code != want {
			t.Fatalf("status %d want %d: %s", w.Code, want, w.Body.String())
		}
	}
	assertStatus(put("location-update-no-admin", body, false), http.StatusForbidden)
	original := version()
	firstUpdate := put("location-update-1", body, true)
	assertStatus(firstUpdate, http.StatusOK)
	if version() == original {
		t.Fatal("location version did not advance")
	}
	replay := put("location-update-1", body, true)
	assertStatus(replay, http.StatusOK)
	var firstResult, replayResult map[string]any
	if err := json.Unmarshal(firstUpdate.Body.Bytes(), &firstResult); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(replay.Body.Bytes(), &replayResult); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(firstResult, replayResult) {
		t.Fatal("replay changed response")
	}
	assertStatus(put("location-update-stale", body, true), http.StatusConflict)
	body["name"] = "Different Name"
	assertStatus(put("location-update-1", body, true), http.StatusConflict)
	body["expected_updated_at"] = ""
	assertStatus(put("location-update-no-version", body, true), http.StatusPreconditionRequired)
	body["expected_updated_at"] = version()
	body["parent_zone_id"] = zone.ID
	assertStatus(put("location-update-cycle-self", body, true), http.StatusConflict)
	var childID int
	if err := db.QueryRow(`SELECT zone_id FROM storage_zones WHERE code='SHELF-A'`).Scan(&childID); err != nil {
		t.Fatal(err)
	}
	body["parent_zone_id"] = childID
	assertStatus(put("location-update-cycle-child", body, true), http.StatusConflict)
	delete(body, "parent_zone_id")
	body["code"] = "SHELF-A"
	assertStatus(put("location-update-duplicate", body, true), http.StatusConflict)
	body["code"] = "MAIN"
	body["barcode"] = "LOC-SHELF-A"
	assertStatus(put("location-update-duplicate-scan", body, true), http.StatusConflict)
	body["barcode"] = "LOC-MAIN"
	body["operational_status"] = "archived"
	assertStatus(put("location-update-archive", body, true), http.StatusConflict)
	body["operational_status"] = "available"
	if _, err := db.Exec(`INSERT INTO devices VALUES('DEV-1',$1,'active','in_storage'),('DEV-2',$1,'active','checked_out'),('DEV-3',$1,'archived','in_storage');`, zone.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO product_locations VALUES(1,$1,2)`, zone.ID); err != nil {
		t.Fatal(err)
	}
	body["is_storable"] = false
	assertStatus(put("location-update-nonstorable", body, true), http.StatusConflict)
	body["is_storable"] = true
	body["capacity"] = 2
	assertStatus(put("location-update-full", body, true), http.StatusConflict)
	body["capacity"] = 3.5
	assertStatus(put("location-update-fraction", body, true), http.StatusBadRequest)
	body["capacity"] = 3
	body["inventory_frequency_days"] = 7
	assertStatus(put("location-update-capacity", body, true), http.StatusOK)
	var nextBefore string
	if err := db.QueryRow(`SELECT next_count_at::text FROM storage_zones WHERE zone_id=$1`, zone.ID).Scan(&nextBefore); err != nil {
		t.Fatal(err)
	}
	body["expected_updated_at"] = version()
	body["name"] = "Metadata Edit"
	assertStatus(put("location-update-metadata", body, true), http.StatusOK)
	var nextAfter string
	if err := db.QueryRow(`SELECT next_count_at::text FROM storage_zones WHERE zone_id=$1`, zone.ID).Scan(&nextAfter); err != nil || nextAfter != nextBefore {
		t.Fatalf("count schedule moved: %s -> %s %v", nextBefore, nextAfter, err)
	}
	body["expected_updated_at"] = version()
	assertStatus(put("location-update-noop", body, true), http.StatusConflict)
	if _, err := db.Exec(`UPDATE storage_zones SET description='UI edit' WHERE zone_id=$1`, zone.ID); err != nil {
		t.Fatal(err)
	}
	body["name"] = "After UI Edit"
	assertStatus(put("location-update-after-ui", body, true), http.StatusConflict)
	body["expected_updated_at"] = version()
	body["description"] = "UI edit"
	if _, err := db.Exec(`CREATE FUNCTION reject_location_audit() RETURNS TRIGGER AS $$ BEGIN RAISE EXCEPTION 'forced audit failure'; END $$ LANGUAGE plpgsql; CREATE TRIGGER fail_audit BEFORE INSERT ON audit_log FOR EACH ROW EXECUTE FUNCTION reject_location_audit()`); err != nil {
		t.Fatal(err)
	}
	assertStatus(put("location-update-audit-failure", body, true), http.StatusInternalServerError)
	if body["expected_updated_at"] != version() {
		t.Fatal("failed audit did not roll back location")
	}
	var failedReceipts int
	if err := db.QueryRow(`SELECT COUNT(*) FROM warehouse_product_mutation_receipts WHERE operation='location.update'`).Scan(&failedReceipts); err != nil || failedReceipts != 3 {
		t.Fatalf("rollback/replay receipt count %d %v", failedReceipts, err)
	}
	if _, err := db.Exec(`DROP TRIGGER fail_audit ON audit_log`); err != nil {
		t.Fatal(err)
	}
	var updates int
	if err := db.QueryRow(`SELECT COUNT(*) FROM audit_log WHERE action='storage_zone.update' AND old_values->>'name' IS NOT NULL AND new_values->>'origin'='MCP/AI'`).Scan(&updates); err != nil || updates != 3 {
		t.Fatalf("update audit count %d %v", updates, err)
	}

	for i := 0; i < 2; i++ {
		if err := EnsureWarehouseMasterVersionSchema(); err != nil {
			t.Fatal(err)
		}
	}
	masterVersion := func(entity string, id int) string {
		var value string
		query := `SELECT to_char(updated_at,'YYYY-MM-DD"T"HH24:MI:SS.US"Z"') FROM manufacturer WHERE manufacturerid=$1`
		if entity == "brand" {
			query = `SELECT to_char(updated_at,'YYYY-MM-DD"T"HH24:MI:SS.US"Z"') FROM brands WHERE brandid=$1`
		}
		if err := db.QueryRow(query, id).Scan(&value); err != nil {
			t.Fatal(err)
		}
		return value
	}
	putMaster := func(entity string, id int, key string, body map[string]any, admin bool) *httptest.ResponseRecorder {
		encoded, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		r := httptest.NewRequest(http.MethodPut, "/api/v1/admin/"+entity+"s/"+fmt.Sprint(id), bytes.NewReader(encoded))
		r = mux.SetURLVars(r, map[string]string{"id": fmt.Sprint(id)})
		r = r.WithContext(context.WithValue(r.Context(), middleware.UserContextKey, &models.User{UserID: 11, Username: "tester", IsAdmin: admin}))
		r.Header.Set("X-Cores-Origin", "MCP/AI")
		r.Header.Set("Idempotency-Key", key)
		w := httptest.NewRecorder()
		if entity == "manufacturer" {
			UpdateManufacturer(w, r)
		} else {
			UpdateBrand(w, r)
		}
		return w
	}
	for _, target := range []struct {
		entity string
		id     int
		fields map[string]any
	}{
		{"manufacturer", created.ManufacturerID, map[string]any{"name": "MA Lighting Updated", "website": "www.malighting.com"}},
		{"brand", brandID, map[string]any{"name": "grandMA3 Updated", "manufacturer_id": created.ManufacturerID}},
	} {
		body := target.fields
		entity := target.entity
		id := target.id
		oldVersion := masterVersion(entity, id)
		body["expected_updated_at"] = oldVersion
		assertStatus(putMaster(entity, id, entity+"-update-no-admin", body, false), http.StatusForbidden)
		body["expected_updated_at"] = ""
		assertStatus(putMaster(entity, id, entity+"-update-no-version", body, true), http.StatusPreconditionRequired)
		body["expected_updated_at"] = oldVersion
		first := putMaster(entity, id, entity+"-update-1", body, true)
		assertStatus(first, http.StatusOK)
		if masterVersion(entity, id) == oldVersion {
			t.Fatal("master version did not advance")
		}
		replay := putMaster(entity, id, entity+"-update-1", body, true)
		assertStatus(replay, http.StatusOK)
		var firstResult, replayResult map[string]any
		if err := json.Unmarshal(first.Body.Bytes(), &firstResult); err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(replay.Body.Bytes(), &replayResult); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(firstResult, replayResult) {
			t.Fatal("master replay changed response")
		}
		assertStatus(putMaster(entity, id, entity+"-update-stale", body, true), http.StatusConflict)
		body["name"] = "Other Name"
		assertStatus(putMaster(entity, id, entity+"-update-1", body, true), http.StatusConflict)
		body["expected_updated_at"] = masterVersion(entity, id)
		body["name"] = target.entity + " unique updated"
		if _, err := db.Exec(`CREATE TRIGGER fail_master_audit BEFORE INSERT ON audit_log FOR EACH ROW EXECUTE FUNCTION reject_location_audit()`); err != nil {
			t.Fatal(err)
		}
		assertStatus(putMaster(entity, id, entity+"-update-audit-failure", body, true), http.StatusInternalServerError)
		if masterVersion(entity, id) != body["expected_updated_at"] {
			t.Fatal("audit failure did not roll back master update")
		}
		if _, err := db.Exec(`DROP TRIGGER fail_master_audit ON audit_log`); err != nil {
			t.Fatal(err)
		}
		var updateCount, receiptCount int
		if err := db.QueryRow(`SELECT COUNT(*) FROM audit_log WHERE action=$1 AND old_values->>'name' IS NOT NULL AND new_values->>'origin'='MCP/AI'`, entity+".update").Scan(&updateCount); err != nil || updateCount != 1 {
			t.Fatalf("master audits %d %v", updateCount, err)
		}
		if err := db.QueryRow(`SELECT COUNT(*) FROM warehouse_product_mutation_receipts WHERE operation=$1`, entity+".update").Scan(&receiptCount); err != nil || receiptCount != 1 {
			t.Fatalf("master receipts %d %v", receiptCount, err)
		}
	}
	var otherManufacturer int
	if err := db.QueryRow(`INSERT INTO manufacturer(name) VALUES('Robe Lighting') RETURNING manufacturerid`).Scan(&otherManufacturer); err != nil {
		t.Fatal(err)
	}
	manufacturerBody := map[string]any{"name": "robe lighting", "website": nil, "expected_updated_at": masterVersion("manufacturer", created.ManufacturerID)}
	assertStatus(putMaster("manufacturer", created.ManufacturerID, "manufacturer-duplicate-update", manufacturerBody, true), http.StatusConflict)
	manufacturerBody["name"] = "MA Lighting Updated"
	manufacturerBody["website"] = "ftp://invalid.example"
	assertStatus(putMaster("manufacturer", created.ManufacturerID, "manufacturer-invalid-website", manufacturerBody, true), http.StatusBadRequest)
	manufacturerBody["website"] = ""
	assertStatus(putMaster("manufacturer", created.ManufacturerID, "manufacturer-clear-website", manufacturerBody, true), http.StatusOK)
	var storedWebsite sql.NullString
	if err := db.QueryRow(`SELECT website FROM manufacturer WHERE manufacturerid=$1`, created.ManufacturerID).Scan(&storedWebsite); err != nil || storedWebsite.Valid {
		t.Fatalf("website not cleared: %#v %v", storedWebsite, err)
	}
	brandBody := map[string]any{"name": "grandMA3 Updated", "manufacturer_id": otherManufacturer, "expected_updated_at": masterVersion("brand", brandID)}
	if _, err := db.Exec(`INSERT INTO products(name,manufacturerid,brandid) VALUES('Console',$1,$2)`, created.ManufacturerID, brandID); err != nil {
		t.Fatal(err)
	}
	assertStatus(putMaster("brand", brandID, "brand-conflicting-products", brandBody, true), http.StatusConflict)
	brandBody["manufacturer_id"] = 999999
	assertStatus(putMaster("brand", brandID, "brand-missing-manufacturer", brandBody, true), http.StatusNotFound)
	brandBody["manufacturer_id"] = otherManufacturer
	if _, err := db.Exec(`DELETE FROM products`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO brands(name,manufacturerid) VALUES('grandMA3 Updated',$1)`, otherManufacturer); err != nil {
		t.Fatal(err)
	}
	assertStatus(putMaster("brand", brandID, "brand-duplicate-update", brandBody, true), http.StatusConflict)
	brandBody["name"] = "Reassigned Brand"
	assertStatus(putMaster("brand", brandID, "brand-reassign-update", brandBody, true), http.StatusOK)
	brandBody["expected_updated_at"] = masterVersion("brand", brandID)
	assertStatus(putMaster("brand", brandID, "brand-update-noop", brandBody, true), http.StatusConflict)
	if _, err := db.Exec(`UPDATE brands SET name='Legacy UI edit' WHERE brandid=$1`, brandID); err != nil {
		t.Fatal(err)
	}
	brandBody["name"] = "After UI edit"
	assertStatus(putMaster("brand", brandID, "brand-stale-ui", brandBody, true), http.StatusConflict)
	brandBody["expected_updated_at"] = masterVersion("brand", brandID)
	brandBody["manufacturer_id"] = nil
	assertStatus(putMaster("brand", brandID, "brand-clear-manufacturer", brandBody, true), http.StatusOK)
	if _, err := db.Exec(`UPDATE manufacturer SET name='Legacy manufacturer edit' WHERE manufacturerid=$1`, created.ManufacturerID); err != nil {
		t.Fatal(err)
	}
	assertStatus(putMaster("manufacturer", created.ManufacturerID, "manufacturer-stale-ui", manufacturerBody, true), http.StatusConflict)

	// Reproduce the startup conflict with both legacy aliases and user-created
	// translated categories; their IDs, abbreviations and references must survive.
	if _, err := db.Exec(`CREATE UNIQUE INDEX uq_categories_name_normalized ON categories(lower(trim(name))); ALTER TABLE products ADD COLUMN categoryid INT REFERENCES categories(categoryid);
INSERT INTO categories(name,abbreviation) VALUES('sound','SND'),('Ton','USER_TON'),('light','LGT'),('Licht','USER_LIC'),('stage','STG'),('other','OTR'),('Sonstiges','USER_SON');
INSERT INTO products(name,categoryid) SELECT 'Legacy category fixture',categoryid FROM categories WHERE name='sound'`); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if _, err := db.Exec(legacyCategoryNormalizationSQL); err != nil {
			t.Fatalf("category normalization restart: %v", err)
		}
	}
	var retained int
	if err := db.QueryRow(`SELECT count(*) FROM categories WHERE lower(trim(name)) IN ('sound','ton','light','licht','other','sonstiges')`).Scan(&retained); err != nil || retained != 6 {
		t.Fatalf("category identities lost: %d %v", retained, err)
	}
	var linkedCategory, abbreviation string
	if err := db.QueryRow(`SELECT c.name FROM products p JOIN categories c ON c.categoryid=p.categoryid WHERE p.name='Legacy category fixture'`).Scan(&linkedCategory); err != nil || linkedCategory != "sound" {
		t.Fatalf("category reference changed: %s %v", linkedCategory, err)
	}
	if err := db.QueryRow(`SELECT abbreviation FROM categories WHERE name='Ton'`).Scan(&abbreviation); err != nil || abbreviation != "USER_TON" {
		t.Fatalf("target category changed: %s %v", abbreviation, err)
	}
	if err := db.QueryRow(`SELECT count(*) FROM categories WHERE name='Bühne' AND abbreviation='BUE'`).Scan(&retained); err != nil || retained != 1 {
		t.Fatalf("safe category alias not translated: %d %v", retained, err)
	}
	if _, err := db.Exec(`CREATE UNIQUE INDEX uq_brands_name_normalized ON brands(lower(trim(name)))`); err != nil {
		t.Fatal(err)
	}
	brandMigration, err := os.ReadFile("../../migrations/048_brand_manufacturer_identity.sql")
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if _, err := db.Exec(string(brandMigration)); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.Exec(`INSERT INTO brands(name,manufacturerid) VALUES('Shared Name',$1),('Shared Name',$2)`, created.ManufacturerID, otherManufacturer); err != nil {
		t.Fatalf("brand identity must include manufacturer: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO brands(name,manufacturerid) VALUES(' shared name ',$1)`, created.ManufacturerID); err == nil {
		t.Fatal("duplicate brand within manufacturer accepted")
	}
	if _, err := db.Exec(`INSERT INTO brands(name) VALUES('Unassigned Name')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO brands(name) VALUES('unassigned name')`); err == nil {
		t.Fatal("duplicate unassigned brand accepted")
	}

}
