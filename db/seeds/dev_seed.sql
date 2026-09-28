-- db/seeds/dev_seed.sql
-- ⚠️ DEVELOPMENT ONLY — DO NOT RUN IN PRODUCTION
--
-- Password for all users: Password1234!
-- bcrypt cost 10 (dev only — production uses 12+)
--
-- تنفيذ:
--   psql -d workhub_dev -f db/seeds/dev_seed.sql

BEGIN;

-- ============================================================
-- Users
-- ============================================================
INSERT INTO users (id, email, display_name, password_hash)
OVERRIDING SYSTEM VALUE
VALUES
    (1, 'owner@workhub.dev',  'System Owner',  '$2a$10$azSJI4h.guX0QJZeSGXz7OCoFdZtySru3yFfDRZmVfDznfxk3IcLm'),
    (2, 'admin@workhub.dev',  'Project Admin', '$2a$10$azSJI4h.guX0QJZeSGXz7OCoFdZtySru3yFfDRZmVfDznfxk3IcLm'),
    (3, 'member@workhub.dev', 'Team Member',   '$2a$10$azSJI4h.guX0QJZeSGXz7OCoFdZtySru3yFfDRZmVfDznfxk3IcLm'),
    (4, 'viewer@workhub.dev', 'Guest Viewer',  '$2a$10$azSJI4h.guX0QJZeSGXz7OCoFdZtySru3yFfDRZmVfDznfxk3IcLm')
ON CONFLICT (id) DO UPDATE SET
    email         = EXCLUDED.email,
    display_name  = EXCLUDED.display_name,
    password_hash = EXCLUDED.password_hash;

-- ============================================================
-- Projects
-- ============================================================
INSERT INTO projects (id, owner_user_id, name, description)
OVERRIDING SYSTEM VALUE
VALUES
    (1, 1, 'Core WorkHub Platform',        'Primary backend microservice and database system'),
    (2, 2, 'Mobile Application iOS/Android','Cross-platform Flutter application for team members'),
    (3, 1, 'DevOps & Infrastructure',      'Kubernetes Helm charts, CI/CD pipelines, and monitoring')
ON CONFLICT (id) DO UPDATE SET
    owner_user_id = EXCLUDED.owner_user_id,
    name          = EXCLUDED.name,
    description   = EXCLUDED.description;

-- ============================================================
-- Memberships (RBAC)
-- ============================================================
INSERT INTO memberships (project_id, user_id, role)
VALUES
    -- Project 1
    (1, 1, 'owner'),
    (1, 2, 'admin'),
    (1, 3, 'member'),
    (1, 4, 'viewer'),
    -- Project 2
    (2, 2, 'owner'),
    (2, 3, 'member'),
    (2, 4, 'viewer'),
    -- Project 3
    (3, 1, 'owner'),
    (3, 2, 'admin')
ON CONFLICT (project_id, user_id) DO UPDATE SET
    role       = EXCLUDED.role,
    removed_at = NULL;

-- ============================================================
-- Tasks
-- ============================================================
INSERT INTO tasks (id, project_id, created_by, assignee_id, title, status, priority)
OVERRIDING SYSTEM VALUE
VALUES
    (1, 1, 1, 3, 'Implement JWT Refresh Token Rotation',            'done',        1),
    (2, 1, 1, 2, 'Zero-Trust RBAC Membership Verification',         'in_progress', 1),
    (3, 1, 1, 3, 'Interactive Swagger UI & OpenAPI Specification',  'done',        2),
    (4, 1, 1, 2, 'Setup PostgreSQL Backup & Restore Procedures',    'done',        2),
    (5, 2, 2, 3, 'Build Flutter Mobile UI Auth Screen',             'in_progress', 2),
    (6, 2, 2, 3, 'Push Notifications Notification Worker',          'todo',        3),
    (7, 3, 1, 2, 'Kubernetes Production Helm Charts Deployment',    'todo',        1),
    (8, 3, 1, 2, 'Configure Redis Cluster Caching & Rate Limiting','in_progress', 2)
ON CONFLICT (id) DO UPDATE SET
    title       = EXCLUDED.title,
    status      = EXCLUDED.status,
    priority    = EXCLUDED.priority,
    assignee_id = EXCLUDED.assignee_id;

-- ============================================================
-- إعادة ضبط IDENTITY sequences (الطريقة الصحيحة)
-- `+1, false` يعني: الـ next value هو max+1 بدون استدعائه.
-- ============================================================
SELECT setval(
    pg_get_serial_sequence('users', 'id'),
    (SELECT COALESCE(max(id), 0) FROM users) + 1,
    false
);
SELECT setval(
    pg_get_serial_sequence('projects', 'id'),
    (SELECT COALESCE(max(id), 0) FROM projects) + 1,
    false
);
SELECT setval(
    pg_get_serial_sequence('tasks', 'id'),
    (SELECT COALESCE(max(id), 0) FROM tasks) + 1,
    false
);

COMMIT;
