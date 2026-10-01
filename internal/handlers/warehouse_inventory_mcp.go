package handlers

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"reflect"
	"strconv"
	"strings"

	"github.com/gorilla/mux"
	"warehousecore/internal/middleware"
	"warehousecore/internal/repository"
)

type inventoryMCPLineInput struct {
	ItemType        string   `json:"item_type"`
	ItemKey         string   `json:"item_key"`
	CountedQuantity *float64 `json:"counted_quantity"`
	ClearCounted    bool     `json:"clear_counted"`
}
type inventoryMCPRequest struct {
	CountID           int64                   `json:"count_id"`
	ZoneID            int64                   `json:"zone_id"`
	BlindCount        *bool                   `json:"blind_count"`
	Notes             *string                 `json:"notes"`
	Lines             []inventoryMCPLineInput `json:"lines"`
	MarkUncountedZero bool                    `json:"mark_uncounted_zero"`
	Reason            string                  `json:"reason"`
	ExpectedUpdatedAt string                  `json:"expected_updated_at"`
	ExpectedContext   string                  `json:"expected_context"`
	ConfirmChange     bool                    `json:"confirm_change"`
	ConfirmationText  string                  `json:"confirmation_text"`
	Preview           bool                    `json:"preview"`
}

const inventoryMCPCountSelect = `SELECT jsonb_build_object('count_id',count_id,'zone_id',zone_id,'status',status,'blind_count',blind_count,'notes',COALESCE(notes,''),'is_archived',is_archived,'archived_at',archived_at,'started_at',started_at,'completed_at',completed_at,'created_at',created_at,'updated_at',to_char(updated_at,'YYYY-MM-DD"T"HH24:MI:SS.US"Z"')) FROM inventory_counts WHERE count_id=$1 FOR UPDATE`
const inventoryMCPZoneSelect = `SELECT jsonb_build_object('zone_id',z.zone_id,'name',z.name,'code',z.code,'is_active',z.is_active,'is_storable',z.is_storable,'operational_status',z.operational_status,'parent_zone_id',z.parent_zone_id,'capacity',z.capacity,'capacity_mode',z.capacity_mode,'max_weight_kg',z.max_weight_kg,'max_volume_m3',z.max_volume_m3,'inventory_frequency_days',z.inventory_frequency_days,'last_counted_at',z.last_counted_at,'next_count_at',z.next_count_at,'updated_at',to_char(z.updated_at,'YYYY-MM-DD"T"HH24:MI:SS.US"Z"'),'profile_id',z.profile_id,'allow_devices',COALESCE(p.allow_devices,true),'allow_quantity_products',COALESCE(p.allow_quantity_products,true),'allow_cases',COALESCE(p.allow_cases,true),'allow_mixed_products',COALESCE(p.allow_mixed_products,true),'allow_cycle_count',COALESCE(p.allow_cycle_count,true),'profile_updated_at',p.updated_at) FROM storage_zones z LEFT JOIN location_profiles p ON p.profile_id=z.profile_id WHERE z.zone_id=$1`

func inventoryMCPJSON(tx *sql.Tx, query string, args ...any) (map[string]any, error) {
	var raw []byte
	if err := tx.QueryRow(query, args...).Scan(&raw); err != nil {
		return nil, err
	}
	out := map[string]any{}
	err := json.Unmarshal(raw, &out)
	return out, err
}

func inventoryMCPHash(value any) string {
	raw, _ := json.Marshal(value)
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

func inventoryMCPRows(tx *sql.Tx, query string, args ...any) ([]map[string]any, error) {
	rows, err := tx.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var raw []byte
		item := map[string]any{}
		if err = rows.Scan(&raw); err != nil {
			return nil, err
		}
		if err = json.Unmarshal(raw, &item); err != nil {
			return nil, err
		}
		items = append(items, item)
		if len(items) > 1000 {
			return nil, fmt.Errorf("Inventory workflow is limited to 1000 records; subdivide the storage zone")
		}
	}
	return items, rows.Err()
}

func inventoryMCPDevice(tx *sql.Tx, key string) (map[string]any, error) {
	row, err := inventoryMCPJSON(tx, `SELECT jsonb_build_object('device_id',d.deviceid,'product_id',d.productid,'lifecycle_status',d.lifecycle_status,'status',d.status,'condition_status',d.condition_status,'zone_id',d.zone_id,'current_case_id',d.current_case_id,'current_location',d.current_location,'updated_at',to_char(d.updated_at,'YYYY-MM-DD"T"HH24:MI:SS.US"Z"'),'product_lifecycle',p.lifecycle_status,'product_updated_at',p.updated_at,'weight',p.weight,'width',p.width,'height',p.height,'depth',p.depth) FROM devices d JOIN products p ON p.productid=d.productid WHERE d.deviceid=$1`, key)
	if err != nil {
		return nil, err
	}
	var jobs, picks, reservations, cases, components, tasks, orders, plans, defects int64
	err = tx.QueryRow(warehouseDeviceDependenciesSQL, key).Scan(&jobs, &picks, &reservations, &cases, &components, &tasks, &orders, &plans, &defects)
	row["dependencies"] = map[string]int64{"jobs": jobs, "picklists": picks, "reservations": reservations, "cases": cases, "components": components, "tasks": tasks, "maintenance_orders": orders, "defects": defects}
	return row, err
}

