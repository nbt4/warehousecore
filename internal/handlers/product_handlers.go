package handlers

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/mux"
	"github.com/pkg/errors"

	"warehousecore/internal/middleware"
	"warehousecore/internal/models"
	"warehousecore/internal/repository"
	"warehousecore/internal/services"
)

var productPictureService = services.NewProductPictureServiceFromEnv()
var errPicturesUnavailable = errors.New("product pictures not available")
var websiteRevalidator = services.NewRevalidatorFromEnv()

func warehouseProductSearchTerms(value string) []string {
	return strings.Fields(strings.ToLower(strings.TrimSpace(value)))
}

// Product represents a product (item type)
type Product struct {
	ProductID           int             `json:"product_id"`
	Name                string          `json:"name"`
	CategoryID          *int            `json:"category_id"`
	SubcategoryID       *string         `json:"subcategory_id"`
	SubbiercategoryID   *string         `json:"subbiercategory_id"`
	ManufacturerID      *int            `json:"manufacturer_id"`
	BrandID             *int            `json:"brand_id"`
	Description         *string         `json:"description"`
	MaintenanceInterval *int            `json:"maintenance_interval"`
	ItemCostPerDay      *float64        `json:"item_cost_per_day"`
	Weight              *float64        `json:"weight"`
	Height              *float64        `json:"height"`
	Width               *float64        `json:"width"`
	Depth               *float64        `json:"depth"`
	PowerConsumption    *float64        `json:"power_consumption"`
	PosInCategory       *int            `json:"pos_in_category"`
	IsAccessory         bool            `json:"is_accessory"`
	IsConsumable        bool            `json:"is_consumable"`
	CountTypeID         *int            `json:"count_type_id"`
	StockQuantity       *float64        `json:"stock_quantity"`
	MinStockLevel       *float64        `json:"min_stock_level"`
	GenericBarcode      *string         `json:"generic_barcode"`
	PricePerUnit        *float64        `json:"price_per_unit"`
	ProductType         string          `json:"product_type"`
	TrackingMode        string          `json:"tracking_mode"`
	LifecycleStatus     string          `json:"lifecycle_status"`
	ProductCode         string          `json:"product_code"`
	ProductKind         string          `json:"product_kind"`
	ModelNumber         *string         `json:"model_number"`
	ManufacturerPartNo  *string         `json:"manufacturer_part_number"`
	EAN                 *string         `json:"ean"`
	Attributes          json.RawMessage `json:"attributes,omitempty"`
	InitialDeviceQty    int             `json:"initial_device_quantity,omitempty"`
	InitialZoneID       *int            `json:"initial_zone_id,omitempty"`
	CreatedDeviceIDs    []string        `json:"created_device_ids,omitempty"`

	// Joined fields for display
	WebsiteVisible   bool     `json:"website_visible"`
	WebsiteImages    []string `json:"website_images,omitempty"`
	WebsiteThumbnail *string  `json:"website_thumbnail,omitempty"`

	CategoryName        *string `json:"category_name,omitempty"`
	SubcategoryName     *string `json:"subcategory_name,omitempty"`
	SubbiercategoryName *string `json:"subbiercategory_name,omitempty"`
	BrandName           *string `json:"brand_name,omitempty"`
	ManufacturerName    *string `json:"manufacturer_name,omitempty"`
	CountTypeName       *string `json:"count_type_name,omitempty"`
	CountTypeAbbr       *string `json:"count_type_abbr,omitempty"`
	DeviceCount         int     `json:"device_count"`
}

// DeviceCreateRequest represents a request to create devices
type DeviceCreateRequest struct {
	ProductID      int     `json:"product_id"`
	Quantity       int     `json:"quantity"`
	StartingNumber *int    `json:"starting_number"` // Optional, if not provided, auto-generate
	Prefix         *string `json:"prefix"`          // Optional device ID prefix
}

// DeviceCreateResponse represents the response after creating devices
type DeviceCreateResponse struct {
	CreatedCount int      `json:"created_count"`
	DeviceIDs    []string `json:"device_ids"`
}

// GetProducts retrieves all products with optional filtering
func GetProducts(w http.ResponseWriter, r *http.Request) {
	search := r.URL.Query().Get("search")
	categoryID := r.URL.Query().Get("category_id")
	subcategoryID := r.URL.Query().Get("subcategory_id")
	lifecycleStatus := strings.TrimSpace(r.URL.Query().Get("lifecycle_status"))

	db := repository.GetSQLDB()

	query := `
		SELECT
			p.productID,
			p.name,
			p.categoryID,
			p.subcategoryID,
			p.subbiercategoryID,
			p.manufacturerid,
			p.brandid,
			p.description,
			p.maintenanceinterval,
			p.itemcostperday,
			p.weight,
			p.height,
			p.width,
			p.depth,
			p.powerconsumption,
			p.pos_in_category,
			COALESCE(p.is_accessory, false) as is_accessory,
			COALESCE(p.is_consumable, false) as is_consumable,
			p.count_type_id,
			p.stock_quantity,
			p.min_stock_level,
			p.generic_barcode,
			p.price_per_unit,
			p.product_type,
			p.tracking_mode,
			p.lifecycle_status,
			p.product_code,
			p.product_kind,
			p.model_number,
			p.manufacturer_part_number,
			p.ean,
			p.attributes,
			COALESCE(p.website_visible, false) as website_visible,
			p.website_thumbnail,
			p.website_images_json,
			c.name as category_name,
			sc.name as subcategory_name,
			sbc.name as subbiercategory_name,
			b.name as brand_name,
			m.name as manufacturer_name,
			ct.name as count_type_name,
			ct.abbreviation as count_type_abbr,
			(SELECT COUNT(*) FROM devices WHERE productID = p.productID) AS device_count
		FROM products p
		LEFT JOIN categories c ON p.categoryID = c.categoryID
		LEFT JOIN subcategories sc ON p.subcategoryID = sc.subcategoryID
		LEFT JOIN subbiercategories sbc ON p.subbiercategoryID = sbc.subbiercategoryID
		LEFT JOIN brands b ON p.brandid = b.brandid
		LEFT JOIN manufacturer m ON p.manufacturerid = m.manufacturerid
		LEFT JOIN count_types ct ON p.count_type_id = ct.count_type_id
		WHERE NOT EXISTS (
			SELECT 1
			FROM product_packages pp
			WHERE LOWER(TRIM(pp.name)) = LOWER(TRIM(p.name))
			  AND NOT EXISTS (
				SELECT 1 FROM product_package_items ppi WHERE ppi.product_id = p.productID
			  )
		)
	`
	if lifecycleStatus == "" {
		lifecycleStatus = "active"
	}

	var args []interface{}
	argIdx := 0
	if lifecycleStatus != "all" {
		if lifecycleStatus != "active" && lifecycleStatus != "archived" {
			respondJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid lifecycle status"})
			return
		}
		argIdx++
		query += fmt.Sprintf(" AND p.lifecycle_status = $%d", argIdx)
		args = append(args, lifecycleStatus)
	}

	if search != "" {
		for _, term := range warehouseProductSearchTerms(search) {
			argIdx++
			query += fmt.Sprintf(` AND (
				CONCAT_WS(' ',p.productID::text,p.name,p.description,b.name,m.name,c.name,sc.name,sbc.name,
				 p.generic_barcode,p.product_type,p.tracking_mode,ct.name,ct.abbreviation,
				 p.weight::text,p.height::text,p.width::text,p.depth::text,p.powerconsumption::text) ILIKE $%d
				OR EXISTS (SELECT 1 FROM devices search_device WHERE search_device.productID=p.productID
				 AND CONCAT_WS(' ',search_device.deviceID,search_device.serialnumber,search_device.barcode,
				 search_device.qr_code,search_device.notes) ILIKE $%d)
			)`, argIdx, argIdx)
			args = append(args, "%"+term+"%")
		}
	}

	if categoryID != "" {
		argIdx++
		query += fmt.Sprintf(" AND p.categoryID = $%d", argIdx)
		args = append(args, categoryID)
	}

	if subcategoryID != "" {
		argIdx++
		query += fmt.Sprintf(" AND p.subcategoryID = $%d", argIdx)
		args = append(args, subcategoryID)
	}

	query += " ORDER BY p.name"

	rows, err := db.Query(query, args...)
	if err != nil {
		log.Printf("Failed to query products: %v", err)
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to fetch products"})
		return
	}
	defer rows.Close()

	var products []Product
	for rows.Next() {
		var p Product
		var rawImages sql.NullString
		var rawAttributes []byte
		err := rows.Scan(
			&p.ProductID,
			&p.Name,
			&p.CategoryID,
			&p.SubcategoryID,
			&p.SubbiercategoryID,
			&p.ManufacturerID,
			&p.BrandID,
			&p.Description,
			&p.MaintenanceInterval,
			&p.ItemCostPerDay,
			&p.Weight,
			&p.Height,
			&p.Width,
			&p.Depth,
			&p.PowerConsumption,
			&p.PosInCategory,
			&p.IsAccessory,
			&p.IsConsumable,
			&p.CountTypeID,
			&p.StockQuantity,
			&p.MinStockLevel,
			&p.GenericBarcode,
			&p.PricePerUnit,
			&p.ProductType,
			&p.TrackingMode,
			&p.LifecycleStatus,
			&p.ProductCode,
			&p.ProductKind,
			&p.ModelNumber,
			&p.ManufacturerPartNo,
			&p.EAN,
			&rawAttributes,
			&p.WebsiteVisible,
			&p.WebsiteThumbnail,
			&rawImages,
			&p.CategoryName,
			&p.SubcategoryName,
			&p.SubbiercategoryName,
			&p.BrandName,
			&p.ManufacturerName,
			&p.CountTypeName,
			&p.CountTypeAbbr,
			&p.DeviceCount,
		)
		if err != nil {
			log.Printf("Failed to scan product: %v", err)
			continue
		}
		if rawImages.Valid && rawImages.String != "" {
			_ = json.Unmarshal([]byte(rawImages.String), &p.WebsiteImages)
		}
		p.Attributes = append(json.RawMessage(nil), rawAttributes...)
		products = append(products, p)
	}

	respondJSON(w, http.StatusOK, products)
}

