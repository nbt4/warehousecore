-- Version package metadata and contents for every writer, including UI/imports.
ALTER TABLE product_packages ADD COLUMN IF NOT EXISTS package_code VARCHAR(32);
ALTER TABLE product_packages ADD COLUMN IF NOT EXISTS updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP;
UPDATE product_packages SET updated_at=CURRENT_TIMESTAMP WHERE updated_at IS NULL;
ALTER TABLE product_packages ALTER COLUMN updated_at SET NOT NULL;
ALTER TABLE product_package_items ADD COLUMN IF NOT EXISTS is_optional BOOLEAN DEFAULT FALSE;
CREATE OR REPLACE FUNCTION touch_warehouse_package_version() RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at := GREATEST(clock_timestamp() AT TIME ZONE 'UTC', OLD.updated_at + INTERVAL '1 microsecond');
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
DROP TRIGGER IF EXISTS product_packages_touch_version ON product_packages;
CREATE TRIGGER product_packages_touch_version BEFORE UPDATE ON product_packages
FOR EACH ROW EXECUTE FUNCTION touch_warehouse_package_version();
CREATE OR REPLACE FUNCTION touch_warehouse_package_contents_version() RETURNS TRIGGER AS $$
BEGIN
    IF TG_OP <> 'INSERT' THEN
        UPDATE product_packages SET updated_at=CURRENT_TIMESTAMP WHERE id=OLD.package_id;
    END IF;
    IF TG_OP <> 'DELETE' THEN
        IF TG_OP='INSERT' THEN
            UPDATE product_packages SET updated_at=CURRENT_TIMESTAMP WHERE id=NEW.package_id;
        ELSIF NEW.package_id IS DISTINCT FROM OLD.package_id THEN
            UPDATE product_packages SET updated_at=CURRENT_TIMESTAMP WHERE id=NEW.package_id;
        END IF;
    END IF;
    RETURN NULL;
END;
$$ LANGUAGE plpgsql;
DROP TRIGGER IF EXISTS product_package_items_touch_version ON product_package_items;
CREATE TRIGGER product_package_items_touch_version AFTER INSERT OR UPDATE OR DELETE ON product_package_items
FOR EACH ROW EXECUTE FUNCTION touch_warehouse_package_contents_version();
INSERT INTO warehouse_schema_migrations(version) VALUES ('050_warehouse_package_version') ON CONFLICT DO NOTHING;
