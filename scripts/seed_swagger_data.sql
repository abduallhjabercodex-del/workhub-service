-- Seed WorkHub test accounts, projects, memberships, and tasks for Swagger UI Developer Portal

-- 1. Insert Users (owner, admin, member, viewer)
-- Password for all accounts is: Password1234!
-- Hash generated with bcrypt cost 12: $2a$12$ktxSBI8GxZrPQKXG1PqHNexBbb0ujVEVHrbQwWDfDjJXoP3oc4.ga
INSERT INTO users (email, display_name, password_hash)
VALUES
  ('owner@workhub.dev', 'System Owner', '$2a$12$ktxSBI8GxZrPQKXG1PqHNexBbb0ujVEVHrbQwWDfDjJXoP3oc4.ga'),
  ('admin@workhub.dev', 'Project Admin', '$2a$12$ktxSBI8GxZrPQKXG1PqHNexBbb0ujVEVHrbQwWDfDjJXoP3oc4.ga'),
  ('member@workhub.dev', 'Team Member', '$2a$12$ktxSBI8GxZrPQKXG1PqHNexBbb0ujVEVHrbQwWDfDjJXoP3oc4.ga'),
  ('viewer@workhub.dev', 'Guest Viewer', '$2a$12$ktxSBI8GxZrPQKXG1PqHNexBbb0ujVEVHrbQwWDfDjJXoP3oc4.ga')
ON CONFLICT (lower(email)) WHERE deleted_at IS NULL DO UPDATE
SET display_name = EXCLUDED.display_name,
    password_hash = EXCLUDED.password_hash;

-- 2. User Quotas
INSERT INTO user_quotas (user_id, max_projects, max_members_per_project, max_tasks_per_project, reason)
SELECT id, 100, 50, 500, 'Enterprise Owner Tier'
FROM users WHERE email = 'owner@workhub.dev'
ON CONFLICT (user_id) DO NOTHING;

INSERT INTO user_quotas (user_id, max_projects, max_members_per_project, max_tasks_per_project, reason)
SELECT id, 20, 20, 100, 'Admin Tier'
FROM users WHERE email = 'admin@workhub.dev'
ON CONFLICT (user_id) DO NOTHING;

-- 3. Projects
INSERT INTO projects (name, description, owner_user_id)
SELECT 'Core WorkHub Platform', 'Main backend APIs and microservices platform', id
FROM users WHERE email = 'owner@workhub.dev'
AND NOT EXISTS (SELECT 1 FROM projects WHERE name = 'Core WorkHub Platform' AND deleted_at IS NULL);

INSERT INTO projects (name, description, owner_user_id)
SELECT 'Mobile Application iOS/Android', 'Cross-platform Flutter application for iOS and Android', id
FROM users WHERE email = 'admin@workhub.dev'
AND NOT EXISTS (SELECT 1 FROM projects WHERE name = 'Mobile Application iOS/Android' AND deleted_at IS NULL);

INSERT INTO projects (name, description, owner_user_id)
SELECT 'DevOps & Infrastructure', 'Kubernetes, Helm, Prometheus and CI/CD pipelines', id
FROM users WHERE email = 'owner@workhub.dev'
AND NOT EXISTS (SELECT 1 FROM projects WHERE name = 'DevOps & Infrastructure' AND deleted_at IS NULL);

-- 4. Memberships
-- Project 1 (Core WorkHub Platform)
INSERT INTO memberships (project_id, user_id, role)
SELECT p.id, u.id, 'owner'
FROM projects p, users u
WHERE p.name = 'Core WorkHub Platform' AND u.email = 'owner@workhub.dev'
ON CONFLICT (project_id, user_id) DO UPDATE SET role = 'owner';

INSERT INTO memberships (project_id, user_id, role)
SELECT p.id, u.id, 'admin'
FROM projects p, users u
WHERE p.name = 'Core WorkHub Platform' AND u.email = 'admin@workhub.dev'
ON CONFLICT (project_id, user_id) DO UPDATE SET role = 'admin';

INSERT INTO memberships (project_id, user_id, role)
SELECT p.id, u.id, 'member'
FROM projects p, users u
WHERE p.name = 'Core WorkHub Platform' AND u.email = 'member@workhub.dev'
ON CONFLICT (project_id, user_id) DO UPDATE SET role = 'member';

INSERT INTO memberships (project_id, user_id, role)
SELECT p.id, u.id, 'viewer'
FROM projects p, users u
WHERE p.name = 'Core WorkHub Platform' AND u.email = 'viewer@workhub.dev'
ON CONFLICT (project_id, user_id) DO UPDATE SET role = 'viewer';

-- Project 2 (Mobile Application iOS/Android)
INSERT INTO memberships (project_id, user_id, role)
SELECT p.id, u.id, 'owner'
FROM projects p, users u
WHERE p.name = 'Mobile Application iOS/Android' AND u.email = 'admin@workhub.dev'
ON CONFLICT (project_id, user_id) DO UPDATE SET role = 'owner';

