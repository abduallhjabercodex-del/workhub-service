package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"

	"workhub-service/internal/domain"
	"workhub-service/internal/infrastructure/bulkhead"
	"workhub-service/internal/infrastructure/metrics"
	"workhub-service/internal/infrastructure/telemetry"
	"workhub-service/internal/middleware"

	"github.com/jackc/pgx/v5"
)

// CreateTask — ينشئ مهمة بسيطة بدون Quota أو Audit.
func (r *PostgresRepository) CreateTask(ctx context.Context, t domain.Task) (domain.Task, error) {
	projectID, err := parseID(t.ProjectID)
	if err != nil {
		return domain.Task{}, ErrInvalidProjectID
	}
	createdBy, err := parseID(t.CreatedBy)
	if err != nil {
		return domain.Task{}, ErrInvalidUserID
	}
	if t.CreatedAt.IsZero() {
		t.CreatedAt = time.Now().UTC()
	}
	var assigneeParam *int64
	if t.AssigneeID != nil && *t.AssigneeID != "" {
		if parsed, err := parseID(*t.AssigneeID); err == nil {
			assigneeParam = &parsed
		}
	}
	var id int64
	stop := metrics.ObserveDBQuery("INSERT", "tasks")
	err = r.pool.QueryRow(ctx, `
		INSERT INTO tasks (project_id, created_by, assignee_id, title, status, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $6)
		RETURNING id`, projectID, createdBy, assigneeParam, t.Title, t.Status, t.CreatedAt).Scan(&id)
	stop(&err)
	if err != nil {
		return domain.Task{}, mapDBError(err)
	}
	t.ID = strconv.FormatInt(id, 10)
	return t, nil
}

