package handlers

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/mux"

	"warehousecore/internal/models"
	"warehousecore/internal/repository"
	"warehousecore/internal/services"
)

type CaseSummary struct {
	CaseID      int      `json:"case_id"`
	Name        string   `json:"name"`
	Description *string  `json:"description,omitempty"`
	Status      string   `json:"status"`
	Width       *float64 `json:"width,omitempty"`
	Height      *float64 `json:"height,omitempty"`
	Depth       *float64 `json:"depth,omitempty"`
	Weight      *float64 `json:"weight,omitempty"`
	ZoneID      *int     `json:"zone_id,omitempty"`
	ZoneName    *string  `json:"zone_name,omitempty"`
	ZoneCode    *string  `json:"zone_code,omitempty"`
	DeviceCount int      `json:"device_count"`
	LabelPath   *string  `json:"label_path,omitempty"`
}

type CaseDetail struct {
	CaseSummary
}

type CaseDevice struct {
	DeviceID     string  `json:"device_id"`
	Status       string  `json:"status"`
	SerialNumber *string `json:"serial_number,omitempty"`
	Barcode      *string `json:"barcode,omitempty"`
	ProductName  *string `json:"product_name,omitempty"`
	ZoneID       *int    `json:"zone_id,omitempty"`
	ZoneName     *string `json:"zone_name,omitempty"`
	ZoneCode     *string `json:"zone_code,omitempty"`
}

func ptrString(ns sql.NullString) *string {
	if !ns.Valid {
		return nil
	}
	val := ns.String
	return &val
}

func ptrFloat64(n sql.NullFloat64) *float64 {
	if !n.Valid {
		return nil
	}
	val := n.Float64
	return &val
}

// PostgresPlaceholders converts MySQL ? placeholders to PostgreSQL $N placeholders
// It also returns an argument counter that can be used for building dynamic queries
type QueryBuilder struct {
	argCount int
}

func NewQueryBuilder() *QueryBuilder {
	return &QueryBuilder{argCount: 0}
}

func (qb *QueryBuilder) NextPlaceholder() string {
	qb.argCount++
	return fmt.Sprintf("$%d", qb.argCount)
}

func (qb *QueryBuilder) CurrentCount() int {
	return qb.argCount
}

func ptrInt(n sql.NullInt64) *int {
	if !n.Valid {
		return nil
	}
	val := int(n.Int64)
	return &val
}

func nullableStringPtr(value *string) interface{} {
	if value == nil {
		return nil
	}
	trimmed := strings.TrimSpace(*value)
	if trimmed == "" {
		return nil
	}
	return trimmed
}

func nullableFloatPtr(value *float64) interface{} {
	if value == nil {
		return nil
	}
	return *value
}

func nullableIntPtr(value *int) interface{} {
	if value == nil {
		return nil
	}
	return *value
}

func loadCaseDetail(db *sql.DB, caseID int64) (*CaseDetail, error) {
	query := `
		SELECT
			c.caseID,
			c.name,
			c.description,
			c.status,
			c.width,
			c.height,
			c.depth,
			c.weight,
			c.zone_id,
			c.label_path,
			COALESCE(z.name, '') AS zone_name,
			COALESCE(z.code, '') AS zone_code,
			COUNT(dc.deviceID) AS device_count
		FROM cases c
		LEFT JOIN devicescases dc ON c.caseID = dc.caseID
		LEFT JOIN storage_zones z ON c.zone_id = z.zone_id
		WHERE c.caseID = $1
		GROUP BY c.caseID, c.name, c.description, c.status, c.width, c.height, c.depth, c.weight, c.zone_id, c.label_path, zone_name, zone_code
	`

	var description sql.NullString
	var width, height, depth, weight sql.NullFloat64
	var zoneID sql.NullInt64
	var labelPath sql.NullString
	var zoneName, zoneCode sql.NullString
	var deviceCount sql.NullInt64

	var detail CaseDetail

	err := db.QueryRow(query, caseID).Scan(
		&detail.CaseID,
		&detail.Name,
		&description,
		&detail.Status,
		&width,
		&height,
		&depth,
		&weight,
		&zoneID,
		&labelPath,
		&zoneName,
		&zoneCode,
		&deviceCount,
	)
	if err != nil {
		return nil, err
	}

	detail.Description = ptrString(description)
	detail.Width = ptrFloat64(width)
	detail.Height = ptrFloat64(height)
	detail.Depth = ptrFloat64(depth)
	detail.Weight = ptrFloat64(weight)
	detail.ZoneID = ptrInt(zoneID)
	detail.LabelPath = ptrString(labelPath)

	if zoneName.Valid && zoneName.String != "" {
		detail.ZoneName = ptrString(zoneName)
	}
	if zoneCode.Valid && zoneCode.String != "" {
		detail.ZoneCode = ptrString(zoneCode)
	}
	if deviceCount.Valid {
		detail.DeviceCount = int(deviceCount.Int64)
	}

	return &detail, nil
}

func loadCaseDevices(db *sql.DB, caseID int64) ([]CaseDevice, error) {
	query := `
		SELECT 
			d.deviceID,
			d.status,
			d.serialnumber,
			d.barcode,
			d.zone_id,
			COALESCE(z.name, '') AS zone_name,
			COALESCE(z.code, '') AS zone_code,
			COALESCE(p.name, '') AS product_name
		FROM devicescases dc
		INNER JOIN devices d ON dc.deviceID = d.deviceID
		LEFT JOIN products p ON d.productID = p.productID
		LEFT JOIN storage_zones z ON d.zone_id = z.zone_id
		WHERE dc.caseID = $1
		ORDER BY d.deviceID ASC
	`

	rows, err := db.Query(query, caseID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	devices := []CaseDevice{}

	for rows.Next() {
		var device CaseDevice
		var serialNumber, barcode, productName sql.NullString
		var zoneID sql.NullInt64
		var zoneName, zoneCode sql.NullString

		if err := rows.Scan(
			&device.DeviceID,
			&device.Status,
			&serialNumber,
			&barcode,
			&zoneID,
			&zoneName,
			&zoneCode,
			&productName,
		); err != nil {
			log.Printf("loadCaseDevices scan error: %v", err)
			continue
		}

		device.SerialNumber = ptrString(serialNumber)
		device.Barcode = ptrString(barcode)
		device.ProductName = ptrString(productName)
		device.ZoneID = ptrInt(zoneID)

		if zoneName.Valid && zoneName.String != "" {
			device.ZoneName = ptrString(zoneName)
		}
		if zoneCode.Valid && zoneCode.String != "" {
			device.ZoneCode = ptrString(zoneCode)
		}

		devices = append(devices, device)
	}

	return devices, nil
}

func loadAvailableCaseDevices(db *sql.DB, caseID *int64, search string, limit int) ([]CaseDevice, error) {
	qb := NewQueryBuilder()
	baseQuery := `
		SELECT DISTINCT
			d.deviceID,
			d.status,
			d.serialnumber,
			d.barcode,
			d.zone_id,
			COALESCE(z.name, '') AS zone_name,
			COALESCE(z.code, '') AS zone_code,
			COALESCE(p.name, '') AS product_name
		FROM devices d
		LEFT JOIN devicescases dc ON d.deviceID = dc.deviceID
		LEFT JOIN products p ON d.productID = p.productID
		LEFT JOIN storage_zones z ON d.zone_id = z.zone_id
		WHERE 1=1
	`

	args := []interface{}{}

	if search != "" {
		baseQuery += " AND (d.deviceID LIKE " + qb.NextPlaceholder() + " OR d.serialnumber LIKE " + qb.NextPlaceholder() + " OR p.name LIKE " + qb.NextPlaceholder() + ")"
		term := "%" + search + "%"
		args = append(args, term, term, term)
	}

	if caseID != nil {
		baseQuery += " AND (dc.caseID IS NULL OR dc.caseID = " + qb.NextPlaceholder() + ")"
		args = append(args, *caseID)
	} else {
		baseQuery += " AND dc.caseID IS NULL"
	}

	baseQuery += " ORDER BY d.deviceID ASC LIMIT " + qb.NextPlaceholder()
	args = append(args, limit)

	rows, err := db.Query(baseQuery, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	devices := []CaseDevice{}

	for rows.Next() {
		var device CaseDevice
		var serialNumber, barcode, productName sql.NullString
		var zoneID sql.NullInt64
		var zoneName, zoneCode sql.NullString

		if err := rows.Scan(
			&device.DeviceID,
			&device.Status,
			&serialNumber,
			&barcode,
			&zoneID,
			&zoneName,
			&zoneCode,
			&productName,
		); err != nil {
			log.Printf("loadAvailableCaseDevices scan error: %v", err)
			continue
		}

		device.SerialNumber = ptrString(serialNumber)
		device.Barcode = ptrString(barcode)
		device.ProductName = ptrString(productName)
		device.ZoneID = ptrInt(zoneID)

		if zoneName.Valid && zoneName.String != "" {
			device.ZoneName = ptrString(zoneName)
		}
		if zoneCode.Valid && zoneCode.String != "" {
			device.ZoneCode = ptrString(zoneCode)
		}

		devices = append(devices, device)
	}

	return devices, nil
}

// HealthCheck returns server health status
func HealthCheck(w http.ResponseWriter, r *http.Request) {
	db := repository.GetSQLDB()
	if err := db.Ping(); err != nil {
		respondJSON(w, http.StatusServiceUnavailable, map[string]interface{}{
			"status": "unhealthy",
			"error":  "database connection failed",
		})
		return
	}

	respondJSON(w, http.StatusOK, map[string]interface{}{
		"status":  "healthy",
		"service": "WarehouseCore",
	})
}

// HandleScan processes barcode/QR scan requests
func HandleScan(w http.ResponseWriter, r *http.Request) {
	var req models.ScanRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid request"})
		return
	}

	// All scans (devices, accessories, consumables) now use the unified scan service
	// The scan service handles automatic zone selection and proper stock synchronization
	scanService := services.NewScanService()
	response, err := scanService.ProcessScan(req, nil, r.RemoteAddr, r.UserAgent())
	if err != nil {
		log.Printf("Scan processing error: %v", err)
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	respondJSON(w, http.StatusOK, response)
}

// handleAccessoryConsumableScan processes accessory/consumable scans directly in WarehouseCore
func handleAccessoryConsumableScan(w http.ResponseWriter, scanReq *models.ScanRequest, isAccessory bool) {
	db := repository.GetDB()

	// Get product details
	var product struct {
		ProductID     int     `gorm:"column:productID"`
		Name          string  `gorm:"column:name"`
		StockQuantity float64 `gorm:"column:stock_quantity"`
		MinStockLevel float64 `gorm:"column:min_stock_level"`
		CountTypeName string  `gorm:"column:count_type_name"`
		CountTypeAbbr string  `gorm:"column:count_type_abbr"`
	}

	err := db.Table("products").
		Select("products.productID, products.name, products.stock_quantity, products.min_stock_level, ct.name as count_type_name, ct.abbreviation as count_type_abbr").
		Joins("LEFT JOIN count_types ct ON products.count_type_id = ct.count_type_id").
		Where("generic_barcode = ?", scanReq.ScanCode).
		First(&product).Error

	if err != nil {
		log.Printf("Failed to get product details: %v", err)
		respondJSON(w, http.StatusNotFound, map[string]interface{}{
			"success": false,
			"message": "Produkt nicht gefunden",
			"action":  scanReq.Action,
		})
		return
	}

	// Determine quantity (for consumables, frontend passes it via prompt, for accessories always 1)
	quantity := 1.0
	if !isAccessory && scanReq.JobID != nil {
		quantity = float64(*scanReq.JobID)
	}

	// Calculate new stock based on action
	var newStock float64
	var message string

	switch scanReq.Action {
	case "intake":
		newStock = product.StockQuantity + quantity
		message = fmt.Sprintf("✅ %s eingelagert: %.1f %s (Neuer Bestand: %.1f)",
			product.Name, quantity, product.CountTypeAbbr, newStock)
	case "outtake":
		if product.StockQuantity < quantity {
			respondJSON(w, http.StatusBadRequest, map[string]interface{}{
				"success": false,
				"message": fmt.Sprintf("❌ Nicht genug Bestand! Verfügbar: %.1f %s", product.StockQuantity, product.CountTypeAbbr),
				"action":  scanReq.Action,
			})
			return
		}
		newStock = product.StockQuantity - quantity
		message = fmt.Sprintf("✅ %s ausgelagert: %.1f %s (Verbleibender Bestand: %.1f)",
			product.Name, quantity, product.CountTypeAbbr, newStock)
	case "check":
		message = fmt.Sprintf("📊 %s - Aktueller Bestand: %.1f %s",
			product.Name, product.StockQuantity, product.CountTypeAbbr)
		if product.StockQuantity <= product.MinStockLevel {
			message += fmt.Sprintf(" ⚠️ Unter Mindestbestand (%.1f)", product.MinStockLevel)
		}
		respondJSON(w, http.StatusOK, map[string]interface{}{
			"success": true,
			"message": message,
			"action":  scanReq.Action,
			"product": map[string]interface{}{
				"product_id":      product.ProductID,
				"name":            product.Name,
				"stock_quantity":  product.StockQuantity,
				"min_stock_level": product.MinStockLevel,
				"unit":            product.CountTypeAbbr,
			},
		})
		return
	default:
		respondJSON(w, http.StatusBadRequest, map[string]interface{}{
			"success": false,
			"message": "Ungültige Aktion",
			"action":  scanReq.Action,
		})
		return
	}

	// Update stock in database
	err = db.Table("products").
		Where("productID = ?", product.ProductID).
		Update("stock_quantity", newStock).Error

	if err != nil {
		log.Printf("Failed to update stock: %v", err)
		respondJSON(w, http.StatusInternalServerError, map[string]interface{}{
			"success": false,
			"message": "Fehler beim Aktualisieren des Bestands",
			"action":  scanReq.Action,
		})
		return
	}

	// Update product location if zone is provided
	zoneInfo := "no zone"
	if scanReq.ZoneID != nil {
		zoneInfo = fmt.Sprintf("zone_id=%d", *scanReq.ZoneID)

		// Update or insert product location
		if scanReq.Action == "intake" {
			// Add quantity to this zone location
			err = db.Exec(`
				INSERT INTO product_locations (product_id, zone_id, quantity)
				VALUES ($1, $2, $3)
				ON DUPLICATE KEY UPDATE quantity = quantity + $4
			`, product.ProductID, *scanReq.ZoneID, quantity, quantity).Error

			if err != nil {
				log.Printf("Failed to update product location: %v", err)
			}
		} else if scanReq.Action == "outtake" {
			// Remove quantity from this zone location
			err = db.Exec(`
				UPDATE product_locations
				SET quantity = GREATEST(0, quantity - $1)
				WHERE product_id = $2 AND zone_id = $3
			`, quantity, product.ProductID, *scanReq.ZoneID).Error

			if err != nil {
				log.Printf("Failed to update product location: %v", err)
			}

			// Delete location if quantity is 0
			db.Exec("DELETE FROM product_locations WHERE product_id = $1 AND zone_id = $2 AND quantity = 0",
				product.ProductID, *scanReq.ZoneID)
		}
	}

	log.Printf("✅ Stock updated: %s | %s | Old: %.1f → New: %.1f | %s",
		product.Name, scanReq.Action, product.StockQuantity, newStock, zoneInfo)

	respondJSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"message": message,
		"action":  scanReq.Action,
		"product": map[string]interface{}{
			"product_id":      product.ProductID,
			"name":            product.Name,
			"stock_quantity":  newStock,
			"min_stock_level": product.MinStockLevel,
			"unit":            product.CountTypeAbbr,
		},
	})
}

