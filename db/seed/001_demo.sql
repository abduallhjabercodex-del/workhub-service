BEGIN;

INSERT INTO users (id, email, display_name, password_hash)
OVERRIDING SYSTEM VALUE
VALUES
    (1, 'owner@workhub.local', 'WorkHub Owner', '$argon2id$v=19$m=65536,t=3,p=1$demo$replace-in-day-11'),
    (2, 'member@workhub.local', 'WorkHub Member', '$argon2id$v=19$m=65536,t=3,p=1$demo$replace-in-day-11')
ON CONFLICT (id) DO NOTHING;

INSERT INTO projects (id, owner_user_id, name, description)
OVERRIDING SYSTEM VALUE
VALUES (1, 1, 'WorkHub Demo', 'Seed project for local development')
ON CONFLICT (id) DO NOTHING;

INSERT INTO memberships (project_id, user_id, role)
VALUES
    (1, 1, 'owner'),
    (1, 2, 'member')
ON CONFLICT (project_id, user_id) DO NOTHING;

INSERT INTO tasks (id, project_id, created_by, assignee_id, title, status, priority)
OVERRIDING SYSTEM VALUE
VALUES
    (1, 1, 1, 2, 'Review relational schema', 'in_progress', 2),
    (2, 1, 1, 2, 'Write integration tests', 'todo', 3)
ON CONFLICT (id) DO NOTHING;

INSERT INTO tags (id, project_id, name, color)
OVERRIDING SYSTEM VALUE
VALUES (1, 1, 'backend', '#2563EB')
ON CONFLICT (id) DO NOTHING;

INSERT INTO task_tags (task_id, tag_id)
VALUES (1, 1)
ON CONFLICT (task_id, tag_id) DO NOTHING;

INSERT INTO audit_events (actor_user_id, project_id, task_id, event_type, payload)
VALUES
    (1, 1, 1, 'task.created', '{"source":"seed"}'),
    (1, 1, 2, 'task.created', '{"source":"seed"}');

-- Keep identity sequences ahead of deterministic demo IDs.
SELECT setval(pg_get_serial_sequence('users', 'id'), COALESCE((SELECT max(id) FROM users), 1), true);
SELECT setval(pg_get_serial_sequence('projects', 'id'), COALESCE((SELECT max(id) FROM projects), 1), true);
SELECT setval(pg_get_serial_sequence('tasks', 'id'), COALESCE((SELECT max(id) FROM tasks), 1), true);
SELECT setval(pg_get_serial_sequence('tags', 'id'), COALESCE((SELECT max(id) FROM tags), 1), true);

COMMIT;
