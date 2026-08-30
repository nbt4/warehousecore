package handlers

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/mux"

	"warehousecore/internal/jobstatus"
	"warehousecore/internal/repository"
	"warehousecore/internal/services"
)

type handlingUnit struct {
	CaseID           int64      `json:"case_id"`
	Name             string     `json:"name"`
	Description      *string    `json:"description,omitempty"`
	LegacyStatus     string     `json:"status"`
	CaseType         string     `json:"case_type"`
	WorkflowStatus   string     `json:"workflow_status"`
	Width            *float64   `json:"width,omitempty"`
	Height           *float64   `json:"height,omitempty"`
	Depth            *float64   `json:"depth,omitempty"`
	Weight           *float64   `json:"weight,omitempty"`
	MaxWeightKg      *float64   `json:"max_weight_kg,omitempty"`
	ZoneID           *int64     `json:"zone_id,omitempty"`
	ZoneName         *string    `json:"zone_name,omitempty"`
	ZoneCode         *string    `json:"zone_code,omitempty"`
	HomeZoneID       *int64     `json:"home_zone_id,omitempty"`
	CurrentJobID     *int64     `json:"current_job_id,omitempty"`
	Barcode          *string    `json:"barcode,omitempty"`
	RFIDTag          *string    `json:"rfid_tag,omitempty"`
	SealedAt         *time.Time `json:"sealed_at,omitempty"`
	DeviceCount      int        `json:"device_count"`
	ProductLineCount int        `json:"product_line_count"`
	ProductQuantity  float64    `json:"product_quantity"`
	ChildCaseCount   int        `json:"child_case_count"`
	ExpectedLines    int        `json:"expected_lines"`
	Complete         bool       `json:"complete"`
}

func scanHandlingUnit(scanner interface{ Scan(...interface{}) error }) (handlingUnit, error) {
	var item handlingUnit
	var description, zoneName, zoneCode, barcode, rfid sql.NullString
	var width, height, depth, weight, maxWeight sql.NullFloat64
	var zoneID, homeZoneID, jobID sql.NullInt64
	var sealed sql.NullTime
	err := scanner.Scan(&item.CaseID, &item.Name, &description, &item.LegacyStatus, &item.CaseType, &item.WorkflowStatus,
		&width, &height, &depth, &weight, &maxWeight, &zoneID, &zoneName, &zoneCode, &homeZoneID, &jobID, &barcode, &rfid, &sealed,
		&item.DeviceCount, &item.ProductLineCount, &item.ProductQuantity, &item.ChildCaseCount, &item.ExpectedLines, &item.Complete)
	if err != nil {
		return item, err
	}
	item.Description = ptrString(description)
	item.Width = ptrFloat64(width)
	item.Height = ptrFloat64(height)
	item.Depth = ptrFloat64(depth)
	item.Weight = ptrFloat64(weight)
	item.MaxWeightKg = ptrFloat64(maxWeight)
	if zoneID.Valid {
		v := zoneID.Int64
		item.ZoneID = &v
	}
	if homeZoneID.Valid {
		v := homeZoneID.Int64
		item.HomeZoneID = &v
	}
	if jobID.Valid {
		v := jobID.Int64
		item.CurrentJobID = &v
	}
	item.ZoneName = ptrString(zoneName)
	item.ZoneCode = ptrString(zoneCode)
	item.Barcode = ptrString(barcode)
	item.RFIDTag = ptrString(rfid)
	if sealed.Valid {
		v := sealed.Time
		item.SealedAt = &v
	}
	return item, nil
}

const handlingUnitSelect = `
	SELECT c.caseID,c.name,c.description,c.status,c.case_type,c.workflow_status,
	       c.width,c.height,c.depth,c.weight,c.max_weight_kg,c.zone_id,z.name,z.code,
	       c.home_zone_id,c.current_job_id,c.barcode,c.rfid_tag,c.sealed_at,
	       (SELECT COUNT(*) FROM devicescases dc WHERE dc.caseID=c.caseID),
	       (SELECT COUNT(*) FROM case_product_contents pc WHERE pc.case_id=c.caseID),
	       COALESCE((SELECT SUM(pc.quantity) FROM case_product_contents pc WHERE pc.case_id=c.caseID),0),
	       (SELECT COUNT(*) FROM case_child_contents cc WHERE cc.parent_case_id=c.caseID),
	       (SELECT COUNT(*) FROM case_content_templates ct WHERE ct.case_id=c.caseID),
	       CASE WHEN NOT EXISTS (
	         SELECT 1 FROM case_content_templates ct
	         WHERE ct.case_id=c.caseID AND COALESCE((
	           SELECT COUNT(*) FROM devicescases dc JOIN devices d ON d.deviceID=dc.deviceID WHERE dc.caseID=c.caseID AND d.productID=ct.product_id
	         ),0) + COALESCE((SELECT pc.quantity FROM case_product_contents pc WHERE pc.case_id=c.caseID AND pc.product_id=ct.product_id),0) < ct.expected_quantity
	       ) THEN TRUE ELSE FALSE END
	FROM cases c LEFT JOIN storage_zones z ON z.zone_id=c.zone_id`

func ListHandlingUnits(w http.ResponseWriter, r *http.Request) {
	query := handlingUnitSelect + ` WHERE 1=1`
	args := []interface{}{}
	if search := strings.TrimSpace(r.URL.Query().Get("search")); search != "" {
		args = append(args, "%"+search+"%")
		query += fmt.Sprintf(` AND (c.name ILIKE $%d OR c.barcode ILIKE $%d OR CAST(c.caseID AS TEXT)=$%d)`, len(args), len(args), len(args))
	}
	if status := strings.TrimSpace(r.URL.Query().Get("workflow_status")); status != "" {
		args = append(args, status)
		query += fmt.Sprintf(` AND c.workflow_status=$%d`, len(args))
	}
	query += ` ORDER BY c.name,c.caseID`
	rows, err := repository.GetSQLDB().Query(query, args...)
	if err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	defer rows.Close()
	items := []handlingUnit{}
	for rows.Next() {
		item, err := scanHandlingUnit(rows)
		if err != nil {
			respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		items = append(items, item)
	}
	respondJSON(w, http.StatusOK, map[string]interface{}{"cases": items, "meta": map[string]int{"count": len(items)}})
}

func GetHandlingUnit(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(mux.Vars(r)["id"], 10, 64)
	if err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "Ungültiges Case"})
		return
	}
	item, err := scanHandlingUnit(repository.GetSQLDB().QueryRow(handlingUnitSelect+` WHERE c.caseID=$1`, id))
	if errors.Is(err, sql.ErrNoRows) {
		respondJSON(w, http.StatusNotFound, map[string]string{"error": "Case nicht gefunden"})
		return
	}
	if err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	respondJSON(w, http.StatusOK, item)
}

