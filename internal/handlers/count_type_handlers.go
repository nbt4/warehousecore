package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/gorilla/mux"

	"warehousecore/internal/repository"
)

// CountType represents a measurement unit for accessories/consumables
type CountType struct {
	CountTypeID  int    `json:"count_type_id"`
	Name         string `json:"name"`
	Abbreviation string `json:"abbreviation"`
	IsDecimal    bool   `json:"is_decimal"`
}

// ListCountTypes returns all measurement units.
func ListCountTypes(w http.ResponseWriter, r *http.Request) {
	db := repository.GetSQLDB()

	rows, err := db.Query("SELECT count_type_id, name, abbreviation, is_decimal FROM count_types ORDER BY name")
	if err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to fetch measurement units"})
		return
	}
	defer rows.Close()

	var countTypes []CountType
	for rows.Next() {
		var ct CountType
		if err := rows.Scan(&ct.CountTypeID, &ct.Name, &ct.Abbreviation, &ct.IsDecimal); err == nil {
			countTypes = append(countTypes, ct)
		}
	}

	respondJSON(w, http.StatusOK, countTypes)
}

type countTypeRequest struct {
	Name         string `json:"name"`
	Abbreviation string `json:"abbreviation"`
	IsDecimal    *bool  `json:"is_decimal,omitempty"`
}

// CreateCountType adds a new measurement unit.
func CreateCountType(w http.ResponseWriter, r *http.Request) {
	var req countTypeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid request body"})
		return
	}

	req.Name = strings.TrimSpace(req.Name)
	req.Abbreviation = strings.TrimSpace(req.Abbreviation)
	if req.Name == "" || req.Abbreviation == "" {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "Name and abbreviation are required"})
		return
	}
	if len(req.Abbreviation) > 10 {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "Abbreviation must be at most 10 characters"})
		return
	}

	isDecimal := false
	if req.IsDecimal != nil {
		isDecimal = *req.IsDecimal
	}

	db := repository.GetSQLDB()
	var id int64
	err := db.QueryRow(
		"INSERT INTO count_types (name, abbreviation, is_decimal) VALUES ($1, $2, $3) RETURNING count_type_id",
		req.Name, req.Abbreviation, isDecimal,
	).Scan(&id)
	if err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to create measurement unit"})
		return
	}
	respondJSON(w, http.StatusCreated, CountType{
		CountTypeID:  int(id),
		Name:         req.Name,
		Abbreviation: req.Abbreviation,
		IsDecimal:    isDecimal,
	})
}

// UpdateCountType updates an existing measurement unit.
func UpdateCountType(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id, err := strconv.Atoi(vars["id"])
	if err != nil || id <= 0 {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid measurement unit ID"})
		return
	}

	var req countTypeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid request body"})
		return
	}

	req.Name = strings.TrimSpace(req.Name)
	req.Abbreviation = strings.TrimSpace(req.Abbreviation)
	if req.Name == "" || req.Abbreviation == "" {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "Name and abbreviation are required"})
		return
	}
	if len(req.Abbreviation) > 10 {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "Abbreviation must be at most 10 characters"})
		return
	}

	isDecimal := false
	if req.IsDecimal != nil {
		isDecimal = *req.IsDecimal
	}

	db := repository.GetSQLDB()
	result, err := db.Exec(
		"UPDATE count_types SET name = $1, abbreviation = $2, is_decimal = $3 WHERE count_type_id = $4",
		req.Name, req.Abbreviation, isDecimal, id,
	)
	if err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to update measurement unit"})
		return
	}

	affected, _ := result.RowsAffected()
	if affected == 0 {
		respondJSON(w, http.StatusNotFound, map[string]string{"error": "Measurement unit not found"})
		return
	}

	respondJSON(w, http.StatusOK, map[string]string{"message": "Measurement unit updated"})
}

// DeleteCountType removes a measurement unit.
func DeleteCountType(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id, err := strconv.Atoi(vars["id"])
	if err != nil || id <= 0 {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid measurement unit ID"})
		return
	}

	db := repository.GetSQLDB()
	result, err := db.Exec("DELETE FROM count_types WHERE count_type_id = $1", id)
	if err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to delete measurement unit"})
		return
	}

	affected, _ := result.RowsAffected()
	if affected == 0 {
		respondJSON(w, http.StatusNotFound, map[string]string{"error": "Measurement unit not found"})
		return
	}

	respondJSON(w, http.StatusOK, map[string]string{"message": "Measurement unit deleted"})
}