// proxyToRentalCore forwards accessory/consumable scans to RentalCore (DEPRECATED - kept for reference)
func proxyToRentalCore(w http.ResponseWriter, r *http.Request, scanReq *models.ScanRequest, isAccessory bool) {
	// Determine endpoint based on product type
	var endpoint string
	if isAccessory {
		endpoint = "/api/v1/scan/accessory"
	} else {
		endpoint = "/api/v1/scan/consumable"
	}

	// Map action to direction
	direction := "check"
	if scanReq.Action == "intake" {
		direction = "in"
	} else if scanReq.Action == "outtake" {
		direction = "out"
	}

	// Build request body
	requestBody := map[string]interface{}{
		"barcode":   scanReq.ScanCode,
		"direction": direction,
	}

	// For accessories, quantity is always 1
	if isAccessory {
		requestBody["quantity"] = 1
	} else {
		// For consumables, quantity is passed via JobID field (temporary workaround)
		if scanReq.JobID != nil {
			requestBody["quantity"] = *scanReq.JobID
		} else {
			requestBody["quantity"] = 1 // Default to 1 if not provided
		}
	}

	// Marshal request
	jsonData, err := json.Marshal(requestBody)
	if err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to create request"})
		return
	}

	// Get RentalCore URL from environment
	rentalCoreURL := os.Getenv("RENTALCORE_URL")
	if rentalCoreURL == "" {
		rentalCoreURL = "http://rentalcore:8081"
	}

	// Create request to RentalCore
	req, err := http.NewRequest("POST", rentalCoreURL+endpoint, bytes.NewBuffer(jsonData))
	if err != nil {
		log.Printf("Failed to create RentalCore request: %v", err)
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to forward to RentalCore"})
		return
	}

	req.Header.Set("Content-Type", "application/json")

	// Send request
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		log.Printf("Failed to reach RentalCore: %v", err)
		respondJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "RentalCore unavailable"})
		return
	}
	defer resp.Body.Close()

	// Read response
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to read RentalCore response"})
		return
	}

	// Forward response
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(resp.StatusCode)
	w.Write(body)
}

// GetScanHistory returns scan event history
func GetScanHistory(w http.ResponseWriter, r *http.Request) {
	limit := parseInt(r.URL.Query().Get("limit"), 50)
	deviceID := r.URL.Query().Get("device_id")

	db := repository.GetSQLDB()
	qb := NewQueryBuilder()
	query := `SELECT scan_id, scan_code, device_id, action, success, timestamp
	          FROM scan_events WHERE 1=1`
	args := []interface{}{}

	if deviceID != "" {
		query += " AND device_id = " + qb.NextPlaceholder()
		args = append(args, deviceID)
	}

	query += " ORDER BY timestamp DESC LIMIT " + qb.NextPlaceholder()
	args = append(args, limit)

	rows, err := db.Query(query, args...)
	if err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	defer rows.Close()

	scans := []map[string]interface{}{}
	for rows.Next() {
		var s models.ScanEvent
		rows.Scan(&s.ScanID, &s.ScanCode, &s.DeviceID, &s.Action, &s.Success, &s.Timestamp)
		scans = append(scans, map[string]interface{}{
			"scan_id":   s.ScanID,
			"scan_code": s.ScanCode,
			"device_id": s.DeviceID,
			"action":    s.Action,
			"success":   s.Success,
			"timestamp": s.Timestamp,
		})
	}

	respondJSON(w, http.StatusOK, scans)
}

// GetDevices returns a list of devices with filters
func GetDevices(w http.ResponseWriter, r *http.Request) {
	status := r.URL.Query().Get("status")
	zoneID := r.URL.Query().Get("zone_id")
	// Get configurable default limit from settings
	defaultLimit := services.GetDeviceLimit()
	limit := parseInt(r.URL.Query().Get("limit"), defaultLimit)

	db := repository.GetSQLDB()
	qb := NewQueryBuilder()
	query := `
		SELECT d.deviceID, d.productID, d.serialnumber, d.status, d.barcode, d.qr_code,
		       d.zone_id, d.condition_rating, d.usage_hours, d.label_path,
		       COALESCE(p.name, '') as product_name,
		       COALESCE(z.name, '') as zone_name,
		       COALESCE(z.code, '') as zone_code,
		       COALESCE(c.name, '') as case_name,
		       COALESCE(CAST(jd.jobID AS TEXT), '') as job_number
		FROM devices d
		LEFT JOIN products p ON d.productID = p.productID
		LEFT JOIN storage_zones z ON d.zone_id = z.zone_id
		LEFT JOIN devicescases dc ON d.deviceID = dc.deviceID
		LEFT JOIN cases c ON dc.caseID = c.caseID
		LEFT JOIN job_devices jd ON d.deviceID = jd.deviceID AND jd.pack_status IN ('packed', 'issued')
		WHERE 1=1`

	args := []interface{}{}
	if status != "" {
		query += " AND d.status = " + qb.NextPlaceholder()
		args = append(args, status)
	}
	if zoneID != "" {
		query += " AND d.zone_id = " + qb.NextPlaceholder()
		args = append(args, zoneID)
	}

	query += " ORDER BY d.deviceID LIMIT " + qb.NextPlaceholder()
	args = append(args, limit)

	rows, err := db.Query(query, args...)
	if err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	defer rows.Close()

	// Response struct with clean JSON types
	type DeviceResponse struct {
		DeviceID        string  `json:"device_id"`
		ProductID       *int64  `json:"product_id,omitempty"`
		ProductName     string  `json:"product_name,omitempty"`
		SerialNumber    *string `json:"serial_number,omitempty"`
		Barcode         *string `json:"barcode,omitempty"`
		QRCode          *string `json:"qr_code,omitempty"`
		Status          string  `json:"status"`
		ZoneID          *int64  `json:"zone_id,omitempty"`
		ZoneName        string  `json:"zone_name,omitempty"`
		ZoneCode        string  `json:"zone_code,omitempty"`
		CaseName        string  `json:"case_name,omitempty"`
		JobNumber       string  `json:"job_number,omitempty"`
		ConditionRating float64 `json:"condition_rating"`
		UsageHours      float64 `json:"usage_hours"`
		LabelPath       *string `json:"label_path,omitempty"`
	}

	devices := []DeviceResponse{}
	for rows.Next() {
		var d models.DeviceWithDetails
		var caseName, jobNumber string
		if err := rows.Scan(&d.DeviceID, &d.ProductID, &d.SerialNumber, &d.Status, &d.Barcode, &d.QRCode,
			&d.ZoneID, &d.ConditionRating, &d.UsageHours, &d.LabelPath, &d.ProductName, &d.ZoneName, &d.ZoneCode,
			&caseName, &jobNumber); err != nil {
			log.Printf("Error scanning device row: %v", err)
			continue
		}

		// Convert to clean response format
		resp := DeviceResponse{
			DeviceID:        d.DeviceID,
			ProductName:     d.ProductName,
			Status:          d.Status,
			ZoneName:        d.ZoneName,
			ZoneCode:        d.ZoneCode,
			CaseName:        caseName,
			JobNumber:       jobNumber,
			ConditionRating: d.ConditionRating,
			UsageHours:      d.UsageHours,
		}

		if d.ProductID.Valid {
			resp.ProductID = &d.ProductID.Int64
		}
		if d.SerialNumber.Valid {
			resp.SerialNumber = &d.SerialNumber.String
		}
		if d.Barcode.Valid {
			resp.Barcode = &d.Barcode.String
		}
		if d.QRCode.Valid {
			resp.QRCode = &d.QRCode.String
		}
		if d.ZoneID.Valid {
			resp.ZoneID = &d.ZoneID.Int64
		}
		if d.LabelPath.Valid {
			resp.LabelPath = &d.LabelPath.String
		}

		devices = append(devices, resp)
	}

	respondJSON(w, http.StatusOK, devices)
}

