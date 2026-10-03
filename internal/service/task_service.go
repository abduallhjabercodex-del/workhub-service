package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"workhub-service/internal/domain"
	"workhub-service/internal/infrastructure/telemetry"
	"workhub-service/internal/middleware"
)

func (s *WorkHub) CreateTask(ctx context.Context, actorUserID, projectID string, in domain.CreateTaskInput) (domain.Task, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, span := telemetry.Tracer().Start(ctx, "WorkHub.CreateTask")
	defer span.End()

	if strings.TrimSpace(projectID) == "" {
		return domain.Task{}, ErrProjectNotFound
	}
	if _, err := s.CheckAccess(ctx, actorUserID, projectID, domain.RoleMember); err != nil {
		return domain.Task{}, err
	}
	proj, err := s.projects.GetProject(ctx, projectID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return domain.Task{}, ErrProjectNotFound
		}
		return domain.Task{}, fmt.Errorf("get project: %w", err)
	}
	title := strings.TrimSpace(in.Title)
	if title == "" || len(title) > 200 {
		return domain.Task{}, ErrInvalidInput
	}

	task := domain.Task{
		ProjectID:  projectID,
		CreatedBy:  actorUserID,
		AssigneeID: in.AssigneeID,
		Title:      title,
		Status:     "todo",
		CreatedAt:  s.now().UTC(),
	}

	payloadMap := map[string]interface{}{
		"title": title,
	}
	if in.AssigneeID != nil {
		payloadMap["assignee_id"] = *in.AssigneeID
	}
	rawPayload, _ := json.Marshal(payloadMap)

	audit := domain.AuditEvent{
		ActorUserID: actorUserID,
		ProjectID:   projectID,
		EventType:   "task.created",
		Payload:     rawPayload,
		CreatedAt:   task.CreatedAt,
	}

	maxTasks := s.GetEffectiveMaxTasks(ctx, proj.OwnerID)
	result, err := s.tasks.CreateTaskWithQuota(ctx, task, audit, maxTasks)
	if err != nil {
		if errors.Is(err, domain.ErrQuotaExceeded) {
			slog.WarnContext(ctx, "quota exceeded for tasks",
				"project_id", projectID, "max", maxTasks)
		}
		return domain.Task{}, err
	}

	slog.InfoContext(ctx, "task created",
		"task_id", result.ID, "project_id", projectID, "actor_user_id", actorUserID)
	s.invalidateSummary(ctx, projectID)

	if s.publisher != nil {
		event := domain.Event{
			ID:            fmt.Sprintf("task-created-%s", result.ID),
			Type:          "task.created",
			AggregateType: "task",
			AggregateID:   result.ID,
			ProjectID:     result.ProjectID,
			TaskID:        result.ID,
			OccurredAt:    result.CreatedAt,
			CorrelationID: middleware.RequestIDFromContext(ctx),
		}
		if pubErr := s.publisher.Publish(ctx, event); pubErr != nil {
			slog.ErrorContext(ctx, "publish task.created failed",
				"error", pubErr, "task_id", result.ID)
		}
	}
	return result, nil
}

func (s *WorkHub) GetTask(ctx context.Context, actorUserID, id string) (domain.Task, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, span := telemetry.Tracer().Start(ctx, "WorkHub.GetTask")
	defer span.End()

	if strings.TrimSpace(id) == "" {
		return domain.Task{}, ErrTaskNotFound
	}
	t, err := s.tasks.GetTask(ctx, id)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return domain.Task{}, ErrTaskNotFound
		}
		return domain.Task{}, fmt.Errorf("get task: %w", err)
	}
	if _, err := s.CheckAccess(ctx, actorUserID, t.ProjectID); err != nil {
		return domain.Task{}, err
	}
	return t, nil
}

// ListTasks — IDOR fix: projectID is required
func (s *WorkHub) ListTasks(
	ctx context.Context,
	actorUserID, projectID, status, q, sortBy string,
	page, limit int,
) ([]domain.Task, int, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, span := telemetry.Tracer().Start(ctx, "WorkHub.ListTasks")
	defer span.End()

	if strings.TrimSpace(projectID) == "" {
		return nil, 0, ErrInvalidInput
	}
	if page < 0 || limit < 0 {
		return nil, 0, ErrInvalidInput
	}
	if _, err := s.CheckAccess(ctx, actorUserID, projectID); err != nil {
		return nil, 0, err
	}
	items, total, err := s.tasks.ListTasks(ctx, projectID, status, q, sortBy, page, limit)
	if err != nil {
		return nil, 0, fmt.Errorf("list tasks: %w", err)
	}
	return items, total, nil
}

