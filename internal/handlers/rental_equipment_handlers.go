package handlers

import (
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/mux"

	"warehousecore/internal/repository"
)

// RentalEquipment represents a product rented from an external supplier
type RentalEquipment struct {
	EquipmentID   int                `json:"equipment_id"`
	ProductName   string             `json:"product_name"`
	SupplierName  string             `json:"supplier_name"`
	RentalPrice   float64            `json:"rental_price"`
	CustomerPrice float64            `json:"customer_price"`
	Category      *string            `json:"category"`
	Description   *string            `json:"description"`
	Notes         *string            `json:"notes"`
	IsActive      bool               `json:"is_active"`
	CreatedAt     time.Time          `json:"created_at"`
	UpdatedAt     time.Time          `json:"updated_at"`
	FieldValues   []RentalFieldValue `json:"field_values"`
}

// RentalEquipmentCreateRequest represents the request to create rental equipment
type RentalEquipmentCreateRequest struct {
	ProductName   string                  `json:"product_name"`
	SupplierName  string                  `json:"supplier_name"`
	RentalPrice   float64                 `json:"rental_price"`
	CustomerPrice float64                 `json:"customer_price"`
	Category      *string                 `json:"category"`
	Description   *string                 `json:"description"`
	Notes         *string                 `json:"notes"`
	IsActive      *bool                   `json:"is_active"`
	FieldValues   []RentalFieldValueInput `json:"field_values"`
}

// GetRentalEquipment retrieves all rental equipment with optional filtering
func GetRentalEquipment(w http.ResponseWriter, r *http.Request) {
	search := r.URL.Query().Get("search")
	supplierFilter := r.URL.Query().Get("supplier")
	activeOnly := r.URL.Query().Get("active_only") == "true"

	db := repository.GetSQLDB()

	query := `
		SELECT
			id,
			name,
			supplier,
			rental_price,
			COALESCE(customer_price, 0) as customer_price,
			category,
			description,
			notes,
			is_active,
			created_at,
			updated_at
		FROM rental_equipment
		WHERE 1=1
	`

	var args []interface{}
	argIdx := 1

	if search != "" {
		for _, term := range warehouseProductSearchTerms(search) {
			placeholder := "$" + strconv.Itoa(argIdx)
			query += ` AND (CONCAT_WS(' ',name,supplier,category,description,notes,rental_price::text,customer_price::text) ILIKE ` + placeholder + `
				OR EXISTS (SELECT 1 FROM rental_equipment_field_values search_value
				 JOIN rental_equipment_field_definitions search_definition ON search_definition.id=search_value.field_definition_id
				 WHERE search_value.equipment_id=rental_equipment.id
				 AND CONCAT_WS(' ',search_definition.name,search_definition.unit,search_value.value) ILIKE ` + placeholder + `))`
			args = append(args, "%"+strings.ToLower(term)+"%")
			argIdx++
		}
	}

	if supplierFilter != "" {
		query += " AND supplier = $" + strconv.Itoa(argIdx)
		args = append(args, supplierFilter)
		argIdx++
	}

	if activeOnly {
		query += " AND is_active = TRUE"
	}

	query += " ORDER BY supplier, name"

	rows, err := db.Query(query, args...)
	if err != nil {
		log.Printf("Failed to query rental equipment: %v", err)
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to fetch rental equipment"})
		return
	}
	defer rows.Close()

	var equipment []RentalEquipment
	for rows.Next() {
		var e RentalEquipment
		err := rows.Scan(
			&e.EquipmentID,
			&e.ProductName,
			&e.SupplierName,
			&e.RentalPrice,
			&e.CustomerPrice,
			&e.Category,
			&e.Description,
			&e.Notes,
			&e.IsActive,
			&e.CreatedAt,
			&e.UpdatedAt,
		)
		if err != nil {
			log.Printf("Failed to scan rental equipment: %v", err)
			continue
		}
		equipment = append(equipment, e)
	}

	if equipment == nil {
		equipment = []RentalEquipment{}
	}

	// Load field values for all equipment items
	ids := make([]int, len(equipment))
	for i, e := range equipment {
		ids[i] = e.EquipmentID
	}
	if fieldMap, err := fetchFieldValues(db, ids); err != nil {
		log.Printf("Failed to fetch field values: %v", err)
	} else {
		for i := range equipment {
			if vals, ok := fieldMap[equipment[i].EquipmentID]; ok {
				equipment[i].FieldValues = vals
			} else {
				equipment[i].FieldValues = []RentalFieldValue{}
			}
		}
	}

	respondJSON(w, http.StatusOK, equipment)
}

