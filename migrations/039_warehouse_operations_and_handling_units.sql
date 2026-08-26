-- Professional storage locations, warehouse work, cycle counts and dynamic handling units.
-- Idempotent: the application runs the equivalent schema initializer on startup.
BEGIN;

CREATE TABLE IF NOT EXISTS warehouse_schema_migrations (
    version VARCHAR(100) PRIMARY KEY,
    applied_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS location_profiles (
    profile_id BIGSERIAL PRIMARY KEY,
    code VARCHAR(50) NOT NULL UNIQUE,
    name VARCHAR(120) NOT NULL,
    description TEXT,
    allow_devices BOOLEAN NOT NULL DEFAULT TRUE,
    allow_quantity_products BOOLEAN NOT NULL DEFAULT TRUE,
    allow_cases BOOLEAN NOT NULL DEFAULT TRUE,
    allow_mixed_products BOOLEAN NOT NULL DEFAULT TRUE,
    allow_cycle_count BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
INSERT INTO location_profiles(code,name,description)
VALUES ('STANDARD','Standard-Lagerplatz','Universeller Lagerplatz für Geräte, Mengenartikel und Cases')
ON CONFLICT (code) DO NOTHING;

ALTER TABLE storage_zones ADD COLUMN IF NOT EXISTS location_kind VARCHAR(30);
ALTER TABLE storage_zones ADD COLUMN IF NOT EXISTS process_role VARCHAR(30);
ALTER TABLE storage_zones ADD COLUMN IF NOT EXISTS operational_status VARCHAR(30);
ALTER TABLE storage_zones ADD COLUMN IF NOT EXISTS is_storable BOOLEAN;
ALTER TABLE storage_zones ADD COLUMN IF NOT EXISTS pick_sequence INT;
ALTER TABLE storage_zones ADD COLUMN IF NOT EXISTS capacity_mode VARCHAR(30);
ALTER TABLE storage_zones ADD COLUMN IF NOT EXISTS max_weight_kg NUMERIC(12,3);
ALTER TABLE storage_zones ADD COLUMN IF NOT EXISTS max_volume_m3 NUMERIC(12,6);
ALTER TABLE storage_zones ADD COLUMN IF NOT EXISTS inventory_frequency_days INT;
ALTER TABLE storage_zones ADD COLUMN IF NOT EXISTS last_counted_at TIMESTAMP;
ALTER TABLE storage_zones ADD COLUMN IF NOT EXISTS next_count_at TIMESTAMP;
ALTER TABLE storage_zones ADD COLUMN IF NOT EXISTS profile_id BIGINT REFERENCES location_profiles(profile_id) ON DELETE SET NULL;

UPDATE storage_zones SET location_kind=CASE type::text
    WHEN 'warehouse' THEN 'site' WHEN 'rack' THEN 'rack' WHEN 'shelf' THEN 'bin'
    WHEN 'vehicle' THEN 'vehicle' WHEN 'stage' THEN 'area' ELSE 'area' END
WHERE location_kind IS NULL OR location_kind='';
UPDATE storage_zones SET process_role=CASE type::text
    WHEN 'stage' THEN 'staging' WHEN 'vehicle' THEN 'transport' ELSE 'storage' END
WHERE process_role IS NULL OR process_role='';
UPDATE storage_zones SET operational_status=CASE WHEN is_active THEN 'available' ELSE 'archived' END
WHERE operational_status IS NULL OR operational_status='';
UPDATE storage_zones SET is_storable=CASE WHEN type::text IN ('warehouse','rack') THEN FALSE ELSE TRUE END
WHERE is_storable IS NULL;
UPDATE storage_zones SET capacity_mode='item_count' WHERE capacity_mode IS NULL OR capacity_mode='';

ALTER TABLE storage_zones ALTER COLUMN location_kind SET DEFAULT 'area';
ALTER TABLE storage_zones ALTER COLUMN location_kind SET NOT NULL;
ALTER TABLE storage_zones ALTER COLUMN process_role SET DEFAULT 'storage';
ALTER TABLE storage_zones ALTER COLUMN process_role SET NOT NULL;
ALTER TABLE storage_zones ALTER COLUMN operational_status SET DEFAULT 'available';
ALTER TABLE storage_zones ALTER COLUMN operational_status SET NOT NULL;
ALTER TABLE storage_zones ALTER COLUMN is_storable SET DEFAULT TRUE;
ALTER TABLE storage_zones ALTER COLUMN is_storable SET NOT NULL;
ALTER TABLE storage_zones ALTER COLUMN capacity_mode SET DEFAULT 'item_count';
ALTER TABLE storage_zones ALTER COLUMN capacity_mode SET NOT NULL;

DO $$ BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname='chk_storage_zones_operational_status') THEN
        ALTER TABLE storage_zones ADD CONSTRAINT chk_storage_zones_operational_status
            CHECK (operational_status IN ('available','blocked','counting','maintenance','archived'));
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname='chk_storage_zones_process_role') THEN
        ALTER TABLE storage_zones ADD CONSTRAINT chk_storage_zones_process_role
            CHECK (process_role IN ('storage','receiving','return','inspection','quarantine','repair','charging','picking','staging','shipping','transport','unknown'));
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname='chk_storage_zones_capacity_positive') THEN
        ALTER TABLE storage_zones ADD CONSTRAINT chk_storage_zones_capacity_positive CHECK (capacity IS NULL OR capacity>0);
    END IF;
