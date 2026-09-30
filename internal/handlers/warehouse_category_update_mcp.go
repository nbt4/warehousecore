package handlers

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"reflect"
	"strconv"
	"strings"

	"warehousecore/internal/middleware"
	"warehousecore/internal/repository"
)

type warehouseCategoryUpdateInput struct {
	Name              string  `json:"name"`
	Abbreviation      *string `json:"abbreviation"`
	CategoryID        *int64  `json:"category_id"`
	SubcategoryID     *string `json:"subcategory_id"`
	ExpectedUpdatedAt string  `json:"expected_updated_at"`
}

func updateWarehouseCategoryMCP(w http.ResponseWriter, r *http.Request, kind, id string) {
	user, ok := middleware.GetUserFromContext(r)
	if !ok || user == nil || !user.IsAdmin {
		respondJSON(w, http.StatusForbidden, map[string]string{"error": "Warehouse administrator permission is required"})
		return
	}
	var input warehouseCategoryUpdateInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid category update body"})
		return
	}
	if strings.TrimSpace(input.ExpectedUpdatedAt) == "" {
		respondJSON(w, http.StatusPreconditionRequired, map[string]string{"error": "The exact expected_updated_at from the preview is required"})
		return
	}
	input.Name = strings.TrimSpace(input.Name)
	if input.Abbreviation != nil {
		value := strings.TrimSpace(*input.Abbreviation)
		input.Abbreviation = &value
	}
	invalid := input.Name == "" || len([]rune(input.Name)) > 100 || input.Abbreviation != nil && len([]rune(*input.Abbreviation)) > 10
	var table, idColumn, idField, parentField, parentExpression string
	var parent any
	switch kind {
	case "category":
		table, idColumn, idField, parentExpression = "categories", "categoryid", "category_id", "NULL::text"
		value, err := strconv.ParseInt(id, 10, 32)
		invalid = invalid || err != nil || value <= 0 || input.Abbreviation == nil || *input.Abbreviation == ""
	case "subcategory":
		table, idColumn, idField, parentField, parentExpression = "subcategories", "subcategoryid", "subcategory_id", "category_id", "categoryid::text"
		invalid = invalid || input.CategoryID == nil || *input.CategoryID <= 0 || *input.CategoryID > math.MaxInt32
		if input.CategoryID != nil {
			parent = *input.CategoryID
		}
	case "third_category":
		table, idColumn, idField, parentField, parentExpression = "subbiercategories", "subbiercategoryid", "subbiercategory_id", "subcategory_id", "subcategoryid"
		if input.SubcategoryID != nil {
			value := strings.TrimSpace(*input.SubcategoryID)
			parent = value
			invalid = invalid || value == "" || len([]rune(value)) > 50
		} else {
			invalid = true
		}
	default:
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "Unsupported category level"})
		return
	}
	if invalid || strings.TrimSpace(id) == "" || len([]rune(id)) > 50 {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid category name, abbreviation, ID or parent"})
		return
	}
	after := map[string]any{"name": input.Name, "abbreviation": nil}
	if input.Abbreviation != nil {
		after["abbreviation"] = *input.Abbreviation
	}
	if parentField != "" {
		after[parentField] = parent
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
	receiptID, replay, err := beginWarehouseProductMutation(tx, r, kind+".update", map[string]any{"id": id, "after": after, "expected_updated_at": input.ExpectedUpdatedAt})
	if err != nil {
		respondWarehouseMutationError(w, err)
		return
	}
	if replay != nil {
		respondJSON(w, http.StatusOK, replay)
		return
	}
	// Include legacy UI writers while validating hierarchy and duplicate names.
	if _, err = tx.Exec(`LOCK TABLE categories,subcategories,subbiercategories IN SHARE ROW EXCLUSIVE MODE`); err != nil {
		respondWarehouseMutationError(w, err)
		return
	}
	var currentName, version string
	var currentAbbreviation, currentParent sql.NullString
	query := fmt.Sprintf(`SELECT name,abbreviation,%s,to_char(updated_at,'YYYY-MM-DD"T"HH24:MI:SS.US"Z"') FROM %s WHERE %s=$1 FOR UPDATE`, parentExpression, table, idColumn)
	err = tx.QueryRow(query, id).Scan(&currentName, &currentAbbreviation, &currentParent, &version)
	if err == sql.ErrNoRows {
		respondJSON(w, http.StatusNotFound, map[string]string{"error": "Category not found"})
		return
	}
	if err != nil {
		respondWarehouseMutationError(w, err)
		return
	}
	if version != input.ExpectedUpdatedAt {
		respondJSON(w, http.StatusConflict, map[string]string{"error": "Category changed since the preview"})
		return
	}
	before := map[string]any{"name": currentName, "abbreviation": nil}
	if currentAbbreviation.Valid {
		before["abbreviation"] = currentAbbreviation.String
	}
	if parentField != "" {
		before[parentField] = nil
		if currentParent.Valid {
			if kind == "subcategory" {
				value, parseErr := strconv.ParseInt(currentParent.String, 10, 64)
				if parseErr != nil {
					respondWarehouseMutationError(w, parseErr)
					return
				}
				before[parentField] = value
			} else {
				before[parentField] = currentParent.String
			}
		}
		var parentCategory int64
		parentQuery := `SELECT categoryid FROM categories WHERE categoryid=$1`
		if kind == "third_category" {
			parentQuery = `SELECT s.categoryid FROM subcategories s JOIN categories c ON c.categoryid=s.categoryid WHERE s.subcategoryid=$1`
		}
		err = tx.QueryRow(parentQuery, parent).Scan(&parentCategory)
		if err == sql.ErrNoRows {
			respondJSON(w, http.StatusNotFound, map[string]string{"error": "Parent category or its ancestry not found"})
			return
		}
		if err != nil {
			respondWarehouseMutationError(w, err)
			return
		}
		if !reflect.DeepEqual(before[parentField], parent) {
			if _, err = tx.Exec(`LOCK TABLE products IN SHARE MODE`); err != nil {
				respondWarehouseMutationError(w, err)
				return
			}
			var conflict bool
			dependencyQuery := `SELECT EXISTS(SELECT 1 FROM products p WHERE (p.subcategoryid=$1 OR p.subbiercategoryid IN (SELECT subbiercategoryid FROM subbiercategories WHERE subcategoryid=$1)) AND (p.categoryid IS DISTINCT FROM $2::int OR p.subcategoryid IS DISTINCT FROM $1))`
			args := []any{id, parentCategory}
			if kind == "third_category" {
				dependencyQuery = `SELECT EXISTS(SELECT 1 FROM products WHERE subbiercategoryid=$1 AND (subcategoryid IS DISTINCT FROM $2::varchar OR categoryid IS DISTINCT FROM $3::int))`
				args = []any{id, parent, parentCategory}
			}
			if err = tx.QueryRow(dependencyQuery, args...).Scan(&conflict); err != nil {
				respondWarehouseMutationError(w, err)
				return
			}
			if conflict {
				respondJSON(w, http.StatusConflict, map[string]string{"error": "Moving this category conflicts with linked products; review and change product assignments first"})
				return
			}
		}
	}
	if reflect.DeepEqual(before, after) {
		respondJSON(w, http.StatusConflict, map[string]string{"error": "No category fields changed"})
		return
	}
	duplicateQuery := fmt.Sprintf(`SELECT EXISTS(SELECT 1 FROM %s WHERE %s<>$1 AND lower(trim(name))=lower($2)`, table, idColumn)
	args := []any{id, input.Name}
	if parentField != "" {
		duplicateQuery += " AND " + strings.ReplaceAll(parentField, "_", "") + "=$3"
		args = append(args, parent)
	}
	duplicateQuery += ")"
	var duplicate bool
	if err = tx.QueryRow(duplicateQuery, args...).Scan(&duplicate); err != nil {
		respondWarehouseMutationError(w, err)
		return
	}
	if duplicate {
		respondJSON(w, http.StatusConflict, map[string]string{"error": "Category name already exists under this parent"})
		return
	}
	updateQuery := fmt.Sprintf(`UPDATE %s SET name=$1,abbreviation=$2`, table)
	args = []any{input.Name, input.Abbreviation, id}
	if parentField != "" {
		updateQuery += "," + strings.ReplaceAll(parentField, "_", "") + "=$4"
		args = append(args, parent)
	}
	updateQuery += fmt.Sprintf(` WHERE %s=$3 RETURNING to_char(updated_at,'YYYY-MM-DD"T"HH24:MI:SS.US"Z"')`, idColumn)
	if err = tx.QueryRow(updateQuery, args...).Scan(&version); err != nil {
		respondWarehouseMutationError(w, err)
		return
	}
	oldJSON, err := json.Marshal(before)
	if err != nil {
		respondWarehouseMutationError(w, err)
		return
	}
	newJSON, err := json.Marshal(map[string]any{"origin": "MCP/AI", "after": after})
	if err != nil {
		respondWarehouseMutationError(w, err)
		return
	}
	if _, err = tx.Exec(`INSERT INTO audit_log(user_id,action,entity_type,entity_id,old_values,new_values,user_agent) VALUES($1,$2,$3,$4,$5::jsonb,$6::jsonb,$7)`, user.UserID, kind+".update", kind, id, string(oldJSON), string(newJSON), r.UserAgent()); err != nil {
		respondWarehouseMutationError(w, err)
		return
	}
	response := map[string]any{idField: id, "name": input.Name, "abbreviation": after["abbreviation"], "updated_at": version}
	if kind == "category" {
		value, _ := strconv.ParseInt(id, 10, 32)
		response[idField] = value
	}
	if parentField != "" {
		response[parentField] = parent
	}
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
