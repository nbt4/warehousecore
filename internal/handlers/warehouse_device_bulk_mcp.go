package handlers

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"strings"

	"warehousecore/internal/middleware"
	"warehousecore/internal/repository"
	"warehousecore/internal/services"
)

type warehouseDeviceBulkItem struct {
	warehouseDeviceFields
	ZoneID          *int64 `json:"zone_id"`
	ConditionStatus string `json:"condition_status"`
}

// Bulk creation is a bounded business transaction, never a loop over HTTP writes.
func CreateDevicesBulkMCP(w http.ResponseWriter, r *http.Request) {
	user, ok := middleware.GetUserFromContext(r)
	if !ok || user == nil || user.UserID == 0 || !user.IsAdmin || !isWarehouseMCPMutation(r) {
		respondJSON(w, http.StatusForbidden, map[string]string{"error": "A signed-in warehouse administrator and MCP origin are required"})
		return
	}
	var in struct {
		Devices          []json.RawMessage `json:"devices"`
		ConfirmCreation  bool              `json:"confirm_creation"`
		ConfirmationText string            `json:"confirmation_text"`
		Preview          bool              `json:"preview"`
	}
	d := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1024*1024))
	d.DisallowUnknownFields()
	if err := d.Decode(&in); err != nil {
		respondJSON(w, 400, map[string]string{"error": "Invalid bounded device batch"})
		return
	}
	if err := d.Decode(new(any)); err != io.EOF || len(in.Devices) < 1 || len(in.Devices) > 100 {
		respondJSON(w, 400, map[string]string{"error": "Exactly one object with 1-100 devices is required"})
		return
	}
	items := make([]warehouseDeviceBulkItem, len(in.Devices))
	serials, codes, occupancy := map[string]int{}, map[string]int{}, map[int64]int{}
	for i, raw := range in.Devices {
		items[i].ConditionRating = 5
		items[i].ConditionStatus = "available"
		decoder := json.NewDecoder(bytes.NewReader(raw))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&items[i]); err != nil || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
			respondJSON(w, 400, map[string]any{"error": "Invalid device fields", "item_index": i})
			return
		}
		item := &items[i]
		if err := normalizeWarehouseDevice(&item.warehouseDeviceFields, true); err != nil {
			respondJSON(w, 400, map[string]any{"error": err.Error(), "item_index": i})
			return
		}
		if !map[string]bool{"available": true, "blocked": true, "defective": true, "maintenance": true, "retired": true}[item.ConditionStatus] {
			respondJSON(w, 400, map[string]any{"error": "Invalid initial condition", "item_index": i})
			return
		}
		if item.ZoneID != nil {
			if *item.ZoneID <= 0 || *item.ZoneID > math.MaxInt32 {
				respondJSON(w, 400, map[string]string{"error": "Invalid zone_id"})
				return
			}
			occupancy[*item.ZoneID]++
		}
		if item.SerialNumber != nil {
			key := strings.ToLower(*item.SerialNumber)
			if previous, exists := serials[key]; exists {
				respondJSON(w, 409, map[string]any{"error": "Duplicate serial within batch", "item_index": i, "conflicting_item_index": previous})
				return
			}
			serials[key] = i
		}
		for _, code := range []*string{item.Barcode, item.QRCode} {
			if code == nil {
				continue
			}
			key := strings.ToLower(*code)
			if previous, exists := codes[key]; exists && previous != i {
				respondJSON(w, 409, map[string]any{"error": "Duplicate scan identity within batch", "item_index": i, "conflicting_item_index": previous})
				return
			}
			codes[key] = i
		}
	}
	preview := in.Preview || !in.ConfirmCreation
	encoded, err := json.Marshal(items)
	if err != nil {
		deviceMCPError(w, err)
		return
	}
	digest := sha256.Sum256(encoded)
	phrase := fmt.Sprintf("CREATE WAREHOUSE DEVICE BATCH %d %x", len(items), digest[:8])
	if !preview && in.ConfirmationText != phrase {
		respondJSON(w, http.StatusPreconditionRequired, map[string]string{"error": "The exact batch-bound confirmation phrase from preview is required"})
		return
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
	var receipt int64
	if !preview {
		var replay json.RawMessage
		receipt, replay, err = beginWarehouseProductMutation(tx, r, "device.bulk_create", items)
		if err != nil {
			deviceMCPError(w, err)
			return
		}
		if replay != nil {
			respondJSON(w, 201, replay)
			return
		}
	}
	if _, err = tx.Exec(`LOCK TABLE devices IN SHARE ROW EXCLUSIVE MODE; LOCK TABLE products,inventory_identifiers,storage_zones,location_profiles,cases,product_locations IN SHARE MODE`); err != nil {
		deviceMCPError(w, err)
		return
	}
	for _, item := range items {
		if err = validateWarehouseDeviceReferences(tx, "", item.warehouseDeviceFields); err != nil {
			deviceMCPError(w, err)
			return
		}
	}
	for zone, count := range occupancy {
		// Bound hierarchy traversal before the shared capacity validator.
		if err = validateCaseMCPHierarchy(tx, zone); err != nil {
			respondJSON(w, 409, map[string]string{"error": err.Error()})
			return
		}
		var allowed bool
		if err = tx.QueryRow(`SELECT COALESCE(p.allow_devices,true) FROM storage_zones z LEFT JOIN location_profiles p ON p.profile_id=z.profile_id WHERE z.zone_id=$1`, zone).Scan(&allowed); err != nil || !allowed {
			respondJSON(w, 409, map[string]string{"error": "Destination does not allow devices"})
			return
		}
		if err = services.ValidateStorageDestination(tx, zone, float64(count)); err != nil {
			respondJSON(w, 409, map[string]string{"error": err.Error()})
			return
		}
	}
	if preview {
		respondJSON(w, 200, map[string]any{"operation_status": "confirmation_required", "preview": true, "ready": true, "devices": items, "count": len(items), "required_confirmation_text": phrase, "warnings": []string{"IDs and omitted scan codes are generated at commit. Every device is committed together; no labels or files are generated."}})
		return
	}
	results := make([]map[string]any, 0, len(items))
	for _, item := range items {
		physical, location := "location_unknown", "location_unknown"
		if item.ZoneID != nil {
			physical, location = "in_storage", "warehouse"
		}
		var id string
		err = tx.QueryRow(`INSERT INTO devices(productid,serialnumber,barcode,qr_code,condition_rating,usage_hours,purchasedate,lastmaintenance,nextmaintenance,notes,status,condition_status,current_location,zone_id) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14) RETURNING deviceid`, item.ProductID, item.SerialNumber, item.Barcode, item.QRCode, item.ConditionRating, item.UsageHours, item.PurchaseDate, item.LastMaintenance, item.NextMaintenance, item.Notes, physical, item.ConditionStatus, location, item.ZoneID).Scan(&id)
		if err != nil {
			deviceMCPError(w, err)
			return
		}
		after, version, lifecycle, physical, condition, zone, _, loadErr := loadDeviceForMCP(tx, id)
		if loadErr != nil {
			deviceMCPError(w, loadErr)
			return
		}
		if err = validateWarehouseDeviceReferences(tx, id, after); err != nil {
			deviceMCPError(w, err)
			return
		}
		newJSON, marshalErr := json.Marshal(map[string]any{"origin": "MCP/AI", "after": after, "updated_at": version, "lifecycle_status": lifecycle})
		if marshalErr != nil {
			deviceMCPError(w, marshalErr)
			return
		}
		var auditID int64
		err = tx.QueryRow(`INSERT INTO audit_log(user_id,action,entity_type,entity_id,old_values,new_values,user_agent) VALUES($1,'device.bulk_create','device',$2,'null'::jsonb,$3::jsonb,$4) RETURNING id`, user.UserID, id, string(newJSON), r.UserAgent()).Scan(&auditID)
		if err != nil {
			deviceMCPError(w, err)
			return
		}
		results = append(results, map[string]any{"device_id": id, "fields": after, "updated_at": version, "lifecycle_status": lifecycle, "physical_status": physical, "condition_status": condition, "zone_id": zone, "audit_id": auditID})
	}
	response := map[string]any{"operation_status": "created", "devices": results, "count": len(results)}
	if err = completeWarehouseProductMutation(tx, receipt, 201, response); err == nil {
		err = tx.Commit()
	}
	if err != nil {
		deviceMCPError(w, err)
		return
	}
	respondJSON(w, 201, response)
}