END $$;
CREATE INDEX IF NOT EXISTS idx_storage_zones_operational_status ON storage_zones(operational_status);
CREATE INDEX IF NOT EXISTS idx_storage_zones_pick_sequence ON storage_zones(pick_sequence,code);

ALTER TABLE cases ADD COLUMN IF NOT EXISTS zone_id INT REFERENCES storage_zones(zone_id) ON DELETE SET NULL;
ALTER TABLE cases ADD COLUMN IF NOT EXISTS barcode VARCHAR(255);
ALTER TABLE cases ADD COLUMN IF NOT EXISTS rfid_tag VARCHAR(255);
ALTER TABLE cases ADD COLUMN IF NOT EXISTS case_type VARCHAR(20);
ALTER TABLE cases ADD COLUMN IF NOT EXISTS workflow_status VARCHAR(30);
ALTER TABLE cases ADD COLUMN IF NOT EXISTS home_zone_id INT REFERENCES storage_zones(zone_id) ON DELETE SET NULL;
ALTER TABLE cases ADD COLUMN IF NOT EXISTS current_job_id BIGINT;
ALTER TABLE cases ADD COLUMN IF NOT EXISTS sealed_at TIMESTAMP;
ALTER TABLE cases ADD COLUMN IF NOT EXISTS max_weight_kg NUMERIC(12,3);
UPDATE cases SET case_type='dynamic' WHERE case_type IS NULL OR case_type='';
UPDATE cases SET workflow_status=CASE WHEN status='rented' THEN 'on_job' WHEN status='maintance' THEN 'maintenance' ELSE 'empty' END
WHERE workflow_status IS NULL OR workflow_status='';
ALTER TABLE cases ALTER COLUMN case_type SET DEFAULT 'dynamic';
ALTER TABLE cases ALTER COLUMN case_type SET NOT NULL;
ALTER TABLE cases ALTER COLUMN workflow_status SET DEFAULT 'empty';
ALTER TABLE cases ALTER COLUMN workflow_status SET NOT NULL;
DO $$ BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname='chk_cases_case_type') THEN
        ALTER TABLE cases ADD CONSTRAINT chk_cases_case_type CHECK (case_type IN ('dynamic','fixed','hybrid'));
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname='chk_cases_workflow_status') THEN
        ALTER TABLE cases ADD CONSTRAINT chk_cases_workflow_status
            CHECK (workflow_status IN ('empty','packing','complete','sealed','staged','on_job','return_check','maintenance'));
    END IF;
END $$;
CREATE UNIQUE INDEX IF NOT EXISTS uq_cases_barcode_normalized ON cases(LOWER(TRIM(barcode))) WHERE NULLIF(TRIM(barcode),'') IS NOT NULL;

