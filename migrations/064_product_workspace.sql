CREATE TABLE IF NOT EXISTS warehouse_picture_folders (
 entity_type TEXT NOT NULL CHECK(entity_type IN ('product','package')),
 entity_id BIGINT NOT NULL, folder_name TEXT NOT NULL,
 PRIMARY KEY(entity_type,entity_id)
);
-- Preserve existing Nextcloud paths across future display-name changes.
INSERT INTO warehouse_picture_folders SELECT 'product',productid,name FROM products ON CONFLICT DO NOTHING;
INSERT INTO warehouse_picture_folders SELECT 'package',id,name FROM product_packages ON CONFLICT DO NOTHING;
CREATE TABLE IF NOT EXISTS warehouse_picture_files (
 entity_type TEXT NOT NULL CHECK(entity_type IN ('product','package')),
 entity_id BIGINT NOT NULL,file_name TEXT NOT NULL,size BIGINT NOT NULL DEFAULT 0,
 content_type TEXT NOT NULL DEFAULT '',modified_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 PRIMARY KEY(entity_type,entity_id,file_name)
);
CREATE TABLE IF NOT EXISTS warehouse_website_content (
 entity_type TEXT NOT NULL CHECK(entity_type IN ('product','package')),
 entity_id BIGINT NOT NULL,draft JSONB,published JSONB,
 revision BIGINT NOT NULL DEFAULT 0,published_revision BIGINT NOT NULL DEFAULT 0,
 updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),published_at TIMESTAMPTZ,
 PRIMARY KEY(entity_type,entity_id)
);
CREATE TABLE IF NOT EXISTS website_revalidation (
 path TEXT PRIMARY KEY,generation BIGINT NOT NULL DEFAULT 0,delivered_generation BIGINT NOT NULL DEFAULT 0,
 status TEXT NOT NULL CHECK(status IN ('pending','processing','synced','error','disabled')),
 requested_at TIMESTAMPTZ NOT NULL DEFAULT now(),synced_at TIMESTAMPTZ,
 next_attempt TIMESTAMPTZ NOT NULL DEFAULT now(),attempts INTEGER NOT NULL DEFAULT 0,last_error TEXT NOT NULL DEFAULT ''
);
-- New entities get immutable, separate folders; legacy mappings above are retained.
CREATE OR REPLACE FUNCTION initialize_catalog_picture_folder() RETURNS TRIGGER AS $$
DECLARE kind TEXT; key BIGINT;
BEGIN
 kind:=CASE WHEN TG_TABLE_NAME='products' THEN 'product' ELSE 'package' END;
 key:=CASE WHEN TG_TABLE_NAME='products' THEN (to_jsonb(NEW)->>'productid')::bigint ELSE (to_jsonb(NEW)->>'id')::bigint END;
 INSERT INTO warehouse_picture_folders(entity_type,entity_id,folder_name)
 VALUES(kind,key,kind||'-'||key::text||'-'||replace(gen_random_uuid()::text,'-','')) ON CONFLICT DO NOTHING;
 RETURN NEW;
END; $$ LANGUAGE plpgsql;
DROP TRIGGER IF EXISTS products_initialize_picture_folder ON products;
CREATE TRIGGER products_initialize_picture_folder AFTER INSERT ON products FOR EACH ROW EXECUTE FUNCTION initialize_catalog_picture_folder();
DROP TRIGGER IF EXISTS packages_initialize_picture_folder ON product_packages;
CREATE TRIGGER packages_initialize_picture_folder AFTER INSERT ON product_packages FOR EACH ROW EXECUTE FUNCTION initialize_catalog_picture_folder();
CREATE OR REPLACE FUNCTION cleanup_catalog_workspace() RETURNS TRIGGER AS $$
DECLARE kind TEXT; key BIGINT;
BEGIN
 kind:=CASE WHEN TG_TABLE_NAME='products' THEN 'product' ELSE 'package' END;
 key:=CASE WHEN TG_TABLE_NAME='products' THEN (to_jsonb(OLD)->>'productid')::bigint ELSE (to_jsonb(OLD)->>'id')::bigint END;
 DELETE FROM warehouse_picture_folders WHERE entity_type=kind AND entity_id=key;
 DELETE FROM warehouse_picture_files WHERE entity_type=kind AND entity_id=key;
 DELETE FROM warehouse_website_content WHERE entity_type=kind AND entity_id=key;
 RETURN OLD;
