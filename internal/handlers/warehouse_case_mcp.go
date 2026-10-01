package handlers

import (
	"database/sql"
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

// A dedicated business endpoint; fields and operations are closed and are
// revalidated inside the transaction even when MCP already returned a preview.
type warehouseCaseRequest struct {
	CaseID            int64    `json:"case_id"`
	Name              *string  `json:"name,omitempty"`
	Description       *string  `json:"description,omitempty"`
	CaseType          *string  `json:"case_type,omitempty"`
	CaseModelID       *int64   `json:"case_model_id,omitempty"`
	Width             *float64 `json:"width,omitempty"`
	Height            *float64 `json:"height,omitempty"`
	Depth             *float64 `json:"depth,omitempty"`
	Weight            *float64 `json:"weight,omitempty"`
	MaxWeightKg       *float64 `json:"max_weight_kg,omitempty"`
	ZoneID            *int64   `json:"zone_id,omitempty"`
	HomeZoneID        *int64   `json:"home_zone_id,omitempty"`
	Barcode           *string  `json:"barcode,omitempty"`
	RFIDTag           *string  `json:"rfid_tag,omitempty"`
	ClearFields       []string `json:"clear_fields,omitempty"`
	AllowDuplicate    bool     `json:"allow_duplicate,omitempty"`
	ExpectedUpdatedAt string   `json:"expected_updated_at,omitempty"`
	ConfirmChange     bool     `json:"confirm_change,omitempty"`
	ConfirmationText  string   `json:"confirmation_text,omitempty"`
	Preview           bool     `json:"preview,omitempty"`
}

const warehouseCaseSnapshotSQL = `SELECT jsonb_build_object('case_id',caseid,'name',name,'description',description,'case_type',case_type,'case_model_id',case_model_id,'width',width,'height',height,'depth',depth,'weight',weight,'max_weight_kg',max_weight_kg,'zone_id',zone_id,'home_zone_id',home_zone_id,'barcode',barcode,'rfid_tag',rfid_tag,'status',status,'workflow_status',workflow_status,'current_job_id',current_job_id,'sealed_at',sealed_at,'lifecycle_status',lifecycle_status,'updated_at',to_char(updated_at,'YYYY-MM-DD"T"HH24:MI:SS.US"Z"')) FROM cases WHERE caseid=$1`

func caseMCPSnapshot(tx *sql.Tx, id int64) (map[string]any, error) {
	var raw []byte
	if err := tx.QueryRow(warehouseCaseSnapshotSQL, id).Scan(&raw); err != nil {
		return nil, err
	}
	var fields map[string]any
	err := json.Unmarshal(raw, &fields)
	return fields, err
}

func caseMCPDependencies(tx *sql.Tx, id int64) (map[string]int64, error) {
	var devices, products, children, parents, tasks, jobs, current int64
	err := tx.QueryRow(`SELECT
	(SELECT count(*) FROM devicescases WHERE caseid=$1)+(SELECT count(*) FROM devices WHERE current_case_id=$1),
	(SELECT count(*) FROM case_product_contents WHERE case_id=$1),
	(SELECT count(*) FROM case_child_contents WHERE parent_case_id=$1),
	(SELECT count(*) FROM case_child_contents WHERE child_case_id=$1),
	(SELECT count(*) FROM warehouse_tasks WHERE case_id=$1 AND lower(COALESCE(status,'')) NOT IN ('done','completed','cancelled','canceled','closed')),
	(SELECT count(*) FROM cases c JOIN jobs j ON j.jobid=c.current_job_id LEFT JOIN status s ON s.statusid=j.statusid WHERE c.caseid=$1 AND j.deleted_at IS NULL AND NOT warehouse_job_status_is_closed(COALESCE(s.status,''))),
	(SELECT count(*) FROM cases WHERE caseid=$1 AND (current_job_id IS NOT NULL OR COALESCE(workflow_status,'') NOT IN ('empty','maintenance') OR COALESCE(status,'') NOT IN ('free','maintenance','maintance') OR sealed_at IS NOT NULL))`, id).Scan(&devices, &products, &children, &parents, &tasks, &jobs, &current)
	return map[string]int64{"devices": devices, "product_contents": products, "child_cases": children, "parent_cases": parents, "open_tasks": tasks, "active_jobs": jobs, "active_workflow": current}, err
}

func patchCaseMCP(in warehouseCaseRequest, before map[string]any, operation string) (map[string]any, error) {
	after := map[string]any{}
	for k, v := range before {
		after[k] = v
	}
	if operation == "create" {
		after["case_type"] = "dynamic"
		after["status"] = "free"
		after["workflow_status"] = "empty"
		after["lifecycle_status"] = "active"
	}
	patch, _ := json.Marshal(in)
	var changes map[string]any
	_ = json.Unmarshal(patch, &changes)
	editable := []string{"name", "description", "case_type", "case_model_id", "width", "height", "depth", "weight", "max_weight_kg", "zone_id", "home_zone_id", "barcode", "rfid_tag"}
	for _, key := range editable {
		if v, ok := changes[key]; ok {
			after[key] = v
		}
	}
	clearable := map[string]bool{"description": true, "case_model_id": true, "width": true, "height": true, "depth": true, "weight": true, "max_weight_kg": true, "home_zone_id": true, "rfid_tag": true}
	for _, key := range in.ClearFields {
		if !clearable[key] {
			return nil, fmt.Errorf("field %s cannot be cleared", key)
		}
		if _, ok := changes[key]; ok {
			return nil, fmt.Errorf("field %s cannot be supplied and cleared", key)
		}
		after[key] = nil
	}
	if operation == "archive" || operation == "restore" {
		if len(in.ClearFields) > 0 {
			return nil, fmt.Errorf("lifecycle operations cannot edit fields")
		}
		for _, key := range editable {
			if _, ok := changes[key]; ok {
				return nil, fmt.Errorf("lifecycle operations cannot edit %s", key)
			}
		}
		target := "archived"
		if operation == "restore" {
			target = "active"
		}
		after["lifecycle_status"] = target
	}
	return after, nil
}

func validateCaseMCP(tx *sql.Tx, fields map[string]any, id int64, allowDuplicate bool) ([]map[string]any, error) {
	for _, entry := range []struct {
		key string
		max int
	}{{"name", 255}, {"description", 4000}, {"barcode", 255}, {"rfid_tag", 255}} {
		if value, ok := fields[entry.key].(string); ok {
			value = strings.TrimSpace(value)
			if len([]rune(value)) > entry.max {
				return nil, fmt.Errorf("%s too long", entry.key)
			}
			fields[entry.key] = value
			if value == "" && entry.key != "name" {
				fields[entry.key] = nil
			}
		}
	}
	if fields["name"] == nil || fields["name"] == "" {
		return nil, fmt.Errorf("name is required")
	}
	if !map[string]bool{"dynamic": true, "fixed": true, "hybrid": true}[fmt.Sprint(fields["case_type"])] {
		return nil, fmt.Errorf("case_type must be dynamic, fixed or hybrid")
	}
	for _, key := range []string{"width", "height", "depth", "weight", "max_weight_kg"} {
		if v, ok := fields[key].(float64); ok {
			if math.IsNaN(v) || math.IsInf(v, 0) || v <= 0 || v > 99999999.99 || math.Abs(v*100-math.Round(v*100)) > 0.00001 {
				return nil, fmt.Errorf("%s requires positive value with at most two decimals", key)
			}
		}
	}
	if w, ok := fields["weight"].(float64); ok {
		if max, ok := fields["max_weight_kg"].(float64); ok && max < w {
			return nil, fmt.Errorf("max_weight_kg cannot be below empty case weight")
		}
	}
	for _, key := range []string{"case_model_id", "zone_id", "home_zone_id"} {
		if v, ok := fields[key].(float64); ok && (v <= 0 || v > math.MaxInt32 || math.Trunc(v) != v) {
			return nil, fmt.Errorf("%s must be a valid positive ID", key)
		}
	}
	if model := fields["case_model_id"]; model != nil {
		var exists bool
		if err := tx.QueryRow(`SELECT EXISTS(SELECT 1 FROM case_models WHERE model_id=$1)`, model).Scan(&exists); err != nil {
			return nil, err
		}
		if !exists {
			return nil, fmt.Errorf("case model not found")
		}
	}
	if zone := fields["home_zone_id"]; zone != nil {
		var exists bool
		err := tx.QueryRow(`SELECT EXISTS(SELECT 1 FROM storage_zones WHERE zone_id=$1 AND is_active AND is_storable AND operational_status<>'archived')`, zone).Scan(&exists)
		if err != nil {
			return nil, err
		}
		if !exists {
			return nil, fmt.Errorf("active storable home zone is required")
		}
		if err := validateCaseMCPHierarchy(tx, int64(zone.(float64))); err != nil {
			return nil, err
		}
	}
	for _, key := range []string{"barcode", "rfid_tag"} {
		code := fields[key]
		if code == nil {
			continue
		}
		var conflict bool
		err := tx.QueryRow(`SELECT EXISTS(SELECT 1 FROM cases WHERE caseid<>$1 AND (lower(trim(barcode))=lower($2) OR lower(trim(rfid_tag))=lower($2))) OR EXISTS(SELECT 1 FROM inventory_identifiers WHERE NOT(entity_type='case' AND entity_key=$1::text) AND lower(trim(code))=lower($2)) OR EXISTS(SELECT 1 FROM devices WHERE lower(trim(deviceid))=lower($2) OR lower(trim(barcode))=lower($2) OR lower(trim(qr_code))=lower($2))`, id, code).Scan(&conflict)
		if err != nil {
			return nil, err
		}
		if conflict {
			return nil, fmt.Errorf("%s is reserved by another inventory record", key)
		}
	}
	rows, err := tx.Query(`SELECT caseid,name,barcode FROM cases WHERE caseid<>$1 AND (lower(trim(name))=lower($2) OR name ILIKE '%'||$2||'%' OR $2 ILIKE '%'||name||'%') ORDER BY caseid LIMIT 20`, id, fields["name"])
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	duplicates := []map[string]any{}
	for rows.Next() {
		var other int64
		var name string
		var barcode sql.NullString
		if err := rows.Scan(&other, &name, &barcode); err != nil {
			return nil, err
		}
		duplicates = append(duplicates, map[string]any{"case_id": other, "name": name, "barcode": ptrString(barcode)})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(duplicates) > 0 && !allowDuplicate {
		return duplicates, fmt.Errorf("similar cases exist; explicitly confirm allow_duplicate")
	}
	return duplicates, nil
}

func CaseMCP(w http.ResponseWriter, r *http.Request) {
	user, ok := middleware.GetUserFromContext(r)
	if !ok || user == nil || user.UserID == 0 || !user.IsAdmin {
		respondJSON(w, 403, map[string]string{"error": "Warehouse administrator required"})
		return
	}
	if !isWarehouseMCPMutation(r) {
		respondJSON(w, 400, map[string]string{"error": "MCP origin required"})
		return
	}
	op := mux.Vars(r)["operation"]
	if !map[string]bool{"create": true, "update": true, "archive": true, "restore": true}[op] {
		respondJSON(w, 400, map[string]string{"error": "Unsupported case operation"})
		return
	}
	var in warehouseCaseRequest
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&in); err != nil {
		respondJSON(w, 400, map[string]string{"error": "Invalid case payload: " + err.Error()})
		return
	}
	if err := dec.Decode(&struct{}{}); err != io.EOF {
		respondJSON(w, 400, map[string]string{"error": "One JSON object required"})
		return
	}
	if (op == "create" && in.CaseID != 0) || (op != "create" && (in.CaseID <= 0 || in.CaseID > math.MaxInt32)) {
		respondJSON(w, 400, map[string]string{"error": "Invalid case_id"})
		return
	}
	preview := in.Preview || !in.ConfirmChange
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
		receipt, replay, err = beginWarehouseProductMutation(tx, r, "case."+op, in)
		if err != nil {
			respondWarehouseMutationError(w, err)
			return
		}
		if replay != nil {
			respondJSON(w, 200, replay)
			return
		}
	}
	// Uniform table ordering avoids races with UI writes and post-preview references.
	if _, err = tx.Exec(`LOCK TABLE cases IN SHARE ROW EXCLUSIVE MODE; LOCK TABLE case_models,storage_zones,inventory_identifiers,devices,devicescases,case_product_contents,case_child_contents,case_content_templates,warehouse_tasks,jobs,status,product_locations,products IN SHARE MODE`); err != nil {
		respondWarehouseMutationError(w, err)
		return
	}
	before := map[string]any{}
	deps := map[string]int64{}
	if op != "create" {
		before, err = caseMCPSnapshot(tx, in.CaseID)
		if err == sql.ErrNoRows {
			respondJSON(w, 404, map[string]string{"error": "Case not found"})
			return
		}
		if err != nil {
			respondWarehouseMutationError(w, err)
			return
		}
		deps, err = caseMCPDependencies(tx, in.CaseID)
		if err != nil {
			respondWarehouseMutationError(w, err)
			return
		}
	}
	after, err := patchCaseMCP(in, before, op)
	if err != nil {
		respondJSON(w, 400, map[string]string{"error": err.Error()})
		return
	}
	missing := []string{}
	warnings := []string{}
	diff := map[string]any{}
	for key, value := range after {
		if !reflect.DeepEqual(value, before[key]) {
			diff[key] = map[string]any{"before": before[key], "after": value}
		}
	}
	if op != "create" {
		if in.ExpectedUpdatedAt == "" {
			missing = append(missing, "expected_updated_at")
		} else if in.ExpectedUpdatedAt != before["updated_at"] {
			missing = append(missing, "stale_version")
		}
	}
	if op == "update" && before["lifecycle_status"] != "active" {
		missing = append(missing, "restore_before_update")
	}
	if op == "update" && in.ZoneID != nil {
		missing = append(missing, "location_changes_require_movement_workflow")
	}
	if op == "update" && len(diff) == 0 {
		missing = append(missing, "changes")
	}
	phrase := ""
	if op == "archive" || op == "restore" {
		phrase = strings.ToUpper(op) + " WAREHOUSE CASE " + strconv.FormatInt(in.CaseID, 10)
		target := "active"
		if op == "archive" {
			target = "archived"
		}
		if before["lifecycle_status"] == target {
			missing = append(missing, "already_"+target)
		}
		if hasDeviceDependencies(deps) {
			missing = append(missing, "active_dependencies")
		}
	}
	var duplicates []map[string]any
	if op == "create" || op == "update" || op == "restore" {
		duplicates, err = validateCaseMCP(tx, after, in.CaseID, in.AllowDuplicate)
		if err != nil {
			missing = append(missing, "valid_fields_and_references")
			warnings = append(warnings, err.Error())
		}
	}
	if op == "update" && after["barcode"] == nil {
		missing = append(missing, "barcode_cannot_be_cleared")
	}
	// Diff follows normalization so it represents exactly what will be persisted.
	diff = map[string]any{}
	for key, value := range after {
		if !reflect.DeepEqual(value, before[key]) {
			diff[key] = map[string]any{"before": before[key], "after": value}
		}
	}
	if op == "update" && len(diff) == 0 && !containsCaseMCPMissing(missing, "changes") {
		missing = append(missing, "changes")
	}
	if op == "update" && hasDeviceDependencies(deps) {
		for _, key := range []string{"barcode", "rfid_tag", "case_type", "case_model_id", "width", "height", "depth", "weight", "max_weight_kg"} {
			if diff[key] != nil {
				missing = append(missing, "dependencies_block_"+key)
			}
		}
	}
	if op == "restore" {
		var invalid int64
		err = tx.QueryRow(`SELECT count(*) FROM case_content_templates ct LEFT JOIN products p ON p.productid=ct.product_id WHERE ct.case_id=$1 AND (p.productid IS NULL OR p.lifecycle_status<>'active')`, in.CaseID).Scan(&invalid)
		if err != nil {
			respondWarehouseMutationError(w, err)
			return
		}
		if invalid > 0 {
			missing = append(missing, "inactive_template_products")
		}
	}
	if (op == "create" || op == "restore") && after["zone_id"] != nil {
		zone := int64(after["zone_id"].(float64))
		incoming := int64(1)
		if op == "restore" {
			incoming = 0
		}
		if err = validateCaseMCPDestination(tx, zone, incoming); err != nil {
			missing = append(missing, "storage_destination")
			warnings = append(warnings, err.Error())
		}
	}
	response := map[string]any{"operation_status": "draft", "current": before, "draft": after, "diff": diff, "dependencies": deps, "similar_cases": duplicates, "required_missing_fields": missing, "warnings": warnings, "ready_to_execute": len(missing) == 0, "confirmation_text_required": phrase, "expected_updated_at": before["updated_at"]}
	if preview || len(missing) > 0 {
		respondJSON(w, 200, response)
		return
	}
	if phrase != "" && in.ConfirmationText != phrase {
		respondJSON(w, 428, map[string]string{"error": "Record-bound lifecycle confirmation required", "confirmation_text_required": phrase})
		return
	}
	if op == "create" {
		err = tx.QueryRow(`INSERT INTO cases(name,description,case_type,case_model_id,width,height,depth,weight,max_weight_kg,zone_id,home_zone_id,barcode,rfid_tag) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13) RETURNING caseid`, after["name"], after["description"], after["case_type"], after["case_model_id"], after["width"], after["height"], after["depth"], after["weight"], after["max_weight_kg"], after["zone_id"], after["home_zone_id"], after["barcode"], after["rfid_tag"]).Scan(&in.CaseID)
	} else if op == "update" {
		_, err = tx.Exec(`UPDATE cases SET name=$1,description=$2,case_type=$3,case_model_id=$4,width=$5,height=$6,depth=$7,weight=$8,max_weight_kg=$9,home_zone_id=$10,barcode=$11,rfid_tag=$12 WHERE caseid=$13`, after["name"], after["description"], after["case_type"], after["case_model_id"], after["width"], after["height"], after["depth"], after["weight"], after["max_weight_kg"], after["home_zone_id"], after["barcode"], after["rfid_tag"], in.CaseID)
	} else {
		_, err = tx.Exec(`UPDATE cases SET lifecycle_status=$1 WHERE caseid=$2`, after["lifecycle_status"], in.CaseID)
	}
	if err != nil {
		respondWarehouseMutationError(w, err)
		return
	}
	after, err = caseMCPSnapshot(tx, in.CaseID)
	if err != nil {
		respondWarehouseMutationError(w, err)
		return
	}
	if op == "create" {
		if _, err = validateCaseMCP(tx, after, in.CaseID, in.AllowDuplicate); err != nil {
			respondJSON(w, 409, map[string]string{"error": err.Error()})
			return
		}
	}
	oldRaw, _ := json.Marshal(before)
	newRaw, _ := json.Marshal(map[string]any{"origin": "MCP/AI", "after": after, "updated_at": after["updated_at"]})
	var audit int64
	err = tx.QueryRow(`INSERT INTO audit_log(user_id,action,entity_type,entity_id,old_values,new_values,user_agent) VALUES($1,$2,'case',$3,$4::jsonb,$5::jsonb,$6) RETURNING id`, user.UserID, "case."+op, strconv.FormatInt(in.CaseID, 10), string(oldRaw), string(newRaw), r.UserAgent()).Scan(&audit)
	if err != nil {
		respondWarehouseMutationError(w, err)
		return
	}
	result := map[string]any{"operation_status": "executed", "case": after, "audit_id": audit, "diff": diff}
	if err = completeWarehouseProductMutation(tx, receipt, 200, result); err == nil {
		err = tx.Commit()
	}
	if err != nil {
		respondWarehouseMutationError(w, err)
		return
	}
	respondJSON(w, 200, result)
}

