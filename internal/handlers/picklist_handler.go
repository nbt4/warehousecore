package handlers

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"github.com/gorilla/mux"
	"math"
	"net/http"
	"strconv"
	"strings"
	"warehousecore/internal/middleware"
	"warehousecore/internal/repository"
	"warehousecore/internal/services"
)

func GetJobPicklist(w http.ResponseWriter, r *http.Request) {
	jobID, err := strconv.Atoi(mux.Vars(r)["id"])
	if err != nil || jobID < 1 {
		respondJSON(w, 400, map[string]string{"error": "Invalid job ID"})
		return
	}
	tx, err := repository.GetSQLDB().BeginTx(r.Context(), nil)
	if err != nil {
		respondJSON(w, 500, map[string]string{"error": "Packbedarf konnte nicht geladen werden"})
		return
	}
	defer tx.Rollback()
	snapshot, err := services.MaterialSnapshotTx(r.Context(), tx, jobID)
	if err != nil {
		respondJSON(w, 500, map[string]string{"error": "Packbedarf konnte nicht geladen werden"})
		return
	}
	counts, err := services.MaterialScanCounts(tx, jobID)
	if err != nil {
		respondJSON(w, 500, map[string]string{"error": "Scanstand konnte nicht geladen werden"})
		return
	}
	positions := []map[string]any{}
	known := map[int]bool{}
	for _, item := range snapshot.Items {
		count := counts[item.ProductID]
		if count.Devices == nil {
			count.Devices = []services.MaterialScanDevice{}
		}
		known[item.ProductID] = true
		positions = append(positions, map[string]any{"position_id": item.ProductID, "product_id": item.ProductID, "product_name": item.Name, "needed": item.Quantity, "scanned": count.Quantity, "fulfilled": count.Quantity >= item.Quantity, "devices": count.Devices, "unit": item.Unit, "tracking_mode": item.Tracking, "origins": item.Origins})
		if count.Quantity > item.Quantity {
			snapshot.Warnings = append(snapshot.Warnings, fmt.Sprintf("%s: bereits gepackte Menge übersteigt den aktuellen Bedarf", item.Name))
		}
	}
	for product, count := range counts {
		if !known[product] && count.Quantity > 0 {
			snapshot.Warnings = append(snapshot.Warnings, fmt.Sprintf("Produkt %d ist noch zugeordnet, wird aber nicht mehr benötigt", product))
		}
	}
	if err = tx.Commit(); err != nil {
		respondJSON(w, 500, map[string]string{"error": "Packbedarf konnte nicht gespeichert werden"})
		return
	}
	respondJSON(w, 200, map[string]any{"job_id": jobID, "version": snapshot.Version, "positions": positions, "warnings": snapshot.Warnings})
}

