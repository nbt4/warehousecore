package handlers

import (
	"encoding/json"
	"net/http"
	"sort"
	"strconv"
	"warehousecore/internal/repository"
)

// Bulk category assignment is atomic and compares every selected product version.
// Child categories are explicitly reset when changing the main category.
func AssignCatalogCategory(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Products []struct {
			ID      int    `json:"product_id"`
			Version string `json:"version"`
		} `json:"products"`
		CategoryID *int `json:"category_id"`
	}
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 32768)).Decode(&input) != nil || len(input.Products) < 1 || len(input.Products) > 100 {
		respondJSON(w, 400, map[string]string{"error": "1 bis 100 Produkte auswählen"})
		return
	}
	sort.Slice(input.Products, func(i, j int) bool { return input.Products[i].ID < input.Products[j].ID })
	for i, p := range input.Products {
		if p.ID < 1 || p.Version == "" || (i > 0 && input.Products[i-1].ID == p.ID) {
			respondJSON(w, 400, map[string]string{"error": "Produkt-ID und aktuelle Version erforderlich"})
			return
		}
	}
	tx, err := repository.GetSQLDB().BeginTx(r.Context(), nil)
	if err != nil {
		respondJSON(w, 500, map[string]string{"error": "Kategorie konnte nicht zugewiesen werden"})
		return
	}
	defer tx.Rollback()
	if input.CategoryID != nil {
		var active bool
		if err = tx.QueryRow(`SELECT lifecycle_status='active' FROM categories WHERE categoryid=$1 FOR SHARE`, *input.CategoryID).Scan(&active); err != nil || !active {
			respondJSON(w, 409, map[string]string{"error": "Aktive Kategorie auswählen"})
			return
		}
	}
	for _, product := range input.Products {
		var version, lifecycle string
		var oldCategory *int
		if err = tx.QueryRow(`SELECT to_char(updated_at,'YYYY-MM-DD"T"HH24:MI:SS.US"Z"'),lifecycle_status,categoryid FROM products WHERE productid=$1 FOR UPDATE`, product.ID).Scan(&version, &lifecycle, &oldCategory); err != nil || lifecycle != "active" || version != product.Version {
			respondJSON(w, 409, map[string]string{"error": "Ein Produkt wurde inzwischen verändert oder archiviert. Auswahl neu laden."})
			return
		}
		if _, err = tx.Exec(`UPDATE products SET categoryid=$1,subcategoryid=NULL,subbiercategoryid=NULL,updated_at=clock_timestamp() AT TIME ZONE 'UTC' WHERE productid=$2`, input.CategoryID, product.ID); err != nil {
			respondJSON(w, 409, map[string]string{"error": "Kategorie konnte nicht zugewiesen werden"})
			return
		}
		if err = recordProductAudit(tx, r, "product.category.assign", product.ID, map[string]any{"category_id": oldCategory}, map[string]any{"category_id": input.CategoryID, "subcategories_cleared": true}); err != nil {
			respondJSON(w, 500, map[string]string{"error": "Änderung konnte nicht protokolliert werden"})
			return
		}
	}
	if err = tx.Commit(); err != nil {
		respondJSON(w, 500, map[string]string{"error": "Kategorie konnte nicht gespeichert werden"})
		return
	}
	websiteRevalidator.Revalidate("/products")
	respondJSON(w, 200, map[string]string{"updated": strconv.Itoa(len(input.Products))})
}
