package storage

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
)

func TestUploadRecreatesDeletedCachedCollectionAndRewinds(t *testing.T) {
	var mu sync.Mutex
	mkcol, puts := 0, 0
	deleted := false
	payloads := []string{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		if r.Method == "MKCOL" {
			mkcol++
			w.WriteHeader(201)
			return
		}
		if r.Method == "PUT" {
			puts++
			data, _ := io.ReadAll(r.Body)
			payloads = append(payloads, string(data))
			if deleted {
				deleted = false
				w.WriteHeader(404)
				return
			}
			w.WriteHeader(201)
			return
		}
		w.WriteHeader(405)
	}))
	defer server.Close()
	client, err := NewNextcloudClient(server.URL, "test", "test", "base")
	if err != nil {
		t.Fatal(err)
	}
	if err = client.Upload("folder/a.jpg", bytes.NewReader([]byte("first")), "image/jpeg"); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	deleted = true
	mu.Unlock()
	if err = client.Upload("folder/b.jpg", bytes.NewReader([]byte("second")), "image/jpeg"); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	defer mu.Unlock()
	if mkcol != 2 || puts != 3 || payloads[1] != "second" || payloads[2] != "second" {
		t.Fatalf("retry failed: MKCOL %d PUT %d payloads %v", mkcol, puts, payloads)
	}
}
