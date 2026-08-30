package handlers

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/gorilla/mux"
	"github.com/lib/pq"
	"warehousecore/internal/models"
	"warehousecore/internal/repository"
)

const (
	cableTrackingQuantity   = "quantity"
	cableTrackingIndividual = "individual"
)

type Cable struct {
	CableID            int          `json:"cable_id"`
	ProductID          int          `json:"product_id"`
	Name               string       `json:"name"`
	Connector1         int          `json:"connector1"`
	Connector2         int          `json:"connector2"`
	Typ                int          `json:"typ"`
	Length             float64      `json:"length"`
	MM2                *float64     `json:"mm2"`
	TrackingMode       string       `json:"tracking_mode"`
	GenericBarcode     *string      `json:"generic_barcode"`
	StockQuantity      float64      `json:"stock_quantity"`
	AvailableQuantity  float64      `json:"available_quantity"`
	UnitCount          int          `json:"unit_count"`
	Connector1Name     string       `json:"connector1_name"`
	Connector2Name     string       `json:"connector2_name"`
	CableTypeName      string       `json:"cable_type_name"`
	Connector1Gender   *string      `json:"connector1_gender"`
	Connector2Gender   *string      `json:"connector2_gender"`
	MigratedFromLegacy bool         `json:"migrated_from_legacy"`
	ZoneStocks         []CableStock `json:"zone_stocks,omitempty"`
	Units              []CableUnit  `json:"units,omitempty"`
}

type CableStock struct {
	ZoneID   *int    `json:"zone_id"`
	ZoneName string  `json:"zone_name"`
	ZoneCode string  `json:"zone_code"`
	Quantity float64 `json:"quantity"`
}

type CableUnit struct {
	DeviceID        string  `json:"device_id"`
	Barcode         *string `json:"barcode"`
	QRCode          *string `json:"qr_code"`
	Status          string  `json:"status"`
	ZoneID          *int    `json:"zone_id"`
	ZoneName        string  `json:"zone_name"`
	ZoneCode        string  `json:"zone_code"`
	ConditionRating float64 `json:"condition_rating"`
	CurrentJobID    *int    `json:"current_job_id"`
}

type CableConnector struct {
	ConnectorID  int     `json:"connector_id"`
	Name         string  `json:"name"`
	Abbreviation *string `json:"abbreviation"`
	Gender       *string `json:"gender"`
}

type CableType struct {
	CableTypeID int    `json:"cable_type_id"`
	Name        string `json:"name"`
	Count       int    `json:"count"`
}

type cableCreateRequest struct {
	Name           string   `json:"name"`
	Connector1     int      `json:"connector1"`
	Connector2     int      `json:"connector2"`
	Typ            int      `json:"typ"`
	Length         float64  `json:"length"`
	MM2            *float64 `json:"mm2"`
	TrackingMode   string   `json:"tracking_mode"`
	GenericBarcode string   `json:"generic_barcode"`
	Quantity       int      `json:"quantity"`
	ZoneID         *int     `json:"zone_id"`
}

type cableUpdateRequest struct {
	Name           *string                  `json:"name"`
	Connector1     *int                     `json:"connector1"`
	Connector2     *int                     `json:"connector2"`
	Typ            *int                     `json:"typ"`
	Length         *float64                 `json:"length"`
	MM2            models.Optional[float64] `json:"mm2"`
	TrackingMode   *string                  `json:"tracking_mode"`
	GenericBarcode *string                  `json:"generic_barcode"`
}

type cableStockRequest struct {
	ZoneID   *int    `json:"zone_id"`
	Quantity float64 `json:"quantity"`
}

type cableUnitRequest struct {
	Quantity int  `json:"quantity"`
	ZoneID   *int `json:"zone_id"`
}

func normalizeCableTrackingMode(value string) (string, error) {
	mode := strings.ToLower(strings.TrimSpace(value))
	if mode == "" {
		return cableTrackingQuantity, nil
	}
	if mode != cableTrackingQuantity && mode != cableTrackingIndividual {
		return "", fmt.Errorf("tracking_mode must be quantity or individual")
	}
	return mode, nil
}

func validateCableCreateRequest(input *cableCreateRequest) error {
	if input == nil {
		return errors.New("request is required")
	}
	if input.Connector1 <= 0 || input.Connector2 <= 0 || input.Typ <= 0 {
		return errors.New("connector1, connector2 and typ are required")
	}
	if input.Length <= 0 {
		return errors.New("length must be greater than 0")
	}
	if input.MM2 != nil && *input.MM2 <= 0 {
		return errors.New("mm2 must be greater than 0 when provided")
	}
	if input.Quantity < 0 {
		return errors.New("quantity must not be negative")
	}
	mode, err := normalizeCableTrackingMode(input.TrackingMode)
	if err != nil {
		return err
	}
	input.TrackingMode = mode
	return nil
}

