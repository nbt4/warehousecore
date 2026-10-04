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
	"time"

	"warehousecore/internal/middleware"
	"warehousecore/internal/models"
	"warehousecore/internal/repository"

	jwt "github.com/golang-jwt/jwt/v5"
	"github.com/gorilla/mux"
)

const caseContentFixtureSQL = `
DROP TRIGGER inventory_test_stock ON product_locations;
ALTER TABLE cases ADD COLUMN description TEXT;ALTER TABLE cases ADD COLUMN case_type TEXT DEFAULT 'dynamic';ALTER TABLE cases ADD COLUMN case_model_id INT;ALTER TABLE cases ADD COLUMN max_weight_kg NUMERIC;ALTER TABLE cases ADD COLUMN home_zone_id INT;ALTER TABLE cases ADD COLUMN barcode TEXT;ALTER TABLE cases ADD COLUMN rfid_tag TEXT;ALTER TABLE cases ADD COLUMN sealed_at TIMESTAMP;
ALTER TABLE case_child_contents ADD COLUMN created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP;ALTER TABLE case_child_contents ADD CONSTRAINT child_identity UNIQUE(child_case_id);
ALTER TABLE case_product_contents ADD COLUMN updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP;ALTER TABLE case_product_contents ADD COLUMN added_from_zone_id INT;ALTER TABLE case_product_contents ADD CONSTRAINT quantity_identity UNIQUE(case_id,product_id);
ALTER TABLE warehouse_tasks ADD COLUMN task_id BIGSERIAL;ALTER TABLE warehouse_tasks ADD COLUMN task_type TEXT;ALTER TABLE warehouse_tasks ADD COLUMN product_id INT;ALTER TABLE warehouse_tasks ADD COLUMN quantity NUMERIC;ALTER TABLE warehouse_tasks ADD COLUMN from_zone_id INT;ALTER TABLE warehouse_tasks ADD COLUMN to_zone_id INT;ALTER TABLE warehouse_tasks ADD COLUMN job_id INT;ALTER TABLE warehouse_tasks ADD COLUMN assigned_to INT;ALTER TABLE warehouse_tasks ADD COLUMN updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP;
ALTER TABLE users ADD COLUMN is_admin BOOL DEFAULT true;
ALTER TABLE jobs ADD COLUMN job_code TEXT;ALTER TABLE jobs ADD COLUMN startdate TIMESTAMP;ALTER TABLE jobs ADD COLUMN enddate TIMESTAMP;ALTER TABLE jobs ADD COLUMN updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP;
CREATE TABLE job_packages(job_package_id INT PRIMARY KEY,job_id INT);ALTER TABLE job_package_reservations ADD COLUMN job_package_id INT;
ALTER TABLE device_movements ADD COLUMN from_case_id INT;ALTER TABLE device_movements ADD COLUMN to_case_id INT;
ALTER TABLE case_events ADD COLUMN device_id TEXT;ALTER TABLE case_events ADD COLUMN product_id INT;ALTER TABLE case_events ADD COLUMN quantity NUMERIC;
DELETE FROM case_child_contents;DELETE FROM case_product_contents;DELETE FROM devicescases;
UPDATE devices SET current_case_id=NULL,zone_id=1,current_location='warehouse';
UPDATE cases SET zone_id=1,workflow_status='empty',max_weight_kg=30,barcode='CASE-'||caseid;
UPDATE storage_zones SET max_weight_kg=100,max_volume_m3=100,capacity=100;
INSERT INTO cases(caseid,name,zone_id,workflow_status,barcode,max_weight_kg) VALUES(3,'Sealed nested kit',1,'sealed','CASE-3',30),(4,'Second outer',1,'empty','CASE-4',30);
`