// GetDevice returns a single device by ID
func GetDevice(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	deviceID := vars["id"]

	db := repository.GetSQLDB()

	// Response struct with clean JSON types
	type DeviceResponse struct {
		DeviceID        string  `json:"device_id"`
		ProductID       *int64  `json:"product_id,omitempty"`
		ProductName     string  `json:"product_name,omitempty"`
		SerialNumber    *string `json:"serial_number,omitempty"`
		Barcode         *string `json:"barcode,omitempty"`
		QRCode          *string `json:"qr_code,omitempty"`
		Status          string  `json:"status"`
		ZoneID          *int64  `json:"zone_id,omitempty"`
		ZoneName        string  `json:"zone_name,omitempty"`
		ZoneCode        string  `json:"zone_code,omitempty"`
		CaseName        string  `json:"case_name,omitempty"`
		JobNumber       string  `json:"job_number,omitempty"`
		ConditionRating float64 `json:"condition_rating"`
		UsageHours      float64 `json:"usage_hours"`
		LabelPath       *string `json:"label_path,omitempty"`
	}

	var device models.DeviceWithDetails
	var caseName, jobNumber string
	err := db.QueryRow(`
		SELECT d.deviceID, d.productID, d.serialnumber, d.status, d.barcode, d.qr_code,
		       d.zone_id, d.condition_rating, d.usage_hours, d.label_path,
		       COALESCE(p.name, '') as product_name,
		       COALESCE(z.name, '') as zone_name,
		       COALESCE(z.code, '') as zone_code,
		       COALESCE(c.name, '') as case_name,
		       COALESCE(CAST(jd.jobID AS TEXT), '') as job_number
		FROM devices d
		LEFT JOIN products p ON d.productID = p.productID
		LEFT JOIN storage_zones z ON d.zone_id = z.zone_id
		LEFT JOIN devicescases dc ON d.deviceID = dc.deviceID
		LEFT JOIN cases c ON dc.caseID = c.caseID
		LEFT JOIN job_devices jd ON d.deviceID = jd.deviceID AND jd.pack_status IN ('packed', 'issued')
		WHERE d.deviceID = $1
	`, deviceID).Scan(&device.DeviceID, &device.ProductID, &device.SerialNumber, &device.Status,
		&device.Barcode, &device.QRCode, &device.ZoneID, &device.ConditionRating, &device.UsageHours, &device.LabelPath,
		&device.ProductName, &device.ZoneName, &device.ZoneCode, &caseName, &jobNumber)

	if err == sql.ErrNoRows {
		respondJSON(w, http.StatusNotFound, map[string]string{"error": "Device not found"})
		return
	}
	if err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	// Convert to clean response format
	resp := DeviceResponse{
		DeviceID:        device.DeviceID,
		ProductName:     device.ProductName,
		Status:          device.Status,
		ZoneName:        device.ZoneName,
		ZoneCode:        device.ZoneCode,
		CaseName:        caseName,
		JobNumber:       jobNumber,
		ConditionRating: device.ConditionRating,
		UsageHours:      device.UsageHours,
	}

	if device.ProductID.Valid {
		resp.ProductID = &device.ProductID.Int64
	}
	if device.SerialNumber.Valid {
		resp.SerialNumber = &device.SerialNumber.String
	}
	if device.Barcode.Valid {
		resp.Barcode = &device.Barcode.String
	}
	if device.QRCode.Valid {
		resp.QRCode = &device.QRCode.String
	}
	if device.ZoneID.Valid {
		resp.ZoneID = &device.ZoneID.Int64
	}
	if device.LabelPath.Valid {
		resp.LabelPath = &device.LabelPath.String
	}

	respondJSON(w, http.StatusOK, resp)
}

// UpdateDeviceStatus updates device status
func UpdateDeviceStatus(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	deviceID := vars["id"]

	var req struct {
		Status string `json:"status"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid request"})
		return
	}

	db := repository.GetSQLDB()
	_, err := db.Exec(`UPDATE devices SET status = $1 WHERE deviceID = $2`, req.Status, deviceID)
	if err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	respondJSON(w, http.StatusOK, map[string]string{"message": "Status updated"})
}

// GetDeviceMovements returns movement history for a device
func GetDeviceMovements(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	deviceID := vars["id"]

	db := repository.GetSQLDB()
	rows, err := db.Query(`
		SELECT movement_id, device_id, action, from_zone_id, to_zone_id, to_job_id, timestamp
		FROM device_movements
		WHERE device_id = $1
		ORDER BY timestamp DESC
		LIMIT 50
	`, deviceID)
	if err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	defer rows.Close()

	movements := []models.DeviceMovement{}
	for rows.Next() {
		var m models.DeviceMovement
		rows.Scan(&m.MovementID, &m.DeviceID, &m.Action, &m.FromZoneID, &m.ToZoneID, &m.ToJobID, &m.Timestamp)
		movements = append(movements, m)
	}

	respondJSON(w, http.StatusOK, movements)
}

// GetZones returns all zones
func GetZones(w http.ResponseWriter, r *http.Request) {
	db := repository.GetSQLDB()
	rows, err := db.Query(`
		SELECT z.zone_id, z.code, z.barcode, z.name, z.type, z.description, z.parent_zone_id, z.capacity, z.is_active
		FROM storage_zones z
		LEFT JOIN storage_zones parent ON parent.zone_id = z.parent_zone_id
		WHERE z.is_active = TRUE
		  AND (z.parent_zone_id IS NULL OR parent.is_active = TRUE)
		ORDER BY z.name
	`)
	if err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	defer rows.Close()

	// Response struct with clean JSON types
	type ZoneResponse struct {
		ZoneID       int64   `json:"zone_id"`
		Code         string  `json:"code"`
		Barcode      *string `json:"barcode,omitempty"`
		Name         string  `json:"name"`
		Type         string  `json:"type"`
		Description  *string `json:"description,omitempty"`
		ParentZoneID *int64  `json:"parent_zone_id,omitempty"`
		Capacity     *int64  `json:"capacity,omitempty"`
		IsActive     bool    `json:"is_active"`
	}

	zones := []ZoneResponse{}
	for rows.Next() {
		var z models.Zone
		if err := rows.Scan(&z.ZoneID, &z.Code, &z.Barcode, &z.Name, &z.Type, &z.Description, &z.ParentZoneID, &z.Capacity, &z.IsActive); err != nil {
			log.Printf("Error scanning zone row: %v", err)
			continue
		}

		// Convert to clean response format
		resp := ZoneResponse{
			ZoneID:   z.ZoneID,
			Code:     z.Code,
			Name:     z.Name,
			Type:     z.Type,
			IsActive: z.IsActive,
		}

		if z.Barcode.Valid {
			resp.Barcode = &z.Barcode.String
		}
		if z.Description.Valid {
			resp.Description = &z.Description.String
		}
		if z.ParentZoneID.Valid {
			resp.ParentZoneID = &z.ParentZoneID.Int64
		}
		if z.Capacity.Valid {
			resp.Capacity = &z.Capacity.Int64
		}

		zones = append(zones, resp)
	}

	respondJSON(w, http.StatusOK, zones)
}

// CreateZone creates a new zone with automatic code generation
func CreateZone(w http.ResponseWriter, r *http.Request) {
	// Input struct for API requests
	var input struct {
		Code         *string `json:"code"` // Optional - will be auto-generated if not provided
		Name         *string `json:"name"` // Optional for shelves - will be auto-generated
		Type         string  `json:"type"`
		Description  *string `json:"description"`
		ParentZoneID *int64  `json:"parent_zone_id"`
		Capacity     *int64  `json:"capacity"`
		IsActive     bool    `json:"is_active"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		log.Printf("Zone creation error - JSON decode: %v", err)
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid request: " + err.Error()})
		return
	}

	zoneService := services.NewZoneService()
	db := repository.GetSQLDB()

	// Auto-generate name for shelves if not provided
	var zoneName string
	if input.Name != nil && *input.Name != "" {
		zoneName = *input.Name
	} else if input.Type == "shelf" {
		// Generate automatic name for shelves (Fächer)
		generatedName, err := zoneService.GenerateShelfName(input.ParentZoneID)
		if err != nil {
			log.Printf("Shelf name generation error: %v", err)
			respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to generate shelf name"})
			return
		}
		zoneName = generatedName
		log.Printf("Auto-generated shelf name: %s", zoneName)
	} else {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "Name is required for non-shelf zones"})
		return
	}

	// Auto-generate code if not provided
	var zoneCode string
	if input.Code != nil && *input.Code != "" {
		zoneCode = *input.Code
	} else {
		generatedCode, err := zoneService.GenerateZoneCode(zoneName, input.Type, input.ParentZoneID)
		if err != nil {
			log.Printf("Zone code generation error: %v", err)
			respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to generate zone code"})
			return
		}
		zoneCode = generatedCode
		log.Printf("Auto-generated zone code: %s for zone: %s", zoneCode, zoneName)
	}

	// Convert pointers to proper SQL values
	var description, parentZoneID, capacity interface{}
	if input.Description != nil && *input.Description != "" {
		description = *input.Description
	} else {
		description = nil
	}
	if input.ParentZoneID != nil {
		parentZoneID = *input.ParentZoneID
	} else {
		parentZoneID = nil
	}
	if input.Capacity != nil {
		capacity = *input.Capacity
	} else {
		capacity = nil
	}

	// Generate barcode for shelves
	var barcode interface{}
	if input.Type == "shelf" {
		// Generate barcode - will be updated with actual ID after insert
		barcode = nil // Will be set after we get the ID
	} else {
		barcode = nil
	}

	var id int64
	err := db.QueryRow(`
		INSERT INTO storage_zones (code, barcode, name, type, description, parent_zone_id, capacity, is_active)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING zone_id
	`, zoneCode, barcode, zoneName, input.Type, description, parentZoneID, capacity, input.IsActive).Scan(&id)
	if err != nil {
		log.Printf("Zone creation error - SQL insert: %v", err)
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	// Generate and update barcode for shelves
	var generatedBarcode *string
	if input.Type == "shelf" {
		barcodeStr := fmt.Sprintf("FACH-%08d", id)
		_, err := db.Exec(`UPDATE storage_zones SET barcode = $1 WHERE zone_id = $2`, barcodeStr, id)
		if err != nil {
			log.Printf("Failed to update barcode: %v", err)
		} else {
			generatedBarcode = &barcodeStr
		}
	}

	// Return the created zone with clean JSON
	type ZoneResponse struct {
		ZoneID       int64   `json:"zone_id"`
		Code         string  `json:"code"`
		Barcode      *string `json:"barcode,omitempty"`
		Name         string  `json:"name"`
		Type         string  `json:"type"`
		Description  *string `json:"description,omitempty"`
		ParentZoneID *int64  `json:"parent_zone_id,omitempty"`
		Capacity     *int64  `json:"capacity,omitempty"`
		IsActive     bool    `json:"is_active"`
	}

	zone := ZoneResponse{
		ZoneID:       id,
		Code:         zoneCode,
		Barcode:      generatedBarcode,
		Name:         zoneName,
		Type:         input.Type,
		Description:  input.Description,
		ParentZoneID: input.ParentZoneID,
		Capacity:     input.Capacity,
		IsActive:     input.IsActive,
	}

	log.Printf("Zone created successfully: %s (Code: %s, ID: %d)", zoneName, zoneCode, id)
	respondJSON(w, http.StatusCreated, zone)
}

// GetZone returns a single zone with details (subzones, devices, breadcrumb)
func GetZone(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	idStr := vars["id"]

	zoneID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid zone ID"})
		return
	}

	zoneService := services.NewZoneService()
	details, err := zoneService.GetZoneDetails(zoneID)
	if err != nil {
		log.Printf("Error getting zone details: %v", err)
		respondJSON(w, http.StatusNotFound, map[string]string{"error": "Zone not found"})
		return
	}

	respondJSON(w, http.StatusOK, details)
}

// GetZoneDevices returns all devices in a specific zone
func GetZoneDevices(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	zoneID := vars["id"]

	db := repository.GetSQLDB()
	rows, err := db.Query(`
		SELECT d.deviceID, d.productID, d.serialnumber, d.status, d.barcode, d.qr_code,
		       d.condition_rating, d.usage_hours,
		       COALESCE(p.name, '') as product_name,
		       COALESCE(m.name, '') as manufacturer,
		       COALESCE(b.name, '') as model,
		       COALESCE(z.code, '') as zone_code
		FROM devices d
		LEFT JOIN products p ON d.productID = p.productID
		LEFT JOIN manufacturer m ON p.manufacturerID = m.manufacturerID
		LEFT JOIN brands b ON p.brandID = b.brandID
		LEFT JOIN storage_zones z ON d.zone_id = z.zone_id
		WHERE d.zone_id = $1 AND d.status = 'in_storage'
		ORDER BY p.name, d.deviceID
	`, zoneID)

	if err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	defer rows.Close()

	type DeviceInZone struct {
		DeviceID        string  `json:"device_id"`
		ProductID       *int    `json:"product_id"`
		ProductName     string  `json:"product_name"`
		SerialNumber    *string `json:"serial_number,omitempty"`
		Manufacturer    string  `json:"manufacturer,omitempty"`
		Model           string  `json:"model,omitempty"`
		Status          string  `json:"status"`
		Barcode         *string `json:"barcode,omitempty"`
		QRCode          *string `json:"qr_code,omitempty"`
		ZoneCode        string  `json:"zone_code,omitempty"`
		ConditionRating float64 `json:"condition_rating"`
		UsageHours      float64 `json:"usage_hours"`
	}

	devices := []DeviceInZone{}
	for rows.Next() {
		var d DeviceInZone
		var productID sql.NullInt64
		var barcode, qrCode, serialNumber sql.NullString

		if err := rows.Scan(&d.DeviceID, &productID, &serialNumber, &d.Status, &barcode, &qrCode,
			&d.ConditionRating, &d.UsageHours, &d.ProductName, &d.Manufacturer, &d.Model, &d.ZoneCode); err != nil {
			log.Printf("Error scanning device row: %v", err)
			continue
		}

		if productID.Valid {
			pid := int(productID.Int64)
			d.ProductID = &pid
		}
		if barcode.Valid {
			d.Barcode = &barcode.String
		}
		if qrCode.Valid {
			d.QRCode = &qrCode.String
		}
		if serialNumber.Valid {
			d.SerialNumber = &serialNumber.String
		}

		devices = append(devices, d)
	}

	respondJSON(w, http.StatusOK, devices)
}