// GetProduct retrieves a single product by ID
func GetProduct(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id, err := strconv.Atoi(vars["id"])
	if err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid product ID"})
		return
	}

	db := repository.GetSQLDB()
	query := `
		SELECT
			p.productID,
			p.name,
			p.categoryID,
			p.subcategoryID,
			p.subbiercategoryID,
			p.manufacturerid,
			p.brandid,
			p.description,
			p.maintenanceinterval,
			p.itemcostperday,
			p.weight,
			p.height,
			p.width,
			p.depth,
			p.powerconsumption,
			p.pos_in_category,
			COALESCE(p.is_accessory, false) as is_accessory,
			COALESCE(p.is_consumable, false) as is_consumable,
			p.count_type_id,
			p.stock_quantity,
			p.min_stock_level,
			p.generic_barcode,
			p.price_per_unit,
			p.product_type,
			p.tracking_mode,
			p.lifecycle_status,
			p.product_code,
			p.product_kind,
			p.model_number,
			p.manufacturer_part_number,
			p.ean,
			p.attributes,
			COALESCE(p.website_visible, false) as website_visible,
			p.website_thumbnail,
			p.website_images_json,
			c.name as category_name,
			sc.name as subcategory_name,
			sbc.name as subbiercategory_name,
			b.name as brand_name,
			m.name as manufacturer_name,
			ct.name as count_type_name,
			ct.abbreviation as count_type_abbr,
			(SELECT COUNT(*) FROM devices WHERE productID = p.productID) AS device_count
		FROM products p
		LEFT JOIN categories c ON p.categoryID = c.categoryID
		LEFT JOIN subcategories sc ON p.subcategoryID = sc.subcategoryID
		LEFT JOIN subbiercategories sbc ON p.subbiercategoryID = sbc.subbiercategoryID
		LEFT JOIN brands b ON p.brandid = b.brandid
		LEFT JOIN manufacturer m ON p.manufacturerid = m.manufacturerid
		LEFT JOIN count_types ct ON p.count_type_id = ct.count_type_id
		WHERE p.productID = $1
	`

	var p Product
	var rawImages sql.NullString
	var rawAttributes []byte
	err = db.QueryRow(query, id).Scan(
		&p.ProductID,
		&p.Name,
		&p.CategoryID,
		&p.SubcategoryID,
		&p.SubbiercategoryID,
		&p.ManufacturerID,
		&p.BrandID,
		&p.Description,
		&p.MaintenanceInterval,
		&p.ItemCostPerDay,
		&p.Weight,
		&p.Height,
		&p.Width,
		&p.Depth,
		&p.PowerConsumption,
		&p.PosInCategory,
		&p.IsAccessory,
		&p.IsConsumable,
		&p.CountTypeID,
		&p.StockQuantity,
		&p.MinStockLevel,
		&p.GenericBarcode,
		&p.PricePerUnit,
		&p.ProductType,
		&p.TrackingMode,
		&p.LifecycleStatus,
		&p.ProductCode,
		&p.ProductKind,
		&p.ModelNumber,
		&p.ManufacturerPartNo,
		&p.EAN,
		&rawAttributes,
		&p.WebsiteVisible,
		&p.WebsiteThumbnail,
		&rawImages,
		&p.CategoryName,
		&p.SubcategoryName,
		&p.SubbiercategoryName,
		&p.BrandName,
		&p.ManufacturerName,
		&p.CountTypeName,
		&p.CountTypeAbbr,
		&p.DeviceCount,
	)

	if err == sql.ErrNoRows {
		respondJSON(w, http.StatusNotFound, map[string]string{"error": "Product not found"})
		return
	} else if err != nil {
		log.Printf("Failed to query product: %v", err)
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to fetch product"})
		return
	}
	if rawImages.Valid && rawImages.String != "" {
		_ = json.Unmarshal([]byte(rawImages.String), &p.WebsiteImages)
	}
	p.Attributes = append(json.RawMessage(nil), rawAttributes...)

	respondJSON(w, http.StatusOK, p)
}

// GetProductPictures lists all stored pictures for a product.
func GetProductPictures(w http.ResponseWriter, r *http.Request) {
	if !productPictureService.Enabled() {
		respondJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "Product pictures are not configured"})
		return
	}

	vars := mux.Vars(r)
	id, err := strconv.Atoi(vars["id"])
	if err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid product ID"})
		return
	}

	productName, err := getProductName(id)
	if err == sql.ErrNoRows {
		respondJSON(w, http.StatusNotFound, map[string]string{"error": "Product not found"})
		return
	} else if err != nil {
		log.Printf("[PICTURES] Failed to resolve product name: %v", err)
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to load product"})
		return
	}

	items, err := productPictureService.ListPictures(productName)
	if err != nil {
		log.Printf("[PICTURES] List failed for product %d: %v", id, err)
		if strings.Contains(strings.ToLower(err.Error()), "404") || strings.Contains(strings.ToLower(err.Error()), "not found") {
			respondJSON(w, http.StatusOK, map[string]interface{}{"pictures": []interface{}{}})
			return
		}
		respondJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "Failed to list pictures"})
		return
	}

	// Sort newest first
	sort.Slice(items, func(i, j int) bool {
		return items[i].ModifiedAt.After(items[j].ModifiedAt)
	})

	type pictureResponse struct {
		FileName    string    `json:"file_name"`
		Size        int64     `json:"size"`
		ContentType string    `json:"content_type"`
		ModifiedAt  time.Time `json:"modified_at"`
		DownloadURL string    `json:"download_url"`
		Thumbnail   string    `json:"thumbnail_url"`
		PreviewURL  string    `json:"preview_url"`
	}

	resp := make([]pictureResponse, 0, len(items))
	for _, pic := range items {
		resp = append(resp, pictureResponse{
			FileName:    pic.FileName,
			Size:        pic.Size,
			ContentType: pic.ContentType,
			ModifiedAt:  pic.ModifiedAt,
			DownloadURL: fmt.Sprintf("/api/v1/admin/products/%d/pictures/%s", id, url.PathEscape(pic.FileName)),
			Thumbnail:   fmt.Sprintf("/api/v1/admin/products/%d/pictures/%s?variant=thumb", id, url.PathEscape(pic.FileName)),
			PreviewURL:  fmt.Sprintf("/api/v1/admin/products/%d/pictures/%s?variant=preview", id, url.PathEscape(pic.FileName)),
		})
	}

	respondJSON(w, http.StatusOK, map[string]interface{}{
		"pictures": resp,
	})
}

