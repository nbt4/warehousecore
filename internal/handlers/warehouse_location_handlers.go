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
	"warehousecore/internal/services"
)

type WarehouseLocation struct {
	ZoneID                 int64      `json:"zone_id"`
	Code                   string     `json:"code"`
	Barcode                *string    `json:"barcode,omitempty"`
	Name                   string     `json:"name"`
	Type                   string     `json:"type"`
	LocationKind           string     `json:"location_kind"`
	ProcessRole            string     `json:"process_role"`
	OperationalStatus      string     `json:"operational_status"`
	Description            *string    `json:"description,omitempty"`
	ParentZoneID           *int64     `json:"parent_zone_id,omitempty"`
	Capacity               *float64   `json:"capacity,omitempty"`
	CapacityMode           string     `json:"capacity_mode"`
	IsStorable             bool       `json:"is_storable"`
	IsActive               bool       `json:"is_active"`
	PickSequence           *int       `json:"pick_sequence,omitempty"`
	MaxWeightKg            *float64   `json:"max_weight_kg,omitempty"`
	MaxVolumeM3            *float64   `json:"max_volume_m3,omitempty"`
	InventoryFrequencyDays *int       `json:"inventory_frequency_days,omitempty"`
	LastCountedAt          *time.Time `json:"last_counted_at,omitempty"`
	NextCountAt            *time.Time `json:"next_count_at,omitempty"`
	DeviceCount            int        `json:"device_count"`
	CaseCount              int        `json:"case_count"`
	ProductQuantity        float64    `json:"product_quantity"`
	ChildCount             int        `json:"child_count"`
	Occupancy              float64    `json:"occupancy"`
	UtilizationPercent     *float64   `json:"utilization_percent,omitempty"`
}

type warehouseLocationInput struct {
	Code                   string   `json:"code"`
	Barcode                *string  `json:"barcode"`
	Name                   string   `json:"name"`
	Type                   string   `json:"type"`
	LocationKind           string   `json:"location_kind"`
	ProcessRole            string   `json:"process_role"`
	OperationalStatus      string   `json:"operational_status"`
	Description            *string  `json:"description"`
	ParentZoneID           *int64   `json:"parent_zone_id"`
	Capacity               *float64 `json:"capacity"`
	CapacityMode           string   `json:"capacity_mode"`
	IsStorable             bool     `json:"is_storable"`
	PickSequence           *int     `json:"pick_sequence"`
	MaxWeightKg            *float64 `json:"max_weight_kg"`
	MaxVolumeM3            *float64 `json:"max_volume_m3"`
	InventoryFrequencyDays *int     `json:"inventory_frequency_days"`
}

func scanWarehouseLocation(scanner interface{ Scan(...interface{}) error }) (WarehouseLocation, error) {
	var item WarehouseLocation
	var barcode, description sql.NullString
	var parentID sql.NullInt64
	var capacity, maxWeight, maxVolume sql.NullFloat64
	var pickSequence, frequency sql.NullInt64
	var lastCounted, nextCount sql.NullTime
	err := scanner.Scan(
		&item.ZoneID, &item.Code, &barcode, &item.Name, &item.Type,
		&item.LocationKind, &item.ProcessRole, &item.OperationalStatus, &description,
		&parentID, &capacity, &item.CapacityMode, &item.IsStorable, &item.IsActive,
		&pickSequence, &maxWeight, &maxVolume, &frequency, &lastCounted, &nextCount,
		&item.DeviceCount, &item.CaseCount, &item.ProductQuantity, &item.ChildCount,
	)
	if err != nil {
		return item, err
	}
	item.Barcode = ptrString(barcode)
	item.Description = ptrString(description)
	if parentID.Valid {
		v := parentID.Int64
		item.ParentZoneID = &v
	}
	if capacity.Valid {
		v := capacity.Float64
		item.Capacity = &v
	}
	if pickSequence.Valid {
		v := int(pickSequence.Int64)
		item.PickSequence = &v
	}
	if maxWeight.Valid {
		v := maxWeight.Float64
		item.MaxWeightKg = &v
	}
	if maxVolume.Valid {
		v := maxVolume.Float64
		item.MaxVolumeM3 = &v
	}
	if frequency.Valid {
		v := int(frequency.Int64)
		item.InventoryFrequencyDays = &v
	}
	if lastCounted.Valid {
		v := lastCounted.Time
		item.LastCountedAt = &v
	}
	if nextCount.Valid {
		v := nextCount.Time
		item.NextCountAt = &v
	}
	item.Occupancy = float64(item.DeviceCount+item.CaseCount) + item.ProductQuantity
	if item.Capacity != nil && *item.Capacity > 0 {
		v := item.Occupancy / *item.Capacity * 100
		item.UtilizationPercent = &v
	}
	return item, nil
}

