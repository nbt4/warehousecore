package handlers

import (
	"database/sql"
	"net/http"
	"strconv"
	"strings"

	"warehousecore/internal/repository"
)

func createWarehouseCategoryMCP(w http.ResponseWriter, r *http.Request, kind, rawName, rawAbbreviation string, parent any) {
	name, abbreviation := strings.TrimSpace(rawName), strings.TrimSpace(rawAbbreviation)
	if name == "" || len([]rune(name)) > 100 || len([]rune(abbreviation)) > 10 || kind == "category" && abbreviation == "" {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "Category name must contain 1-100 characters and abbreviation at most 10; top-level abbreviation is required"})
		return
	}
	parentKey := ""
	switch kind {
	case "category":
		parent = nil
	case "subcategory":
		id, ok := parent.(int)
		if !ok || id <= 0 {
			respondJSON(w, http.StatusBadRequest, map[string]string{"error": "Existing category_id is required"})
			return
		}
		parentKey = strconv.Itoa(id)
	case "third_category":
		id, ok := parent.(string)
		if !ok || strings.TrimSpace(id) == "" {
			respondJSON(w, http.StatusBadRequest, map[string]string{"error": "Existing subcategory_id is required"})
			return
		}
		parentKey = strings.TrimSpace(id)
		parent = parentKey
	default:
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "Unsupported category level"})
		return
	}
	tx, err := repository.GetSQLDB().BeginTx(r.Context(), nil)
	if err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to begin category creation"})
		return
	}
	defer tx.Rollback()
	receiptID, replay, err := beginWarehouseProductMutation(tx, r, kind+".create", map[string]any{"name": name, "abbreviation": abbreviation, "parent": parentKey})
	if err != nil {
		respondWarehouseMutationError(w, err)
		return
	}
	if replay != nil {
		respondJSON(w, http.StatusCreated, replay)
		return
	}
	if kind != "category" {
		var found int
		var parentQuery string
		if kind == "subcategory" {
			parentQuery = `SELECT categoryid FROM categories WHERE categoryid=$1 FOR KEY SHARE`
		} else {
			parentQuery = `SELECT 1 FROM subcategories WHERE subcategoryid=$1 FOR KEY SHARE`
		}
		if err := tx.QueryRow(parentQuery, parent).Scan(&found); err == sql.ErrNoRows {
			respondJSON(w, http.StatusNotFound, map[string]string{"error": "Parent category not found"})
			return
		} else if err != nil {
			respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to load parent category"})
			return
		}
	}
	if _, err := tx.Exec(`SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, "warehouse."+kind+"."+parentKey+"."+strings.ToLower(name)); err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to lock category name"})
		return
	}
	var duplicateQuery, insertQuery, idField, parentField string
	switch kind {
	case "category":
		duplicateQuery = `SELECT categoryid::text FROM categories WHERE lower(trim(name))=lower($1) LIMIT 1`
		insertQuery = `INSERT INTO categories(name,abbreviation) VALUES($1,$2) RETURNING categoryid::text`
		idField = "category_id"
	case "subcategory":
		duplicateQuery = `SELECT subcategoryid FROM subcategories WHERE categoryid=$2 AND lower(trim(name))=lower($1) LIMIT 1`
		insertQuery = `INSERT INTO subcategories(subcategoryid,name,abbreviation,categoryid) VALUES(gen_random_uuid()::varchar,$1,$2,$3) RETURNING subcategoryid`
		idField, parentField = "subcategory_id", "category_id"
	case "third_category":
		duplicateQuery = `SELECT subbiercategoryid FROM subbiercategories WHERE subcategoryid=$2 AND lower(trim(name))=lower($1) LIMIT 1`
		insertQuery = `INSERT INTO subbiercategories(subbiercategoryid,name,abbreviation,subcategoryid) VALUES(gen_random_uuid()::varchar,$1,$2,$3) RETURNING subbiercategoryid`
		idField, parentField = "subbiercategory_id", "subcategory_id"
	}
	var duplicateID string
	var duplicateErr error
	if kind == "category" {
		duplicateErr = tx.QueryRow(duplicateQuery, name).Scan(&duplicateID)
	} else {
		duplicateErr = tx.QueryRow(duplicateQuery, name, parent).Scan(&duplicateID)
	}
	if duplicateErr == nil {
		respondJSON(w, http.StatusConflict, map[string]any{"error": "Category already exists under this parent", idField: duplicateID})
		return
	} else if duplicateErr != sql.ErrNoRows {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to check category duplicates"})
		return
	}
	var id string
	if kind == "category" {
		err = tx.QueryRow(insertQuery, name, abbreviation).Scan(&id)
	} else {
		err = tx.QueryRow(insertQuery, name, abbreviation, parent).Scan(&id)
	}
	if err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to create category"})
		return
	}
	response := map[string]any{"name": name, "abbreviation": abbreviation}
	if kind == "category" {
		parsed, _ := strconv.Atoi(id)
		response[idField] = parsed
	} else {
		response[idField] = id
		response[parentField] = parent
	}
	after := map[string]any{"origin": "MCP/AI", "name": name, "abbreviation": abbreviation, idField: response[idField]}
	if parentField != "" {
		after[parentField] = parent
	}
	if err := recordWarehouseMasterAudit(tx, r, kind, id, after); err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to audit category"})
		return
	}
	if err := completeWarehouseProductMutation(tx, receiptID, http.StatusCreated, response); err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to store category receipt"})
		return
	}
	if err := tx.Commit(); err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to commit category"})
		return
	}
	respondJSON(w, http.StatusCreated, response)
}
