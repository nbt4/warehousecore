package handlers

import (
	"database/sql"
	"net/http"
	"strings"

	"warehousecore/internal/repository"
)

type scanResolution struct {
	Kind     string              `json:"kind"`
	Zone     *scanResolutionZone `json:"zone,omitempty"`
	Job      *scanResolutionJob  `json:"job,omitempty"`
	Case     *handlingUnit       `json:"case,omitempty"`
	DeviceID string              `json:"device_id,omitempty"`
	Product  *scanProductInfo    `json:"product,omitempty"`
}

type scanResolutionZone struct {
	ZoneID int64  `json:"zone_id"`
	Code   string `json:"code"`
	Name   string `json:"name"`
}

type scanResolutionJob struct {
	JobID    int64  `json:"job_id"`
	JobCode  string `json:"job_code"`
	Title    string `json:"title"`
	StatusID int    `json:"status_id"`
	Status   string `json:"status"`
}

type scanProductInfo struct {
	ProductID    int64   `json:"product_id"`
	Name         string  `json:"name"`
	Description  string  `json:"description"`
	Barcode      string  `json:"barcode"`
	TrackingMode string  `json:"tracking_mode"`
	Stock        float64 `json:"stock"`
	Unit         string  `json:"unit"`
	Brand        string  `json:"brand"`
	Manufacturer string  `json:"manufacturer"`
	Category     string  `json:"category"`
	DeviceCount  int64   `json:"device_count"`
}

// ResolveScan identifies the scanned entity without changing inventory state.
func ResolveScan(w http.ResponseWriter, r *http.Request) {
	code := strings.TrimSpace(r.URL.Query().Get("scan_code"))
	if code == "" {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "scan_code is required"})
		return
	}
	db := repository.GetSQLDB()
	var zone scanResolutionZone
	if err := db.QueryRow(`SELECT zone_id,code,name FROM storage_zones WHERE is_active AND (UPPER(code)=UPPER($1) OR UPPER(COALESCE(barcode,''))=UPPER($1)) LIMIT 1`, code).Scan(&zone.ZoneID, &zone.Code, &zone.Name); err == nil {
		respondJSON(w, http.StatusOK, scanResolution{Kind: "zone", Zone: &zone})
		return
	} else if err != sql.ErrNoRows {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	var job scanResolutionJob
	if err := db.QueryRow(`SELECT j.jobid,COALESCE(j.job_code,'JOB'||LPAD(j.jobid::text,6,'0')),COALESCE(j.description,''),j.statusid,COALESCE(s.status,'') FROM jobs j LEFT JOIN status s ON s.statusid=j.statusid WHERE j.deleted_at IS NULL AND (UPPER(COALESCE(j.job_code,''))=UPPER($1) OR j.jobid::text=$1 OR UPPER('JOB'||LPAD(j.jobid::text,6,'0'))=UPPER($1)) LIMIT 1`, code).Scan(&job.JobID, &job.JobCode, &job.Title, &job.StatusID, &job.Status); err == nil {
		respondJSON(w, http.StatusOK, scanResolution{Kind: "job", Job: &job})
		return
	} else if err != sql.ErrNoRows {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	unit, err := scanHandlingUnit(db.QueryRow(handlingUnitSelect+` WHERE LOWER(COALESCE(c.barcode,''))=LOWER($1) OR LOWER(COALESCE(c.rfid_tag,''))=LOWER($1) OR LOWER('CASE-'||c.caseID::text)=LOWER($1) OR EXISTS(SELECT 1 FROM inventory_identifiers ii WHERE ii.entity_type='case' AND ii.entity_key=c.caseID::text AND ii.active AND LOWER(ii.code)=LOWER($1)) LIMIT 1`, code))
	if err == nil {
		respondJSON(w, http.StatusOK, scanResolution{Kind: "case", Case: &unit})
		return
	}
	if err != sql.ErrNoRows {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	var deviceID string
	if err := db.QueryRow(`SELECT d.deviceID FROM devices d WHERE UPPER(d.deviceID)=UPPER($1) OR UPPER(COALESCE(d.barcode,''))=UPPER($1) OR UPPER(COALESCE(d.qr_code,''))=UPPER($1) OR EXISTS(SELECT 1 FROM inventory_identifiers ii WHERE ii.entity_type='device' AND ii.entity_key=d.deviceID AND ii.active AND UPPER(ii.code)=UPPER($1)) LIMIT 1`, code).Scan(&deviceID); err == nil {
		respondJSON(w, http.StatusOK, scanResolution{Kind: "device", DeviceID: deviceID})
		return
	} else if err != sql.ErrNoRows {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	var product scanProductInfo
	if err := db.QueryRow(`
		SELECT p.productID,p.name,COALESCE(p.description,''),COALESCE(p.generic_barcode,''),COALESCE(p.tracking_mode,'none'),COALESCE(p.stock_quantity,0),COALESCE(ct.abbreviation,'Stk'),COALESCE(b.name,''),COALESCE(m.name,''),COALESCE(c.name,''),(SELECT COUNT(*) FROM devices d WHERE d.productID=p.productID)
		FROM products p
		LEFT JOIN count_types ct ON ct.count_type_id=p.count_type_id
		LEFT JOIN brands b ON b.brandid=p.brandid
		LEFT JOIN manufacturer m ON m.manufacturerid=p.manufacturerid
		LEFT JOIN categories c ON c.categoryID=p.categoryID
		WHERE p.lifecycle_status='active' AND (UPPER(COALESCE(p.generic_barcode,''))=UPPER($1) OR p.productID::text=$1 OR UPPER('PRD-'||LPAD(p.productID::text,6,'0'))=UPPER($1) OR EXISTS(SELECT 1 FROM inventory_identifiers ii WHERE ii.entity_type='product' AND ii.entity_key=p.productID::text AND ii.active AND UPPER(ii.code)=UPPER($1)))
		LIMIT 1`, code).Scan(&product.ProductID, &product.Name, &product.Description, &product.Barcode, &product.TrackingMode, &product.Stock, &product.Unit, &product.Brand, &product.Manufacturer, &product.Category, &product.DeviceCount); err == nil {
		respondJSON(w, http.StatusOK, scanResolution{Kind: "product", Product: &product})
		return
	} else if err != sql.ErrNoRows {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	respondJSON(w, http.StatusNotFound, map[string]string{"error": "Scan-Code wurde nicht erkannt"})
}
