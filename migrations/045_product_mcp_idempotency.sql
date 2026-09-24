CREATE TABLE IF NOT EXISTS warehouse_product_mutation_receipts (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL,
    operation VARCHAR(80) NOT NULL,
    key_hash CHAR(64) NOT NULL,
    request_hash CHAR(64) NOT NULL,
    response JSONB NOT NULL DEFAULT '{}'::jsonb,
    status_code INTEGER NOT NULL DEFAULT 200,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (user_id, operation, key_hash)
);

INSERT INTO warehouse_schema_migrations(version)
VALUES('045_product_mcp_idempotency')
ON CONFLICT(version) DO NOTHING;
