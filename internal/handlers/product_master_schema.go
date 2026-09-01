package handlers

import (
	"fmt"

	"warehousecore/internal/repository"
)

// EnsureProductMasterSchema installs immutable suite-wide inventory codes and
// the richer product master data model. All statements are intentionally
// idempotent because WarehouseCore also upgrades installations that skipped
// one or more historical migration files.
func EnsureProductMasterSchema() error {
	db := repository.GetSQLDB()
	if db == nil {
		return fmt.Errorf("database is not initialized")
	}

	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin product master schema transaction: %w", err)
	}
	defer tx.Rollback()

	statements := []string{
		`CREATE SEQUENCE IF NOT EXISTS product_master_code_seq START WITH 1`,
		`CREATE SEQUENCE IF NOT EXISTS device_master_code_seq START WITH 1`,
		`CREATE SEQUENCE IF NOT EXISTS case_master_code_seq START WITH 1`,
		`ALTER TABLE products ADD COLUMN IF NOT EXISTS product_code VARCHAR(32)`,
		`ALTER TABLE products ADD COLUMN IF NOT EXISTS product_kind VARCHAR(24)`,
		`ALTER TABLE products ADD COLUMN IF NOT EXISTS model_number VARCHAR(160)`,
		`ALTER TABLE products ADD COLUMN IF NOT EXISTS manufacturer_part_number VARCHAR(160)`,
		`ALTER TABLE products ADD COLUMN IF NOT EXISTS ean VARCHAR(32)`,
		`ALTER TABLE products ADD COLUMN IF NOT EXISTS attributes JSONB NOT NULL DEFAULT '{}'::jsonb`,
		`ALTER TABLE products ALTER COLUMN product_code SET DEFAULT ('PRD-' || LPAD(nextval('product_master_code_seq')::text, 8, '0'))`,
		`UPDATE products SET product_code='PRD-' || LPAD(productID::text, 8, '0') WHERE NULLIF(TRIM(product_code),'') IS NULL`,
		`SELECT setval('product_master_code_seq',GREATEST(COALESCE((SELECT MAX(productID) FROM products),0),COALESCE((SELECT MAX(SUBSTRING(product_code FROM 5)::BIGINT) FROM products WHERE product_code ~ '^PRD-[0-9]+$'),0),(SELECT last_value FROM product_master_code_seq),1),TRUE)`,
		`UPDATE products SET product_kind=CASE WHEN EXISTS(SELECT 1 FROM cable_products cp WHERE cp.product_id=products.productID) THEN 'cable' WHEN product_type='consumable' THEN 'consumable' ELSE 'standard' END WHERE NULLIF(TRIM(product_kind),'') IS NULL`,
		`UPDATE products p SET attributes=jsonb_build_object(CASE WHEN LOWER(sbc.name) IN ('wired','wireless') THEN 'connection' WHEN LOWER(sbc.name) IN ('active','passive') THEN 'amplification' WHEN LOWER(sbc.name) IN ('analog','digital') THEN 'signal_type' WHEN LOWER(sbc.name) IN ('big','small') THEN 'size_class' WHEN LOWER(sbc.name) IN ('onepoint','twopoint','threepoint','fourpoint') THEN 'truss_system' WHEN LOWER(sbc.name) IN ('threeleg','fourleg') THEN 'leg_layout' WHEN LOWER(sbc.name)='notebook needed' THEN 'requires_notebook' ELSE 'variant' END,CASE WHEN LOWER(sbc.name)='notebook needed' THEN to_jsonb(TRUE) ELSE to_jsonb(LOWER(TRIM(sbc.name))) END) FROM subbiercategories sbc WHERE p.subbiercategoryID=sbc.subbiercategoryID AND sbc.name IS NOT NULL AND COALESCE(p.attributes,'{}'::jsonb)='{}'::jsonb`,
		`UPDATE products SET generic_barcode=product_code WHERE NULLIF(TRIM(generic_barcode),'') IS NULL`,
		`ALTER TABLE products ALTER COLUMN product_code SET NOT NULL`,
		`ALTER TABLE products ALTER COLUMN product_kind SET DEFAULT 'standard'`,
		`ALTER TABLE products ALTER COLUMN product_kind SET NOT NULL`,
		`DO $$ BEGIN IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname='chk_products_product_kind') THEN ALTER TABLE products ADD CONSTRAINT chk_products_product_kind CHECK(product_kind IN ('standard','cable','consumable','container','service')); END IF; END $$`,
		`CREATE UNIQUE INDEX IF NOT EXISTS uq_products_product_code_normalized ON products(LOWER(TRIM(product_code)))`,
		`CREATE UNIQUE INDEX IF NOT EXISTS uq_products_ean_normalized ON products(LOWER(TRIM(ean))) WHERE NULLIF(TRIM(ean),'') IS NOT NULL`,
		`CREATE UNIQUE INDEX IF NOT EXISTS uq_devices_barcode_normalized ON devices(LOWER(TRIM(barcode))) WHERE NULLIF(TRIM(barcode),'') IS NOT NULL`,
		`CREATE UNIQUE INDEX IF NOT EXISTS uq_devices_qr_normalized ON devices(LOWER(TRIM(qr_code))) WHERE NULLIF(TRIM(qr_code),'') IS NOT NULL`,
		`UPDATE devices SET barcode=deviceID WHERE NULLIF(TRIM(barcode),'') IS NULL`,
		`UPDATE devices SET qr_code='WH:' || deviceID WHERE NULLIF(TRIM(qr_code),'') IS NULL`,
		`SELECT setval('device_master_code_seq',GREATEST((SELECT COUNT(*) FROM devices),COALESCE((SELECT MAX(SUBSTRING(deviceID FROM 5)::BIGINT) FROM devices WHERE deviceID ~ '^DEV-[0-9]+$'),0),(SELECT last_value FROM device_master_code_seq),1),TRUE)`,
		`UPDATE cases SET barcode='CAS-' || LPAD(caseID::text, 8, '0') WHERE NULLIF(TRIM(barcode),'') IS NULL`,
		`SELECT setval('case_master_code_seq',GREATEST(COALESCE((SELECT MAX(caseID) FROM cases),0),COALESCE((SELECT MAX(SUBSTRING(barcode FROM 5)::BIGINT) FROM cases WHERE barcode ~ '^CAS-[0-9]+$'),0),(SELECT last_value FROM case_master_code_seq),1),TRUE)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS uq_brands_name_normalized ON brands(LOWER(TRIM(name)))`,
		`CREATE UNIQUE INDEX IF NOT EXISTS uq_manufacturer_name_normalized ON manufacturer(LOWER(TRIM(name)))`,
		`CREATE UNIQUE INDEX IF NOT EXISTS uq_categories_name_normalized ON categories(LOWER(TRIM(name)))`,
		`CREATE UNIQUE INDEX IF NOT EXISTS uq_subcategories_parent_name_normalized ON subcategories(categoryID,LOWER(TRIM(name)))`,
		`CREATE UNIQUE INDEX IF NOT EXISTS uq_third_categories_parent_name_normalized ON subbiercategories(subcategoryID,LOWER(TRIM(name)))`,
		`ALTER TABLE product_dependencies ADD COLUMN IF NOT EXISTS relation_type VARCHAR(24)`,
		`ALTER TABLE product_dependencies ADD COLUMN IF NOT EXISTS assignment_scope VARCHAR(20)`,
		`UPDATE product_dependencies SET relation_type=CASE WHEN is_optional THEN 'recommended' ELSE 'required' END WHERE relation_type IS NULL OR relation_type=''`,
		`UPDATE product_dependencies SET assignment_scope='product' WHERE assignment_scope IS NULL OR assignment_scope=''`,
		`ALTER TABLE product_dependencies ALTER COLUMN relation_type SET DEFAULT 'recommended'`,
		`ALTER TABLE product_dependencies ALTER COLUMN relation_type SET NOT NULL`,
		`ALTER TABLE product_dependencies ALTER COLUMN assignment_scope SET DEFAULT 'product'`,
		`ALTER TABLE product_dependencies ALTER COLUMN assignment_scope SET NOT NULL`,
		`DO $$ BEGIN IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname='chk_product_dependencies_relation_type') THEN ALTER TABLE product_dependencies ADD CONSTRAINT chk_product_dependencies_relation_type CHECK(relation_type IN ('required','recommended','compatible','consumes','alternative','included')); END IF; END $$`,
		`CREATE TABLE IF NOT EXISTS device_components(device_id VARCHAR(255) NOT NULL REFERENCES devices(deviceID) ON DELETE CASCADE, component_device_id VARCHAR(255) NOT NULL UNIQUE REFERENCES devices(deviceID) ON DELETE RESTRICT, relation_type VARCHAR(24) NOT NULL DEFAULT 'included' CHECK(relation_type IN ('required','included','assigned')), notes TEXT, created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP, PRIMARY KEY(device_id,component_device_id), CHECK(device_id<>component_device_id))`,
		`CREATE TABLE IF NOT EXISTS case_models(model_id BIGSERIAL PRIMARY KEY, name VARCHAR(255) NOT NULL, description TEXT, created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP, updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS uq_case_models_name_normalized ON case_models(LOWER(TRIM(name)))`,
		`ALTER TABLE cases ADD COLUMN IF NOT EXISTS case_model_id BIGINT REFERENCES case_models(model_id) ON DELETE SET NULL`,
		`CREATE TABLE IF NOT EXISTS inventory_identifiers(identifier_id BIGSERIAL PRIMARY KEY, entity_type VARCHAR(20) NOT NULL CHECK(entity_type IN ('product','device','case')), entity_key VARCHAR(255) NOT NULL, code VARCHAR(255) NOT NULL, identifier_kind VARCHAR(24) NOT NULL DEFAULT 'canonical', active BOOLEAN NOT NULL DEFAULT TRUE, created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP, UNIQUE(entity_type,entity_key,identifier_kind))`,
		`CREATE UNIQUE INDEX IF NOT EXISTS uq_inventory_identifiers_code_normalized ON inventory_identifiers(LOWER(TRIM(code))) WHERE active`,
		`INSERT INTO inventory_identifiers(entity_type,entity_key,code,identifier_kind) SELECT 'product',productID::text,generic_barcode,'canonical' FROM products WHERE NULLIF(TRIM(generic_barcode),'') IS NOT NULL ON CONFLICT(entity_type,entity_key,identifier_kind) DO UPDATE SET code=EXCLUDED.code,active=TRUE`,
		`INSERT INTO inventory_identifiers(entity_type,entity_key,code,identifier_kind) SELECT 'product',productID::text,product_code,'product_code' FROM products WHERE LOWER(TRIM(product_code))<>LOWER(TRIM(generic_barcode)) ON CONFLICT(entity_type,entity_key,identifier_kind) DO UPDATE SET code=EXCLUDED.code,active=TRUE`,
		`INSERT INTO inventory_identifiers(entity_type,entity_key,code,identifier_kind) SELECT 'device',deviceID,barcode,'canonical' FROM devices WHERE NULLIF(TRIM(barcode),'') IS NOT NULL ON CONFLICT(entity_type,entity_key,identifier_kind) DO UPDATE SET code=EXCLUDED.code,active=TRUE`,
		`INSERT INTO inventory_identifiers(entity_type,entity_key,code,identifier_kind) SELECT 'case',caseID::text,barcode,'canonical' FROM cases WHERE NULLIF(TRIM(barcode),'') IS NOT NULL ON CONFLICT(entity_type,entity_key,identifier_kind) DO UPDATE SET code=EXCLUDED.code,active=TRUE`,
		`INSERT INTO inventory_identifiers(entity_type,entity_key,code,identifier_kind) SELECT 'device',deviceID,deviceID,'legacy_id' FROM devices WHERE LOWER(TRIM(deviceID))<>LOWER(TRIM(barcode)) ON CONFLICT(entity_type,entity_key,identifier_kind) DO UPDATE SET code=EXCLUDED.code,active=TRUE`,
		`CREATE OR REPLACE FUNCTION generate_product_master_code() RETURNS TRIGGER AS $$ BEGIN IF NEW.product_code IS NULL OR TRIM(NEW.product_code)='' THEN NEW.product_code := 'PRD-' || LPAD(nextval('product_master_code_seq')::text,8,'0'); END IF; IF NEW.generic_barcode IS NULL OR TRIM(NEW.generic_barcode)='' THEN NEW.generic_barcode := NEW.product_code; END IF; RETURN NEW; END; $$ LANGUAGE plpgsql`,
		`DROP TRIGGER IF EXISTS products_master_code_before_insert ON products`,
		`CREATE TRIGGER products_master_code_before_insert BEFORE INSERT ON products FOR EACH ROW EXECUTE FUNCTION generate_product_master_code()`,
		`CREATE OR REPLACE FUNCTION generate_device_id() RETURNS TRIGGER AS $$ BEGIN IF NEW.deviceID IS NULL OR TRIM(NEW.deviceID)='' THEN NEW.deviceID := 'DEV-' || LPAD(nextval('device_master_code_seq')::text,8,'0'); END IF; IF NEW.barcode IS NULL OR TRIM(NEW.barcode)='' THEN NEW.barcode := NEW.deviceID; END IF; IF NEW.qr_code IS NULL OR TRIM(NEW.qr_code)='' THEN NEW.qr_code := 'WH:' || NEW.deviceID; END IF; RETURN NEW; END; $$ LANGUAGE plpgsql`,
		`DROP TRIGGER IF EXISTS devices_before_insert ON devices`,
		`CREATE TRIGGER devices_before_insert BEFORE INSERT ON devices FOR EACH ROW EXECUTE FUNCTION generate_device_id()`,
		`CREATE OR REPLACE FUNCTION generate_case_barcode() RETURNS TRIGGER AS $$ BEGIN IF NEW.barcode IS NULL OR TRIM(NEW.barcode)='' THEN NEW.barcode := 'CAS-' || LPAD(nextval('case_master_code_seq')::text,8,'0'); END IF; RETURN NEW; END; $$ LANGUAGE plpgsql`,
		`DROP TRIGGER IF EXISTS cases_master_code_before_insert ON cases`,
		`CREATE TRIGGER cases_master_code_before_insert BEFORE INSERT ON cases FOR EACH ROW EXECUTE FUNCTION generate_case_barcode()`,
		`CREATE OR REPLACE FUNCTION sync_product_identifier() RETURNS TRIGGER AS $$ BEGIN INSERT INTO inventory_identifiers(entity_type,entity_key,code,identifier_kind,active) VALUES('product',NEW.productID::text,NEW.generic_barcode,'canonical',TRUE) ON CONFLICT(entity_type,entity_key,identifier_kind) DO UPDATE SET code=EXCLUDED.code,active=TRUE; RETURN NEW; END; $$ LANGUAGE plpgsql`,
		`DROP TRIGGER IF EXISTS products_identifier_after_write ON products`,
		`CREATE TRIGGER products_identifier_after_write AFTER INSERT OR UPDATE OF generic_barcode ON products FOR EACH ROW EXECUTE FUNCTION sync_product_identifier()`,
		`CREATE OR REPLACE FUNCTION sync_device_identifier() RETURNS TRIGGER AS $$ BEGIN INSERT INTO inventory_identifiers(entity_type,entity_key,code,identifier_kind,active) VALUES('device',NEW.deviceID,NEW.barcode,'canonical',TRUE) ON CONFLICT(entity_type,entity_key,identifier_kind) DO UPDATE SET code=EXCLUDED.code,active=TRUE; RETURN NEW; END; $$ LANGUAGE plpgsql`,
		`DROP TRIGGER IF EXISTS devices_identifier_after_write ON devices`,
		`CREATE TRIGGER devices_identifier_after_write AFTER INSERT OR UPDATE OF barcode ON devices FOR EACH ROW EXECUTE FUNCTION sync_device_identifier()`,
		`CREATE OR REPLACE FUNCTION sync_case_identifier() RETURNS TRIGGER AS $$ BEGIN INSERT INTO inventory_identifiers(entity_type,entity_key,code,identifier_kind,active) VALUES('case',NEW.caseID::text,NEW.barcode,'canonical',TRUE) ON CONFLICT(entity_type,entity_key,identifier_kind) DO UPDATE SET code=EXCLUDED.code,active=TRUE; RETURN NEW; END; $$ LANGUAGE plpgsql`,
		`DROP TRIGGER IF EXISTS cases_identifier_after_write ON cases`,
		`CREATE TRIGGER cases_identifier_after_write AFTER INSERT OR UPDATE OF barcode ON cases FOR EACH ROW EXECUTE FUNCTION sync_case_identifier()`,
		`INSERT INTO case_models(name) SELECT DISTINCT TRIM(name) FROM cases WHERE NULLIF(TRIM(name),'') IS NOT NULL ON CONFLICT DO NOTHING`,
		`UPDATE cases c SET case_model_id=cm.model_id FROM case_models cm WHERE c.case_model_id IS NULL AND LOWER(TRIM(cm.name))=LOWER(TRIM(c.name))`,
		`UPDATE cases c SET case_type='fixed' WHERE c.case_type='dynamic' AND EXISTS(SELECT 1 FROM devicescases dc WHERE dc.caseID=c.caseID)`,
		`INSERT INTO case_content_templates(case_id,product_id,expected_quantity) SELECT dc.caseID,d.productID,COUNT(*) FROM devicescases dc JOIN devices d ON d.deviceID=dc.deviceID WHERE d.productID IS NOT NULL GROUP BY dc.caseID,d.productID ON CONFLICT(case_id,product_id) DO NOTHING`,
		`UPDATE cases c SET workflow_status='complete' WHERE c.workflow_status='empty' AND c.case_type='fixed' AND EXISTS(SELECT 1 FROM devicescases dc WHERE dc.caseID=c.caseID)`,
		`UPDATE categories SET name='Ton',abbreviation='TON' WHERE LOWER(TRIM(name))='sound'`,
		`UPDATE categories SET name='Licht',abbreviation='LIC' WHERE LOWER(TRIM(name))='light'`,
		`UPDATE categories SET name='Bühne',abbreviation='BUE' WHERE LOWER(TRIM(name))='stage'`,
		`UPDATE categories SET name='Effekte',abbreviation='EFF' WHERE LOWER(TRIM(name))='effect'`,
		`UPDATE categories SET name='IT & Steuerung',abbreviation='ITS' WHERE LOWER(TRIM(name))='assets'`,
		`UPDATE categories SET name='Sonstiges',abbreviation='SON' WHERE LOWER(TRIM(name))='other' AND NOT EXISTS(SELECT 1 FROM categories x WHERE LOWER(TRIM(x.name))='sonstiges')`,
		`DELETE FROM categories c WHERE LOWER(TRIM(c.name))='sonstiges' AND NOT EXISTS(SELECT 1 FROM subcategories s WHERE s.categoryID=c.categoryID) AND EXISTS(SELECT 1 FROM categories x WHERE LOWER(TRIM(x.name))='other')`,
		`UPDATE categories SET name='Sonstiges',abbreviation='SON' WHERE LOWER(TRIM(name))='other'`,
		`DO $$ DECLARE cable_category_id INT; BEGIN INSERT INTO categories(name,abbreviation) VALUES('Kabel & Adapter','KAB') ON CONFLICT DO NOTHING; SELECT categoryID INTO cable_category_id FROM categories WHERE LOWER(TRIM(name))='kabel & adapter' LIMIT 1; INSERT INTO subcategories(subcategoryID,name,abbreviation,categoryID) VALUES ('KAB-AUDIO','Audio','AUD',cable_category_id),('KAB-POWER','Strom','PWR',cable_category_id),('KAB-DATA','Daten','DAT',cable_category_id),('KAB-COMBI','Kombikabel','KOM',cable_category_id) ON CONFLICT(subcategoryID) DO NOTHING; UPDATE products p SET categoryID=cable_category_id,subcategoryID=CASE WHEN LOWER(ct.name) LIKE '%audio%' THEN 'KAB-AUDIO' WHEN LOWER(ct.name) LIKE '%strom%' THEN 'KAB-POWER' WHEN LOWER(ct.name) LIKE '%kombi%' THEN 'KAB-COMBI' ELSE 'KAB-DATA' END FROM cable_products cp JOIN cable_types ct ON ct.cable_typesID=cp.cable_type_id WHERE p.productID=cp.product_id; END $$`,
		`INSERT INTO warehouse_schema_migrations(version) VALUES('043_product_master_v2') ON CONFLICT(version) DO NOTHING`,
	}

	for _, statement := range statements {
		if _, err := tx.Exec(statement); err != nil {
			return fmt.Errorf("apply product master schema: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit product master schema: %w", err)
	}
	return nil
}
