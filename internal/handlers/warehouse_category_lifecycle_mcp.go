package handlers

import (
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/gorilla/mux"
	"warehousecore/internal/middleware"
	"warehousecore/internal/repository"
)

type warehouseCategoryLifecycleRequest struct {
	ID                   string `json:"id"`
	ExpectedUpdatedAt    string `json:"expected_updated_at"`
	ExpectedDependencies string `json:"expected_dependencies"`
	ConfirmLifecycle     bool   `json:"confirm_lifecycle"`
	ConfirmationText     string `json:"confirmation_text"`
	Preview              bool   `json:"preview"`
}

// Each SQL fragment belongs to a closed, named category level. No caller can
// supply a table, column, relationship query or mutation expression.
type categoryLifecycleSpec struct{ table, key, parent, products, children, ancestry string }

func warehouseCategoryLifecycleSpec(kind string) (categoryLifecycleSpec, bool) {
	switch kind {
	case "category":
		return categoryLifecycleSpec{"categories", "categoryid", "NULL::text", `p.categoryid=$1::int OR p.subcategoryid IN (SELECT subcategoryid FROM subcategories WHERE categoryid=$1::int) OR p.subbiercategoryid IN (SELECT t.subbiercategoryid FROM subbiercategories t JOIN subcategories s ON s.subcategoryid=t.subcategoryid WHERE s.categoryid=$1::int)`,
			`SELECT jsonb_build_object('id',subcategoryid,'name',name,'abbreviation',abbreviation,'parent',categoryid,'lifecycle_status',lifecycle_status,'updated_at',updated_at) AS v,lifecycle_status FROM subcategories WHERE categoryid=$1::int UNION ALL SELECT jsonb_build_object('id',t.subbiercategoryid,'name',t.name,'abbreviation',t.abbreviation,'parent',t.subcategoryid,'lifecycle_status',t.lifecycle_status,'updated_at',t.updated_at),t.lifecycle_status FROM subbiercategories t JOIN subcategories s ON s.subcategoryid=t.subcategoryid WHERE s.categoryid=$1::int`,
			`SELECT '[]'::jsonb,true`}, true
	case "subcategory":
		return categoryLifecycleSpec{"subcategories", "subcategoryid", "categoryid::text", `p.subcategoryid=$1 OR p.subbiercategoryid IN (SELECT subbiercategoryid FROM subbiercategories WHERE subcategoryid=$1)`,
			`SELECT jsonb_build_object('id',subbiercategoryid,'name',name,'abbreviation',abbreviation,'parent',subcategoryid,'lifecycle_status',lifecycle_status,'updated_at',updated_at) AS v,lifecycle_status FROM subbiercategories WHERE subcategoryid=$1`,
			`SELECT COALESCE(jsonb_agg(jsonb_build_object('id',c.categoryid,'name',c.name,'abbreviation',c.abbreviation,'lifecycle_status',c.lifecycle_status,'updated_at',to_char(c.updated_at,'YYYY-MM-DD"T"HH24:MI:SS.US"Z"'))),'[]'::jsonb),COALESCE(bool_and(c.lifecycle_status='active'),false) FROM categories c JOIN subcategories s ON s.categoryid=c.categoryid WHERE s.subcategoryid=$1`}, true
	case "third_category":
		return categoryLifecycleSpec{"subbiercategories", "subbiercategoryid", "subcategoryid", `p.subbiercategoryid=$1`,
			`SELECT '{}'::jsonb AS v,''::text AS lifecycle_status WHERE false AND $1::text IS NOT NULL`,
			`SELECT COALESCE(jsonb_agg(jsonb_build_object('id',s.subcategoryid,'name',s.name,'abbreviation',s.abbreviation,'lifecycle_status',s.lifecycle_status,'updated_at',to_char(s.updated_at,'YYYY-MM-DD"T"HH24:MI:SS.US"Z"'),'category_id',c.categoryid,'category_name',c.name,'category_lifecycle',c.lifecycle_status,'category_version',to_char(c.updated_at,'YYYY-MM-DD"T"HH24:MI:SS.US"Z"'))),'[]'::jsonb),COALESCE(bool_and(s.lifecycle_status='active' AND c.lifecycle_status='active'),false) FROM subcategories s JOIN categories c ON c.categoryid=s.categoryid JOIN subbiercategories t ON t.subcategoryid=s.subcategoryid WHERE t.subbiercategoryid=$1`}, true
	}
	return categoryLifecycleSpec{}, false
}
func validWarehouseCategoryLifecycleID(kind, id string) bool {
	if strings.TrimSpace(id) != id || id == "" || len([]rune(id)) > 50 {
		return false
	}
	if kind == "category" {
		n, err := strconv.ParseInt(id, 10, 32)
		return err == nil && n > 0 && strconv.FormatInt(n, 10) == id
	}
	return !strings.ContainsAny(id, "\x00\r\n")
}

