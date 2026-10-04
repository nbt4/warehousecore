package handlers

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"sort"
	"strings"

	jwt "github.com/golang-jwt/jwt/v5"
	"warehousecore/internal/middleware"
	"warehousecore/internal/models"
	"warehousecore/internal/repository"
	"warehousecore/internal/services"
)

// The import contract accepts creation fields only; IDs, website publishing and
// physical lifecycle state cannot be supplied as arbitrary Product fields.
type warehouseProductImportFields struct {
	Name                     string          `json:"name"`
	CategoryID               *int            `json:"category_id"`
	SubcategoryID            *string         `json:"subcategory_id"`
	SubbiercategoryID        *string         `json:"subbiercategory_id"`
	ManufacturerID           *int            `json:"manufacturer_id"`
	BrandID                  *int            `json:"brand_id"`
	Description              *string         `json:"description"`
	MaintenanceInterval      *int            `json:"maintenance_interval"`
	ItemCostPerDay           *float64        `json:"item_cost_per_day"`
	Weight                   *float64        `json:"weight"`
	Height                   *float64        `json:"height"`
	Width                    *float64        `json:"width"`
	Depth                    *float64        `json:"depth"`
	PowerConsumption         *float64        `json:"power_consumption"`
	PosInCategory            *int            `json:"pos_in_category"`
	IsAccessory              bool            `json:"is_accessory"`
	IsConsumable             bool            `json:"is_consumable"`
	CountTypeID              *int            `json:"count_type_id"`
	StockQuantity            *float64        `json:"stock_quantity"`
	MinStockLevel            *float64        `json:"min_stock_level"`
	GenericBarcode           *string         `json:"generic_barcode"`
	PricePerUnit             *float64        `json:"price_per_unit"`
	ProductType              string          `json:"product_type"`
	TrackingMode             string          `json:"tracking_mode"`
	ProductKind              string          `json:"product_kind"`
	ModelNumber              *string         `json:"model_number"`
	ManufacturerPartNo       *string         `json:"manufacturer_part_number"`
	EAN                      *string         `json:"ean"`
	Attributes               json.RawMessage `json:"attributes,omitempty"`
	InitialDeviceQty         int             `json:"initial_device_quantity,omitempty"`
	InitialZoneID            *int            `json:"initial_zone_id,omitempty"`
	ProcurementProductID     *int64          `json:"procurement_product_id,omitempty"`
	ManufacturerNameInput    *string         `json:"manufacturer_name_input,omitempty"`
	ManufacturerWebsiteInput *string         `json:"manufacturer_website_input,omitempty"`
	BrandNameInput           *string         `json:"brand_name_input,omitempty"`
	CategoryNameInput        *string         `json:"category_name_input,omitempty"`
	CategoryAbbrInput        *string         `json:"category_abbreviation_input,omitempty"`
	SubcategoryNameInput     *string         `json:"subcategory_name_input,omitempty"`
	SubcategoryAbbrInput     *string         `json:"subcategory_abbreviation_input,omitempty"`
	ThirdCategoryNameInput   *string         `json:"third_category_name_input,omitempty"`
	ThirdCategoryAbbrInput   *string         `json:"third_category_abbreviation_input,omitempty"`
	AllowSimilarProduct      bool            `json:"allow_similar_product"`
	AcceptIncomplete         bool            `json:"accept_incomplete"`
}

type warehouseProductBatchRequest struct {
	Products         []warehouseProductImportFields `json:"products"`
	ExpectedContext  string                         `json:"expected_context"`
	ConfirmCreation  bool                           `json:"confirm_creation"`
	ConfirmationText string                         `json:"confirmation_text"`
	Preview          bool                           `json:"preview"`
}

