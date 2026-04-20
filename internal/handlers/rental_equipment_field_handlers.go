package handlers

import (
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/gorilla/mux"

	"warehousecore/internal/repository"
)

type RentalFieldDefinition struct {
	ID              int       `json:"id"`
	Name            string    `json:"name"`
	FieldType       string    `json:"field_type"`
	Unit            *string   `json:"unit"`
	DropdownOptions *string   `json:"dropdown_options"`
	IsActive        bool      `json:"is_active"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

type RentalFieldDefinitionRequest struct {
	Name            string  `json:"name"`
	FieldType       string  `json:"field_type"`
	Unit            *string `json:"unit"`
	DropdownOptions *string `json:"dropdown_options"`
	IsActive        bool    `json:"is_active"`
}

type RentalFieldValue struct {
	ID                int                    `json:"id"`
	EquipmentID       int                    `json:"equipment_id"`
	FieldDefinitionID int                    `json:"field_definition_id"`
	Value             string                 `json:"value"`
	SortOrder         int                    `json:"sort_order"`
	Definition        *RentalFieldDefinition `json:"definition,omitempty"`
}

type RentalFieldValueInput struct {
	FieldDefinitionID int    `json:"field_definition_id"`
	Value             string `json:"value"`
	SortOrder         int    `json:"sort_order"`
}

func GetRentalFieldDefinitions(w http.ResponseWriter, r *http.Request) {
	db := repository.GetSQLDB()
	activeOnly := r.URL.Query().Get("active_only") == "true"

	query := `SELECT id, name, field_type, unit, dropdown_options, is_active, created_at, updated_at
	          FROM rental_equipment_field_definitions`
	if activeOnly {
		query += ` WHERE is_active = true`
	}
	query += ` ORDER BY name`

	rows, err := db.Query(query)
	if err != nil {
		log.Printf("Failed to query field definitions: %v", err)
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to fetch field definitions"})
		return
	}
	defer rows.Close()

	var defs []RentalFieldDefinition
	for rows.Next() {
		var d RentalFieldDefinition
		if err := rows.Scan(&d.ID, &d.Name, &d.FieldType, &d.Unit, &d.DropdownOptions, &d.IsActive, &d.CreatedAt, &d.UpdatedAt); err != nil {
			log.Printf("Failed to scan field definition: %v", err)
			continue
		}
		defs = append(defs, d)
	}
	if defs == nil {
		defs = []RentalFieldDefinition{}
	}
	respondJSON(w, http.StatusOK, defs)
}

func CreateRentalFieldDefinition(w http.ResponseWriter, r *http.Request) {
	db := repository.GetSQLDB()
	var req RentalFieldDefinitionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid request body"})
		return
	}
	if req.Name == "" {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "Name is required"})
		return
	}
	if req.FieldType != "text" && req.FieldType != "number" && req.FieldType != "dropdown" {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "field_type must be text, number, or dropdown"})
		return
	}

	var id int
	err := db.QueryRow(`
		INSERT INTO rental_equipment_field_definitions (name, field_type, unit, dropdown_options, is_active)
		VALUES ($1, $2, $3, $4, $5) RETURNING id`,
		req.Name, req.FieldType, req.Unit, req.DropdownOptions, req.IsActive,
	).Scan(&id)
	if err != nil {
		log.Printf("Failed to create field definition: %v", err)
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to create field definition"})
		return
	}

	var def RentalFieldDefinition
	db.QueryRow(`SELECT id, name, field_type, unit, dropdown_options, is_active, created_at, updated_at
	             FROM rental_equipment_field_definitions WHERE id = $1`, id).
		Scan(&def.ID, &def.Name, &def.FieldType, &def.Unit, &def.DropdownOptions, &def.IsActive, &def.CreatedAt, &def.UpdatedAt)
	respondJSON(w, http.StatusCreated, def)
}

func UpdateRentalFieldDefinition(w http.ResponseWriter, r *http.Request) {
	db := repository.GetSQLDB()
	vars := mux.Vars(r)
	id, err := strconv.Atoi(vars["id"])
	if err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid ID"})
		return
	}

	var req RentalFieldDefinitionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid request body"})
		return
	}
	if req.Name == "" {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "Name is required"})
		return
	}

	result, err := db.Exec(`
		UPDATE rental_equipment_field_definitions
		SET name=$1, field_type=$2, unit=$3, dropdown_options=$4, is_active=$5, updated_at=NOW()
		WHERE id=$6`,
		req.Name, req.FieldType, req.Unit, req.DropdownOptions, req.IsActive, id,
	)
	if err != nil {
		log.Printf("Failed to update field definition: %v", err)
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to update field definition"})
		return
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		respondJSON(w, http.StatusNotFound, map[string]string{"error": "Field definition not found"})
		return
	}

	var def RentalFieldDefinition
	db.QueryRow(`SELECT id, name, field_type, unit, dropdown_options, is_active, created_at, updated_at
	             FROM rental_equipment_field_definitions WHERE id = $1`, id).
		Scan(&def.ID, &def.Name, &def.FieldType, &def.Unit, &def.DropdownOptions, &def.IsActive, &def.CreatedAt, &def.UpdatedAt)
	respondJSON(w, http.StatusOK, def)
}

func DeleteRentalFieldDefinition(w http.ResponseWriter, r *http.Request) {
	db := repository.GetSQLDB()
	vars := mux.Vars(r)
	id, err := strconv.Atoi(vars["id"])
	if err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid ID"})
		return
	}

	var count int
	db.QueryRow(`SELECT COUNT(*) FROM rental_equipment_field_values WHERE field_definition_id = $1`, id).Scan(&count)
	if count > 0 {
		_, err := db.Exec(`UPDATE rental_equipment_field_definitions SET is_active=false, updated_at=NOW() WHERE id=$1`, id)
		if err != nil {
			respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to deactivate field definition"})
			return
		}
		respondJSON(w, http.StatusOK, map[string]string{"message": "Field definition deactivated (in use by products)", "action": "deactivated"})
		return
	}

	result, err := db.Exec(`DELETE FROM rental_equipment_field_definitions WHERE id=$1`, id)
	if err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to delete field definition"})
		return
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		respondJSON(w, http.StatusNotFound, map[string]string{"error": "Field definition not found"})
		return
	}
	respondJSON(w, http.StatusOK, map[string]string{"message": "Field definition deleted", "action": "deleted"})
}

// fetchFieldValues returns all field values for the given equipment IDs with definitions attached.
func fetchFieldValues(db *sql.DB, equipmentIDs []int) (map[int][]RentalFieldValue, error) {
	if len(equipmentIDs) == 0 {
		return map[int][]RentalFieldValue{}, nil
	}

	placeholders := ""
	args := make([]interface{}, len(equipmentIDs))
	for i, id := range equipmentIDs {
		if i > 0 {
			placeholders += ","
		}
		placeholders += "$" + strconv.Itoa(i+1)
		args[i] = id
	}

	rows, err := db.Query(`
		SELECT v.id, v.equipment_id, v.field_definition_id, v.value, v.sort_order,
		       d.id, d.name, d.field_type, d.unit, d.dropdown_options, d.is_active, d.created_at, d.updated_at
		FROM rental_equipment_field_values v
		JOIN rental_equipment_field_definitions d ON d.id = v.field_definition_id
		WHERE v.equipment_id IN (`+placeholders+`)
		ORDER BY v.equipment_id, v.sort_order`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make(map[int][]RentalFieldValue)
	for rows.Next() {
		var v RentalFieldValue
		var d RentalFieldDefinition
		if err := rows.Scan(
			&v.ID, &v.EquipmentID, &v.FieldDefinitionID, &v.Value, &v.SortOrder,
			&d.ID, &d.Name, &d.FieldType, &d.Unit, &d.DropdownOptions, &d.IsActive, &d.CreatedAt, &d.UpdatedAt,
		); err != nil {
			continue
		}
		v.Definition = &d
		result[v.EquipmentID] = append(result[v.EquipmentID], v)
	}
	return result, nil
}

// saveFieldValues replaces all field values for the given equipment.
func saveFieldValues(db *sql.DB, equipmentID int, inputs []RentalFieldValueInput) error {
	_, err := db.Exec(`DELETE FROM rental_equipment_field_values WHERE equipment_id = $1`, equipmentID)
	if err != nil {
		return err
	}
	for _, inp := range inputs {
		_, err := db.Exec(`
			INSERT INTO rental_equipment_field_values (equipment_id, field_definition_id, value, sort_order)
			VALUES ($1, $2, $3, $4)`,
			equipmentID, inp.FieldDefinitionID, inp.Value, inp.SortOrder,
		)
		if err != nil {
			return err
		}
	}
	return nil
}
