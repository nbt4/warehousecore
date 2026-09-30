-- Version all device writers and keep scan identifiers aligned with lifecycle.
-- Bootstrap scan registry for consolidated databases before service startup.
CREATE TABLE IF NOT EXISTS inventory_identifiers(identifier_id BIGSERIAL PRIMARY KEY, entity_type VARCHAR(20) NOT NULL CHECK(entity_type IN ('product','device','case')), entity_key VARCHAR(255) NOT NULL, code VARCHAR(255) NOT NULL, identifier_kind VARCHAR(24) NOT NULL DEFAULT 'canonical', active BOOLEAN NOT NULL DEFAULT TRUE, created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP, UNIQUE(entity_type,entity_key,identifier_kind));
CREATE UNIQUE INDEX IF NOT EXISTS uq_inventory_identifiers_code_normalized ON inventory_identifiers(LOWER(TRIM(code))) WHERE active;

ALTER TABLE devices ADD COLUMN IF NOT EXISTS updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP;
UPDATE devices SET updated_at=CURRENT_TIMESTAMP WHERE updated_at IS NULL;
ALTER TABLE devices ALTER COLUMN updated_at SET NOT NULL;
CREATE OR REPLACE FUNCTION touch_warehouse_device_version() RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at := GREATEST(clock_timestamp() AT TIME ZONE 'UTC', OLD.updated_at + INTERVAL '1 microsecond');
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
DROP TRIGGER IF EXISTS devices_touch_version ON devices;
CREATE TRIGGER devices_touch_version BEFORE UPDATE ON devices
FOR EACH ROW EXECUTE FUNCTION touch_warehouse_device_version();
CREATE OR REPLACE FUNCTION sync_device_identifier() RETURNS TRIGGER AS $$
BEGIN
    DELETE FROM inventory_identifiers WHERE entity_type='device' AND entity_key=NEW.deviceid AND identifier_kind='legacy_id';
    INSERT INTO inventory_identifiers(entity_type,entity_key,code,identifier_kind,active)
    VALUES('device',NEW.deviceid,NEW.barcode,'canonical',NEW.lifecycle_status='active')
    ON CONFLICT(entity_type,entity_key,identifier_kind) DO UPDATE SET code=EXCLUDED.code,active=EXCLUDED.active;
    IF LOWER(TRIM(NEW.barcode))<>LOWER(TRIM(NEW.deviceid)) THEN
        INSERT INTO inventory_identifiers(entity_type,entity_key,code,identifier_kind,active)
        VALUES('device',NEW.deviceid,NEW.deviceid,'legacy_id',NEW.lifecycle_status='active')
        ON CONFLICT(entity_type,entity_key,identifier_kind) DO UPDATE SET code=EXCLUDED.code,active=EXCLUDED.active;
    END IF;
    UPDATE inventory_identifiers SET active=(NEW.lifecycle_status='active')
    WHERE entity_type='device' AND entity_key=NEW.deviceid
    AND active IS DISTINCT FROM (NEW.lifecycle_status='active');
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
DROP TRIGGER IF EXISTS devices_identifier_after_write ON devices;
CREATE TRIGGER devices_identifier_after_write AFTER INSERT OR UPDATE OF barcode,lifecycle_status ON devices
FOR EACH ROW EXECUTE FUNCTION sync_device_identifier();
UPDATE inventory_identifiers i SET active=(d.lifecycle_status='active')
FROM devices d WHERE i.entity_type='device' AND i.entity_key=d.deviceid
AND i.active IS DISTINCT FROM (d.lifecycle_status='active');
CREATE INDEX IF NOT EXISTS idx_audit_device_entity ON audit_log(entity_type,entity_id,id DESC);
INSERT INTO warehouse_schema_migrations(version) VALUES ('051_warehouse_device_version') ON CONFLICT DO NOTHING;
