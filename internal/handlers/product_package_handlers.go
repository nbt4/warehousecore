package handlers

import (
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"math/big"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/mux"

	"warehousecore/internal/models"
	"warehousecore/internal/repository"
)

type productPackageRequest struct {
	Name           string                      `json:"name"`
	Description    *string                     `json:"description"`
	Price          *float64                    `json:"price"`
	Category       *string                     `json:"category"`
	Items          []models.ProductPackageItem `json:"items"`
	Aliases        []string                    `json:"aliases"`
	WebsiteVisible bool                        `json:"website_visible"`
}

var (
	packageStorageMu    sync.Mutex
	packageStorageReady bool
)

// GetProductPackages retrieves all product packages with optional search.
func GetProductPackages(w http.ResponseWriter, r *http.Request) {
	db := repository.GetSQLDB()
	if err := ensureProductPackageStorage(db); err != nil {
		packageStorageError(w, err)
		return
	}

	query := `
		SELECT pp.id,
		       COALESCE(NULLIF(pp.package_code, ''), NULLIF(pp.code, ''), ''),
		       pp.name, pp.description, pp.price, pp.category,
		       COALESCE(pp.website_visible, FALSE), pp.website_image_url,
		       pp.website_images_json, pp.alias_json, pp.created_at, pp.updated_at,
		       COALESCE((SELECT SUM(ppi.quantity) FROM product_package_items ppi WHERE ppi.package_id = pp.id), 0)
		FROM product_packages pp
		WHERE COALESCE(pp.is_active, TRUE) = TRUE`
	args := []interface{}{}
	if search := strings.TrimSpace(r.URL.Query().Get("search")); search != "" {
		query += " AND (pp.name ILIKE $1 OR COALESCE(pp.description, '') ILIKE $1 OR COALESCE(pp.code, '') ILIKE $1)"
		args = append(args, "%"+search+"%")
	}
	query += " ORDER BY pp.name"

	rows, err := db.Query(query, args...)
	if err != nil {
		log.Printf("[PACKAGES] Failed to query packages: %v", err)
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to fetch product packages"})
		return
	}
	defer rows.Close()

	packages := make([]models.ProductPackageWithItems, 0)
	for rows.Next() {
		pkg, err := scanProductPackage(rows.Scan)
		if err != nil {
			log.Printf("[PACKAGES] Failed to scan package: %v", err)
			continue
		}
		packages = append(packages, pkg)
	}
	respondJSON(w, http.StatusOK, packages)
}

// GetProductPackage retrieves a single product package with its products and quantities.
func GetProductPackage(w http.ResponseWriter, r *http.Request) {
	id, ok := packageIDFromRequest(w, r, "id")
	if !ok {
		return
	}
	db := repository.GetSQLDB()
	if err := ensureProductPackageStorage(db); err != nil {
		packageStorageError(w, err)
		return
	}

	row := db.QueryRow(`
		SELECT pp.id,
		       COALESCE(NULLIF(pp.package_code, ''), NULLIF(pp.code, ''), ''),
		       pp.name, pp.description, pp.price, pp.category,
		       COALESCE(pp.website_visible, FALSE), pp.website_image_url,
		       pp.website_images_json, pp.alias_json, pp.created_at, pp.updated_at,
		       COALESCE((SELECT SUM(ppi.quantity) FROM product_package_items ppi WHERE ppi.package_id = pp.id), 0)
		FROM product_packages pp
		WHERE pp.id = $1`, id)
	pkg, err := scanProductPackage(row.Scan)
	if err == sql.ErrNoRows {
		respondJSON(w, http.StatusNotFound, map[string]string{"error": "Product package not found"})
		return
	}
	if err != nil {
		log.Printf("[PACKAGES] Failed to query package %d: %v", id, err)
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to fetch product package"})
		return
	}

	items, err := fetchPackageItems(db, id)
	if err != nil {
		log.Printf("[PACKAGES] Failed to query items for package %d: %v", id, err)
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to fetch package items"})
		return
	}
	pkg.Items = items
	respondJSON(w, http.StatusOK, pkg)
}