func inventoryMCPItem(tx *sql.Tx, kind, key string, zone int64) (map[string]any, error) {
	var row map[string]any
	var err error
	switch kind {
	case "device":
		row, err = inventoryMCPDevice(tx, key)
	case "product":
		row, err = inventoryMCPJSON(tx, `SELECT jsonb_build_object('product_id',p.productid,'name',p.name,'lifecycle_status',p.lifecycle_status,'tracking_mode',p.tracking_mode,'stock_quantity',p.stock_quantity,'updated_at',to_char(p.updated_at,'YYYY-MM-DD"T"HH24:MI:SS.US"Z"'),'quantity_in_zone',COALESCE(l.quantity,0),'location_updated_at',l.updated_at,'weight',p.weight,'width',p.width,'height',p.height,'depth',p.depth) FROM products p LEFT JOIN product_locations l ON l.product_id=p.productid AND l.zone_id=$2 WHERE p.productid=$1`, key, zone)
	case "case":
		row, err = inventoryMCPJSON(tx, `SELECT jsonb_build_object('case_id',caseid,'name',name,'lifecycle_status',lifecycle_status,'status',status,'workflow_status',workflow_status,'current_job_id',current_job_id,'zone_id',zone_id,'weight',weight,'width',width,'height',height,'depth',depth,'updated_at',to_char(updated_at,'YYYY-MM-DD"T"HH24:MI:SS.US"Z"'),'nested',EXISTS(SELECT 1 FROM case_child_contents WHERE child_case_id=caseid),'open_tasks',(SELECT count(*) FROM warehouse_tasks WHERE case_id=caseid AND status NOT IN ('done','completed','cancelled','closed'))) FROM cases WHERE caseid=$1`, key)
		if err != nil {
			return nil, err
		}
		// UNION bounds cycles without repeatedly traversing the same case.
		tree, treeErr := inventoryMCPRows(tx, `WITH RECURSIVE tree(case_id) AS (SELECT $1::int UNION SELECT cc.child_case_id FROM case_child_contents cc JOIN tree t ON t.case_id=cc.parent_case_id) SELECT jsonb_build_object('case_id',c.caseid,'lifecycle_status',c.lifecycle_status,'status',c.status,'workflow_status',c.workflow_status,'current_job_id',c.current_job_id,'weight',c.weight,'updated_at',c.updated_at) FROM cases c JOIN tree t ON t.case_id=c.caseid ORDER BY c.caseid LIMIT 1001`, key)
		if treeErr != nil {
			return nil, treeErr
		}
		row["case_tree"] = tree
		packed, packedErr := inventoryMCPRows(tx, `WITH RECURSIVE tree(case_id) AS (SELECT $1::int UNION SELECT cc.child_case_id FROM case_child_contents cc JOIN tree t ON t.case_id=cc.parent_case_id) SELECT jsonb_build_object('device_id',dc.deviceid,'case_id',dc.caseid) FROM devicescases dc JOIN tree t ON t.case_id=dc.caseid ORDER BY dc.deviceid LIMIT 1001`, key)
		if packedErr != nil {
			return nil, packedErr
		}
		devices := []map[string]any{}
		for _, p := range packed {
			d, e := inventoryMCPDevice(tx, fmt.Sprint(p["device_id"]))
			if e != nil {
				return nil, e
			}
			d["packed_case_id"] = p["case_id"]
			devices = append(devices, d)
		}
		row["packed_devices"] = devices
		contents, contentsErr := inventoryMCPRows(tx, `WITH RECURSIVE tree(case_id) AS (SELECT $1::int UNION SELECT cc.child_case_id FROM case_child_contents cc JOIN tree t ON t.case_id=cc.parent_case_id) SELECT jsonb_build_object('case_id',cp.case_id,'product_id',cp.product_id,'quantity',cp.quantity,'lifecycle_status',p.lifecycle_status,'weight',p.weight,'updated_at',p.updated_at) FROM case_product_contents cp JOIN tree t ON t.case_id=cp.case_id JOIN products p ON p.productid=cp.product_id ORDER BY cp.case_id,cp.product_id LIMIT 1001`, key)
		if contentsErr != nil {
			return nil, contentsErr
		}
		row["product_contents"] = contents
	default:
		return nil, fmt.Errorf("Exact device, product or case item type required")
	}
	return row, err
}

func inventoryMCPStock(tx *sql.Tx, zone int64) ([]map[string]any, error) {
	items, err := inventoryMCPRows(tx, `SELECT jsonb_build_object('item_type',s.kind,'item_key',s.key,'expected_quantity',s.qty) FROM (
 SELECT 'device'::text kind,d.deviceid::text key,1::numeric qty FROM devices d WHERE d.zone_id=$1 AND d.lifecycle_status='active' AND d.current_case_id IS NULL AND NOT EXISTS(SELECT 1 FROM devicescases dc WHERE dc.deviceid=d.deviceid)
 UNION ALL SELECT 'product',pl.product_id::text,pl.quantity FROM product_locations pl WHERE pl.zone_id=$1 AND pl.quantity<>0
 UNION ALL SELECT 'case',c.caseid::text,1 FROM cases c WHERE c.zone_id=$1 AND c.lifecycle_status='active' AND NOT EXISTS(SELECT 1 FROM case_child_contents cc WHERE cc.child_case_id=c.caseid)
 ) s ORDER BY s.kind,s.key LIMIT 1001`, zone)
	if err != nil {
		return nil, err
	}
	for _, item := range items {
		record, e := inventoryMCPItem(tx, fmt.Sprint(item["item_type"]), fmt.Sprint(item["item_key"]), zone)
		if e != nil {
			return nil, e
		}
		item["record"] = record
	}
	return items, nil
}

func inventoryMCPLines(tx *sql.Tx, id int64) ([]map[string]any, error) {
	return inventoryMCPRows(tx, `SELECT jsonb_build_object('line_id',line_id,'item_type',item_type,'item_key',item_key,'expected_quantity',expected_quantity,'counted_quantity',counted_quantity,'updated_at',to_char(updated_at,'YYYY-MM-DD"T"HH24:MI:SS.US"Z"')) FROM inventory_count_lines WHERE count_id=$1 ORDER BY item_type,item_key LIMIT 1001`, id)
}

