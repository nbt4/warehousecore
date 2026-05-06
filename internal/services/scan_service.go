package services

import (
	"database/sql"
	"fmt"
	"log"
	"strconv"
	"time"

	"warehousecore/internal/led"
	"warehousecore/internal/models"
	"warehousecore/internal/repository"
)

// ScanService handles all scan-related business logic
type ScanService struct {
	db *sql.DB
}

// NewScanService creates a new scan service
func NewScanService() *ScanService {
	return &ScanService{
		db: repository.GetSQLDB(),
	}
}

// ProcessScan handles a barcode/QR scan and performs the appropriate action
func (s *ScanService) ProcessScan(req models.ScanRequest, userID *int64, ipAddr, userAgent string) (*models.ScanResponse, error) {
	// Start transaction
	tx, err := s.db.Begin()
	if err != nil {
		return nil, fmt.Errorf("failed to start transaction: %w", err)
	}
	defer tx.Rollback()

	// Find device by barcode/QR code
	device, err := s.findDeviceByScan(req.ScanCode)
	if err != nil {
		// Device not found - check if it's a consumable/accessory
		consumable, consumableErr := s.findConsumableByScan(req.ScanCode)
		if consumableErr != nil {
			// Neither device nor consumable found
			s.logScanEvent(tx, req.ScanCode, nil, req.Action, req.JobID, req.ZoneID, userID, false, err.Error(), ipAddr, userAgent)
			tx.Commit()
			return &models.ScanResponse{
				Success: false,
				Message: fmt.Sprintf("Product not found: %v", err),
			}, nil
		}

		// Found consumable - handle consumable scan
		return s.processConsumableScan(tx, consumable, req, userID, ipAddr, userAgent)
	}

	// Check for duplicate scan (same job)
	if req.JobID != nil && device.CurrentJobID.Valid && device.CurrentJobID.Int64 == *req.JobID {
		// Duplicate scan - treat as job complete signal
		s.logScanEvent(tx, req.ScanCode, &device.DeviceID, "check", req.JobID, req.ZoneID, userID, true, "", ipAddr, userAgent)
		tx.Commit()

		return &models.ScanResponse{
			Success:   true,
			Message:   "Duplicate scan detected - job ready to complete",
			Device:    s.getDeviceWithDetails(device.DeviceID),
			Action:    "check",
			Duplicate: true,
		}, nil
	}

	// Process action
	var response *models.ScanResponse
	var movement *models.DeviceMovement

	switch req.Action {
	case "intake":
		response, movement, err = s.processIntake(tx, device, req.ZoneID)
	case "outtake":
		response, movement, err = s.processOuttake(tx, device, req.JobID)
	case "check":
		response, err = s.processCheck(tx, device)
	case "transfer":
		response, movement, err = s.processTransfer(tx, device, req.ZoneID)
	default:
		err = fmt.Errorf("unknown action: %s", req.Action)
	}

	if err != nil {
		s.logScanEvent(tx, req.ScanCode, &device.DeviceID, req.Action, req.JobID, req.ZoneID, userID, false, err.Error(), ipAddr, userAgent)
		tx.Commit()
		return &models.ScanResponse{
			Success: false,
			Message: fmt.Sprintf("Action failed: %v", err),
			Device:  s.getDeviceWithDetails(device.DeviceID),
		}, nil
	}

	// Log successful scan
	s.logScanEvent(tx, req.ScanCode, &device.DeviceID, req.Action, req.JobID, req.ZoneID, userID, true, "", ipAddr, userAgent)

	// Commit transaction
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("failed to commit: %w", err)
	}

	response.Device = s.getDeviceWithDetails(device.DeviceID)
	response.Movement = movement

	// Update LED status after successful outtake
	if req.Action == "outtake" && movement != nil && movement.FromZoneID.Valid && req.JobID != nil {
		go s.updateLEDsAfterOuttake(*req.JobID, movement.FromZoneID.Int64)
	}

	return response, nil
}