// GetZoneProducts returns all products (consumables/accessories) in a zone
func GetZoneProducts(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	zoneID := vars["id"]

	db := repository.GetSQLDB()

	type ProductInZone struct {
		ProductID    int     `json:"product_id"`
		ProductName  string  `json:"product_name"`
		Quantity     float64 `json:"quantity"`
		Unit         string  `json:"unit"`
		IsAccessory  bool    `json:"is_accessory"`
		IsConsumable bool    `json:"is_consumable"`
	}

	rows, err := db.Query(`
		SELECT pl.product_id, p.name, pl.quantity,
		       COALESCE(ct.abbreviation, ''),
		       COALESCE(p.is_accessory, FALSE),
		       COALESCE(p.is_consumable, FALSE)
		FROM product_locations pl
		LEFT JOIN products p ON pl.product_id = p.productID
		LEFT JOIN count_types ct ON p.count_type_id = ct.count_type_id
		WHERE pl.zone_id = $1 AND pl.quantity > 0
		ORDER BY p.name
	`, zoneID)

	if err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	defer rows.Close()

	products := []ProductInZone{}
	for rows.Next() {
		var prod ProductInZone
		if err := rows.Scan(&prod.ProductID, &prod.ProductName, &prod.Quantity, &prod.Unit, &prod.IsAccessory, &prod.IsConsumable); err != nil {
			log.Printf("Error scanning product row: %v", err)
			continue
		}
		products = append(products, prod)
	}

	respondJSON(w, http.StatusOK, products)
}

// UpdateZone updates a zone
func UpdateZone(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]

	var zone models.Zone
	if err := json.NewDecoder(r.Body).Decode(&zone); err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid request"})
		return
	}

	db := repository.GetSQLDB()
	_, err := db.Exec(`
		UPDATE storage_zones
		SET code = $1, name = $2, type = $3, description = $4, parent_zone_id = $5, capacity = $6
		WHERE zone_id = $7
	`, zone.Code, zone.Name, zone.Type, zone.Description, zone.ParentZoneID, zone.Capacity, id)
	if err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	respondJSON(w, http.StatusOK, map[string]string{"message": "Zone updated"})
}

// DeleteZone soft-deletes a zone
func DeleteZone(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]

	db := repository.GetSQLDB()
	_, err := db.Exec(`UPDATE storage_zones SET is_active = FALSE WHERE zone_id = $1`, id)
	if err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	respondJSON(w, http.StatusOK, map[string]string{"message": "Zone deleted"})
}

// GetZoneByBarcode finds a zone by barcode or code
func GetZoneByBarcode(w http.ResponseWriter, r *http.Request) {
	scanCode := r.URL.Query().Get("scan_code")
	if scanCode == "" {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "scan_code parameter required"})
		return
	}

	db := repository.GetSQLDB()
	var zone models.Zone
	err := db.QueryRow(`
		SELECT zone_id, code, barcode, name, type, description, parent_zone_id, capacity, is_active
		FROM storage_zones
		WHERE (barcode = $1 OR code = $2) AND is_active = TRUE
		LIMIT 1
	`, scanCode, scanCode).Scan(&zone.ZoneID, &zone.Code, &zone.Barcode, &zone.Name, &zone.Type,
		&zone.Description, &zone.ParentZoneID, &zone.Capacity, &zone.IsActive)

	if err == sql.ErrNoRows {
		respondJSON(w, http.StatusNotFound, map[string]string{"error": "Zone not found"})
		return
	}
	if err != nil {
		log.Printf("Error finding zone by barcode: %v", err)
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	// Response struct with clean JSON types
	type ZoneResponse struct {
		ZoneID       int64   `json:"zone_id"`
		Code         string  `json:"code"`
		Barcode      *string `json:"barcode,omitempty"`
		Name         string  `json:"name"`
		Type         string  `json:"type"`
		Description  *string `json:"description,omitempty"`
		ParentZoneID *int64  `json:"parent_zone_id,omitempty"`
		Capacity     *int64  `json:"capacity,omitempty"`
		IsActive     bool    `json:"is_active"`
	}

	resp := ZoneResponse{
		ZoneID:   zone.ZoneID,
		Code:     zone.Code,
		Name:     zone.Name,
		Type:     zone.Type,
		IsActive: zone.IsActive,
	}

	if zone.Barcode.Valid {
		resp.Barcode = &zone.Barcode.String
	}
	if zone.Description.Valid {
		resp.Description = &zone.Description.String
	}
	if zone.ParentZoneID.Valid {
		resp.ParentZoneID = &zone.ParentZoneID.Int64
	}
	if zone.Capacity.Valid {
		resp.Capacity = &zone.Capacity.Int64
	}

	respondJSON(w, http.StatusOK, resp)
}