const warehouseLocationSelect = `
	SELECT z.zone_id, z.code, z.barcode, z.name, z.type::text,
	       z.location_kind, z.process_role, z.operational_status, z.description,
	       z.parent_zone_id, z.capacity, z.capacity_mode, z.is_storable, z.is_active,
	       z.pick_sequence, z.max_weight_kg, z.max_volume_m3, z.inventory_frequency_days,
	       z.last_counted_at, z.next_count_at,
	       (SELECT COUNT(*) FROM devices d WHERE d.zone_id = z.zone_id AND d.status = 'in_storage'),
	       (SELECT COUNT(*) FROM cases c WHERE c.zone_id = z.zone_id),
	       COALESCE((SELECT SUM(pl.quantity) FROM product_locations pl WHERE pl.zone_id = z.zone_id), 0),
	       (SELECT COUNT(*) FROM storage_zones child WHERE child.parent_zone_id = z.zone_id AND child.is_active)
	FROM storage_zones z`

func GetWarehouseLocations(w http.ResponseWriter, r *http.Request) {
	db := repository.GetSQLDB()
	query := warehouseLocationSelect
	if r.URL.Query().Get("include_archived") != "true" {
		query += ` WHERE z.is_active = TRUE`
	}
	query += ` ORDER BY COALESCE(z.pick_sequence, 2147483647), z.code`
	rows, err := db.Query(query)
	if err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	defer rows.Close()
	items := []WarehouseLocation{}
	for rows.Next() {
		item, err := scanWarehouseLocation(rows)
		if err != nil {
			respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		items = append(items, item)
	}
	respondJSON(w, http.StatusOK, items)
}

func GetWarehouseLocation(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(mux.Vars(r)["id"], 10, 64)
	if err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "Ungültiger Lagerplatz"})
		return
	}
	item, err := scanWarehouseLocation(repository.GetSQLDB().QueryRow(warehouseLocationSelect+` WHERE z.zone_id = $1`, id))
	if errors.Is(err, sql.ErrNoRows) {
		respondJSON(w, http.StatusNotFound, map[string]string{"error": "Lagerplatz nicht gefunden"})
		return
	}
	if err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	respondJSON(w, http.StatusOK, item)
}

func validateWarehouseLocationInput(input *warehouseLocationInput) error {
	input.Name = strings.TrimSpace(input.Name)
	input.Code = strings.ToUpper(strings.TrimSpace(input.Code))
	if input.Name == "" {
		return fmt.Errorf("Name ist erforderlich")
	}
	if input.LocationKind == "" {
		input.LocationKind = "area"
	}
	if input.ProcessRole == "" {
		input.ProcessRole = "storage"
	}
	if input.OperationalStatus == "" {
		input.OperationalStatus = "available"
	}
	if input.CapacityMode == "" {
		input.CapacityMode = "item_count"
	}
	if input.Type == "" {
		switch input.LocationKind {
		case "site":
			input.Type = "warehouse"
		case "rack":
			input.Type = "rack"
		case "bin", "level":
			input.Type = "shelf"
		case "vehicle":
			input.Type = "vehicle"
		default:
			input.Type = "other"
		}
	}
	if input.Capacity != nil && *input.Capacity <= 0 {
		return fmt.Errorf("Kapazität muss größer als null sein")
	}
	if input.InventoryFrequencyDays != nil && *input.InventoryFrequencyDays < 0 {
		return fmt.Errorf("Inventurintervall darf nicht negativ sein")
	}
	return nil
}

func normalizeWarehouseLocationBarcode(barcode *string, code string) *string {
	if barcode != nil {
		if value := strings.TrimSpace(*barcode); value != "" {
			return &value
		}
	}
	value := "LOC-" + strings.ToUpper(strings.TrimSpace(code))
	return &value
}