func FindHandlingUnitByScan(w http.ResponseWriter, r *http.Request) {
	code := strings.TrimSpace(r.URL.Query().Get("scan_code"))
	if code == "" {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "Scan-Code fehlt"})
		return
	}
	item, err := scanHandlingUnit(repository.GetSQLDB().QueryRow(handlingUnitSelect+` WHERE LOWER(COALESCE(c.barcode,''))=LOWER($1) OR LOWER(COALESCE(c.rfid_tag,''))=LOWER($1) OR LOWER('CASE-'||c.caseID::text)=LOWER($1) LIMIT 1`, code))
	if errors.Is(err, sql.ErrNoRows) {
		respondJSON(w, http.StatusNotFound, map[string]string{"error": "Case nicht gefunden"})
		return
	}
	if err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	respondJSON(w, http.StatusOK, item)
}

type handlingUnitInput struct {
	Name        string   `json:"name"`
	Description *string  `json:"description"`
	CaseType    string   `json:"case_type"`
	Width       *float64 `json:"width"`
	Height      *float64 `json:"height"`
	Depth       *float64 `json:"depth"`
	Weight      *float64 `json:"weight"`
	MaxWeightKg *float64 `json:"max_weight_kg"`
	ZoneID      *int64   `json:"zone_id"`
	HomeZoneID  *int64   `json:"home_zone_id"`
	Barcode     *string  `json:"barcode"`
	RFIDTag     *string  `json:"rfid_tag"`
}

func decodeHandlingUnitInput(r *http.Request) (handlingUnitInput, error) {
	var input handlingUnitInput
	err := json.NewDecoder(r.Body).Decode(&input)
	input.Name = strings.TrimSpace(input.Name)
	if input.Name == "" {
		return input, fmt.Errorf("Name ist erforderlich")
	}
	if input.CaseType == "" {
		input.CaseType = "dynamic"
	}
	if input.CaseType != "dynamic" && input.CaseType != "fixed" && input.CaseType != "hybrid" {
		return input, fmt.Errorf("Ungültiger Case-Typ")
	}
	return input, err
}

func CreateHandlingUnit(w http.ResponseWriter, r *http.Request) {
	input, err := decodeHandlingUnitInput(r)
	if err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	db := repository.GetSQLDB()
	if input.ZoneID != nil {
		if err := services.ValidateStorageDestination(db, *input.ZoneID, 1); err != nil {
			respondJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
			return
		}
	}
	var id int64
	err = db.QueryRow(`INSERT INTO cases(name,description,width,height,depth,weight,max_weight_kg,status,workflow_status,case_type,zone_id,home_zone_id,barcode,rfid_tag) VALUES($1,$2,$3,$4,$5,$6,$7,'free','empty',$8,$9,$10,$11,$12) RETURNING caseID`, input.Name, input.Description, input.Width, input.Height, input.Depth, input.Weight, input.MaxWeightKg, input.CaseType, input.ZoneID, input.HomeZoneID, nullableStringPtr(input.Barcode), nullableStringPtr(input.RFIDTag)).Scan(&id)
	if err != nil {
		respondJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
		return
	}
	if input.Barcode == nil || strings.TrimSpace(*input.Barcode) == "" {
		_, _ = db.Exec(`UPDATE cases SET barcode=$1 WHERE caseID=$2`, fmt.Sprintf("CASE-%08d", id), id)
	}
	respondJSON(w, http.StatusCreated, map[string]interface{}{"case_id": id, "message": "Case erstellt"})
}

func UpdateHandlingUnit(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(mux.Vars(r)["id"], 10, 64)
	if err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "Ungültiges Case"})
		return
	}
	input, err := decodeHandlingUnitInput(r)
	if err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	if input.ZoneID != nil {
		if err := services.ValidateStorageDestination(repository.GetSQLDB(), *input.ZoneID, 1); err != nil {
			respondJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
			return
		}
	}
	result, err := repository.GetSQLDB().Exec(`UPDATE cases SET name=$1,description=$2,width=$3,height=$4,depth=$5,weight=$6,max_weight_kg=$7,case_type=$8,zone_id=$9,home_zone_id=$10,barcode=COALESCE($11,barcode),rfid_tag=$12 WHERE caseID=$13`, input.Name, input.Description, input.Width, input.Height, input.Depth, input.Weight, input.MaxWeightKg, input.CaseType, input.ZoneID, input.HomeZoneID, nullableStringPtr(input.Barcode), nullableStringPtr(input.RFIDTag), id)
	if err != nil {
		respondJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
		return
	}
	if n, _ := result.RowsAffected(); n == 0 {
		respondJSON(w, http.StatusNotFound, map[string]string{"error": "Case nicht gefunden"})
		return
	}
	respondJSON(w, http.StatusOK, map[string]string{"message": "Case aktualisiert"})
}

func DeleteHandlingUnit(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(mux.Vars(r)["id"], 10, 64)
	if err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "Ungültiges Case"})
		return
	}
	db := repository.GetSQLDB()
	var contents int
	err = db.QueryRow(`SELECT (SELECT COUNT(*) FROM devicescases WHERE caseID=$1)+(SELECT COUNT(*) FROM case_product_contents WHERE case_id=$1)+(SELECT COUNT(*) FROM case_child_contents WHERE parent_case_id=$1)+(SELECT COUNT(*) FROM case_child_contents WHERE child_case_id=$1)`, id).Scan(&contents)
	if err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if contents > 0 {
		respondJSON(w, http.StatusConflict, map[string]string{"error": "Case ist nicht leer und kann nicht gelöscht werden"})
		return
	}
	result, err := db.Exec(`DELETE FROM cases WHERE caseID=$1`, id)
	if err != nil {
		respondJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
		return
	}
	if n, _ := result.RowsAffected(); n == 0 {
		respondJSON(w, http.StatusNotFound, map[string]string{"error": "Case nicht gefunden"})
		return
	}
	respondJSON(w, http.StatusOK, map[string]string{"message": "Case gelöscht"})
}