// GetJobs returns jobs filtered by status
func GetJobs(w http.ResponseWriter, r *http.Request) {
	status := r.URL.Query().Get("status")

	db := repository.GetSQLDB()
	qb := NewQueryBuilder()
	query := `
		SELECT j.jobID,
		       COALESCE(j.job_code, CONCAT('JOB', LPAD(CAST(j.jobID AS TEXT), 6, '0'))) AS job_code,
		       j.description, j.startDate, j.endDate, s.status,
		       COALESCE(c.firstName, '') as customer_first_name,
		       COALESCE(c.lastName, '') as customer_last_name,
		       COUNT(DISTINCT jd.deviceID) as device_count
		FROM jobs j
		LEFT JOIN status s ON j.statusID = s.statusID
		LEFT JOIN customers c ON j.customerID = c.customerID
		LEFT JOIN job_devices jd ON j.jobID = jd.jobID
		WHERE 1=1`

	args := []interface{}{}
	// Exclude completed/cancelled statuses regardless of language (DE+EN)
	excludedStatuses := []interface{}{"Abgeschlossen", "Abgerechnet", "Storniert", "canceled", "paid"}
	placeholders := make([]string, len(excludedStatuses))
	for i := range excludedStatuses {
		placeholders[i] = qb.NextPlaceholder()
	}
	query += " AND (s.status IS NULL OR s.status NOT IN (" + strings.Join(placeholders, ",") + "))"
	args = append(args, excludedStatuses...)
	if status != "" && status != "open" {
		query += " AND s.status = " + qb.NextPlaceholder()
		args = append(args, status)
	}

	query += " GROUP BY j.jobID, j.job_code, j.description, j.startDate, j.endDate, s.status, c.firstName, c.lastName ORDER BY j.startDate ASC"

	rows, err := db.Query(query, args...)
	if err != nil {
		log.Printf("Error getting jobs: %v", err)
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	defer rows.Close()

	type JobResponse struct {
		JobID             int     `json:"job_id"`
		JobCode           string  `json:"job_code"`
		Description       *string `json:"description,omitempty"`
		StartDate         *string `json:"start_date,omitempty"`
		EndDate           *string `json:"end_date,omitempty"`
		Status            string  `json:"status"`
		CustomerFirstName string  `json:"customer_first_name,omitempty"`
		CustomerLastName  string  `json:"customer_last_name,omitempty"`
		DeviceCount       int     `json:"device_count"`
	}

	jobs := []JobResponse{}
	for rows.Next() {
		var j JobResponse
		var description, startDate, endDate sql.NullString

		if err := rows.Scan(&j.JobID, &j.JobCode, &description, &startDate, &endDate, &j.Status,
			&j.CustomerFirstName, &j.CustomerLastName, &j.DeviceCount); err != nil {
			log.Printf("Error scanning job row: %v", err)
			continue
		}

		if description.Valid {
			j.Description = &description.String
		}
		if startDate.Valid {
			j.StartDate = &startDate.String
		}
		if endDate.Valid {
			j.EndDate = &endDate.String
		}

		jobs = append(jobs, j)
	}

	respondJSON(w, http.StatusOK, jobs)
}

func GetJobSummary(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	jobIDStr := vars["id"]
	jobID, err := strconv.Atoi(jobIDStr)
	if err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid job ID"})
		return
	}

	db := repository.GetSQLDB()

	// Get job details
	var (
		jobCode                             sql.NullString
		description, startDate, endDate     sql.NullString
		status                              string
		customerFirstName, customerLastName string
	)

	err = db.QueryRow(`
		SELECT j.job_code, j.description, j.startDate, j.endDate, s.status,
		       COALESCE(c.firstName, '') as customer_first_name,
		       COALESCE(c.lastName, '') as customer_last_name
		FROM jobs j
		LEFT JOIN status s ON j.statusID = s.statusID
		LEFT JOIN customers c ON j.customerID = c.customerID
		WHERE j.jobID = $1
	`, jobID).Scan(&jobCode, &description, &startDate, &endDate, &status, &customerFirstName, &customerLastName)

	if err == sql.ErrNoRows {
		respondJSON(w, http.StatusNotFound, map[string]string{"error": "Job not found"})
		return
	}
	if err != nil {
		log.Printf("Error getting job: %v", err)
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	// Get devices for this job with their current status
	rows, err := db.Query(`
		SELECT jd.deviceID, d.status, d.barcode, d.qr_code,
		       COALESCE(p.name, '') as product_name,
		       COALESCE(z.name, '') as zone_name,
		       jd.pack_status
		FROM job_devices jd
		LEFT JOIN devices d ON jd.deviceID = d.deviceID
		LEFT JOIN products p ON d.productID = p.productID
		LEFT JOIN storage_zones z ON d.zone_id = z.zone_id
		WHERE jd.jobID = $1
		ORDER BY p.name, jd.deviceID
	`, jobID)

	if err != nil {
		log.Printf("Error getting job devices: %v", err)
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	defer rows.Close()

	type JobDevice struct {
		DeviceID    string  `json:"device_id"`
		Status      string  `json:"status"`
		ProductName string  `json:"product_name"`
		ZoneName    string  `json:"zone_name,omitempty"`
		Barcode     *string `json:"barcode,omitempty"`
		QRCode      *string `json:"qr_code,omitempty"`
		PackStatus  string  `json:"pack_status"`
		Scanned     bool    `json:"scanned"` // true if pack_status is 'issued' or device status is 'on_job'
	}

	devices := []JobDevice{}
	for rows.Next() {
		var jd JobDevice
		var barcode, qrCode sql.NullString

		if err := rows.Scan(&jd.DeviceID, &jd.Status, &barcode, &qrCode,
			&jd.ProductName, &jd.ZoneName, &jd.PackStatus); err != nil {
			log.Printf("Error scanning device row: %v", err)
			continue
		}

		if barcode.Valid {
			jd.Barcode = &barcode.String
		}
		if qrCode.Valid {
			jd.QRCode = &qrCode.String
		}

		// Mark as scanned only if device is currently on_job
		// If device is back in storage, it should not be highlighted as scanned
		jd.Scanned = jd.Status == "on_job"

		devices = append(devices, jd)
	}

	jobCodeValue := fmt.Sprintf("JOB%06d", jobID)
	if jobCode.Valid && jobCode.String != "" {
		jobCodeValue = jobCode.String
	}

	response := map[string]interface{}{
		"job_id":              jobID,
		"job_code":            jobCodeValue,
		"status":              status,
		"customer_first_name": customerFirstName,
		"customer_last_name":  customerLastName,
		"devices":             devices,
	}

	if description.Valid {
		response["description"] = description.String
	}
	if startDate.Valid {
		response["start_date"] = startDate.String
	}
	if endDate.Valid {
		response["end_date"] = endDate.String
	}

	respondJSON(w, http.StatusOK, response)
}

func CompleteJob(w http.ResponseWriter, r *http.Request) {
	respondJSON(w, http.StatusOK, map[string]string{"message": "Job completed"})
}

func GetCases(w http.ResponseWriter, r *http.Request) {
	db := repository.GetSQLDB()

	search := strings.TrimSpace(r.URL.Query().Get("search"))
	status := strings.TrimSpace(r.URL.Query().Get("status"))
	// Get configurable limit from settings
	limit := services.GetCaseLimit()

	qb := NewQueryBuilder()
	query := `
		SELECT
			c.caseID,
			c.name,
			c.description,
			c.status,
			c.width,
			c.height,
			c.depth,
			c.weight,
			c.zone_id,
			c.label_path,
			COALESCE(z.name, '') AS zone_name,
			COALESCE(z.code, '') AS zone_code,
			COUNT(dc.deviceID) AS device_count
		FROM cases c
		LEFT JOIN devicescases dc ON c.caseID = dc.caseID
		LEFT JOIN storage_zones z ON c.zone_id = z.zone_id
		WHERE 1=1
	`

	args := []interface{}{}

	if search != "" {
		query += " AND (c.name LIKE " + qb.NextPlaceholder() + " OR c.description LIKE " + qb.NextPlaceholder() + ")"
		searchTerm := "%" + search + "%"
		args = append(args, searchTerm, searchTerm)
	}

	if status != "" {
		query += " AND c.status = " + qb.NextPlaceholder()
		args = append(args, status)
	}

	query += `
		GROUP BY c.caseID, c.name, c.description, c.status, c.width, c.height, c.depth, c.weight, c.zone_id, c.label_path, zone_name, zone_code
		ORDER BY c.name ASC
		LIMIT ` + qb.NextPlaceholder() + `
	`
	args = append(args, limit)

	rows, err := db.Query(query, args...)
	if err != nil {
		log.Printf("GetCases query error: %v", err)
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to load cases"})
		return
	}
	defer rows.Close()

	cases := []CaseSummary{}

	for rows.Next() {
		var item CaseSummary
		var description sql.NullString
		var width, height, depth, weight sql.NullFloat64
		var zoneID sql.NullInt64
		var labelPath sql.NullString
		var zoneName, zoneCode sql.NullString
		var deviceCount sql.NullInt64

		err := rows.Scan(
			&item.CaseID,
			&item.Name,
			&description,
			&item.Status,
			&width,
			&height,
			&depth,
			&weight,
			&zoneID,
			&labelPath,
			&zoneName,
			&zoneCode,
			&deviceCount,
		)
		if err != nil {
			log.Printf("GetCases scan error: %v", err)
			continue
		}

		descPtr := ptrString(description)
		if descPtr != nil && strings.TrimSpace(*descPtr) == "" {
			descPtr = nil
		}
		item.Description = descPtr
		item.Width = ptrFloat64(width)
		item.Height = ptrFloat64(height)
		item.Depth = ptrFloat64(depth)
		item.Weight = ptrFloat64(weight)
		item.ZoneID = ptrInt(zoneID)
		item.LabelPath = ptrString(labelPath)

		if zoneName.Valid && strings.TrimSpace(zoneName.String) != "" {
			item.ZoneName = ptrString(zoneName)
		}
		if zoneCode.Valid && strings.TrimSpace(zoneCode.String) != "" {
			item.ZoneCode = ptrString(zoneCode)
		}
		if deviceCount.Valid {
			item.DeviceCount = int(deviceCount.Int64)
		}

		cases = append(cases, item)
	}

	respondJSON(w, http.StatusOK, map[string]interface{}{
		"cases": cases,
		"meta": map[string]interface{}{
			"count": len(cases),
		},
	})
}

func GetCase(w http.ResponseWriter, r *http.Request) {
	db := repository.GetSQLDB()
	vars := mux.Vars(r)
	caseID := vars["id"]

	query := `
		SELECT
			c.caseID,
			c.name,
			c.description,
			c.status,
			c.width,
			c.height,
			c.depth,
			c.weight,
			c.zone_id,
			c.label_path,
			COALESCE(z.name, '') AS zone_name,
			COALESCE(z.code, '') AS zone_code,
			COUNT(dc.deviceID) AS device_count
		FROM cases c
		LEFT JOIN devicescases dc ON c.caseID = dc.caseID
		LEFT JOIN storage_zones z ON c.zone_id = z.zone_id
		WHERE c.caseID = $1
		GROUP BY c.caseID, c.name, c.description, c.status, c.width, c.height, c.depth, c.weight, c.zone_id, c.label_path, zone_name, zone_code
	`

	var description sql.NullString
	var width, height, depth, weight sql.NullFloat64
	var zoneID sql.NullInt64
	var labelPath sql.NullString
	var zoneName, zoneCode sql.NullString
	var deviceCount sql.NullInt64

	type CaseDetail struct {
		CaseID      int      `json:"case_id"`
		Name        string   `json:"name"`
		Description *string  `json:"description,omitempty"`
		Status      string   `json:"status"`
		Width       *float64 `json:"width,omitempty"`
		Height      *float64 `json:"height,omitempty"`
		Depth       *float64 `json:"depth,omitempty"`
		Weight      *float64 `json:"weight,omitempty"`
		ZoneID      *int     `json:"zone_id,omitempty"`
		ZoneName    *string  `json:"zone_name,omitempty"`
		ZoneCode    *string  `json:"zone_code,omitempty"`
		DeviceCount int      `json:"device_count"`
		LabelPath   *string  `json:"label_path,omitempty"`
	}

	var item CaseDetail

	err := db.QueryRow(query, caseID).Scan(
		&item.CaseID,
		&item.Name,
		&description,
		&item.Status,
		&width,
		&height,
		&depth,
		&weight,
		&zoneID,
		&labelPath,
		&zoneName,
		&zoneCode,
		&deviceCount,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			respondJSON(w, http.StatusNotFound, map[string]string{"error": "Case not found"})
			return
		}
		log.Printf("GetCase query error: %v", err)
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to load case"})
		return
	}

	if description.Valid {
		item.Description = &description.String
	}
	if width.Valid {
		value := width.Float64
		item.Width = &value
	}
	if height.Valid {
		value := height.Float64
		item.Height = &value
	}
	if depth.Valid {
		value := depth.Float64
		item.Depth = &value
	}
	if weight.Valid {
		value := weight.Float64
		item.Weight = &value
	}
	if zoneID.Valid {
		value := int(zoneID.Int64)
		item.ZoneID = &value
	}
	if zoneName.Valid && zoneName.String != "" {
		val := zoneName.String
		item.ZoneName = &val
	}
	if zoneCode.Valid && zoneCode.String != "" {
		val := zoneCode.String
		item.ZoneCode = &val
	}
	if labelPath.Valid && labelPath.String != "" {
		val := labelPath.String
		item.LabelPath = &val
	}
	if deviceCount.Valid {
		item.DeviceCount = int(deviceCount.Int64)
	}

	respondJSON(w, http.StatusOK, item)
}

func GetCaseContents(w http.ResponseWriter, r *http.Request) {
	db := repository.GetSQLDB()
	vars := mux.Vars(r)
	caseID := vars["id"]

	query := `
		SELECT 
			d.deviceID,
			d.status,
			d.serialnumber,
			d.barcode,
			d.zone_id,
			COALESCE(z.name, '') AS zone_name,
			COALESCE(z.code, '') AS zone_code,
			COALESCE(p.name, '') AS product_name
		FROM devicescases dc
		INNER JOIN devices d ON dc.deviceID = d.deviceID
		LEFT JOIN products p ON d.productID = p.productID
		LEFT JOIN storage_zones z ON d.zone_id = z.zone_id
		WHERE dc.caseID = $1
		ORDER BY d.deviceID ASC
	`

	rows, err := db.Query(query, caseID)
	if err != nil {
		log.Printf("GetCaseContents query error: %v", err)
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to load case devices"})
		return
	}
	defer rows.Close()

	type CaseDevice struct {
		DeviceID     string  `json:"device_id"`
		Status       string  `json:"status"`
		SerialNumber *string `json:"serial_number,omitempty"`
		Barcode      *string `json:"barcode,omitempty"`
		ProductName  *string `json:"product_name,omitempty"`
		ZoneID       *int    `json:"zone_id,omitempty"`
		ZoneName     *string `json:"zone_name,omitempty"`
		ZoneCode     *string `json:"zone_code,omitempty"`
	}

	devices := []CaseDevice{}

	for rows.Next() {
		var device CaseDevice
		var serialNumber, barcode, productName sql.NullString
		var zoneID sql.NullInt64
		var zoneName, zoneCode sql.NullString

		err := rows.Scan(
			&device.DeviceID,
			&device.Status,
			&serialNumber,
			&barcode,
			&zoneID,
			&zoneName,
			&zoneCode,
			&productName,
		)
		if err != nil {
			log.Printf("GetCaseContents scan error: %v", err)
			continue
		}

		if serialNumber.Valid && serialNumber.String != "" {
			val := serialNumber.String
			device.SerialNumber = &val
		}
		if barcode.Valid && barcode.String != "" {
			val := barcode.String
			device.Barcode = &val
		}
		if productName.Valid && productName.String != "" {
			val := productName.String
			device.ProductName = &val
		}
		if zoneID.Valid {
			value := int(zoneID.Int64)
			device.ZoneID = &value
		}
		if zoneName.Valid && zoneName.String != "" {
			val := zoneName.String
			device.ZoneName = &val
		}
		if zoneCode.Valid && zoneCode.String != "" {
			val := zoneCode.String
			device.ZoneCode = &val
		}

		devices = append(devices, device)
	}

	respondJSON(w, http.StatusOK, devices)
}

// CreateCase creates a new case
func CreateCase(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name        string   `json:"name"`
		Description *string  `json:"description"`
		Width       *float64 `json:"width"`
		Height      *float64 `json:"height"`
		Depth       *float64 `json:"depth"`
		Weight      *float64 `json:"weight"`
		Status      string   `json:"status"`
		ZoneID      *int     `json:"zone_id"`
		Barcode     *string  `json:"barcode"`
		RFIDTag     *string  `json:"rfid_tag"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid request body"})
		return
	}

	// Validation
	if strings.TrimSpace(req.Name) == "" {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "Name is required"})
		return
	}
	if req.Status == "" {
		req.Status = "free" // Default status
	}
	if req.Status != "free" && req.Status != "rented" && req.Status != "maintance" {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid status"})
		return
	}

	db := repository.GetSQLDB()
	var caseID int64
	err := db.QueryRow(`
		INSERT INTO cases (name, description, width, height, depth, weight, status, zone_id, barcode, rfid_tag)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		RETURNING caseID
	`, req.Name, req.Description, req.Width, req.Height, req.Depth, req.Weight, req.Status, req.ZoneID, req.Barcode, req.RFIDTag).Scan(&caseID)

	if err != nil {
		log.Printf("CreateCase error: %v", err)
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to create case"})
		return
	}

	respondJSON(w, http.StatusCreated, map[string]interface{}{
		"case_id": caseID,
		"message": "Case created successfully",
	})
}

// UpdateCase updates an existing case
func UpdateCase(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	caseID := vars["id"]

	var req struct {
		Name        string   `json:"name"`
		Description *string  `json:"description"`
		Width       *float64 `json:"width"`
		Height      *float64 `json:"height"`
		Depth       *float64 `json:"depth"`
		Weight      *float64 `json:"weight"`
		Status      string   `json:"status"`
		ZoneID      *int     `json:"zone_id"`
		Barcode     *string  `json:"barcode"`
		RFIDTag     *string  `json:"rfid_tag"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid request body"})
		return
	}

	// Validation
	if strings.TrimSpace(req.Name) == "" {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "Name is required"})
		return
	}
	if req.Status != "free" && req.Status != "rented" && req.Status != "maintance" && req.Status != "" {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid status"})
		return
	}

	db := repository.GetSQLDB()
	result, err := db.Exec(`
		UPDATE cases
		SET name = $1, description = $2, width = $3, height = $4, depth = $5,
		    weight = $6, status = $7, zone_id = $8, barcode = $9, rfid_tag = $10
		WHERE caseID = $11
	`, req.Name, req.Description, req.Width, req.Height, req.Depth, req.Weight, req.Status, req.ZoneID, req.Barcode, req.RFIDTag, caseID)

	if err != nil {
		log.Printf("UpdateCase error: %v", err)
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to update case"})
		return
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		log.Printf("UpdateCase RowsAffected error: %v", err)
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to verify update"})
		return
	}

	if rowsAffected == 0 {
		respondJSON(w, http.StatusNotFound, map[string]string{"error": "Case not found"})
		return
	}

	respondJSON(w, http.StatusOK, map[string]string{"message": "Case updated successfully"})
}