func validateWarehouseParent(db *sql.DB, locationID int64, parentID *int64) error {
	if parentID == nil {
		return nil
	}
	if *parentID == locationID && locationID > 0 {
		return fmt.Errorf("ein Lagerplatz kann nicht sein eigenes Elternobjekt sein")
	}
	var active bool
	var status string
	if err := db.QueryRow(`SELECT is_active,operational_status FROM storage_zones WHERE zone_id=$1`, *parentID).Scan(&active, &status); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("übergeordneter Bereich wurde nicht gefunden")
		}
		return err
	}
	if !active || status == "archived" {
		return fmt.Errorf("übergeordneter Bereich ist nicht aktiv")
	}
	if locationID <= 0 {
		return nil
	}
	var createsCycle bool
	err := db.QueryRow(`WITH RECURSIVE descendants(zone_id) AS (
		SELECT zone_id FROM storage_zones WHERE parent_zone_id=$1
		UNION ALL
		SELECT z.zone_id FROM storage_zones z JOIN descendants d ON z.parent_zone_id=d.zone_id
	) SELECT EXISTS(SELECT 1 FROM descendants WHERE zone_id=$2)`, locationID, *parentID).Scan(&createsCycle)
	if err != nil {
		return err
	}
	if createsCycle {
		return fmt.Errorf("diese Zuordnung würde einen Kreis in der Lagerstruktur erzeugen")
	}
	return nil
}

func CreateWarehouseLocation(w http.ResponseWriter, r *http.Request) {
	var input warehouseLocationInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "Ungültige Anfrage"})
		return
	}
	if err := validateWarehouseLocationInput(&input); err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	db := repository.GetSQLDB()
	if err := validateWarehouseParent(db, 0, input.ParentZoneID); err != nil {
		respondJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
		return
	}
	if input.Code == "" {
		code, err := services.NewZoneService().GenerateZoneCode(input.Name, input.Type, input.ParentZoneID)
		if err != nil {
			respondJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		input.Code = code
	}
	input.Barcode = normalizeWarehouseLocationBarcode(input.Barcode, input.Code)
	var id int64
	err := db.QueryRow(`
		INSERT INTO storage_zones
		(code, barcode, name, type, description, parent_zone_id, capacity, is_active,
		 location_kind, process_role, operational_status, is_storable, pick_sequence,
		 capacity_mode, max_weight_kg, max_volume_m3, inventory_frequency_days, next_count_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,TRUE,$8,$9,$10,$11,$12,$13,$14,$15,$16,
		 CASE WHEN $16::int > 0 THEN CURRENT_TIMESTAMP + ($16::text || ' days')::interval ELSE NULL END)
		RETURNING zone_id
	`, input.Code, nullableStringPtr(input.Barcode), input.Name, input.Type, nullableStringPtr(input.Description),
		input.ParentZoneID, input.Capacity, input.LocationKind, input.ProcessRole, input.OperationalStatus,
		input.IsStorable, input.PickSequence, input.CapacityMode, input.MaxWeightKg, input.MaxVolumeM3,
		input.InventoryFrequencyDays).Scan(&id)
	if err != nil {
		respondJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
		return
	}
	respondJSON(w, http.StatusCreated, map[string]interface{}{"zone_id": id, "message": "Lagerplatz erstellt"})
}

func UpdateWarehouseLocation(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(mux.Vars(r)["id"], 10, 64)
	if err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "Ungültiger Lagerplatz"})
		return
	}
	var input warehouseLocationInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "Ungültige Anfrage"})
		return
	}
	if err := validateWarehouseLocationInput(&input); err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	if input.Code == "" {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "Code ist erforderlich"})
		return
	}
	input.Barcode = normalizeWarehouseLocationBarcode(input.Barcode, input.Code)
	db := repository.GetSQLDB()
	if err := validateWarehouseParent(db, id, input.ParentZoneID); err != nil {
		respondJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
		return
	}
	active := input.OperationalStatus != "archived"
	result, err := db.Exec(`
		UPDATE storage_zones SET code=$1, barcode=$2, name=$3, type=$4, description=$5,
		 parent_zone_id=$6, capacity=$7, is_active=$8, location_kind=$9, process_role=$10,
		 operational_status=$11, is_storable=$12, pick_sequence=$13, capacity_mode=$14,
		 max_weight_kg=$15, max_volume_m3=$16, inventory_frequency_days=$17,
		 next_count_at=CASE WHEN $17::int > 0 THEN COALESCE(last_counted_at,CURRENT_TIMESTAMP) + ($17::text || ' days')::interval ELSE NULL END
		WHERE zone_id=$18
	`, input.Code, nullableStringPtr(input.Barcode), input.Name, input.Type, nullableStringPtr(input.Description),
		input.ParentZoneID, input.Capacity, active, input.LocationKind, input.ProcessRole,
		input.OperationalStatus, input.IsStorable, input.PickSequence, input.CapacityMode,
		input.MaxWeightKg, input.MaxVolumeM3, input.InventoryFrequencyDays, id)
	if err != nil {
		respondJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
		return
	}
	if n, _ := result.RowsAffected(); n == 0 {
		respondJSON(w, http.StatusNotFound, map[string]string{"error": "Lagerplatz nicht gefunden"})
		return
	}
	respondJSON(w, http.StatusOK, map[string]string{"message": "Lagerplatz aktualisiert"})
}

