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

func CaseWorkflowMCP(w http.ResponseWriter, r *http.Request) {
	op := mux.Vars(r)["operation"]
	user, _, err := warehouseScopedAdminActor(r, "cores:warehouse:update")
	if err != nil {
		respondWarehouseMutationError(w, err)
		return
	}
	var in warehouseCaseWorkflowRequest
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10))
	dec.DisallowUnknownFields()
	if err = dec.Decode(&in); err != nil {
		respondJSON(w, 400, map[string]string{"error": "Invalid closed case workflow payload"})
		return
	}
	if err = dec.Decode(&struct{}{}); err != io.EOF {
		respondJSON(w, 400, map[string]string{"error": "One JSON object required"})
		return
	}
	if err = validateCaseWorkflowInput(op, in); err != nil {
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
		receipt, replay, err = beginWarehouseProductMutation(tx, r, "case_workflow."+op, in)
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
	if _, err = tx.Exec(`LOCK TABLE cases,devices,products,devicescases,case_product_contents,case_child_contents,product_locations,storage_zones,device_movements,case_events,job_devices,job_history,jobs IN SHARE ROW EXCLUSIVE MODE;LOCK TABLE location_profiles,case_content_templates,warehouse_tasks,status,job_edit_sessions,job_positions,job_position_devices,job_packages,job_package_reservations,device_components,maintenance_orders,maintenance_plans,defect_reports IN SHARE MODE`); err != nil {
		respondWarehouseMutationError(w, err)
		return
	}
	refs, err := caseWorkflowContext(tx, in, int64(user.UserID))
	if err == sql.ErrNoRows {
		respondJSON(w, 404, map[string]string{"error": "Exact case or job not found"})
		return
	}
	if err != nil {
		respondWarehouseMutationError(w, err)
		return
	}
	draft, missing, err := caseWorkflowProjection(tx, op, in, refs, int64(user.UserID))
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
		respondJSON(w, 400, map[string]string{"error": "Case workflow context exceeds two MiB; subdivide case or location"})
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
	phrase := fmt.Sprintf("%s WAREHOUSE CASE WORKFLOW %d %s", strings.ToUpper(op), in.CaseID, fingerprint[:16])
	result := map[string]any{"operation_status": "draft", "references": refs, "draft": draft, "expected_updated_at": version, "expected_context": fingerprint, "confirmation_text_required": phrase, "required_missing_fields": missing, "ready_to_execute": len(missing) == 0, "effects": map[string]any{"whole_case_tree": true, "physical_contents_preserved": true, "total_quantity_preserved": true, "reservation_memberships_preserved": true, "tasks_preserved": true, "templates_preserved": true, "device_condition_preserved": true}}
	if preview || len(missing) > 0 {
		respondJSON(w, 200, result)
		return
	}
	if in.ConfirmationText != phrase {
		respondJSON(w, 428, map[string]string{"error": "Exact context-bound case workflow confirmation required", "confirmation_text_required": phrase})
		return
	}
	if err = applyCaseWorkflow(tx, r, op, in, refs, draft, int64(user.UserID)); err != nil {
		respondWarehouseMutationError(w, err)
		return
	}
	after, err := caseWorkflowContext(tx, in, int64(user.UserID))
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
	audit, err := maintenanceMCPAudit(tx, r, "case_workflow."+op, "case", caseContentKey(in.CaseID), context, after, fmt.Sprint(after["case"].(map[string]any)["updated_at"]))
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

// State, every movement/event, job history, audit and receipt share one commit.
// Memberships, quantities, task states and device condition are never rewritten.
func applyCaseWorkflow(tx *sql.Tx, r *http.Request, op string, in warehouseCaseWorkflowRequest, refs, draft map[string]any, userID int64) error {
	exec := func(q string, args ...any) error { _, err := tx.Exec(q, args...); return err }
	root := refs["case"].(map[string]any)
	jobID := caseContentID(draft["job_id"])
	job, _ := refs["target_job"].(map[string]any)
	for _, c := range caseContentRows(refs, "case_states") {
		id := caseContentID(c["case_id"])
		switch op {
		case "seal":
			if id == in.CaseID {
				if err := exec(`UPDATE cases SET workflow_status='sealed',sealed_at=CURRENT_TIMESTAMP WHERE caseid=$1`, id); err != nil {
					return err
				}
			}
		case "unseal":
			if id == in.CaseID {
				if err := exec(`UPDATE cases SET workflow_status=$2,sealed_at=NULL WHERE caseid=$1`, id, draft["case"].(map[string]any)["workflow_status"]); err != nil {
					return err
				}
			}
		case "move":
			if id == in.CaseID {
				if err := exec(`UPDATE cases SET zone_id=$2 WHERE caseid=$1`, id, in.DestinationZoneID); err != nil {
					return err
				}
			}
		case "dispatch":
			if err := exec(`UPDATE cases SET zone_id=NULL,status='rented',workflow_status='on_job',current_job_id=$2 WHERE caseid=$1`, id, jobID); err != nil {
				return err
			}
		case "return":
			var zone any
			if id == in.CaseID {
				zone = in.DestinationZoneID
			}
			if err := exec(`UPDATE cases SET zone_id=$2,status='free',current_job_id=NULL,workflow_status=CASE WHEN $3::text='sealed' AND sealed_at IS NOT NULL THEN 'sealed' ELSE 'return_check' END,sealed_at=CASE WHEN $3::text='sealed' THEN sealed_at ELSE NULL END WHERE caseid=$1`, id, zone, in.ReturnMode); err != nil {
				return err
			}
		case "inspect_return":
			if err := exec(`UPDATE cases SET sealed_at=NULL,workflow_status=CASE WHEN EXISTS(SELECT 1 FROM devicescases WHERE caseid=$1) OR EXISTS(SELECT 1 FROM case_product_contents WHERE case_id=$1) OR EXISTS(SELECT 1 FROM case_child_contents WHERE parent_case_id=$1) THEN 'packing' ELSE 'empty' END WHERE caseid=$1`, id); err != nil {
				return err
			}
		}
		var zone, eventJob any
		if in.DestinationZoneID > 0 {
			zone = in.DestinationZoneID
		} else {
			zone = root["zone_id"]
		}
		if jobID > 0 {
			eventJob = jobID
		}
		if err := exec(`INSERT INTO case_events(case_id,event_type,zone_id,job_id,metadata) VALUES($1,$2,$3,$4,jsonb_build_object('origin','MCP/AI','actor_id',$5::bigint,'outer_case_id',$6::bigint,'return_mode',$7::text,'accept_incomplete_template',$8::boolean,'inspection_passed',$9::boolean))`, id, op, zone, eventJob, userID, in.CaseID, in.ReturnMode, in.AcceptIncompleteTemplate, in.InspectionPassed); err != nil {
			return err
		}
	}
	for _, d := range caseContentRows(root, "packed_devices") {
		device := fmt.Sprint(d["device_id"])
		packedCase := caseContentID(d["packed_case_id"])
		switch op {
		case "dispatch":
			if err := exec(`UPDATE devices SET zone_id=NULL,status='on_job',current_location=$2 WHERE deviceid=$1`, device, "job:"+fmt.Sprint(job["job_code"])); err != nil {
				return err
			}
			if err := exec(`INSERT INTO job_devices(deviceid,jobid,pack_status,pack_ts) VALUES($1,$2,'issued',CURRENT_TIMESTAMP) ON CONFLICT(deviceid,jobid) DO UPDATE SET pack_status='issued',pack_ts=CURRENT_TIMESTAMP`, device, jobID); err != nil {
				return err
			}
		case "return":
			status := "return_pending"
			if in.ReturnMode == "sealed" {
				status = "in_storage"
			}
			if err := exec(`UPDATE devices SET zone_id=NULL,status=$2,current_location=$3 WHERE deviceid=$1`, device, status, fmt.Sprintf("case:%d", packedCase)); err != nil {
				return err
			}
			if err := exec(`UPDATE job_devices SET pack_status='returned',pack_ts=CURRENT_TIMESTAMP WHERE deviceid=$1 AND jobid=$2 AND pack_status='issued'`, device, jobID); err != nil {
				return err
			}
		case "inspect_return":
			if err := exec(`UPDATE devices SET status='in_storage',zone_id=NULL,current_location=$2 WHERE deviceid=$1`, device, fmt.Sprintf("case:%d", packedCase)); err != nil {
				return err
			}
		}
		if op == "move" || op == "dispatch" || op == "return" {
			var toZone any
			if in.DestinationZoneID > 0 {
				toZone = in.DestinationZoneID
			}
			if err := exec(`INSERT INTO device_movements(device_id,from_zone_id,to_zone_id,from_case_id,to_case_id,moved_by,movement_type,reason,metadata) VALUES($1,$2,$3,$4,$4,$5,$6,'Confirmed whole-case physical movement',jsonb_build_object('origin','MCP/AI','outer_case_id',$7::bigint,'job_id',$8::bigint))`, device, root["zone_id"], toZone, packedCase, userID, "case_"+op, in.CaseID, jobID); err != nil {
				return err
			}
		}
	}
	if op == "dispatch" || op == "return" {
		if err := exec(`INSERT INTO job_history(job_id,user_id,change_type,field_name,old_value,new_value,description,user_agent) VALUES($1,$2,'updated','case_workflow',$3,$4,'Confirmed MCP/AI whole-case workflow',$5)`, jobID, userID, fmt.Sprint(root["workflow_status"]), fmt.Sprintf("%s case:%d", op, in.CaseID), boundedUTF8(r.UserAgent(), 255)); err != nil {
			return err
		}
		if err := exec(`UPDATE jobs SET updated_at=CURRENT_TIMESTAMP WHERE jobid=$1`, jobID); err != nil {
			return err
		}
	}
	return nil
}

func boundedUTF8(s string, max int) string {
	runes := []rune(s)
	if len(runes) > max {
		return string(runes[:max])
	}
	return s
}