// DeleteProductPicture deletes a stored image for a product.
func DeleteProductPicture(w http.ResponseWriter, r *http.Request) {
	if !productPictureService.Enabled() {
		respondJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "Product pictures are not configured"})
		return
	}

	vars := mux.Vars(r)
	id, err := strconv.Atoi(vars["id"])
	if err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid product ID"})
		return
	}

	filename, err := url.PathUnescape(vars["filename"])
	if err != nil || filename == "" {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid filename"})
		return
	}

	productName, err := getProductName(id)
	if err == sql.ErrNoRows {
		respondJSON(w, http.StatusNotFound, map[string]string{"error": "Product not found"})
		return
	} else if err != nil {
		log.Printf("[PICTURES] Failed to resolve product name: %v", err)
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to load product"})
		return
	}

	if err := productPictureService.DeletePicture(productName, filename); err != nil {
		log.Printf("[PICTURES] Delete failed for product %d (%s): %v", id, filename, err)
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to delete picture"})
		return
	}
	productPictureService.ClearCachedVariants(productName, filename)
	websiteRevalidator.Revalidate("/products")

	respondJSON(w, http.StatusOK, map[string]string{"message": "Picture deleted"})
}

// UploadProductPictures stores one or more images for a product in Nextcloud.
func UploadProductPictures(w http.ResponseWriter, r *http.Request) {
	if !productPictureService.Enabled() {
		respondJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "Product pictures are not configured"})
		return
	}

	vars := mux.Vars(r)
	id, err := strconv.Atoi(vars["id"])
	if err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid product ID"})
		return
	}

	productName, err := getProductName(id)
	if err == sql.ErrNoRows {
		respondJSON(w, http.StatusNotFound, map[string]string{"error": "Product not found"})
		return
	} else if err != nil {
		log.Printf("[PICTURES] Failed to resolve product name: %v", err)
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to load product"})
		return
	}

	if err := r.ParseMultipartForm(productPictureService.MaxFileSize() * 4); err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid multipart form: " + err.Error()})
		return
	}

	files := r.MultipartForm.File["files"]
	if len(files) == 0 {
		if singleFile, singleHeader, err := r.FormFile("file"); err == nil {
			singleFile.Close()
			files = append(files, singleHeader)
		}
	}
	if len(files) == 0 {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "No files provided"})
		return
	}

	uploaded := make([]string, 0, len(files))

	for _, header := range files {
		src, err := header.Open()
		if err != nil {
			log.Printf("[PICTURES] Failed to open upload %s: %v", header.Filename, err)
			respondJSON(w, http.StatusBadRequest, map[string]string{"error": "Failed to read uploaded file"})
			return
		}

		stored, err := productPictureService.UploadPicture(productName, src, header)
		src.Close()
		if err != nil {
			log.Printf("[PICTURES] Upload failed for product %d: %v", id, err)
			respondJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		uploaded = append(uploaded, stored)
		productPictureService.WarmPictureVariants(productName, stored)
	}

	// Update product's website images in database
	db := repository.GetSQLDB()

	// Get thumbnail index if provided
	thumbnailIndexStr := r.FormValue("thumbnail_index")
	var thumbnailFilename string
	if thumbnailIndexStr != "" {
		if thumbnailIdx, err := strconv.Atoi(thumbnailIndexStr); err == nil && thumbnailIdx >= 0 && thumbnailIdx < len(uploaded) {
			thumbnailFilename = uploaded[thumbnailIdx]
		}
	}

	// If no thumbnail specified but images were uploaded, use the first one
	if thumbnailFilename == "" && len(uploaded) > 0 {
		thumbnailFilename = uploaded[0]
	}

	// Convert uploaded filenames to JSON array for website_images_json
	imagesJSON, err := json.Marshal(uploaded)
	if err != nil {
		log.Printf("[PICTURES] Failed to marshal images JSON: %v", err)
	} else {
		_, err = db.Exec(`
			UPDATE products
			SET website_thumbnail = $1, website_images_json = $2
			WHERE productID = $3
		`, thumbnailFilename, imagesJSON, id)
		if err != nil {
			log.Printf("[PICTURES] Failed to update product images in database: %v", err)
		}
	}
	websiteRevalidator.Revalidate("/products")

	respondJSON(w, http.StatusCreated, map[string]interface{}{
		"message":          "Pictures uploaded successfully",
		"uploaded_files":   uploaded,
		"uploaded_count":   len(uploaded),
		"product_name":     productName,
		"thumbnail":        thumbnailFilename,
		"nextcloud_folder": productPictureService.FolderForProduct(productName),
	})
}

// DownloadProductPicture streams a product picture from Nextcloud.
func DownloadProductPicture(w http.ResponseWriter, r *http.Request) {
	if !productPictureService.Enabled() {
		respondJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "Product pictures are not configured"})
		return
	}

	vars := mux.Vars(r)
	id, err := strconv.Atoi(vars["id"])
	if err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid product ID"})
		return
	}

	filename, err := url.PathUnescape(vars["filename"])
	if err != nil || filename == "" {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid filename"})
		return
	}

	productName, err := getProductName(id)
	if err == sql.ErrNoRows {
		respondJSON(w, http.StatusNotFound, map[string]string{"error": "Product not found"})
		return
	} else if err != nil {
		log.Printf("[PICTURES] Failed to resolve product name: %v", err)
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to load product"})
		return
	}

	variant := strings.TrimSpace(r.URL.Query().Get("variant"))
	format := strings.TrimSpace(r.URL.Query().Get("format"))

	reader, contentType, err := productPictureService.DownloadPictureWithVariant(productName, filename, variant, format)
	if err != nil {
		log.Printf("[PICTURES] Download failed for product %d (%s): %v", id, filename, err)
		status := http.StatusNotFound
		if strings.Contains(err.Error(), "upload") || strings.Contains(err.Error(), "list") {
			status = http.StatusServiceUnavailable
		}
		respondJSON(w, status, map[string]string{"error": "File not found or storage unavailable"})
		return
	}
	defer reader.Close()

	if contentType != "" {
		w.Header().Set("Content-Type", contentType)
	} else {
		w.Header().Set("Content-Type", "application/octet-stream")
	}
	w.Header().Set("Cache-Control", "public, max-age=86400")
	w.Header().Set("Content-Disposition", fmt.Sprintf("inline; filename=\"%s\"", url.PathEscape(filename)))

	if _, err := io.Copy(w, reader); err != nil {
		log.Printf("[PICTURES] Failed to stream %s: %v", filename, err)
	}
}

func getProductName(productID int) (string, error) {
	db := repository.GetSQLDB()
	var name string
	err := db.QueryRow("SELECT name FROM products WHERE productID = $1", productID).Scan(&name)
	return name, err
}