END; $$ LANGUAGE plpgsql;
DROP TRIGGER IF EXISTS products_cleanup_workspace ON products;
CREATE TRIGGER products_cleanup_workspace AFTER DELETE ON products FOR EACH ROW EXECUTE FUNCTION cleanup_catalog_workspace();
DROP TRIGGER IF EXISTS packages_cleanup_workspace ON product_packages;
CREATE TRIGGER packages_cleanup_workspace AFTER DELETE ON product_packages FOR EACH ROW EXECUTE FUNCTION cleanup_catalog_workspace();
ALTER TABLE count_types ADD COLUMN IF NOT EXISTS lifecycle_status TEXT NOT NULL DEFAULT 'active' CHECK(lifecycle_status IN ('active','archived'));
ALTER TABLE count_types ADD COLUMN IF NOT EXISTS updated_at TIMESTAMP NOT NULL DEFAULT now();
ALTER TABLE cable_types ADD COLUMN IF NOT EXISTS lifecycle_status TEXT NOT NULL DEFAULT 'active' CHECK(lifecycle_status IN ('active','archived'));
ALTER TABLE cable_types ADD COLUMN IF NOT EXISTS updated_at TIMESTAMP NOT NULL DEFAULT now();
ALTER TABLE cable_connectors ADD COLUMN IF NOT EXISTS lifecycle_status TEXT NOT NULL DEFAULT 'active' CHECK(lifecycle_status IN ('active','archived'));
ALTER TABLE cable_connectors ADD COLUMN IF NOT EXISTS updated_at TIMESTAMP NOT NULL DEFAULT now();
CREATE OR REPLACE FUNCTION guard_catalog_unit_and_connector_lifecycle() RETURNS TRIGGER AS $$
DECLARE referenced BOOLEAN:=false;
BEGIN
 IF OLD.lifecycle_status='archived' AND NEW.lifecycle_status<>'active' THEN RAISE EXCEPTION 'Restore master record before editing' USING ERRCODE='23514'; END IF;
 IF NEW.lifecycle_status='archived' AND OLD.lifecycle_status='active' THEN
  IF TG_TABLE_NAME='count_types' THEN SELECT EXISTS(SELECT 1 FROM products WHERE count_type_id=OLD.count_type_id AND lifecycle_status='active') INTO referenced;
  ELSIF TG_TABLE_NAME='cable_types' THEN SELECT EXISTS(SELECT 1 FROM cable_products cp JOIN products p ON p.productid=cp.product_id WHERE cp.cable_type_id=OLD.cable_typesid AND p.lifecycle_status='active') INTO referenced;
  ELSE SELECT EXISTS(SELECT 1 FROM cable_products cp JOIN products p ON p.productid=cp.product_id WHERE (cp.connector_a_id=OLD.cable_connectorsid OR cp.connector_b_id=OLD.cable_connectorsid) AND p.lifecycle_status='active') INTO referenced; END IF;
  IF referenced THEN RAISE EXCEPTION 'Active products block master archive' USING ERRCODE='23514'; END IF;
 END IF;
 NEW.updated_at:=GREATEST(clock_timestamp() AT TIME ZONE 'UTC',OLD.updated_at+INTERVAL '1 microsecond');
 RETURN NEW;
END; $$ LANGUAGE plpgsql;
DROP TRIGGER IF EXISTS count_types_catalog_lifecycle ON count_types;
CREATE TRIGGER count_types_catalog_lifecycle BEFORE UPDATE ON count_types FOR EACH ROW EXECUTE FUNCTION guard_catalog_unit_and_connector_lifecycle();
DROP TRIGGER IF EXISTS cable_types_catalog_lifecycle ON cable_types;
CREATE TRIGGER cable_types_catalog_lifecycle BEFORE UPDATE ON cable_types FOR EACH ROW EXECUTE FUNCTION guard_catalog_unit_and_connector_lifecycle();
DROP TRIGGER IF EXISTS cable_connectors_catalog_lifecycle ON cable_connectors;
CREATE TRIGGER cable_connectors_catalog_lifecycle BEFORE UPDATE ON cable_connectors FOR EACH ROW EXECUTE FUNCTION guard_catalog_unit_and_connector_lifecycle();

CREATE TABLE IF NOT EXISTS warehouse_picture_deletions (
 entity_type TEXT NOT NULL,entity_id BIGINT NOT NULL,folder_name TEXT NOT NULL,file_name TEXT NOT NULL,
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(),PRIMARY KEY(entity_type,entity_id,file_name));

CREATE TABLE IF NOT EXISTS warehouse_material_snapshots (
 job_id INTEGER PRIMARY KEY REFERENCES jobs(jobid) ON DELETE CASCADE,
 version BIGINT NOT NULL DEFAULT 1,source_hash TEXT NOT NULL,roots JSONB NOT NULL,graph JSONB NOT NULL,
 items JSONB NOT NULL,warnings JSONB NOT NULL,updated_at TIMESTAMPTZ NOT NULL DEFAULT now());
CREATE TABLE IF NOT EXISTS warehouse_material_scans (
 id BIGSERIAL PRIMARY KEY,receipt TEXT NOT NULL UNIQUE,job_id INTEGER NOT NULL REFERENCES jobs(jobid) ON DELETE CASCADE,
 product_id INTEGER NOT NULL REFERENCES products(productid),device_id VARCHAR(50) REFERENCES devices(deviceid),
 quantity NUMERIC(14,3) NOT NULL CHECK(quantity>0),scanned_by TEXT NOT NULL DEFAULT '',scanned_at TIMESTAMPTZ NOT NULL DEFAULT now());
CREATE UNIQUE INDEX IF NOT EXISTS warehouse_material_device_scan ON warehouse_material_scans(job_id,device_id) WHERE device_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS warehouse_material_job_scan ON warehouse_material_scans(job_id,product_id);
