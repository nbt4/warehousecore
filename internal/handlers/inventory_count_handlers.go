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

	"warehousecore/internal/repository"
)

type inventoryCount struct {
	CountID       int64      `json:"count_id"`
	ZoneID        int64      `json:"zone_id"`
	ZoneCode      string     `json:"zone_code"`
	ZoneName      string     `json:"zone_name"`
	Status        string     `json:"status"`
	BlindCount    bool       `json:"blind_count"`
	LineCount     int        `json:"line_count"`
	CountedLines  int        `json:"counted_lines"`
	VarianceLines int        `json:"variance_lines"`
	StartedAt     *time.Time `json:"started_at,omitempty"`
	CompletedAt   *time.Time `json:"completed_at,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`
}

type inventoryCountLine struct {
	LineID           int64    `json:"line_id"`
	ItemType         string   `json:"item_type"`
	ItemKey          string   `json:"item_key"`
	ItemName         string   `json:"item_name"`
	ExpectedQuantity *float64 `json:"expected_quantity,omitempty"`
	CountedQuantity  *float64 `json:"counted_quantity,omitempty"`
	Variance         *float64 `json:"variance,omitempty"`
}

func ListInventoryCounts(w http.ResponseWriter, r *http.Request) {
	rows, err := repository.GetSQLDB().Query(`SELECT c.count_id,c.zone_id,z.code,z.name,c.status,c.blind_count,
		COUNT(l.line_id),COUNT(l.counted_quantity),COUNT(*) FILTER (WHERE l.counted_quantity IS NOT NULL AND l.counted_quantity<>l.expected_quantity),
		c.started_at,c.completed_at,c.created_at
		FROM inventory_counts c JOIN storage_zones z ON z.zone_id=c.zone_id
		LEFT JOIN inventory_count_lines l ON l.count_id=c.count_id
		GROUP BY c.count_id,z.code,z.name ORDER BY c.created_at DESC LIMIT 200`)
	if err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	defer rows.Close()
	items := []inventoryCount{}
	for rows.Next() {
		var item inventoryCount
		var started, completed sql.NullTime
		if err := rows.Scan(&item.CountID, &item.ZoneID, &item.ZoneCode, &item.ZoneName, &item.Status, &item.BlindCount,
			&item.LineCount, &item.CountedLines, &item.VarianceLines, &started, &completed, &item.CreatedAt); err != nil {
			respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		if started.Valid {
			item.StartedAt = &started.Time
		}
		if completed.Valid {
			item.CompletedAt = &completed.Time
		}
		items = append(items, item)
	}
	respondJSON(w, http.StatusOK, items)
}

func CreateInventoryCount(w http.ResponseWriter, r *http.Request) {
	var input struct {
		ZoneID     int64 `json:"zone_id"`
		BlindCount bool  `json:"blind_count"`
	}
	if json.NewDecoder(r.Body).Decode(&input) != nil || input.ZoneID <= 0 {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "Lagerplatz ist erforderlich"})
		return
	}
	db := repository.GetSQLDB()
	tx, err := db.Begin()
	if err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	defer tx.Rollback()
	var active, storable bool
	var operationalStatus string
	if err = tx.QueryRow(`SELECT is_active,is_storable,operational_status FROM storage_zones WHERE zone_id=$1 FOR UPDATE`, input.ZoneID).Scan(&active, &storable, &operationalStatus); err != nil {
		respondJSON(w, http.StatusNotFound, map[string]string{"error": "Lagerplatz nicht gefunden"})
		return
	}
	if !active || !storable || operationalStatus != "available" {
		respondJSON(w, http.StatusConflict, map[string]string{"error": "Nur ein verfügbarer, direkt belegbarer Lagerplatz kann gezählt werden"})
		return
	}
	var openCount int
	_ = tx.QueryRow(`SELECT COUNT(*) FROM inventory_counts WHERE zone_id=$1 AND status IN ('open','counting','review')`, input.ZoneID).Scan(&openCount)
	if openCount > 0 {
		respondJSON(w, http.StatusConflict, map[string]string{"error": "Für diesen Lagerplatz läuft bereits eine Inventur"})
		return
	}
	var countID int64
	if err = tx.QueryRow(`INSERT INTO inventory_counts(zone_id,status,blind_count,started_at) VALUES($1,'counting',$2,CURRENT_TIMESTAMP) RETURNING count_id`, input.ZoneID, input.BlindCount).Scan(&countID); err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	statements := []string{
		`INSERT INTO inventory_count_lines(count_id,item_type,item_key,expected_quantity)
		 SELECT $1,'device',d.deviceID,1 FROM devices d WHERE d.zone_id=$2 AND NOT EXISTS(SELECT 1 FROM devicescases dc WHERE dc.deviceID=d.deviceID)`,
		`INSERT INTO inventory_count_lines(count_id,item_type,item_key,expected_quantity)
		 SELECT $1,'product',pl.product_id::text,pl.quantity FROM product_locations pl WHERE pl.zone_id=$2 AND pl.quantity<>0`,
		`INSERT INTO inventory_count_lines(count_id,item_type,item_key,expected_quantity)
		 SELECT $1,'case',c.caseID::text,1 FROM cases c WHERE c.zone_id=$2 AND NOT EXISTS(SELECT 1 FROM case_child_contents cc WHERE cc.child_case_id=c.caseID)`,
	}
	for _, statement := range statements {
		if _, err = tx.Exec(statement, countID, input.ZoneID); err != nil {
			respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
	}
	if _, err = tx.Exec(`UPDATE storage_zones SET operational_status='counting' WHERE zone_id=$1`, input.ZoneID); err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if err = tx.Commit(); err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	respondJSON(w, http.StatusCreated, map[string]interface{}{"count_id": countID, "message": "Inventur gestartet"})
}