type huDevice struct {
	DeviceID              string `json:"device_id"`
	ProductID             *int64 `json:"product_id,omitempty"`
	ProductName           string `json:"product_name"`
	Status                string `json:"status"`
	SerialNumber, Barcode *string
}
type huProduct struct {
	ProductID    int64   `json:"product_id"`
	ProductName  string  `json:"product_name"`
	Quantity     float64 `json:"quantity"`
	Unit         string  `json:"unit"`
	SourceZoneID *int64  `json:"source_zone_id,omitempty"`
}
type huTemplateLine struct {
	ProductID        int64   `json:"product_id"`
	ProductName      string  `json:"product_name"`
	ExpectedQuantity float64 `json:"expected_quantity"`
	ActualQuantity   float64 `json:"actual_quantity"`
	Complete         bool    `json:"complete"`
}
type huChild struct {
	CaseID         int64   `json:"case_id"`
	Name           string  `json:"name"`
	Barcode        *string `json:"barcode,omitempty"`
	WorkflowStatus string  `json:"workflow_status"`
}

func GetHandlingUnitInventory(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(mux.Vars(r)["id"], 10, 64)
	if err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "Ungültiges Case"})
		return
	}
	db := repository.GetSQLDB()
	devices := []huDevice{}
	rows, err := db.Query(`SELECT d.deviceID,d.productID,COALESCE(p.name,''),d.status,d.serialnumber,d.barcode FROM devicescases dc JOIN devices d ON d.deviceID=dc.deviceID LEFT JOIN products p ON p.productID=d.productID WHERE dc.caseID=$1 ORDER BY p.name,d.deviceID`, id)
	if err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	for rows.Next() {
		var x huDevice
		var pid sql.NullInt64
		var serial, barcode sql.NullString
		if rows.Scan(&x.DeviceID, &pid, &x.ProductName, &x.Status, &serial, &barcode) == nil {
			if pid.Valid {
				v := pid.Int64
				x.ProductID = &v
			}
			x.SerialNumber = ptrString(serial)
			x.Barcode = ptrString(barcode)
			devices = append(devices, x)
		}
	}
	rows.Close()
	products := []huProduct{}
	rows, err = db.Query(`SELECT pc.product_id,p.name,pc.quantity,COALESCE(ct.abbreviation,'Stk'),pc.added_from_zone_id FROM case_product_contents pc JOIN products p ON p.productID=pc.product_id LEFT JOIN count_types ct ON ct.count_type_id=p.count_type_id WHERE pc.case_id=$1 ORDER BY p.name`, id)
	if err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	for rows.Next() {
		var x huProduct
		var source sql.NullInt64
		if rows.Scan(&x.ProductID, &x.ProductName, &x.Quantity, &x.Unit, &source) == nil {
			if source.Valid {
				v := source.Int64
				x.SourceZoneID = &v
			}
			products = append(products, x)
		}
	}
	rows.Close()
	template := []huTemplateLine{}
	rows, err = db.Query(`SELECT ct.product_id,p.name,ct.expected_quantity,COALESCE((SELECT COUNT(*) FROM devicescases dc JOIN devices d ON d.deviceID=dc.deviceID WHERE dc.caseID=ct.case_id AND d.productID=ct.product_id),0)+COALESCE((SELECT pc.quantity FROM case_product_contents pc WHERE pc.case_id=ct.case_id AND pc.product_id=ct.product_id),0) FROM case_content_templates ct JOIN products p ON p.productID=ct.product_id WHERE ct.case_id=$1 ORDER BY p.name`, id)
	if err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	for rows.Next() {
		var x huTemplateLine
		if rows.Scan(&x.ProductID, &x.ProductName, &x.ExpectedQuantity, &x.ActualQuantity) == nil {
			x.Complete = x.ActualQuantity >= x.ExpectedQuantity
			template = append(template, x)
		}
	}
	rows.Close()
	children := []huChild{}
	rows, err = db.Query(`SELECT c.caseID,c.name,c.barcode,c.workflow_status FROM case_child_contents cc JOIN cases c ON c.caseID=cc.child_case_id WHERE cc.parent_case_id=$1 ORDER BY c.name`, id)
	if err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	for rows.Next() {
		var x huChild
		var barcode sql.NullString
		if rows.Scan(&x.CaseID, &x.Name, &barcode, &x.WorkflowStatus) == nil {
			x.Barcode = ptrString(barcode)
			children = append(children, x)
		}
	}
	rows.Close()
	complete := true
	for _, line := range template {
		if !line.Complete {
			complete = false
			break
		}
	}
	respondJSON(w, http.StatusOK, map[string]interface{}{"devices": devices, "products": products, "template": template, "child_cases": children, "complete": complete})
}

type packScanInput struct {
	ScanCode     string  `json:"scan_code"`
	Quantity     float64 `json:"quantity"`
	SourceZoneID *int64  `json:"source_zone_id"`
}