func parseCableID(r *http.Request) (int, error) {
	id, err := strconv.Atoi(mux.Vars(r)["id"])
	if err != nil || id <= 0 {
		return 0, errors.New("invalid cable ID")
	}
	return id, nil
}

func cableSelectQuery() string {
	return `
		SELECT cp.cable_product_id, cp.product_id, p.name,
		       cp.connector_a_id, cp.connector_b_id, cp.cable_type_id,
		       cp.length_m, cp.cross_section_mm2, cp.tracking_mode,
		       p.generic_barcode, COALESCE(p.stock_quantity, 0),
		       COALESCE(device_totals.total_count, 0),
		       COALESCE(device_totals.available_count, 0),
		       cc1.name, cc1.gender, cc2.name, cc2.gender, ct.name,
		       cp.migrated_from_legacy
		FROM cable_products cp
		JOIN products p ON p.productid = cp.product_id
		JOIN cable_connectors cc1 ON cc1.cable_connectorsid = cp.connector_a_id
		JOIN cable_connectors cc2 ON cc2.cable_connectorsid = cp.connector_b_id
		JOIN cable_types ct ON ct.cable_typesid = cp.cable_type_id
		LEFT JOIN brands b ON b.brandid = p.brandid
		LEFT JOIN manufacturer m ON m.manufacturerid = p.manufacturerid
		LEFT JOIN LATERAL (
			SELECT COUNT(*) AS total_count,
			       COUNT(*) FILTER (WHERE d.status='in_storage' AND d.condition_status='available') AS available_count
			FROM devices d
			WHERE d.productid = cp.product_id
		) device_totals ON TRUE
	`
}

func scanCable(scanner interface{ Scan(...interface{}) error }) (*Cable, error) {
	var cable Cable
	var productStock float64
	var availableUnits int
	if err := scanner.Scan(
		&cable.CableID,
		&cable.ProductID,
		&cable.Name,
		&cable.Connector1,
		&cable.Connector2,
		&cable.Typ,
		&cable.Length,
		&cable.MM2,
		&cable.TrackingMode,
		&cable.GenericBarcode,
		&productStock,
		&cable.UnitCount,
		&availableUnits,
		&cable.Connector1Name,
		&cable.Connector1Gender,
		&cable.Connector2Name,
		&cable.Connector2Gender,
		&cable.CableTypeName,
		&cable.MigratedFromLegacy,
	); err != nil {
		return nil, err
	}
	if cable.TrackingMode == cableTrackingIndividual {
		cable.StockQuantity = float64(cable.UnitCount)
		cable.AvailableQuantity = float64(availableUnits)
	} else {
		cable.StockQuantity = productStock
		cable.AvailableQuantity = productStock
	}
	return &cable, nil
}

func GetAllCables(w http.ResponseWriter, r *http.Request) {
	db := repository.GetSQLDB()
	query := cableSelectQuery() + ` WHERE 1=1`
	args := make([]interface{}, 0, 8)
	addArg := func(value interface{}) string {
		args = append(args, value)
		return fmt.Sprintf("$%d", len(args))
	}

	if search := strings.TrimSpace(r.URL.Query().Get("search")); search != "" {
		for _, term := range warehouseProductSearchTerms(search) {
			placeholder := addArg("%" + term + "%")
			query += fmt.Sprintf(` AND CONCAT_WS(' ',p.name,p.generic_barcode,b.name,m.name,cc1.name,cc2.name,
				ct.name,cp.length_m::text,cp.cross_section_mm2::text) ILIKE %s`, placeholder)
		}
	}
	if value, err := strconv.Atoi(r.URL.Query().Get("connector1")); err == nil && value > 0 {
		placeholder := addArg(value)
		query += fmt.Sprintf(" AND (cp.connector_a_id = %s OR cp.connector_b_id = %s)", placeholder, placeholder)
	}
	if value, err := strconv.Atoi(r.URL.Query().Get("connector2")); err == nil && value > 0 {
		placeholder := addArg(value)
		query += fmt.Sprintf(" AND (cp.connector_a_id = %s OR cp.connector_b_id = %s)", placeholder, placeholder)
	}
	if value, err := strconv.Atoi(r.URL.Query().Get("type")); err == nil && value > 0 {
		query += " AND cp.cable_type_id = " + addArg(value)
	}
	if value, err := strconv.ParseFloat(r.URL.Query().Get("length_min"), 64); err == nil && value >= 0 {
		query += " AND cp.length_m >= " + addArg(value)
	}
	if value, err := strconv.ParseFloat(r.URL.Query().Get("length_max"), 64); err == nil && value >= 0 {
		query += " AND cp.length_m <= " + addArg(value)
	}
	if mode := strings.TrimSpace(r.URL.Query().Get("tracking_mode")); mode != "" {
		query += " AND cp.tracking_mode = " + addArg(mode)
	}
	query += " ORDER BY ct.name, p.name, cp.length_m, cp.cable_product_id"

	rows, err := db.Query(query, args...)
	if err != nil {
		log.Printf("[CABLE LIST] query failed: %v", err)
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to fetch cable inventory"})
		return
	}
	defer rows.Close()

	cables := make([]Cable, 0)
	for rows.Next() {
		cable, err := scanCable(rows)
		if err != nil {
			log.Printf("[CABLE LIST] scan failed: %v", err)
			continue
		}
		cables = append(cables, *cable)
	}
	respondJSON(w, http.StatusOK, cables)
}