func productImportActor(r *http.Request) (*models.User, bool, error) {
	user, ok := middleware.GetUserFromContext(r)
	if !ok || user == nil || user.UserID == 0 || !user.IsActive || !user.IsAdmin || !isWarehouseMCPMutation(r) {
		return nil, false, &warehouseMutationError{403, "admin_required", "An active signed-in warehouse administrator and MCP origin are required"}
	}
	cookie, err := r.Cookie("cores_token")
	if err != nil {
		return nil, false, &warehouseMutationError{403, "scope_required", "A signed warehouse create scope is required"}
	}
	claims := struct {
		UserID    uint   `json:"uid"`
		Scope     string `json:"mcp_scope"`
		Financial bool   `json:"mcp_warehouse_financial"`
		jwt.RegisteredClaims
	}{}
	secret := os.Getenv("CORES_JWT_SECRET")
	if secret == "" {
		secret = os.Getenv("JWT_SECRET")
	}
	token, err := jwt.ParseWithClaims(cookie.Value, &claims, func(*jwt.Token) (any, error) { return []byte(secret), nil }, jwt.WithValidMethods([]string{"HS256"}), jwt.WithExpirationRequired())
	if secret == "" || err != nil || token == nil || !token.Valid || claims.UserID != user.UserID || claims.Scope != "cores:warehouse:create" {
		return nil, false, &warehouseMutationError{403, "scope_required", "The matching signed warehouse create scope is required"}
	}
	return user, claims.Financial, nil
}

// Recheck the current database role before every operation, including saved
// replay. The user lock keeps role revocation from racing the transaction.
func lockProductImportActor(tx *sql.Tx, user *models.User) error {
	var active, admin bool
	if err := tx.QueryRow(`SELECT is_active,is_admin FROM users WHERE userid=$1 FOR SHARE`, user.UserID).Scan(&active, &admin); err != nil {
		return err
	}
	if !active || !admin {
		return &warehouseMutationError{403, "admin_required", "Current warehouse administrator rights are required"}
	}
	return nil
}

func normalizedProductImport(in warehouseProductImportFields) (Product, error) {
	raw, err := json.Marshal(in)
	if err != nil {
		return Product{}, err
	}
	var product Product
	if err = json.Unmarshal(raw, &product); err != nil {
		return product, err
	}
	if err = normalizeProductRequest(&product); err != nil {
		return product, err
	}
	for _, id := range []*int{product.CategoryID, product.ManufacturerID, product.BrandID, product.CountTypeID, product.InitialZoneID} {
		if id != nil && (*id <= 0 || *id > math.MaxInt32) {
			return product, fmt.Errorf("Positive serial reference IDs are required")
		}
	}
	for _, value := range []*float64{product.ItemCostPerDay, product.Weight, product.Height, product.Width, product.Depth, product.PowerConsumption, product.StockQuantity, product.MinStockLevel, product.PricePerUnit} {
		if value != nil && (math.IsNaN(*value) || math.IsInf(*value, 0) || *value > 99999999.99) {
			return product, fmt.Errorf("Product numbers must be finite and at most 99999999.99")
		}
	}
	for _, v := range []*string{product.ModelNumber, product.ManufacturerPartNo, product.EAN, product.GenericBarcode, product.SubcategoryID, product.SubbiercategoryID, product.ManufacturerNameInput, product.BrandNameInput, product.CategoryNameInput, product.CategoryAbbrInput, product.SubcategoryNameInput, product.SubcategoryAbbrInput, product.ThirdCategoryNameInput, product.ThirdCategoryAbbrInput} {
		if v != nil {
			*v = strings.TrimSpace(*v)
			if *v == "" || len(*v) > 255 {
				return product, fmt.Errorf("Optional identities and master inputs must contain 1-255 bytes")
			}
		}
	}
	for field, spec := range map[string]struct {
		value *string
		limit int
	}{
		"model_number": {product.ModelNumber, 160}, "manufacturer_part_number": {product.ManufacturerPartNo, 160}, "ean": {product.EAN, 32}, "generic_barcode": {product.GenericBarcode, 100}, "subcategory_id": {product.SubcategoryID, 50}, "subbiercategory_id": {product.SubbiercategoryID, 50}, "category_name_input": {product.CategoryNameInput, 100}, "subcategory_name_input": {product.SubcategoryNameInput, 100}, "third_category_name_input": {product.ThirdCategoryNameInput, 100}, "category_abbreviation_input": {product.CategoryAbbrInput, 10}, "subcategory_abbreviation_input": {product.SubcategoryAbbrInput, 10}, "third_category_abbreviation_input": {product.ThirdCategoryAbbrInput, 10},
	} {
		if spec.value != nil && len(*spec.value) > spec.limit {
			return product, fmt.Errorf("%s exceeds %d bytes", field, spec.limit)
		}
	}
	for _, pair := range []struct {
		id   any
		name *string
	}{{product.ManufacturerID, product.ManufacturerNameInput}, {product.BrandID, product.BrandNameInput}, {product.CategoryID, product.CategoryNameInput}, {product.SubcategoryID, product.SubcategoryNameInput}, {product.SubbiercategoryID, product.ThirdCategoryNameInput}} {
		raw, _ := json.Marshal(pair.id)
		if string(raw) != "null" && pair.name != nil {
			return product, fmt.Errorf("Choose an existing master ID or approved name_input, never both")
		}
	}
	if product.Description != nil && len(*product.Description) > 16000 {
		return product, fmt.Errorf("Description exceeds 16000 bytes")
	}
	if product.ManufacturerWebsiteInput != nil {
		normalized, valid := normalizedManufacturerWebsite(product.ManufacturerWebsiteInput)
		if !valid {
			return product, fmt.Errorf("Invalid manufacturer website")
		}
		product.ManufacturerWebsiteInput = normalized
	}
	if product.ProcurementProductID != nil && (*product.ProcurementProductID <= 0 || *product.ProcurementProductID > math.MaxInt32) {
		return product, fmt.Errorf("Invalid procurement product ID")
	}
	if len(product.Attributes) == 0 {
		product.Attributes = json.RawMessage(`{}`)
	}
	var attributes map[string]any
	if json.Unmarshal(product.Attributes, &attributes) != nil || attributes == nil || len(product.Attributes) > 65536 {
		return product, fmt.Errorf("Attributes must be a bounded JSON object")
	}
	if product.StockQuantity != nil && *product.StockQuantity > 0 && (product.TrackingMode != "quantity" || product.InitialZoneID == nil) {
		return product, fmt.Errorf("Initial quantity requires quantity tracking and a storage destination")
	}
	return product, nil
}