func PackHandlingUnitScan(w http.ResponseWriter, r *http.Request) {
	caseID, err := strconv.ParseInt(mux.Vars(r)["id"], 10, 64)
	if err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "Ungültiges Case"})
		return
	}
	var input packScanInput
	if json.NewDecoder(r.Body).Decode(&input) != nil || strings.TrimSpace(input.ScanCode) == "" {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "Scan-Code fehlt"})
		return
	}
	if input.Quantity <= 0 {
		input.Quantity = 1
	}
	db := repository.GetSQLDB()
	tx, err := db.Begin()
	if err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	defer tx.Rollback()
	var caseType, status string
	var sealed sql.NullTime
	if err = tx.QueryRow(`SELECT case_type,workflow_status,sealed_at FROM cases WHERE caseID=$1 FOR UPDATE`, caseID).Scan(&caseType, &status, &sealed); err != nil {
		respondJSON(w, http.StatusNotFound, map[string]string{"error": "Case nicht gefunden"})
		return
	}
	if sealed.Valid || status == "on_job" {
		respondJSON(w, http.StatusConflict, map[string]string{"error": "Versiegeltes oder ausgegebenes Case kann nicht verändert werden"})
		return
	}
	code := strings.TrimSpace(input.ScanCode)
	var deviceID string
	var productID sql.NullInt64
	var deviceStatus, conditionStatus string
	err = tx.QueryRow(`SELECT deviceID,productID,status,condition_status FROM devices WHERE UPPER(deviceID)=UPPER($1) OR UPPER(COALESCE(barcode,''))=UPPER($1) OR UPPER(COALESCE(qr_code,''))=UPPER($1) LIMIT 1`, code).Scan(&deviceID, &productID, &deviceStatus, &conditionStatus)
	if err == nil {
		if deviceStatus == "on_job" || deviceStatus == "return_pending" {
			respondJSON(w, http.StatusConflict, map[string]string{"error": "Ausgegebenes Gerät bzw. offener Rücklauf kann nicht gepackt werden"})
			return
		}
		if conditionStatus != "available" {
			respondJSON(w, http.StatusConflict, map[string]string{"error": "Nur einsatzbereite Geräte können gepackt werden; Betriebszustand: " + conditionStatus})
			return
		}
		var other sql.NullInt64
		checkErr := tx.QueryRow(`SELECT caseID FROM devicescases WHERE deviceID=$1`, deviceID).Scan(&other)
		if checkErr != nil && checkErr != sql.ErrNoRows {
			respondJSON(w, http.StatusInternalServerError, map[string]string{"error": checkErr.Error()})
			return
		}
		if other.Valid {
			if other.Int64 == caseID {
				respondJSON(w, http.StatusOK, map[string]interface{}{"message": "Gerät ist bereits im Case", "duplicate": true})
				return
			}
			respondJSON(w, http.StatusConflict, map[string]string{"error": fmt.Sprintf("Gerät ist bereits in Case %d", other.Int64)})
			return
		}
		if _, err = tx.Exec(`INSERT INTO devicescases(deviceID,caseID) VALUES($1,$2)`, deviceID, caseID); err != nil {
			respondJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
			return
		}
		_, _ = tx.Exec(`UPDATE devices SET status='in_storage',zone_id=NULL,current_location=$2 WHERE deviceID=$1`, deviceID, fmt.Sprintf("case:%d", caseID))
		_, _ = tx.Exec(`INSERT INTO case_events(case_id,event_type,device_id) VALUES($1,'pack_device',$2)`, caseID, deviceID)
		_, _ = tx.Exec(`UPDATE cases SET workflow_status='packing' WHERE caseID=$1 AND workflow_status='empty'`, caseID)
		if err = tx.Commit(); err != nil {
			respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		respondJSON(w, http.StatusOK, map[string]interface{}{"message": "Gerät ins Case gepackt", "item_type": "device", "device_id": deviceID})
		return
	}
	var foundProduct int64
	var productName string
	err = tx.QueryRow(`SELECT productID,name FROM products WHERE lifecycle_status='active' AND tracking_mode='quantity' AND (UPPER(COALESCE(generic_barcode,''))=UPPER($1) OR CAST(productID AS TEXT)=$1) LIMIT 1`, code).Scan(&foundProduct, &productName)
	if err == nil {
		var source sql.NullInt64
		var available float64
		if input.SourceZoneID != nil {
			err = tx.QueryRow(`SELECT zone_id,quantity FROM product_locations WHERE product_id=$1 AND zone_id=$2 AND quantity >= $3 FOR UPDATE`, foundProduct, *input.SourceZoneID, input.Quantity).Scan(&source, &available)
		} else {
			err = tx.QueryRow(`SELECT zone_id,quantity FROM product_locations WHERE product_id=$1 AND quantity >= $2 ORDER BY zone_id NULLS LAST,quantity DESC LIMIT 1 FOR UPDATE`, foundProduct, input.Quantity).Scan(&source, &available)
		}
		if err != nil {
			respondJSON(w, http.StatusConflict, map[string]string{"error": "Nicht genügend verfügbarer Bestand für " + productName})
			return
		}
		_, err = tx.Exec(`UPDATE product_locations SET quantity=quantity-$1,updated_at=CURRENT_TIMESTAMP WHERE product_id=$2 AND zone_id IS NOT DISTINCT FROM $3`, input.Quantity, foundProduct, source)
		if err != nil {
			respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		_, err = tx.Exec(`INSERT INTO case_product_contents(case_id,product_id,quantity,added_from_zone_id) VALUES($1,$2,$3,$4) ON CONFLICT(case_id,product_id) DO UPDATE SET quantity=case_product_contents.quantity+EXCLUDED.quantity,updated_at=CURRENT_TIMESTAMP`, caseID, foundProduct, input.Quantity, source)
		if err != nil {
			respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		_, _ = tx.Exec(`INSERT INTO case_events(case_id,event_type,product_id,quantity,zone_id) VALUES($1,'pack_product',$2,$3,$4)`, caseID, foundProduct, input.Quantity, source)
		_, _ = tx.Exec(`UPDATE cases SET workflow_status='packing' WHERE caseID=$1 AND workflow_status='empty'`, caseID)
		if err = tx.Commit(); err != nil {
			respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		respondJSON(w, http.StatusOK, map[string]interface{}{"message": fmt.Sprintf("%.2f × %s ins Case gepackt", input.Quantity, productName), "item_type": "product", "product_id": foundProduct})
		return
	}
	var childID int64
	var childName, childStatus string
	err = tx.QueryRow(`SELECT caseID,name,workflow_status FROM cases WHERE LOWER(COALESCE(barcode,''))=LOWER($1) OR LOWER(COALESCE(rfid_tag,''))=LOWER($1) OR LOWER('CASE-'||caseID::text)=LOWER($1) LIMIT 1`, code).Scan(&childID, &childName, &childStatus)
	if err == nil {
		if childID == caseID {
			respondJSON(w, http.StatusBadRequest, map[string]string{"error": "Ein Case kann nicht in sich selbst gepackt werden"})
			return
		}
		if childStatus == "on_job" {
			respondJSON(w, http.StatusConflict, map[string]string{"error": "Ein ausgegebenes Case kann nicht eingepackt werden"})
			return
		}
		var cycle bool
		err = tx.QueryRow(`WITH RECURSIVE descendants(case_id) AS (SELECT child_case_id FROM case_child_contents WHERE parent_case_id=$1 UNION ALL SELECT cc.child_case_id FROM case_child_contents cc JOIN descendants d ON cc.parent_case_id=d.case_id) SELECT EXISTS(SELECT 1 FROM descendants WHERE case_id=$2)`, childID, caseID).Scan(&cycle)
		if err != nil {
			respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		if cycle {
			respondJSON(w, http.StatusConflict, map[string]string{"error": "Diese Verschachtelung würde einen Kreis erzeugen"})
			return
		}
		_, err = tx.Exec(`INSERT INTO case_child_contents(parent_case_id,child_case_id) VALUES($1,$2)`, caseID, childID)
		if err != nil {
			respondJSON(w, http.StatusConflict, map[string]string{"error": "Case ist bereits in einem anderen Case"})
			return
		}
		_, _ = tx.Exec(`UPDATE cases SET zone_id=NULL WHERE caseID=$1`, childID)
		_, _ = tx.Exec(`INSERT INTO case_events(case_id,event_type,metadata) VALUES($1,'pack_child_case',jsonb_build_object('child_case_id',$2::bigint))`, caseID, childID)
		_, _ = tx.Exec(`UPDATE cases SET workflow_status='packing' WHERE caseID=$1 AND workflow_status='empty'`, caseID)
		if err = tx.Commit(); err != nil {
			respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		respondJSON(w, http.StatusOK, map[string]interface{}{"message": childName + " ins Case gepackt", "item_type": "case", "case_id": childID})
		return
	}
	respondJSON(w, http.StatusNotFound, map[string]string{"error": "Kein Gerät, Mengenartikel oder Case zu diesem Scan-Code gefunden"})
}

func RemoveHandlingUnitDevice(w http.ResponseWriter, r *http.Request) {
	caseID, err := strconv.ParseInt(mux.Vars(r)["id"], 10, 64)
	if err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "Ungültiges Case"})
		return
	}
	deviceID := mux.Vars(r)["device_id"]
	db := repository.GetSQLDB()
	tx, err := db.Begin()
	if err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	defer tx.Rollback()
	result, err := tx.Exec(`DELETE FROM devicescases WHERE caseID=$1 AND deviceID=$2`, caseID, deviceID)
	if err != nil {
		respondJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
		return
	}
	if n, _ := result.RowsAffected(); n == 0 {
		respondJSON(w, http.StatusNotFound, map[string]string{"error": "Gerät nicht im Case"})
		return
	}
	_, _ = tx.Exec(`UPDATE devices SET status='location_unknown',zone_id=NULL,current_location='location_unknown' WHERE deviceID=$1`, deviceID)
	_, _ = tx.Exec(`INSERT INTO case_events(case_id,event_type,device_id) VALUES($1,'remove_device',$2)`, caseID, deviceID)
	if err = tx.Commit(); err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	respondJSON(w, http.StatusOK, map[string]string{"message": "Gerät entfernt; Lagerplatz ist jetzt unbekannt"})
}

type removeProductInput struct {
	Quantity          float64 `json:"quantity"`
	DestinationZoneID *int64  `json:"destination_zone_id"`
}

func RemoveHandlingUnitProduct(w http.ResponseWriter, r *http.Request) {
	caseID, err := strconv.ParseInt(mux.Vars(r)["id"], 10, 64)
	if err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "Ungültiges Case"})
		return
	}
	productID, err := strconv.ParseInt(mux.Vars(r)["product_id"], 10, 64)
	if err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "Ungültiger Artikel"})
		return
	}
	var input removeProductInput
	if json.NewDecoder(r.Body).Decode(&input) != nil || input.Quantity <= 0 {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "Menge muss größer als null sein"})
		return
	}
	if input.DestinationZoneID == nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "Rücklagerplatz ist erforderlich"})
		return
	}
	db := repository.GetSQLDB()
	if err := services.ValidateStorageDestination(db, *input.DestinationZoneID, input.Quantity); err != nil {
		respondJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
		return
	}
	tx, err := db.Begin()
	if err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	defer tx.Rollback()
	var current float64
	if err = tx.QueryRow(`SELECT quantity FROM case_product_contents WHERE case_id=$1 AND product_id=$2 FOR UPDATE`, caseID, productID).Scan(&current); err != nil || current < input.Quantity {
		respondJSON(w, http.StatusConflict, map[string]string{"error": "Menge ist nicht im Case verfügbar"})
		return
	}
	if current == input.Quantity {
		_, err = tx.Exec(`DELETE FROM case_product_contents WHERE case_id=$1 AND product_id=$2`, caseID, productID)
	} else {
		_, err = tx.Exec(`UPDATE case_product_contents SET quantity=quantity-$1,updated_at=CURRENT_TIMESTAMP WHERE case_id=$2 AND product_id=$3`, input.Quantity, caseID, productID)
	}
	if err == nil {
		_, err = tx.Exec(`INSERT INTO product_locations(product_id,zone_id,quantity) VALUES($1,$2,$3) ON CONFLICT(product_id,zone_id) DO UPDATE SET quantity=product_locations.quantity+EXCLUDED.quantity,updated_at=CURRENT_TIMESTAMP`, productID, *input.DestinationZoneID, input.Quantity)
	}
	if err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	_, _ = tx.Exec(`INSERT INTO case_events(case_id,event_type,product_id,quantity,zone_id) VALUES($1,'remove_product',$2,$3,$4)`, caseID, productID, input.Quantity, *input.DestinationZoneID)
	if err = tx.Commit(); err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	respondJSON(w, http.StatusOK, map[string]string{"message": "Artikel zurückgelagert"})
}