func normalizeProductRequest(req *Product) error {
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		return fmt.Errorf("Product name is required")
	}
	if len(req.Name) > 255 {
		return fmt.Errorf("Product name must be at most 255 characters")
	}

	if req.ProductType == "" {
		switch {
		case req.IsConsumable:
			req.ProductType = "consumable"
		case req.IsAccessory:
			req.ProductType = "accessory"
		default:
			req.ProductType = "equipment"
		}
	}
	switch req.ProductType {
	case "equipment":
		req.IsAccessory = false
		req.IsConsumable = false
	case "accessory":
		req.IsAccessory = true
		req.IsConsumable = false
	case "consumable":
		req.IsAccessory = false
		req.IsConsumable = true
	default:
		return fmt.Errorf("Invalid product type")
	}

	if req.TrackingMode == "" {
		if req.ProductType == "equipment" {
			req.TrackingMode = "individual"
		} else {
			req.TrackingMode = "quantity"
		}
	}
	switch req.TrackingMode {
	case "individual", "quantity", "none":
	default:
		return fmt.Errorf("Invalid tracking mode")
	}
	if req.ProductType == "consumable" && req.TrackingMode != "quantity" {
		return fmt.Errorf("Consumables must use quantity tracking")
	}
	if req.ProductType == "equipment" && req.TrackingMode == "quantity" {
		return fmt.Errorf("Equipment must use individual or no tracking")
	}
	if req.TrackingMode == "quantity" && req.CountTypeID == nil {
		return fmt.Errorf("A measurement unit is required for quantity-tracked products")
	}
	if req.ProductKind == "" {
		if req.ProductType == "consumable" {
			req.ProductKind = "consumable"
		} else {
			req.ProductKind = "standard"
		}
	}
	validKinds := map[string]bool{"standard": true, "cable": true, "consumable": true, "container": true, "service": true}
	if !validKinds[req.ProductKind] {
		return fmt.Errorf("Invalid product class")
	}
	if req.InitialDeviceQty < 0 || req.InitialDeviceQty > 1000 {
		return fmt.Errorf("Initial device quantity must be between 0 and 1000")
	}
	if req.InitialDeviceQty > 0 && req.TrackingMode != "individual" {
		return fmt.Errorf("Only individually tracked products can receive devices")
	}
	if len(req.Attributes) > 0 && !json.Valid(req.Attributes) {
		return fmt.Errorf("Invalid product attributes")
	}

	if req.Description != nil {
		value := strings.TrimSpace(*req.Description)
		if value == "" {
			req.Description = nil
		} else {
			req.Description = &value
		}
	}
	if req.GenericBarcode != nil {
		value := strings.TrimSpace(*req.GenericBarcode)
		if value == "" {
			req.GenericBarcode = nil
		} else {
			req.GenericBarcode = &value
		}
	}

	for label, value := range map[string]*float64{
		"Daily price": req.ItemCostPerDay, "Weight": req.Weight, "Height": req.Height,
		"Width": req.Width, "Depth": req.Depth, "Power consumption": req.PowerConsumption,
		"Stock quantity": req.StockQuantity, "Minimum stock level": req.MinStockLevel,
		"Unit price": req.PricePerUnit,
	} {
		if value != nil && *value < 0 {
			return fmt.Errorf("%s cannot be negative", label)
		}
	}
	if req.MaintenanceInterval != nil && *req.MaintenanceInterval < 0 {
		return fmt.Errorf("Maintenance interval cannot be negative")
	}
	if req.PosInCategory != nil && *req.PosInCategory < 1 {
		return fmt.Errorf("Category position must be at least 1")
	}
	return nil
}

func validateProductRelations(db *sql.DB, req *Product, productID int) error {
	var exists bool
	if err := db.QueryRow(`SELECT EXISTS (
		SELECT 1 FROM products WHERE LOWER(TRIM(name)) = LOWER(TRIM($1)) AND productid <> $2
	)`, req.Name, productID).Scan(&exists); err != nil {
		return err
	}
	if exists {
		return fmt.Errorf("A product with this name already exists")
	}
	if req.GenericBarcode != nil {
		if err := db.QueryRow(`SELECT EXISTS (
			SELECT 1 FROM products WHERE LOWER(TRIM(generic_barcode)) = LOWER(TRIM($1)) AND productid <> $2
		)`, *req.GenericBarcode, productID).Scan(&exists); err != nil {
			return err
		}
		if exists {
			return fmt.Errorf("This barcode is already assigned to another product")
		}
	}

	if req.SubcategoryID != nil {
		var categoryID int
		if err := db.QueryRow("SELECT categoryid FROM subcategories WHERE subcategoryid = $1", *req.SubcategoryID).Scan(&categoryID); err != nil {
			if err == sql.ErrNoRows {
				return fmt.Errorf("Selected subcategory does not exist")
			}
			return err
		}
		if req.CategoryID == nil || *req.CategoryID != categoryID {
			return fmt.Errorf("Selected subcategory does not belong to the category")
		}
	}
	if req.SubbiercategoryID != nil {
		var subcategoryID string
		if err := db.QueryRow("SELECT subcategoryid FROM subbiercategories WHERE subbiercategoryid = $1", *req.SubbiercategoryID).Scan(&subcategoryID); err != nil {
			if err == sql.ErrNoRows {
				return fmt.Errorf("Selected third-level category does not exist")
			}
			return err
		}
		if req.SubcategoryID == nil || *req.SubcategoryID != subcategoryID {
			return fmt.Errorf("Selected third-level category does not belong to the subcategory")
		}
	}
	if req.CountTypeID != nil {
		if err := db.QueryRow("SELECT EXISTS(SELECT 1 FROM count_types WHERE count_type_id = $1)", *req.CountTypeID).Scan(&exists); err != nil {
			return err
		}
		if !exists {
			return fmt.Errorf("Selected measurement unit does not exist")
		}
	}
	if req.BrandID != nil {
		var brandManufacturer sql.NullInt64
		if err := db.QueryRow("SELECT manufacturerid FROM brands WHERE brandid = $1", *req.BrandID).Scan(&brandManufacturer); err != nil {
			if err == sql.ErrNoRows {
				return fmt.Errorf("Selected brand does not exist")
			}
			return err
		}
		if brandManufacturer.Valid {
			if req.ManufacturerID == nil {
				value := int(brandManufacturer.Int64)
				req.ManufacturerID = &value
			} else if *req.ManufacturerID != int(brandManufacturer.Int64) {
				return fmt.Errorf("Selected brand does not belong to the manufacturer")
			}
		}
	}
	return nil
}

func recordProductAudit(tx *sql.Tx, r *http.Request, action string, productID int, oldValues, newValues interface{}) error {
	var userID interface{}
	if user, ok := middleware.GetUserFromContext(r); ok {
		userID = user.UserID
	}
	oldJSON, _ := json.Marshal(oldValues)
	newJSON, _ := json.Marshal(newValues)
	ipAddress := r.RemoteAddr
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		ipAddress = host
	}
	if len(ipAddress) > 45 {
		ipAddress = ipAddress[:45]
	}
	_, err := tx.Exec(`
		INSERT INTO audit_log (user_id, action, entity_type, entity_id, old_values, new_values, ip_address, user_agent)
		VALUES ($1, $2, 'product', $3, $4::jsonb, $5::jsonb, $6, $7)
	`, userID, action, strconv.Itoa(productID), string(oldJSON), string(newJSON), ipAddress, r.UserAgent())
	if err != nil {
		log.Printf("[PRODUCT AUDIT] Failed to record %s for product %d: %v", action, productID, err)
	}
	return err
}

