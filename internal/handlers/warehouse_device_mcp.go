package handlers

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"reflect"
	"strings"
	"time"

	"github.com/gorilla/mux"
	"github.com/lib/pq"
	"warehousecore/internal/middleware"
	"warehousecore/internal/repository"
	"warehousecore/internal/services"
)

type warehouseDeviceFields struct {
	ProductID       int64   `json:"product_id"`
	SerialNumber    *string `json:"serial_number"`
	Barcode         *string `json:"barcode"`
	QRCode          *string `json:"qr_code"`
	ConditionRating float64 `json:"condition_rating"`
	UsageHours      float64 `json:"usage_hours"`
	PurchaseDate    *string `json:"purchase_date"`
	LastMaintenance *string `json:"last_maintenance"`
	NextMaintenance *string `json:"next_maintenance"`
	Notes           *string `json:"notes"`
}
type warehouseDeviceRequest struct {
	warehouseDeviceFields
	ZoneID            *int64 `json:"zone_id"`
	ConditionStatus   string `json:"condition_status"`
	ExpectedUpdatedAt string `json:"expected_updated_at"`
	ConfirmLifecycle  bool   `json:"confirm_lifecycle"`
	ConfirmRevert     bool   `json:"confirm_revert"`
	ConfirmationText  string `json:"confirmation_text"`
	AuditID           int64  `json:"audit_id"`
}

const warehouseDeviceDependenciesSQL = `SELECT
 (SELECT count(*) FROM job_devices jd JOIN jobs j ON j.jobid=jd.jobid JOIN status s ON s.statusid=j.statusid WHERE jd.deviceid=$1 AND ((j.deleted_at IS NULL AND NOT warehouse_job_status_is_closed(s.status)) OR jd.pack_status IN ('packed','issued'))) AS jobs,
 (SELECT count(*) FROM job_position_devices pd JOIN job_positions p ON p.position_id=pd.position_id JOIN jobs j ON j.jobid=p.job_id JOIN status s ON s.statusid=j.statusid WHERE pd.device_id=$1 AND j.deleted_at IS NULL AND NOT warehouse_job_status_is_closed(s.status)) AS picklists,
 (SELECT count(*) FROM job_package_reservations WHERE device_id=$1 AND reservation_status<>'released') AS reservations,
 (SELECT count(*) FROM devicescases WHERE deviceid=$1)+(SELECT count(*) FROM devices WHERE deviceid=$1 AND current_case_id IS NOT NULL) AS cases,
 (SELECT count(*) FROM device_components WHERE device_id=$1 OR component_device_id=$1) AS components,
 (SELECT count(*) FROM warehouse_tasks WHERE device_id=$1 AND lower(status) NOT IN ('completed','done','cancelled','canceled','closed')) AS tasks,
 (SELECT count(*) FROM maintenance_orders WHERE device_id=$1 AND status NOT IN ('completed','cancelled')) AS maintenance_orders,
 (SELECT count(*) FROM maintenance_plans WHERE device_id=$1 AND is_active) AS maintenance_plans,
 (SELECT count(*) FROM defect_reports WHERE device_id=$1 AND lower(COALESCE(status,'')) NOT IN ('resolved','closed','done','completed')) AS defects
`

const warehouseDeviceSnapshotSQL = `SELECT jsonb_build_object(
 'product_id',productid,'serial_number',serialnumber,'barcode',barcode,'qr_code',qr_code,
 'condition_rating',COALESCE(condition_rating,5),'usage_hours',COALESCE(usage_hours,0),
 'purchase_date',to_char(purchasedate,'YYYY-MM-DD'),'last_maintenance',to_char(lastmaintenance,'YYYY-MM-DD'),'next_maintenance',to_char(nextmaintenance,'YYYY-MM-DD'),'notes',notes),
 to_char(updated_at,'YYYY-MM-DD"T"HH24:MI:SS.US"Z"'),lifecycle_status,status,condition_status,zone_id,archived_by_product
 FROM devices WHERE deviceid=$1 FOR UPDATE`

