package services

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
)

const MaterialSnapshotSchema = `CREATE TABLE IF NOT EXISTS warehouse_material_snapshots (
 job_id INTEGER PRIMARY KEY REFERENCES jobs(jobid) ON DELETE CASCADE,
 version BIGINT NOT NULL DEFAULT 1,source_hash TEXT NOT NULL,roots JSONB NOT NULL,graph JSONB NOT NULL,
 items JSONB NOT NULL,warnings JSONB NOT NULL,updated_at TIMESTAMPTZ NOT NULL DEFAULT now());
CREATE TABLE IF NOT EXISTS warehouse_material_scans (
 id BIGSERIAL PRIMARY KEY,receipt TEXT NOT NULL UNIQUE,job_id INTEGER NOT NULL REFERENCES jobs(jobid) ON DELETE CASCADE,
 product_id INTEGER NOT NULL REFERENCES products(productid),device_id VARCHAR(50) REFERENCES devices(deviceid),
 quantity NUMERIC(14,3) NOT NULL CHECK(quantity>0),scanned_by TEXT NOT NULL DEFAULT '',scanned_at TIMESTAMPTZ NOT NULL DEFAULT now());
CREATE UNIQUE INDEX IF NOT EXISTS warehouse_material_device_scan ON warehouse_material_scans(job_id,device_id) WHERE device_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS warehouse_material_job_scan ON warehouse_material_scans(job_id,product_id);`

type MaterialSnapshot struct {
	JobID      int            `json:"job_id"`
	Version    int64          `json:"version"`
	SourceHash string         `json:"source_hash"`
	Roots      []MaterialRoot `json:"roots"`
	Graph      MaterialGraph  `json:"-"`
	MaterialPreview
}

