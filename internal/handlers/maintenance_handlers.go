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

	"warehousecore/internal/middleware"
	"warehousecore/internal/repository"
)

var maintenanceOrderTypes = map[string]bool{
	"defect": true, "preventive": true, "inspection": true, "calibration": true,
}

var maintenancePriorities = map[string]bool{
	"low": true, "normal": true, "high": true, "critical": true,
}

var maintenanceStatuses = map[string]bool{
	"open": true, "planned": true, "in_progress": true, "waiting_parts": true, "completed": true, "cancelled": true,
}

var maintenanceOutcomes = map[string]bool{
	"passed": true, "passed_with_notes": true, "failed": true, "repaired": true,
}

func maintenanceActorID(r *http.Request) interface{} {
	if user, ok := middleware.GetUserFromContext(r); ok && user != nil {
		return user.UserID
	}
	return nil
}

func parseMaintenanceDate(value string) (*time.Time, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, nil
	}
	parsed, err := time.Parse("2006-01-02", value)
	if err != nil {
		return nil, fmt.Errorf("Datum muss das Format JJJJ-MM-TT haben")
	}
	return &parsed, nil
}

func maintenanceNullableString(value string) interface{} {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return value
}

type maintenanceExecer interface {
	Exec(query string, args ...interface{}) (sql.Result, error)
}

func syncDeviceNextMaintenance(execer maintenanceExecer, deviceID string) error {
	_, err := execer.Exec(`UPDATE devices SET nextmaintenance=(
		SELECT MIN(next_due_at) FROM maintenance_plans WHERE device_id=$1 AND is_active
	),updated_at=CURRENT_TIMESTAMP WHERE deviceID=$1`, deviceID)
	return err
}

func formatMaintenanceDate(value sql.NullTime) *string {
	if !value.Valid {
		return nil
	}
	formatted := value.Time.Format("2006-01-02")
	return &formatted
}

func formatMaintenanceTime(value sql.NullTime) *string {
	if !value.Valid {
		return nil
	}
	formatted := value.Time.Format(time.RFC3339)
	return &formatted
}

func ensureDueMaintenanceOrders(db *sql.DB) error {
	_, err := db.Exec(`WITH inserted AS (
		INSERT INTO maintenance_orders(device_id,plan_id,order_type,priority,status,title,description,due_at,reported_by)
		SELECT p.device_id,p.plan_id,p.maintenance_type,'normal','planned',p.name,p.instructions,p.next_due_at,p.created_by
		FROM maintenance_plans p
		WHERE p.is_active
		  AND p.next_due_at <= CURRENT_DATE + p.lead_time_days
		  AND NOT EXISTS (
			SELECT 1 FROM maintenance_orders o WHERE o.plan_id=p.plan_id
			AND o.status IN ('open','planned','in_progress','waiting_parts')
		  )
		ON CONFLICT DO NOTHING
		RETURNING order_id,reported_by
	)
	INSERT INTO maintenance_order_events(order_id,event_type,to_status,notes,actor_id)
	SELECT order_id,'auto_created','planned','Automatisch aus fälligem Wartungsplan erzeugt',reported_by FROM inserted`)
	return err
}

type maintenanceOrderResponse struct {
	OrderID         int64    `json:"order_id"`
	DeviceID        string   `json:"device_id"`
	PlanID          *int64   `json:"plan_id,omitempty"`
	OrderType       string   `json:"order_type"`
	Priority        string   `json:"priority"`
	Status          string   `json:"status"`
	Title           string   `json:"title"`
	Description     string   `json:"description,omitempty"`
	DueAt           *string  `json:"due_at,omitempty"`
	ScheduledAt     *string  `json:"scheduled_at,omitempty"`
	ReportedBy      *int64   `json:"reported_by,omitempty"`
	ReportedByName  string   `json:"reported_by_name,omitempty"`
	AssignedTo      *int64   `json:"assigned_to,omitempty"`
	AssignedToName  string   `json:"assigned_to_name,omitempty"`
	StartedAt       *string  `json:"started_at,omitempty"`
	CompletedAt     *string  `json:"completed_at,omitempty"`
	Outcome         string   `json:"outcome,omitempty"`
	Resolution      string   `json:"resolution,omitempty"`
	Cost            *float64 `json:"cost,omitempty"`
	CreatedAt       string   `json:"created_at"`
	UpdatedAt       string   `json:"updated_at"`
	ProductName     string   `json:"product_name,omitempty"`
	SerialNumber    string   `json:"serial_number,omitempty"`
	DeviceCondition string   `json:"device_condition"`
	ZoneName        string   `json:"zone_name,omitempty"`
	PlanName        string   `json:"plan_name,omitempty"`
}

