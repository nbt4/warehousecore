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
			pd.relation_type,
			pd.assignment_scope,
			pd.default_quantity,
			pd.notes,
			TO_CHAR(pd.created_at, 'YYYY-MM-DD HH24:MI:SS') as created_at,
 to_char(pd.updated_at,'YYYY-MM-DD"T"HH24:MI:SS.US"Z"') as updated_at,pd.lifecycle_status
		FROM product_dependencies pd
		JOIN products p ON pd.dependency_product_id = p.productid
		LEFT JOIN count_types ct ON p.count_type_id = ct.count_type_id
		WHERE pd.product_id = ? AND (?='all' OR pd.lifecycle_status=?) AND EXISTS(SELECT 1 FROM products source WHERE source.productid=pd.product_id AND source.lifecycle_status='active')
		ORDER BY pd.created_at DESC
	`, productID, dependencyLifecycle(r), dependencyLifecycle(r)).Scan(&dependencies).Error

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
	if isWarehouseMCPMutation(r) {
		createProductDependencyMCP(w, r, productID, req)
		return
	}

	saveProductDependency(w, r, productID, 0, req)

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

	setProductDependencyLifecycle(w, r, productID, depID, "archived")
}