// CreateProductPackage creates a package directly in the shared package tables.
// Packages intentionally do not create mirror rows in products.
func CreateProductPackage(w http.ResponseWriter, r *http.Request) {
	var req productPackageRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid request body"})
		return
	}
	if err := validateProductPackageRequest(&req); err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	db := repository.GetSQLDB()
	if err := ensureProductPackageStorage(db); err != nil {
		packageStorageError(w, err)
		return
	}
	code, err := generatePackageCode(db)
	if err != nil {
		log.Printf("[PACKAGES] Failed to generate package code: %v", err)
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to assign package code"})
		return
	}
	aliasJSON := marshalStringSlice(normalizeAliases(req.Aliases))

	tx, err := db.Begin()
	if err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to create package"})
		return
	}
	defer tx.Rollback()

	var id int64
	err = tx.QueryRow(`
		INSERT INTO product_packages
			(code, package_code, name, description, price, category, is_active, website_visible, alias_json)
		VALUES ($1, $1, $2, $3, $4, $5, TRUE, $6, $7)
		RETURNING id`, code, strings.TrimSpace(req.Name), req.Description, req.Price, cleanOptionalString(req.Category), req.WebsiteVisible, aliasJSON).Scan(&id)
	if err != nil {
		log.Printf("[PACKAGES] Failed to create package: %v", err)
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to create package"})
		return
	}
	if err := replacePackageItems(tx, int(id), req.Items); err != nil {
		log.Printf("[PACKAGES] Failed to save package items: %v", err)
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "Failed to save package products"})
		return
	}
	if err := tx.Commit(); err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to create package"})
		return
	}

	websiteRevalidator.Revalidate("/products")
	respondJSON(w, http.StatusCreated, map[string]interface{}{
		"package_id": id, "package_code": code, "message": "Product package created successfully",
	})
}

// UpdateProductPackage updates package metadata and replaces its item list atomically.
func UpdateProductPackage(w http.ResponseWriter, r *http.Request) {
	id, ok := packageIDFromRequest(w, r, "id")
	if !ok {
		return
	}
	var req productPackageRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid request body"})
		return
	}
	if err := validateProductPackageRequest(&req); err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	db := repository.GetSQLDB()
	if err := ensureProductPackageStorage(db); err != nil {
		packageStorageError(w, err)
		return
	}
	tx, err := db.Begin()
	if err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to update package"})
		return
	}
	defer tx.Rollback()

	result, err := tx.Exec(`
		UPDATE product_packages
		SET name = $1, description = $2, price = $3, category = $4,
		    website_visible = $5, alias_json = $6, updated_at = CURRENT_TIMESTAMP
		WHERE id = $7`, strings.TrimSpace(req.Name), req.Description, req.Price,
		cleanOptionalString(req.Category), req.WebsiteVisible,
		marshalStringSlice(normalizeAliases(req.Aliases)), id)
	if err != nil {
		log.Printf("[PACKAGES] Failed to update package %d: %v", id, err)
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to update package"})
		return
	}
	if affected, _ := result.RowsAffected(); affected == 0 {
		respondJSON(w, http.StatusNotFound, map[string]string{"error": "Product package not found"})
		return
	}
	if err := replacePackageItems(tx, id, req.Items); err != nil {
		log.Printf("[PACKAGES] Failed to update items for package %d: %v", id, err)
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "Failed to save package products"})
		return
	}
	if err := tx.Commit(); err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to update package"})
		return
	}
	websiteRevalidator.Revalidate("/products")
	respondJSON(w, http.StatusOK, map[string]string{"message": "Product package updated successfully"})
}

// DeleteProductPackage deletes a package. Its component products remain untouched.
func DeleteProductPackage(w http.ResponseWriter, r *http.Request) {
	id, ok := packageIDFromRequest(w, r, "id")
	if !ok {
		return
	}
	db := repository.GetSQLDB()
	if err := ensureProductPackageStorage(db); err != nil {
		packageStorageError(w, err)
		return
	}
	result, err := db.Exec("DELETE FROM product_packages WHERE id = $1", id)
	if err != nil {
		log.Printf("[PACKAGES] Failed to delete package %d: %v", id, err)
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to delete package"})
		return
	}
	if affected, _ := result.RowsAffected(); affected == 0 {
		respondJSON(w, http.StatusNotFound, map[string]string{"error": "Product package not found"})
		return
	}
	websiteRevalidator.Revalidate("/products")
	respondJSON(w, http.StatusOK, map[string]string{"message": "Product package deleted successfully"})
}

type PackageAliasEntry struct {
	Alias       string   `json:"alias"`
	PackageID   int      `json:"package_id"`
	PackageCode string   `json:"package_code"`
	PackageName string   `json:"package_name"`
	Price       *float64 `json:"price,omitempty"`
}