func (s *WorkHub) UpdateTask(ctx context.Context, actorUserID, id string, in domain.UpdateTaskInput) (domain.Task, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, span := telemetry.Tracer().Start(ctx, "WorkHub.UpdateTask")
	defer span.End()

	if strings.TrimSpace(id) == "" {
		return domain.Task{}, ErrTaskNotFound
	}
	t, err := s.tasks.GetTask(ctx, id)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return domain.Task{}, ErrTaskNotFound
		}
		return domain.Task{}, fmt.Errorf("get task: %w", err)
	}
	if _, err := s.CheckAccess(ctx, actorUserID, t.ProjectID, domain.RoleMember); err != nil {
		return domain.Task{}, err
	}
	if in.Title != nil {
		title := strings.TrimSpace(*in.Title)
		if title == "" || len(title) > 200 {
			return domain.Task{}, ErrInvalidInput
		}
		t.Title = title
	}
	if in.Status != nil {
		status := strings.TrimSpace(*in.Status)
		switch status {
		case "todo", "in_progress", "done":
			t.Status = status
		default:
			return domain.Task{}, ErrInvalidInput
		}
	}
	if in.AssigneeID != nil {
		t.AssigneeID = in.AssigneeID
	}
	result, err := s.tasks.UpdateTask(ctx, t)
	if err != nil {
		return domain.Task{}, err
	}
	s.invalidateSummary(ctx, t.ProjectID)

	updatePayloadMap := map[string]interface{}{
		"title":  result.Title,
		"status": result.Status,
	}
	if result.AssigneeID != nil {
		updatePayloadMap["assignee_id"] = *result.AssigneeID
	}
	rawUpdatePayload, _ := json.Marshal(updatePayloadMap)
	s.recordAudit(ctx, domain.AuditEvent{
		ActorUserID: actorUserID,
		ProjectID:   result.ProjectID,
		TaskID:      result.ID,
		EventType:   "task.updated",
		Payload:     rawUpdatePayload,
		CreatedAt:   s.now().UTC(),
	})

	if s.publisher != nil {
		event := domain.Event{
			ID:            fmt.Sprintf("task-updated-%s-%d", result.ID, s.now().UnixNano()),
			Type:          "task.updated",
			AggregateType: "task",
			AggregateID:   result.ID,
			ProjectID:     result.ProjectID,
			TaskID:        result.ID,
			OccurredAt:    s.now().UTC(),
			CorrelationID: middleware.RequestIDFromContext(ctx),
		}
		if pubErr := s.publisher.Publish(ctx, event); pubErr != nil {
			slog.ErrorContext(ctx, "publish task.updated failed",
				"error", pubErr, "task_id", result.ID)
		}
	}
	return result, nil
}

func (s *WorkHub) DeleteTask(ctx context.Context, actorUserID, id string) error {
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, span := telemetry.Tracer().Start(ctx, "WorkHub.DeleteTask")
	defer span.End()

	if strings.TrimSpace(id) == "" {
		return ErrTaskNotFound
	}
	t, err := s.tasks.GetTask(ctx, id)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return ErrTaskNotFound
		}
		return fmt.Errorf("get task: %w", err)
	}
	// Creator can delete their own task with RoleMember access.
	// Deleting other members' tasks requires RoleAdmin.
	requiredRole := domain.RoleAdmin
	if t.CreatedBy == actorUserID {
		requiredRole = domain.RoleMember
	}
	if _, err := s.CheckAccess(ctx, actorUserID, t.ProjectID, requiredRole); err != nil {
		return err
	}
	if err := s.tasks.DeleteTask(ctx, id); err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return ErrTaskNotFound
		}
		return fmt.Errorf("delete task: %w", err)
	}
	s.invalidateSummary(ctx, t.ProjectID)

	delPayload, _ := json.Marshal(map[string]interface{}{
		"title": t.Title,
	})
	s.recordAudit(ctx, domain.AuditEvent{
		ActorUserID: actorUserID,
		ProjectID:   t.ProjectID,
		TaskID:      t.ID,
		EventType:   "task.deleted",
		Payload:     delPayload,
		CreatedAt:   s.now().UTC(),
	})

	if s.publisher != nil {
		event := domain.Event{
			ID:            fmt.Sprintf("task-deleted-%s-%d", t.ID, s.now().UnixNano()),
			Type:          "task.deleted",
			AggregateType: "task",
			AggregateID:   t.ID,
			ProjectID:     t.ProjectID,
			TaskID:        t.ID,
			OccurredAt:    s.now().UTC(),
			CorrelationID: middleware.RequestIDFromContext(ctx),
		}
		if pubErr := s.publisher.Publish(ctx, event); pubErr != nil {
			slog.ErrorContext(ctx, "publish task.deleted failed",
				"error", pubErr, "task_id", t.ID)
		}
	}
	return nil
}