func ArchiveWarehouseLocation(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(mux.Vars(r)["id"], 10, 64)
	if err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "Ungültiger Lagerplatz"})
		return
	}
	db := repository.GetSQLDB()
	usage, err := getWarehouseLocationUsage(db, id)
	if err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if !usage.Empty() {
		respondJSON(w, http.StatusConflict, map[string]interface{}{
			"error": "Lagerplatz ist nicht leer", "children": usage.Children, "devices": usage.Devices, "cases": usage.Cases, "product_quantity": usage.Products,
		})
		return
	}
	result, err := db.Exec(`UPDATE storage_zones SET is_active=FALSE, operational_status='archived' WHERE zone_id=$1`, id)
	if err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if n, _ := result.RowsAffected(); n == 0 {
		respondJSON(w, http.StatusNotFound, map[string]string{"error": "Lagerplatz nicht gefunden"})
		return
	}
	respondJSON(w, http.StatusOK, map[string]string{"message": "Lagerplatz archiviert"})
}

type warehouseLocationUsage struct {
	Children int
	Devices  int
	Cases    int
	Products float64
}

func (u warehouseLocationUsage) Empty() bool {
	return u.Children+u.Devices+u.Cases == 0 && u.Products == 0
}

func getWarehouseLocationUsage(db *sql.DB, id int64) (warehouseLocationUsage, error) {
	var usage warehouseLocationUsage
	err := db.QueryRow(`SELECT
		(SELECT COUNT(*) FROM storage_zones WHERE parent_zone_id=$1 AND is_active),
		(SELECT COUNT(*) FROM devices WHERE zone_id=$1),
		(SELECT COUNT(*) FROM cases WHERE zone_id=$1),
		COALESCE((SELECT SUM(quantity) FROM product_locations WHERE zone_id=$1),0)`, id).
		Scan(&usage.Children, &usage.Devices, &usage.Cases, &usage.Products)
	return usage, err
}

func GetWarehouseOverview(w http.ResponseWriter, r *http.Request) {
	db := repository.GetSQLDB()
	type overview struct {
		ActiveLocations int     `json:"active_locations"`
		Blocked         int     `json:"blocked_locations"`
		UnplacedDevices int     `json:"unplaced_devices"`
		UnplacedCases   int     `json:"unplaced_cases"`
		UnplacedStock   float64 `json:"unplaced_product_quantity"`
		OpenTasks       int     `json:"open_tasks"`
		CountsDue       int     `json:"counts_due"`
	}
	var result overview
	err := db.QueryRow(`SELECT
		(SELECT COUNT(*) FROM storage_zones WHERE is_active),
		(SELECT COUNT(*) FROM storage_zones WHERE is_active AND operational_status <> 'available'),
		(SELECT COUNT(*) FROM devices
		 WHERE zone_id IS NULL
		   AND COALESCE(status,'') NOT IN ('on_job','rented')
		   AND NOT EXISTS (SELECT 1 FROM devicescases dc WHERE dc.deviceID=devices.deviceID)),
		(SELECT COUNT(*) FROM cases
		 WHERE zone_id IS NULL
		   AND workflow_status <> 'on_job'
		   AND NOT EXISTS (SELECT 1 FROM case_child_contents cc WHERE cc.child_case_id=cases.caseID)),
		COALESCE((SELECT SUM(quantity) FROM product_locations WHERE zone_id IS NULL),0),
		(SELECT COUNT(*) FROM warehouse_tasks WHERE status IN ('open','in_progress')),
		(SELECT COUNT(*) FROM storage_zones WHERE is_active AND next_count_at IS NOT NULL AND next_count_at <= CURRENT_TIMESTAMP)
	`).Scan(&result.ActiveLocations, &result.Blocked, &result.UnplacedDevices, &result.UnplacedCases, &result.UnplacedStock, &result.OpenTasks, &result.CountsDue)
	if err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	respondJSON(w, http.StatusOK, result)
}

