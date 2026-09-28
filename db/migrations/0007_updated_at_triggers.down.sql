-- 0007_updated_at_triggers.down.sql
BEGIN;

DROP TRIGGER IF EXISTS users_set_updated_at       ON users;
DROP TRIGGER IF EXISTS projects_set_updated_at    ON projects;
DROP TRIGGER IF EXISTS tasks_set_updated_at       ON tasks;
DROP TRIGGER IF EXISTS user_quotas_set_updated_at ON user_quotas;

DROP FUNCTION IF EXISTS set_updated_at();

COMMIT;