// GetRentalEquipmentByID retrieves a single rental equipment item
func GetRentalEquipmentByID(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	idStr := vars["id"]
	id, err := strconv.Atoi(idStr)
	if err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid equipment ID"})
		return
	}

	db := repository.GetSQLDB()

	var e RentalEquipment
	err = db.QueryRow(`
		SELECT
			id,
			name,
			supplier,
			rental_price,
			COALESCE(customer_price, 0) as customer_price,
			category,
			description,
			notes,
			is_active,
			created_at,
			updated_at
		FROM rental_equipment
		WHERE id = $1
	`, id).Scan(
		&e.EquipmentID,
		&e.ProductName,
		&e.SupplierName,
		&e.RentalPrice,
		&e.CustomerPrice,
		&e.Category,
		&e.Description,
		&e.Notes,
		&e.IsActive,
		&e.CreatedAt,
		&e.UpdatedAt,
	)

	if err == sql.ErrNoRows {
		respondJSON(w, http.StatusNotFound, map[string]string{"error": "Rental equipment not found"})
		return
	}
	if err != nil {
		log.Printf("Failed to get rental equipment: %v", err)
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to fetch rental equipment"})
		return
	}

	if fieldMap, err := fetchFieldValues(db, []int{e.EquipmentID}); err != nil {
		log.Printf("Failed to fetch field values: %v", err)
		e.FieldValues = []RentalFieldValue{}
	} else if vals, ok := fieldMap[e.EquipmentID]; ok {
		e.FieldValues = vals
	} else {
		e.FieldValues = []RentalFieldValue{}
	}

	respondJSON(w, http.StatusOK, e)
}

// CreateRentalEquipment creates a new rental equipment item
func CreateRentalEquipment(w http.ResponseWriter, r *http.Request) {
	var req RentalEquipmentCreateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid request body"})
		return
	}

	if req.ProductName == "" {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "Product name is required"})
		return
	}

	db := repository.GetSQLDB()

	isActive := true
	if req.IsActive != nil {
		isActive = *req.IsActive
	}

	var id int64
	err := db.QueryRow(`
		INSERT INTO rental_equipment (
			name,
			supplier,
			rental_price,
			customer_price,
			category,
			description,
			notes,
			is_active,
			created_at,
			updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, NOW(), NOW())
		RETURNING id
	`,
		req.ProductName,
		req.SupplierName,
		req.RentalPrice,
		req.CustomerPrice,
		req.Category,
		req.Description,
		req.Notes,
		isActive,
	).Scan(&id)

	if err != nil {
		log.Printf("Failed to create rental equipment: %v", err)
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to create rental equipment"})
		return
	}

	if len(req.FieldValues) > 0 {
		if err := saveFieldValues(db, int(id), req.FieldValues); err != nil {
			log.Printf("Failed to save field values for new equipment %d: %v", id, err)
		}
	}

	// Fetch the created equipment
	var e RentalEquipment
	err = db.QueryRow(`
		SELECT
			id,
			name,
			supplier,
			rental_price,
			COALESCE(customer_price, 0) as customer_price,
			category,
			description,
			notes,
			is_active,
			created_at,
			updated_at
		FROM rental_equipment
		WHERE id = $1
	`, id).Scan(
		&e.EquipmentID,
		&e.ProductName,
		&e.SupplierName,
		&e.RentalPrice,
		&e.CustomerPrice,
		&e.Category,
		&e.Description,
		&e.Notes,
		&e.IsActive,
		&e.CreatedAt,
		&e.UpdatedAt,
	)

	if err != nil {
		log.Printf("Failed to fetch created rental equipment: %v", err)
		respondJSON(w, http.StatusCreated, map[string]interface{}{"equipment_id": id})
		return
	}

	respondJSON(w, http.StatusCreated, e)
}