// CreateTaskWithQuota — إنشاء ذرّي مع Quota + Audit + Outbox.
func (r *PostgresRepository) CreateTaskWithQuota(
	ctx context.Context,
	t domain.Task,
	audit domain.AuditEvent,
	maxTasks int,
) (domain.Task, error) {
	ctx, span := telemetry.Tracer().Start(ctx, "PostgresRepository.CreateTaskWithQuota")
	defer span.End()

	projectID, err := parseID(t.ProjectID)
	if err != nil {
		return domain.Task{}, ErrInvalidProjectID
	}
	createdBy, err := parseID(t.CreatedBy)
	if err != nil {
		return domain.Task{}, ErrInvalidUserID
	}
	if t.CreatedAt.IsZero() {
		t.CreatedAt = time.Now().UTC()
	}
	if audit.CreatedAt.IsZero() {
		audit.CreatedAt = t.CreatedAt
	}

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return domain.Task{}, mapDBError(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	stopLock := metrics.ObserveDBQuery("SELECT_LOCK", "tasks")
	_, errLock := tx.Exec(ctx,
		`SELECT pg_advisory_xact_lock(hashtext('quota:tasks')::int, hashtext($1::text)::int)`,
		strconv.FormatInt(projectID, 10),
	)
	stopLock(&errLock)
	if errLock != nil {
		return domain.Task{}, mapDBError(errLock)
	}

	var count int
	stopCount := metrics.ObserveDBQuery("SELECT_COUNT", "tasks")
	errCount := tx.QueryRow(ctx,
		`SELECT count(*) FROM tasks WHERE project_id = $1 AND deleted_at IS NULL`,
		projectID,
	).Scan(&count)
	stopCount(&errCount)
	if errCount != nil {
		return domain.Task{}, mapDBError(errCount)
	}

	if maxTasks >= 0 && count >= maxTasks {
		return domain.Task{}, &domain.QuotaError{
			Resource: "tasks", Current: count, Max: maxTasks,
		}
	}

	var assigneeParam *int64
	if t.AssigneeID != nil && *t.AssigneeID != "" {
		if parsed, err := parseID(*t.AssigneeID); err == nil {
			assigneeParam = &parsed
		}
	}

	var id int64
	stopInsert := metrics.ObserveDBQuery("INSERT", "tasks")
	errInsert := tx.QueryRow(ctx, `
		INSERT INTO tasks (project_id, created_by, assignee_id, title, status, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $6)
		RETURNING id`, projectID, createdBy, assigneeParam, t.Title, t.Status, t.CreatedAt).Scan(&id)
	stopInsert(&errInsert)
	if errInsert != nil {
		return domain.Task{}, mapDBError(errInsert)
	}
	t.ID = strconv.FormatInt(id, 10)

	var actor any
	if audit.ActorUserID != "" {
		actorID, parseErr := parseID(audit.ActorUserID)
		if parseErr != nil {
			return domain.Task{}, ErrInvalidUserID
		}
		actor = actorID
	}

	stopAudit := metrics.ObserveDBQuery("INSERT", "audit_events")
	_, errAudit := tx.Exec(ctx, `
		INSERT INTO audit_events (actor_user_id, project_id, task_id, event_type, payload, created_at)
		VALUES ($1, $2, $3, $4, $5::jsonb, $6)`,
		actor, projectID, id, audit.EventType, audit.Payload, audit.CreatedAt,
	)
	stopAudit(&errAudit)
	if errAudit != nil {
		return domain.Task{}, mapDBError(errAudit)
	}

	payload, marshalErr := json.Marshal(map[string]any{
		"taskId":        t.ID,
		"projectId":     t.ProjectID,
		"title":         t.Title,
		"status":        t.Status,
		"correlationId": middleware.RequestIDFromContext(ctx),
	})
	if marshalErr != nil {
		return domain.Task{}, marshalErr
	}

	eventID := fmt.Sprintf("task-created-%s-%d", t.ID, t.CreatedAt.UnixNano())
	stopOutbox := metrics.ObserveDBQuery("INSERT", "outbox_events")
	_, errOutbox := tx.Exec(ctx, `
		INSERT INTO outbox_events (event_id, event_type, aggregate_type, aggregate_id, payload, created_at)
		VALUES ($1, $2, $3, $4, $5::jsonb, $6)`,
		eventID, "task.created", "task", id, payload, t.CreatedAt,
	)
	stopOutbox(&errOutbox)
	if errOutbox != nil {
		return domain.Task{}, mapDBError(errOutbox)
	}

	if err := tx.Commit(ctx); err != nil {
		return domain.Task{}, mapDBError(err)
	}
	return t, nil
}

func (r *PostgresRepository) CountProjectTasks(ctx context.Context, projectIDStr string) (int, error) {
	projectID, err := parseID(projectIDStr)
	if err != nil {
		return 0, ErrInvalidProjectID
	}
	var count int
	stop := metrics.ObserveDBQuery("SELECT_COUNT", "tasks")
	err = r.pool.QueryRow(ctx,
		`SELECT count(*) FROM tasks WHERE project_id = $1 AND deleted_at IS NULL`,
		projectID,
	).Scan(&count)
	stop(&err)
	if err != nil {
		return 0, mapDBError(err)
	}
	return count, nil
}

// ListTasks — يستخدم Bulkhead لحماية DB Pool. projectID إلزامي.
func (r *PostgresRepository) ListTasks(
	ctx context.Context,
	projectID, status, q, sortBy string,
	page, limit int,
) ([]domain.Task, int, error) {
	pid, err := parseID(projectID)
	if err != nil {
		return nil, 0, ErrInvalidProjectID
	}

	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 100 {
		limit = 20
	}
	offset := (page - 1) * limit

	orderCol, ok := allowedTaskSort[sortBy]
	if sortBy == "" {
		orderCol, ok = "created_at", true
	}
	if !ok {
		return nil, 0, ErrInvalidSort
	}
	orderDir := "DESC"
	if sortBy == "title" {
		orderDir = "ASC"
	}
	order := fmt.Sprintf("%s %s, id ASC", orderCol, orderDir)
	search := escapeLike(q)

	var total int
	var items []domain.Task

	bulkErr := r.listTaskBulkhead.Do(ctx, func(innerCtx context.Context) error {
		stopCount := metrics.ObserveDBQuery("SELECT_COUNT", "tasks")
		errCount := r.pool.QueryRow(innerCtx, `
			SELECT count(*) FROM tasks
			WHERE project_id = $1
			  AND deleted_at IS NULL
			  AND ($2 = '' OR status = $2)
			  AND ($3 = '' OR title ILIKE '%' || $3 || '%' ESCAPE '\')`,
			pid, status, search,
		).Scan(&total)
		stopCount(&errCount)
		if errCount != nil {
			return errCount
		}

		stopList := metrics.ObserveDBQuery("SELECT", "tasks")
		rows, err := r.pool.Query(innerCtx, fmt.Sprintf(`
			SELECT id, project_id, created_by, assignee_id, title, status, created_at
			FROM tasks
			WHERE project_id = $1
			  AND deleted_at IS NULL
			  AND ($2 = '' OR status = $2)
			  AND ($3 = '' OR title ILIKE '%%' || $3 || '%%' ESCAPE '\')
			ORDER BY %s
			LIMIT $4 OFFSET $5`, order),
			pid, status, search, limit, offset,
		)
		stopList(&err)
		if err != nil {
			return err
		}
		defer rows.Close()

		items = make([]domain.Task, 0, limit)
		for rows.Next() {
			var t domain.Task
			var id, rowProjectID, createdBy int64
			var assigneeID *int64
			if err := rows.Scan(&id, &rowProjectID, &createdBy, &assigneeID, &t.Title, &t.Status, &t.CreatedAt); err != nil {
				return err
			}
			t.ID = strconv.FormatInt(id, 10)
			t.ProjectID = strconv.FormatInt(rowProjectID, 10)
			t.CreatedBy = strconv.FormatInt(createdBy, 10)
			if assigneeID != nil {
				assigneeStr := strconv.FormatInt(*assigneeID, 10)
				t.AssigneeID = &assigneeStr
			}
			items = append(items, t)
		}
		return rows.Err()
	})

	if bulkErr != nil {
		if errors.Is(bulkErr, bulkhead.ErrBulkheadFull) {
			return nil, 0, fmt.Errorf("system busy: %w", bulkErr)
		}
		return nil, 0, mapDBError(bulkErr)
	}

	return items, total, nil
}

func (r *PostgresRepository) GetTask(ctx context.Context, id string) (domain.Task, error) {
	taskID, err := parseID(id)
	if err != nil {
		return domain.Task{}, ErrNotFound
	}
	var t domain.Task
	var numericID, projectID, createdBy int64
	var assigneeID *int64
	stop := metrics.ObserveDBQuery("SELECT", "tasks")
	err = r.pool.QueryRow(ctx, `
		SELECT id, project_id, created_by, assignee_id, title, status, created_at
		FROM tasks WHERE id = $1 AND deleted_at IS NULL`, taskID).
		Scan(&numericID, &projectID, &createdBy, &assigneeID, &t.Title, &t.Status, &t.CreatedAt)
	stop(&err)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Task{}, ErrNotFound
	}
	if err != nil {
		return domain.Task{}, mapDBError(err)
	}
	t.ID = strconv.FormatInt(numericID, 10)
	t.ProjectID = strconv.FormatInt(projectID, 10)
	t.CreatedBy = strconv.FormatInt(createdBy, 10)
	if assigneeID != nil {
		assigneeStr := strconv.FormatInt(*assigneeID, 10)
		t.AssigneeID = &assigneeStr
	}
	return t, nil
}