// GetProductPackageAliasMap returns flattened OCR aliases from alias_json.
func GetProductPackageAliasMap(w http.ResponseWriter, _ *http.Request) {
	db := repository.GetSQLDB()
	if err := ensureProductPackageStorage(db); err != nil {
		packageStorageError(w, err)
		return
	}
	rows, err := db.Query(`
		SELECT id, COALESCE(NULLIF(package_code, ''), NULLIF(code, ''), ''), name, price, alias_json
		FROM product_packages WHERE COALESCE(is_active, TRUE) = TRUE ORDER BY name`)
	if err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to fetch package aliases"})
		return
	}
	defer rows.Close()
	entries := make([]PackageAliasEntry, 0)
	for rows.Next() {
		var id int
		var code, name string
		var price sql.NullFloat64
		var raw sql.NullString
		if err := rows.Scan(&id, &code, &name, &price, &raw); err != nil {
			continue
		}
		for _, alias := range parseStringSlice(raw) {
			entry := PackageAliasEntry{Alias: alias, PackageID: id, PackageCode: code, PackageName: name}
			if price.Valid {
				value := price.Float64
				entry.Price = &value
			}
			entries = append(entries, entry)
		}
	}
	respondJSON(w, http.StatusOK, entries)
}

// AddItemToPackage adds a product or updates its quantity.
func AddItemToPackage(w http.ResponseWriter, r *http.Request) {
	id, ok := packageIDFromRequest(w, r, "id")
	if !ok {
		return
	}
	var req struct {
		ProductID int `json:"product_id"`
		Quantity  int `json:"quantity"`
	}
	if json.NewDecoder(r.Body).Decode(&req) != nil || req.ProductID <= 0 || req.Quantity <= 0 {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "Valid product ID and quantity are required"})
		return
	}
	db := repository.GetSQLDB()
	if err := ensureProductPackageStorage(db); err != nil {
		packageStorageError(w, err)
		return
	}
	result, err := db.Exec("UPDATE product_package_items SET quantity = $1 WHERE package_id = $2 AND product_id = $3", req.Quantity, id, req.ProductID)
	if err == nil {
		if affected, _ := result.RowsAffected(); affected == 0 {
			_, err = db.Exec("INSERT INTO product_package_items (package_id, product_id, quantity) VALUES ($1, $2, $3)", id, req.ProductID, req.Quantity)
		}
	}
	if err != nil {
		log.Printf("[PACKAGES] Failed to add product %d to package %d: %v", req.ProductID, id, err)
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "Failed to add product to package"})
		return
	}
	respondJSON(w, http.StatusOK, map[string]string{"message": "Product added to package successfully"})
}

// RemoveItemFromPackage removes an item by its row ID.
func RemoveItemFromPackage(w http.ResponseWriter, r *http.Request) {
	packageID, ok := packageIDFromRequest(w, r, "package_id")
	if !ok {
		return
	}
	itemID, ok := packageIDFromRequest(w, r, "item_id")
	if !ok {
		return
	}
	db := repository.GetSQLDB()
	result, err := db.Exec("DELETE FROM product_package_items WHERE package_id = $1 AND id = $2", packageID, itemID)
	if err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to remove package product"})
		return
	}
	if affected, _ := result.RowsAffected(); affected == 0 {
		respondJSON(w, http.StatusNotFound, map[string]string{"error": "Package product not found"})
		return
	}
	respondJSON(w, http.StatusOK, map[string]string{"message": "Product removed from package successfully"})
}

// GetProductPackagePictures lists package images stored in the same Nextcloud image service as products.
func GetProductPackagePictures(w http.ResponseWriter, r *http.Request) {
	id, name, ok := resolvePackageName(w, r)
	if !ok {
		return
	}
	if !productPictureService.Enabled() {
		respondJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "Product pictures are not configured"})
		return
	}
	items, err := productPictureService.ListPictures(name)
	if err != nil {
		if isStorageNotFound(err) {
			respondJSON(w, http.StatusOK, map[string]interface{}{"pictures": []interface{}{}})
			return
		}
		log.Printf("[PACKAGE PICTURES] List failed for package %d: %v", id, err)
		respondJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "Failed to list pictures"})
		return
	}
	sort.Slice(items, func(i, j int) bool { return items[i].ModifiedAt.After(items[j].ModifiedAt) })
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
		base := fmt.Sprintf("/api/v1/admin/product-packages/%d/pictures/%s", id, url.PathEscape(pic.FileName))
		resp = append(resp, pictureResponse{pic.FileName, pic.Size, pic.ContentType, pic.ModifiedAt, base, base + "?variant=thumb", base + "?variant=preview"})
	}
	respondJSON(w, http.StatusOK, map[string]interface{}{"pictures": resp})
}

