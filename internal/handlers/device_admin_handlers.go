package handlers

import (
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strings"

	"github.com/gorilla/mux"
	"warehousecore/internal/models"
	"warehousecore/internal/repository"
	"warehousecore/internal/services"
)

type DeviceAdminResponse struct {
	DeviceID        string  `json:"device_id"`
	ProductID       *int    `json:"product_id,omitempty"`
	ProductName     string  `json:"product_name,omitempty"`
	ProductCategory string  `json:"product_category,omitempty"`
	SerialNumber    *string `json:"serial_number,omitempty"`
	Barcode         *string `json:"barcode,omitempty"`
	QRCode          *string `json:"qr_code,omitempty"`
	Status          string  `json:"status"`
	ConditionStatus string  `json:"condition_status"`
	CurrentLocation *string `json:"current_location,omitempty"`
	ZoneID          *int    `json:"zone_id,omitempty"`
	ZoneName        string  `json:"zone_name,omitempty"`
	ZoneCode        string  `json:"zone_code,omitempty"`
	CaseID          *int    `json:"case_id,omitempty"`
	CaseName        string  `json:"case_name,omitempty"`
	CurrentJobID    *int    `json:"current_job_id,omitempty"`
	JobNumber       string  `json:"job_number,omitempty"`
	ConditionRating float64 `json:"condition_rating"`
	UsageHours      float64 `json:"usage_hours"`
	PurchaseDate    *string `json:"purchase_date,omitempty"`
	LastMaintenance *string `json:"last_maintenance,omitempty"`
	NextMaintenance *string `json:"next_maintenance,omitempty"`
	Notes           *string `json:"notes,omitempty"`
	LabelPath       *string `json:"label_path,omitempty"`
}

func formatNullTime(value sql.NullTime) *string {
	if !value.Valid {
		return nil
	}
	formatted := value.Time.Format("2006-01-02")
	return &formatted
}

func nullIntToPtr(n sql.NullInt64) *int {
	if !n.Valid {
		return nil
	}
	val := int(n.Int64)
	return &val
}

func toDeviceAdminResponse(device *models.DeviceWithDetails) DeviceAdminResponse {
	if device == nil {
		return DeviceAdminResponse{}
	}

	return DeviceAdminResponse{
		DeviceID:        device.DeviceID,
		ProductID:       nullIntToPtr(device.ProductID),
		ProductName:     device.ProductName,
		ProductCategory: device.ProductCategory,
		SerialNumber:    ptrString(device.SerialNumber),
		Barcode:         ptrString(device.Barcode),
		QRCode:          ptrString(device.QRCode),
		Status:          device.Status,
		ConditionStatus: device.ConditionStatus,
		CurrentLocation: ptrString(device.CurrentLocation),
		ZoneID:          nullIntToPtr(device.ZoneID),
		ZoneName:        device.ZoneName,
		ZoneCode:        device.ZoneCode,
		CaseID:          nullIntToPtr(device.CaseID),
		CaseName:        device.CaseName,
		CurrentJobID:    nullIntToPtr(device.CurrentJobID),
		JobNumber:       device.JobNumber,
		ConditionRating: device.ConditionRating,
		UsageHours:      device.UsageHours,
		PurchaseDate:    formatNullTime(device.PurchaseDate),
		LastMaintenance: formatNullTime(device.LastMaintenance),
		NextMaintenance: formatNullTime(device.NextMaintenance),
		Notes:           ptrString(device.Notes),
		LabelPath:       ptrString(device.LabelPath),
	}
}

// ===========================
// DEVICE ADMIN HANDLERS
// ===========================