// processIntake handles device intake from job back to warehouse
func (s *ScanService) processIntake(tx *sql.Tx, device *models.Device, zoneID *int64) (*models.ScanResponse, *models.DeviceMovement, error) {
	previousStatus := device.Status
	var fromJobID *int64
	if device.CurrentJobID.Valid {
		fromJobID = &device.CurrentJobID.Int64
	}

	// Update device status to in_storage
	_, err := tx.Exec(`
		UPDATE devices
		SET status = 'in_storage', zone_id = $1, current_location = 'warehouse'
		WHERE deviceID = $2
	`, zoneID, device.DeviceID)
	if err != nil {
		return nil, nil, err
	}

	// Reset pack status instead of removing from job
	// This makes it appear as "not scanned" again in the job
	if fromJobID != nil {
		_, err = tx.Exec(`
			UPDATE job_devices
			SET pack_status = 'pending', pack_ts = NULL
			WHERE deviceID = $1 AND jobID = $2
		`, device.DeviceID, *fromJobID)
		if err != nil {
			log.Printf("Warning: failed to reset pack status: %v", err)
		} else {
			log.Printf("Reset pack_status to 'pending' for device %s in job %d", device.DeviceID, *fromJobID)
		}
	}

	// Create movement record
	movement := &models.DeviceMovement{
		DeviceID:  device.DeviceID,
		Action:    "intake",
		FromJobID: models.IntToNullInt64(fromJobID),
		ToZoneID:  models.IntToNullInt64(zoneID),
		Timestamp: time.Now(),
	}

	err = tx.QueryRow(`
		INSERT INTO device_movements (device_id, movement_type, from_job_id, to_zone_id, created_at)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING movement_id
	`, movement.DeviceID, movement.Action, movement.FromJobID, movement.ToZoneID, movement.Timestamp).Scan(&movement.MovementID)
	if err != nil {
		return nil, nil, err
	}

	return &models.ScanResponse{
		Success:        true,
		Message:        "Device successfully returned to warehouse",
		Action:         "intake",
		PreviousStatus: previousStatus,
		NewStatus:      "in_storage",
	}, movement, nil
}

// processOuttake handles device outtake from warehouse to job
func (s *ScanService) processOuttake(tx *sql.Tx, device *models.Device, jobID *int64) (*models.ScanResponse, *models.DeviceMovement, error) {
	if jobID == nil {
		return nil, nil, fmt.Errorf("job_id is required for outtake")
	}

	previousStatus := device.Status
	var fromZoneID *int64
	if device.ZoneID.Valid {
		fromZoneID = &device.ZoneID.Int64
	}

	// Update device status to on_job
	_, err := tx.Exec(`
		UPDATE devices
		SET status = 'on_job', zone_id = NULL
		WHERE deviceID = $1
	`, device.DeviceID)
	if err != nil {
		return nil, nil, err
	}

	// Assign to job and update pack_status to issued
	_, err = tx.Exec(`
		INSERT INTO job_devices (deviceID, jobID, pack_status)
		VALUES ($1, $2, 'issued')
		ON CONFLICT (deviceID, jobID) DO UPDATE SET pack_status = 'issued'
	`, device.DeviceID, *jobID)
	if err != nil {
		return nil, nil, err
	}

	// Create movement record
	movement := &models.DeviceMovement{
		DeviceID:   device.DeviceID,
		Action:     "outtake",
		FromZoneID: models.IntToNullInt64(fromZoneID),
		ToJobID:    models.IntToNullInt64(jobID),
		Timestamp:  time.Now(),
	}

	err = tx.QueryRow(`
		INSERT INTO device_movements (device_id, movement_type, from_zone_id, to_job_id, created_at)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING movement_id
	`, movement.DeviceID, movement.Action, movement.FromZoneID, movement.ToJobID, movement.Timestamp).Scan(&movement.MovementID)
	if err != nil {
		return nil, nil, err
	}

	// Load suggested dependencies for this product
	var suggestedDeps []models.ProductDependencyWithDetails
	rows, err := s.db.Query(`
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
			pd.default_quantity,
			pd.notes,
			TO_CHAR(pd.created_at, 'YYYY-MM-DD HH24:MI:SS') as created_at
		FROM product_dependencies pd
		JOIN products p ON pd.dependency_product_id = p.productID
		LEFT JOIN count_types ct ON p.count_type_id = ct.count_type_id
		WHERE pd.product_id = $1
		ORDER BY pd.is_optional ASC, pd.created_at DESC
	`, device.ProductID)
	if err != nil {
		log.Printf("Failed to query product dependencies: %v", err)
		suggestedDeps = []models.ProductDependencyWithDetails{}
	} else {
		defer rows.Close()
		for rows.Next() {
			var dep models.ProductDependencyWithDetails
			err := rows.Scan(
				&dep.ID,
				&dep.ProductID,
				&dep.DependencyProductID,
				&dep.DependencyName,
				&dep.IsAccessory,
				&dep.IsConsumable,
				&dep.GenericBarcode,
				&dep.CountTypeAbbr,
				&dep.StockQuantity,
				&dep.IsOptional,
				&dep.DefaultQuantity,
				&dep.Notes,
				&dep.CreatedAt,
			)
			if err != nil {
				log.Printf("Failed to scan dependency row: %v", err)
				continue
			}
			suggestedDeps = append(suggestedDeps, dep)
		}
	}
	if suggestedDeps == nil {
		suggestedDeps = []models.ProductDependencyWithDetails{}
	}

	return &models.ScanResponse{
		Success:        true,
		Message:        "Device assigned to job",
		Action:         "outtake",
		PreviousStatus: previousStatus,
		NewStatus:      "on_job",
		SuggestedDeps:  suggestedDeps,
	}, movement, nil
}