// CreateProduct creates a new product
func CreateProduct(w http.ResponseWriter, r *http.Request) {
	var req Product
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid request body"})
		return
	}

	if err := normalizeProductRequest(&req); err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	db := repository.GetSQLDB()
	if err := validateProductRelations(db, &req, 0); err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	tx, err := db.Begin()
	if err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to create product"})
		return
	}
	defer tx.Rollback()
	if len(req.Attributes) == 0 {
		req.Attributes = json.RawMessage(`{}`)
	}

	initialStock := 0.0
	if req.TrackingMode == "quantity" && req.StockQuantity != nil {
		initialStock = *req.StockQuantity
	}
	if req.InitialZoneID != nil {
		incoming := float64(req.InitialDeviceQty)
		if req.TrackingMode == "quantity" {
			incoming = initialStock
		}
		if err := services.ValidateStorageDestination(tx, int64(*req.InitialZoneID), incoming); err != nil {
			respondJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
			return
		}
	}
	var id int64
	err = tx.QueryRow(`
		INSERT INTO products (
			name, categoryID, subcategoryID, subbiercategoryID, manufacturerid, brandid,
			description, maintenanceinterval, itemcostperday, weight, height, width, depth,
			powerconsumption, pos_in_category, is_accessory, is_consumable, count_type_id,
			stock_quantity, min_stock_level, generic_barcode, price_per_unit,
			product_type, tracking_mode, lifecycle_status, product_kind, model_number,
			manufacturer_part_number, ean, attributes, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21, $22, $23, $24, 'active', $25, $26, $27, $28, $29::jsonb, CURRENT_TIMESTAMP)
		RETURNING productID, product_code, generic_barcode
	`,
		req.Name, req.CategoryID, req.SubcategoryID, req.SubbiercategoryID,
		req.ManufacturerID, req.BrandID, req.Description, req.MaintenanceInterval,
		req.ItemCostPerDay, req.Weight, req.Height, req.Width, req.Depth,
		req.PowerConsumption, req.PosInCategory, req.IsAccessory, req.IsConsumable,
		req.CountTypeID, 0, req.MinStockLevel, req.GenericBarcode, req.PricePerUnit,
		req.ProductType, req.TrackingMode, req.ProductKind, req.ModelNumber,
		req.ManufacturerPartNo, req.EAN, string(req.Attributes),
	).Scan(&id, &req.ProductCode, &req.GenericBarcode)

	if err != nil {
		log.Printf("Failed to create product: %v", err)
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to create product"})
		return
	}
	if req.TrackingMode == "quantity" && initialStock > 0 {
		if _, err := tx.Exec(`INSERT INTO product_locations (product_id, zone_id, quantity) VALUES ($1, $2, $3)`, id, req.InitialZoneID, initialStock); err != nil {
			log.Printf("Failed to create initial stock for product %d: %v", id, err)
			respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to create initial product stock"})
			return
		}
	}
	if req.TrackingMode == "individual" && req.InitialDeviceQty > 0 {
		status := "location_unknown"
		location := "location_unknown"
		if req.InitialZoneID != nil {
			status = "in_storage"
			location = "warehouse"
		}
		for i := 0; i < req.InitialDeviceQty; i++ {
			var deviceID string
			if err := tx.QueryRow(`INSERT INTO devices(productID,status,condition_status,current_location,zone_id) VALUES($1,$2,'available',$3,$4) RETURNING deviceID`, id, status, location, req.InitialZoneID).Scan(&deviceID); err != nil {
				log.Printf("Failed to create initial device for product %d: %v", id, err)
				respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Product and devices could not be created atomically"})
				return
			}
			req.CreatedDeviceIDs = append(req.CreatedDeviceIDs, deviceID)
		}
	}

	req.ProductID = int(id)
	req.LifecycleStatus = "active"
	if req.TrackingMode == "quantity" {
		req.StockQuantity = &initialStock
	}
	if err := recordProductAudit(tx, r, "product.create", int(id), nil, req); err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to record product change"})
		return
	}
	if err := tx.Commit(); err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to create product"})
		return
	}
	websiteRevalidator.Revalidate("/products")

	respondJSON(w, http.StatusCreated, req)
}

// UpdateProduct updates an existing product
func UpdateProduct(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id, err := strconv.Atoi(vars["id"])
	if err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid product ID"})
		return
	}

	db := repository.GetSQLDB()

	var req Product
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid request body"})
		return
	}
	if err := normalizeProductRequest(&req); err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	if err := validateProductRelations(db, &req, id); err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	tx, err := db.Begin()
	if err != nil {
		log.Printf("Failed to start product update transaction: %v", err)
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to update product"})
		return
	}
	defer tx.Rollback()

	var existingTracking, existingLifecycle, oldValues string
	var currentStock float64
	err = tx.QueryRow(`
		SELECT tracking_mode, lifecycle_status, COALESCE(stock_quantity, 0), row_to_json(p)::text
		FROM products p WHERE productid = $1
	`, id).Scan(&existingTracking, &existingLifecycle, &currentStock, &oldValues)
	if err == sql.ErrNoRows {
		respondJSON(w, http.StatusNotFound, map[string]string{"error": "Product not found"})
		return
	}
	if err != nil {
		log.Printf("Failed to load product before update: %v", err)
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to update product"})
		return
	}
	req.LifecycleStatus = existingLifecycle

	if existingTracking != req.TrackingMode {
		if existingTracking == "individual" && req.TrackingMode != "individual" {
			var deviceCount int
			if err := tx.QueryRow("SELECT COUNT(*) FROM devices WHERE productid = $1", id).Scan(&deviceCount); err != nil {
				respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to validate tracking mode"})
				return
			}
			if deviceCount > 0 {
				respondJSON(w, http.StatusConflict, map[string]string{"error": "Products with devices must use individual tracking"})
				return
			}
		}
		if existingTracking == "quantity" && req.TrackingMode != "quantity" {
			var locationStock float64
			if err := tx.QueryRow("SELECT COALESCE(SUM(quantity), 0) FROM product_locations WHERE product_id = $1", id).Scan(&locationStock); err != nil {
				respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to validate tracking mode"})
				return
			}
			if locationStock > 0 {
				respondJSON(w, http.StatusConflict, map[string]string{"error": "Quantity-tracked products with stock cannot change tracking mode"})
				return
			}
		}
	}

	var attributes interface{}
	if len(req.Attributes) > 0 {
		attributes = string(req.Attributes)
	}
	result, err := tx.Exec(`
		UPDATE products SET
			name = $1, categoryID = $2, subcategoryID = $3, subbiercategoryID = $4,
			manufacturerid = $5, brandid = $6, description = $7, maintenanceinterval = $8,
			itemcostperday = $9, weight = $10, height = $11, width = $12, depth = $13,
			powerconsumption = $14, pos_in_category = $15,
			is_accessory = $16, is_consumable = $17, count_type_id = $18,
			min_stock_level = $19, generic_barcode = COALESCE($20,generic_barcode), price_per_unit = $21,
			product_type = $22, tracking_mode = $23, product_kind=$24, model_number=$25,
			manufacturer_part_number=$26, ean=$27, attributes=COALESCE($28::jsonb,attributes), updated_at = CURRENT_TIMESTAMP
		WHERE productID = $29
	`,
		req.Name, req.CategoryID, req.SubcategoryID, req.SubbiercategoryID,
		req.ManufacturerID, req.BrandID, req.Description, req.MaintenanceInterval,
		req.ItemCostPerDay, req.Weight, req.Height, req.Width, req.Depth,
		req.PowerConsumption, req.PosInCategory,
		req.IsAccessory, req.IsConsumable, req.CountTypeID,
		req.MinStockLevel, req.GenericBarcode, req.PricePerUnit,
		req.ProductType, req.TrackingMode, req.ProductKind, req.ModelNumber,
		req.ManufacturerPartNo, req.EAN, attributes,
		id,
	)

	if err != nil {
		log.Printf("Failed to update product: %v", err)
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to update product"})
		return
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		// Check whether values are unchanged; verify existence before treating as not found.
		var exists bool
		if err := tx.QueryRow("SELECT EXISTS(SELECT 1 FROM products WHERE productID = $1)", id).Scan(&exists); err != nil {
			log.Printf("Failed to verify product existence after update: %v", err)
			respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to update product"})
			return
		}
		if !exists {
			respondJSON(w, http.StatusNotFound, map[string]string{"error": "Product not found"})
			return
		}
	}

	if req.TrackingMode == "quantity" {
		desiredStock := currentStock
		if req.StockQuantity != nil {
			desiredStock = *req.StockQuantity
		}
		var currentLocationStock float64
		var assignedLocationCount int
		if err := tx.QueryRow(`
			SELECT COALESCE(SUM(quantity), 0), COUNT(*) FILTER (WHERE zone_id IS NOT NULL)
			FROM product_locations WHERE product_id = $1
		`, id).Scan(&currentLocationStock, &assignedLocationCount); err != nil {
			respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to update product stock"})
			return
		}
		if desiredStock != currentLocationStock {
			if assignedLocationCount > 0 {
				respondJSON(w, http.StatusConflict, map[string]string{
					"error": "Stock distributed across zones must be adjusted from the warehouse or scan workflow",
				})
				return
			}
			if _, err := tx.Exec(`
				INSERT INTO product_locations (product_id, zone_id, quantity, updated_at)
				VALUES ($1, NULL, $2, CURRENT_TIMESTAMP)
				ON CONFLICT (product_id, zone_id) DO UPDATE
				SET quantity = EXCLUDED.quantity, updated_at = CURRENT_TIMESTAMP
			`, id, desiredStock); err != nil {
				log.Printf("Failed to update unassigned stock for product %d: %v", id, err)
				respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to update product stock"})
				return
			}
		}
	} else if existingTracking == "quantity" {
		if _, err := tx.Exec("DELETE FROM product_locations WHERE product_id = $1", id); err != nil {
			respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to change tracking mode"})
			return
		}
		if _, err := tx.Exec(`
			UPDATE products
			SET stock_quantity = CASE
				WHEN tracking_mode = 'individual' THEN (SELECT COUNT(*) FROM devices WHERE productid = $1)
				ELSE NULL
			END
			WHERE productid = $1
		`, id); err != nil {
			respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to change tracking mode"})
			return
		}
	}

	if err := recordProductAudit(tx, r, "product.update", id, json.RawMessage(oldValues), req); err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to record product change"})
		return
	}

	if err := tx.Commit(); err != nil {
		log.Printf("Failed to commit product update transaction: %v", err)
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to update product"})
		return
	}

	websiteRevalidator.Revalidate("/products")

	respondJSON(w, http.StatusOK, map[string]string{"message": "Product updated successfully"})
}

