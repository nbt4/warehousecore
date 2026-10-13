package services

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"sync"
	"time"
)

// PublicationStatus distinguishes saving catalog data from refreshing the website.
type PublicationStatus struct {
	Path                string     `json:"path"`
	Status              string     `json:"status"`
	Generation          int64      `json:"generation"`
	DeliveredGeneration int64      `json:"delivered_generation"`
	LastError           string     `json:"last_error,omitempty"`
	RequestedAt         *time.Time `json:"requested_at,omitempty"`
	SyncedAt            *time.Time `json:"synced_at,omitempty"`
}

type Revalidator struct {
	url    string
	secret string
	db     *sql.DB
	once   sync.Once
	wake   chan struct{}
	client *http.Client
}

func NewRevalidatorFromEnv() *Revalidator {
	return &Revalidator{url: os.Getenv("TSWEB_REVALIDATE_URL"), secret: os.Getenv("TSWEB_REVALIDATE_SECRET"), client: &http.Client{Timeout: 10 * time.Second}}
}

func (r *Revalidator) Configured() bool { return r != nil && r.url != "" && r.secret != "" }

// Start resumes persisted work after a restart. Only one request runs at a time;
// mutations arriving during a request retain a newer generation for the next pass.
func (r *Revalidator) Start(ctx context.Context, db *sql.DB) {
	r.once.Do(func() {
		r.db = db
		r.wake = make(chan struct{}, 1)
		if r.Configured() {
			if _, err := db.ExecContext(ctx, `UPDATE website_revalidation SET status='pending',next_attempt=now() WHERE status='disabled' AND generation>delivered_generation`); err != nil {
				log.Printf("[REVALIDATE] Failed to resume queued website updates")
			}
		}
		go r.run(ctx)
	})
}

func (r *Revalidator) Queue(path string) (PublicationStatus, error) {
	status := PublicationStatus{Path: path, Status: "disabled"}
	if r == nil || r.db == nil {
		return status, nil
	}
	state := "disabled"
	if r.Configured() {
		state = "pending"
	}
	err := r.db.QueryRow(`INSERT INTO website_revalidation(path,generation,status,requested_at,next_attempt)
 VALUES($1,1,$2,now(),now()) ON CONFLICT(path) DO UPDATE SET generation=website_revalidation.generation+1,
 status=EXCLUDED.status,requested_at=now(),next_attempt=now(),attempts=0,last_error=''
 RETURNING generation,delivered_generation,status`, path, state).Scan(&status.Generation, &status.DeliveredGeneration, &status.Status)
	if err == nil && state == "pending" {
		select {
		case r.wake <- struct{}{}:
		default:
		}
	}
	return status, err
}

// Revalidate queues a durable update; remote latency never blocks an upload.
func (r *Revalidator) Revalidate(path string) {
	if _, err := r.Queue(path); err != nil {
		log.Printf("[REVALIDATE] Failed to queue website update")
	}
}

func (r *Revalidator) Status(path string) (PublicationStatus, error) {
	status := PublicationStatus{Path: path, Status: "disabled"}
	if r == nil || r.db == nil {
		return status, nil
	}
	if r.Configured() {
		status.Status = "idle"
	}
	err := r.db.QueryRow(`SELECT status,generation,delivered_generation,last_error,requested_at,synced_at FROM website_revalidation WHERE path=$1`, path).
		Scan(&status.Status, &status.Generation, &status.DeliveredGeneration, &status.LastError, &status.RequestedAt, &status.SyncedAt)
	if err == sql.ErrNoRows {
		return status, nil
	}
	if !r.Configured() {
		status.Status = "disabled"
		status.LastError = ""
	}
	return status, err
}

func (r *Revalidator) run(ctx context.Context) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		case <-r.wake:
		}
		if !r.Configured() {
			continue
		}
		for {
			var path string
			var generation int64
			var attempts int
			err := r.db.QueryRowContext(ctx, `SELECT path,generation,attempts FROM website_revalidation
    WHERE generation>delivered_generation AND status<>'disabled' AND next_attempt<=now() ORDER BY requested_at LIMIT 1`).Scan(&path, &generation, &attempts)
			if err != nil {
				break
			}
			_, err = r.db.ExecContext(ctx, `UPDATE website_revalidation SET status='processing' WHERE path=$1 AND generation=$2`, path, generation)
			if err != nil {
				break
			}
			err = r.deliver(ctx, path)
			if err == nil {
				_, err = r.db.ExecContext(ctx, `UPDATE website_revalidation SET delivered_generation=GREATEST(delivered_generation,$2),
     status=CASE WHEN generation=$2 THEN 'synced' ELSE 'pending' END,synced_at=now(),last_error='',attempts=0 WHERE path=$1`, path, generation)
			} else {
				delay := time.Duration(1<<min(attempts, 8)) * time.Second
				_, err = r.db.ExecContext(ctx, `UPDATE website_revalidation SET status=CASE WHEN generation=$2 THEN 'error' ELSE 'pending' END,
     last_error=CASE WHEN generation=$2 THEN 'Website konnte nicht aktualisiert werden' ELSE '' END,
     attempts=CASE WHEN generation=$2 THEN attempts+1 ELSE attempts END,
     next_attempt=CASE WHEN generation=$2 THEN now()+$3::interval ELSE next_attempt END WHERE path=$1`, path, generation, fmt.Sprintf("%d seconds", int(delay.Seconds())))
			}
			if err != nil {
				break
			}
		}
	}
}

func (r *Revalidator) deliver(ctx context.Context, path string) error {
	requestCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	body, _ := json.Marshal(map[string]string{"path": path})
	req, err := http.NewRequestWithContext(requestCtx, http.MethodPost, r.url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("invalid website endpoint")
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+r.secret)
	client := r.client
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	response, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("website update failed")
	}
	defer response.Body.Close()
	io.Copy(io.Discard, io.LimitReader(response.Body, 1<<20))
	if response.StatusCode >= 300 {
		return fmt.Errorf("website returned HTTP %d", response.StatusCode)
	}
	return nil
}

func RevalidateWithBase(baseURL, secret, path string) {
	if baseURL == "" || secret == "" {
		return
	}
	r := &Revalidator{url: baseURL, secret: secret}
	// Legacy callers do not share a database; bound their asynchronous request.
	go func() {
		if err := r.deliver(context.Background(), path); err != nil {
			log.Printf("[REVALIDATE] Website update failed")
		}
	}()
}
