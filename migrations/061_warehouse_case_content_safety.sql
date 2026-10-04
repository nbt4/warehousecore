-- Physical case contents must respect all sealed ancestors and stock totals.
CREATE OR REPLACE FUNCTION guard_warehouse_case_content() RETURNS TRIGGER AS $$
DECLARE parent_id INT; child_id INT; dev_id TEXT; product_id_value INT; c RECORD; d RECORD;
BEGIN
 IF TG_TABLE_NAME='devicescases' THEN
  IF TG_OP='DELETE' THEN parent_id:=OLD.caseid; ELSE parent_id:=NEW.caseid;dev_id:=NEW.deviceid; END IF;
  IF TG_OP='UPDATE' AND (NEW.caseid<>OLD.caseid OR NEW.deviceid<>OLD.deviceid) THEN RAISE EXCEPTION 'Unpack before changing physical membership identity'; END IF;
 ELSIF TG_TABLE_NAME='case_child_contents' THEN
  IF TG_OP='DELETE' THEN parent_id:=OLD.parent_case_id;child_id:=OLD.child_case_id; ELSE parent_id:=NEW.parent_case_id;child_id:=NEW.child_case_id; END IF;
  IF TG_OP='UPDATE' AND (NEW.parent_case_id<>OLD.parent_case_id OR NEW.child_case_id<>OLD.child_case_id) THEN RAISE EXCEPTION 'Unpack before changing nested case identity'; END IF;
 ELSE
  IF TG_OP='DELETE' THEN parent_id:=OLD.case_id; ELSE parent_id:=NEW.case_id;product_id_value:=NEW.product_id; END IF;
  IF TG_OP='UPDATE' AND (NEW.case_id<>OLD.case_id OR NEW.product_id<>OLD.product_id) THEN RAISE EXCEPTION 'Unpack before changing quantity membership identity'; END IF;
 END IF;
 FOR c IN WITH RECURSIVE ancestors(case_id) AS(SELECT parent_id UNION SELECT cc.parent_case_id FROM case_child_contents cc JOIN ancestors a ON a.case_id=cc.child_case_id) SELECT x.* FROM cases x JOIN ancestors a ON a.case_id=x.caseid ORDER BY x.caseid FOR UPDATE OF x LOOP
  IF c.lifecycle_status<>'active' OR c.status<>'free' OR c.current_job_id IS NOT NULL OR c.sealed_at IS NOT NULL OR c.workflow_status NOT IN ('empty','packing','complete','return_check') THEN RAISE EXCEPTION 'Active open case and ancestors required for physical content changes'; END IF;
 END LOOP;
 IF TG_OP<>'DELETE' AND dev_id IS NOT NULL THEN
  SELECT d1.*,p.lifecycle_status AS product_lifecycle INTO d FROM devices d1 JOIN products p ON p.productid=d1.productid WHERE d1.deviceid=dev_id FOR UPDATE OF d1;
  IF NOT FOUND OR d.lifecycle_status<>'active' OR d.product_lifecycle<>'active' OR d.condition_status<>'available' OR d.status NOT IN ('in_storage','location_unknown') THEN RAISE EXCEPTION 'Available physical device required'; END IF;
  IF EXISTS(SELECT 1 FROM devicescases WHERE deviceid=dev_id AND caseid<>parent_id) OR (d.current_case_id IS NOT NULL AND d.current_case_id<>parent_id) THEN RAISE EXCEPTION 'Unpack device from its existing case first'; END IF;
 END IF;
 IF TG_OP<>'DELETE' AND product_id_value IS NOT NULL THEN
  IF NOT EXISTS(SELECT 1 FROM products WHERE productid=product_id_value AND lifecycle_status='active' AND tracking_mode='quantity') THEN RAISE EXCEPTION 'Active quantity product required'; END IF;
  IF TG_OP='UPDATE' THEN NEW.updated_at:=GREATEST(clock_timestamp() AT TIME ZONE 'UTC',OLD.updated_at+INTERVAL '1 microsecond'); END IF;
 END IF;
 IF TG_TABLE_NAME='case_child_contents' THEN
  IF EXISTS(WITH RECURSIVE tree(case_id) AS(SELECT child_id UNION SELECT cc.child_case_id FROM case_child_contents cc JOIN tree t ON t.case_id=cc.parent_case_id) SELECT 1 FROM tree WHERE case_id=parent_id) THEN RAISE EXCEPTION 'Nested case cycle forbidden'; END IF;
  IF EXISTS(WITH RECURSIVE tree(case_id) AS(SELECT child_id UNION SELECT cc.child_case_id FROM case_child_contents cc JOIN tree t ON t.case_id=cc.parent_case_id) SELECT 1 FROM tree JOIN cases x ON x.caseid=tree.case_id WHERE x.lifecycle_status<>'active' OR x.status<>'free' OR x.current_job_id IS NOT NULL OR x.workflow_status IN('on_job','maintenance')) THEN RAISE EXCEPTION 'Available nested case tree required'; END IF;
 END IF;
 IF TG_OP='DELETE' THEN RETURN OLD; END IF;
 RETURN NEW;
