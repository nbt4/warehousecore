package handlers

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"warehousecore/internal/repository"

	"github.com/gorilla/mux"
)

func CaseContentMCP(w http.ResponseWriter, r *http.Request) {
	op := mux.Vars(r)["operation"]
	user, _, err := warehouseScopedAdminActor(r, "cores:warehouse:update")
	if err != nil {
		respondWarehouseMutationError(w, err)
		return
	}
	var in warehouseCaseContentRequest
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10))
	dec.DisallowUnknownFields()
	if err = dec.Decode(&in); err != nil {
		respondJSON(w, 400, map[string]string{"error": "Invalid closed physical content payload"})
		return
	}
	if err = dec.Decode(&struct{}{}); err != io.EOF {
		respondJSON(w, 400, map[string]string{"error": "One JSON object required"})
		return
	}
	if err = validateCaseContentInput(op, in); err != nil {
		respondJSON(w, 400, map[string]string{"error": err.Error()})
		return
	}
	preview := in.Preview || !in.ConfirmChange
	tx, err := repository.GetSQLDB().BeginTx(r.Context(), nil)
	if err != nil {
		respondWarehouseMutationError(w, err)
		return
	}
	defer tx.Rollback()
	if _, err = tx.Exec(`SET LOCAL lock_timeout='5s';SET LOCAL statement_timeout='20s'`); err != nil {
		respondWarehouseMutationError(w, err)
		return
	}
	if err = lockProductImportActor(tx, user); err != nil {
		respondWarehouseMutationError(w, err)
		return
	}
	var receipt int64
	if !preview {
		var replay json.RawMessage
		receipt, replay, err = beginWarehouseProductMutation(tx, r, "case_content."+op, in)
		if err != nil {
			respondWarehouseMutationError(w, err)
			return
		}
		if replay != nil {
			respondJSON(w, 200, replay)
			return
		}
	}
	// Serialize all native physical writers, including their reference edits. The
	// receipt and fresh actor are locked before looking at an old saved response.
	if _, err = tx.Exec(`LOCK TABLE cases,devices,products,devicescases,case_product_contents,case_child_contents,product_locations,storage_zones,device_movements,case_events IN SHARE ROW EXCLUSIVE MODE;LOCK TABLE location_profiles,case_content_templates,warehouse_tasks,jobs,status,job_devices,job_positions,job_position_devices,job_packages,job_package_reservations,device_components,maintenance_orders,maintenance_plans,defect_reports IN SHARE MODE`); err != nil {
		respondWarehouseMutationError(w, err)
		return
	}
	refs, err := caseContentContext(tx, in)
	if err == sql.ErrNoRows {
		respondJSON(w, 404, map[string]string{"error": "Exact physical case or item not found"})
		return
	}
	if err != nil {
		respondWarehouseMutationError(w, err)
		return
	}
	draft, missing, err := caseContentProjection(tx, op, in, refs)
	if err == sql.ErrNoRows {
		respondJSON(w, 404, map[string]string{"error": "Exact storage location not found"})
		return
	}
	if err != nil {
		respondWarehouseMutationError(w, err)
		return
	}
	context := map[string]any{"references": refs, "draft": draft}
	raw, err := json.Marshal(context)
	if err != nil || len(raw) > 2*1024*1024-16384 {
		respondJSON(w, 400, map[string]string{"error": "Physical context exceeds two MiB; subdivide case or location"})
		return
	}
	fingerprint := inventoryMCPHash(context)
	root := refs["case"].(map[string]any)
	version := fmt.Sprint(root["updated_at"])
	if in.ExpectedUpdatedAt != "" && in.ExpectedUpdatedAt != version {
		missing = append(missing, "stale_case_version")
	}
	if in.ExpectedContext != "" && in.ExpectedContext != fingerprint {
		missing = append(missing, "stale_context")
	}
	if !preview {
		if in.ExpectedUpdatedAt == "" {
			missing = append(missing, "expected_updated_at")
		}
		if in.ExpectedContext == "" {
			missing = append(missing, "expected_context")
		}
	}
	phrase := fmt.Sprintf("%s WAREHOUSE CASE CONTENT %d %s", strings.ToUpper(op), in.CaseID, fingerprint[:16])
	result := map[string]any{"operation_status": "draft", "references": refs, "draft": draft, "expected_updated_at": version, "expected_context": fingerprint, "confirmation_text_required": phrase, "required_missing_fields": missing, "ready_to_execute": len(missing) == 0, "effects": map[string]any{"physical_stock_moves": true, "total_quantity_preserved": true, "job_reservations_preserved": true, "tasks_preserved": true, "templates_preserved": true}}
	if preview || len(missing) > 0 {
		respondJSON(w, 200, result)
		return
	}
	if in.ConfirmationText != phrase {
		respondJSON(w, 428, map[string]string{"error": "Exact context-bound physical movement confirmation required", "confirmation_text_required": phrase})
		return
	}
	if err = applyCaseContent(tx, r, op, in, refs, draft, int64(user.UserID)); err != nil {
		respondWarehouseMutationError(w, err)
		return
	}
	after, err := caseContentContext(tx, in)
	if err != nil {
		respondWarehouseMutationError(w, err)
		return
	}
	locationsAfter := []map[string]any{}
	for _, location := range draft["locations"].([]map[string]any) {
		zone := location["zone"].(map[string]any)
		stock, e := inventoryMCPStock(tx, caseContentID(zone["zone_id"]))
		if e != nil {
			respondWarehouseMutationError(w, e)
			return
		}
		locationsAfter = append(locationsAfter, map[string]any{"zone": zone, "stock": stock})
	}
	after["locations"] = locationsAfter
	audit, err := maintenanceMCPAudit(tx, r, "case_content."+op, "case", caseContentKey(in.CaseID), context, after, fmt.Sprint(after["case"].(map[string]any)["updated_at"]))
	if err != nil {
		respondWarehouseMutationError(w, err)
		return
	}
	result = map[string]any{"operation_status": "executed", "operation": op, "case": after["case"], "references": after, "audit_id": audit, "effects": result["effects"]}
	if err = completeWarehouseProductMutation(tx, receipt, 200, result); err == nil {
		err = tx.Commit()
	}
	if err != nil {
		respondWarehouseMutationError(w, err)
		return
	}
	respondJSON(w, 200, result)
}