func TestWarehouseCaseContentAtomicConservationContextAndReplay(t *testing.T) {
	dsn := os.Getenv("WAREHOUSE_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("owned _test PostgreSQL required")
	}
	parsed, err := url.Parse(dsn)
	if err != nil || !strings.HasSuffix(parsed.Path, "_test") {
		t.Fatal("owned database required")
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
	exec(`DROP SCHEMA IF EXISTS warehouse_case_content_test CASCADE;CREATE SCHEMA warehouse_case_content_test;SET search_path TO warehouse_case_content_test`)
	defer db.Exec(`DROP SCHEMA warehouse_case_content_test CASCADE`)
	exec(warehouseDeviceFixtureSQL)
	exec(inventoryMCPFixtureSQL)
	exec(caseContentFixtureSQL)
	old := repository.DB
	repository.DB = db
	defer func() { repository.DB = old }()
	for _, ensure := range []func() error{EnsureWarehouseDeviceVersionSchema, EnsureWarehouseCaseVersionSchema, EnsureWarehouseCaseTemplateLifecycleSchema, EnsureWarehouseCaseContentSafetySchema} {
		if err = ensure(); err != nil {
			t.Fatal(err)
		}
	}
	// Stock sync applies to both quantity locations and packed contents.
	exec(`CREATE TRIGGER product_locations_sync_stock AFTER INSERT OR UPDATE OR DELETE ON product_locations FOR EACH ROW EXECUTE FUNCTION sync_product_stock_from_locations()`)
	t.Setenv("CORES_JWT_SECRET", strings.Repeat("k", 48))
	call := func(op, key, scope string, in map[string]any, want int) map[string]any {
		t.Helper()
		raw, _ := json.Marshal(in)
		r := httptest.NewRequest("POST", "/contents", bytes.NewReader(raw))
		r = mux.SetURLVars(r, map[string]string{"operation": op})
		r = r.WithContext(context.WithValue(r.Context(), middleware.UserContextKey, &models.User{UserID: 11, IsActive: true, IsAdmin: true}))
		r.Header.Set("X-Cores-Origin", "MCP/AI")
		r.Header.Set("Idempotency-Key", key)
		token, e := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{"uid": 11, "mcp_scope": scope, "exp": time.Now().Add(time.Hour).Unix()}).SignedString([]byte(strings.Repeat("k", 48)))
		if e != nil {
			t.Fatal(e)
		}
		r.AddCookie(&http.Cookie{Name: "cores_token", Value: token})
		w := httptest.NewRecorder()
		CaseContentMCP(w, r)
		var out map[string]any
		if json.Unmarshal(w.Body.Bytes(), &out) != nil || w.Code != want {
			t.Fatalf("%s status%d want%d: %s", op, w.Code, want, w.Body.String())
		}
		return out
	}
	run := func(op, key string, in map[string]any, want int) map[string]any {
		return call(op, key, "cores:warehouse:update", in, want)
	}
	snapshot := func() string {
		t.Helper()
		var value string
		err := db.QueryRow(`SELECT jsonb_build_array((SELECT jsonb_agg(to_jsonb(t) ORDER BY caseid) FROM cases t),(SELECT jsonb_agg(to_jsonb(t) ORDER BY deviceid) FROM devices t),(SELECT jsonb_agg(to_jsonb(t) ORDER BY productid) FROM products t),(SELECT jsonb_agg(to_jsonb(t) ORDER BY caseid,deviceid) FROM devicescases t),(SELECT jsonb_agg(to_jsonb(t) ORDER BY case_id,product_id) FROM case_product_contents t),(SELECT jsonb_agg(to_jsonb(t) ORDER BY parent_case_id,child_case_id) FROM case_child_contents t),(SELECT jsonb_agg(to_jsonb(t) ORDER BY product_id,zone_id) FROM product_locations t),(SELECT count(*) FROM audit_log),(SELECT count(*) FROM case_events),(SELECT count(*) FROM device_movements),(SELECT count(*) FROM warehouse_product_mutation_receipts))::text`).Scan(&value)
		if err != nil {
			t.Fatal(err)
		}
		return value
	}
	confirm := func(op string, in map[string]any) map[string]any {
		t.Helper()
		in["preview"] = true
		in["confirm_change"] = false
		delete(in, "expected_context")
		delete(in, "expected_updated_at")
		delete(in, "confirmation_text")
		before := snapshot()
		out := run(op, "", in, 200)
		if out["ready_to_execute"] != true || snapshot() != before {
			t.Fatalf("preview failed or mutated: %#v", out)
		}
		in["expected_context"] = out["expected_context"]
		in["expected_updated_at"] = out["expected_updated_at"]
		in["confirmation_text"] = out["confirmation_text_required"]
		in["confirm_change"] = true
		in["preview"] = false
		return in
	}
	count := func(q string) float64 {
		t.Helper()
		var n float64
		if err := db.QueryRow(q).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	in := confirm("pack_product", map[string]any{"case_id": 1, "product_id": 3, "quantity": 2.125, "source_zone_id": 1})
	call("pack_product", "", "cores:warehouse:create", in, 403)
	run("pack_product", "", in, 428)
	before := snapshot()
	bad := caseContentCopy(in)
	bad["confirmation_text"] = "PACK_PRODUCT"
	run("pack_product", "wrong-phrase", bad, 428)
	if snapshot() != before {
		t.Fatal("wrong confirmation changed state")
	}
	for _, table := range []string{"case_events", "audit_log", "warehouse_product_mutation_receipts"} {
		exec(fmt.Sprintf(`CREATE FUNCTION reject_case_content_%s() RETURNS TRIGGER LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'injected final failure';END $$;CREATE TRIGGER reject_case_content BEFORE INSERT ON %s FOR EACH ROW EXECUTE FUNCTION reject_case_content_%s()`, table, table, table))
		run("pack_product", "pack-quantity-retry", in, 500)
		if snapshot() != before {
			t.Fatal("partial physical write after", table)
		}
		exec("DROP TRIGGER reject_case_content ON " + table)
	}
	done := run("pack_product", "pack-quantity-retry", in, 200)
	if done["operation_status"] != "executed" || count(`SELECT stock_quantity FROM products WHERE productid=3`) != 5.5 || count(`SELECT quantity FROM product_locations WHERE product_id=3 AND zone_id=1`) != 3.375 {
		t.Fatal(done)
	}
	after := snapshot()
	for i := 0; i < 2; i++ {
		if err = EnsureWarehouseCaseContentSafetySchema(); err != nil {
			t.Fatal(err)
		}
		if snapshot() != after {
			t.Fatal("migration restart changed business versions")
		}
		if !reflect.DeepEqual(run("pack_product", "pack-quantity-retry", in, 200), done) || snapshot() != after {
			t.Fatal("durable replay mutated")
		}
	}
	exec(`UPDATE users SET is_admin=false`)
	run("pack_product", "pack-quantity-retry", in, 403)
	exec(`UPDATE users SET is_admin=true`)
	// Device pack retains future pick/package reservations and captures job version.
	exec(`INSERT INTO job_devices VALUES('DEV-00000001',1,'reserved');INSERT INTO job_packages VALUES(1,1);INSERT INTO job_package_reservations VALUES('DEV-00000001','reserved',1)`)
	dev := confirm("pack_device", map[string]any{"case_id": 1, "device_id": "DEV-00000001"})
	exec(`UPDATE jobs SET updated_at=updated_at+INTERVAL '1 microsecond' WHERE jobid=1`)
	stale := run("pack_device", "stale-job", dev, 200)
	if stale["ready_to_execute"] != false {
		t.Fatal("job version change ignored", stale)
	}
	dev = confirm("pack_device", dev)
	run("pack_device", "pack-device", dev, 200)
	if count(`SELECT current_case_id FROM devices WHERE deviceid='DEV-00000001'`) != 1 || count(`SELECT count(*) FROM job_package_reservations WHERE reservation_status='reserved'`) != 1 {
		t.Fatal("device membership or reservation lost")
	}
	unpack := confirm("unpack_device", map[string]any{"case_id": 1, "device_id": "DEV-00000001", "destination_zone_id": 3})
	run("unpack_device", "unpack-device", unpack, 200)
	if count(`SELECT count(*) FROM devices WHERE deviceid='DEV-00000001' AND zone_id=3 AND current_case_id IS NULL`) != 1 {
		t.Fatal("device detach inconsistent")
	}
	partial := confirm("unpack_product", map[string]any{"case_id": 1, "product_id": 3, "quantity": .125, "destination_zone_id": 3})
	run("unpack_product", "unpack-partial", partial, 200)
	if count(`SELECT stock_quantity FROM products WHERE productid=3`) != 5.5 || count(`SELECT quantity FROM case_product_contents WHERE case_id=1`) != 2 {
		t.Fatal("partial quantity not conserved")
	}
	// Populate and seal a child kit through ordinary SQL, then move it as a unit.
	exec(`UPDATE cases SET workflow_status='empty' WHERE caseid=3;INSERT INTO devicescases VALUES('DEV-00000002',3);UPDATE cases SET workflow_status='sealed',sealed_at=CURRENT_TIMESTAMP WHERE caseid=3`)
	child := confirm("pack_case", map[string]any{"case_id": 1, "child_case_id": 3})
	run("pack_case", "pack-child", child, 200)
	blocked := run("pack_device", "", map[string]any{"case_id": 3, "device_id": "DEV-00000003"}, 200)
	if blocked["ready_to_execute"] != false {
		t.Fatal("sealed nested parent accepted")
	}
	if _, err = db.Exec(`DELETE FROM devicescases WHERE caseid=3`); err == nil {
		t.Fatal("native sealed content bypass")
	}
	out := confirm("unpack_case", map[string]any{"case_id": 1, "child_case_id": 3, "destination_zone_id": 3})
	run("unpack_case", "unpack-child", out, 200)
	if count(`SELECT count(*) FROM cases WHERE caseid=3 AND sealed_at IS NOT NULL AND workflow_status='sealed' AND zone_id=3`) != 1 {
		t.Fatal("child seal lost")
	}
	// Complete unpack moves only direct contents and the empty outer case.
	child = confirm("pack_case", map[string]any{"case_id": 1, "child_case_id": 3})
	run("pack_case", "repack-child", child, 200)
	all := confirm("unpack_all", map[string]any{"case_id": 1, "destination_zone_id": 3})
	run("unpack_all", "unpack-all", all, 200)
	if count(`SELECT stock_quantity FROM products WHERE productid=3`) != 5.5 || count(`SELECT count(*) FROM devicescases WHERE caseid=3`) != 1 || count(`SELECT count(*) FROM cases WHERE caseid=1 AND zone_id=3 AND workflow_status='empty'`) != 1 {
		t.Fatal("complete unpack lost nested stock")
	}
	// Capacity, missing weight, defects and malformed contracts fail before writes.
	for _, q := range []string{`UPDATE storage_zones SET capacity=1 WHERE zone_id=3`, `UPDATE storage_zones SET capacity=100;UPDATE cases SET max_weight_kg=.1 WHERE caseid=1`, `UPDATE cases SET max_weight_kg=30;UPDATE products SET weight=NULL WHERE productid=3`} {
		exec(q)
		state := snapshot()
		out := run("pack_product", "", map[string]any{"case_id": 1, "product_id": 3, "quantity": .125, "source_zone_id": 3}, 200)
		if out["ready_to_execute"] != false || snapshot() != state {
			t.Fatal("physical projection guard ignored", out)
		}
	}
	exec(`ALTER TABLE products ALTER COLUMN stock_quantity TYPE DOUBLE PRECISION;UPDATE products SET weight=1 WHERE productid=3;UPDATE cases SET max_weight_kg=NULL WHERE caseid=1;UPDATE storage_zones SET capacity=NULL,max_weight_kg=NULL,max_volume_m3=NULL;INSERT INTO case_product_contents(case_id,product_id,quantity) VALUES(1,3,10000000)`)
	overflow := run("unpack_product", "", map[string]any{"case_id": 1, "product_id": 3, "quantity": 10000000, "destination_zone_id": 3}, 200)
	missingBound := false
	for _, field := range overflow["required_missing_fields"].([]any) {
		missingBound = missingBound || field == "bounded_location_quantity"
	}
	if !missingBound || overflow["ready_to_execute"] != false {
		t.Fatal("storage numeric overflow accepted", overflow)
	}

	// Simulate retained pre-guard ambiguity in this owned schema only. Unpacking
	// one membership must not strand the same device in another physical case.
	exec(`ALTER TABLE devicescases DISABLE TRIGGER devicescases_guard_physical;INSERT INTO devicescases(caseid,deviceid) VALUES(1,'DEV-00000001'),(4,'DEV-00000001');ALTER TABLE devicescases ENABLE TRIGGER devicescases_guard_physical`)
	state := snapshot()
	ambiguous := run("unpack_device", "", map[string]any{"case_id": 1, "device_id": "DEV-00000001", "destination_zone_id": 3}, 200)
	if ambiguous["ready_to_execute"] != false || snapshot() != state {
		t.Fatal("ambiguous physical membership accepted", ambiguous)
	}

	for _, bad := range []map[string]any{{"case_id": 1, "device_id": "DEV-00000001", "quantity": 1}, {"case_id": 1, "device_id": "DEV-00000001", "unexpected": true}} {
		run("pack_device", "", bad, 400)
	}
}

func TestWarehouseCaseContentLegacyDeniesSignedDelegationWithoutOrigin(t *testing.T) {
	t.Setenv("CORES_JWT_SECRET", strings.Repeat("k", 48))
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{"uid": 11, "mcp_scope": "cores:warehouse:update", "exp": time.Now().Add(time.Hour).Unix()}).SignedString([]byte(strings.Repeat("k", 48)))
	if err != nil {
		t.Fatal(err)
	}
	for _, handler := range []http.HandlerFunc{CreateCase, UpdateCase, DeleteCase, CreateHandlingUnit, UpdateHandlingUnit, DeleteHandlingUnit, SealHandlingUnit, UnsealHandlingUnit, DispatchHandlingUnit, MoveHandlingUnit, ReturnHandlingUnit, PackHandlingUnitScan, RemoveHandlingUnitDevice, RemoveHandlingUnitProduct, RemoveHandlingUnitChild, UnpackHandlingUnit, AddDevicesToCase, RemoveDeviceFromCase} {
		r := httptest.NewRequest("POST", "/legacy", strings.NewReader(`{}`))
		r.AddCookie(&http.Cookie{Name: "cores_token", Value: token})
		w := httptest.NewRecorder()
		handler(w, r)
		if w.Code != 403 {
			t.Fatal("legacy bypass", w.Code)
		}
	}
}
