package models

import (
	"database/sql"
	"time"
)

// DefectSeverity represents how severe a defect is
type DefectSeverity string

const (
	SeverityLow      DefectSeverity = "low"
	SeverityMedium   DefectSeverity = "medium"
	SeverityHigh     DefectSeverity = "high"
	SeverityCritical DefectSeverity = "critical"
)

// DefectStatus represents the current status of a defect report
type DefectStatus string

const (
	DefectOpen       DefectStatus = "open"
	DefectInProgress DefectStatus = "in_progress"
	DefectRepaired   DefectStatus = "repaired"
	DefectClosed     DefectStatus = "closed"
)

// DefectReport represents a reported defect on a device
type DefectReport struct {
	DefectID    int64           `json:"defect_id" db:"defect_id"`
	DeviceID    string          `json:"device_id" db:"device_id"`
	Severity    string          `json:"severity" db:"severity"`
	Status      string          `json:"status" db:"status"`
	Title       string          `json:"title" db:"title"`
	Description string          `json:"description" db:"description"`
	ReportedBy  sql.NullInt64   `json:"reported_by" db:"reported_by"`
	ReportedAt  time.Time       `json:"reported_at" db:"reported_at"`
	AssignedTo  sql.NullInt64   `json:"assigned_to" db:"assigned_to"`
	RepairedBy  sql.NullInt64   `json:"repaired_by" db:"repaired_by"`
	RepairedAt  sql.NullTime    `json:"repaired_at" db:"repaired_at"`
	RepairCost  sql.NullFloat64 `json:"repair_cost" db:"repair_cost"`
	RepairNotes sql.NullString  `json:"repair_notes" db:"repair_notes"`
	ClosedAt    sql.NullTime    `json:"closed_at" db:"closed_at"`
	Images      sql.NullString  `json:"images" db:"images"`     // JSON array of image URLs
	Metadata    sql.NullString  `json:"metadata" db:"metadata"` // JSON field
}

// DefectReportWithDetails includes device and user information
type DefectReportWithDetails struct {
	DefectReport
	DeviceName     string `json:"device_name,omitempty" db:"device_name"`
	ProductName    string `json:"product_name,omitempty" db:"product_name"`
	ReportedByName string `json:"reported_by_name,omitempty" db:"reported_by_name"`
	AssignedToName string `json:"assigned_to_name,omitempty" db:"assigned_to_name"`
	RepairedByName string `json:"repaired_by_name,omitempty" db:"repaired_by_name"`
}

// MaintenanceLog represents a maintenance activity (from existing table)
type MaintenanceLog struct {
	MaintenanceLogID int             `json:"maintenance_log_id" db:"maintenanceLogID"`
	DeviceID         string          `json:"device_id" db:"deviceID"`
	Date             sql.NullTime    `json:"date" db:"date"`
	EmployeeID       sql.NullInt64   `json:"employee_id" db:"employeeID"`
	Cost             sql.NullFloat64 `json:"cost" db:"cost"`
	Notes            sql.NullString  `json:"notes" db:"notes"`
}

// InspectionSchedule represents periodic inspection requirements
type InspectionSchedule struct {
	ScheduleID     int64          `json:"schedule_id" db:"schedule_id"`
	DeviceID       sql.NullString `json:"device_id" db:"device_id"`
	ProductID      sql.NullInt64  `json:"product_id" db:"product_id"`
	InspectionType string         `json:"inspection_type" db:"inspection_type"`
	IntervalDays   int            `json:"interval_days" db:"interval_days"`
	LastInspection sql.NullTime   `json:"last_inspection" db:"last_inspection"`
	NextInspection sql.NullTime   `json:"next_inspection" db:"next_inspection"`
	IsActive       bool           `json:"is_active" db:"is_active"`
	Notes          sql.NullString `json:"notes" db:"notes"`
	CreatedAt      time.Time      `json:"created_at" db:"created_at"`
	UpdatedAt      time.Time      `json:"updated_at" db:"updated_at"`
}
