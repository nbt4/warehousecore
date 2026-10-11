package models

import (
	"database/sql"
)

// Case represents a storage case (from existing table)
type Case struct {
	CaseID      int             `json:"case_id" db:"caseID"`
	Name        string          `json:"name" db:"name"`
	Description sql.NullString  `json:"description" db:"description"`
	Width       sql.NullFloat64 `json:"width" db:"width"`
	Height      sql.NullFloat64 `json:"height" db:"height"`
	Depth       sql.NullFloat64 `json:"depth" db:"depth"`
	Weight      sql.NullFloat64 `json:"weight" db:"weight"`
	Status      string          `json:"status" db:"status"`
	ZoneID      sql.NullInt64   `json:"zone_id,omitempty" db:"zone_id"`
	Barcode     sql.NullString  `json:"barcode,omitempty" db:"barcode"`
	RFIDTag     sql.NullString  `json:"rfid_tag,omitempty" db:"rfid_tag"`
}

// CaseWithContents includes devices inside the case
type CaseWithContents struct {
	Case
	DeviceCount int      `json:"device_count" db:"device_count"`
	Devices     []Device `json:"devices,omitempty"`
	ZoneName    string   `json:"zone_name,omitempty" db:"zone_name"`
}

// DeviceCase represents the relationship between devices and cases (existing table)
type DeviceCase struct {
	DeviceID string `json:"device_id" db:"deviceID"`
	CaseID   int    `json:"case_id" db:"caseID"`
}
