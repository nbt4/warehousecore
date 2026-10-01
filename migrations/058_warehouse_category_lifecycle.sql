ALTER TABLE categories ADD COLUMN IF NOT EXISTS lifecycle_status VARCHAR(20) NOT NULL DEFAULT 'active' CHECK(lifecycle_status IN ('active','archived'));
ALTER TABLE categories ADD COLUMN IF NOT EXISTS archived_at TIMESTAMP;
ALTER TABLE subcategories ADD COLUMN IF NOT EXISTS lifecycle_status VARCHAR(20) NOT NULL DEFAULT 'active' CHECK(lifecycle_status IN ('active','archived'));
ALTER TABLE subcategories ADD COLUMN IF NOT EXISTS archived_at TIMESTAMP;
ALTER TABLE subbiercategories ADD COLUMN IF NOT EXISTS lifecycle_status VARCHAR(20) NOT NULL DEFAULT 'active' CHECK(lifecycle_status IN ('active','archived'));
ALTER TABLE subbiercategories ADD COLUMN IF NOT EXISTS archived_at TIMESTAMP;
CREATE OR REPLACE FUNCTION guard_warehouse_category_lifecycle() RETURNS TRIGGER AS $$
DECLARE active_refs BOOLEAN; any_refs BOOLEAN;
BEGIN
 IF TG_TABLE_NAME='categories' THEN
  SELECT EXISTS(SELECT 1 FROM products WHERE categoryid=OLD.categoryid) OR EXISTS(SELECT 1 FROM subcategories WHERE categoryid=OLD.categoryid) INTO any_refs;
  SELECT EXISTS(SELECT 1 FROM products WHERE categoryid=OLD.categoryid AND lifecycle_status='active') OR EXISTS(SELECT 1 FROM subcategories WHERE categoryid=OLD.categoryid AND lifecycle_status='active') INTO active_refs;
 ELSIF TG_TABLE_NAME='subcategories' THEN
  SELECT EXISTS(SELECT 1 FROM products WHERE subcategoryid=OLD.subcategoryid) OR EXISTS(SELECT 1 FROM subbiercategories WHERE subcategoryid=OLD.subcategoryid) INTO any_refs;
  SELECT EXISTS(SELECT 1 FROM products WHERE subcategoryid=OLD.subcategoryid AND lifecycle_status='active') OR EXISTS(SELECT 1 FROM subbiercategories WHERE subcategoryid=OLD.subcategoryid AND lifecycle_status='active') INTO active_refs;
 ELSE
  SELECT EXISTS(SELECT 1 FROM products WHERE subbiercategoryid=OLD.subbiercategoryid) INTO any_refs;
  SELECT EXISTS(SELECT 1 FROM products WHERE subbiercategoryid=OLD.subbiercategoryid AND lifecycle_status='active') INTO active_refs;
 END IF;
 IF TG_OP='DELETE' THEN
  IF any_refs THEN RAISE EXCEPTION 'Referenced categories must be archived; history cannot be detached' USING ERRCODE='23514'; END IF;
  RETURN OLD;
 END IF;
 IF OLD.lifecycle_status='archived' AND (to_jsonb(NEW)-'updated_at'-'lifecycle_status'-'archived_at') IS DISTINCT FROM (to_jsonb(OLD)-'updated_at'-'lifecycle_status'-'archived_at') THEN
  RAISE EXCEPTION 'Restore archived category before editing; restoration preserves fields' USING ERRCODE='23514';
 END IF;
 IF NEW.lifecycle_status='archived' AND OLD.lifecycle_status='active' AND active_refs THEN
  RAISE EXCEPTION 'Active products or child categories block archive' USING ERRCODE='23514';
 END IF;
 NEW.archived_at:=CASE WHEN NEW.lifecycle_status='archived' THEN COALESCE(OLD.archived_at,clock_timestamp() AT TIME ZONE 'UTC') ELSE NULL END;
 RETURN NEW;
