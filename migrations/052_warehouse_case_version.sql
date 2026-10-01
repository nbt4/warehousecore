-- Case lifecycle and monotonic versions for metadata and physical contents.
ALTER TABLE cases ADD COLUMN IF NOT EXISTS rfid_tag VARCHAR(255);
CREATE TABLE IF NOT EXISTS case_product_contents(content_id BIGSERIAL PRIMARY KEY,case_id INT NOT NULL REFERENCES cases(caseid) ON DELETE CASCADE,product_id INT NOT NULL REFERENCES products(productid) ON DELETE RESTRICT,quantity NUMERIC(12,3) NOT NULL CHECK(quantity>0),added_from_zone_id INT REFERENCES storage_zones(zone_id) ON DELETE SET NULL,created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,UNIQUE(case_id,product_id));
CREATE TABLE IF NOT EXISTS case_content_templates(template_line_id BIGSERIAL PRIMARY KEY,case_id INT NOT NULL REFERENCES cases(caseid) ON DELETE CASCADE,product_id INT NOT NULL REFERENCES products(productid) ON DELETE RESTRICT,expected_quantity NUMERIC(12,3) NOT NULL CHECK(expected_quantity>0),created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,UNIQUE(case_id,product_id));
CREATE TABLE IF NOT EXISTS case_child_contents(parent_case_id INT NOT NULL REFERENCES cases(caseid) ON DELETE CASCADE,child_case_id INT NOT NULL UNIQUE REFERENCES cases(caseid) ON DELETE CASCADE,created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,PRIMARY KEY(parent_case_id,child_case_id),CHECK(parent_case_id<>child_case_id));

ALTER TABLE cases ADD COLUMN IF NOT EXISTS lifecycle_status VARCHAR(20) NOT NULL DEFAULT 'active' CHECK(lifecycle_status IN ('active','archived'));
DO $$ BEGIN IF NOT EXISTS(SELECT 1 FROM pg_constraint WHERE conrelid='cases'::regclass AND conname='cases_mcp_lifecycle_check') THEN ALTER TABLE cases ADD CONSTRAINT cases_mcp_lifecycle_check CHECK(lifecycle_status IN ('active','archived')); END IF; END $$;
ALTER TABLE cases ADD COLUMN IF NOT EXISTS updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP;
UPDATE cases SET updated_at=CURRENT_TIMESTAMP WHERE updated_at IS NULL;
ALTER TABLE cases ALTER COLUMN updated_at SET NOT NULL;
CREATE OR REPLACE FUNCTION touch_warehouse_case_version() RETURNS TRIGGER AS $$
BEGIN
 IF OLD.lifecycle_status='archived' AND NEW.lifecycle_status='archived' THEN
  RAISE EXCEPTION 'Restore archived case before editing';
 END IF;
 NEW.updated_at := GREATEST(clock_timestamp() AT TIME ZONE 'UTC', OLD.updated_at + INTERVAL '1 microsecond');
 RETURN NEW;
END;
$$ LANGUAGE plpgsql;
DROP TRIGGER IF EXISTS cases_touch_version ON cases;
CREATE TRIGGER cases_touch_version BEFORE UPDATE ON cases FOR EACH ROW EXECUTE FUNCTION touch_warehouse_case_version();
CREATE OR REPLACE FUNCTION sync_case_identifier() RETURNS TRIGGER AS $$
BEGIN
 INSERT INTO inventory_identifiers(entity_type,entity_key,code,identifier_kind,active)
 VALUES('case',NEW.caseid::text,NEW.barcode,'canonical',NEW.lifecycle_status='active')
 ON CONFLICT(entity_type,entity_key,identifier_kind) DO UPDATE SET code=EXCLUDED.code,active=EXCLUDED.active;
 DELETE FROM inventory_identifiers WHERE entity_type='case' AND entity_key=NEW.caseid::text AND identifier_kind='rfid';
 IF NULLIF(TRIM(NEW.rfid_tag),'') IS NOT NULL AND LOWER(TRIM(NEW.rfid_tag))<>LOWER(TRIM(NEW.barcode)) THEN
  INSERT INTO inventory_identifiers(entity_type,entity_key,code,identifier_kind,active) VALUES('case',NEW.caseid::text,NEW.rfid_tag,'rfid',NEW.lifecycle_status='active') ON CONFLICT(entity_type,entity_key,identifier_kind) DO UPDATE SET code=EXCLUDED.code,active=EXCLUDED.active;
 END IF;
 UPDATE inventory_identifiers SET active=(NEW.lifecycle_status='active') WHERE entity_type='case' AND entity_key=NEW.caseid::text AND active IS DISTINCT FROM (NEW.lifecycle_status='active');
 RETURN NEW;
