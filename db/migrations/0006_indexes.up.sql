-- 0006_indexes.up.sql
-- فهارس إضافية للأداء.
BEGIN;

-- للـ ProjectSummary aggregation و project-scoped task lookups
-- (فهرس موجود في 0001 لكن نُكرّره هنا لأمان idempotency إن كانت migration جزئية)
CREATE INDEX IF NOT EXISTS tasks_project_summary_idx
    ON tasks (project_id)
    INCLUDE (status)
    WHERE deleted_at IS NULL;

-- للـ global task listing (مستقبلي)
CREATE INDEX IF NOT EXISTS tasks_global_status_idx
    ON tasks (status, created_at DESC)
    WHERE deleted_at IS NULL;

COMMIT;
