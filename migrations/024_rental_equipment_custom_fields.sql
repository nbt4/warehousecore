CREATE TABLE IF NOT EXISTS rental_equipment_field_definitions (
    id SERIAL PRIMARY KEY,
    name VARCHAR(100) NOT NULL,
    field_type VARCHAR(20) NOT NULL CHECK (field_type IN ('text', 'number', 'dropdown')),
    unit VARCHAR(20),
    dropdown_options TEXT,
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS rental_equipment_field_values (
    id SERIAL PRIMARY KEY,
    equipment_id INT NOT NULL,
    field_definition_id INT NOT NULL,
    value VARCHAR(500) NOT NULL,
    sort_order INT NOT NULL DEFAULT 0,
    CONSTRAINT uq_equipment_field UNIQUE (equipment_id, field_definition_id),
    FOREIGN KEY (equipment_id) REFERENCES rental_equipment(id) ON DELETE CASCADE,
    FOREIGN KEY (field_definition_id) REFERENCES rental_equipment_field_definitions(id) ON DELETE RESTRICT
);
