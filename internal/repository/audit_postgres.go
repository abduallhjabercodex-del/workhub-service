package repository

import (
	"context"
	"encoding/json"
	"strconv"
	"time"

	"workhub-service/internal/domain"
	"workhub-service/internal/infrastructure/metrics"
)

func (r *PostgresRepository) ListProjectAuditEvents(
	ctx context.Context,
	projectID string,
	limit, offset int,
) ([]domain.AuditEvent, int, error) {
	pid, err := parseID(projectID)
	if err != nil {
		return nil, 0, ErrInvalidProjectID
	}
	if limit < 1 || limit > 100 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}

	var total int
	stopCount := metrics.ObserveDBQuery("SELECT_COUNT", "audit_events")
	errCount := r.pool.QueryRow(ctx,
		`SELECT count(*) FROM audit_events WHERE project_id = $1`, pid,
	).Scan(&total)
	stopCount(&errCount)
	if errCount != nil {
		return nil, 0, mapDBError(errCount)
	}

	stopList := metrics.ObserveDBQuery("SELECT", "audit_events")
	rows, err := r.pool.Query(ctx, `
		SELECT a.id, COALESCE(a.actor_user_id, 0), COALESCE(u.display_name, 'System'), COALESCE(u.email, ''),
		       a.project_id, a.task_id, a.event_type, a.payload, a.created_at
		FROM audit_events a
		LEFT JOIN users u ON u.id = a.actor_user_id
		WHERE a.project_id = $1
		ORDER BY a.created_at DESC, a.id DESC
		LIMIT $2 OFFSET $3`,
		pid, limit, offset,
	)
	stopList(&err)
	if err != nil {
		return nil, 0, mapDBError(err)
	}
	defer rows.Close()

	events := make([]domain.AuditEvent, 0, limit)
	for rows.Next() {
		var a domain.AuditEvent
		var id, actorID, projID int64
		var taskID *int64
		var rawPayload []byte
		if err := rows.Scan(
			&id, &actorID, &a.ActorDisplayName, &a.ActorEmail,
			&projID, &taskID, &a.EventType, &rawPayload, &a.CreatedAt,
		); err != nil {
			return nil, 0, mapDBError(err)
		}
		a.ID = strconv.FormatInt(id, 10)
		if actorID > 0 {
			a.ActorUserID = strconv.FormatInt(actorID, 10)
		}
		a.ProjectID = strconv.FormatInt(projID, 10)
		if taskID != nil {
			a.TaskID = strconv.FormatInt(*taskID, 10)
		}
		if len(rawPayload) > 0 {
			a.Payload = json.RawMessage(rawPayload)
		} else {
			a.Payload = json.RawMessage(`{}`)
		}
		events = append(events, a)
	}
	return events, total, mapDBError(rows.Err())
}

func (r *PostgresRepository) RecordAuditEvent(ctx context.Context, event domain.AuditEvent) error {
	pID, err := parseID(event.ProjectID)
	if err != nil {
		return ErrInvalidProjectID
	}
	var actorID *int64
	if event.ActorUserID != "" {
		if parsed, err := parseID(event.ActorUserID); err == nil {
			actorID = &parsed
		}
	}
	var taskID *int64
	if event.TaskID != "" {
		if parsed, err := parseID(event.TaskID); err == nil {
			taskID = &parsed
		}
	}
	payloadBytes := event.Payload
	if len(payloadBytes) == 0 {
		payloadBytes = []byte(`{}`)
	}
	createdAt := event.CreatedAt
	if createdAt.IsZero() {
		createdAt = time.Now().UTC()
	}

	stop := metrics.ObserveDBQuery("INSERT", "audit_events")
	_, err = r.pool.Exec(ctx, `
		INSERT INTO audit_events (actor_user_id, project_id, task_id, event_type, payload, created_at)
		VALUES ($1, $2, $3, $4, $5::jsonb, $6)`,
		actorID, pID, taskID, event.EventType, payloadBytes, createdAt,
	)
	stop(&err)
	if err != nil {
		return mapDBError(err)
	}
	return nil
}
