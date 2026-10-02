package handlers

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
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

func TestWarehouseRelationsLifecycleAtomicDependencies(t *testing.T) {
	dsn := os.Getenv("WAREHOUSE_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("disposable _test database required")
	}
	u, err := url.Parse(dsn)
	if err != nil || !strings.HasSuffix(u.Path, "_test") {
		t.Fatal("dedicated _test database required")
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	const schema = "warehouse_relation_lifecycle_test"
	if _, err = db.Exec(`DROP SCHEMA IF EXISTS ` + schema + ` CASCADE;CREATE SCHEMA ` + schema + `;SET search_path TO ` + schema); err != nil {
		t.Fatal(err)
	}
	defer db.Exec(`DROP SCHEMA IF EXISTS ` + schema + ` CASCADE`)
	fixture := `CREATE TABLE warehouse_schema_migrations(version TEXT PRIMARY KEY);
 CREATE TABLE products(productid INT PRIMARY KEY,name TEXT,product_code TEXT,lifecycle_status TEXT DEFAULT 'active',updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP);
 CREATE TABLE product_dependencies(id SERIAL PRIMARY KEY,product_id INT REFERENCES products,dependency_product_id INT REFERENCES products,is_optional BOOL,relation_type TEXT,assignment_scope TEXT,default_quantity NUMERIC(10,2),notes TEXT,created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,UNIQUE(product_id,dependency_product_id));
 CREATE TABLE status(statusid INT PRIMARY KEY,status TEXT);
 CREATE TABLE jobs(jobid INT PRIMARY KEY,statusid INT,updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,startdate DATE,enddate DATE,deleted_at TIMESTAMP);
 CREATE TABLE job_product_requirements(job_id INT,product_id INT,quantity INT);
 CREATE TABLE job_positions(job_id INT,product_id INT);
 CREATE TABLE devices(deviceid TEXT PRIMARY KEY,productid INT);
 CREATE TABLE job_devices(jobid INT,deviceid TEXT);
 CREATE TABLE audit_log(id BIGSERIAL PRIMARY KEY,user_id BIGINT,action TEXT,entity_type TEXT,entity_id TEXT,old_values JSONB,new_values JSONB,user_agent TEXT);
 CREATE TABLE warehouse_product_mutation_receipts(id BIGSERIAL PRIMARY KEY,user_id BIGINT NOT NULL,operation VARCHAR(80) NOT NULL,key_hash CHAR(64) NOT NULL,request_hash CHAR(64) NOT NULL,response JSONB NOT NULL DEFAULT '{}'::jsonb,status_code INTEGER NOT NULL DEFAULT 200,created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,UNIQUE(user_id,operation,key_hash));
 CREATE FUNCTION warehouse_job_status_is_closed(state TEXT) RETURNS BOOLEAN AS $$ SELECT lower(trim(COALESCE(state,''))) IN ('completed','cancelled','abgeschlossen','storniert') $$ LANGUAGE SQL IMMUTABLE;
 INSERT INTO products(productid,name,product_code) VALUES(1,'Parent','P1'),(2,'Related','P2'),(3,'Ancestor','P3');
 INSERT INTO status VALUES(1,'open'),(2,'completed');`
	if _, err = db.Exec(fixture); err != nil {
		t.Fatal(err)
	}
	old := repository.DB
	repository.DB = db
	defer func() { repository.DB = old }()
	for i := 0; i < 2; i++ {
		if err = EnsureWarehouseProductRelationLifecycleSchema(); err != nil {
			t.Fatal(err)
		}
	}
	request := func(op, key string, in map[string]any, admin bool) (map[string]any, int) {
		t.Helper()
		raw, _ := json.Marshal(in)
		r := httptest.NewRequest("POST", "/mcp/product-relations/"+op, bytes.NewReader(raw))
		r = mux.SetURLVars(r, map[string]string{"operation": op})
		r = r.WithContext(context.WithValue(r.Context(), middleware.UserContextKey, &models.User{UserID: 11, IsAdmin: admin, Username: "tester"}))
		r.Header.Set("X-Cores-Origin", "MCP/AI")
		r.Header.Set("Idempotency-Key", key)
		w := httptest.NewRecorder()
		WarehouseProductRelationMCP(w, r)
		out := map[string]any{}
		if err = json.Unmarshal(w.Body.Bytes(), &out); err != nil {
			t.Fatal(w.Body.String())
		}
		return out, w.Code
	}
	preview := func(op string, in map[string]any) map[string]any {
		t.Helper()
		out, status := request(op, "", in, true)
		if status != 200 {
			t.Fatalf("preview %d: %v", status, out)
		}
		return out
	}
	contains := func(out map[string]any, field string) bool {
		fields, _ := out["required_fields"].([]any)
		for _, f := range fields {
			if f == field {
				return true
			}
		}
		return false
	}
	prepare := func(op string, in map[string]any) map[string]any {
		t.Helper()
		p := preview(op, in)
		if p["ready_to_execute"] != true {
			t.Fatalf("not ready %v", p)
		}
		out := map[string]any{}
		for k, v := range in {
			out[k] = v
		}
		out["expected_updated_at"] = p["expected_updated_at"]
		out["expected_context"] = p["expected_context"]
		out["confirm_change"] = true
		out["confirmation_text"] = p["required_confirmation_text"]
		return out
	}
	execSQL := func(query string) {
		t.Helper()
		if _, err = db.Exec(query); err != nil {
			t.Fatal(err)
		}
	}
	blockedSQL := func(query string) {
		t.Helper()
		if _, err = db.Exec(query); err == nil {
			t.Fatal("unchecked writer accepted " + query)
		}
	}
	count := func(table string) int {
		var n int
		if err = db.QueryRow("SELECT count(*) FROM " + table).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	create := map[string]any{"product_id": 1, "dependency_product_id": 2, "relation_type": "required", "assignment_scope": "case", "default_quantity": 2.25, "notes": " retained work note "}
	if _, status := request("create", "", create, false); status != 403 {
		t.Fatal("nonadmin accepted")
	}
	for _, quantity := range []float64{0, -1, 1.001, 100000000} {
		bad := map[string]any{}
		for k, v := range create {
			bad[k] = v
		}
		bad["default_quantity"] = quantity
		if !contains(preview("create", bad), "default_quantity") {
			t.Fatal("invalid quantity", quantity)
		}
	}
	draft := prepare("create", create)
	if count("product_dependencies") != 0 || count("warehouse_product_mutation_receipts") != 0 {
		t.Fatal("preview mutated")
	}
	execSQL(`UPDATE products SET name='Changed target' WHERE productid=2`)
	if out, status := request("create", "stale-create", draft, true); status != 200 || !contains(out, "expected_context") {
		t.Fatal("changed related product accepted", out, status)
	}
	draft = prepare("create", create)
	first, status := request("create", "relation-create", draft, true)
	if status != 200 || first["operation_status"] != "created" {
		t.Fatalf("create %d: %v", status, first)
	}
	id := int64(first["relationship"].(map[string]any)["relation_id"].(float64))
	existing := map[string]any{"relation_id": id}
	repeated, status := request("create", "relation-create", draft, true)
	if status != 200 || !reflect.DeepEqual(first, repeated) {
		t.Fatal("durable replay changed")
	}
	if !contains(preview("create", create), "existing_relationship") {
		t.Fatal("duplicate relation not identified")
	}
	cycle := map[string]any{"product_id": 2, "dependency_product_id": 1, "relation_type": "included"}
	if !contains(preview("create", cycle), "dependency_cycle") {
		t.Fatal("mandatory cycle accepted")
	}
	// Reciprocal discovery links remain valid and do not enter the mandatory graph.
	cycle["relation_type"] = "compatible"
	if p := preview("create", cycle); p["ready_to_execute"] != true {
		t.Fatal("reciprocal compatibility blocked", p)
	}
	update := map[string]any{"relation_id": id, "default_quantity": 3.5, "notes": ""}
	in := prepare("update", update)
	// Every relation writer invalidates both products, including changes unrelated
	// to the exact row; complete graph contexts also reject concurrent inserts.
	execSQL(`INSERT INTO product_dependencies(product_id,dependency_product_id,relation_type,assignment_scope,default_quantity,is_optional) VALUES(3,1,'recommended','product',1,true)`)
	if out, status := request("update", "stale-graph", in, true); status != 200 || !contains(out, "expected_context") {
		t.Fatal("changed dependency graph accepted", status, out)
	}
	in = prepare("update", update)
	execSQL(`CREATE FUNCTION reject_relation_audit() RETURNS TRIGGER AS $$ BEGIN RAISE EXCEPTION 'forced final audit rollback';END $$ LANGUAGE plpgsql;CREATE TRIGGER reject_audit BEFORE INSERT ON audit_log FOR EACH ROW EXECUTE FUNCTION reject_relation_audit()`)
	before := preview("update", update)
	if out, status := request("update", "relation-update-retry", in, true); status != 500 {
		t.Fatal("audit failure accepted", status, out)
	}
	if after := preview("update", update); after["expected_context"] != before["expected_context"] {
		t.Fatal("failed audit changed row/product versions")
	}
	execSQL(`DROP TRIGGER reject_audit ON audit_log;DROP FUNCTION reject_relation_audit()`)
	changed, status := request("update", "relation-update-retry", in, true)
	if status != 200 || changed["relationship"].(map[string]any)["notes"] != nil || changed["relationship"].(map[string]any)["relation_type"] != "required" {
		t.Fatal("partial update/clear failed", status, changed)
	}
	oldVersion := first["updated_at"]
	if p := preview("update", map[string]any{"relation_id": id, "notes": "other", "expected_updated_at": oldVersion}); !contains(p, "expected_updated_at") {
		t.Fatal("stale relation accepted", p)
	}
	// A job rooted at an ancestor uses this relation through recursive expansion.
	execSQL(`INSERT INTO jobs(jobid,statusid) VALUES(1,1);INSERT INTO job_product_requirements VALUES(1,3,2)`)
	if p := preview("archive", existing); !contains(p, "active_jobs") {
		t.Fatal("ancestor job not blocked", p)
	}
	blockedSQL(fmt.Sprintf(`UPDATE product_dependencies SET lifecycle_status='archived' WHERE id=%d`, id))
	blockedSQL(`UPDATE products SET lifecycle_status='archived' WHERE productid=2`)
	execSQL(`UPDATE jobs SET statusid=2`)
	a := prepare("archive", existing)
	archived, status := request("archive", "relation-archive", a, true)
	if status != 200 || archived["operation_status"] != "archived" {
		t.Fatal("archive", status, archived)
	}
	relation := archived["relationship"].(map[string]any)
	if relation["default_quantity"] != 3.5 || relation["relation_type"] != "required" || relation["assignment_scope"] != "case" {
		t.Fatal("archive changed fields", relation)
	}
	if p := preview("update", map[string]any{"relation_id": id, "notes": "bad"}); !contains(p, "lifecycle_status") {
		t.Fatal("archived edit accepted", p)
	}
	blockedSQL(fmt.Sprintf(`UPDATE product_dependencies SET notes='bad' WHERE id=%d`, id))
	blockedSQL(fmt.Sprintf(`DELETE FROM product_dependencies WHERE id=%d`, id))
	blockedSQL(fmt.Sprintf(`UPDATE product_dependencies SET lifecycle_status='active',notes='hidden restore edit' WHERE id=%d`, id))
	execSQL(`UPDATE products SET lifecycle_status='archived' WHERE productid=2`)
	if p := preview("restore", existing); !contains(p, "active_products") {
		t.Fatal("inactive restore target accepted", p)
	}
	execSQL(`UPDATE products SET lifecycle_status='active',updated_at=clock_timestamp() WHERE productid=2`)
	r := prepare("restore", existing)
	restored, status := request("restore", "relation-restore", r, true)
	if status != 200 || restored["operation_status"] != "restored" {
		t.Fatal("restore", status, restored)
	}
	replay, status := request("archive", "relation-archive", a, true)
	if status != 200 || !reflect.DeepEqual(archived, replay) {
		t.Fatal("archive replay after restore changed")
	}
	if count("audit_log") != 4 || count("warehouse_product_mutation_receipts") != 4 {
		t.Fatal("rollback/replay duplicated audit or receipt")
	}
	// All three direct job input sources and missing statuses are conservative.
	for _, input := range []string{`DELETE FROM job_product_requirements;INSERT INTO job_positions VALUES(1,1)`, `DELETE FROM job_positions;INSERT INTO devices VALUES('R',1);INSERT INTO job_devices VALUES(1,'R')`} {
		execSQL(`UPDATE jobs SET statusid=NULL;` + input)
		if p := preview("archive", existing); !contains(p, "active_jobs") {
			t.Fatal("active unknown-status job source accepted", p)
		}
	}
}
