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
	"warehousecore/internal/middleware"
	"warehousecore/internal/models"
	"warehousecore/internal/repository"
)

const inventoryMCPFixtureSQL = `
ALTER TABLE products ADD COLUMN updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP;
ALTER TABLE products ADD COLUMN stock_quantity NUMERIC(10,3) DEFAULT 0;
ALTER TABLE products ADD COLUMN weight NUMERIC DEFAULT 2;
ALTER TABLE products ADD COLUMN width NUMERIC DEFAULT 20;
ALTER TABLE products ADD COLUMN height NUMERIC DEFAULT 20;
ALTER TABLE products ADD COLUMN depth NUMERIC DEFAULT 20;
UPDATE products SET tracking_mode='quantity',weight=1 WHERE productid=3;
CREATE TABLE location_profiles(profile_id BIGINT PRIMARY KEY,allow_devices BOOLEAN DEFAULT true,allow_quantity_products BOOLEAN DEFAULT true,allow_cases BOOLEAN DEFAULT true,allow_mixed_products BOOLEAN DEFAULT true,allow_cycle_count BOOLEAN DEFAULT true,updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP);
INSERT INTO location_profiles(profile_id) VALUES(1);
ALTER TABLE storage_zones ADD COLUMN updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP;
ALTER TABLE storage_zones ADD COLUMN profile_id BIGINT DEFAULT 1;
ALTER TABLE storage_zones ADD COLUMN capacity_mode TEXT DEFAULT 'item_count';
ALTER TABLE storage_zones ADD COLUMN max_weight_kg NUMERIC;
ALTER TABLE storage_zones ADD COLUMN max_volume_m3 NUMERIC;
ALTER TABLE storage_zones ADD COLUMN inventory_frequency_days INT DEFAULT 7;
ALTER TABLE storage_zones ADD COLUMN last_counted_at TIMESTAMP;
ALTER TABLE storage_zones ADD COLUMN next_count_at TIMESTAMP;
UPDATE storage_zones SET capacity=100,max_weight_kg=100,max_volume_m3=100 WHERE zone_id=1;
UPDATE storage_zones SET is_storable=true WHERE zone_id=3;
ALTER TABLE cases ADD PRIMARY KEY(caseid);
ALTER TABLE cases ADD COLUMN name TEXT;
ALTER TABLE cases ADD COLUMN lifecycle_status TEXT DEFAULT 'active';
ALTER TABLE cases ADD COLUMN status TEXT DEFAULT 'free';
ALTER TABLE cases ADD COLUMN workflow_status TEXT DEFAULT 'sealed';
ALTER TABLE cases ADD COLUMN current_job_id BIGINT;
ALTER TABLE cases ADD COLUMN updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP;
ALTER TABLE cases ADD COLUMN weight NUMERIC DEFAULT 2;
ALTER TABLE cases ADD COLUMN width NUMERIC DEFAULT 50;
ALTER TABLE cases ADD COLUMN height NUMERIC DEFAULT 50;
ALTER TABLE cases ADD COLUMN depth NUMERIC DEFAULT 50;
INSERT INTO cases(caseid,name,zone_id) VALUES(1,'Root case',1),(2,'Packed child',NULL);
ALTER TABLE devicescases ADD COLUMN caseid INT;
CREATE TABLE case_child_contents(parent_case_id INT,child_case_id INT);
INSERT INTO case_child_contents VALUES(1,2);
CREATE TABLE case_product_contents(case_id INT,product_id INT,quantity NUMERIC(12,3));
INSERT INTO case_product_contents VALUES(2,3,2);
ALTER TABLE warehouse_tasks ADD COLUMN case_id INT;
ALTER TABLE product_locations ADD COLUMN product_id INT;
ALTER TABLE product_locations ADD COLUMN updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP;
ALTER TABLE product_locations ADD CONSTRAINT inventory_test_product_zone UNIQUE(product_id,zone_id);
INSERT INTO product_locations(product_id,zone_id,quantity) VALUES(3,1,5.5);
UPDATE products SET stock_quantity=5.5 WHERE productid=3;
CREATE FUNCTION inventory_test_stock() RETURNS TRIGGER AS $$ BEGIN UPDATE products SET stock_quantity=(SELECT SUM(quantity) FROM product_locations WHERE product_id=NEW.product_id) WHERE productid=NEW.product_id;RETURN NEW;END $$ LANGUAGE plpgsql;
CREATE TRIGGER inventory_test_stock AFTER INSERT OR UPDATE ON product_locations FOR EACH ROW EXECUTE FUNCTION inventory_test_stock();
CREATE FUNCTION inventory_test_version() RETURNS TRIGGER AS $$ BEGIN NEW.updated_at:=GREATEST(clock_timestamp() AT TIME ZONE 'UTC',OLD.updated_at+INTERVAL '1 microsecond');RETURN NEW;END $$ LANGUAGE plpgsql;
CREATE TRIGGER inventory_test_product_version BEFORE UPDATE ON products FOR EACH ROW EXECUTE FUNCTION inventory_test_version();
CREATE TRIGGER inventory_test_case_version BEFORE UPDATE ON cases FOR EACH ROW EXECUTE FUNCTION inventory_test_version();
CREATE TRIGGER inventory_test_zone_version BEFORE UPDATE ON storage_zones FOR EACH ROW EXECUTE FUNCTION inventory_test_version();
CREATE TABLE users(userid BIGINT PRIMARY KEY,username TEXT,is_active BOOLEAN);
INSERT INTO users VALUES(11,'counter',true);
CREATE TABLE inventory_counts(count_id BIGSERIAL PRIMARY KEY,zone_id INT,status TEXT DEFAULT 'open',blind_count BOOLEAN DEFAULT true,started_at TIMESTAMP,completed_at TIMESTAMP,created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP);
CREATE TABLE inventory_count_lines(line_id BIGSERIAL PRIMARY KEY,count_id BIGINT REFERENCES inventory_counts(count_id),item_type TEXT,item_key TEXT,expected_quantity NUMERIC(12,3) DEFAULT 0,counted_quantity NUMERIC(12,3),created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,UNIQUE(count_id,item_type,item_key));
ALTER TABLE device_movements ADD COLUMN movement_id BIGSERIAL PRIMARY KEY;
ALTER TABLE device_movements ADD COLUMN from_zone_id INT;
ALTER TABLE device_movements ADD COLUMN to_zone_id INT;
ALTER TABLE device_movements ADD COLUMN moved_by INT;
ALTER TABLE device_movements ADD COLUMN movement_type TEXT;
ALTER TABLE device_movements ADD COLUMN reason TEXT;
ALTER TABLE device_movements ADD COLUMN metadata JSONB;
CREATE TABLE case_events(event_id BIGSERIAL PRIMARY KEY,case_id INT,event_type TEXT,zone_id INT,metadata JSONB);
INSERT INTO devices(productid,status,condition_status,zone_id) VALUES(1,'in_storage','available',1),(1,'in_storage','available',3);
INSERT INTO devices(productid,status,condition_status,zone_id,current_case_id,current_location) VALUES(1,'in_storage','available',NULL,2,'case:sealed');
INSERT INTO devicescases(deviceid,caseid) VALUES('DEV-00000003',2);
`