const maintenanceOrderSelect = `SELECT o.order_id,o.device_id,o.plan_id,o.order_type,o.priority,o.status,o.title,
	COALESCE(o.description,''),o.due_at,o.scheduled_at,o.reported_by,
	COALESCE(NULLIF(CONCAT_WS(' ',ru.first_name,ru.last_name),''),ru.username,''),
	o.assigned_to,COALESCE(NULLIF(CONCAT_WS(' ',au.first_name,au.last_name),''),au.username,''),
	o.started_at,o.completed_at,COALESCE(o.outcome,''),COALESCE(o.resolution,''),o.cost,o.created_at,o.updated_at,
	COALESCE(p.name,''),COALESCE(d.serialnumber,''),d.condition_status,COALESCE(z.name,''),COALESCE(mp.name,'')
	FROM maintenance_orders o
	JOIN devices d ON d.deviceID=o.device_id
	LEFT JOIN products p ON p.productID=d.productID
	LEFT JOIN storage_zones z ON z.zone_id=d.zone_id
	LEFT JOIN users ru ON ru.userID=o.reported_by
	LEFT JOIN users au ON au.userID=o.assigned_to
	LEFT JOIN maintenance_plans mp ON mp.plan_id=o.plan_id`

func scanMaintenanceOrder(scanner interface{ Scan(...interface{}) error }) (maintenanceOrderResponse, error) {
	var result maintenanceOrderResponse
	var planID, reportedBy, assignedTo sql.NullInt64
	var dueAt, scheduledAt, startedAt, completedAt sql.NullTime
	var cost sql.NullFloat64
	var createdAt, updatedAt time.Time
	err := scanner.Scan(&result.OrderID, &result.DeviceID, &planID, &result.OrderType, &result.Priority, &result.Status,
		&result.Title, &result.Description, &dueAt, &scheduledAt, &reportedBy, &result.ReportedByName,
		&assignedTo, &result.AssignedToName, &startedAt, &completedAt, &result.Outcome, &result.Resolution,
		&cost, &createdAt, &updatedAt, &result.ProductName, &result.SerialNumber, &result.DeviceCondition,
		&result.ZoneName, &result.PlanName)
	if err != nil {
		return result, err
	}
	if planID.Valid {
		value := planID.Int64
		result.PlanID = &value
	}
	if reportedBy.Valid {
		value := reportedBy.Int64
		result.ReportedBy = &value
	}
	if assignedTo.Valid {
		value := assignedTo.Int64
		result.AssignedTo = &value
	}
	if cost.Valid {
		value := cost.Float64
		result.Cost = &value
	}
	result.DueAt = formatMaintenanceDate(dueAt)
	result.ScheduledAt = formatMaintenanceTime(scheduledAt)
	result.StartedAt = formatMaintenanceTime(startedAt)
	result.CompletedAt = formatMaintenanceTime(completedAt)
	result.CreatedAt = createdAt.Format(time.RFC3339)
	result.UpdatedAt = updatedAt.Format(time.RFC3339)
	return result, nil
}

// GetMaintenanceOverview returns the action-oriented maintenance dashboard.
func GetMaintenanceOverview(w http.ResponseWriter, r *http.Request) {
	db := repository.GetSQLDB()
	if err := ensureDueMaintenanceOrders(db); err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Fällige Wartungsaufträge konnten nicht erzeugt werden"})
		return
	}
	var overdue, dueSoon, defects, inProgress, completedMonth, activePlans, unavailable int
	var monthlyCost float64
	err := db.QueryRow(`SELECT
		COUNT(*) FILTER (WHERE status NOT IN ('completed','cancelled') AND due_at<CURRENT_DATE),
		COUNT(*) FILTER (WHERE status NOT IN ('completed','cancelled') AND due_at BETWEEN CURRENT_DATE AND CURRENT_DATE+30),
		COUNT(*) FILTER (WHERE order_type='defect' AND status NOT IN ('completed','cancelled')),
		COUNT(*) FILTER (WHERE status IN ('in_progress','waiting_parts')),
		COUNT(*) FILTER (WHERE status='completed' AND completed_at>=date_trunc('month',CURRENT_DATE)),
		COALESCE(SUM(cost) FILTER (WHERE status='completed' AND completed_at>=date_trunc('month',CURRENT_DATE)),0)
		FROM maintenance_orders`).Scan(&overdue, &dueSoon, &defects, &inProgress, &completedMonth, &monthlyCost)
	if err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Wartungskennzahlen konnten nicht geladen werden"})
		return
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM maintenance_plans WHERE is_active`).Scan(&activePlans); err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Wartungspläne konnten nicht gezählt werden"})
		return
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM devices WHERE condition_status IN ('blocked','defective','maintenance')`).Scan(&unavailable); err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Gerätezustände konnten nicht gezählt werden"})
		return
	}
	respondJSON(w, http.StatusOK, map[string]interface{}{
		"overdue_orders": overdue, "due_soon_orders": dueSoon, "open_defects": defects,
		"in_progress_orders": inProgress, "completed_this_month": completedMonth,
		"active_plans": activePlans, "unavailable_devices": unavailable, "cost_this_month": monthlyCost,
	})
}

