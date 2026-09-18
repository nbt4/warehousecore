-- Product master data v2 is installed idempotently by
-- internal/handlers/product_master_schema.go during application startup.
-- Keep the dependency base table here as well so standalone migration runners
-- and the runtime upgrader provide the same clean-install contract.
CREATE TABLE IF NOT EXISTS product_dependencies (
    id SERIAL PRIMARY KEY,
    product_id INTEGER NOT NULL REFERENCES products(productid) ON DELETE CASCADE,
    dependency_product_id INTEGER NOT NULL REFERENCES products(productid) ON DELETE CASCADE,
    is_optional BOOLEAN DEFAULT TRUE,
    default_quantity NUMERIC(10,2) DEFAULT 1.0,
    notes VARCHAR(500),
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT unique_dependency UNIQUE (product_id, dependency_product_id)
);
CREATE INDEX IF NOT EXISTS idx_product_dependencies_product_id ON product_dependencies(product_id);
CREATE INDEX IF NOT EXISTS idx_product_dependencies_dep_product_id ON product_dependencies(dependency_product_id);

INSERT INTO warehouse_schema_migrations(version)
VALUES ('043_product_master_v2')
ON CONFLICT(version) DO NOTHING;
