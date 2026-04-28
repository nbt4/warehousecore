package handlers

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/gorilla/mux"

	"warehousecore/internal/repository"
)

// Category represents a top-level category
type Category struct {
	CategoryID   int    `json:"category_id"`
	Name         string `json:"name"`
	Abbreviation string `json:"abbreviation"`
}

// Subcategory represents a second-level category
type Subcategory struct {
	SubcategoryID string `json:"subcategory_id"`
	Name          string `json:"name"`
	Abbreviation  string `json:"abbreviation"`
	CategoryID    int    `json:"category_id"`
}

// Subbiercategory represents a third-level category
type Subbiercategory struct {
	SubbiercategoryID string `json:"subbiercategory_id"`
	Name              string `json:"name"`
	Abbreviation      string `json:"abbreviation"`
	SubcategoryID     string `json:"subcategory_id"`
}

// GetCategories retrieves all categories
func GetCategories(w http.ResponseWriter, r *http.Request) {
	db := repository.GetSQLDB()
	rows, err := db.Query("SELECT categoryID, name, abbreviation FROM categories ORDER BY name")
	if err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to fetch categories"})
		return
	}
	defer rows.Close()

	var categories []Category
	for rows.Next() {
		var c Category
		if err := rows.Scan(&c.CategoryID, &c.Name, &c.Abbreviation); err != nil {
			continue
		}
		categories = append(categories, c)
	}

	respondJSON(w, http.StatusOK, categories)
}

// CreateCategory creates a new category
func CreateCategory(w http.ResponseWriter, r *http.Request) {
	var req Category
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid request body"})
		return
	}

	if req.Name == "" || req.Abbreviation == "" {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "Name and abbreviation are required"})
		return
	}

	db := repository.GetSQLDB()
	var id int64
	err := db.QueryRow(
		"INSERT INTO categories (name, abbreviation) VALUES ($1, $2) RETURNING categoryID",
		req.Name, req.Abbreviation,
	).Scan(&id)
	if err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to create category"})
		return
	}

	req.CategoryID = int(id)

	respondJSON(w, http.StatusCreated, req)
}

// UpdateCategory updates an existing category
func UpdateCategory(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id, err := strconv.Atoi(vars["id"])
	if err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid category ID"})
		return
	}

	var req Category
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid request body"})
		return
	}

	db := repository.GetSQLDB()
	result, err := db.Exec(
		"UPDATE categories SET name = $1, abbreviation = $2 WHERE categoryID = $3",
		req.Name, req.Abbreviation, id,
	)
	if err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to update category"})
		return
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		respondJSON(w, http.StatusNotFound, map[string]string{"error": "Category not found"})
		return
	}

	respondJSON(w, http.StatusOK, map[string]string{"message": "Category updated successfully"})
}

// DeleteCategory deletes a category
func DeleteCategory(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id, err := strconv.Atoi(vars["id"])
	if err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid category ID"})
		return
	}

	db := repository.GetSQLDB()
	result, err := db.Exec("DELETE FROM categories WHERE categoryID = $1", id)
	if err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to delete category"})
		return
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		respondJSON(w, http.StatusNotFound, map[string]string{"error": "Category not found"})
		return
	}

	respondJSON(w, http.StatusOK, map[string]string{"message": "Category deleted successfully"})
}

// GetSubcategories retrieves subcategories, optionally filtered by category_id
func GetSubcategories(w http.ResponseWriter, r *http.Request) {
	categoryID := r.URL.Query().Get("category_id")

	db := repository.GetSQLDB()
	var rows *sql.Rows
	var err error

	if categoryID != "" {
		rows, err = db.Query(
			"SELECT subcategoryID, name, abbreviation, categoryID FROM subcategories WHERE categoryID = $1 ORDER BY name",
			categoryID,
		)
	} else {
		rows, err = db.Query("SELECT subcategoryID, name, abbreviation, categoryID FROM subcategories ORDER BY name")
	}

	if err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to fetch subcategories"})
		return
	}
	defer rows.Close()

	var subcategories []Subcategory
	for rows.Next() {
		var sc Subcategory
		if err := rows.Scan(&sc.SubcategoryID, &sc.Name, &sc.Abbreviation, &sc.CategoryID); err != nil {
			continue
		}
		subcategories = append(subcategories, sc)
	}

	respondJSON(w, http.StatusOK, subcategories)
}

// CreateSubcategory creates a new subcategory
func CreateSubcategory(w http.ResponseWriter, r *http.Request) {
	var req Subcategory
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid request body"})
		return
	}

	if req.Name == "" || req.CategoryID == 0 {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "Name and category_id are required"})
		return
	}

	db := repository.GetSQLDB()
	var newID string
	err := db.QueryRow(
		"INSERT INTO subcategories (subcategoryID, name, abbreviation, categoryID) VALUES (gen_random_uuid()::varchar, $1, $2, $3) RETURNING subcategoryID",
		req.Name, req.Abbreviation, req.CategoryID,
	).Scan(&newID)
	if err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to create subcategory"})
		return
	}
	req.SubcategoryID = newID

	respondJSON(w, http.StatusCreated, req)
}