func validateCaseMCPDestination(tx *sql.Tx, zone int64, incoming int64) error {
	var active, storable, allowed bool
	var status string
	var capacity sql.NullFloat64
	err := tx.QueryRow(`SELECT z.is_active,z.is_storable,z.operational_status,z.capacity,COALESCE(p.allow_cases,true) FROM storage_zones z LEFT JOIN location_profiles p ON p.profile_id=z.profile_id WHERE z.zone_id=$1`, zone).Scan(&active, &storable, &status, &capacity, &allowed)
	if err != nil {
		return err
	}
	if !active || !storable || status != "available" || !allowed {
		return fmt.Errorf("available storable case destination is required")
	}
	if err = validateCaseMCPHierarchy(tx, zone); err != nil {
		return err
	}
	if capacity.Valid {
		var used float64
		err = tx.QueryRow(`SELECT (SELECT count(*) FROM devices WHERE zone_id=$1 AND status='in_storage' AND lifecycle_status='active')+(SELECT count(*) FROM cases WHERE zone_id=$1)+COALESCE((SELECT sum(quantity) FROM product_locations WHERE zone_id=$1),0)`, zone).Scan(&used)
		if err != nil {
			return err
		}
		if used+float64(incoming) > capacity.Float64 {
			return fmt.Errorf("storage capacity is exceeded")
		}
	}
	return nil
}
func validateCaseMCPHierarchy(tx *sql.Tx, zone int64) error {
	var invalid bool
	err := tx.QueryRow(`WITH RECURSIVE ancestors AS (
 SELECT zone_id,parent_zone_id,is_active,operational_status,ARRAY[zone_id] AS path,false AS cycle FROM storage_zones WHERE zone_id=$1
 UNION ALL SELECT p.zone_id,p.parent_zone_id,p.is_active,p.operational_status,a.path||p.zone_id,p.zone_id=ANY(a.path) FROM ancestors a JOIN storage_zones p ON p.zone_id=a.parent_zone_id WHERE NOT a.cycle AND cardinality(a.path)<=100
 ) SELECT EXISTS(SELECT 1 FROM ancestors a WHERE NOT is_active OR operational_status='archived' OR cycle OR cardinality(path)>100 OR (parent_zone_id IS NOT NULL AND NOT EXISTS(SELECT 1 FROM storage_zones p WHERE p.zone_id=a.parent_zone_id)))`, zone).Scan(&invalid)
	if err != nil {
		return err
	}
	if invalid {
		return fmt.Errorf("inactive, missing or cyclic storage ancestor")
	}
	return nil
}

func containsCaseMCPMissing(fields []string, key string) bool {
	for _, field := range fields {
		if field == key {
			return true
		}
	}
	return false
}
