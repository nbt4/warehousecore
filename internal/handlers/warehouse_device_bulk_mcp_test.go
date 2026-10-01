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
	"testing"

	"warehousecore/internal/middleware"
	"warehousecore/internal/models"
	"warehousecore/internal/repository"
)

func TestWarehouseDeviceBulkAtomicity(t *testing.T) {
	dsn := os.Getenv("WAREHOUSE_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("disposable _test database required")
	}
	parsed, err := url.Parse(dsn)
	if err != nil || !strings.HasSuffix(parsed.Path, "_test") {
		t.Fatal("dedicated _test database required")
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
	exec("DROP SCHEMA IF EXISTS warehouse_device_bulk_test CASCADE; CREATE SCHEMA warehouse_device_bulk_test; SET search_path TO warehouse_device_bulk_test")
	defer db.Exec("DROP SCHEMA warehouse_device_bulk_test CASCADE")
	exec(warehouseDeviceFixtureSQL)
	exec(`ALTER TABLE storage_zones ADD COLUMN profile_id INT; CREATE TABLE location_profiles(profile_id INT,allow_devices BOOLEAN); INSERT INTO location_profiles VALUES(1,false); UPDATE storage_zones SET capacity=3 WHERE zone_id=1; INSERT INTO storage_zones VALUES(4,NULL,'DISALLOWED','Disallowed',true,true,'available',NULL,1)`)
	old := repository.DB
	repository.DB = db
	defer func() { repository.DB = old }()
	if err := EnsureWarehouseDeviceVersionSchema(); err != nil {
		t.Fatal(err)
	}
	call := func(body string, key string, admin bool, want int) *httptest.ResponseRecorder {
		t.Helper()
		r := httptest.NewRequest(http.MethodPost, "/batch", bytes.NewBufferString(body))
		r.Header.Set("X-Cores-Origin", "MCP/AI")
		r.Header.Set("Idempotency-Key", key)
		r = r.WithContext(context.WithValue(r.Context(), middleware.UserContextKey, &models.User{UserID: 11, IsAdmin: admin}))
		w := httptest.NewRecorder()
		CreateDevicesBulkMCP(w, r)
		if w.Code != want {
			t.Fatalf("status %d want %d: %s", w.Code, want, w.Body.String())
		}
		return w
	}
	counts := func(devices, audits, receipts int) {
		t.Helper()
		for table, want := range map[string]int{"devices": devices, "audit_log": audits, "warehouse_product_mutation_receipts": receipts} {
			var n int
			if err := db.QueryRow("SELECT count(*) FROM " + table).Scan(&n); err != nil || n != want {
				t.Fatalf("%s count %d want %d: %v", table, n, want, err)
			}
		}
	}
	phraseFor := func(body string) string {
		t.Helper()
		var payload map[string]any
		if err := json.Unmarshal([]byte(body), &payload); err != nil {
			t.Fatal(err)
		}
		payload["preview"] = true
		raw, _ := json.Marshal(payload)
		w := call(string(raw), "", true, 200)
		var preview map[string]any
		if err := json.Unmarshal(w.Body.Bytes(), &preview); err != nil {
			t.Fatal(err)
		}
		payload["preview"] = false
		payload["confirmation_text"] = preview["required_confirmation_text"]
		raw, _ = json.Marshal(payload)
		return string(raw)
	}
	draft := `{"devices":[{"product_id":1,"serial_number":"SER-A","zone_id":1},{"product_id":2,"serial_number":"SER-B","zone_id":1,"condition_rating":4.5,"condition_status":"defective"}]}`
	call(draft, "", false, 403)
	call(draft, "", true, 200)
	counts(0, 0, 0)
	confirmed := strings.TrimSuffix(draft, "}") + `,"confirm_creation":true}`
	preview := strings.TrimSuffix(confirmed, "}") + `,"preview":true}`
	call(preview, "", true, 200)
	counts(0, 0, 0)
	call(confirmed, "", true, 428)
	counts(0, 0, 0)
	for _, body := range []string{
		`{"devices":[{"product_id":1,"serial_number":"Same"},{"product_id":1,"serial_number":" same "}]}`,
		`{"devices":[{"product_id":1,"barcode":"SCAN"},{"product_id":1,"qr_code":" scan "}]}`,
		`{"devices":[{"product_id":1,"zone_id":4}]}`,
		`{"devices":[{"product_id":3}]}`,
		`{"devices":[{"product_id":1,"zone_id":1},{"product_id":1,"zone_id":1},{"product_id":1,"zone_id":1},{"product_id":1,"zone_id":1}]}`,
	} {
		call(body, "", true, 409)
		counts(0, 0, 0)
	}
	for _, body := range []string{`{"devices":[]}`, `{"devices":[null]}`, `{"devices":[{"product_id":1,"sql":"bad"}]}`, `{"devices":[{"product_id":1,"condition_rating":4.55}]}`, draft + ` {}`, `{"devices":[{"product_id":1}],"unknown":true}`} {
		call(body, "", true, 400)
		counts(0, 0, 0)
	}
	var oversized []map[string]any
	for i := 0; i < 101; i++ {
		oversized = append(oversized, map[string]any{"product_id": 1})
	}
	raw, _ := json.Marshal(map[string]any{"devices": oversized})
	call(string(raw), "", true, 400)
	call(`{"devices":[{"product_id":1,"notes":"`+strings.Repeat("a", 1024*1024)+`"}]}`, "", true, 400)
	counts(0, 0, 0)
	// A failure on the second audit rolls back the first device, identities and receipt.
	exec(`CREATE FUNCTION reject_bulk_audit() RETURNS TRIGGER AS $$ BEGIN IF NEW.new_values#>>'{after,notes}'='fail-audit' THEN RAISE EXCEPTION 'audit failed'; END IF; RETURN NEW; END $$ LANGUAGE plpgsql; CREATE TRIGGER reject_bulk_audit BEFORE INSERT ON audit_log FOR EACH ROW EXECUTE FUNCTION reject_bulk_audit()`)
	fail := `{"devices":[{"product_id":1,"serial_number":"ROLLBACK-A"},{"product_id":1,"serial_number":"ROLLBACK-B","notes":"fail-audit"}],"confirm_creation":true}`
	call(phraseFor(fail), "bulk-audit-failure", true, 500)
	counts(0, 0, 0)
	var identifiers int
	db.QueryRow("SELECT count(*) FROM inventory_identifiers").Scan(&identifiers)
	if identifiers != 0 {
		t.Fatal("scan identity leaked after rollback")
	}
	exec("DROP TRIGGER reject_bulk_audit ON audit_log")
	confirmed = phraseFor(confirmed)
	created := call(confirmed, "bulk-create-once", true, 201)
	counts(2, 2, 1)
	replay := call(confirmed, "bulk-create-once", true, 201)
	counts(2, 2, 1)
	var a, b map[string]any
	json.Unmarshal(created.Body.Bytes(), &a)
	json.Unmarshal(replay.Body.Bytes(), &b)
	if !reflect.DeepEqual(a, b) {
		t.Fatalf("replay differs: %#v %#v", a, b)
	}
	changed := strings.ReplaceAll(strings.ReplaceAll(confirmed, "SER-A", "Changed"), `"zone_id":1`, `"zone_id":null`)
	call(changed, "bulk-wrong-phrase", true, 428)
	call(phraseFor(strings.ReplaceAll(changed, "SER-B", "ChangedB")), "bulk-create-once", true, 409)
	counts(2, 2, 1)
	// Combined occupancy includes existing stock, even though each single item fits.
	call(`{"devices":[{"product_id":1,"zone_id":1},{"product_id":1,"zone_id":1}]}`, "", true, 409)
	counts(2, 2, 1)
	// Archived serials remain reserved.
	exec("UPDATE devices SET lifecycle_status='archived' WHERE serialnumber='SER-A'")
	call(`{"devices":[{"product_id":1,"serial_number":"ser-a"}]}`, "", true, 409)
	// Cycles are rejected by bounded traversal rather than exhausting a recursive query.
	exec("UPDATE storage_zones SET parent_zone_id=zone_id WHERE zone_id=1")
	call(`{"devices":[{"product_id":1,"zone_id":1}]}`, "", true, 409)
	counts(2, 2, 1)
}
