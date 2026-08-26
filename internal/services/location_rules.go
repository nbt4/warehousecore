package services

import (
	"database/sql"
	"fmt"
)

type rowQueryer interface {
	QueryRow(query string, args ...interface{}) *sql.Row
}

// LocationCapacity is the directly stored load of a location. Parent location
// occupancy is intentionally calculated by the overview API, not for writes.
type LocationCapacity struct {
	Devices  float64 `json:"devices"`
	Cases    float64 `json:"cases"`
	Products float64 `json:"products"`
	Total    float64 `json:"total"`
}

// ValidateStorageDestination protects scanner and assignment workflows from
// writing inventory into archived, blocked, non-storable or full places.
func ValidateStorageDestination(q rowQueryer, zoneID int64, incoming float64) error {
	if zoneID <= 0 {
		return fmt.Errorf("ungültiger Lagerplatz")
	}
	if incoming <= 0 {
		incoming = 1
	}

	var active, storable bool
	var status, code string
	var capacity sql.NullFloat64
	err := q.QueryRow(`
		SELECT is_active, is_storable, operational_status, code, capacity
		FROM storage_zones WHERE zone_id = $1
	`, zoneID).Scan(&active, &storable, &status, &code, &capacity)
	if err == sql.ErrNoRows {
		return fmt.Errorf("Lagerplatz wurde nicht gefunden")
	}
	if err != nil {
		return fmt.Errorf("Lagerplatz konnte nicht geprüft werden: %w", err)
	}
	if !active || status == "archived" {
		return fmt.Errorf("Lagerplatz %s ist archiviert", code)
	}
	if status != "available" {
		return fmt.Errorf("Lagerplatz %s ist aktuell %s", code, status)
	}
	if !storable {
		return fmt.Errorf("%s ist ein Strukturbereich und kein belegbarer Lagerplatz", code)
	}

	var invalidAncestors int
	err = q.QueryRow(`
		WITH RECURSIVE ancestors AS (
			SELECT zone_id, parent_zone_id, is_active, operational_status
			FROM storage_zones WHERE zone_id = $1
			UNION ALL
			SELECT parent.zone_id, parent.parent_zone_id, parent.is_active, parent.operational_status
			FROM storage_zones parent JOIN ancestors child ON parent.zone_id = child.parent_zone_id
		)
		SELECT COUNT(*) FROM ancestors
		WHERE NOT is_active OR operational_status = 'archived'
	`, zoneID).Scan(&invalidAncestors)
	if err != nil {
		return fmt.Errorf("Lagerhierarchie konnte nicht geprüft werden: %w", err)
	}
	if invalidAncestors > 0 {
		return fmt.Errorf("Lagerplatz %s liegt unter einem archivierten Bereich", code)
	}

	if capacity.Valid {
		var used float64
		err = q.QueryRow(`
			SELECT
				(SELECT COUNT(*) FROM devices WHERE zone_id = $1 AND status = 'in_storage')::numeric +
				(SELECT COUNT(*) FROM cases WHERE zone_id = $1)::numeric +
				COALESCE((SELECT SUM(quantity) FROM product_locations WHERE zone_id = $1), 0)
		`, zoneID).Scan(&used)
		if err != nil {
			return fmt.Errorf("Kapazität konnte nicht geprüft werden: %w", err)
		}
		if used+incoming > capacity.Float64 {
			return fmt.Errorf("Lagerplatz %s ist voll (%.2f von %.2f belegt)", code, used, capacity.Float64)
		}
	}
	return nil
}