// ListMaintenanceOrders returns either the active worklist or history.
func ListMaintenanceOrders(w http.ResponseWriter, r *http.Request) {
	db := repository.GetSQLDB()
	if err := ensureDueMaintenanceOrders(db); err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Fällige Wartungsaufträge konnten nicht erzeugt werden"})
		return
	}
	query := maintenanceOrderSelect + ` WHERE 1=1`
	args := []interface{}{}
	qb := NewQueryBuilder()
	scope := strings.TrimSpace(r.URL.Query().Get("scope"))
	if scope == "active" {
		query += ` AND o.status NOT IN ('completed','cancelled')`
	} else if scope == "history" {
		query += ` AND o.status IN ('completed','cancelled')`
	}
	if status := strings.TrimSpace(r.URL.Query().Get("status")); status != "" {
		if !maintenanceStatuses[status] {
			respondJSON(w, http.StatusBadRequest, map[string]string{"error": "Ungültiger Wartungsstatus"})
			return
		}
		query += " AND o.status=" + qb.NextPlaceholder()
		args = append(args, status)
	}
	if orderType := strings.TrimSpace(r.URL.Query().Get("type")); orderType != "" {
		if !maintenanceOrderTypes[orderType] {
			respondJSON(w, http.StatusBadRequest, map[string]string{"error": "Ungültige Wartungsart"})
			return
		}
		query += " AND o.order_type=" + qb.NextPlaceholder()
		args = append(args, orderType)
	}
	if search := strings.TrimSpace(r.URL.Query().Get("search")); search != "" {
		placeholder := qb.NextPlaceholder()
		query += " AND (o.title ILIKE " + placeholder + " OR o.device_id ILIKE " + placeholder + " OR p.name ILIKE " + placeholder + " OR d.serialnumber ILIKE " + placeholder + ")"
		args = append(args, "%"+search+"%")
	}
	query += ` ORDER BY
		CASE WHEN o.status NOT IN ('completed','cancelled') AND o.due_at<CURRENT_DATE THEN 0
		     WHEN o.status IN ('in_progress','waiting_parts') THEN 1 ELSE 2 END,
		CASE o.priority WHEN 'critical' THEN 0 WHEN 'high' THEN 1 WHEN 'normal' THEN 2 ELSE 3 END,
		o.due_at NULLS LAST,o.created_at DESC LIMIT 250`
	rows, err := db.Query(query, args...)
	if err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Wartungsaufträge konnten nicht geladen werden"})
		return
	}
	defer rows.Close()
	result := []maintenanceOrderResponse{}
	for rows.Next() {
		item, err := scanMaintenanceOrder(rows)
		if err != nil {
			respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Wartungsauftrag konnte nicht gelesen werden"})
			return
		}
		result = append(result, item)
	}
	respondJSON(w, http.StatusOK, result)
}

type maintenanceOrderInput struct {
	DeviceID    string   `json:"device_id"`
	OrderType   string   `json:"order_type"`
	Priority    string   `json:"priority"`
	Title       string   `json:"title"`
	Description string   `json:"description"`
	DueAt       string   `json:"due_at"`
	AssignedTo  *int64   `json:"assigned_to"`
	Cost        *float64 `json:"cost,omitempty"`
}

func validateMaintenanceOrderInput(input *maintenanceOrderInput) (*time.Time, error) {
	input.DeviceID = strings.TrimSpace(input.DeviceID)
	input.OrderType = strings.TrimSpace(input.OrderType)
	input.Priority = strings.TrimSpace(input.Priority)
	input.Title = strings.TrimSpace(input.Title)
	input.Description = strings.TrimSpace(input.Description)
	if input.DeviceID == "" || input.Title == "" {
		return nil, fmt.Errorf("Gerät und Titel sind erforderlich")
	}
	if !maintenanceOrderTypes[input.OrderType] {
		return nil, fmt.Errorf("Ungültige Wartungsart")
	}
	if input.Priority == "" {
		input.Priority = "normal"
	}
	if !maintenancePriorities[input.Priority] {
		return nil, fmt.Errorf("Ungültige Priorität")
	}
	if input.Cost != nil && *input.Cost < 0 {
		return nil, fmt.Errorf("Kosten dürfen nicht negativ sein")
	}
	return parseMaintenanceDate(input.DueAt)
}

