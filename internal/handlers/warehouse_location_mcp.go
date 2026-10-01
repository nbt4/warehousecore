package handlers

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strings"

	"warehousecore/internal/repository"
)

// createWarehouseLocationMCP keeps the location, audit entry and replay receipt
// in one transaction. The public UI route retains its existing behavior.
func createWarehouseLocationMCP(w http.ResponseWriter, r *http.Request, input warehouseLocationInput) {
	if input.Code == "" || len(input.Code) > 50 || len([]rune(input.Name)) > 100 {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "Code (1-50) and name (1-100) are required"})
		return
	}
	if input.OperationalStatus != "available" {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "New MCP locations must start as available"})
		return
	}
	if !warehouseLocationTypeAllowed(input.Type) || !warehouseLocationKindAllowed(input.LocationKind) || !warehouseLocationRoleAllowed(input.ProcessRole) || input.CapacityMode != "item_count" {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid location type, kind, process role or capacity mode"})
		return
	}
	if input.ParentZoneID != nil && *input.ParentZoneID <= 0 {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "Parent zone ID must be positive"})
		return
	}
	if input.MaxWeightKg != nil && *input.MaxWeightKg <= 0 || input.MaxVolumeM3 != nil && *input.MaxVolumeM3 <= 0 {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "Weight and volume limits must be positive"})
		return
	}
	input.Barcode = normalizeWarehouseLocationBarcode(input.Barcode, input.Code)
	if len(*input.Barcode) > 255 {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "Barcode is too long"})
		return
	}
	tx, err := repository.GetSQLDB().BeginTx(r.Context(), nil)
	if err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to begin location creation"})
		return
	}
	defer tx.Rollback()
	receiptID, replay, err := beginWarehouseProductMutation(tx, r, "location.create", input)
	if err != nil {
		respondWarehouseMutationError(w, err)
		return
	}
	if replay != nil {
		respondJSON(w, http.StatusCreated, replay)
		return
	}
	if input.ParentZoneID != nil {
		var active bool
		var status string
		err := tx.QueryRow(`SELECT is_active,operational_status FROM storage_zones WHERE zone_id=$1 FOR SHARE`, *input.ParentZoneID).Scan(&active, &status)
		if err == sql.ErrNoRows {
			respondJSON(w, http.StatusNotFound, map[string]string{"error": "Parent location not found"})
			return
		}
		if err != nil {
			respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to load parent location"})
			return
		}
		if !active || status == "archived" {
			respondJSON(w, http.StatusConflict, map[string]string{"error": "Parent location is inactive"})
			return
		}
	}
	for _, lockKey := range []string{"warehouse.location.code." + input.Code, "warehouse.location.name." + strings.ToLower(input.Name)} {
		if _, err := tx.Exec(`SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, lockKey); err != nil {
			respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to lock location identity"})
			return
		}
	}
	var duplicateID int64
	err = tx.QueryRow(`SELECT zone_id FROM storage_zones WHERE lower(trim(code))=lower($1) OR lower(trim(barcode))=lower($2) OR (lower(trim(name))=lower($3) AND parent_zone_id IS NOT DISTINCT FROM $4) LIMIT 1`, input.Code, *input.Barcode, input.Name, input.ParentZoneID).Scan(&duplicateID)
	if err == nil {
		respondJSON(w, http.StatusConflict, map[string]any{"error": "Location code, barcode or name already exists", "zone_id": duplicateID})
		return
	}
	if err != sql.ErrNoRows {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to check location duplicates"})
		return
	}
	var id int64
	var version string
	err = tx.QueryRow(`INSERT INTO storage_zones
		(code,barcode,name,type,description,parent_zone_id,capacity,is_active,location_kind,process_role,operational_status,is_storable,pick_sequence,capacity_mode,max_weight_kg,max_volume_m3,inventory_frequency_days,next_count_at)
		VALUES($1,$2,$3,$4,$5,$6,$7,TRUE,$8,$9,$10,$11,$12,$13,$14,$15,$16,
		CASE WHEN $16::int > 0 THEN CURRENT_TIMESTAMP + ($16::text || ' days')::interval ELSE NULL END)
		RETURNING zone_id,to_char(updated_at,'YYYY-MM-DD"T"HH24:MI:SS.US"Z"')`, input.Code, *input.Barcode, input.Name, input.Type, nullableStringPtr(input.Description), input.ParentZoneID, input.Capacity,
		input.LocationKind, input.ProcessRole, input.OperationalStatus, input.IsStorable, input.PickSequence, input.CapacityMode,
		input.MaxWeightKg, input.MaxVolumeM3, input.InventoryFrequencyDays).Scan(&id, &version)
	if err != nil {
		respondJSON(w, http.StatusConflict, map[string]string{"error": "Location could not be created"})
		return
	}
	response := map[string]any{"zone_id": id, "code": input.Code, "barcode": *input.Barcode, "name": input.Name, "parent_zone_id": input.ParentZoneID, "updated_at": version}
	encoded, err := json.Marshal(input)
	if err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to encode location audit"})
		return
	}
	var after map[string]any
	if err := json.Unmarshal(encoded, &after); err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to encode location audit"})
		return
	}
	after["origin"], after["zone_id"], after["updated_at"], after["is_active"] = "MCP/AI", id, version, true
	if err := recordWarehouseMasterAudit(tx, r, "storage_zone", id, after); err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to audit location"})
		return
	}
	if err := completeWarehouseProductMutation(tx, receiptID, http.StatusCreated, response); err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to store location receipt"})
		return
	}
	if err := tx.Commit(); err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to commit location"})
		return
	}
	respondJSON(w, http.StatusCreated, response)
}

func warehouseLocationTypeAllowed(value string) bool {
	switch value {
	case "shelf", "rack", "case", "vehicle", "stage", "warehouse", "other":
		return true
	}
	return false
}

func warehouseLocationKindAllowed(value string) bool {
	switch value {
	case "site", "rack", "bin", "level", "vehicle", "area":
		return true
	}
	return false
}

func warehouseLocationRoleAllowed(value string) bool {
	switch value {
	case "storage", "receiving", "return", "inspection", "quarantine", "repair", "charging", "picking", "staging", "shipping", "transport", "unknown":
		return true
	}
	return false
}
