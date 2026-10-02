package handlers

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strings"
)

type productLifecycleRequest struct {
	ExpectedUpdatedAt string `json:"expectedUpdatedAt"`
}

func beginProductLifecycleMutation(tx *sql.Tx, r *http.Request, operation string, productID int, version string) (int64, json.RawMessage, error) {
	if !isWarehouseMCPMutation(r) {
		return 0, nil, nil
	}
	var input productLifecycleRequest
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		return 0, nil, &warehouseMutationError{http.StatusBadRequest, "invalid_payload", "A lifecycle request body is required"}
	}
	if strings.TrimSpace(input.ExpectedUpdatedAt) == "" {
		return 0, nil, &warehouseMutationError{http.StatusPreconditionRequired, "version_required", "The exact expectedUpdatedAt from the preview is required"}
	}
	receiptID, replay, err := beginWarehouseProductMutation(tx, r, operation, map[string]any{"product_id": productID, "expected_updated_at": input.ExpectedUpdatedAt})
	if err != nil {
		return 0, nil, err
	}
	if replay != nil {
		return 0, replay, nil
	}
	if input.ExpectedUpdatedAt != version {
		return 0, nil, &warehouseMutationError{http.StatusConflict, "stale_product", "The product changed since the preview"}
	}
	return receiptID, nil, nil
}

func productArchiveDependencies(tx *sql.Tx, productID int) (int, int, error) {
	var openRequirements, packedDevices int
	err := tx.QueryRow(`SELECT COUNT(*) FROM job_product_requirements r
		JOIN jobs j ON j.jobid=r.job_id JOIN status s ON s.statusid=j.statusid
		WHERE r.product_id=$1 AND j.deleted_at IS NULL AND COALESCE(to_jsonb(r)->>'deleted_at','')=''
		AND lower(trim(s.status)) NOT IN ('abgeschlossen','storniert','completed','paid','canceled','cancelled','abgerechnet')`, productID).Scan(&openRequirements)
	if err != nil {
		return 0, 0, err
	}
	err = tx.QueryRow(`SELECT COUNT(*) FROM job_devices jd JOIN devices d ON d.deviceid=jd.deviceid
		WHERE d.productid=$1 AND jd.pack_status IN ('packed','issued')`, productID).Scan(&packedDevices)
	return openRequirements, packedDevices, err
}