func CreateMaintenanceOrder(w http.ResponseWriter, r *http.Request) {
	var input maintenanceOrderInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "Ungültige Anfrage"})
		return
	}
	dueAt, err := validateMaintenanceOrderInput(&input)
	if err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	db := repository.GetSQLDB()
	tx, err := db.Begin()
	if err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Wartungsauftrag konnte nicht begonnen werden"})
		return
	}
	defer tx.Rollback()
	var condition string
	if err := tx.QueryRow(`SELECT condition_status FROM devices WHERE deviceID=$1 FOR UPDATE`, input.DeviceID).Scan(&condition); errors.Is(err, sql.ErrNoRows) {
		respondJSON(w, http.StatusNotFound, map[string]string{"error": "Gerät wurde nicht gefunden"})
		return
	} else if err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Gerät konnte nicht geprüft werden"})
		return
	}
	if condition == "retired" {
		respondJSON(w, http.StatusConflict, map[string]string{"error": "Für ausgemusterte Geräte kann kein Wartungsauftrag erstellt werden"})
		return
	}
	actor := maintenanceActorID(r)
	var orderID int64
	err = tx.QueryRow(`INSERT INTO maintenance_orders(device_id,order_type,priority,status,title,description,due_at,reported_by,assigned_to,cost)
		VALUES($1,$2,$3,'open',$4,$5,$6,$7,$8,$9) RETURNING order_id`, input.DeviceID, input.OrderType,
		input.Priority, input.Title, maintenanceNullableString(input.Description), dueAt, actor, input.AssignedTo, input.Cost).Scan(&orderID)
	if err != nil {
		respondJSON(w, http.StatusConflict, map[string]string{"error": "Wartungsauftrag konnte nicht erstellt werden"})
		return
	}
	if _, err := tx.Exec(`INSERT INTO maintenance_order_events(order_id,event_type,to_status,notes,actor_id) VALUES($1,'created','open',$2,$3)`, orderID, input.Description, actor); err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Wartungsereignis konnte nicht protokolliert werden"})
		return
	}
	if input.OrderType == "defect" {
		if _, err := tx.Exec(`UPDATE devices SET condition_status='defective',updated_at=CURRENT_TIMESTAMP WHERE deviceID=$1`, input.DeviceID); err != nil {
			respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Gerätezustand konnte nicht aktualisiert werden"})
			return
		}
	}
	if err := tx.Commit(); err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Wartungsauftrag konnte nicht gespeichert werden"})
		return
	}
	respondJSON(w, http.StatusCreated, map[string]interface{}{"order_id": orderID, "message": "Wartungsauftrag erstellt"})
}

func UpdateMaintenanceOrder(w http.ResponseWriter, r *http.Request) {
	orderID, err := strconv.ParseInt(mux.Vars(r)["id"], 10, 64)
	if err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "Ungültiger Wartungsauftrag"})
		return
	}
	var input maintenanceOrderInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "Ungültige Anfrage"})
		return
	}
	dueAt, err := validateMaintenanceOrderInput(&input)
	if err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	tx, err := repository.GetSQLDB().Begin()
	if err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Wartungsauftrag konnte nicht begonnen werden"})
		return
	}
	defer tx.Rollback()
	result, err := tx.Exec(`UPDATE maintenance_orders SET priority=$1,title=$2,
		description=$3,due_at=$4,assigned_to=$5,cost=$6,updated_at=CURRENT_TIMESTAMP
		WHERE order_id=$7 AND device_id=$8 AND order_type=$9 AND status NOT IN ('completed','cancelled')`,
		input.Priority, input.Title, maintenanceNullableString(input.Description), dueAt, input.AssignedTo,
		input.Cost, orderID, input.DeviceID, input.OrderType)
	if err != nil {
		respondJSON(w, http.StatusConflict, map[string]string{"error": "Wartungsauftrag konnte nicht aktualisiert werden"})
		return
	}
	if count, _ := result.RowsAffected(); count == 0 {
		respondJSON(w, http.StatusNotFound, map[string]string{"error": "Aktiver Wartungsauftrag wurde nicht gefunden"})
		return
	}
	if _, err := tx.Exec(`INSERT INTO maintenance_order_events(order_id,event_type,notes,actor_id)
		VALUES($1,'updated','Stammdaten des Auftrags aktualisiert',$2)`, orderID, maintenanceActorID(r)); err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Änderung konnte nicht protokolliert werden"})
		return
	}
	if err := tx.Commit(); err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Wartungsauftrag konnte nicht gespeichert werden"})
		return
	}
	respondJSON(w, http.StatusOK, map[string]string{"message": "Wartungsauftrag aktualisiert"})
}

func validMaintenanceTransition(from, to string) bool {
	allowed := map[string]map[string]bool{
		"open":          {"planned": true, "in_progress": true, "cancelled": true},
		"planned":       {"open": true, "in_progress": true, "cancelled": true},
		"in_progress":   {"waiting_parts": true, "completed": true, "cancelled": true},
		"waiting_parts": {"in_progress": true, "completed": true, "cancelled": true},
	}
	return allowed[from][to]
}

type maintenanceTransitionInput struct {
	Status     string   `json:"status"`
	Outcome    string   `json:"outcome"`
	Resolution string   `json:"resolution"`
	Notes      string   `json:"notes"`
	Cost       *float64 `json:"cost"`
	NextDueAt  string   `json:"next_due_at"`
}

