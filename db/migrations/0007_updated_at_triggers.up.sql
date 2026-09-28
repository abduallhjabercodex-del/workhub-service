-- 0007_updated_at_triggers.up.sql
-- Trigger موحّد لتحديث updated_at تلقائيًا.
BEGIN;

CREATE OR REPLACE FUNCTION set_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = now();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- users
DROP TRIGGER IF EXISTS users_set_updated_at ON users;
CREATE TRIGGER users_set_updated_at
    BEFORE UPDATE ON users
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- projects
DROP TRIGGER IF EXISTS projects_set_updated_at ON projects;
CREATE TRIGGER projects_set_updated_at
    BEFORE UPDATE ON projects
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- tasks
DROP TRIGGER IF EXISTS tasks_set_updated_at ON tasks;
CREATE TRIGGER tasks_set_updated_at
    BEFORE UPDATE ON tasks
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- user_quotas
DROP TRIGGER IF EXISTS user_quotas_set_updated_at ON user_quotas;
CREATE TRIGGER user_quotas_set_updated_at
    BEFORE UPDATE ON user_quotas
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

COMMIT;
