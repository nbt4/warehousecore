package handlers

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"strings"

	"warehousecore/internal/middleware"
	"warehousecore/internal/repository"
)

const warehousePackageLifecycleDependenciesSQL = `SELECT
 (SELECT count(*) FROM job_packages jp LEFT JOIN jobs j ON j.jobid=jp.job_id LEFT JOIN status s ON s.statusid=j.statusid WHERE jp.package_id=$1 AND j.deleted_at IS NULL AND NOT warehouse_job_status_is_closed(COALESCE(s.status,''))) AS active_jobs,
 (SELECT count(*) FROM job_package_reservations r JOIN job_packages jp ON jp.job_package_id=r.job_package_id WHERE jp.package_id=$1 AND r.reservation_status<>'released') AS open_reservations,
 (SELECT count(*) FROM job_packages WHERE package_id=$1) AS historic_job_uses`

func ArchiveProductPackageMCP(w http.ResponseWriter, r *http.Request) {
	mutateWarehousePackageLifecycleMCP(w, r, "archive")
}
func RestoreProductPackageMCP(w http.ResponseWriter, r *http.Request) {
	mutateWarehousePackageLifecycleMCP(w, r, "restore")
}
func mutateWarehousePackageLifecycleMCP(w http.ResponseWriter, r *http.Request, operation string) {
	if !isWarehouseMCPMutation(r) {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "Use the guided MCP package lifecycle workflow"})
		return
	}
	user, ok := middleware.GetUserFromContext(r)
	if !ok || user == nil || !user.IsAdmin {
		respondJSON(w, http.StatusForbidden, map[string]string{"error": "Warehouse administrator permission is required"})
		return
	}
	id, ok := packageIDFromRequest(w, r, "id")
	if !ok {
		return
	}
	if id > math.MaxInt32 {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid package ID"})
		return
	}
	var in struct {
		ExpectedUpdatedAt string `json:"expected_updated_at"`
		ConfirmLifecycle  bool   `json:"confirm_lifecycle"`
		ConfirmationText  string `json:"confirmation_text"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid package lifecycle body"})
		return
	}
	phrase := fmt.Sprintf("%s WAREHOUSE PACKAGE %d", strings.ToUpper(operation), id)
	if in.ExpectedUpdatedAt == "" || !in.ConfirmLifecycle || in.ConfirmationText != phrase {
		respondJSON(w, http.StatusPreconditionRequired, map[string]string{"error": "Exact version, explicit lifecycle confirmation and package-bound phrase are required"})
		return
	}
	tx, err := repository.GetSQLDB().BeginTx(r.Context(), nil)
	if err != nil {
		respondWarehouseMutationError(w, err)
		return
	}
	defer tx.Rollback()
	if _, err = tx.Exec(`SET LOCAL lock_timeout='5s';SET LOCAL statement_timeout='15s'`); err != nil {
		respondWarehouseMutationError(w, err)
		return
	}
	receipt, replay, err := beginWarehouseProductMutation(tx, r, "package."+operation, map[string]any{"id": id, "expected_updated_at": in.ExpectedUpdatedAt, "confirmation_text": phrase})
	if err != nil {
		respondWarehouseMutationError(w, err)
		return
	}
	if replay != nil {
		respondJSON(w, http.StatusOK, replay)
		return
	}
	if _, err = tx.Exec(`LOCK TABLE product_packages,product_package_items IN SHARE ROW EXCLUSIVE MODE; LOCK TABLE products,job_packages,jobs,status,job_package_reservations IN SHARE MODE`); err != nil {
		respondWarehouseMutationError(w, err)
		return
	}
	fields, version, code, active, err := loadWarehousePackage(tx, int64(id))
	if err == sql.ErrNoRows {
		respondJSON(w, http.StatusNotFound, map[string]string{"error": "Package not found"})
		return
	}
	if err != nil {
		respondWarehouseMutationError(w, err)
		return
	}
	if version != in.ExpectedUpdatedAt {
		respondJSON(w, http.StatusConflict, map[string]string{"error": "Package changed since preview"})
		return
	}
	target := operation == "restore"
	if active == target {
		respondJSON(w, http.StatusConflict, map[string]string{"error": "Current package lifecycle does not permit this action"})
		return
	}
	var jobs, reservations, history int64
	if err = tx.QueryRow(warehousePackageLifecycleDependenciesSQL, id).Scan(&jobs, &reservations, &history); err != nil {
		respondWarehouseMutationError(w, err)
		return
	}
	if jobs > 0 || reservations > 0 {
		respondJSON(w, http.StatusConflict, map[string]any{"error": "Active jobs or reservations block package lifecycle changes", "active_jobs": jobs, "open_reservations": reservations})
		return
	}
	if target {
		// Validate the stored contents without replacing any metadata or item rows.
		validation := fields
		validation.Aliases = append([]string(nil), fields.Aliases...)
		validation.Items = append([]warehousePackageItem(nil), fields.Items...)
		if err = normalizeWarehousePackage(&validation); err != nil {
			respondJSON(w, http.StatusConflict, map[string]string{"error": "Stored package fields or contents must be valid before restoration"})
			return
		}
		var duplicate bool
		if err = tx.QueryRow(`SELECT EXISTS(SELECT 1 FROM product_packages WHERE id<>$1 AND lower(trim(name))=lower($2))`, id, validation.Name).Scan(&duplicate); err != nil {
			respondWarehouseMutationError(w, err)
			return
		}
		if duplicate {
			respondJSON(w, http.StatusConflict, map[string]string{"error": "Package name conflicts with another record"})
			return
		}
		for _, item := range validation.Items {
			var productActive bool
			err = tx.QueryRow(`SELECT COALESCE(lifecycle_status,'active')='active' FROM products WHERE productid=$1`, item.ProductID).Scan(&productActive)
			if err == sql.ErrNoRows {
				respondJSON(w, http.StatusConflict, map[string]string{"error": "A package product no longer exists"})
				return
			}
			if err != nil {
				respondWarehouseMutationError(w, err)
				return
			}
			if !productActive {
				respondJSON(w, http.StatusConflict, map[string]string{"error": "All package products must be active before restoration"})
				return
			}
		}
	}
	before := map[string]any{"is_active": active, "website_visible": fields.WebsiteVisible, "name": fields.Name}
	if _, err = tx.Exec(`UPDATE product_packages SET is_active=$1,website_visible=false WHERE id=$2`, target, id); err != nil {
		respondWarehouseMutationError(w, err)
		return
	}
	fields, version, code, active, err = loadWarehousePackage(tx, int64(id))
	if err != nil {
		respondWarehouseMutationError(w, err)
		return
	}
	oldJSON, _ := json.Marshal(before)
	newJSON, _ := json.Marshal(map[string]any{"origin": "MCP/AI", "after": fields, "is_active": active, "updated_at": version})
	var auditID int64
	err = tx.QueryRow(`INSERT INTO audit_log(user_id,action,entity_type,entity_id,old_values,new_values,user_agent) VALUES($1,$2,'package',$3,$4::jsonb,$5::jsonb,$6) RETURNING id`, user.UserID, "package."+operation, fmt.Sprint(id), string(oldJSON), string(newJSON), r.UserAgent()).Scan(&auditID)
	if err != nil {
		respondWarehouseMutationError(w, err)
		return
	}
	response := map[string]any{"package_id": id, "package_code": code, "updated_at": version, "is_active": active, "package": fields, "audit_id": auditID}
	if err = completeWarehouseProductMutation(tx, receipt, http.StatusOK, response); err != nil {
		respondWarehouseMutationError(w, err)
		return
	}
	if err = tx.Commit(); err != nil {
		respondWarehouseMutationError(w, err)
		return
	}
	websiteRevalidator.Revalidate("/products")
	respondJSON(w, http.StatusOK, response)
}