INSERT INTO memberships (project_id, user_id, role)
SELECT p.id, u.id, 'member'
FROM projects p, users u
WHERE p.name = 'Mobile Application iOS/Android' AND u.email = 'member@workhub.dev'
ON CONFLICT (project_id, user_id) DO UPDATE SET role = 'member';

INSERT INTO memberships (project_id, user_id, role)
SELECT p.id, u.id, 'viewer'
FROM projects p, users u
WHERE p.name = 'Mobile Application iOS/Android' AND u.email = 'viewer@workhub.dev'
ON CONFLICT (project_id, user_id) DO UPDATE SET role = 'viewer';

-- Project 3 (DevOps & Infrastructure)
INSERT INTO memberships (project_id, user_id, role)
SELECT p.id, u.id, 'owner'
FROM projects p, users u
WHERE p.name = 'DevOps & Infrastructure' AND u.email = 'owner@workhub.dev'
ON CONFLICT (project_id, user_id) DO UPDATE SET role = 'owner';

INSERT INTO memberships (project_id, user_id, role)
SELECT p.id, u.id, 'admin'
FROM projects p, users u
WHERE p.name = 'DevOps & Infrastructure' AND u.email = 'admin@workhub.dev'
ON CONFLICT (project_id, user_id) DO UPDATE SET role = 'admin';

-- 5. Tasks
-- Project 1 Tasks
INSERT INTO tasks (project_id, created_by, title, status, priority)
SELECT p.id, u.id, 'Implement JWT Refresh Token Rotation', 'done', 1
FROM projects p, users u
WHERE p.name = 'Core WorkHub Platform' AND u.email = 'owner@workhub.dev'
AND NOT EXISTS (SELECT 1 FROM tasks t WHERE t.project_id = p.id AND t.title = 'Implement JWT Refresh Token Rotation');

INSERT INTO tasks (project_id, created_by, title, status, priority)
SELECT p.id, u.id, 'Zero-Trust RBAC Membership Verification', 'in_progress', 2
FROM projects p, users u
WHERE p.name = 'Core WorkHub Platform' AND u.email = 'owner@workhub.dev'
AND NOT EXISTS (SELECT 1 FROM tasks t WHERE t.project_id = p.id AND t.title = 'Zero-Trust RBAC Membership Verification');

INSERT INTO tasks (project_id, created_by, title, status, priority)
SELECT p.id, u.id, 'Interactive Swagger UI & OpenAPI Specification', 'done', 2
FROM projects p, users u
WHERE p.name = 'Core WorkHub Platform' AND u.email = 'owner@workhub.dev'
AND NOT EXISTS (SELECT 1 FROM tasks t WHERE t.project_id = p.id AND t.title = 'Interactive Swagger UI & OpenAPI Specification');

INSERT INTO tasks (project_id, created_by, title, status, priority)
SELECT p.id, u.id, 'Setup PostgreSQL Backup & Restore Procedures', 'done', 1
FROM projects p, users u
WHERE p.name = 'Core WorkHub Platform' AND u.email = 'owner@workhub.dev'
AND NOT EXISTS (SELECT 1 FROM tasks t WHERE t.project_id = p.id AND t.title = 'Setup PostgreSQL Backup & Restore Procedures');

-- Project 2 Tasks
INSERT INTO tasks (project_id, created_by, title, status, priority)
SELECT p.id, u.id, 'Build Flutter Mobile UI Auth Screen', 'in_progress', 3
FROM projects p, users u
WHERE p.name = 'Mobile Application iOS/Android' AND u.email = 'admin@workhub.dev'
AND NOT EXISTS (SELECT 1 FROM tasks t WHERE t.project_id = p.id AND t.title = 'Build Flutter Mobile UI Auth Screen');

INSERT INTO tasks (project_id, created_by, title, status, priority)
SELECT p.id, u.id, 'Push Notifications Notification Worker', 'todo', 4
FROM projects p, users u
WHERE p.name = 'Mobile Application iOS/Android' AND u.email = 'admin@workhub.dev'
AND NOT EXISTS (SELECT 1 FROM tasks t WHERE t.project_id = p.id AND t.title = 'Push Notifications Notification Worker');

-- Project 3 Tasks
INSERT INTO tasks (project_id, created_by, title, status, priority)
SELECT p.id, u.id, 'Kubernetes Production Helm Charts Deployment', 'todo', 2
FROM projects p, users u
WHERE p.name = 'DevOps & Infrastructure' AND u.email = 'owner@workhub.dev'
AND NOT EXISTS (SELECT 1 FROM tasks t WHERE t.project_id = p.id AND t.title = 'Kubernetes Production Helm Charts Deployment');

INSERT INTO tasks (project_id, created_by, title, status, priority)
SELECT p.id, u.id, 'Configure Redis Cluster Caching & Rate Limiting', 'in_progress', 2
FROM projects p, users u
WHERE p.name = 'DevOps & Infrastructure' AND u.email = 'owner@workhub.dev'
AND NOT EXISTS (SELECT 1 FROM tasks t WHERE t.project_id = p.id AND t.title = 'Configure Redis Cluster Caching & Rate Limiting');
