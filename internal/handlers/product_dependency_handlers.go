package handlers

import (
	"encoding/json"
	"log"
	"net/http"
	"strconv"

	"github.com/gorilla/mux"

	"warehousecore/internal/models"
	"warehousecore/internal/repository"
)

// GetProductDependencies returns all dependencies for a product
func GetProductDependencies(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	productID, err := strconv.Atoi(vars["id"])
	if err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid product ID"})
		return
	}

	db := repository.GetDB()

	var dependencies []models.ProductDependencyWithDetails
	err = db.Raw(`
		SELECT
			pd.id,
			pd.product_id,
			pd.dependency_product_id,
			p.name as dependency_name,
			COALESCE(p.is_accessory, false) as is_accessory,
			COALESCE(p.is_consumable, false) as is_consumable,
			p.generic_barcode,
			ct.abbreviation as count_type_abbr,
			p.stock_quantity,
			pd.is_optional,
			pd.default_quantity,
			pd.notes,
			TO_CHAR(pd.created_at, 'YYYY-MM-DD HH24:MI:SS') as created_at
		FROM product_dependencies pd
		JOIN products p ON pd.dependency_product_id = p.productid
		LEFT JOIN count_types ct ON p.count_type_id = ct.count_type_id
		WHERE pd.product_id = ?
		ORDER BY pd.created_at DESC
	`, productID).Scan(&dependencies).Error

	if err != nil {
		log.Printf("Failed to fetch product dependencies: %v", err)
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to fetch dependencies"})
		return
	}

	if dependencies == nil {
		dependencies = []models.ProductDependencyWithDetails{}
	}

	respondJSON(w, http.StatusOK, dependencies)
}

// CreateProductDependency adds a new dependency to a product
func CreateProductDependency(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	productID, err := strconv.Atoi(vars["id"])
	if err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid product ID"})
		return
	}

	var req models.CreateProductDependencyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid request body"})
		return
	}

	// Validate that dependency product exists and is accessory/consumable
	db := repository.GetDB()
	var depProduct struct {
		ProductID    int  `gorm:"column:productid"`
		IsAccessory  bool `gorm:"column:is_accessory"`
		IsConsumable bool `gorm:"column:is_consumable"`
	}

	err = db.Table("products").
		Select("productid, COALESCE(is_accessory, false) as is_accessory, COALESCE(is_consumable, false) as is_consumable").
		Where("productid = ?", req.DependencyProductID).
		First(&depProduct).Error

	if err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "Dependency product not found"})
		return
	}

	if !depProduct.IsAccessory && !depProduct.IsConsumable {
		respondJSON(w, http.StatusBadRequest, map[string]string{
			"error": "Dependency must be an accessory or consumable",
		})
		return
	}

	// Prevent self-dependency
	if productID == req.DependencyProductID {
		respondJSON(w, http.StatusBadRequest, map[string]string{
			"error": "A product cannot depend on itself",
		})
		return
	}

	// Set default quantity if not provided
	if req.DefaultQuantity <= 0 {
		req.DefaultQuantity = 1.0
	}

	// Create dependency
	result := db.Exec(`
		INSERT INTO product_dependencies (product_id, dependency_product_id, is_optional, default_quantity, notes)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (product_id, dependency_product_id) DO UPDATE SET
			is_optional = EXCLUDED.is_optional,
			default_quantity = EXCLUDED.default_quantity,
			notes = EXCLUDED.notes,
			updated_at = NOW()
	`, productID, req.DependencyProductID, req.IsOptional, req.DefaultQuantity, req.Notes)

	if result.Error != nil {
		log.Printf("Failed to create product dependency: %v", result.Error)
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to create dependency"})
		return
	}

	// Return the created/updated dependency
	var dependency models.ProductDependencyWithDetails
	err = db.Raw(`
		SELECT
			pd.id,
			pd.product_id,
			pd.dependency_product_id,
			p.name as dependency_name,
			COALESCE(p.is_accessory, false) as is_accessory,
			COALESCE(p.is_consumable, false) as is_consumable,
			p.generic_barcode,
			ct.abbreviation as count_type_abbr,
			p.stock_quantity,
			pd.is_optional,
			pd.default_quantity,
			pd.notes,
			TO_CHAR(pd.created_at, 'YYYY-MM-DD HH24:MI:SS') as created_at
		FROM product_dependencies pd
		JOIN products p ON pd.dependency_product_id = p.productid
		LEFT JOIN count_types ct ON p.count_type_id = ct.count_type_id
		WHERE pd.product_id = ? AND pd.dependency_product_id = ?
	`, productID, req.DependencyProductID).Scan(&dependency).Error

	if err != nil {
		log.Printf("Failed to fetch created dependency: %v", err)
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Dependency created but failed to fetch details"})
		return
	}

	respondJSON(w, http.StatusCreated, dependency)
}

// DeleteProductDependency removes a dependency from a product
func DeleteProductDependency(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	productID, err := strconv.Atoi(vars["id"])
	if err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid product ID"})
		return
	}

	depID, err := strconv.Atoi(vars["dep_id"])
	if err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid dependency ID"})
		return
	}

	db := repository.GetDB()

	result := db.Exec("DELETE FROM product_dependencies WHERE id = ? AND product_id = ?", depID, productID)
	if result.Error != nil {
		log.Printf("Failed to delete product dependency: %v", result.Error)
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to delete dependency"})
		return
	}

	if result.RowsAffected == 0 {
		respondJSON(w, http.StatusNotFound, map[string]string{"error": "Dependency not found"})
		return
	}

	respondJSON(w, http.StatusOK, map[string]string{"message": "Dependency deleted successfully"})
}
