package handlers

import (
	"database/sql"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/mux"
	"warehousecore/internal/repository"
)

func TestUpdateWarehouseTaskStatusPostgres(t *testing.T) {
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
	if _, err := db.Exec(`CREATE SCHEMA warehouse_task_status_test; SET search_path TO warehouse_task_status_test;
CREATE TABLE warehouse_tasks (
 task_id BIGSERIAL PRIMARY KEY,
 status VARCHAR(20) NOT NULL DEFAULT 'open' CHECK (status IN ('open','in_progress','done','cancelled')),
 updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
 completed_at TIMESTAMP
);
INSERT INTO warehouse_tasks (updated_at) VALUES ('2000-01-01');`); err != nil {
		t.Fatal(err)
	}
	defer db.Exec("DROP SCHEMA warehouse_task_status_test CASCADE")
	oldDB := repository.DB
	repository.DB = db
	defer func() { repository.DB = oldDB }()

	request := func(t *testing.T, id, body string, want int) {
		t.Helper()
		r := httptest.NewRequest(http.MethodPatch, "/warehouse/tasks/"+id+"/status", strings.NewReader(body))
		r = mux.SetURLVars(r, map[string]string{"id": id})
		w := httptest.NewRecorder()
		UpdateWarehouseTaskStatus(w, r)
		if w.Code != want {
			t.Fatalf("status %d want %d: %s", w.Code, want, w.Body.String())
		}
	}
	var previousUpdated time.Time
	if err := db.QueryRow("SELECT updated_at FROM warehouse_tasks WHERE task_id=1").Scan(&previousUpdated); err != nil {
		t.Fatal(err)
	}
	// VARCHAR matches migration 039; TEXT would hide parameter type conflicts.
	for _, status := range []string{"in_progress", "done", "cancelled", "open"} {
		t.Run(status, func(t *testing.T) {
			request(t, "1", `{"status":"`+status+`"}`, http.StatusOK)
			var stored string
			var updated time.Time
			var completed sql.NullTime
			if err := db.QueryRow("SELECT status,updated_at,completed_at FROM warehouse_tasks WHERE task_id=1").Scan(&stored, &updated, &completed); err != nil {
				t.Fatal(err)
			}
			if stored != status || !updated.After(previousUpdated) {
				t.Fatalf("status/timestamp not updated: %s %s", stored, updated)
			}
			if completed.Valid != (status == "done") {
				t.Fatalf("unexpected completed_at for %s: %v", status, completed)
			}
			previousUpdated = updated
		})
	}
	request(t, "999", `{"status":"in_progress"}`, http.StatusNotFound)
	request(t, "invalid", `{"status":"in_progress"}`, http.StatusBadRequest)
	request(t, "1", `{`, http.StatusBadRequest)
	request(t, "1", `{"status":"invalid"}`, http.StatusBadRequest)
	var stored string
	var updated time.Time
	if err := db.QueryRow("SELECT status,updated_at FROM warehouse_tasks WHERE task_id=1").Scan(&stored, &updated); err != nil {
		t.Fatal(err)
	}
	if stored != "open" || !updated.Equal(previousUpdated) {
		t.Fatal("rejected requests changed the task")
	}
}
