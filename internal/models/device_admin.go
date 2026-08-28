package models

// DeviceCreateInput represents the payload for creating one or more devices via the admin API.
type DeviceCreateInput struct {
	ProductID         int      `json:"product_id"`
	Status            string   `json:"status,omitempty"`
	ConditionStatus   string   `json:"condition_status,omitempty"`
	SerialNumber      *string  `json:"serial_number,omitempty"`
	Barcode           *string  `json:"barcode,omitempty"`
	QRCode            *string  `json:"qr_code,omitempty"`
	CurrentLocation   *string  `json:"current_location,omitempty"`
	ZoneID            *int     `json:"zone_id,omitempty"`
	ConditionRating   *float64 `json:"condition_rating,omitempty"`
	UsageHours        *float64 `json:"usage_hours,omitempty"`
	PurchaseDate      *string  `json:"purchase_date,omitempty"`
	LastMaintenance   *string  `json:"last_maintenance,omitempty"`
	NextMaintenance   *string  `json:"next_maintenance,omitempty"`
	Notes             *string  `json:"notes,omitempty"`
	Quantity          int      `json:"quantity,omitempty"`
	AutoGenerateLabel *bool    `json:"auto_generate_label,omitempty"`
	LabelTemplateID   *int     `json:"label_template_id,omitempty"`
	RegenerateCodes   *bool    `json:"regenerate_codes,omitempty"`
	DevicePrefix      *string  `json:"device_prefix,omitempty"`
	StartingSerial    *int     `json:"starting_serial,omitempty"`
	IncrementSerial   bool     `json:"increment_serial,omitempty"`
}

// DeviceUpdateInput represents the payload for updating an existing device.
type DeviceUpdateInput struct {
	ProductID       Optional[int]     `json:"product_id"`
	Status          Optional[string]  `json:"status"`
	ConditionStatus Optional[string]  `json:"condition_status"`
	SerialNumber    Optional[string]  `json:"serial_number"`
	Barcode         Optional[string]  `json:"barcode"`
	QRCode          Optional[string]  `json:"qr_code"`
	CurrentLocation Optional[string]  `json:"current_location"`
	ZoneID          Optional[int]     `json:"zone_id"`
	ConditionRating Optional[float64] `json:"condition_rating"`
	UsageHours      Optional[float64] `json:"usage_hours"`
	PurchaseDate    Optional[string]  `json:"purchase_date"`
	LastMaintenance Optional[string]  `json:"last_maintenance"`
	NextMaintenance Optional[string]  `json:"next_maintenance"`
	Notes           Optional[string]  `json:"notes"`
	RegenerateLabel Optional[bool]    `json:"regenerate_label"`
	LabelTemplateID Optional[int]     `json:"label_template_id"`
	RegenerateCodes Optional[bool]    `json:"regenerate_codes"`
}
