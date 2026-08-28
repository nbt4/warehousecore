BEGIN;

ALTER TABLE devices ADD COLUMN IF NOT EXISTS condition_status VARCHAR(30) DEFAULT 'available';
ALTER TABLE devices ADD COLUMN IF NOT EXISTS status_updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP;

UPDATE devices SET condition_status = CASE LOWER(TRIM(COALESCE(status,'')))
    WHEN 'blocked' THEN 'blocked' WHEN 'defective' THEN 'defective' WHEN 'defect' THEN 'defective'
    WHEN 'repair' THEN 'maintenance' WHEN 'maintenance' THEN 'maintenance' WHEN 'maintance' THEN 'maintenance'
    WHEN 'retired' THEN 'retired' ELSE COALESCE(NULLIF(condition_status,''),'available') END
WHERE NOT EXISTS (SELECT 1 FROM warehouse_schema_migrations WHERE version='040_device_status_and_condition');

CREATE INDEX IF NOT EXISTS idx_job_devices_issued_device
    ON job_devices(deviceid, jobid) WHERE pack_status = 'issued';

CREATE OR REPLACE FUNCTION warehouse_job_status_is_closed(job_status TEXT)
RETURNS BOOLEAN AS $$
    SELECT LOWER(TRIM(COALESCE(job_status, ''))) IN (
        'abgeschlossen','abgerechnet','storniert','completed','paid','canceled','cancelled'
    );
$$ LANGUAGE SQL IMMUTABLE;

UPDATE devices d SET status = CASE
    WHEN EXISTS (
        SELECT 1 FROM job_devices jd JOIN jobs j ON j.jobid=jd.jobid JOIN status s ON s.statusid=j.statusid
        WHERE jd.deviceid=d.deviceid AND jd.pack_status='issued' AND NOT warehouse_job_status_is_closed(s.status)
    ) THEN 'on_job'
    WHEN d.status='return_pending' OR EXISTS (
        SELECT 1 FROM job_devices jd JOIN jobs j ON j.jobid=jd.jobid JOIN status s ON s.statusid=j.statusid
        WHERE jd.deviceid=d.deviceid AND jd.pack_status='issued' AND warehouse_job_status_is_closed(s.status)
    ) THEN 'return_pending'
    WHEN d.zone_id IS NOT NULL OR EXISTS (SELECT 1 FROM devicescases dc WHERE dc.deviceid=d.deviceid) THEN 'in_storage'
    ELSE 'location_unknown' END,
    status_updated_at=CURRENT_TIMESTAMP,updated_at=CURRENT_TIMESTAMP
WHERE NOT EXISTS (SELECT 1 FROM warehouse_schema_migrations WHERE version='040_device_status_and_condition');

UPDATE devices SET condition_status='available'
WHERE condition_status IS NULL OR condition_status NOT IN ('available','blocked','defective','maintenance','retired');

ALTER TABLE devices ALTER COLUMN status SET DEFAULT 'location_unknown';
ALTER TABLE devices ALTER COLUMN status SET NOT NULL;
ALTER TABLE devices ALTER COLUMN condition_status SET DEFAULT 'available';
ALTER TABLE devices ALTER COLUMN condition_status SET NOT NULL;
ALTER TABLE devices ALTER COLUMN status_updated_at SET DEFAULT CURRENT_TIMESTAMP;
ALTER TABLE devices ALTER COLUMN status_updated_at SET NOT NULL;

DO $$ BEGIN IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname='chk_devices_physical_status') THEN
    ALTER TABLE devices ADD CONSTRAINT chk_devices_physical_status
        CHECK (status IN ('in_storage','on_job','return_pending','location_unknown'));
END IF; END $$;
DO $$ BEGIN IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname='chk_devices_condition_status') THEN
    ALTER TABLE devices ADD CONSTRAINT chk_devices_condition_status
        CHECK (condition_status IN ('available','blocked','defective','maintenance','retired'));
END IF; END $$;

CREATE INDEX IF NOT EXISTS idx_devices_physical_status ON devices(status);
CREATE INDEX IF NOT EXISTS idx_devices_condition_status ON devices(condition_status);

