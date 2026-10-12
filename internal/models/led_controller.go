package models

import "time"

// LEDController represents a physical LED controller device (e.g., ESP32)
type LEDController struct {
	ID              int        `json:"id" gorm:"column:id;primaryKey;autoIncrement"`
	ControllerID    string     `json:"controller_id" gorm:"column:controller_id;uniqueIndex;not null"`
	DisplayName     string     `json:"display_name" gorm:"column:display_name;not null"`
	TopicSuffix     string     `json:"topic_suffix" gorm:"column:topic_suffix;not null"`
	IsActive        bool       `json:"is_active" gorm:"column:is_active;not null;default:true"`
	LastSeen        *time.Time `json:"last_seen" gorm:"column:last_seen"`
	Metadata        JSONMap    `json:"metadata" gorm:"column:metadata;type:json"`
	IPAddress       *string    `json:"ip_address" gorm:"column:ip_address"`
	Hostname        *string    `json:"hostname" gorm:"column:hostname"`
	FirmwareVersion *string    `json:"firmware_version" gorm:"column:firmware_version"`
	MacAddress      *string    `json:"mac_address" gorm:"column:mac_address"`
	StatusData      JSONMap    `json:"status_data" gorm:"column:status_data;type:json"`
	CreatedAt       time.Time  `json:"created_at" gorm:"column:created_at"`
	UpdatedAt       time.Time  `json:"updated_at" gorm:"column:updated_at"`

	ZoneTypes []ZoneType `json:"zone_types,omitempty" gorm:"many2many:led_controller_zone_types;joinForeignKey:ControllerID;JoinReferences:ZoneTypeID"`
}

// TableName specifies table name for LEDController
func (LEDController) TableName() string {
	return "led_controllers"
}

// LEDControllerZoneType represents the mapping between controllers and zone types
type LEDControllerZoneType struct {
	ControllerID int       `json:"controller_id" gorm:"column:controller_id;primaryKey"`
	ZoneTypeID   int       `json:"zone_type_id" gorm:"column:zone_type_id;primaryKey"`
	CreatedAt    time.Time `json:"created_at" gorm:"column:created_at"`
}

// TableName specifies table name
func (LEDControllerZoneType) TableName() string {
	return "led_controller_zone_types"
}

// LEDControllerHeartbeat represents telemetry data sent by ESP controllers
type LEDControllerHeartbeat struct {
	ControllerID    string `json:"controller_id"`
	TopicSuffix     string `json:"topic_suffix"`
	WarehouseID     string `json:"warehouse_id"`
	IPAddress       string `json:"ip_address"`
	Hostname        string `json:"hostname"`
	FirmwareVersion string `json:"firmware_version"`
	MacAddress      string `json:"mac_address"`
	WifiRSSI        *int   `json:"wifi_rssi"`
	UptimeSeconds   *int64 `json:"uptime_seconds"`
	LedCount        *int   `json:"led_count"`
	ActiveLEDs      *int   `json:"active_leds"`
	Status          string `json:"status,omitempty"` // "online" or "offline"
}
