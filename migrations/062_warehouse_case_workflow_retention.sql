-- Preserve case identities and physical movement history across lifecycle.
-- Fresh suite initialization runs before the owning-Core operations bootstrap.
CREATE TABLE IF NOT EXISTS case_events (
 event_id BIGSERIAL PRIMARY KEY,case_id INT NOT NULL REFERENCES cases(caseid) ON DELETE CASCADE,
 event_type VARCHAR(40) NOT NULL,device_id VARCHAR(255),product_id INT,
 quantity NUMERIC(12,3),zone_id INT,job_id BIGINT,
 metadata JSONB NOT NULL DEFAULT '{}'::jsonb,created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_case_events_case_created ON case_events(case_id,created_at DESC);
CREATE OR REPLACE FUNCTION retain_warehouse_case_identity() RETURNS TRIGGER AS $$
BEGIN RAISE EXCEPTION 'Archive cases; physical history and case identity cannot be deleted';END;
$$ LANGUAGE plpgsql;
DROP TRIGGER IF EXISTS cases_guard_retention ON cases;
CREATE TRIGGER cases_guard_retention BEFORE DELETE ON cases FOR EACH ROW EXECUTE FUNCTION retain_warehouse_case_identity();
CREATE OR REPLACE FUNCTION retain_warehouse_case_event() RETURNS TRIGGER AS $$
BEGIN RAISE EXCEPTION 'Physical case events are retained and immutable';END;
$$ LANGUAGE plpgsql;
DROP TRIGGER IF EXISTS case_events_guard_retention ON case_events;
CREATE TRIGGER case_events_guard_retention BEFORE UPDATE OR DELETE ON case_events FOR EACH ROW EXECUTE FUNCTION retain_warehouse_case_event();
CREATE OR REPLACE FUNCTION touch_warehouse_case_event_version() RETURNS TRIGGER AS $$
BEGIN UPDATE cases SET updated_at=updated_at WHERE caseid=NEW.case_id;RETURN NEW;END;
$$ LANGUAGE plpgsql;
DROP TRIGGER IF EXISTS case_events_touch_version ON case_events;
CREATE TRIGGER case_events_touch_version AFTER INSERT ON case_events FOR EACH ROW EXECUTE FUNCTION touch_warehouse_case_event_version();
INSERT INTO warehouse_schema_migrations(version) VALUES('062_warehouse_case_workflow_retention') ON CONFLICT DO NOTHING;
