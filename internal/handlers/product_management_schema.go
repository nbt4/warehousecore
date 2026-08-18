package handlers

import (
	"fmt"

	"warehousecore/internal/repository"
)

// EnsureProductManagementSchema consolidates product lifecycle and inventory
// rules for installations that predate the typed product model.
func EnsureProductManagementSchema() error {
	db := repository.GetSQLDB()
	if db == nil {
		return fmt.Errorf("database is not initialized")
	}

	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin product management schema transaction: %w", err)
	}
	defer tx.Rollback()

	statements := []string{
		`ALTER TABLE products ADD COLUMN IF NOT EXISTS product_type VARCHAR(20)`,
		`ALTER TABLE products ADD COLUMN IF NOT EXISTS tracking_mode VARCHAR(20)`,
		`ALTER TABLE products ADD COLUMN IF NOT EXISTS lifecycle_status VARCHAR(20)`,
		`UPDATE products
		 SET product_type = CASE
		     WHEN COALESCE(is_consumable, FALSE) THEN 'consumable'
		     WHEN COALESCE(is_accessory, FALSE) THEN 'accessory'
		     ELSE 'equipment'
		 END
		 WHERE product_type IS NULL OR product_type = ''`,
		`UPDATE products
		 SET tracking_mode = CASE
		     WHEN COALESCE(is_accessory, FALSE) OR COALESCE(is_consumable, FALSE) THEN 'quantity'
		     ELSE 'individual'
		 END
		 WHERE tracking_mode IS NULL OR tracking_mode = ''`,
		`UPDATE products p
		 SET tracking_mode = cp.tracking_mode
		 FROM cable_products cp
		 WHERE cp.product_id = p.productid`,
		`UPDATE products SET lifecycle_status = 'active'
		 WHERE lifecycle_status IS NULL OR lifecycle_status = ''`,
		`ALTER TABLE products ALTER COLUMN product_type SET DEFAULT 'equipment'`,
		`ALTER TABLE products ALTER COLUMN product_type SET NOT NULL`,
		`ALTER TABLE products ALTER COLUMN tracking_mode SET DEFAULT 'individual'`,
		`ALTER TABLE products ALTER COLUMN tracking_mode SET NOT NULL`,
		`ALTER TABLE products ALTER COLUMN lifecycle_status SET DEFAULT 'active'`,
		`ALTER TABLE products ALTER COLUMN lifecycle_status SET NOT NULL`,
		`DO $$ BEGIN
		 IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'chk_products_product_type') THEN
		   ALTER TABLE products ADD CONSTRAINT chk_products_product_type
		     CHECK (product_type IN ('equipment', 'accessory', 'consumable'));
		 END IF;
		END $$`,
		`DO $$ BEGIN
		 IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'chk_products_tracking_mode') THEN
		   ALTER TABLE products ADD CONSTRAINT chk_products_tracking_mode
		     CHECK (tracking_mode IN ('individual', 'quantity', 'none'));
		 END IF;
		END $$`,
		`DO $$ BEGIN
		 IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'chk_products_lifecycle_status') THEN
		   ALTER TABLE products ADD CONSTRAINT chk_products_lifecycle_status
		     CHECK (lifecycle_status IN ('active', 'archived'));
		 END IF;
		END $$`,
		`DO $$ BEGIN
		 IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'chk_product_locations_nonnegative') THEN
		   ALTER TABLE product_locations ADD CONSTRAINT chk_product_locations_nonnegative
		     CHECK (quantity >= 0) NOT VALID;
		 END IF;
		END $$`,
		`ALTER TABLE product_locations VALIDATE CONSTRAINT chk_product_locations_nonnegative`,
		`CREATE INDEX IF NOT EXISTS idx_products_lifecycle_status ON products(lifecycle_status)`,
		`CREATE INDEX IF NOT EXISTS idx_products_product_type ON products(product_type)`,
		`CREATE INDEX IF NOT EXISTS idx_products_tracking_mode ON products(tracking_mode)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS uq_products_name_normalized
		 ON products (LOWER(TRIM(name)))`,
		`CREATE UNIQUE INDEX IF NOT EXISTS uq_products_barcode_normalized
		 ON products (LOWER(TRIM(generic_barcode)))
		 WHERE NULLIF(TRIM(generic_barcode), '') IS NOT NULL`,
		`INSERT INTO product_locations (product_id, zone_id, quantity)
		 SELECT p.productid, NULL, p.stock_quantity
		 FROM products p
		 WHERE p.tracking_mode = 'quantity'
		   AND COALESCE(p.stock_quantity, 0) > 0
		   AND NOT EXISTS (SELECT 1 FROM product_locations pl WHERE pl.product_id = p.productid)
		 ON CONFLICT (product_id, zone_id) DO NOTHING`,
		`UPDATE products p
		 SET stock_quantity = COALESCE((
		   SELECT SUM(pl.quantity) FROM product_locations pl WHERE pl.product_id = p.productid
		 ), 0), updated_at = CURRENT_TIMESTAMP
		 WHERE p.tracking_mode = 'quantity'`,
		`CREATE OR REPLACE FUNCTION sync_product_stock_from_locations()
		 RETURNS TRIGGER AS $$
		 DECLARE affected_product_id INT;
		 BEGIN
		   IF TG_OP = 'DELETE' THEN
		     affected_product_id := OLD.product_id;
		   ELSE
		     affected_product_id := NEW.product_id;
		   END IF;

		   UPDATE products
		   SET stock_quantity = COALESCE((
		     SELECT SUM(quantity) FROM product_locations WHERE product_id = affected_product_id
		   ), 0), updated_at = CURRENT_TIMESTAMP
		   WHERE productid = affected_product_id AND tracking_mode = 'quantity';

		   IF TG_OP = 'UPDATE' AND OLD.product_id IS DISTINCT FROM NEW.product_id THEN
		     UPDATE products
		     SET stock_quantity = COALESCE((
		       SELECT SUM(quantity) FROM product_locations WHERE product_id = OLD.product_id
		     ), 0), updated_at = CURRENT_TIMESTAMP
		     WHERE productid = OLD.product_id AND tracking_mode = 'quantity';
		   END IF;

		   IF TG_OP = 'DELETE' THEN
		     RETURN OLD;
		   END IF;
		   RETURN NEW;
		 END;
		 $$ LANGUAGE plpgsql`,
		`DROP TRIGGER IF EXISTS product_locations_sync_stock ON product_locations`,
		`CREATE TRIGGER product_locations_sync_stock
		 AFTER INSERT OR UPDATE OR DELETE ON product_locations
		 FOR EACH ROW EXECUTE FUNCTION sync_product_stock_from_locations()`,
		`INSERT INTO warehouse_schema_migrations (version)
		 VALUES ('037_product_management_safety')
		 ON CONFLICT (version) DO NOTHING`,
	}

	for _, statement := range statements {
		if _, err := tx.Exec(statement); err != nil {
			return fmt.Errorf("apply product management schema: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit product management schema: %w", err)
	}
	return nil
}
