package handlers

import (
	"bytes"
	"encoding/csv"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/gorilla/mux"
	"warehousecore/internal/repository"
)

// UTF-8 BOM for Excel compatibility
var utf8BOM = []byte{0xEF, 0xBB, 0xBF}

// ExportCSV handles CSV export requests based on export type
func ExportCSV(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	exportType := vars["type"]

	var csvData []byte
	var filename string
	var err error

	timestamp := time.Now().Format("2006-01-02")

	switch exportType {
	case "products":
		csvData, err = exportProducts()
		filename = fmt.Sprintf("produkte_%s.csv", timestamp)
	case "products-with-count":
		csvData, err = exportProductsWithDeviceCount()
		filename = fmt.Sprintf("produkte_mit_anzahl_%s.csv", timestamp)
	case "products-with-brand":
		csvData, err = exportProductsWithBrandManufacturer()
		filename = fmt.Sprintf("produkte_mit_marke_%s.csv", timestamp)
	case "devices":
		csvData, err = exportAllDevices()
		filename = fmt.Sprintf("geraete_%s.csv", timestamp)
	case "manufacturers":
		csvData, err = exportManufacturers()
		filename = fmt.Sprintf("hersteller_%s.csv", timestamp)
	case "manufacturers-with-brands":
		csvData, err = exportManufacturersWithBrands()
		filename = fmt.Sprintf("hersteller_mit_marken_%s.csv", timestamp)
	case "brands":
		csvData, err = exportBrands()
		filename = fmt.Sprintf("marken_%s.csv", timestamp)
	case "zones":
		csvData, err = exportStorageZones()
		filename = fmt.Sprintf("lagerbereiche_%s.csv", timestamp)
	case "cables":
		csvData, err = exportCables()
		filename = fmt.Sprintf("kabel_%s.csv", timestamp)
	case "jobs":
		csvData, err = exportJobs()
		filename = fmt.Sprintf("jobs_%s.csv", timestamp)
	default:
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid export type"})
		return
	}

	if err != nil {
		log.Printf("[EXPORT] Error generating CSV for type %s: %v", exportType, err)
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	// Set headers for CSV download
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%s", filename))
	w.Header().Set("Content-Length", strconv.Itoa(len(csvData)))

	// Write UTF-8 BOM + CSV data
	w.Write(csvData)
}

// Helper function to create CSV with BOM
func createCSV(headers []string, rows [][]string) ([]byte, error) {
	var buf bytes.Buffer

	// Write UTF-8 BOM for Excel compatibility
	buf.Write(utf8BOM)

	writer := csv.NewWriter(&buf)
	writer.Comma = ';' // Use semicolon for German CSV format

	// Write headers
	if err := writer.Write(headers); err != nil {
		return nil, err
	}

	// Write rows
	for _, row := range rows {
		if err := writer.Write(row); err != nil {
			return nil, err
		}
	}

	writer.Flush()
	if err := writer.Error(); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

// formatBool converts boolean to German Ja/Nein
func formatBool(b bool) string {
	if b {
		return "Ja"
	}
	return "Nein"
}

// formatNullString handles NULL string values
func formatNullString(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// formatNullInt handles NULL int values
func formatNullInt(i *int) string {
	if i == nil {
		return ""
	}
	return strconv.Itoa(*i)
}

// formatNullFloat handles NULL float values with German decimal separator
func formatNullFloat(f *float64) string {
	if f == nil {
		return ""
	}
	// Use comma as decimal separator for German format
	return fmt.Sprintf("%.2f", *f)
}

// formatDate formats time to German date format
func formatDate(t *time.Time) string {
	if t == nil {
		return ""
	}
	return t.Format("02.01.2006 15:04")
}

// ============================
// EXPORT FUNCTIONS
// ============================

// exportProducts exports all products with basic information
func exportProducts() ([]byte, error) {
	db := repository.GetSQLDB()

	query := `
		SELECT
			p.productID,
			p.name,
			p.description,
			c.name as category_name,
			sc.name as subcategory_name,
			p.is_accessory,
			p.is_consumable,
			p.itemcostperday,
			p.weight,
			p.height,
			p.width,
			p.depth,
			p.powerconsumption,
			p.generic_barcode
		FROM products p
		LEFT JOIN categories c ON p.categoryID = c.categoryid
		LEFT JOIN subcategories sc ON p.subcategoryID = sc.subcategoryid
		ORDER BY p.name
	`

	rows, err := db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	headers := []string{
		"Produkt-ID", "Name", "Beschreibung", "Kategorie", "Unterkategorie",
		"Zubehör", "Verbrauchsmaterial", "Kosten/Tag", "Gewicht (kg)", "Höhe (cm)",
		"Breite (cm)", "Tiefe (cm)", "Stromverbrauch (W)", "Barcode",
	}

	var csvRows [][]string

	for rows.Next() {
		var productID int
		var name string
		var description, categoryName, subcategoryName, genericBarcode *string
		var isAccessory, isConsumable bool
		var itemCostPerDay, weight, height, width, depth, powerConsumption *float64

		err := rows.Scan(
			&productID, &name, &description, &categoryName, &subcategoryName,
			&isAccessory, &isConsumable, &itemCostPerDay, &weight, &height,
			&width, &depth, &powerConsumption, &genericBarcode,
		)
		if err != nil {
			return nil, err
		}

		csvRows = append(csvRows, []string{
			strconv.Itoa(productID),
			name,
			formatNullString(description),
			formatNullString(categoryName),
			formatNullString(subcategoryName),
			formatBool(isAccessory),
			formatBool(isConsumable),
			formatNullFloat(itemCostPerDay),
			formatNullFloat(weight),
			formatNullFloat(height),
			formatNullFloat(width),
			formatNullFloat(depth),
			formatNullFloat(powerConsumption),
			formatNullString(genericBarcode),
		})
	}

	return createCSV(headers, csvRows)
}

// exportProductsWithDeviceCount exports products with their device counts
func exportProductsWithDeviceCount() ([]byte, error) {
	db := repository.GetSQLDB()

	query := `
		SELECT
			p.productID,
			p.name,
			c.name as category_name,
			COUNT(d.deviceID) as device_count,
			SUM(CASE WHEN d.status = 'available' THEN 1 ELSE 0 END) as available_count,
			SUM(CASE WHEN d.status = 'in_use' THEN 1 ELSE 0 END) as in_use_count,
			SUM(CASE WHEN d.status = 'defect' THEN 1 ELSE 0 END) as defect_count
		FROM products p
		LEFT JOIN categories c ON p.categoryID = c.categoryid
		LEFT JOIN devices d ON p.productID = d.productID
		GROUP BY p.productID, p.name, c.name
		ORDER BY p.name
	`

	rows, err := db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	headers := []string{
		"Produkt-ID", "Name", "Kategorie", "Gesamt Geräte",
		"Verfügbar", "Im Einsatz", "Defekt",
	}

	var csvRows [][]string

	for rows.Next() {
		var productID int
		var name string
		var categoryName *string
		var deviceCount, availableCount, inUseCount, defectCount int

		err := rows.Scan(
			&productID, &name, &categoryName, &deviceCount,
			&availableCount, &inUseCount, &defectCount,
		)
		if err != nil {
			return nil, err
		}

		csvRows = append(csvRows, []string{
			strconv.Itoa(productID),
			name,
			formatNullString(categoryName),
			strconv.Itoa(deviceCount),
			strconv.Itoa(availableCount),
			strconv.Itoa(inUseCount),
			strconv.Itoa(defectCount),
		})
	}

	return createCSV(headers, csvRows)
}

// exportProductsWithBrandManufacturer exports products with brand and manufacturer info
func exportProductsWithBrandManufacturer() ([]byte, error) {
	db := repository.GetSQLDB()

	query := `
		SELECT
			p.productID,
			p.name,
			c.name as category_name,
			m.name as manufacturer_name,
			b.name as brand_name,
			p.description,
			p.itemcostperday
		FROM products p
		LEFT JOIN categories c ON p.categoryID = c.categoryid
		LEFT JOIN manufacturer m ON p.manufacturerID = m.manufacturerid
		LEFT JOIN brands b ON p.brandID = b.brandid
		ORDER BY p.name
	`

	rows, err := db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	headers := []string{
		"Produkt-ID", "Name", "Kategorie", "Hersteller", "Marke",
		"Beschreibung", "Kosten/Tag",
	}

	var csvRows [][]string

	for rows.Next() {
		var productID int
		var name string
		var categoryName, manufacturerName, brandName, description *string
		var itemCostPerDay *float64

		err := rows.Scan(
			&productID, &name, &categoryName, &manufacturerName,
			&brandName, &description, &itemCostPerDay,
		)
		if err != nil {
			return nil, err
		}

		csvRows = append(csvRows, []string{
			strconv.Itoa(productID),
			name,
			formatNullString(categoryName),
			formatNullString(manufacturerName),
			formatNullString(brandName),
			formatNullString(description),
			formatNullFloat(itemCostPerDay),
		})
	}

	return createCSV(headers, csvRows)
}

// exportAllDevices exports all devices with full details
func exportAllDevices() ([]byte, error) {
	db := repository.GetSQLDB()

	query := `
		SELECT
			d.deviceid,
			COALESCE(p.name, '') as product_name,
			d.serialnumber,
			d.status,
			d.purchasedate,
			d.lastmaintenance,
			d.notes,
			z.name as zone_name,
			c.name as case_name
		FROM devices d
		LEFT JOIN products p ON d.productid = p.productid
		LEFT JOIN storage_zones z ON d.zone_id = z.zone_id
		LEFT JOIN cases c ON d.current_case_id = c.caseid
		ORDER BY d.deviceid
	`

	rows, err := db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	headers := []string{
		"Geräte-ID", "Produkt", "Seriennummer", "Status", "Kaufdatum",
		"Letzte Wartung", "Notizen", "Lagerbereich", "Case",
	}

	var csvRows [][]string

	for rows.Next() {
		var deviceID, productName string
		var serialNumber, notes, zoneName, caseName *string
		var status string
		var purchaseDate, lastMaintenanceDate *time.Time

		err := rows.Scan(
			&deviceID, &productName, &serialNumber, &status, &purchaseDate,
			&lastMaintenanceDate, &notes, &zoneName, &caseName,
		)
		if err != nil {
			return nil, err
		}

		csvRows = append(csvRows, []string{
			deviceID,
			productName,
			formatNullString(serialNumber),
			status,
			formatDate(purchaseDate),
			formatDate(lastMaintenanceDate),
			formatNullString(notes),
			formatNullString(zoneName),
			formatNullString(caseName),
		})
	}

	return createCSV(headers, csvRows)
}

// exportManufacturers exports all manufacturers
func exportManufacturers() ([]byte, error) {
	db := repository.GetSQLDB()

	query := `
		SELECT
			manufacturerid,
			name,
			website
		FROM manufacturer
		ORDER BY name
	`

	rows, err := db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	headers := []string{
		"ID", "Name", "Webseite",
	}

	var csvRows [][]string

	for rows.Next() {
		var id int
		var name string
		var website *string

		err := rows.Scan(&id, &name, &website)
		if err != nil {
			return nil, err
		}

		csvRows = append(csvRows, []string{
			strconv.Itoa(id),
			name,
			formatNullString(website),
		})
	}

	return createCSV(headers, csvRows)
}

// exportManufacturersWithBrands exports manufacturers with their brands
func exportManufacturersWithBrands() ([]byte, error) {
	db := repository.GetSQLDB()

	query := `
		SELECT
			m.manufacturerid,
			m.name as manufacturer_name,
			STRING_AGG(b.name, ', ') as brands
		FROM manufacturer m
		LEFT JOIN brands b ON m.manufacturerid = b.manufacturerid
		GROUP BY m.manufacturerid, m.name
		ORDER BY m.name
	`

	rows, err := db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	headers := []string{
		"ID", "Hersteller", "Marken",
	}

	var csvRows [][]string

	for rows.Next() {
		var id int
		var manufacturerName string
		var brands *string

		err := rows.Scan(&id, &manufacturerName, &brands)
		if err != nil {
			return nil, err
		}

		csvRows = append(csvRows, []string{
			strconv.Itoa(id),
			manufacturerName,
			formatNullString(brands),
		})
	}

	return createCSV(headers, csvRows)
}

// exportBrands exports all brands
func exportBrands() ([]byte, error) {
	db := repository.GetSQLDB()

	query := `
		SELECT
			b.brandid,
			b.name,
			m.name as manufacturer_name
		FROM brands b
		LEFT JOIN manufacturer m ON b.manufacturerid = m.manufacturerid
		ORDER BY b.name
	`

	rows, err := db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	headers := []string{
		"ID", "Markenname", "Hersteller",
	}

	var csvRows [][]string

	for rows.Next() {
		var id int
		var name string
		var manufacturerName *string

		err := rows.Scan(&id, &name, &manufacturerName)
		if err != nil {
			return nil, err
		}

		csvRows = append(csvRows, []string{
			strconv.Itoa(id),
			name,
			formatNullString(manufacturerName),
		})
	}

	return createCSV(headers, csvRows)
}

// exportStorageZones exports all storage zones with details
func exportStorageZones() ([]byte, error) {
	db := repository.GetSQLDB()

	query := `
		SELECT
			z.zone_id,
			z.name,
			z.type::text as zone_type,
			z.barcode,
			z.capacity,
			z.location,
			z.description,
			COUNT(DISTINCT d.deviceID) as device_count
		FROM storage_zones z
		LEFT JOIN devices d ON z.zone_id = d.zone_id
		GROUP BY z.zone_id, z.name, z.type, z.barcode, z.capacity, z.location, z.description
		ORDER BY z.name
	`

	rows, err := db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	headers := []string{
		"Zonen-ID", "Name", "Typ", "Barcode", "Kapazität",
		"Standort", "Notizen", "Anzahl Geräte",
	}

	var csvRows [][]string

	for rows.Next() {
		var zoneID int
		var name string
		var zoneType, barcode, location, description *string
		var capacity *int
		var deviceCount int

		err := rows.Scan(
			&zoneID, &name, &zoneType, &barcode, &capacity,
			&location, &description, &deviceCount,
		)
		if err != nil {
			return nil, err
		}

		csvRows = append(csvRows, []string{
			strconv.Itoa(zoneID),
			name,
			formatNullString(zoneType),
			formatNullString(barcode),
			formatNullInt(capacity),
			formatNullString(location),
			formatNullString(description),
			strconv.Itoa(deviceCount),
		})
	}

	return createCSV(headers, csvRows)
}

// exportCables exports all cables with specifications
func exportCables() ([]byte, error) {
	db := repository.GetSQLDB()

	query := `
		SELECT
			c.cableid,
			COALESCE(c.name, ''),
			COALESCE(ct.name, ''),
			COALESCE(cc1.name, ''),
			COALESCE(cc2.name, ''),
			c.length,
			c.mm2
		FROM cables c
		LEFT JOIN cable_types ct ON c.typ = ct.cable_typesid
		LEFT JOIN cable_connectors cc1 ON c.connector1 = cc1.cable_connectorsid
		LEFT JOIN cable_connectors cc2 ON c.connector2 = cc2.cable_connectorsid
		ORDER BY c.cableid
	`

	rows, err := db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	headers := []string{
		"ID", "Name", "Kabeltyp", "Stecker A", "Stecker B",
		"Länge (m)", "Querschnitt (mm²)",
	}

	var csvRows [][]string

	for rows.Next() {
		var id int
		var name, cableType, connectorA, connectorB string
		var length *float64
		var mm2 *float64

		err := rows.Scan(&id, &name, &cableType, &connectorA, &connectorB, &length, &mm2)
		if err != nil {
			return nil, err
		}

		csvRows = append(csvRows, []string{
			strconv.Itoa(id),
			name,
			cableType,
			connectorA,
			connectorB,
			formatNullFloat(length),
			formatNullFloat(mm2),
		})
	}

	return createCSV(headers, csvRows)
}

// exportJobs exports all jobs with complete information
func exportJobs() ([]byte, error) {
	db := repository.GetSQLDB()

	query := `
		SELECT
			j.jobid,
			COALESCE(j.job_code, ''),
			COALESCE(cu.name, ''),
			j.startdate,
			j.enddate,
			COALESCE(s.status, ''),
			COALESCE(j.description, ''),
			COUNT(DISTINCT jd.deviceid) as device_count
		FROM jobs j
		LEFT JOIN customers cu ON j.customerid = cu.customerid
		LEFT JOIN status s ON j.statusid = s.statusid
		LEFT JOIN job_devices jd ON j.jobid = jd.jobid
		WHERE j.deleted_at IS NULL
		GROUP BY j.jobid, j.job_code, cu.name, j.startdate, j.enddate, s.status, j.description
		ORDER BY j.startdate DESC
	`

	rows, err := db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	headers := []string{
		"Job-ID", "Job-Code", "Kunde", "Startdatum", "Enddatum",
		"Status", "Beschreibung", "Anzahl Geräte",
	}

	var csvRows [][]string

	for rows.Next() {
		var jobID int
		var jobCode, customerName, statusName, description string
		var startDate, endDate *time.Time
		var deviceCount int

		err := rows.Scan(
			&jobID, &jobCode, &customerName, &startDate,
			&endDate, &statusName, &description, &deviceCount,
		)
		if err != nil {
			return nil, err
		}

		csvRows = append(csvRows, []string{
			strconv.Itoa(jobID),
			jobCode,
			customerName,
			formatDate(startDate),
			formatDate(endDate),
			statusName,
			description,
			strconv.Itoa(deviceCount),
		})
	}

	return createCSV(headers, csvRows)
}
