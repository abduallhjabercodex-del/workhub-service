\set ON_ERROR_STOP on

BEGIN;

-- These tests are intended to run against a disposable database after 001_init.up.sql.
INSERT INTO users (email, display_name, password_hash)
VALUES ('constraint-owner@example.test', 'Constraint Owner', 'hash');

INSERT INTO projects (owner_user_id, name)
SELECT id, 'Constraint Project'
FROM users
WHERE email = 'constraint-owner@example.test';

INSERT INTO memberships (project_id, user_id, role)
SELECT p.id, p.owner_user_id, 'owner'
FROM projects AS p
WHERE p.name = 'Constraint Project';

DO $$
BEGIN
    BEGIN
        INSERT INTO users (email, display_name, password_hash)
        VALUES ('CONSTRAINT-OWNER@example.test', 'Duplicate', 'hash');
        RAISE EXCEPTION 'expected case-insensitive email uniqueness violation';
    EXCEPTION WHEN unique_violation THEN
        RAISE NOTICE 'PASS: email uniqueness is case-insensitive';
    END;
END $$;

DO $$
BEGIN
    BEGIN
        INSERT INTO tasks (project_id, created_by, title, status)
        SELECT p.id, p.owner_user_id, 'Invalid status', 'blocked'
        FROM projects AS p
        WHERE p.name = 'Constraint Project';
        RAISE EXCEPTION 'expected task status check violation';
    EXCEPTION WHEN check_violation THEN
        RAISE NOTICE 'PASS: task status check rejects unknown values';
    END;
END $$;

DO $$
BEGIN
    BEGIN
        INSERT INTO tasks (project_id, created_by, title, priority)
        SELECT p.id, p.owner_user_id, 'Invalid priority', 9
        FROM projects AS p
        WHERE p.name = 'Constraint Project';
        RAISE EXCEPTION 'expected task priority check violation';
    EXCEPTION WHEN check_violation THEN
        RAISE NOTICE 'PASS: task priority check rejects out-of-range values';
    END;
END $$;

DO $$
BEGIN
    BEGIN
        INSERT INTO memberships (project_id, user_id, role)
        SELECT p.id, p.owner_user_id, 'member'
        FROM projects AS p
        WHERE p.name = 'Constraint Project';
        RAISE EXCEPTION 'expected membership duplicate violation';
    EXCEPTION WHEN unique_violation THEN
        RAISE NOTICE 'PASS: membership primary key prevents duplicates';
    END;
END $$;

DO $$
BEGIN
    BEGIN
        INSERT INTO task_tags (task_id, tag_id) VALUES (999999, 999999);
        RAISE EXCEPTION 'expected task_tags foreign key violation';
    EXCEPTION WHEN foreign_key_violation THEN
        RAISE NOTICE 'PASS: task_tags foreign keys protect references';
    END;
END $$;

ROLLBACK;
