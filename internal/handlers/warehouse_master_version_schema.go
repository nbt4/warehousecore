package handlers

import (
	"fmt"
	"warehousecore/internal/repository"
)

const warehouseMasterVersionSQL = `ALTER TABLE manufacturer ADD COLUMN IF NOT EXISTS updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP;
ALTER TABLE brands ADD COLUMN IF NOT EXISTS updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP;
CREATE OR REPLACE FUNCTION touch_warehouse_master_version() RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at := GREATEST(clock_timestamp() AT TIME ZONE 'UTC', OLD.updated_at + INTERVAL '1 microsecond');
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
DROP TRIGGER IF EXISTS manufacturer_touch_version ON manufacturer;
CREATE TRIGGER manufacturer_touch_version BEFORE UPDATE ON manufacturer
FOR EACH ROW EXECUTE FUNCTION touch_warehouse_master_version();
DROP TRIGGER IF EXISTS brands_touch_version ON brands;
CREATE TRIGGER brands_touch_version BEFORE UPDATE ON brands
FOR EACH ROW EXECUTE FUNCTION touch_warehouse_master_version();
INSERT INTO warehouse_schema_migrations(version) VALUES ('047_warehouse_master_version') ON CONFLICT DO NOTHING;
`

func EnsureWarehouseMasterVersionSchema() error {
	db := repository.GetSQLDB()
	if db == nil {
		return fmt.Errorf("database is not initialized")
	}
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.Exec(warehouseMasterVersionSQL + warehouseMasterLifecycleSQL); err != nil {
		return fmt.Errorf("apply warehouse master version schema: %w", err)
	}
	return tx.Commit()
}