type importMasterSpec struct {
	kind, table, id    string
	idValue            any
	name, abbreviation *string
}

// Table identifiers come exclusively from this fixed business catalog.
func productImportMasters(tx *sql.Tx, product Product) ([]map[string]any, []string, error) {
	specs := []importMasterSpec{
		{"manufacturer", "manufacturer", "manufacturerid", product.ManufacturerID, product.ManufacturerNameInput, nil},
		{"brand", "brands", "brandid", product.BrandID, product.BrandNameInput, nil},
		{"category", "categories", "categoryid", product.CategoryID, product.CategoryNameInput, product.CategoryAbbrInput},
		{"subcategory", "subcategories", "subcategoryid", product.SubcategoryID, product.SubcategoryNameInput, product.SubcategoryAbbrInput},
		{"third_category", "subbiercategories", "subbiercategoryid", product.SubbiercategoryID, product.ThirdCategoryNameInput, product.ThirdCategoryAbbrInput},
		{"count_type", "count_types", "count_type_id", product.CountTypeID, nil, nil},
	}
	plans := []map[string]any{}
	missing := []string{}
	resolved := map[string]map[string]any{}
	for _, spec := range specs {
		idRaw, _ := json.Marshal(spec.idValue)
		hasID := string(idRaw) != "null"
		name := optionalInput(spec.name)
		if !hasID && name == "" {
			continue
		}
		rows, err := inventoryMCPRows(tx, `SELECT to_jsonb(t) FROM `+spec.table+` t WHERE ($1::text IS NOT NULL AND `+spec.id+`::text=$1) OR ($1::text IS NULL AND lower(trim(name))=lower($2)) ORDER BY `+spec.id+` LIMIT 3`, spec.idValue, name)
		if err != nil {
			return nil, nil, err
		}
		plan := map[string]any{"entity": spec.kind, "records": rows, "action": "create", "name": name, "abbreviation": optionalInput(spec.abbreviation)}
		if len(rows) > 1 {
			missing = append(missing, spec.kind+"_ambiguous")
		}
		if len(rows) == 1 {
			plan["action"] = "reference"
			resolved[spec.kind] = rows[0]
			if state, ok := rows[0]["lifecycle_status"]; ok && state != "active" {
				missing = append(missing, spec.kind+"_restore")
			}
		} else if hasID {
			missing = append(missing, spec.kind+"_not_found")
		} else if spec.kind == "category" && optionalInput(spec.abbreviation) == "" {
			missing = append(missing, "category_abbreviation_input")
		}
		if spec.kind == "subcategory" && product.CategoryID == nil && product.CategoryNameInput == nil {
			missing = append(missing, "subcategory_parent")
		}
		if spec.kind == "third_category" && product.SubcategoryID == nil && product.SubcategoryNameInput == nil {
			missing = append(missing, "third_category_parent")
		}
		if spec.kind == "brand" && product.ManufacturerID == nil && product.ManufacturerNameInput == nil {
			missing = append(missing, "brand_manufacturer")
		}
		plans = append(plans, plan)
	}
	if product.CategoryID == nil && product.CategoryNameInput == nil {
		missing = append(missing, "category_id|category_name_input")
	}
	if product.ManufacturerID == nil && product.ManufacturerNameInput == nil {
		missing = append(missing, "manufacturer_id|manufacturer_name_input")
	}
	if b, ok := resolved["brand"]; ok && b["manufacturerid"] != nil {
		if m, ok := resolved["manufacturer"]; ok && b["manufacturerid"] != m["manufacturerid"] {
			missing = append(missing, "brand_manufacturer_mismatch")
		}
	}
	if sub, ok := resolved["subcategory"]; ok {
		if c, ok := resolved["category"]; !ok || sub["categoryid"] != c["categoryid"] {
			missing = append(missing, "subcategory_parent_mismatch")
		}
	}
	if sub, ok := resolved["third_category"]; ok {
		if parent, ok := resolved["subcategory"]; !ok || sub["subcategoryid"] != parent["subcategoryid"] {
			missing = append(missing, "third_category_parent_mismatch")
		}
	}
	return plans, missing, nil
}