func GetCable(w http.ResponseWriter, r *http.Request) {
	id, err := parseCableID(r)
	if err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	cable, err := loadCable(repository.GetSQLDB(), id, true)
	if errors.Is(err, sql.ErrNoRows) {
		respondJSON(w, http.StatusNotFound, map[string]string{"error": "Cable product not found"})
		return
	}
	if err != nil {
		log.Printf("[CABLE GET] failed: %v", err)
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to fetch cable product"})
		return
	}
	respondJSON(w, http.StatusOK, cable)
}

func loadCable(db *sql.DB, id int, includeInventory bool) (*Cable, error) {
	cable, err := scanCable(db.QueryRow(cableSelectQuery()+` WHERE cp.cable_product_id = $1`, id))
	if err != nil {
		return nil, err
	}
	if !includeInventory {
		return cable, nil
	}

	stockRows, err := db.Query(`
		SELECT pl.zone_id, COALESCE(z.name, 'Ohne Lagerzone'), COALESCE(z.code, ''), pl.quantity
		FROM product_locations pl
		LEFT JOIN storage_zones z ON z.zone_id = pl.zone_id
		WHERE pl.product_id = $1
		ORDER BY z.name NULLS LAST
	`, cable.ProductID)
	if err != nil {
		return nil, err
	}
	cable.ZoneStocks = make([]CableStock, 0)
	for stockRows.Next() {
		var stock CableStock
		var zoneID sql.NullInt64
		if err := stockRows.Scan(&zoneID, &stock.ZoneName, &stock.ZoneCode, &stock.Quantity); err != nil {
			stockRows.Close()
			return nil, err
		}
		stock.ZoneID = nullIntToPtr(zoneID)
		cable.ZoneStocks = append(cable.ZoneStocks, stock)
	}
	stockRows.Close()

	unitRows, err := db.Query(`
		SELECT d.deviceid, d.barcode, d.qr_code, d.status, d.zone_id,
		       COALESCE(z.name, 'Ohne Lagerzone'), COALESCE(z.code, ''),
		       COALESCE(d.condition_rating, 5), assigned_job.jobid
		FROM devices d
		LEFT JOIN storage_zones z ON z.zone_id = d.zone_id
		LEFT JOIN LATERAL (
			SELECT jd.jobid
			FROM job_devices jd
			WHERE jd.deviceid = d.deviceid
			ORDER BY jd.jobid DESC
			LIMIT 1
		) assigned_job ON TRUE
		WHERE d.productid = $1
		ORDER BY d.deviceid
	`, cable.ProductID)
	if err != nil {
		return nil, err
	}
	cable.Units = make([]CableUnit, 0)
	for unitRows.Next() {
		var unit CableUnit
		var zoneID, jobID sql.NullInt64
		if err := unitRows.Scan(&unit.DeviceID, &unit.Barcode, &unit.QRCode, &unit.Status, &zoneID, &unit.ZoneName, &unit.ZoneCode, &unit.ConditionRating, &jobID); err != nil {
			unitRows.Close()
			return nil, err
		}
		unit.ZoneID = nullIntToPtr(zoneID)
		unit.CurrentJobID = nullIntToPtr(jobID)
		cable.Units = append(cable.Units, unit)
	}
	unitRows.Close()
	return cable, nil
}