END;
$$ LANGUAGE plpgsql;
DROP TRIGGER IF EXISTS cases_identifier_after_write ON cases;
CREATE TRIGGER cases_identifier_after_write AFTER INSERT OR UPDATE OF barcode,rfid_tag,lifecycle_status ON cases FOR EACH ROW EXECUTE FUNCTION sync_case_identifier();
UPDATE inventory_identifiers i SET active=(c.lifecycle_status='active') FROM cases c WHERE i.entity_type='case' AND i.entity_key=c.caseid::text AND i.active IS DISTINCT FROM (c.lifecycle_status='active');
CREATE OR REPLACE FUNCTION touch_warehouse_case_contents_version() RETURNS TRIGGER AS $$
DECLARE old_id INT; new_id INT; old_child INT; new_child INT;
BEGIN
 IF TG_TABLE_NAME='case_child_contents' THEN
  IF TG_OP<>'INSERT' THEN old_id:=OLD.parent_case_id; old_child:=OLD.child_case_id; END IF;
  IF TG_OP<>'DELETE' THEN new_id:=NEW.parent_case_id; new_child:=NEW.child_case_id; END IF;
 ELSIF TG_TABLE_NAME='devicescases' THEN
  IF TG_OP<>'INSERT' THEN old_id:=OLD.caseid; END IF;
  IF TG_OP<>'DELETE' THEN new_id:=NEW.caseid; END IF;
 ELSE
  IF TG_OP<>'INSERT' THEN old_id:=OLD.case_id; END IF;
  IF TG_OP<>'DELETE' THEN new_id:=NEW.case_id; END IF;
 END IF;
 UPDATE cases SET updated_at=CURRENT_TIMESTAMP WHERE caseid IN (old_id,new_id,old_child,new_child);
 RETURN NULL;
END;
$$ LANGUAGE plpgsql;
DROP TRIGGER IF EXISTS devicescases_touch_case_version ON devicescases;
CREATE TRIGGER devicescases_touch_case_version AFTER INSERT OR UPDATE OR DELETE ON devicescases FOR EACH ROW EXECUTE FUNCTION touch_warehouse_case_contents_version();
DROP TRIGGER IF EXISTS case_product_contents_touch_version ON case_product_contents;
CREATE TRIGGER case_product_contents_touch_version AFTER INSERT OR UPDATE OR DELETE ON case_product_contents FOR EACH ROW EXECUTE FUNCTION touch_warehouse_case_contents_version();
DROP TRIGGER IF EXISTS case_child_contents_touch_version ON case_child_contents;
CREATE TRIGGER case_child_contents_touch_version AFTER INSERT OR UPDATE OR DELETE ON case_child_contents FOR EACH ROW EXECUTE FUNCTION touch_warehouse_case_contents_version();
DROP TRIGGER IF EXISTS case_content_templates_touch_version ON case_content_templates;
CREATE TRIGGER case_content_templates_touch_version AFTER INSERT OR UPDATE OR DELETE ON case_content_templates FOR EACH ROW EXECUTE FUNCTION touch_warehouse_case_contents_version();
INSERT INTO warehouse_schema_migrations(version) VALUES('052_warehouse_case_version') ON CONFLICT DO NOTHING;