CREATE TABLE IF NOT EXISTS case_product_contents (
    content_id BIGSERIAL PRIMARY KEY,
    case_id INT NOT NULL REFERENCES cases(caseID) ON DELETE CASCADE,
    product_id INT NOT NULL REFERENCES products(productID) ON DELETE RESTRICT,
    quantity NUMERIC(12,3) NOT NULL CHECK (quantity>0),
    added_from_zone_id INT REFERENCES storage_zones(zone_id) ON DELETE SET NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(case_id,product_id)
);
CREATE TABLE IF NOT EXISTS case_content_templates (
    template_line_id BIGSERIAL PRIMARY KEY,
    case_id INT NOT NULL REFERENCES cases(caseID) ON DELETE CASCADE,
    product_id INT NOT NULL REFERENCES products(productID) ON DELETE RESTRICT,
    expected_quantity NUMERIC(12,3) NOT NULL CHECK (expected_quantity>0),
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(case_id,product_id)
);
CREATE TABLE IF NOT EXISTS case_child_contents (
    parent_case_id INT NOT NULL REFERENCES cases(caseID) ON DELETE CASCADE,
    child_case_id INT NOT NULL UNIQUE REFERENCES cases(caseID) ON DELETE CASCADE,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY(parent_case_id,child_case_id),
    CHECK(parent_case_id<>child_case_id)
);
CREATE TABLE IF NOT EXISTS case_events (
    event_id BIGSERIAL PRIMARY KEY,
    case_id INT NOT NULL REFERENCES cases(caseID) ON DELETE CASCADE,
    event_type VARCHAR(40) NOT NULL,
    device_id VARCHAR(255), product_id INT, quantity NUMERIC(12,3), zone_id INT, job_id BIGINT,
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_case_events_case_created ON case_events(case_id,created_at DESC);

CREATE TABLE IF NOT EXISTS warehouse_tasks (
    task_id BIGSERIAL PRIMARY KEY,
    task_type VARCHAR(30) NOT NULL CHECK (task_type IN ('putaway','move','pick','replenish','count','inspect','pack','return')),
    status VARCHAR(20) NOT NULL DEFAULT 'open' CHECK (status IN ('open','in_progress','done','cancelled')),
    priority INT NOT NULL DEFAULT 50 CHECK (priority BETWEEN 0 AND 100),
    from_zone_id INT REFERENCES storage_zones(zone_id) ON DELETE SET NULL,
    to_zone_id INT REFERENCES storage_zones(zone_id) ON DELETE SET NULL,
    case_id INT REFERENCES cases(caseID) ON DELETE SET NULL,
    device_id VARCHAR(255), product_id INT, quantity NUMERIC(12,3), job_id BIGINT, assigned_to BIGINT,
    due_at TIMESTAMP, notes TEXT,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    completed_at TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_warehouse_tasks_status_priority ON warehouse_tasks(status,priority DESC,due_at);

CREATE TABLE IF NOT EXISTS inventory_counts (
    count_id BIGSERIAL PRIMARY KEY,
    zone_id INT NOT NULL REFERENCES storage_zones(zone_id) ON DELETE RESTRICT,
    status VARCHAR(20) NOT NULL DEFAULT 'open' CHECK (status IN ('open','counting','review','approved','cancelled')),
    blind_count BOOLEAN NOT NULL DEFAULT TRUE,
    started_at TIMESTAMP, completed_at TIMESTAMP,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE TABLE IF NOT EXISTS inventory_count_lines (
    line_id BIGSERIAL PRIMARY KEY,
    count_id BIGINT NOT NULL REFERENCES inventory_counts(count_id) ON DELETE CASCADE,
    item_type VARCHAR(20) NOT NULL CHECK (item_type IN ('device','product','case')),
    item_key VARCHAR(255) NOT NULL,
    expected_quantity NUMERIC(12,3) NOT NULL DEFAULT 0,
    counted_quantity NUMERIC(12,3),
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(count_id,item_type,item_key)
);

DO $$ BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname='fk_devices_storage_zone') THEN
        ALTER TABLE devices ADD CONSTRAINT fk_devices_storage_zone
            FOREIGN KEY(zone_id) REFERENCES storage_zones(zone_id) ON DELETE SET NULL NOT VALID;
    END IF;
END $$;

INSERT INTO warehouse_schema_migrations(version)
VALUES ('039_warehouse_operations_and_handling_units') ON CONFLICT (version) DO NOTHING;
COMMIT;
