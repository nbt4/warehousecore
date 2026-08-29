package services

import (
	"database/sql"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"warehousecore/internal/repository"
)

// ZoneService handles zone-related business logic
type ZoneService struct {
	db *sql.DB
}

// NewZoneService creates a new zone service
func NewZoneService() *ZoneService {
	return &ZoneService{
		db: repository.GetSQLDB(),
	}
}

// GenerateZoneCode generates a smart, hierarchical zone code
// Examples:
// - Warehouse: WDB (from "Weidelbach")
// - Rack in WDB: WDB-RG-01
// - Shelf in Rack: WDB-RG-01-F-01
// - Vehicle: WDB-VH-01
func (s *ZoneService) GenerateZoneCode(zoneName, zoneType string, parentZoneID *int64) (string, error) {
	// If there's a parent, get its code and build hierarchically
	if parentZoneID != nil && *parentZoneID > 0 {
		var parentCode string
		err := s.db.QueryRow(`SELECT code FROM storage_zones WHERE zone_id = $1`, *parentZoneID).Scan(&parentCode)
		if err != nil {
			return "", fmt.Errorf("parent zone not found: %v", err)
		}

		// Get next number for this type under this parent
		typePrefix := getTypePrefix(zoneType)
		codePrefix := fmt.Sprintf("%s-%s-", parentCode, typePrefix)
		rows, err := s.db.Query(`SELECT code FROM storage_zones WHERE parent_zone_id = $1`, *parentZoneID)
		if err != nil {
			return "", err
		}
		defer rows.Close()

		codes := make([]string, 0)
		for rows.Next() {
			var code string
			if err := rows.Scan(&code); err != nil {
				return "", err
			}
			codes = append(codes, code)
		}
		if err := rows.Err(); err != nil {
			return "", err
		}

		return fmt.Sprintf("%s%02d", codePrefix, nextZoneSequence(codes, codePrefix)), nil
	}

	// Root level zone (warehouse)
	prefix := generatePrefix(zoneName)

	// Check if this prefix exists and add number if needed
	var count int
	err := s.db.QueryRow(`
		SELECT COUNT(*) FROM storage_zones WHERE code LIKE $1
	`, prefix+"%").Scan(&count)

	if err != nil {
		return "", err
	}

	if count == 0 {
		return prefix, nil
	}

	return fmt.Sprintf("%s-%02d", prefix, count+1), nil
}

func nextZoneSequence(codes []string, prefix string) int {
	maxSequence := 0
	for _, code := range codes {
		if !strings.HasPrefix(code, prefix) {
			continue
		}
		sequence, err := strconv.Atoi(strings.TrimPrefix(code, prefix))
		if err == nil && sequence > maxSequence {
			maxSequence = sequence
		}
	}
	return maxSequence + 1
}

// generatePrefix creates a 3-letter prefix from a zone name
func generatePrefix(name string) string {
	// Remove special characters and convert to uppercase
	reg := regexp.MustCompile("[^a-zA-Z]+")
	cleaned := reg.ReplaceAllString(name, "")
	cleaned = strings.ToUpper(cleaned)

	if len(cleaned) <= 3 {
		return cleaned
	}

	// Take first 3 consonants or first 3 letters
	consonants := regexp.MustCompile("[BCDFGHJKLMNPQRSTVWXYZ]")
	matches := consonants.FindAllString(cleaned, -1)

	if len(matches) >= 3 {
		return strings.Join(matches[:3], "")
	}

	return cleaned[:3]
}

// GenerateShelfName generates an automatic name for a shelf (Fach)
// Returns "Fach 01", "Fach 02", etc based on existing shelves in parent zone
func (s *ZoneService) GenerateShelfName(parentZoneID *int64) (string, error) {
	if parentZoneID == nil || *parentZoneID == 0 {
		return "", fmt.Errorf("parent zone ID required for shelf creation")
	}

	// Count existing shelves in this parent zone
	var count int
	err := s.db.QueryRow(`
		SELECT COUNT(*) FROM storage_zones
		WHERE parent_zone_id = $1 AND type = 'shelf' AND is_active = TRUE
	`, *parentZoneID).Scan(&count)

	if err != nil {
		return "", err
	}

	// Generate name: Fach 01, Fach 02, etc.
	return fmt.Sprintf("Fach %02d", count+1), nil
}

// getTypePrefix returns the type prefix for zone codes
func getTypePrefix(zoneType string) string {
	prefixes := map[string]string{
		"warehouse": "LGR", // Lager
		"rack":      "RG",  // Regal (Shelving Unit)
		"gitterbox": "GB",  // Gitterbox
		"shelf":     "F",   // Fach (Shelf/Compartment)
	}

	if prefix, ok := prefixes[zoneType]; ok {
		return prefix
	}
	return "OT"
}

