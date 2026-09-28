-- 0006_indexes.down.sql
BEGIN;

DROP INDEX IF EXISTS tasks_project_summary_idx;
DROP INDEX IF EXISTS tasks_global_status_idx;

COMMIT;