func inventoryMCPHideBlind(lines []map[string]any, blind bool) []map[string]any {
	copy := make([]map[string]any, 0, len(lines))
	for _, line := range lines {
		next := map[string]any{}
		for key, value := range line {
			if key != "record" && (!blind || key != "expected_quantity") {
				next[key] = value
			}
		}
		copy = append(copy, next)
	}
	return copy
}

func inventoryMCPBlockedItem(kind string, row map[string]any, packed bool) bool {
	if row["lifecycle_status"] != "active" {
		return true
	}
	if kind == "product" {
		return row["tracking_mode"] != "quantity"
	}
	if kind == "case" {
		if row["status"] != "free" || row["nested"] == true || row["current_job_id"] != nil || row["workflow_status"] == "on_job" || row["workflow_status"] == "maintenance" || maintenanceMCPID(row["open_tasks"]) > 0 {
			return true
		}
		if tree, ok := row["case_tree"].([]map[string]any); ok {
			for _, c := range tree {
				if c["lifecycle_status"] != "active" || c["status"] != "free" || c["current_job_id"] != nil || c["workflow_status"] == "on_job" || c["workflow_status"] == "maintenance" {
					return true
				}
			}
		}
		if devices, ok := row["packed_devices"].([]map[string]any); ok {
			for _, d := range devices {
				if inventoryMCPBlockedItem("device", d, true) {
					return true
				}
			}
		}
		if contents, ok := row["product_contents"].([]map[string]any); ok {
			for _, p := range contents {
				if p["lifecycle_status"] != "active" {
					return true
				}
			}
		}
		return false
	}
	if row["product_lifecycle"] != "active" || row["condition_status"] == "retired" {
		return true
	}
	if !packed && (row["status"] != "in_storage" && row["status"] != "location_unknown" || row["current_case_id"] != nil) {
		return true
	}
	if packed && row["status"] != "in_storage" {
		return true
	}
	deps, _ := row["dependencies"].(map[string]int64)
	for key, n := range deps {
		if n > 0 && !(packed && key == "cases") {
			return true
		}
	}
	return false
}

// Products and case dimensions are stored in centimetres, weights in kg.
// A packed case occupies its outer envelope; its weight includes its entire
// case tree and contents, without counting packed items again as direct stock.
func inventoryMCPCapacity(lines []map[string]any, refs map[string]any, zone map[string]any) (map[string]any, []string) {
	count, weight, volume := 0.0, 0.0, 0.0
	weightKnown, volumeKnown := true, true
	kinds, products := map[string]bool{}, map[string]bool{}
	addWeight := func(row map[string]any, quantity float64) {
		kg, ok := row["weight"].(float64)
		if !ok || kg < 0 || math.IsNaN(kg) || math.IsInf(kg, 0) {
			weightKnown = false
			return
		}
		weight += kg * quantity
	}
	for _, line := range lines {
		qty, _ := line["counted_quantity"].(float64)
		if qty <= 0 {
			continue
		}
		kind, key := fmt.Sprint(line["item_type"]), fmt.Sprint(line["item_key"])
		row, _ := refs[kind+":"+key].(map[string]any)
		count += qty
		kinds[kind] = true
		if kind == "product" {
			products[key] = true
		} else if kind == "device" {
			products[fmt.Sprint(row["product_id"])] = true
		}
		width, wok := row["width"].(float64)
		height, hok := row["height"].(float64)
		depth, dok := row["depth"].(float64)
		if !wok || !hok || !dok || width <= 0 || height <= 0 || depth <= 0 {
			volumeKnown = false
		} else {
			volume += width * height * depth * qty / 1000000
		}
		if kind != "case" {
			addWeight(row, qty)
			continue
		}
		if tree, ok := row["case_tree"].([]map[string]any); ok {
			for _, c := range tree {
				addWeight(c, qty)
			}
		} else {
			weightKnown = false
		}
		if packed, ok := row["packed_devices"].([]map[string]any); ok {
			for _, d := range packed {
				addWeight(d, qty)
				products[fmt.Sprint(d["product_id"])] = true
			}
		}
		if contents, ok := row["product_contents"].([]map[string]any); ok {
			for _, p := range contents {
				n, _ := p["quantity"].(float64)
				addWeight(p, n*qty)
				products[fmt.Sprint(p["product_id"])] = true
			}
		}
	}
	required := []string{}
	if fmt.Sprint(zone["capacity_mode"]) != "item_count" {
		required = append(required, "item_count_capacity_mode")
	}
	if cap, ok := zone["capacity"].(float64); ok && count > cap {
		required = append(required, "projected_capacity")
	}
	if cap, ok := zone["max_weight_kg"].(float64); ok {
		if !weightKnown {
			required = append(required, "complete_item_and_case_content_weights")
		} else if weight > cap {
			required = append(required, "projected_weight_capacity")
		}
	}
	if cap, ok := zone["max_volume_m3"].(float64); ok {
		if !volumeKnown {
			required = append(required, "complete_item_outer_dimensions")
		} else if volume > cap {
			required = append(required, "projected_volume_capacity")
		}
	}
	if kinds["device"] && zone["allow_devices"] != true || kinds["product"] && zone["allow_quantity_products"] != true || kinds["case"] && zone["allow_cases"] != true {
		required = append(required, "location_profile_item_types")
	}
	if zone["allow_mixed_products"] == false && len(products) > 1 {
		required = append(required, "location_profile_mixed_products")
	}
	result := map[string]any{"item_count": count, "weight_kg": nil, "volume_m3": nil, "capacity": zone["capacity"], "max_weight_kg": zone["max_weight_kg"], "max_volume_m3": zone["max_volume_m3"]}
	if weightKnown {
		result["weight_kg"] = weight
	}
	if volumeKnown {
		result["volume_m3"] = volume
	}
	return result, required
}