// UpdateSubcategory updates an existing subcategory
func UpdateSubcategory(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]

	var req Subcategory
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid request body"})
		return
	}

	db := repository.GetSQLDB()
	result, err := db.Exec(
		"UPDATE subcategories SET name = $1, abbreviation = $2, categoryID = $3 WHERE subcategoryID = $4",
		req.Name, req.Abbreviation, req.CategoryID, id,
	)
	if err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to update subcategory"})
		return
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		respondJSON(w, http.StatusNotFound, map[string]string{"error": "Subcategory not found"})
		return
	}

	respondJSON(w, http.StatusOK, map[string]string{"message": "Subcategory updated successfully"})
}

// DeleteSubcategory deletes a subcategory
func DeleteSubcategory(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]

	db := repository.GetSQLDB()
	result, err := db.Exec("DELETE FROM subcategories WHERE subcategoryID = $1", id)
	if err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to delete subcategory"})
		return
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		respondJSON(w, http.StatusNotFound, map[string]string{"error": "Subcategory not found"})
		return
	}

	respondJSON(w, http.StatusOK, map[string]string{"message": "Subcategory deleted successfully"})
}

// GetSubbiercategories retrieves subbiercategories, optionally filtered by subcategory_id
func GetSubbiercategories(w http.ResponseWriter, r *http.Request) {
	subcategoryID := r.URL.Query().Get("subcategory_id")

	db := repository.GetSQLDB()
	var rows *sql.Rows
	var err error

	if subcategoryID != "" {
		rows, err = db.Query(
			"SELECT subbiercategoryID, name, abbreviation, subcategoryID FROM subbiercategories WHERE subcategoryID = $1 ORDER BY name",
			subcategoryID,
		)
	} else {
		rows, err = db.Query("SELECT subbiercategoryID, name, abbreviation, subcategoryID FROM subbiercategories ORDER BY name")
	}

	if err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to fetch subbiercategories"})
		return
	}
	defer rows.Close()

	var subbiercategories []Subbiercategory
	for rows.Next() {
		var sbc Subbiercategory
		if err := rows.Scan(&sbc.SubbiercategoryID, &sbc.Name, &sbc.Abbreviation, &sbc.SubcategoryID); err != nil {
			continue
		}
		subbiercategories = append(subbiercategories, sbc)
	}

	respondJSON(w, http.StatusOK, subbiercategories)
}

// CreateSubbiercategory creates a new subbiercategory
func CreateSubbiercategory(w http.ResponseWriter, r *http.Request) {
	var req Subbiercategory
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid request body"})
		return
	}

	if req.Name == "" || req.SubcategoryID == "" {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "Name and subcategory_id are required"})
		return
	}

	db := repository.GetSQLDB()
	var newID string
	err := db.QueryRow(
		"INSERT INTO subbiercategories (subbiercategoryID, name, abbreviation, subcategoryID) VALUES (gen_random_uuid()::varchar, $1, $2, $3) RETURNING subbiercategoryID",
		req.Name, req.Abbreviation, req.SubcategoryID,
	).Scan(&newID)
	if err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to create subbiercategory"})
		return
	}
	req.SubbiercategoryID = newID

	respondJSON(w, http.StatusCreated, req)
}

// UpdateSubbiercategory updates an existing subbiercategory
func UpdateSubbiercategory(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]

	var req Subbiercategory
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid request body"})
		return
	}

	db := repository.GetSQLDB()
	result, err := db.Exec(
		"UPDATE subbiercategories SET name = $1, abbreviation = $2, subcategoryID = $3 WHERE subbiercategoryID = $4",
		req.Name, req.Abbreviation, req.SubcategoryID, id,
	)
	if err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to update subbiercategory"})
		return
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		respondJSON(w, http.StatusNotFound, map[string]string{"error": "Subbiercategory not found"})
		return
	}

	respondJSON(w, http.StatusOK, map[string]string{"message": "Subbiercategory updated successfully"})
}

// DeleteSubbiercategory deletes a subbiercategory
func DeleteSubbiercategory(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]

	db := repository.GetSQLDB()
	result, err := db.Exec("DELETE FROM subbiercategories WHERE subbiercategoryID = $1", id)
	if err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to delete subbiercategory"})
		return
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		respondJSON(w, http.StatusNotFound, map[string]string{"error": "Subbiercategory not found"})
		return
	}

	respondJSON(w, http.StatusOK, map[string]string{"message": "Subbiercategory deleted successfully"})
}
