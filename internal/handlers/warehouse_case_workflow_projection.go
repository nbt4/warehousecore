package handlers

import (
	"database/sql"
	"fmt"
	"sort"

	"warehousecore/internal/jobstatus"
)

func caseWorkflowProjection(tx *sql.Tx, op string, in warehouseCaseWorkflowRequest, refs map[string]any, userID int64) (map[string]any, []string, error) {
	root := refs["case"].(map[string]any)
	meta := root["metadata"].(map[string]any)
	missing := []string{}
	warnings := []string{}
	if root["nested"] == true {
		missing = append(missing, "unnest_outer_case_before_workflow")
	}
	if root["lifecycle_status"] != "active" {
		missing = append(missing, "active_case")
	}
	if op != "return" && root["current_job_id"] != nil {
		missing = append(missing, "return_issued_case_first")
	}
	open := map[string]bool{"empty": true, "packing": true, "complete": true, "return_check": true}
	if op == "seal" && (!open[fmt.Sprint(root["workflow_status"])] || meta["sealed_at"] != nil || root["status"] != "free") {
		missing = append(missing, "active_open_available_case")
	}
	if op == "unseal" && (meta["sealed_at"] == nil || !map[string]bool{"sealed": true, "staged": true}[fmt.Sprint(root["workflow_status"])] || root["status"] != "free") {
		missing = append(missing, "sealed_available_case")
	}
	if op == "move" && (root["workflow_status"] == "on_job" || root["status"] == "rented" || caseContentID(root["zone_id"]) == in.DestinationZoneID) {
		missing = append(missing, "changed_destination_for_unissued_case")
	}
	if op == "dispatch" && (root["workflow_status"] != "sealed" || meta["sealed_at"] == nil || root["status"] != "free") {
		missing = append(missing, "seal_case_before_dispatch")
	}
	if op == "return" && (root["workflow_status"] != "on_job" || root["status"] != "rented" || root["current_job_id"] == nil) {
		missing = append(missing, "issued_case_with_retained_job")
	}
	if op == "inspect_return" && (root["workflow_status"] != "return_check" || root["status"] != "free" || root["current_job_id"] != nil) {
		missing = append(missing, "returned_case_pending_inspection")
	}
	if op == "inspect_return" && !in.InspectionPassed {
		missing = append(missing, "explicit_whole_tree_inspection_passed")
	}
	if op == "return" && in.ReturnMode == "sealed" && meta["sealed_at"] == nil {
		missing = append(missing, "retained_outer_seal_required")
	}
	job, _ := refs["target_job"].(map[string]any)
	jobID := caseContentID(root["current_job_id"])
	if op == "dispatch" {
		jobID = in.JobID
	}
	if op == "dispatch" && (job == nil || job["deleted_at"] != nil || caseContentID(job["status_id"]) != jobstatus.ConfirmedID || job["startdate"] == nil || job["enddate"] == nil || job["valid_dates"] != true) {
		missing = append(missing, "confirmed_live_scheduled_job")
	}
	if (op == "dispatch" || op == "return") && len(caseContentRows(refs, "active_editors")) > 0 {
		missing = append(missing, "active_job_editor")
	}
	states := caseContentRows(refs, "case_states")
	ids := map[int64]bool{}
	for _, c := range states {
		ids[caseContentID(c["case_id"])] = true
		if c["lifecycle_status"] != "active" {
			missing = append(missing, "active_complete_case_tree")
		}
		if (op == "seal" || op == "dispatch" || op == "inspect_return") && (c["status"] != "free" || !map[string]bool{"empty": true, "packing": true, "complete": true, "return_check": true, "sealed": true, "staged": true}[fmt.Sprint(c["workflow_status"])]) {
			missing = append(missing, "operational_complete_case_tree")
		}
		if op == "return" {
			if caseContentID(c["current_job_id"]) != jobID || c["workflow_status"] != "on_job" || c["status"] != "rented" {
				missing = append(missing, "whole_tree_issued_to_same_job")
			}
		} else if c["current_job_id"] != nil || c["workflow_status"] == "on_job" || c["status"] == "rented" {
			missing = append(missing, "unissued_complete_case_tree")
		}
	}
	devices := caseContentRows(root, "packed_devices")
	for _, d := range devices {
		deps, _ := d["dependencies"].(map[string]int64)
		membership := deps["cases"]
		expected := int64(1)
		if d["current_case_id"] != nil {
			expected++
		}
		if d["current_case_id"] == nil || membership != expected || (d["current_case_id"] != nil && caseContentID(d["current_case_id"]) != caseContentID(d["packed_case_id"])) {
			missing = append(missing, "consistent_device_case_membership")
		}
		if d["lifecycle_status"] != "active" || d["product_lifecycle"] != "active" {
			missing = append(missing, "active_physical_devices_and_products")
		}
		if op == "seal" || op == "dispatch" || op == "return" && in.ReturnMode == "sealed" || op == "inspect_return" {
			if d["condition_status"] != "available" || deps["maintenance_orders"] > 0 || deps["defects"] > 0 {
				missing = append(missing, "operational_devices_without_unresolved_work")
			}
		}
		if op == "return" {
			if d["status"] != "on_job" && d["status"] != "return_pending" {
				missing = append(missing, "issued_or_pending_return_devices")
			}
		} else if op == "inspect_return" {
			if d["status"] != "return_pending" && d["status"] != "in_storage" {
				missing = append(missing, "pending_or_completed_return_inspection_device")
			}
		} else if op == "move" || op == "unseal" {
			if d["status"] != "return_pending" && d["status"] != "in_storage" {
				missing = append(missing, "unissued_physical_devices")
			}
		} else if d["status"] != "in_storage" {
			missing = append(missing, "stored_physical_devices")
		}
	}
	if op == "dispatch" || op == "move" {
		selected := map[string]bool{}
		for _, d := range devices {
			selected[fmt.Sprint(d["device_id"])] = true
		}
		for _, edge := range caseContentRows(refs, "component_memberships") {
			if !selected[fmt.Sprint(edge["device_id"])] || !selected[fmt.Sprint(edge["component_device_id"])] {
				missing = append(missing, "whole_physical_component_assembly")
			}
		}
	}
	for _, p := range caseContentRows(root, "product_contents") {
		if p["lifecycle_status"] != "active" {
			missing = append(missing, "active_packed_quantity_products")
		}
	}
	incomplete := []map[string]any{}
	stateByID := map[int64]map[string]any{}
	for _, c := range states {
		stateByID[caseContentID(c["case_id"])] = c
	}
	for _, line := range caseContentRows(refs, "active_templates") {
		c := stateByID[caseContentID(line["case_id"])]
		if line["product_lifecycle"] != "active" {
			missing = append(missing, "active_template_products")
		}
		expected, _ := line["expected_quantity"].(float64)
		actual, _ := line["actual_quantity"].(float64)
		if c != nil && (c["case_type"] == "fixed" || c["case_type"] == "hybrid") && actual < expected {
			incomplete = append(incomplete, line)
		}
	}
	if op == "seal" && len(incomplete) > 0 {
		if !in.AcceptIncompleteTemplate {
			missing = append(missing, "explicit_incomplete_template_consent")
		} else {
			warnings = append(warnings, "Expected contents are incomplete. This seal records the explicitly reviewed exception; physical quantity is unchanged.")
		}
	}
	for _, task := range caseContentRows(refs, "open_tasks") {
		compatible := false
		switch op {
		case "inspect_return":
			compatible = task["task_type"] == "inspect" || task["task_type"] == "return"
		case "seal", "unseal":
			compatible = task["task_type"] == "pack" || op == "unseal" && (task["task_type"] == "inspect" || task["task_type"] == "return")
		case "dispatch":
			compatible = (task["task_type"] == "pick" || task["task_type"] == "pack") && (task["job_id"] == nil || caseContentID(task["job_id"]) == jobID)
		case "return":
			compatible = (task["task_type"] == "return" || task["task_type"] == "inspect") && (task["job_id"] == nil || caseContentID(task["job_id"]) == jobID)
		case "move":
			compatible = task["task_type"] == "move" && (task["to_zone_id"] == nil || caseContentID(task["to_zone_id"]) == in.DestinationZoneID) && (task["from_zone_id"] == nil || caseContentID(task["from_zone_id"]) == caseContentID(root["zone_id"]))
		}
		if !compatible || task["status"] == "in_progress" && task["assigned_to"] != nil && caseContentID(task["assigned_to"]) != userID {
			missing = append(missing, "resolve_conflicting_case_work_task")
		}
	}
	// Inspect every subtree limit without double counting a packed device or case.
	weights := []map[string]any{}
	for _, c := range states {
		descendants := map[int64]bool{caseContentID(c["case_id"]): true}
		changed := true
		for changed {
			changed = false
			for _, edge := range caseContentRows(root, "nesting") {
				parent, child := caseContentID(edge["parent_case_id"]), caseContentID(edge["child_case_id"])
				if descendants[parent] && !descendants[child] && ids[child] {
					descendants[child] = true
					changed = true
				}
			}
		}
		subtree := caseContentCopy(root)
		nodes := []map[string]any{}
		ds := []map[string]any{}
		ps := []map[string]any{}
		for _, node := range states {
			if descendants[caseContentID(node["case_id"])] {
				nodes = append(nodes, node)
			}
		}
		for _, d := range devices {
			if descendants[caseContentID(d["packed_case_id"])] {
				ds = append(ds, d)
			}
		}
		for _, p := range caseContentRows(root, "product_contents") {
			if descendants[caseContentID(p["case_id"])] {
				ps = append(ps, p)
			}
		}
		subtree["case_tree"] = nodes
		subtree["packed_devices"] = ds
		subtree["product_contents"] = ps
		limit := map[string]any{"capacity_mode": "item_count", "allow_cases": true, "allow_mixed_products": true}
		if op == "seal" || op == "dispatch" || op == "move" {
			limit["max_weight_kg"] = c["max_weight_kg"]
		}
		summary, required := inventoryMCPCapacity([]map[string]any{{"item_type": "case", "item_key": "subtree", "counted_quantity": float64(1)}}, map[string]any{"case:subtree": subtree}, limit)
		missing = append(missing, required...)
		weights = append(weights, map[string]any{"case_id": c["case_id"], "gross_weight_kg": summary["weight_kg"], "max_weight_kg": c["max_weight_kg"]})
	}
	if op == "dispatch" {
		conflicts, err := inventoryMCPRows(tx, `WITH RECURSIVE tree(case_id) AS(SELECT $1::int UNION SELECT cc.child_case_id FROM case_child_contents cc JOIN tree t ON cc.parent_case_id=t.case_id), selected_devices AS(SELECT deviceid FROM devicescases WHERE caseid IN(SELECT case_id FROM tree)), target AS(SELECT startdate,enddate FROM jobs WHERE jobid=$2), related AS(
 SELECT jd.deviceid AS device_id,jd.jobid AS job_id,jd.pack_status='issued' AS issued FROM job_devices jd JOIN selected_devices d ON d.deviceid=jd.deviceid
 UNION SELECT pd.device_id,jp.job_id,false FROM job_position_devices pd JOIN selected_devices d ON d.deviceid=pd.device_id JOIN job_positions jp ON jp.position_id=pd.position_id
 UNION SELECT pr.device_id,jp.job_id,false FROM job_package_reservations pr JOIN selected_devices d ON d.deviceid=pr.device_id JOIN job_packages jp ON jp.job_package_id=pr.job_package_id WHERE pr.reservation_status<>'released'
 ) SELECT DISTINCT jsonb_build_object('device_id',r.device_id,'job_id',j.jobid,'job_code',j.job_code,'issued',r.issued,'startdate',j.startdate,'enddate',j.enddate,'updated_at',j.updated_at) FROM related r JOIN jobs j ON j.jobid=r.job_id LEFT JOIN status s ON s.statusid=j.statusid CROSS JOIN target t WHERE j.jobid<>$2 AND (r.issued OR j.deleted_at IS NULL AND NOT warehouse_job_status_is_closed(COALESCE(s.status,'')) AND COALESCE(j.startdate,'-infinity'::date)<=COALESCE(t.enddate,'infinity'::date) AND COALESCE(j.enddate,'infinity'::date)>=COALESCE(t.startdate,'-infinity'::date)) LIMIT 1001`, in.CaseID, in.JobID)
		if err != nil {
			return nil, nil, err
		}
		refs["schedule_conflicts"] = conflicts
		if len(conflicts) > 0 {
			missing = append(missing, "device_schedule_or_other_issued_job_conflicts")
		}
	}
	if op != "return" {
		for _, a := range caseContentRows(refs, "device_assignments") {
			if a["kind"] == "job_device" && a["state"].(map[string]any)["pack_status"] == "issued" {
				missing = append(missing, "return_issued_equipment_first")
			}
		}
	}
	if op == "return" {
		issued := map[string]bool{}
		for _, a := range caseContentRows(refs, "device_assignments") {
			if a["kind"] == "job_device" && caseContentID(a["job_id"]) == jobID && a["state"].(map[string]any)["pack_status"] == "issued" {
				issued[fmt.Sprint(a["device_id"])] = true
			}
		}
		for _, d := range devices {
			if !issued[fmt.Sprint(d["device_id"])] {
				missing = append(missing, "retained_issued_device_assignment_for_current_job")
			}
		}
	}
	if op == "return" {
		for _, a := range caseContentRows(refs, "device_assignments") {
			if a["kind"] != "job_device" {
				continue
			}
			state := a["state"].(map[string]any)
			if state["pack_status"] == "issued" && caseContentID(a["job_id"]) != jobID {
				missing = append(missing, "device_issued_to_another_job")
			}
		}
	}
	source := caseContentID(root["zone_id"])
	destination := source
	if op == "move" || op == "return" {
		destination = in.DestinationZoneID
	}
	if op == "dispatch" {
		destination = 0
	}
	if op == "seal" || op == "dispatch" {
		if source <= 0 {
			missing = append(missing, "located_outer_case")
		}
	}
	// Opening changes no storage or usable condition and remains possible when a
	// location is blocked. Every actual outbound/inbound move rechecks both sides.
	locations := []map[string]any{}
	if op != "unseal" {
		var err error
		var required []string
		locations, required, err = caseWorkflowLocations(tx, root, source, destination, op == "move" || op == "return" || op == "dispatch")
		if err != nil {
			return nil, nil, err
		}
		missing = append(missing, required...)
	}
	if op == "inspect_return" || op == "return" && in.ReturnMode == "inspect" {
		for _, loc := range locations {
			zone := loc["zone"].(map[string]any)
			if caseContentID(zone["zone_id"]) == destination && !map[string]bool{"return": true, "inspection": true, "quarantine": true}[fmt.Sprint(zone["process_role"])] {
				missing = append(missing, "return_inspection_or_quarantine_destination")
			}
		}
	}
	after := caseContentCopy(meta)
	switch op {
	case "seal":
		after["workflow_status"] = "sealed"
		after["sealed_at"] = "transaction_timestamp"
	case "unseal":
		after["workflow_status"] = "empty"
		if len(devices) > 0 || len(caseContentRows(root, "product_contents")) > 0 || len(states) > 1 {
			after["workflow_status"] = "packing"
		}
		for _, d := range devices {
			if d["status"] == "return_pending" {
				after["workflow_status"] = "return_check"
			}
		}
		after["sealed_at"] = nil
	case "inspect_return":
		after["workflow_status"] = "empty"
		if len(devices) > 0 || len(caseContentRows(root, "product_contents")) > 0 || len(states) > 1 {
			after["workflow_status"] = "packing"
		}
		after["sealed_at"] = nil
	case "move":
		after["zone_id"] = destination
	case "dispatch":
		after["zone_id"] = nil
		after["status"] = "rented"
		after["workflow_status"] = "on_job"
		after["current_job_id"] = jobID
	case "return":
		after["zone_id"] = destination
		after["status"] = "free"
		after["current_job_id"] = nil
		after["workflow_status"] = "return_check"
		if in.ReturnMode == "sealed" {
			after["workflow_status"] = "sealed"
		} else {
			after["sealed_at"] = nil
		}
	}
	return map[string]any{"operation": op, "case": after, "job_id": jobID, "return_mode": in.ReturnMode, "accept_incomplete_template": in.AcceptIncompleteTemplate, "inspection_passed": in.InspectionPassed, "incomplete_templates": incomplete, "subtree_weights": weights, "locations": locations, "warnings": warnings, "physical_contents_preserved": true, "total_quantity_preserved": true, "task_status_preserved": true}, missing, nil
}

