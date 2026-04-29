-- Migration 032: Fix label_templates column names to match Go model
-- The table was created by 000_combined_init.sql with wrong column names:
--   width_mm       -> width
--   height_mm      -> height
--   template_content -> template_json
-- Migration 016 was skipped (MySQL syntax, table already existed).

ALTER TABLE label_templates
    RENAME COLUMN width_mm       TO width,
    RENAME COLUMN height_mm      TO height,
    RENAME COLUMN template_content TO template_json;

-- Ensure template_json is NOT NULL (was nullable as template_content)
ALTER TABLE label_templates
    ALTER COLUMN template_json SET NOT NULL,
    ALTER COLUMN template_json SET DEFAULT '[]';

-- Drop template_type which is not in the Go model
ALTER TABLE label_templates
    DROP COLUMN IF EXISTS template_type;

-- Wipe the old placeholder rows that have broken/empty template_json
DELETE FROM label_templates WHERE template_json IS NULL OR template_json = '';
