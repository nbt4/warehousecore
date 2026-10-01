package handlers

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"reflect"
	"strings"

	"github.com/gorilla/mux"
	"warehousecore/internal/middleware"
	"warehousecore/internal/repository"
)

type warehouseMaintenancePlanRequest struct {
	PlanID                  int64    `json:"plan_id"`
	DeviceID                *string  `json:"device_id"`
	Name                    *string  `json:"name"`
	MaintenanceType         *string  `json:"maintenance_type"`
	IntervalDays            *int     `json:"interval_days"`
	LeadTimeDays            *int     `json:"lead_time_days"`
	Instructions            *string  `json:"instructions"`
	NextDueAt               *string  `json:"next_due_at"`
	ClearFields             []string `json:"clear_fields"`
	ExpectedUpdatedAt       string   `json:"expected_updated_at"`
	ExpectedDeviceUpdatedAt string   `json:"expected_device_updated_at"`
	ConfirmChange           bool     `json:"confirm_change"`
	ConfirmationText        string   `json:"confirmation_text"`
	Preview                 bool     `json:"preview"`
}

const maintenanceMCPPlanSelect = `SELECT jsonb_build_object('plan_id',plan_id,'device_id',device_id,'name',name,'maintenance_type',maintenance_type,'interval_days',interval_days,'lead_time_days',lead_time_days,'instructions',COALESCE(instructions,''),'next_due_at',to_char(next_due_at,'YYYY-MM-DD'),'last_completed_at',last_completed_at,'is_active',is_active,'created_by',created_by,'created_at',created_at,'updated_at',to_char(updated_at,'YYYY-MM-DD"T"HH24:MI:SS.US"Z"')) FROM maintenance_plans WHERE plan_id=$1 FOR UPDATE`

func maintenanceMCPAudit(tx *sql.Tx, r *http.Request, action, entity, id string, before, after any, version string) (int64, error) {
	user, _ := middleware.GetUserFromContext(r)
	oldJSON, err := json.Marshal(before)
	if err != nil {
		return 0, err
	}
	newJSON, err := json.Marshal(map[string]any{"origin": "MCP/AI", "after": after, "updated_at": version})
	if err != nil {
		return 0, err
	}
	var auditID int64
	err = tx.QueryRow(`INSERT INTO audit_log(user_id,action,entity_type,entity_id,old_values,new_values,user_agent) VALUES($1,$2,$3,$4,$5::jsonb,$6::jsonb,$7) RETURNING id`, user.UserID, action, entity, id, string(oldJSON), string(newJSON), r.UserAgent()).Scan(&auditID)
	return auditID, err
}