func CreateCable(w http.ResponseWriter, r *http.Request) {
	var input cableCreateRequest
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid request body"})
		return
	}
	if err := validateCableCreateRequest(&input); err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	db := repository.GetSQLDB()
	tx, err := db.Begin()
	if err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to start transaction"})
		return
	}
	defer tx.Rollback()

	name, err := resolveCableName(tx, input.Name, input.Typ, input.Connector1, input.Connector2, input.Length)
	if err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	countTypeID, err := ensurePieceCountType(tx)
	if err != nil {
		log.Printf("[CABLE CREATE] count type failed: %v", err)
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to prepare cable product"})
		return
	}

	var productID int
	if err := tx.QueryRow(`
		INSERT INTO products (name, is_accessory, is_consumable, count_type_id, stock_quantity, created_at, updated_at)
		VALUES ($1, TRUE, FALSE, $2, 0, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
		RETURNING productid
	`, name, countTypeID).Scan(&productID); err != nil {
		log.Printf("[CABLE CREATE] product insert failed: %v", err)
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to create cable product"})
		return
	}

	barcode := strings.TrimSpace(input.GenericBarcode)
	if barcode == "" {
		barcode = cableProductBarcode(productID)
	}
	if err := ensureProductBarcodeAvailable(tx, barcode, productID); err != nil {
		respondJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
		return
	}
	if _, err := tx.Exec(`UPDATE products SET generic_barcode = $1 WHERE productid = $2`, barcode, productID); err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to assign product barcode"})
		return
	}

	var cableID int
	if err := tx.QueryRow(`
		INSERT INTO cable_products (
			product_id, connector_a_id, connector_b_id, cable_type_id,
			length_m, cross_section_mm2, tracking_mode
		) VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING cable_product_id
	`, productID, input.Connector1, input.Connector2, input.Typ, input.Length, nullableFloatPtr(input.MM2), input.TrackingMode).Scan(&cableID); err != nil {
		log.Printf("[CABLE CREATE] specification insert failed: %v", err)
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid cable specification"})
		return
	}

	if input.Quantity > 0 {
		if input.TrackingMode == cableTrackingIndividual {
			if err := createCableUnits(tx, productID, input.Quantity, input.ZoneID); err != nil {
				log.Printf("[CABLE CREATE] unit creation failed: %v", err)
				respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to create individual cables"})
				return
			}
		} else if err := setCableStock(tx, productID, input.ZoneID, float64(input.Quantity)); err != nil {
			log.Printf("[CABLE CREATE] stock creation failed: %v", err)
			respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to create cable stock"})
			return
		}
	}

	if err := syncCableProductStock(tx, productID, input.TrackingMode); err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to synchronize cable stock"})
		return
	}
	if err := tx.Commit(); err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to commit cable product"})
		return
	}
	respondJSON(w, http.StatusCreated, map[string]interface{}{"cable_id": cableID, "product_id": productID, "message": "Cable product created successfully"})
}

func UpdateCable(w http.ResponseWriter, r *http.Request) {
	id, err := parseCableID(r)
	if err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	var input cableUpdateRequest
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid request body"})
		return
	}
	if input.Length != nil && *input.Length <= 0 {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "Length must be greater than 0"})
		return
	}
	if input.MM2.Set && input.MM2.Valid && input.MM2.Value <= 0 {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "mm2 must be greater than 0"})
		return
	}

	db := repository.GetSQLDB()
	tx, err := db.Begin()
	if err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to start transaction"})
		return
	}
	defer tx.Rollback()

	var productID int
	var currentMode string
	if err := tx.QueryRow(`SELECT product_id, tracking_mode FROM cable_products WHERE cable_product_id = $1 FOR UPDATE`, id).Scan(&productID, &currentMode); errors.Is(err, sql.ErrNoRows) {
		respondJSON(w, http.StatusNotFound, map[string]string{"error": "Cable product not found"})
		return
	} else if err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to load cable product"})
		return
	}

	if input.TrackingMode != nil {
		mode, err := normalizeCableTrackingMode(*input.TrackingMode)
		if err != nil {
			respondJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		if mode != currentMode {
			var quantity float64
			var unitCount int
			if err := tx.QueryRow(`SELECT COALESCE(stock_quantity, 0), (SELECT COUNT(*) FROM devices WHERE productid = $1) FROM products WHERE productid = $1`, productID).Scan(&quantity, &unitCount); err != nil {
				respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to validate tracking mode"})
				return
			}
			if quantity > 0 || unitCount > 0 {
				respondJSON(w, http.StatusConflict, map[string]string{"error": "Tracking mode can only be changed when the cable has no stock"})
				return
			}
			if _, err := tx.Exec(`UPDATE cable_products SET tracking_mode = $1 WHERE cable_product_id = $2`, mode, id); err != nil {
				respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to update tracking mode"})
				return
			}
			currentMode = mode
		}
	}

	updates := make([]string, 0, 5)
	args := make([]interface{}, 0, 6)
	add := func(column string, value interface{}) {
		args = append(args, value)
		updates = append(updates, fmt.Sprintf("%s = $%d", column, len(args)))
	}
	if input.Connector1 != nil {
		add("connector_a_id", *input.Connector1)
	}
	if input.Connector2 != nil {
		add("connector_b_id", *input.Connector2)
	}
	if input.Typ != nil {
		add("cable_type_id", *input.Typ)
	}
	if input.Length != nil {
		add("length_m", *input.Length)
	}
	if input.MM2.Set {
		if input.MM2.Valid {
			add("cross_section_mm2", input.MM2.Value)
		} else {
			add("cross_section_mm2", nil)
		}
	}
	if len(updates) > 0 {
		args = append(args, id)
		query := fmt.Sprintf("UPDATE cable_products SET %s, updated_at = CURRENT_TIMESTAMP WHERE cable_product_id = $%d", strings.Join(updates, ", "), len(args))
		if _, err := tx.Exec(query, args...); err != nil {
			respondJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid cable specification"})
			return
		}
	}

	if input.Name != nil {
		name := strings.TrimSpace(*input.Name)
		if name == "" {
			var typeName, connectorA, connectorB string
			var length float64
			if err := tx.QueryRow(`
				SELECT ct.name, COALESCE(NULLIF(ca.abbreviation, ''), ca.name), COALESCE(NULLIF(cb.abbreviation, ''), cb.name), cp.length_m
				FROM cable_products cp
				JOIN cable_types ct ON ct.cable_typesid = cp.cable_type_id
				JOIN cable_connectors ca ON ca.cable_connectorsid = cp.connector_a_id
				JOIN cable_connectors cb ON cb.cable_connectorsid = cp.connector_b_id
				WHERE cp.cable_product_id = $1
			`, id).Scan(&typeName, &connectorA, &connectorB, &length); err != nil {
				respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to generate cable name"})
				return
			}
			name = buildCableProductName(typeName, connectorA, connectorB, length)
		}
		if _, err := tx.Exec(`UPDATE products SET name = $1, updated_at = CURRENT_TIMESTAMP WHERE productid = $2`, name, productID); err != nil {
			respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to update cable name"})
			return
		}
	}
	if input.GenericBarcode != nil {
		barcode := strings.TrimSpace(*input.GenericBarcode)
		if barcode == "" {
			barcode = cableProductBarcode(productID)
		}
		if err := ensureProductBarcodeAvailable(tx, barcode, productID); err != nil {
			respondJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
			return
		}
		if _, err := tx.Exec(`UPDATE products SET generic_barcode = $1, updated_at = CURRENT_TIMESTAMP WHERE productid = $2`, barcode, productID); err != nil {
			respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to update product barcode"})
			return
		}
	}
	if err := syncCableProductStock(tx, productID, currentMode); err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to synchronize cable stock"})
		return
	}
	if err := tx.Commit(); err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to commit cable update"})
		return
	}
	respondJSON(w, http.StatusOK, map[string]string{"message": "Cable product updated successfully"})
}