func normalizeWarehouseDevice(f *warehouseDeviceFields, create bool) error {
	if f.ProductID <= 0 || f.ProductID > math.MaxInt32 {
		return fmt.Errorf("existing individually tracked product_id is required")
	}
	for _, field := range []struct {
		value **string
		max   int
	}{{&f.SerialNumber, 255}, {&f.Barcode, 255}, {&f.QRCode, 255}, {&f.Notes, 4000}} {
		if *field.value != nil {
			v := strings.TrimSpace(**field.value)
			if len([]rune(v)) > field.max {
				return fmt.Errorf("device text field is too long")
			}
			if v == "" {
				*field.value = nil
			} else {
				*field.value = &v
			}
		}
	}
	if !create && (f.Barcode == nil || f.QRCode == nil) {
		return fmt.Errorf("barcode and qr_code cannot be cleared")
	}
	if math.IsNaN(f.ConditionRating) || math.IsInf(f.ConditionRating, 0) || f.ConditionRating < 0 || f.ConditionRating > 5 || math.Abs(f.ConditionRating*10-math.Round(f.ConditionRating*10)) > 0.00001 {
		return fmt.Errorf("condition_rating requires 0-5 with at most one decimal")
	}
	if math.IsNaN(f.UsageHours) || math.IsInf(f.UsageHours, 0) || f.UsageHours < 0 || f.UsageHours > 99999999.99 || math.Abs(f.UsageHours*100-math.Round(f.UsageHours*100)) > 0.00001 {
		return fmt.Errorf("usage_hours requires 0-99999999.99 with at most two decimals")
	}
	for _, date := range []**string{&f.PurchaseDate, &f.LastMaintenance, &f.NextMaintenance} {
		if *date != nil {
			v := strings.TrimSpace(**date)
			if v == "" {
				*date = nil
				continue
			}
			if _, err := time.Parse("2006-01-02", v); err != nil {
				return fmt.Errorf("dates must be valid YYYY-MM-DD dates")
			}
			*date = &v
		}
	}
	if f.LastMaintenance != nil && f.NextMaintenance != nil && *f.LastMaintenance > *f.NextMaintenance {
		return fmt.Errorf("next_maintenance cannot precede last_maintenance")
	}
	return nil
}

func deviceDependencyCounts(tx *sql.Tx, id string) (map[string]int64, error) {
	counts := make([]int64, 9)
	ptrs := make([]any, 9)
	for i := range counts {
		ptrs[i] = &counts[i]
	}
	if err := tx.QueryRow(warehouseDeviceDependenciesSQL, id).Scan(ptrs...); err != nil {
		return nil, err
	}
	out := map[string]int64{}
	for i, name := range []string{"jobs", "picklists", "reservations", "cases", "components", "tasks", "maintenance_orders", "maintenance_plans", "defects"} {
		out[name] = counts[i]
	}
	return out, nil
}
func hasDeviceDependencies(counts map[string]int64) bool {
	for _, count := range counts {
		if count > 0 {
			return true
		}
	}
	return false
}

func validateWarehouseDeviceReferences(tx *sql.Tx, id string, fields warehouseDeviceFields) error {
	var active bool
	err := tx.QueryRow(`SELECT COALESCE(lifecycle_status,'active')='active' AND tracking_mode='individual' FROM products WHERE productid=$1`, fields.ProductID).Scan(&active)
	if err == sql.ErrNoRows {
		return &warehouseMutationError{http.StatusNotFound, "product_not_found", "Product not found"}
	}
	if err != nil {
		return err
	}
	if !active {
		return &warehouseMutationError{http.StatusConflict, "product_unavailable", "An active individually tracked product is required"}
	}
	if fields.SerialNumber != nil {
		var duplicate bool
		if err = tx.QueryRow(`SELECT EXISTS(SELECT 1 FROM devices WHERE deviceid<>$1 AND lower(trim(serialnumber))=lower($2))`, id, *fields.SerialNumber).Scan(&duplicate); err != nil {
			return err
		}
		if duplicate {
			return &warehouseMutationError{http.StatusConflict, "duplicate_serial", "Serial number belongs to another device, including archived devices"}
		}
	}
	codes := []*string{fields.Barcode, fields.QRCode}
	if id != "" {
		codes = append(codes, &id)
	}
	for _, code := range codes {
		if code == nil {
			continue
		}
		var conflict bool
		err = tx.QueryRow(`SELECT EXISTS(SELECT 1 FROM devices WHERE deviceid<>$1 AND (lower(trim(deviceid))=lower($2) OR lower(trim(barcode))=lower($2) OR lower(trim(qr_code))=lower($2))) OR EXISTS(SELECT 1 FROM inventory_identifiers WHERE NOT(entity_type='device' AND entity_key=$1) AND lower(trim(code))=lower($2))`, id, *code).Scan(&conflict)
		if err != nil {
			return err
		}
		if conflict {
			return &warehouseMutationError{http.StatusConflict, "identifier_conflict", "Scan code is already reserved by another inventory record"}
		}
	}
	return nil
}