// processCheck verifies device status without changing it
func (s *ScanService) processCheck(tx *sql.Tx, device *models.Device) (*models.ScanResponse, error) {
	return &models.ScanResponse{
		Success: true,
		Message: fmt.Sprintf("Device status: %s", device.Status),
		Action:  "check",
	}, nil
}

// processTransfer moves device between zones
func (s *ScanService) processTransfer(tx *sql.Tx, device *models.Device, toZoneID *int64) (*models.ScanResponse, *models.DeviceMovement, error) {
	if toZoneID == nil {
		return nil, nil, fmt.Errorf("zone_id is required for transfer")
	}

	var fromZoneID *int64
	if device.ZoneID.Valid {
		fromZoneID = &device.ZoneID.Int64
	}

	// Update device zone
	_, err := tx.Exec(`UPDATE devices SET zone_id = $1 WHERE deviceID = $2`, *toZoneID, device.DeviceID)
	if err != nil {
		return nil, nil, err
	}

	// Create movement record
	movement := &models.DeviceMovement{
		DeviceID:   device.DeviceID,
		Action:     "transfer",
		FromZoneID: models.IntToNullInt64(fromZoneID),
		ToZoneID:   models.IntToNullInt64(toZoneID),
		Timestamp:  time.Now(),
	}

	err = tx.QueryRow(`
		INSERT INTO device_movements (device_id, movement_type, from_zone_id, to_zone_id, created_at)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING movement_id
	`, movement.DeviceID, movement.Action, movement.FromZoneID, movement.ToZoneID, movement.Timestamp).Scan(&movement.MovementID)
	if err != nil {
		return nil, nil, err
	}

	return &models.ScanResponse{
		Success: true,
		Message: "Device transferred to new zone",
		Action:  "transfer",
	}, movement, nil
}

// findDeviceByScan looks up a device by barcode or QR code
func (s *ScanService) findDeviceByScan(scanCode string) (*models.Device, error) {
	var device models.Device
	err := s.db.QueryRow(`
		SELECT deviceID, productID, serialnumber, barcode, qr_code, status,
		       current_location, zone_id, condition_rating, usage_hours
		FROM devices
		WHERE barcode = $1 OR qr_code = $2 OR deviceID = $3
		LIMIT 1
	`, scanCode, scanCode, scanCode).Scan(
		&device.DeviceID, &device.ProductID, &device.SerialNumber,
		&device.Barcode, &device.QRCode, &device.Status,
		&device.CurrentLocation, &device.ZoneID, &device.ConditionRating, &device.UsageHours,
	)
	if err != nil {
		return nil, err
	}

	// Get current job if on_job
	if device.Status == "on_job" || device.Status == "rented" {
		s.db.QueryRow(`
			SELECT jobID FROM job_devices WHERE deviceID = $1 LIMIT 1
		`, device.DeviceID).Scan(&device.CurrentJobID)
	}

	return &device, nil
}

