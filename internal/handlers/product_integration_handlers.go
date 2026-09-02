package handlers

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/gorilla/mux"

	"warehousecore/internal/middleware"
	"warehousecore/internal/repository"
)

type procurementCatalogProduct struct {
	ProductID      int64           `json:"product_id"`
	SKU            string          `json:"sku"`
	Name           string          `json:"name"`
	Description    string          `json:"description"`
	Manufacturer   string          `json:"manufacturer"`
	Model          string          `json:"model"`
	Unit           string          `json:"unit"`
	Category       string          `json:"category"`
	ReorderPoint   float64         `json:"reorder_point"`
	TargetStock    float64         `json:"target_stock"`
	BestPriceCents int64           `json:"best_price_cents"`
	PreferredOffer *int64          `json:"preferred_supplier_id,omitempty"`
	PurchaseURL    string          `json:"purchase_url"`
	Parameters     json.RawMessage `json:"parameters"`
	Attributes     json.RawMessage `json:"attributes"`
	WarehouseID    *int64          `json:"warehouse_product_id,omitempty"`
}

type warehouseLinkProduct struct {
	ProductID       int64    `json:"product_id"`
	ProductCode     string   `json:"product_code"`
	Name            string   `json:"name"`
	Manufacturer    string   `json:"manufacturer"`
	Model           string   `json:"model"`
	ManufacturerSKU string   `json:"manufacturer_part_number"`
	EAN             string   `json:"ean"`
	StockQuantity   float64  `json:"stock_quantity"`
	MinimumStock    float64  `json:"min_stock_level"`
	TrackingMode    string   `json:"tracking_mode"`
	ProcurementID   *int64   `json:"procurement_product_id,omitempty"`
	SuggestedID     *int64   `json:"suggested_procurement_product_id,omitempty"`
	SuggestionScore int      `json:"suggestion_score"`
	SuggestionNotes []string `json:"suggestion_reasons"`
}

func normalizedProductIdentity(value string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return unicode.ToLower(r)
		}
		return -1
	}, strings.TrimSpace(value))
}

func rawJSONMap(values ...json.RawMessage) map[string]any {
	result := map[string]any{}
	for _, raw := range values {
		var current map[string]any
		if json.Unmarshal(raw, &current) == nil {
			for key, value := range current {
				result[key] = value
			}
		}
	}
	return result
}

func catalogEAN(product procurementCatalogProduct) string {
	for key, value := range rawJSONMap(product.Attributes, product.Parameters) {
		normalized := normalizedProductIdentity(key)
		if normalized == "ean" || normalized == "gtin" || normalized == "gtin13" {
			return fmt.Sprint(value)
		}
	}
	return ""
}

func scoreCatalogProduct(warehouse warehouseLinkProduct, procurement procurementCatalogProduct) (int, []string) {
	score, reasons := 0, []string{}
	if left, right := normalizedProductIdentity(warehouse.EAN), normalizedProductIdentity(catalogEAN(procurement)); left != "" && left == right {
		score += 100
		reasons = append(reasons, "EAN identisch")
	}
	if left, right := normalizedProductIdentity(warehouse.ManufacturerSKU), normalizedProductIdentity(procurement.SKU); left != "" && left == right {
		score += 80
		reasons = append(reasons, "Herstellerartikelnummer identisch")
	}
	if left, right := normalizedProductIdentity(warehouse.Model), normalizedProductIdentity(procurement.Model); left != "" && left == right {
		score += 55
		reasons = append(reasons, "Modell identisch")
	}
	if left, right := normalizedProductIdentity(warehouse.Manufacturer), normalizedProductIdentity(procurement.Manufacturer); left != "" && left == right {
		score += 25
		reasons = append(reasons, "Hersteller identisch")
	}
	if left, right := normalizedProductIdentity(warehouse.Name), normalizedProductIdentity(procurement.Name); left != "" && left == right {
		score += 60
		reasons = append(reasons, "Name identisch")
	} else if left != "" && right != "" && (strings.Contains(left, right) || strings.Contains(right, left)) {
		score += 25
		reasons = append(reasons, "Name ähnlich")
	}
	return score, reasons
}

