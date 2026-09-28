-- 0005_user_quotas.up.sql
BEGIN;

CREATE TABLE IF NOT EXISTS user_quotas (
    user_id                 BIGINT PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    max_projects            INT,
    max_members_per_project INT,
    max_tasks_per_project   INT,
    set_by                  BIGINT REFERENCES users(id) ON DELETE SET NULL,
    set_at                  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at              TIMESTAMPTZ NOT NULL DEFAULT now(),
    reason                  TEXT,
    -- القيم: NULL = استخدم الافتراضي، -1 = unlimited، 0 = blocked، >0 = حد
    CONSTRAINT user_quotas_projects_range
        CHECK (max_projects IS NULL OR max_projects >= -1),
    CONSTRAINT user_quotas_members_range
        CHECK (max_members_per_project IS NULL OR max_members_per_project >= -1),
    CONSTRAINT user_quotas_tasks_range
        CHECK (max_tasks_per_project IS NULL OR max_tasks_per_project >= -1)
);

-- للاستعلام "ما quotas التي عدّلها Admin X؟"
CREATE INDEX IF NOT EXISTS user_quotas_set_by_idx
    ON user_quotas (set_by)
    WHERE set_by IS NOT NULL;

COMMIT;
