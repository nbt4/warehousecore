package handlers

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"reflect"
	"regexp"
	"strings"
	"time"

	"github.com/gorilla/mux"
	"warehousecore/internal/middleware"
	"warehousecore/internal/repository"
)

type warehouseMaintenanceOrderRequest struct {
	OrderID                 int64    `json:"order_id"`
	DeviceID                *string  `json:"device_id"`
	PlanID                  *int64   `json:"plan_id"`
	OrderType               *string  `json:"order_type"`
	Priority                *string  `json:"priority"`
	Title                   *string  `json:"title"`
	Description             *string  `json:"description"`
	DueAt                   *string  `json:"due_at"`
	ScheduledAt             *string  `json:"scheduled_at"`
	AssignedTo              *int64   `json:"assigned_to"`
	CostAmount              *string  `json:"cost_amount"`
	ClearFields             []string `json:"clear_fields"`
	Status                  string   `json:"status"`
	Outcome                 string   `json:"outcome"`
	Resolution              string   `json:"resolution"`
	Notes                   string   `json:"notes"`
	NextDueAt               *string  `json:"next_due_at"`
	ExpectedUpdatedAt       string   `json:"expected_updated_at"`
	ExpectedDeviceUpdatedAt string   `json:"expected_device_updated_at"`
	ExpectedPlanUpdatedAt   string   `json:"expected_plan_updated_at"`
	ConfirmChange           bool     `json:"confirm_change"`
	ConfirmationText        string   `json:"confirmation_text"`
	Preview                 bool     `json:"preview"`
}

const maintenanceMCPOrderSelect = `SELECT jsonb_build_object('order_id',order_id,'legacy_defect_id',legacy_defect_id,'device_id',device_id,'plan_id',plan_id,'order_type',order_type,'priority',priority,'status',status,'title',title,'description',COALESCE(description,''),'due_at',to_char(due_at,'YYYY-MM-DD'),'scheduled_at',to_char(scheduled_at,'YYYY-MM-DD"T"HH24:MI:SS.US"Z"'),'reported_by',reported_by,'assigned_to',assigned_to,'started_at',started_at,'completed_at',completed_at,'outcome',outcome,'resolution',resolution,'cost_amount',cost::text,'is_archived',is_archived,'archived_at',archived_at,'created_at',created_at,'updated_at',to_char(updated_at,'YYYY-MM-DD"T"HH24:MI:SS.US"Z"')) FROM maintenance_orders WHERE order_id=$1 FOR UPDATE`

func loadMaintenanceMCPJSON(tx *sql.Tx, query string, id any) (map[string]any, error) {
	var raw []byte
	if err := tx.QueryRow(query, id).Scan(&raw); err != nil {
		return nil, err
	}
	out := map[string]any{}
	err := json.Unmarshal(raw, &out)
	return out, err
}
func maintenanceMCPID(value any) int64 {
	if value == nil {
		return 0
	}
	switch v := value.(type) {
	case int64:
		return v
	case float64:
		return int64(v)
	}
	return 0
}

// Determine the full condition effect without writing, preserving manual blocks
// and retirement, and considering all other unresolved canonical work orders.
func maintenanceMCPCondition(tx *sql.Tx, device string, id int64, kind, status, outcome, current string) (string, error) {
	if current == "blocked" || current == "retired" {
		return current, nil
	}
	if status == "in_progress" || status == "waiting_parts" {
		return "maintenance", nil
	}
	if kind == "defect" && (status == "open" || status == "planned") || outcome == "failed" {
		return "defective", nil
	}
	var defects, working int
	err := tx.QueryRow(`SELECT count(*) FILTER(WHERE order_type='defect'),count(*) FILTER(WHERE status IN ('in_progress','waiting_parts')) FROM maintenance_orders WHERE device_id=$1 AND order_id<>$2 AND status NOT IN ('completed','cancelled')`, device, id).Scan(&defects, &working)
	if err != nil {
		return "", err
	}
	if defects > 0 {
		return "defective", nil
	}
	if working > 0 {
		return "maintenance", nil
	}
	// Unlinked legacy reports can still block release of a device. Both legacy
	// schemas are supported; canonical linked reports are considered above.
	var legacy int
	err = tx.QueryRow(`SELECT count(*) FROM defect_reports d WHERE device_id=$1 AND lower(status) NOT IN ('resolved','repaired','closed','done') AND NOT EXISTS(SELECT 1 FROM maintenance_orders o WHERE o.legacy_defect_id=d.defect_id)`, device).Scan(&legacy)
	if err != nil {
		return "", err
	}
	if legacy > 0 {
		return "defective", nil
	}
	return "available", nil
}