// UploadProductPackagePictures uploads images and appends them to the website selection.
func UploadProductPackagePictures(w http.ResponseWriter, r *http.Request) {
	id, name, ok := resolvePackageName(w, r)
	if !ok {
		return
	}
	if !productPictureService.Enabled() {
		respondJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "Product pictures are not configured"})
		return
	}
	if err := r.ParseMultipartForm(productPictureService.MaxFileSize() * 4); err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid multipart form: " + err.Error()})
		return
	}
	files := r.MultipartForm.File["files"]
	if len(files) == 0 {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "No files provided"})
		return
	}
	uploaded := make([]string, 0, len(files))
	for _, header := range files {
		src, err := header.Open()
		if err != nil {
			respondJSON(w, http.StatusBadRequest, map[string]string{"error": "Failed to read uploaded file"})
			return
		}
		stored, uploadErr := productPictureService.UploadPicture(name, src, header)
		src.Close()
		if uploadErr != nil {
			respondJSON(w, http.StatusBadRequest, map[string]string{"error": uploadErr.Error()})
			return
		}
		uploaded = append(uploaded, stored)
		productPictureService.WarmPictureVariants(name, stored)
	}

	db := repository.GetSQLDB()
	var currentRaw sql.NullString
	var currentThumb sql.NullString
	if err := db.QueryRow("SELECT website_images_json, website_image_url FROM product_packages WHERE id = $1", id).Scan(&currentRaw, &currentThumb); err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Pictures uploaded but package metadata could not be updated"})
		return
	}
	images := sanitizeWebsiteImages(append(parseStringSlice(currentRaw), uploaded...))
	thumbnail := ""
	if currentThumb.Valid {
		thumbnail = currentThumb.String
	}
	if index, err := strconv.Atoi(r.FormValue("thumbnail_index")); err == nil && index >= 0 && index < len(uploaded) {
		thumbnail = uploaded[index]
	}
	if thumbnail == "" && len(images) > 0 {
		thumbnail = images[0]
	}
	if _, err := db.Exec("UPDATE product_packages SET website_image_url = $1, website_images_json = $2, updated_at = CURRENT_TIMESTAMP WHERE id = $3", nullString(thumbnail), marshalStringSlice(images), id); err != nil {
		log.Printf("[PACKAGE PICTURES] Failed to update package %d image metadata: %v", id, err)
	}
	websiteRevalidator.Revalidate("/products")
	respondJSON(w, http.StatusCreated, map[string]interface{}{"message": "Pictures uploaded successfully", "uploaded_files": uploaded, "uploaded_count": len(uploaded), "thumbnail": thumbnail})
}

// DeleteProductPackagePicture deletes an image and removes it from website metadata.
func DeleteProductPackagePicture(w http.ResponseWriter, r *http.Request) {
	id, name, ok := resolvePackageName(w, r)
	if !ok {
		return
	}
	filename, err := url.PathUnescape(mux.Vars(r)["filename"])
	if err != nil || filename == "" {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid filename"})
		return
	}
	if err := productPictureService.DeletePicture(name, filename); err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to delete picture"})
		return
	}
	productPictureService.ClearCachedVariants(name, filename)
	db := repository.GetSQLDB()
	var raw, thumb sql.NullString
	if err := db.QueryRow("SELECT website_images_json, website_image_url FROM product_packages WHERE id = $1", id).Scan(&raw, &thumb); err == nil {
		images := make([]string, 0)
		for _, image := range parseStringSlice(raw) {
			if image != filename {
				images = append(images, image)
			}
		}
		thumbnail := ""
		if thumb.Valid && thumb.String != filename {
			thumbnail = thumb.String
		} else if len(images) > 0 {
			thumbnail = images[0]
		}
		_, _ = db.Exec("UPDATE product_packages SET website_image_url = $1, website_images_json = $2 WHERE id = $3", nullString(thumbnail), marshalStringSlice(images), id)
	}
	websiteRevalidator.Revalidate("/products")
	respondJSON(w, http.StatusOK, map[string]string{"message": "Picture deleted"})
}