func TestWarehouseInventoryAtomicCountReviewApprovalAndHistory(t *testing.T) {
	dsn := os.Getenv("WAREHOUSE_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("disposable _test database required")
	}
	parsed, err := url.Parse(dsn)
	if err != nil || !strings.HasSuffix(parsed.Path, "_test") {
		t.Fatal("dedicated test database required")
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
	exec("DROP SCHEMA IF EXISTS warehouse_inventory_mcp_test CASCADE;CREATE SCHEMA warehouse_inventory_mcp_test;SET search_path TO warehouse_inventory_mcp_test")
	defer db.Exec("DROP SCHEMA warehouse_inventory_mcp_test CASCADE")
	exec(warehouseDeviceFixtureSQL)
	exec(inventoryMCPFixtureSQL)
	old := repository.DB
	repository.DB = db
	defer func() { repository.DB = old }()
	for i := 0; i < 2; i++ {
		for _, ensure := range []func() error{EnsureWarehouseDeviceVersionSchema, EnsureWarehouseInventoryLifecycleSchema} {
			if err := ensure(); err != nil {
				t.Fatal(err)
			}
		}
	}
	count := func(table string) int {
		t.Helper()
		var n int
		if err := db.QueryRow("SELECT count(*) FROM " + table).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	call := func(op, key string, in map[string]any, admin bool, want int) map[string]any {
		t.Helper()
		raw, _ := json.Marshal(in)
		r := httptest.NewRequest(http.MethodPost, "/inventory", bytes.NewReader(raw))
		r = mux.SetURLVars(r, map[string]string{"operation": op})
		r = r.WithContext(context.WithValue(r.Context(), middleware.UserContextKey, &models.User{UserID: 11, IsAdmin: admin}))
		r.Header.Set("X-Cores-Origin", "MCP/AI")
		r.Header.Set("Idempotency-Key", key)
		w := httptest.NewRecorder()
		WarehouseInventoryMCP(w, r)
		if w.Code != want {
			t.Fatalf("%s got %d want %d: %s", op, w.Code, want, w.Body.String())
		}
		out := map[string]any{}
		if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
			t.Fatal(err)
		}
		return out
	}
	blocked := func(out map[string]any, reason string) {
		t.Helper()
		if out["ready_to_execute"] != false || !strings.Contains(fmt.Sprint(out["required_fields"]), reason) {
			t.Fatalf("missing blocker %s: %#v", reason, out)
		}
	}
	confirm := func(op, key string, in map[string]any) map[string]any {
		t.Helper()
		p := call(op, "", in, true, 200)
		if p["ready_to_execute"] != true {
			t.Fatal("preview blocked", op, p)
		}
		in["confirm_change"] = true
		in["expected_updated_at"] = p["expected_updated_at"]
		in["expected_context"] = p["expected_context"]
		if phrase, ok := p["required_confirmation_text"]; ok {
			in["confirmation_text"] = phrase
		}
		return call(op, key, in, true, 200)
	}
	create := map[string]any{"zone_id": 1, "notes": "private count notes"}
	call("create", "", create, false, 403)
	p := call("create", "", create, true, 200)
	if p["ready_to_execute"] != true || len(p["lines"].([]any)) != 3 {
		t.Fatal("incorrect start snapshot", p)
	}
	for _, line := range p["lines"].([]any) {
		if _, ok := line.(map[string]any)["expected_quantity"]; ok {
			t.Fatal("blind start leaked baseline")
		}
	}
	if count("inventory_counts")+count("inventory_count_lines")+count("audit_log")+count("warehouse_product_mutation_receipts") != 0 {
		t.Fatal("preview wrote")
	}
	created := confirm("create", "inventory-create-once", create)
	id := int64(created["inventory_count"].(map[string]any)["count_id"].(float64))
	if count("inventory_counts") != 1 || count("inventory_count_lines") != 3 || count("inventory_count_events") != 1 {
		t.Fatal("creation not atomic")
	}
	if !reflect.DeepEqual(created, call("create", "inventory-create-once", create, true, 200)) {
		t.Fatal("durable create replay changed")
	}
	blocked(call("create", "", map[string]any{"zone_id": 1}, true, 200), "no_other_active_count")
	blocked(call("review", "", map[string]any{"count_id": id}, true, 200), "count_all_lines_or_explicit_mark_uncounted_zero")
	wrongApproval := call("approve", "", map[string]any{"count_id": id}, true, 200)
	blocked(wrongApproval, "review_status_required")
	if _, ok := wrongApproval["references"]; ok {
		t.Fatal("premature approval leaked blind stock")
	}
	if len(wrongApproval["effects"].(map[string]any)["stock_adjustments"].([]any)) != 0 {
		t.Fatal("premature approval leaked expected quantities")
	}
	for _, quantity := range []float64{-1, .5, 2} {
		call("set_lines", "", map[string]any{"count_id": id, "lines": []any{map[string]any{"item_type": "device", "item_key": "DEV-00000001", "counted_quantity": quantity}}}, true, 400)
	}
	lineChanges := map[string]any{"count_id": id, "lines": []any{map[string]any{"item_type": "device", "item_key": "DEV-00000001", "counted_quantity": 0}, map[string]any{"item_type": "device", "item_key": "DEV-00000002", "counted_quantity": 1}, map[string]any{"item_type": "product", "item_key": "3", "counted_quantity": 6.25}}}
	p = call("set_lines", "", lineChanges, true, 200)
	lineChanges["confirm_change"] = true
	lineChanges["expected_updated_at"] = p["expected_updated_at"]
	lineChanges["expected_context"] = p["expected_context"]
	exec(fmt.Sprintf("INSERT INTO inventory_count_events(count_id,event_type) VALUES(%d,'concurrent UI event')", id))
	blocked(call("set_lines", "inventory-stale-event", lineChanges, true, 200), "expected_updated_at")
	delete(lineChanges, "confirm_change")
	delete(lineChanges, "expected_updated_at")
	delete(lineChanges, "expected_context")
	confirm("set_lines", "inventory-lines-once", lineChanges)
	if count("device_movements") != 0 || count("inventory_adjustments") != 0 {
		t.Fatal("counting moved stock")
	}
	exec("INSERT INTO job_devices(deviceid,jobid,pack_status) VALUES('DEV-00000002',1,'assigned')")
	blocked(call("set_lines", "", map[string]any{"count_id": id, "lines": []any{map[string]any{"item_type": "device", "item_key": "DEV-00000002", "counted_quantity": 1}}}, true, 200), "available_item.device")
	exec("DELETE FROM job_devices")
	review := confirm("review", "inventory-review-once", map[string]any{"count_id": id, "mark_uncounted_zero": true})
	if review["inventory_count"].(map[string]any)["status"] != "review" || count("device_movements") != 0 {
		t.Fatal("review moved stock")
	}
	approve := map[string]any{"count_id": id}
	exec("INSERT INTO inventory_counts(zone_id,status) VALUES(3,'counting')")
	blocked(call("approve", "", approve, true, 200), "available_source_zone.device:DEV-00000002")
	exec("DELETE FROM inventory_counts WHERE zone_id=3")
	p = call("approve", "", approve, true, 200)
	if p["ready_to_execute"] != true || len(p["effects"].(map[string]any)["stock_adjustments"].([]any)) != 4 {
		t.Fatal("wrong physical review", p)
	}
	approve["confirm_change"] = true
	approve["expected_updated_at"] = p["expected_updated_at"]
	approve["expected_context"] = p["expected_context"]
	approve["confirmation_text"] = "APPROVE WAREHOUSE INVENTORY COUNT 999"
	call("approve", "inventory-wrong-phrase", approve, true, 428)
	approve["confirmation_text"] = p["required_confirmation_text"]
	// Fail the last audit, after every physical adjustment/event/zone write.
	exec(`CREATE FUNCTION inventory_test_reject_audit() RETURNS TRIGGER AS $$ BEGIN IF NEW.action='inventory_count.approve' THEN RAISE EXCEPTION 'injected inventory audit failure';END IF;RETURN NEW;END $$ LANGUAGE plpgsql;CREATE TRIGGER inventory_test_reject_audit BEFORE INSERT ON audit_log FOR EACH ROW EXECUTE FUNCTION inventory_test_reject_audit()`)
	receipts, events, audits := count("warehouse_product_mutation_receipts"), count("inventory_count_events"), count("audit_log")
	call("approve", "inventory-approve-after-failure", approve, true, 500)
	if count("device_movements") != 0 || count("case_events") != 0 || count("inventory_adjustments") != 0 || count("warehouse_product_mutation_receipts") != receipts || count("inventory_count_events") != events || count("audit_log") != audits {
		t.Fatal("failed approval leaked writes")
	}
	var state string
	var quantity float64
	if err := db.QueryRow("SELECT status FROM inventory_counts WHERE count_id=$1", id).Scan(&state); err != nil || state != "review" {
		t.Fatal("failed approval changed count", state, err)
	}
	if err := db.QueryRow("SELECT quantity FROM product_locations WHERE zone_id=1 AND product_id=3").Scan(&quantity); err != nil || quantity != 5.5 {
		t.Fatal("failed approval changed quantity", quantity, err)
	}
	exec("DROP TRIGGER inventory_test_reject_audit ON audit_log")
	approved := call("approve", "inventory-approve-after-failure", approve, true, 200)
	if approved["inventory_count"].(map[string]any)["status"] != "approved" || count("inventory_adjustments") != 4 || count("device_movements") != 2 || count("case_events") != 1 {
		t.Fatal("approval incomplete", approved)
	}
	var correct bool
	if err := db.QueryRow(`SELECT (SELECT status='location_unknown' AND zone_id IS NULL FROM devices WHERE deviceid='DEV-00000001') AND (SELECT status='in_storage' AND zone_id=1 FROM devices WHERE deviceid='DEV-00000002') AND (SELECT zone_id IS NULL FROM cases WHERE caseid=1) AND (SELECT zone_id IS NULL AND current_case_id=2 AND status='in_storage' FROM devices WHERE deviceid='DEV-00000003') AND (SELECT quantity=6.25 FROM product_locations WHERE zone_id=1 AND product_id=3) AND (SELECT stock_quantity=6.25 FROM products WHERE productid=3) AND (SELECT operational_status='available' AND last_counted_at IS NOT NULL AND next_count_at IS NOT NULL FROM storage_zones WHERE zone_id=1)`).Scan(&correct); err != nil || !correct {
		t.Fatal("incorrect physical effects", correct, err)
	}
	if !reflect.DeepEqual(approved, call("approve", "inventory-approve-after-failure", approve, true, 200)) || count("inventory_adjustments") != 4 {
		t.Fatal("durable approval replay moved stock twice")
	}
	confirm("archive", "inventory-archive", map[string]any{"count_id": id})
	if _, err := db.Exec("UPDATE inventory_count_lines SET counted_quantity=1 WHERE count_id=$1", id); err == nil {
		t.Fatal("archived count lines edited")
	}
	confirm("restore", "inventory-restore", map[string]any{"count_id": id})
	blocked(call("return_for_counting", "", map[string]any{"count_id": id, "reason": "recount"}, true, 200), "review_status_required")
	// A new count cannot reconcile a changed start snapshot, even with a fresh
	// final preview. Cancellation releases the zone and permits a real recount.
	second := confirm("create", "inventory-second-count", map[string]any{"zone_id": 1})
	secondID := int64(second["inventory_count"].(map[string]any)["count_id"].(float64))
	exec("UPDATE product_locations SET quantity=7 WHERE zone_id=1 AND product_id=3")
	confirm("review", "inventory-second-review", map[string]any{"count_id": secondID, "mark_uncounted_zero": true})
	blocked(call("approve", "", map[string]any{"count_id": secondID}, true, 200), "stock_changed_since_count_start")
	confirm("cancel", "inventory-cancel-changed-count", map[string]any{"count_id": secondID, "reason": "Stock changed; recount with fresh snapshot"})
	if count("inventory_adjustments") != 4 {
		t.Fatal("cancellation adjusted stock")
	}
}
