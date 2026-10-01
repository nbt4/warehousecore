package handlers

import (
	"database/sql"
	"encoding/json"
	"math"
	"net/http"
	"reflect"
	"strings"

	"warehousecore/internal/middleware"
	"warehousecore/internal/repository"
)

// updateWarehouseLocationMCP commits the versioned edit, audit and replay receipt
// together. Table locks also serialize hierarchy changes from legacy UI writers.
func updateWarehouseLocationMCP(w http.ResponseWriter, r *http.Request, id int64, input warehouseLocationInput) {
	user, ok := middleware.GetUserFromContext(r)
	if !ok || user == nil || !user.IsAdmin {
		respondJSON(w, http.StatusForbidden, map[string]string{"error": "Warehouse administrator permission is required"})
		return
	}
	if strings.TrimSpace(input.ExpectedUpdatedAt) == "" {
		respondJSON(w, http.StatusPreconditionRequired, map[string]string{"error": "The exact expected_updated_at from the preview is required"})
		return
	}
	if err := validateWarehouseLocationInput(&input); err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	input.Barcode = normalizeWarehouseLocationBarcode(input.Barcode, input.Code)
	invalidIdentity := input.Code == "" || len(input.Code) > 50 || len([]rune(input.Name)) > 100 || len(*input.Barcode) > 255
	invalidKind := !warehouseLocationTypeAllowed(input.Type) || !warehouseLocationKindAllowed(input.LocationKind) || !warehouseLocationRoleAllowed(input.ProcessRole) || input.CapacityMode != "item_count"
	invalidParent := input.ParentZoneID != nil && (*input.ParentZoneID <= 0 || *input.ParentZoneID > math.MaxInt32)
	invalidCapacity := input.Capacity != nil && (math.Trunc(*input.Capacity) != *input.Capacity || *input.Capacity > math.MaxInt32)
	invalidWeight := input.MaxWeightKg != nil && (*input.MaxWeightKg <= 0 || *input.MaxWeightKg > 999999999.999)
	invalidVolume := input.MaxVolumeM3 != nil && (*input.MaxVolumeM3 <= 0 || *input.MaxVolumeM3 > 999999.999999)
	invalidSequence := input.PickSequence != nil && (*input.PickSequence < math.MinInt32 || *input.PickSequence > math.MaxInt32)
	invalidFrequency := input.InventoryFrequencyDays != nil && *input.InventoryFrequencyDays > math.MaxInt32
	if invalidIdentity || invalidKind || invalidParent || invalidCapacity || invalidWeight || invalidVolume || invalidSequence || invalidFrequency {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid location fields or limits"})
		return
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
	receiptID, replay, err := beginWarehouseProductMutation(tx, r, "location.update", struct {
		ID    int64                  `json:"zone_id"`
		Input warehouseLocationInput `json:"input"`
	}{id, input})
	if err != nil {
		respondWarehouseMutationError(w, err)
		return
	}
	if replay != nil {
		respondJSON(w, http.StatusOK, replay)
		return
	}
	if _, err = tx.Exec(`LOCK TABLE storage_zones IN SHARE ROW EXCLUSIVE MODE`); err != nil {
		respondWarehouseMutationError(w, err)
		return
	}
	var raw []byte
	var version string
	var active bool
	err = tx.QueryRow(`SELECT row_to_json(z),to_char(updated_at,'YYYY-MM-DD"T"HH24:MI:SS.US"Z"'),is_active FROM storage_zones z WHERE zone_id=$1 FOR UPDATE`, id).Scan(&raw, &version, &active)
	if err == sql.ErrNoRows {
		respondJSON(w, http.StatusNotFound, map[string]string{"error": "Location not found"})
		return
	}
	if err != nil {
		respondWarehouseMutationError(w, err)
		return
	}
	if input.ExpectedUpdatedAt != version {
		respondJSON(w, http.StatusConflict, map[string]string{"error": "Location changed since the preview"})
		return
	}
	var before warehouseLocationInput
	if err = json.Unmarshal(raw, &before); err != nil {
		respondWarehouseMutationError(w, err)
		return
	}
	if !active || before.OperationalStatus == "archived" || input.OperationalStatus != before.OperationalStatus {
		respondJSON(w, http.StatusConflict, map[string]string{"error": "Updates require an active location and must preserve its operational status"})
		return
	}
	before.ExpectedUpdatedAt = input.ExpectedUpdatedAt
	if before.Description != nil && strings.TrimSpace(*before.Description) == "" {
		before.Description = nil
	}
	if input.Description != nil {
		value := strings.TrimSpace(*input.Description)
		input.Description = &value
		if value == "" {
			input.Description = nil
		}
	}
	if reflect.DeepEqual(before, input) {
		respondJSON(w, http.StatusConflict, map[string]string{"error": "No location fields changed"})
		return
	}
	if input.ParentZoneID != nil {
		var invalid bool
		err = tx.QueryRow(`WITH RECURSIVE ancestors AS (
   SELECT zone_id,parent_zone_id,is_active,operational_status FROM storage_zones WHERE zone_id=$1
   UNION SELECT z.zone_id,z.parent_zone_id,z.is_active,z.operational_status FROM storage_zones z JOIN ancestors a ON z.zone_id=a.parent_zone_id
  ) SELECT NOT EXISTS(SELECT 1 FROM ancestors) OR EXISTS(SELECT 1 FROM ancestors WHERE zone_id=$2 OR NOT is_active OR operational_status='archived')`, *input.ParentZoneID, id).Scan(&invalid)
		if err != nil {
			respondWarehouseMutationError(w, err)
			return
		}
		if invalid {
			respondJSON(w, http.StatusConflict, map[string]string{"error": "Parent is missing, inactive or would create a hierarchy cycle"})
			return
		}
	}
	if !reflect.DeepEqual(before.Capacity, input.Capacity) || before.IsStorable != input.IsStorable {
		// Prevent inventory writers from changing the occupancy while applying limits.
		if _, err = tx.Exec(`LOCK TABLE devices,cases,product_locations IN SHARE MODE`); err != nil {
			respondWarehouseMutationError(w, err)
			return
		}
		var used float64
		err = tx.QueryRow(`SELECT (SELECT COUNT(*) FROM devices WHERE zone_id=$1 AND status='in_storage' AND lifecycle_status='active') + (SELECT COUNT(*) FROM cases WHERE zone_id=$1) + COALESCE((SELECT SUM(quantity) FROM product_locations WHERE zone_id=$1),0)`, id).Scan(&used)
		if err != nil {
			respondWarehouseMutationError(w, err)
			return
		}
		if !input.IsStorable && used > 0 || input.Capacity != nil && *input.Capacity < used {
			respondJSON(w, http.StatusConflict, map[string]any{"error": "Location limits conflict with current inventory", "occupancy": used})
			return
		}
	}
	var duplicate bool
	err = tx.QueryRow(`SELECT EXISTS(SELECT 1 FROM storage_zones WHERE zone_id<>$1 AND (lower(trim(code))=lower($2) OR lower(trim(barcode))=lower($3) OR (lower(trim(name))=lower($4) AND parent_zone_id IS NOT DISTINCT FROM $5)))`, id, input.Code, *input.Barcode, input.Name, input.ParentZoneID).Scan(&duplicate)
	if err != nil {
		respondWarehouseMutationError(w, err)
		return
	}
	if duplicate {
		respondJSON(w, http.StatusConflict, map[string]string{"error": "Location code, barcode or name already exists"})
		return
	}
	err = tx.QueryRow(`UPDATE storage_zones SET code=$1,barcode=$2,name=$3,type=$4,description=$5,parent_zone_id=$6,capacity=$7,
 location_kind=$8,process_role=$9,is_storable=$10,pick_sequence=$11,capacity_mode=$12,max_weight_kg=$13,max_volume_m3=$14,inventory_frequency_days=$15,
 next_count_at=CASE WHEN inventory_frequency_days IS NOT DISTINCT FROM $15::int THEN next_count_at WHEN $15::int>0 THEN COALESCE(last_counted_at,CURRENT_TIMESTAMP)+($15::text||' days')::interval ELSE NULL END
 WHERE zone_id=$16 RETURNING to_char(updated_at,'YYYY-MM-DD"T"HH24:MI:SS.US"Z"')`, input.Code, *input.Barcode, input.Name, input.Type, input.Description, input.ParentZoneID, input.Capacity, input.LocationKind, input.ProcessRole, input.IsStorable, input.PickSequence, input.CapacityMode, input.MaxWeightKg, input.MaxVolumeM3, input.InventoryFrequencyDays, id).Scan(&version)
	if err != nil {
		respondWarehouseMutationError(w, err)
		return
	}
	before.ExpectedUpdatedAt = ""
	input.ExpectedUpdatedAt = ""
	oldJSON, err := json.Marshal(before)
	if err != nil {
		respondWarehouseMutationError(w, err)
		return
	}
	newJSON, err := json.Marshal(map[string]any{"origin": "MCP/AI", "after": input, "updated_at": version, "is_active": true})
	if err != nil {
		respondWarehouseMutationError(w, err)
		return
	}
	if _, err = tx.Exec(`INSERT INTO audit_log(user_id,action,entity_type,entity_id,old_values,new_values,user_agent) VALUES($1,'storage_zone.update','storage_zone',$2,$3::jsonb,$4::jsonb,$5)`, user.UserID, id, string(oldJSON), string(newJSON), r.UserAgent()); err != nil {
		respondWarehouseMutationError(w, err)
		return
	}
	response := map[string]any{"zone_id": id, "code": input.Code, "barcode": *input.Barcode, "name": input.Name, "updated_at": version}
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
