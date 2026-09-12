-- All queries are parameterized. Values such as $1 and $2 are supplied by the driver.

-- 1. Project members and their roles.
SELECT u.id, u.email, u.display_name, m.role, m.joined_at
FROM memberships AS m
JOIN users AS u ON u.id = m.user_id
WHERE m.project_id = $1
  AND m.removed_at IS NULL
  AND u.deleted_at IS NULL
ORDER BY m.joined_at, u.id;

-- 2. Tasks with optional status filter and pagination.
SELECT t.id, t.project_id, t.title, t.status, t.priority, t.assignee_id, t.created_at
FROM tasks AS t
WHERE t.project_id = $1
  AND t.deleted_at IS NULL
  AND ($2::text = '' OR t.status = $2)
ORDER BY t.created_at DESC, t.id DESC
LIMIT $3 OFFSET $4;

-- 3. Task count for pagination metadata.
SELECT count(*)
FROM tasks AS t
WHERE t.project_id = $1
  AND t.deleted_at IS NULL
  AND ($2::text = '' OR t.status = $2);

-- 4. Project summary using aggregation.
SELECT p.id, p.name,
       count(t.id) FILTER (WHERE t.deleted_at IS NULL) AS task_count,
       count(t.id) FILTER (WHERE t.status = 'done' AND t.deleted_at IS NULL) AS completed_count
FROM projects AS p
LEFT JOIN tasks AS t ON t.project_id = p.id
WHERE p.id = $1
  AND p.deleted_at IS NULL
GROUP BY p.id, p.name;

-- 5. CTE: active projects with unfinished task counts.
WITH open_tasks AS (
    SELECT project_id, count(*) AS open_count
    FROM tasks
    WHERE deleted_at IS NULL AND status <> 'done'
    GROUP BY project_id
)
SELECT p.id, p.name, coalesce(o.open_count, 0) AS open_count
FROM projects AS p
LEFT JOIN open_tasks AS o ON o.project_id = p.id
WHERE p.owner_user_id = $1
  AND p.deleted_at IS NULL
ORDER BY p.created_at DESC;

-- 6. Safe membership check used by authorization.
SELECT 1
FROM memberships
WHERE project_id = $1
  AND user_id = $2
  AND removed_at IS NULL;