func WarehouseMaintenanceOrderMCP(w http.ResponseWriter, r *http.Request) {
	user, ok := middleware.GetUserFromContext(r)
	if !ok || user == nil || user.UserID == 0 || !user.IsAdmin || !isWarehouseMCPMutation(r) {
		respondJSON(w, 403, map[string]string{"error": "Signed-in warehouse administrator and MCP origin required"})
		return
	}
	op := mux.Vars(r)["operation"]
	defect := mux.Vars(r)["entity"] == "defects"
	if !map[string]bool{"create": true, "update": true, "transition": true, "complete": true, "cancel": true, "reopen": true, "archive": true, "restore": true}[op] {
		respondJSON(w, 400, map[string]string{"error": "Unsupported maintenance operation"})
		return
	}
	var in warehouseMaintenanceOrderRequest
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 65536))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&in); err != nil {
		respondJSON(w, 400, map[string]string{"error": "Invalid bounded maintenance order body"})
		return
	}
	if err := decoder.Decode(new(any)); err != io.EOF || op == "create" && in.OrderID != 0 || op != "create" && in.OrderID <= 0 {
		respondJSON(w, 400, map[string]string{"error": "One object and exact operation-specific order ID required"})
		return
	}
	if in.AssignedTo != nil && *in.AssignedTo <= 0 || in.PlanID != nil && *in.PlanID <= 0 {
		respondJSON(w, 400, map[string]string{"error": "Positive plan/assignee IDs required; use clear_fields for optional assignment"})
		return
	}

	preview := in.Preview || !in.ConfirmChange
	elevated := op == "complete" || op == "cancel" || op == "reopen" || op == "archive" || op == "restore"
	phrase := fmt.Sprintf("%s WAREHOUSE MAINTENANCE ORDER %d", strings.ToUpper(op), in.OrderID)
	if !preview && (in.ExpectedDeviceUpdatedAt == "" || op != "create" && in.ExpectedUpdatedAt == "" || elevated && in.ConfirmationText != phrase) {
		respondJSON(w, 428, map[string]string{"error": "Exact order/device versions and elevated record-bound confirmation required"})
		return
	}
	if len([]rune(in.Notes)) > 4000 || len([]rune(in.Resolution)) > 4000 {
		respondJSON(w, 400, map[string]string{"error": "Notes and resolution are bounded to 4000 characters"})
		return
	}
	if in.CostAmount != nil && !regexp.MustCompile(`^(0|[1-9][0-9]{0,9})(\.[0-9]{1,2})?$`).MatchString(*in.CostAmount) {
		respondJSON(w, 400, map[string]string{"error": "Cost must be an exact nonnegative decimal, maximum 9999999999.99"})
		return
	}
	tx, err := repository.GetSQLDB().BeginTx(r.Context(), nil)
	if err != nil {
		respondMaintenanceMCPError(w, err)
		return
	}
	defer tx.Rollback()
	if _, err = tx.Exec(`SET LOCAL lock_timeout='5s'; SET LOCAL statement_timeout='15s'`); err != nil {
		respondMaintenanceMCPError(w, err)
		return
	}
	var receipt int64
	if !preview {
		var replay json.RawMessage
		receipt, replay, err = beginWarehouseProductMutation(tx, r, "maintenance_order."+mux.Vars(r)["entity"]+"."+op, in)
		if err != nil {
			respondMaintenanceMCPError(w, err)
			return
		}
		if replay != nil {
			respondJSON(w, 200, replay)
			return
		}
	}
	if _, err = tx.Exec(`LOCK TABLE devices,maintenance_plans,maintenance_orders,maintenance_order_events,defect_reports IN SHARE ROW EXCLUSIVE MODE; LOCK TABLE products,users IN SHARE MODE`); err != nil {
		respondMaintenanceMCPError(w, err)
		return
	}
	current := map[string]any{}
	draft := map[string]any{"device_id": "", "plan_id": nil, "order_type": "preventive", "priority": "normal", "status": "open", "title": "", "description": "", "due_at": nil, "scheduled_at": nil, "assigned_to": nil, "cost_amount": nil, "is_archived": false, "outcome": nil, "resolution": nil}
	required := []string{}
	if op != "create" {
		current, err = loadMaintenanceMCPJSON(tx, maintenanceMCPOrderSelect, in.OrderID)
		if err == sql.ErrNoRows {
			respondJSON(w, 404, map[string]string{"error": "Maintenance order not found"})
			return
		}
		if err != nil {
			respondMaintenanceMCPError(w, err)
			return
		}
		for key := range draft {
			draft[key] = current[key]
		}
		if in.ExpectedUpdatedAt != "" && in.ExpectedUpdatedAt != current["updated_at"] {
			required = append(required, "expected_updated_at")
		}
	}
	if defect && op == "create" {
		draft["order_type"] = "defect"
	}
	if defect && draft["order_type"] != "defect" {
		respondJSON(w, 400, map[string]string{"error": "Defect tools require a canonical defect order ID"})
		return
	}
	if op == "create" || op == "update" {
		for key, value := range map[string]*string{"device_id": in.DeviceID, "order_type": in.OrderType, "priority": in.Priority, "title": in.Title, "description": in.Description, "due_at": in.DueAt, "scheduled_at": in.ScheduledAt, "cost_amount": in.CostAmount} {
			if value != nil {
				draft[key] = *value
			}
		}
		if in.PlanID != nil {
			draft["plan_id"] = *in.PlanID
		}
		if in.AssignedTo != nil {
			draft["assigned_to"] = *in.AssignedTo
		}
		supplied := map[string]bool{"description": in.Description != nil, "due_at": in.DueAt != nil, "scheduled_at": in.ScheduledAt != nil, "assigned_to": in.AssignedTo != nil, "cost_amount": in.CostAmount != nil}
		seen := map[string]bool{}
		for _, key := range in.ClearFields {
			if _, ok := supplied[key]; !ok || supplied[key] || seen[key] {
				respondJSON(w, 400, map[string]string{"error": "Invalid, repeated or conflicting clear_fields"})
				return
			}
			seen[key] = true
			draft[key] = nil
			if key == "description" {
				draft[key] = ""
			}
		}
		if op == "update" && (draft["device_id"] != current["device_id"] || draft["order_type"] != current["order_type"] || maintenanceMCPID(draft["plan_id"]) != maintenanceMCPID(current["plan_id"])) {
			respondJSON(w, 400, map[string]string{"error": "Device, type and plan association are immutable"})
			return
		}
	} else if in.DeviceID != nil || in.PlanID != nil || in.OrderType != nil || in.Priority != nil || in.Title != nil || in.Description != nil || in.DueAt != nil || in.ScheduledAt != nil || in.AssignedTo != nil || len(in.ClearFields) > 0 {
		respondJSON(w, 400, map[string]string{"error": "Use metadata update separately from lifecycle/status"})
		return
	}
	if in.CostAmount != nil && op != "create" && op != "update" && op != "complete" {
		respondJSON(w, 400, map[string]string{"error": "Cost is only editable on create/update/complete"})
		return
	}
	if in.CostAmount != nil && op == "complete" {
		draft["cost_amount"] = *in.CostAmount
	}
	kind, _ := draft["order_type"].(string)
	device, _ := draft["device_id"].(string)
	title, _ := draft["title"].(string)
	description, _ := draft["description"].(string)
	priority, _ := draft["priority"].(string)
	if device == "" || device != strings.TrimSpace(device) || len(device) > 50 || strings.TrimSpace(title) == "" || len([]rune(title)) > 200 || len([]rune(description)) > 4000 || !maintenanceOrderTypes[kind] || !maintenancePriorities[priority] || defect && kind != "defect" {
		respondJSON(w, 400, map[string]string{"error": "Exact device, type, priority and bounded nonempty title required"})
		return
	}
	draft["title"] = strings.TrimSpace(title)
	for _, key := range []string{"due_at", "scheduled_at"} {
		if v, ok := draft[key].(string); ok {
			if v == "" {
				draft[key] = nil
				continue
			}
			format := "2006-01-02"
			if key == "scheduled_at" {
				format = time.RFC3339Nano
			}
			parsed, parseErr := time.Parse(format, v)
			if parseErr != nil {
				respondJSON(w, 400, map[string]string{"error": "Valid YYYY-MM-DD due date or RFC3339 scheduled time required"})
				return
			}
			if key == "scheduled_at" {
				draft[key] = parsed.UTC().Format("2006-01-02T15:04:05.000000Z")
			}
		}
	}
	state := fmt.Sprint(draft["status"])
	archived := draft["is_archived"] == true
	if op == "restore" {
		if !archived {
			required = append(required, "archived_order_required")
		}
		draft["is_archived"] = false
	} else if archived {
		required = append(required, "restore_before_change")
	}
	if op == "archive" {
		if state != "completed" && state != "cancelled" {
			required = append(required, "terminal_order_required")
		}
		draft["is_archived"] = true
	}
	if op == "update" && (state == "completed" || state == "cancelled") {
		required = append(required, "reopen_before_edit")
	}
	if op == "transition" {
		if !map[string]bool{"open": true, "planned": true, "in_progress": true, "waiting_parts": true}[in.Status] || !validMaintenanceTransition(state, in.Status) {
			required = append(required, "valid_status_transition")
		}
		draft["status"] = in.Status
	}
	if op == "complete" {
		if !validMaintenanceTransition(state, "completed") {
			required = append(required, "start_work_before_completion")
		}
		if !maintenanceOutcomes[in.Outcome] || strings.TrimSpace(in.Resolution) == "" {
			required = append(required, "outcome_and_resolution")
		}
		draft["status"] = "completed"
		draft["outcome"] = in.Outcome
		draft["resolution"] = strings.TrimSpace(in.Resolution)
	}
	if op == "cancel" {
		if !validMaintenanceTransition(state, "cancelled") {
			required = append(required, "valid_cancellation")
		}
		if strings.TrimSpace(in.Notes) == "" {
			required = append(required, "cancellation_reason")
		}
		draft["status"] = "cancelled"
	}
	if op == "reopen" {
		if state != "completed" && state != "cancelled" {
			required = append(required, "terminal_order_required")
		}
		if strings.TrimSpace(in.Notes) == "" {
			required = append(required, "reopen_reason")
		}
		draft["status"] = "open"
		draft["outcome"] = nil
		draft["resolution"] = nil
	}
	if op != "complete" && (in.Outcome != "" || in.Resolution != "" || in.NextDueAt != nil) || op != "transition" && in.Status != "" {
		respondJSON(w, 400, map[string]string{"error": "Outcome/resolution/next date only belong to complete; status only to transition"})
		return
	}
	fields, deviceVersion, life, physical, condition, _, _, err := loadDeviceForMCP(tx, device)
	if err == sql.ErrNoRows {
		respondJSON(w, 404, map[string]string{"error": "Device not found"})
		return
	}
	if err != nil {
		respondMaintenanceMCPError(w, err)
		return
	}
	if in.ExpectedDeviceUpdatedAt != "" && in.ExpectedDeviceUpdatedAt != deviceVersion {
		required = append(required, "expected_device_updated_at")
	}
	if op == "create" || op == "update" || op == "transition" || op == "reopen" {
		var active bool
		if err = tx.QueryRow(`SELECT EXISTS(SELECT 1 FROM products WHERE productid=$1 AND lifecycle_status='active')`, fields.ProductID).Scan(&active); err != nil {
			respondMaintenanceMCPError(w, err)
			return
		}
		if life != "active" || condition == "retired" || !active {
			required = append(required, "active_device_and_product_required")
		}
	}
	if assigned := maintenanceMCPID(draft["assigned_to"]); assigned != 0 && (op == "create" || op == "update" && in.AssignedTo != nil) {
		var active bool
		if assigned < 0 {
			respondJSON(w, 400, map[string]string{"error": "Positive assigned user ID required"})
			return
		}
		if err = tx.QueryRow(`SELECT EXISTS(SELECT 1 FROM users WHERE userid=$1 AND is_active)`, assigned).Scan(&active); err != nil {
			respondMaintenanceMCPError(w, err)
			return
		}
		if !active {
			required = append(required, "active_assignee_required")
		}
	}
	planID := maintenanceMCPID(draft["plan_id"])
	plan := map[string]any{}
	newPlanDue := any(nil)
	if planID != 0 {
		plan, err = loadMaintenanceMCPJSON(tx, maintenanceMCPPlanSelect, planID)
		if err == sql.ErrNoRows {
			respondJSON(w, 404, map[string]string{"error": "Maintenance plan not found"})
			return
		}
		if err != nil {
			respondMaintenanceMCPError(w, err)
			return
		}
		if plan["device_id"] != device || plan["maintenance_type"] != kind || kind == "defect" {
			required = append(required, "matching_device_plan_type")
		}
		if (op == "create" || op == "update" || op == "transition" || op == "reopen") && plan["is_active"] != true {
			required = append(required, "active_plan_required")
		}
		if in.ExpectedPlanUpdatedAt != "" && in.ExpectedPlanUpdatedAt != plan["updated_at"] {
			required = append(required, "expected_plan_updated_at")
		}
		if !preview && in.ExpectedPlanUpdatedAt == "" {
			respondJSON(w, 428, map[string]string{"error": "Exact linked plan version required"})
			return
		}
		if op == "complete" || op == "cancel" {
			if err = tx.QueryRow(`SELECT to_char(CASE WHEN $1='complete' THEN CURRENT_DATE+interval_days ELSE GREATEST(next_due_at+interval_days,CURRENT_DATE+interval_days) END,'YYYY-MM-DD') FROM maintenance_plans WHERE plan_id=$2`, op, planID).Scan(&newPlanDue); err != nil {
				respondMaintenanceMCPError(w, err)
				return
			}
		}
	}
	if planID < 0 {
		respondJSON(w, 400, map[string]string{"error": "Positive linked plan ID required"})
		return
	}
	newDeviceDue := any(fields.NextMaintenance)
	newLastMaintenance := any(fields.LastMaintenance)
	if op == "complete" {
		if in.NextDueAt != nil {
			date, e := parseMaintenanceDate(*in.NextDueAt)
			if e != nil || date == nil {
				respondJSON(w, 400, map[string]string{"error": "Valid nonempty next due date required"})
				return
			}
			newDeviceDue = date.Format("2006-01-02")
			if planID > 0 {
				newPlanDue = newDeviceDue
			}
		}
		var today string
		if err = tx.QueryRow(`SELECT CURRENT_DATE::text`).Scan(&today); err != nil {
			respondMaintenanceMCPError(w, err)
			return
		}
		newLastMaintenance = today
	}
	if planID > 0 && (op == "complete" || op == "cancel") {
		var other sql.NullString
		if err = tx.QueryRow(`SELECT to_char(min(next_due_at),'YYYY-MM-DD') FROM maintenance_plans WHERE device_id=$1 AND is_active AND plan_id<>$2`, device, planID).Scan(&other); err != nil {
			respondMaintenanceMCPError(w, err)
			return
		}
		newDeviceDue = newPlanDue
		if plan["is_active"] != true {
			newDeviceDue = nil
		}
		if other.Valid && (newDeviceDue == nil || other.String < fmt.Sprint(newDeviceDue)) {
			newDeviceDue = other.String
		}
	}
	newState := fmt.Sprint(draft["status"])
	newCondition := condition
	conditionChange := op == "create" && kind == "defect" || op == "transition" || op == "complete" || op == "cancel" || op == "reopen"
	if conditionChange {
		newCondition, err = maintenanceMCPCondition(tx, device, in.OrderID, kind, newState, in.Outcome, condition)
		if err != nil {
			respondMaintenanceMCPError(w, err)
			return
		}
	}
	var duplicates, planConflicts int
	if err = tx.QueryRow(`SELECT count(*) FILTER(WHERE lower(trim(title))=lower(trim($3)) AND order_type=$4),count(*) FILTER(WHERE plan_id=$5 AND $5<>0) FROM maintenance_orders WHERE device_id=$1 AND order_id<>$2 AND status NOT IN ('completed','cancelled')`, device, in.OrderID, draft["title"], kind, planID).Scan(&duplicates, &planConflicts); err != nil {
		respondMaintenanceMCPError(w, err)
		return
	}
	if (op == "create" || op == "reopen") && duplicates > 0 {
		required = append(required, "duplicate_open_order")
	}
	if newState != "completed" && newState != "cancelled" && planConflicts > 0 {
		required = append(required, "existing_open_plan_order")
	}
	diff := map[string]any{}
	for k, v := range draft {
		if !reflect.DeepEqual(current[k], v) {
			diff[k] = map[string]any{"before": current[k], "after": v}
		}
	}
	if op == "update" && len(diff) == 0 {
		required = append(required, "changed_fields")
	}
	deviceBefore := map[string]any{"device_id": device, "physical_status": physical, "condition_status": condition, "last_maintenance": fields.LastMaintenance, "next_maintenance": fields.NextMaintenance, "updated_at": deviceVersion}
	effects := map[string]any{"condition_status": map[string]any{"before": condition, "after": newCondition}, "last_maintenance": map[string]any{"before": fields.LastMaintenance, "after": newLastMaintenance}, "next_maintenance": map[string]any{"before": fields.NextMaintenance, "after": newDeviceDue}, "plan_next_due_at": newPlanDue, "duplicate_open_orders": duplicates, "open_plan_orders": planConflicts, "physical_status": physical, "legacy_defect_id": current["legacy_defect_id"]}
	legacySnapshot := map[string]any{}
	if legacy := maintenanceMCPID(current["legacy_defect_id"]); legacy > 0 {
		legacySnapshot, err = loadMaintenanceMCPJSON(tx, `SELECT to_jsonb(d) FROM defect_reports d WHERE defect_id=$1 FOR UPDATE`, legacy)
		if err != nil {
			respondMaintenanceMCPError(w, err)
			return
		}
	}

	result := map[string]any{"operation_status": "confirmation_required", "ready_to_execute": len(required) == 0, "current": current, "draft": draft, "diff": diff, "device": deviceBefore, "plan": plan, "effects": effects, "legacy_defect": legacySnapshot, "expected_updated_at": current["updated_at"], "expected_device_updated_at": deviceVersion, "expected_plan_updated_at": plan["updated_at"], "required_fields": required, "preview": true}
	if elevated {
		result["required_confirmation_text"] = phrase
	}
	if preview || len(required) > 0 {
		if len(required) > 0 {
			result["operation_status"] = "needs_input"
		}
		respondJSON(w, 200, result)
		return
	}
	id := in.OrderID
	if op == "create" {
		err = tx.QueryRow(`INSERT INTO maintenance_orders(device_id,plan_id,order_type,priority,status,title,description,due_at,scheduled_at,reported_by,assigned_to,cost) VALUES($1,$2,$3,$4,'open',$5,$6,$7,$8,$9,$10,$11::numeric) RETURNING order_id`, device, draft["plan_id"], kind, priority, draft["title"], maintenanceNullableString(description), draft["due_at"], draft["scheduled_at"], user.UserID, draft["assigned_to"], draft["cost_amount"]).Scan(&id)
	} else if op == "archive" || op == "restore" {
		_, err = tx.Exec(`UPDATE maintenance_orders SET is_archived=$1 WHERE order_id=$2`, draft["is_archived"], id)
	} else if op == "update" {
		_, err = tx.Exec(`UPDATE maintenance_orders SET priority=$1,title=$2,description=$3,due_at=$4,scheduled_at=$5,assigned_to=$6,cost=$7::numeric WHERE order_id=$8`, priority, draft["title"], maintenanceNullableString(description), draft["due_at"], draft["scheduled_at"], draft["assigned_to"], draft["cost_amount"], id)
	} else {
		_, err = tx.Exec(`UPDATE maintenance_orders SET status=$1::text,started_at=CASE WHEN $1::text='in_progress' THEN COALESCE(started_at,CURRENT_TIMESTAMP) WHEN $5='reopen' THEN NULL ELSE started_at END,completed_at=CASE WHEN $1::text IN ('completed','cancelled') THEN CURRENT_TIMESTAMP WHEN $5='reopen' THEN NULL ELSE completed_at END,outcome=$2,resolution=$3,cost=$4::numeric WHERE order_id=$6`, newState, draft["outcome"], draft["resolution"], draft["cost_amount"], op, id)
	}
	if err != nil {
		respondMaintenanceMCPError(w, err)
		return
	}
	if _, err = tx.Exec(`INSERT INTO maintenance_order_events(order_id,event_type,from_status,to_status,notes,actor_id) VALUES($1,$2,$3,$4,$5,$6)`, id, "mcp_"+op, current["status"], newState, maintenanceNullableString(in.Notes), user.UserID); err != nil {
		respondMaintenanceMCPError(w, err)
		return
	}
	// Keep migrated defects and their archive dependencies consistent. Both
	// legacy schemas accept closed; retained rows and links are never deleted.
	if legacy := maintenanceMCPID(current["legacy_defect_id"]); legacy > 0 && (op == "complete" || op == "cancel" || op == "reopen") {
		beforeLegacy, e := loadMaintenanceMCPJSON(tx, `SELECT to_jsonb(d) FROM defect_reports d WHERE defect_id=$1 FOR UPDATE`, legacy)
		if e != nil {
			respondMaintenanceMCPError(w, e)
			return
		}
		legacyState := "closed"
		if op == "reopen" {
			legacyState = "open"
		}
		legacySQL := "UPDATE defect_reports SET status=$1::text"
		if _, ok := beforeLegacy["updated_at"]; ok {
			legacySQL += ",updated_at=clock_timestamp() AT TIME ZONE 'UTC'"
		}
		report := in.Resolution
		if op == "cancel" || op == "reopen" {
			report = in.Notes
		}
		args := []any{legacyState, legacy, report}
		if _, ok := beforeLegacy["resolved_at"]; ok {
			legacySQL += ",resolved_at=CASE WHEN $1::text='closed' THEN CURRENT_TIMESTAMP ELSE NULL END,resolution=$3"
		} else {
			if _, ok := beforeLegacy["closed_at"]; ok {
				legacySQL += ",closed_at=CASE WHEN $1::text='closed' THEN CURRENT_TIMESTAMP ELSE NULL END"
			}
			if _, ok := beforeLegacy["repaired_at"]; ok {
				legacySQL += ",repaired_at=CASE WHEN $1::text='closed' THEN CURRENT_TIMESTAMP ELSE NULL END"
			}
			if _, ok := beforeLegacy["repair_notes"]; ok {
				legacySQL += ",repair_notes=$3"
			} else {
				args = args[:2]
			}
		}
		legacySQL += " WHERE defect_id=$2"
		if _, err = tx.Exec(legacySQL, args...); err != nil {
			respondMaintenanceMCPError(w, err)
			return
		}
		afterLegacy, e := loadMaintenanceMCPJSON(tx, `SELECT to_jsonb(d) FROM defect_reports d WHERE defect_id=$1`, legacy)
		if e != nil {
			respondMaintenanceMCPError(w, e)
			return
		}
		if _, err = maintenanceMCPAudit(tx, r, "maintenance_order.sync_legacy_defect", "defect", fmt.Sprint(legacy), beforeLegacy, afterLegacy, fmt.Sprint(afterLegacy["updated_at"])); err != nil {
			respondMaintenanceMCPError(w, err)
			return
		}
	}
	if planID > 0 && (op == "complete" || op == "cancel") {
		_, err = tx.Exec(`UPDATE maintenance_plans SET next_due_at=$1::date,last_completed_at=CASE WHEN $2='complete' THEN CURRENT_TIMESTAMP ELSE last_completed_at END WHERE plan_id=$3`, newPlanDue, op, planID)
		if err != nil {
			respondMaintenanceMCPError(w, err)
			return
		}
	}
	if conditionChange || op == "complete" || planID > 0 && op == "cancel" {
		_, err = tx.Exec(`UPDATE devices SET condition_status=$1,lastmaintenance=$2::date,nextmaintenance=$3::date WHERE deviceid=$4`, newCondition, newLastMaintenance, newDeviceDue, device)
		if err != nil {
			respondMaintenanceMCPError(w, err)
			return
		}
	}
	if planID > 0 {
		after, e := loadMaintenanceMCPJSON(tx, maintenanceMCPPlanSelect, planID)
		if e != nil {
			respondMaintenanceMCPError(w, e)
			return
		}
		if _, err = maintenanceMCPAudit(tx, r, "maintenance_order.sync_plan", "maintenance_plan", fmt.Sprint(planID), plan, after, fmt.Sprint(after["updated_at"])); err != nil {
			respondMaintenanceMCPError(w, err)
			return
		}
		plan = after
	}
	after, err := loadMaintenanceMCPJSON(tx, maintenanceMCPOrderSelect, id)
	if err != nil {
		respondMaintenanceMCPError(w, err)
		return
	}
	deviceFields, newDeviceVersion, _, _, newDeviceCondition, _, _, err := loadDeviceForMCP(tx, device)
	if err != nil {
		respondMaintenanceMCPError(w, err)
		return
	}
	deviceAfter := map[string]any{"device_id": device, "physical_status": physical, "condition_status": newDeviceCondition, "last_maintenance": deviceFields.LastMaintenance, "next_maintenance": deviceFields.NextMaintenance, "updated_at": newDeviceVersion}
	if newDeviceVersion != deviceVersion {
		if _, err = maintenanceMCPAudit(tx, r, "maintenance_order.sync_device", "device", device, deviceBefore, deviceAfter, newDeviceVersion); err != nil {
			respondMaintenanceMCPError(w, err)
			return
		}
	}
	auditID, err := maintenanceMCPAudit(tx, r, "maintenance_order."+op, "maintenance_order", fmt.Sprint(id), current, after, fmt.Sprint(after["updated_at"]))
	if err != nil {
		respondMaintenanceMCPError(w, err)
		return
	}
	result = map[string]any{"operation_status": map[string]string{"create": "created", "update": "updated", "transition": "transitioned", "complete": "completed", "cancel": "cancelled", "reopen": "reopened", "archive": "archived", "restore": "restored"}[op], "order": after, "device": deviceAfter, "plan": plan, "audit_id": auditID}
	if err = completeWarehouseProductMutation(tx, receipt, 200, result); err == nil {
		err = tx.Commit()
	}
	if err != nil {
		respondMaintenanceMCPError(w, err)
		return
	}
	respondJSON(w, 200, result)
}

func respondMaintenanceMCPError(w http.ResponseWriter, err error) {
	log.Printf("[WAREHOUSE MCP] operation failed: %v", err)
	if _, ok := err.(*warehouseMutationError); ok {
		respondWarehouseMutationError(w, err)
		return
	}
	respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Warehouse operation failed"})
}
