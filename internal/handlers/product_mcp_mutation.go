package handlers

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strings"

	"warehousecore/internal/middleware"
)

var warehouseMutationKeyPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:-]{7,127}$`)

type warehouseMutationError struct {
	status int
	code   string
	text   string
}

func (e *warehouseMutationError) Error() string { return e.text }

func isWarehouseMCPMutation(r *http.Request) bool {
	return strings.EqualFold(strings.TrimSpace(r.Header.Get("X-Cores-Origin")), "MCP/AI")
}

func beginWarehouseProductMutation(tx *sql.Tx, r *http.Request, productID int, payload any) (int64, json.RawMessage, error) {
	if !isWarehouseMCPMutation(r) {
		return 0, nil, nil
	}
	user, ok := middleware.GetUserFromContext(r)
	if !ok || user == nil {
		return 0, nil, &warehouseMutationError{http.StatusUnauthorized, "user_required", "A signed-in suite user is required"}
	}
	key := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	if !warehouseMutationKeyPattern.MatchString(key) {
		return 0, nil, &warehouseMutationError{http.StatusPreconditionRequired, "idempotency_key_required", "A valid Idempotency-Key is required"}
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return 0, nil, err
	}
	keyDigest := sha256.Sum256([]byte(key))
	requestDigest := sha256.Sum256(encoded)
	operation := fmt.Sprintf("product_update:%d", productID)
	keyHash := hex.EncodeToString(keyDigest[:])
	requestHash := hex.EncodeToString(requestDigest[:])
	var id int64
	err = tx.QueryRow(`INSERT INTO warehouse_product_mutation_receipts (user_id,operation,key_hash,request_hash)
		VALUES ($1,$2,$3,$4) ON CONFLICT DO NOTHING RETURNING id`, user.UserID, operation, keyHash, requestHash).Scan(&id)
	if err == nil {
		return id, nil, nil
	}
	if err != sql.ErrNoRows {
		return 0, nil, err
	}
	var previousHash string
	var response json.RawMessage
	if err := tx.QueryRow(`SELECT request_hash,response FROM warehouse_product_mutation_receipts
		WHERE user_id=$1 AND operation=$2 AND key_hash=$3`, user.UserID, operation, keyHash).Scan(&previousHash, &response); err != nil {
		return 0, nil, err
	}
	if previousHash != requestHash {
		return 0, nil, &warehouseMutationError{http.StatusConflict, "idempotency_payload_conflict", "The key was already used with different product data"}
	}
	return 0, response, nil
}

func completeWarehouseProductMutation(tx *sql.Tx, receiptID int64, response any) error {
	if receiptID == 0 {
		return nil
	}
	encoded, err := json.Marshal(response)
	if err != nil {
		return err
	}
	_, err = tx.Exec(`UPDATE warehouse_product_mutation_receipts SET response=$1::jsonb,status_code=200 WHERE id=$2`, string(encoded), receiptID)
	return err
}

func respondWarehouseMutationError(w http.ResponseWriter, err error) {
	if flow, ok := err.(*warehouseMutationError); ok {
		respondJSON(w, flow.status, map[string]string{"error": flow.text, "code": flow.code})
		return
	}
	respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to update product"})
}
