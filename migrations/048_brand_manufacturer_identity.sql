-- Brand identity is name plus manufacturer; unassigned brands also stay unique.
CREATE UNIQUE INDEX IF NOT EXISTS uq_brands_manufacturer_name_normalized
ON brands(manufacturerid,LOWER(TRIM(name))) NULLS NOT DISTINCT;
DROP INDEX IF EXISTS uq_brands_name_normalized;
INSERT INTO warehouse_schema_migrations(version)
VALUES ('048_brand_manufacturer_identity') ON CONFLICT DO NOTHING;
