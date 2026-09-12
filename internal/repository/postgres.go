package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"workhub-service/internal/domain"
)

var (
	ErrConflict   = errors.New("storage conflict")
	ErrConstraint = errors.New("storage constraint violation")
)

// TaskAuditRepository marks repositories that can persist a task and its audit atomically.
type TaskAuditRepository interface {
	TaskRepository
	CreateTaskWithAudit(context.Context, domain.Task, domain.AuditEvent) (domain.Task, error)
}

type PostgresRepository struct {
	pool         *pgxpool.Pool
	defaultOwner int64
}

func NewPostgresRepository(pool *pgxpool.Pool, defaultOwner int64) *PostgresRepository {
	if defaultOwner < 1 {
		defaultOwner = 1
	}
	return &PostgresRepository{pool: pool, defaultOwner: defaultOwner}
}

func (r *PostgresRepository) Close() { r.pool.Close() }

func (r *PostgresRepository) CreateProject(ctx context.Context, p domain.Project) (domain.Project, error) {
	var id int64
	err := r.pool.QueryRow(ctx, `
		INSERT INTO projects (owner_user_id, name, created_at, updated_at)
		VALUES ($1, $2, $3, $3)
		RETURNING id`, r.defaultOwner, p.Name, p.CreatedAt).Scan(&id)
	if err != nil {
		return domain.Project{}, mapDBError(err)
	}
	p.ID = strconv.FormatInt(id, 10)
	return p, nil
}

func (r *PostgresRepository) ListProjects(ctx context.Context, q, sortBy string) ([]domain.Project, error) {
	order := "created_at ASC"
	if sortBy == "name" {
		order = "name ASC"
	}
	rows, err := r.pool.Query(ctx, fmt.Sprintf(`
		SELECT id, name, created_at
		FROM projects
		WHERE deleted_at IS NULL AND ($1 = '' OR name ILIKE '%%' || $1 || '%%')
		ORDER BY %s`, order), q)
	if err != nil {
		return nil, mapDBError(err)
	}
	defer rows.Close()
	items := make([]domain.Project, 0)
	for rows.Next() {
		var id int64
		var p domain.Project
		if err := rows.Scan(&id, &p.Name, &p.CreatedAt); err != nil {
			return nil, mapDBError(err)
		}
		p.ID = strconv.FormatInt(id, 10)
		items = append(items, p)
	}
	return items, mapDBError(rows.Err())
}

func (r *PostgresRepository) GetProject(ctx context.Context, id string) (domain.Project, error) {
	projectID, err := parseID(id)
	if err != nil {
		return domain.Project{}, ErrNotFound
	}
	var p domain.Project
	var numericID int64
	err = r.pool.QueryRow(ctx, `
		SELECT id, name, created_at
		FROM projects WHERE id = $1 AND deleted_at IS NULL`, projectID).
		Scan(&numericID, &p.Name, &p.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Project{}, ErrNotFound
	}
	if err != nil {
		return domain.Project{}, mapDBError(err)
	}
	p.ID = strconv.FormatInt(numericID, 10)
	return p, nil
}

func (r *PostgresRepository) CreateTask(ctx context.Context, t domain.Task) (domain.Task, error) {
	projectID, err := parseID(t.ProjectID)
	if err != nil {
		return domain.Task{}, err
	}
	var id int64
	err = r.pool.QueryRow(ctx, `
		INSERT INTO tasks (project_id, created_by, title, status, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $5)
		RETURNING id`, projectID, r.defaultOwner, t.Title, t.Status, t.CreatedAt).Scan(&id)
	if err != nil {
		return domain.Task{}, mapDBError(err)
	}
	t.ID = strconv.FormatInt(id, 10)
	return t, nil
}