func RemoveHandlingUnitChild(w http.ResponseWriter, r *http.Request) {
	caseID, _ := strconv.ParseInt(mux.Vars(r)["id"], 10, 64)
	childID, _ := strconv.ParseInt(mux.Vars(r)["child_id"], 10, 64)
	var input struct {
		DestinationZoneID *int64 `json:"destination_zone_id"`
	}
	_ = json.NewDecoder(r.Body).Decode(&input)
	if input.DestinationZoneID == nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "Rücklagerplatz ist erforderlich"})
		return
	}
	if err := services.ValidateStorageDestination(repository.GetSQLDB(), *input.DestinationZoneID, 1); err != nil {
		respondJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
		return
	}
	tx, _ := repository.GetSQLDB().Begin()
	defer tx.Rollback()
	result, err := tx.Exec(`DELETE FROM case_child_contents WHERE parent_case_id=$1 AND child_case_id=$2`, caseID, childID)
	if err != nil {
		respondJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
		return
	}
	if n, _ := result.RowsAffected(); n == 0 {
		respondJSON(w, http.StatusNotFound, map[string]string{"error": "Untercase nicht gefunden"})
		return
	}
	_, _ = tx.Exec(`UPDATE cases SET zone_id=$1 WHERE caseID=$2`, *input.DestinationZoneID, childID)
	_ = tx.Commit()
	respondJSON(w, http.StatusOK, map[string]string{"message": "Untercase zurückgelagert"})
}

