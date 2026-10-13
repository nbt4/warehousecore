package handlers

import (
	"context"
	"database/sql"
	"encoding/json"
	"github.com/gorilla/mux"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"
	"warehousecore/internal/middleware"
	"warehousecore/internal/models"
	"warehousecore/internal/repository"
	"warehousecore/internal/services"
)

func workspaceTestDB(t *testing.T) *sql.DB {
	t.Helper()
	dsn := os.Getenv("WAREHOUSE_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("disposable _test PostgreSQL required")
	}
	u, err := url.Parse(dsn)
	if err != nil || !strings.HasSuffix(u.Path, "_test") {
		t.Fatal("owned _test database required")
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	schema := "catalog_workspace_test"
	if _, err = db.Exec(`DROP SCHEMA IF EXISTS ` + schema + ` CASCADE;CREATE SCHEMA ` + schema + `;SET search_path TO ` + schema); err != nil {
		t.Fatal(err)
	}
	old := repository.DB
	repository.DB = db
	t.Cleanup(func() { repository.DB = old; db.Exec(`DROP SCHEMA ` + schema + ` CASCADE`); db.Close() })
	setup := productImportFixtureSQL + `ALTER TABLE products ADD COLUMN website_visible BOOLEAN DEFAULT false;ALTER TABLE products ADD COLUMN website_featured BOOLEAN DEFAULT false;ALTER TABLE products ADD COLUMN website_thumbnail TEXT;ALTER TABLE products ADD COLUMN website_images_json TEXT DEFAULT '[]';ALTER TABLE count_types ADD COLUMN is_decimal BOOLEAN DEFAULT false;
CREATE TABLE product_packages(id SERIAL PRIMARY KEY,name TEXT,description TEXT,is_active BOOLEAN DEFAULT true,website_visible BOOLEAN DEFAULT false,website_image_url TEXT,website_images_json TEXT DEFAULT '[]',updated_at TIMESTAMP DEFAULT now());
CREATE TABLE cable_types(cable_typesid INT PRIMARY KEY,name TEXT);CREATE TABLE cable_connectors(cable_connectorsid INT PRIMARY KEY,name TEXT,abbreviation TEXT,gender TEXT);CREATE TABLE cable_products(product_id INT,cable_type_id INT,connector_a_id INT,connector_b_id INT);
CREATE TABLE status(statusid INT PRIMARY KEY,status TEXT);INSERT INTO status VALUES(1,'open'),(2,'closed');CREATE FUNCTION warehouse_job_status_is_closed(TEXT) RETURNS BOOLEAN LANGUAGE SQL AS $$ SELECT $1='closed' $$;
CREATE TABLE jobs(jobid INT PRIMARY KEY,statusid INT,deleted_at TIMESTAMP);INSERT INTO jobs VALUES(1,1,NULL),(2,1,NULL),(3,2,NULL);
CREATE TABLE job_positions(position_id INT PRIMARY KEY,job_id INT,product_id INT,position_type TEXT,quantity NUMERIC,unit TEXT,sort_order INT,deleted_at TIMESTAMPTZ);CREATE TABLE job_product_requirements(id INT,job_id INT,product_id INT,quantity INT,manual_quantity INT,deleted_at TIMESTAMPTZ);
CREATE TABLE product_dependencies(id INT PRIMARY KEY,product_id INT,dependency_product_id INT,relation_type TEXT,assignment_scope TEXT,default_quantity NUMERIC,lifecycle_status TEXT);
ALTER TABLE devices ADD COLUMN serialnumber TEXT;ALTER TABLE devices ADD COLUMN notes TEXT;ALTER TABLE devices ADD COLUMN barcode TEXT;ALTER TABLE devices ADD COLUMN qr_code TEXT;
CREATE TABLE job_devices(jobid INT,deviceid TEXT,pack_status TEXT DEFAULT 'pending',PRIMARY KEY(jobid,deviceid));CREATE TABLE job_position_devices(position_id INT,device_id TEXT,scanned_at TIMESTAMPTZ DEFAULT now(),scanned_by TEXT DEFAULT '');CREATE TABLE devicescases(deviceid TEXT,caseid INT);CREATE TABLE device_components(device_id TEXT,component_device_id TEXT,relation_type TEXT);
ALTER TABLE audit_log ADD COLUMN timestamp TIMESTAMPTZ DEFAULT now();`
	if _, err = db.Exec(setup + productWorkspaceSQL + services.MaterialSnapshotSchema); err != nil {
		t.Fatal(err)
	}
	return db
}
func workspaceCall(t *testing.T, handler http.HandlerFunc, body string, vars map[string]string, want int) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest("POST", "/api/v1/admin/test", strings.NewReader(body))
	request = mux.SetURLVars(request, vars)
	request = request.WithContext(context.WithValue(request.Context(), middleware.UserContextKey, &models.User{UserID: 11, IsActive: true, IsAdmin: true}))
	response := httptest.NewRecorder()
	handler(response, request)
	if response.Code != want {
		t.Fatalf("status %d want %d: %s", response.Code, want, response.Body.String())
	}
	return response
}
func TestWebsiteDraftPublicationRevisionAndImmutableFolders(t *testing.T) {
	db := workspaceTestDB(t)
	old := websiteRevalidator
	websiteRevalidator = services.NewRevalidatorFromEnv()
	t.Cleanup(func() { websiteRevalidator = old })
	exec := func(q string) {
		t.Helper()
		if _, err := db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	exec(`INSERT INTO products(productid,name,description,lifecycle_status,tracking_mode,product_type,product_kind) VALUES(1,'Original','Internal','active','individual','equipment','standard');INSERT INTO warehouse_picture_files(entity_type,entity_id,file_name) VALUES('product',1,'cover.jpg');`)
	vars := map[string]string{"id": "1"}
	workspaceCall(t, UpdateProductWebsite, `{"action":"save_draft","title":"Public title","description":"Public text","website_images":["cover.jpg"],"website_thumbnail":"cover.jpg","website_featured":true,"expected_revision":0}`, vars, 200)
	var visible bool
	var description string
	if err := db.QueryRow(`SELECT website_visible,description FROM products WHERE productid=1`).Scan(&visible, &description); err != nil || visible || description != "Internal" {
		t.Fatalf("draft mutated live/internal: %v %s %v", visible, description, err)
	}
	workspaceCall(t, UpdateProductWebsite, `{"action":"publish","expected_revision":0}`, vars, 409)
	workspaceCall(t, UpdateProductWebsite, `{"action":"publish","expected_revision":1}`, vars, 200)
	var featured bool
	if err := db.QueryRow(`SELECT website_visible,website_featured,description FROM products WHERE productid=1`).Scan(&visible, &featured, &description); err != nil || !visible || !featured || description != "Internal" {
		t.Fatal("publish changed internal or lost featured", err)
	}
	var folder string
	db.QueryRow(`SELECT folder_name FROM warehouse_picture_folders WHERE entity_id=1 AND entity_type='product'`).Scan(&folder)
	exec(`UPDATE products SET name='Renamed' WHERE productid=1;INSERT INTO products(productid,name,lifecycle_status,tracking_mode,product_type,product_kind) VALUES(2,'Original','active','individual','equipment','standard')`)
	var second string
	db.QueryRow(`SELECT folder_name FROM warehouse_picture_folders WHERE entity_id=2 AND entity_type='product'`).Scan(&second)
	if folder == second || !strings.HasPrefix(folder, "product-1-") {
		t.Fatal("reused mutable image folder", folder, second)
	}
	request := httptest.NewRequest("GET", "/api/v1/public/products/1/pictures/cover.jpg", nil)
	if !publicCatalogPictureAllowed(request, "product", 1, "cover.jpg") || publicCatalogPictureAllowed(request, "product", 1, "unpublished.jpg") {
		t.Fatal("public media selection ignored")
	}
	exec(`INSERT INTO warehouse_picture_deletions(entity_type,entity_id,folder_name,file_name) VALUES('product',1,'folder','cover.jpg')`)
	if publicCatalogPictureAllowed(request, "product", 1, "cover.jpg") {
		t.Fatal("pending deletion remained public")
	}
}
func TestMaterialScansShareSnapshotAndReserveIncludedDevices(t *testing.T) {
	db := workspaceTestDB(t)
	exec := func(q string) {
		t.Helper()
		if _, err := db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	exec(`INSERT INTO products(productid,name,lifecycle_status,tracking_mode,product_type,product_kind,count_type_id) VALUES(1,'Head','active','individual','equipment','standard',1),(2,'Component','active','individual','accessory','standard',1),(3,'Tape','active','quantity','consumable','consumable',1),(4,'Nested','active','individual','accessory','standard',1);
INSERT INTO devices(deviceid,productid,lifecycle_status,condition_status,status) VALUES('HEAD',1,'active','available','in_storage'),('COMP',2,'active','available','in_storage'),('NEST',4,'active','available','in_storage');INSERT INTO job_positions VALUES(1,1,1,'product',2,'Stk',1,NULL),(2,2,2,'product',1,'Stk',1,NULL);INSERT INTO job_product_requirements VALUES(1,1,1,3,1,NULL);
INSERT INTO product_dependencies VALUES(1,1,2,'included','device',1,'active'),(2,2,4,'included','device',1,'active'),(3,1,3,'consumes','device',0.25,'active');INSERT INTO device_components VALUES('HEAD','COMP','included'),('COMP','NEST','included');INSERT INTO job_position_devices(position_id,device_id) VALUES(2,'COMP');`)
	vars := map[string]string{"id": "1"}
	workspaceCall(t, ScanDeviceToPicklist, `{"device_id":"HEAD"}`, vars, 409)
	var count int
	db.QueryRow(`SELECT count(*) FROM warehouse_material_scans`).Scan(&count)
	if count != 0 {
		t.Fatal("partial scan committed")
	}
	exec(`DELETE FROM job_position_devices WHERE position_id=2`)
	workspaceCall(t, ScanDeviceToPicklist, `{"device_id":"HEAD"}`, vars, 201)
	workspaceCall(t, ScanDeviceToPicklist, `{"device_id":"HEAD"}`, vars, 200)
	db.QueryRow(`SELECT count(*) FROM warehouse_material_scans`).Scan(&count)
	if count != 3 {
		t.Fatal("nested components missing or duplicate", count)
	}
	workspaceCall(t, ScanDeviceToPicklist, `{"product_id":3,"quantity":0.75,"receipt":"tape-1"}`, vars, 201)
	workspaceCall(t, ScanDeviceToPicklist, `{"product_id":3,"quantity":0.75,"receipt":"tape-1"}`, vars, 200)
	workspaceCall(t, ScanDeviceToPicklist, `{"product_id":3,"quantity":0.75,"receipt":"tape-2"}`, vars, 409)
	response := workspaceCall(t, GetJobPicklist, "", vars, 200)
	var result struct {
		Positions []struct {
			ProductID int     `json:"product_id"`
			Needed    float64 `json:"needed"`
			Scanned   float64 `json:"scanned"`
		}
		Version int64 `json:"version"`
	}
	json.Unmarshal(response.Body.Bytes(), &result)
	for _, item := range result.Positions {
		if item.ProductID == 1 && item.Needed != 3 {
			t.Fatal("manual demand double counted", item)
		}
		if item.ProductID == 3 && (item.Needed != 0.75 || item.Scanned != 0.75) {
			t.Fatal("decimal requirement lost", item)
		}
	}
	exec(`UPDATE product_dependencies SET default_quantity=99 WHERE id=3;UPDATE job_positions SET quantity=3 WHERE position_id=1`)
	response = workspaceCall(t, GetJobPicklist, "", vars, 200)
	json.Unmarshal(response.Body.Bytes(), &result)
	if result.Version != 2 {
		t.Fatal("source change did not version snapshot", result.Version)
	}
	for _, item := range result.Positions {
		if item.ProductID == 3 && item.Needed != 1 {
			t.Fatal("catalog rule changed existing job", item)
		}
	}
}

func TestCatalogSearchPaginationAndBulkCategoryAreAtomic(t *testing.T) {
	db := workspaceTestDB(t)
	if _, err := db.Exec(`INSERT INTO categories(categoryid,name,lifecycle_status) VALUES(1,'Lighting','active');INSERT INTO products(productid,name,model_number,lifecycle_status,tracking_mode,product_type,product_kind,attributes) VALUES(1,'Head','MODEL-42','active','individual','equipment','standard','{}'),(2,'Another','MODEL-43','active','individual','equipment','standard','{}')`); err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest("GET", "/api/v1/admin/products?search=MODEL-42&limit=50&offset=0", nil)
	response := httptest.NewRecorder()
	GetProducts(response, request)
	if response.Code != 200 || response.Header().Get("X-Total-Count") != "1" {
		t.Fatal(response.Code, response.Body.String())
	}
	var products []Product
	if json.Unmarshal(response.Body.Bytes(), &products) != nil || len(products) != 1 || products[0].Version == "" {
		t.Fatal("identity search/version failed", response.Body.String())
	}
	request = httptest.NewRequest("GET", "/api/v1/admin/products?limit=wrong&offset=0", nil)
	response = httptest.NewRecorder()
	GetProducts(response, request)
	if response.Code != 400 {
		t.Fatal("invalid limit accepted")
	}
	var version string
	db.QueryRow(`SELECT to_char(updated_at,'YYYY-MM-DD"T"HH24:MI:SS.US"Z"') FROM products WHERE productid=1`).Scan(&version)
	body := `{"products":[{"product_id":1,"version":"` + version + `"},{"product_id":2,"version":"stale"}],"category_id":1}`
	workspaceCall(t, AssignCatalogCategory, body, nil, 409)
	var changed int
	db.QueryRow(`SELECT count(*) FROM products WHERE categoryid=1`).Scan(&changed)
	if changed != 0 {
		t.Fatal("partial bulk mutation committed")
	}
	workspaceCall(t, AssignCatalogCategory, `{"products":[{"product_id":1,"version":"`+version+`"}],"category_id":1}`, nil, 200)
}

func TestDeviceEditorReturnsVersionAndRejectsConcurrentUpdate(t *testing.T) {
	db := workspaceTestDB(t)
	_, err := db.Exec(`ALTER TABLE devices ADD COLUMN updated_at TIMESTAMP DEFAULT now();ALTER TABLE devices ADD COLUMN current_case_id INT;ALTER TABLE devices ADD COLUMN condition_rating NUMERIC DEFAULT 5;ALTER TABLE devices ADD COLUMN usage_hours NUMERIC DEFAULT 0;ALTER TABLE devices ADD COLUMN purchasedate DATE;ALTER TABLE devices ADD COLUMN lastmaintenance DATE;ALTER TABLE devices ADD COLUMN nextmaintenance DATE;ALTER TABLE devices ADD COLUMN label_path TEXT;ALTER TABLE devices ADD COLUMN archived_at TIMESTAMP;ALTER TABLE devices ADD COLUMN archived_by_product BOOLEAN DEFAULT false;ALTER TABLE cases ADD COLUMN name TEXT;ALTER TABLE jobs ADD COLUMN job_code TEXT;ALTER TABLE jobs ADD COLUMN startdate DATE;
INSERT INTO products(productid,name,lifecycle_status,tracking_mode,product_type,product_kind) VALUES(1,'Device product','active','individual','equipment','standard');INSERT INTO devices(deviceid,productid,status,condition_status,lifecycle_status) VALUES('EDIT',1,'in_storage','available','active')`)
	if err != nil {
		t.Fatal(err)
	}
	vars := map[string]string{"id": "EDIT"}
	response := workspaceCall(t, GetDeviceAdmin, "", vars, 200)
	var device DeviceAdminResponse
	json.Unmarshal(response.Body.Bytes(), &device)
	if device.Version == "" {
		t.Fatal("device version missing", response.Body.String())
	}
	if _, err = db.Exec(`UPDATE devices SET notes='Concurrent note',updated_at=updated_at+interval '1 second' WHERE deviceid='EDIT'`); err != nil {
		t.Fatal(err)
	}
	workspaceCall(t, UpdateDevice, `{"serial_number":"New serial","expected_updated_at":"`+device.Version+`"}`, vars, 409)
	var notes string
	db.QueryRow(`SELECT notes FROM devices WHERE deviceid='EDIT'`).Scan(&notes)
	if notes != "Concurrent note" {
		t.Fatal("overwrote concurrent edit")
	}
}

func TestIncludedMaterialAcceptsLeafAtMaximumDepth(t *testing.T) {
	db := workspaceTestDB(t)
	if _, err := db.Exec(`INSERT INTO products(productid,name,lifecycle_status,tracking_mode,product_type,product_kind) SELECT id,'Chain '||id,'active','individual','equipment','standard' FROM generate_series(1,6) id;INSERT INTO devices(deviceid,productid,status,condition_status,lifecycle_status) SELECT 'CHAIN'||id,id,'in_storage','available','active' FROM generate_series(1,6) id;INSERT INTO product_dependencies SELECT id,id,id+1,'included','device',1,'active' FROM generate_series(1,5) id;INSERT INTO device_components SELECT 'CHAIN'||id,'CHAIN'||(id+1),'included' FROM generate_series(1,5) id;INSERT INTO job_positions VALUES(1,1,1,'product',1,'Stk',1,NULL)`); err != nil {
		t.Fatal(err)
	}
	workspaceCall(t, ScanDeviceToPicklist, `{"device_id":"CHAIN1"}`, map[string]string{"id": "1"}, 201)
	var count int
	if err := db.QueryRow(`SELECT count(*) FROM warehouse_material_scans`).Scan(&count); err != nil || count != 6 {
		t.Fatal("maximum-depth leaf rejected or omitted", count, err)
	}
}

func TestExternalRentalUpdateKeepsMetadataWhenFieldSaveFails(t *testing.T) {
	db := workspaceTestDB(t)
	_, err := db.Exec(`CREATE TABLE rental_equipment(id INT PRIMARY KEY,name TEXT,supplier TEXT,rental_price NUMERIC,customer_price NUMERIC,category TEXT,description TEXT,notes TEXT,is_active BOOLEAN,created_at TIMESTAMP DEFAULT now(),updated_at TIMESTAMP DEFAULT now());INSERT INTO rental_equipment(id,name,supplier,rental_price,customer_price,is_active) VALUES(1,'Original rental','Supplier',10,20,true);CREATE TABLE rental_equipment_field_definitions(id INT PRIMARY KEY);INSERT INTO rental_equipment_field_definitions VALUES(1);CREATE TABLE rental_equipment_field_values(equipment_id INT REFERENCES rental_equipment(id),field_definition_id INT REFERENCES rental_equipment_field_definitions(id),value TEXT,sort_order INT);INSERT INTO rental_equipment_field_values VALUES(1,1,'Retained',0)`)
	if err != nil {
		t.Fatal(err)
	}
	var version time.Time
	db.QueryRow(`SELECT updated_at FROM rental_equipment WHERE id=1`).Scan(&version)
	body := `{"product_name":"Changed","supplier_name":"Supplier","rental_price":10,"customer_price":20,"expected_updated_at":"` + version.Format(time.RFC3339Nano) + `","field_values":[{"field_definition_id":99,"value":"invalid","sort_order":0}]}`
	workspaceCall(t, UpdateRentalEquipment, body, map[string]string{"id": "1"}, 400)
	var name, value string
	db.QueryRow(`SELECT name FROM rental_equipment WHERE id=1`).Scan(&name)
	db.QueryRow(`SELECT value FROM rental_equipment_field_values WHERE equipment_id=1`).Scan(&value)
	if name != "Original rental" || value != "Retained" {
		t.Fatal("partial rental update committed", name, value)
	}
	db.Exec(`UPDATE rental_equipment SET updated_at=updated_at+interval '1 second' WHERE id=1`)
	workspaceCall(t, UpdateRentalEquipment, body, map[string]string{"id": "1"}, 409)
}