func DeleteCable(w http.ResponseWriter, r *http.Request) {
	id, err := parseCableID(r)
	if err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	db := repository.GetSQLDB()
	var productID int
	var stock float64
	var units int
	err = db.QueryRow(`
		SELECT cp.product_id, COALESCE(p.stock_quantity, 0), (SELECT COUNT(*) FROM devices d WHERE d.productid = cp.product_id)
		FROM cable_products cp JOIN products p ON p.productid = cp.product_id
		WHERE cp.cable_product_id = $1
	`, id).Scan(&productID, &stock, &units)
	if errors.Is(err, sql.ErrNoRows) {
		respondJSON(w, http.StatusNotFound, map[string]string{"error": "Cable product not found"})
		return
	}
	if err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to load cable product"})
		return
	}
	if stock > 0 || units > 0 {
		respondJSON(w, http.StatusConflict, map[string]string{"error": "Cable product can only be deleted when its stock is zero"})
		return
	}
	result, err := db.Exec(`DELETE FROM products WHERE productid = $1`, productID)
	if err != nil {
		respondJSON(w, http.StatusConflict, map[string]string{"error": "Cable product is still used by a job or package"})
		return
	}
	if affected, _ := result.RowsAffected(); affected == 0 {
		respondJSON(w, http.StatusNotFound, map[string]string{"error": "Cable product not found"})
		return
	}
	respondJSON(w, http.StatusOK, map[string]string{"message": "Cable product deleted successfully"})
}

func SetCableStock(w http.ResponseWriter, r *http.Request) {
	id, err := parseCableID(r)
	if err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	var input cableStockRequest
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil || input.Quantity < 0 {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "quantity must be zero or greater"})
		return
	}
	db := repository.GetSQLDB()
	tx, err := db.Begin()
	if err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to start transaction"})
		return
	}
	defer tx.Rollback()
	var productID int
	var mode string
	if err := tx.QueryRow(`SELECT product_id, tracking_mode FROM cable_products WHERE cable_product_id = $1 FOR UPDATE`, id).Scan(&productID, &mode); errors.Is(err, sql.ErrNoRows) {
		respondJSON(w, http.StatusNotFound, map[string]string{"error": "Cable product not found"})
		return
	} else if err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to load cable product"})
		return
	}
	if mode != cableTrackingQuantity {
		respondJSON(w, http.StatusConflict, map[string]string{"error": "Stock levels can only be set for quantity-tracked cables"})
		return
	}
	if err := setCableStock(tx, productID, input.ZoneID, input.Quantity); err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to update cable stock"})
		return
	}
	if err := syncCableProductStock(tx, productID, mode); err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to synchronize cable stock"})
		return
	}
	if err := tx.Commit(); err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to commit stock update"})
		return
	}
	respondJSON(w, http.StatusOK, map[string]string{"message": "Cable stock updated successfully"})
}

