package handlers

import (
	"fmt"
	"warehousecore/internal/repository"
)

const warehouseInventoryLifecycleSQL = `ALTER TABLE inventory_counts ADD COLUMN IF NOT EXISTS updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP;
ALTER TABLE inventory_counts ADD COLUMN IF NOT EXISTS is_archived BOOLEAN NOT NULL DEFAULT false;
ALTER TABLE inventory_counts ADD COLUMN IF NOT EXISTS archived_at TIMESTAMP;
ALTER TABLE inventory_counts ADD COLUMN IF NOT EXISTS notes TEXT;
ALTER TABLE inventory_counts ADD COLUMN IF NOT EXISTS stock_baseline JSONB;
ALTER TABLE inventory_count_lines ADD COLUMN IF NOT EXISTS updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP;
CREATE TABLE IF NOT EXISTS inventory_count_events(
 event_id BIGSERIAL PRIMARY KEY,count_id BIGINT NOT NULL REFERENCES inventory_counts(count_id) ON DELETE RESTRICT,
 event_type VARCHAR(40) NOT NULL,from_status VARCHAR(20),to_status VARCHAR(20),reason TEXT,
 actor_id BIGINT REFERENCES users(userid) ON DELETE SET NULL,created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE TABLE IF NOT EXISTS inventory_adjustments(
 adjustment_id BIGSERIAL PRIMARY KEY,count_id BIGINT NOT NULL REFERENCES inventory_counts(count_id) ON DELETE RESTRICT,
 item_type VARCHAR(20) NOT NULL,item_key VARCHAR(255) NOT NULL,zone_id INT NOT NULL REFERENCES storage_zones(zone_id) ON DELETE RESTRICT,
 quantity_before NUMERIC(12,3) NOT NULL,quantity_after NUMERIC(12,3) NOT NULL,
 actor_id BIGINT REFERENCES users(userid) ON DELETE SET NULL,created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
 UNIQUE(count_id,item_type,item_key)
);
CREATE OR REPLACE FUNCTION guard_inventory_count_version() RETURNS TRIGGER AS $$ BEGIN
 IF NEW.is_archived AND NEW.status NOT IN ('approved','cancelled') THEN RAISE EXCEPTION 'Only terminal counts can be archived';END IF;
 IF OLD.is_archived AND (to_jsonb(NEW)-'updated_at'-'is_archived'-'archived_at') IS DISTINCT FROM (to_jsonb(OLD)-'updated_at'-'is_archived'-'archived_at') THEN RAISE EXCEPTION 'Restore count before editing';END IF;
 IF NEW.zone_id IS DISTINCT FROM OLD.zone_id OR NEW.stock_baseline IS DISTINCT FROM OLD.stock_baseline THEN RAISE EXCEPTION 'Count zone and stock baseline are immutable';END IF;
 IF OLD.status IN ('approved','cancelled') AND NEW.status IS DISTINCT FROM OLD.status THEN RAISE EXCEPTION 'Terminal counts cannot reopen; create a new count';END IF;
 IF NEW.status='approved' AND OLD.status<>'approved' AND OLD.stock_baseline IS NOT NULL AND COALESCE(current_setting('warehouse.inventory_approval',true),'')<>'guided' THEN RAISE EXCEPTION 'Guided count requires its checked approval workflow';END IF;
 NEW.updated_at:=GREATEST(clock_timestamp() AT TIME ZONE 'UTC',OLD.updated_at+INTERVAL '1 microsecond');
 NEW.archived_at:=CASE WHEN NEW.is_archived THEN COALESCE(OLD.archived_at,clock_timestamp() AT TIME ZONE 'UTC') ELSE NULL END;
 RETURN NEW;
END; $$ LANGUAGE plpgsql;
DROP TRIGGER IF EXISTS inventory_counts_guard_version ON inventory_counts;
CREATE TRIGGER inventory_counts_guard_version BEFORE UPDATE ON inventory_counts FOR EACH ROW EXECUTE FUNCTION guard_inventory_count_version();
CREATE OR REPLACE FUNCTION guard_inventory_count_line() RETURNS TRIGGER AS $$ DECLARE cid BIGINT;state TEXT;archived BOOLEAN; BEGIN
 cid:=CASE WHEN TG_OP='DELETE' THEN OLD.count_id ELSE NEW.count_id END;
 SELECT status,is_archived INTO state,archived FROM inventory_counts WHERE count_id=cid FOR UPDATE;
 IF archived OR state NOT IN ('open','counting','review') THEN RAISE EXCEPTION 'Only active count lines can change';END IF;
 IF TG_OP='UPDATE' AND (NEW.count_id,NEW.item_type,NEW.item_key,NEW.expected_quantity) IS DISTINCT FROM (OLD.count_id,OLD.item_type,OLD.item_key,OLD.expected_quantity) THEN RAISE EXCEPTION 'Line identity and baseline are immutable';END IF;
 IF TG_OP<>'DELETE' THEN
  IF NEW.expected_quantity<0 OR NEW.counted_quantity<0 OR NEW.item_type IN ('device','case') AND (NEW.expected_quantity NOT IN (0,1) OR NEW.counted_quantity NOT IN (0,1)) THEN RAISE EXCEPTION 'Invalid count quantity';END IF;
  NEW.updated_at:=CASE WHEN TG_OP='INSERT' THEN clock_timestamp() AT TIME ZONE 'UTC' ELSE GREATEST(clock_timestamp() AT TIME ZONE 'UTC',OLD.updated_at+INTERVAL '1 microsecond') END;
 END IF;
 IF TG_OP='DELETE' THEN RETURN OLD;END IF;RETURN NEW;
END; $$ LANGUAGE plpgsql;
DROP TRIGGER IF EXISTS inventory_count_lines_guard ON inventory_count_lines;
CREATE TRIGGER inventory_count_lines_guard BEFORE INSERT OR UPDATE OR DELETE ON inventory_count_lines FOR EACH ROW EXECUTE FUNCTION guard_inventory_count_line();
CREATE OR REPLACE FUNCTION touch_inventory_count_child_version() RETURNS TRIGGER AS $$ BEGIN
 IF TG_OP<>'INSERT' THEN UPDATE inventory_counts SET updated_at=updated_at WHERE count_id=OLD.count_id;END IF;
 IF TG_OP<>'DELETE' THEN
  IF TG_OP='INSERT' OR NEW.count_id IS DISTINCT FROM OLD.count_id THEN UPDATE inventory_counts SET updated_at=updated_at WHERE count_id=NEW.count_id;END IF;
 END IF;
 RETURN NULL;
END; $$ LANGUAGE plpgsql;
DROP TRIGGER IF EXISTS inventory_count_lines_touch_version ON inventory_count_lines;
CREATE TRIGGER inventory_count_lines_touch_version AFTER INSERT OR UPDATE OR DELETE ON inventory_count_lines FOR EACH ROW EXECUTE FUNCTION touch_inventory_count_child_version();
DROP TRIGGER IF EXISTS inventory_count_events_touch_version ON inventory_count_events;
CREATE TRIGGER inventory_count_events_touch_version AFTER INSERT OR UPDATE OR DELETE ON inventory_count_events FOR EACH ROW EXECUTE FUNCTION touch_inventory_count_child_version();
INSERT INTO warehouse_schema_migrations(version) VALUES('057_warehouse_inventory_lifecycle') ON CONFLICT DO NOTHING;
`

func EnsureWarehouseInventoryLifecycleSchema() error {
	db := repository.GetSQLDB()
	if db == nil {
		return fmt.Errorf("database is not initialized")
	}
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.Exec(warehouseInventoryLifecycleSQL); err != nil {
		return fmt.Errorf("apply warehouse inventory lifecycle: %w", err)
	}
	return tx.Commit()
}