func GetInventoryCount(w http.ResponseWriter, r *http.Request) {
	countID, err := strconv.ParseInt(mux.Vars(r)["id"], 10, 64)
	if err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "Ungültige Inventur"})
		return
	}
	db := repository.GetSQLDB()
	var count inventoryCount
	var started, completed sql.NullTime
	err = db.QueryRow(`SELECT c.count_id,c.zone_id,z.code,z.name,c.status,c.blind_count,
		(SELECT COUNT(*) FROM inventory_count_lines WHERE count_id=c.count_id),
		(SELECT COUNT(*) FROM inventory_count_lines WHERE count_id=c.count_id AND counted_quantity IS NOT NULL),
		(SELECT COUNT(*) FROM inventory_count_lines WHERE count_id=c.count_id AND counted_quantity IS NOT NULL AND counted_quantity<>expected_quantity),
		c.started_at,c.completed_at,c.created_at FROM inventory_counts c JOIN storage_zones z ON z.zone_id=c.zone_id WHERE c.count_id=$1`, countID).
		Scan(&count.CountID, &count.ZoneID, &count.ZoneCode, &count.ZoneName, &count.Status, &count.BlindCount,
			&count.LineCount, &count.CountedLines, &count.VarianceLines, &started, &completed, &count.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		respondJSON(w, http.StatusNotFound, map[string]string{"error": "Inventur nicht gefunden"})
		return
	}
	if err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if started.Valid {
		count.StartedAt = &started.Time
	}
	if completed.Valid {
		count.CompletedAt = &completed.Time
	}
	rows, err := db.Query(`SELECT l.line_id,l.item_type,l.item_key,
		CASE l.item_type WHEN 'device' THEN COALESCE((SELECT p.name||' · '||d.deviceID FROM devices d LEFT JOIN products p ON p.productID=d.productID WHERE d.deviceID=l.item_key),l.item_key)
		WHEN 'product' THEN COALESCE((SELECT p.name FROM products p WHERE p.productID=l.item_key::int),l.item_key)
		WHEN 'case' THEN COALESCE((SELECT c.name FROM cases c WHERE c.caseID=l.item_key::int),l.item_key) END,
		l.expected_quantity,l.counted_quantity FROM inventory_count_lines l WHERE l.count_id=$1 ORDER BY l.item_type,l.item_key`, countID)
	if err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	defer rows.Close()
	lines := []inventoryCountLine{}
	for rows.Next() {
		var line inventoryCountLine
		var expected float64
		var counted sql.NullFloat64
		if err := rows.Scan(&line.LineID, &line.ItemType, &line.ItemKey, &line.ItemName, &expected, &counted); err != nil {
			respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		if !count.BlindCount || count.Status != "counting" {
			line.ExpectedQuantity = &expected
		}
		if counted.Valid {
			line.CountedQuantity = &counted.Float64
			if line.ExpectedQuantity != nil {
				variance := counted.Float64 - expected
				line.Variance = &variance
			}
		}
		lines = append(lines, line)
	}
	respondJSON(w, http.StatusOK, map[string]interface{}{"count": count, "lines": lines})
}

