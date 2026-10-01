package handlers

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"strings"

	"github.com/gorilla/mux"
	"warehousecore/internal/middleware"
	"warehousecore/internal/repository"
)

const warehouseLocationLifecycleDependenciesSQL = `WITH RECURSIVE descendants AS (
 SELECT zone_id,is_active FROM storage_zones WHERE parent_zone_id=$1
 UNION SELECT z.zone_id,z.is_active FROM storage_zones z JOIN descendants d ON z.parent_zone_id=d.zone_id
) SELECT
 (SELECT count(*) FROM descendants WHERE is_active) AS active_descendants,
 (SELECT count(*) FROM devices WHERE zone_id=$1 AND COALESCE(lifecycle_status,'active')='active') AS active_devices,
 (SELECT count(*) FROM cases WHERE zone_id=$1) AS cases,
 (SELECT count(*) FROM cases WHERE home_zone_id=$1) AS home_cases,
 (SELECT count(*) FROM product_locations WHERE zone_id=$1 AND quantity<>0) AS stock_lines,
 (SELECT count(*) FROM warehouse_tasks WHERE (from_zone_id=$1 OR to_zone_id=$1) AND lower(COALESCE(status,'')) NOT IN ('done','cancelled')) AS open_tasks,
 (SELECT count(*) FROM inventory_counts WHERE zone_id=$1 AND lower(COALESCE(status,'')) NOT IN ('approved','cancelled')) AS open_counts,
 (SELECT count(*) FROM inventory_counts WHERE zone_id=$1) AS historic_counts`

// Only an unchanged, audited archive can restore the previous operational state.
// Legacy archives and subsequent edits restore blocked until reviewed in the UI.
const warehouseLocationRestoreStatusSQL = `SELECT COALESCE((SELECT CASE WHEN old_values->>'operational_status' IN ('available','blocked','maintenance') THEN old_values->>'operational_status' ELSE 'blocked' END
 FROM audit_log WHERE entity_type='storage_zone' AND entity_id=$1 AND action='storage_zone.archive' AND new_values->>'updated_at'=$2 ORDER BY id DESC LIMIT 1),'blocked') AS restore_status`