// getDeviceWithDetails fetches device with related data
func (s *ScanService) getDeviceWithDetails(deviceID string) *models.DeviceWithDetails {
	var device models.DeviceWithDetails
	err := s.db.QueryRow(`
		SELECT d.deviceID, d.productID, d.serialnumber, d.barcode, d.qr_code, d.status,
		       d.current_location, d.zone_id, d.condition_rating, d.usage_hours,
		       COALESCE(p.name, '') as product_name,
		       COALESCE(z.name, '') as zone_name,
		       COALESCE(c.name, '') as case_name,
		       COALESCE(CAST(j.jobID AS CHAR), '') as job_number
		FROM devices d
		LEFT JOIN products p ON d.productID = p.productID
		LEFT JOIN storage_zones z ON d.zone_id = z.zone_id
		LEFT JOIN devicescases dc ON d.deviceID = dc.deviceID
		LEFT JOIN cases c ON dc.caseID = c.caseID
		LEFT JOIN job_devices jd ON d.deviceID = jd.deviceID
		LEFT JOIN jobs j ON jd.jobID = j.jobID
		WHERE d.deviceID = $1
		LIMIT 1
	`, deviceID).Scan(
		&device.DeviceID, &device.ProductID, &device.SerialNumber,
		&device.Barcode, &device.QRCode, &device.Status,
		&device.CurrentLocation, &device.ZoneID, &device.ConditionRating, &device.UsageHours,
		&device.ProductName, &device.ZoneName, &device.CaseName, &device.JobNumber,
	)
	if err != nil {
		log.Printf("Error fetching device details: %v", err)
		return nil
	}
	return &device
}

// logScanEvent records a scan event
func (s *ScanService) logScanEvent(tx *sql.Tx, scanCode string, deviceID *string, action string, jobID, zoneID, userID *int64, success bool, errorMsg, ipAddr, userAgent string) {
	scanResult := "success"
	if !success {
		scanResult = "error"
	}
	meta := fmt.Sprintf(`{"job_id":%s,"error_message":"%s","ip_address":"%s","user_agent":"%s"}`,
		nullInt64JSON(jobID), escapeJSON(errorMsg), escapeJSON(ipAddr), escapeJSON(userAgent))
	_, err := tx.Exec(`
		INSERT INTO scan_events
		(device_id, zone_id, scan_type, barcode_value, scanned_by, scan_result, metadata)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`, deviceID, zoneID, action, scanCode, userID, scanResult, meta)
	if err != nil {
		log.Printf("Failed to log scan event: %v", err)
	}
}

func nullInt64JSON(v *int64) string {
	if v == nil {
		return "null"
	}
	return strconv.FormatInt(*v, 10)
}

func escapeJSON(s string) string {
	result := make([]byte, 0, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == '"' || c == '\\' {
			result = append(result, '\\')
		}
		result = append(result, c)
	}
	return string(result)
}

// syncProductStockFromLocations recalculates product.stock_quantity from sum of product_locations
func (s *ScanService) syncProductStockFromLocations(productID int64) error {
	_, err := s.db.Exec(`
		UPDATE products
		SET stock_quantity = (
			SELECT COALESCE(SUM(quantity), 0)
			FROM product_locations
			WHERE product_id = $1
		)
		WHERE productID = $2
		AND (is_consumable = TRUE OR is_accessory = TRUE)
	`, productID, productID)
	if err != nil {
		log.Printf("Warning: Failed to sync stock_quantity for product %d: %v", productID, err)
	}
	return err
}