type templateInput struct {
	ProductID        int64   `json:"product_id"`
	ScanCode         string  `json:"scan_code"`
	ExpectedQuantity float64 `json:"expected_quantity"`
}

func UpsertHandlingUnitTemplate(w http.ResponseWriter, r *http.Request) {
	caseID, _ := strconv.ParseInt(mux.Vars(r)["id"], 10, 64)
	var input templateInput
	if json.NewDecoder(r.Body).Decode(&input) != nil || input.ExpectedQuantity <= 0 {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "Artikel und Sollmenge sind erforderlich"})
		return
	}
	if input.ProductID <= 0 && strings.TrimSpace(input.ScanCode) != "" {
		err := repository.GetSQLDB().QueryRow(`SELECT productID FROM products WHERE lifecycle_status='active' AND (UPPER(COALESCE(generic_barcode,''))=UPPER($1) OR CAST(productID AS TEXT)=$1) LIMIT 1`, strings.TrimSpace(input.ScanCode)).Scan(&input.ProductID)
		if err != nil {
			respondJSON(w, http.StatusNotFound, map[string]string{"error": "Artikel für Soll-Inhalt nicht gefunden"})
			return
		}
	}
	if input.ProductID <= 0 {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "Artikel und Sollmenge sind erforderlich"})
		return
	}
	_, err := repository.GetSQLDB().Exec(`INSERT INTO case_content_templates(case_id,product_id,expected_quantity) VALUES($1,$2,$3) ON CONFLICT(case_id,product_id) DO UPDATE SET expected_quantity=EXCLUDED.expected_quantity,updated_at=CURRENT_TIMESTAMP`, caseID, input.ProductID, input.ExpectedQuantity)
	if err != nil {
		respondJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
		return
	}
	respondJSON(w, http.StatusOK, map[string]string{"message": "Soll-Inhalt gespeichert"})
}
func DeleteHandlingUnitTemplate(w http.ResponseWriter, r *http.Request) {
	caseID, _ := strconv.ParseInt(mux.Vars(r)["id"], 10, 64)
	productID, _ := strconv.ParseInt(mux.Vars(r)["product_id"], 10, 64)
	result, err := repository.GetSQLDB().Exec(`DELETE FROM case_content_templates WHERE case_id=$1 AND product_id=$2`, caseID, productID)
	if err != nil {
		respondJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
		return
	}
	if n, _ := result.RowsAffected(); n == 0 {
		respondJSON(w, http.StatusNotFound, map[string]string{"error": "Soll-Inhalt nicht gefunden"})
		return
	}
	respondJSON(w, http.StatusOK, map[string]string{"message": "Soll-Inhalt entfernt"})
}

func SealHandlingUnit(w http.ResponseWriter, r *http.Request) {
	caseID, _ := strconv.ParseInt(mux.Vars(r)["id"], 10, 64)
	var input struct {
		Force bool `json:"force"`
	}
	_ = json.NewDecoder(r.Body).Decode(&input)
	db := repository.GetSQLDB()
	var caseType string
	var missing int
	err := db.QueryRow(`SELECT case_type,(SELECT COUNT(*) FROM case_content_templates ct WHERE ct.case_id=c.caseID AND COALESCE((SELECT COUNT(*) FROM devicescases dc JOIN devices d ON d.deviceID=dc.deviceID WHERE dc.caseID=c.caseID AND d.productID=ct.product_id),0)+COALESCE((SELECT quantity FROM case_product_contents pc WHERE pc.case_id=c.caseID AND pc.product_id=ct.product_id),0)<ct.expected_quantity) FROM cases c WHERE c.caseID=$1`, caseID).Scan(&caseType, &missing)
	if err != nil {
		respondJSON(w, http.StatusNotFound, map[string]string{"error": "Case nicht gefunden"})
		return
	}
	if (caseType == "fixed" || caseType == "hybrid") && missing > 0 && !input.Force {
		respondJSON(w, http.StatusConflict, map[string]interface{}{"error": "Soll-Inhalt ist noch nicht vollständig", "missing_lines": missing})
		return
	}
	_, err = db.Exec(`UPDATE cases SET workflow_status='sealed',sealed_at=CURRENT_TIMESTAMP WHERE caseID=$1`, caseID)
	if err != nil {
		respondJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
		return
	}
	_, _ = db.Exec(`INSERT INTO case_events(case_id,event_type,metadata) VALUES($1,'seal',jsonb_build_object('forced',$2::boolean))`, caseID, input.Force)
	respondJSON(w, http.StatusOK, map[string]string{"message": "Case versiegelt"})
}
func UnsealHandlingUnit(w http.ResponseWriter, r *http.Request) {
	caseID, _ := strconv.ParseInt(mux.Vars(r)["id"], 10, 64)
	result, err := repository.GetSQLDB().Exec(`UPDATE cases SET workflow_status='packing',sealed_at=NULL WHERE caseID=$1 AND workflow_status<>'on_job'`, caseID)
	if err != nil {
		respondJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
		return
	}
	if n, _ := result.RowsAffected(); n == 0 {
		respondJSON(w, http.StatusConflict, map[string]string{"error": "Ausgegebenes Case kann nicht geöffnet werden"})
		return
	}
	_, _ = repository.GetSQLDB().Exec(`INSERT INTO case_events(case_id,event_type) VALUES($1,'unseal')`, caseID)
	respondJSON(w, http.StatusOK, map[string]string{"message": "Case geöffnet"})
}

type dispatchInput struct {
	JobID int64 `json:"job_id"`
	Force bool  `json:"force"`
}

