package handlers

import (
	"database/sql"
	"fmt"
	"strings"

	"warehousecore/internal/repository"
)

const cableInventorySchemaVersion = "033_hybrid_cable_inventory"

type legacyCableGroup struct {
	connectorAID  int
	connectorBID  int
	cableTypeID   int
	length        float64
	crossSection  sql.NullFloat64
	name          sql.NullString
	connectorA    string
	connectorB    string
	cableTypeName string
	quantity      int
}

// EnsureCableInventorySchema installs the product-backed cable inventory schema
// and migrates legacy cable rows exactly once. It is intentionally idempotent so
// existing installations can upgrade during a normal WarehouseCore restart.
func EnsureCableInventorySchema() error {
	db := repository.GetSQLDB()
	if db == nil {
		return fmt.Errorf("database is not initialized")
	}

	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin cable inventory migration: %w", err)
	}
	defer tx.Rollback()

	statements := []string{
		`CREATE TABLE IF NOT EXISTS warehouse_schema_migrations (
			version VARCHAR(100) PRIMARY KEY,
			applied_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS product_locations (
			location_id SERIAL PRIMARY KEY,
			product_id INT NOT NULL REFERENCES products(productid) ON DELETE CASCADE,
			zone_id INT NULL REFERENCES storage_zones(zone_id) ON DELETE CASCADE,
			quantity NUMERIC(10,3) NOT NULL DEFAULT 0,
			updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)`,
		`ALTER TABLE product_locations ALTER COLUMN zone_id DROP NOT NULL`,
		`CREATE INDEX IF NOT EXISTS idx_product_locations_zone ON product_locations(zone_id)`,
		`CREATE TABLE IF NOT EXISTS cable_products (
			cable_product_id BIGSERIAL PRIMARY KEY,
			product_id INT NOT NULL UNIQUE REFERENCES products(productid) ON DELETE CASCADE,
			connector_a_id INT NOT NULL REFERENCES cable_connectors(cable_connectorsid) ON DELETE RESTRICT,
			connector_b_id INT NOT NULL REFERENCES cable_connectors(cable_connectorsid) ON DELETE RESTRICT,
			cable_type_id INT NOT NULL REFERENCES cable_types(cable_typesid) ON DELETE RESTRICT,
			length_m NUMERIC(10,2) NOT NULL CHECK (length_m > 0),
			cross_section_mm2 NUMERIC(10,2),
			tracking_mode VARCHAR(20) NOT NULL DEFAULT 'quantity' CHECK (tracking_mode IN ('quantity', 'individual')),
			migrated_from_legacy BOOLEAN NOT NULL DEFAULT FALSE,
			created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE INDEX IF NOT EXISTS idx_cable_products_connector_a ON cable_products(connector_a_id)`,
		`CREATE INDEX IF NOT EXISTS idx_cable_products_connector_b ON cable_products(connector_b_id)`,
		`CREATE INDEX IF NOT EXISTS idx_cable_products_type ON cable_products(cable_type_id)`,
	}
	for _, statement := range statements {
		if _, err := tx.Exec(statement); err != nil {
			return fmt.Errorf("prepare cable inventory schema: %w", err)
		}
	}

	// PostgreSQL UNIQUE constraints treat NULL as distinct. Consolidate any old
	// unassigned rows, then add a NULL-aware unique index for safe stock upserts.
	if _, err := tx.Exec(`
		WITH totals AS (
			SELECT product_id, SUM(quantity) AS quantity, MIN(location_id) AS keep_id
			FROM product_locations
			WHERE zone_id IS NULL
			GROUP BY product_id
		), updated AS (
			UPDATE product_locations pl
			SET quantity = totals.quantity, updated_at = CURRENT_TIMESTAMP
			FROM totals
			WHERE pl.location_id = totals.keep_id
		)
		DELETE FROM product_locations pl
		USING totals
		WHERE pl.product_id = totals.product_id
		  AND pl.zone_id IS NULL
		  AND pl.location_id <> totals.keep_id
	`); err != nil {
		return fmt.Errorf("consolidate unassigned cable stock: %w", err)
	}
	if _, err := tx.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS uq_product_locations_product_zone ON product_locations(product_id, zone_id) NULLS NOT DISTINCT`); err != nil {
		return fmt.Errorf("create product location uniqueness index: %w", err)
	}

	var applied bool
	if err := tx.QueryRow(`SELECT EXISTS (SELECT 1 FROM warehouse_schema_migrations WHERE version = $1)`, cableInventorySchemaVersion).Scan(&applied); err != nil {
		return fmt.Errorf("check cable inventory migration: %w", err)
	}
	if !applied {
		if err := migrateLegacyCables(tx); err != nil {
			return err
		}
		if _, err := tx.Exec(`INSERT INTO warehouse_schema_migrations (version) VALUES ($1)`, cableInventorySchemaVersion); err != nil {
			return fmt.Errorf("record cable inventory migration: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit cable inventory migration: %w", err)
	}
	return nil
}

func migrateLegacyCables(tx *sql.Tx) error {
	var legacyTableExists bool
	if err := tx.QueryRow(`SELECT to_regclass('public.cables') IS NOT NULL`).Scan(&legacyTableExists); err != nil {
		return fmt.Errorf("check legacy cable table: %w", err)
	}
	if !legacyTableExists {
		return nil
	}

	rows, err := tx.Query(`
		SELECT c.connector1, c.connector2, c.typ, c.length, c.mm2,
		       MAX(NULLIF(BTRIM(c.name), '')) AS name,
		       COALESCE(NULLIF(cc1.abbreviation, ''), cc1.name),
		       COALESCE(NULLIF(cc2.abbreviation, ''), cc2.name),
		       ct.name,
		       COUNT(*)
		FROM cables c
		JOIN cable_connectors cc1 ON cc1.cable_connectorsid = c.connector1
		JOIN cable_connectors cc2 ON cc2.cable_connectorsid = c.connector2
		JOIN cable_types ct ON ct.cable_typesid = c.typ
		GROUP BY c.connector1, c.connector2, c.typ, c.length, c.mm2, cc1.abbreviation, cc1.name, cc2.abbreviation, cc2.name, ct.name
		ORDER BY c.typ, c.connector1, c.connector2, c.length, c.mm2
	`)
	if err != nil {
		return fmt.Errorf("read legacy cables: %w", err)
	}

	groups := make([]legacyCableGroup, 0)
	for rows.Next() {
		var group legacyCableGroup
		if err := rows.Scan(
			&group.connectorAID,
			&group.connectorBID,
			&group.cableTypeID,
			&group.length,
			&group.crossSection,
			&group.name,
			&group.connectorA,
			&group.connectorB,
			&group.cableTypeName,
			&group.quantity,
		); err != nil {
			rows.Close()
			return fmt.Errorf("scan legacy cable group: %w", err)
		}
		groups = append(groups, group)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return fmt.Errorf("iterate legacy cable groups: %w", err)
	}
	rows.Close()

	if len(groups) == 0 {
		return nil
	}

	var countTypeID int
	if err := tx.QueryRow(`
		INSERT INTO count_types (name, abbreviation, is_decimal)
		VALUES ('Stück', 'Stk', FALSE)
		ON CONFLICT (name) DO UPDATE SET abbreviation = COALESCE(count_types.abbreviation, EXCLUDED.abbreviation)
		RETURNING count_type_id
	`).Scan(&countTypeID); err != nil {
		return fmt.Errorf("ensure cable count type: %w", err)
	}

	for _, group := range groups {
		name := strings.TrimSpace(group.name.String)
		if name == "" {
			name = buildCableProductName(group.cableTypeName, group.connectorA, group.connectorB, group.length)
		}

		var productID int
		if err := tx.QueryRow(`
			INSERT INTO products (name, is_accessory, is_consumable, count_type_id, stock_quantity, created_at, updated_at)
			VALUES ($1, TRUE, FALSE, $2, $3, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
			RETURNING productid
		`, name, countTypeID, group.quantity).Scan(&productID); err != nil {
			return fmt.Errorf("create cable product %q: %w", name, err)
		}

		barcode := cableProductBarcode(productID)
		if _, err := tx.Exec(`UPDATE products SET generic_barcode = $1 WHERE productid = $2`, barcode, productID); err != nil {
			return fmt.Errorf("assign cable product barcode: %w", err)
		}
		if _, err := tx.Exec(`
			INSERT INTO cable_products (
				product_id, connector_a_id, connector_b_id, cable_type_id,
				length_m, cross_section_mm2, tracking_mode, migrated_from_legacy
			) VALUES ($1, $2, $3, $4, $5, $6, 'quantity', TRUE)
		`, productID, group.connectorAID, group.connectorBID, group.cableTypeID, group.length, nullableLegacyFloat(group.crossSection)); err != nil {
			return fmt.Errorf("create cable specification for product %d: %w", productID, err)
		}
		if _, err := tx.Exec(`
			INSERT INTO product_locations (product_id, zone_id, quantity)
			VALUES ($1, NULL, $2)
			ON CONFLICT (product_id, zone_id) DO UPDATE
			SET quantity = EXCLUDED.quantity, updated_at = CURRENT_TIMESTAMP
		`, productID, group.quantity); err != nil {
			return fmt.Errorf("migrate cable stock for product %d: %w", productID, err)
		}
	}

	return nil
}

func nullableLegacyFloat(value sql.NullFloat64) interface{} {
	if !value.Valid {
		return nil
	}
	return value.Float64
}

func buildCableProductName(cableType, connectorA, connectorB string, length float64) string {
	return fmt.Sprintf("%s (%s – %s) · %g m", cableType, connectorA, connectorB, length)
}

func cableProductBarcode(productID int) string {
	return fmt.Sprintf("CAB-P-%06d", productID)
}
