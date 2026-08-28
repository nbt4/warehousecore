package models

import (
	"database/sql"
	"time"
)

// DeviceStatus represents the current status of a device
type DeviceStatus string

const (
	StatusInStorage       DeviceStatus = "in_storage"
	StatusOnJob           DeviceStatus = "on_job"
	StatusReturnPending   DeviceStatus = "return_pending"
	StatusLocationUnknown DeviceStatus = "location_unknown"
)

// DeviceCondition describes whether a device may be used. It is deliberately
// independent from DeviceStatus, which only describes the physical flow.
type DeviceCondition string

const (
	ConditionAvailable   DeviceCondition = "available"
	ConditionBlocked     DeviceCondition = "blocked"
	ConditionDefective   DeviceCondition = "defective"
	ConditionMaintenance DeviceCondition = "maintenance"
	ConditionRetired     DeviceCondition = "retired"
)

// Device represents a physical device in the warehouse
type Device struct {
	DeviceID          string         `json:"device_id" db:"deviceID"`
	ProductID         sql.NullInt64  `json:"product_id" db:"productID"`
	ProductName       string         `json:"product_name,omitempty" db:"product_name"`
	SerialNumber      sql.NullString `json:"serial_number" db:"serialnumber"`
	Barcode           sql.NullString `json:"barcode" db:"barcode"`
	QRCode            sql.NullString `json:"qr_code" db:"qr_code"`
	Status            string         `json:"status" db:"status"`
	ConditionStatus   string         `json:"condition_status" db:"condition_status"`
	CurrentLocation   sql.NullString `json:"current_location" db:"current_location"`
	ZoneID            sql.NullInt64  `json:"zone_id,omitempty" db:"zone_id"`
	CaseID            sql.NullInt64  `json:"case_id,omitempty" db:"case_id"`
	CurrentJobID      sql.NullInt64  `json:"current_job_id,omitempty" db:"current_job_id"`
	CurrentJobCode    string         `json:"current_job_code,omitempty" db:"current_job_code"`
	CurrentJobStatus  string         `json:"current_job_status,omitempty" db:"current_job_status"`
	CurrentPackStatus string         `json:"current_pack_status,omitempty" db:"current_pack_status"`
	ConditionRating   float64        `json:"condition_rating" db:"condition_rating"`
	UsageHours        float64        `json:"usage_hours" db:"usage_hours"`
	PurchaseDate      sql.NullTime   `json:"purchase_date" db:"purchaseDate"`
	LastMaintenance   sql.NullTime   `json:"last_maintenance" db:"lastmaintenance"`
	NextMaintenance   sql.NullTime   `json:"next_maintenance" db:"nextmaintenance"`
	Notes             sql.NullString `json:"notes" db:"notes"`
	ImageURL          sql.NullString `json:"image_url,omitempty"`
}

// DeviceWithDetails includes related product and location information
type DeviceWithDetails struct {
	Device
	ProductName     string         `json:"product_name" db:"product_name"`
	ProductCategory string         `json:"product_category,omitempty" db:"product_category"`
	ZoneName        string         `json:"zone_name,omitempty" db:"zone_name"`
	ZoneCode        string         `json:"zone_code,omitempty" db:"zone_code"`
	CaseName        string         `json:"case_name,omitempty" db:"case_name"`
	JobNumber       string         `json:"job_number,omitempty" db:"job_number"`
	StatusLabel     string         `json:"status_label,omitempty"`
	StatusDetail    string         `json:"status_detail,omitempty"`
	NeedsReturn     bool           `json:"needs_return"`
	LabelPath       sql.NullString `json:"label_path,omitempty" db:"label_path"`
}

// DeviceFilter represents query filters for devices
type DeviceFilter struct {
	Status     string
	ZoneID     *int64
	CaseID     *int64
	JobID      *int64
	SearchTerm string
	Limit      int
	Offset     int
}

// ScanResult represents the result of a barcode/QR scan
type ScanResult struct {
	Success     bool      `json:"success"`
	Device      *Device   `json:"device,omitempty"`
	Message     string    `json:"message"`
	Action      string    `json:"action"`
	PreviousJob *int64    `json:"previous_job,omitempty"`
	AssignedJob *int64    `json:"assigned_job,omitempty"`
	Duplicate   bool      `json:"duplicate"`
	Timestamp   time.Time `json:"timestamp"`
}
