-- Version all three warehouse category levels, including UI and import writes.
ALTER TABLE categories ADD COLUMN IF NOT EXISTS updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP;
ALTER TABLE subcategories ADD COLUMN IF NOT EXISTS updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP;
ALTER TABLE subbiercategories ADD COLUMN IF NOT EXISTS updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP;
CREATE OR REPLACE FUNCTION touch_warehouse_category_version() RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at := GREATEST(clock_timestamp() AT TIME ZONE 'UTC', OLD.updated_at + INTERVAL '1 microsecond');
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
DROP TRIGGER IF EXISTS categories_touch_version ON categories;
CREATE TRIGGER categories_touch_version BEFORE UPDATE ON categories
FOR EACH ROW EXECUTE FUNCTION touch_warehouse_category_version();
DROP TRIGGER IF EXISTS subcategories_touch_version ON subcategories;
CREATE TRIGGER subcategories_touch_version BEFORE UPDATE ON subcategories
FOR EACH ROW EXECUTE FUNCTION touch_warehouse_category_version();
DROP TRIGGER IF EXISTS subbiercategories_touch_version ON subbiercategories;
CREATE TRIGGER subbiercategories_touch_version BEFORE UPDATE ON subbiercategories
FOR EACH ROW EXECUTE FUNCTION touch_warehouse_category_version();
INSERT INTO warehouse_schema_migrations(version) VALUES ('049_warehouse_category_version') ON CONFLICT DO NOTHING;
