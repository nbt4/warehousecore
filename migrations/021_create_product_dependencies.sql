-- Migration: Create product_dependencies table
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