func syncDeviceConditionAfterMaintenance(tx *sql.Tx, deviceID string, orderID int64, status, outcome string) error {
	if status == "in_progress" || status == "waiting_parts" {
		_, err := tx.Exec(`UPDATE devices SET condition_status='maintenance',updated_at=CURRENT_TIMESTAMP
			WHERE deviceID=$1 AND condition_status NOT IN ('retired','blocked')`, deviceID)
		return err
	}
	if status != "completed" && status != "cancelled" {
		return nil
	}
	if outcome == "failed" {
		_, err := tx.Exec(`UPDATE devices SET condition_status='defective',updated_at=CURRENT_TIMESTAMP WHERE deviceID=$1 AND condition_status<>'retired'`, deviceID)
		return err
	}
	var defectBlockers, workBlockers int
	err := tx.QueryRow(`SELECT
		COUNT(*) FILTER (WHERE order_type='defect'),
		COUNT(*) FILTER (WHERE status IN ('in_progress','waiting_parts'))
		FROM maintenance_orders WHERE device_id=$1 AND order_id<>$2 AND status IN ('open','planned','in_progress','waiting_parts')`, deviceID, orderID).
		Scan(&defectBlockers, &workBlockers)
	if err != nil {
		return err
	}
	target := "available"
	if defectBlockers > 0 {
		target = "defective"
	} else if workBlockers > 0 {
		target = "maintenance"
	}
	_, err = tx.Exec(`UPDATE devices SET condition_status=$1,updated_at=CURRENT_TIMESTAMP
		WHERE deviceID=$2 AND condition_status NOT IN ('retired','blocked')`, target, deviceID)
	return err
}

func TransitionMaintenanceOrder(w http.ResponseWriter, r *http.Request) {
	orderID, err := strconv.ParseInt(mux.Vars(r)["id"], 10, 64)
	if err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "Ungültiger Wartungsauftrag"})
		return
	}
	var input maintenanceTransitionInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "Ungültige Anfrage"})
		return
	}
	input.Status = strings.TrimSpace(input.Status)
	input.Outcome = strings.TrimSpace(input.Outcome)
	input.Resolution = strings.TrimSpace(input.Resolution)
	input.Notes = strings.TrimSpace(input.Notes)
	if !maintenanceStatuses[input.Status] {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "Ungültiger Zielstatus"})
		return
	}
	if input.Status == "completed" {
		if !maintenanceOutcomes[input.Outcome] || input.Resolution == "" {
			respondJSON(w, http.StatusBadRequest, map[string]string{"error": "Ergebnis und Abschlussnotiz sind erforderlich"})
			return
		}
		if input.Cost != nil && *input.Cost < 0 {
			respondJSON(w, http.StatusBadRequest, map[string]string{"error": "Kosten dürfen nicht negativ sein"})
			return
		}
	}
	nextDueAt, err := parseMaintenanceDate(input.NextDueAt)
	if err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	db := repository.GetSQLDB()
	tx, err := db.Begin()
	if err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Statuswechsel konnte nicht begonnen werden"})
		return
	}
	defer tx.Rollback()
	var currentStatus, deviceID string
	var planID sql.NullInt64
	if err := tx.QueryRow(`SELECT status,device_id,plan_id FROM maintenance_orders WHERE order_id=$1 FOR UPDATE`, orderID).Scan(&currentStatus, &deviceID, &planID); errors.Is(err, sql.ErrNoRows) {
		respondJSON(w, http.StatusNotFound, map[string]string{"error": "Wartungsauftrag wurde nicht gefunden"})
		return
	} else if err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Wartungsauftrag konnte nicht geladen werden"})
		return
	}
	if !validMaintenanceTransition(currentStatus, input.Status) {
		respondJSON(w, http.StatusConflict, map[string]string{"error": fmt.Sprintf("Statuswechsel von %s nach %s ist nicht zulässig", currentStatus, input.Status)})
		return
	}
	actor := maintenanceActorID(r)
	_, err = tx.Exec(`UPDATE maintenance_orders SET status=$1,
		started_at=CASE WHEN $1='in_progress' THEN COALESCE(started_at,CURRENT_TIMESTAMP) ELSE started_at END,
		completed_at=CASE WHEN $1 IN ('completed','cancelled') THEN CURRENT_TIMESTAMP ELSE completed_at END,
		outcome=CASE WHEN $1='completed' THEN $2 ELSE outcome END,
		resolution=CASE WHEN $1='completed' THEN $3 ELSE resolution END,
		cost=CASE WHEN $1='completed' THEN COALESCE($4,cost) ELSE cost END,updated_at=CURRENT_TIMESTAMP
		WHERE order_id=$5`, input.Status, maintenanceNullableString(input.Outcome), maintenanceNullableString(input.Resolution), input.Cost, orderID)
	if err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Wartungsstatus konnte nicht aktualisiert werden"})
		return
	}
	eventNotes := input.Notes
	if eventNotes == "" && input.Status == "completed" {
		eventNotes = input.Resolution
	}
	if _, err := tx.Exec(`INSERT INTO maintenance_order_events(order_id,event_type,from_status,to_status,notes,actor_id)
		VALUES($1,'status_changed',$2,$3,$4,$5)`, orderID, currentStatus, input.Status, maintenanceNullableString(eventNotes), actor); err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Statusereignis konnte nicht protokolliert werden"})
		return
	}
	var effectiveNextDue *time.Time
	if input.Status == "completed" && planID.Valid {
		var next time.Time
		if err := tx.QueryRow(`UPDATE maintenance_plans SET last_completed_at=CURRENT_TIMESTAMP,
			next_due_at=COALESCE($1::date,CURRENT_DATE+interval_days),updated_at=CURRENT_TIMESTAMP WHERE plan_id=$2 RETURNING next_due_at`, nextDueAt, planID.Int64).Scan(&next); err != nil {
			respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Nächster Wartungstermin konnte nicht berechnet werden"})
			return
		}
		effectiveNextDue = &next
	} else if input.Status == "cancelled" && planID.Valid {
		// Cancelling a generated order explicitly skips this plan cycle. Advancing
		// the due date prevents the next overview refresh from recreating the same
		// work immediately while keeping the recurring plan active.
		if _, err := tx.Exec(`UPDATE maintenance_plans SET
			next_due_at=GREATEST(next_due_at+interval_days,CURRENT_DATE+interval_days),
			updated_at=CURRENT_TIMESTAMP WHERE plan_id=$1`, planID.Int64); err != nil {
			respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Wartungsintervall konnte nicht übersprungen werden"})
			return
		}
	} else if input.Status == "completed" {
		effectiveNextDue = nextDueAt
	}
	if input.Status == "completed" {
		if _, err := tx.Exec(`UPDATE devices SET lastmaintenance=CURRENT_DATE,nextmaintenance=$1,updated_at=CURRENT_TIMESTAMP WHERE deviceID=$2`, effectiveNextDue, deviceID); err != nil {
			respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Wartungsdaten des Geräts konnten nicht aktualisiert werden"})
			return
		}
	}
	if planID.Valid && (input.Status == "completed" || input.Status == "cancelled") {
		if err := syncDeviceNextMaintenance(tx, deviceID); err != nil {
			respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Nächster Gerätetermin konnte nicht synchronisiert werden"})
			return
		}
	}
	if err := syncDeviceConditionAfterMaintenance(tx, deviceID, orderID, input.Status, input.Outcome); err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Gerätezustand konnte nicht synchronisiert werden"})
		return
	}
	if err := tx.Commit(); err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Statuswechsel konnte nicht gespeichert werden"})
		return
	}
	respondJSON(w, http.StatusOK, map[string]string{"message": "Wartungsstatus aktualisiert"})
}

