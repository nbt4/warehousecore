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
	"time"

	"warehousecore/internal/middleware"
	"warehousecore/internal/models"
	"warehousecore/internal/repository"

	jwt "github.com/golang-jwt/jwt/v5"
	"github.com/gorilla/mux"
)

func TestWarehouseCaseTemplateRetentionAtomicContextAndReplay(t *testing.T) {
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
	exec(`DROP SCHEMA IF EXISTS warehouse_case_template_test CASCADE;CREATE SCHEMA warehouse_case_template_test;SET search_path TO warehouse_case_template_test`)
	defer db.Exec(`DROP SCHEMA warehouse_case_template_test CASCADE`)
	exec(warehouseCaseFixtureSQL)
	exec(`ALTER TABLE products ADD COLUMN name TEXT DEFAULT 'Test product';ALTER TABLE products ADD COLUMN tracking_mode TEXT DEFAULT 'individual';ALTER TABLE products ADD COLUMN updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP;
 ALTER TABLE devices ADD COLUMN productid INT;
 ALTER TABLE case_content_templates ADD COLUMN template_line_id BIGSERIAL PRIMARY KEY;ALTER TABLE case_content_templates ADD COLUMN created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP;ALTER TABLE case_content_templates ADD COLUMN updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP;ALTER TABLE case_content_templates ADD CONSTRAINT template_case_product UNIQUE(case_id,product_id);
 CREATE TABLE users(userid INT PRIMARY KEY,is_active BOOL,is_admin BOOL);INSERT INTO users VALUES(11,true,true);
 INSERT INTO cases(name) VALUES('Template case'),('Other case');
 INSERT INTO products(productid,lifecycle_status,tracking_mode) VALUES(3,'active','quantity'),(4,'active','none');`)
	old := repository.DB
	repository.DB = db
	defer func() { repository.DB = old }()
	if err = EnsureWarehouseCaseVersionSchema(); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if err = EnsureWarehouseCaseTemplateLifecycleSchema(); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("CORES_JWT_SECRET", strings.Repeat("k", 48))
	run := func(op, key, scope string, in map[string]any, status int) map[string]any {
		t.Helper()
		raw, _ := json.Marshal(in)
		r := httptest.NewRequest(http.MethodPost, "/templates", bytes.NewReader(raw))
		r = mux.SetURLVars(r, map[string]string{"operation": op})
		r = r.WithContext(context.WithValue(r.Context(), middleware.UserContextKey, &models.User{UserID: 11, IsAdmin: true, IsActive: true}))
		r.Header.Set("X-Cores-Origin", "MCP/AI")
		r.Header.Set("Idempotency-Key", key)
		token, e := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{"uid": 11, "mcp_scope": scope, "exp": time.Now().Add(time.Hour).Unix()}).SignedString([]byte(strings.Repeat("k", 48)))
		if e != nil {
			t.Fatal(e)
		}
		r.AddCookie(&http.Cookie{Name: "cores_token", Value: token})
		w := httptest.NewRecorder()
		CaseTemplateMCP(w, r)
		var out map[string]any
		if json.Unmarshal(w.Body.Bytes(), &out) != nil || w.Code != status {
			t.Fatalf("%s status %d want %d: %s", op, w.Code, status, w.Body.String())
		}
		return out
	}
	scope := func(op string) string {
		action := "update"
		if op == "create" {
			action = "create"
		}
		if op == "archive" || op == "restore" {
			action = "archive"
		}
		return "cores:warehouse:" + action
	}
	call := func(op, key string, in map[string]any, status int) map[string]any {
		t.Helper()
		return run(op, key, scope(op), in, status)
	}
	count := func(table string) int {
		t.Helper()
		var n int
		if err := db.QueryRow(`SELECT count(*) FROM ` + table).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	confirm := func(op string, in map[string]any) map[string]any {
		t.Helper()
		in["preview"] = true
		in["confirm_change"] = false
		delete(in, "expected_context")
		delete(in, "expected_updated_at")
		delete(in, "confirmation_text")
		out := call(op, "", in, 200)
		if out["ready_to_execute"] != true {
			t.Fatal(out)
		}
		in["expected_context"] = out["expected_context"]
		in["expected_updated_at"] = out["expected_updated_at"]
		in["confirmation_text"] = out["confirmation_text_required"]
		in["confirm_change"] = true
		in["preview"] = false
		return in
	}
	in := map[string]any{"case_id": 1, "product_id": 1, "expected_quantity": 2}
	preview := call("create", "", in, 200)
	if preview["ready_to_execute"] != true || count("case_content_templates") != 0 || count("audit_log") != 0 || count("warehouse_product_mutation_receipts") != 0 {
		t.Fatal(preview)
	}
	run("create", "", "cores:warehouse:update", in, 403)
	in = confirm("create", in)
	call("create", "", in, 428)
	phrase := in["confirmation_text"]
	in["confirmation_text"] = "CREATE"
	call("create", "template-wrong-phrase", in, 428)
	in["confirmation_text"] = phrase
	exec(`CREATE FUNCTION reject_template_audit() RETURNS TRIGGER LANGUAGE plpgsql AS $$ BEGIN IF NEW.entity_type='case_template' THEN RAISE EXCEPTION 'injected final audit failure';END IF;RETURN NEW;END $$;CREATE TRIGGER reject_template_audit BEFORE INSERT ON audit_log FOR EACH ROW EXECUTE FUNCTION reject_template_audit();`)
	call("create", "template-atomic-retry", in, 500)
	if count("case_content_templates") != 0 || count("audit_log") != 0 || count("warehouse_product_mutation_receipts") != 0 {
		t.Fatal("partial template or receipt persisted")
	}
	exec(`DROP TRIGGER reject_template_audit ON audit_log`)
	created := call("create", "template-atomic-retry", in, 200)
	lineID := created["template"].(map[string]any)["template_line_id"]
	if created["operation_status"] != "executed" || count("case_content_templates") != 1 || count("audit_log") != 1 {
		t.Fatal(created)
	}
	for i := 0; i < 2; i++ {
		if err = EnsureWarehouseCaseTemplateLifecycleSchema(); err != nil {
			t.Fatal(err)
		}
		replay := call("create", "template-atomic-retry", in, 200)
		if !reflect.DeepEqual(replay, created) || count("audit_log") != 1 {
			t.Fatal("retained replay changed", replay)
		}
	}
	exec(`UPDATE users SET is_admin=false WHERE userid=11`)
	call("create", "template-atomic-retry", in, 403)
	exec(`UPDATE users SET is_admin=true WHERE userid=11`)
	in["expected_quantity"] = 3
	call("create", "template-atomic-retry", in, 409)
	stale := confirm("update", map[string]any{"case_id": 1, "template_line_id": lineID, "expected_quantity": 4})
	exec(`UPDATE products SET updated_at=updated_at+INTERVAL '1 second' WHERE productid=1`)
	if out := call("update", "template-stale-product", stale, 200); out["ready_to_execute"] != false {
		t.Fatal("product version ignored")
	}
	stale = confirm("update", stale)
	exec(`UPDATE case_content_templates SET expected_quantity=3 WHERE case_id=1`)
	if out := call("update", "template-stale-native", stale, 200); out["ready_to_execute"] != false {
		t.Fatal("native line edit ignored")
	}
	edit := confirm("update", stale)
	call("update", "template-update-1", edit, 200)
	archive := confirm("archive", map[string]any{"case_id": 1, "template_line_id": lineID})
	archived := call("archive", "template-archive-1", archive, 200)
	if archived["template"].(map[string]any)["lifecycle_status"] != "archived" || count("case_content_templates") != 1 {
		t.Fatal(archived)
	}
	if out := call("create", "", map[string]any{"case_id": 1, "product_id": 1, "expected_quantity": 2}, 200); out["ready_to_execute"] != false {
		t.Fatal("retained identity recreated")
	}
	if _, err = db.Exec(`DELETE FROM case_content_templates WHERE case_id=1`); err == nil {
		t.Fatal("hard deletion accepted")
	}
	exec(`UPDATE products SET lifecycle_status='archived' WHERE productid=1`)
	if out := call("restore", "", map[string]any{"case_id": 1, "template_line_id": lineID}, 200); out["ready_to_execute"] != false {
		t.Fatal("inactive product restored")
	}
	exec(`UPDATE products SET lifecycle_status='active' WHERE productid=1`)
	restore := confirm("restore", map[string]any{"case_id": 1, "template_line_id": lineID})
	restored := call("restore", "template-restore-1", restore, 200)
	if restored["template"].(map[string]any)["template_line_id"] != lineID || restored["template"].(map[string]any)["expected_quantity"] != float64(4) {
		t.Fatal("restore lost original identity or quantity")
	}
	for _, q := range []float64{0, -1, 1.2345, 1000000000} {
		call("create", "", map[string]any{"case_id": 1, "product_id": 3, "expected_quantity": q}, 400)
	}
	for _, product := range []int{2, 4} {
		if out := call("create", "", map[string]any{"case_id": 1, "product_id": product, "expected_quantity": 1}, 200); out["ready_to_execute"] != false {
			t.Fatal(out)
		}
	}
	if out := call("update", "", map[string]any{"case_id": 1, "template_line_id": lineID, "expected_quantity": 1.5}, 200); out["ready_to_execute"] != false {
		t.Fatal("fractional serialized template accepted")
	}
	call("update", "", map[string]any{"case_id": 2, "template_line_id": lineID, "expected_quantity": 1}, 404)
	call("archive", "", map[string]any{"case_id": 1, "template_line_id": lineID, "expected_quantity": 1}, 400)
	call("update", "", map[string]any{"case_id": 1, "template_line_id": lineID, "arbitrary": "sql"}, 400)
	exec(`UPDATE cases SET workflow_status='sealed',sealed_at=CURRENT_TIMESTAMP WHERE caseid=1`)
	if _, err = db.Exec(`UPDATE case_content_templates SET expected_quantity=5 WHERE case_id=1`); err == nil {
		t.Fatal("sealed native edit accepted")
	}
	exec(`UPDATE cases SET workflow_status='packing',sealed_at=NULL WHERE caseid=1;INSERT INTO case_child_contents(parent_case_id,child_case_id) VALUES(2,1)`)
	if _, err = db.Exec(`UPDATE case_content_templates SET lifecycle_status='archived' WHERE case_id=1`); err == nil {
		t.Fatal("nested native edit accepted")
	}
	exec(`DELETE FROM case_child_contents WHERE child_case_id=1`)
	if _, err = db.Exec(`UPDATE case_content_templates SET product_id=3 WHERE case_id=1`); err == nil {
		t.Fatal("immutable product changed")
	}
	if count("devicescases") != 0 || count("case_product_contents") != 0 || count("product_locations") != 0 {
		t.Fatal("template changed physical stock")
	}
}

func TestWarehouseCaseTemplateLegacyCannotBypassDelegation(t *testing.T) {
	secret := strings.Repeat("k", 48)
	t.Setenv("CORES_JWT_SECRET", secret)
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{"uid": 11, "mcp_scope": "cores:warehouse:update", "exp": time.Now().Add(time.Hour).Unix()}).SignedString([]byte(secret))
	if err != nil {
		t.Fatal(err)
	}
	for _, handler := range []http.HandlerFunc{UpsertHandlingUnitTemplate, DeleteHandlingUnitTemplate} {
		for _, origin := range []bool{false, true} {
			r := httptest.NewRequest(http.MethodPost, "/template", strings.NewReader(`{"product_id":1,"expected_quantity":1}`))
			r.AddCookie(&http.Cookie{Name: "cores_token", Value: token})
			if origin {
				r.Header.Set("X-Cores-Origin", "MCP/AI")
			}
			w := httptest.NewRecorder()
			handler(w, r)
			if w.Code != 403 {
				t.Fatal("legacy bypass", origin, w.Code, w.Body.String())
			}
		}
	}
}
