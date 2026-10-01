BEGIN;
ALTER TABLE warehouse_tasks ADD COLUMN IF NOT EXISTS is_archived BOOLEAN NOT NULL DEFAULT false;
ALTER TABLE warehouse_tasks ADD COLUMN IF NOT EXISTS archived_at TIMESTAMP;
ALTER TABLE warehouse_tasks ADD COLUMN IF NOT EXISTS started_at TIMESTAMP;
CREATE TABLE IF NOT EXISTS warehouse_task_events(
 event_id BIGSERIAL PRIMARY KEY,task_id BIGINT NOT NULL REFERENCES warehouse_tasks(task_id) ON DELETE RESTRICT,
 event_type VARCHAR(30) NOT NULL,from_status VARCHAR(20),to_status VARCHAR(20),reason TEXT,
 actor_id BIGINT REFERENCES users(userid) ON DELETE SET NULL,created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE OR REPLACE FUNCTION guard_warehouse_task_version() RETURNS TRIGGER AS $$
BEGIN
 IF NEW.is_archived AND NEW.status NOT IN ('done','cancelled') THEN RAISE EXCEPTION 'Only terminal tasks can be archived';END IF;
 IF OLD.is_archived AND (to_jsonb(NEW)-'updated_at'-'is_archived'-'archived_at') IS DISTINCT FROM (to_jsonb(OLD)-'updated_at'-'is_archived'-'archived_at') THEN
  RAISE EXCEPTION 'Restore task before editing';
 END IF;
 NEW.updated_at:=GREATEST(clock_timestamp() AT TIME ZONE 'UTC',OLD.updated_at+INTERVAL '1 microsecond');
 NEW.archived_at:=CASE WHEN NEW.is_archived THEN COALESCE(OLD.archived_at,clock_timestamp() AT TIME ZONE 'UTC') ELSE NULL END;
 RETURN NEW;
END; $$ LANGUAGE plpgsql;
DROP TRIGGER IF EXISTS warehouse_tasks_guard_version ON warehouse_tasks;
CREATE TRIGGER warehouse_tasks_guard_version BEFORE UPDATE ON warehouse_tasks FOR EACH ROW EXECUTE FUNCTION guard_warehouse_task_version();
CREATE OR REPLACE FUNCTION touch_warehouse_task_event_version() RETURNS TRIGGER AS $$
BEGIN
 IF TG_OP<>'INSERT' THEN UPDATE warehouse_tasks SET updated_at=updated_at WHERE task_id=OLD.task_id;END IF;
 IF TG_OP<>'DELETE' THEN
  IF TG_OP='INSERT' OR NEW.task_id IS DISTINCT FROM OLD.task_id THEN UPDATE warehouse_tasks SET updated_at=updated_at WHERE task_id=NEW.task_id;END IF;
 END IF;
 RETURN NULL;
END; $$ LANGUAGE plpgsql;
DROP TRIGGER IF EXISTS warehouse_task_events_touch_version ON warehouse_task_events;
CREATE TRIGGER warehouse_task_events_touch_version AFTER INSERT OR UPDATE OR DELETE ON warehouse_task_events FOR EACH ROW EXECUTE FUNCTION touch_warehouse_task_event_version();
CREATE OR REPLACE FUNCTION touch_warehouse_task_dependencies() RETURNS TRIGGER AS $$
DECLARE ids JSONB;rowdata JSONB; BEGIN
 ids:=CASE WHEN TG_OP='INSERT' THEN jsonb_build_array(to_jsonb(NEW)) WHEN TG_OP='DELETE' THEN jsonb_build_array(to_jsonb(OLD)) ELSE jsonb_build_array(to_jsonb(OLD),to_jsonb(NEW)) END;
 FOR rowdata IN SELECT value FROM jsonb_array_elements(ids) LOOP
  UPDATE devices SET updated_at=updated_at WHERE deviceid=rowdata->>'device_id';
  UPDATE cases SET updated_at=updated_at WHERE caseid=(rowdata->>'case_id')::bigint;
  UPDATE products SET updated_at=updated_at WHERE productid=(rowdata->>'product_id')::bigint;
  UPDATE storage_zones SET updated_at=updated_at WHERE zone_id IN ((rowdata->>'from_zone_id')::bigint,(rowdata->>'to_zone_id')::bigint);
  UPDATE jobs SET updated_at=GREATEST(clock_timestamp() AT TIME ZONE 'UTC',updated_at+INTERVAL '1 microsecond') WHERE jobid=(rowdata->>'job_id')::bigint;
 END LOOP;
 RETURN NULL;
END; $$ LANGUAGE plpgsql;
DROP TRIGGER IF EXISTS warehouse_tasks_touch_dependencies ON warehouse_tasks;
CREATE TRIGGER warehouse_tasks_touch_dependencies AFTER INSERT OR UPDATE OR DELETE ON warehouse_tasks FOR EACH ROW EXECUTE FUNCTION touch_warehouse_task_dependencies();
INSERT INTO warehouse_schema_migrations(version) VALUES('056_warehouse_task_lifecycle') ON CONFLICT DO NOTHING;
COMMIT;
