package repository

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	"workhub-service/internal/domain"
	"workhub-service/internal/infrastructure/metrics"

	"github.com/jackc/pgx/v5"
)

// listProjectsSQLBase is the query without ORDER BY. The ORDER BY
// and pagination clauses are appended at runtime using a whitelisted
// column and hardcoded direction — never user input.
const listProjectsSQLBase = `
SELECT p.id, p.owner_user_id, p.name, p.created_at
FROM projects p
WHERE p.deleted_at IS NULL
  AND ($1 = '' OR p.name ILIKE '%' || $1 || '%' ESCAPE '\')
  AND (
        $2::bigint IS NULL
     OR p.owner_user_id = $2
     OR EXISTS (
          SELECT 1
          FROM memberships m
          WHERE m.project_id = p.id
            AND m.user_id = $2
            AND m.removed_at IS NULL
     )
  )
`

// CreateProject — ينشئ مشروعًا بمالك صريح. لا Quota.
func (r *PostgresRepository) CreateProject(ctx context.Context, p domain.Project) (domain.Project, error) {
	return r.createProjectTx(ctx, p, -1, false)
}

// CreateProjectWithQuota — ينشئ مشروعًا مع فحص Quota ذرّي.
// maxProjects: -1 = unlimited, 0 = blocked, >0 = exact limit.
func (r *PostgresRepository) CreateProjectWithQuota(ctx context.Context, p domain.Project, maxProjects int) (domain.Project, error) {
	return r.createProjectTx(ctx, p, maxProjects, true)
}

// createProjectTx — التنفيذ المشترك (يقلل التكرار).
func (r *PostgresRepository) createProjectTx(
	ctx context.Context,
	p domain.Project,
	maxProjects int,
	enforceQuota bool,
) (domain.Project, error) {
	if p.OwnerID == "" {
		return domain.Project{}, ErrInvalidUserID
	}
	ownerID, err := parseID(p.OwnerID)
	if err != nil {
		return domain.Project{}, ErrInvalidUserID
	}

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return domain.Project{}, mapDBError(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if enforceQuota {
		// Advisory lock بصيغة (int, int) — يمنع تصادم 32-bit.
		stopLock := metrics.ObserveDBQuery("SELECT_LOCK", "projects")
		_, errLock := tx.Exec(ctx,
			`SELECT pg_advisory_xact_lock(hashtext('quota:projects')::int, hashtext($1::text)::int)`,// وبالتالي User A لا يحتاج أن ينتظر User B بسبب Quota المشاريع الخاصة بهما.
			strconv.FormatInt(ownerID, 10),
		)
		stopLock(&errLock)
		if errLock != nil {
			return domain.Project{}, mapDBError(errLock)
		}

		var count int
		stopCount := metrics.ObserveDBQuery("SELECT_COUNT", "projects")
		errCount := tx.QueryRow(ctx,
			`SELECT count(*) FROM projects WHERE owner_user_id = $1 AND deleted_at IS NULL`,
			ownerID,
		).Scan(&count)
		stopCount(&errCount)
		if errCount != nil {
			return domain.Project{}, mapDBError(errCount)
		}

		if maxProjects >= 0 && count >= maxProjects {
			return domain.Project{}, &domain.QuotaError{
				Resource: "projects", Current: count, Max: maxProjects,
			}
		}
	}

	var id int64
	stopInsert := metrics.ObserveDBQuery("INSERT", "projects")
	errInsert := tx.QueryRow(ctx, `
		INSERT INTO projects (owner_user_id, name, created_at, updated_at)
		VALUES ($1, $2, $3, $3)
		RETURNING id`, ownerID, p.Name, p.CreatedAt).Scan(&id)
	stopInsert(&errInsert)
	if errInsert != nil {
		return domain.Project{}, mapDBError(errInsert)
	}

	stopMember := metrics.ObserveDBQuery("INSERT", "memberships")
	_, errMember := tx.Exec(ctx, `
		INSERT INTO memberships (project_id, user_id, role, joined_at)
		VALUES ($1, $2, 'owner', $3)`, id, ownerID, p.CreatedAt)
	stopMember(&errMember)
	if errMember != nil {
		return domain.Project{}, mapDBError(errMember)
	}

	if err := tx.Commit(ctx); err != nil {
		return domain.Project{}, mapDBError(err)
	}

	p.ID = strconv.FormatInt(id, 10)
	p.OwnerID = strconv.FormatInt(ownerID, 10)
	return p, nil
}

func (r *PostgresRepository) CountUserProjects(ctx context.Context, ownerIDStr string) (int, error) {
	ownerID, err := parseID(ownerIDStr)
	if err != nil {
		return 0, ErrInvalidUserID
	}
	var count int
	stop := metrics.ObserveDBQuery("SELECT_COUNT", "projects")
	err = r.pool.QueryRow(ctx,
		`SELECT count(*) FROM projects WHERE owner_user_id = $1 AND deleted_at IS NULL`,
		ownerID,
	).Scan(&count)
	stop(&err)
	if err != nil {
		return 0, mapDBError(err)
	}
	return count, nil
}

func (r *PostgresRepository) ListProjects(
	ctx context.Context,
	userID, q, sortBy string,
	limit, offset int,
) ([]domain.Project, error) {
	orderCol, ok := allowedProjectSort[sortBy]
	if sortBy == "" {
		orderCol, ok = "p.created_at", true
	}
	if !ok {
		return nil, ErrInvalidSort
	}
	orderDir := "DESC"
	if sortBy == "name" {
		orderDir = "ASC"
	}
	order := fmt.Sprintf("%s %s, p.id %s", orderCol, orderDir, orderDir)

	var numUserID *int64
	if userID != "" {
		parsed, err := parseID(userID)
		if err != nil {
			return nil, ErrInvalidUserID
		}
		numUserID = &parsed
	}

	search := escapeLike(q)

	if limit <= 0 || limit > 100 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}

	query := listProjectsSQLBase + " ORDER BY " + order + " LIMIT $3 OFFSET $4"
	stop := metrics.ObserveDBQuery("SELECT", "projects")
	rows, err := r.pool.Query(ctx, query, search, numUserID, limit, offset)
	stop(&err)
	if err != nil {
		return nil, mapDBError(err)
	}
	defer rows.Close()

	items := make([]domain.Project, 0, limit)
	for rows.Next() {
		var id, ownerID int64
		var p domain.Project
		if err := rows.Scan(&id, &ownerID, &p.Name, &p.CreatedAt); err != nil {
			return nil, mapDBError(err)
		}
		p.ID = strconv.FormatInt(id, 10)
		p.OwnerID = strconv.FormatInt(ownerID, 10)
		items = append(items, p)
	}

	if err := rows.Err(); err != nil {
		return nil, mapDBError(err)
	}

	return items, nil
}