// DeleteProduct archives a product. Devices and historical references remain intact.
func DeleteProduct(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id, err := strconv.Atoi(vars["id"])
	if err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid product ID"})
		return
	}

	db := repository.GetSQLDB()
	tx, err := db.Begin()
	if err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to archive product"})
		return
	}
	defer tx.Rollback()

	var productName, oldStatus string
	err = tx.QueryRow("SELECT name, lifecycle_status FROM products WHERE productID = $1 FOR UPDATE", id).Scan(&productName, &oldStatus)
	if err == sql.ErrNoRows {
		respondJSON(w, http.StatusNotFound, map[string]string{"error": "Product not found"})
		return
	} else if err != nil {
		log.Printf("Failed to query product: %v", err)
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to fetch product"})
		return
	}

	var deviceCount int
	err = tx.QueryRow("SELECT COUNT(*) FROM devices WHERE productID = $1", id).Scan(&deviceCount)
	if err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to check product devices"})
		return
	}

	result, err := tx.Exec(`
		UPDATE products
		SET lifecycle_status = 'archived', website_visible = FALSE, updated_at = CURRENT_TIMESTAMP
		WHERE productID = $1
	`, id)
	if err != nil {
		log.Printf("[PRODUCT ARCHIVE] Failed to archive product %d: %v", id, err)
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to archive product"})
		return
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		respondJSON(w, http.StatusNotFound, map[string]string{"error": "Product not found"})
		return
	}

	if err := recordProductAudit(tx, r, "product.archive", id,
		map[string]interface{}{"lifecycle_status": oldStatus},
		map[string]interface{}{"lifecycle_status": "archived", "website_visible": false},
	); err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to record product change"})
		return
	}
	if err := tx.Commit(); err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to archive product"})
		return
	}

	log.Printf("[PRODUCT ARCHIVE] Archived product %d (%s); preserved %d device(s)", id, productName, deviceCount)
	websiteRevalidator.Revalidate("/products")

	respondJSON(w, http.StatusOK, map[string]interface{}{
		"message":           "Product archived successfully",
		"preserved_devices": deviceCount,
	})
}

// RestoreProduct reactivates an archived product.
func RestoreProduct(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(mux.Vars(r)["id"])
	if err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid product ID"})
		return
	}
	tx, err := repository.GetSQLDB().Begin()
	if err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to restore product"})
		return
	}
	defer tx.Rollback()
	result, err := tx.Exec(`UPDATE products SET lifecycle_status = 'active', updated_at = CURRENT_TIMESTAMP WHERE productid = $1`, id)
	if err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to restore product"})
		return
	}
	if affected, _ := result.RowsAffected(); affected == 0 {
		respondJSON(w, http.StatusNotFound, map[string]string{"error": "Product not found"})
		return
	}
	if err := recordProductAudit(tx, r, "product.restore", id,
		map[string]interface{}{"lifecycle_status": "archived"},
		map[string]interface{}{"lifecycle_status": "active"},
	); err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to record product change"})
		return
	}
	if err := tx.Commit(); err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to restore product"})
		return
	}
	websiteRevalidator.Revalidate("/products")
	respondJSON(w, http.StatusOK, map[string]string{"message": "Product restored successfully"})
}

// CreateDevicesForProduct creates multiple devices for a product
func CreateDevicesForProduct(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	productID, err := strconv.Atoi(vars["id"])
	if err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid product ID"})
		return
	}

	var req DeviceCreateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid request body"})
		return
	}

	if req.Quantity <= 0 {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "Valid quantity is required"})
		return
	}

	if req.Quantity > 1000 {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "Cannot create more than 1000 devices at once"})
		return
	}

	var trackingMode, lifecycleStatus string
	err = repository.GetSQLDB().QueryRow(`SELECT tracking_mode,lifecycle_status FROM products WHERE productID=$1`, productID).Scan(&trackingMode, &lifecycleStatus)
	if err == sql.ErrNoRows {
		respondJSON(w, http.StatusNotFound, map[string]string{"error": "Product not found"})
		return
	} else if err != nil {
		log.Printf("Failed to query product: %v", err)
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to fetch product"})
		return
	}
	if lifecycleStatus != "active" {
		respondJSON(w, http.StatusConflict, map[string]string{"error": "Archived products cannot receive new devices"})
		return
	}
	if trackingMode != "individual" {
		respondJSON(w, http.StatusConflict, map[string]string{"error": "Only individually tracked products can receive devices"})
		return
	}
	autoLabel := true
	devices, err := services.NewDeviceAdminService().CreateDevices(r.Context(), &models.DeviceCreateInput{
		ProductID: productID, Quantity: req.Quantity, AutoGenerateLabel: &autoLabel,
	})
	if err != nil {
		log.Printf("[DEVICE CREATE] Atomic creation failed for product %d: %v", productID, err)
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	createdDeviceIDs := make([]string, 0, len(devices))
	for _, device := range devices {
		createdDeviceIDs = append(createdDeviceIDs, device.DeviceID)
	}
	sort.Strings(createdDeviceIDs)
	respondJSON(w, http.StatusCreated, DeviceCreateResponse{CreatedCount: len(createdDeviceIDs), DeviceIDs: createdDeviceIDs})
}

