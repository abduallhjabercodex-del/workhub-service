BEGIN;

-- Supports project summary aggregation and project-scoped task lookups.
-- The partial predicate matches the application's deleted_at filter.
CREATE INDEX tasks_project_summary_idx
    ON tasks (project_id)
    INCLUDE (status)
    WHERE deleted_at IS NULL;

COMMIT;