// DeleteCase deletes a case
func DeleteCase(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	caseID := vars["id"]

	db := repository.GetSQLDB()

	// Check if case has devices
	var deviceCount int
	err := db.QueryRow("SELECT COUNT(*) FROM devicescases WHERE caseID = $1", caseID).Scan(&deviceCount)
	if err != nil {
		log.Printf("DeleteCase check devices error: %v", err)
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to check case devices"})
		return
	}

	if deviceCount > 0 {
		respondJSON(w, http.StatusBadRequest, map[string]string{
			"error":   "Cannot delete case with devices",
			"message": fmt.Sprintf("Case contains %d device(s). Please remove devices first.", deviceCount),
		})
		return
	}

	// Delete the case
	result, err := db.Exec("DELETE FROM cases WHERE caseID = $1", caseID)
	if err != nil {
		log.Printf("DeleteCase error: %v", err)
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to delete case"})
		return
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		log.Printf("DeleteCase RowsAffected error: %v", err)
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to verify deletion"})
		return
	}

	if rowsAffected == 0 {
		respondJSON(w, http.StatusNotFound, map[string]string{"error": "Case not found"})
		return
	}

	respondJSON(w, http.StatusOK, map[string]string{"message": "Case deleted successfully"})
}

// AddDevicesToCase adds multiple devices to a case
// POST /api/v1/cases/{id}/devices
// Body: {"device_ids": ["DEV001", "DEV002"]}
func AddDevicesToCase(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	caseIDStr := vars["id"]

	caseID, err := strconv.Atoi(caseIDStr)
	if err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid case ID"})
		return
	}

	var req struct {
		DeviceIDs []string `json:"device_ids"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid request body"})
		return
	}

	if len(req.DeviceIDs) == 0 {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "No device IDs provided"})
		return
	}

	db := repository.GetSQLDB()

	// Check if case exists
	var exists int
	err = db.QueryRow("SELECT COUNT(*) FROM cases WHERE caseID = $1", caseID).Scan(&exists)
	if err != nil {
		log.Printf("AddDevicesToCase check case error: %v", err)
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to verify case"})
		return
	}

	if exists == 0 {
		respondJSON(w, http.StatusNotFound, map[string]string{"error": "Case not found"})
		return
	}

	// Add devices to case
	successCount := 0
	skippedCount := 0
	errors := []string{}

	for _, deviceID := range req.DeviceIDs {
		// Check if device exists
		var deviceExists int
		err = db.QueryRow("SELECT COUNT(*) FROM devices WHERE deviceID = $1", deviceID).Scan(&deviceExists)
		if err != nil || deviceExists == 0 {
			errors = append(errors, fmt.Sprintf("Device %s not found", deviceID))
			skippedCount++
			continue
		}

		// Check if device is already in a case
		var existingCaseID sql.NullInt64
		err = db.QueryRow("SELECT caseID FROM devicescases WHERE deviceID = $1", deviceID).Scan(&existingCaseID)
		if err != nil && err != sql.ErrNoRows {
			log.Printf("AddDevicesToCase check existing case error: %v", err)
			errors = append(errors, fmt.Sprintf("Failed to check device %s", deviceID))
			skippedCount++
			continue
		}

		if existingCaseID.Valid {
			errors = append(errors, fmt.Sprintf("Device %s is already in case %d", deviceID, existingCaseID.Int64))
			skippedCount++
			continue
		}

		// Add device to case
		_, err = db.Exec("INSERT INTO devicescases (deviceID, caseID) VALUES ($1, $2)", deviceID, caseID)
		if err != nil {
			log.Printf("AddDevicesToCase insert error for %s: %v", deviceID, err)
			errors = append(errors, fmt.Sprintf("Failed to add device %s", deviceID))
			skippedCount++
			continue
		}

		successCount++
	}

	response := map[string]interface{}{
		"success_count": successCount,
		"skipped_count": skippedCount,
		"total":         len(req.DeviceIDs),
	}

	if len(errors) > 0 {
		response["errors"] = errors
	}

	if successCount > 0 {
		response["message"] = fmt.Sprintf("Successfully added %d device(s) to case", successCount)
	}

	respondJSON(w, http.StatusOK, response)
}

// RemoveDeviceFromCase removes a device from a case
// DELETE /api/v1/cases/{id}/devices/{device_id}
func RemoveDeviceFromCase(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	caseIDStr := vars["id"]
	deviceID := vars["device_id"]

	caseID, err := strconv.Atoi(caseIDStr)
	if err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid case ID"})
		return
	}

	db := repository.GetSQLDB()

	// Check if device is in this case
	var exists int
	err = db.QueryRow("SELECT COUNT(*) FROM devicescases WHERE caseID = $1 AND deviceID = $2", caseID, deviceID).Scan(&exists)
	if err != nil {
		log.Printf("RemoveDeviceFromCase check error: %v", err)
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to verify device in case"})
		return
	}

	if exists == 0 {
		respondJSON(w, http.StatusNotFound, map[string]string{"error": "Device not found in this case"})
		return
	}

	// Remove device from case
	_, err = db.Exec("DELETE FROM devicescases WHERE caseID = $1 AND deviceID = $2", caseID, deviceID)
	if err != nil {
		log.Printf("RemoveDeviceFromCase delete error: %v", err)
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to remove device from case"})
		return
	}

	respondJSON(w, http.StatusOK, map[string]string{"message": "Device removed from case successfully"})
}

// GetDefects returns defect reports with filters
func GetDefects(w http.ResponseWriter, r *http.Request) {
	status := r.URL.Query().Get("status")
	severity := r.URL.Query().Get("severity")
	deviceID := r.URL.Query().Get("device_id")

	db := repository.GetSQLDB()
	qb := NewQueryBuilder()
	query := `
		SELECT dr.defect_id, dr.device_id, dr.severity, dr.status, dr.title, dr.description,
		       dr.reported_at, dr.repair_cost, dr.repaired_at, dr.closed_at,
		       COALESCE(p.name, '') as product_name
		FROM defect_reports dr
		LEFT JOIN devices d ON dr.device_id = d.deviceID
		LEFT JOIN products p ON d.productID = p.productID
		WHERE 1=1`

	args := []interface{}{}
	if status != "" {
		query += " AND dr.status = " + qb.NextPlaceholder()
		args = append(args, status)
	}
	if severity != "" {
		query += " AND dr.severity = " + qb.NextPlaceholder()
		args = append(args, severity)
	}
	if deviceID != "" {
		query += " AND dr.device_id = " + qb.NextPlaceholder()
		args = append(args, deviceID)
	}

	query += " ORDER BY dr.reported_at DESC LIMIT 100"

	rows, err := db.Query(query, args...)
	if err != nil {
		log.Printf("Error getting defects: %v", err)
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	defer rows.Close()

	type DefectResponse struct {
		DefectID    int64    `json:"defect_id"`
		DeviceID    string   `json:"device_id"`
		Severity    string   `json:"severity"`
		Status      string   `json:"status"`
		Title       string   `json:"title"`
		Description string   `json:"description"`
		ReportedAt  string   `json:"reported_at"`
		RepairCost  *float64 `json:"repair_cost,omitempty"`
		RepairedAt  *string  `json:"repaired_at,omitempty"`
		ClosedAt    *string  `json:"closed_at,omitempty"`
		ProductName string   `json:"product_name,omitempty"`
	}

	defects := []DefectResponse{}
	for rows.Next() {
		var d DefectResponse
		var reportedAt, repairedAt, closedAt sql.NullTime
		var repairCost sql.NullFloat64

		if err := rows.Scan(&d.DefectID, &d.DeviceID, &d.Severity, &d.Status, &d.Title, &d.Description,
			&reportedAt, &repairCost, &repairedAt, &closedAt, &d.ProductName); err != nil {
			log.Printf("Error scanning defect row: %v", err)
			continue
		}

		if reportedAt.Valid {
			d.ReportedAt = reportedAt.Time.Format("2006-01-02T15:04:05Z")
		}
		if repairCost.Valid {
			cost := repairCost.Float64
			d.RepairCost = &cost
		}
		if repairedAt.Valid {
			repaired := repairedAt.Time.Format("2006-01-02T15:04:05Z")
			d.RepairedAt = &repaired
		}
		if closedAt.Valid {
			closed := closedAt.Time.Format("2006-01-02T15:04:05Z")
			d.ClosedAt = &closed
		}

		defects = append(defects, d)
	}

	respondJSON(w, http.StatusOK, defects)
}

func CreateDefect(w http.ResponseWriter, r *http.Request) {
	var input struct {
		DeviceID    string `json:"device_id"`
		Severity    string `json:"severity"`
		Title       string `json:"title"`
		Description string `json:"description"`
		AssignedTo  *int64 `json:"assigned_to,omitempty"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid request"})
		return
	}

	if input.DeviceID == "" || input.Title == "" {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "device_id and title are required"})
		return
	}

	db := repository.GetSQLDB()

	// Set device status to defective
	_, err := db.Exec(`UPDATE devices SET status = 'defective' WHERE deviceID = $1`, input.DeviceID)
	if err != nil {
		log.Printf("Error updating device status: %v", err)
	}

	// Create defect report
	var defectID int64
	err = db.QueryRow(`
		INSERT INTO defect_reports (device_id, severity, title, description, assigned_to, status)
		VALUES ($1, $2, $3, $4, $5, 'open')
		RETURNING defect_id
	`, input.DeviceID, input.Severity, input.Title, input.Description, input.AssignedTo).Scan(&defectID)

	if err != nil {
		log.Printf("Error creating defect: %v", err)
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	respondJSON(w, http.StatusCreated, map[string]interface{}{
		"defect_id": defectID,
		"message":   "Defect report created successfully",
	})
}