func (r *PostgresRepository) CreateTaskWithAudit(ctx context.Context, t domain.Task, audit domain.AuditEvent) (domain.Task, error) {
	projectID, err := parseID(t.ProjectID)
	if err != nil {
		return domain.Task{}, err
	}
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return domain.Task{}, mapDBError(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var id int64
	if err = tx.QueryRow(ctx, `
		INSERT INTO tasks (project_id, created_by, title, status, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $5)
		RETURNING id`, projectID, r.defaultOwner, t.Title, t.Status, t.CreatedAt).Scan(&id); err != nil {
		return domain.Task{}, mapDBError(err)
	}
	t.ID = strconv.FormatInt(id, 10)
	var actor any
	if audit.ActorUserID != "" {
		actorID, parseErr := parseID(audit.ActorUserID)
		if parseErr != nil {
			return domain.Task{}, parseErr
		}
		actor = actorID
	}
	if _, err = tx.Exec(ctx, `
		INSERT INTO audit_events (actor_user_id, project_id, task_id, event_type, payload, created_at)
		VALUES ($1, $2, $3, $4, $5::jsonb, $6)`, actor, projectID, id, audit.EventType, audit.Payload, audit.CreatedAt); err != nil {
		return domain.Task{}, mapDBError(err)
	}
	payload, marshalErr := json.Marshal(map[string]any{"taskId": t.ID, "projectId": t.ProjectID, "title": t.Title, "status": t.Status})
	if marshalErr != nil {
		return domain.Task{}, marshalErr
	}
	eventID := fmt.Sprintf("task-created-%s-%d", t.ID, t.CreatedAt.UnixNano())
	if _, err = tx.Exec(ctx, `
		INSERT INTO outbox_events (event_id, event_type, aggregate_type, aggregate_id, payload, created_at)
		VALUES ($1, $2, $3, $4, $5::jsonb, $6)`, eventID, "task.created", "task", id, payload, t.CreatedAt); err != nil {
		return domain.Task{}, mapDBError(err)
	}
	if err = tx.Commit(ctx); err != nil {
		return domain.Task{}, mapDBError(err)
	}
	return t, nil
}

func (r *PostgresRepository) ListTasks(ctx context.Context, projectID, status, q, sortBy string, page, limit int) ([]domain.Task, int, error) {
	pid, err := parseID(projectID)
	if err != nil {
		return nil, 0, ErrNotFound
	}
	if page < 1 {
		page = 1
	}
	if limit < 1 {
		limit = 20
	}
	offset := (page - 1) * limit
	order := "created_at DESC, id DESC"
	if sortBy == "title" {
		order = "title ASC, id ASC"
	}
	var total int
	if err := r.pool.QueryRow(ctx, `
		SELECT count(*) FROM tasks
		WHERE project_id = $1 AND deleted_at IS NULL
		  AND ($2 = '' OR status = $2)
		  AND ($3 = '' OR title ILIKE '%%' || $3 || '%%')`, pid, status, q).Scan(&total); err != nil {
		return nil, 0, mapDBError(err)
	}
	rows, err := r.pool.Query(ctx, fmt.Sprintf(`
		SELECT id, project_id, title, status, created_at
		FROM tasks
		WHERE project_id = $1 AND deleted_at IS NULL
		  AND ($2 = '' OR status = $2)
			AND ($3 = '' OR title ILIKE '%%' || $3 || '%%')
			ORDER BY %s
			LIMIT $4 OFFSET $5`, order), pid, status, q, limit, offset)
	if err != nil {
		return nil, 0, mapDBError(err)
	}
	defer rows.Close()
	items := make([]domain.Task, 0)
	for rows.Next() {
		var t domain.Task
		var id, rowProjectID int64
		if err := rows.Scan(&id, &rowProjectID, &t.Title, &t.Status, &t.CreatedAt); err != nil {
			return nil, 0, mapDBError(err)
		}
		t.ID = strconv.FormatInt(id, 10)
		t.ProjectID = strconv.FormatInt(rowProjectID, 10)
		items = append(items, t)
	}
	return items, total, mapDBError(rows.Err())
}

func (r *PostgresRepository) GetProjectSummary(ctx context.Context, projectID string) (domain.ProjectSummary, error) {
	pid, err := parseID(projectID)
	if err != nil {
		return domain.ProjectSummary{}, ErrNotFound
	}
	var summary domain.ProjectSummary
	var numericID int64
	err = r.pool.QueryRow(ctx, `
		SELECT p.id,
		       count(t.id),
		       count(t.id) FILTER (WHERE t.status = 'done'),
		       count(t.id) FILTER (WHERE t.status <> 'done')
		FROM projects p
		LEFT JOIN tasks t ON t.project_id = p.id AND t.deleted_at IS NULL
		WHERE p.id = $1 AND p.deleted_at IS NULL
		GROUP BY p.id`, pid).Scan(&numericID, &summary.TaskCount, &summary.CompletedCount, &summary.OpenCount)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.ProjectSummary{}, ErrNotFound
	}
	if err != nil {
		return domain.ProjectSummary{}, mapDBError(err)
	}
	summary.ProjectID = strconv.FormatInt(numericID, 10)
	return summary, nil
}

func (r *PostgresRepository) GetTask(ctx context.Context, id string) (domain.Task, error) {
	taskID, err := parseID(id)
	if err != nil {
		return domain.Task{}, ErrNotFound
	}
	var t domain.Task
	var numericID, projectID int64
	err = r.pool.QueryRow(ctx, `
		SELECT id, project_id, title, status, created_at
		FROM tasks WHERE id = $1 AND deleted_at IS NULL`, taskID).
		Scan(&numericID, &projectID, &t.Title, &t.Status, &t.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Task{}, ErrNotFound
	}
	if err != nil {
		return domain.Task{}, mapDBError(err)
	}
	t.ID = strconv.FormatInt(numericID, 10)
	t.ProjectID = strconv.FormatInt(projectID, 10)
	return t, nil
}

func (r *PostgresRepository) UpdateTask(ctx context.Context, t domain.Task) (domain.Task, error) {
	id, err := parseID(t.ID)
	if err != nil {
		return domain.Task{}, ErrNotFound
	}
	command, err := r.pool.Exec(ctx, `
		UPDATE tasks SET title = $2, status = $3, updated_at = now()
		WHERE id = $1 AND deleted_at IS NULL`, id, t.Title, t.Status)
	if err != nil {
		return domain.Task{}, mapDBError(err)
	}
	if command.RowsAffected() == 0 {
		return domain.Task{}, ErrNotFound
	}
	return t, nil
}

func parseID(raw string) (int64, error) {
	id, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
	if err != nil || id < 1 {
		return 0, fmt.Errorf("invalid numeric id")
	}
	return id, nil
}

func mapDBError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "23505":
			return fmt.Errorf("%w: %v", ErrConflict, err)
		case "23503", "23514", "23502", "22P02":
			return fmt.Errorf("%w: %v", ErrConstraint, err)
		}
	}
	return fmt.Errorf("database operation: %w", err)
}