func DispatchHandlingUnit(w http.ResponseWriter, r *http.Request) {
	caseID, _ := strconv.ParseInt(mux.Vars(r)["id"], 10, 64)
	var input dispatchInput
	if json.NewDecoder(r.Body).Decode(&input) != nil || input.JobID <= 0 {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "Job ist erforderlich"})
		return
	}
	db := repository.GetSQLDB()
	tx, err := db.Begin()
	if err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	defer tx.Rollback()
	var status string
	if err = tx.QueryRow(`SELECT workflow_status FROM cases WHERE caseID=$1 FOR UPDATE`, caseID).Scan(&status); err != nil {
		respondJSON(w, http.StatusNotFound, map[string]string{"error": "Case nicht gefunden"})
		return
	}
	if status != "sealed" && !input.Force {
		respondJSON(w, http.StatusConflict, map[string]string{"error": "Case muss vor der Ausgabe versiegelt werden"})
		return
	}
	var unavailable int
	if err = tx.QueryRow(`WITH RECURSIVE tree AS (SELECT $1::int AS case_id UNION ALL SELECT cc.child_case_id FROM case_child_contents cc JOIN tree t ON cc.parent_case_id=t.case_id)
		SELECT COUNT(*) FROM devicescases dc JOIN tree t ON t.case_id=dc.caseID JOIN devices d ON d.deviceID=dc.deviceID WHERE d.condition_status<>'available' OR d.status<>'in_storage'`, caseID).Scan(&unavailable); err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if unavailable > 0 {
		respondJSON(w, http.StatusConflict, map[string]string{"error": fmt.Sprintf("%d Gerät(e) im Case sind nicht einsatzbereit oder nicht eingelagert", unavailable)})
		return
	}
	var jobCode, jobStatus string
	var jobStatusID int
	err = tx.QueryRow(`SELECT j.job_code,j.statusid,COALESCE(s.status,'') FROM jobs j LEFT JOIN status s ON s.statusid=j.statusid WHERE j.jobid=$1 AND j.deleted_at IS NULL`, input.JobID).Scan(&jobCode, &jobStatusID, &jobStatus)
	if err != nil {
		respondJSON(w, http.StatusNotFound, map[string]string{"error": "Job nicht gefunden"})
		return
	}
	if jobStatusID != jobstatus.ConfirmedID {
		respondJSON(w, http.StatusConflict, map[string]string{"error": fmt.Sprintf("Job hat den Status %s; Case-Ausgaben sind nur für bestätigte Jobs möglich", jobStatus)})
		return
	}
	_, err = tx.Exec(`WITH RECURSIVE tree AS (SELECT $1::int AS case_id UNION ALL SELECT cc.child_case_id FROM case_child_contents cc JOIN tree t ON cc.parent_case_id=t.case_id) UPDATE cases c SET workflow_status='on_job',status='rented',current_job_id=$2,zone_id=NULL FROM tree WHERE c.caseID=tree.case_id`, caseID, input.JobID)
	if err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	_, err = tx.Exec(`WITH RECURSIVE tree AS (SELECT $1::int AS case_id UNION ALL SELECT cc.child_case_id FROM case_child_contents cc JOIN tree t ON cc.parent_case_id=t.case_id), packed AS (SELECT dc.deviceID FROM devicescases dc JOIN tree t ON t.case_id=dc.caseID) UPDATE devices d SET status='on_job',zone_id=NULL,current_location=$2 FROM packed WHERE d.deviceID=packed.deviceID`, caseID, "job:"+jobCode)
	if err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	_, err = tx.Exec(`WITH RECURSIVE tree AS (SELECT $1::int AS case_id UNION ALL SELECT cc.child_case_id FROM case_child_contents cc JOIN tree t ON cc.parent_case_id=t.case_id), packed AS (SELECT dc.deviceID FROM devicescases dc JOIN tree t ON t.case_id=dc.caseID) INSERT INTO job_devices(deviceID,jobID,pack_status,pack_ts) SELECT deviceID,$2,'issued',CURRENT_TIMESTAMP FROM packed ON CONFLICT(deviceID,jobID) DO UPDATE SET pack_status='issued',pack_ts=CURRENT_TIMESTAMP`, caseID, input.JobID)
	if err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	_, _ = tx.Exec(`INSERT INTO case_events(case_id,event_type,job_id,metadata) VALUES($1,'dispatch',$2,jsonb_build_object('forced',$3::boolean))`, caseID, input.JobID, input.Force)
	if err = tx.Commit(); err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	respondJSON(w, http.StatusOK, map[string]string{"message": "Case für " + jobCode + " ausgegeben"})
}

type returnInput struct {
	DestinationZoneID int64  `json:"destination_zone_id"`
	Mode              string `json:"mode"`
}

func ReturnHandlingUnit(w http.ResponseWriter, r *http.Request) {
	caseID, _ := strconv.ParseInt(mux.Vars(r)["id"], 10, 64)
	var input returnInput
	if json.NewDecoder(r.Body).Decode(&input) != nil || input.DestinationZoneID <= 0 {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "Rücklaufzone ist erforderlich"})
		return
	}
	if err := services.ValidateStorageDestination(repository.GetSQLDB(), input.DestinationZoneID, 1); err != nil {
		respondJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
		return
	}
	if input.Mode == "" {
		input.Mode = "inspect"
	}
	if input.Mode != "inspect" && input.Mode != "sealed" {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "Rücklaufmodus muss inspect oder sealed sein"})
		return
	}
	db := repository.GetSQLDB()
	tx, err := db.Begin()
	if err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	defer tx.Rollback()
	var currentStatus string
	if err = tx.QueryRow(`SELECT workflow_status FROM cases WHERE caseID=$1 FOR UPDATE`, caseID).Scan(&currentStatus); err != nil {
		respondJSON(w, http.StatusNotFound, map[string]string{"error": "Case nicht gefunden"})
		return
	}
	if currentStatus != "on_job" {
		respondJSON(w, http.StatusConflict, map[string]string{"error": "Nur ein ausgegebenes Case kann zurückgenommen werden"})
		return
	}
	_, err = tx.Exec(`WITH RECURSIVE tree AS (
		SELECT $3::int AS case_id,TRUE AS is_root
		UNION ALL SELECT cc.child_case_id,FALSE FROM case_child_contents cc JOIN tree t ON cc.parent_case_id=t.case_id
	) UPDATE cases c SET
		workflow_status=CASE WHEN $2::text='sealed' AND c.sealed_at IS NOT NULL THEN 'sealed' ELSE 'return_check' END,
		status='free',current_job_id=NULL,zone_id=CASE WHEN tree.is_root THEN $1::int ELSE NULL END,
		sealed_at=CASE WHEN $2::text='sealed' THEN c.sealed_at ELSE NULL END
		FROM tree WHERE c.caseID=tree.case_id`, input.DestinationZoneID, input.Mode, caseID)
	if err != nil {
		respondJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
		return
	}
	_, err = tx.Exec(`WITH RECURSIVE tree AS (
		SELECT $1::int AS case_id UNION ALL SELECT cc.child_case_id FROM case_child_contents cc JOIN tree t ON cc.parent_case_id=t.case_id
	), packed AS (SELECT dc.deviceID FROM devicescases dc JOIN tree t ON t.case_id=dc.caseID)
	UPDATE devices d SET status=CASE WHEN $2::text='sealed' THEN 'in_storage' ELSE 'return_pending' END,
		zone_id=NULL,current_location=CASE WHEN $2::text='sealed' THEN 'case:sealed' ELSE 'case:return' END
	FROM packed WHERE d.deviceID=packed.deviceID`, caseID, input.Mode)
	if err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	_, err = tx.Exec(`WITH RECURSIVE tree AS (
		SELECT $1::int AS case_id UNION ALL SELECT cc.child_case_id FROM case_child_contents cc JOIN tree t ON cc.parent_case_id=t.case_id
	), packed AS (SELECT dc.deviceID FROM devicescases dc JOIN tree t ON t.case_id=dc.caseID)
	UPDATE job_devices jd SET pack_status='returned',pack_ts=CURRENT_TIMESTAMP FROM packed WHERE jd.deviceID=packed.deviceID AND jd.pack_status='issued'`, caseID)
	if err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	_, _ = tx.Exec(`INSERT INTO case_events(case_id,event_type,zone_id,metadata) VALUES($1,'return',$2,jsonb_build_object('mode',$3::text))`, caseID, input.DestinationZoneID, input.Mode)
	if err = tx.Commit(); err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	respondJSON(w, http.StatusOK, map[string]string{"message": "Case im Rücklauf erfasst"})
}