// GetAllDevicesAdmin retrieves all devices with full details for admin use
func GetAllDevicesAdmin(w http.ResponseWriter, r *http.Request) {
	db := repository.GetSQLDB()

	query := `
		SELECT d.deviceID, d.productID, d.serialnumber, d.barcode, d.qr_code, d.status, d.condition_status,
		       d.current_location, d.zone_id,
		       d.condition_rating, d.usage_hours, d.purchaseDate, d.lastmaintenance, d.nextmaintenance,
		       d.notes, d.label_path,
		       COALESCE(p.name, '') AS product_name,
		       COALESCE(cat.name, '') AS product_category,
		       COALESCE(z.name, '') AS zone_name,
		       COALESCE(z.code, '') AS zone_code,
		       dc.caseID,
		       COALESCE(c.name, '') AS case_name,
		       jd.jobID,
		       COALESCE(j.job_code, '') AS job_number
		FROM devices d
		LEFT JOIN products p ON d.productID = p.productID
		LEFT JOIN categories cat ON p.categoryID = cat.categoryID
		LEFT JOIN storage_zones z ON d.zone_id = z.zone_id
		LEFT JOIN devicescases dc ON d.deviceID = dc.deviceID
		LEFT JOIN cases c ON dc.caseID = c.caseID
		LEFT JOIN job_devices jd ON d.deviceID = jd.deviceID
		LEFT JOIN jobs j ON jd.jobID = j.jobID
		ORDER BY d.deviceID DESC
	`

	rows, err := db.Query(query)
	if err != nil {
		log.Printf("[DEVICE LIST] Failed to query devices: %v", err)
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to fetch devices"})
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
			&device.ConditionStatus,
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
			log.Printf("[DEVICE LIST] Failed to scan device: %v", err)
			continue
		}

		responses = append(responses, toDeviceAdminResponse(&device))
	}

	respondJSON(w, http.StatusOK, responses)
}

// CreateDevice creates a single device or multiple devices with the admin service
func CreateDevice(w http.ResponseWriter, r *http.Request) {
	var input models.DeviceCreateInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid request body"})
		return
	}

	// Validate required fields
	if input.ProductID <= 0 {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "Product ID is required"})
		return
	}

	// Limit batch creation
	if input.Quantity > 100 {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "Cannot create more than 100 devices at once"})
		return
	}

	service := services.NewDeviceAdminService()
	devices, err := service.CreateDevices(r.Context(), &input)
	if err != nil {
		log.Printf("[DEVICE CREATE] Failed: %v", err)
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	// Return single device if quantity was 1, otherwise return array
	if len(devices) == 1 && input.Quantity <= 1 {
		respondJSON(w, http.StatusCreated, toDeviceAdminResponse(devices[0]))
	} else {
		responses := make([]DeviceAdminResponse, 0, len(devices))
		for _, device := range devices {
			responses = append(responses, toDeviceAdminResponse(device))
		}
		respondJSON(w, http.StatusCreated, responses)
	}
}

// UpdateDevice updates an existing device
func UpdateDevice(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	deviceID := vars["id"]
	if deviceID == "" {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "Device ID is required"})
		return
	}

	var input models.DeviceUpdateInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid request body"})
		return
	}

	service := services.NewDeviceAdminService()
	device, err := service.UpdateDevice(r.Context(), deviceID, &input)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			respondJSON(w, http.StatusNotFound, map[string]string{"error": "Device not found"})
			return
		}
		log.Printf("[DEVICE UPDATE] Failed for %s: %v", deviceID, err)
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	respondJSON(w, http.StatusOK, toDeviceAdminResponse(device))
}

// DeleteDevice deletes a device
func DeleteDevice(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	deviceID := vars["id"]
	if deviceID == "" {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "Device ID is required"})
		return
	}

	service := services.NewDeviceAdminService()
	err := service.DeleteDevice(r.Context(), deviceID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			respondJSON(w, http.StatusNotFound, map[string]string{"error": "Device not found"})
			return
		}
		log.Printf("[DEVICE DELETE] Failed for %s: %v", deviceID, err)
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	respondJSON(w, http.StatusOK, map[string]string{"message": "Device deleted successfully"})
}