CREATE TABLE IF NOT EXISTS device_status_history (
    history_id BIGSERIAL PRIMARY KEY,
    device_id VARCHAR(255) NOT NULL REFERENCES devices(deviceID) ON DELETE CASCADE,
    previous_status VARCHAR(30), new_status VARCHAR(30) NOT NULL,
    previous_condition VARCHAR(30), new_condition VARCHAR(30) NOT NULL,
    previous_zone_id INT, new_zone_id INT,
    previous_location VARCHAR(255), new_location VARCHAR(255),
    change_source VARCHAR(50) NOT NULL DEFAULT 'database',
    changed_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_device_status_history_device_time
    ON device_status_history(device_id,changed_at DESC);

CREATE OR REPLACE FUNCTION audit_device_status_change() RETURNS TRIGGER AS $$
BEGIN
    IF OLD.status IS DISTINCT FROM NEW.status THEN NEW.status_updated_at := CURRENT_TIMESTAMP; END IF;
    IF OLD.status IS DISTINCT FROM NEW.status OR OLD.condition_status IS DISTINCT FROM NEW.condition_status
       OR OLD.zone_id IS DISTINCT FROM NEW.zone_id OR OLD.current_location IS DISTINCT FROM NEW.current_location THEN
        INSERT INTO device_status_history(device_id,previous_status,new_status,previous_condition,new_condition,
            previous_zone_id,new_zone_id,previous_location,new_location,change_source)
        VALUES(NEW.deviceID,OLD.status,NEW.status,OLD.condition_status,NEW.condition_status,
            OLD.zone_id,NEW.zone_id,OLD.current_location,NEW.current_location,
            CASE WHEN OLD.condition_status IS DISTINCT FROM NEW.condition_status THEN 'condition_workflow'
                 WHEN NEW.status='on_job' THEN 'dispatch_workflow'
                 WHEN NEW.status='return_pending' THEN 'return_workflow'
                 WHEN OLD.zone_id IS DISTINCT FROM NEW.zone_id OR NEW.status IN ('in_storage','location_unknown') THEN 'location_workflow'
                 ELSE 'database' END);
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS devices_audit_status_change ON devices;
CREATE TRIGGER devices_audit_status_change
BEFORE UPDATE OF status,condition_status,zone_id,current_location ON devices
FOR EACH ROW EXECUTE FUNCTION audit_device_status_change();

CREATE OR REPLACE FUNCTION sync_devices_after_job_status_change() RETURNS TRIGGER AS $$
DECLARE new_status_name TEXT;
BEGIN
    SELECT status INTO new_status_name FROM status WHERE statusid=NEW.statusid;
    IF warehouse_job_status_is_closed(new_status_name) THEN
        UPDATE devices d SET status='return_pending',zone_id=NULL,current_location='return_pending',updated_at=CURRENT_TIMESTAMP
        FROM job_devices jd WHERE jd.jobid=NEW.jobid AND jd.deviceid=d.deviceid AND jd.pack_status='issued'
          AND d.status='on_job' AND NOT EXISTS (
            SELECT 1 FROM job_devices other_jd JOIN jobs other_job ON other_job.jobid=other_jd.jobid
            JOIN status other_status ON other_status.statusid=other_job.statusid
            WHERE other_jd.deviceid=d.deviceid AND other_jd.jobid<>NEW.jobid AND other_jd.pack_status='issued'
              AND NOT warehouse_job_status_is_closed(other_status.status));
    ELSE
        UPDATE devices d SET status='on_job',zone_id=NULL,current_location='job:'||COALESCE(NEW.job_code,NEW.jobid::text),updated_at=CURRENT_TIMESTAMP
        FROM job_devices jd WHERE jd.jobid=NEW.jobid AND jd.deviceid=d.deviceid
          AND jd.pack_status='issued' AND d.status='return_pending';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS jobs_sync_device_status ON jobs;
CREATE TRIGGER jobs_sync_device_status AFTER UPDATE OF statusid ON jobs
FOR EACH ROW WHEN (OLD.statusid IS DISTINCT FROM NEW.statusid)
EXECUTE FUNCTION sync_devices_after_job_status_change();

INSERT INTO warehouse_schema_migrations(version) VALUES ('040_device_status_and_condition')
ON CONFLICT(version) DO NOTHING;

COMMIT;
