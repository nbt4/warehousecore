-- Retain manufacturer/brand history and protect active references for every writer.
ALTER TABLE manufacturer ADD COLUMN IF NOT EXISTS lifecycle_status VARCHAR(20) NOT NULL DEFAULT 'active' CHECK(lifecycle_status IN ('active','archived'));
ALTER TABLE manufacturer ADD COLUMN IF NOT EXISTS archived_at TIMESTAMP;
ALTER TABLE brands ADD COLUMN IF NOT EXISTS lifecycle_status VARCHAR(20) NOT NULL DEFAULT 'active' CHECK(lifecycle_status IN ('active','archived'));
ALTER TABLE brands ADD COLUMN IF NOT EXISTS archived_at TIMESTAMP;

CREATE OR REPLACE FUNCTION guard_warehouse_manufacturer_lifecycle() RETURNS TRIGGER AS $$
BEGIN
 IF OLD.lifecycle_status='archived' AND NEW.lifecycle_status<>'active' THEN
  RAISE EXCEPTION 'Restore archived manufacturer before editing' USING ERRCODE='23514';
 END IF;
 IF NEW.lifecycle_status='archived' AND OLD.lifecycle_status='active' AND (
  EXISTS(SELECT 1 FROM products WHERE manufacturerid=OLD.manufacturerid AND lifecycle_status='active') OR
  EXISTS(SELECT 1 FROM brands WHERE manufacturerid=OLD.manufacturerid AND lifecycle_status='active')) THEN
  RAISE EXCEPTION 'Active products or brands block manufacturer archive' USING ERRCODE='23514';
 END IF;
 RETURN NEW;
END; $$ LANGUAGE plpgsql;
DROP TRIGGER IF EXISTS manufacturer_guard_lifecycle ON manufacturer;
CREATE TRIGGER manufacturer_guard_lifecycle BEFORE UPDATE ON manufacturer FOR EACH ROW EXECUTE FUNCTION guard_warehouse_manufacturer_lifecycle();

CREATE OR REPLACE FUNCTION guard_warehouse_brand_lifecycle() RETURNS TRIGGER AS $$
BEGIN
 IF TG_OP='UPDATE' THEN
  IF OLD.lifecycle_status='archived' AND NEW.lifecycle_status<>'active' THEN
   RAISE EXCEPTION 'Restore archived brand before editing' USING ERRCODE='23514';
  END IF;
  IF NEW.lifecycle_status='archived' AND OLD.lifecycle_status='active' AND EXISTS(SELECT 1 FROM products WHERE brandid=OLD.brandid AND lifecycle_status='active') THEN
   RAISE EXCEPTION 'Active products block brand archive' USING ERRCODE='23514';
  END IF;
 END IF;
 IF NEW.lifecycle_status='active' AND NEW.manufacturerid IS NOT NULL THEN
  PERFORM 1 FROM manufacturer WHERE manufacturerid=NEW.manufacturerid AND lifecycle_status='active' FOR SHARE;
  IF NOT FOUND THEN RAISE EXCEPTION 'Active manufacturer required' USING ERRCODE='23514'; END IF;
 END IF;
 RETURN NEW;
END; $$ LANGUAGE plpgsql;
DROP TRIGGER IF EXISTS brands_guard_lifecycle ON brands;
CREATE TRIGGER brands_guard_lifecycle BEFORE INSERT OR UPDATE ON brands FOR EACH ROW EXECUTE FUNCTION guard_warehouse_brand_lifecycle();

CREATE OR REPLACE FUNCTION guard_warehouse_product_master_lifecycle() RETURNS TRIGGER AS $$
BEGIN
 IF NEW.lifecycle_status='active' THEN
  IF NEW.manufacturerid IS NOT NULL THEN
   PERFORM 1 FROM manufacturer WHERE manufacturerid=NEW.manufacturerid AND lifecycle_status='active' FOR SHARE;
   IF NOT FOUND THEN RAISE EXCEPTION 'Active product requires active manufacturer' USING ERRCODE='23514'; END IF;
  END IF;
  IF NEW.brandid IS NOT NULL THEN
   PERFORM 1 FROM brands WHERE brandid=NEW.brandid AND lifecycle_status='active' FOR SHARE;
   IF NOT FOUND THEN RAISE EXCEPTION 'Active product requires active brand' USING ERRCODE='23514'; END IF;
  END IF;
 END IF;
 RETURN NEW;
END; $$ LANGUAGE plpgsql;
DROP TRIGGER IF EXISTS products_guard_master_lifecycle ON products;
CREATE TRIGGER products_guard_master_lifecycle BEFORE INSERT OR UPDATE OF manufacturerid,brandid,lifecycle_status ON products FOR EACH ROW EXECUTE FUNCTION guard_warehouse_product_master_lifecycle();
INSERT INTO warehouse_schema_migrations(version) VALUES ('053_warehouse_master_lifecycle') ON CONFLICT DO NOTHING;
