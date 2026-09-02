package handlers

import (
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/gorilla/mux"

	"warehousecore/internal/repository"
)

// GetJobPicklist handles GET /api/v1/jobs/{id}/picklist
func GetJobPicklist(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	jobIDStr := vars["id"]
	jobID, err := strconv.Atoi(jobIDStr)
	if err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid job ID"})
		return
	}

	db := repository.GetSQLDB()

	rows, err := db.Query(`
		SELECT
			jp.position_id,
			jp.product_id,
			COALESCE(p.name, jp.description, '') AS product_name,
			jp.quantity AS needed,
			COUNT(jpd.id) AS scanned,
			jp.sort_order
		FROM job_positions jp
		LEFT JOIN products p ON p.productid = jp.product_id
		LEFT JOIN job_position_devices jpd ON jpd.position_id = jp.position_id
		WHERE jp.job_id = $1 AND jp.position_type = 'product'
		GROUP BY jp.position_id, jp.product_id, p.name, jp.description, jp.quantity, jp.sort_order
		ORDER BY jp.sort_order ASC, jp.position_id ASC
	`, jobID)
	if err != nil {
		log.Printf("Error querying picklist for job %d: %v", jobID, err)
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	defer rows.Close()

	type PicklistDevice struct {
		DeviceID  string    `json:"device_id"`
		ScannedAt time.Time `json:"scanned_at"`
		ScannedBy string    `json:"scanned_by"`
	}

	type PicklistPosition struct {
		PositionID  int              `json:"position_id"`
		ProductID   *int             `json:"product_id"`
		ProductName string           `json:"product_name"`
		Needed      float64          `json:"needed"`
		Scanned     int              `json:"scanned"`
		Fulfilled   bool             `json:"fulfilled"`
		Devices     []PicklistDevice `json:"devices"`
	}

	positions := []PicklistPosition{}
	for rows.Next() {
		var pos PicklistPosition
		var productID sql.NullInt64
		var sortOrder sql.NullInt64

		if err := rows.Scan(
			&pos.PositionID,
			&productID,
			&pos.ProductName,
			&pos.Needed,
			&pos.Scanned,
			&sortOrder,
		); err != nil {
			log.Printf("Error scanning picklist row: %v", err)
			continue
		}
		if productID.Valid {
			id := int(productID.Int64)
			pos.ProductID = &id
		}
		pos.Fulfilled = float64(pos.Scanned) >= pos.Needed
		pos.Devices = []PicklistDevice{}
		positions = append(positions, pos)
	}
	rows.Close()

	// Fetch devices for each position
	for i, pos := range positions {
		devRows, err := db.Query(`
			SELECT device_id, scanned_at, scanned_by
			FROM job_position_devices
			WHERE position_id = $1
		`, pos.PositionID)
		if err != nil {
			log.Printf("Error querying devices for position %d: %v", pos.PositionID, err)
			continue
		}
		defer devRows.Close() // FIXED: added defer rows.Close()
		devices := []PicklistDevice{}
		for devRows.Next() {
			var d PicklistDevice
			if err := devRows.Scan(&d.DeviceID, &d.ScannedAt, &d.ScannedBy); err != nil {
				log.Printf("Error scanning device row for position %d: %v", pos.PositionID, err)
				continue
			}
			devices = append(devices, d)
		}
		devRows.Close()
		positions[i].Devices = devices
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"job_id":    jobID,
		"positions": positions,
	})
}

// ScanDeviceToPicklist handles POST /api/v1/jobs/{id}/picklist/scan
func ScanDeviceToPicklist(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	jobIDStr := vars["id"]
	jobID, err := strconv.Atoi(jobIDStr)
	if err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid job ID"})
		return
	}

	var req struct {
		DeviceID  string `json:"device_id"`
		ScannedBy string `json:"scanned_by"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid request body"})
		return
	}
	if req.DeviceID == "" {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "device_id is required"})
		return
	}

	db := repository.GetSQLDB()

	// Find a matching open position for this device
	var positionID int
	var quantity int
	err = db.QueryRow(`
		SELECT jp.position_id, jp.quantity
		FROM job_positions jp
		JOIN devices d ON d.productid = jp.product_id
		LEFT JOIN (
			SELECT position_id, COUNT(*) AS cnt FROM job_position_devices GROUP BY position_id
		) counts ON counts.position_id = jp.position_id
		WHERE jp.job_id = $1
		  AND jp.position_type = 'product'
		  AND d.deviceid = $2
		  AND COALESCE(counts.cnt, 0) < jp.quantity
		ORDER BY jp.sort_order ASC
		LIMIT 1
	`, jobID, req.DeviceID).Scan(&positionID, &quantity)

	if err == sql.ErrNoRows {
		respondJSON(w, http.StatusNotFound, map[string]string{"error": "No matching open position found for this device"})
		return
	}
	if err != nil {
		log.Printf("Error finding position for device %s on job %d: %v", req.DeviceID, jobID, err)
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	// Insert device into position (ignore duplicate)
	_, err = db.Exec(`
		INSERT INTO job_position_devices (position_id, device_id, scanned_by)
		VALUES ($1, $2, $3)
		ON CONFLICT (position_id, device_id) DO NOTHING
	`, positionID, req.DeviceID, req.ScannedBy)
	if err != nil {
		log.Printf("Error inserting device %s into position %d: %v", req.DeviceID, positionID, err)
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"position_id": positionID,
		"device_id":   req.DeviceID,
		"message":     "Device assigned",
	})
}
