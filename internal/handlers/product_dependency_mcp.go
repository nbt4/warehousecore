package handlers

import (
	"database/sql"
	"math"
	"net/http"
	"strings"

	"warehousecore/internal/models"
	"warehousecore/internal/repository"
)

func createProductDependencyMCP(w http.ResponseWriter, r *http.Request, productID int, req models.CreateProductDependencyRequest) {
	if productID <= 0 || req.DependencyProductID <= 0 || productID == req.DependencyProductID {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "Two different valid product IDs are required"})
		return
	}
	if strings.TrimSpace(req.ExpectedUpdatedAt) == "" {
		respondJSON(w, http.StatusPreconditionRequired, map[string]string{"error": "The exact expectedUpdatedAt from the preview is required"})
		return
	}
	req.RelationType = strings.ToLower(strings.TrimSpace(req.RelationType))
	if req.RelationType == "" {
		req.RelationType = "recommended"
	}
	allowed := map[string]bool{"required": true, "recommended": true, "compatible": true, "consumes": true, "alternative": true, "included": true}
	if !allowed[req.RelationType] {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid relationship type"})
		return
	}
	req.AssignmentScope = strings.ToLower(strings.TrimSpace(req.AssignmentScope))
	if req.AssignmentScope == "" {
		req.AssignmentScope = "product"
	}
	if req.AssignmentScope != "product" && req.AssignmentScope != "device" && req.AssignmentScope != "case" {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid assignment scope"})
		return
	}
	if req.DefaultQuantity <= 0 || math.IsNaN(req.DefaultQuantity) || math.IsInf(req.DefaultQuantity, 0) {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "A positive finite default quantity is required"})
		return
	}
	if req.Notes != nil {
		value := strings.TrimSpace(*req.Notes)
		if len([]rune(value)) > 500 {
			respondJSON(w, http.StatusBadRequest, map[string]string{"error": "Notes must not exceed 500 characters"})
			return
		}
		req.Notes = &value
	}
	req.IsOptional = req.RelationType != "required" && req.RelationType != "included" && req.RelationType != "consumes"

	tx, err := repository.GetSQLDB().BeginTx(r.Context(), nil)
	if err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to begin relationship update"})
		return
	}
	defer tx.Rollback()
	products, err := tx.Query(`SELECT productid,to_char(updated_at,'YYYY-MM-DD"T"HH24:MI:SS.US"Z"'),lifecycle_status
		FROM products WHERE productid IN($1,$2) ORDER BY productid FOR UPDATE`, productID, req.DependencyProductID)
	if err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to load products"})
		return
	}
	type lockedProduct struct{ version, status string }
	locked := make(map[int]lockedProduct, 2)
	for products.Next() {
		var id int
		var row lockedProduct
		if err := products.Scan(&id, &row.version, &row.status); err != nil {
			products.Close()
			respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to load products"})
			return
		}
		locked[id] = row
	}
	err = products.Err()
	products.Close()
	if err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to load products"})
		return
	}
	parent, parentFound := locked[productID]
	target, targetFound := locked[req.DependencyProductID]
	if !parentFound || !targetFound {
		respondJSON(w, http.StatusNotFound, map[string]string{"error": "Product or dependency product not found"})
		return
	}
	receiptID, replay, err := beginWarehouseProductMutation(tx, r, "product.relation.link", map[string]any{"product_id": productID, "dependency_product_id": req.DependencyProductID, "relation_type": req.RelationType, "assignment_scope": req.AssignmentScope, "default_quantity": req.DefaultQuantity, "notes": req.Notes, "expected_updated_at": req.ExpectedUpdatedAt})
	if err != nil {
		respondWarehouseMutationError(w, err)
		return
	}
	if replay != nil {
		respondJSON(w, http.StatusOK, replay)
		return
	}
	if req.ExpectedUpdatedAt != parent.version {
		respondJSON(w, http.StatusConflict, map[string]string{"error": "Product changed since the preview"})
		return
	}
	if parent.status != "active" {
		respondJSON(w, http.StatusConflict, map[string]string{"error": "Archived products cannot be linked"})
		return
	}
	if target.status != "active" {
		respondJSON(w, http.StatusConflict, map[string]string{"error": "Archived dependency products cannot be linked"})
		return
	}
	var oldID int64
	var oldType, oldScope string
	var oldQuantity float64
	var oldNotes sql.NullString
	err = tx.QueryRow(`SELECT id,relation_type,assignment_scope,default_quantity,notes FROM product_dependencies WHERE product_id=$1 AND dependency_product_id=$2 FOR UPDATE`, productID, req.DependencyProductID).Scan(&oldID, &oldType, &oldScope, &oldQuantity, &oldNotes)
	if err != nil && err != sql.ErrNoRows {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to load existing relationship"})
		return
	}
	var before any
	if err == nil {
		before = map[string]any{"relation_type": oldType, "assignment_scope": oldScope, "default_quantity": oldQuantity, "notes": oldNotes.String}
		newNotes := ""
		if req.Notes != nil {
			newNotes = *req.Notes
		}
		if oldType == req.RelationType && oldScope == req.AssignmentScope && oldQuantity == req.DefaultQuantity && oldNotes.String == newNotes {
			respondJSON(w, http.StatusConflict, map[string]string{"error": "Relationship is already unchanged"})
			return
		}
	}
	var relationID int64
	if err := tx.QueryRow(`INSERT INTO product_dependencies(product_id,dependency_product_id,is_optional,relation_type,assignment_scope,default_quantity,notes)
		VALUES($1,$2,$3,$4,$5,$6,$7) ON CONFLICT(product_id,dependency_product_id) DO UPDATE SET
		is_optional=EXCLUDED.is_optional,relation_type=EXCLUDED.relation_type,assignment_scope=EXCLUDED.assignment_scope,
		default_quantity=EXCLUDED.default_quantity,notes=EXCLUDED.notes,updated_at=clock_timestamp() RETURNING id`,
		productID, req.DependencyProductID, req.IsOptional, req.RelationType, req.AssignmentScope, req.DefaultQuantity, req.Notes).Scan(&relationID); err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to save relationship"})
		return
	}
	var updatedAt string
	if err := tx.QueryRow(`UPDATE products SET updated_at=GREATEST(clock_timestamp(),updated_at+interval '1 microsecond') WHERE productid=$1
		RETURNING to_char(updated_at,'YYYY-MM-DD"T"HH24:MI:SS.US"Z"')`, productID).Scan(&updatedAt); err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to update product version"})
		return
	}
	after := map[string]any{"origin": "MCP/AI", "dependency_product_id": req.DependencyProductID, "relation_id": relationID, "relation_type": req.RelationType, "assignment_scope": req.AssignmentScope, "default_quantity": req.DefaultQuantity, "notes": req.Notes}
	if err := recordProductAudit(tx, r, "product.relation.link", productID, before, after); err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to audit relationship"})
		return
	}
	response := map[string]any{"product_id": productID, "dependency_product_id": req.DependencyProductID, "relation_id": relationID, "updated_at": updatedAt, "relation_type": req.RelationType, "assignment_scope": req.AssignmentScope, "default_quantity": req.DefaultQuantity}
	if err := completeWarehouseProductMutation(tx, receiptID, http.StatusOK, response); err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to store relationship receipt"})
		return
	}
	if err := tx.Commit(); err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to commit relationship"})
		return
	}
	respondJSON(w, http.StatusOK, response)
}