// Job locking serializes source refreshes and scans. Rules remain frozen while
// quantities and assigned cases follow the current booking, without deleting scans.
func LoadMaterialSnapshot(ctx context.Context, db *sql.DB, jobID int) (*MaterialSnapshot, error) {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	result, err := MaterialSnapshotTx(ctx, tx, jobID)
	if err != nil {
		return nil, err
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return result, nil
}
func MaterialSnapshotTx(ctx context.Context, tx *sql.Tx, jobID int) (*MaterialSnapshot, error) {
	var exists int
	if err := tx.QueryRowContext(ctx, `SELECT jobid FROM jobs WHERE jobid=$1 AND deleted_at IS NULL FOR UPDATE`, jobID).Scan(&exists); err != nil {
		return nil, err
	}
	roots, err := loadMaterialRoots(tx, jobID)
	if err != nil {
		return nil, err
	}
	caseCounts, err := materialCaseCounts(tx, jobID)
	if err != nil {
		return nil, err
	}
	rawRoots, _ := json.Marshal(roots)
	rawSource, _ := json.Marshal(struct {
		Roots []MaterialRoot
		Cases map[int]int
	}{roots, caseCounts})
	hash := sha256.Sum256(rawSource)
	fingerprint := hex.EncodeToString(hash[:])
	result := &MaterialSnapshot{JobID: jobID, Roots: roots, Version: 1, SourceHash: fingerprint}
	var previousHash string
	var graphJSON, itemsJSON, warningsJSON []byte
	err = tx.QueryRowContext(ctx, `SELECT version,source_hash,graph,items,warnings FROM warehouse_material_snapshots WHERE job_id=$1`, jobID).Scan(&result.Version, &previousHash, &graphJSON, &itemsJSON, &warningsJSON)
	if err != nil && err != sql.ErrNoRows {
		return nil, err
	}
	if err == nil {
		if err = json.Unmarshal(graphJSON, &result.Graph); err != nil {
			return nil, err
		}
		if previousHash == fingerprint {
			if err = json.Unmarshal(itemsJSON, &result.Items); err != nil {
				return nil, err
			}
			if err = json.Unmarshal(warningsJSON, &result.Warnings); err != nil {
				return nil, err
			}
			return result, nil
		}
		result.Version++
	}
	current, err := LoadMaterialGraph(tx)
	if err != nil {
		return nil, err
	}
	result.Graph = FreezeMaterialGraph(current, roots, result.Graph)
	for _, root := range roots {
		if _, ok := result.Graph.Products[root.ProductID]; !ok {
			return nil, fmt.Errorf("gebuchtes Produkt %d ist nicht mehr verfügbar", root.ProductID)
		}
	}
	result.MaterialPreview, err = ExpandMaterialWithCases(result.Graph, roots, caseCounts)
	if err != nil {
		return nil, err
	}
	graphJSON, _ = json.Marshal(result.Graph)
	itemsJSON, _ = json.Marshal(result.Items)
	warningsJSON, _ = json.Marshal(result.Warnings)
	_, err = tx.ExecContext(ctx, `INSERT INTO warehouse_material_snapshots(job_id,version,source_hash,roots,graph,items,warnings) VALUES($1,$2,$3,$4::jsonb,$5::jsonb,$6::jsonb,$7::jsonb) ON CONFLICT(job_id) DO UPDATE SET version=EXCLUDED.version,source_hash=EXCLUDED.source_hash,roots=EXCLUDED.roots,graph=EXCLUDED.graph,items=EXCLUDED.items,warnings=EXCLUDED.warnings,updated_at=now()`, jobID, result.Version, fingerprint, string(rawRoots), string(graphJSON), string(itemsJSON), string(warningsJSON))
	return result, err
}
func loadMaterialRoots(tx *sql.Tx, jobID int) ([]MaterialRoot, error) {
	rows, err := tx.Query(`WITH positions AS (
 SELECT product_id,SUM(quantity)::numeric quantity,MIN(unit) unit FROM job_positions WHERE job_id=$1 AND position_type='product' AND product_id IS NOT NULL AND COALESCE(to_jsonb(job_positions)->>'deleted_at','')='' GROUP BY product_id
 ), roots AS (
 SELECT p.product_id,p.quantity+COALESCE((to_jsonb(r)->>'manual_quantity')::numeric,0) quantity,p.unit FROM positions p LEFT JOIN job_product_requirements r ON r.job_id=$1 AND r.product_id=p.product_id AND COALESCE(to_jsonb(r)->>'deleted_at','')=''
 UNION ALL SELECT r.product_id,r.quantity::numeric,'Stk' FROM job_product_requirements r WHERE r.job_id=$1 AND COALESCE(to_jsonb(r)->>'deleted_at','')='' AND NOT EXISTS(SELECT 1 FROM positions p WHERE p.product_id=r.product_id)
 ), assigned AS (
 SELECT deviceid AS device_id FROM job_devices WHERE jobid=$1
 UNION SELECT device_id FROM warehouse_material_scans WHERE job_id=$1 AND device_id IS NOT NULL
 UNION SELECT jpd.device_id FROM job_position_devices jpd JOIN job_positions jp ON jp.position_id=jpd.position_id WHERE jp.job_id=$1 AND COALESCE(to_jsonb(jp)->>'deleted_at','')=''
 ) SELECT roots.product_id,roots.quantity::float8,COALESCE(roots.unit,'Stk'),(SELECT COUNT(DISTINCT dc.caseid) FROM assigned a JOIN devices d ON d.deviceid=a.device_id JOIN devicescases dc ON dc.deviceid=d.deviceid WHERE d.productid=roots.product_id) FROM roots WHERE roots.quantity>0 ORDER BY roots.product_id`, jobID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []MaterialRoot{}
	for rows.Next() {
		var root MaterialRoot
		if err = rows.Scan(&root.ProductID, &root.Quantity, &root.Unit, &root.Cases); err != nil {
			return nil, err
		}
		result = append(result, root)
	}
	return result, rows.Err()
}

type MaterialScanDevice struct {
	DeviceID  string `json:"device_id"`
	ScannedAt string `json:"scanned_at"`
	ScannedBy string `json:"scanned_by"`
}
type MaterialScanCount struct {
	Quantity float64
	Devices  []MaterialScanDevice
}

func MaterialScanCounts(tx *sql.Tx, jobID int) (map[int]MaterialScanCount, error) {
	result := map[int]MaterialScanCount{}
	rows, err := tx.Query(`WITH physical AS (
 SELECT s.product_id,s.device_id,s.scanned_at,s.scanned_by FROM warehouse_material_scans s WHERE s.job_id=$1 AND s.device_id IS NOT NULL
 UNION ALL SELECT d.productid,jd.deviceid,(to_jsonb(jd)->>'pack_ts')::timestamptz,'' FROM job_devices jd JOIN devices d ON d.deviceid=jd.deviceid WHERE jd.jobid=$1 AND jd.pack_status IN ('packed','issued')
 UNION ALL SELECT d.productid,jpd.device_id,jpd.scanned_at,jpd.scanned_by FROM job_position_devices jpd JOIN job_positions jp ON jp.position_id=jpd.position_id JOIN devices d ON d.deviceid=jpd.device_id WHERE jp.job_id=$1 AND COALESCE(to_jsonb(jp)->>'deleted_at','')=''
 ), unique_devices AS (SELECT DISTINCT ON(device_id) * FROM physical ORDER BY device_id,scanned_at DESC NULLS LAST)
 SELECT product_id,device_id,COALESCE(scanned_at::text,''),COALESCE(scanned_by,'') FROM unique_devices ORDER BY product_id,device_id`, jobID)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var product int
		var device MaterialScanDevice
		if err = rows.Scan(&product, &device.DeviceID, &device.ScannedAt, &device.ScannedBy); err != nil {
			rows.Close()
			return nil, err
		}
		count := result[product]
		count.Quantity++
		count.Devices = append(count.Devices, device)
		result[product] = count
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	rows, err = tx.Query(`SELECT product_id,SUM(quantity)::float8 FROM warehouse_material_scans WHERE job_id=$1 AND device_id IS NULL GROUP BY product_id`, jobID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var product int
		var quantity float64
		if err = rows.Scan(&product, &quantity); err != nil {
			return nil, err
		}
		count := result[product]
		count.Quantity += quantity
		result[product] = count
	}
	return result, rows.Err()
}

func materialCaseCounts(tx *sql.Tx, jobID int) (map[int]int, error) {
	rows, err := tx.Query(`WITH assigned AS (SELECT deviceid device_id FROM job_devices WHERE jobid=$1 UNION SELECT device_id FROM warehouse_material_scans WHERE job_id=$1 AND device_id IS NOT NULL UNION SELECT pd.device_id FROM job_position_devices pd JOIN job_positions jp ON jp.position_id=pd.position_id WHERE jp.job_id=$1 AND COALESCE(to_jsonb(jp)->>'deleted_at','')='') SELECT d.productid,COUNT(DISTINCT dc.caseid) FROM assigned a JOIN devices d ON d.deviceid=a.device_id JOIN devicescases dc ON dc.deviceid=d.deviceid GROUP BY d.productid`, jobID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	counts := map[int]int{}
	for rows.Next() {
		var product, count int
		if err = rows.Scan(&product, &count); err != nil {
			return nil, err
		}
		counts[product] = count
	}
	return counts, rows.Err()
}