func queryProcurementProducts(db *sql.DB) ([]procurementCatalogProduct, error) {
	rows, err := db.Query(`
		SELECT pp.id,pp.sku,pp.name,COALESCE(pp.description,''),COALESCE(pp.manufacturer,''),COALESCE(pp.model,''),COALESCE(pp.unit,'Stk.'),
		       COALESCE(pc.name,''),COALESCE(pp.reorder_point,0),COALESCE(pp.target_stock,0),
		       COALESCE(best.price_cents,0),best.supplier_id,COALESCE(best.purchase_url,''),COALESCE(pp.parameters,'{}'::jsonb),COALESCE(pp.attributes,'{}'::jsonb),cpl.warehouse_product_id
		FROM proc_products pp
		LEFT JOIN proc_categories pc ON pc.id=pp.category_id
		LEFT JOIN core_product_links cpl ON cpl.procurement_product_id=pp.id
		LEFT JOIN LATERAL (SELECT o.price_cents,o.supplier_id,o.purchase_url FROM proc_offers o LEFT JOIN proc_suppliers s ON s.id=o.supplier_id WHERE o.product_id=pp.id AND o.active=TRUE ORDER BY s.preferred DESC,o.price_cents ASC LIMIT 1) best ON TRUE
		WHERE pp.active=TRUE ORDER BY pp.name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []procurementCatalogProduct{}
	for rows.Next() {
		var item procurementCatalogProduct
		var preferred, warehouseID sql.NullInt64
		var parameters, attributes []byte
		if err := rows.Scan(&item.ProductID, &item.SKU, &item.Name, &item.Description, &item.Manufacturer, &item.Model, &item.Unit, &item.Category, &item.ReorderPoint, &item.TargetStock, &item.BestPriceCents, &preferred, &item.PurchaseURL, &parameters, &attributes, &warehouseID); err != nil {
			return nil, err
		}
		if preferred.Valid {
			value := preferred.Int64
			item.PreferredOffer = &value
		}
		if warehouseID.Valid {
			value := warehouseID.Int64
			item.WarehouseID = &value
		}
		item.Parameters, item.Attributes = append([]byte(nil), parameters...), append([]byte(nil), attributes...)
		result = append(result, item)
	}
	return result, rows.Err()
}

func ListProductProcurementLinks(w http.ResponseWriter, _ *http.Request) {
	db := repository.GetSQLDB()
	procurement, err := queryProcurementProducts(db)
	if err != nil {
		respondJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "ProcurementCore-Produktstamm ist nicht verfügbar"})
		return
	}
	rows, err := db.Query(`SELECT p.productID,p.product_code,p.name,COALESCE(m.name,''),COALESCE(p.model_number,''),COALESCE(p.manufacturer_part_number,''),COALESCE(p.ean,''),COALESCE(p.stock_quantity,0),COALESCE(p.min_stock_level,0),p.tracking_mode,cpl.procurement_product_id FROM products p LEFT JOIN manufacturer m ON m.manufacturerid=p.manufacturerid LEFT JOIN core_product_links cpl ON cpl.warehouse_product_id=p.productID WHERE p.lifecycle_status='active' ORDER BY p.name`)
	if err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	defer rows.Close()
	products := []warehouseLinkProduct{}
	for rows.Next() {
		var item warehouseLinkProduct
		var procurementID sql.NullInt64
		if err := rows.Scan(&item.ProductID, &item.ProductCode, &item.Name, &item.Manufacturer, &item.Model, &item.ManufacturerSKU, &item.EAN, &item.StockQuantity, &item.MinimumStock, &item.TrackingMode, &procurementID); err != nil {
			continue
		}
		if procurementID.Valid {
			value := procurementID.Int64
			item.ProcurementID = &value
		} else {
			bestScore, bestIndex := 0, -1
			for index, candidate := range procurement {
				if candidate.WarehouseID != nil {
					continue
				}
				score, _ := scoreCatalogProduct(item, candidate)
				if score > bestScore {
					bestScore, bestIndex = score, index
				}
			}
			if bestIndex >= 0 && bestScore >= 20 {
				value := procurement[bestIndex].ProductID
				item.SuggestedID = &value
				item.SuggestionScore, item.SuggestionNotes = scoreCatalogProduct(item, procurement[bestIndex])
			}
		}
		products = append(products, item)
	}
	respondJSON(w, http.StatusOK, map[string]any{"products": products, "procurement_products": procurement})
}

func GetProcurementProductPreview(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(mux.Vars(r)["id"], 10, 64)
	if err != nil || id <= 0 {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "Ungültige Procurement-Produkt-ID"})
		return
	}
	db := repository.GetSQLDB()
	products, err := queryProcurementProducts(db)
	if err != nil {
		respondJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "ProcurementCore-Produktstamm ist nicht verfügbar"})
		return
	}
	var source *procurementCatalogProduct
	for index := range products {
		if products[index].ProductID == id {
			source = &products[index]
			break
		}
	}
	if source == nil {
		respondJSON(w, http.StatusNotFound, map[string]string{"error": "Procurement-Produkt nicht gefunden"})
		return
	}
	var linkedWarehouse sql.NullInt64
	_ = db.QueryRow("SELECT warehouse_product_id FROM core_product_links WHERE procurement_product_id=$1", id).Scan(&linkedWarehouse)
	if linkedWarehouse.Valid {
		respondJSON(w, http.StatusConflict, map[string]any{"error": "Procurement-Produkt ist bereits verknüpft", "warehouse_product_id": linkedWarehouse.Int64})
		return
	}
	attributes := rawJSONMap(source.Attributes, source.Parameters)
	ean := catalogEAN(*source)
	var manufacturerID sql.NullInt64
	if source.Manufacturer != "" {
		_ = db.QueryRow("SELECT manufacturerid FROM manufacturer WHERE LOWER(TRIM(name))=LOWER(TRIM($1)) LIMIT 1", source.Manufacturer).Scan(&manufacturerID)
	}
	var categoryID sql.NullInt64
	if source.Category != "" {
		_ = db.QueryRow("SELECT categoryid FROM categories WHERE LOWER(TRIM(name))=LOWER(TRIM($1)) LIMIT 1", source.Category).Scan(&categoryID)
	}
	productKind, productType, trackingMode := "standard", "equipment", "individual"
	identity := strings.ToLower(source.Name + " " + source.Category)
	if strings.Contains(identity, "kabel") || strings.Contains(identity, "adapter") {
		productKind = "cable"
		_ = db.QueryRow("SELECT categoryid FROM categories WHERE LOWER(TRIM(name))='kabel & adapter' LIMIT 1").Scan(&categoryID)
	}
	if strings.Contains(identity, "verbrauch") || strings.Contains(identity, "batterie") || strings.Contains(identity, "tape") || strings.Contains(identity, "klebe") {
		productKind, productType, trackingMode = "consumable", "consumable", "quantity"
	}
	var countTypeID sql.NullInt64
	_ = db.QueryRow("SELECT count_type_id FROM count_types WHERE LOWER(TRIM(name))=LOWER(TRIM($1)) OR LOWER(TRIM(abbreviation))=LOWER(TRIM($1)) LIMIT 1", source.Unit).Scan(&countTypeID)
	result := map[string]any{"procurement_product_id": source.ProductID, "procurement_sku": source.SKU, "source_name": source.Name, "name": source.Name, "description": source.Description, "model_number": source.Model, "manufacturer_part_number": source.SKU, "ean": ean, "attributes": attributes, "product_kind": productKind, "product_type": productType, "tracking_mode": trackingMode, "min_stock_level": source.ReorderPoint, "price_per_unit": float64(source.BestPriceCents) / 100}
	if manufacturerID.Valid {
		result["manufacturer_id"] = manufacturerID.Int64
		result["manufacturer_name"] = source.Manufacturer
	} else if source.Manufacturer != "" {
		result["manufacturer_suggestion"] = source.Manufacturer
	}
	if source.Manufacturer != "" {
		result["manufacturer_name_input"] = source.Manufacturer
	}
	if categoryID.Valid {
		result["category_id"] = categoryID.Int64
	}
	if countTypeID.Valid {
		result["count_type_id"] = countTypeID.Int64
	}
	respondJSON(w, http.StatusOK, result)
}

func LinkProductToProcurement(w http.ResponseWriter, r *http.Request) {
	warehouseID, err := strconv.ParseInt(mux.Vars(r)["id"], 10, 64)
	if err != nil || warehouseID <= 0 {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "Ungültige Produkt-ID"})
		return
	}
	var input struct {
		ProcurementProductID int64 `json:"procurement_product_id"`
	}
	if json.NewDecoder(r.Body).Decode(&input) != nil || input.ProcurementProductID <= 0 {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "Procurement-Produkt fehlt"})
		return
	}
	db := repository.GetSQLDB()
	var warehouseExists, procurementExists bool
	if db.QueryRow("SELECT EXISTS(SELECT 1 FROM products WHERE productID=$1 AND lifecycle_status='active')", warehouseID).Scan(&warehouseExists) != nil || db.QueryRow("SELECT EXISTS(SELECT 1 FROM proc_products WHERE id=$1 AND active=TRUE)", input.ProcurementProductID).Scan(&procurementExists) != nil || !warehouseExists || !procurementExists {
		respondJSON(w, http.StatusNotFound, map[string]string{"error": "Produkt nicht gefunden oder archiviert"})
		return
	}
	linkedBy, linkedName := int64(0), ""
	if user, ok := middleware.GetUserFromContext(r); ok && user != nil {
		linkedBy = int64(user.UserID)
		linkedName = user.Username
	}
	var linkID int64
	err = db.QueryRow(`INSERT INTO core_product_links(procurement_product_id,warehouse_product_id,link_method,linked_by,linked_by_name) VALUES($1,$2,'manual',$3,$4) RETURNING id`, input.ProcurementProductID, warehouseID, linkedBy, linkedName).Scan(&linkID)
	if err != nil {
		respondJSON(w, http.StatusConflict, map[string]string{"error": "Eines der Produkte ist bereits anderweitig verknüpft"})
		return
	}
	respondJSON(w, http.StatusCreated, map[string]any{"link_id": linkID, "procurement_product_id": input.ProcurementProductID, "warehouse_product_id": warehouseID})
}

func UnlinkProductFromProcurement(w http.ResponseWriter, r *http.Request) {
	warehouseID, err := strconv.ParseInt(mux.Vars(r)["id"], 10, 64)
	if err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "Ungültige Produkt-ID"})
		return
	}
	result, err := repository.GetSQLDB().Exec("DELETE FROM core_product_links WHERE warehouse_product_id=$1", warehouseID)
	if err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	count, _ := result.RowsAffected()
	if count == 0 {
		respondJSON(w, http.StatusNotFound, map[string]string{"error": "Keine Verknüpfung vorhanden"})
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func CreateProductRequisition(w http.ResponseWriter, r *http.Request) {
	warehouseID, err := strconv.ParseInt(mux.Vars(r)["id"], 10, 64)
	if err != nil || warehouseID <= 0 {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "Ungültige Produkt-ID"})
		return
	}
	var input struct {
		Quantity      float64 `json:"quantity"`
		NeededBy      *string `json:"needed_by"`
		CostCenter    string  `json:"cost_center"`
		Justification string  `json:"justification"`
	}
	if json.NewDecoder(r.Body).Decode(&input) != nil || input.Quantity <= 0 || math.IsNaN(input.Quantity) || math.IsInf(input.Quantity, 0) {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "Eine positive Bedarfsmenge ist erforderlich"})
		return
	}
	db := repository.GetSQLDB()
	var procurementID int64
	var productName, unit string
	var price int64
	var supplier sql.NullInt64
	var purchaseURL string
	err = db.QueryRow(`SELECT pp.id,pp.name,COALESCE(pp.unit,'Stk.'),COALESCE(best.price_cents,0),best.supplier_id,COALESCE(best.purchase_url,'') FROM core_product_links cpl JOIN proc_products pp ON pp.id=cpl.procurement_product_id LEFT JOIN LATERAL (SELECT o.price_cents,o.supplier_id,o.purchase_url FROM proc_offers o LEFT JOIN proc_suppliers s ON s.id=o.supplier_id WHERE o.product_id=pp.id AND o.active=TRUE ORDER BY s.preferred DESC,o.price_cents ASC LIMIT 1) best ON TRUE WHERE cpl.warehouse_product_id=$1 AND pp.active=TRUE`, warehouseID).Scan(&procurementID, &productName, &unit, &price, &supplier, &purchaseURL)
	if err == sql.ErrNoRows {
		respondJSON(w, http.StatusConflict, map[string]string{"error": "Produkt muss zuerst mit ProcurementCore verknüpft werden"})
		return
	}
	if err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	var neededBy any
	if input.NeededBy != nil && strings.TrimSpace(*input.NeededBy) != "" {
		parsed, parseErr := time.Parse("2006-01-02", strings.TrimSpace(*input.NeededBy))
		if parseErr != nil {
			respondJSON(w, http.StatusBadRequest, map[string]string{"error": "Ungültiges Bedarfsdatum"})
			return
		}
		neededBy = parsed
	}
	userID, userName := int64(0), "WarehouseCore"
	if user, ok := middleware.GetUserFromContext(r); ok && user != nil {
		userID = int64(user.UserID)
		userName = user.Username
	}
	tx, err := db.Begin()
	if err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Bedarf konnte nicht angelegt werden"})
		return
	}
	defer tx.Rollback()
	number := fmt.Sprintf("BAN-%s-%04d", time.Now().Format("20060102-150405"), time.Now().Nanosecond()%10000)
	total := int64(math.Round(float64(price) * input.Quantity))
	title := "Bedarf: " + productName
	justification := strings.TrimSpace(input.Justification)
	if justification == "" {
		justification = "Aus WarehouseCore gemeldeter Lagerbedarf"
	}
	var requisitionID int64
	err = tx.QueryRow(`INSERT INTO proc_requisitions(number,title,status,requester_id,requester_name,cost_center,justification,needed_by,estimated_total_cents,created_at,updated_at) VALUES($1,$2,'draft',$3,$4,$5,$6,$7,$8,CURRENT_TIMESTAMP,CURRENT_TIMESTAMP) RETURNING id`, number, title, userID, userName, strings.TrimSpace(input.CostCenter), justification, neededBy, total).Scan(&requisitionID)
	if err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Bedarfskopf konnte nicht angelegt werden"})
		return
	}
	var supplierValue any
	if supplier.Valid {
		supplierValue = supplier.Int64
	}
	if _, err = tx.Exec(`INSERT INTO proc_requisition_lines(requisition_id,product_id,description,quantity,unit,estimated_price_cents,preferred_supplier_id,purchase_url) VALUES($1,$2,$3,$4,$5,$6,$7,$8)`, requisitionID, procurementID, productName, input.Quantity, unit, price, supplierValue, purchaseURL); err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Bedarfsposition konnte nicht angelegt werden"})
		return
	}
	_, _ = tx.Exec(`INSERT INTO proc_activities(entity_type,entity_id,action,user_id,username,details,created_at) VALUES('requisition',$1,'created_from_warehouse',$2,$3,$4,CURRENT_TIMESTAMP)`, requisitionID, userID, userName, fmt.Sprintf("warehouse_product=%d", warehouseID))
	if err = tx.Commit(); err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Bedarf konnte nicht gespeichert werden"})
		return
	}
	respondJSON(w, http.StatusCreated, map[string]any{"requisition_id": requisitionID, "number": number, "status": "draft"})
}