func WarehouseInventoryMCP(w http.ResponseWriter, r *http.Request) {
	user, ok := middleware.GetUserFromContext(r)
	if !ok || user == nil || user.UserID == 0 || !user.IsAdmin || !isWarehouseMCPMutation(r) {
		respondJSON(w, 403, map[string]string{"error": "Signed-in warehouse administrator and MCP origin required"})
		return
	}
	op := mux.Vars(r)["operation"]
	if !map[string]bool{"create": true, "update": true, "set_lines": true, "review": true, "return_for_counting": true, "approve": true, "cancel": true, "archive": true, "restore": true}[op] {
		respondJSON(w, 400, map[string]string{"error": "Unsupported guided inventory operation"})
		return
	}
	var in inventoryMCPRequest
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 65536))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&in); err != nil {
		respondJSON(w, 400, map[string]string{"error": "Invalid bounded inventory body"})
		return
	}
	if err := dec.Decode(new(any)); err != io.EOF || op == "create" && (in.CountID != 0 || in.ZoneID <= 0) || op != "create" && (in.CountID <= 0 || in.ZoneID != 0) {
		respondJSON(w, 400, map[string]string{"error": "One object and exact operation-specific count/zone ID required"})
		return
	}
	if len(in.Lines) > 100 || len([]rune(in.Reason)) > 4000 || in.Notes != nil && len([]rune(*in.Notes)) > 4000 {
		respondJSON(w, 400, map[string]string{"error": "At most 100 line changes and 4000 characters per note/reason"})
		return
	}
	if op != "create" && op != "update" && (in.BlindCount != nil || in.Notes != nil) || op != "set_lines" && len(in.Lines) > 0 || op != "review" && in.MarkUncountedZero {
		respondJSON(w, 400, map[string]string{"error": "Metadata, line changes and review acknowledgements belong to separate actions"})
		return
	}
	preview := in.Preview || !in.ConfirmChange
	elevated := op == "review" || op == "approve" || op == "cancel" || op == "archive" || op == "restore"
	phrase := fmt.Sprintf("%s WAREHOUSE INVENTORY COUNT %d", strings.ToUpper(op), in.CountID)
	if !preview && (in.ExpectedContext == "" || op != "create" && in.ExpectedUpdatedAt == "" || elevated && in.ConfirmationText != phrase) {
		respondJSON(w, 428, map[string]string{"error": "Exact context/count versions and count-bound elevated confirmation required"})
		return
	}
	tx, err := repository.GetSQLDB().BeginTx(r.Context(), nil)
	if err != nil {
		respondMaintenanceMCPError(w, err)
		return
	}
	defer tx.Rollback()
	if _, err = tx.Exec(`SET LOCAL lock_timeout='5s'; SET LOCAL statement_timeout='30s'`); err != nil {
		respondMaintenanceMCPError(w, err)
		return
	}
	var receipt int64
	if !preview {
		var replay json.RawMessage
		receipt, replay, err = beginWarehouseProductMutation(tx, r, "inventory_count."+op, in)
		if err != nil {
			respondMaintenanceMCPError(w, err)
			return
		}
		if replay != nil {
			respondJSON(w, 200, replay)
			return
		}
	}
	// Writers and dependency triggers share these locks. Every snapshot and all
	// approval effects stay in one transaction, including scanner/UI changes.
	if _, err = tx.Exec(`LOCK TABLE inventory_counts,inventory_count_lines,inventory_count_events,inventory_adjustments,devices,cases,products,storage_zones,product_locations,device_movements,case_events IN SHARE ROW EXCLUSIVE MODE; LOCK TABLE location_profiles,devicescases,case_product_contents,case_child_contents,warehouse_tasks,job_devices,jobs,status,job_positions,job_position_devices,job_package_reservations,device_components,maintenance_orders,maintenance_plans,defect_reports IN SHARE MODE`); err != nil {
		respondMaintenanceMCPError(w, err)
		return
	}
	current := map[string]any{}
	draft := map[string]any{"zone_id": in.ZoneID, "status": "counting", "blind_count": true, "notes": "", "is_archived": false}
	lines := []map[string]any{}
	required := []string{}
	if op != "create" {
		current, err = inventoryMCPJSON(tx, inventoryMCPCountSelect, in.CountID)
		if err == sql.ErrNoRows {
			respondJSON(w, 404, map[string]string{"error": "Inventory count not found"})
			return
		}
		if err != nil {
			respondMaintenanceMCPError(w, err)
			return
		}
		for key, value := range current {
			draft[key] = value
		}
		lines, err = inventoryMCPLines(tx, in.CountID)
		if err != nil {
			respondMaintenanceMCPError(w, err)
			return
		}
		if in.ExpectedUpdatedAt != "" && in.ExpectedUpdatedAt != current["updated_at"] {
			required = append(required, "expected_updated_at")
		}
	}
	zoneID := maintenanceMCPID(draft["zone_id"])
	if op == "create" {
		zoneID = in.ZoneID
	}
	zone, err := inventoryMCPJSON(tx, inventoryMCPZoneSelect, zoneID)
	if err == sql.ErrNoRows {
		respondJSON(w, 404, map[string]string{"error": "Inventory zone not found"})
		return
	}
	if err != nil {
		respondMaintenanceMCPError(w, err)
		return
	}
	if op == "create" || op == "update" {
		if in.BlindCount != nil {
			draft["blind_count"] = *in.BlindCount
		}
		if in.Notes != nil {
			draft["notes"] = *in.Notes
		}
	}
	state := fmt.Sprint(draft["status"])
	terminal := state == "approved" || state == "cancelled"
	if op == "restore" {
		if draft["is_archived"] != true {
			required = append(required, "archived_count_required")
		}
		draft["is_archived"] = false
	} else if draft["is_archived"] == true {
		required = append(required, "restore_before_change")
	}
	if (op == "update" || op == "set_lines" || op == "review") && state != "counting" {
		required = append(required, "counting_status_required")
	}
	if op == "return_for_counting" || op == "approve" {
		if state != "review" {
			required = append(required, "review_status_required")
		}
	}
	if op == "cancel" && terminal {
		required = append(required, "active_count_required")
	}
	if op == "archive" && !terminal {
		required = append(required, "terminal_count_required")
	}
	if (op == "cancel" || op == "return_for_counting") && strings.TrimSpace(in.Reason) == "" {
		required = append(required, "reason")
	}
	if op == "create" || !terminal && op != "archive" && op != "restore" {
		if zone["is_active"] != true || zone["is_storable"] != true || zone["allow_cycle_count"] != true {
			required = append(required, "active_storable_cycle_count_zone")
		}
		want := "counting"
		if op == "create" {
			want = "available"
		}
		if zone["operational_status"] != want {
			required = append(required, "zone_status."+want)
		}
		if err = validateCaseMCPHierarchy(tx, zoneID); err != nil {
			required = append(required, "active_zone_hierarchy")
		}
		var competing int
		err = tx.QueryRow(`SELECT count(*) FROM inventory_counts WHERE zone_id=$1 AND count_id<>$2 AND status IN ('open','counting','review') AND NOT is_archived`, zoneID, in.CountID).Scan(&competing)
		if err != nil {
			respondMaintenanceMCPError(w, err)
			return
		}
		if competing > 0 {
			required = append(required, "no_other_active_count")
		}
	}
	stock := []map[string]any{}
	if op != "archive" && op != "restore" {
		stock, err = inventoryMCPStock(tx, zoneID)
	}
	if err != nil {
		respondMaintenanceMCPError(w, err)
		return
	}
	if op == "create" {
		for _, item := range stock {
			lines = append(lines, map[string]any{"item_type": item["item_type"], "item_key": item["item_key"], "expected_quantity": item["expected_quantity"], "counted_quantity": nil})
		}
	}
	beforeLines := inventoryMCPHideBlind(lines, false)
	seen := map[string]bool{}
	if op == "set_lines" {
		if len(in.Lines) == 0 {
			required = append(required, "lines")
		}
		for _, change := range in.Lines {
			if !map[string]bool{"device": true, "product": true, "case": true}[change.ItemType] || change.ItemKey == "" || len(change.ItemKey) > 255 || strings.TrimSpace(change.ItemKey) != change.ItemKey {
				respondJSON(w, 400, map[string]string{"error": "Exact bounded typed item identity required"})
				return
			}
			if change.ItemType != "device" {
				id, e := strconv.ParseInt(change.ItemKey, 10, 32)
				if e != nil || id <= 0 || strconv.FormatInt(id, 10) != change.ItemKey {
					respondJSON(w, 400, map[string]string{"error": "Canonical positive numeric case/product item key required"})
					return
				}
			}
			identity := change.ItemType + ":" + change.ItemKey
			if seen[identity] || change.ClearCounted == (change.CountedQuantity != nil) {
				respondJSON(w, 400, map[string]string{"error": "Unique line and exactly one counted_quantity or clear_counted required"})
				return
			}
			seen[identity] = true
			var qty any
			if change.CountedQuantity != nil {
				q := *change.CountedQuantity
				s := strconv.FormatFloat(q, 'f', -1, 64)
				decimal := strings.SplitN(s, ".", 2)
				if math.IsNaN(q) || math.IsInf(q, 0) || q < 0 || q > 9999999.999 || len(decimal) == 2 && len(decimal[1]) > 3 || change.ItemType != "product" && q != 0 && q != 1 {
					respondJSON(w, 400, map[string]string{"error": "Nonnegative quantity up to 9999999.999 with at most three decimals; device/case must be zero or one"})
					return
				}
				qty = q
			}
			found := false
			for _, line := range lines {
				if line["item_type"] == change.ItemType && line["item_key"] == change.ItemKey {
					line["counted_quantity"] = qty
					found = true
					break
				}
			}
			if !found {
				if change.ClearCounted {
					required = append(required, "existing_line_for_clear")
				}
				lines = append(lines, map[string]any{"item_type": change.ItemType, "item_key": change.ItemKey, "expected_quantity": float64(0), "counted_quantity": qty})
			}
		}
	}
	if len(lines) > 1000 {
		respondJSON(w, 400, map[string]string{"error": "At most 1000 lines per zone count"})
		return
	}
	missing := []map[string]any{}
	for _, line := range lines {
		if line["counted_quantity"] == nil {
			missing = append(missing, map[string]any{"item_type": line["item_type"], "item_key": line["item_key"]})
			if op == "review" && in.MarkUncountedZero {
				line["counted_quantity"] = float64(0)
			}
		}
	}
	if op == "review" {
		if len(missing) > 0 && !in.MarkUncountedZero {
			required = append(required, "count_all_lines_or_explicit_mark_uncounted_zero")
		}
		draft["status"] = "review"
	}
	if op == "return_for_counting" {
		draft["status"] = "counting"
	}
	if op == "approve" {
		draft["status"] = "approved"
		if len(missing) > 0 {
			required = append(required, "all_lines_counted")
		}
	}
	if op == "cancel" {
		draft["status"] = "cancelled"
	}
	if op == "archive" {
		draft["is_archived"] = true
	}
	refs := map[string]any{}
	sourceZones := map[string]any{}
	effects := []map[string]any{}
	for _, line := range lines {
		if op == "archive" || op == "restore" {
			continue
		}
		kind, key := fmt.Sprint(line["item_type"]), fmt.Sprint(line["item_key"])
		identity := kind + ":" + key
		row, e := inventoryMCPItem(tx, kind, key, zoneID)
		if e != nil && e != sql.ErrNoRows {
			respondMaintenanceMCPError(w, e)
			return
		}
		refs[identity] = row
		if row != nil && kind != "product" && row["zone_id"] != nil && maintenanceMCPID(row["zone_id"]) != zoneID && op != "archive" && op != "restore" {
			sourceID := fmt.Sprint(row["zone_id"])
			if sourceZones[sourceID] == nil {
				source, e := inventoryMCPJSON(tx, inventoryMCPZoneSelect, row["zone_id"])
				if e != nil {
					respondMaintenanceMCPError(w, e)
					return
				}
				counts, countErr := inventoryMCPRows(tx, `SELECT jsonb_build_object('count_id',count_id,'status',status,'updated_at',updated_at) FROM inventory_counts WHERE zone_id=$1 AND status IN ('open','counting','review') AND NOT is_archived ORDER BY count_id LIMIT 1001`, row["zone_id"])
				if countErr != nil {
					respondMaintenanceMCPError(w, countErr)
					return
				}
				source["active_counts"] = counts
				sourceZones[sourceID] = source
			}
		}
		if (op == "set_lines" && seen[identity] || op == "approve") && (e == sql.ErrNoRows || inventoryMCPBlockedItem(kind, row, false)) {
			required = append(required, "available_item."+identity)
		}
		if op == "approve" && row != nil {
			qty := line["counted_quantity"]
			before := line["expected_quantity"]
			var target any = zoneID
			if kind != "product" && maintenanceMCPID(qty) == 0 {
				target = nil
			}
			if target != nil && kind != "product" && row["zone_id"] != nil && maintenanceMCPID(row["zone_id"]) != zoneID {
				var sourceActive bool
				var sourceState string
				e = tx.QueryRow(`SELECT is_active,operational_status FROM storage_zones WHERE zone_id=$1`, row["zone_id"]).Scan(&sourceActive, &sourceState)
				source, _ := sourceZones[fmt.Sprint(row["zone_id"])].(map[string]any)
				sourceCounts, _ := source["active_counts"].([]map[string]any)
				if e != nil || !sourceActive || sourceState != "available" || len(sourceCounts) > 0 || validateCaseMCPHierarchy(tx, maintenanceMCPID(row["zone_id"])) != nil {
					required = append(required, "available_source_zone."+identity)
				}
			}
			effect := map[string]any{"item_type": kind, "item_key": key, "quantity_before": before, "quantity_after": qty}
			if kind != "product" {
				if maintenanceMCPID(before) == 0 && target == nil {
					target = row["zone_id"]
					effect["no_physical_change"] = true
				}
				effect["from_zone_id"] = row["zone_id"]
				effect["to_zone_id"] = target
				if kind == "device" {
					effect["physical_status_before"] = row["status"]
					effect["physical_status_after"] = "in_storage"
					effect["current_location_before"] = row["current_location"]
					effect["current_location_after"] = "warehouse"
					effect["condition_status_preserved"] = row["condition_status"]
					if target == nil {
						effect["physical_status_after"] = "location_unknown"
						effect["current_location_after"] = "location_unknown"
					}
					if effect["no_physical_change"] == true || target != nil && maintenanceMCPID(row["zone_id"]) == zoneID && row["status"] == "in_storage" {
						effect["no_physical_change"] = true
						effect["physical_status_after"] = row["status"]
						effect["current_location_after"] = row["current_location"]
					}
				}
				if kind == "case" {
					effect["packed_contents_remain_packed"] = true
				}
			}
			effects = append(effects, effect)
		}
	}
	ancestors, err := inventoryMCPRows(tx, `WITH RECURSIVE tree AS (SELECT zone_id,parent_zone_id,is_active,operational_status,updated_at,ARRAY[zone_id] AS path FROM storage_zones WHERE zone_id=$1 UNION ALL SELECT p.zone_id,p.parent_zone_id,p.is_active,p.operational_status,p.updated_at,t.path||p.zone_id FROM storage_zones p JOIN tree t ON p.zone_id=t.parent_zone_id WHERE NOT p.zone_id=ANY(t.path) AND cardinality(t.path)<64) SELECT jsonb_build_object('zone_id',zone_id,'parent_zone_id',parent_zone_id,'is_active',is_active,'operational_status',operational_status,'updated_at',updated_at) FROM tree ORDER BY zone_id`, zoneID)
	if err != nil {
		respondMaintenanceMCPError(w, err)
		return
	}
	contextFields := map[string]any{"zone": zone, "ancestors": ancestors, "stock": stock, "references": refs, "current": current, "lines_before": beforeLines, "source_zones": sourceZones}
	encodedContext, encodeErr := json.Marshal(contextFields)
	if encodeErr != nil {
		respondMaintenanceMCPError(w, encodeErr)
		return
	}
	if len(encodedContext) > 2*1024*1024-16384 {
		respondJSON(w, 400, map[string]string{"error": "Full inventory context exceeds two MiB; subdivide the count zone"})
		return
	}
	contextVersion := inventoryMCPHash(contextFields)
	if in.ExpectedContext != "" && in.ExpectedContext != contextVersion {
		required = append(required, "expected_context")
	}
	if op == "approve" {
		var baselineRaw []byte
		var baseline any
		if err = tx.QueryRow(`SELECT stock_baseline FROM inventory_counts WHERE count_id=$1`, in.CountID).Scan(&baselineRaw); err != nil {
			respondMaintenanceMCPError(w, err)
			return
		}
		if len(baselineRaw) == 0 {
			required = append(required, "guided_count_baseline_required")
		} else if err = json.Unmarshal(baselineRaw, &baseline); err != nil {
			respondMaintenanceMCPError(w, err)
			return
		} else if inventoryMCPHash(baseline) != inventoryMCPHash(stock) {
			required = append(required, "stock_changed_since_count_start_cancel_and_recount")
		}
		_, capacityRequired := inventoryMCPCapacity(lines, refs, zone)
		required = append(required, capacityRequired...)
	}
	blind := current["blind_count"] == true && (state == "counting" || state == "open") || op == "create" && draft["blind_count"] == true
	diff := map[string]any{}
	for key, value := range draft {
		if !reflect.DeepEqual(current[key], value) {
			diff[key] = map[string]any{"before": current[key], "after": value}
		}
	}
	result := map[string]any{"operation_status": "confirmation_required", "preview": true, "ready_to_execute": len(required) == 0, "current": current, "draft": draft, "diff": diff, "lines": inventoryMCPHideBlind(lines, blind), "lines_before": inventoryMCPHideBlind(beforeLines, blind), "uncounted_lines": missing, "expected_updated_at": current["updated_at"], "expected_context": contextVersion, "zone": zone, "required_fields": required, "effects": map[string]any{"stock_adjustments": effects, "zero_uncounted_lines": op == "review" && in.MarkUncountedZero, "inventory_movement": op == "approve", "history_preserved": true}}
	if elevated {
		result["required_confirmation_text"] = phrase
	}
	workflowEffects := result["effects"].(map[string]any)
	workflowEffects["zone_operational_status"] = zone["operational_status"]
	if op == "create" {
		workflowEffects["zone_operational_status"] = "counting"
	}
	if op == "approve" || op == "cancel" {
		workflowEffects["zone_operational_status"] = "available"
	}
	workflowEffects["line_quantity_mode"] = "replace"
	workflowEffects["serialized_condition_preserved"] = true
	if op == "approve" {
		workflowEffects["last_counted_at_rule"] = "approval transaction time"
		workflowEffects["next_count_at_rule"] = "approval time plus zone inventory_frequency_days, or null when disabled"
	}

	if blind {
		result["effects"].(map[string]any)["stock_adjustments"] = []map[string]any{}
	}
	if op == "approve" && !blind {
		load, _ := inventoryMCPCapacity(lines, refs, zone)
		result["references"] = refs
		result["source_zones"] = sourceZones
		result["projected_load"] = load
	}

	if encoded, _ := json.Marshal(result); len(encoded) > 2*1024*1024-1024 {
		respondJSON(w, 400, map[string]string{"error": "Inventory preview exceeds two MiB; subdivide the count zone"})
		return
	}
	if preview || len(required) > 0 {
		if len(required) > 0 {
			result["operation_status"] = "needs_input"
		}
		respondJSON(w, 200, result)
		return
	}
	id := in.CountID
	zoneBefore := zone
	if op == "create" {
		baseline, _ := json.Marshal(stock)
		err = tx.QueryRow(`INSERT INTO inventory_counts(zone_id,status,blind_count,notes,started_at,stock_baseline) VALUES($1,'counting',$2,$3,CURRENT_TIMESTAMP,$4::jsonb) RETURNING count_id`, zoneID, draft["blind_count"], draft["notes"], string(baseline)).Scan(&id)
		if err == nil {
			for _, line := range lines {
				_, err = tx.Exec(`INSERT INTO inventory_count_lines(count_id,item_type,item_key,expected_quantity) VALUES($1,$2,$3,$4)`, id, line["item_type"], line["item_key"], line["expected_quantity"])
				if err != nil {
					break
				}
			}
		}
		if err == nil {
			_, err = tx.Exec(`UPDATE storage_zones SET operational_status='counting' WHERE zone_id=$1`, zoneID)
		}
	} else if op == "set_lines" {
		for _, line := range lines {
			identity := fmt.Sprint(line["item_type"]) + ":" + fmt.Sprint(line["item_key"])
			if !seen[identity] {
				continue
			}
			_, err = tx.Exec(`INSERT INTO inventory_count_lines(count_id,item_type,item_key,expected_quantity,counted_quantity) VALUES($1,$2,$3,$4,$5) ON CONFLICT(count_id,item_type,item_key) DO UPDATE SET counted_quantity=EXCLUDED.counted_quantity`, id, line["item_type"], line["item_key"], line["expected_quantity"], line["counted_quantity"])
			if err != nil {
				break
			}
		}
	} else if op == "update" {
		_, err = tx.Exec(`UPDATE inventory_counts SET blind_count=$1,notes=$2 WHERE count_id=$3`, draft["blind_count"], draft["notes"], id)
	} else if op == "archive" || op == "restore" {
		_, err = tx.Exec(`UPDATE inventory_counts SET is_archived=$1 WHERE count_id=$2`, draft["is_archived"], id)
	} else {
		if op == "review" && in.MarkUncountedZero {
			_, err = tx.Exec(`UPDATE inventory_count_lines SET counted_quantity=0 WHERE count_id=$1 AND counted_quantity IS NULL`, id)
		}
		if err == nil && op == "approve" {
			err = inventoryMCPApply(tx, r, id, zoneID, lines, refs)
		}
		if err == nil {
			_, err = tx.Exec(`UPDATE inventory_counts SET status=$1::text,completed_at=CASE WHEN $1::text IN ('approved','cancelled') THEN CURRENT_TIMESTAMP ELSE NULL END WHERE count_id=$2`, draft["status"], id)
		}
		if err == nil && (op == "approve" || op == "cancel") {
			_, err = tx.Exec(`UPDATE storage_zones SET operational_status='available',last_counted_at=CASE WHEN $2 THEN CURRENT_TIMESTAMP ELSE last_counted_at END,next_count_at=CASE WHEN NOT $2 THEN next_count_at WHEN inventory_frequency_days>0 THEN CURRENT_TIMESTAMP+(inventory_frequency_days::text||' days')::interval ELSE NULL END WHERE zone_id=$1`, zoneID, op == "approve")
		}
	}
	if err != nil {
		respondMaintenanceMCPError(w, err)
		return
	}
	if _, err = tx.Exec(`INSERT INTO inventory_count_events(count_id,event_type,from_status,to_status,reason,actor_id) VALUES($1,$2,$3,$4,$5,$6)`, id, "mcp_"+op, current["status"], draft["status"], maintenanceNullableString(in.Reason), user.UserID); err != nil {
		respondMaintenanceMCPError(w, err)
		return
	}
	after, err := inventoryMCPJSON(tx, inventoryMCPCountSelect, id)
	if err != nil {
		respondMaintenanceMCPError(w, err)
		return
	}
	afterLines, err := inventoryMCPLines(tx, id)
	if err != nil {
		respondMaintenanceMCPError(w, err)
		return
	}
	if !reflect.DeepEqual(beforeLines, afterLines) {
		if _, err = maintenanceMCPAudit(tx, r, "inventory_count."+op+".lines", "inventory_count_lines", fmt.Sprint(id), beforeLines, afterLines, fmt.Sprint(after["updated_at"])); err != nil {
			respondMaintenanceMCPError(w, err)
			return
		}
	}
	zoneAfter, err := inventoryMCPJSON(tx, inventoryMCPZoneSelect, zoneID)
	if err != nil {
		respondMaintenanceMCPError(w, err)
		return
	}
	if !reflect.DeepEqual(zoneBefore, zoneAfter) {
		if _, err = maintenanceMCPAudit(tx, r, "inventory_count."+op+".zone", "location", fmt.Sprint(zoneID), zoneBefore, zoneAfter, fmt.Sprint(zoneAfter["updated_at"])); err != nil {
			respondMaintenanceMCPError(w, err)
			return
		}
	}
	auditID, err := maintenanceMCPAudit(tx, r, "inventory_count."+op, "inventory_count", fmt.Sprint(id), current, after, fmt.Sprint(after["updated_at"]))
	if err != nil {
		respondMaintenanceMCPError(w, err)
		return
	}
	blindAfter := after["blind_count"] == true && after["status"] == "counting"
	result = map[string]any{"operation_status": op + "_completed", "inventory_count": after, "lines": inventoryMCPHideBlind(afterLines, blindAfter), "zone": zoneAfter, "audit_id": auditID, "effects": effects}
	if err = completeWarehouseProductMutation(tx, receipt, 200, result); err == nil {
		err = tx.Commit()
	}
	if err != nil {
		respondMaintenanceMCPError(w, err)
		return
	}
	respondJSON(w, 200, result)
}

