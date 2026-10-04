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
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	jwt "github.com/golang-jwt/jwt/v5"
	"warehousecore/internal/middleware"
	"warehousecore/internal/models"
	"warehousecore/internal/repository"
)

const productImportFixtureSQL = `CREATE TABLE products (
			productid SERIAL PRIMARY KEY, name TEXT NOT NULL, categoryid INT, subcategoryid TEXT, subbiercategoryid TEXT,
			manufacturerid INT, brandid INT, description TEXT, maintenanceinterval INT, itemcostperday FLOAT8,
			weight FLOAT8, height FLOAT8, width FLOAT8, depth FLOAT8, powerconsumption FLOAT8, pos_in_category INT,
			is_accessory BOOLEAN, is_consumable BOOLEAN, count_type_id INT, stock_quantity FLOAT8, min_stock_level FLOAT8,
			generic_barcode TEXT, price_per_unit FLOAT8, product_type TEXT, tracking_mode TEXT, lifecycle_status TEXT,
			product_kind TEXT, model_number TEXT, manufacturer_part_number TEXT, ean TEXT, attributes JSONB,
			updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP, product_code TEXT NOT NULL DEFAULT 'PRD-TEST');

CREATE TABLE users(userid INT PRIMARY KEY,is_active BOOL,is_admin BOOL);
INSERT INTO users VALUES(11,true,true);
CREATE TABLE manufacturer(manufacturerid SERIAL PRIMARY KEY,name TEXT,website TEXT,lifecycle_status TEXT DEFAULT 'active',updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP);
CREATE TABLE brands(brandid SERIAL PRIMARY KEY,name TEXT,manufacturerid INT,lifecycle_status TEXT DEFAULT 'active',updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP);
CREATE TABLE categories(categoryid SERIAL PRIMARY KEY,name TEXT,abbreviation TEXT,lifecycle_status TEXT DEFAULT 'active',updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP);
CREATE TABLE subcategories(subcategoryid TEXT PRIMARY KEY,name TEXT,abbreviation TEXT,categoryid INT,lifecycle_status TEXT DEFAULT 'active',updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP);
CREATE TABLE subbiercategories(subbiercategoryid TEXT PRIMARY KEY,name TEXT,abbreviation TEXT,subcategoryid TEXT,lifecycle_status TEXT DEFAULT 'active',updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP);
CREATE TABLE count_types(count_type_id SERIAL PRIMARY KEY,name TEXT,abbreviation TEXT);
INSERT INTO count_types(name,abbreviation) VALUES('Piece','pc');
CREATE TABLE storage_zones(zone_id SERIAL PRIMARY KEY,parent_zone_id INT,name TEXT,code TEXT,is_active BOOL,is_storable BOOL,operational_status TEXT,capacity NUMERIC,capacity_mode TEXT,max_weight_kg NUMERIC,max_volume_m3 NUMERIC,inventory_frequency_days INT,last_counted_at TIMESTAMP,next_count_at TIMESTAMP,updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,profile_id INT);
CREATE TABLE location_profiles(profile_id INT PRIMARY KEY,allow_devices BOOL,allow_quantity_products BOOL,allow_cases BOOL,allow_mixed_products BOOL,allow_cycle_count BOOL,updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP);
INSERT INTO location_profiles VALUES(1,true,true,true,true,true,CURRENT_TIMESTAMP);
INSERT INTO storage_zones(name,code,is_active,is_storable,operational_status,capacity,profile_id) VALUES('Shelf','SHELF',true,true,'available',5.5,1);
CREATE TABLE devices(deviceid TEXT PRIMARY KEY DEFAULT gen_random_uuid()::text,productid INT,status TEXT,condition_status TEXT,current_location TEXT,zone_id INT,lifecycle_status TEXT DEFAULT 'active');
CREATE TABLE cases(caseid INT PRIMARY KEY,zone_id INT,lifecycle_status TEXT);
CREATE TABLE product_locations(product_id INT,zone_id INT,quantity NUMERIC,updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP);
CREATE TABLE proc_products(id SERIAL PRIMARY KEY,sku TEXT,name TEXT,active BOOL,updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP);
CREATE TABLE core_product_links(id SERIAL PRIMARY KEY,procurement_product_id INT UNIQUE,warehouse_product_id INT UNIQUE,link_method TEXT,linked_by BIGINT,linked_by_name TEXT);
CREATE TABLE inventory_identifiers(entity_type TEXT,entity_key TEXT,code TEXT,identifier_kind TEXT,active BOOL,UNIQUE(entity_type,entity_key,identifier_kind));
CREATE TABLE audit_log(id BIGSERIAL PRIMARY KEY,user_id BIGINT,action TEXT,entity_type TEXT,entity_id TEXT,old_values JSONB,new_values JSONB,ip_address TEXT,user_agent TEXT);
CREATE TABLE warehouse_product_mutation_receipts(id BIGSERIAL PRIMARY KEY,user_id BIGINT,operation TEXT,key_hash TEXT,request_hash TEXT,response JSONB DEFAULT '{}'::jsonb,status_code INT,UNIQUE(user_id,operation,key_hash));
`

