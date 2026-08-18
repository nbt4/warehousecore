package handlers

import (
	"fmt"

	"warehousecore/internal/repository"
)

// EnsureDeviceStatusSchema keeps physical device availability separate from
// job completion. Closed jobs mark issued devices as awaiting return until an
// intake scan confirms their warehouse location.
func EnsureDeviceStatusSchema() error {
	db := repository.GetSQLDB()
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin device status schema transaction: %w", err)
	}
	defer tx.Rollback()

	statements := []string{
		`CREATE INDEX IF NOT EXISTS idx_job_devices_issued_device
		 ON job_devices(deviceid, jobid) WHERE pack_status = 'issued'`,
		`CREATE OR REPLACE FUNCTION warehouse_job_status_is_closed(job_status TEXT)
		 RETURNS BOOLEAN AS $$
			 SELECT LOWER(TRIM(COALESCE(job_status, ''))) IN (
				 'abgeschlossen', 'abgerechnet', 'storniert',
				 'completed', 'paid', 'canceled', 'cancelled'
			 );
		 $$ LANGUAGE SQL IMMUTABLE`,
		`UPDATE devices d
		 SET status = 'return_pending', updated_at = CURRENT_TIMESTAMP
		 WHERE d.status IN ('on_job', 'rented')
		   AND EXISTS (
			 SELECT 1 FROM job_devices jd
			 JOIN jobs j ON j.jobid = jd.jobid
			 JOIN status s ON s.statusid = j.statusid
			 WHERE jd.deviceid = d.deviceid AND jd.pack_status = 'issued'
			   AND warehouse_job_status_is_closed(s.status)
		   )
		   AND NOT EXISTS (
			 SELECT 1 FROM job_devices jd
			 JOIN jobs j ON j.jobid = jd.jobid
			 JOIN status s ON s.statusid = j.statusid
			 WHERE jd.deviceid = d.deviceid AND jd.pack_status = 'issued'
			   AND NOT warehouse_job_status_is_closed(s.status)
		   )`,
		`UPDATE devices d
		 SET status = CASE
			 WHEN d.zone_id IS NOT NULL OR LOWER(COALESCE(d.current_location, '')) = 'warehouse'
				 THEN 'in_storage'
			 ELSE 'location_unknown'
		 END, updated_at = CURRENT_TIMESTAMP
		 WHERE d.status IN ('on_job', 'rented')
		   AND NOT EXISTS (
			 SELECT 1 FROM job_devices jd
			 WHERE jd.deviceid = d.deviceid AND jd.pack_status = 'issued'
		   )`,
		`CREATE OR REPLACE FUNCTION sync_devices_after_job_status_change()
		 RETURNS TRIGGER AS $$
		 DECLARE new_status_name TEXT;
		 BEGIN
			 SELECT status INTO new_status_name FROM status WHERE statusid = NEW.statusid;
			 IF warehouse_job_status_is_closed(new_status_name) THEN
				 UPDATE devices d
				 SET status = 'return_pending', updated_at = CURRENT_TIMESTAMP
				 FROM job_devices jd
				 WHERE jd.jobid = NEW.jobid AND jd.deviceid = d.deviceid
				   AND jd.pack_status = 'issued' AND d.status IN ('on_job', 'rented')
				   AND NOT EXISTS (
					 SELECT 1 FROM job_devices other_jd
					 JOIN jobs other_job ON other_job.jobid = other_jd.jobid
					 JOIN status other_status ON other_status.statusid = other_job.statusid
					 WHERE other_jd.deviceid = d.deviceid AND other_jd.jobid <> NEW.jobid
					   AND other_jd.pack_status = 'issued'
					   AND NOT warehouse_job_status_is_closed(other_status.status)
				   );
			 ELSE
				 UPDATE devices d
				 SET status = 'on_job', updated_at = CURRENT_TIMESTAMP
				 FROM job_devices jd
				 WHERE jd.jobid = NEW.jobid AND jd.deviceid = d.deviceid
				   AND jd.pack_status = 'issued' AND d.status = 'return_pending';
			 END IF;
			 RETURN NEW;
		 END;
		 $$ LANGUAGE plpgsql`,
		`DROP TRIGGER IF EXISTS jobs_sync_device_status ON jobs`,
		`CREATE TRIGGER jobs_sync_device_status
		 AFTER UPDATE OF statusid ON jobs
		 FOR EACH ROW
		 WHEN (OLD.statusid IS DISTINCT FROM NEW.statusid)
		 EXECUTE FUNCTION sync_devices_after_job_status_change()`,
		`INSERT INTO warehouse_schema_migrations (version)
		 VALUES ('038_device_status_lifecycle') ON CONFLICT (version) DO NOTHING`,
	}

	for _, statement := range statements {
		if _, err := tx.Exec(statement); err != nil {
			return fmt.Errorf("apply device status schema: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit device status schema: %w", err)
	}
	return nil
}