// A receipt makes a repeated quantity confirmation idempotent. Individual scans
// are also unique per job/device, including historical position allocations.
func ScanDeviceToPicklist(w http.ResponseWriter, r *http.Request) {
	jobID, err := strconv.Atoi(mux.Vars(r)["id"])
	if err != nil || jobID < 1 {
		respondJSON(w, 400, map[string]string{"error": "Invalid job ID"})
		return
	}
	var input struct {
		DeviceID  string  `json:"device_id"`
		ProductID int     `json:"product_id"`
		Quantity  float64 `json:"quantity"`
		Receipt   string  `json:"receipt"`
	}
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 8192)).Decode(&input) != nil {
		respondJSON(w, 400, map[string]string{"error": "Invalid request body"})
		return
	}
	input.DeviceID = strings.TrimSpace(input.DeviceID)
	input.Receipt = strings.TrimSpace(input.Receipt)
	if input.DeviceID == "" && (input.ProductID < 1 || input.Receipt == "" || len(input.Receipt) > 100 || input.Quantity <= 0 || math.IsInf(input.Quantity, 0) || math.IsNaN(input.Quantity) || input.Quantity > 1e9 || math.Abs(input.Quantity*1000-math.Round(input.Quantity*1000)) > 0.00001) {
		respondJSON(w, 400, map[string]string{"error": "Produkt, Menge (maximal drei Nachkommastellen) und Beleg-ID erforderlich"})
		return
	}
	tx, err := repository.GetSQLDB().BeginTx(r.Context(), nil)
	if err != nil {
		respondJSON(w, 500, map[string]string{"error": "Scan konnte nicht gespeichert werden"})
		return
	}
	defer tx.Rollback()
	snapshot, err := services.MaterialSnapshotTx(r.Context(), tx, jobID)
	if err != nil {
		respondJSON(w, 404, map[string]string{"error": "Aktiver Job nicht gefunden"})
		return
	}
	var closed bool
	if err = tx.QueryRow(`SELECT warehouse_job_status_is_closed(COALESCE(s.status,'')) FROM jobs j LEFT JOIN status s ON s.statusid=j.statusid WHERE j.jobid=$1`, jobID).Scan(&closed); err != nil || closed {
		respondJSON(w, 409, map[string]string{"error": "Abgeschlossene Jobs können nicht weiter gepackt werden"})
		return
	}
	actor := ""
	if user, ok := middleware.GetUserFromContext(r); ok {
		actor = strconv.Itoa(int(user.UserID))
	}
	productID := input.ProductID
	quantity := input.Quantity
	receipt := fmt.Sprintf("%d:%s", jobID, input.Receipt)
	if input.DeviceID != "" {
		var lifecycle, condition, productLifecycle string
		if err = tx.QueryRow(`SELECT d.productid,d.deviceid,d.lifecycle_status,d.condition_status,p.lifecycle_status FROM devices d JOIN products p ON p.productid=d.productid WHERE d.deviceid=$1 OR d.barcode=$1 OR d.qr_code=$1 ORDER BY (d.deviceid=$1) DESC LIMIT 1 FOR UPDATE OF d`, input.DeviceID).Scan(&productID, &input.DeviceID, &lifecycle, &condition, &productLifecycle); err != nil {
			respondJSON(w, 404, map[string]string{"error": "Gerät nicht gefunden"})
			return
		}
		if lifecycle != "active" || condition != "available" || productLifecycle != "active" {
			respondJSON(w, 409, map[string]string{"error": "Gerät ist gesperrt, defekt oder archiviert"})
			return
		}
		if err = validateMaterialDevice(tx, input.DeviceID, jobID); err != nil {
			respondJSON(w, 409, map[string]string{"error": err.Error()})
			return
		}

		quantity = 1
		receipt = fmt.Sprintf("%d:device:%s", jobID, input.DeviceID)
	}
	var oldProduct int
	var oldDevice sql.NullString
	var oldQuantity float64
	err = tx.QueryRow(`SELECT product_id,device_id,quantity FROM warehouse_material_scans WHERE receipt=$1`, receipt).Scan(&oldProduct, &oldDevice, &oldQuantity)
	if err == nil {
		if oldProduct != productID || oldQuantity != quantity || oldDevice.String != input.DeviceID {
			respondJSON(w, 409, map[string]string{"error": "Beleg-ID wurde bereits für einen anderen Scan verwendet"})
			return
		}
		respondJSON(w, 200, map[string]any{"device_id": input.DeviceID, "product_id": productID, "replayed": true})
		return
	}
	if err != sql.ErrNoRows {
		respondJSON(w, 500, map[string]string{"error": "Scan konnte nicht geprüft werden"})
		return
	}
	counts, err := services.MaterialScanCounts(tx, jobID)
	if err != nil {
		respondJSON(w, 500, map[string]string{"error": "Scanstand konnte nicht geladen werden"})
		return
	}
	var needed float64
	tracking := ""
	for _, item := range snapshot.Items {
		if item.ProductID == productID {
			needed = item.Quantity
			tracking = item.Tracking
		}
	}
	if input.DeviceID != "" {
		for _, d := range counts[productID].Devices {
			if d.DeviceID == input.DeviceID {
				respondJSON(w, 200, map[string]any{"device_id": input.DeviceID, "product_id": productID, "replayed": true})
				return
			}
		}
	} else if tracking != "quantity" {
		respondJSON(w, 409, map[string]string{"error": "Dieses Produkt wird über einzelne Geräte gepackt"})
		return
	}
	if input.DeviceID == "" {
		var active bool
		if err = tx.QueryRow(`SELECT lifecycle_status='active' AND tracking_mode='quantity' FROM products WHERE productid=$1 FOR UPDATE`, productID).Scan(&active); err != nil || !active {
			respondJSON(w, 409, map[string]string{"error": "Aktives Mengenprodukt erforderlich"})
			return
		}
	}

	if needed == 0 || counts[productID].Quantity+quantity > needed+0.00001 {
		respondJSON(w, 409, map[string]string{"error": "Kein offener Bedarf für diese Menge vorhanden"})
		return
	}
	var device any
	if input.DeviceID != "" {
		device = input.DeviceID
	}
	if _, err = tx.Exec(`INSERT INTO warehouse_material_scans(receipt,job_id,product_id,device_id,quantity,scanned_by) VALUES($1,$2,$3,$4,$5,$6)`, receipt, jobID, productID, device, quantity, actor); err != nil {
		respondJSON(w, 409, map[string]string{"error": "Gerät oder Beleg wurde inzwischen bereits gescannt"})
		return
	}
	if input.DeviceID != "" {
		if _, err = tx.Exec(`INSERT INTO job_devices(jobid,deviceid) VALUES($1,$2) ON CONFLICT DO NOTHING`, jobID, input.DeviceID); err != nil {
			respondJSON(w, 409, map[string]string{"error": "Gerät konnte nicht reserviert werden"})
			return
		}
		// Included components count only after their actual parent device is scanned.
		if err = recordIncludedMaterial(tx, snapshot, counts, jobID, productID, input.DeviceID, actor, map[string]bool{input.DeviceID: true}, 0); err != nil {
			respondJSON(w, 409, map[string]string{"error": err.Error()})
			return
		}
	}
	if err = recordCatalogAudit(tx, r, "job", strconv.Itoa(jobID), "material.scan", map[string]any{"product_id": productID, "device_id": input.DeviceID, "quantity": quantity}); err != nil {
		respondJSON(w, 500, map[string]string{"error": "Scan konnte nicht protokolliert werden"})
		return
	}
	if err = tx.Commit(); err != nil {
		respondJSON(w, 409, map[string]string{"error": "Scan konnte nicht gespeichert werden"})
		return
	}
	respondJSON(w, 201, map[string]any{"device_id": input.DeviceID, "product_id": productID, "message": "Material zugeordnet"})
}
func recordIncludedMaterial(tx *sql.Tx, snapshot *services.MaterialSnapshot, counts map[int]services.MaterialScanCount, jobID, parentProduct int, parentDevice, actor string, visited map[string]bool, depth int) error {
	targets := map[int]float64{}
	for _, rule := range snapshot.Graph.Rules[parentProduct] {
		if rule.Type == "included" && rule.Scope != "case" {
			targets[rule.Target] += rule.Quantity
		}
	}
	if len(targets) == 0 {
		return nil
	}
	if depth >= 5 {
		return fmt.Errorf("Enthaltene Geräte überschreiten die maximale Packtiefe")
	}
	rows, err := tx.Query(`SELECT d.deviceid,d.productid,d.lifecycle_status,d.condition_status FROM device_components dc JOIN devices d ON d.deviceid=dc.component_device_id WHERE dc.device_id=$1 AND dc.relation_type='included' ORDER BY d.deviceid FOR UPDATE OF d`, parentDevice)
	if err != nil {
		return err
	}
	type child struct {
		id              string
		product         int
		life, condition string
	}
	children := []child{}
	for rows.Next() {
		var c child
		if err = rows.Scan(&c.id, &c.product, &c.life, &c.condition); err != nil {
			rows.Close()
			return err
		}
		children = append(children, c)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for _, c := range children {
		if targets[c.product] <= 0 {
			continue
		}
		if c.life != "active" || c.condition != "available" {
			return fmt.Errorf("Enthaltenes Gerät %s ist nicht verfügbar", c.id)
		}
		if visited[c.id] {
			return fmt.Errorf("Kreis in enthaltenen Geräten")
		}
		visited[c.id] = true
		if err = validateMaterialDevice(tx, c.id, jobID); err != nil {
			return err
		}

		needed := 0.0
		for _, item := range snapshot.Items {
			if item.ProductID == c.product {
				needed = item.Quantity
			}
		}
		count := counts[c.product]
		if count.Quantity >= needed {
			continue
		}
		result, err := tx.Exec(`INSERT INTO warehouse_material_scans(receipt,job_id,product_id,device_id,quantity,scanned_by) VALUES($1,$2,$3,$4,1,$5) ON CONFLICT(job_id,device_id) WHERE device_id IS NOT NULL DO NOTHING`, fmt.Sprintf("%d:device:%s", jobID, c.id), jobID, c.product, c.id, actor)
		if err != nil {
			return err
		}
		n, _ := result.RowsAffected()
		if n > 0 {
			count.Quantity++
			count.Devices = append(count.Devices, services.MaterialScanDevice{DeviceID: c.id})
			counts[c.product] = count
		}
		targets[c.product]--
		if _, err = tx.Exec(`INSERT INTO job_devices(jobid,deviceid) VALUES($1,$2) ON CONFLICT DO NOTHING`, jobID, c.id); err != nil {
			return err
		}
		if n > 0 {
			if err = recordIncludedMaterial(tx, snapshot, counts, jobID, c.product, c.id, actor, visited, depth+1); err != nil {
				return err
			}
		}

	}
	return nil
}

func validateMaterialDevice(tx *sql.Tx, deviceID string, jobID int) error {
	var available, conflicting bool
	err := tx.QueryRow(`SELECT d.lifecycle_status='active' AND p.lifecycle_status='active' AND d.condition_status='available' AND d.status IN ('in_storage','location_unknown') FROM devices d JOIN products p ON p.productid=d.productid WHERE d.deviceid=$1 FOR UPDATE OF d`, deviceID).Scan(&available)
	if err != nil {
		return err
	}
	if !available {
		return fmt.Errorf("Gerät %s ist nicht verfügbar", deviceID)
	}
	err = tx.QueryRow(`SELECT EXISTS(SELECT 1 FROM job_devices jd JOIN jobs j ON j.jobid=jd.jobid LEFT JOIN status s ON s.statusid=j.statusid WHERE jd.deviceid=$1 AND jd.jobid<>$2 AND ((j.deleted_at IS NULL AND NOT warehouse_job_status_is_closed(COALESCE(s.status,''))) OR jd.pack_status IN ('packed','issued'))) OR EXISTS(SELECT 1 FROM job_position_devices pd JOIN job_positions jp ON jp.position_id=pd.position_id JOIN jobs j ON j.jobid=jp.job_id LEFT JOIN status s ON s.statusid=j.statusid WHERE pd.device_id=$1 AND jp.job_id<>$2 AND j.deleted_at IS NULL AND NOT warehouse_job_status_is_closed(COALESCE(s.status,'')))`, deviceID, jobID).Scan(&conflicting)
	if err != nil {
		return err
	}
	if conflicting {
		return fmt.Errorf("Gerät %s ist bereits einem anderen Job zugeordnet", deviceID)
	}
	return nil
}