func loadDeviceForMCP(tx *sql.Tx, id string) (warehouseDeviceFields, string, string, string, string, *int64, bool, error) {
	var fields warehouseDeviceFields
	var raw []byte
	var version, lifecycle, physical, condition string
	var zone sql.NullInt64
	var byProduct bool
	err := tx.QueryRow(warehouseDeviceSnapshotSQL, id).Scan(&raw, &version, &lifecycle, &physical, &condition, &zone, &byProduct)
	if err != nil {
		return fields, "", "", "", "", nil, false, err
	}
	if err = json.Unmarshal(raw, &fields); err != nil {
		return fields, "", "", "", "", nil, false, err
	}
	var zoneID *int64
	if zone.Valid {
		zoneID = &zone.Int64
	}
	return fields, version, lifecycle, physical, condition, zoneID, byProduct, nil
}

func deviceRevertPhrase(id string, auditID int64) string {
	return fmt.Sprintf("REVERT WAREHOUSE DEVICE %s UPDATE %d", id, auditID)
}
func RevertDeviceUpdateMCP(w http.ResponseWriter, r *http.Request) {
	if !isWarehouseMCPMutation(r) {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "This endpoint requires the guided MCP workflow"})
		return
	}
	mutateWarehouseDeviceMCP(w, r, "revert_update", mux.Vars(r)["id"])
}
func deviceMCPError(w http.ResponseWriter, err error) {
	if e, ok := err.(*pq.Error); ok && (e.Code == "23505" || e.Code == "23503") {
		respondJSON(w, http.StatusConflict, map[string]string{"error": "Device identifiers or references conflict with current records"})
		return
	}
	respondWarehouseMutationError(w, err)
}