END;
$$ LANGUAGE plpgsql;
DROP TRIGGER IF EXISTS devicescases_guard_physical ON devicescases;
CREATE TRIGGER devicescases_guard_physical BEFORE INSERT OR UPDATE OR DELETE ON devicescases FOR EACH ROW EXECUTE FUNCTION guard_warehouse_case_content();
DROP TRIGGER IF EXISTS case_product_contents_guard_physical ON case_product_contents;
CREATE TRIGGER case_product_contents_guard_physical BEFORE INSERT OR UPDATE OR DELETE ON case_product_contents FOR EACH ROW EXECUTE FUNCTION guard_warehouse_case_content();
DROP TRIGGER IF EXISTS case_child_contents_guard_physical ON case_child_contents;
CREATE TRIGGER case_child_contents_guard_physical BEFORE INSERT OR UPDATE OR DELETE ON case_child_contents FOR EACH ROW EXECUTE FUNCTION guard_warehouse_case_content();
CREATE OR REPLACE FUNCTION sync_warehouse_device_case_membership() RETURNS TRIGGER AS $$
BEGIN
 IF TG_OP='DELETE' THEN UPDATE devices SET current_case_id=NULL WHERE deviceid=OLD.deviceid AND current_case_id=OLD.caseid; RETURN OLD; END IF;
 UPDATE devices SET current_case_id=NEW.caseid,zone_id=NULL,status='in_storage',current_location='case:'||NEW.caseid::text WHERE deviceid=NEW.deviceid;
 RETURN NEW;
END;
$$ LANGUAGE plpgsql;
DROP TRIGGER IF EXISTS devicescases_sync_membership ON devicescases;
CREATE TRIGGER devicescases_sync_membership AFTER INSERT OR DELETE ON devicescases FOR EACH ROW EXECUTE FUNCTION sync_warehouse_device_case_membership();
CREATE OR REPLACE FUNCTION sync_product_stock_from_locations() RETURNS TRIGGER AS $$
DECLARE affected INT; old_product INT;
BEGIN
 IF TG_OP='DELETE' THEN affected:=OLD.product_id; ELSE affected:=NEW.product_id; END IF;
 IF TG_OP='UPDATE' THEN old_product:=OLD.product_id; END IF;
 UPDATE products p SET stock_quantity=COALESCE((SELECT sum(quantity) FROM product_locations WHERE product_id=p.productid),0)+COALESCE((SELECT sum(quantity) FROM case_product_contents WHERE product_id=p.productid),0),updated_at=CURRENT_TIMESTAMP
 WHERE p.productid IN(affected,old_product) AND p.tracking_mode='quantity';
 IF TG_OP='DELETE' THEN RETURN OLD; END IF;
 RETURN NEW;
END;
$$ LANGUAGE plpgsql;
DROP TRIGGER IF EXISTS case_product_contents_sync_stock ON case_product_contents;
CREATE TRIGGER case_product_contents_sync_stock AFTER INSERT OR UPDATE OR DELETE ON case_product_contents FOR EACH ROW EXECUTE FUNCTION sync_product_stock_from_locations();
UPDATE products p SET stock_quantity=COALESCE((SELECT sum(quantity) FROM product_locations WHERE product_id=p.productid),0)+COALESCE((SELECT sum(quantity) FROM case_product_contents WHERE product_id=p.productid),0)
WHERE p.tracking_mode='quantity' AND p.stock_quantity IS DISTINCT FROM COALESCE((SELECT sum(quantity) FROM product_locations WHERE product_id=p.productid),0)+COALESCE((SELECT sum(quantity) FROM case_product_contents WHERE product_id=p.productid),0);
INSERT INTO warehouse_schema_migrations(version) VALUES('061_warehouse_case_content_safety') ON CONFLICT DO NOTHING;
