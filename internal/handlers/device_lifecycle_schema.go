package handlers

import (
	"fmt"

	"warehousecore/internal/repository"
)

// EnsureDeviceLifecycleSchema installs the archive state used to keep retired
// inventory out of operational workflows without losing its history.
func EnsureDeviceLifecycleSchema() error {
	db := repository.GetSQLDB()
	if db == nil {
		return fmt.Errorf("database is not initialized")
	}

	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin device lifecycle schema transaction: %w", err)
	}
	defer tx.Rollback()

	statements := []string{
		`ALTER TABLE devices ADD COLUMN IF NOT EXISTS lifecycle_status VARCHAR(20)`,
		`ALTER TABLE devices ADD COLUMN IF NOT EXISTS archived_at TIMESTAMP`,
		`ALTER TABLE devices ADD COLUMN IF NOT EXISTS archived_by_product BOOLEAN`,
		`UPDATE devices SET lifecycle_status='active' WHERE lifecycle_status IS NULL OR lifecycle_status=''`,
		`UPDATE devices SET archived_by_product=FALSE WHERE archived_by_product IS NULL`,
		`ALTER TABLE devices ALTER COLUMN lifecycle_status SET DEFAULT 'active'`,
		`ALTER TABLE devices ALTER COLUMN lifecycle_status SET NOT NULL`,
		`ALTER TABLE devices ALTER COLUMN archived_by_product SET DEFAULT FALSE`,
		`ALTER TABLE devices ALTER COLUMN archived_by_product SET NOT NULL`,
		`DO $$ BEGIN IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname='chk_devices_lifecycle_status') THEN ALTER TABLE devices ADD CONSTRAINT chk_devices_lifecycle_status CHECK(lifecycle_status IN ('active','archived')); END IF; END $$`,
		`CREATE INDEX IF NOT EXISTS idx_devices_lifecycle_status ON devices(lifecycle_status)`,
		`CREATE INDEX IF NOT EXISTS idx_devices_product_lifecycle ON devices(productID,lifecycle_status)`,
		`INSERT INTO warehouse_schema_migrations(version) VALUES('044_device_lifecycle') ON CONFLICT(version) DO NOTHING`,
	}
	for _, statement := range statements {
		if _, err := tx.Exec(statement); err != nil {
			return fmt.Errorf("apply device lifecycle schema: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit device lifecycle schema: %w", err)
	}
	return nil
}
