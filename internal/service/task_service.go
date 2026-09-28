package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"workhub-service/internal/domain"
	"workhub-service/internal/infrastructure/telemetry"
	"workhub-service/internal/middleware"
)

func (s *WorkHub) CreateTask(ctx context.Context, actorUserID, projectID string, in domain.CreateTaskInput) (domain.Task, error) {
	ctx, span := telemetry.Tracer().Start(ctx, "WorkHub.CreateTask")
	defer span.End()

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
		ProjectID: projectID,
		CreatedBy: actorUserID, // ← يستخدم الفاعل الفعلي
		Title:     title,
		Status:    "todo",
		CreatedAt: s.now().UTC(),
	}
	audit := domain.AuditEvent{
		ActorUserID: actorUserID, // ← إضافة الفاعل للـ Audit
		ProjectID:   projectID,
		EventType:   "task.created",
		Payload:     []byte(`{"source":"workhub"}`),
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

// ListTasks — إصلاح IDOR: projectID إلزامي
func (s *WorkHub) ListTasks(
	ctx context.Context,
	actorUserID, projectID, status, q, sortBy string,
	page, limit int,
) ([]domain.Task, int, error) {
	if projectID == "" {
		return nil, 0, ErrInvalidInput // ← رفض الفراغ
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
	result, err := s.tasks.UpdateTask(ctx, t)
	if err == nil {
		s.invalidateSummary(ctx, t.ProjectID)
	}
	return result, err
}

func (s *WorkHub) DeleteTask(ctx context.Context, actorUserID, id string) error {
	t, err := s.tasks.GetTask(ctx, id)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return ErrTaskNotFound
		}
		return fmt.Errorf("get task: %w", err)
	}
	if _, err := s.CheckAccess(ctx, actorUserID, t.ProjectID, domain.RoleAdmin); err != nil {
		return err
	}
	if err := s.tasks.DeleteTask(ctx, id); err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return ErrTaskNotFound
		}
		return fmt.Errorf("delete task: %w", err)
	}
	s.invalidateSummary(ctx, t.ProjectID)
	return nil
}
