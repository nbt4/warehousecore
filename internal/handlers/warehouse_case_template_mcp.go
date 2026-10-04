package handlers

import (
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"strconv"
	"strings"

	"warehousecore/internal/repository"

	"github.com/gorilla/mux"
)

type warehouseCaseTemplateRequest struct {
	CaseID            int64    `json:"case_id"`
	ProductID         int64    `json:"product_id,omitempty"`
	TemplateLineID    int64    `json:"template_line_id,omitempty"`
	ExpectedQuantity  *float64 `json:"expected_quantity,omitempty"`
	ExpectedUpdatedAt string   `json:"expected_updated_at,omitempty"`
	ExpectedContext   string   `json:"expected_context,omitempty"`
	ConfirmChange     bool     `json:"confirm_change,omitempty"`
	ConfirmationText  string   `json:"confirmation_text,omitempty"`
	Preview           bool     `json:"preview,omitempty"`
}

const caseTemplateSnapshotSQL = `SELECT jsonb_build_object('template_line_id',template_line_id,'case_id',case_id,'product_id',product_id,'expected_quantity',expected_quantity,'lifecycle_status',lifecycle_status,'archived_at',archived_at,'updated_at',to_char(updated_at,'YYYY-MM-DD"T"HH24:MI:SS.US"Z"')) FROM case_content_templates WHERE case_id=$1 ORDER BY template_line_id`

