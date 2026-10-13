package handlers

import (
	"database/sql"
	"encoding/json"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/mux"
	"warehousecore/internal/repository"
)

type productStockLocation struct {
	Source   string  `json:"source"`
	ID       *int    `json:"id"`
	Name     string  `json:"name"`
	Code     string  `json:"code"`
	Quantity float64 `json:"quantity"`
}

func GetProductStock(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(mux.Vars(r)["id"])
	if err != nil {
		respondJSON(w, 400, map[string]string{"error": "Invalid product ID"})
		return
	}
	rows, err := repository.GetSQLDB().QueryContext(r.Context(), `SELECT 'zone',pl.zone_id,COALESCE(z.name,'Ohne Lagerzone'),COALESCE(z.code,''),pl.quantity FROM product_locations pl LEFT JOIN storage_zones z ON z.zone_id=pl.zone_id WHERE pl.product_id=$1
 UNION ALL SELECT 'case',c.caseid,c.name,COALESCE(c.case_code,''),pc.quantity FROM case_product_contents pc JOIN cases c ON c.caseid=pc.case_id WHERE pc.product_id=$1`, id)
	if err != nil {
		respondJSON(w, 500, map[string]string{"error": "Bestand konnte nicht geladen werden"})
		return
	}
	defer rows.Close()
	result := []productStockLocation{}
	for rows.Next() {
		var item productStockLocation
		if err = rows.Scan(&item.Source, &item.ID, &item.Name, &item.Code, &item.Quantity); err != nil {
			respondJSON(w, 500, map[string]string{"error": "Bestand konnte nicht geladen werden"})
			return
		}
		result = append(result, item)
	}
	if err = rows.Err(); err != nil {
		respondJSON(w, 500, map[string]string{"error": "Bestand konnte nicht geladen werden"})
		return
	}
	respondJSON(w, 200, result)
}
func CorrectProductStock(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(mux.Vars(r)["id"])
	if err != nil {
		respondJSON(w, 400, map[string]string{"error": "Invalid product ID"})
		return
	}
	var in struct {
		ZoneID           *int     `json:"zone_id"`
		Quantity         float64  `json:"quantity"`
		ExpectedQuantity *float64 `json:"expected_quantity"`
		Reason           string   `json:"reason"`
	}
	if json.NewDecoder(r.Body).Decode(&in) != nil || math.IsNaN(in.Quantity) || math.IsInf(in.Quantity, 0) || in.Quantity < 0 || in.Quantity > 9999999 || math.Abs(in.Quantity*1000-math.Round(in.Quantity*1000)) > 0.00001 || len(strings.TrimSpace(in.Reason)) < 3 || len(in.Reason) > 2000 || in.ExpectedQuantity == nil {
		respondJSON(w, 400, map[string]string{"error": "Menge, vorheriger Bestand und eine Begründung sind erforderlich"})
		return
	}
	tx, err := repository.GetSQLDB().BeginTx(r.Context(), nil)
	if err != nil {
		respondJSON(w, 500, map[string]string{"error": "Bestand konnte nicht gespeichert werden"})
		return
	}
	defer tx.Rollback()
	var tracking, lifecycle string
	if err = tx.QueryRow(`SELECT tracking_mode,lifecycle_status FROM products WHERE productid=$1 FOR UPDATE`, id).Scan(&tracking, &lifecycle); err == sql.ErrNoRows {
		respondJSON(w, 404, map[string]string{"error": "Produkt nicht gefunden"})
		return
	}
	if err != nil {
		respondJSON(w, 500, map[string]string{"error": "Bestand konnte nicht gespeichert werden"})
		return
	}
	if tracking != "quantity" || lifecycle != "active" {
		respondJSON(w, 409, map[string]string{"error": "Nur aktive Mengenprodukte können korrigiert werden"})
		return
	}
	var decimal bool
	if err = tx.QueryRow(`SELECT COALESCE(ct.is_decimal,false) FROM products p LEFT JOIN count_types ct ON ct.count_type_id=p.count_type_id WHERE p.productid=$1`, id).Scan(&decimal); err != nil {
		respondJSON(w, 500, map[string]string{"error": "Einheit konnte nicht geladen werden"})
		return
	}
	if !decimal && in.Quantity != math.Trunc(in.Quantity) {
		respondJSON(w, 400, map[string]string{"error": "Diese Einheit erlaubt nur ganze Mengen"})
		return
	}

	if in.ZoneID != nil {
		var active bool
		if err = tx.QueryRow(`SELECT is_active FROM storage_zones WHERE zone_id=$1 FOR SHARE`, *in.ZoneID).Scan(&active); err != nil || !active {
			respondJSON(w, 409, map[string]string{"error": "Lagerzone ist nicht aktiv"})
			return
		}
	}
	var previous float64
	if err = tx.QueryRow(`SELECT COALESCE(SUM(quantity),0) FROM product_locations WHERE product_id=$1 AND zone_id IS NOT DISTINCT FROM $2`, id, in.ZoneID).Scan(&previous); err != nil {
		respondJSON(w, 500, map[string]string{"error": "Bestand konnte nicht geprüft werden"})
		return
	}
	if math.Abs(previous-*in.ExpectedQuantity) > 0.00001 {
		respondJSON(w, 409, map[string]string{"error": "Bestand wurde inzwischen verändert. Bitte neu laden."})
		return
	}
	_, err = tx.Exec(`INSERT INTO product_locations(product_id,zone_id,quantity) VALUES($1,$2,$3) ON CONFLICT(product_id,zone_id) DO UPDATE SET quantity=EXCLUDED.quantity,updated_at=now()`, id, in.ZoneID, in.Quantity)
	if err == nil {
		err = recordProductAudit(tx, r, "product.stock.correct", id, map[string]any{"zone_id": in.ZoneID, "quantity": previous}, map[string]any{"zone_id": in.ZoneID, "quantity": in.Quantity, "reason": strings.TrimSpace(in.Reason)})
	}
	if err != nil {
		respondJSON(w, 409, map[string]string{"error": "Bestand konnte nicht korrigiert werden"})
		return
	}
	if tx.Commit() != nil {
		respondJSON(w, 500, map[string]string{"error": "Bestand konnte nicht gespeichert werden"})
		return
	}
	respondJSON(w, 200, map[string]any{"quantity": in.Quantity})
}
func GetProductHistory(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(mux.Vars(r)["id"])
	if err != nil {
		respondJSON(w, 400, map[string]string{"error": "Invalid product ID"})
		return
	}
	rows, err := repository.GetSQLDB().QueryContext(r.Context(), `SELECT id,action,timestamp,user_id FROM audit_log WHERE entity_type='product' AND entity_id=$1 ORDER BY timestamp DESC,id DESC LIMIT 100`, strconv.Itoa(id))
	if err != nil {
		respondJSON(w, 500, map[string]string{"error": "Verlauf konnte nicht geladen werden"})
		return
	}
	defer rows.Close()
	type event struct {
		ID        int       `json:"id"`
		Action    string    `json:"action"`
		Timestamp time.Time `json:"timestamp"`
		UserID    *int      `json:"user_id"`
	}
	result := []event{}
	for rows.Next() {
		var item event
		if rows.Scan(&item.ID, &item.Action, &item.Timestamp, &item.UserID) != nil {
			respondJSON(w, 500, map[string]string{"error": "Verlauf konnte nicht geladen werden"})
			return
		}
		result = append(result, item)
	}
	if rows.Err() != nil {
		respondJSON(w, 500, map[string]string{"error": "Verlauf konnte nicht geladen werden"})
		return
	}
	respondJSON(w, 200, result)
}
