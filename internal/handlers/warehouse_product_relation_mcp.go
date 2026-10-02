package handlers

import (
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"strings"

	"github.com/gorilla/mux"
	"warehousecore/internal/middleware"
	"warehousecore/internal/repository"
)

type warehouseProductRelationRequest struct {
	RelationID               int64    `json:"relation_id"`
	ProductID                int64    `json:"product_id"`
	DependencyProductID      int64    `json:"dependency_product_id"`
	RelationType             *string  `json:"relation_type"`
	AssignmentScope          *string  `json:"assignment_scope"`
	DefaultQuantity          *float64 `json:"default_quantity"`
	Notes                    *string  `json:"notes"`
	ExpectedUpdatedAt        string   `json:"expected_updated_at"`
	ExpectedProductUpdatedAt string   `json:"expected_product_updated_at"`
	ExpectedContext          string   `json:"expected_context"`
	ConfirmChange            bool     `json:"confirm_change"`
	ConfirmationText         string   `json:"confirmation_text"`
	Preview                  bool     `json:"preview"`
}

const warehouseRelationSnapshotSQL = `SELECT jsonb_build_object('relation_id',id,'product_id',product_id,'dependency_product_id',dependency_product_id,'relation_type',relation_type,'assignment_scope',assignment_scope,'default_quantity',default_quantity,'is_optional',is_optional,'notes',notes,'lifecycle_status',lifecycle_status,'archived_at',archived_at,'created_at',created_at,'updated_at',to_char(updated_at,'YYYY-MM-DD"T"HH24:MI:SS.US"Z"')) FROM product_dependencies WHERE id=$1`