type warehouseTaskInput struct {
	TaskType   string     `json:"task_type"`
	Priority   int        `json:"priority"`
	FromZoneID *int64     `json:"from_zone_id"`
	ToZoneID   *int64     `json:"to_zone_id"`
	CaseID     *int64     `json:"case_id"`
	DeviceID   *string    `json:"device_id"`
	ProductID  *int64     `json:"product_id"`
	Quantity   *float64   `json:"quantity"`
	JobID      *int64     `json:"job_id"`
	DueAt      *time.Time `json:"due_at"`
	Notes      *string    `json:"notes"`
}

func GetWarehouseTasks(w http.ResponseWriter, r *http.Request) {
	status := strings.TrimSpace(r.URL.Query().Get("status"))
	query := `SELECT task_id,task_type,status,priority,from_zone_id,to_zone_id,case_id,device_id,product_id,quantity,job_id,due_at,notes,created_at,updated_at,completed_at FROM warehouse_tasks`
	args := []interface{}{}
	if status != "" {
		query += ` WHERE status=$1`
		args = append(args, status)
	}
	query += ` ORDER BY CASE status WHEN 'in_progress' THEN 0 WHEN 'open' THEN 1 ELSE 2 END, priority DESC, due_at NULLS LAST, task_id`
	rows, err := repository.GetSQLDB().Query(query, args...)
	if err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	defer rows.Close()
	items := []map[string]interface{}{}
	for rows.Next() {
		var id int64
		var taskType, taskStatus string
		var priority int
		var from, to, caseID, productID, jobID sql.NullInt64
		var device, notes sql.NullString
		var quantity sql.NullFloat64
		var due, created, updated, completed sql.NullTime
		if err := rows.Scan(&id, &taskType, &taskStatus, &priority, &from, &to, &caseID, &device, &productID, &quantity, &jobID, &due, &notes, &created, &updated, &completed); err != nil {
			continue
		}
		item := map[string]interface{}{"task_id": id, "task_type": taskType, "status": taskStatus, "priority": priority, "created_at": created.Time, "updated_at": updated.Time}
		if from.Valid {
			item["from_zone_id"] = from.Int64
		}
		if to.Valid {
			item["to_zone_id"] = to.Int64
		}
		if caseID.Valid {
			item["case_id"] = caseID.Int64
		}
		if device.Valid {
			item["device_id"] = device.String
		}
		if productID.Valid {
			item["product_id"] = productID.Int64
		}
		if quantity.Valid {
			item["quantity"] = quantity.Float64
		}
		if jobID.Valid {
			item["job_id"] = jobID.Int64
		}
		if due.Valid {
			item["due_at"] = due.Time
		}
		if notes.Valid {
			item["notes"] = notes.String
		}
		if completed.Valid {
			item["completed_at"] = completed.Time
		}
		items = append(items, item)
	}
	respondJSON(w, http.StatusOK, items)
}

func CreateWarehouseTask(w http.ResponseWriter, r *http.Request) {
	var input warehouseTaskInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil || strings.TrimSpace(input.TaskType) == "" {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "Aufgabentyp ist erforderlich"})
		return
	}
	if input.Priority == 0 {
		input.Priority = 50
	}
	var id int64
	err := repository.GetSQLDB().QueryRow(`INSERT INTO warehouse_tasks(task_type,priority,from_zone_id,to_zone_id,case_id,device_id,product_id,quantity,job_id,due_at,notes) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11) RETURNING task_id`, input.TaskType, input.Priority, input.FromZoneID, input.ToZoneID, input.CaseID, input.DeviceID, input.ProductID, input.Quantity, input.JobID, input.DueAt, input.Notes).Scan(&id)
	if err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	respondJSON(w, http.StatusCreated, map[string]interface{}{"task_id": id, "message": "Lageraufgabe erstellt"})
}

func UpdateWarehouseTaskStatus(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(mux.Vars(r)["id"], 10, 64)
	if err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "Ungültige Aufgabe"})
		return
	}
	var input struct {
		Status string `json:"status"`
	}
	if json.NewDecoder(r.Body).Decode(&input) != nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "Ungültige Anfrage"})
		return
	}
	result, err := repository.GetSQLDB().Exec(`UPDATE warehouse_tasks SET status=$1,updated_at=CURRENT_TIMESTAMP,completed_at=CASE WHEN $1='done' THEN CURRENT_TIMESTAMP ELSE NULL END WHERE task_id=$2`, input.Status, id)
	if err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	if n, _ := result.RowsAffected(); n == 0 {
		respondJSON(w, http.StatusNotFound, map[string]string{"error": "Aufgabe nicht gefunden"})
		return
	}
	respondJSON(w, http.StatusOK, map[string]string{"message": "Aufgabe aktualisiert"})
}