// DownloadProductPackagePicture streams an admin or public package image.
func DownloadProductPackagePicture(w http.ResponseWriter, r *http.Request) {
	id, name, ok := resolvePackageName(w, r)
	if !ok {
		return
	}
	filename, err := url.PathUnescape(mux.Vars(r)["filename"])
	if err != nil || filename == "" {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid filename"})
		return
	}
	reader, contentType, err := productPictureService.DownloadPictureWithVariant(name, filename, strings.TrimSpace(r.URL.Query().Get("variant")), strings.TrimSpace(r.URL.Query().Get("format")))
	if err != nil {
		log.Printf("[PACKAGE PICTURES] Download failed for package %d: %v", id, err)
		respondJSON(w, http.StatusNotFound, map[string]string{"error": "File not found or storage unavailable"})
		return
	}
	defer reader.Close()
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Cache-Control", "public, max-age=86400")
	w.Header().Set("Content-Disposition", fmt.Sprintf("inline; filename=\"%s\"", url.PathEscape(filename)))
	if _, err := io.Copy(w, reader); err != nil {
		log.Printf("[PACKAGE PICTURES] Failed to stream %s: %v", filename, err)
	}
}

// UpdateProductPackageWebsite controls public visibility and selected images.
func UpdateProductPackageWebsite(w http.ResponseWriter, r *http.Request) {
	id, name, ok := resolvePackageName(w, r)
	if !ok {
		return
	}
	var req struct {
		WebsiteVisible   bool     `json:"website_visible"`
		WebsiteThumbnail *string  `json:"website_thumbnail"`
		WebsiteImages    []string `json:"website_images"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid request body"})
		return
	}
	images := sanitizeWebsiteImages(req.WebsiteImages)
	thumbnail := req.WebsiteThumbnail
	if thumbnail != nil && strings.TrimSpace(*thumbnail) == "" {
		thumbnail = nil
	}
	if productPictureService.Enabled() {
		pictures, err := productPictureService.ListPictures(name)
		if err != nil && !isStorageNotFound(err) {
			respondJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "Failed to validate package images"})
			return
		}
		allowed := make(map[string]bool, len(pictures))
		for _, picture := range pictures {
			allowed[picture.FileName] = true
		}
		filtered := images[:0]
		for _, image := range images {
			if allowed[image] {
				filtered = append(filtered, image)
			}
		}
		images = filtered
		if thumbnail != nil && !allowed[*thumbnail] {
			thumbnail = nil
		}
	}
	if thumbnail == nil && len(images) > 0 {
		thumbnail = &images[0]
	}
	db := repository.GetSQLDB()
	result, err := db.Exec(`UPDATE product_packages SET website_visible = $1, website_image_url = $2, website_images_json = $3, updated_at = CURRENT_TIMESTAMP WHERE id = $4`, req.WebsiteVisible, thumbnail, marshalStringSlice(images), id)
	if err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to update package website settings"})
		return
	}
	if affected, _ := result.RowsAffected(); affected == 0 {
		respondJSON(w, http.StatusNotFound, map[string]string{"error": "Product package not found"})
		return
	}
	websiteRevalidator.Revalidate("/products")
	respondJSON(w, http.StatusOK, map[string]interface{}{"message": "Website settings updated", "website_visible": req.WebsiteVisible, "website_thumbnail": thumbnail, "website_images": images})
}

type rowScanner func(dest ...interface{}) error

func scanProductPackage(scan rowScanner) (models.ProductPackageWithItems, error) {
	var pkg models.ProductPackageWithItems
	var rawImages, rawAliases sql.NullString
	err := scan(&pkg.PackageID, &pkg.PackageCode, &pkg.Name, &pkg.Description, &pkg.Price,
		&pkg.Category, &pkg.WebsiteVisible, &pkg.WebsiteThumbnail, &rawImages, &rawAliases,
		&pkg.CreatedAt, &pkg.UpdatedAt, &pkg.TotalItems)
	if err != nil {
		return pkg, err
	}
	pkg.WebsiteImages = parseStringSlice(rawImages)
	pkg.Aliases = parseStringSlice(rawAliases)
	return pkg, nil
}

func fetchPackageItems(db *sql.DB, packageID int) ([]models.PackageItemDetail, error) {
	rows, err := db.Query(`
		SELECT ppi.id, ppi.product_id, p.name, COALESCE(ppi.quantity, 1), c.name, b.name
		FROM product_package_items ppi
		JOIN products p ON p.productID = ppi.product_id
		LEFT JOIN categories c ON p.categoryID = c.categoryID
		LEFT JOIN brands b ON p.brandID = b.brandID
		WHERE ppi.package_id = $1 ORDER BY p.name`, packageID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]models.PackageItemDetail, 0)
	for rows.Next() {
		var item models.PackageItemDetail
		if err := rows.Scan(&item.PackageItemID, &item.ProductID, &item.ProductName, &item.Quantity, &item.CategoryName, &item.BrandName); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func replacePackageItems(tx *sql.Tx, packageID int, requested []models.ProductPackageItem) error {
	if _, err := tx.Exec("DELETE FROM product_package_items WHERE package_id = $1", packageID); err != nil {
		return err
	}
	quantities := make(map[int]int)
	for _, item := range requested {
		if item.ProductID > 0 && item.Quantity > 0 {
			quantities[item.ProductID] += item.Quantity
		}
	}
	if len(quantities) == 0 {
		return errors.New("a package must contain at least one product")
	}
	for productID, quantity := range quantities {
		if _, err := tx.Exec("INSERT INTO product_package_items (package_id, product_id, quantity) VALUES ($1, $2, $3)", packageID, productID, quantity); err != nil {
			return err
		}
	}
	return nil
}

func validateProductPackageRequest(req *productPackageRequest) error {
	if strings.TrimSpace(req.Name) == "" {
		return errors.New("package name is required")
	}
	validItems := 0
	for _, item := range req.Items {
		if item.ProductID > 0 && item.Quantity > 0 {
			validItems++
		}
	}
	if validItems == 0 {
		return errors.New("a package must contain at least one product")
	}
	return nil
}

func ensureProductPackageStorage(db *sql.DB) error {
	if db == nil {
		return errors.New("database connection is nil")
	}
	packageStorageMu.Lock()
	defer packageStorageMu.Unlock()
	if packageStorageReady {
		return nil
	}
	statements := []string{
		`CREATE TABLE IF NOT EXISTS product_packages (
			id SERIAL PRIMARY KEY, name VARCHAR(255) NOT NULL, code VARCHAR(50) UNIQUE,
			description TEXT, price DECIMAL(10,2) DEFAULT 0, category VARCHAR(100),
			is_active BOOLEAN DEFAULT TRUE, website_visible BOOLEAN DEFAULT FALSE,
			website_image_url VARCHAR(512), website_sort_order INT DEFAULT 0,
			alias_json TEXT, created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP)`,
		`ALTER TABLE product_packages ADD COLUMN IF NOT EXISTS package_code VARCHAR(32)`,
		`ALTER TABLE product_packages ADD COLUMN IF NOT EXISTS code VARCHAR(50)`,
		`ALTER TABLE product_packages ADD COLUMN IF NOT EXISTS category VARCHAR(100)`,
		`ALTER TABLE product_packages ADD COLUMN IF NOT EXISTS is_active BOOLEAN DEFAULT TRUE`,
		`ALTER TABLE product_packages ADD COLUMN IF NOT EXISTS website_visible BOOLEAN DEFAULT FALSE`,
		`ALTER TABLE product_packages ADD COLUMN IF NOT EXISTS website_image_url VARCHAR(512)`,
		`ALTER TABLE product_packages ADD COLUMN IF NOT EXISTS website_sort_order INT DEFAULT 0`,
		`ALTER TABLE product_packages ADD COLUMN IF NOT EXISTS website_images_json TEXT`,
		`ALTER TABLE product_packages ADD COLUMN IF NOT EXISTS alias_json TEXT`,
		`ALTER TABLE product_packages ADD COLUMN IF NOT EXISTS created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP`,
		`ALTER TABLE product_packages ADD COLUMN IF NOT EXISTS updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP`,
		`CREATE TABLE IF NOT EXISTS product_package_items (
			id SERIAL PRIMARY KEY, package_id INT NOT NULL REFERENCES product_packages(id) ON DELETE CASCADE,
			product_id INT NOT NULL REFERENCES products(productID) ON DELETE CASCADE,
			quantity INT NOT NULL DEFAULT 1, is_optional BOOLEAN DEFAULT FALSE,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP)`,
		`UPDATE product_packages SET package_code = COALESCE(NULLIF(code, ''), 'PKG-' || LPAD(id::text, 6, '0')) WHERE package_code IS NULL OR package_code = ''`,
		`UPDATE product_packages SET code = package_code WHERE code IS NULL OR code = ''`,
		`UPDATE product_packages pp
		 SET website_image_url = COALESCE(pp.website_image_url, p.website_thumbnail),
		     website_images_json = COALESCE(pp.website_images_json, p.website_images_json)
		 FROM products p
		 WHERE LOWER(TRIM(pp.name)) = LOWER(TRIM(p.name))
		   AND NOT EXISTS (SELECT 1 FROM product_package_items ppi WHERE ppi.product_id = p.productID)
		   AND (pp.website_image_url IS NULL OR pp.website_images_json IS NULL)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS uq_product_packages_package_code ON product_packages(package_code)`,
		`CREATE INDEX IF NOT EXISTS idx_product_package_items_package ON product_package_items(package_id)`,
		`CREATE INDEX IF NOT EXISTS idx_product_package_items_product ON product_package_items(product_id)`,
	}
	for _, statement := range statements {
		if _, err := db.Exec(statement); err != nil {
			return err
		}
	}
	packageStorageReady = true
	return nil
}

func normalizeAliases(values []string) []string {
	seen := make(map[string]bool)
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		key := strings.ToLower(value)
		if value == "" || seen[key] {
			continue
		}
		seen[key] = true
		result = append(result, value)
	}
	return result
}

func parseStringSlice(raw sql.NullString) []string {
	result := make([]string, 0)
	if raw.Valid && strings.TrimSpace(raw.String) != "" {
		_ = json.Unmarshal([]byte(raw.String), &result)
	}
	return sanitizeWebsiteImages(result)
}

func marshalStringSlice(values []string) interface{} {
	if len(values) == 0 {
		return nil
	}
	data, err := json.Marshal(values)
	if err != nil {
		return nil
	}
	return string(data)
}

func cleanOptionalString(value *string) interface{} {
	if value == nil || strings.TrimSpace(*value) == "" {
		return nil
	}
	return strings.TrimSpace(*value)
}

func nullString(value string) interface{} {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return value
}

func packageIDFromRequest(w http.ResponseWriter, r *http.Request, key string) (int, bool) {
	id, err := strconv.Atoi(mux.Vars(r)[key])
	if err != nil || id <= 0 {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid package ID"})
		return 0, false
	}
	return id, true
}

func resolvePackageName(w http.ResponseWriter, r *http.Request) (int, string, bool) {
	id, ok := packageIDFromRequest(w, r, "id")
	if !ok {
		return 0, "", false
	}
	db := repository.GetSQLDB()
	if err := ensureProductPackageStorage(db); err != nil {
		packageStorageError(w, err)
		return 0, "", false
	}
	var name string
	if err := db.QueryRow("SELECT name FROM product_packages WHERE id = $1", id).Scan(&name); err == sql.ErrNoRows {
		respondJSON(w, http.StatusNotFound, map[string]string{"error": "Product package not found"})
		return 0, "", false
	} else if err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to load product package"})
		return 0, "", false
	}
	return id, name, true
}

func packageStorageError(w http.ResponseWriter, err error) {
	log.Printf("[PACKAGES] Failed to prepare package storage: %v", err)
	respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to prepare package storage"})
}

func isStorageNotFound(err error) bool {
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "404") || strings.Contains(message, "not found")
}

const packageCodeCharset = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"

func generatePackageCode(db *sql.DB) (string, error) {
	for attempts := 0; attempts < 20; attempts++ {
		var value strings.Builder
		for i := 0; i < 5; i++ {
			n, err := rand.Int(rand.Reader, big.NewInt(int64(len(packageCodeCharset))))
			if err != nil {
				return "", err
			}
			value.WriteByte(packageCodeCharset[n.Int64()])
		}
		code := "PKG-" + value.String()
		var exists bool
		if err := db.QueryRow("SELECT EXISTS(SELECT 1 FROM product_packages WHERE code = $1 OR package_code = $1)", code).Scan(&exists); err != nil {
			return "", err
		}
		if !exists {
			return code, nil
		}
	}
	return "", errors.New("could not generate unique package code")
}