func CreateCableUnits(w http.ResponseWriter, r *http.Request) {
	id, err := parseCableID(r)
	if err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	var input cableUnitRequest
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil || input.Quantity <= 0 || input.Quantity > 500 {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "quantity must be between 1 and 500"})
		return
	}
	db := repository.GetSQLDB()
	tx, err := db.Begin()
	if err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to start transaction"})
		return
	}
	defer tx.Rollback()
	var productID int
	var mode string
	if err := tx.QueryRow(`SELECT product_id, tracking_mode FROM cable_products WHERE cable_product_id = $1 FOR UPDATE`, id).Scan(&productID, &mode); errors.Is(err, sql.ErrNoRows) {
		respondJSON(w, http.StatusNotFound, map[string]string{"error": "Cable product not found"})
		return
	} else if err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to load cable product"})
		return
	}
	if mode != cableTrackingIndividual {
		respondJSON(w, http.StatusConflict, map[string]string{"error": "Units can only be created for individually tracked cables"})
		return
	}
	if err := createCableUnits(tx, productID, input.Quantity, input.ZoneID); err != nil {
		log.Printf("[CABLE UNITS] creation failed: %v", err)
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to create cable units"})
		return
	}
	if err := syncCableProductStock(tx, productID, mode); err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to synchronize cable units"})
		return
	}
	if err := tx.Commit(); err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to commit cable units"})
		return
	}
	respondJSON(w, http.StatusCreated, map[string]interface{}{"created_count": input.Quantity, "message": "Cable units created successfully"})
}

func DeleteCableUnit(w http.ResponseWriter, r *http.Request) {
	id, err := parseCableID(r)
	if err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	deviceID := strings.TrimSpace(mux.Vars(r)["device_id"])
	if deviceID == "" {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid device ID"})
		return
	}
	db := repository.GetSQLDB()
	tx, err := db.Begin()
	if err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to start transaction"})
		return
	}
	defer tx.Rollback()
	var productID int
	if err := tx.QueryRow(`SELECT product_id FROM cable_products WHERE cable_product_id = $1`, id).Scan(&productID); errors.Is(err, sql.ErrNoRows) {
		respondJSON(w, http.StatusNotFound, map[string]string{"error": "Cable product not found"})
		return
	} else if err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to load cable product"})
		return
	}
	var references int
	if err := tx.QueryRow(`
		SELECT (SELECT COUNT(*) FROM job_devices WHERE deviceid = $1) +
		       (SELECT COUNT(*) FROM devicescases WHERE deviceid = $1)
	`, deviceID).Scan(&references); err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to validate cable unit"})
		return
	}
	if references > 0 {
		respondJSON(w, http.StatusConflict, map[string]string{"error": "Cable unit is assigned to a job or case"})
		return
	}
	result, err := tx.Exec(`DELETE FROM devices WHERE deviceid = $1 AND productid = $2`, deviceID, productID)
	if err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to delete cable unit"})
		return
	}
	if affected, _ := result.RowsAffected(); affected == 0 {
		respondJSON(w, http.StatusNotFound, map[string]string{"error": "Cable unit not found"})
		return
	}
	if err := syncCableProductStock(tx, productID, cableTrackingIndividual); err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to synchronize cable units"})
		return
	}
	if err := tx.Commit(); err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to commit cable unit deletion"})
		return
	}
	respondJSON(w, http.StatusOK, map[string]string{"message": "Cable unit deleted successfully"})
}

func ensurePieceCountType(tx *sql.Tx) (int, error) {
	var id int
	err := tx.QueryRow(`
		INSERT INTO count_types (name, abbreviation, is_decimal)
		VALUES ('Stück', 'Stk', FALSE)
		ON CONFLICT (name) DO UPDATE SET abbreviation = COALESCE(count_types.abbreviation, EXCLUDED.abbreviation)
		RETURNING count_type_id
	`).Scan(&id)
	return id, err
}

func resolveCableName(tx *sql.Tx, requested string, typeID, connectorAID, connectorBID int, length float64) (string, error) {
	if name := strings.TrimSpace(requested); name != "" {
		return name, nil
	}
	var typeName, connectorA, connectorB string
	err := tx.QueryRow(`
		SELECT ct.name,
		       COALESCE(NULLIF(ca.abbreviation, ''), ca.name),
		       COALESCE(NULLIF(cb.abbreviation, ''), cb.name)
		FROM cable_types ct, cable_connectors ca, cable_connectors cb
		WHERE ct.cable_typesid = $1 AND ca.cable_connectorsid = $2 AND cb.cable_connectorsid = $3
	`, typeID, connectorAID, connectorBID).Scan(&typeName, &connectorA, &connectorB)
	if errors.Is(err, sql.ErrNoRows) {
		return "", errors.New("unknown cable type or connector")
	}
	if err != nil {
		return "", err
	}
	return buildCableProductName(typeName, connectorA, connectorB, length), nil
}

