package handlers

import (
	"fmt"

	"warehousecore/internal/repository"
)

const warehouseLocationBarcodeSchemaVersion = "041_automatic_warehouse_location_barcodes"

// EnsureWarehouseLocationBarcodeSchema assigns deterministic barcodes to
// existing locations and prevents duplicate location labels.
func EnsureWarehouseLocationBarcodeSchema() error {
	db := repository.GetSQLDB()
	if db == nil {
		return fmt.Errorf("database is not initialized")
	}

	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin warehouse location barcode schema transaction: %w", err)
	}
	defer tx.Rollback()

	statements := []string{
		`UPDATE storage_zones
		 SET barcode = 'LOC-' || UPPER(TRIM(code))
		 WHERE NULLIF(TRIM(barcode), '') IS NULL`,
		`CREATE UNIQUE INDEX IF NOT EXISTS uq_storage_zones_barcode_normalized
		 ON storage_zones (LOWER(TRIM(barcode)))
		 WHERE NULLIF(TRIM(barcode), '') IS NOT NULL`,
		`INSERT INTO warehouse_schema_migrations (version) VALUES ('041_automatic_warehouse_location_barcodes')
		 ON CONFLICT (version) DO NOTHING`,
	}
	for _, statement := range statements {
		if _, err := tx.Exec(statement); err != nil {
			return fmt.Errorf("apply warehouse location barcode schema: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit warehouse location barcode schema: %w", err)
	}
	return nil
}
