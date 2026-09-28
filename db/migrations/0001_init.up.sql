-- 0001_init.up.sql
-- الجداول الأساسية: users, projects, memberships, tasks, tags, task_tags, audit_events
-- ملاحظة: role CHECK لا يشمل viewer هنا — يُضاف في 0004.

BEGIN;

-- ============================================================
-- users
-- ============================================================
CREATE TABLE users (
    id             BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    email          TEXT NOT NULL,
    display_name   TEXT NOT NULL,
    password_hash  TEXT NOT NULL,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at     TIMESTAMPTZ,
    CONSTRAINT users_email_not_blank         CHECK (btrim(email) <> ''),
    CONSTRAINT users_display_name_not_blank  CHECK (btrim(display_name) <> ''),
    CONSTRAINT users_password_hash_not_blank CHECK (btrim(password_hash) <> '')
);

CREATE UNIQUE INDEX users_email_lower_uq
    ON users (lower(email))
    WHERE deleted_at IS NULL;

-- ============================================================
-- projects
-- ============================================================
CREATE TABLE projects (
    id             BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    owner_user_id  BIGINT NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    name           TEXT NOT NULL,
    description    TEXT NOT NULL DEFAULT '',
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at     TIMESTAMPTZ,
    CONSTRAINT projects_name_not_blank CHECK (btrim(name) <> '')
);

CREATE INDEX projects_owner_idx
    ON projects (owner_user_id)
    WHERE deleted_at IS NULL;

-- ============================================================
-- memberships
-- ملاحظة: لا يوجد memberships_active_role (كان يكسر RemoveProjectMemberSafely).
-- ============================================================
CREATE TABLE memberships (
    project_id  BIGINT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    user_id     BIGINT NOT NULL REFERENCES users(id)    ON DELETE CASCADE,
    role        TEXT NOT NULL DEFAULT 'member',
    joined_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    removed_at  TIMESTAMPTZ,
    PRIMARY KEY (project_id, user_id),
    CONSTRAINT memberships_role_valid CHECK (role IN ('owner', 'admin', 'member'))
);

CREATE INDEX memberships_user_idx
    ON memberships (user_id)
    WHERE removed_at IS NULL;

CREATE INDEX memberships_project_idx
    ON memberships (project_id)
    WHERE removed_at IS NULL;

-- ============================================================
-- tasks
-- ============================================================
CREATE TABLE tasks (
    id           BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    project_id   BIGINT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    created_by   BIGINT NOT NULL REFERENCES users(id)    ON DELETE RESTRICT,
    assignee_id  BIGINT REFERENCES users(id)             ON DELETE SET NULL,
    title        TEXT NOT NULL,
    description  TEXT NOT NULL DEFAULT '',
    status       TEXT NOT NULL DEFAULT 'todo',
    priority     SMALLINT NOT NULL DEFAULT 3,
    due_at       TIMESTAMPTZ,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at   TIMESTAMPTZ,
    CONSTRAINT tasks_title_not_blank  CHECK (btrim(title) <> ''),
    CONSTRAINT tasks_status_valid     CHECK (status IN ('todo', 'in_progress', 'done')),
    CONSTRAINT tasks_priority_valid   CHECK (priority BETWEEN 1 AND 5)
);

CREATE INDEX tasks_project_status_created_idx
    ON tasks (project_id, status, created_at DESC)
    WHERE deleted_at IS NULL;

CREATE INDEX tasks_assignee_idx
    ON tasks (assignee_id)
    WHERE deleted_at IS NULL;

-- ============================================================
-- tags
-- ============================================================
CREATE TABLE tags (
    id          BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    project_id  BIGINT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    name        TEXT NOT NULL,
    color       TEXT NOT NULL DEFAULT '#64748B',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT tags_name_not_blank CHECK (btrim(name) <> ''),
    CONSTRAINT tags_color_hex      CHECK (color ~ '^#[0-9A-Fa-f]{6}$'),
    CONSTRAINT tags_project_name_uq UNIQUE (project_id, name)
);

-- ============================================================
-- task_tags
-- ============================================================
CREATE TABLE task_tags (
    task_id     BIGINT NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
    tag_id      BIGINT NOT NULL REFERENCES tags(id)  ON DELETE CASCADE,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (task_id, tag_id)
);

-- ============================================================
-- audit_events
-- ============================================================
CREATE TABLE audit_events (
    id              BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    actor_user_id   BIGINT REFERENCES users(id)    ON DELETE SET NULL,
    project_id      BIGINT REFERENCES projects(id) ON DELETE CASCADE,
    task_id         BIGINT REFERENCES tasks(id)    ON DELETE CASCADE,
    event_type      TEXT NOT NULL,
    payload         JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT audit_event_type_not_blank CHECK (btrim(event_type) <> ''),
    CONSTRAINT audit_event_target_exists  CHECK (project_id IS NOT NULL OR task_id IS NOT NULL)
);

CREATE INDEX audit_events_project_created_idx
    ON audit_events (project_id, created_at DESC);

CREATE INDEX audit_events_task_created_idx
    ON audit_events (task_id, created_at DESC);

-- فهرس جديد: للاستعلام "ماذا فعل المستخدم X؟"
CREATE INDEX audit_events_actor_idx
    ON audit_events (actor_user_id, created_at DESC)
    WHERE actor_user_id IS NOT NULL;

COMMIT;