func inventoryMCPApply(tx *sql.Tx, r *http.Request, id, zone int64, lines []map[string]any, refs map[string]any) error {
	user, _ := middleware.GetUserFromContext(r)
	if _, err := tx.Exec(`SELECT set_config('warehouse.inventory_approval','guided',true)`); err != nil {
		return err
	}
	for _, line := range lines {
		kind, key := fmt.Sprint(line["item_type"]), fmt.Sprint(line["item_key"])
		before := refs[kind+":"+key].(map[string]any)
		qty := line["counted_quantity"]
		var destination any = zone
		if kind != "product" && maintenanceMCPID(qty) == 0 {
			destination = nil
		}
		var err error
		switch kind {
		case "device":
			// An unexpected item explicitly counted zero remains at its source.
			if maintenanceMCPID(line["expected_quantity"]) == 0 && destination == nil {
				continue
			}
			if maintenanceMCPID(before["zone_id"]) == zone && destination != nil && before["status"] == "in_storage" {
				continue
			}
			_, err = tx.Exec(`UPDATE devices SET zone_id=$1,status=CASE WHEN $1::int IS NULL THEN 'location_unknown' ELSE 'in_storage' END,current_location=CASE WHEN $1::int IS NULL THEN 'location_unknown' ELSE 'warehouse' END WHERE deviceid=$2`, destination, key)
			if err == nil {
				_, err = tx.Exec(`INSERT INTO device_movements(device_id,from_zone_id,to_zone_id,moved_by,movement_type,reason,metadata) VALUES($1,$2,$3,$4,'inventory_adjustment','Confirmed guided inventory count',jsonb_build_object('count_id',$5::bigint,'origin','MCP/AI'))`, key, before["zone_id"], destination, user.UserID, id)
			}
		case "case":
			if maintenanceMCPID(line["expected_quantity"]) == 0 && destination == nil {
				continue
			}
			if maintenanceMCPID(before["zone_id"]) == zone && destination != nil {
				continue
			}
			_, err = tx.Exec(`UPDATE cases SET zone_id=$1 WHERE caseid=$2`, destination, key)
			if err == nil {
				_, err = tx.Exec(`INSERT INTO case_events(case_id,event_type,zone_id,metadata) VALUES($1,'inventory_adjustment',$2,jsonb_build_object('count_id',$3::bigint,'origin','MCP/AI','from_zone_id',$4::int))`, key, destination, id, before["zone_id"])
			}
		case "product":
			if reflect.DeepEqual(line["expected_quantity"], qty) {
				continue
			}
			_, err = tx.Exec(`INSERT INTO product_locations(product_id,zone_id,quantity) VALUES($1,$2,$3) ON CONFLICT(product_id,zone_id) DO UPDATE SET quantity=EXCLUDED.quantity,updated_at=clock_timestamp() AT TIME ZONE 'UTC'`, key, zone, qty)
		}
		if err != nil {
			return err
		}
		if _, err = tx.Exec(`INSERT INTO inventory_adjustments(count_id,item_type,item_key,zone_id,quantity_before,quantity_after,actor_id) VALUES($1,$2,$3,$4,$5,$6,$7)`, id, kind, key, zone, line["expected_quantity"], qty, user.UserID); err != nil {
			return err
		}
		after, err := inventoryMCPItem(tx, kind, key, zone)
		if err != nil {
			return err
		}
		entity := kind
		auditEntityID := key
		if kind == "product" {
			entity = "product_location"
			auditEntityID = fmt.Sprintf("%s:%d", key, zone)
		}
		if _, err = maintenanceMCPAudit(tx, r, "inventory_count.approve.adjustment", entity, auditEntityID, before, after, fmt.Sprint(after["updated_at"])); err != nil {
			return err
		}
		if kind == "product" {
			if _, err = maintenanceMCPAudit(tx, r, "inventory_count.approve.stock", "product", key, before, after, fmt.Sprint(after["updated_at"])); err != nil {
				return err
			}
		}
	}
	return nil
}
