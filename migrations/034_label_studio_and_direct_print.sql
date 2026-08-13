-- Label Studio: target-specific templates, generated assets and direct printing.

ALTER TABLE label_templates
    ADD COLUMN IF NOT EXISTS target_type VARCHAR(20) NOT NULL DEFAULT 'device',
    ADD COLUMN IF NOT EXISTS revision INTEGER NOT NULL DEFAULT 1;

CREATE INDEX IF NOT EXISTS idx_label_templates_target ON label_templates(target_type);
CREATE UNIQUE INDEX IF NOT EXISTS uq_label_templates_default_target
    ON label_templates(target_type) WHERE is_default;

CREATE TABLE IF NOT EXISTS label_assets (
    target_type VARCHAR(20) NOT NULL,
    target_id VARCHAR(255) NOT NULL,
    template_id INTEGER NULL REFERENCES label_templates(id) ON DELETE SET NULL,
    template_revision INTEGER NOT NULL DEFAULT 1,
    source_updated_at TIMESTAMP NULL,
    label_path VARCHAR(512) NOT NULL,
    generated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (target_type, target_id)
);

CREATE TABLE IF NOT EXISTS label_printers (
    id SERIAL PRIMARY KEY,
    name VARCHAR(255) NOT NULL UNIQUE,
    driver VARCHAR(30) NOT NULL DEFAULT 'zpl_tcp',
    address VARCHAR(255) NOT NULL,
    port INTEGER NOT NULL DEFAULT 9100,
    dpi INTEGER NOT NULL DEFAULT 203,
    is_default BOOLEAN NOT NULL DEFAULT FALSE,
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT chk_label_printer_driver CHECK (driver IN ('zpl_tcp')),
    CONSTRAINT chk_label_printer_port CHECK (port BETWEEN 1 AND 65535),
    CONSTRAINT chk_label_printer_dpi CHECK (dpi IN (203, 300, 600))
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_label_printers_default
    ON label_printers(is_default) WHERE is_default;

CREATE TABLE IF NOT EXISTS label_print_jobs (
    id BIGSERIAL PRIMARY KEY,
    target_type VARCHAR(20) NOT NULL,
    target_id VARCHAR(255) NOT NULL,
    template_id INTEGER NULL REFERENCES label_templates(id) ON DELETE SET NULL,
    printer_id INTEGER NULL REFERENCES label_printers(id) ON DELETE SET NULL,
    copies INTEGER NOT NULL DEFAULT 1,
    status VARCHAR(20) NOT NULL DEFAULT 'queued',
    label_path VARCHAR(512),
    error_message TEXT,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    started_at TIMESTAMP NULL,
    completed_at TIMESTAMP NULL,
    CONSTRAINT chk_label_print_job_status CHECK (status IN ('queued', 'printing', 'completed', 'failed')),
    CONSTRAINT chk_label_print_job_copies CHECK (copies BETWEEN 1 AND 1000)
);

CREATE INDEX IF NOT EXISTS idx_label_print_jobs_created ON label_print_jobs(created_at DESC);
CREATE INDEX IF NOT EXISTS idx_label_print_jobs_status ON label_print_jobs(status);

INSERT INTO label_assets (target_type, target_id, label_path, source_updated_at)
SELECT 'device', deviceid, label_path, updated_at
FROM devices
WHERE label_path IS NOT NULL AND label_path <> ''
ON CONFLICT (target_type, target_id) DO NOTHING;

INSERT INTO label_assets (target_type, target_id, label_path, source_updated_at)
SELECT 'case', caseid::text, label_path, updated_at
FROM cases
WHERE label_path IS NOT NULL AND label_path <> ''
ON CONFLICT (target_type, target_id) DO NOTHING;

INSERT INTO label_templates (name, description, width, height, template_json, is_default, target_type, revision)
SELECT 'Standard Geräte-Label', 'Geräte-Label 51x25mm', 51, 25, '[{"type":"qrcode","x":2,"y":2,"width":21,"height":21,"rotation":0,"content":"code","style":{"format":"qr"}},{"type":"text","x":25,"y":3,"width":24,"height":7,"rotation":0,"content":"product_name","style":{"font_size":9,"font_weight":"bold","font_family":"Arial","color":"#000000","alignment":"left"}},{"type":"text","x":25,"y":13,"width":24,"height":5,"rotation":0,"content":"device_id","style":{"font_size":8,"font_weight":"normal","font_family":"Arial","color":"#000000","alignment":"left"}}]', TRUE, 'device', 1
WHERE NOT EXISTS (SELECT 1 FROM label_templates WHERE target_type = 'device' AND is_default)
ON CONFLICT (name) DO UPDATE SET target_type = 'device', is_default = TRUE;

INSERT INTO label_templates (name, description, width, height, template_json, is_default, target_type, revision)
SELECT 'Standard Produkt-Label', 'Artikel- und Mengenlabel 51x25mm', 51, 25, '[{"type":"barcode","x":2,"y":2,"width":47,"height":11,"rotation":0,"content":"code","style":{"format":"code128"}},{"type":"text","x":2,"y":14,"width":47,"height":5,"rotation":0,"content":"name","style":{"font_size":9,"font_weight":"bold","font_family":"Arial","color":"#000000","alignment":"left"}},{"type":"text","x":2,"y":20,"width":47,"height":4,"rotation":0,"content":"code","style":{"font_size":7,"font_weight":"normal","font_family":"Arial","color":"#000000","alignment":"left"}}]', TRUE, 'product', 1
WHERE NOT EXISTS (SELECT 1 FROM label_templates WHERE target_type = 'product' AND is_default)
ON CONFLICT (name) DO UPDATE SET target_type = 'product', is_default = TRUE;

INSERT INTO label_templates (name, description, width, height, template_json, is_default, target_type, revision)
SELECT 'Standard Case-Label', 'Case-Label 51x25mm', 51, 25, '[{"type":"qrcode","x":2,"y":2,"width":21,"height":21,"rotation":0,"content":"code","style":{"format":"qr"}},{"type":"text","x":25,"y":4,"width":24,"height":7,"rotation":0,"content":"name","style":{"font_size":10,"font_weight":"bold","font_family":"Arial","color":"#000000","alignment":"left"}},{"type":"text","x":25,"y":14,"width":24,"height":5,"rotation":0,"content":"code","style":{"font_size":8,"font_weight":"normal","font_family":"Arial","color":"#000000","alignment":"left"}}]', TRUE, 'case', 1
WHERE NOT EXISTS (SELECT 1 FROM label_templates WHERE target_type = 'case' AND is_default)
ON CONFLICT (name) DO UPDATE SET target_type = 'case', is_default = TRUE;

INSERT INTO label_templates (name, description, width, height, template_json, is_default, target_type, revision)
SELECT 'Standard Zonen-Label', 'Lagerzonen-Label 62x29mm', 62, 29, '[{"type":"qrcode","x":2,"y":2,"width":25,"height":25,"rotation":0,"content":"code","style":{"format":"qr"}},{"type":"text","x":30,"y":4,"width":30,"height":8,"rotation":0,"content":"zone_code","style":{"font_size":14,"font_weight":"bold","font_family":"Arial","color":"#000000","alignment":"left"}},{"type":"text","x":30,"y":15,"width":30,"height":6,"rotation":0,"content":"name","style":{"font_size":9,"font_weight":"normal","font_family":"Arial","color":"#000000","alignment":"left"}}]', TRUE, 'zone', 1
WHERE NOT EXISTS (SELECT 1 FROM label_templates WHERE target_type = 'zone' AND is_default)
ON CONFLICT (name) DO UPDATE SET target_type = 'zone', is_default = TRUE;

INSERT INTO warehouse_schema_migrations (version)
VALUES ('034_label_studio_and_direct_print')
ON CONFLICT (version) DO NOTHING;