// updateLEDsAfterOuttake updates LED colors for the zone after a device is taken out
func (s *ScanService) updateLEDsAfterOuttake(jobID int64, fromZoneID int64) {
	// Get zone code from zone ID
	var zoneCode string
	err := s.db.QueryRow(`
		SELECT code FROM storage_zones WHERE zone_id = $1
	`, fromZoneID).Scan(&zoneCode)
	if err != nil {
		log.Printf("[LED] Failed to get zone code for zone_id %d: %v", fromZoneID, err)
		return
	}

	if zoneCode == "" {
		log.Printf("[LED] Zone %d has no code, skipping LED update", fromZoneID)
		return
	}

	// Update LED for this bin
	ledService := led.GetService()
	jobIDStr := strconv.FormatInt(jobID, 10)

	if err := ledService.UpdateBinAfterScan(jobIDStr, zoneCode); err != nil {
		log.Printf("[LED] Failed to update bin %s after scan: %v", zoneCode, err)
	} else {
		log.Printf("[LED] Successfully updated bin %s after device removal for job %d", zoneCode, jobID)
	}
}

// ConsumableProduct represents a consumable or accessory product
type ConsumableProduct struct {
	ProductID    int64
	Name         string
	IsConsumable bool
	IsAccessory  bool
	Barcode      sql.NullString
	Unit         string // e.g., "kg", "l", "Stk"
}

// findConsumableByScan looks up a consumable/accessory by barcode or product ID
func (s *ScanService) findConsumableByScan(scanCode string) (*ConsumableProduct, error) {
	var product ConsumableProduct
	err := s.db.QueryRow(`
		SELECT p.productID, p.name, p.is_consumable, p.is_accessory, p.generic_barcode,
		       COALESCE(ct.abbreviation, 'Stk') as unit
		FROM products p
		LEFT JOIN count_types ct ON p.count_type_id = ct.count_type_id
		WHERE (p.is_consumable = TRUE OR p.is_accessory = TRUE)
		  AND (p.generic_barcode = $1 OR CAST(p.productID AS CHAR) = $2)
		LIMIT 1
	`, scanCode, scanCode).Scan(
		&product.ProductID, &product.Name, &product.IsConsumable,
		&product.IsAccessory, &product.Barcode, &product.Unit,
	)
	if err != nil {
		return nil, err
	}
	return &product, nil
}

// processConsumableScan handles scanning of consumables/accessories
func (s *ScanService) processConsumableScan(tx *sql.Tx, product *ConsumableProduct, req models.ScanRequest, userID *int64, ipAddr, userAgent string) (*models.ScanResponse, error) {
	productIDStr := fmt.Sprintf("PROD-%d", product.ProductID)

	// Handle different actions
	switch req.Action {
	case "intake":
		return s.processConsumableIntake(tx, product, req.ZoneID, req.JobID, &productIDStr, req.ScanCode, userID, ipAddr, userAgent)
	case "outtake":
		return s.processConsumableOuttake(tx, product, req.ZoneID, req.JobID, &productIDStr, req.ScanCode, userID, ipAddr, userAgent)
	case "check":
		return s.processConsumableCheck(tx, product, &productIDStr, req.ScanCode, userID, ipAddr, userAgent)
	default:
		err := fmt.Errorf("unsupported action for consumables: %s", req.Action)
		s.logScanEvent(tx, req.ScanCode, &productIDStr, req.Action, req.JobID, req.ZoneID, userID, false, err.Error(), ipAddr, userAgent)
		tx.Commit()
		return &models.ScanResponse{
			Success: false,
			Message: err.Error(),
		}, nil
	}
}