END; $$ LANGUAGE plpgsql;
DROP TRIGGER IF EXISTS categories_guard_lifecycle ON categories;
CREATE TRIGGER categories_guard_lifecycle BEFORE UPDATE OR DELETE ON categories FOR EACH ROW EXECUTE FUNCTION guard_warehouse_category_lifecycle();
DROP TRIGGER IF EXISTS subcategories_guard_lifecycle ON subcategories;
CREATE TRIGGER subcategories_guard_lifecycle BEFORE UPDATE OR DELETE ON subcategories FOR EACH ROW EXECUTE FUNCTION guard_warehouse_category_lifecycle();
DROP TRIGGER IF EXISTS subbiercategories_guard_lifecycle ON subbiercategories;
CREATE TRIGGER subbiercategories_guard_lifecycle BEFORE UPDATE OR DELETE ON subbiercategories FOR EACH ROW EXECUTE FUNCTION guard_warehouse_category_lifecycle();
CREATE OR REPLACE FUNCTION guard_warehouse_category_parent() RETURNS TRIGGER AS $$ BEGIN
 IF NEW.lifecycle_status='active' THEN
  IF TG_TABLE_NAME='subcategories' THEN
   PERFORM 1 FROM categories WHERE categoryid=NEW.categoryid AND lifecycle_status='active' FOR SHARE;
  ELSE
   PERFORM 1 FROM subcategories s JOIN categories c ON c.categoryid=s.categoryid WHERE s.subcategoryid=NEW.subcategoryid AND s.lifecycle_status='active' AND c.lifecycle_status='active' FOR SHARE OF s,c;
  END IF;
  IF NOT FOUND THEN RAISE EXCEPTION 'Active category ancestry required' USING ERRCODE='23514'; END IF;
 END IF;
 RETURN NEW;
END; $$ LANGUAGE plpgsql;
DROP TRIGGER IF EXISTS subcategories_guard_parent ON subcategories;
CREATE TRIGGER subcategories_guard_parent BEFORE INSERT OR UPDATE OF categoryid,lifecycle_status ON subcategories FOR EACH ROW EXECUTE FUNCTION guard_warehouse_category_parent();
DROP TRIGGER IF EXISTS subbiercategories_guard_parent ON subbiercategories;
CREATE TRIGGER subbiercategories_guard_parent BEFORE INSERT OR UPDATE OF subcategoryid,lifecycle_status ON subbiercategories FOR EACH ROW EXECUTE FUNCTION guard_warehouse_category_parent();
CREATE OR REPLACE FUNCTION guard_warehouse_product_category_lifecycle() RETURNS TRIGGER AS $$ BEGIN
 IF NEW.lifecycle_status='active' THEN
  IF NEW.categoryid IS NOT NULL THEN
   PERFORM 1 FROM categories WHERE categoryid=NEW.categoryid AND lifecycle_status='active' FOR SHARE;
   IF NOT FOUND THEN RAISE EXCEPTION 'Active product requires active category' USING ERRCODE='23514'; END IF;
  END IF;
  IF NEW.subcategoryid IS NOT NULL THEN
   PERFORM 1 FROM subcategories s JOIN categories c ON c.categoryid=s.categoryid WHERE s.subcategoryid=NEW.subcategoryid AND s.categoryid=NEW.categoryid AND s.lifecycle_status='active' AND c.lifecycle_status='active' FOR SHARE OF s,c;
   IF NOT FOUND THEN RAISE EXCEPTION 'Active product requires matching active subcategory ancestry' USING ERRCODE='23514'; END IF;
  END IF;
  IF NEW.subbiercategoryid IS NOT NULL THEN
   PERFORM 1 FROM subbiercategories t JOIN subcategories s ON s.subcategoryid=t.subcategoryid JOIN categories c ON c.categoryid=s.categoryid WHERE t.subbiercategoryid=NEW.subbiercategoryid AND t.subcategoryid=NEW.subcategoryid AND s.categoryid=NEW.categoryid AND t.lifecycle_status='active' AND s.lifecycle_status='active' AND c.lifecycle_status='active' FOR SHARE OF t,s,c;
   IF NOT FOUND THEN RAISE EXCEPTION 'Active product requires matching active third-category ancestry' USING ERRCODE='23514'; END IF;
  END IF;
 END IF;
 RETURN NEW;
END; $$ LANGUAGE plpgsql;
DROP TRIGGER IF EXISTS products_guard_category_lifecycle ON products;
CREATE TRIGGER products_guard_category_lifecycle BEFORE INSERT OR UPDATE OF categoryid,subcategoryid,subbiercategoryid,lifecycle_status ON products FOR EACH ROW EXECUTE FUNCTION guard_warehouse_product_category_lifecycle();
INSERT INTO warehouse_schema_migrations(version) VALUES('058_warehouse_category_lifecycle') ON CONFLICT DO NOTHING;
