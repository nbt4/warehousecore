-- Product master data v2 is installed idempotently by
-- internal/handlers/product_master_schema.go during application startup.
-- Keeping this marker migration documents the release order for clean stacks.
INSERT INTO warehouse_schema_migrations(version)
VALUES ('043_product_master_v2')
ON CONFLICT(version) DO NOTHING;