func ArchiveWarehouseLocationMCP(w http.ResponseWriter, r *http.Request) {
	mutateWarehouseLocationLifecycleMCP(w, r, "archive")
}
func RestoreWarehouseLocationMCP(w http.ResponseWriter, r *http.Request) {
	mutateWarehouseLocationLifecycleMCP(w, r, "restore")
}
func mutateWarehouseLocationLifecycleMCP(w http.ResponseWriter, r *http.Request, operation string) {
	if !isWarehouseMCPMutation(r) {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "Use the guided MCP location lifecycle workflow"})
		return
	}
	user, ok := middleware.GetUserFromContext(r)
	if !ok || user == nil || !user.IsAdmin {
		respondJSON(w, http.StatusForbidden, map[string]string{"error": "Warehouse administrator permission is required"})
		return
	}
	id, err := strconv.ParseInt(mux.Vars(r)["id"], 10, 64)
	if err != nil || id <= 0 || id > math.MaxInt32 {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid location ID"})
		return
	}
	var in struct {
		ExpectedUpdatedAt string `json:"expected_updated_at"`
		ConfirmLifecycle  bool   `json:"confirm_lifecycle"`
		ConfirmationText  string `json:"confirmation_text"`
	}
	phrase := fmt.Sprintf("%s WAREHOUSE LOCATION %d", strings.ToUpper(operation), id)
	if json.NewDecoder(r.Body).Decode(&in) != nil || in.ExpectedUpdatedAt == "" || !in.ConfirmLifecycle || in.ConfirmationText != phrase {
		respondJSON(w, http.StatusPreconditionRequired, map[string]string{"error": "Exact version, explicit lifecycle confirmation and location-bound phrase are required"})
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
	receipt, replay, err := beginWarehouseProductMutation(tx, r, "location."+operation, map[string]any{"id": id, "expected_updated_at": in.ExpectedUpdatedAt, "confirmation_text": phrase})
	if err != nil {
		respondWarehouseMutationError(w, err)
		return
	}
	if replay != nil {
		respondJSON(w, http.StatusOK, replay)
		return
	}
	if _, err = tx.Exec(`LOCK TABLE storage_zones IN SHARE ROW EXCLUSIVE MODE; LOCK TABLE devices,cases,product_locations,warehouse_tasks,inventory_counts IN SHARE MODE`); err != nil {
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
	if version != in.ExpectedUpdatedAt {
		respondJSON(w, http.StatusConflict, map[string]string{"error": "Location changed since preview"})
		return
	}
	var before map[string]any
	if err = json.Unmarshal(raw, &before); err != nil {
		respondWarehouseMutationError(w, err)
		return
	}
	target := operation == "restore"
	if active == target || !target && before["operational_status"] == "archived" {
		respondJSON(w, http.StatusConflict, map[string]string{"error": "Current location lifecycle does not permit this action"})
		return
	}
	var children, devices, cases, homes, stock, tasks, counts, history int64
	if err = tx.QueryRow(warehouseLocationLifecycleDependenciesSQL, id).Scan(&children, &devices, &cases, &homes, &stock, &tasks, &counts, &history); err != nil {
		respondWarehouseMutationError(w, err)
		return
	}
	if children+devices+cases+homes+stock+tasks+counts > 0 || !target && before["operational_status"] == "counting" {
		respondJSON(w, http.StatusConflict, map[string]any{"error": "Inventory, active descendants, home cases, open tasks or counts block location lifecycle changes", "active_descendants": children, "active_devices": devices, "cases": cases, "home_cases": homes, "stock_lines": stock, "open_tasks": tasks, "open_counts": counts})
		return
	}
	status := "archived"
	if target {
		var validation warehouseLocationInput
		if err = json.Unmarshal(raw, &validation); err != nil {
			respondWarehouseMutationError(w, err)
			return
		}
		original := validation
		if err = validateWarehouseLocationInput(&validation); err != nil || original.Code == "" || original.Name == "" || original.Barcode == nil || *original.Barcode == "" || len(original.Code) > 50 || len([]rune(original.Name)) > 100 || len(*original.Barcode) > 255 || !warehouseLocationTypeAllowed(original.Type) || !warehouseLocationKindAllowed(original.LocationKind) || !warehouseLocationRoleAllowed(original.ProcessRole) || original.CapacityMode != "item_count" {
			respondJSON(w, http.StatusConflict, map[string]string{"error": "Stored location fields must be valid before restoration"})
			return
		}
		invalidCapacity := original.Capacity != nil && (math.IsNaN(*original.Capacity) || math.IsInf(*original.Capacity, 0) || math.Trunc(*original.Capacity) != *original.Capacity || *original.Capacity > math.MaxInt32)
		invalidWeight := original.MaxWeightKg != nil && (*original.MaxWeightKg <= 0 || *original.MaxWeightKg > 999999999.999 || math.IsNaN(*original.MaxWeightKg) || math.IsInf(*original.MaxWeightKg, 0))
		invalidVolume := original.MaxVolumeM3 != nil && (*original.MaxVolumeM3 <= 0 || *original.MaxVolumeM3 > 999999.999999 || math.IsNaN(*original.MaxVolumeM3) || math.IsInf(*original.MaxVolumeM3, 0))
		invalidSequence := original.PickSequence != nil && (*original.PickSequence < math.MinInt32 || *original.PickSequence > math.MaxInt32)
		invalidFrequency := original.InventoryFrequencyDays != nil && *original.InventoryFrequencyDays > math.MaxInt32
		if invalidCapacity || invalidWeight || invalidVolume || invalidSequence || invalidFrequency {
			respondJSON(w, http.StatusConflict, map[string]string{"error": "Stored location limits must be valid before restoration"})
			return
		}
		if original.ParentZoneID != nil {
			var invalid bool
			err = tx.QueryRow(`WITH RECURSIVE ancestors AS (SELECT zone_id,parent_zone_id,is_active,operational_status,ARRAY[zone_id] AS path,false AS cycle FROM storage_zones WHERE zone_id=$1 UNION ALL SELECT z.zone_id,z.parent_zone_id,z.is_active,z.operational_status,a.path||z.zone_id,z.zone_id=ANY(a.path) FROM storage_zones z JOIN ancestors a ON z.zone_id=a.parent_zone_id WHERE NOT a.cycle) SELECT NOT EXISTS(SELECT 1 FROM ancestors) OR EXISTS(SELECT 1 FROM ancestors a LEFT JOIN storage_zones p ON p.zone_id=a.parent_zone_id WHERE a.zone_id=$2 OR NOT a.is_active OR a.operational_status='archived' OR a.cycle OR (a.parent_zone_id IS NOT NULL AND p.zone_id IS NULL))`, *original.ParentZoneID, id).Scan(&invalid)
			if err != nil {
				respondWarehouseMutationError(w, err)
				return
			}
			if invalid {
				respondJSON(w, http.StatusConflict, map[string]string{"error": "Parent hierarchy is missing, inactive or cyclic"})
				return
			}
		}
		var duplicate bool
		if err = tx.QueryRow(`SELECT EXISTS(SELECT 1 FROM storage_zones WHERE zone_id<>$1 AND (lower(trim(code))=lower(trim($2)) OR lower(trim(barcode))=lower(trim($3)) OR (lower(trim(name))=lower(trim($4)) AND parent_zone_id IS NOT DISTINCT FROM $5)))`, id, original.Code, *original.Barcode, original.Name, original.ParentZoneID).Scan(&duplicate); err != nil {
			respondWarehouseMutationError(w, err)
			return
		}
		if duplicate {
			respondJSON(w, http.StatusConflict, map[string]string{"error": "Location identity conflicts with another record"})
			return
		}
		if err = tx.QueryRow(warehouseLocationRestoreStatusSQL, fmt.Sprint(id), version).Scan(&status); err != nil {
			respondWarehouseMutationError(w, err)
			return
		}
	}
	if err = tx.QueryRow(`UPDATE storage_zones SET is_active=$1,operational_status=$2 WHERE zone_id=$3 RETURNING to_char(updated_at,'YYYY-MM-DD"T"HH24:MI:SS.US"Z"')`, target, status, id).Scan(&version); err != nil {
		respondWarehouseMutationError(w, err)
		return
	}
	after := map[string]any{}
	for key, value := range before {
		after[key] = value
	}
	after["is_active"], after["operational_status"], after["updated_at"] = target, status, version
	oldJSON, _ := json.Marshal(before)
	newJSON, _ := json.Marshal(map[string]any{"origin": "MCP/AI", "after": after, "updated_at": version})
	var auditID int64
	err = tx.QueryRow(`INSERT INTO audit_log(user_id,action,entity_type,entity_id,old_values,new_values,user_agent) VALUES($1,$2,'storage_zone',$3,$4::jsonb,$5::jsonb,$6) RETURNING id`, user.UserID, "storage_zone."+operation, fmt.Sprint(id), string(oldJSON), string(newJSON), r.UserAgent()).Scan(&auditID)
	if err != nil {
		respondWarehouseMutationError(w, err)
		return
	}
	response := map[string]any{"zone_id": id, "code": before["code"], "barcode": before["barcode"], "name": before["name"], "is_active": target, "operational_status": status, "updated_at": version, "audit_id": auditID}
	if err = completeWarehouseProductMutation(tx, receipt, http.StatusOK, response); err != nil {
		respondWarehouseMutationError(w, err)
		return
	}
	if err = tx.Commit(); err != nil {
		respondWarehouseMutationError(w, err)
		return
	}
	respondJSON(w, http.StatusOK, response)
}
