-- Automatic, deterministic barcodes for all warehouse locations.
BEGIN;

UPDATE storage_zones
SET barcode = 'LOC-' || UPPER(TRIM(code))
WHERE NULLIF(TRIM(barcode), '') IS NULL;

CREATE UNIQUE INDEX IF NOT EXISTS uq_storage_zones_barcode_normalized
ON storage_zones (LOWER(TRIM(barcode)))
WHERE NULLIF(TRIM(barcode), '') IS NOT NULL;

INSERT INTO warehouse_schema_migrations (version)
VALUES ('041_automatic_warehouse_location_barcodes')
ON CONFLICT (version) DO NOTHING;

COMMIT;
