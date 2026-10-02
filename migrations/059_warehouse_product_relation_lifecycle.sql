ALTER TABLE product_dependencies ADD COLUMN IF NOT EXISTS lifecycle_status VARCHAR(20) NOT NULL DEFAULT 'active' CHECK(lifecycle_status IN ('active','archived'));
ALTER TABLE product_dependencies ADD COLUMN IF NOT EXISTS archived_at TIMESTAMP;
CREATE OR REPLACE FUNCTION warehouse_relation_active_jobs(pid INT) RETURNS JSONB AS $$
 WITH RECURSIVE ancestors AS (
  SELECT pid AS product_id,ARRAY[pid] AS path,0 AS depth
  UNION ALL
  SELECT pd.product_id,a.path||pd.product_id,a.depth+1 FROM ancestors a JOIN product_dependencies pd ON pd.dependency_product_id=a.product_id
  WHERE pd.lifecycle_status='active' AND a.depth<5 AND NOT pd.product_id=ANY(a.path)
 ), affected AS (
  SELECT DISTINCT j.jobid,s.status,j.updated_at,j.startdate,j.enddate FROM jobs j LEFT JOIN status s ON s.statusid=j.statusid
  WHERE j.deleted_at IS NULL AND NOT warehouse_job_status_is_closed(COALESCE(s.status,'')) AND (
   EXISTS(SELECT 1 FROM job_product_requirements r WHERE r.job_id=j.jobid AND r.product_id IN (SELECT product_id FROM ancestors)) OR
   EXISTS(SELECT 1 FROM job_positions p WHERE p.job_id=j.jobid AND p.product_id IN (SELECT product_id FROM ancestors)) OR
   EXISTS(SELECT 1 FROM job_devices jd JOIN devices d ON d.deviceid=jd.deviceid WHERE jd.jobid=j.jobid AND d.productid IN (SELECT product_id FROM ancestors)))
  ORDER BY j.jobid LIMIT 1001
 ) SELECT COALESCE(jsonb_agg(jsonb_build_object('job_id',jobid,'status',status,'updated_at',updated_at,'start_date',startdate,'end_date',enddate) ORDER BY jobid),'[]'::jsonb) FROM affected;
$$ LANGUAGE SQL STABLE;
CREATE OR REPLACE FUNCTION guard_warehouse_product_relation() RETURNS TRIGGER AS $$ BEGIN
 IF TG_OP='DELETE' THEN RAISE EXCEPTION 'Archive product relationships to retain identity and history' USING ERRCODE='23514'; END IF;
 IF TG_OP='UPDATE' THEN
  IF (NEW.product_id,NEW.dependency_product_id,NEW.id) IS DISTINCT FROM (OLD.product_id,OLD.dependency_product_id,OLD.id) THEN RAISE EXCEPTION 'Relation identity is immutable' USING ERRCODE='23514'; END IF;
  IF OLD.lifecycle_status='archived' AND (NEW.lifecycle_status<>'active' OR (to_jsonb(NEW)-'updated_at'-'lifecycle_status'-'archived_at') IS DISTINCT FROM (to_jsonb(OLD)-'updated_at'-'lifecycle_status'-'archived_at')) THEN RAISE EXCEPTION 'Restore relationship before editing; restoration preserves fields' USING ERRCODE='23514'; END IF;
  NEW.updated_at:=GREATEST(clock_timestamp() AT TIME ZONE 'UTC',OLD.updated_at+INTERVAL '1 microsecond');
 ELSE NEW.updated_at:=clock_timestamp() AT TIME ZONE 'UTC'; END IF;
 IF jsonb_array_length(warehouse_relation_active_jobs(NEW.product_id))>0 THEN RAISE EXCEPTION 'Active jobs using this product or an ancestor block relation changes' USING ERRCODE='23514'; END IF;
 IF NEW.lifecycle_status='active' THEN
  PERFORM 1 FROM products WHERE productid=NEW.product_id AND lifecycle_status='active' FOR SHARE;
  IF NOT FOUND THEN RAISE EXCEPTION 'Active source product required' USING ERRCODE='23514'; END IF;
  PERFORM 1 FROM products WHERE productid=NEW.dependency_product_id AND lifecycle_status='active' FOR SHARE;
  IF NOT FOUND THEN RAISE EXCEPTION 'Active related product required' USING ERRCODE='23514'; END IF;
  IF NEW.relation_type IN ('required','included','consumes') AND EXISTS(
   WITH RECURSIVE reachable AS (SELECT NEW.dependency_product_id AS product_id UNION SELECT pd.dependency_product_id FROM reachable r JOIN product_dependencies pd ON pd.product_id=r.product_id WHERE pd.lifecycle_status='active' AND pd.relation_type IN ('required','included','consumes') AND pd.id<>NEW.id)
   SELECT 1 FROM reachable WHERE product_id=NEW.product_id) THEN RAISE EXCEPTION 'Mandatory dependency cycle rejected' USING ERRCODE='23514'; END IF;
  IF NEW.product_id=NEW.dependency_product_id OR NEW.default_quantity<=0 OR NEW.default_quantity>99999999.99 OR NEW.relation_type NOT IN ('required','recommended','compatible','consumes','alternative','included') OR NEW.assignment_scope NOT IN ('product','device','case') THEN RAISE EXCEPTION 'Invalid typed relationship' USING ERRCODE='23514'; END IF;
 END IF;
 NEW.archived_at:=CASE WHEN NEW.lifecycle_status='archived' THEN CASE WHEN TG_OP='UPDATE' THEN COALESCE(OLD.archived_at,clock_timestamp() AT TIME ZONE 'UTC') ELSE clock_timestamp() AT TIME ZONE 'UTC' END ELSE NULL END;
 RETURN NEW;