func GetMaintenanceOrderEvents(w http.ResponseWriter, r *http.Request) {
	orderID, err := strconv.ParseInt(mux.Vars(r)["id"], 10, 64)
	if err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "Ungültiger Wartungsauftrag"})
		return
	}
	rows, err := repository.GetSQLDB().Query(`SELECT e.event_id,e.event_type,COALESCE(e.from_status,''),COALESCE(e.to_status,''),
		COALESCE(e.notes,''),e.actor_id,COALESCE(NULLIF(CONCAT_WS(' ',u.first_name,u.last_name),''),u.username,''),e.created_at
		FROM maintenance_order_events e LEFT JOIN users u ON u.userID=e.actor_id WHERE e.order_id=$1 ORDER BY e.created_at DESC,e.event_id DESC`, orderID)
	if err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Wartungshistorie konnte nicht geladen werden"})
		return
	}
	defer rows.Close()
	type eventResponse struct {
		EventID    int64  `json:"event_id"`
		EventType  string `json:"event_type"`
		FromStatus string `json:"from_status,omitempty"`
		ToStatus   string `json:"to_status,omitempty"`
		Notes      string `json:"notes,omitempty"`
		ActorID    *int64 `json:"actor_id,omitempty"`
		ActorName  string `json:"actor_name,omitempty"`
		CreatedAt  string `json:"created_at"`
	}
	result := []eventResponse{}
	for rows.Next() {
		var item eventResponse
		var actorID sql.NullInt64
		var createdAt time.Time
		if err := rows.Scan(&item.EventID, &item.EventType, &item.FromStatus, &item.ToStatus, &item.Notes, &actorID, &item.ActorName, &createdAt); err != nil {
			respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Wartungsereignis konnte nicht gelesen werden"})
			return
		}
		if actorID.Valid {
			value := actorID.Int64
			item.ActorID = &value
		}
		item.CreatedAt = createdAt.Format(time.RFC3339)
		result = append(result, item)
	}
	respondJSON(w, http.StatusOK, result)
}

type maintenancePlanInput struct {
	DeviceID        string `json:"device_id"`
	Name            string `json:"name"`
	MaintenanceType string `json:"maintenance_type"`
	IntervalDays    int    `json:"interval_days"`
	LeadTimeDays    int    `json:"lead_time_days"`
	Instructions    string `json:"instructions"`
	NextDueAt       string `json:"next_due_at"`
	IsActive        *bool  `json:"is_active"`
}

