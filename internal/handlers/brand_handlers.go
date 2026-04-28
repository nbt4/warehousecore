package handlers

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/gorilla/mux"

	"warehousecore/internal/repository"
)

// Brand represents a product brand with optional manufacturer association.
type Brand struct {
	BrandID          int     `json:"brand_id"`
	Name             string  `json:"name"`
	ManufacturerID   *int    `json:"manufacturer_id,omitempty"`
	ManufacturerName *string `json:"manufacturer_name,omitempty"`
}

// Manufacturer represents a product manufacturer.
type Manufacturer struct {
	ManufacturerID int     `json:"manufacturer_id"`
	Name           string  `json:"name"`
	Website        *string `json:"website,omitempty"`
}

// GetBrands returns all brands with manufacturer metadata.
func GetBrands(w http.ResponseWriter, r *http.Request) {
	db := repository.GetSQLDB()
	rows, err := db.Query(`
		SELECT b.brandID, b.name, b.manufacturerID, m.name
		FROM brands b
		LEFT JOIN manufacturer m ON b.manufacturerID = m.manufacturerID
		ORDER BY b.name
	`)
	if err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to fetch brands"})
		return
	}
	defer rows.Close()

	var brands []Brand
	for rows.Next() {
		var (
			brandID        int
			name           string
			manufacturerID sql.NullInt64
			manufacturer   sql.NullString
		)

		if err := rows.Scan(&brandID, &name, &manufacturerID, &manufacturer); err != nil {
			continue
		}

		brand := Brand{
			BrandID: brandID,
			Name:    name,
		}
		if manufacturerID.Valid {
			id := int(manufacturerID.Int64)
			brand.ManufacturerID = &id
		}
		if manufacturer.Valid {
			value := manufacturer.String
			brand.ManufacturerName = &value
		}

		brands = append(brands, brand)
	}

	respondJSON(w, http.StatusOK, brands)
}

// CreateBrand creates a new brand.
func CreateBrand(w http.ResponseWriter, r *http.Request) {
	var payload struct {
		Name           string `json:"name"`
		ManufacturerID *int   `json:"manufacturer_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid request body"})
		return
	}

	if payload.Name == "" {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "Name is required"})
		return
	}

	db := repository.GetSQLDB()
	var id int64
	err := db.QueryRow(
		"INSERT INTO brands (name, manufacturerID) VALUES ($1, $2) RETURNING brandID",
		payload.Name,
		payload.ManufacturerID,
	).Scan(&id)
	if err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to create brand"})
		return
	}
	brand := Brand{
		BrandID: int(id),
		Name:    payload.Name,
	}

	if payload.ManufacturerID != nil {
		brand.ManufacturerID = payload.ManufacturerID

		var manufacturerName sql.NullString
		if err := db.QueryRow(
			"SELECT name FROM manufacturer WHERE manufacturerID = $1",
			*payload.ManufacturerID,
		).Scan(&manufacturerName); err == nil && manufacturerName.Valid {
			name := manufacturerName.String
			brand.ManufacturerName = &name
		}
	}

	respondJSON(w, http.StatusCreated, brand)
}

// UpdateBrand updates an existing brand.
func UpdateBrand(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id, err := strconv.Atoi(vars["id"])
	if err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid brand ID"})
		return
	}

	var payload struct {
		Name           string `json:"name"`
		ManufacturerID *int   `json:"manufacturer_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid request body"})
		return
	}

	if payload.Name == "" {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "Name is required"})
		return
	}

	db := repository.GetSQLDB()
	result, err := db.Exec(
		"UPDATE brands SET name = $1, manufacturerID = $2 WHERE brandID = $3",
		payload.Name,
		payload.ManufacturerID,
		id,
	)
	if err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to update brand"})
		return
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		respondJSON(w, http.StatusNotFound, map[string]string{"error": "Brand not found"})
		return
	}

	response := Brand{
		BrandID: id,
		Name:    payload.Name,
	}
	if payload.ManufacturerID != nil {
		response.ManufacturerID = payload.ManufacturerID

		var manufacturerName sql.NullString
		if err := db.QueryRow(
			"SELECT name FROM manufacturer WHERE manufacturerID = $1",
			*payload.ManufacturerID,
		).Scan(&manufacturerName); err == nil && manufacturerName.Valid {
			name := manufacturerName.String
			response.ManufacturerName = &name
		}
	}

	respondJSON(w, http.StatusOK, response)
}

