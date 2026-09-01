package models

import "time"

// ProductDependency represents a relationship between a product and its dependencies
type ProductDependency struct {
	ID                  int       `json:"id" db:"id"`
	ProductID           int       `json:"product_id" db:"product_id"`
	DependencyProductID int       `json:"dependency_product_id" db:"dependency_product_id"`
	IsOptional          bool      `json:"is_optional" db:"is_optional"`
	RelationType        string    `json:"relation_type" db:"relation_type"`
	AssignmentScope     string    `json:"assignment_scope" db:"assignment_scope"`
	DefaultQuantity     float64   `json:"default_quantity" db:"default_quantity"`
	Notes               *string   `json:"notes,omitempty" db:"notes"`
	CreatedAt           time.Time `json:"created_at" db:"created_at"`
	UpdatedAt           time.Time `json:"updated_at" db:"updated_at"`
}

// ProductDependencyWithDetails includes product information for the dependency
type ProductDependencyWithDetails struct {
	ID                  int      `json:"id"`
	ProductID           int      `json:"product_id"`
	DependencyProductID int      `json:"dependency_product_id"`
	DependencyName      string   `json:"dependency_name"`
	IsAccessory         bool     `json:"is_accessory"`
	IsConsumable        bool     `json:"is_consumable"`
	GenericBarcode      *string  `json:"generic_barcode,omitempty"`
	CountTypeAbbr       *string  `json:"count_type_abbr,omitempty"`
	StockQuantity       *float64 `json:"stock_quantity,omitempty"`
	IsOptional          bool     `json:"is_optional"`
	RelationType        string   `json:"relation_type"`
	AssignmentScope     string   `json:"assignment_scope"`
	DefaultQuantity     float64  `json:"default_quantity"`
	Notes               *string  `json:"notes,omitempty"`
	CreatedAt           string   `json:"created_at"`
}

// CreateProductDependencyRequest represents a request to create a dependency
type CreateProductDependencyRequest struct {
	DependencyProductID int     `json:"dependency_product_id" binding:"required"`
	IsOptional          bool    `json:"is_optional"`
	RelationType        string  `json:"relation_type"`
	AssignmentScope     string  `json:"assignment_scope"`
	DefaultQuantity     float64 `json:"default_quantity"`
	Notes               *string `json:"notes,omitempty"`
}