func validateMaintenancePlanInput(input *maintenancePlanInput) (*time.Time, error) {
	input.DeviceID = strings.TrimSpace(input.DeviceID)
	input.Name = strings.TrimSpace(input.Name)
	input.MaintenanceType = strings.TrimSpace(input.MaintenanceType)
	input.Instructions = strings.TrimSpace(input.Instructions)
	if input.DeviceID == "" || input.Name == "" {
		return nil, fmt.Errorf("Gerät und Planname sind erforderlich")
	}
	if input.MaintenanceType == "defect" || !maintenanceOrderTypes[input.MaintenanceType] {
		return nil, fmt.Errorf("Wartungspläne unterstützen Wartung, Prüfung oder Kalibrierung")
	}
	if input.IntervalDays < 1 || input.IntervalDays > 3650 {
		return nil, fmt.Errorf("Intervall muss zwischen 1 und 3650 Tagen liegen")
	}
	if input.LeadTimeDays < 0 || input.LeadTimeDays > 365 {
		return nil, fmt.Errorf("Vorlauf muss zwischen 0 und 365 Tagen liegen")
	}
	nextDueAt, err := parseMaintenanceDate(input.NextDueAt)
	if err != nil || nextDueAt == nil {
		if err != nil {
			return nil, err
		}
		return nil, fmt.Errorf("Nächste Fälligkeit ist erforderlich")
	}
	return nextDueAt, nil
}

func ListMaintenancePlans(w http.ResponseWriter, r *http.Request) {
	rows, err := repository.GetSQLDB().Query(`SELECT mp.plan_id,mp.device_id,mp.name,mp.maintenance_type,mp.interval_days,
		mp.lead_time_days,COALESCE(mp.instructions,''),mp.next_due_at,mp.last_completed_at,mp.is_active,mp.created_at,mp.updated_at,
		COALESCE(p.name,''),COALESCE(d.serialnumber,''),d.condition_status,
		EXISTS(SELECT 1 FROM maintenance_orders o WHERE o.plan_id=mp.plan_id AND o.status IN ('open','planned','in_progress','waiting_parts'))
		FROM maintenance_plans mp JOIN devices d ON d.deviceID=mp.device_id LEFT JOIN products p ON p.productID=d.productID
		ORDER BY mp.is_active DESC,mp.next_due_at,mp.name`)
	if err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Wartungspläne konnten nicht geladen werden"})
		return
	}
	defer rows.Close()
	type planResponse struct {
		PlanID          int64   `json:"plan_id"`
		DeviceID        string  `json:"device_id"`
		Name            string  `json:"name"`
		MaintenanceType string  `json:"maintenance_type"`
		IntervalDays    int     `json:"interval_days"`
		LeadTimeDays    int     `json:"lead_time_days"`
		Instructions    string  `json:"instructions,omitempty"`
		NextDueAt       string  `json:"next_due_at"`
		LastCompletedAt *string `json:"last_completed_at,omitempty"`
		IsActive        bool    `json:"is_active"`
		CreatedAt       string  `json:"created_at"`
		UpdatedAt       string  `json:"updated_at"`
		ProductName     string  `json:"product_name,omitempty"`
		SerialNumber    string  `json:"serial_number,omitempty"`
		DeviceCondition string  `json:"device_condition"`
		HasActiveOrder  bool    `json:"has_active_order"`
	}
	result := []planResponse{}
	for rows.Next() {
		var item planResponse
		var nextDue, lastCompleted sql.NullTime
		var createdAt, updatedAt time.Time
		if err := rows.Scan(&item.PlanID, &item.DeviceID, &item.Name, &item.MaintenanceType, &item.IntervalDays, &item.LeadTimeDays,
			&item.Instructions, &nextDue, &lastCompleted, &item.IsActive, &createdAt, &updatedAt, &item.ProductName,
			&item.SerialNumber, &item.DeviceCondition, &item.HasActiveOrder); err != nil {
			respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Wartungsplan konnte nicht gelesen werden"})
			return
		}
		if nextDue.Valid {
			item.NextDueAt = nextDue.Time.Format("2006-01-02")
		}
		item.LastCompletedAt = formatMaintenanceTime(lastCompleted)
		item.CreatedAt = createdAt.Format(time.RFC3339)
		item.UpdatedAt = updatedAt.Format(time.RFC3339)
		result = append(result, item)
	}
	respondJSON(w, http.StatusOK, result)
}

