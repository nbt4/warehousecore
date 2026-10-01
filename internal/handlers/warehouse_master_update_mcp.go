package handlers

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"reflect"
	"strings"

	"warehousecore/internal/middleware"
	"warehousecore/internal/repository"
)

type warehouseManufacturerUpdateInput struct {
	Name              string  `json:"name"`
	Website           *string `json:"website"`
	ExpectedUpdatedAt string  `json:"expected_updated_at"`
}

type warehouseBrandUpdateInput struct {
	Name              string `json:"name"`
	ManufacturerID    *int   `json:"manufacturer_id"`
	ExpectedUpdatedAt string `json:"expected_updated_at"`
}

// All writers, including the legacy UI, acquire table locks on modification.
// These locks keep duplicate and brand/product checks valid through the commit.
func updateWarehouseMasterMCP(w http.ResponseWriter, r *http.Request, entity string, id int, name string, website *string, manufacturerID *int, expected string) {
	user, ok := middleware.GetUserFromContext(r)
	if !ok || user == nil || !user.IsAdmin {
		respondJSON(w, http.StatusForbidden, map[string]string{"error": "Warehouse administrator permission is required"})
		return
	}
	if strings.TrimSpace(expected) == "" {
		respondJSON(w, http.StatusPreconditionRequired, map[string]string{"error": "The exact expected_updated_at from the preview is required"})
		return
	}
	name = strings.TrimSpace(name)
	if id <= 0 || id > math.MaxInt32 || name == "" || len([]rune(name)) > 255 || manufacturerID != nil && (*manufacturerID <= 0 || *manufacturerID > math.MaxInt32) {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid master record ID, name or manufacturer_id"})
		return
	}
	var valid bool
	website, valid = normalizedManufacturerWebsite(website)
	if !valid {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "Website must be an HTTP(S) URL of at most 255 characters"})
		return
	}
	after := map[string]any{"name": name}
	if entity == "manufacturer" {
		after["website"] = website
	} else {
		after["manufacturer_id"] = manufacturerID
	}
	tx, err := repository.GetSQLDB().BeginTx(r.Context(), nil)
	if err != nil {
		respondWarehouseMutationError(w, err)
		return
	}
	defer tx.Rollback()
	if _, err = tx.Exec(`SET LOCAL lock_timeout='5s'; SET LOCAL statement_timeout='15s'`); err != nil {
		respondWarehouseMutationError(w, err)
		return
	}
	receiptID, replay, err := beginWarehouseProductMutation(tx, r, entity+".update", map[string]any{"id": id, "after": after, "expected_updated_at": expected})
	if err != nil {
		respondWarehouseMutationError(w, err)
		return
	}
	if replay != nil {
		respondJSON(w, http.StatusOK, replay)
		return
	}
	if _, err = tx.Exec(`LOCK TABLE manufacturer,brands IN SHARE ROW EXCLUSIVE MODE`); err != nil {
		respondWarehouseMutationError(w, err)
		return
	}
	var currentName, version, lifecycle string
	var currentWebsite sql.NullString
	var currentManufacturer sql.NullInt64
	if entity == "manufacturer" {
		err = tx.QueryRow(`SELECT name,website,lifecycle_status,to_char(updated_at,'YYYY-MM-DD"T"HH24:MI:SS.US"Z"') FROM manufacturer WHERE manufacturerid=$1 FOR UPDATE`, id).Scan(&currentName, &currentWebsite, &lifecycle, &version)
	} else {
		err = tx.QueryRow(`SELECT name,manufacturerid,lifecycle_status,to_char(updated_at,'YYYY-MM-DD"T"HH24:MI:SS.US"Z"') FROM brands WHERE brandid=$1 FOR UPDATE`, id).Scan(&currentName, &currentManufacturer, &lifecycle, &version)
	}
	if err == sql.ErrNoRows {
		respondJSON(w, http.StatusNotFound, map[string]string{"error": "Master record not found"})
		return
	}
	if err != nil {
		respondWarehouseMutationError(w, err)
		return
	}
	if lifecycle != "active" {
		respondJSON(w, 409, map[string]string{"error": "Restore archived master record before editing"})
		return
	}
	if expected != version {
		respondJSON(w, http.StatusConflict, map[string]string{"error": "Master record changed since the preview"})
		return
	}
	before := map[string]any{"name": currentName}
	if entity == "manufacturer" {
		var previous *string
		if currentWebsite.Valid {
			previous = &currentWebsite.String
		}
		before["website"] = previous
	} else {
		var previous *int
		if currentManufacturer.Valid {
			value := int(currentManufacturer.Int64)
			previous = &value
		}
		before["manufacturer_id"] = previous
		if manufacturerID != nil {
			var exists bool
			if err = tx.QueryRow(`SELECT EXISTS(SELECT 1 FROM manufacturer WHERE manufacturerid=$1 AND lifecycle_status='active')`, *manufacturerID).Scan(&exists); err != nil {
				respondWarehouseMutationError(w, err)
				return
			}
			if !exists {
				respondJSON(w, http.StatusNotFound, map[string]string{"error": "Manufacturer not found"})
				return
			}
		}
		if !reflect.DeepEqual(previous, manufacturerID) {
			if _, err = tx.Exec(`LOCK TABLE products IN SHARE MODE`); err != nil {
				respondWarehouseMutationError(w, err)
				return
			}
			var conflict bool
			if err = tx.QueryRow(`SELECT EXISTS(SELECT 1 FROM products WHERE brandid=$1 AND manufacturerid IS DISTINCT FROM $2::int)`, id, manufacturerID).Scan(&conflict); err != nil {
				respondWarehouseMutationError(w, err)
				return
			}
			if conflict {
				respondJSON(w, http.StatusConflict, map[string]string{"error": "Linked products require their current manufacturer; update product associations before moving this brand"})
				return
			}
		}
	}
	if reflect.DeepEqual(before, after) {
		respondJSON(w, http.StatusConflict, map[string]string{"error": "No master fields changed"})
		return
	}
	var duplicate bool
	if entity == "manufacturer" {
		err = tx.QueryRow(`SELECT EXISTS(SELECT 1 FROM manufacturer WHERE manufacturerid<>$1 AND lower(trim(name))=lower($2))`, id, name).Scan(&duplicate)
	} else {
		err = tx.QueryRow(`SELECT EXISTS(SELECT 1 FROM brands WHERE brandid<>$1 AND lower(trim(name))=lower($2) AND manufacturerid IS NOT DISTINCT FROM $3::int)`, id, name, manufacturerID).Scan(&duplicate)
	}
	if err != nil {
		respondWarehouseMutationError(w, err)
		return
	}
	if duplicate {
		respondJSON(w, http.StatusConflict, map[string]string{"error": "A master record with this name already exists"})
		return
	}
	if entity == "manufacturer" {
		err = tx.QueryRow(`UPDATE manufacturer SET name=$1,website=$2 WHERE manufacturerid=$3 RETURNING to_char(updated_at,'YYYY-MM-DD"T"HH24:MI:SS.US"Z"')`, name, website, id).Scan(&version)
	} else {
		err = tx.QueryRow(`UPDATE brands SET name=$1,manufacturerid=$2 WHERE brandid=$3 RETURNING to_char(updated_at,'YYYY-MM-DD"T"HH24:MI:SS.US"Z"')`, name, manufacturerID, id).Scan(&version)
	}
	if err != nil {
		respondWarehouseMutationError(w, err)
		return
	}
	oldJSON, err := json.Marshal(before)
	if err != nil {
		respondWarehouseMutationError(w, err)
		return
	}
	newJSON, err := json.Marshal(map[string]any{"origin": "MCP/AI", "after": after, "updated_at": version})
	if err != nil {
		respondWarehouseMutationError(w, err)
		return
	}
	if _, err = tx.Exec(`INSERT INTO audit_log(user_id,action,entity_type,entity_id,old_values,new_values,user_agent) VALUES($1,$2,$3,$4,$5::jsonb,$6::jsonb,$7)`, user.UserID, entity+".update", entity, fmt.Sprint(id), string(oldJSON), string(newJSON), r.UserAgent()); err != nil {
		respondWarehouseMutationError(w, err)
		return
	}
	response := map[string]any{"name": name, "updated_at": version}
	for key, value := range after {
		response[key] = value
	}
	response[entity+"_id"] = id
	if err = completeWarehouseProductMutation(tx, receiptID, http.StatusOK, response); err != nil {
		respondWarehouseMutationError(w, err)
		return
	}
	if err = tx.Commit(); err != nil {
		respondWarehouseMutationError(w, err)
		return
	}
	respondJSON(w, http.StatusOK, response)
}