func ScanInventoryCount(w http.ResponseWriter, r *http.Request) {
	countID, _ := strconv.ParseInt(mux.Vars(r)["id"], 10, 64)
	var input struct {
		ScanCode string  `json:"scan_code"`
		Quantity float64 `json:"quantity"`
	}
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
	var status string
	if err = tx.QueryRow(`SELECT status FROM inventory_counts WHERE count_id=$1 FOR UPDATE`, countID).Scan(&status); err != nil {
		respondJSON(w, http.StatusNotFound, map[string]string{"error": "Inventur nicht gefunden"})
		return
	}
	if status != "counting" {
		respondJSON(w, http.StatusConflict, map[string]string{"error": "Inventur ist nicht im Zählmodus"})
		return
	}
	code := strings.TrimSpace(input.ScanCode)
	itemType, itemKey, itemName := "", "", ""
	var deviceID string
	var productName sql.NullString
	err = tx.QueryRow(`SELECT d.deviceID,p.name FROM devices d LEFT JOIN products p ON p.productID=d.productID
		WHERE UPPER(d.deviceID)=UPPER($1) OR UPPER(COALESCE(d.barcode,''))=UPPER($1) OR UPPER(COALESCE(d.qr_code,''))=UPPER($1) LIMIT 1`, code).Scan(&deviceID, &productName)
	if err == nil {
		var packed bool
		_ = tx.QueryRow(`SELECT EXISTS(SELECT 1 FROM devicescases WHERE deviceID=$1)`, deviceID).Scan(&packed)
		if packed {
			respondJSON(w, http.StatusConflict, map[string]string{"error": "Gerät befindet sich in einem Case und kann nicht direkt gezählt werden"})
			return
		}
		itemType, itemKey, itemName = "device", deviceID, productName.String+" · "+deviceID
		input.Quantity = 1
	} else {
		var productID int64
		err = tx.QueryRow(`SELECT productID,name FROM products WHERE lifecycle_status='active' AND tracking_mode='quantity' AND (UPPER(COALESCE(generic_barcode,''))=UPPER($1) OR productID::text=$1) LIMIT 1`, code).Scan(&productID, &itemName)
		if err == nil {
			itemType, itemKey = "product", strconv.FormatInt(productID, 10)
		} else {
			var caseID int64
			err = tx.QueryRow(`SELECT caseID,name FROM cases WHERE LOWER(COALESCE(barcode,''))=LOWER($1) OR LOWER(COALESCE(rfid_tag,''))=LOWER($1) OR LOWER('CASE-'||caseID::text)=LOWER($1) LIMIT 1`, code).Scan(&caseID, &itemName)
			if err == nil {
				var nested bool
				_ = tx.QueryRow(`SELECT EXISTS(SELECT 1 FROM case_child_contents WHERE child_case_id=$1)`, caseID).Scan(&nested)
				if nested {
					respondJSON(w, http.StatusConflict, map[string]string{"error": "Case befindet sich in einem anderen Case und kann nicht direkt gezählt werden"})
					return
				}
				itemType, itemKey, input.Quantity = "case", strconv.FormatInt(caseID, 10), 1
			}
		}
	}
	if itemType == "" {
		respondJSON(w, http.StatusNotFound, map[string]string{"error": "Kein Gerät, Mengenartikel oder Case zu diesem Scan-Code gefunden"})
		return
	}
	if itemType == "product" {
		_, err = tx.Exec(`INSERT INTO inventory_count_lines(count_id,item_type,item_key,expected_quantity,counted_quantity)
			VALUES($1,$2,$3,0,$4) ON CONFLICT(count_id,item_type,item_key) DO UPDATE SET counted_quantity=COALESCE(inventory_count_lines.counted_quantity,0)+EXCLUDED.counted_quantity`, countID, itemType, itemKey, input.Quantity)
	} else {
		_, err = tx.Exec(`INSERT INTO inventory_count_lines(count_id,item_type,item_key,expected_quantity,counted_quantity)
			VALUES($1,$2,$3,0,1) ON CONFLICT(count_id,item_type,item_key) DO UPDATE SET counted_quantity=1`, countID, itemType, itemKey)
	}
	if err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if err = tx.Commit(); err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	respondJSON(w, http.StatusOK, map[string]interface{}{"message": itemName + " gezählt", "item_type": itemType, "item_key": itemKey})
}

