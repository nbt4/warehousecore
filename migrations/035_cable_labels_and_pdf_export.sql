-- Restrict product-style label templates to the cable workflow and expose the
-- cable naming consistently. The target_type remains "product" internally so
-- existing cable templates and generated assets continue to work.

UPDATE label_templates
SET name = 'Standard Kabel-Label',
    description = 'Kabel- und Mengenlabel 51x25mm'
WHERE target_type = 'product'
  AND name = 'Standard Produkt-Label'
  AND NOT EXISTS (
      SELECT 1 FROM label_templates WHERE name = 'Standard Kabel-Label'
  );

INSERT INTO warehouse_schema_migrations (version)
VALUES ('035_cable_labels_and_pdf_export')
ON CONFLICT (version) DO NOTHING;
