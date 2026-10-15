package handlers

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"os"
	"reflect"
	"sort"
	"warehousecore/internal/middleware"

	"github.com/golang-jwt/jwt/v5"
	"github.com/lib/pq"
)

// Classification uses the same signed cookie as native authentication. A bad
// extra claim or missing origin must never reopen an unguided writer.
func rejectDelegatedRentalCatalog(w http.ResponseWriter, r *http.Request) bool {
	delegated := isWarehouseMCPMutation(r)
	cookie, err := r.Cookie("cores_token")
	secret := os.Getenv("CORES_JWT_SECRET")
	if secret == "" {
		secret = os.Getenv("JWT_SECRET")
	}
	if err == nil && secret != "" {
		claims := jwt.MapClaims{}
		token, e := jwt.ParseWithClaims(cookie.Value, claims, func(*jwt.Token) (any, error) { return []byte(secret), nil }, jwt.WithValidMethods([]string{"HS256", "HS384", "HS512"}), jwt.WithoutClaimsValidation())
		_, hasScope := claims["mcp_scope"]
		delegated = delegated || (e == nil && token.Valid && hasScope)
	}
	if delegated {
		respondJSON(w, 403, map[string]string{"error": "Use the named RentalCore MCP catalog workflow with signed action, financial scope and final confirmation", "code": "guided_rental_catalog_required"})
	}
	return delegated
}

func rentalNativeCatalogError(w http.ResponseWriter, err error) {
	var pg *pq.Error
	if errors.As(err, &pg) && (pg.Code == "23514" || pg.Code == "23505" || pg.Code == "55P03" || pg.Code == "40P01" || pg.Code == "40001") {
		respondJSON(w, 409, map[string]string{"error": "Katalogkonflikt: offene Jobs, archivierte Artikel oder doppelte Identität. Bitte neu laden; Archivierung und Wiederherstellung erhalten alle Felder.", "code": "external_catalog_conflict"})
		return
	}
	respondJSON(w, 500, map[string]string{"error": "Fremdmietartikel konnte nicht gespeichert werden"})
}

func rentalNativeCatalogPricesValid(req RentalEquipmentCreateRequest) bool {
	for _, price := range []float64{req.RentalPrice, req.CustomerPrice} {
		if math.IsNaN(price) || math.IsInf(price, 0) || price < 0 || price > 99999999.99 || math.Abs(price*100-math.Round(price*100)) > 0.00001 {
			return false
		}
	}
	return true
}

func recordRentalCatalogAudit(tx *sql.Tx, r *http.Request, id int, action string, before json.RawMessage) error {
	var after json.RawMessage
	if err := tx.QueryRow(`SELECT to_jsonb(re) FROM rental_equipment re WHERE id=$1`, id).Scan(&after); err != nil {
		return err
	}
	var uid any
	if user, ok := middleware.GetUserFromContext(r); ok {
		uid = user.UserID
	}
	if len(before) == 0 {
		before = json.RawMessage(`{}`)
	}
	payload, err := json.Marshal(map[string]any{"origin": "UI", "before": before, "after": after})
	if err != nil {
		return err
	}
	_, err = tx.Exec(`INSERT INTO audit_log(user_id,entity_type,entity_id,action,old_values,new_values) VALUES($1,'rental_equipment',$2,$3,$4::jsonb,$5::jsonb)`, uid, fmt.Sprint(id), action, string(before), string(payload))
	return err
}

// The UI sends its complete editor during lifecycle changes. Accept identical
// submitted properties, preserve omitted ones, and never rewrite retained rows.
func rentalCatalogPropertiesUnchanged(tx *sql.Tx, id int, submitted []RentalFieldValueInput) (bool, error) {
	if submitted == nil {
		return true, nil
	}
	rows, err := tx.Query(`SELECT field_definition_id,value,sort_order FROM rental_equipment_field_values WHERE equipment_id=$1 ORDER BY field_definition_id,sort_order,value`, id)
	if err != nil {
		return false, err
	}
	defer rows.Close()
	existing := []RentalFieldValueInput{}
	for rows.Next() {
		var value RentalFieldValueInput
		if err = rows.Scan(&value.FieldDefinitionID, &value.Value, &value.SortOrder); err != nil {
			return false, err
		}
		existing = append(existing, value)
	}
	if err = rows.Err(); err != nil {
		return false, err
	}
	proposed := append([]RentalFieldValueInput{}, submitted...)
	sort.Slice(proposed, func(i, j int) bool {
		a, b := proposed[i], proposed[j]
		if a.FieldDefinitionID != b.FieldDefinitionID {
			return a.FieldDefinitionID < b.FieldDefinitionID
		}
		if a.SortOrder != b.SortOrder {
			return a.SortOrder < b.SortOrder
		}
		return a.Value < b.Value
	})
	return reflect.DeepEqual(existing, proposed), nil
}
