package handlers

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"warehousecore/internal/middleware"
	"warehousecore/internal/repository"
)

func createManufacturerMCP(w http.ResponseWriter, r *http.Request, rawName string, rawWebsite *string) {
	name := strings.TrimSpace(rawName)
	if name == "" || len([]rune(name)) > 255 {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "Manufacturer name must contain 1-255 characters"})
		return
	}
	website, valid := normalizedManufacturerWebsite(rawWebsite)
	if !valid {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "Website must be an HTTP(S) URL of at most 255 characters"})
		return
	}
	tx, err := repository.GetSQLDB().BeginTx(r.Context(), nil)
	if err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to begin manufacturer creation"})
		return
	}
	defer tx.Rollback()
	receiptID, replay, err := beginWarehouseProductMutation(tx, r, "manufacturer.create", map[string]any{"name": name, "website": website})
	if err != nil {
		respondWarehouseMutationError(w, err)
		return
	}
	if replay != nil {
		respondJSON(w, http.StatusCreated, replay)
		return
	}
	// Serialize duplicate checks with standalone master updates before querying.
	if _, err := tx.Exec(`SET LOCAL lock_timeout='5s'; SET LOCAL statement_timeout='15s'; LOCK TABLE manufacturer,brands IN SHARE ROW EXCLUSIVE MODE`); err != nil {
		respondWarehouseMutationError(w, err)
		return
	}
	if _, err := tx.Exec(`SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, "warehouse.manufacturer."+strings.ToLower(name)); err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to lock manufacturer name"})
		return
	}
	var existingID int
	if err := tx.QueryRow(`SELECT manufacturerid FROM manufacturer WHERE lower(trim(name))=lower($1) LIMIT 1`, name).Scan(&existingID); err == nil {
		respondJSON(w, http.StatusConflict, map[string]any{"error": "Manufacturer already exists", "manufacturer_id": existingID})
		return
	} else if err != sql.ErrNoRows {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to check manufacturer duplicates"})
		return
	}
	var id int
	if err := tx.QueryRow(`INSERT INTO manufacturer(name,website) VALUES($1,$2) RETURNING manufacturerid`, name, website).Scan(&id); err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to create manufacturer"})
		return
	}
	after := map[string]any{"origin": "MCP/AI", "name": name, "website": website}
	if err := recordWarehouseMasterAudit(tx, r, "manufacturer", id, after); err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to audit manufacturer"})
		return
	}
	response := map[string]any{"manufacturer_id": id, "name": name, "website": website}
	if err := completeWarehouseProductMutation(tx, receiptID, http.StatusCreated, response); err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to store manufacturer receipt"})
		return
	}
	if err := tx.Commit(); err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to commit manufacturer"})
		return
	}
	respondJSON(w, http.StatusCreated, response)
}

func createBrandMCP(w http.ResponseWriter, r *http.Request, rawName string, manufacturerID *int) {
	name := strings.TrimSpace(rawName)
	if name == "" || len([]rune(name)) > 255 || manufacturerID == nil || *manufacturerID <= 0 {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "Brand name and existing manufacturer_id are required"})
		return
	}
	tx, err := repository.GetSQLDB().BeginTx(r.Context(), nil)
	if err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to begin brand creation"})
		return
	}
	defer tx.Rollback()
	receiptID, replay, err := beginWarehouseProductMutation(tx, r, "brand.create", map[string]any{"name": name, "manufacturer_id": *manufacturerID})
	if err != nil {
		respondWarehouseMutationError(w, err)
		return
	}
	if replay != nil {
		respondJSON(w, http.StatusCreated, replay)
		return
	}
	// Serialize duplicate checks with standalone master updates before querying.
	if _, err := tx.Exec(`SET LOCAL lock_timeout='5s'; SET LOCAL statement_timeout='15s'; LOCK TABLE manufacturer,brands IN SHARE ROW EXCLUSIVE MODE`); err != nil {
		respondWarehouseMutationError(w, err)
		return
	}
	var manufacturerName string
	if err := tx.QueryRow(`SELECT name FROM manufacturer WHERE manufacturerid=$1 FOR KEY SHARE`, *manufacturerID).Scan(&manufacturerName); err == sql.ErrNoRows {
		respondJSON(w, http.StatusNotFound, map[string]string{"error": "Manufacturer not found"})
		return
	} else if err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to load manufacturer"})
		return
	}
	if _, err := tx.Exec(`SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, "warehouse.brand."+strings.ToLower(name)+"."+strconv.Itoa(*manufacturerID)); err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to lock brand name"})
		return
	}
	var existingID int
	if err := tx.QueryRow(`SELECT brandid FROM brands WHERE manufacturerid=$1 AND lower(trim(name))=lower($2) LIMIT 1`, *manufacturerID, name).Scan(&existingID); err == nil {
		respondJSON(w, http.StatusConflict, map[string]any{"error": "Brand already exists for manufacturer", "brand_id": existingID})
		return
	} else if err != sql.ErrNoRows {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to check brand duplicates"})
		return
	}
	var id int
	if err := tx.QueryRow(`INSERT INTO brands(name,manufacturerid) VALUES($1,$2) RETURNING brandid`, name, *manufacturerID).Scan(&id); err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to create brand"})
		return
	}
	after := map[string]any{"origin": "MCP/AI", "name": name, "manufacturer_id": *manufacturerID}
	if err := recordWarehouseMasterAudit(tx, r, "brand", id, after); err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to audit brand"})
		return
	}
	response := map[string]any{"brand_id": id, "name": name, "manufacturer_id": *manufacturerID, "manufacturer_name": manufacturerName}
	if err := completeWarehouseProductMutation(tx, receiptID, http.StatusCreated, response); err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to store brand receipt"})
		return
	}
	if err := tx.Commit(); err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to commit brand"})
		return
	}
	respondJSON(w, http.StatusCreated, response)
}

func normalizedManufacturerWebsite(raw *string) (*string, bool) {
	if raw == nil {
		return nil, true
	}
	value := strings.TrimSpace(*raw)
	if value == "" {
		return nil, true
	}
	if !strings.Contains(value, "://") {
		value = "https://" + value
	}
	parsed, err := url.Parse(value)
	if err != nil || (parsed.Scheme != "https" && parsed.Scheme != "http") || parsed.Hostname() == "" || parsed.User != nil || len([]rune(value)) > 255 {
		return nil, false
	}
	return &value, true
}

func recordWarehouseMasterAudit(tx *sql.Tx, r *http.Request, entity string, id any, after any) error {
	user, _ := middleware.GetUserFromContext(r)
	if user == nil || user.UserID == 0 {
		return &warehouseMutationError{http.StatusUnauthorized, "user_required", "A signed-in suite user is required"}
	}
	encoded, err := json.Marshal(after)
	if err != nil {
		return err
	}
	ip := r.RemoteAddr
	if host, _, splitErr := net.SplitHostPort(ip); splitErr == nil {
		ip = host
	}
	if len(ip) > 45 {
		ip = ip[:45]
	}
	_, err = tx.Exec(`INSERT INTO audit_log(user_id,action,entity_type,entity_id,old_values,new_values,ip_address,user_agent)
		VALUES($1,$2,$3,$4,'{}'::jsonb,$5::jsonb,$6,$7)`, user.UserID, entity+".create", entity, fmt.Sprint(id), string(encoded), ip, r.UserAgent())
	return err
}
