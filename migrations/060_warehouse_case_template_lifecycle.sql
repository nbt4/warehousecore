-- Retain case template identities and guard every native writer.
ALTER TABLE case_content_templates ADD COLUMN IF NOT EXISTS lifecycle_status VARCHAR(20) NOT NULL DEFAULT 'active';
ALTER TABLE case_content_templates ADD COLUMN IF NOT EXISTS archived_at TIMESTAMP;
ALTER TABLE case_content_templates ADD COLUMN IF NOT EXISTS updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP;
DO $$ BEGIN IF NOT EXISTS(SELECT 1 FROM pg_constraint WHERE conrelid='case_content_templates'::regclass AND conname='case_templates_lifecycle_check') THEN
 ALTER TABLE case_content_templates ADD CONSTRAINT case_templates_lifecycle_check CHECK(lifecycle_status IN ('active','archived'));
END IF; END $$;
CREATE OR REPLACE FUNCTION guard_warehouse_case_template() RETURNS TRIGGER AS $$
DECLARE c RECORD; p RECORD;
BEGIN
 IF TG_OP='DELETE' THEN RAISE EXCEPTION 'Archive case template lines; retained identity cannot be deleted'; END IF;
 IF TG_OP='UPDATE' AND (NEW.template_line_id<>OLD.template_line_id OR NEW.case_id<>OLD.case_id OR NEW.product_id<>OLD.product_id OR NEW.created_at IS DISTINCT FROM OLD.created_at) THEN
  RAISE EXCEPTION 'Case template identity is immutable';
 END IF;
 SELECT lifecycle_status,workflow_status,current_job_id,sealed_at INTO c FROM cases WHERE caseid=NEW.case_id FOR UPDATE;
 IF NOT FOUND OR c.lifecycle_status<>'active' OR c.current_job_id IS NOT NULL OR c.sealed_at IS NOT NULL OR c.workflow_status NOT IN ('empty','packing','return_check','complete') THEN
  RAISE EXCEPTION 'Active open case required before changing its template';
 END IF;
 IF EXISTS(SELECT 1 FROM case_child_contents WHERE child_case_id=NEW.case_id) THEN RAISE EXCEPTION 'Unnest case before editing its template'; END IF;
 IF TG_OP='UPDATE' AND OLD.lifecycle_status='archived' AND NEW.lifecycle_status='archived' THEN RAISE EXCEPTION 'Restore template line before editing'; END IF;
 IF TG_OP='INSERT' AND NEW.lifecycle_status<>'active' THEN RAISE EXCEPTION 'New template line must be active'; END IF;
 IF NEW.lifecycle_status='active' THEN
  SELECT lifecycle_status,tracking_mode INTO p FROM products WHERE productid=NEW.product_id FOR SHARE;
  IF NOT FOUND OR p.lifecycle_status<>'active' OR p.tracking_mode NOT IN ('individual','quantity') THEN RAISE EXCEPTION 'Active physical product required for a case template'; END IF;
  IF p.tracking_mode='individual' AND NEW.expected_quantity<>trunc(NEW.expected_quantity) THEN RAISE EXCEPTION 'Serialized product requires whole expected quantity'; END IF;
 END IF;
 IF TG_OP='UPDATE' THEN
  NEW.updated_at:=GREATEST(clock_timestamp() AT TIME ZONE 'UTC',OLD.updated_at+INTERVAL '1 microsecond');
  IF NEW.lifecycle_status='archived' THEN NEW.archived_at:=clock_timestamp() AT TIME ZONE 'UTC'; ELSE NEW.archived_at:=NULL; END IF;
 END IF;
 RETURN NEW;
END;
$$ LANGUAGE plpgsql;
DROP TRIGGER IF EXISTS case_templates_guard_lifecycle ON case_content_templates;
CREATE TRIGGER case_templates_guard_lifecycle BEFORE INSERT OR UPDATE OR DELETE ON case_content_templates FOR EACH ROW EXECUTE FUNCTION guard_warehouse_case_template();
INSERT INTO warehouse_schema_migrations(version) VALUES('060_warehouse_case_template_lifecycle') ON CONFLICT DO NOTHING;