func (r *PostgresRepository) UpdateTask(ctx context.Context, t domain.Task) (domain.Task, error) {
	id, err := parseID(t.ID)
	if err != nil {
		return domain.Task{}, ErrNotFound
	}
	var assigneeParam *int64
	if t.AssigneeID != nil && *t.AssigneeID != "" {
		if parsed, err := parseID(*t.AssigneeID); err == nil {
			assigneeParam = &parsed
		}
	}
	stop := metrics.ObserveDBQuery("UPDATE", "tasks")
	command, err := r.pool.Exec(ctx, `
		UPDATE tasks SET title = $2, status = $3, assignee_id = $4, updated_at = now()
		WHERE id = $1 AND deleted_at IS NULL`, id, t.Title, t.Status, assigneeParam)
	stop(&err)
	if err != nil {
		return domain.Task{}, mapDBError(err)
	}
	if command.RowsAffected() == 0 {
		return domain.Task{}, ErrNotFound
	}
	return t, nil
}

func (r *PostgresRepository) DeleteTask(ctx context.Context, id string) error {
	taskID, err := parseID(id)
	if err != nil {
		return ErrNotFound
	}
	stop := metrics.ObserveDBQuery("UPDATE", "tasks")
	command, err := r.pool.Exec(ctx, `
		UPDATE tasks SET deleted_at = now(), updated_at = now()
		WHERE id = $1 AND deleted_at IS NULL`, taskID)
	stop(&err)
	if err != nil {
		return mapDBError(err)
	}
	if command.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}