// GetProductDevices retrieves all devices for a specific product
func GetProductDevices(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	productID, err := strconv.Atoi(vars["id"])
	if err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid product ID"})
		return
	}

	db := repository.GetSQLDB()

	query := `
		WITH latest_job AS (
			SELECT jd.deviceID, MAX(jd.jobID) AS jobID
			FROM job_devices jd
			GROUP BY jd.deviceID
		)
		SELECT d.deviceID, d.productID, d.serialnumber, d.barcode, d.qr_code, d.status,
		       d.current_location, d.zone_id,
		       d.condition_rating, d.usage_hours, d.purchaseDate, d.lastmaintenance, d.nextmaintenance,
		       d.notes, d.label_path,
		       COALESCE(p.name, '') AS product_name,
		       COALESCE(cat.name, '') AS product_category,
		       COALESCE(z.name, '') AS zone_name,
		       COALESCE(z.code, '') AS zone_code,
		       dc.caseID,
		       COALESCE(c.name, '') AS case_name,
		       lj.jobID,
		       COALESCE(j.job_code, '') AS job_number
		FROM devices d
		LEFT JOIN products p ON d.productID = p.productID
		LEFT JOIN categories cat ON p.categoryID = cat.categoryID
		LEFT JOIN storage_zones z ON d.zone_id = z.zone_id
		LEFT JOIN devicescases dc ON d.deviceID = dc.deviceID
		LEFT JOIN cases c ON dc.caseID = c.caseID
		LEFT JOIN latest_job lj ON lj.deviceID = d.deviceID
		LEFT JOIN jobs j ON lj.jobID = j.jobID
		WHERE d.productID = $1
		ORDER BY d.deviceID ASC
	`

	rows, err := db.Query(query, productID)
	if err != nil {
		log.Printf("[PRODUCT DEVICES] Failed to query devices for product %d: %v", productID, err)
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to fetch product devices"})
		return
	}
	defer rows.Close()

	var responses []DeviceAdminResponse
	for rows.Next() {
		var device models.DeviceWithDetails
		err := rows.Scan(
			&device.DeviceID,
			&device.ProductID,
			&device.SerialNumber,
			&device.Barcode,
			&device.QRCode,
			&device.Status,
			&device.CurrentLocation,
			&device.ZoneID,
			&device.ConditionRating,
			&device.UsageHours,
			&device.PurchaseDate,
			&device.LastMaintenance,
			&device.NextMaintenance,
			&device.Notes,
			&device.LabelPath,
			&device.ProductName,
			&device.ProductCategory,
			&device.ZoneName,
			&device.ZoneCode,
			&device.CaseID,
			&device.CaseName,
			&device.CurrentJobID,
			&device.JobNumber,
		)
		if err != nil {
			log.Printf("[PRODUCT DEVICES] Failed to scan device: %v", err)
			continue
		}

		responses = append(responses, toDeviceAdminResponse(&device))
	}

	respondJSON(w, http.StatusOK, responses)
}

// GetLowStockAlerts returns products with stock below minimum level
func GetLowStockAlerts(w http.ResponseWriter, r *http.Request) {
	db := repository.GetDB()

	type LowStockAlert struct {
		ProductID      int     `json:"product_id"`
		Name           string  `json:"name"`
		StockQuantity  float64 `json:"stock_quantity"`
		MinStockLevel  float64 `json:"min_stock_level"`
		CountTypeName  string  `json:"count_type_name"`
		CountTypeAbbr  string  `json:"count_type_abbr"`
		GenericBarcode string  `json:"generic_barcode"`
		IsAccessory    bool    `json:"is_accessory"`
		IsConsumable   bool    `json:"is_consumable"`
	}

	var alerts []LowStockAlert
	err := db.Raw(`
		SELECT
			p.productID,
			p.name,
			COALESCE(p.stock_quantity, 0) as stock_quantity,
			COALESCE(p.min_stock_level, 0) as min_stock_level,
			COALESCE(ct.name, 'Units') as count_type_name,
			COALESCE(ct.abbreviation, 'Stk') as count_type_abbr,
			COALESCE(p.generic_barcode, '') as generic_barcode,
			COALESCE(p.is_accessory, false) as is_accessory,
			COALESCE(p.is_consumable, false) as is_consumable
		FROM products p
		LEFT JOIN count_types ct ON p.count_type_id = ct.count_type_id
			WHERE (p.is_consumable = TRUE OR p.is_accessory = TRUE)
			  AND p.lifecycle_status = 'active'
			  AND p.min_stock_level IS NOT NULL
		  AND p.min_stock_level > 0
		  AND COALESCE(p.stock_quantity, 0) < p.min_stock_level
		ORDER BY (COALESCE(p.stock_quantity, 0) / p.min_stock_level) ASC
	`).Scan(&alerts).Error
	if err != nil {
		log.Printf("Failed to fetch low stock alerts: %v", err)
		respondJSON(w, http.StatusInternalServerError, map[string]string{
			"error": "Failed to fetch low stock alerts",
		})
		return
	}

	if alerts == nil {
		alerts = []LowStockAlert{}
	}

	respondJSON(w, http.StatusOK, map[string]interface{}{
		"alerts": alerts,
	})
}