// GetZoneDetails returns zone with subzones and device count
func (s *ZoneService) GetZoneDetails(zoneID int64) (map[string]interface{}, error) {
	// Get zone info
	var (
		code, name, zoneType string
		description          sql.NullString
		parentZoneID         sql.NullInt64
		capacity             sql.NullInt64
		isActive             bool
	)

	err := s.db.QueryRow(`
		SELECT zone_id, code, name, type, description, parent_zone_id, capacity, is_active
		FROM storage_zones
		WHERE zone_id = $1 AND is_active = TRUE
	`, zoneID).Scan(&zoneID, &code, &name, &zoneType, &description, &parentZoneID, &capacity, &isActive)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("zone not found")
	}
	if err != nil {
		return nil, err
	}

	// Get subzones
	subzones, err := s.getSubzones(zoneID)
	if err != nil {
		return nil, err
	}

	// Get devices in this zone AND all subzones (recursive)
	deviceCount := s.getDeviceCountRecursive(zoneID)

	// Get breadcrumb path
	breadcrumb, err := s.getBreadcrumb(zoneID)
	if err != nil {
		return nil, err
	}

	result := map[string]interface{}{
		"zone_id":      zoneID,
		"code":         code,
		"name":         name,
		"type":         zoneType,
		"is_active":    isActive,
		"device_count": deviceCount,
		"subzones":     subzones,
		"breadcrumb":   breadcrumb,
	}

	if description.Valid {
		result["description"] = description.String
	}
	if capacity.Valid {
		result["capacity"] = capacity.Int64
	}
	if parentZoneID.Valid {
		result["parent_zone_id"] = parentZoneID.Int64
	}

	return result, nil
}

// getSubzones returns all child zones
func (s *ZoneService) getSubzones(parentID int64) ([]map[string]interface{}, error) {
	rows, err := s.db.Query(`
		SELECT zone_id, code, name, type, capacity,
		       (SELECT COUNT(*) FROM storage_zones WHERE parent_zone_id = sz.zone_id AND is_active = TRUE) as subzone_count
		FROM storage_zones sz
		WHERE parent_zone_id = $1 AND is_active = TRUE
		ORDER BY code
	`, parentID)

	if err != nil {
		return nil, err
	}
	defer rows.Close()

	subzones := []map[string]interface{}{}
	for rows.Next() {
		var (
			zoneID, subzoneCount int64
			code, name, zoneType string
			capacity             sql.NullInt64
		)

		if err := rows.Scan(&zoneID, &code, &name, &zoneType, &capacity, &subzoneCount); err != nil {
			continue
		}

		// Get device count recursively for this subzone (includes all its subzones)
		deviceCount := s.getDeviceCountRecursive(zoneID)

		zone := map[string]interface{}{
			"zone_id":       zoneID,
			"code":          code,
			"name":          name,
			"type":          zoneType,
			"device_count":  deviceCount,
			"subzone_count": subzoneCount,
		}

		if capacity.Valid {
			zone["capacity"] = capacity.Int64
		}

		subzones = append(subzones, zone)
	}

	return subzones, nil
}

// getBreadcrumb returns the hierarchical path to a zone
func (s *ZoneService) getBreadcrumb(zoneID int64) ([]map[string]interface{}, error) {
	breadcrumb := []map[string]interface{}{}
	currentID := zoneID

	for {
		var (
			id, code, name string
			parentID       sql.NullInt64
		)

		err := s.db.QueryRow(`
			SELECT zone_id, code, name, parent_zone_id
			FROM storage_zones
			WHERE zone_id = $1
		`, currentID).Scan(&id, &code, &name, &parentID)

		if err != nil {
			break
		}

		breadcrumb = append([]map[string]interface{}{{
			"zone_id": id,
			"code":    code,
			"name":    name,
		}}, breadcrumb...)

		if !parentID.Valid {
			break
		}
		currentID = parentID.Int64
	}

	return breadcrumb, nil
}

// getDeviceCountRecursive returns device count including all subzones recursively
func (s *ZoneService) getDeviceCountRecursive(zoneID int64) int {
	var count int
	err := s.db.QueryRow(`
		WITH RECURSIVE zone_tree AS (
			-- Start with the given zone
			SELECT zone_id FROM storage_zones WHERE zone_id = $1 AND is_active = TRUE
			UNION ALL
			-- Recursively add all child zones
			SELECT sz.zone_id
			FROM storage_zones sz
			INNER JOIN zone_tree zt ON sz.parent_zone_id = zt.zone_id
			WHERE sz.is_active = TRUE
		)
		-- Count devices in the zone and all subzones
		SELECT COUNT(*)
		FROM devices d
		INNER JOIN zone_tree zt ON d.zone_id = zt.zone_id
		WHERE d.status = 'in_storage'
	`, zoneID).Scan(&count)

	if err != nil {
		return 0
	}
	return count
}