// GetDeviceAdmin retrieves a single device with full details
func GetDeviceAdmin(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	deviceID := vars["id"]
	if deviceID == "" {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "Device ID is required"})
		return
	}

	service := services.NewDeviceAdminService()
	device, err := service.FetchDevice(r.Context(), deviceID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			respondJSON(w, http.StatusNotFound, map[string]string{"error": "Device not found"})
			return
		}
		log.Printf("[DEVICE GET] Failed for %s: %v", deviceID, err)
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	respondJSON(w, http.StatusOK, toDeviceAdminResponse(device))
}

// GenerateDeviceQR generates a QR code image for a device
func GenerateDeviceQR(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	deviceID := vars["id"]
	if deviceID == "" {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "Device ID is required"})
		return
	}

	// Verify device exists and get QR code value
	db := repository.GetSQLDB()
	var qrCode string
	err := db.QueryRow(`SELECT COALESCE(qr_code, $1) FROM devices WHERE deviceID = $2`,
		"QR-"+deviceID, deviceID).Scan(&qrCode)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			respondJSON(w, http.StatusNotFound, map[string]string{"error": "Device not found"})
			return
		}
		log.Printf("[DEVICE QR] Failed to fetch device %s: %v", deviceID, err)
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to fetch device"})
		return
	}

	// Generate QR code image (returns base64 string)
	labelService := services.NewLabelService()
	qrImageBase64, err := labelService.GenerateQRCode(qrCode, 256)
	if err != nil {
		log.Printf("[DEVICE QR] Failed to generate QR for %s: %v", deviceID, err)
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to generate QR code"})
		return
	}

	// Decode base64 to bytes
	const prefix = "data:image/png;base64,"
	if strings.HasPrefix(qrImageBase64, prefix) {
		qrImageBase64 = qrImageBase64[len(prefix):]
	}

	qrImageBytes, err := base64.StdEncoding.DecodeString(qrImageBase64)
	if err != nil {
		log.Printf("[DEVICE QR] Failed to decode base64 QR for %s: %v", deviceID, err)
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to decode QR code"})
		return
	}

	// Return PNG image
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Content-Disposition", "attachment; filename=\"device-"+deviceID+"-qr.png\"")
	w.Write(qrImageBytes)
}

// GenerateDeviceBarcode generates a barcode image for a device
func GenerateDeviceBarcode(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	deviceID := vars["id"]
	if deviceID == "" {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "Device ID is required"})
		return
	}

	// Verify device exists and get barcode value
	db := repository.GetSQLDB()
	var barcode string
	err := db.QueryRow(`SELECT COALESCE(barcode, $1) FROM devices WHERE deviceID = $2`,
		deviceID, deviceID).Scan(&barcode)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			respondJSON(w, http.StatusNotFound, map[string]string{"error": "Device not found"})
			return
		}
		log.Printf("[DEVICE BARCODE] Failed to fetch device %s: %v", deviceID, err)
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to fetch device"})
		return
	}

	// Generate barcode image (returns base64 string)
	labelService := services.NewLabelService()
	barcodeImageBase64, err := labelService.GenerateBarcode(barcode, 300, 100)
	if err != nil {
		log.Printf("[DEVICE BARCODE] Failed to generate barcode for %s: %v", deviceID, err)
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to generate barcode"})
		return
	}

	// Decode base64 to bytes
	const barcodePrefix = "data:image/png;base64,"
	if strings.HasPrefix(barcodeImageBase64, barcodePrefix) {
		barcodeImageBase64 = barcodeImageBase64[len(barcodePrefix):]
	}

	barcodeImageBytes, err := base64.StdEncoding.DecodeString(barcodeImageBase64)
	if err != nil {
		log.Printf("[DEVICE BARCODE] Failed to decode base64 barcode for %s: %v", deviceID, err)
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to decode barcode"})
		return
	}

	// Return PNG image
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Content-Disposition", "attachment; filename=\"device-"+deviceID+"-barcode.png\"")
	w.Write(barcodeImageBytes)
}