func caseWorkflowLocations(tx *sql.Tx, root map[string]any, source, destination int64, moving bool) ([]map[string]any, []string, error) {
	ids := []int64{}
	if source > 0 {
		ids = append(ids, source)
	}
	if destination > 0 && destination != source {
		ids = append(ids, destination)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	contexts := []map[string]any{}
	missing := []string{}
	for _, id := range ids {
		zone, err := inventoryMCPJSON(tx, inventoryMCPZoneSelect, id)
		if err != nil {
			return nil, nil, err
		}
		var role string
		if err = tx.QueryRow(`SELECT process_role FROM storage_zones WHERE zone_id=$1`, id).Scan(&role); err != nil {
			return nil, nil, err
		}
		zone["process_role"] = role
		if zone["is_storable"] != true {
			missing = append(missing, "storable_storage_location")
		}
		ancestors := []map[string]any{}
		parent := id
		seen := map[int64]bool{}
		for parent > 0 {
			if seen[parent] || len(seen) >= 64 {
				missing = append(missing, "acyclic_bounded_storage_hierarchy")
				break
			}
			seen[parent] = true
			a, e := inventoryMCPJSON(tx, `SELECT jsonb_build_object('zone_id',zone_id,'parent_zone_id',parent_zone_id,'is_active',is_active,'operational_status',operational_status,'updated_at',updated_at) FROM storage_zones WHERE zone_id=$1`, parent)
			if e == sql.ErrNoRows {
				missing = append(missing, "complete_storage_hierarchy")
				break
			}
			if e != nil {
				return nil, nil, e
			}
			ancestors = append(ancestors, a)
			if a["is_active"] != true || a["operational_status"] != "available" {
				missing = append(missing, "available_storage_and_ancestors")
			}
			parent = caseContentID(a["parent_zone_id"])
		}
		stock, err := inventoryMCPStock(tx, id)
		if err != nil {
			return nil, nil, err
		}
		lines := []map[string]any{}
		stockRefs := map[string]any{}
		found := false
		key := fmt.Sprint(root["case_id"])
		for _, s := range stock {
			kind, item := fmt.Sprint(s["item_type"]), fmt.Sprint(s["item_key"])
			q := s["expected_quantity"]
			if kind == "case" && item == key {
				found = true
				if moving && id == source && source != destination {
					q = float64(0)
				}
			}
			lines = append(lines, map[string]any{"item_type": kind, "item_key": item, "counted_quantity": q})
			stockRefs[kind+":"+item] = s["record"]
		}
		if id == destination && moving && !found {
			lines = append(lines, map[string]any{"item_type": "case", "item_key": key, "counted_quantity": float64(1)})
			stockRefs["case:"+key] = root
		}
		if id == source && source > 0 && !found {
			missing = append(missing, "consistent_outer_case_storage")
		}
		capacity, required := inventoryMCPCapacity(lines, stockRefs, zone)
		missing = append(missing, required...)
		contexts = append(contexts, map[string]any{"zone": zone, "ancestors": ancestors, "stock_before": stock, "projected_capacity": capacity})
	}
	return contexts, missing, nil
}