type unpackInput struct {
	DestinationZoneID int64 `json:"destination_zone_id"`
}

func UnpackHandlingUnit(w http.ResponseWriter, r *http.Request) {
	caseID, _ := strconv.ParseInt(mux.Vars(r)["id"], 10, 64)
	var input unpackInput
	if json.NewDecoder(r.Body).Decode(&input) != nil || input.DestinationZoneID <= 0 {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "Lagerplatz ist erforderlich"})
		return
	}
	db := repository.GetSQLDB()
	if err := services.ValidateStorageDestination(db, input.DestinationZoneID, 1); err != nil {
		respondJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
		return
	}
	tx, err := db.Begin()
	if err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	defer tx.Rollback()
	var workflowStatus string
	if err = tx.QueryRow(`SELECT workflow_status FROM cases WHERE caseID=$1 FOR UPDATE`, caseID).Scan(&workflowStatus); err != nil {
		respondJSON(w, http.StatusNotFound, map[string]string{"error": "Case nicht gefunden"})
		return
	}
	if workflowStatus == "on_job" || workflowStatus == "sealed" {
		respondJSON(w, http.StatusConflict, map[string]string{"error": "Case muss zurückgenommen und geöffnet sein, bevor es entpackt wird"})
		return
	}
	_, err = tx.Exec(`UPDATE devices SET status='in_storage',zone_id=$1,current_location='warehouse' WHERE deviceID IN (SELECT deviceID FROM devicescases WHERE caseID=$2)`, input.DestinationZoneID, caseID)
	if err == nil {
		_, err = tx.Exec(`INSERT INTO product_locations(product_id,zone_id,quantity) SELECT product_id,$1,quantity FROM case_product_contents WHERE case_id=$2 ON CONFLICT(product_id,zone_id) DO UPDATE SET quantity=product_locations.quantity+EXCLUDED.quantity,updated_at=CURRENT_TIMESTAMP`, input.DestinationZoneID, caseID)
	}
	if err == nil {
		_, err = tx.Exec(`UPDATE cases SET zone_id=$1,status='free',current_job_id=NULL,workflow_status=CASE WHEN sealed_at IS NOT NULL THEN 'sealed' ELSE 'return_check' END WHERE caseID IN (SELECT child_case_id FROM case_child_contents WHERE parent_case_id=$2)`, input.DestinationZoneID, caseID)
	}
	if err == nil {
		_, err = tx.Exec(`DELETE FROM devicescases WHERE caseID=$1`, caseID)
	}
	if err == nil {
		_, err = tx.Exec(`DELETE FROM case_product_contents WHERE case_id=$1`, caseID)
	}
	if err == nil {
		_, err = tx.Exec(`DELETE FROM case_child_contents WHERE parent_case_id=$1`, caseID)
	}
	if err == nil {
		_, err = tx.Exec(`UPDATE cases SET workflow_status='empty',status='free',sealed_at=NULL,current_job_id=NULL,zone_id=$1 WHERE caseID=$2`, input.DestinationZoneID, caseID)
	}
	if err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	_, _ = tx.Exec(`INSERT INTO case_events(case_id,event_type,zone_id) VALUES($1,'unpack',$2)`, caseID, input.DestinationZoneID)
	if err = tx.Commit(); err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	respondJSON(w, http.StatusOK, map[string]string{"message": "Case vollständig entpackt und Bestand zurückgelagert"})
}

func GetHandlingUnitEvents(w http.ResponseWriter, r *http.Request) {
	caseID, _ := strconv.ParseInt(mux.Vars(r)["id"], 10, 64)
	rows, err := repository.GetSQLDB().Query(`SELECT event_id,event_type,device_id,product_id,quantity,zone_id,job_id,metadata,created_at FROM case_events WHERE case_id=$1 ORDER BY created_at DESC,event_id DESC LIMIT 200`, caseID)
	if err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	defer rows.Close()
	items := []map[string]interface{}{}
	for rows.Next() {
		var id int64
		var eventType string
		var device sql.NullString
		var product, zone, job sql.NullInt64
		var qty sql.NullFloat64
		var metadata []byte
		var created time.Time
		if rows.Scan(&id, &eventType, &device, &product, &qty, &zone, &job, &metadata, &created) != nil {
			continue
		}
		item := map[string]interface{}{"event_id": id, "event_type": eventType, "created_at": created, "metadata": json.RawMessage(metadata)}
		if device.Valid {
			item["device_id"] = device.String
		}
		if product.Valid {
			item["product_id"] = product.Int64
		}
		if qty.Valid {
			item["quantity"] = qty.Float64
		}
		if zone.Valid {
			item["zone_id"] = zone.Int64
		}
		if job.Valid {
			item["job_id"] = job.Int64
		}
		items = append(items, item)
	}
	respondJSON(w, http.StatusOK, items)
}
