package handlers

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"warehousecore/internal/middleware"
	"warehousecore/internal/repository"
)

func warehouseCategoryDeletionPhrase(kind, id string) string {
	return "DELETE WAREHOUSE " + strings.ToUpper(kind) + " " + id
}

func deleteWarehouseCategoryMCP(w http.ResponseWriter, r *http.Request, kind, id string) {
	user, ok := middleware.GetUserFromContext(r)
	if !ok || user == nil || !user.IsAdmin {
		respondJSON(w, http.StatusForbidden, map[string]string{"error": "Warehouse administrator permission is required"})
		return
	}
	var input struct {
		ExpectedUpdatedAt string `json:"expected_updated_at"`
		ConfirmationText  string `json:"confirmation_text"`
		ConfirmDelete     bool   `json:"confirm_delete"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid deletion body"})
		return
	}
	if input.ExpectedUpdatedAt == "" {
		respondJSON(w, http.StatusPreconditionRequired, map[string]string{"error": "The exact expected_updated_at from the preview is required"})
		return
	}
	var table, column, parentExpr string
	switch kind {
	case "category":
		value, err := strconv.ParseInt(id, 10, 32)
		if err != nil || value <= 0 {
			respondJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid category ID"})
			return
		}
		id = strconv.FormatInt(value, 10)
		table, column, parentExpr = "categories", "categoryid", "NULL::text"
	case "subcategory":
		table, column, parentExpr = "subcategories", "subcategoryid", "categoryid::text"
	case "third_category":
		table, column, parentExpr = "subbiercategories", "subbiercategoryid", "subcategoryid"
	default:
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "Unsupported category level"})
		return
	}
	if strings.TrimSpace(id) == "" || len([]rune(id)) > 50 {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid category ID"})
		return
	}
	if !input.ConfirmDelete || input.ConfirmationText != warehouseCategoryDeletionPhrase(kind, id) {
		respondJSON(w, http.StatusPreconditionRequired, map[string]string{"error": "Explicit confirmation and the exact record-bound deletion phrase are required"})
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
	receiptID, replay, err := beginWarehouseProductMutation(tx, r, kind+".delete", map[string]any{"id": id, "expected_updated_at": input.ExpectedUpdatedAt, "confirmation_text": input.ConfirmationText})
	if err != nil {
		respondWarehouseMutationError(w, err)
		return
	}
	if replay != nil {
		respondJSON(w, http.StatusOK, replay)
		return
	}
	// Block inserts and edits until every current reference has been checked.
	if _, err = tx.Exec(`LOCK TABLE categories,subcategories,subbiercategories IN SHARE ROW EXCLUSIVE MODE; LOCK TABLE products IN SHARE MODE`); err != nil {
		respondWarehouseMutationError(w, err)
		return
	}
	var name, version string
	var abbreviation, parent sql.NullString
	query := fmt.Sprintf(`SELECT name,abbreviation,%s,to_char(updated_at,'YYYY-MM-DD"T"HH24:MI:SS.US"Z"') FROM %s WHERE %s=$1 FOR UPDATE`, parentExpr, table, column)
	err = tx.QueryRow(query, id).Scan(&name, &abbreviation, &parent, &version)
	if err == sql.ErrNoRows {
		respondJSON(w, http.StatusNotFound, map[string]string{"error": "Category not found"})
		return
	}
	if err != nil {
		respondWarehouseMutationError(w, err)
		return
	}
	if version != input.ExpectedUpdatedAt {
		respondJSON(w, http.StatusConflict, map[string]string{"error": "Category changed since the deletion preview"})
		return
	}
	var products, children int64
	usageQuery := `SELECT (SELECT COUNT(*) FROM products WHERE categoryid=$1),(SELECT COUNT(*) FROM subcategories WHERE categoryid=$1)`
	if kind == "subcategory" {
		usageQuery = `SELECT (SELECT COUNT(*) FROM products WHERE subcategoryid=$1),(SELECT COUNT(*) FROM subbiercategories WHERE subcategoryid=$1)`
	}
	if kind == "third_category" {
		usageQuery = `SELECT COUNT(*),0 FROM products WHERE subbiercategoryid=$1`
	}
	if err = tx.QueryRow(usageQuery, id).Scan(&products, &children); err != nil {
		respondWarehouseMutationError(w, err)
		return
	}
	if products > 0 || children > 0 {
		respondJSON(w, http.StatusConflict, map[string]any{"error": "Only unused categories without children can be deleted", "product_count": products, "child_count": children})
		return
	}
	// Reject unknown referencing tables as well: SET NULL/CASCADE must never
	// silently modify extension data through this narrowly scoped MCP workflow.
	var unsupportedReferences bool
	if err = tx.QueryRow(`SELECT EXISTS(SELECT 1 FROM pg_constraint WHERE contype='f' AND confrelid=to_regclass($1) AND conrelid NOT IN (to_regclass('products'),to_regclass('categories'),to_regclass('subcategories'),to_regclass('subbiercategories')))`, table).Scan(&unsupportedReferences); err != nil {
		respondWarehouseMutationError(w, err)
		return
	}
	if unsupportedReferences {
		respondJSON(w, http.StatusConflict, map[string]string{"error": "Additional foreign-key references require review before this category can be deleted"})
		return
	}
	before := map[string]any{"name": name, "abbreviation": nil, "updated_at": version}
	if abbreviation.Valid {
		before["abbreviation"] = abbreviation.String
	}
	if parent.Valid {
		before["parent_id"] = parent.String
	}
	if _, err = tx.Exec(fmt.Sprintf(`DELETE FROM %s WHERE %s=$1`, table, column), id); err != nil {
		respondWarehouseMutationError(w, err)
		return
	}
	oldJSON, err := json.Marshal(before)
	if err != nil {
		respondWarehouseMutationError(w, err)
		return
	}
	if _, err = tx.Exec(`INSERT INTO audit_log(user_id,action,entity_type,entity_id,old_values,new_values,user_agent) VALUES($1,$2,$3,$4,$5::jsonb,'{"origin":"MCP/AI","deleted":true}'::jsonb,$6)`, user.UserID, kind+".delete", kind, id, string(oldJSON), r.UserAgent()); err != nil {
		respondWarehouseMutationError(w, err)
		return
	}
	response := map[string]any{"deleted": true, "entity": kind, "id": id, "name": name, "previous_updated_at": version}
	if err = completeWarehouseProductMutation(tx, receiptID, http.StatusOK, response); err != nil {
		respondWarehouseMutationError(w, err)
		return
	}
	if err = tx.Commit(); err != nil {
		respondWarehouseMutationError(w, err)
		return
	}
	respondJSON(w, http.StatusOK, response)
}