func CompleteInventoryCount(w http.ResponseWriter, r *http.Request) {
	countID, _ := strconv.ParseInt(mux.Vars(r)["id"], 10, 64)
	tx, err := repository.GetSQLDB().Begin()
	if err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	defer tx.Rollback()
	result, err := tx.Exec(`UPDATE inventory_counts SET status='review' WHERE count_id=$1 AND status='counting'`, countID)
	if err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if n, _ := result.RowsAffected(); n == 0 {
		respondJSON(w, http.StatusConflict, map[string]string{"error": "Inventur ist nicht im Zählmodus"})
		return
	}
	if _, err = tx.Exec(`UPDATE inventory_count_lines SET counted_quantity=0 WHERE count_id=$1 AND counted_quantity IS NULL`, countID); err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if err = tx.Commit(); err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	respondJSON(w, http.StatusOK, map[string]string{"message": "Zählung zur Prüfung abgeschlossen"})
}

func ApproveInventoryCount(w http.ResponseWriter, r *http.Request) {
	countID, _ := strconv.ParseInt(mux.Vars(r)["id"], 10, 64)
	db := repository.GetSQLDB()
	tx, err := db.Begin()
	if err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	defer tx.Rollback()
	var zoneID int64
	var status string
	if err = tx.QueryRow(`SELECT zone_id,status FROM inventory_counts WHERE count_id=$1 FOR UPDATE`, countID).Scan(&zoneID, &status); err != nil {
		respondJSON(w, http.StatusNotFound, map[string]string{"error": "Inventur nicht gefunden"})
		return
	}
	if status != "review" {
		respondJSON(w, http.StatusConflict, map[string]string{"error": "Inventur muss zuerst geprüft werden"})
		return
	}
	statements := []string{
		`UPDATE devices d SET zone_id=NULL,status='location_unknown',current_location='location_unknown' WHERE d.zone_id=$2 AND EXISTS(SELECT 1 FROM inventory_count_lines l WHERE l.count_id=$1 AND l.item_type='device' AND l.item_key=d.deviceID AND l.counted_quantity=0)`,
		`UPDATE devices d SET zone_id=$2,status='in_storage',current_location='warehouse' FROM inventory_count_lines l WHERE l.count_id=$1 AND l.item_type='device' AND l.counted_quantity>0 AND d.deviceID=l.item_key`,
		`UPDATE cases c SET zone_id=NULL WHERE c.zone_id=$2 AND EXISTS(SELECT 1 FROM inventory_count_lines l WHERE l.count_id=$1 AND l.item_type='case' AND l.item_key=c.caseID::text AND l.counted_quantity=0)`,
		`UPDATE cases c SET zone_id=$2 FROM inventory_count_lines l WHERE l.count_id=$1 AND l.item_type='case' AND l.counted_quantity>0 AND c.caseID::text=l.item_key`,
		`INSERT INTO product_locations(product_id,zone_id,quantity) SELECT item_key::int,$2,counted_quantity FROM inventory_count_lines WHERE count_id=$1 AND item_type='product' ON CONFLICT(product_id,zone_id) DO UPDATE SET quantity=EXCLUDED.quantity,updated_at=CURRENT_TIMESTAMP`,
	}
	for _, statement := range statements {
		if _, err = tx.Exec(statement, countID, zoneID); err != nil {
			respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
	}
	if _, err = tx.Exec(`UPDATE inventory_counts SET status='approved',completed_at=CURRENT_TIMESTAMP WHERE count_id=$1`, countID); err == nil {
		_, err = tx.Exec(`UPDATE storage_zones SET operational_status='available',last_counted_at=CURRENT_TIMESTAMP,next_count_at=CASE WHEN inventory_frequency_days>0 THEN CURRENT_TIMESTAMP+(inventory_frequency_days::text||' days')::interval ELSE NULL END WHERE zone_id=$1`, zoneID)
	}
	if err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if err = tx.Commit(); err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	respondJSON(w, http.StatusOK, map[string]string{"message": "Inventur freigegeben und Bestände abgeglichen"})
}

func CancelInventoryCount(w http.ResponseWriter, r *http.Request) {
	countID, _ := strconv.ParseInt(mux.Vars(r)["id"], 10, 64)
	db := repository.GetSQLDB()
	result, err := db.Exec(`WITH cancelled AS (UPDATE inventory_counts SET status='cancelled',completed_at=CURRENT_TIMESTAMP WHERE count_id=$1 AND status IN ('open','counting','review') RETURNING zone_id) UPDATE storage_zones SET operational_status='available' WHERE zone_id IN (SELECT zone_id FROM cancelled)`, countID)
	if err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if n, _ := result.RowsAffected(); n == 0 {
		respondJSON(w, http.StatusConflict, map[string]string{"error": fmt.Sprintf("Inventur %d kann nicht abgebrochen werden", countID)})
		return
	}
	respondJSON(w, http.StatusOK, map[string]string{"message": "Inventur abgebrochen"})
}
