package handlers

import (
	"fmt"
	"warehousecore/internal/repository"
)

const warehouseLocationVersionSQL = `ALTER TABLE storage_zones ADD COLUMN IF NOT EXISTS updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP;
CREATE OR REPLACE FUNCTION touch_warehouse_location_version() RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at := GREATEST(clock_timestamp() AT TIME ZONE 'UTC', OLD.updated_at + INTERVAL '1 microsecond');
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
DROP TRIGGER IF EXISTS storage_zones_touch_version ON storage_zones;
CREATE TRIGGER storage_zones_touch_version BEFORE UPDATE ON storage_zones
FOR EACH ROW EXECUTE FUNCTION touch_warehouse_location_version();
INSERT INTO warehouse_schema_migrations(version) VALUES ('046_warehouse_location_version') ON CONFLICT DO NOTHING;
`

func EnsureWarehouseLocationVersionSchema() error {
	db := repository.GetSQLDB()
	if db == nil {
		return fmt.Errorf("database is not initialized")
	}
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.Exec(warehouseLocationVersionSQL); err != nil {
		return fmt.Errorf("apply warehouse location version schema: %w", err)
	}
	return tx.Commit()
}
