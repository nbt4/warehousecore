package handlers

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"reflect"
	"sort"
	"strings"

	"warehousecore/internal/middleware"
	"warehousecore/internal/repository"
)

type warehousePackageItem struct {
	ProductID  int64 `json:"product_id"`
	Quantity   int64 `json:"quantity"`
	IsOptional bool  `json:"is_optional"`
}
type warehousePackageFields struct {
	Name           string                 `json:"name"`
	Description    *string                `json:"description"`
	Price          *float64               `json:"price"`
	Category       *string                `json:"category"`
	WebsiteVisible bool                   `json:"website_visible"`
	Aliases        []string               `json:"aliases"`
	Items          []warehousePackageItem `json:"items"`
}

// This endpoint receives the full prepared replacement, never a partial patch.
func normalizeWarehousePackage(fields *warehousePackageFields) error {
	fields.Name = strings.TrimSpace(fields.Name)
	if fields.Name == "" || len([]rune(fields.Name)) > 255 {
		return fmt.Errorf("name must contain 1-255 characters")
	}
	for _, f := range []struct {
		p   **string
		max int
	}{{&fields.Description, 4000}, {&fields.Category, 100}} {
		if *f.p != nil {
			v := strings.TrimSpace(**f.p)
			if len([]rune(v)) > f.max {
				return fmt.Errorf("description or category is too long")
			}
			if v == "" {
				*f.p = nil
			} else {
				*f.p = &v
			}
		}
	}
	if fields.Price != nil {
		v := *fields.Price
		if math.IsNaN(v) || math.IsInf(v, 0) || v < 0 || v > 99999999.99 || math.Abs(v*100-math.Round(v*100)) > 0.00001 {
			return fmt.Errorf("price must be nonnegative with at most two decimals, maximum 99999999.99")
		}
	}
	if len(fields.Aliases) > 50 {
		return fmt.Errorf("at most 50 aliases are allowed")
	}
	fields.Aliases = normalizeAliases(fields.Aliases)
	for _, alias := range fields.Aliases {
		if len([]rune(alias)) > 160 {
			return fmt.Errorf("aliases must contain at most 160 characters")
		}
	}
	if len(fields.Items) == 0 || len(fields.Items) > 200 {
		return fmt.Errorf("a package requires 1-200 product lines")
	}
	seen := map[int64]bool{}
	for _, item := range fields.Items {
		if item.ProductID <= 0 || item.ProductID > math.MaxInt32 || item.Quantity <= 0 || item.Quantity > 1000000 || seen[item.ProductID] {
			return fmt.Errorf("product lines require distinct valid product IDs and quantities from 1 to 1000000")
		}
		seen[item.ProductID] = true
	}
	sort.Slice(fields.Items, func(i, j int) bool { return fields.Items[i].ProductID < fields.Items[j].ProductID })
	return nil
}

func loadWarehousePackage(tx *sql.Tx, id int64) (warehousePackageFields, string, string, bool, error) {
	var fields warehousePackageFields
	var description, category sql.NullString
	var price sql.NullFloat64
	var aliases, items, version, code string
	var active bool
	err := tx.QueryRow(`SELECT name,description,price,category,COALESCE(website_visible,false),
 COALESCE(NULLIF(alias_json,''),'[]'),
 COALESCE((SELECT jsonb_agg(jsonb_build_object('product_id',product_id,'quantity',COALESCE(quantity,1),'is_optional',COALESCE(is_optional,false)) ORDER BY product_id,id)::text FROM product_package_items WHERE package_id=$1),'[]'),
 to_char(updated_at,'YYYY-MM-DD"T"HH24:MI:SS.US"Z"'),COALESCE(NULLIF(package_code,''),code,''),COALESCE(is_active,true)
 FROM product_packages WHERE id=$1 FOR UPDATE`, id).Scan(&fields.Name, &description, &price, &category, &fields.WebsiteVisible, &aliases, &items, &version, &code, &active)
	if err != nil {
		return fields, "", "", false, err
	}
	if description.Valid {
		fields.Description = &description.String
	}
	if category.Valid {
		fields.Category = &category.String
	}
	if price.Valid {
		fields.Price = &price.Float64
	}
	if err = json.Unmarshal([]byte(aliases), &fields.Aliases); err != nil {
		return fields, "", "", false, err
	}
	if fields.Aliases == nil {
		fields.Aliases = []string{}
	}
	if err = json.Unmarshal([]byte(items), &fields.Items); err != nil {
		return fields, "", "", false, err
	}
	return fields, version, code, active, nil
}