func TestWarehouseProductBatchAtomicContextAndReplay(t *testing.T) {
	dsn := os.Getenv("WAREHOUSE_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("disposable _test PostgreSQL required")
	}
	parsed, err := url.Parse(dsn)
	if err != nil || !strings.HasSuffix(parsed.Path, "_test") {
		t.Fatal("owned test database required")
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
	exec(`DROP SCHEMA IF EXISTS warehouse_product_import_test CASCADE;CREATE SCHEMA warehouse_product_import_test;SET search_path TO warehouse_product_import_test`)
	defer db.Exec(`DROP SCHEMA warehouse_product_import_test CASCADE`)
	exec(productImportFixtureSQL)
	old := repository.DB
	repository.DB = db
	defer func() { repository.DB = old }()
	t.Setenv("CORES_JWT_SECRET", strings.Repeat("s", 48))
	call := func(body, key, scope string, want int) *httptest.ResponseRecorder {
		t.Helper()
		r := httptest.NewRequest(http.MethodPost, "/batch", strings.NewReader(body))
		r.Header.Set("X-Cores-Origin", "MCP/AI")
		r.Header.Set("Idempotency-Key", key)
		token, e := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{"uid": 11, "mcp_scope": scope, "exp": time.Now().Add(time.Minute).Unix()}).SignedString([]byte(os.Getenv("CORES_JWT_SECRET")))
		if e != nil {
			t.Fatal(e)
		}
		r.AddCookie(&http.Cookie{Name: "cores_token", Value: token})
		r = r.WithContext(context.WithValue(r.Context(), middleware.UserContextKey, &models.User{UserID: 11, IsActive: true, IsAdmin: true, Username: "Import admin"}))
		w := httptest.NewRecorder()
		CreateProductsBulkMCP(w, r)
		if w.Code != want {
			t.Fatalf("got %d want %d: %s", w.Code, want, w.Body.String())
		}
		return w
	}
	counts := func(products, masters, devices, locations, audits, receipts int) {
		t.Helper()
		for table, want := range map[string]int{"products": products, "manufacturer": masters, "categories": masters, "devices": devices, "product_locations": locations, "audit_log": audits, "warehouse_product_mutation_receipts": receipts} {
			var n int
			if e := db.QueryRow("SELECT count(*) FROM " + table).Scan(&n); e != nil || n != want {
				t.Fatalf("%s got %d want %d: %v", table, n, want, e)
			}
		}
	}
	input := map[string]any{"products": []map[string]any{
		{"name": "Batch mixer", "manufacturer_name_input": "Batch manufacturer", "category_name_input": "Batch category", "category_abbreviation_input": "BATCH", "description": "Digital mixer", "model_number": "MX1", "initial_device_quantity": 2, "initial_zone_id": 1, "weight": 4.5, "attributes": map[string]any{"ports": 4}},
		{"name": "Batch tape", "manufacturer_name_input": "Batch manufacturer", "category_name_input": "Batch category", "category_abbreviation_input": "BATCH", "description": "Tape", "model_number": "T1", "product_type": "consumable", "tracking_mode": "quantity", "count_type_id": 1, "stock_quantity": 3.5, "initial_zone_id": 1, "generic_barcode": "TAPE-UNIQUE"},
	}}
	encode := func(v any) string {
		t.Helper()
		raw, e := json.Marshal(v)
		if e != nil {
			t.Fatal(e)
		}
		return string(raw)
	}
	prepare := func(v map[string]any) map[string]any {
		t.Helper()
		v["preview"] = true
		var preview map[string]any
		w := call(encode(v), "", "cores:warehouse:create", 200)
		if json.Unmarshal(w.Body.Bytes(), &preview) != nil {
			t.Fatal("invalid preview")
		}
		v["preview"] = false
		v["expected_context"] = preview["expected_context"]
		v["confirmation_text"] = preview["required_confirmation_text"]
		return preview
	}
	call(encode(input), "", "cores:warehouse:update", 403)
	counts(0, 0, 0, 0, 0, 0)
	preview := prepare(input)
	if preview["ready"] != true {
		t.Fatalf("not ready %#v", preview)
	}
	counts(0, 0, 0, 0, 0, 0)
	// Pure preview must not consume any generated business or audit sequence.
	for _, sequence := range []string{"products_productid_seq", "manufacturer_manufacturerid_seq", "categories_categoryid_seq", "audit_log_id_seq", "warehouse_product_mutation_receipts_id_seq"} {
		var called bool
		if e := db.QueryRow("SELECT is_called FROM " + sequence).Scan(&called); e != nil || called {
			t.Fatalf("preview consumed %s: %v", sequence, e)
		}
	}
	input["confirm_creation"] = true
	call(encode(input), "", "cores:warehouse:create", 428)
	counts(0, 0, 0, 0, 0, 0)
	saved := input["confirmation_text"]
	input["confirmation_text"] = "WRONG"
	call(encode(input), "batch-wrong-phrase", "cores:warehouse:create", 428)
	input["confirmation_text"] = saved
	// A profile change invalidates the final context even if capacity still fits.
	exec(`UPDATE location_profiles SET updated_at=updated_at+interval '1 microsecond'`)
	call(encode(input), "batch-stale", "cores:warehouse:create", 409)
	counts(0, 0, 0, 0, 0, 0)
	prepare(input)
	exec(`CREATE FUNCTION reject_second_product_audit() RETURNS TRIGGER AS $$ BEGIN IF NEW.new_values#>>'{after,name}'='Batch tape' THEN RAISE EXCEPTION 'audit failure';END IF;RETURN NEW;END $$ LANGUAGE plpgsql;CREATE TRIGGER reject_second_product_audit BEFORE INSERT ON audit_log FOR EACH ROW EXECUTE FUNCTION reject_second_product_audit()`)
	call(encode(input), "batch-create-atomic", "cores:warehouse:create", 500)
	counts(0, 0, 0, 0, 0, 0)
	exec(`DROP TRIGGER reject_second_product_audit ON audit_log`)
	created := call(encode(input), "batch-create-atomic", "cores:warehouse:create", 201)
	counts(2, 1, 2, 1, 4, 1)
	var quantity float64
	if e := db.QueryRow(`SELECT quantity FROM product_locations`).Scan(&quantity); e != nil || quantity != 3.5 {
		t.Fatalf("stock %.2f %v", quantity, e)
	}
	replay := call(encode(input), "batch-create-atomic", "cores:warehouse:create", 201)
	var first, again map[string]any
	json.Unmarshal(created.Body.Bytes(), &first)
	json.Unmarshal(replay.Body.Bytes(), &again)
	if !reflect.DeepEqual(first, again) {
		t.Fatal("saved response changed")
	}
	counts(2, 1, 2, 1, 4, 1)
	exec(`UPDATE users SET is_admin=false WHERE userid=11`)
	call(encode(input), "batch-create-atomic", "cores:warehouse:create", 403)
	exec(`UPDATE users SET is_admin=true WHERE userid=11`)
	input["expected_context"] = strings.Repeat("0", 64)
	call(encode(input), "batch-create-atomic", "cores:warehouse:create", 409)
	input["expected_context"] = first["reviewed_context"]
	var group sync.WaitGroup
	for i := 0; i < 2; i++ {
		group.Add(1)
		go func() { defer group.Done(); call(encode(input), "batch-create-atomic", "cores:warehouse:create", 201) }()
	}
	group.Wait()
	counts(2, 1, 2, 1, 4, 1)
	// Every retained product/master audit is explicitly attributed to MCP/AI.
	var native int
	if e := db.QueryRow(`SELECT count(*) FROM audit_log WHERE new_values->>'origin' IS DISTINCT FROM 'MCP/AI'`).Scan(&native); e != nil || native != 0 {
		t.Fatal("origin missing", e)
	}
	for _, body := range []string{`{"products":[]}`, `{"products":[null]}`, `{"products":[{"name":"X","sql":"bad"}]}`, `{"products":[{"name":"X","product_id":1}]}`, `{"products":[{"name":"X","stock_quantity":1}]}`, `{"products":[{"name":"X","attributes":[]}]} {}`} {
		call(body, "", "cores:warehouse:create", 400)
	}
	// Data imported from an archived identity is never silently recreated.
	exec(`UPDATE products SET lifecycle_status='archived' WHERE name='Batch tape'`)
	for _, product := range input["products"].([]map[string]any) {
		delete(product, "initial_device_quantity")
		delete(product, "initial_zone_id")
		delete(product, "stock_quantity")
	}
	input["confirm_creation"] = false
	input["preview"] = true
	w := call(encode(input), "", "cores:warehouse:create", 200)
	if !bytes.Contains(w.Body.Bytes(), []byte("existing_product")) {
		t.Fatal("archived duplicate not reported")
	}
}
