package models

import (
	"database/sql"
	"time"
)

// ScanEvent represents a barcode or QR code scan event
type ScanEvent struct {
	ScanID      int64          `json:"scan_id" db:"scan_id"`
	ScanCode    string         `json:"scan_code" db:"scan_code"`
	ScanType    string         `json:"scan_type" db:"scan_type"` // barcode, qr_code
	DeviceID    sql.NullString `json:"device_id" db:"device_id"`
	Action      sql.NullString `json:"action" db:"action"`
	JobID       sql.NullInt64  `json:"job_id" db:"job_id"`
	ZoneID      sql.NullInt64  `json:"zone_id" db:"zone_id"`
	UserID      sql.NullInt64  `json:"user_id" db:"user_id"`
	Success     bool           `json:"success" db:"success"`
	ErrorMessage sql.NullString `json:"error_message" db:"error_message"`
	Metadata    sql.NullString `json:"metadata" db:"metadata"` // JSON field
	IPAddress   sql.NullString `json:"ip_address" db:"ip_address"`
	UserAgent   sql.NullString `json:"user_agent" db:"user_agent"`
	Timestamp   time.Time      `json:"timestamp" db:"timestamp"`
}

// ScanRequest represents an incoming scan request
type ScanRequest struct {
	ScanCode   string  `json:"scan_code" binding:"required"`
	Action     string  `json:"action" binding:"required"` // intake, outtake, check
	JobID      *int64  `json:"job_id,omitempty"`
	ZoneID     *int64  `json:"zone_id,omitempty"`
	Notes      string  `json:"notes,omitempty"`
}

// ScanResponse represents the response to a scan request
type ScanResponse struct {
	Success          bool                          `json:"success"`
	Message          string                        `json:"message"`
	Device           *DeviceWithDetails             `json:"device,omitempty"`
	Product          *ProductInfo                   `json:"product,omitempty"`
	Movement         *DeviceMovement                `json:"movement,omitempty"`
	Action           string                         `json:"action"`
	PreviousStatus   string                         `json:"previous_status,omitempty"`
	NewStatus        string                         `json:"new_status,omitempty"`
	Duplicate        bool                           `json:"duplicate"`
	JobInfo          *JobInfo                       `json:"job_info,omitempty"`
	SuggestedDeps    []ProductDependencyWithDetails `json:"suggested_dependencies,omitempty"`
	Swapped          bool                           `json:"swapped,omitempty"`
	SwappedFrom      string                         `json:"swapped_from,omitempty"`
}

// JobInfo contains basic job information for scan responses
type JobInfo struct {
	JobID       int64  `json:"job_id"`
	JobNumber   string `json:"job_number"`
	CustomerName string `json:"customer_name,omitempty"`
	EventDate   string `json:"event_date,omitempty"`
}

// ProductInfo contains basic product information for scan responses (consumables/accessories)
type ProductInfo struct {
	ProductID    int     `json:"product_id"`
	Name         string  `json:"name"`
	Unit         string  `json:"unit"` // e.g., "kg", "l", "Stk"
	Stock        float64 `json:"stock"`
	IsAccessory  bool    `json:"is_accessory"`
	IsConsumable bool    `json:"is_consumable"`
}
