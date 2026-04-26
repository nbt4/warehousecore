package handlers

import (
	"crypto/rand"
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/gorilla/mux"

	"warehousecore/internal/repository"
)

type APIKey struct {
	ID         int        `json:"id"`
	Name       string     `json:"name"`
	IsActive   bool       `json:"is_active"`
	CreatedAt  time.Time  `json:"created_at"`
	LastUsedAt *time.Time `json:"last_used_at,omitempty"`
}

type CreateAPIKeyResponse struct {
	APIKey
	PlainText string `json:"api_key"`
}

// ListAPIKeys returns all API keys (without plaintext).
func ListAPIKeys(w http.ResponseWriter, r *http.Request) {
	db := repository.GetSQLDB()
	rows, err := db.Query(`SELECT id, name, is_active, created_at, last_used_at FROM api_keys ORDER BY created_at DESC`)
	if err != nil {
		log.Printf("[APIKEY] failed to list keys: %v", err)
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to list API keys"})
		return
	}
	defer rows.Close()

	var keys []APIKey
	for rows.Next() {
		var k APIKey
		if err := rows.Scan(&k.ID, &k.Name, &k.IsActive, &k.CreatedAt, &k.LastUsedAt); err != nil {
			continue
		}
		keys = append(keys, k)
	}

	respondJSON(w, http.StatusOK, map[string]interface{}{"keys": keys})
}

// CreateAPIKey creates a new key and returns the plaintext once.
func CreateAPIKey(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid JSON"})
		return
	}
	if body.Name == "" {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "name is required"})
		return
	}

	rawKey := generateAPIKey()
	hash := repository.HashAPIKey(rawKey)
	prefix := rawKey[:8]

	db := repository.GetSQLDB()
	var id int64
	err := db.QueryRow(`INSERT INTO api_keys (name, key_hash, key_prefix, is_active) VALUES ($1, $2, $3, TRUE) RETURNING id`, body.Name, hash, prefix).Scan(&id)
	if err != nil {
		log.Printf("[APIKEY] failed to create key: %v", err)
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to create API key"})
		return
	}
	respondJSON(w, http.StatusCreated, CreateAPIKeyResponse{
		APIKey: APIKey{
			ID:        int(id),
			Name:      body.Name,
			IsActive:  true,
			CreatedAt: time.Now(),
		},
		PlainText: rawKey,
	})
}

// UpdateAPIKeyStatus toggles activation.
func UpdateAPIKeyStatus(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id, err := strconv.Atoi(vars["id"])
	if err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid id"})
		return
	}

	var body struct {
		IsActive bool `json:"is_active"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid JSON"})
		return
	}

	db := repository.GetSQLDB()
	res, err := db.Exec(`UPDATE api_keys SET is_active = $1 WHERE id = $2`, body.IsActive, id)
	if err != nil {
		log.Printf("[APIKEY] failed to update key %d: %v", id, err)
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to update API key"})
		return
	}
	if rows, _ := res.RowsAffected(); rows == 0 {
		respondJSON(w, http.StatusNotFound, map[string]string{"error": "API key not found"})
		return
	}
	respondJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// DeleteAPIKey hard-deletes a key (optional cleanup).
func DeleteAPIKey(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id, err := strconv.Atoi(vars["id"])
	if err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid id"})
		return
	}
	db := repository.GetSQLDB()
	res, err := db.Exec(`DELETE FROM api_keys WHERE id = $1`, id)
	if err != nil {
		log.Printf("[APIKEY] failed to delete key %d: %v", id, err)
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to delete API key"})
		return
	}
	if rows, _ := res.RowsAffected(); rows == 0 {
		respondJSON(w, http.StatusNotFound, map[string]string{"error": "API key not found"})
		return
	}
	respondJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

func generateAPIKey() string {
	const alphabet = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	const length = 48
	b := make([]byte, length)
	rand.Read(b)
	for i := 0; i < length; i++ {
		b[i] = alphabet[int(b[i])%len(alphabet)]
	}
	return string(b)
}