func applyCaseContent(tx *sql.Tx, r *http.Request, op string, in warehouseCaseContentRequest, refs, draft map[string]any, userID int64) error {
	exec := func(q string, args ...any) error { _, err := tx.Exec(q, args...); return err }
	// Every event and movement is checked in the same transaction as membership,
	// locations, derived stock, case/device versions, audit and durable receipt.
	event := func(kind string, device any, product any, quantity any, zone any) error {
		return exec(`INSERT INTO case_events(case_id,event_type,device_id,product_id,quantity,zone_id,metadata) VALUES($1,$2,$3,$4,$5,$6,jsonb_build_object('origin','MCP/AI','child_case_id',$7::bigint,'actor_id',$8::bigint))`, in.CaseID, kind, device, product, quantity, zone, in.ChildCaseID, userID)
	}
	detachDevice := func(d map[string]any) error {
		key := fmt.Sprint(d["device_id"])
		if err := exec(`DELETE FROM devicescases WHERE caseid=$1 AND deviceid=$2`, in.CaseID, key); err != nil {
			return err
		}
		if err := exec(`UPDATE devices SET current_case_id=NULL,zone_id=$1,status='in_storage',current_location='warehouse' WHERE deviceid=$2`, in.DestinationZoneID, key); err != nil {
			return err
		}
		return exec(`INSERT INTO device_movements(device_id,from_zone_id,to_zone_id,from_case_id,moved_by,movement_type,reason,metadata) VALUES($1,$2,$3,$4,$5,'case_unpack','Confirmed physical case unpack',jsonb_build_object('origin','MCP/AI'))`, key, d["zone_id"], in.DestinationZoneID, in.CaseID, userID)
	}
	detachQuantity := func(product int64, q float64) error {
		var existing float64
		if err := tx.QueryRow(`SELECT quantity FROM case_product_contents WHERE case_id=$1 AND product_id=$2`, in.CaseID, product).Scan(&existing); err != nil {
			return err
		}
		if q == existing {
			if err := exec(`DELETE FROM case_product_contents WHERE case_id=$1 AND product_id=$2`, in.CaseID, product); err != nil {
				return err
			}
		} else {
			if err := exec(`UPDATE case_product_contents SET quantity=quantity-$1 WHERE case_id=$2 AND product_id=$3`, q, in.CaseID, product); err != nil {
				return err
			}
		}
		return exec(`INSERT INTO product_locations(product_id,zone_id,quantity) VALUES($1,$2,$3) ON CONFLICT(product_id,zone_id) DO UPDATE SET quantity=product_locations.quantity+EXCLUDED.quantity,updated_at=CURRENT_TIMESTAMP`, product, in.DestinationZoneID, q)
	}
	detachCase := func(child int64) error {
		if err := exec(`DELETE FROM case_child_contents WHERE parent_case_id=$1 AND child_case_id=$2`, in.CaseID, child); err != nil {
			return err
		}
		return exec(`UPDATE cases SET zone_id=$1 WHERE caseid=$2`, in.DestinationZoneID, child)
	}
	switch op {
	case "pack_device":
		d := refs["device"].(map[string]any)
		if err := exec(`INSERT INTO devicescases(caseid,deviceid) VALUES($1,$2)`, in.CaseID, in.DeviceID); err != nil {
			return err
		}
		if err := exec(`INSERT INTO device_movements(device_id,from_zone_id,to_case_id,moved_by,movement_type,reason,metadata) VALUES($1,$2,$3,$4,'case_pack','Confirmed physical case pack',jsonb_build_object('origin','MCP/AI'))`, in.DeviceID, d["zone_id"], in.CaseID, userID); err != nil {
			return err
		}
	case "pack_product":
		if err := exec(`UPDATE product_locations SET quantity=quantity-$1,updated_at=CURRENT_TIMESTAMP WHERE product_id=$2 AND zone_id=$3`, *in.Quantity, in.ProductID, in.SourceZoneID); err != nil {
			return err
		}
		if err := exec(`INSERT INTO case_product_contents(case_id,product_id,quantity,added_from_zone_id) VALUES($1,$2,$3,$4) ON CONFLICT(case_id,product_id) DO UPDATE SET quantity=case_product_contents.quantity+EXCLUDED.quantity`, in.CaseID, in.ProductID, *in.Quantity, in.SourceZoneID); err != nil {
			return err
		}
	case "pack_case":
		if err := exec(`INSERT INTO case_child_contents(parent_case_id,child_case_id) VALUES($1,$2)`, in.CaseID, in.ChildCaseID); err != nil {
			return err
		}
		if err := exec(`UPDATE cases SET zone_id=NULL WHERE caseid=$1`, in.ChildCaseID); err != nil {
			return err
		}
	case "unpack_device":
		if err := detachDevice(refs["device"].(map[string]any)); err != nil {
			return err
		}
	case "unpack_product":
		if err := detachQuantity(in.ProductID, *in.Quantity); err != nil {
			return err
		}
	case "unpack_case":
		if err := detachCase(in.ChildCaseID); err != nil {
			return err
		}
	case "unpack_all":
		for _, item := range draft["detached_contents"].([]map[string]any) {
			row := item["record"].(map[string]any)
			switch item["item_type"] {
			case "device":
				if err := detachDevice(row); err != nil {
					return err
				}
			case "product":
				if err := detachQuantity(caseContentID(row["product_id"]), item["quantity"].(float64)); err != nil {
					return err
				}
			case "case":
				if err := detachCase(caseContentID(row["case_id"])); err != nil {
					return err
				}
			}
		}
		if err := exec(`UPDATE cases SET zone_id=$1 WHERE caseid=$2`, in.DestinationZoneID, in.CaseID); err != nil {
			return err
		}
	}
	if err := exec(`UPDATE cases SET workflow_status=CASE WHEN EXISTS(SELECT 1 FROM devicescases WHERE caseid=$1) OR EXISTS(SELECT 1 FROM case_product_contents WHERE case_id=$1) OR EXISTS(SELECT 1 FROM case_child_contents WHERE parent_case_id=$1) THEN 'packing' ELSE 'empty' END WHERE caseid=$1`, in.CaseID); err != nil {
		return err
	}
	var device, product, zone any
	if in.DeviceID != "" {
		device = in.DeviceID
	}
	if in.ProductID > 0 {
		product = in.ProductID
	}
	if in.DestinationZoneID > 0 {
		zone = in.DestinationZoneID
	} else {
		zone = draft["source_zone_id"]
	}
	return event(op, device, product, in.Quantity, zone)
}
