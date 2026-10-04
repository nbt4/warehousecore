package handlers

import (
	"database/sql"
	"net/url"
	"os"
	"strings"
	"testing"
	"warehousecore/internal/repository"
)

func TestWarehouseCaseTemplateStartupRetainsStockReferenceVersions(t *testing.T) {
	dsn := os.Getenv("WAREHOUSE_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("owned _test PostgreSQL required")
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
	exec(`DROP SCHEMA IF EXISTS warehouse_case_template_startup_test CASCADE;CREATE SCHEMA warehouse_case_template_startup_test;SET search_path TO warehouse_case_template_startup_test`)
	defer db.Exec(`DROP SCHEMA warehouse_case_template_startup_test CASCADE`)
	exec(productImportFixtureSQL)
	exec(`CREATE TABLE warehouse_schema_migrations(version TEXT PRIMARY KEY);INSERT INTO warehouse_schema_migrations VALUES('033_hybrid_cable_inventory');
 CREATE TABLE cable_connectors(cable_connectorsid INT PRIMARY KEY);CREATE TABLE cable_types(cable_typesid INT PRIMARY KEY);INSERT INTO cable_connectors VALUES(1);INSERT INTO cable_types VALUES(1);
 ALTER TABLE product_locations ADD COLUMN location_id SERIAL PRIMARY KEY;ALTER TABLE product_locations ADD CONSTRAINT chk_product_locations_nonnegative CHECK(quantity>=0);
 INSERT INTO products(name,tracking_mode,product_type,lifecycle_status,stock_quantity,is_accessory,product_code,generic_barcode) VALUES('Cable fixture','quantity','accessory','active',2.5,true,'TPL-STARTUP','TPL-STARTUP');
 INSERT INTO product_locations(product_id,zone_id,quantity) VALUES(1,NULL,2.5);`)
	old := repository.DB
	repository.DB = db
	defer func() { repository.DB = old }()
	if err = EnsureCableInventorySchema(); err != nil {
		t.Fatal(err)
	}
	exec(`INSERT INTO cable_products(product_id,connector_a_id,connector_b_id,cable_type_id,length_m,tracking_mode) VALUES(1,1,1,1,1,'quantity')`)
	if err = EnsureProductManagementSchema(); err != nil {
		t.Fatal(err)
	}
	exec(`INSERT INTO cases(caseid,zone_id,lifecycle_status) VALUES(1,NULL,'active');INSERT INTO case_product_contents(case_id,product_id,quantity) VALUES(1,1,1);UPDATE product_locations SET quantity=1.5 WHERE product_id=1;`)
	exec(`CREATE FUNCTION guard_startup_product() RETURNS TRIGGER LANGUAGE plpgsql AS $$ BEGIN IF OLD.lifecycle_status='archived' THEN RAISE EXCEPTION 'archived product cannot be edited';END IF;NEW.updated_at:=GREATEST(clock_timestamp(),OLD.updated_at+INTERVAL '1 microsecond');RETURN NEW;END $$;CREATE TRIGGER guard_startup_product BEFORE UPDATE ON products FOR EACH ROW EXECUTE FUNCTION guard_startup_product();`)
	snapshot := func() string {
		t.Helper()
		var s string
		if err := db.QueryRow(`SELECT jsonb_build_array((SELECT jsonb_agg(to_jsonb(p) ORDER BY productid) FROM products p),(SELECT jsonb_agg(to_jsonb(pl) ORDER BY location_id) FROM product_locations pl))::text`).Scan(&s); err != nil {
			t.Fatal(err)
		}
		return s
	}
	for _, archived := range []bool{false, true} {
		if archived {
			exec(`UPDATE products SET lifecycle_status='archived' WHERE productid=1`)
		}
		before := snapshot()
		for i := 0; i < 2; i++ {
			if err = EnsureCableInventorySchema(); err != nil {
				t.Fatal(err)
			}
			if err = EnsureProductManagementSchema(); err != nil {
				t.Fatal(err)
			}
		}
		if after := snapshot(); before != after {
			t.Fatal("startup changed stock or exact reference versions", before, after)
		}
	}
}
