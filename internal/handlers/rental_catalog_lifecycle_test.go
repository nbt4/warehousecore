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
	"time"

	"warehousecore/internal/middleware"
	"warehousecore/internal/models"
	"warehousecore/internal/repository"

	"github.com/golang-jwt/jwt/v5"
	"github.com/gorilla/mux"
)

func TestNativeExternalCatalogWritersRejectDelegationBeforeDatabase(t *testing.T) {
	t.Setenv("CORES_JWT_SECRET", strings.Repeat("native-catalog-", 3))
	for _, scope := range []any{"cores:rental:update", "cores:warehouse:create", nil, 1, []any{"cores:rental:archive"}} {
		token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{"uid": 1, "mcp_scope": scope, "exp": time.Now().Add(time.Minute).Unix()}).SignedString([]byte(os.Getenv("CORES_JWT_SECRET")))
		if err != nil {
			t.Fatal(err)
		}
		for _, fn := range []http.HandlerFunc{CreateRentalEquipment, UpdateRentalEquipment, DeleteRentalEquipment, CreateRentalFieldDefinition, UpdateRentalFieldDefinition, DeleteRentalFieldDefinition} {
			req := httptest.NewRequest("POST", "/", strings.NewReader(`{}`))
			req.AddCookie(&http.Cookie{Name: "cores_token", Value: token})
			response := httptest.NewRecorder()
			fn(response, req)
			if response.Code != 403 || !strings.Contains(response.Body.String(), "guided_rental_catalog_required") {
				t.Fatal("native delegation bypass", response.Code, response.Body.String())
			}
		}
	}
}

