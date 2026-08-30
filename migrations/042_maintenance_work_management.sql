-- Canonical maintenance work management: recurring plans, work orders and audit events.
-- The application initializer contains the same idempotent schema and legacy migration.
BEGIN;

CREATE TABLE IF NOT EXISTS maintenance_plans (
    plan_id BIGSERIAL PRIMARY KEY,
    device_id VARCHAR(255) NOT NULL REFERENCES devices(deviceID) ON DELETE CASCADE,
    name VARCHAR(160) NOT NULL,
    maintenance_type VARCHAR(30) NOT NULL CHECK (maintenance_type IN ('preventive','inspection','calibration')),
    interval_days INT NOT NULL CHECK (interval_days BETWEEN 1 AND 3650),
    lead_time_days INT NOT NULL DEFAULT 14 CHECK (lead_time_days BETWEEN 0 AND 365),
    instructions TEXT,
    next_due_at DATE NOT NULL,
    last_completed_at TIMESTAMP,
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    created_by BIGINT REFERENCES users(userID) ON DELETE SET NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT uq_maintenance_plan_device_name UNIQUE(device_id,name)
);

CREATE TABLE IF NOT EXISTS maintenance_orders (
    order_id BIGSERIAL PRIMARY KEY,
    legacy_defect_id BIGINT UNIQUE,
    device_id VARCHAR(255) NOT NULL REFERENCES devices(deviceID) ON DELETE RESTRICT,
    plan_id BIGINT REFERENCES maintenance_plans(plan_id) ON DELETE SET NULL,
    order_type VARCHAR(30) NOT NULL CHECK (order_type IN ('defect','preventive','inspection','calibration')),
    priority VARCHAR(20) NOT NULL DEFAULT 'normal' CHECK (priority IN ('low','normal','high','critical')),
    status VARCHAR(30) NOT NULL DEFAULT 'open' CHECK (status IN ('open','planned','in_progress','waiting_parts','completed','cancelled')),
    title VARCHAR(200) NOT NULL,
    description TEXT,
    due_at DATE,
    scheduled_at TIMESTAMP,
    reported_by BIGINT REFERENCES users(userID) ON DELETE SET NULL,
    assigned_to BIGINT REFERENCES users(userID) ON DELETE SET NULL,
    started_at TIMESTAMP,
    completed_at TIMESTAMP,
    outcome VARCHAR(30) CHECK (outcome IS NULL OR outcome IN ('passed','passed_with_notes','failed','repaired')),
    resolution TEXT,
    cost NUMERIC(12,2) CHECK (cost IS NULL OR cost >= 0),
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS maintenance_order_events (
    event_id BIGSERIAL PRIMARY KEY,
    order_id BIGINT NOT NULL REFERENCES maintenance_orders(order_id) ON DELETE CASCADE,
    event_type VARCHAR(40) NOT NULL,
    from_status VARCHAR(30),
    to_status VARCHAR(30),
    notes TEXT,
    actor_id BIGINT REFERENCES users(userID) ON DELETE SET NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_maintenance_plans_due ON maintenance_plans(is_active,next_due_at);
CREATE INDEX IF NOT EXISTS idx_maintenance_orders_worklist ON maintenance_orders(status,due_at,priority);
CREATE INDEX IF NOT EXISTS idx_maintenance_orders_device ON maintenance_orders(device_id,created_at DESC);
CREATE INDEX IF NOT EXISTS idx_maintenance_orders_plan ON maintenance_orders(plan_id);
CREATE INDEX IF NOT EXISTS idx_maintenance_events_order_time ON maintenance_order_events(order_id,created_at DESC);
CREATE UNIQUE INDEX IF NOT EXISTS uq_maintenance_active_plan_order ON maintenance_orders(plan_id)
WHERE plan_id IS NOT NULL AND status IN ('open','planned','in_progress','waiting_parts');

DO $$ BEGIN
    IF to_regclass('public.defect_reports') IS NOT NULL
       AND NOT EXISTS (SELECT 1 FROM warehouse_schema_migrations WHERE version='042_maintenance_work_management') THEN
        IF EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name='defect_reports' AND column_name='title') THEN
            EXECUTE $migration$
                INSERT INTO maintenance_orders(legacy_defect_id,device_id,order_type,priority,status,title,description,
                    reported_by,assigned_to,due_at,started_at,completed_at,outcome,resolution,cost,created_at,updated_at)
                SELECT defect_id,device_id,'defect',
                    CASE severity WHEN 'low' THEN 'low' WHEN 'high' THEN 'high' WHEN 'critical' THEN 'critical' ELSE 'normal' END,
                    CASE status WHEN 'in_progress' THEN 'in_progress' WHEN 'repaired' THEN 'completed' WHEN 'closed' THEN 'completed' ELSE 'open' END,
                    COALESCE(NULLIF(title,''),'Defektmeldung #'||defect_id),description,reported_by,assigned_to,NULL,
                    CASE WHEN status='in_progress' THEN reported_at ELSE NULL END,
                    COALESCE(closed_at,repaired_at),CASE WHEN status IN ('repaired','closed') THEN 'repaired' ELSE NULL END,
                    repair_notes,repair_cost,reported_at,reported_at
                FROM defect_reports ON CONFLICT (legacy_defect_id) DO NOTHING
            $migration$;
        ELSE
            INSERT INTO maintenance_orders(legacy_defect_id,device_id,order_type,priority,status,title,description,
                reported_by,completed_at,outcome,resolution,created_at,updated_at)
            SELECT defect_id,device_id,'defect',
                CASE severity WHEN 'minor' THEN 'low' WHEN 'major' THEN 'high' WHEN 'critical' THEN 'critical' ELSE 'normal' END,
                CASE status WHEN 'resolved' THEN 'completed' WHEN 'closed' THEN 'completed' WHEN 'in_progress' THEN 'in_progress' ELSE 'open' END,
                'Defektmeldung #'||defect_id,description,reported_by,resolved_at,
                CASE WHEN resolved_at IS NOT NULL THEN 'repaired' ELSE NULL END,resolution,created_at,updated_at
            FROM defect_reports ON CONFLICT (legacy_defect_id) DO NOTHING;
        END IF;
    END IF;
END $$;

INSERT INTO maintenance_plans(device_id,name,maintenance_type,interval_days,lead_time_days,next_due_at,last_completed_at,is_active)
SELECT d.deviceID,'Regelmäßige Wartung','preventive',COALESCE(NULLIF(p.maintenanceinterval,0),365),14,
       d.nextmaintenance,d.lastmaintenance,TRUE
FROM devices d LEFT JOIN products p ON p.productID=d.productID
WHERE d.nextmaintenance IS NOT NULL
ON CONFLICT (device_id,name) DO NOTHING;

INSERT INTO warehouse_schema_migrations(version)
VALUES ('042_maintenance_work_management')
ON CONFLICT(version) DO NOTHING;

COMMIT;