func CreateProductsBulkMCP(w http.ResponseWriter, r *http.Request) {
	user, financial, err := productImportActor(r)
	if err != nil {
		respondWarehouseMutationError(w, err)
		return
	}
	var in warehouseProductBatchRequest
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1024*1024))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&in) != nil || decoder.Decode(new(any)) != io.EOF || len(in.Products) < 1 || len(in.Products) > 100 {
		respondJSON(w, 400, map[string]string{"error": "One closed product batch with 1-100 creation drafts is required"})
		return
	}
	products := make([]Product, len(in.Products))
	totalDevices := 0
	names, identities, procIDs := map[string]bool{}, map[string]bool{}, map[int64]bool{}
	for i, item := range in.Products {
		if (item.ItemCostPerDay != nil || item.PricePerUnit != nil) && !financial {
			respondJSON(w, 403, map[string]string{"error": "Explicit signed warehouse financial scope is required for product prices"})
			return
		}
		if products[i], err = normalizedProductImport(item); err != nil {
			respondJSON(w, 400, map[string]any{"error": err.Error(), "item_index": i})
			return
		}
		totalDevices += products[i].InitialDeviceQty
		if totalDevices > 1000 {
			respondJSON(w, 400, map[string]string{"error": "A batch is limited to 1000 initial devices total"})
			return
		}
		key := strings.ToLower(products[i].Name)
		if names[key] {
			respondJSON(w, 409, map[string]string{"error": "Duplicate names in batch"})
			return
		}
		names[key] = true
		for field, value := range map[string]*string{"barcode": products[i].GenericBarcode, "ean": products[i].EAN, "part": products[i].ManufacturerPartNo} {
			if value != nil {
				key = field + ":" + strings.ToLower(*value)
				if identities[key] {
					respondJSON(w, 409, map[string]string{"error": "Duplicate product identity in batch"})
					return
				}
				identities[key] = true
			}
		}
		if id := products[i].ProcurementProductID; id != nil {
			if procIDs[*id] {
				respondJSON(w, 409, map[string]string{"error": "Duplicate procurement mapping in batch"})
				return
			}
			procIDs[*id] = true
		}
	}
	preview := in.Preview || !in.ConfirmCreation
	if !preview && len(in.ExpectedContext) != 64 {
		respondJSON(w, 428, map[string]string{"error": "The exact final expected_context is required"})
		return
	}
	tx, err := repository.GetSQLDB().BeginTx(r.Context(), nil)
	if err != nil {
		respondWarehouseMutationError(w, err)
		return
	}
	defer tx.Rollback()
	if _, err = tx.Exec(`SET LOCAL lock_timeout='5s';SET LOCAL statement_timeout='15s'`); err == nil {
		err = lockProductImportActor(tx, user)
	}
	if err != nil {
		respondWarehouseMutationError(w, err)
		return
	}
	var receipt int64
	if !preview {
		bound := in
		bound.Preview = false
		var replay json.RawMessage
		receipt, replay, err = beginWarehouseProductMutation(tx, r, "product.bulk_create", bound)
		if err != nil {
			respondWarehouseMutationError(w, err)
			return
		}
		if replay != nil {
			respondJSON(w, 201, replay)
			return
		}
	}
	// Legacy UI imports and other writers take ordinary row/table locks, so they
	// cannot invalidate the reviewed references between validation and commit.
	if _, err = tx.Exec(`LOCK TABLE products,manufacturer,brands,categories,subcategories,subbiercategories IN SHARE ROW EXCLUSIVE MODE;LOCK TABLE count_types,storage_zones,location_profiles,devices,cases,product_locations,proc_products,core_product_links IN SHARE MODE`); err != nil {
		respondWarehouseMutationError(w, err)
		return
	}
	reviews := []map[string]any{}
	masterDefinitions := map[string]string{}
	ready := true
	occupancy := map[int64]float64{}
	for i, product := range products {
		plans, missing, loadErr := productImportMasters(tx, product)
		if loadErr != nil {
			respondWarehouseMutationError(w, loadErr)
			return
		}
		for _, plan := range plans {
			if plan["action"] == "create" {
				kind := plan["entity"].(string)
				key := kind + ":" + strings.ToLower(fmt.Sprint(plan["name"]))
				definition := map[string]any{"plan": plan}
				if kind == "manufacturer" {
					definition["website"] = product.ManufacturerWebsiteInput
				}
				if kind == "brand" {
					definition["manufacturer_id"] = product.ManufacturerID
					definition["manufacturer_name"] = product.ManufacturerNameInput
				}
				if kind == "subcategory" {
					definition["category_id"] = product.CategoryID
					definition["category_name"] = product.CategoryNameInput
				}
				if kind == "third_category" {
					definition["subcategory_id"] = product.SubcategoryID
					definition["subcategory_name"] = product.SubcategoryNameInput
				}
				hash := inventoryMCPHash(definition)
				if prior, ok := masterDefinitions[key]; ok && prior != hash {
					missing = append(missing, "conflicting_shared_master_definition")
				}
				masterDefinitions[key] = hash
			}
		}
		candidates, loadErr := inventoryMCPRows(tx, `SELECT jsonb_build_object('product_id',productid,'name',name,'ean',ean,'generic_barcode',generic_barcode,'manufacturer_part_number',manufacturer_part_number,'lifecycle_status',lifecycle_status,'updated_at',to_char(updated_at,'YYYY-MM-DD"T"HH24:MI:SS.US"Z"')) FROM products WHERE lower(trim(name))=lower($1) OR ($2::text IS NOT NULL AND lower(trim(generic_barcode))=lower($2)) OR ($3::text IS NOT NULL AND lower(trim(ean))=lower($3)) OR ($4::text IS NOT NULL AND lower(trim(manufacturer_part_number))=lower($4)) OR strpos(lower(name),lower($1))>0 ORDER BY (lower(trim(name))=lower($1) OR ($2::text IS NOT NULL AND lower(trim(generic_barcode))=lower($2)) OR ($3::text IS NOT NULL AND lower(trim(ean))=lower($3)) OR ($4::text IS NOT NULL AND lower(trim(manufacturer_part_number))=lower($4))) DESC,productid LIMIT 20`, product.Name, product.GenericBarcode, product.EAN, product.ManufacturerPartNo)
		if loadErr != nil {
			respondWarehouseMutationError(w, loadErr)
			return
		}
		for _, candidate := range candidates {
			exact := strings.EqualFold(strings.TrimSpace(candidate["name"].(string)), product.Name)
			for key, value := range map[string]*string{"ean": product.EAN, "generic_barcode": product.GenericBarcode, "manufacturer_part_number": product.ManufacturerPartNo} {
				if value != nil && candidate[key] != nil && strings.EqualFold(*value, fmt.Sprint(candidate[key])) {
					exact = true
				}
			}
			if exact {
				missing = append(missing, "existing_product")
			} else if !in.Products[i].AllowSimilarProduct {
				missing = append(missing, "similar_product_review")
			}
		}
		reserved := []map[string]any{}
		if product.GenericBarcode != nil {
			var e error
			reserved, e = inventoryMCPRows(tx, `SELECT jsonb_build_object('entity_type',entity_type,'entity_key',entity_key,'identifier_kind',identifier_kind,'active',active,'code',code) FROM inventory_identifiers WHERE lower(trim(code))=lower($1) ORDER BY entity_type,entity_key,identifier_kind LIMIT 20`, *product.GenericBarcode)
			if e != nil {
				respondWarehouseMutationError(w, e)
				return
			}
			if len(reserved) > 0 {
				missing = append(missing, "reserved_scan_identity")
			}
		}
		recommended := []string{}
		if product.Description == nil {
			recommended = append(recommended, "description")
		}
		if product.ModelNumber == nil && product.ManufacturerPartNo == nil {
			recommended = append(recommended, "model_number|manufacturer_part_number")
		}
		if len(recommended) > 0 && !in.Products[i].AcceptIncomplete {
			missing = append(missing, "accept_incomplete")
		}
		var mapping any
		if id := product.ProcurementProductID; id != nil {
			rows, e := inventoryMCPRows(tx, `SELECT jsonb_build_object('id',p.id,'sku',p.sku,'name',p.name,'active',p.active,'updated_at',p.updated_at,'existing_link',to_jsonb(l)) FROM proc_products p LEFT JOIN core_product_links l ON l.procurement_product_id=p.id WHERE p.id=$1`, *id)
			if e != nil {
				respondWarehouseMutationError(w, e)
				return
			}
			mapping = rows
			if len(rows) != 1 || rows[0]["active"] != true || rows[0]["existing_link"] != nil {
				missing = append(missing, "available_procurement_product")
			}
		}
		if product.InitialZoneID != nil {
			if e := validateCaseMCPHierarchy(tx, int64(*product.InitialZoneID)); e != nil {
				respondJSON(w, 409, map[string]string{"error": e.Error()})
				return
			}
			occupancy[int64(*product.InitialZoneID)] += float64(product.InitialDeviceQty)
			if product.TrackingMode == "quantity" && product.StockQuantity != nil {
				occupancy[int64(*product.InitialZoneID)] += *product.StockQuantity
			}
		}
		reviews = append(reviews, map[string]any{"item_index": i, "draft": product, "master_data_plan": plans, "similar_products": candidates, "reserved_scan_identities": reserved, "procurement_mapping": mapping, "required_missing_fields": missing, "recommended_missing_fields": recommended})
		if len(missing) > 0 {
			ready = false
		}
	}
	destinations := []map[string]any{}
	for zone, incoming := range occupancy {
		// Full shared destination snapshot includes profile versions and constraints.
		snapshot, e := inventoryMCPJSON(tx, inventoryMCPZoneSelect, zone)
		if e != nil {
			respondWarehouseMutationError(w, e)
			return
		}
		snapshot["incoming_quantity"] = incoming
		ancestors, e := inventoryMCPRows(tx, `WITH RECURSIVE ancestors AS (SELECT zone_id,parent_zone_id FROM storage_zones WHERE zone_id=$1 UNION SELECT p.zone_id,p.parent_zone_id FROM storage_zones p JOIN ancestors a ON p.zone_id=a.parent_zone_id) SELECT to_jsonb(z) FROM storage_zones z JOIN ancestors a ON a.zone_id=z.zone_id ORDER BY z.zone_id`, zone)
		if e != nil {
			respondWarehouseMutationError(w, e)
			return
		}
		snapshot["ancestors"] = ancestors
		for _, product := range products {
			if product.InitialZoneID != nil && int64(*product.InitialZoneID) == zone {
				if product.InitialDeviceQty > 0 && snapshot["allow_devices"] != true || product.StockQuantity != nil && *product.StockQuantity > 0 && snapshot["allow_quantity_products"] != true {
					respondJSON(w, 409, map[string]string{"error": "Storage profile does not permit the initial inventory"})
					return
				}
			}
		}
		var occupancyNow []byte
		e = tx.QueryRow(`SELECT jsonb_build_object('devices',(SELECT count(*) FROM devices WHERE zone_id=$1 AND status='in_storage' AND lifecycle_status='active'),'products',(SELECT COALESCE(sum(quantity),0) FROM product_locations WHERE zone_id=$1),'cases',(SELECT count(*) FROM cases WHERE zone_id=$1))`, zone).Scan(&occupancyNow)
		if e != nil {
			respondWarehouseMutationError(w, e)
			return
		}
		snapshot["occupancy"] = json.RawMessage(occupancyNow)
		destinations = append(destinations, snapshot)
	}
	// Sort map-derived destinations to make preview hashes deterministic.
	sort.Slice(destinations, func(i, j int) bool {
		return destinations[i]["zone_id"].(float64) < destinations[j]["zone_id"].(float64)
	})
	for zone, incoming := range occupancy {
		if e := services.ValidateStorageDestination(tx, zone, incoming); e != nil {
			respondJSON(w, 409, map[string]string{"error": e.Error()})
			return
		}
	}
	context := inventoryMCPHash(map[string]any{"products": products, "reviews": reviews, "destinations": destinations})
	phrase := fmt.Sprintf("CREATE WAREHOUSE PRODUCT BATCH %d %s", len(products), context[:16])
	if preview {
		respondJSON(w, 200, map[string]any{"operation_status": "confirmation_required", "preview": true, "ready": ready, "products": in.Products, "items": reviews, "destinations": destinations, "count": len(products), "expected_context": context, "required_confirmation_text": phrase})
		return
	}
	if !ready || in.ExpectedContext != context {
		respondJSON(w, 409, map[string]string{"error": "Product drafts or references changed, or unresolved questions remain; prepare the batch again"})
		return
	}
	if in.ConfirmationText != phrase {
		respondJSON(w, 428, map[string]string{"error": "The exact batch and context-bound confirmation phrase is required"})
		return
	}
	created := make([]Product, 0, len(products))
	for _, product := range products {
		if err = createProductInTransaction(tx, r, &product); err != nil {
			respondWarehouseMutationError(w, err)
			return
		}
		// The bootstrap also retains the immutable product code as a scan alias
		// when a user supplied a distinct barcode. Allocate it now, in the same
		// transaction, rather than letting a later restart add another identity.
		if product.GenericBarcode != nil && !strings.EqualFold(strings.TrimSpace(*product.GenericBarcode), strings.TrimSpace(product.ProductCode)) {
			_, err = tx.Exec(`INSERT INTO inventory_identifiers(entity_type,entity_key,code,identifier_kind,active) VALUES('product',$1,$2,'product_code',true) ON CONFLICT(entity_type,entity_key,identifier_kind) DO UPDATE SET code=EXCLUDED.code,active=EXCLUDED.active`, fmt.Sprint(product.ProductID), product.ProductCode)
			if err != nil {
				respondWarehouseMutationError(w, err)
				return
			}
		}
		created = append(created, product)
	}
	response := map[string]any{"operation_status": "created", "products": created, "count": len(created), "reviewed_context": context}
	if err = completeWarehouseProductMutation(tx, receipt, 201, response); err == nil {
		err = tx.Commit()
	}
	if err != nil {
		respondWarehouseMutationError(w, err)
		return
	}
	websiteRevalidator.Revalidate("/products")
	respondJSON(w, 201, response)
}
