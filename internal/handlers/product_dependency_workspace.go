package handlers

import (
	"database/sql"
	"encoding/json"
	"math"
	"net/http"
	"strconv"
	"strings"

	"github.com/gorilla/mux"
	"warehousecore/internal/models"
	"warehousecore/internal/repository"
)

func dependencyLifecycle(r *http.Request) string {
	value := r.URL.Query().Get("lifecycle_status")
	if value == "all" || value == "archived" {
		return value
	}
	return "active"
}
func UpdateProductDependency(w http.ResponseWriter, r *http.Request) {
	productID, err := strconv.Atoi(mux.Vars(r)["id"])
	depID, err2 := strconv.Atoi(mux.Vars(r)["dep_id"])
	if err != nil || err2 != nil {
		respondJSON(w, 400, map[string]string{"error": "Invalid relationship ID"})
		return
	}
	var input models.CreateProductDependencyRequest
	if json.NewDecoder(r.Body).Decode(&input) != nil {
		respondJSON(w, 400, map[string]string{"error": "Invalid relationship"})
		return
	}
	saveProductDependency(w, r, productID, depID, input)
}
func RestoreProductDependency(w http.ResponseWriter, r *http.Request) {
	productID, err := strconv.Atoi(mux.Vars(r)["id"])
	depID, err2 := strconv.Atoi(mux.Vars(r)["dep_id"])
	if err != nil || err2 != nil {
		respondJSON(w, 400, map[string]string{"error": "Invalid relationship ID"})
		return
	}
	setProductDependencyLifecycle(w, r, productID, depID, "active")
}
func saveProductDependency(w http.ResponseWriter, r *http.Request, productID, relationID int, input models.CreateProductDependencyRequest) {
	if input.RelationType == "" {
		input.RelationType = "required"
		if input.IsOptional {
			input.RelationType = "recommended"
		}
	}
	if input.AssignmentScope == "" {
		input.AssignmentScope = "product"
	}
	if input.DefaultQuantity == 0 {
		input.DefaultQuantity = 1
	}
	relations := map[string]bool{"required": true, "recommended": true, "compatible": true, "consumes": true, "alternative": true, "included": true}
	if !relations[input.RelationType] || (input.AssignmentScope != "product" && input.AssignmentScope != "device" && input.AssignmentScope != "case") || math.IsNaN(input.DefaultQuantity) || math.IsInf(input.DefaultQuantity, 0) || input.DefaultQuantity <= 0 || math.Abs(input.DefaultQuantity*100-math.Round(input.DefaultQuantity*100)) > 0.00001 || input.DefaultQuantity > 99999999.99 {
		respondJSON(w, 400, map[string]string{"error": "Beziehungsart, Bezug und Menge sind ungültig"})
		return
	}
	if input.Notes != nil && len(*input.Notes) > 4000 {
		respondJSON(w, 400, map[string]string{"error": "Hinweis ist zu lang"})
		return
	}
	tx, err := repository.GetSQLDB().BeginTx(r.Context(), nil)
	if err != nil {
		respondJSON(w, 500, map[string]string{"error": "Beziehung konnte nicht gespeichert werden"})
		return
	}
	defer tx.Rollback()
	var existingStatus, version string
	existingID := 0
	if relationID > 0 {
		err = tx.QueryRow(`SELECT id,dependency_product_id,lifecycle_status,to_char(updated_at,'YYYY-MM-DD"T"HH24:MI:SS.US"Z"') FROM product_dependencies WHERE product_id=$1 AND id=$2 FOR UPDATE`, productID, relationID).Scan(&existingID, &input.DependencyProductID, &existingStatus, &version)
	} else {
		err = tx.QueryRow(`SELECT id,lifecycle_status,to_char(updated_at,'YYYY-MM-DD"T"HH24:MI:SS.US"Z"') FROM product_dependencies WHERE product_id=$1 AND dependency_product_id=$2 FOR UPDATE`, productID, input.DependencyProductID).Scan(&existingID, &existingStatus, &version)
	}
	if err != nil && err != sql.ErrNoRows {
		respondJSON(w, 500, map[string]string{"error": "Beziehung konnte nicht geladen werden"})
		return
	}
	if relationID > 0 && err == sql.ErrNoRows {
		respondJSON(w, 404, map[string]string{"error": "Beziehung nicht gefunden"})
		return
	}
	if input.ExpectedUpdatedAt != "" && input.ExpectedUpdatedAt != version {
		respondJSON(w, 409, map[string]string{"error": "Beziehung wurde inzwischen verändert. Bitte neu laden."})
		return
	}
	if existingStatus == "archived" {
		respondJSON(w, 409, map[string]string{"error": "Archivierte Beziehung zuerst wiederherstellen"})
		return
	}
	if input.DependencyProductID <= 0 || input.DependencyProductID == productID {
		respondJSON(w, 400, map[string]string{"error": "Ein anderes aktives Produkt auswählen"})
		return
	}
	var activeCount int
	if err = tx.QueryRow(`SELECT count(*) FROM products WHERE productid IN ($1,$2) AND lifecycle_status='active'`, productID, input.DependencyProductID).Scan(&activeCount); err != nil || activeCount != 2 {
		respondJSON(w, 409, map[string]string{"error": "Beide Produkte müssen aktiv sein"})
		return
	}
	optional := input.RelationType != "required" && input.RelationType != "included" && input.RelationType != "consumes"
	if existingID == 0 {
		err = tx.QueryRow(`INSERT INTO product_dependencies(product_id,dependency_product_id,relation_type,assignment_scope,default_quantity,is_optional,notes) VALUES($1,$2,$3,$4,$5,$6,$7) RETURNING id`, productID, input.DependencyProductID, input.RelationType, input.AssignmentScope, input.DefaultQuantity, optional, input.Notes).Scan(&existingID)
	} else {
		_, err = tx.Exec(`UPDATE product_dependencies SET relation_type=$1,assignment_scope=$2,default_quantity=$3,is_optional=$4,notes=$5 WHERE id=$6`, input.RelationType, input.AssignmentScope, input.DefaultQuantity, optional, input.Notes, existingID)
	}
	if err == nil {
		err = recordProductAudit(tx, r, "product.relation.update", productID, nil, map[string]any{"relation_id": existingID, "dependency_product_id": input.DependencyProductID, "relation_type": input.RelationType, "assignment_scope": input.AssignmentScope, "default_quantity": input.DefaultQuantity})
	}
	if err != nil {
		dependencyWriteError(w, err)
		return
	}
	if tx.Commit() != nil {
		respondJSON(w, 500, map[string]string{"error": "Beziehung konnte nicht gespeichert werden"})
		return
	}
	// Preserve the legacy object response for integrations and the previous modal.
	var result models.ProductDependencyWithDetails
	err = repository.GetDB().Raw(`SELECT pd.id,pd.product_id,pd.dependency_product_id,p.name AS dependency_name,pd.relation_type,pd.assignment_scope,pd.default_quantity,pd.is_optional,pd.notes,pd.lifecycle_status,to_char(pd.updated_at,'YYYY-MM-DD"T"HH24:MI:SS.US"Z"') AS updated_at FROM product_dependencies pd JOIN products p ON p.productid=pd.dependency_product_id WHERE pd.id=?`, existingID).Scan(&result).Error
	if err != nil {
		respondJSON(w, 500, map[string]string{"error": "Beziehung gespeichert, Details konnten nicht geladen werden"})
		return
	}
	respondJSON(w, 201, result)
}
func setProductDependencyLifecycle(w http.ResponseWriter, r *http.Request, productID, relationID int, status string) {
	tx, err := repository.GetSQLDB().BeginTx(r.Context(), nil)
	if err != nil {
		respondJSON(w, 500, map[string]string{"error": "Beziehung konnte nicht gespeichert werden"})
		return
	}
	defer tx.Rollback()
	var input struct {
		Version string `json:"expectedUpdatedAt"`
	}
	if r.Body != nil {
		_ = json.NewDecoder(r.Body).Decode(&input)
	}
	if input.Version != "" {
		var version string
		if err = tx.QueryRow(`SELECT to_char(updated_at,'YYYY-MM-DD"T"HH24:MI:SS.US"Z"') FROM product_dependencies WHERE product_id=$1 AND id=$2 FOR UPDATE`, productID, relationID).Scan(&version); err != nil || version != input.Version {
			respondJSON(w, 409, map[string]string{"error": "Beziehung wurde inzwischen verändert. Bitte neu laden."})
			return
		}
	}

	result, err := tx.Exec(`UPDATE product_dependencies SET lifecycle_status=$1 WHERE product_id=$2 AND id=$3 AND lifecycle_status<>$1`, status, productID, relationID)
	if err != nil {
		dependencyWriteError(w, err)
		return
	}
	count, _ := result.RowsAffected()
	if count == 0 {
		respondJSON(w, 409, map[string]string{"error": "Beziehung ist bereits in diesem Zustand oder nicht vorhanden"})
		return
	}
	action := "product.relation.archive"
	if status == "active" {
		action = "product.relation.restore"
	}
	if err = recordProductAudit(tx, r, action, productID, nil, map[string]any{"relation_id": relationID, "lifecycle_status": status}); err != nil {
		respondJSON(w, 500, map[string]string{"error": "Beziehung konnte nicht protokolliert werden"})
		return
	}
	if tx.Commit() != nil {
		respondJSON(w, 500, map[string]string{"error": "Beziehung konnte nicht gespeichert werden"})
		return
	}
	respondJSON(w, 200, map[string]string{"lifecycle_status": status})
}
func dependencyWriteError(w http.ResponseWriter, err error) {
	message := "Beziehung konnte nicht geändert werden"
	if strings.Contains(err.Error(), "Active jobs") {
		message = "Aktive Jobs verwenden diese Packregel. Änderungen sind erst nach Abschluss möglich."
	} else if strings.Contains(err.Error(), "cycle") {
		message = "Diese Beziehung würde einen Kreis in den Packregeln erzeugen."
	} else if strings.Contains(err.Error(), "active") {
		message = "Beide verknüpften Produkte müssen aktiv sein."
	}
	respondJSON(w, 409, map[string]string{"error": message})
}