func WarehouseCategoryLifecycleMCP(w http.ResponseWriter, r *http.Request) {
	user, ok := middleware.GetUserFromContext(r)
	if !ok || user == nil || user.UserID <= 0 || !user.IsAdmin || !isWarehouseMCPMutation(r) {
		respondJSON(w, 403, map[string]string{"error": "Signed-in warehouse administrator and guided MCP origin required"})
		return
	}
	kind, op := mux.Vars(r)["entity"], mux.Vars(r)["operation"]
	spec, ok := warehouseCategoryLifecycleSpec(kind)
	if !ok || op != "archive" && op != "restore" {
		respondJSON(w, 400, map[string]string{"error": "Unsupported category lifecycle operation"})
		return
	}
	var in warehouseCategoryLifecycleRequest
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16384))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&in); err != nil {
		respondJSON(w, 400, map[string]string{"error": "Invalid bounded lifecycle object"})
		return
	}
	if decoder.Decode(new(any)) != io.EOF || !validWarehouseCategoryLifecycleID(kind, in.ID) {
		respondJSON(w, 400, map[string]string{"error": "Exact canonical category ID and one JSON object required"})
		return
	}
	preview := in.Preview || !in.ConfirmLifecycle
	phrase := fmt.Sprintf("%s WAREHOUSE %s %s", strings.ToUpper(op), strings.ToUpper(kind), in.ID)
	if !preview && (in.ExpectedUpdatedAt == "" || len(in.ExpectedDependencies) != 64 || in.ConfirmationText != phrase) {
		respondJSON(w, 428, map[string]string{"error": "Exact version, dependency fingerprint and record-bound confirmation required"})
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
		var replay json.RawMessage
		receipt, replay, err = beginWarehouseProductMutation(tx, r, kind+"."+op, map[string]any{"id": in.ID, "expected_updated_at": in.ExpectedUpdatedAt, "expected_dependencies": in.ExpectedDependencies, "confirmation_text": phrase})
		if err != nil {
			respondWarehouseMutationError(w, err)
			return
		}
		if replay != nil {
			respondJSON(w, 200, replay)
			return
		}
	}
	// Also freeze the legacy UI and product writers during dependency validation.
	if _, err = tx.Exec(`LOCK TABLE categories,subcategories,subbiercategories IN SHARE ROW EXCLUSIVE MODE;LOCK TABLE products IN SHARE MODE`); err != nil {
		respondWarehouseMutationError(w, err)
		return
	}
	query := `SELECT jsonb_build_object('id',` + spec.key + `::text,'name',name,'abbreviation',abbreviation,'parent_id',` + spec.parent + `,'lifecycle_status',lifecycle_status,'archived_at',archived_at,'updated_at',to_char(updated_at,'YYYY-MM-DD"T"HH24:MI:SS.US"Z"')) FROM ` + spec.table + ` WHERE ` + spec.key + `=$1 FOR UPDATE`
	var raw []byte
	if err = tx.QueryRow(query, in.ID).Scan(&raw); err == sql.ErrNoRows {
		respondJSON(w, 404, map[string]string{"error": "Category not found"})
		return
	} else if err != nil {
		respondWarehouseMutationError(w, err)
		return
	}
	current := map[string]any{}
	if err = json.Unmarshal(raw, &current); err != nil {
		respondWarehouseMutationError(w, err)
		return
	}
	version := current["updated_at"].(string)
	var activeProducts, allProducts, activeChildren, allChildren int64
	var productsHash, childrenHash string
	productQuery := `SELECT count(*) FILTER(WHERE p.lifecycle_status='active'),count(*),encode(sha256(convert_to(COALESCE(string_agg(jsonb_build_object('id',p.productid,'name',p.name,'category_id',p.categoryid,'subcategory_id',p.subcategoryid,'third_category_id',p.subbiercategoryid,'lifecycle_status',p.lifecycle_status,'updated_at',p.updated_at)::text,',' ORDER BY p.productid),''),'UTF8')),'hex') FROM products p WHERE ` + spec.products
	if err = tx.QueryRow(productQuery, in.ID).Scan(&activeProducts, &allProducts, &productsHash); err != nil {
		respondWarehouseMutationError(w, err)
		return
	}
	if err = tx.QueryRow(`SELECT count(*) FILTER(WHERE lifecycle_status='active'),count(*),encode(sha256(convert_to(COALESCE(string_agg(v::text,',' ORDER BY v::text),''),'UTF8')),'hex') FROM (`+spec.children+`) refs`, in.ID).Scan(&activeChildren, &allChildren, &childrenHash); err != nil {
		respondWarehouseMutationError(w, err)
		return
	}
	var parentsRaw []byte
	var activeParents bool
	parentArgs := []any{in.ID}
	if kind == "category" {
		parentArgs = nil
	}
	if err = tx.QueryRow(spec.ancestry, parentArgs...).Scan(&parentsRaw, &activeParents); err != nil {
		respondWarehouseMutationError(w, err)
		return
	}
	var parents any
	if err = json.Unmarshal(parentsRaw, &parents); err != nil {
		respondWarehouseMutationError(w, err)
		return
	}
	dependencies := map[string]any{"active_products": activeProducts, "all_products": allProducts, "active_children": activeChildren, "all_children": allChildren, "parents": parents}
	fingerprintRaw, _ := json.Marshal(map[string]any{"record": current, "dependencies": dependencies, "products": productsHash, "children": childrenHash})
	fingerprint := fmt.Sprintf("%x", sha256.Sum256(fingerprintRaw))
	required := []string{}
	if in.ExpectedUpdatedAt != "" && in.ExpectedUpdatedAt != version {
		required = append(required, "expected_updated_at")
	}
	if in.ExpectedDependencies != "" && in.ExpectedDependencies != fingerprint {
		required = append(required, "expected_dependencies")
	}
	from, to := "active", "archived"
	if op == "restore" {
		from, to = to, from
	}
	if current["lifecycle_status"] != from {
		required = append(required, "lifecycle_status")
	}
	if op == "archive" && (activeProducts > 0 || activeChildren > 0) {
		required = append(required, "active_dependencies")
	}
	if op == "restore" {
		if !activeParents {
			required = append(required, "active_parent_ancestry")
		}
		name, _ := current["name"].(string)
		abbr, _ := current["abbreviation"].(string)
		if strings.TrimSpace(name) == "" || len([]rune(name)) > 100 || len([]rune(abbr)) > 10 || kind == "category" && strings.TrimSpace(abbr) == "" {
			required = append(required, "invalid_retained_fields")
		}
		duplicateQuery := `SELECT EXISTS(SELECT 1 FROM ` + spec.table + ` WHERE ` + spec.key + `<>$1 AND lower(trim(name))=lower(trim($2))`
		args := []any{in.ID, name}
		if kind != "category" {
			column := "categoryid"
			if kind == "third_category" {
				column = "subcategoryid"
			}
			duplicateQuery += ` AND ` + column + `::text IS NOT DISTINCT FROM $3::text`
			args = append(args, current["parent_id"])
		}
		var duplicate bool
		if err = tx.QueryRow(duplicateQuery+`)`, args...).Scan(&duplicate); err != nil {
			respondWarehouseMutationError(w, err)
			return
		}
		if duplicate {
			required = append(required, "duplicate_retained_identity")
		}
	}
	proposed := map[string]any{}
	for k, v := range current {
		proposed[k] = v
	}
	proposed["lifecycle_status"] = to
	// The archive timestamp/version are generated only on execution, not invented.
	if to == "active" {
		proposed["archived_at"] = nil
	}
	result := map[string]any{"operation_status": "confirmation_required", "current": current, "draft": proposed, "dependencies": dependencies, "diff": map[string]any{"lifecycle_status": map[string]any{"before": current["lifecycle_status"], "after": to}}, "expected_updated_at": version, "expected_dependencies": fingerprint, "required_confirmation_text": phrase, "ready_to_execute": len(required) == 0, "required_fields": required, "preview": true}
	if preview || len(required) > 0 {
		if len(required) > 0 {
			result["operation_status"] = "needs_input"
		}
		respondJSON(w, 200, result)
		return
	}
	if _, err = tx.Exec(`UPDATE `+spec.table+` SET lifecycle_status=$1 WHERE `+spec.key+`=$2`, to, in.ID); err != nil {
		respondWarehouseMutationError(w, err)
		return
	}
	if err = tx.QueryRow(query, in.ID).Scan(&raw); err != nil {
		respondWarehouseMutationError(w, err)
		return
	}
	if err = json.Unmarshal(raw, &proposed); err != nil {
		respondWarehouseMutationError(w, err)
		return
	}
	newRaw, _ := json.Marshal(map[string]any{"origin": "MCP/AI", "after": proposed, "updated_at": proposed["updated_at"]})
	oldRaw, _ := json.Marshal(current)
	var audit int64
	if err = tx.QueryRow(`INSERT INTO audit_log(user_id,action,entity_type,entity_id,old_values,new_values,user_agent) VALUES($1,$2,$3,$4,$5::jsonb,$6::jsonb,$7) RETURNING id`, user.UserID, kind+"."+op, kind, in.ID, string(oldRaw), string(newRaw), r.UserAgent()).Scan(&audit); err != nil {
		respondWarehouseMutationError(w, err)
		return
	}
	status := "archived"
	if op == "restore" {
		status = "restored"
	}
	result = map[string]any{"operation_status": status, kind: proposed, "audit_id": audit, "updated_at": proposed["updated_at"]}
	if err = completeWarehouseProductMutation(tx, receipt, 200, result); err == nil {
		err = tx.Commit()
	}
	if err != nil {
		respondWarehouseMutationError(w, err)
		return
	}
	respondJSON(w, 200, result)
}