// UpdateRentalEquipment updates an existing rental equipment item
func UpdateRentalEquipment(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	idStr := vars["id"]
	id, err := strconv.Atoi(idStr)
	if err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid equipment ID"})
		return
	}

	var req RentalEquipmentCreateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid request body"})
		return
	}

	if req.ProductName == "" {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "Product name is required"})
		return
	}

	db := repository.GetSQLDB()

	isActive := true
	if req.IsActive != nil {
		isActive = *req.IsActive
	}

	_, err = db.Exec(`
		UPDATE rental_equipment SET
			name = $1,
			supplier = $2,
			rental_price = $3,
			customer_price = $4,
			category = $5,
			description = $6,
			notes = $7,
			is_active = $8,
			updated_at = NOW()
		WHERE id = $9
	`,
		req.ProductName,
		req.SupplierName,
		req.RentalPrice,
		req.CustomerPrice,
		req.Category,
		req.Description,
		req.Notes,
		isActive,
		id,
	)

	if err != nil {
		log.Printf("Failed to update rental equipment: %v", err)
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to update rental equipment"})
		return
	}

	if err := saveFieldValues(db, id, req.FieldValues); err != nil {
		log.Printf("Failed to save field values for equipment %d: %v", id, err)
	}

	// Fetch the updated equipment
	var e RentalEquipment
	err = db.QueryRow(`
		SELECT
			id,
			name,
			supplier,
			rental_price,
			COALESCE(customer_price, 0) as customer_price,
			category,
			description,
			notes,
			is_active,
			created_at,
			updated_at
		FROM rental_equipment
		WHERE id = $1
	`, id).Scan(
		&e.EquipmentID,
		&e.ProductName,
		&e.SupplierName,
		&e.RentalPrice,
		&e.CustomerPrice,
		&e.Category,
		&e.Description,
		&e.Notes,
		&e.IsActive,
		&e.CreatedAt,
		&e.UpdatedAt,
	)

	if err != nil {
		log.Printf("Failed to fetch updated rental equipment: %v", err)
		respondJSON(w, http.StatusOK, map[string]string{"message": "Updated successfully"})
		return
	}

	respondJSON(w, http.StatusOK, e)
}

// DeleteRentalEquipment deletes a rental equipment item
func DeleteRentalEquipment(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	idStr := vars["id"]
	id, err := strconv.Atoi(idStr)
	if err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid equipment ID"})
		return
	}

	db := repository.GetSQLDB()

	_, err = db.Exec("DELETE FROM rental_equipment WHERE id = $1", id)
	if err != nil {
		log.Printf("Failed to delete rental equipment: %v", err)
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to delete rental equipment"})
		return
	}

	respondJSON(w, http.StatusOK, map[string]string{"message": "Deleted successfully"})
}

// GetRentalEquipmentSuppliers returns a list of unique suppliers
func GetRentalEquipmentSuppliers(w http.ResponseWriter, r *http.Request) {
	db := repository.GetSQLDB()

	rows, err := db.Query(`
		SELECT DISTINCT supplier
		FROM rental_equipment
		WHERE supplier IS NOT NULL AND supplier != ''
		ORDER BY supplier
	`)
	if err != nil {
		log.Printf("Failed to query suppliers: %v", err)
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to fetch suppliers"})
		return
	}
	defer rows.Close()

	var suppliers []string
	for rows.Next() {
		var supplier string
		if err := rows.Scan(&supplier); err != nil {
			continue
		}
		suppliers = append(suppliers, supplier)
	}

	if suppliers == nil {
		suppliers = []string{}
	}

	respondJSON(w, http.StatusOK, suppliers)
}

// SearchSupplierContacts searches the customers table for contacts with is_supplier=true
func SearchSupplierContacts(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	if q == "" {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "Query parameter required"})
		return
	}

	db := repository.GetSQLDB()
	pattern := "%" + q + "%"

	rows, err := db.Query(`
		SELECT customerid, COALESCE(companyname, ''), COALESCE(firstname, ''), COALESCE(lastname, '')
		FROM customers
		WHERE is_supplier = true
		  AND (companyname ILIKE $1 OR firstname ILIKE $1 OR lastname ILIKE $1)
		ORDER BY COALESCE(companyname, lastname, firstname)
		LIMIT 10
	`, pattern)
	if err != nil {
		log.Printf("Failed to search supplier contacts: %v", err)
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Search failed"})
		return
	}
	defer rows.Close()

	type result struct {
		ID          int    `json:"id"`
		DisplayName string `json:"display_name"`
	}

	var results []result
	for rows.Next() {
		var id int
		var company, first, last string
		if err := rows.Scan(&id, &company, &first, &last); err != nil {
			continue
		}
		name := company
		if name == "" {
			name = (first + " " + last)
		}
		results = append(results, result{ID: id, DisplayName: name})
	}

	if results == nil {
		results = []result{}
	}

	respondJSON(w, http.StatusOK, results)
}