// UpdateProductWebsite updates website visibility and selected pictures without touching other product fields.
func UpdateProductWebsite(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id, err := strconv.Atoi(vars["id"])
	if err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid product ID"})
		return
	}

	var payload struct {
		WebsiteVisible   *bool    `json:"website_visible"`
		WebsiteImages    []string `json:"website_images"`
		WebsiteThumbnail *string  `json:"website_thumbnail"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid request body"})
		return
	}

	images := sanitizeWebsiteImages(payload.WebsiteImages)
	if payload.WebsiteThumbnail != nil && strings.TrimSpace(*payload.WebsiteThumbnail) == "" {
		payload.WebsiteThumbnail = nil
	}

	filteredImages, filteredThumb, err := filterAllowedImages(id, images, payload.WebsiteThumbnail)
	if err != nil && !errors.Is(err, errPicturesUnavailable) {
		log.Printf("[WEBSITE] Failed to validate images for product %d: %v", id, err)
		respondJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "Failed to validate product images"})
		return
	}

	websiteVisible := false
	if payload.WebsiteVisible != nil {
		websiteVisible = *payload.WebsiteVisible
	} else {
		if err := repository.GetSQLDB().QueryRow("SELECT website_visible FROM products WHERE productID = $1", id).Scan(&websiteVisible); err != nil {
			log.Printf("[WEBSITE] Failed to load current website visibility for product %d: %v", id, err)
			respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to update product"})
			return
		}
	}
	if websiteVisible {
		var lifecycleStatus string
		if err := repository.GetSQLDB().QueryRow("SELECT lifecycle_status FROM products WHERE productid = $1", id).Scan(&lifecycleStatus); err != nil {
			respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to update product"})
			return
		}
		if lifecycleStatus != "active" {
			respondJSON(w, http.StatusConflict, map[string]string{"error": "Archived products cannot be published"})
			return
		}
	}

	db := repository.GetSQLDB()
	result, err := db.Exec(`
		UPDATE products
		SET website_visible = $1, website_thumbnail = $2, website_images_json = $3
		WHERE productID = $4
	`, websiteVisible, filteredThumb, nullJSONFromSlice(filteredImages), id)
	if err != nil {
		log.Printf("[WEBSITE] Failed to update product %d: %v", id, err)
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to update product"})
		return
	}
	if rows, _ := result.RowsAffected(); rows == 0 {
		var exists bool
		if err := db.QueryRow("SELECT EXISTS(SELECT 1 FROM products WHERE productID = $1)", id).Scan(&exists); err != nil {
			log.Printf("[WEBSITE] Failed to verify product %d after website update: %v", id, err)
			respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to update product"})
			return
		}
		if !exists {
			respondJSON(w, http.StatusNotFound, map[string]string{"error": "Product not found"})
			return
		}
		// Values unchanged, treat as success.
	}

	respondJSON(w, http.StatusOK, map[string]interface{}{
		"message":           "Website settings updated",
		"website_visible":   websiteVisible,
		"website_thumbnail": filteredThumb,
		"website_images":    filteredImages,
	})

	// Trigger ISR revalidation for product listing
	websiteRevalidator.Revalidate("/products")

}

type WebsiteProduct struct {
	ProductID    int      `json:"product_id"`
	Name         string   `json:"name"`
	Brand        *string  `json:"brand,omitempty"`
	Category     *string  `json:"category,omitempty"`
	Subcategory  *string  `json:"subcategory,omitempty"`
	Description  *string  `json:"description,omitempty"`
	PricePerUnit *float64 `json:"price_per_unit,omitempty"`
	Thumbnail    *string  `json:"thumbnail,omitempty"`
	Images       []string `json:"images"`
}

// GetWebsiteProducts exposes products for the public website (visible flag + selected pictures).
func GetWebsiteProducts(w http.ResponseWriter, r *http.Request) {
	db := repository.GetSQLDB()
	rows, err := db.Query(`
		SELECT p.productID, p.name, b.name as brand_name, p.description, p.price_per_unit, p.website_thumbnail, p.website_images_json,
			c.name as category_name, sc.name as subcategory_name
		FROM products p
		LEFT JOIN brands b ON p.brandID = b.brandID
		LEFT JOIN categories c ON p.categoryID = c.categoryID
		LEFT JOIN subcategories sc ON p.subcategoryID = sc.subcategoryID
		WHERE p.website_visible = TRUE
		  AND p.lifecycle_status = 'active'
		  AND NOT EXISTS (
			SELECT 1
			FROM product_packages pp
			WHERE LOWER(TRIM(pp.name)) = LOWER(TRIM(p.name))
			  AND NOT EXISTS (
				SELECT 1 FROM product_package_items ppi WHERE ppi.product_id = p.productID
			  )
		  )
		ORDER BY COALESCE(p.pos_in_category, 0), p.name
	`)
	if err != nil {
		log.Printf("[WEBSITE] Failed to load website products: %v", err)
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to fetch products"})
		return
	}
	defer rows.Close()

	var result []WebsiteProduct
	for rows.Next() {
		var (
			p       WebsiteProduct
			rawImgs sql.NullString
		)
		if err := rows.Scan(&p.ProductID, &p.Name, &p.Brand, &p.Description, &p.PricePerUnit, &p.Thumbnail, &rawImgs, &p.Category, &p.Subcategory); err != nil {
			log.Printf("[WEBSITE] Failed to scan product: %v", err)
			continue
		}
		if rawImgs.Valid && len(rawImgs.String) > 0 {
			_ = json.Unmarshal([]byte(rawImgs.String), &p.Images)
		}
		p.Images = sanitizeWebsiteImages(p.Images)
		if len(p.Images) == 0 && p.Thumbnail != nil {
			p.Images = []string{*p.Thumbnail}
		}
		p.Images = buildPublicImageURLs(p.ProductID, p.Images)
		if p.Thumbnail != nil {
			thumb := buildPublicImageURLs(p.ProductID, []string{*p.Thumbnail})
			if len(thumb) > 0 {
				p.Thumbnail = &thumb[0]
			} else {
				p.Thumbnail = nil
			}
		}
		result = append(result, p)
	}

	respondJSON(w, http.StatusOK, map[string]interface{}{"products": result})
}

// GetWebsitePackages exposes product packages for the public website.
func GetWebsitePackages(w http.ResponseWriter, r *http.Request) {
	db := repository.GetSQLDB()
	if err := ensureProductPackageStorage(db); err != nil {
		packageStorageError(w, err)
		return
	}

	rows, err := db.Query(`
		SELECT
			pp.id,
			COALESCE(pp.package_code, pp.code, '') as package_code,
			pp.name,
			pp.description,
			pp.price,
			pp.website_image_url,
			pp.website_images_json
		FROM product_packages pp
		WHERE pp.website_visible = TRUE AND pp.is_active = TRUE
		ORDER BY pp.website_sort_order, pp.name
	`)
	if err != nil {
		log.Printf("[WEBSITE] Failed to load website packages: %v", err)
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to fetch packages"})
		return
	}
	defer rows.Close()

	type PackageItem struct {
		ProductID int    `json:"product_id"`
		Name      string `json:"name"`
		Quantity  int    `json:"quantity"`
	}
	type WebsitePackage struct {
		PackageID   int           `json:"package_id"`
		PackageCode string        `json:"package_code"`
		Name        string        `json:"name"`
		Description *string       `json:"description,omitempty"`
		Price       *float64      `json:"price,omitempty"`
		Thumbnail   *string       `json:"thumbnail,omitempty"`
		Images      []string      `json:"images"`
		Items       []PackageItem `json:"items"`
	}

	var result []WebsitePackage

	for rows.Next() {
		var pkg WebsitePackage
		var rawImages sql.NullString
		if err := rows.Scan(&pkg.PackageID, &pkg.PackageCode, &pkg.Name, &pkg.Description, &pkg.Price, &pkg.Thumbnail, &rawImages); err != nil {
			log.Printf("[WEBSITE] Failed to scan package: %v", err)
			continue
		}
		pkg.Images = buildPublicPackageImageURLs(pkg.PackageID, parseStringSlice(rawImages))
		if len(pkg.Images) == 0 && pkg.Thumbnail != nil {
			pkg.Images = buildPublicPackageImageURLs(pkg.PackageID, []string{*pkg.Thumbnail})
		}
		if pkg.Thumbnail != nil {
			thumbnail := buildPublicPackageImageURLs(pkg.PackageID, []string{*pkg.Thumbnail})
			if len(thumbnail) > 0 {
				pkg.Thumbnail = &thumbnail[0]
			} else {
				pkg.Thumbnail = nil
			}
		}

		items, err := loadPackageItems(db, pkg.PackageID)
		if err != nil {
			log.Printf("[WEBSITE] Failed to load items for package %d: %v", pkg.PackageID, err)
		} else {
			for _, it := range items {
				pkg.Items = append(pkg.Items, PackageItem{
					ProductID: it.ProductID,
					Name:      it.Name,
					Quantity:  it.Quantity,
				})
			}
		}
		result = append(result, pkg)
	}

	respondJSON(w, http.StatusOK, map[string]interface{}{"packages": result})
}

func buildPublicPackageImageURLs(packageID int, images []string) []string {
	result := make([]string, 0, len(images))
	for _, image := range sanitizeWebsiteImages(images) {
		result = append(result, fmt.Sprintf("/api/v1/public/packages/%d/pictures/%s?variant=preview&format=webp", packageID, url.PathEscape(image)))
	}
	return result
}

type packageItemRow struct {
	ProductID int
	Quantity  int
	Name      string
}

func loadPackageItems(db *sql.DB, packageID int) ([]packageItemRow, error) {
	rows, err := db.Query(`
		SELECT ppi.product_id, ppi.quantity, p.name
		FROM product_package_items ppi
		JOIN products p ON p.productID = ppi.product_id
		WHERE ppi.package_id = $1
	`, packageID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []packageItemRow
	for rows.Next() {
		var row packageItemRow
		if err := rows.Scan(&row.ProductID, &row.Quantity, &row.Name); err != nil {
			continue
		}
		items = append(items, row)
	}
	return items, nil
}

func sanitizeWebsiteImages(images []string) []string {
	seen := make(map[string]bool)
	out := make([]string, 0, len(images))
	for _, img := range images {
		img = strings.TrimSpace(img)
		if img == "" || seen[img] {
			continue
		}
		seen[img] = true
		out = append(out, img)
	}
	return out
}

func nullJSONFromSlice(slice []string) interface{} {
	if len(slice) == 0 {
		return nil
	}
	b, err := json.Marshal(slice)
	if err != nil {
		return nil
	}
	return b
}

func filterAllowedImages(productID int, images []string, thumb *string) ([]string, *string, error) {
	if !productPictureService.Enabled() {
		return images, thumb, errPicturesUnavailable
	}

	name, err := getProductName(productID)
	if err != nil {
		return nil, thumb, err
	}

	pics, err := productPictureService.ListPictures(name)
	if err != nil {
		log.Printf("[WEBSITE] Skip image validation for product %d: %v", productID, err)
		return images, thumb, errPicturesUnavailable
	}
	allowed := make(map[string]bool, len(pics))
	for _, p := range pics {
		allowed[p.FileName] = true
	}

	filtered := make([]string, 0, len(images))
	for _, img := range images {
		if allowed[img] {
			filtered = append(filtered, img)
		}
	}
	if thumb != nil && !allowed[*thumb] {
		thumb = nil
	}
	return filtered, thumb, nil
}

func buildPublicImageURLs(productID int, files []string) []string {
	out := make([]string, 0, len(files))
	for _, f := range files {
		// Use preview variant with WebP format for optimal web performance (25-35% smaller than JPEG)
		out = append(out, fmt.Sprintf("/api/v1/public/products/%d/pictures/%s?variant=preview&format=webp", productID, url.PathEscape(f)))
	}
	return out
}