func UpdateDefect(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	defectID := vars["id"]

	var input struct {
		Status      *string  `json:"status,omitempty"`
		AssignedTo  *int64   `json:"assigned_to,omitempty"`
		RepairCost  *float64 `json:"repair_cost,omitempty"`
		RepairNotes *string  `json:"repair_notes,omitempty"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid request"})
		return
	}

	db := repository.GetSQLDB()

	// Build dynamic UPDATE query with PostgreSQL placeholders
	updates := []string{}
	args := []interface{}{}
	argNum := 1

	if input.Status != nil {
		updates = append(updates, fmt.Sprintf("status = $%d", argNum))
		argNum++
		args = append(args, *input.Status)

		// Update timestamps based on status
		if *input.Status == "repaired" {
			updates = append(updates, "repaired_at = NOW()")
		} else if *input.Status == "closed" {
			updates = append(updates, "closed_at = NOW()")
		}
	}
	if input.AssignedTo != nil {
		updates = append(updates, fmt.Sprintf("assigned_to = $%d", argNum))
		argNum++
		args = append(args, *input.AssignedTo)
	}
	if input.RepairCost != nil {
		updates = append(updates, fmt.Sprintf("repair_cost = $%d", argNum))
		argNum++
		args = append(args, *input.RepairCost)
	}
	if input.RepairNotes != nil {
		updates = append(updates, fmt.Sprintf("repair_notes = $%d", argNum))
		argNum++
		args = append(args, *input.RepairNotes)
	}

	if len(updates) == 0 {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "No fields to update"})
		return
	}

	query := fmt.Sprintf("UPDATE defect_reports SET %s WHERE defect_id = $%d", strings.Join(updates, ", "), argNum)
	args = append(args, defectID)

	_, err := db.Exec(query, args...)
	if err != nil {
		log.Printf("Error updating defect: %v", err)
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	// If status is repaired or closed, update device status
	if input.Status != nil && (*input.Status == "repaired" || *input.Status == "closed") {
		var deviceID string
		db.QueryRow(`SELECT device_id FROM defect_reports WHERE defect_id = $1`, defectID).Scan(&deviceID)
		if deviceID != "" {
			db.Exec(`UPDATE devices SET status = 'in_storage' WHERE deviceID = $1`, deviceID)
		}
	}

	respondJSON(w, http.StatusOK, map[string]string{"message": "Defect updated successfully"})
}

func GetInspections(w http.ResponseWriter, r *http.Request) {
	status := r.URL.Query().Get("status") // upcoming, overdue, all
	deviceID := r.URL.Query().Get("device_id")

	db := repository.GetSQLDB()
	query := `
		SELECT i.schedule_id, i.device_id, i.product_id, i.inspection_type,
		       i.interval_days, i.last_inspection, i.next_inspection, i.is_active,
		       COALESCE(p.name, '') as product_name,
		       COALESCE(d.deviceID, '') as device_name
		FROM inspection_schedules i
		LEFT JOIN products p ON i.product_id = p.productID
		LEFT JOIN devices d ON i.device_id = d.deviceID
		WHERE 1=1`

	args := []interface{}{}
	qb := NewQueryBuilder()

	if status == "upcoming" {
		query += " AND i.next_inspection >= NOW() AND i.next_inspection <= NOW() + INTERVAL '30 days' AND i.is_active = TRUE"
	} else if status == "overdue" {
		query += " AND i.next_inspection < NOW() AND i.is_active = TRUE"
	} else if status == "active" {
		query += " AND i.is_active = TRUE"
	}

	if deviceID != "" {
		query += " AND i.device_id = " + qb.NextPlaceholder()
		args = append(args, deviceID)
	}

	query += " ORDER BY i.next_inspection ASC LIMIT 100"

	rows, err := db.Query(query, args...)
	if err != nil {
		log.Printf("Error getting inspections: %v", err)
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	defer rows.Close()

	type InspectionResponse struct {
		ScheduleID     int64   `json:"schedule_id"`
		DeviceID       *string `json:"device_id,omitempty"`
		ProductID      *int64  `json:"product_id,omitempty"`
		InspectionType string  `json:"inspection_type"`
		IntervalDays   int     `json:"interval_days"`
		LastInspection *string `json:"last_inspection,omitempty"`
		NextInspection *string `json:"next_inspection,omitempty"`
		IsActive       bool    `json:"is_active"`
		ProductName    string  `json:"product_name,omitempty"`
		DeviceName     string  `json:"device_name,omitempty"`
	}

	inspections := []InspectionResponse{}
	for rows.Next() {
		var i InspectionResponse
		var deviceID sql.NullString
		var lastInspection, nextInspection sql.NullTime
		var prodID sql.NullInt64

		if err := rows.Scan(&i.ScheduleID, &deviceID, &prodID, &i.InspectionType,
			&i.IntervalDays, &lastInspection, &nextInspection, &i.IsActive,
			&i.ProductName, &i.DeviceName); err != nil {
			log.Printf("Error scanning inspection row: %v", err)
			continue
		}

		if deviceID.Valid {
			i.DeviceID = &deviceID.String
		}
		if prodID.Valid {
			pid := prodID.Int64
			i.ProductID = &pid
		}
		if lastInspection.Valid {
			last := lastInspection.Time.Format("2006-01-02T15:04:05Z")
			i.LastInspection = &last
		}
		if nextInspection.Valid {
			next := nextInspection.Time.Format("2006-01-02T15:04:05Z")
			i.NextInspection = &next
		}

		inspections = append(inspections, i)
	}

	respondJSON(w, http.StatusOK, inspections)
}

func GetDashboardStats(w http.ResponseWriter, r *http.Request) {
	db := repository.GetSQLDB()

	var inStorage, onJob, defective int
	db.QueryRow(`SELECT COUNT(*) FROM devices WHERE status = 'in_storage'`).Scan(&inStorage)
	db.QueryRow(`SELECT COUNT(*) FROM devices WHERE status = 'on_job' OR status = 'rented'`).Scan(&onJob)
	db.QueryRow(`SELECT COUNT(*) FROM devices WHERE status = 'defective'`).Scan(&defective)

	respondJSON(w, http.StatusOK, map[string]interface{}{
		"in_storage": inStorage,
		"on_job":     onJob,
		"defective":  defective,
		"total":      inStorage + onJob + defective,
	})
}

func GetMovements(w http.ResponseWriter, r *http.Request) {
	limit := parseInt(r.URL.Query().Get("limit"), 10)
	if limit <= 0 {
		limit = 10
	}
	if limit > 100 {
		limit = 100
	}

	db := repository.GetSQLDB()
	rows, err := db.Query(`
		SELECT
			dm.movement_id,
			dm.device_id,
			dm.movement_type as action,
			dm.from_zone_id,
			dm.to_zone_id,
			dm.from_job_id,
			dm.to_job_id,
			dm.created_at as timestamp,
			d.barcode,
			d.serialnumber,
			p.name,
			fz.name,
			tz.name,
			fj.description,
			tj.description,
			COALESCE(NULLIF(TRIM(CONCAT(u.first_name, ' ', u.last_name)), ''), u.username) as performed_by
		FROM device_movements dm
		LEFT JOIN devices d ON dm.device_id = d.deviceID
		LEFT JOIN products p ON d.productID = p.productID
		LEFT JOIN storage_zones fz ON dm.from_zone_id = fz.zone_id
		LEFT JOIN storage_zones tz ON dm.to_zone_id = tz.zone_id
		LEFT JOIN jobs fj ON dm.from_job_id = fj.jobID
		LEFT JOIN jobs tj ON dm.to_job_id = tj.jobID
		LEFT JOIN users u ON dm.moved_by = u.userID
		ORDER BY dm.created_at DESC
		LIMIT $1
	`, limit)
	if err != nil {
		log.Printf("Error querying movements: %v", err)
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to load movements"})
		return
	}
	defer rows.Close()

	type movementResponse struct {
		MovementID         int64     `json:"movement_id"`
		DeviceID           string    `json:"device_id"`
		Action             string    `json:"action"`
		Timestamp          time.Time `json:"timestamp"`
		FromZoneID         *int64    `json:"from_zone_id,omitempty"`
		ToZoneID           *int64    `json:"to_zone_id,omitempty"`
		FromJobID          *int64    `json:"from_job_id,omitempty"`
		ToJobID            *int64    `json:"to_job_id,omitempty"`
		Barcode            *string   `json:"barcode,omitempty"`
		SerialNumber       *string   `json:"serial_number,omitempty"`
		ProductName        *string   `json:"product_name,omitempty"`
		FromZoneName       *string   `json:"from_zone_name,omitempty"`
		ToZoneName         *string   `json:"to_zone_name,omitempty"`
		FromJobDescription *string   `json:"from_job_description,omitempty"`
		ToJobDescription   *string   `json:"to_job_description,omitempty"`
		PerformedBy        *string   `json:"performed_by,omitempty"`
	}

	movements := []movementResponse{}

	for rows.Next() {
		var (
			movement     movementResponse
			fromZoneID   sql.NullInt64
			toZoneID     sql.NullInt64
			fromJobID    sql.NullInt64
			toJobID      sql.NullInt64
			barcode      sql.NullString
			serial       sql.NullString
			product      sql.NullString
			fromZoneName sql.NullString
			toZoneName   sql.NullString
			fromJobDesc  sql.NullString
			toJobDesc    sql.NullString
			performedBy  sql.NullString
		)

		if err := rows.Scan(
			&movement.MovementID,
			&movement.DeviceID,
			&movement.Action,
			&fromZoneID,
			&toZoneID,
			&fromJobID,
			&toJobID,
			&movement.Timestamp,
			&barcode,
			&serial,
			&product,
			&fromZoneName,
			&toZoneName,
			&fromJobDesc,
			&toJobDesc,
			&performedBy,
		); err != nil {
			log.Printf("Error scanning movement row: %v", err)
			continue
		}

		if fromZoneID.Valid {
			value := fromZoneID.Int64
			movement.FromZoneID = &value
		}
		if toZoneID.Valid {
			value := toZoneID.Int64
			movement.ToZoneID = &value
		}
		if fromJobID.Valid {
			value := fromJobID.Int64
			movement.FromJobID = &value
		}
		if toJobID.Valid {
			value := toJobID.Int64
			movement.ToJobID = &value
		}
		if barcode.Valid {
			value := barcode.String
			movement.Barcode = &value
		}
		if serial.Valid {
			value := serial.String
			movement.SerialNumber = &value
		}
		if product.Valid {
			value := product.String
			movement.ProductName = &value
		}
		if fromZoneName.Valid {
			value := fromZoneName.String
			movement.FromZoneName = &value
		}
		if toZoneName.Valid {
			value := toZoneName.String
			movement.ToZoneName = &value
		}
		if fromJobDesc.Valid {
			value := fromJobDesc.String
			movement.FromJobDescription = &value
		}
		if toJobDesc.Valid {
			value := toJobDesc.String
			movement.ToJobDescription = &value
		}
		if performedBy.Valid {
			value := performedBy.String
			movement.PerformedBy = &value
		}

		movements = append(movements, movement)
	}

	if err := rows.Err(); err != nil {
		log.Printf("Error iterating movement rows: %v", err)
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to process movements"})
		return
	}

	respondJSON(w, http.StatusOK, movements)
}

// buildDeviceMap creates a complete device map from database values
func buildDeviceMap(deviceID, productName, status, barcode, qrCode, serialNumber sql.NullString,
	productID sql.NullInt64, zoneID sql.NullInt64, zoneName, zoneCode sql.NullString,
	caseID sql.NullInt64, caseName sql.NullString, currentJobID sql.NullInt64, jobNumber sql.NullString,
	conditionRating, usageHours sql.NullFloat64, labelPath, purchaseDate, lastMaintenance, nextMaintenance, notes sql.NullString) map[string]interface{} {

	device := map[string]interface{}{
		"device_id":    deviceID.String,
		"product_name": productName.String,
		"status":       status.String,
	}

	if productID.Valid {
		device["product_id"] = productID.Int64
	}
	if barcode.Valid && barcode.String != "" {
		device["barcode"] = barcode.String
	}
	if qrCode.Valid && qrCode.String != "" {
		device["qr_code"] = qrCode.String
	}
	if serialNumber.Valid && serialNumber.String != "" {
		device["serial_number"] = serialNumber.String
	}
	if zoneID.Valid {
		device["zone_id"] = zoneID.Int64
		if zoneName.Valid && zoneName.String != "" {
			device["zone_name"] = zoneName.String
		}
		if zoneCode.Valid && zoneCode.String != "" {
			device["zone_code"] = zoneCode.String
		}
	}
	if caseID.Valid {
		device["case_id"] = caseID.Int64
		if caseName.Valid && caseName.String != "" {
			device["case_name"] = caseName.String
		}
	}
	if currentJobID.Valid {
		device["current_job_id"] = currentJobID.Int64
		if jobNumber.Valid && jobNumber.String != "" {
			device["job_number"] = jobNumber.String
		}
	}
	if conditionRating.Valid && conditionRating.Float64 > 0 {
		device["condition_rating"] = conditionRating.Float64
	}
	if usageHours.Valid && usageHours.Float64 > 0 {
		device["usage_hours"] = usageHours.Float64
	}
	if labelPath.Valid && labelPath.String != "" {
		device["label_path"] = labelPath.String
	}
	if purchaseDate.Valid && purchaseDate.String != "" {
		device["purchase_date"] = purchaseDate.String
	}
	if lastMaintenance.Valid && lastMaintenance.String != "" {
		device["last_maintenance"] = lastMaintenance.String
	}
	if nextMaintenance.Valid && nextMaintenance.String != "" {
		device["next_maintenance"] = nextMaintenance.String
	}
	if notes.Valid && notes.String != "" {
		device["notes"] = notes.String
	}

	return device
}

// GetDeviceTree returns devices organized in a hierarchical tree structure by categories
func GetDeviceTree(w http.ResponseWriter, r *http.Request) {
	db := repository.GetSQLDB()

	// Query for device tree with categories - Include ALL categories, devices, consumables, and accessories
	// This ensures newly created categories and consumables/accessories appear immediately in the tree
	query := `
		WITH latest_job AS (
			SELECT jd.deviceID, jd.jobID
			FROM job_devices jd
			INNER JOIN (
				SELECT deviceID, MAX(jobID) AS jobID
				FROM job_devices
				GROUP BY deviceID
			) latest ON jd.deviceID = latest.deviceID AND jd.jobID = latest.jobID
		)
		SELECT
			c.categoryID,
			c.name as category_name,
			sc.subcategoryID,
			sc.name as subcategory_name,
			sbc.subbiercategoryID,
			sbc.name as subbiercategory_name,
			p.productID,
			COALESCE(p.name, '') as product_name,
			CASE WHEN p.is_consumable = TRUE THEN 1 ELSE 0 END as is_consumable,
			CASE WHEN p.is_accessory = TRUE THEN 1 ELSE 0 END as is_accessory,
			COALESCE(p.stock_quantity, 0) as stock_quantity,
			COALESCE(ct.abbreviation, '') as unit,
			d.deviceID,
			d.status,
			d.barcode,
			d.qr_code,
			d.serialnumber,
			d.zone_id,
			COALESCE(z.name, '') as zone_name,
			COALESCE(z.code, '') as zone_code,
			dc.caseID as case_id,
			COALESCE(cs.name, '') as case_name,
			lj.jobID as current_job_id,
			COALESCE(j.job_code, '') as job_number,
			COALESCE(d.condition_rating, 0) as condition_rating,
			COALESCE(d.usage_hours, 0) as usage_hours,
			d.label_path,
			d.purchaseDate,
			d.lastmaintenance,
			d.nextmaintenance,
			d.notes
		FROM categories c
		LEFT JOIN subcategories sc ON c.categoryID = sc.categoryID
		LEFT JOIN subbiercategories sbc ON sc.subcategoryID = sbc.subcategoryID
		LEFT JOIN products p ON (sbc.subbiercategoryID = p.subbiercategoryID OR (sc.subcategoryID = p.subcategoryID AND p.subbiercategoryID IS NULL))
		LEFT JOIN count_types ct ON p.count_type_id = ct.count_type_id
		LEFT JOIN devices d ON p.productID = d.productID
		LEFT JOIN storage_zones z ON d.zone_id = z.zone_id
		LEFT JOIN devicescases dc ON d.deviceID = dc.deviceID
		LEFT JOIN cases cs ON dc.caseID = cs.caseID
		LEFT JOIN latest_job lj ON d.deviceID = lj.deviceID
		LEFT JOIN jobs j ON lj.jobID = j.jobID
		ORDER BY c.name, sc.name, sbc.name, p.name, d.deviceID
	`

	rows, err := db.Query(query)
	if err != nil {
		log.Printf("Error querying device tree: %v", err)
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	defer rows.Close()

	// Build tree structure
	categories := make(map[int]*map[string]interface{})
	subcategories := make(map[string]*map[string]interface{})
	subbiercategories := make(map[string]*map[string]interface{})

	// Track which products we've already added (to avoid duplicates for consumables/accessories)
	addedProducts := make(map[int]bool)
	// Track devices to avoid duplicates (e.g., multiple job/device rows)
	seenDevices := make(map[string]bool)

	for rows.Next() {
		var categoryID sql.NullInt64
		var subcategoryID, subbiercategoryID sql.NullString
		var categoryName, subcategoryName, subbiercategoryName sql.NullString
		var productID sql.NullInt64
		var productName sql.NullString
		var isConsumable, isAccessory int
		var stockQuantity float64
		var unit sql.NullString
		var deviceID, status, barcode, qrCode, serialNumber sql.NullString
		var zoneID sql.NullInt64
		var zoneName, zoneCode sql.NullString
		var caseID sql.NullInt64
		var caseName sql.NullString
		var currentJobID sql.NullInt64
		var jobNumber sql.NullString
		var conditionRating, usageHours sql.NullFloat64
		var labelPath, purchaseDate, lastMaintenance, nextMaintenance, notes sql.NullString

		err := rows.Scan(&categoryID, &categoryName, &subcategoryID, &subcategoryName,
			&subbiercategoryID, &subbiercategoryName, &productID, &productName,
			&isConsumable, &isAccessory, &stockQuantity, &unit,
			&deviceID, &status, &barcode, &qrCode, &serialNumber, &zoneID, &zoneName, &zoneCode,
			&caseID, &caseName, &currentJobID, &jobNumber,
			&conditionRating, &usageHours, &labelPath, &purchaseDate, &lastMaintenance, &nextMaintenance, &notes)
		if err != nil {
			log.Printf("Error scanning row: %v", err)
			continue
		}

		// Skip if no category
		if !categoryID.Valid {
			continue
		}

		// Get or create category (always create, even if empty)
		catID := int(categoryID.Int64)
		if _, exists := categories[catID]; !exists {
			categories[catID] = &map[string]interface{}{
				"id":             catID,
				"name":           categoryName.String,
				"subcategories":  []interface{}{},
				"direct_devices": []interface{}{},
				"device_count":   0,
			}
		}

		// Process subcategory if exists (create even if empty)
		if subcategoryID.Valid && subcategoryID.String != "" {
			subCatID := subcategoryID.String
			if _, exists := subcategories[subCatID]; !exists {
				subcategories[subCatID] = &map[string]interface{}{
					"id":                subCatID,
					"name":              subcategoryName.String,
					"subbiercategories": []interface{}{},
					"direct_devices":    []interface{}{},
					"device_count":      0,
				}
				// Add subcategory to category
				cat := *categories[catID]
				cat["subcategories"] = append(cat["subcategories"].([]interface{}), subcategories[subCatID])
			}

			// Process subbiercategory if exists (create even if empty)
			if subbiercategoryID.Valid && subbiercategoryID.String != "" {
				subBierCatID := subbiercategoryID.String
				if _, exists := subbiercategories[subBierCatID]; !exists {
					subbiercategories[subBierCatID] = &map[string]interface{}{
						"id":           subBierCatID,
						"name":         subbiercategoryName.String,
						"devices":      []interface{}{},
						"device_count": 0,
					}
					// Add subbiercategory to subcategory
					subCat := *subcategories[subCatID]
					subCat["subbiercategories"] = append(subCat["subbiercategories"].([]interface{}), subbiercategories[subBierCatID])
				}

				// Add device to subbiercategory if exists
				if deviceID.Valid && deviceID.String != "" {
					if seenDevices[deviceID.String] {
						continue
					}
					device := buildDeviceMap(deviceID, productName, status, barcode, qrCode, serialNumber,
						productID, zoneID, zoneName, zoneCode, caseID, caseName, currentJobID, jobNumber,
						conditionRating, usageHours, labelPath, purchaseDate, lastMaintenance, nextMaintenance, notes)

					seenDevices[deviceID.String] = true
					subBierCat := *subbiercategories[subBierCatID]
					subBierCat["devices"] = append(subBierCat["devices"].([]interface{}), device)
					subBierCat["device_count"] = subBierCat["device_count"].(int) + 1

					// Update counts
					subCat := *subcategories[subCatID]
					subCat["device_count"] = subCat["device_count"].(int) + 1
					cat := *categories[catID]
					cat["device_count"] = cat["device_count"].(int) + 1
				} else if productID.Valid && (isConsumable == 1 || isAccessory == 1) {
					// Add consumable/accessory as product item if no device exists
					// Only add once per product (avoid duplicates from LEFT JOIN)
					prodID := int(productID.Int64)
					if !addedProducts[prodID] {
						addedProducts[prodID] = true

						productItem := map[string]interface{}{
							"device_id":      fmt.Sprintf("PROD-%d", prodID),
							"product_name":   productName.String,
							"status":         "in_storage",
							"is_consumable":  isConsumable == 1,
							"is_accessory":   isAccessory == 1,
							"stock_quantity": stockQuantity,
						}
						if unit.Valid {
							productItem["unit"] = unit.String
						}

						subBierCat := *subbiercategories[subBierCatID]
						subBierCat["devices"] = append(subBierCat["devices"].([]interface{}), productItem)
						subBierCat["device_count"] = subBierCat["device_count"].(int) + 1

						// Update counts
						subCat := *subcategories[subCatID]
						subCat["device_count"] = subCat["device_count"].(int) + 1
						cat := *categories[catID]
						cat["device_count"] = cat["device_count"].(int) + 1
					}
				}
			} else if productID.Valid {
				// Product belongs directly to subcategory (no subbiercategory)
				// Handle devices or consumables/accessories at subcategory level
				if deviceID.Valid && deviceID.String != "" {
					// Add device directly to subcategory
					if seenDevices[deviceID.String] {
						continue
					}
					device := buildDeviceMap(deviceID, productName, status, barcode, qrCode, serialNumber,
						productID, zoneID, zoneName, zoneCode, caseID, caseName, currentJobID, jobNumber,
						conditionRating, usageHours, labelPath, purchaseDate, lastMaintenance, nextMaintenance, notes)

					seenDevices[deviceID.String] = true
					subCat := *subcategories[subCatID]
					subCat["direct_devices"] = append(subCat["direct_devices"].([]interface{}), device)
					subCat["device_count"] = subCat["device_count"].(int) + 1

					// Update category count
					cat := *categories[catID]
					cat["device_count"] = cat["device_count"].(int) + 1
				} else if isConsumable == 1 || isAccessory == 1 {
					// Add consumable/accessory directly to subcategory
					prodID := int(productID.Int64)
					if !addedProducts[prodID] {
						addedProducts[prodID] = true

						productItem := map[string]interface{}{
							"device_id":      fmt.Sprintf("PROD-%d", prodID),
							"product_name":   productName.String,
							"status":         "in_storage",
							"is_consumable":  isConsumable == 1,
							"is_accessory":   isAccessory == 1,
							"stock_quantity": stockQuantity,
						}
						if unit.Valid {
							productItem["unit"] = unit.String
						}

						subCat := *subcategories[subCatID]
						subCat["direct_devices"] = append(subCat["direct_devices"].([]interface{}), productItem)
						subCat["device_count"] = subCat["device_count"].(int) + 1

						// Update category count
						cat := *categories[catID]
						cat["device_count"] = cat["device_count"].(int) + 1
					}
				}
			}
		}
	}

	// Convert map to slice
	treeData := []interface{}{}
	for _, cat := range categories {
		treeData = append(treeData, *cat)
	}

	respondJSON(w, http.StatusOK, map[string]interface{}{
		"treeData": treeData,
	})
}

// AssignDevicesToZone assigns multiple devices to a storage zone
func AssignDevicesToZone(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	zoneID := vars["id"]

	var input struct {
		DeviceIDs []string `json:"device_ids"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid request body"})
		return
	}

	if len(input.DeviceIDs) == 0 {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "No devices specified"})
		return
	}

	db := repository.GetSQLDB()

	// Verify zone exists
	var exists bool
	err := db.QueryRow("SELECT EXISTS(SELECT 1 FROM storage_zones WHERE zone_id = $1)", zoneID).Scan(&exists)
	if err != nil || !exists {
		respondJSON(w, http.StatusNotFound, map[string]string{"error": "Zone not found"})
		return
	}

	// Update devices
	successCount := 0
	failedDevices := []string{}

	for _, deviceID := range input.DeviceIDs {
		result, err := db.Exec(`
			UPDATE devices
			SET zone_id = $1,
			    status = CASE
			        WHEN status = 'on_job' OR status = 'rented' THEN status
			        ELSE 'in_storage'
			    END
			WHERE deviceID = $2
		`, zoneID, deviceID)

		if err != nil {
			log.Printf("Error assigning device %s to zone %s: %v", deviceID, zoneID, err)
			failedDevices = append(failedDevices, deviceID)
			continue
		}

		rowsAffected, _ := result.RowsAffected()
		if rowsAffected > 0 {
			successCount++

			// Log movement
			_, _ = db.Exec(`
				INSERT INTO device_movements (device_id, from_zone_id, to_zone_id, movement_type, moved_at)
				SELECT $1, zone_id, $2, 'assignment', NOW()
				FROM devices WHERE deviceID = $3
			`, deviceID, zoneID, deviceID)
		} else {
			failedDevices = append(failedDevices, deviceID)
		}
	}

	response := map[string]interface{}{
		"success":      successCount,
		"total":        len(input.DeviceIDs),
		"failed_count": len(failedDevices),
	}

	if len(failedDevices) > 0 {
		response["failed_devices"] = failedDevices
	}

	respondJSON(w, http.StatusOK, response)
}