func mutateWarehousePackageMCP(w http.ResponseWriter, r *http.Request, id int64) {
	user, ok := middleware.GetUserFromContext(r)
	if !ok || user == nil || !user.IsAdmin {
		respondJSON(w, http.StatusForbidden, map[string]string{"error": "Warehouse administrator permission is required"})
		return
	}
	var input struct {
		warehousePackageFields
		ExpectedUpdatedAt string `json:"expected_updated_at"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid package body"})
		return
	}
	if err := normalizeWarehousePackage(&input.warehousePackageFields); err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	if id > 0 && input.ExpectedUpdatedAt == "" {
		respondJSON(w, http.StatusPreconditionRequired, map[string]string{"error": "The exact expected_updated_at from the preview is required"})
		return
	}
	operation, status := "package.create", http.StatusCreated
	if id > 0 {
		operation, status = "package.update", http.StatusOK
	}
	tx, err := repository.GetSQLDB().BeginTx(r.Context(), nil)
	if err != nil {
		respondWarehouseMutationError(w, err)
		return
	}
	defer tx.Rollback()
	if _, err = tx.Exec(`SET LOCAL lock_timeout='5s'; SET LOCAL statement_timeout='15s'`); err != nil {
		respondWarehouseMutationError(w, err)
		return
	}
	receiptID, replay, err := beginWarehouseProductMutation(tx, r, operation, map[string]any{"id": id, "after": input.warehousePackageFields, "expected_updated_at": input.ExpectedUpdatedAt})
	if err != nil {
		respondWarehouseMutationError(w, err)
		return
	}
	if replay != nil {
		respondJSON(w, status, replay)
		return
	}
	// Include UI item writers, product lifecycle writes and new job references.
	if _, err = tx.Exec(`LOCK TABLE product_packages,product_package_items IN SHARE ROW EXCLUSIVE MODE; LOCK TABLE products,job_packages IN SHARE MODE`); err != nil {
		respondWarehouseMutationError(w, err)
		return
	}
	var before any
	var code, version string
	if id > 0 {
		current, v, c, active, loadErr := loadWarehousePackage(tx, id)
		if loadErr == sql.ErrNoRows {
			respondJSON(w, http.StatusNotFound, map[string]string{"error": "Package not found"})
			return
		}
		if loadErr != nil {
			respondWarehouseMutationError(w, loadErr)
			return
		}
		if !active {
			respondJSON(w, http.StatusConflict, map[string]string{"error": "Only active packages can be updated"})
			return
		}
		if v != input.ExpectedUpdatedAt {
			respondJSON(w, http.StatusConflict, map[string]string{"error": "Package changed since the preview"})
			return
		}
		if reflect.DeepEqual(current, input.warehousePackageFields) {
			respondJSON(w, http.StatusConflict, map[string]string{"error": "No package fields changed"})
			return
		}
		if !reflect.DeepEqual(current.Items, input.Items) || !reflect.DeepEqual(current.Price, input.Price) {
			var used bool
			if err = tx.QueryRow(`SELECT EXISTS(SELECT 1 FROM job_packages WHERE package_id=$1)`, id).Scan(&used); err != nil {
				respondWarehouseMutationError(w, err)
				return
			}
			if used {
				respondJSON(w, http.StatusConflict, map[string]string{"error": "Package price and contents used in jobs must be preserved; create a new package"})
				return
			}
		}
		before, code = current, c
	}
	var duplicate bool
	if err = tx.QueryRow(`SELECT EXISTS(SELECT 1 FROM product_packages WHERE id<>$1 AND lower(trim(name))=lower($2))`, id, input.Name).Scan(&duplicate); err != nil {
		respondWarehouseMutationError(w, err)
		return
	}
	if duplicate {
		respondJSON(w, http.StatusConflict, map[string]string{"error": "Package name already exists"})
		return
	}
	for _, item := range input.Items {
		var active bool
		if err = tx.QueryRow(`SELECT COALESCE(lifecycle_status,'active')='active' FROM products WHERE productid=$1`, item.ProductID).Scan(&active); err == sql.ErrNoRows {
			respondJSON(w, http.StatusNotFound, map[string]string{"error": "Package product not found"})
			return
		}
		if err != nil {
			respondWarehouseMutationError(w, err)
			return
		}
		if !active {
			respondJSON(w, http.StatusConflict, map[string]string{"error": "Package products must be active"})
			return
		}
	}
	aliases, _ := json.Marshal(input.Aliases)
	if id == 0 {
		code, err = generatePackageCode(tx)
		if err != nil {
			respondWarehouseMutationError(w, err)
			return
		}
		err = tx.QueryRow(`INSERT INTO product_packages(code,package_code,name,description,price,category,is_active,website_visible,alias_json) VALUES($1,$1,$2,$3,$4,$5,true,$6,$7) RETURNING id`, code, input.Name, input.Description, input.Price, input.Category, input.WebsiteVisible, string(aliases)).Scan(&id)
	} else {
		_, err = tx.Exec(`UPDATE product_packages SET name=$1,description=$2,price=$3,category=$4,website_visible=$5,alias_json=$6 WHERE id=$7`, input.Name, input.Description, input.Price, input.Category, input.WebsiteVisible, string(aliases), id)
	}
	if err != nil {
		respondWarehouseMutationError(w, err)
		return
	}
	// Preserve line IDs and timestamps for metadata-only changes.
	replace := before == nil
	if current, ok := before.(warehousePackageFields); ok {
		replace = !reflect.DeepEqual(current.Items, input.Items)
	}
	if replace {
		if _, err = tx.Exec(`DELETE FROM product_package_items WHERE package_id=$1`, id); err != nil {
			respondWarehouseMutationError(w, err)
			return
		}
		for _, item := range input.Items {
			if _, err = tx.Exec(`INSERT INTO product_package_items(package_id,product_id,quantity,is_optional) VALUES($1,$2,$3,$4)`, id, item.ProductID, item.Quantity, item.IsOptional); err != nil {
				respondWarehouseMutationError(w, err)
				return
			}
		}
	}
	if err = tx.QueryRow(`SELECT to_char(updated_at,'YYYY-MM-DD"T"HH24:MI:SS.US"Z"') FROM product_packages WHERE id=$1`, id).Scan(&version); err != nil {
		respondWarehouseMutationError(w, err)
		return
	}
	oldJSON, err := json.Marshal(before)
	if err != nil {
		respondWarehouseMutationError(w, err)
		return
	}
	newJSON, err := json.Marshal(map[string]any{"origin": "MCP/AI", "after": input.warehousePackageFields, "updated_at": version, "is_active": true})
	if err != nil {
		respondWarehouseMutationError(w, err)
		return
	}
	if _, err = tx.Exec(`INSERT INTO audit_log(user_id,action,entity_type,entity_id,old_values,new_values,user_agent) VALUES($1,$2,'package',$3,$4::jsonb,$5::jsonb,$6)`, user.UserID, operation, fmt.Sprint(id), string(oldJSON), string(newJSON), r.UserAgent()); err != nil {
		respondWarehouseMutationError(w, err)
		return
	}
	response := map[string]any{"package_id": id, "package_code": code, "updated_at": version, "package": input.warehousePackageFields}
	if err = completeWarehouseProductMutation(tx, receiptID, status, response); err != nil {
		respondWarehouseMutationError(w, err)
		return
	}
	if err = tx.Commit(); err != nil {
		respondWarehouseMutationError(w, err)
		return
	}
	websiteRevalidator.Revalidate("/products")
	respondJSON(w, status, response)
}
