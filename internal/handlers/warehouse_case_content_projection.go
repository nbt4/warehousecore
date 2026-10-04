package handlers

import (
	"database/sql"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

func caseContentCopy(row map[string]any) map[string]any {
	out := map[string]any{}
	for k, v := range row {
		out[k] = v
	}
	return out
}
func caseContentRows(row map[string]any, key string) []map[string]any {
	v, _ := row[key].([]map[string]any)
	return v
}
func caseContentID(v any) int64 { return maintenanceMCPID(v) }

// Reservation and picklist references are retained, not cancelled by physical
// packing. Issued equipment, defects, components and maintenance still block it.
func caseContentDeviceBlocked(d map[string]any) bool {
	if d["lifecycle_status"] != "active" || d["product_lifecycle"] != "active" || d["condition_status"] != "available" || d["status"] != "in_storage" {
		return true
	}
	deps, _ := d["dependencies"].(map[string]int64)
	return deps["components"] > 0 || deps["maintenance_orders"] > 0 || deps["defects"] > 0
}
func caseContentMembershipInvalid(d map[string]any) bool {
	deps, _ := d["dependencies"].(map[string]int64)
	expected := int64(1)
	if d["current_case_id"] != nil {
		expected++
	}
	return deps["cases"] != expected || d["current_case_id"] != nil && caseContentID(d["current_case_id"]) != caseContentID(d["packed_case_id"])
}

func caseContentTreeBlocked(row map[string]any) bool {
	for _, c := range caseContentRows(row, "case_tree") {
		if c["lifecycle_status"] != "active" || c["status"] != "free" || c["current_job_id"] != nil || c["workflow_status"] == "on_job" || c["workflow_status"] == "maintenance" {
			return true
		}
	}
	for _, d := range caseContentRows(row, "packed_devices") {
		if caseContentDeviceBlocked(d) || caseContentMembershipInvalid(d) {
			return true
		}
	}
	for _, p := range caseContentRows(row, "product_contents") {
		if p["lifecycle_status"] != "active" {
			return true
		}
	}
	return false
}

func caseContentProjection(tx *sql.Tx, op string, in warehouseCaseContentRequest, refs map[string]any) (map[string]any, []string, error) {
	root := refs["case"].(map[string]any)
	meta := root["metadata"].(map[string]any)
	missing := []string{}
	if meta["lifecycle_status"] != "active" || meta["status"] != "free" || meta["current_job_id"] != nil || meta["sealed_at"] != nil || !map[string]bool{"empty": true, "packing": true, "complete": true, "return_check": true}[fmt.Sprint(meta["workflow_status"])] {
		missing = append(missing, "active_open_available_case")
	}
	if root["nested"] == true {
		missing = append(missing, "unnest_parent_before_content_change")
	}
	if caseContentTreeBlocked(root) {
		missing = append(missing, "available_physical_case_tree")
	}
	projected := caseContentCopy(root)
	devices := append([]map[string]any{}, caseContentRows(root, "packed_devices")...)
	products := append([]map[string]any{}, caseContentRows(root, "product_contents")...)
	tree := append([]map[string]any{}, caseContentRows(root, "case_tree")...)
	// Every direct item detached by unpack_all is retained with its whole subtree.
	moved := []map[string]any{}
	addMoved := func(kind, key string, q float64, row map[string]any) {
		moved = append(moved, map[string]any{"item_type": kind, "item_key": key, "quantity": q, "record": row})
	}
	selectedDevice, _ := refs["device"].(map[string]any)
	selectedProduct, _ := refs["product"].(map[string]any)
	child, _ := refs["child_case"].(map[string]any)
	packedDevice := false
	for _, d := range devices {
		if d["device_id"] == in.DeviceID && caseContentID(d["packed_case_id"]) == in.CaseID {
			packedDevice = true
		}
	}
	packedQty := 0.0
	for _, p := range products {
		if caseContentID(p["case_id"]) == in.CaseID && caseContentID(p["product_id"]) == in.ProductID {
			packedQty, _ = p["quantity"].(float64)
		}
	}
	childDirect := false
	for _, n := range caseContentRows(root, "nesting") {
		if caseContentID(n["parent_case_id"]) == in.CaseID && caseContentID(n["child_case_id"]) == in.ChildCaseID {
			childDirect = true
		}
	}
	filterChild := func(c map[string]any) {
		ids := map[int64]bool{}
		for _, n := range caseContentRows(c, "case_tree") {
			ids[caseContentID(n["case_id"])] = true
		}
		nextTree := []map[string]any{}
		for _, n := range tree {
			if !ids[caseContentID(n["case_id"])] {
				nextTree = append(nextTree, n)
			}
		}
		tree = nextTree
		nextDevices := []map[string]any{}
		for _, n := range devices {
			if !ids[caseContentID(n["packed_case_id"])] {
				nextDevices = append(nextDevices, n)
			}
		}
		devices = nextDevices
		nextProducts := []map[string]any{}
		for _, n := range products {
			if !ids[caseContentID(n["case_id"])] {
				nextProducts = append(nextProducts, n)
			}
		}
		products = nextProducts
	}
	switch op {
	case "pack_device":
		if caseContentDeviceBlocked(selectedDevice) || selectedDevice["current_case_id"] != nil || caseContentID(selectedDevice["zone_id"]) == 0 || caseContentID(selectedDevice["dependencies"].(map[string]int64)["cases"]) > 0 {
			missing = append(missing, "available_unpacked_located_device")
		}
		d := caseContentCopy(selectedDevice)
		d["packed_case_id"] = float64(in.CaseID)
		devices = append(devices, d)
	case "unpack_device":
		if !packedDevice {
			missing = append(missing, "device_is_direct_case_content")
		}
		next := []map[string]any{}
		for _, d := range devices {
			if d["device_id"] != in.DeviceID || caseContentID(d["packed_case_id"]) != in.CaseID {
				next = append(next, d)
			}
		}
		devices = next
		addMoved("device", in.DeviceID, 1, selectedDevice)
	case "pack_product", "unpack_product":
		if selectedProduct["lifecycle_status"] != "active" || selectedProduct["tracking_mode"] != "quantity" {
			missing = append(missing, "active_quantity_product")
		}
		q := *in.Quantity
		if op == "pack_product" {
			available, _ := selectedProduct["quantity_in_zone"].(float64)
			if q > available {
				missing = append(missing, "sufficient_source_quantity")
			}
			packedQty += q
		} else {
			if q > packedQty {
				missing = append(missing, "sufficient_direct_packed_quantity")
			}
			packedQty -= q
			addMoved("product", fmt.Sprint(in.ProductID), q, selectedProduct)
		}
		if packedQty > 999999999.999 {
			missing = append(missing, "bounded_packed_quantity")
		}
		next := []map[string]any{}
		for _, p := range products {
			if caseContentID(p["case_id"]) != in.CaseID || caseContentID(p["product_id"]) != in.ProductID {
				next = append(next, p)
			}
		}
		products = next
		if packedQty > 0 {
			p := caseContentCopy(selectedProduct)
			p["case_id"] = float64(in.CaseID)
			p["quantity"] = packedQty
			products = append(products, p)
		}
	case "pack_case":
		if in.ChildCaseID == in.CaseID || child["nested"] == true || caseContentID(child["zone_id"]) == 0 || caseContentTreeBlocked(child) {
			missing = append(missing, "available_unnested_located_child_tree")
		}
		for _, c := range caseContentRows(child, "case_tree") {
			if caseContentID(c["case_id"]) == in.CaseID {
				missing = append(missing, "acyclic_case_tree")
			}
		}
		tree = append(tree, caseContentRows(child, "case_tree")...)
		devices = append(devices, caseContentRows(child, "packed_devices")...)
		products = append(products, caseContentRows(child, "product_contents")...)
	case "unpack_case":
		if !childDirect {
			missing = append(missing, "child_is_direct_case_content")
		}
		filterChild(child)
		addMoved("case", fmt.Sprint(in.ChildCaseID), 1, child)
	case "unpack_all":
		for _, d := range devices {
			if caseContentID(d["packed_case_id"]) == in.CaseID {
				addMoved("device", fmt.Sprint(d["device_id"]), 1, d)
			}
		}
		for _, p := range products {
			if caseContentID(p["case_id"]) == in.CaseID {
				full, e := inventoryMCPItem(tx, "product", fmt.Sprint(p["product_id"]), 0)
				if e != nil {
					return nil, nil, e
				}
				addMoved("product", fmt.Sprint(p["product_id"]), p["quantity"].(float64), full)
			}
		}
		for _, n := range caseContentRows(root, "nesting") {
			if caseContentID(n["parent_case_id"]) == in.CaseID {
				full, e := inventoryMCPItem(tx, "case", fmt.Sprint(n["child_case_id"]), 0)
				if e != nil {
					return nil, nil, e
				}
				addMoved("case", fmt.Sprint(n["child_case_id"]), 1, full)
			}
		}
		if len(moved) == 0 {
			missing = append(missing, "nonempty_case")
		}
		devices = []map[string]any{}
		products = []map[string]any{}
		tree = []map[string]any{tree[0]}
		for _, n := range caseContentRows(root, "case_tree") {
			if caseContentID(n["case_id"]) == in.CaseID {
				tree = []map[string]any{n}
				break
			}
		}
	}
	projected["packed_devices"] = devices
	projected["product_contents"] = products
	projected["case_tree"] = tree
	// Case gross weight counts all empty cases plus physical contents exactly once.
	weightSummary, weightMissing := inventoryMCPCapacity([]map[string]any{{"item_type": "case", "item_key": fmt.Sprint(in.CaseID), "counted_quantity": float64(1)}}, map[string]any{"case:" + fmt.Sprint(in.CaseID): projected}, map[string]any{"capacity_mode": "item_count", "allow_cases": true, "allow_mixed_products": true, "max_weight_kg": meta["max_weight_kg"]})
	missing = append(missing, weightMissing...)
	zones := map[int64]bool{}
	rootZone := caseContentID(root["zone_id"])
	if rootZone > 0 {
		zones[rootZone] = true
	}
	if strings.HasPrefix(op, "pack_") && rootZone <= 0 {
		missing = append(missing, "located_parent_case")
	}
	sourceZone := in.SourceZoneID
	if op == "pack_device" {
		sourceZone = caseContentID(selectedDevice["zone_id"])
	}
	if op == "pack_case" {
		sourceZone = caseContentID(child["zone_id"])
	}
	if sourceZone > 0 {
		zones[sourceZone] = true
	}
	if in.DestinationZoneID > 0 {
		zones[in.DestinationZoneID] = true
	}
	zoneIDs := []int64{}
	for id := range zones {
		zoneIDs = append(zoneIDs, id)
	}
	sort.Slice(zoneIDs, func(i, j int) bool { return zoneIDs[i] < zoneIDs[j] })
	zoneContexts := []map[string]any{}
	for _, id := range zoneIDs {
		zone, e := inventoryMCPJSON(tx, inventoryMCPZoneSelect, id)
		if e != nil {
			return nil, nil, e
		}
		ancestors := []map[string]any{}
		seen := map[int64]bool{}
		parent := id
		for parent > 0 {
			if seen[parent] || len(seen) >= 64 {
				missing = append(missing, "acyclic_bounded_location_hierarchy")
				break
			}
			seen[parent] = true
			a, e := inventoryMCPJSON(tx, `SELECT jsonb_build_object('zone_id',zone_id,'parent_zone_id',parent_zone_id,'is_active',is_active,'operational_status',operational_status,'updated_at',updated_at) FROM storage_zones WHERE zone_id=$1`, parent)
			if e == sql.ErrNoRows {
				missing = append(missing, "complete_location_hierarchy")
				break
			}
			if e != nil {
				return nil, nil, e
			}
			ancestors = append(ancestors, a)
			if a["is_active"] != true || a["operational_status"] != "available" {
				missing = append(missing, "available_location_and_ancestors")
			}
			parent = caseContentID(a["parent_zone_id"])
		}
		if zone["is_storable"] != true {
			missing = append(missing, "storable_location")
		}
		stock, e := inventoryMCPStock(tx, id)
		if e != nil {
			return nil, nil, e
		}
		entries := map[string]map[string]any{}
		stockRefs := map[string]any{}
		for _, s := range stock {
			key := fmt.Sprint(s["item_type"]) + ":" + fmt.Sprint(s["item_key"])
			entries[key] = map[string]any{"item_type": s["item_type"], "item_key": s["item_key"], "counted_quantity": s["expected_quantity"]}
			stockRefs[key] = s["record"]
		}
		change := func(kind, key string, delta float64, row map[string]any) {
			k := kind + ":" + key
			q := 0.0
			if prev := entries[k]; prev != nil {
				q, _ = prev["counted_quantity"].(float64)
			}
			entries[k] = map[string]any{"item_type": kind, "item_key": key, "counted_quantity": q + delta}
			stockRefs[k] = row
		}
		if id == rootZone {
			stockRefs["case:"+fmt.Sprint(in.CaseID)] = projected
		}
		if id == sourceZone {
			switch op {
			case "pack_device":
				change("device", in.DeviceID, -1, selectedDevice)
			case "pack_product":
				change("product", fmt.Sprint(in.ProductID), -*in.Quantity, selectedProduct)
			case "pack_case":
				change("case", fmt.Sprint(in.ChildCaseID), -1, child)
			}
		}
		if op == "unpack_all" && id == rootZone && id != in.DestinationZoneID {
			change("case", fmt.Sprint(in.CaseID), -1, projected)
		}
		if id == in.DestinationZoneID {
			for _, m := range moved {
				change(fmt.Sprint(m["item_type"]), fmt.Sprint(m["item_key"]), m["quantity"].(float64), m["record"].(map[string]any))
			}
			if op == "unpack_all" && id != rootZone {
				change("case", fmt.Sprint(in.CaseID), 1, projected)
			}
		}
		keys := []string{}
		for k := range entries {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		lines := []map[string]any{}
		for _, k := range keys {
			line := entries[k]
			if line["item_type"] == "product" && line["counted_quantity"].(float64) > 9999999.999 {
				missing = append(missing, "bounded_location_quantity")
			}
			if line["counted_quantity"].(float64) < 0 {
				missing = append(missing, "consistent_source_stock")
			}
			lines = append(lines, line)
		}
		capacity, required := inventoryMCPCapacity(lines, stockRefs, zone)
		missing = append(missing, required...)
		zoneContexts = append(zoneContexts, map[string]any{"zone": zone, "ancestors": ancestors, "stock_before": stock, "projected_capacity": capacity})
	}
	// Curated task versions bind scheduling changes, while compatible pack work
	// remains open. No task is silently completed by these physical actions.
	tasks, e := inventoryMCPRows(tx, `WITH RECURSIVE tree(case_id) AS(SELECT $1::int UNION SELECT $3::int WHERE $3>0 UNION SELECT cc.child_case_id FROM tree t JOIN case_child_contents cc ON cc.parent_case_id=t.case_id) SELECT jsonb_build_object('task_id',task_id,'task_type',task_type,'status',status,'case_id',case_id,'device_id',device_id,'product_id',product_id,'quantity',quantity,'from_zone_id',from_zone_id,'to_zone_id',to_zone_id,'job_id',job_id,'assigned_to',assigned_to,'updated_at',updated_at) FROM warehouse_tasks WHERE status IN('open','in_progress') AND (case_id IN(SELECT case_id FROM tree) OR device_id=$2 OR device_id IN(SELECT dc.deviceid FROM devicescases dc JOIN tree t ON dc.caseid=t.case_id) OR ($4>0 AND product_id=$4)) ORDER BY task_id LIMIT 1001`, in.CaseID, in.DeviceID, in.ChildCaseID, in.ProductID)
	if e != nil {
		return nil, nil, e
	}
	for _, task := range tasks {
		if task["task_type"] != "pack" {
			missing = append(missing, "resolve_conflicting_physical_task")
		}
	}
	var issued bool
	e = tx.QueryRow(`WITH RECURSIVE tree(case_id) AS(SELECT $1::int UNION SELECT $3::int WHERE $3>0 UNION SELECT cc.child_case_id FROM tree t JOIN case_child_contents cc ON cc.parent_case_id=t.case_id) SELECT EXISTS(SELECT 1 FROM job_devices jd WHERE pack_status='issued' AND (deviceid=$2 OR deviceid IN(SELECT deviceid FROM devicescases WHERE caseid IN(SELECT case_id FROM tree))))`, in.CaseID, in.DeviceID, in.ChildCaseID).Scan(&issued)
	if e != nil {
		return nil, nil, e
	}
	if issued {
		missing = append(missing, "return_issued_equipment_first")
	}
	draft := map[string]any{"operation": op, "case_id": in.CaseID, "device_id": in.DeviceID, "product_id": in.ProductID, "child_case_id": in.ChildCaseID, "quantity": in.Quantity, "source_zone_id": sourceZone, "destination_zone_id": in.DestinationZoneID, "projected_case": projected, "gross_weight": weightSummary, "detached_contents": moved, "locations": zoneContexts, "tasks": tasks, "job_reservations_preserved": true}
	return draft, missing, nil
}

func caseContentKey(id int64) string { return strconv.FormatInt(id, 10) }