func ensureProductBarcodeAvailable(tx *sql.Tx, barcode string, productID int) error {
	var existing int
	err := tx.QueryRow(`SELECT productid FROM products WHERE generic_barcode = $1 AND productid <> $2 LIMIT 1`, barcode, productID).Scan(&existing)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	return fmt.Errorf("barcode %q is already used by another product", barcode)
}

func setCableStock(tx *sql.Tx, productID int, zoneID *int, quantity float64) error {
	if quantity == 0 {
		_, err := tx.Exec(`DELETE FROM product_locations WHERE product_id = $1 AND zone_id IS NOT DISTINCT FROM $2`, productID, nullableIntPtr(zoneID))
		return err
	}
	_, err := tx.Exec(`
		INSERT INTO product_locations (product_id, zone_id, quantity, updated_at)
		VALUES ($1, $2, $3, CURRENT_TIMESTAMP)
		ON CONFLICT (product_id, zone_id) DO UPDATE
		SET quantity = EXCLUDED.quantity, updated_at = CURRENT_TIMESTAMP
	`, productID, nullableIntPtr(zoneID), quantity)
	return err
}

func syncCableProductStock(tx *sql.Tx, productID int, mode string) error {
	var query string
	if mode == cableTrackingIndividual {
		query = `UPDATE products SET stock_quantity = (SELECT COUNT(*) FROM devices WHERE productid = $1), updated_at = CURRENT_TIMESTAMP WHERE productid = $1`
	} else {
		query = `UPDATE products SET stock_quantity = (SELECT COALESCE(SUM(quantity), 0) FROM product_locations WHERE product_id = $1), updated_at = CURRENT_TIMESTAMP WHERE productid = $1`
	}
	_, err := tx.Exec(query, productID)
	return err
}

func createCableUnits(tx *sql.Tx, productID, quantity int, zoneID *int) error {
	if _, err := tx.Exec(`SELECT pg_advisory_xact_lock($1)`, productID); err != nil {
		return err
	}
	var nextNumber int
	pattern := fmt.Sprintf("^CAB-%d-[0-9]+$", productID)
	err := tx.QueryRow(`
		SELECT COALESCE(MAX((regexp_match(deviceid, '([0-9]+)$'))[1]::INT), 0) + 1
		FROM devices
		WHERE productid = $1 AND deviceid ~ $2
	`, productID, pattern).Scan(&nextNumber)
	if err != nil {
		return err
	}
	status := "location_unknown"
	currentLocation := interface{}("location_unknown")
	if zoneID != nil {
		status = "in_storage"
		currentLocation = "warehouse"
	}
	for index := 0; index < quantity; index++ {
		deviceID := fmt.Sprintf("CAB-%d-%04d", productID, nextNumber+index)
		_, err := tx.Exec(`
			INSERT INTO devices (
				deviceid, productid, status, barcode, qr_code,
				current_location, zone_id, condition_rating, usage_hours,
				created_at, updated_at
			) VALUES ($1, $2, $3, $1, $4, $5, $6, 5, 0, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
		`, deviceID, productID, status, "QR-"+deviceID, currentLocation, nullableIntPtr(zoneID))
		if err != nil {
			return err
		}
	}
	return nil
}

func GetCableConnectors(w http.ResponseWriter, _ *http.Request) {
	rows, err := repository.GetSQLDB().Query(`SELECT cable_connectorsid, name, abbreviation, gender FROM cable_connectors ORDER BY name, gender`)
	if err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to fetch cable connectors"})
		return
	}
	defer rows.Close()
	connectors := make([]CableConnector, 0)
	for rows.Next() {
		var connector CableConnector
		if err := rows.Scan(&connector.ConnectorID, &connector.Name, &connector.Abbreviation, &connector.Gender); err == nil {
			connectors = append(connectors, connector)
		}
	}
	respondJSON(w, http.StatusOK, connectors)
}

func GetCableTypes(w http.ResponseWriter, _ *http.Request) {
	rows, err := repository.GetSQLDB().Query(`
		SELECT ct.cable_typesid, ct.name, COUNT(cp.cable_product_id)
		FROM cable_types ct
		LEFT JOIN cable_products cp ON cp.cable_type_id = ct.cable_typesid
		GROUP BY ct.cable_typesid, ct.name
		ORDER BY ct.name
	`)
	if err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to fetch cable types"})
		return
	}
	defer rows.Close()
	types := make([]CableType, 0)
	for rows.Next() {
		var cableType CableType
		if err := rows.Scan(&cableType.CableTypeID, &cableType.Name, &cableType.Count); err == nil {
			types = append(types, cableType)
		}
	}
	respondJSON(w, http.StatusOK, types)
}