// DeleteBrand removes a brand.
func DeleteBrand(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id, err := strconv.Atoi(vars["id"])
	if err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid brand ID"})
		return
	}

	db := repository.GetSQLDB()
	result, err := db.Exec("DELETE FROM brands WHERE brandID = $1", id)
	if err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to delete brand"})
		return
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		respondJSON(w, http.StatusNotFound, map[string]string{"error": "Brand not found"})
		return
	}

	respondJSON(w, http.StatusOK, map[string]string{"message": "Brand deleted successfully"})
}

// GetManufacturers returns all manufacturers.
func GetManufacturers(w http.ResponseWriter, r *http.Request) {
	db := repository.GetSQLDB()
	rows, err := db.Query("SELECT manufacturerID, name, website FROM manufacturer ORDER BY name")
	if err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to fetch manufacturers"})
		return
	}
	defer rows.Close()

	var manufacturers []Manufacturer
	for rows.Next() {
		var (
			manufacturerID int
			name           string
			website        sql.NullString
		)

		if err := rows.Scan(&manufacturerID, &name, &website); err != nil {
			continue
		}

		manufacturer := Manufacturer{
			ManufacturerID: manufacturerID,
			Name:           name,
		}
		if website.Valid {
			value := website.String
			manufacturer.Website = &value
		}

		manufacturers = append(manufacturers, manufacturer)
	}

	respondJSON(w, http.StatusOK, manufacturers)
}

// CreateManufacturer creates a new manufacturer.
func CreateManufacturer(w http.ResponseWriter, r *http.Request) {
	var payload struct {
		Name    string  `json:"name"`
		Website *string `json:"website"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid request body"})
		return
	}

	if payload.Name == "" {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "Name is required"})
		return
	}

	db := repository.GetSQLDB()
	var id int64
	err := db.QueryRow(
		"INSERT INTO manufacturer (name, website) VALUES ($1, $2) RETURNING manufacturerID",
		payload.Name,
		payload.Website,
	).Scan(&id)
	if err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to create manufacturer"})
		return
	}
	manufacturer := Manufacturer{
		ManufacturerID: int(id),
		Name:           payload.Name,
		Website:        payload.Website,
	}

	respondJSON(w, http.StatusCreated, manufacturer)
}

// UpdateManufacturer updates an existing manufacturer.
func UpdateManufacturer(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id, err := strconv.Atoi(vars["id"])
	if err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid manufacturer ID"})
		return
	}

	var payload struct {
		Name    string  `json:"name"`
		Website *string `json:"website"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid request body"})
		return
	}

	if payload.Name == "" {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "Name is required"})
		return
	}

	db := repository.GetSQLDB()
	result, err := db.Exec(
		"UPDATE manufacturer SET name = $1, website = $2 WHERE manufacturerID = $3",
		payload.Name,
		payload.Website,
		id,
	)
	if err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to update manufacturer"})
		return
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		respondJSON(w, http.StatusNotFound, map[string]string{"error": "Manufacturer not found"})
		return
	}

	respondJSON(w, http.StatusOK, Manufacturer{
		ManufacturerID: id,
		Name:           payload.Name,
		Website:        payload.Website,
	})
}

// DeleteManufacturer removes a manufacturer.
func DeleteManufacturer(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id, err := strconv.Atoi(vars["id"])
	if err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid manufacturer ID"})
		return
	}

	db := repository.GetSQLDB()
	result, err := db.Exec("DELETE FROM manufacturer WHERE manufacturerID = $1", id)
	if err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to delete manufacturer"})
		return
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		respondJSON(w, http.StatusNotFound, map[string]string{"error": "Manufacturer not found"})
		return
	}

	respondJSON(w, http.StatusOK, map[string]string{"message": "Manufacturer deleted successfully"})
}