func TestNativeExternalCatalogArchivePreservesPricesAndAtomicAudit(t *testing.T) {
	dsn := os.Getenv("WAREHOUSE_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("isolated _test PostgreSQL required")
	}
	u, err := url.Parse(dsn)
	if err != nil || !strings.HasSuffix(u.Path, "_test") {
		t.Fatal("dedicated test database required")
	}
	q := u.Query()
	q.Set("search_path", "native_external_catalog_test")
	u.RawQuery = q.Encode()
	db, err := sql.Open("postgres", u.String())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	db.SetMaxOpenConns(2)
	exec := func(query string, args ...any) {
		t.Helper()
		if _, e := db.Exec(query, args...); e != nil {
			t.Fatal(e)
		}
	}
	exec(`DROP SCHEMA IF EXISTS native_external_catalog_test CASCADE;CREATE SCHEMA native_external_catalog_test`)
	defer db.Exec(`DROP SCHEMA native_external_catalog_test CASCADE`)
	raw, err := os.ReadFile("testdata/external_catalog_lifecycle.sql")
	if err != nil {
		t.Fatal(err)
	}
	exec(string(raw))
	exec(RentalExternalCatalogLifecycleSQL)
	previous := repository.DB
	repository.DB = db
	defer func() { repository.DB = previous }()
	router := mux.NewRouter()
	router.HandleFunc("/catalog", CreateRentalEquipment).Methods("POST")
	router.HandleFunc("/catalog/{id}", UpdateRentalEquipment).Methods("PUT")
	router.HandleFunc("/catalog/{id}", DeleteRentalEquipment).Methods("DELETE")
	call := func(method, path string, in any) *httptest.ResponseRecorder {
		t.Helper()
		body, _ := json.Marshal(in)
		req := httptest.NewRequest(method, path, bytes.NewReader(body))
		req = req.WithContext(context.WithValue(req.Context(), middleware.UserContextKey, &models.User{UserID: 1, IsAdmin: true}))
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		return w
	}
	exec(`INSERT INTO job_rental_equipment(job_id,equipment_id,total_cost,rental_unit_price) VALUES(1,1,12.35,12.35)`)
	if w := call("DELETE", "/catalog/1", nil); w.Code != 409 {
		t.Fatal("open job archive", w.Code, w.Body.String())
	}
	exec(`UPDATE jobs SET statusid=2 WHERE jobid=1`)
	if w := call("DELETE", "/catalog/1", nil); w.Code != 200 || !strings.Contains(w.Body.String(), "Archived") {
		t.Fatal(w.Code, w.Body.String())
	}
	var active bool
	var price, cost float64
	var value string
	if err = db.QueryRow(`SELECT re.is_active,re.rental_price,r.total_cost,v.value FROM rental_equipment re JOIN job_rental_equipment r ON r.equipment_id=re.id JOIN rental_equipment_field_values v ON v.equipment_id=re.id WHERE re.id=1`).Scan(&active, &price, &cost, &value); err != nil || active || price != 12.35 || cost != 12.35 || value != "IP44" {
		t.Fatal("archive lost snapshots or custom fields", active, price, cost, value, err)
	}
	if w := call("DELETE", "/catalog/1", nil); w.Code != 200 {
		t.Fatal("repeat native archive", w.Code)
	}
	var count int
	db.QueryRow(`SELECT count(*) FROM audit_log WHERE action='rental_equipment.archive' AND old_values->>'is_active'='true' AND new_values#>>'{after,is_active}'='false'`).Scan(&count)
	if count != 1 {
		t.Fatal("native archive audit missing/duplicated", count)
	}
	before := map[string]any{"product_name": "Synthetic Rental", "supplier_name": "Synthetic Supplier", "rental_price": 12.35, "customer_price": 25.50, "category": "Audio", "description": "Business description", "notes": "Private catalog note", "is_active": true}

	before["field_values"] = []map[string]any{{"field_definition_id": 1, "value": "IP65", "sort_order": 0}}
	if w := call("PUT", "/catalog/1", before); w.Code != 409 {
		t.Fatal("native restore changed retained properties", w.Code, w.Body.String())
	}
	before["field_values"] = []map[string]any{}
	if w := call("PUT", "/catalog/1", before); w.Code != 409 {
		t.Fatal("native restore silently ignored explicit property removal", w.Code, w.Body.String())
	}
	before["field_values"] = []map[string]any{{"field_definition_id": 1, "value": "IP44", "sort_order": 0}}
	if w := call("PUT", "/catalog/1", before); w.Code != 200 {
		t.Fatal("native restore", w.Code, w.Body.String())
	}
	db.QueryRow(`SELECT value FROM rental_equipment_field_values WHERE equipment_id=1`).Scan(&value)
	if value != "IP44" {
		t.Fatal("native restore deleted custom values")
	}
	// An explicit empty editor list is unchanged when no values are stored.
	exec(`DELETE FROM rental_equipment_field_values WHERE equipment_id=1`)
	if w := call("DELETE", "/catalog/1", nil); w.Code != 200 {
		t.Fatal("archive without custom values", w.Code, w.Body.String())
	}
	before["field_values"] = []map[string]any{}
	if w := call("PUT", "/catalog/1", before); w.Code != 200 {
		t.Fatal("restore without custom values", w.Code, w.Body.String())
	}
	exec(`CREATE FUNCTION fail_native_audit() RETURNS trigger AS $$BEGIN RAISE EXCEPTION 'synthetic audit failure';END;$$ LANGUAGE plpgsql;CREATE TRIGGER fail_native_audit BEFORE INSERT ON audit_log FOR EACH ROW EXECUTE FUNCTION fail_native_audit()`)
	if w := call("DELETE", "/catalog/1", nil); w.Code != 500 {
		t.Fatal("audit failure not reported", w.Code, w.Body.String())
	}
	db.QueryRow(`SELECT is_active FROM rental_equipment WHERE id=1`).Scan(&active)
	if !active {
		t.Fatal("native audit failure committed archive")
	}
	before["product_name"] = "Rollback product"
	if w := call("POST", "/catalog", before); w.Code != 500 {
		t.Fatal("native create audit failure", w.Code, w.Body.String())
	}
	db.QueryRow(`SELECT count(*) FROM rental_equipment`).Scan(&count)
	if count != 1 {
		t.Fatal("native creation left partial record", count)
	}
}

func TestExternalCatalogMigrationMatchesNativeStartup(t *testing.T) {
	raw, err := os.ReadFile("../../migrations/065_rental_external_catalog_lifecycle.sql")
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(raw)) != strings.TrimSpace(RentalExternalCatalogLifecycleSQL) {
		t.Fatal("native startup/catalog migration diverged")
	}
}