func WarehouseProductRelationMCP(w http.ResponseWriter, r *http.Request) {
	user, ok := middleware.GetUserFromContext(r)
	if !ok || user == nil || user.UserID <= 0 || !user.IsAdmin || !isWarehouseMCPMutation(r) {
		respondJSON(w, 403, map[string]string{"error": "Signed-in warehouse administrator and MCP origin required"})
		return
	}
	op := mux.Vars(r)["operation"]
	if op != "create" && op != "link" && op != "update" && op != "archive" && op != "restore" {
		respondJSON(w, 400, map[string]string{"error": "Unsupported relationship operation"})
		return
	}
	var in warehouseProductRelationRequest
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16384))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&in) != nil || decoder.Decode(new(any)) != io.EOF {
		respondJSON(w, 400, map[string]string{"error": "One bounded typed relationship object required"})
		return
	}
	if in.RelationID < 0 || in.RelationID > math.MaxInt32 || in.ProductID < 0 || in.ProductID > math.MaxInt32 || in.DependencyProductID < 0 || in.DependencyProductID > math.MaxInt32 {
		respondJSON(w, 400, map[string]string{"error": "Positive serial identifiers required"})
		return
	}
	if (op == "create" || op == "link") && (in.ProductID == 0 || in.DependencyProductID == 0 || in.ProductID == in.DependencyProductID || in.RelationID != 0) || (op == "update" || op == "archive" || op == "restore") && in.RelationID == 0 {
		respondJSON(w, 400, map[string]string{"error": "Create/link requires two distinct product IDs; existing actions require an exact relation ID"})
		return
	}
	if (op == "archive" || op == "restore") && (in.RelationType != nil || in.AssignmentScope != nil || in.DefaultQuantity != nil || in.Notes != nil) {
		respondJSON(w, 400, map[string]string{"error": "Lifecycle retains all fields; use update separately"})
		return
	}
	preview := in.Preview || !in.ConfirmChange
	if !preview && len(in.ExpectedContext) != 64 {
		respondJSON(w, 428, map[string]string{"error": "Exact expected_context from the final preview required"})
		return
	}
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
	var receipt int64
	if !preview {
		bound := in
		bound.Preview = false
		var replay json.RawMessage
		receipt, replay, err = beginWarehouseProductMutation(tx, r, "product_relation."+op, bound)
		if err != nil {
			respondWarehouseMutationError(w, err)
			return
		}
		if replay != nil {
			respondJSON(w, 200, replay)
			return
		}
	}
	if _, err = tx.Exec(`LOCK TABLE products,product_dependencies IN SHARE ROW EXCLUSIVE MODE;LOCK TABLE jobs,status,job_product_requirements,job_positions,job_devices,devices IN SHARE MODE`); err != nil {
		respondWarehouseMutationError(w, err)
		return
	}
	var id int64
	var current map[string]any
	if in.RelationID > 0 {
		id = in.RelationID
	} else {
		err = tx.QueryRow(`SELECT id FROM product_dependencies WHERE product_id=$1 AND dependency_product_id=$2`, in.ProductID, in.DependencyProductID).Scan(&id)
		if err != nil && err != sql.ErrNoRows {
			respondWarehouseMutationError(w, err)
			return
		}
	}
	if id > 0 {
		var raw []byte
		if err = tx.QueryRow(warehouseRelationSnapshotSQL, id).Scan(&raw); err == sql.ErrNoRows {
			respondJSON(w, 404, map[string]string{"error": "Relationship not found"})
			return
		} else if err != nil {
			respondWarehouseMutationError(w, err)
			return
		}
		if err = json.Unmarshal(raw, &current); err != nil {
			respondWarehouseMutationError(w, err)
			return
		}
	}
	required := []string{}
	if op == "create" && current != nil {
		required = append(required, "existing_relationship")
	}
	if current != nil {
		source, target := int64(current["product_id"].(float64)), int64(current["dependency_product_id"].(float64))
		if in.ProductID != 0 && in.ProductID != source || in.DependencyProductID != 0 && in.DependencyProductID != target {
			required = append(required, "immutable_product_ids")
		}
		in.ProductID, in.DependencyProductID = source, target
		from := "active"
		if op == "restore" {
			from = "archived"
		}
		if current["lifecycle_status"] != from {
			required = append(required, "lifecycle_status")
		}
		if !preview && in.ExpectedUpdatedAt == "" || in.ExpectedUpdatedAt != "" && in.ExpectedUpdatedAt != current["updated_at"] {
			required = append(required, "expected_updated_at")
		}
	}
	var rawProducts []byte
	if err = tx.QueryRow(`SELECT COALESCE(jsonb_agg(jsonb_build_object('product_id',productid,'name',name,'product_code',product_code,'lifecycle_status',lifecycle_status,'updated_at',to_char(updated_at,'YYYY-MM-DD"T"HH24:MI:SS.US"Z"')) ORDER BY productid),'[]'::jsonb) FROM products WHERE productid IN ($1,$2)`, in.ProductID, in.DependencyProductID).Scan(&rawProducts); err != nil {
		respondWarehouseMutationError(w, err)
		return
	}
	var products []map[string]any
	if err = json.Unmarshal(rawProducts, &products); err != nil {
		respondWarehouseMutationError(w, err)
		return
	}
	if in.ExpectedProductUpdatedAt != "" {
		for _, p := range products {
			if int64(p["product_id"].(float64)) == in.ProductID && p["updated_at"] != in.ExpectedProductUpdatedAt {
				required = append(required, "expected_product_updated_at")
			}
		}
	}
	if len(products) != 2 {
		required = append(required, "product_ids")
	}
	if op != "archive" {
		for _, p := range products {
			if p["lifecycle_status"] != "active" {
				required = append(required, "active_products")
				break
			}
		}
	}
	var rawJobs []byte
	if err = tx.QueryRow(`SELECT warehouse_relation_active_jobs($1::int)`, in.ProductID).Scan(&rawJobs); err != nil {
		respondWarehouseMutationError(w, err)
		return
	}
	var jobs []map[string]any
	if err = json.Unmarshal(rawJobs, &jobs); err != nil {
		respondWarehouseMutationError(w, err)
		return
	}
	if len(jobs) > 0 {
		required = append(required, "active_jobs")
	}
	graphHash := ""
	if err = tx.QueryRow(`SELECT encode(sha256(convert_to(COALESCE(string_agg(jsonb_build_object('id',id,'source',product_id,'target',dependency_product_id,'relation_type',relation_type,'lifecycle_status',lifecycle_status,'updated_at',updated_at)::text,',' ORDER BY id),''),'UTF8')),'hex') FROM product_dependencies`).Scan(&graphHash); err != nil {
		respondWarehouseMutationError(w, err)
		return
	}
	proposed := map[string]any{"product_id": in.ProductID, "dependency_product_id": in.DependencyProductID, "relation_type": "recommended", "assignment_scope": "product", "default_quantity": float64(1), "notes": nil, "is_optional": true, "lifecycle_status": "active"}
	if current != nil {
		for k, v := range current {
			proposed[k] = v
		}
	}
	if in.RelationType != nil {
		proposed["relation_type"] = strings.ToLower(strings.TrimSpace(*in.RelationType))
	}
	if in.AssignmentScope != nil {
		proposed["assignment_scope"] = strings.ToLower(strings.TrimSpace(*in.AssignmentScope))
	}
	if in.DefaultQuantity != nil {
		proposed["default_quantity"] = *in.DefaultQuantity
	}
	if in.Notes != nil {
		value := strings.TrimSpace(*in.Notes)
		proposed["notes"] = nil
		if value != "" {
			proposed["notes"] = value
		}
	}
	relation, _ := proposed["relation_type"].(string)
	scope, _ := proposed["assignment_scope"].(string)
	quantity, _ := proposed["default_quantity"].(float64)
	if op != "archive" {
		if !containsRelationType(relation) {
			required = append(required, "relation_type")
		}
		if scope != "product" && scope != "device" && scope != "case" {
			required = append(required, "assignment_scope")
		}
		if quantity <= 0 || quantity > 99999999.99 || math.IsNaN(quantity) || math.IsInf(quantity, 0) || math.Abs(quantity*100-math.Round(quantity*100)) > 0.000001 {
			required = append(required, "default_quantity")
		}
		if notes, ok := proposed["notes"].(string); ok && len([]rune(notes)) > 500 {
			required = append(required, "notes")
		}
	}
	if op == "archive" {
		proposed["lifecycle_status"] = "archived"
	} else if op == "restore" {
		proposed["lifecycle_status"] = "active"
		proposed["archived_at"] = nil
	}
	if op != "archive" && op != "restore" {
		proposed["is_optional"] = relation != "required" && relation != "included" && relation != "consumes"
	}
	// Mandatory dependency graphs cannot gain cycles. Compatibility/alternative
	// relationships can be reciprocal because they are discovery relationships.
	if proposed["lifecycle_status"] == "active" && (relation == "required" || relation == "included" || relation == "consumes") {
		var cycle bool
		if err = tx.QueryRow(`WITH RECURSIVE reachable AS (SELECT $1::int AS product_id UNION SELECT pd.dependency_product_id FROM reachable r JOIN product_dependencies pd ON pd.product_id=r.product_id WHERE pd.lifecycle_status='active' AND pd.relation_type IN ('required','included','consumes') AND pd.id<>$3) SELECT EXISTS(SELECT 1 FROM reachable WHERE product_id=$2::int)`, in.DependencyProductID, in.ProductID, id).Scan(&cycle); err != nil {
			respondWarehouseMutationError(w, err)
			return
		}
		if cycle {
			required = append(required, "dependency_cycle")
		}
	}
	diff := map[string]any{}
	for _, field := range []string{"relation_type", "assignment_scope", "default_quantity", "notes", "is_optional", "lifecycle_status"} {
		var before any
		if current != nil {
			before = current[field]
		}
		b, _ := json.Marshal(before)
		a, _ := json.Marshal(proposed[field])
		if string(a) != string(b) {
			diff[field] = map[string]any{"before": before, "after": proposed[field]}
		}
	}
	if current != nil && len(diff) == 0 {
		required = append(required, "changed_fields")
	}
	contextRaw, _ := json.Marshal(map[string]any{"current": current, "products": products, "active_jobs": jobs, "graph": graphHash, "draft": proposed})
	fingerprint := fmt.Sprintf("%x", sha256.Sum256(contextRaw))
	if in.ExpectedContext != "" && in.ExpectedContext != fingerprint {
		required = append(required, "expected_context")
	}
	phrase := fmt.Sprintf("%s WAREHOUSE PRODUCT RELATION %d", strings.ToUpper(op), id)
	if op == "create" || op == "link" {
		phrase = fmt.Sprintf("%s WAREHOUSE PRODUCT %d TO %d", strings.ToUpper(op), in.ProductID, in.DependencyProductID)
	}
	if !preview && in.ConfirmationText != phrase {
		respondJSON(w, 428, map[string]string{"error": "Exact record-bound confirmation from preview required"})
		return
	}
	version := ""
	if current != nil {
		version = current["updated_at"].(string)
	}
	result := map[string]any{"operation_status": "confirmation_required", "current": current, "draft": proposed, "products": products, "active_jobs": jobs, "diff": diff, "expected_updated_at": version, "expected_context": fingerprint, "required_confirmation_text": phrase, "ready_to_execute": len(required) == 0, "required_fields": required, "preview": true, "effects": map[string]any{"stock_movement": false, "existing_requirements_changed": false, "active_discovery_and_packing_expansion": proposed["lifecycle_status"] == "active"}}
	if preview || len(required) > 0 {
		if len(required) > 0 {
			result["operation_status"] = "needs_input"
		}
		respondJSON(w, 200, result)
		return
	}
	if current == nil {
		err = tx.QueryRow(`INSERT INTO product_dependencies(product_id,dependency_product_id,relation_type,assignment_scope,default_quantity,notes,is_optional) VALUES($1,$2,$3,$4,$5,$6,$7) RETURNING id`, in.ProductID, in.DependencyProductID, relation, scope, quantity, proposed["notes"], proposed["is_optional"]).Scan(&id)
	} else if op == "archive" || op == "restore" {
		_, err = tx.Exec(`UPDATE product_dependencies SET lifecycle_status=$1 WHERE id=$2`, proposed["lifecycle_status"], id)
	} else {
		_, err = tx.Exec(`UPDATE product_dependencies SET relation_type=$1,assignment_scope=$2,default_quantity=$3,notes=$4,is_optional=$5 WHERE id=$6`, relation, scope, quantity, proposed["notes"], proposed["is_optional"], id)
	}
	if err != nil {
		respondWarehouseMutationError(w, err)
		return
	}
	var afterRaw []byte
	if err = tx.QueryRow(warehouseRelationSnapshotSQL, id).Scan(&afterRaw); err != nil {
		respondWarehouseMutationError(w, err)
		return
	}
	var after map[string]any
	if err = json.Unmarshal(afterRaw, &after); err != nil {
		respondWarehouseMutationError(w, err)
		return
	}
	audit, err := maintenanceMCPAudit(tx, r, "product_relation."+op, "product_relation", fmt.Sprint(id), current, after, after["updated_at"].(string))
	if err != nil {
		respondWarehouseMutationError(w, err)
		return
	}
	status := "updated"
	if current == nil {
		status = "created"
	}
	if op == "archive" {
		status = "archived"
	}
	if op == "restore" {
		status = "restored"
	}
	result = map[string]any{"operation_status": status, "relationship": after, "audit_id": audit, "updated_at": after["updated_at"]}
	if err = completeWarehouseProductMutation(tx, receipt, 200, result); err == nil {
		err = tx.Commit()
	}
	if err != nil {
		respondWarehouseMutationError(w, err)
		return
	}
	respondJSON(w, 200, result)
}
func containsRelationType(value string) bool {
	switch value {
	case "required", "recommended", "compatible", "consumes", "alternative", "included":
		return true
	}
	return false
}