func WarehouseMaintenancePlanMCP(w http.ResponseWriter, r *http.Request) {
	user, ok := middleware.GetUserFromContext(r)
	if !ok || user == nil || user.UserID == 0 || !user.IsAdmin || !isWarehouseMCPMutation(r) {
		respondJSON(w, 403, map[string]string{"error": "Signed-in warehouse administrator and MCP origin required"})
		return
	}
	op := mux.Vars(r)["operation"]
	if !map[string]bool{"create": true, "update": true, "archive": true, "restore": true}[op] {
		respondJSON(w, 400, map[string]string{"error": "Unsupported maintenance plan operation"})
		return
	}
	var in warehouseMaintenancePlanRequest
	d := json.NewDecoder(http.MaxBytesReader(w, r.Body, 65536))
	d.DisallowUnknownFields()
	if err := d.Decode(&in); err != nil {
		respondJSON(w, 400, map[string]string{"error": "Invalid bounded maintenance plan body"})
		return
	}
	if err := d.Decode(new(any)); err != io.EOF || op != "create" && in.PlanID <= 0 || op == "create" && in.PlanID != 0 {
		respondJSON(w, 400, map[string]string{"error": "One JSON object and the exact operation-specific plan ID required"})
		return
	}
	preview := in.Preview || !in.ConfirmChange
	phrase := fmt.Sprintf("%s WAREHOUSE MAINTENANCE PLAN %d", strings.ToUpper(op), in.PlanID)
	if !preview && (in.ExpectedDeviceUpdatedAt == "" || op != "create" && in.ExpectedUpdatedAt == "" || (op == "archive" || op == "restore") && in.ConfirmationText != phrase) {
		respondJSON(w, 428, map[string]string{"error": "Exact plan/device versions and lifecycle confirmation phrase required"})
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
	var receipt int64
	if !preview {
		var replay json.RawMessage
		receipt, replay, err = beginWarehouseProductMutation(tx, r, "maintenance_plan."+op, in)
		if err != nil {
			respondWarehouseMutationError(w, err)
			return
		}
		if replay != nil {
			respondJSON(w, 200, replay)
			return
		}
	}
	if _, err = tx.Exec(`LOCK TABLE devices,maintenance_plans,maintenance_orders IN SHARE ROW EXCLUSIVE MODE; LOCK TABLE products IN SHARE MODE`); err != nil {
		respondWarehouseMutationError(w, err)
		return
	}
	current := map[string]any{}
	fields := maintenancePlanInput{MaintenanceType: "preventive", LeadTimeDays: 14}
	active := true
	required := []string{}
	if op != "create" {
		var raw []byte
		if err = tx.QueryRow(maintenanceMCPPlanSelect, in.PlanID).Scan(&raw); err == sql.ErrNoRows {
			respondJSON(w, 404, map[string]string{"error": "Maintenance plan not found"})
			return
		} else if err != nil {
			respondWarehouseMutationError(w, err)
			return
		}
		if err = json.Unmarshal(raw, &current); err != nil {
			respondWarehouseMutationError(w, err)
			return
		}
		if err = json.Unmarshal(raw, &fields); err != nil {
			respondWarehouseMutationError(w, err)
			return
		}
		active = current["is_active"].(bool)
		if in.ExpectedUpdatedAt != "" && in.ExpectedUpdatedAt != current["updated_at"] {
			required = append(required, "expected_updated_at")
		}
		if op == "restore" && active || op != "restore" && !active {
			required = append(required, "plan_lifecycle")
		}
	}
	original := fields
	if in.DeviceID != nil {
		if op != "create" && *in.DeviceID != fields.DeviceID {
			respondJSON(w, 400, map[string]string{"error": "Plan device assignment is immutable"})
			return
		}
		fields.DeviceID = *in.DeviceID
	}
	if in.Name != nil {
		fields.Name = *in.Name
	}
	if in.MaintenanceType != nil {
		fields.MaintenanceType = *in.MaintenanceType
	}
	if in.IntervalDays != nil {
		fields.IntervalDays = *in.IntervalDays
	}
	if in.LeadTimeDays != nil {
		fields.LeadTimeDays = *in.LeadTimeDays
	}
	if in.Instructions != nil {
		fields.Instructions = *in.Instructions
	}
	if in.NextDueAt != nil {
		fields.NextDueAt = *in.NextDueAt
	}
	for _, key := range in.ClearFields {
		if key != "instructions" || in.Instructions != nil {
			respondJSON(w, 400, map[string]string{"error": "Only instructions can be cleared; do not also supply it"})
			return
		}
		fields.Instructions = ""
	}
	if op == "archive" || op == "restore" {
		if in.Name != nil || in.MaintenanceType != nil || in.IntervalDays != nil || in.LeadTimeDays != nil || in.Instructions != nil || in.NextDueAt != nil || len(in.ClearFields) > 0 {
			respondJSON(w, 400, map[string]string{"error": "Lifecycle preserves plan fields; use update separately"})
			return
		}
		active = op == "restore"
	}
	nextDue, err := validateMaintenancePlanInput(&fields)
	if err != nil || len([]rune(fields.Name)) > 160 || len([]rune(fields.Instructions)) > 4000 || len([]rune(fields.DeviceID)) > 50 || in.DeviceID != nil && strings.TrimSpace(*in.DeviceID) != *in.DeviceID {
		if err == nil {
			err = fmt.Errorf("Invalid plan text or exact device ID")
		}
		respondJSON(w, 400, map[string]string{"error": err.Error()})
		return
	}
	fields.NextDueAt = nextDue.Format("2006-01-02")
	if op == "archive" || op == "restore" {
		fields = original
	}
	if op == "update" && reflect.DeepEqual(fields, original) {
		required = append(required, "changed_fields")
	}
	deviceFields, deviceVersion, deviceLife, physical, condition, _, _, err := loadDeviceForMCP(tx, fields.DeviceID)
	if err == sql.ErrNoRows {
		respondJSON(w, 404, map[string]string{"error": "Device not found"})
		return
	} else if err != nil {
		respondWarehouseMutationError(w, err)
		return
	}
	if in.ExpectedDeviceUpdatedAt != "" && in.ExpectedDeviceUpdatedAt != deviceVersion {
		required = append(required, "expected_device_updated_at")
	}
	if op != "archive" && (deviceLife != "active" || condition == "retired") {
		required = append(required, "active_device_required")
	}
	var activeProduct bool
	if err = tx.QueryRow(`SELECT EXISTS(SELECT 1 FROM products WHERE productid=$1 AND lifecycle_status='active')`, deviceFields.ProductID).Scan(&activeProduct); err != nil {
		respondWarehouseMutationError(w, err)
		return
	}
	if op != "archive" && !activeProduct {
		required = append(required, "active_product_required")
	}
	var openOrders, allOrders int64
	if err = tx.QueryRow(`SELECT count(*) FILTER(WHERE status NOT IN ('completed','cancelled')),count(*) FROM maintenance_orders WHERE plan_id=$1`, in.PlanID).Scan(&openOrders, &allOrders); err != nil {
		respondWarehouseMutationError(w, err)
		return
	}
	if (op == "archive" || op == "restore") && openOrders > 0 {
		required = append(required, "open_maintenance_orders")
	}
	var duplicate bool
	if err = tx.QueryRow(`SELECT EXISTS(SELECT 1 FROM maintenance_plans WHERE device_id=$1 AND plan_id<>$2 AND lower(trim(name))=lower(trim($3)))`, fields.DeviceID, in.PlanID, fields.Name).Scan(&duplicate); err != nil {
		respondWarehouseMutationError(w, err)
		return
	}
	if duplicate {
		required = append(required, "duplicate_plan")
	}
	var due bool
	if err = tx.QueryRow(`SELECT $1::date<=CURRENT_DATE+$2::int`, nextDue, fields.LeadTimeDays).Scan(&due); err != nil {
		respondWarehouseMutationError(w, err)
		return
	}
	generate := active && due && openOrders == 0
	var otherDue sql.NullString
	if err = tx.QueryRow(`SELECT to_char(min(next_due_at),'YYYY-MM-DD') FROM maintenance_plans WHERE device_id=$1 AND is_active AND plan_id<>$2`, fields.DeviceID, in.PlanID).Scan(&otherDue); err != nil {
		respondWarehouseMutationError(w, err)
		return
	}
	var effectiveDue *string
	if otherDue.Valid {
		effectiveDue = &otherDue.String
	}
	if active && (effectiveDue == nil || fields.NextDueAt < *effectiveDue) {
		effectiveDue = &fields.NextDueAt
	}
	draft := map[string]any{"device_id": fields.DeviceID, "name": fields.Name, "maintenance_type": fields.MaintenanceType, "interval_days": fields.IntervalDays, "lead_time_days": fields.LeadTimeDays, "instructions": fields.Instructions, "next_due_at": fields.NextDueAt, "is_active": active}
	diff := map[string]any{}
	for key, value := range draft {
		if !reflect.DeepEqual(current[key], value) {
			diff[key] = map[string]any{"before": current[key], "after": value}
		}
	}
	deviceBefore := map[string]any{"device_id": fields.DeviceID, "next_maintenance": deviceFields.NextMaintenance, "condition_status": condition, "physical_status": physical, "updated_at": deviceVersion}
	effects := map[string]any{"device_next_maintenance": map[string]any{"before": deviceFields.NextMaintenance, "after": effectiveDue}, "generate_planned_order": generate, "open_orders": openOrders, "historical_and_active_orders": allOrders}
	if generate {
		effects["planned_order_draft"] = map[string]any{"device_id": fields.DeviceID, "order_type": fields.MaintenanceType, "title": fields.Name, "description": fields.Instructions, "due_at": fields.NextDueAt, "status": "planned", "priority": "normal"}
	}
	result := map[string]any{"operation_status": "confirmation_required", "ready_to_execute": len(required) == 0, "current": current, "draft": draft, "diff": diff, "effects": effects, "device": deviceBefore, "expected_device_updated_at": deviceVersion, "expected_updated_at": current["updated_at"], "required_fields": required, "preview": true}
	if op == "archive" || op == "restore" {
		result["required_confirmation_text"] = phrase
	}
	if preview || len(required) > 0 {
		if len(required) > 0 {
			result["operation_status"] = "needs_input"
		}
		respondJSON(w, 200, result)
		return
	}
	id := in.PlanID
	if op == "create" {
		err = tx.QueryRow(`INSERT INTO maintenance_plans(device_id,name,maintenance_type,interval_days,lead_time_days,instructions,next_due_at,is_active,created_by) VALUES($1,$2,$3,$4,$5,$6,$7,true,$8) RETURNING plan_id`, fields.DeviceID, fields.Name, fields.MaintenanceType, fields.IntervalDays, fields.LeadTimeDays, maintenanceNullableString(fields.Instructions), nextDue, user.UserID).Scan(&id)
	} else if op == "archive" || op == "restore" {
		_, err = tx.Exec(`UPDATE maintenance_plans SET is_active=$1 WHERE plan_id=$2`, active, id)
	} else {
		_, err = tx.Exec(`UPDATE maintenance_plans SET name=$1,maintenance_type=$2,interval_days=$3,lead_time_days=$4,instructions=$5,next_due_at=$6,is_active=$7 WHERE plan_id=$8`, fields.Name, fields.MaintenanceType, fields.IntervalDays, fields.LeadTimeDays, maintenanceNullableString(fields.Instructions), nextDue, active, id)
	}
	if err != nil {
		respondWarehouseMutationError(w, err)
		return
	}
	generated := map[string]any{}
	if generate {
		var orderID int64
		err = tx.QueryRow(`INSERT INTO maintenance_orders(device_id,plan_id,order_type,priority,status,title,description,due_at,reported_by) VALUES($1,$2,$3,'normal','planned',$4,$5,$6,$7) RETURNING order_id`, fields.DeviceID, id, fields.MaintenanceType, fields.Name, maintenanceNullableString(fields.Instructions), nextDue, user.UserID).Scan(&orderID)
		if err != nil {
			respondWarehouseMutationError(w, err)
			return
		}
		if _, err = tx.Exec(`INSERT INTO maintenance_order_events(order_id,event_type,to_status,notes,actor_id) VALUES($1,'auto_created','planned','MCP/AI: created atomically with maintenance plan',$2)`, orderID, user.UserID); err != nil {
			respondWarehouseMutationError(w, err)
			return
		}
		var orderVersion string
		if err = tx.QueryRow(`SELECT to_char(updated_at,'YYYY-MM-DD"T"HH24:MI:SS.US"Z"') FROM maintenance_orders WHERE order_id=$1`, orderID).Scan(&orderVersion); err != nil {
			respondWarehouseMutationError(w, err)
			return
		}
		generated = effects["planned_order_draft"].(map[string]any)
		generated["order_id"] = orderID
		generated["plan_id"] = id
		generated["updated_at"] = orderVersion
		auditID, auditErr := maintenanceMCPAudit(tx, r, "maintenance_order.auto_create", "maintenance_order", fmt.Sprint(orderID), nil, generated, orderVersion)
		if auditErr != nil {
			respondWarehouseMutationError(w, auditErr)
			return
		}
		generated["audit_id"] = auditID
	}
	if err = syncDeviceNextMaintenance(tx, fields.DeviceID); err != nil {
		respondWarehouseMutationError(w, err)
		return
	}
	_, newDeviceVersion, _, _, _, _, _, err := loadDeviceForMCP(tx, fields.DeviceID)
	if err != nil {
		respondWarehouseMutationError(w, err)
		return
	}
	deviceAfter := map[string]any{"device_id": fields.DeviceID, "next_maintenance": effectiveDue, "condition_status": condition, "physical_status": physical, "updated_at": newDeviceVersion}
	if _, err = maintenanceMCPAudit(tx, r, "maintenance_plan.sync_device", "device", fields.DeviceID, deviceBefore, deviceAfter, newDeviceVersion); err != nil {
		respondWarehouseMutationError(w, err)
		return
	}
	var raw []byte
	if err = tx.QueryRow(maintenanceMCPPlanSelect, id).Scan(&raw); err != nil {
		respondWarehouseMutationError(w, err)
		return
	}
	after := map[string]any{}
	if err = json.Unmarshal(raw, &after); err != nil {
		respondWarehouseMutationError(w, err)
		return
	}
	var before any
	if op != "create" {
		before = current
	}
	auditID, err := maintenanceMCPAudit(tx, r, "maintenance_plan."+op, "maintenance_plan", fmt.Sprint(id), before, after, after["updated_at"].(string))
	if err != nil {
		respondWarehouseMutationError(w, err)
		return
	}
	result = map[string]any{"operation_status": map[string]string{"create": "created", "update": "updated", "archive": "archived", "restore": "restored"}[op], "plan": after, "device": deviceAfter, "generated_order": generated, "audit_id": auditID}
	if err = completeWarehouseProductMutation(tx, receipt, 200, result); err == nil {
		err = tx.Commit()
	}
	if err != nil {
		respondWarehouseMutationError(w, err)
		return
	}
	respondJSON(w, 200, result)
}
