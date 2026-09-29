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

	"github.com/gorilla/mux"
	_ "github.com/lib/pq"
	"warehousecore/internal/middleware"
	"warehousecore/internal/models"
	"warehousecore/internal/repository"
)

func TestWarehouseProductMCPRelationVersionAuditAndReplay(t *testing.T) {
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
	const schema = "warehouse_product_mcp_relation_test"
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
		`CREATE TABLE products(productid INT PRIMARY KEY,name TEXT,lifecycle_status TEXT,updated_at TIMESTAMP)`,
		`CREATE TABLE product_dependencies(id SERIAL PRIMARY KEY,product_id INT,dependency_product_id INT,is_optional BOOL,relation_type TEXT,assignment_scope TEXT,default_quantity NUMERIC(10,2),notes TEXT,created_at TIMESTAMP DEFAULT now(),updated_at TIMESTAMP DEFAULT now(),UNIQUE(product_id,dependency_product_id))`,
		`CREATE TABLE audit_log(id BIGSERIAL PRIMARY KEY,user_id BIGINT,action TEXT,entity_type TEXT,entity_id TEXT,old_values JSONB,new_values JSONB,ip_address TEXT,user_agent TEXT)`,
		`CREATE TABLE warehouse_product_mutation_receipts(id BIGSERIAL PRIMARY KEY,user_id BIGINT NOT NULL,operation VARCHAR(80) NOT NULL,key_hash CHAR(64) NOT NULL,request_hash CHAR(64) NOT NULL,response JSONB NOT NULL DEFAULT '{}'::jsonb,status_code INTEGER NOT NULL DEFAULT 200,created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,UNIQUE(user_id,operation,key_hash))`,
		`INSERT INTO products VALUES(1,'Mixer','active','2026-09-24T08:00:00.123456'),(2,'Cable','active','2026-09-24T08:00:00.123456')`,
	} {
		if _, err := db.Exec(statement); err != nil {
			t.Fatal(err)
		}
	}
	previousDB := repository.DB
	repository.DB = db
	defer func() { repository.DB = previousDB }()
	request := func(key, version, relation string) *httptest.ResponseRecorder {
		body, err := json.Marshal(map[string]any{"dependency_product_id": 2, "relation_type": relation, "assignment_scope": "product", "default_quantity": 2, "expectedUpdatedAt": version})
		if err != nil {
			t.Fatal(err)
		}
		r := httptest.NewRequest(http.MethodPost, "/api/v1/admin/products/1/dependencies", bytes.NewReader(body))
		r = mux.SetURLVars(r, map[string]string{"id": "1"})
		r = r.WithContext(context.WithValue(r.Context(), middleware.UserContextKey, &models.User{UserID: 11, Username: "tester", IsAdmin: true}))
		r.Header.Set("X-Cores-Origin", "MCP/AI")
		r.Header.Set("Idempotency-Key", key)
		w := httptest.NewRecorder()
		CreateProductDependency(w, r)
		return w
	}
	const version = "2026-09-24T08:00:00.123456Z"
	if got := request("relation-no-version", "", "compatible"); got.Code != http.StatusPreconditionRequired {
		t.Fatalf("missing version accepted: %d %s", got.Code, got.Body.String())
	}
	first := request("relation-create-1", version, "compatible")
	if first.Code != http.StatusOK {
		t.Fatalf("create: %d %s", first.Code, first.Body.String())
	}
	var created struct {
		RelationID int    `json:"relation_id"`
		UpdatedAt  string `json:"updated_at"`
	}
	if err := json.Unmarshal(first.Body.Bytes(), &created); err != nil || created.RelationID <= 0 || created.UpdatedAt == version {
		t.Fatalf("create response: %s %v", first.Body.String(), err)
	}
	if replay := request("relation-create-1", version, "compatible"); replay.Code != http.StatusOK {
		t.Fatalf("replay: %d %s", replay.Code, replay.Body.String())
	} else {
		var repeated struct {
			RelationID int `json:"relation_id"`
		}
		if err := json.Unmarshal(replay.Body.Bytes(), &repeated); err != nil || repeated.RelationID != created.RelationID {
			t.Fatalf("replay response: %s %v", replay.Body.String(), err)
		}
	}
	if changed := request("relation-create-1", version, "alternative"); changed.Code != http.StatusConflict {
		t.Fatalf("reused key accepted different payload: %d %s", changed.Code, changed.Body.String())
	}
	if stale := request("relation-update-stale", version, "alternative"); stale.Code != http.StatusConflict {
		t.Fatalf("stale version accepted: %d %s", stale.Code, stale.Body.String())
	}
	if second := request("relation-update-1", created.UpdatedAt, "alternative"); second.Code != http.StatusOK {
		t.Fatalf("update: %d %s", second.Code, second.Body.String())
	}
	var relationCount, auditCount, receiptCount int
	if err := db.QueryRow(`SELECT COUNT(*) FROM product_dependencies WHERE product_id=1 AND dependency_product_id=2 AND relation_type='alternative'`).Scan(&relationCount); err != nil || relationCount != 1 {
		t.Fatalf("relationship: %d %v", relationCount, err)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM audit_log WHERE action='product.relation.link' AND new_values->>'origin'='MCP/AI'`).Scan(&auditCount); err != nil || auditCount != 2 {
		t.Fatalf("audits: %d %v", auditCount, err)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM warehouse_product_mutation_receipts`).Scan(&receiptCount); err != nil || receiptCount != 2 {
		t.Fatalf("receipts: %d %v", receiptCount, err)
	}
}

func TestWarehouseProductUpdateReportsInvalidFieldType(t *testing.T) {
	r := httptest.NewRequest(http.MethodPut, "/api/v1/admin/products/1", strings.NewReader(`{"item_cost_per_day":"120.00"}`))
	r = mux.SetURLVars(r, map[string]string{"id": "1"})
	w := httptest.NewRecorder()
	UpdateProduct(w, r)
	if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "item_cost_per_day") {
		t.Fatalf("type error should name rejected field: %d %s", w.Code, w.Body.String())
	}
}
