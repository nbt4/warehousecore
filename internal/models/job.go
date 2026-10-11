package models

import (
	"database/sql"
)

// Job represents a rental job (from existing RentalCore table)
type Job struct {
	JobID         int64          `json:"job_id" db:"jobID"`
	JobNumber     string         `json:"job_number" db:"jobnumber"`
	CustomerID    int64          `json:"customer_id" db:"customerID"`
	EventDate     sql.NullTime   `json:"event_date" db:"eventDate"`
	EventLocation sql.NullString `json:"event_location" db:"eventLocation"`
	Status        string         `json:"status" db:"status"`
	Notes         sql.NullString `json:"notes" db:"notes"`
}

// JobDevice represents a device assigned to a job (existing table)
type JobDevice struct {
	JobID      int64  `json:"job_id" db:"jobID"`
	DeviceID   string `json:"device_id" db:"deviceID"`
	PackStatus string `json:"pack_status" db:"pack_status"` // pending, packed, issued, returned
}

// JobSummary contains job information with device counts
type JobSummary struct {
	Job
	CustomerName    string              `json:"customer_name" db:"customer_name"`
	TotalDevices    int                 `json:"total_devices"`
	PackedDevices   int                 `json:"packed_devices"`
	MissingDevices  int                 `json:"missing_devices"`
	ImportedDevices int                 `json:"imported_devices"`
	AssignedDevices []DeviceWithDetails `json:"assigned_devices,omitempty"`
	PackedList      []string            `json:"packed_list,omitempty"`
	MissingList     []string            `json:"missing_list,omitempty"`
}

// JobCompleteRequest represents a request to complete a job
type JobCompleteRequest struct {
	JobID     int64  `json:"job_id" binding:"required"`
	Notes     string `json:"notes,omitempty"`
	ForcedEnd bool   `json:"forced_end"` // true if manually ended with missing items
}