func CreateCableType(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil || strings.TrimSpace(input.Name) == "" {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "Name is required"})
		return
	}
	var id int
	if err := repository.GetSQLDB().QueryRow(`INSERT INTO cable_types (name) VALUES ($1) RETURNING cable_typesid`, strings.TrimSpace(input.Name)).Scan(&id); err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to create cable type"})
		return
	}
	respondJSON(w, http.StatusCreated, CableType{CableTypeID: id, Name: strings.TrimSpace(input.Name)})
}

func UpdateCableType(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(mux.Vars(r)["id"])
	var input struct {
		Name string `json:"name"`
	}
	if err != nil || json.NewDecoder(r.Body).Decode(&input) != nil || strings.TrimSpace(input.Name) == "" {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "Valid ID and name are required"})
		return
	}
	result, err := repository.GetSQLDB().Exec(`UPDATE cable_types SET name = $1 WHERE cable_typesid = $2`, strings.TrimSpace(input.Name), id)
	if err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to update cable type"})
		return
	}
	if affected, _ := result.RowsAffected(); affected == 0 {
		respondJSON(w, http.StatusNotFound, map[string]string{"error": "Cable type not found"})
		return
	}
	respondJSON(w, http.StatusOK, map[string]string{"message": "Cable type updated successfully"})
}

func DeleteCableType(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(mux.Vars(r)["id"])
	if err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid cable type ID"})
		return
	}
	result, err := repository.GetSQLDB().Exec(`DELETE FROM cable_types WHERE cable_typesid = $1`, id)
	if isForeignKeyViolation(err) {
		respondJSON(w, http.StatusConflict, map[string]string{"error": "Cable type is still in use"})
		return
	}
	if err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to delete cable type"})
		return
	}
	if affected, _ := result.RowsAffected(); affected == 0 {
		respondJSON(w, http.StatusNotFound, map[string]string{"error": "Cable type not found"})
		return
	}
	respondJSON(w, http.StatusOK, map[string]string{"message": "Cable type deleted successfully"})
}

func CreateCableConnector(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Name         string  `json:"name"`
		Abbreviation *string `json:"abbreviation"`
		Gender       *string `json:"gender"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil || strings.TrimSpace(input.Name) == "" {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "Name is required"})
		return
	}
	var id int
	err := repository.GetSQLDB().QueryRow(`
		INSERT INTO cable_connectors (name, abbreviation, gender) VALUES ($1, $2, $3) RETURNING cable_connectorsid
	`, strings.TrimSpace(input.Name), nullableStringPtr(input.Abbreviation), nullableStringPtr(input.Gender)).Scan(&id)
	if err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to create cable connector"})
		return
	}
	respondJSON(w, http.StatusCreated, CableConnector{ConnectorID: id, Name: strings.TrimSpace(input.Name), Abbreviation: input.Abbreviation, Gender: input.Gender})
}

func UpdateCableConnector(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(mux.Vars(r)["id"])
	var input struct {
		Name         string  `json:"name"`
		Abbreviation *string `json:"abbreviation"`
		Gender       *string `json:"gender"`
	}
	if err != nil || json.NewDecoder(r.Body).Decode(&input) != nil || strings.TrimSpace(input.Name) == "" {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "Valid ID and name are required"})
		return
	}
	result, err := repository.GetSQLDB().Exec(`
		UPDATE cable_connectors SET name = $1, abbreviation = $2, gender = $3 WHERE cable_connectorsid = $4
	`, strings.TrimSpace(input.Name), nullableStringPtr(input.Abbreviation), nullableStringPtr(input.Gender), id)
	if err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to update cable connector"})
		return
	}
	if affected, _ := result.RowsAffected(); affected == 0 {
		respondJSON(w, http.StatusNotFound, map[string]string{"error": "Cable connector not found"})
		return
	}
	respondJSON(w, http.StatusOK, map[string]string{"message": "Cable connector updated successfully"})
}

func DeleteCableConnector(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(mux.Vars(r)["id"])
	if err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid cable connector ID"})
		return
	}
	result, err := repository.GetSQLDB().Exec(`DELETE FROM cable_connectors WHERE cable_connectorsid = $1`, id)
	if isForeignKeyViolation(err) {
		respondJSON(w, http.StatusConflict, map[string]string{"error": "Cable connector is still in use"})
		return
	}
	if err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to delete cable connector"})
		return
	}
	if affected, _ := result.RowsAffected(); affected == 0 {
		respondJSON(w, http.StatusNotFound, map[string]string{"error": "Cable connector not found"})
		return
	}
	respondJSON(w, http.StatusOK, map[string]string{"message": "Cable connector deleted successfully"})
}

func isForeignKeyViolation(err error) bool {
	var pqError *pq.Error
	return errors.As(err, &pqError) && pqError.Code == "23503"
}