// Helper functions
func respondJSON(w http.ResponseWriter, statusCode int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	json.NewEncoder(w).Encode(data)
}

func parseInt(s string, defaultVal int) int {
	if s == "" {
		return defaultVal
	}
	val, err := strconv.Atoi(s)
	if err != nil {
		return defaultVal
	}
	return val
}

// GetMaintenanceStats returns maintenance dashboard statistics
func GetMaintenanceStats(w http.ResponseWriter, r *http.Request) {
	db := repository.GetSQLDB()

	var openDefects, inProgressDefects, repairedDefects int
	var overdueInspections, upcomingInspections int

	db.QueryRow(`SELECT COUNT(*) FROM defect_reports WHERE status = 'open'`).Scan(&openDefects)
	db.QueryRow(`SELECT COUNT(*) FROM defect_reports WHERE status = 'in_progress'`).Scan(&inProgressDefects)
	db.QueryRow(`SELECT COUNT(*) FROM defect_reports WHERE status = 'repaired'`).Scan(&repairedDefects)
	db.QueryRow(`SELECT COUNT(*) FROM inspection_schedules WHERE next_inspection < NOW() AND is_active = TRUE`).Scan(&overdueInspections)
	db.QueryRow(`SELECT COUNT(*) FROM inspection_schedules WHERE next_inspection >= NOW() AND next_inspection <= NOW() + INTERVAL '30 days' AND is_active = TRUE`).Scan(&upcomingInspections)

	respondJSON(w, http.StatusOK, map[string]interface{}{
		"open_defects":         openDefects,
		"in_progress_defects":  inProgressDefects,
		"repaired_defects":     repairedDefects,
		"overdue_inspections":  overdueInspections,
		"upcoming_inspections": upcomingInspections,
	})
}