END; $$ LANGUAGE plpgsql;
DROP TRIGGER IF EXISTS product_dependencies_guard_lifecycle ON product_dependencies;
CREATE TRIGGER product_dependencies_guard_lifecycle BEFORE INSERT OR UPDATE OR DELETE ON product_dependencies FOR EACH ROW EXECUTE FUNCTION guard_warehouse_product_relation();
CREATE OR REPLACE FUNCTION touch_warehouse_relation_products() RETURNS TRIGGER AS $$ BEGIN
 IF TG_OP<>'INSERT' THEN UPDATE products SET updated_at=GREATEST(clock_timestamp() AT TIME ZONE 'UTC',updated_at+INTERVAL '1 microsecond') WHERE productid IN (OLD.product_id,OLD.dependency_product_id); END IF;
 IF TG_OP='INSERT' THEN UPDATE products SET updated_at=GREATEST(clock_timestamp() AT TIME ZONE 'UTC',updated_at+INTERVAL '1 microsecond') WHERE productid IN (NEW.product_id,NEW.dependency_product_id); END IF;
 RETURN NULL;
END; $$ LANGUAGE plpgsql;
DROP TRIGGER IF EXISTS product_dependencies_touch_products ON product_dependencies;
CREATE TRIGGER product_dependencies_touch_products AFTER INSERT OR UPDATE OR DELETE ON product_dependencies FOR EACH ROW EXECUTE FUNCTION touch_warehouse_relation_products();
CREATE OR REPLACE FUNCTION touch_warehouse_product_version() RETURNS TRIGGER AS $$ BEGIN
 NEW.updated_at:=GREATEST(clock_timestamp() AT TIME ZONE 'UTC',OLD.updated_at+INTERVAL '1 microsecond');RETURN NEW;
END; $$ LANGUAGE plpgsql;
DROP TRIGGER IF EXISTS products_touch_version ON products;
CREATE TRIGGER products_touch_version BEFORE UPDATE ON products FOR EACH ROW EXECUTE FUNCTION touch_warehouse_product_version();
CREATE OR REPLACE FUNCTION guard_warehouse_product_relation_jobs() RETURNS TRIGGER AS $$ BEGIN
 IF OLD.lifecycle_status='active' AND NEW.lifecycle_status='archived' AND jsonb_array_length(warehouse_relation_active_jobs(OLD.productid))>0 THEN RAISE EXCEPTION 'Active jobs using this product or a dependency ancestor block product archive' USING ERRCODE='23514'; END IF;RETURN NEW;
END; $$ LANGUAGE plpgsql;
DROP TRIGGER IF EXISTS products_guard_relation_jobs ON products;
CREATE TRIGGER products_guard_relation_jobs BEFORE UPDATE OF lifecycle_status ON products FOR EACH ROW EXECUTE FUNCTION guard_warehouse_product_relation_jobs();
INSERT INTO warehouse_schema_migrations(version) VALUES('059_warehouse_product_relation_lifecycle') ON CONFLICT DO NOTHING;