// processConsumableIntake increases stock when returning consumable to warehouse
func (s *ScanService) processConsumableIntake(tx *sql.Tx, product *ConsumableProduct, zoneID *int64, jobID *int64, productIDStr *string, scanCode string, userID *int64, ipAddr, userAgent string) (*models.ScanResponse, error) {
	if zoneID == nil {
		err := fmt.Errorf("zone_id is required for consumable intake")
		s.logScanEvent(tx, scanCode, productIDStr, "intake", nil, zoneID, userID, false, err.Error(), ipAddr, userAgent)
		tx.Commit()
		return &models.ScanResponse{
			Success: false,
			Message: err.Error(),
		}, nil
	}

	// Get quantity from JobID field (frontend passes quantity via this field as a workaround)
	quantity := 1.0
	if jobID != nil && *jobID > 0 {
		quantity = float64(*jobID)
	}

	// Update or insert stock in product_locations (single source of truth)
	_, err := tx.Exec(`
		INSERT INTO product_locations (product_id, zone_id, quantity)
		VALUES ($1, $2, $3)
		ON CONFLICT (product_id, zone_id) DO UPDATE SET quantity = product_locations.quantity + $4
	`, product.ProductID, *zoneID, quantity, quantity)
	if err != nil {
		s.logScanEvent(tx, scanCode, productIDStr, "intake", nil, zoneID, userID, false, err.Error(), ipAddr, userAgent)
		tx.Commit()
		return &models.ScanResponse{
			Success: false,
			Message: fmt.Sprintf("Failed to update stock: %v", err),
		}, nil
	}

	// Also update products.stock_quantity to keep it in sync (calculated from sum)
	_, err = tx.Exec(`
		UPDATE products
		SET stock_quantity = (SELECT COALESCE(SUM(quantity), 0) FROM product_locations WHERE product_id = $1)
		WHERE productID = $2
	`, product.ProductID, product.ProductID)
	if err != nil {
		log.Printf("Warning: failed to sync products.stock_quantity: %v", err)
	}

	s.logScanEvent(tx, scanCode, productIDStr, "intake", nil, zoneID, userID, true, "", ipAddr, userAgent)

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("failed to commit: %w", err)
	}

	return &models.ScanResponse{
		Success: true,
		Message: fmt.Sprintf("%s: +%.1f %s returned to warehouse", product.Name, quantity, product.Unit),
		Action:  "intake",
	}, nil
}

