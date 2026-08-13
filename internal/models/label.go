package models

import (
	"time"
)

// LabelTemplate represents a saved label design
type LabelTemplate struct {
	ID           int       `json:"id" gorm:"primaryKey;column:id"`
	Name         string    `json:"name" gorm:"column:name;size:255;not null"`
	Description  string    `json:"description" gorm:"column:description;type:text"`
	Width        float64   `json:"width" gorm:"column:width;not null"`                               // in mm
	Height       float64   `json:"height" gorm:"column:height;not null"`                             // in mm
	TemplateJSON string    `json:"template_json" gorm:"column:template_json;type:longtext;not null"` // JSON with elements
	IsDefault    bool      `json:"is_default" gorm:"column:is_default;default:false"`
	TargetType   string    `json:"target_type" gorm:"column:target_type;size:20;not null;default:device"`
	Revision     int       `json:"revision" gorm:"column:revision;not null;default:1"`
	CreatedAt    time.Time `json:"created_at" gorm:"column:created_at"`
	UpdatedAt    time.Time `json:"updated_at" gorm:"column:updated_at"`
}

// LabelPrinter is a network printer profile used for direct label printing.
type LabelPrinter struct {
	ID        int       `json:"id" gorm:"primaryKey;column:id"`
	Name      string    `json:"name" gorm:"column:name;size:255;not null"`
	Driver    string    `json:"driver" gorm:"column:driver;size:30;not null"`
	Address   string    `json:"address" gorm:"column:address;size:255;not null"`
	Port      int       `json:"port" gorm:"column:port;not null;default:9100"`
	DPI       int       `json:"dpi" gorm:"column:dpi;not null;default:203"`
	IsDefault bool      `json:"is_default" gorm:"column:is_default;default:false"`
	IsActive  bool      `json:"is_active" gorm:"column:is_active;default:true"`
	CreatedAt time.Time `json:"created_at" gorm:"column:created_at"`
	UpdatedAt time.Time `json:"updated_at" gorm:"column:updated_at"`
}

func (LabelPrinter) TableName() string { return "label_printers" }

// LabelPrintJob records each requested direct-print operation.
type LabelPrintJob struct {
	ID           int64      `json:"id" gorm:"primaryKey;column:id"`
	TargetType   string     `json:"target_type" gorm:"column:target_type"`
	TargetID     string     `json:"target_id" gorm:"column:target_id"`
	TemplateID   *int       `json:"template_id" gorm:"column:template_id"`
	PrinterID    *int       `json:"printer_id" gorm:"column:printer_id"`
	Copies       int        `json:"copies" gorm:"column:copies"`
	Status       string     `json:"status" gorm:"column:status"`
	LabelPath    string     `json:"label_path" gorm:"column:label_path"`
	ErrorMessage string     `json:"error_message" gorm:"column:error_message"`
	CreatedAt    time.Time  `json:"created_at" gorm:"column:created_at"`
	StartedAt    *time.Time `json:"started_at" gorm:"column:started_at"`
	CompletedAt  *time.Time `json:"completed_at" gorm:"column:completed_at"`
	PrinterName  string     `json:"printer_name,omitempty" gorm:"column:printer_name;->"`
}

func (LabelPrintJob) TableName() string { return "label_print_jobs" }

func (LabelTemplate) TableName() string {
	return "label_templates"
}

// LabelElement represents an element in a label design (stored in TemplateJSON)
type LabelElement struct {
	Type      string            `json:"type"` // "barcode", "qrcode", "text", "image"
	X         float64           `json:"x"`
	Y         float64           `json:"y"`
	Width     float64           `json:"width"`
	Height    float64           `json:"height"`
	Rotation  float64           `json:"rotation"`
	Content   string            `json:"content"` // field name or static text
	Style     LabelElementStyle `json:"style"`
	ImageData string            `json:"image_data,omitempty"` // For static images
}

// LabelElementStyle defines styling for label elements
type LabelElementStyle struct {
	FontSize   int    `json:"font_size"`
	FontWeight string `json:"font_weight"`
	FontFamily string `json:"font_family"`
	Color      string `json:"color"`
	Alignment  string `json:"alignment"`
	Format     string `json:"format"` // For barcodes: "code128", "qr", "ean13"
}
