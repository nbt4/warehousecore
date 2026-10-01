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
	"time"

	"github.com/gorilla/mux"
	"warehousecore/internal/middleware"
	"warehousecore/internal/repository"
)

type warehouseTaskMCPRequest struct {
	TaskID             int64             `json:"task_id"`
	TaskType           *string           `json:"task_type"`
	Priority           *int              `json:"priority"`
	FromZoneID         *int64            `json:"from_zone_id"`
	ToZoneID           *int64            `json:"to_zone_id"`
	CaseID             *int64            `json:"case_id"`
	DeviceID           *string           `json:"device_id"`
	ProductID          *int64            `json:"product_id"`
	Quantity           *float64          `json:"quantity"`
	JobID              *int64            `json:"job_id"`
	AssignedTo         *int64            `json:"assigned_to"`
	DueAt              *string           `json:"due_at"`
	Notes              *string           `json:"notes"`
	ClearFields        []string          `json:"clear_fields"`
	Reason             string            `json:"reason"`
	ExpectedUpdatedAt  string            `json:"expected_updated_at"`
	ExpectedReferences map[string]string `json:"expected_references"`
	ConfirmCreation    bool              `json:"confirm_creation"`
	ConfirmChange      bool              `json:"confirm_change"`
	ConfirmationText   string            `json:"confirmation_text"`
	Preview            bool              `json:"preview"`
}

const warehouseTaskMCPSelect = `SELECT jsonb_build_object('task_id',task_id,'task_type',task_type,'status',status,'priority',priority,'from_zone_id',from_zone_id,'to_zone_id',to_zone_id,'case_id',case_id,'device_id',device_id,'product_id',product_id,'quantity',quantity,'job_id',job_id,'assigned_to',assigned_to,'due_at',to_char(due_at,'YYYY-MM-DD"T"HH24:MI:SS.US"Z"'),'notes',COALESCE(notes,''),'started_at',started_at,'completed_at',completed_at,'is_archived',is_archived,'archived_at',archived_at,'created_at',created_at,'updated_at',to_char(updated_at,'YYYY-MM-DD"T"HH24:MI:SS.US"Z"')) FROM warehouse_tasks WHERE task_id=$1 FOR UPDATE`

type taskMCPReferenceSpec struct{ entity, query string }

var taskMCPReferences = map[string]taskMCPReferenceSpec{
	"from_zone_id": {"location", `SELECT jsonb_build_object('zone_id',zone_id,'name',name,'code',code,'is_active',is_active,'operational_status',operational_status,'updated_at',to_char(updated_at,'YYYY-MM-DD"T"HH24:MI:SS.US"Z"')) FROM storage_zones WHERE zone_id=$1`},
	"to_zone_id":   {"location", `SELECT jsonb_build_object('zone_id',zone_id,'name',name,'code',code,'is_active',is_active,'operational_status',operational_status,'updated_at',to_char(updated_at,'YYYY-MM-DD"T"HH24:MI:SS.US"Z"')) FROM storage_zones WHERE zone_id=$1`},
	"case_id":      {"case", `SELECT jsonb_build_object('case_id',caseid,'name',name,'lifecycle_status',lifecycle_status,'zone_id',zone_id,'updated_at',to_char(updated_at,'YYYY-MM-DD"T"HH24:MI:SS.US"Z"')) FROM cases WHERE caseid=$1`},
	"device_id":    {"device", `SELECT jsonb_build_object('device_id',d.deviceid,'product_id',d.productid,'lifecycle_status',d.lifecycle_status,'condition_status',d.condition_status,'physical_status',d.status,'zone_id',d.zone_id,'case_id',d.current_case_id,'product_lifecycle',p.lifecycle_status,'product_updated_at',to_char(p.updated_at,'YYYY-MM-DD"T"HH24:MI:SS.US"Z"'),'updated_at',to_char(d.updated_at,'YYYY-MM-DD"T"HH24:MI:SS.US"Z"')) FROM devices d JOIN products p ON p.productid=d.productid WHERE d.deviceid=$1`},
	"product_id":   {"product", `SELECT jsonb_build_object('product_id',productid,'name',name,'lifecycle_status',lifecycle_status,'tracking_mode',tracking_mode,'updated_at',to_char(updated_at,'YYYY-MM-DD"T"HH24:MI:SS.US"Z"')) FROM products WHERE productid=$1`},
	"job_id":       {"job", `SELECT jsonb_build_object('job_id',j.jobid,'status',s.status,'closed',warehouse_job_status_is_closed(s.status),'deleted_at',j.deleted_at,'updated_at',to_char(j.updated_at,'YYYY-MM-DD"T"HH24:MI:SS.US"Z"')) FROM jobs j LEFT JOIN status s ON s.statusid=j.statusid WHERE j.jobid=$1`},
	"assigned_to":  {"user_assignment", `SELECT jsonb_build_object('user_id',userid,'username',username,'is_active',is_active,'updated_at',to_char(updated_at,'YYYY-MM-DD"T"HH24:MI:SS.US"Z"')) FROM users WHERE userid=$1`},
}

