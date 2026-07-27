-- Reconcile WarehouseCore package fields with the shared RentalCore package schema.
-- Product packages remain independent records; component products live only in
-- product_package_items and are not mirrored into products.

ALTER TABLE product_packages
    ADD COLUMN IF NOT EXISTS package_code VARCHAR(32),
    ADD COLUMN IF NOT EXISTS website_images_json TEXT;

UPDATE product_packages
SET package_code = COALESCE(NULLIF(code, ''), 'PKG-' || LPAD(id::text, 6, '0'))
WHERE package_code IS NULL OR package_code = '';

UPDATE product_packages
SET code = package_code
WHERE code IS NULL OR code = '';

CREATE UNIQUE INDEX IF NOT EXISTS uq_product_packages_package_code
    ON product_packages(package_code);

CREATE INDEX IF NOT EXISTS idx_product_package_items_package
    ON product_package_items(package_id);

CREATE INDEX IF NOT EXISTS idx_product_package_items_product
    ON product_package_items(product_id);

-- Preserve website image selections from legacy package mirror products where
-- the same product is not also a real component of any package.
UPDATE product_packages pp
SET website_image_url = COALESCE(pp.website_image_url, p.website_thumbnail),
    website_images_json = COALESCE(pp.website_images_json, p.website_images_json)
FROM products p
WHERE LOWER(TRIM(pp.name)) = LOWER(TRIM(p.name))
  AND NOT EXISTS (
      SELECT 1 FROM product_package_items ppi WHERE ppi.product_id = p.productID
  )
  AND (pp.website_image_url IS NULL OR pp.website_images_json IS NULL);