func CaseTemplateMCP(w http.ResponseWriter, r *http.Request) {
	op := mux.Vars(r)["operation"]
	action := "update"
	if op == "create" {
		action = "create"
	}
	if op == "archive" || op == "restore" {
		action = "archive"
	}
	if !map[string]bool{"create": true, "update": true, "archive": true, "restore": true}[op] {
		respondJSON(w, 400, map[string]string{"error": "Unsupported template operation"})
		return
	}
	user, _, err := warehouseScopedAdminActor(r, "cores:warehouse:"+action)
	if err != nil {
		respondWarehouseMutationError(w, err)
		return
	}
	var in warehouseCaseTemplateRequest
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10))
	dec.DisallowUnknownFields()
	if err = dec.Decode(&in); err != nil {
		respondJSON(w, 400, map[string]string{"error": "Invalid closed template payload"})
		return
	}
	if err = dec.Decode(&struct{}{}); err != io.EOF {
		respondJSON(w, 400, map[string]string{"error": "One JSON object required"})
		return
	}
	if in.CaseID <= 0 || in.CaseID > math.MaxInt32 || in.ProductID < 0 || in.ProductID > math.MaxInt32 || in.TemplateLineID < 0 {
		respondJSON(w, 400, map[string]string{"error": "Invalid template identity"})
		return
	}
	if (op == "create" && (in.ProductID == 0 || in.TemplateLineID != 0)) || (op != "create" && (in.TemplateLineID == 0 || in.ProductID != 0)) || ((op == "archive" || op == "restore") && in.ExpectedQuantity != nil) {
		respondJSON(w, 400, map[string]string{"error": "Create requires product_id; retained actions require template_line_id and preserve product identity; lifecycle cannot edit quantity"})
		return
	}
	if in.ExpectedQuantity != nil {
		q := *in.ExpectedQuantity
		if math.IsNaN(q) || math.IsInf(q, 0) || q <= 0 || q > 999999999.999 || math.Abs(q*1000-math.Round(q*1000)) > 0.00001 {
			respondJSON(w, 400, map[string]string{"error": "Expected quantity must be positive, at most 999999999.999 with three decimals"})
			return
		}
	}
	preview := in.Preview || !in.ConfirmChange
	tx, err := repository.GetSQLDB().BeginTx(r.Context(), nil)
	if err != nil {
		respondWarehouseMutationError(w, err)
		return
	}
	defer tx.Rollback()
	if _, err = tx.Exec(`SET LOCAL lock_timeout='5s';SET LOCAL statement_timeout='15s'`); err != nil {
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
		receipt, replay, err = beginWarehouseProductMutation(tx, r, "case_template."+op, in)
		if err != nil {
			respondWarehouseMutationError(w, err)
			return
		}
		if replay != nil {
			respondJSON(w, 200, replay)
			return
		}
	}
	// Same case-first ordering as metadata workflows; excludes native writes while
	// exact template, physical contents and product context are checked and applied.
	if _, err = tx.Exec(`LOCK TABLE cases IN SHARE ROW EXCLUSIVE MODE;LOCK TABLE case_content_templates IN SHARE ROW EXCLUSIVE MODE;LOCK TABLE products,devices,devicescases,case_product_contents,case_child_contents,warehouse_tasks,jobs,status IN SHARE MODE`); err != nil {
		respondWarehouseMutationError(w, err)
		return
	}
	parent, err := caseMCPSnapshot(tx, in.CaseID)
	if err == sql.ErrNoRows {
		respondJSON(w, 404, map[string]string{"error": "Case not found"})
		return
	}
	if err != nil {
		respondWarehouseMutationError(w, err)
		return
	}
	lines, err := inventoryMCPRows(tx, caseTemplateSnapshotSQL, in.CaseID)
	if err != nil {
		respondWarehouseMutationError(w, err)
		return
	}
	before := map[string]any{}
	productID := in.ProductID
	if op != "create" {
		for _, line := range lines {
			if fmt.Sprint(line["template_line_id"]) == strconv.FormatInt(in.TemplateLineID, 10) {
				before = line
				productID = int64(line["product_id"].(float64))
				break
			}
		}
		if len(before) == 0 {
			respondJSON(w, 404, map[string]string{"error": "Retained template line not found in this case"})
			return
		}
	}
	products, err := inventoryMCPRows(tx, `SELECT jsonb_build_object('product_id',productid,'name',name,'tracking_mode',tracking_mode,'lifecycle_status',lifecycle_status,'updated_at',to_char(updated_at,'YYYY-MM-DD"T"HH24:MI:SS.US"Z"')) FROM products WHERE productid=$2 OR productid IN(SELECT product_id FROM case_content_templates WHERE case_id=$1) ORDER BY productid`, in.CaseID, productID)
	if err != nil {
		respondWarehouseMutationError(w, err)
		return
	}
	actual, err := inventoryMCPRows(tx, `SELECT jsonb_build_object('product_id',p.productid,'device_count',(SELECT count(*) FROM devicescases dc JOIN devices d ON d.deviceid=dc.deviceid WHERE dc.caseid=$1 AND d.productid=p.productid),'packed_quantity',COALESCE((SELECT quantity FROM case_product_contents WHERE case_id=$1 AND product_id=p.productid),0)) FROM products p WHERE p.productid=$2 OR p.productid IN(SELECT product_id FROM case_content_templates WHERE case_id=$1) ORDER BY p.productid`, in.CaseID, productID)
	if err != nil {
		respondWarehouseMutationError(w, err)
		return
	}
	deps, err := caseMCPDependencies(tx, in.CaseID)
	if err != nil {
		respondWarehouseMutationError(w, err)
		return
	}
	after := map[string]any{}
	for k, v := range before {
		after[k] = v
	}
	if op == "create" {
		after = map[string]any{"case_id": in.CaseID, "product_id": productID, "lifecycle_status": "active"}
	}
	if in.ExpectedQuantity != nil {
		after["expected_quantity"] = *in.ExpectedQuantity
	}
	if op == "archive" {
		after["lifecycle_status"] = "archived"
	}
	if op == "restore" {
		after["lifecycle_status"] = "active"
	}
	missing := []string{}
	if parent["lifecycle_status"] != "active" || parent["current_job_id"] != nil || parent["sealed_at"] != nil || !map[string]bool{"empty": true, "packing": true, "return_check": true, "complete": true}[fmt.Sprint(parent["workflow_status"])] {
		missing = append(missing, "active_open_case")
	}
	if deps["parent_cases"] > 0 {
		missing = append(missing, "unnest_case_before_template_change")
	}
	if op != "archive" {
		var product map[string]any
		for _, p := range products {
			if fmt.Sprint(p["product_id"]) == strconv.FormatInt(productID, 10) {
				product = p
				break
			}
		}
		if product == nil || product["lifecycle_status"] != "active" || !map[string]bool{"individual": true, "quantity": true}[fmt.Sprint(product["tracking_mode"])] {
			missing = append(missing, "active_physical_product")
		}
		q, ok := after["expected_quantity"].(float64)
		if !ok || q <= 0 {
			missing = append(missing, "expected_quantity")
		}
		if product != nil && product["tracking_mode"] == "individual" && q != math.Trunc(q) {
			missing = append(missing, "whole_serialized_quantity")
		}
	}
	if op == "create" {
		for _, line := range lines {
			if fmt.Sprint(line["product_id"]) == strconv.FormatInt(productID, 10) {
				missing = append(missing, "existing_retained_template_use_update_or_restore")
				break
			}
		}
	}
	if op == "update" && (before["lifecycle_status"] != "active" || in.ExpectedQuantity == nil || before["expected_quantity"] == after["expected_quantity"]) {
		missing = append(missing, "changed_active_template_quantity")
	}
	if (op == "archive" && before["lifecycle_status"] != "active") || (op == "restore" && before["lifecycle_status"] != "archived") {
		missing = append(missing, "lifecycle_transition")
	}
	version := fmt.Sprint(parent["updated_at"])
	if in.ExpectedUpdatedAt != "" && in.ExpectedUpdatedAt != version {
		missing = append(missing, "stale_case_version")
	}
	contextRaw, _ := json.Marshal(map[string]any{"operation": op, "case": parent, "templates": lines, "products": products, "actual_contents": actual, "dependencies": deps, "draft": after})
	fingerprint := fmt.Sprintf("%x", sha256.Sum256(contextRaw))
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
	phrase := fmt.Sprintf("%s WAREHOUSE CASE TEMPLATE %d PRODUCT %d %s", strings.ToUpper(op), in.CaseID, productID, fingerprint[:16])
	diff := map[string]any{}
	for _, field := range []string{"expected_quantity", "lifecycle_status"} {
		if before[field] != after[field] {
			diff[field] = map[string]any{"before": before[field], "after": after[field]}
		}
	}
	result := map[string]any{"operation_status": "draft", "case": parent, "current": before, "draft": after, "templates": lines, "products": products, "actual_contents": actual, "dependencies": deps, "diff": diff, "expected_updated_at": version, "expected_context": fingerprint, "confirmation_text_required": phrase, "required_missing_fields": missing, "ready_to_execute": len(missing) == 0, "effects": map[string]any{"physical_stock_changed": false, "template_complete_recomputed": true, "identity_retained": true}}
	if preview || len(missing) > 0 {
		respondJSON(w, 200, result)
		return
	}
	if in.ConfirmationText != phrase {
		respondJSON(w, 428, map[string]string{"error": "Exact context-bound template confirmation required", "confirmation_text_required": phrase})
		return
	}
	if op == "create" {
		err = tx.QueryRow(`INSERT INTO case_content_templates(case_id,product_id,expected_quantity) VALUES($1,$2,$3) RETURNING template_line_id`, in.CaseID, productID, after["expected_quantity"]).Scan(&in.TemplateLineID)
	} else if op == "update" {
		_, err = tx.Exec(`UPDATE case_content_templates SET expected_quantity=$1 WHERE template_line_id=$2 AND case_id=$3`, after["expected_quantity"], in.TemplateLineID, in.CaseID)
	} else {
		_, err = tx.Exec(`UPDATE case_content_templates SET lifecycle_status=$1 WHERE template_line_id=$2 AND case_id=$3`, after["lifecycle_status"], in.TemplateLineID, in.CaseID)
	}
	if err != nil {
		respondWarehouseMutationError(w, err)
		return
	}
	lines, err = inventoryMCPRows(tx, caseTemplateSnapshotSQL, in.CaseID)
	if err != nil {
		respondWarehouseMutationError(w, err)
		return
	}
	for _, line := range lines {
		if fmt.Sprint(line["template_line_id"]) == strconv.FormatInt(in.TemplateLineID, 10) {
			after = line
			break
		}
	}
	parent, err = caseMCPSnapshot(tx, in.CaseID)
	if err != nil {
		respondWarehouseMutationError(w, err)
		return
	}
	oldRaw, _ := json.Marshal(before)
	newRaw, _ := json.Marshal(map[string]any{"origin": "MCP/AI", "after": after, "updated_at": after["updated_at"], "case_updated_at": parent["updated_at"]})
	var audit int64
	err = tx.QueryRow(`INSERT INTO audit_log(user_id,action,entity_type,entity_id,old_values,new_values,user_agent) VALUES($1,$2,'case_template',$3,$4::jsonb,$5::jsonb,$6) RETURNING id`, user.UserID, "case_template."+op, strconv.FormatInt(in.TemplateLineID, 10), string(oldRaw), string(newRaw), r.UserAgent()).Scan(&audit)
	if err != nil {
		respondWarehouseMutationError(w, err)
		return
	}
	result = map[string]any{"operation_status": "executed", "case": parent, "template": after, "audit_id": audit, "diff": diff, "effects": result["effects"]}
	if err = completeWarehouseProductMutation(tx, receipt, 200, result); err == nil {
		err = tx.Commit()
	}
	if err != nil {
		respondWarehouseMutationError(w, err)
		return
	}
	respondJSON(w, 200, result)
}
