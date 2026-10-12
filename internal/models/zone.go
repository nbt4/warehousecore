package models

import (
	"database/sql"
	"time"
)

// ZoneKind represents the type of storage zone
type ZoneKind string

const (
	ZoneKindShelf     ZoneKind = "shelf"
	ZoneKindRack      ZoneKind = "rack"
	ZoneKindCase      ZoneKind = "case"
	ZoneKindVehicle   ZoneKind = "vehicle"
	ZoneKindStage     ZoneKind = "stage"
	ZoneKindWarehouse ZoneKind = "warehouse"
	ZoneKindOther     ZoneKind = "other"
)

// Zone represents a logical storage area in the warehouse
type Zone struct {
	ZoneID       int64          `json:"zone_id" db:"zone_id"`
	Code         string         `json:"code" db:"code"`
	Barcode      sql.NullString `json:"barcode" db:"barcode"`
	Name         string         `json:"name" db:"name"`
	Type         string         `json:"type" db:"type"`
	Description  sql.NullString `json:"description" db:"description"`
	ParentZoneID sql.NullInt64  `json:"parent_zone_id" db:"parent_zone_id"`
	Capacity     sql.NullInt64  `json:"capacity" db:"capacity"`
	CurrentCount int            `json:"current_count" db:"current_count"`
	Location     sql.NullString `json:"location" db:"location"`
	Metadata     sql.NullString `json:"metadata" db:"metadata"` // JSON field for flexible attributes
	IsActive     bool           `json:"is_active" db:"is_active"`
	CreatedAt    time.Time      `json:"created_at" db:"created_at"`
	UpdatedAt    time.Time      `json:"updated_at" db:"updated_at"`
}

// ZoneWithDetails includes device and case counts
type ZoneWithDetails struct {
	Zone
	DeviceCount    int    `json:"device_count" db:"device_count"`
	CaseCount      int    `json:"case_count" db:"case_count"`
	ParentZoneName string `json:"parent_zone_name,omitempty" db:"parent_zone_name"`
}
