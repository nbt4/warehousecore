-- A featured product stays in the regular public Mietpark list.
ALTER TABLE products ADD COLUMN IF NOT EXISTS website_featured BOOLEAN NOT NULL DEFAULT FALSE;
CREATE INDEX IF NOT EXISTS idx_products_website_featured
  ON products (website_featured) WHERE website_visible = TRUE AND lifecycle_status = 'active';