func loadTaskMCPReferences(tx *sql.Tx, draft, current map[string]any) (map[string]any, map[string]string, error) {
	snapshots := map[string]any{}
	versions := map[string]string{}
	for field, spec := range taskMCPReferences {
		values := map[string]any{field: draft[field]}
		if current[field] != nil && !reflect.DeepEqual(current[field], draft[field]) {
			values["current."+field] = current[field]
		}
		for name, value := range values {
			if value == nil {
				continue
			}
			row, err := loadMaintenanceMCPJSON(tx, spec.query, value)
			if err != nil && err != sql.ErrNoRows {
				return nil, nil, err
			}
			record := map[string]any{"entity": spec.entity, "id": value, "exists": err != sql.ErrNoRows, "record": row}
			encoded, err := json.Marshal(record)
			if err != nil {
				return nil, nil, err
			}
			digest := sha256.Sum256(encoded)
			versions[name] = hex.EncodeToString(digest[:])
			snapshots[name] = record
		}
	}
	return snapshots, versions, nil
}

func WarehouseTaskMCP(w http.ResponseWriter, r *http.Request) {
	user, ok := middleware.GetUserFromContext(r)
	if !ok || user == nil || user.UserID == 0 || !user.IsAdmin || !isWarehouseMCPMutation(r) {
		respondJSON(w, 403, map[string]string{"error": "Signed-in warehouse administrator and MCP origin required"})
		return
	}
	op := mux.Vars(r)["operation"]
	if !map[string]bool{"create": true, "update": true, "start": true, "complete": true, "cancel": true, "reopen": true, "archive": true, "restore": true}[op] {
		respondJSON(w, 400, map[string]string{"error": "Unsupported warehouse task operation"})
		return
	}
	var in warehouseTaskMCPRequest
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 65536))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&in); err != nil {
		respondJSON(w, 400, map[string]string{"error": "Invalid bounded task body"})
		return
	}
	if err := decoder.Decode(new(any)); err != io.EOF || op == "create" && in.TaskID != 0 || op != "create" && in.TaskID <= 0 {
		respondJSON(w, 400, map[string]string{"error": "One object and exact operation-specific task ID required"})
		return
	}
	preview := in.Preview || !(op == "create" && in.ConfirmCreation || op != "create" && in.ConfirmChange)
	elevated := op == "complete" || op == "cancel" || op == "reopen" || op == "archive" || op == "restore"
	phrase := fmt.Sprintf("%s WAREHOUSE TASK %d", strings.ToUpper(op), in.TaskID)
	if !preview && (op != "create" && in.ExpectedUpdatedAt == "" || elevated && in.ConfirmationText != phrase) {
		respondJSON(w, 428, map[string]string{"error": "Exact task version and elevated task-bound confirmation required"})
		return
	}
	if len([]rune(in.Reason)) > 4000 || len(in.ExpectedReferences) > 14 {
		respondJSON(w, 400, map[string]string{"error": "Bounded reason and reference map required"})
		return
	}
	tx, err := repository.GetSQLDB().BeginTx(r.Context(), nil)
	if err != nil {
		respondMaintenanceMCPError(w, err)
		return
	}
	defer tx.Rollback()
	if _, err = tx.Exec(`SET LOCAL lock_timeout='5s';SET LOCAL statement_timeout='15s'`); err != nil {
		respondMaintenanceMCPError(w, err)
		return
	}
	var receipt int64
	if !preview {
		var replay json.RawMessage
		receipt, replay, err = beginWarehouseProductMutation(tx, r, "warehouse_task."+op, in)
		if err != nil {
			respondMaintenanceMCPError(w, err)
			return
		}
		if replay != nil {
			respondJSON(w, 200, replay)
			return
		}
	}
	if _, err = tx.Exec(`LOCK TABLE warehouse_tasks,warehouse_task_events,storage_zones,cases,devices,products,jobs IN SHARE ROW EXCLUSIVE MODE;LOCK TABLE status,users IN SHARE MODE`); err != nil {
		respondMaintenanceMCPError(w, err)
		return
	}
	current := map[string]any{}
	draft := map[string]any{"task_type": "", "status": "open", "priority": 50, "from_zone_id": nil, "to_zone_id": nil, "case_id": nil, "device_id": nil, "product_id": nil, "quantity": nil, "job_id": nil, "assigned_to": nil, "due_at": nil, "notes": "", "is_archived": false}
	required := []string{}
	if op != "create" {
		current, err = loadMaintenanceMCPJSON(tx, warehouseTaskMCPSelect, in.TaskID)
		if err == sql.ErrNoRows {
			respondJSON(w, 404, map[string]string{"error": "Warehouse task not found"})
			return
		}
		if err != nil {
			respondMaintenanceMCPError(w, err)
			return
		}
		for field := range draft {
			draft[field] = current[field]
		}
		if in.ExpectedUpdatedAt != "" && in.ExpectedUpdatedAt != current["updated_at"] {
			required = append(required, "expected_updated_at")
		}
	}
	if op == "create" || op == "update" {
		for field, value := range map[string]*string{"task_type": in.TaskType, "device_id": in.DeviceID, "due_at": in.DueAt, "notes": in.Notes} {
			if value != nil {
				draft[field] = *value
			}
		}
		for field, value := range map[string]*int64{"from_zone_id": in.FromZoneID, "to_zone_id": in.ToZoneID, "case_id": in.CaseID, "product_id": in.ProductID, "job_id": in.JobID, "assigned_to": in.AssignedTo} {
			if value != nil {
				if *value <= 0 {
					respondJSON(w, 400, map[string]string{"error": "Positive reference IDs required; use clear_fields to remove optional references"})
					return
				}
				draft[field] = float64(*value)
			}
		}
		if in.Priority != nil {
			draft["priority"] = float64(*in.Priority)
		}
		if in.Quantity != nil {
			draft["quantity"] = *in.Quantity
		}
		supplied := map[string]bool{"from_zone_id": in.FromZoneID != nil, "to_zone_id": in.ToZoneID != nil, "case_id": in.CaseID != nil, "device_id": in.DeviceID != nil, "product_id": in.ProductID != nil, "quantity": in.Quantity != nil, "job_id": in.JobID != nil, "assigned_to": in.AssignedTo != nil, "due_at": in.DueAt != nil, "notes": in.Notes != nil}
		seen := map[string]bool{}
		for _, field := range in.ClearFields {
			if _, ok := supplied[field]; !ok || supplied[field] || seen[field] {
				respondJSON(w, 400, map[string]string{"error": "Invalid, duplicate or conflicting clear_fields"})
				return
			}
			seen[field] = true
			draft[field] = nil
			if field == "notes" {
				draft[field] = ""
			}
		}
	} else if in.TaskType != nil || in.Priority != nil || in.FromZoneID != nil || in.ToZoneID != nil || in.CaseID != nil || in.DeviceID != nil || in.ProductID != nil || in.Quantity != nil || in.JobID != nil || in.AssignedTo != nil || in.DueAt != nil || in.Notes != nil || len(in.ClearFields) > 0 {
		respondJSON(w, 400, map[string]string{"error": "Use metadata update separately from task state/lifecycle"})
		return
	}
	kind, _ := draft["task_type"].(string)
	if !map[string]bool{"putaway": true, "move": true, "pick": true, "replenish": true, "count": true, "inspect": true, "pack": true, "return": true}[kind] {
		required = append(required, "task_type")
	}
	priority := maintenanceMCPID(draft["priority"])
	if value, ok := draft["priority"].(int); ok {
		priority = int64(value)
	}
	if priority < 0 || priority > 100 {
		respondJSON(w, 400, map[string]string{"error": "Priority must be 0-100"})
		return
	}
	if notes, _ := draft["notes"].(string); len([]rune(notes)) > 4000 {
		respondJSON(w, 400, map[string]string{"error": "Task notes must not exceed 4000 characters"})
		return
	}
	if device, ok := draft["device_id"].(string); ok {
		if device == "" {
			draft["device_id"] = nil
		} else if strings.TrimSpace(device) != device || len(device) > 50 {
			respondJSON(w, 400, map[string]string{"error": "Exact bounded device ID required"})
			return
		}
	}
	if quantity, ok := draft["quantity"].(float64); ok {
		if math.IsNaN(quantity) || math.IsInf(quantity, 0) || quantity <= 0 || quantity > 999999999.999 || func() bool {
			decimal := strconv.FormatFloat(quantity, 'f', -1, 64)
			dot := strings.IndexByte(decimal, '.')
			return dot >= 0 && len(decimal)-dot-1 > 3
		}() {
			respondJSON(w, 400, map[string]string{"error": "Quantity must be positive with at most three decimal places and maximum 999999999.999"})
			return
		}
		if draft["device_id"] != nil && quantity != 1 {
			required = append(required, "serialized_device_quantity_one")
		}
	}
	if due, ok := draft["due_at"].(string); ok {
		if due == "" {
			draft["due_at"] = nil
		} else {
			parsed, e := time.Parse(time.RFC3339Nano, due)
			if e != nil {
				respondJSON(w, 400, map[string]string{"error": "Due date must be RFC3339 with timezone"})
				return
			}
			draft["due_at"] = parsed.UTC().Format("2006-01-02T15:04:05.000000Z")
		}
	}
	hasContext := false
	for _, field := range []string{"from_zone_id", "to_zone_id", "case_id", "device_id", "product_id", "job_id"} {
		hasContext = hasContext || draft[field] != nil
	}
	if !hasContext {
		required = append(required, "context")
	}
	state := fmt.Sprint(draft["status"])
	terminal := state == "done" || state == "cancelled"
	archived := draft["is_archived"] == true
	if op == "restore" {
		if !archived {
			required = append(required, "archived_task_required")
		}
		draft["is_archived"] = false
	} else if archived {
		required = append(required, "restore_before_change")
	}
	if op == "update" && terminal {
		required = append(required, "reopen_before_edit")
	}
	if op == "start" {
		if state != "open" {
			required = append(required, "open_task_required")
		}
		draft["status"] = "in_progress"
	}
	if op == "complete" || op == "cancel" {
		if terminal {
			required = append(required, "active_task_required")
		}
		draft["status"] = map[string]string{"complete": "done", "cancel": "cancelled"}[op]
	}
	if op == "cancel" || op == "reopen" {
		if strings.TrimSpace(in.Reason) == "" {
			required = append(required, "reason")
		}
	}
	if op == "reopen" {
		if !terminal {
			required = append(required, "terminal_task_required")
		}
		draft["status"] = "open"
	}
	if op == "archive" {
		if !terminal {
			required = append(required, "terminal_task_required")
		}
		draft["is_archived"] = true
	}
	refs, versions, err := loadTaskMCPReferences(tx, draft, current)
	if err != nil {
		respondMaintenanceMCPError(w, err)
		return
	}
	validateActive := op == "create" || op == "update" || op == "start" || op == "reopen"
	for field, record := range refs {
		if strings.HasPrefix(field, "current.") {
			continue
		}
		ref := record.(map[string]any)
		if !validateActive {
			continue
		}
		row, _ := ref["record"].(map[string]any)
		invalid := ref["exists"] != true || row["is_active"] == false || row["lifecycle_status"] != nil && row["lifecycle_status"] != "active" || row["deleted_at"] != nil || row["closed"] == true || row["condition_status"] == "retired" || row["product_lifecycle"] != nil && row["product_lifecycle"] != "active"
		if invalid {
			required = append(required, "active_reference."+field)
		}
		if field == "device_id" && draft["product_id"] != nil && maintenanceMCPID(draft["product_id"]) != maintenanceMCPID(row["product_id"]) {
			required = append(required, "matching_device_product")
		}
	}
	if !preview && len(versions) > 0 && len(in.ExpectedReferences) == 0 {
		respondJSON(w, 428, map[string]string{"error": "Exact complete reference versions from preview required"})
		return
	}
	if len(in.ExpectedReferences) > 0 && !reflect.DeepEqual(in.ExpectedReferences, versions) {
		required = append(required, "expected_references")
	}
	diff := map[string]any{}
	for field, value := range draft {
		if !reflect.DeepEqual(current[field], value) {
			diff[field] = map[string]any{"before": current[field], "after": value}
		}
	}
	if op == "update" && len(diff) == 0 {
		required = append(required, "changed_fields")
	}
	result := map[string]any{"operation_status": "confirmation_required", "ready_to_execute": len(required) == 0, "current": current, "draft": draft, "diff": diff, "references": refs, "expected_references": versions, "expected_updated_at": current["updated_at"], "required_fields": required, "preview": true, "effects": map[string]any{"inventory_movement": false, "task_status": draft["status"], "reference_versions_advance": true, "history_preserved": true}}
	if elevated {
		result["required_confirmation_text"] = phrase
	}
	if op == "create" {
		result["ready_to_create"] = len(required) == 0
		result["creation_status"] = "confirmation_required"
	}
	if preview || len(required) > 0 {
		if len(required) > 0 {
			result["operation_status"] = "needs_input"
			if op == "create" {
				result["creation_status"] = "needs_input"
			}
		}
		respondJSON(w, 200, result)
		return
	}
	id := in.TaskID
	if op == "create" {
		err = tx.QueryRow(`INSERT INTO warehouse_tasks(task_type,priority,from_zone_id,to_zone_id,case_id,device_id,product_id,quantity,job_id,assigned_to,due_at,notes) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12) RETURNING task_id`, kind, priority, draft["from_zone_id"], draft["to_zone_id"], draft["case_id"], draft["device_id"], draft["product_id"], draft["quantity"], draft["job_id"], draft["assigned_to"], draft["due_at"], maintenanceNullableString(fmt.Sprint(draft["notes"]))).Scan(&id)
	} else if op == "update" {
		_, err = tx.Exec(`UPDATE warehouse_tasks SET task_type=$1,priority=$2,from_zone_id=$3,to_zone_id=$4,case_id=$5,device_id=$6,product_id=$7,quantity=$8,job_id=$9,assigned_to=$10,due_at=$11,notes=$12 WHERE task_id=$13`, kind, priority, draft["from_zone_id"], draft["to_zone_id"], draft["case_id"], draft["device_id"], draft["product_id"], draft["quantity"], draft["job_id"], draft["assigned_to"], draft["due_at"], maintenanceNullableString(fmt.Sprint(draft["notes"])), id)
	} else if op == "archive" || op == "restore" {
		_, err = tx.Exec(`UPDATE warehouse_tasks SET is_archived=$1 WHERE task_id=$2`, draft["is_archived"], id)
	} else {
		_, err = tx.Exec(`UPDATE warehouse_tasks SET status=$1::text,started_at=CASE WHEN $1::text='in_progress' THEN CURRENT_TIMESTAMP WHEN $2='reopen' THEN NULL ELSE started_at END,completed_at=CASE WHEN $1::text IN ('done','cancelled') THEN CURRENT_TIMESTAMP WHEN $2='reopen' THEN NULL ELSE completed_at END WHERE task_id=$3`, draft["status"], op, id)
	}
	if err != nil {
		respondMaintenanceMCPError(w, err)
		return
	}
	if _, err = tx.Exec(`INSERT INTO warehouse_task_events(task_id,event_type,from_status,to_status,reason,actor_id) VALUES($1,$2,$3,$4,$5,$6)`, id, "mcp_"+op, current["status"], draft["status"], maintenanceNullableString(in.Reason), user.UserID); err != nil {
		respondMaintenanceMCPError(w, err)
		return
	}
	after, err := loadMaintenanceMCPJSON(tx, warehouseTaskMCPSelect, id)
	if err != nil {
		respondMaintenanceMCPError(w, err)
		return
	}
	refsAfter, versionsAfter, err := loadTaskMCPReferences(tx, draft, current)
	if err != nil {
		respondMaintenanceMCPError(w, err)
		return
	}
	seen := map[string]bool{}
	for field, record := range refsAfter {
		before := refs[field].(map[string]any)
		next := record.(map[string]any)
		identity := fmt.Sprint(next["entity"]) + ":" + fmt.Sprint(next["id"])
		if seen[identity] || versions[field] == versionsAfter[field] {
			continue
		}
		seen[identity] = true
		snapshot, _ := next["record"].(map[string]any)
		if _, err = maintenanceMCPAudit(tx, r, "warehouse_task.touch_reference", fmt.Sprint(next["entity"]), fmt.Sprint(next["id"]), before["record"], next["record"], fmt.Sprint(snapshot["updated_at"])); err != nil {
			respondMaintenanceMCPError(w, err)
			return
		}
	}
	auditID, err := maintenanceMCPAudit(tx, r, "warehouse_task."+op, "warehouse_task", fmt.Sprint(id), current, after, fmt.Sprint(after["updated_at"]))
	if err != nil {
		respondMaintenanceMCPError(w, err)
		return
	}
	result = map[string]any{"operation_status": map[string]string{"create": "created", "update": "updated", "start": "started", "complete": "completed", "cancel": "cancelled", "reopen": "reopened", "archive": "archived", "restore": "restored"}[op], "warehouse_task": after, "audit_id": auditID, "references": refsAfter, "expected_references": versionsAfter}
	if op == "create" {
		result["creation_status"] = "created"
	}
	if err = completeWarehouseProductMutation(tx, receipt, 200, result); err == nil {
		err = tx.Commit()
	}
	if err != nil {
		respondMaintenanceMCPError(w, err)
		return
	}
	respondJSON(w, 200, result)
}