func mutateWarehouseDeviceMCP(w http.ResponseWriter, r *http.Request, operation, id string) {
	user, ok := middleware.GetUserFromContext(r)
	if !ok || user == nil || !user.IsAdmin {
		respondJSON(w, http.StatusForbidden, map[string]string{"error": "Warehouse administrator permission is required"})
		return
	}
	if operation != "create" && (id == "" || strings.TrimSpace(id) != id || len([]rune(id)) > 50) {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "Exact device ID is required"})
		return
	}
	input := warehouseDeviceRequest{warehouseDeviceFields: warehouseDeviceFields{ConditionRating: 5}, ConditionStatus: "available"}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid device body"})
		return
	}
	if operation != "create" && input.ExpectedUpdatedAt == "" {
		respondJSON(w, http.StatusPreconditionRequired, map[string]string{"error": "The exact expected_updated_at is required"})
		return
	}
	if operation == "archive" || operation == "restore" {
		if !input.ConfirmLifecycle || input.ConfirmationText != strings.ToUpper(operation)+" WAREHOUSE DEVICE "+id {
			respondJSON(w, http.StatusPreconditionRequired, map[string]string{"error": "Explicit lifecycle confirmation and exact device-bound phrase are required"})
			return
		}
	}
	if operation == "revert_update" && (input.AuditID <= 0 || !input.ConfirmRevert || input.ConfirmationText != deviceRevertPhrase(id, input.AuditID)) {
		respondJSON(w, http.StatusPreconditionRequired, map[string]string{"error": "Explicit revert confirmation and exact audit/device-bound phrase are required"})
		return
	}
	if operation == "create" || operation == "update" {
		if err := normalizeWarehouseDevice(&input.warehouseDeviceFields, operation == "create"); err != nil {
			respondJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
	}
	if operation == "update" && (input.ZoneID != nil || input.ConditionStatus != "available") {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "Location and condition changes use the existing movement/status workflows"})
		return
	}
	payload := map[string]any{"id": id, "expected_updated_at": input.ExpectedUpdatedAt}
	if operation == "create" || operation == "update" {
		payload["after"] = input.warehouseDeviceFields
	}
	if operation == "create" {
		payload["zone_id"], payload["condition_status"] = input.ZoneID, input.ConditionStatus
	}
	if operation == "archive" || operation == "restore" || operation == "revert_update" {
		payload["confirmation_text"] = input.ConfirmationText
	}
	if operation == "revert_update" {
		payload["audit_id"] = input.AuditID
	}
	tx, err := repository.GetSQLDB().BeginTx(r.Context(), nil)
	if err != nil {
		deviceMCPError(w, err)
		return
	}
	defer tx.Rollback()
	if _, err = tx.Exec(`SET LOCAL lock_timeout='5s'; SET LOCAL statement_timeout='15s'`); err != nil {
		deviceMCPError(w, err)
		return
	}
	receipt, replay, err := beginWarehouseProductMutation(tx, r, "device."+operation, payload)
	if err != nil {
		deviceMCPError(w, err)
		return
	}
	status := http.StatusOK
	if operation == "create" {
		status = http.StatusCreated
	}
	if replay != nil {
		respondJSON(w, status, replay)
		return
	}
	if _, err = tx.Exec(`LOCK TABLE devices IN SHARE ROW EXCLUSIVE MODE; LOCK TABLE products,inventory_identifiers,storage_zones,cases,product_locations,job_devices,jobs,status,job_position_devices,job_positions,job_package_reservations,devicescases,device_components,warehouse_tasks,maintenance_orders,maintenance_plans,defect_reports,device_movements,audit_log IN SHARE MODE`); err != nil {
		deviceMCPError(w, err)
		return
	}
	var before any
	var version string
	var lifecycle, physical, condition string
	var zone *int64
	if operation == "create" {
		if input.ZoneID != nil && (*input.ZoneID <= 0 || *input.ZoneID > math.MaxInt32) {
			respondJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid zone ID"})
			return
		}
		if !map[string]bool{"available": true, "blocked": true, "defective": true, "maintenance": true, "retired": true}[input.ConditionStatus] {
			respondJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid initial condition"})
			return
		}
		if err = validateWarehouseDeviceReferences(tx, "", input.warehouseDeviceFields); err != nil {
			deviceMCPError(w, err)
			return
		}
		physical, condition, zone = "location_unknown", input.ConditionStatus, input.ZoneID
		location := "location_unknown"
		if zone != nil {
			if err = services.ValidateStorageDestination(tx, *zone, 1); err != nil {
				respondJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
				return
			}
			physical, location = "in_storage", "warehouse"
		}
		err = tx.QueryRow(`INSERT INTO devices(productid,serialnumber,barcode,qr_code,condition_rating,usage_hours,purchasedate,lastmaintenance,nextmaintenance,notes,status,condition_status,current_location,zone_id) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14) RETURNING deviceid`, input.ProductID, input.SerialNumber, input.Barcode, input.QRCode, input.ConditionRating, input.UsageHours, input.PurchaseDate, input.LastMaintenance, input.NextMaintenance, input.Notes, physical, condition, location, zone).Scan(&id)
		if err != nil {
			deviceMCPError(w, err)
			return
		}
	} else {
		current, v, l, p, c, z, _, loadErr := loadDeviceForMCP(tx, id)
		if loadErr == sql.ErrNoRows {
			respondJSON(w, http.StatusNotFound, map[string]string{"error": "Device not found"})
			return
		}
		if loadErr != nil {
			deviceMCPError(w, loadErr)
			return
		}
		version, lifecycle, physical, condition, zone = v, l, p, c, z
		if version != input.ExpectedUpdatedAt {
			respondJSON(w, http.StatusConflict, map[string]string{"error": "Device changed since preview"})
			return
		}
		if operation == "update" || operation == "revert_update" {
			if lifecycle != "active" {
				respondJSON(w, http.StatusConflict, map[string]string{"error": "Restore the archived device before editing"})
				return
			}
			if operation == "revert_update" {
				var previousRaw []byte
				var actor int64
				var auditAction, resultVersion, origin string
				var latest int64
				err = tx.QueryRow(`SELECT COALESCE(user_id,0),COALESCE(action,''),COALESCE(old_values,'null'::jsonb),COALESCE(new_values->>'updated_at',''),COALESCE(new_values->>'origin',''),(SELECT max(id) FROM audit_log WHERE entity_type='device' AND entity_id=$2) FROM audit_log WHERE id=$1 AND entity_type='device' AND entity_id=$2`, input.AuditID, id).Scan(&actor, &auditAction, &previousRaw, &resultVersion, &origin, &latest)
				if err == sql.ErrNoRows {
					respondJSON(w, http.StatusNotFound, map[string]string{"error": "Device update audit not found"})
					return
				}
				if err != nil {
					deviceMCPError(w, err)
					return
				}
				if actor != int64(user.UserID) || origin != "MCP/AI" || auditAction != "device.update" || latest != input.AuditID || resultVersion != version {
					respondJSON(w, http.StatusConflict, map[string]string{"error": "Only your latest unchanged MCP device update can be reverted"})
					return
				}
				if err = json.Unmarshal(previousRaw, &input.warehouseDeviceFields); err != nil {
					deviceMCPError(w, err)
					return
				}
				if err = normalizeWarehouseDevice(&input.warehouseDeviceFields, false); err != nil {
					respondJSON(w, http.StatusConflict, map[string]string{"error": "Previous fields cannot be safely restored"})
					return
				}
			}
			if reflect.DeepEqual(current, input.warehouseDeviceFields) {
				respondJSON(w, http.StatusConflict, map[string]string{"error": "No device fields changed"})
				return
			}
			identityChanged := current.ProductID != input.ProductID || !reflect.DeepEqual(current.SerialNumber, input.SerialNumber) || !reflect.DeepEqual(current.Barcode, input.Barcode) || !reflect.DeepEqual(current.QRCode, input.QRCode)
			if identityChanged {
				deps, e := deviceDependencyCounts(tx, id)
				if e != nil {
					deviceMCPError(w, e)
					return
				}
				if hasDeviceDependencies(deps) || physical == "on_job" || physical == "return_pending" {
					respondJSON(w, http.StatusConflict, map[string]any{"error": "Active dependencies block device identity changes", "dependencies": deps})
					return
				}
			}
			if current.ProductID != input.ProductID {
				var history bool
				err = tx.QueryRow(`SELECT EXISTS(SELECT 1 FROM job_devices WHERE deviceid=$1) OR EXISTS(SELECT 1 FROM device_movements WHERE device_id=$1) OR EXISTS(SELECT 1 FROM maintenance_orders WHERE device_id=$1) OR EXISTS(SELECT 1 FROM maintenance_plans WHERE device_id=$1) OR EXISTS(SELECT 1 FROM defect_reports WHERE device_id=$1) OR EXISTS(SELECT 1 FROM job_package_reservations WHERE device_id=$1) OR EXISTS(SELECT 1 FROM job_position_devices WHERE device_id=$1)`, id).Scan(&history)
				if err != nil {
					deviceMCPError(w, err)
					return
				}
				if history {
					respondJSON(w, http.StatusConflict, map[string]string{"error": "Device history blocks product reassociation"})
					return
				}
			}
			if err = validateWarehouseDeviceReferences(tx, id, input.warehouseDeviceFields); err != nil {
				deviceMCPError(w, err)
				return
			}
			before = current
			_, err = tx.Exec(`UPDATE devices SET productid=$1,serialnumber=$2,barcode=$3,qr_code=$4,condition_rating=$5,usage_hours=$6,purchasedate=$7,lastmaintenance=$8,nextmaintenance=$9,notes=$10 WHERE deviceid=$11`, input.ProductID, input.SerialNumber, input.Barcode, input.QRCode, input.ConditionRating, input.UsageHours, input.PurchaseDate, input.LastMaintenance, input.NextMaintenance, input.Notes, id)
		} else {
			wanted, target := "active", "archived"
			if operation == "restore" {
				wanted, target = target, wanted
			}
			if lifecycle != wanted {
				respondJSON(w, http.StatusConflict, map[string]string{"error": "Current lifecycle does not permit this action"})
				return
			}
			deps, e := deviceDependencyCounts(tx, id)
			if e != nil {
				deviceMCPError(w, e)
				return
			}
			if hasDeviceDependencies(deps) || physical == "on_job" || physical == "return_pending" {
				respondJSON(w, http.StatusConflict, map[string]any{"error": "Active dependencies block the lifecycle change", "dependencies": deps})
				return
			}
			if operation == "restore" {
				var aliasConflict bool
				if err = tx.QueryRow(`SELECT EXISTS(SELECT 1 FROM inventory_identifiers own WHERE own.entity_type='device' AND own.entity_key=$1 AND (EXISTS(SELECT 1 FROM inventory_identifiers other WHERE NOT(other.entity_type='device' AND other.entity_key=$1) AND lower(trim(other.code))=lower(trim(own.code))) OR EXISTS(SELECT 1 FROM devices d WHERE d.deviceid<>$1 AND (lower(trim(d.deviceid))=lower(trim(own.code)) OR lower(trim(d.barcode))=lower(trim(own.code)) OR lower(trim(d.qr_code))=lower(trim(own.code))))))`, id).Scan(&aliasConflict); err != nil {
					deviceMCPError(w, err)
					return
				}
				if aliasConflict {
					respondJSON(w, http.StatusConflict, map[string]string{"error": "A reserved device alias conflicts with another inventory record"})
					return
				}
				if err = validateWarehouseDeviceReferences(tx, id, current); err != nil {
					deviceMCPError(w, err)
					return
				}
				if zone != nil {
					if err = services.ValidateStorageDestination(tx, *zone, 1); err != nil {
						respondJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
						return
					}
				}
				if physical == "in_storage" && zone == nil {
					respondJSON(w, http.StatusConflict, map[string]string{"error": "Stored device lacks a valid storage destination"})
					return
				}
			}
			before = map[string]any{"fields": current, "lifecycle_status": lifecycle}
			_, err = tx.Exec(`UPDATE devices SET lifecycle_status=$1::text,archived_at=CASE WHEN $1::text='archived' THEN CURRENT_TIMESTAMP ELSE NULL END,archived_by_product=false WHERE deviceid=$2`, target, id)
		}
		if err == nil && (operation == "archive" || operation == "restore") {
			_, err = tx.Exec(`UPDATE inventory_identifiers SET active=$1 WHERE entity_type='device' AND entity_key=$2`, operation == "restore", id)
		}
		if err != nil {
			deviceMCPError(w, err)
			return
		}
	}
	after, v, l, p, c, z, byProduct, err := loadDeviceForMCP(tx, id)
	if err != nil {
		deviceMCPError(w, err)
		return
	}
	if operation == "create" {
		if err = validateWarehouseDeviceReferences(tx, id, after); err != nil {
			deviceMCPError(w, err)
			return
		}
	}
	version, lifecycle, physical, condition, zone = v, l, p, c, z
	response := map[string]any{"device_id": id, "fields": after, "updated_at": version, "lifecycle_status": lifecycle, "physical_status": physical, "condition_status": condition, "zone_id": zone, "archived_by_product": byProduct}
	oldJSON, _ := json.Marshal(before)
	newJSON, err := json.Marshal(map[string]any{"origin": "MCP/AI", "after": after, "updated_at": version, "lifecycle_status": lifecycle, "reverted_audit_id": input.AuditID})
	if err != nil {
		deviceMCPError(w, err)
		return
	}
	var auditID int64
	err = tx.QueryRow(`INSERT INTO audit_log(user_id,action,entity_type,entity_id,old_values,new_values,user_agent) VALUES($1,$2,'device',$3,$4::jsonb,$5::jsonb,$6) RETURNING id`, user.UserID, "device."+operation, id, string(oldJSON), string(newJSON), r.UserAgent()).Scan(&auditID)
	if err != nil {
		deviceMCPError(w, err)
		return
	}
	response["audit_id"] = auditID
	if err = completeWarehouseProductMutation(tx, receipt, status, response); err != nil {
		deviceMCPError(w, err)
		return
	}
	if err = tx.Commit(); err != nil {
		deviceMCPError(w, err)
		return
	}
	respondJSON(w, status, response)
}
