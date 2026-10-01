package handlers

import (
	"fmt"
	"warehousecore/internal/repository"
)

const warehouseMaintenanceVersionSQL = `CREATE OR REPLACE FUNCTION touch_warehouse_maintenance_version() RETURNS TRIGGER AS $$
BEGIN
 NEW.updated_at := GREATEST(clock_timestamp() AT TIME ZONE 'UTC',OLD.updated_at+INTERVAL '1 microsecond');
 RETURN NEW;
END; $$ LANGUAGE plpgsql;
DROP TRIGGER IF EXISTS maintenance_plans_touch_version ON maintenance_plans;
CREATE TRIGGER maintenance_plans_touch_version BEFORE UPDATE ON maintenance_plans FOR EACH ROW EXECUTE FUNCTION touch_warehouse_maintenance_version();
DROP TRIGGER IF EXISTS maintenance_orders_touch_version ON maintenance_orders;
CREATE TRIGGER maintenance_orders_touch_version BEFORE UPDATE ON maintenance_orders FOR EACH ROW EXECUTE FUNCTION touch_warehouse_maintenance_version();
CREATE OR REPLACE FUNCTION touch_warehouse_maintenance_plan_orders() RETURNS TRIGGER AS $$
BEGIN
 IF TG_OP<>'INSERT' AND OLD.plan_id IS NOT NULL THEN
  UPDATE maintenance_plans SET updated_at=updated_at WHERE plan_id=OLD.plan_id;
 END IF;
 IF TG_OP<>'DELETE' AND NEW.plan_id IS NOT NULL THEN
  IF TG_OP='INSERT' OR NEW.plan_id IS DISTINCT FROM OLD.plan_id THEN
   UPDATE maintenance_plans SET updated_at=updated_at WHERE plan_id=NEW.plan_id;
  END IF;
 END IF;
 RETURN NULL;
END; $$ LANGUAGE plpgsql;
DROP TRIGGER IF EXISTS maintenance_orders_touch_plan_version ON maintenance_orders;
CREATE TRIGGER maintenance_orders_touch_plan_version AFTER INSERT OR UPDATE OR DELETE ON maintenance_orders FOR EACH ROW EXECUTE FUNCTION touch_warehouse_maintenance_plan_orders();
INSERT INTO warehouse_schema_migrations(version) VALUES ('054_warehouse_maintenance_version') ON CONFLICT DO NOTHING;
`

func EnsureWarehouseMaintenanceVersionSchema() error {
	db := repository.GetSQLDB()
	if db == nil {
		return fmt.Errorf("database is not initialized")
	}
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.Exec(warehouseMaintenanceVersionSQL); err != nil {
		return fmt.Errorf("apply maintenance versions: %w", err)
	}
	return tx.Commit()
}
