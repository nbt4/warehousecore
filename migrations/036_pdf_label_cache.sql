-- Persist generated labels as PDFs. Existing PNG paths are invalidated so the
-- next explicit generation, export or print recreates the canonical PDF.

UPDATE devices
SET label_path = NULL
WHERE LOWER(COALESCE(label_path, '')) LIKE '%.png';

UPDATE cases
SET label_path = NULL
WHERE LOWER(COALESCE(label_path, '')) LIKE '%.png';

UPDATE storage_zones
SET label_url = NULL
WHERE LOWER(COALESCE(label_url, '')) LIKE '%.png';

DELETE FROM label_assets
WHERE LOWER(label_path) LIKE '%.png';

INSERT INTO warehouse_schema_migrations (version)
VALUES ('036_pdf_label_cache')
ON CONFLICT (version) DO NOTHING;