// processConsumableOuttake decreases stock when taking consumable from warehouse
func (s *ScanService) processConsumableOuttake(tx *sql.Tx, product *ConsumableProduct, zoneID *int64, jobID *int64, productIDStr *string, scanCode string, userID *int64, ipAddr, userAgent string) (*models.ScanResponse, error) {
	// Get quantity from JobID field (frontend passes quantity via this field as a workaround)
	quantity := 1.0
	if jobID != nil && *jobID > 0 {
		quantity = float64(*jobID)
	}

	// If no zone specified, automatically select the zone with the most stock
	var selectedZoneID sql.NullInt64
	var currentStock float64
	var err error

	if zoneID == nil {
		// Auto-select zone with most stock
		err = tx.QueryRow(`
			SELECT zone_id, quantity
			FROM product_locations
			WHERE product_id = $1 AND quantity >= $2
			ORDER BY quantity DESC
			LIMIT 1
		`, product.ProductID, quantity).Scan(&selectedZoneID, &currentStock)

		if err == sql.ErrNoRows {
			err = fmt.Errorf("no stock available for %s", product.Name)
			s.logScanEvent(tx, scanCode, productIDStr, "outtake", jobID, nil, userID, false, err.Error(), ipAddr, userAgent)
			tx.Commit()
			return &models.ScanResponse{
				Success: false,
				Message: err.Error(),
			}, nil
		} else if err != nil {
			s.logScanEvent(tx, scanCode, productIDStr, "outtake", jobID, nil, userID, false, err.Error(), ipAddr, userAgent)
			tx.Commit()
			return &models.ScanResponse{
				Success: false,
				Message: fmt.Sprintf("Failed to find stock location: %v", err),
			}, nil
		}

		// Set zoneID to the selected zone (might be NULL)
		if selectedZoneID.Valid {
			zoneIDValue := selectedZoneID.Int64
			zoneID = &zoneIDValue
		} else {
			// Zone is NULL - this is valid for product_locations
			zoneID = nil
		}
	} else {
		// Use specified zone - check stock
		err = tx.QueryRow(`
			SELECT COALESCE(quantity, 0) FROM product_locations
			WHERE product_id = $1 AND zone_id = $2
		`, product.ProductID, *zoneID).Scan(&currentStock)
		if err != nil && err != sql.ErrNoRows {
			s.logScanEvent(tx, scanCode, productIDStr, "outtake", jobID, zoneID, userID, false, err.Error(), ipAddr, userAgent)
			tx.Commit()
			return &models.ScanResponse{
				Success: false,
				Message: fmt.Sprintf("Failed to check stock: %v", err),
			}, nil
		}
	}

	if currentStock < quantity {
		err = fmt.Errorf("insufficient stock (available: %.0f, requested: %.0f)", currentStock, quantity)
		s.logScanEvent(tx, scanCode, productIDStr, "outtake", jobID, zoneID, userID, false, err.Error(), ipAddr, userAgent)
		tx.Commit()
		return &models.ScanResponse{
			Success: false,
			Message: err.Error(),
		}, nil
	}

	// Decrease stock in product_locations (single source of truth)
	result, err := tx.Exec(`
		UPDATE product_locations SET quantity = quantity - $1
		WHERE product_id = $2 AND zone_id IS NOT DISTINCT FROM $3
	`, quantity, product.ProductID, zoneID)
	if err != nil {
		s.logScanEvent(tx, scanCode, productIDStr, "outtake", jobID, zoneID, userID, false, err.Error(), ipAddr, userAgent)
		tx.Commit()
		return &models.ScanResponse{
			Success: false,
			Message: fmt.Sprintf("Failed to update stock: %v", err),
		}, nil
	}

	// Verify the update actually affected a row
	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		err := fmt.Errorf("no stock location found for zone_id=%v", zoneID)
		s.logScanEvent(tx, scanCode, productIDStr, "outtake", jobID, zoneID, userID, false, err.Error(), ipAddr, userAgent)
		tx.Commit()
		return &models.ScanResponse{
			Success: false,
			Message: err.Error(),
		}, nil
	}

	// Also update products.stock_quantity to keep it in sync (calculated from sum)
	_, err = tx.Exec(`
		UPDATE products
		SET stock_quantity = (SELECT COALESCE(SUM(quantity), 0) FROM product_locations WHERE product_id = $1)
		WHERE productID = $2
	`, product.ProductID, product.ProductID)
	if err != nil {
		log.Printf("Warning: failed to sync products.stock_quantity: %v", err)
	}

	s.logScanEvent(tx, scanCode, productIDStr, "outtake", jobID, zoneID, userID, true, "", ipAddr, userAgent)

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("failed to commit: %w", err)
	}

	return &models.ScanResponse{
		Success: true,
		Message: fmt.Sprintf("%s: -%.1f %s taken from warehouse", product.Name, quantity, product.Unit),
		Action:  "outtake",
	}, nil
}

// processConsumableCheck shows consumable stock status
func (s *ScanService) processConsumableCheck(tx *sql.Tx, product *ConsumableProduct, productIDStr *string, scanCode string, userID *int64, ipAddr, userAgent string) (*models.ScanResponse, error) {
	// Get total stock
	var totalStock float64
	err := s.db.QueryRow(`
		SELECT COALESCE(stock_quantity, 0) FROM products WHERE productID = $1
	`, product.ProductID).Scan(&totalStock)
	if err != nil {
		s.logScanEvent(tx, scanCode, productIDStr, "check", nil, nil, userID, false, err.Error(), ipAddr, userAgent)
		tx.Commit()
		return &models.ScanResponse{
			Success: false,
			Message: fmt.Sprintf("Failed to get stock: %v", err),
		}, nil
	}

	s.logScanEvent(tx, scanCode, productIDStr, "check", nil, nil, userID, true, "", ipAddr, userAgent)
	tx.Commit()

	productType := "Consumable"
	if product.IsAccessory {
		productType = "Accessory"
	}

	return &models.ScanResponse{
		Success: true,
		Message: fmt.Sprintf("%s: %s (Stock: %.0f %s)", product.Name, productType, totalStock, product.Unit),
		Action:  "check",
		Product: &models.ProductInfo{
			ProductID:    int(product.ProductID),
			Name:         product.Name,
			Unit:         product.Unit,
			Stock:        totalStock,
			IsAccessory:  product.IsAccessory,
			IsConsumable: product.IsConsumable,
		},
	}, nil
}
