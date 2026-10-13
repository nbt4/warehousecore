package services

import (
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestDurableWebsiteRefreshCoalescesInflightChanges(t *testing.T) {
	dsn := os.Getenv("WAREHOUSE_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("disposable _test PostgreSQL required")
	}
	u, err := url.Parse(dsn)
	if err != nil || !strings.HasSuffix(u.Path, "_test") {
		t.Fatal("owned _test database required")
	}
	admin, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close()
	schema := "website_refresh_test"
	if _, err = admin.Exec(`DROP SCHEMA IF EXISTS ` + schema + ` CASCADE;CREATE SCHEMA ` + schema); err != nil {
		t.Fatal(err)
	}
	defer admin.Exec(`DROP SCHEMA ` + schema + ` CASCADE`)
	q := u.Query()
	q.Set("search_path", schema)
	u.RawQuery = q.Encode()
	db, err := sql.Open("postgres", u.String())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	_, err = db.Exec(`CREATE TABLE website_revalidation(path TEXT PRIMARY KEY,generation BIGINT DEFAULT 0,delivered_generation BIGINT DEFAULT 0,status TEXT,requested_at TIMESTAMPTZ DEFAULT now(),synced_at TIMESTAMPTZ,next_attempt TIMESTAMPTZ DEFAULT now(),attempts INT DEFAULT 0,last_error TEXT DEFAULT '')`)
	if err != nil {
		t.Fatal(err)
	}
	started := make(chan struct{})
	release := make(chan struct{})
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) == 1 {
			close(started)
			select {
			case <-release:
			case <-r.Context().Done():
				return
			}
		}
		w.WriteHeader(200)
	}))
	defer server.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	rv := &Revalidator{url: server.URL, secret: "test", client: server.Client()}
	rv.Start(ctx, db)
	start := time.Now()
	if _, err = rv.Queue("/products"); err != nil {
		t.Fatal(err)
	}
	if time.Since(start) > 500*time.Millisecond {
		t.Fatal("save waited for website")
	}
	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("worker did not start")
	}
	if _, err = rv.Queue("/products"); err != nil {
		t.Fatal(err)
	}
	close(release)
	deadline := time.Now().Add(5 * time.Second)
	for {
		state, err := rv.Status("/products")
		if err != nil {
			t.Fatal(err)
		}
		if state.Status == "synced" && state.DeliveredGeneration == 2 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("latest mutation lost: %+v", state)
		}
		time.Sleep(10 * time.Millisecond)
	}
	if calls.Load() != 2 {
		t.Fatalf("expected two generations, got %d", calls.Load())
	}
	disabled := &Revalidator{db: db}
	state, err := disabled.Status("/products")
	if err != nil || state.Status != "disabled" {
		t.Fatalf("unconfigured refresh reported success: %+v %v", state, err)
	}
	cancel()
}
