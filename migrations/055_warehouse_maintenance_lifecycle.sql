BEGIN;
ALTER TABLE maintenance_orders ADD COLUMN IF NOT EXISTS is_archived BOOLEAN NOT NULL DEFAULT false;
ALTER TABLE maintenance_orders ADD COLUMN IF NOT EXISTS archived_at TIMESTAMP;
CREATE OR REPLACE FUNCTION guard_warehouse_maintenance_archive() RETURNS TRIGGER AS $$
BEGIN
 IF NEW.is_archived AND NEW.status NOT IN ('completed','cancelled') THEN
  RAISE EXCEPTION 'Only terminal maintenance orders can be archived';
 END IF;
 IF OLD.is_archived AND (to_jsonb(NEW)-'updated_at'-'is_archived'-'archived_at') IS DISTINCT FROM (to_jsonb(OLD)-'updated_at'-'is_archived'-'archived_at') THEN
  RAISE EXCEPTION 'Restore maintenance order before editing';
 END IF;
 NEW.archived_at := CASE WHEN NEW.is_archived THEN COALESCE(OLD.archived_at,clock_timestamp() AT TIME ZONE 'UTC') ELSE NULL END;
 RETURN NEW;
END; $$ LANGUAGE plpgsql;
DROP TRIGGER IF EXISTS maintenance_orders_guard_archive ON maintenance_orders;
CREATE TRIGGER maintenance_orders_guard_archive BEFORE UPDATE ON maintenance_orders FOR EACH ROW EXECUTE FUNCTION guard_warehouse_maintenance_archive();
CREATE OR REPLACE FUNCTION touch_warehouse_maintenance_order_events() RETURNS TRIGGER AS $$
BEGIN
 IF TG_OP<>'INSERT' THEN UPDATE maintenance_orders SET updated_at=updated_at WHERE order_id=OLD.order_id; END IF;
 IF TG_OP<>'DELETE' THEN
  IF TG_OP='INSERT' OR NEW.order_id IS DISTINCT FROM OLD.order_id THEN
   UPDATE maintenance_orders SET updated_at=updated_at WHERE order_id=NEW.order_id;
  END IF;
 END IF;
 RETURN NULL;
END; $$ LANGUAGE plpgsql;
DROP TRIGGER IF EXISTS maintenance_events_touch_order_version ON maintenance_order_events;
CREATE TRIGGER maintenance_events_touch_order_version AFTER INSERT OR UPDATE OR DELETE ON maintenance_order_events FOR EACH ROW EXECUTE FUNCTION touch_warehouse_maintenance_order_events();
CREATE OR REPLACE FUNCTION touch_warehouse_legacy_defect_dependencies() RETURNS TRIGGER AS $$
BEGIN
 IF TG_OP<>'INSERT' THEN
  UPDATE maintenance_orders SET updated_at=updated_at WHERE legacy_defect_id=OLD.defect_id;
  UPDATE devices SET updated_at=updated_at WHERE deviceid=OLD.device_id;
 END IF;
 IF TG_OP<>'DELETE' THEN
  IF TG_OP='INSERT' OR NEW.device_id IS DISTINCT FROM OLD.device_id THEN
   UPDATE devices SET updated_at=updated_at WHERE deviceid=NEW.device_id;
  END IF;
 END IF;
 RETURN NULL;
END; $$ LANGUAGE plpgsql;
DROP TRIGGER IF EXISTS defect_reports_touch_maintenance_version ON defect_reports;
CREATE TRIGGER defect_reports_touch_maintenance_version AFTER INSERT OR UPDATE OR DELETE ON defect_reports FOR EACH ROW EXECUTE FUNCTION touch_warehouse_legacy_defect_dependencies();
CREATE OR REPLACE FUNCTION touch_warehouse_maintenance_device_dependencies() RETURNS TRIGGER AS $$
BEGIN
 IF TG_OP<>'INSERT' THEN UPDATE devices SET updated_at=updated_at WHERE deviceid=OLD.device_id; END IF;
 IF TG_OP<>'DELETE' THEN
  IF TG_OP='INSERT' OR NEW.device_id IS DISTINCT FROM OLD.device_id THEN
   UPDATE devices SET updated_at=updated_at WHERE deviceid=NEW.device_id;
  END IF;
 END IF;
 RETURN NULL;
END; $$ LANGUAGE plpgsql;
DROP TRIGGER IF EXISTS maintenance_orders_touch_device_version ON maintenance_orders;
CREATE TRIGGER maintenance_orders_touch_device_version AFTER INSERT OR UPDATE OR DELETE ON maintenance_orders FOR EACH ROW EXECUTE FUNCTION touch_warehouse_maintenance_device_dependencies();
DROP TRIGGER IF EXISTS maintenance_plans_touch_device_version ON maintenance_plans;
CREATE TRIGGER maintenance_plans_touch_device_version AFTER INSERT OR UPDATE OR DELETE ON maintenance_plans FOR EACH ROW EXECUTE FUNCTION touch_warehouse_maintenance_device_dependencies();
INSERT INTO warehouse_schema_migrations(version) VALUES ('055_warehouse_maintenance_lifecycle') ON CONFLICT DO NOTHING;
COMMIT;
