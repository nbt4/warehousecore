-- Product-backed cable inventory with quantity and individual tracking.
BEGIN;

CREATE TABLE IF NOT EXISTS warehouse_schema_migrations (
    version VARCHAR(100) PRIMARY KEY,
    applied_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS product_locations (
    location_id SERIAL PRIMARY KEY,
    product_id INT NOT NULL REFERENCES products(productid) ON DELETE CASCADE,
    zone_id INT NULL REFERENCES storage_zones(zone_id) ON DELETE CASCADE,
    quantity NUMERIC(10,3) NOT NULL DEFAULT 0,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
ALTER TABLE product_locations ALTER COLUMN zone_id DROP NOT NULL;

WITH totals AS (
    SELECT product_id, SUM(quantity) AS quantity, MIN(location_id) AS keep_id
    FROM product_locations
    WHERE zone_id IS NULL
    GROUP BY product_id
), updated AS (
    UPDATE product_locations pl
    SET quantity = totals.quantity, updated_at = CURRENT_TIMESTAMP
    FROM totals
    WHERE pl.location_id = totals.keep_id
)
DELETE FROM product_locations pl
USING totals
WHERE pl.product_id = totals.product_id
  AND pl.zone_id IS NULL
  AND pl.location_id <> totals.keep_id;

CREATE INDEX IF NOT EXISTS idx_product_locations_zone ON product_locations(zone_id);
CREATE UNIQUE INDEX IF NOT EXISTS uq_product_locations_product_zone
    ON product_locations(product_id, zone_id) NULLS NOT DISTINCT;

CREATE TABLE IF NOT EXISTS cable_products (
    cable_product_id BIGSERIAL PRIMARY KEY,
    product_id INT NOT NULL UNIQUE REFERENCES products(productid) ON DELETE CASCADE,
    connector_a_id INT NOT NULL REFERENCES cable_connectors(cable_connectorsid) ON DELETE RESTRICT,
    connector_b_id INT NOT NULL REFERENCES cable_connectors(cable_connectorsid) ON DELETE RESTRICT,
    cable_type_id INT NOT NULL REFERENCES cable_types(cable_typesid) ON DELETE RESTRICT,
    length_m NUMERIC(10,2) NOT NULL CHECK (length_m > 0),
    cross_section_mm2 NUMERIC(10,2),
    tracking_mode VARCHAR(20) NOT NULL DEFAULT 'quantity'
        CHECK (tracking_mode IN ('quantity', 'individual')),
    migrated_from_legacy BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_cable_products_connector_a ON cable_products(connector_a_id);
CREATE INDEX IF NOT EXISTS idx_cable_products_connector_b ON cable_products(connector_b_id);
CREATE INDEX IF NOT EXISTS idx_cable_products_type ON cable_products(cable_type_id);

-- Existing installations are migrated by WarehouseCore at startup. The marker
-- is written there only after all legacy rows have been converted successfully.

COMMIT;