func CreateMaintenancePlan(w http.ResponseWriter, r *http.Request) {
	var input maintenancePlanInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "Ungültige Anfrage"})
		return
	}
	nextDueAt, err := validateMaintenancePlanInput(&input)
	if err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	active := true
	if input.IsActive != nil {
		active = *input.IsActive
	}
	tx, err := repository.GetSQLDB().Begin()
	if err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Wartungsplan konnte nicht begonnen werden"})
		return
	}
	defer tx.Rollback()
	var planID int64
	err = tx.QueryRow(`INSERT INTO maintenance_plans(device_id,name,maintenance_type,interval_days,lead_time_days,
		instructions,next_due_at,is_active,created_by) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9) RETURNING plan_id`, input.DeviceID,
		input.Name, input.MaintenanceType, input.IntervalDays, input.LeadTimeDays, maintenanceNullableString(input.Instructions), nextDueAt, active, maintenanceActorID(r)).Scan(&planID)
	if err != nil {
		respondJSON(w, http.StatusConflict, map[string]string{"error": "Wartungsplan konnte nicht erstellt werden; Gerät und Planname müssen eindeutig sein"})
		return
	}
	if err := syncDeviceNextMaintenance(tx, input.DeviceID); err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Nächster Gerätetermin konnte nicht synchronisiert werden"})
		return
	}
	if err := tx.Commit(); err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Wartungsplan konnte nicht gespeichert werden"})
		return
	}
	if active {
		_ = ensureDueMaintenanceOrders(repository.GetSQLDB())
	}
	respondJSON(w, http.StatusCreated, map[string]interface{}{"plan_id": planID, "message": "Wartungsplan erstellt"})
}

func UpdateMaintenancePlan(w http.ResponseWriter, r *http.Request) {
	planID, err := strconv.ParseInt(mux.Vars(r)["id"], 10, 64)
	if err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "Ungültiger Wartungsplan"})
		return
	}
	var input maintenancePlanInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "Ungültige Anfrage"})
		return
	}
	nextDueAt, err := validateMaintenancePlanInput(&input)
	if err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	active := true
	if input.IsActive != nil {
		active = *input.IsActive
	}
	tx, err := repository.GetSQLDB().Begin()
	if err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Wartungsplan konnte nicht begonnen werden"})
		return
	}
	defer tx.Rollback()
	result, err := tx.Exec(`UPDATE maintenance_plans SET name=$1,maintenance_type=$2,interval_days=$3,
		lead_time_days=$4,instructions=$5,next_due_at=$6,is_active=$7,updated_at=CURRENT_TIMESTAMP
		WHERE plan_id=$8 AND device_id=$9`, input.Name, input.MaintenanceType, input.IntervalDays,
		input.LeadTimeDays, maintenanceNullableString(input.Instructions), nextDueAt, active, planID, input.DeviceID)
	if err != nil {
		respondJSON(w, http.StatusConflict, map[string]string{"error": "Wartungsplan konnte nicht aktualisiert werden"})
		return
	}
	if count, _ := result.RowsAffected(); count == 0 {
		respondJSON(w, http.StatusNotFound, map[string]string{"error": "Wartungsplan wurde nicht gefunden"})
		return
	}
	if err := syncDeviceNextMaintenance(tx, input.DeviceID); err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Nächster Gerätetermin konnte nicht synchronisiert werden"})
		return
	}
	if err := tx.Commit(); err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Wartungsplan konnte nicht gespeichert werden"})
		return
	}
	if active {
		_ = ensureDueMaintenanceOrders(repository.GetSQLDB())
	}
	respondJSON(w, http.StatusOK, map[string]string{"message": "Wartungsplan aktualisiert"})
}

func GetMaintenanceOptions(w http.ResponseWriter, r *http.Request) {
	type deviceOption struct {
		DeviceID        string `json:"device_id"`
		ProductName     string `json:"product_name"`
		SerialNumber    string `json:"serial_number,omitempty"`
		Barcode         string `json:"barcode,omitempty"`
		Status          string `json:"status"`
		ConditionStatus string `json:"condition_status"`
	}
	devices := []deviceOption{}
	rows, err := repository.GetSQLDB().Query(`SELECT d.deviceID,COALESCE(p.name,''),COALESCE(d.serialnumber,''),COALESCE(d.barcode,''),d.status,d.condition_status
		FROM devices d LEFT JOIN products p ON p.productID=d.productID WHERE d.condition_status<>'retired' ORDER BY p.name,d.deviceID`)
	if err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Geräteauswahl konnte nicht geladen werden"})
		return
	}
	for rows.Next() {
		var item deviceOption
		if err := rows.Scan(&item.DeviceID, &item.ProductName, &item.SerialNumber, &item.Barcode, &item.Status, &item.ConditionStatus); err != nil {
			rows.Close()
			respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Gerät konnte nicht gelesen werden"})
			return
		}
		devices = append(devices, item)
	}
	rows.Close()
	type userOption struct {
		UserID int64  `json:"user_id"`
		Name   string `json:"name"`
	}
	users := []userOption{}
	userRows, err := repository.GetSQLDB().Query(`SELECT userID,COALESCE(NULLIF(CONCAT_WS(' ',first_name,last_name),''),username) FROM users WHERE is_active ORDER BY 2`)
	if err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Mitarbeiterauswahl konnte nicht geladen werden"})
		return
	}
	defer userRows.Close()
	for userRows.Next() {
		var item userOption
		if err := userRows.Scan(&item.UserID, &item.Name); err != nil {
			respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Mitarbeiter konnte nicht gelesen werden"})
			return
		}
		users = append(users, item)
	}
	respondJSON(w, http.StatusOK, map[string]interface{}{"devices": devices, "users": users})
}