func (r *PostgresRepository) GetProject(ctx context.Context, id string) (domain.Project, error) {
	projectID, err := parseID(id)
	if err != nil {
		return domain.Project{}, ErrInvalidProjectID
	}
	var p domain.Project
	var numericID, ownerID int64
	stop := metrics.ObserveDBQuery("SELECT", "projects")
	err = r.pool.QueryRow(ctx, `
		SELECT id, owner_user_id, name, created_at
		FROM projects WHERE id = $1 AND deleted_at IS NULL`, projectID).
		Scan(&numericID, &ownerID, &p.Name, &p.CreatedAt)
	stop(&err)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Project{}, ErrNotFound
	}
	if err != nil {
		return domain.Project{}, mapDBError(err)
	}
	p.ID = strconv.FormatInt(numericID, 10)
	p.OwnerID = strconv.FormatInt(ownerID, 10)
	return p, nil
}

func (r *PostgresRepository) GetProjectSummary(ctx context.Context, projectID string) (domain.ProjectSummary, error) {
	pid, err := parseID(projectID)
	if err != nil {
		return domain.ProjectSummary{}, ErrInvalidProjectID
	}
	var summary domain.ProjectSummary
	var numericID int64
	stop := metrics.ObserveDBQuery("SELECT", "projects")
	err = r.pool.QueryRow(ctx, `
		SELECT p.id,
		       count(t.id),
		       count(t.id) FILTER (WHERE t.status = 'done'),
		       count(t.id) FILTER (WHERE t.status <> 'done')
		FROM projects p
		LEFT JOIN tasks t ON t.project_id = p.id AND t.deleted_at IS NULL
		WHERE p.id = $1 AND p.deleted_at IS NULL
		GROUP BY p.id`, pid).Scan(&numericID, &summary.TaskCount, &summary.CompletedCount, &summary.OpenCount)
	stop(&err)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.ProjectSummary{}, ErrNotFound
	}
	if err != nil {
		return domain.ProjectSummary{}, mapDBError(err)
	}
	summary.ProjectID = strconv.FormatInt(numericID, 10)
	return summary, nil
}

func (r *PostgresRepository) DeleteProject(ctx context.Context, idStr string) error {
	id, err := parseID(idStr)
	if err != nil {
		return ErrInvalidProjectID
	}
	stop := metrics.ObserveDBQuery("UPDATE", "projects")
	tag, err := r.pool.Exec(ctx,
		`UPDATE projects SET deleted_at = NOW() WHERE id = $1 AND deleted_at IS NULL`, id)
	stop(&err)
	if err != nil {
		return mapDBError(err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}
