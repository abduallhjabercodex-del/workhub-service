package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"workhub-service/internal/domain"
	"workhub-service/internal/repository"
)

var (
	ErrInvalidInput    = errors.New("invalid input")
	ErrProjectNotFound = errors.New("project not found")
	ErrTaskNotFound    = errors.New("task not found")
	ErrStorage         = errors.New("storage error")
)

type SummaryCache interface {
	Get(context.Context, string) (domain.ProjectSummary, bool)
	Set(context.Context, domain.ProjectSummary)
	Invalidate(context.Context, string)
}

type EventPublisher interface {
	Publish(context.Context, domain.Event) error
}

type WorkHub struct {
	projects  repository.ProjectRepository
	tasks     repository.TaskRepository
	summaries repository.SummaryRepository
	cache     SummaryCache
	publisher EventPublisher
	now       func() time.Time
	ids       func() string
}

func NewWorkHub(projects repository.ProjectRepository, tasks repository.TaskRepository) *WorkHub {
	workhub := &WorkHub{projects: projects, tasks: tasks, now: time.Now, ids: func() string { return fmt.Sprintf("%d", time.Now().UnixNano()) }}
	// Type Assertion
	//
	if summaries, ok := projects.(repository.SummaryRepository); ok {
		workhub.summaries = summaries
	}
	return workhub
}

// خادم الكاش (Redis) في نظامنا هو عنصر اختياري (Optional Layer) وليس عنصراً إجبارياً لقيام الخدمة!
// إذا لم يتم تزويد الكاش عند بناء الـ WorkHub، فستظل الخدمة تعمل بشكل صحيح، لكنها ستعتمد فقط على قاعدة البيانات.
func (s *WorkHub) SetSummaryCache(cache SummaryCache)         { s.cache = cache }
func (s *WorkHub) SetEventPublisher(publisher EventPublisher) { s.publisher = publisher }

func (s *WorkHub) CreateProject(ctx context.Context, in domain.CreateProjectInput) (domain.Project, error) {
	name := strings.TrimSpace(in.Name)
	if name == "" || len(name) > 120 {
		return domain.Project{}, ErrInvalidInput
	}
	return s.projects.CreateProject(ctx, domain.Project{ID: s.ids(), Name: name, CreatedAt: s.now().UTC()})
}

func (s *WorkHub) ListProjects(ctx context.Context, q, sortBy string) ([]domain.Project, error) {
	items, err := s.projects.ListProjects(ctx, q, sortBy)
	if err != nil {
		return nil, fmt.Errorf("list projects: %w", err)
	}
	return items, nil
}

func (s *WorkHub) ProjectSummary(ctx context.Context, projectID string) (domain.ProjectSummary, error) {
	if s.summaries == nil {
		return domain.ProjectSummary{}, ErrStorage
	}
	if s.cache != nil {
		if summary, ok := s.cache.Get(ctx, projectID); ok {
			return summary, nil
		}
	}
	summary, err := s.summaries.GetProjectSummary(ctx, projectID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return domain.ProjectSummary{}, ErrProjectNotFound
		}
		return domain.ProjectSummary{}, fmt.Errorf("get project summary: %w", err)
	}
	if s.cache != nil {
		s.cache.Set(ctx, summary)
	}
	return summary, nil
}

func (s *WorkHub) invalidateSummary(ctx context.Context, projectID string) {
	if s.cache != nil {
		s.cache.Invalidate(ctx, projectID)
	}
}

func (s *WorkHub) CreateTask(ctx context.Context, projectID string, in domain.CreateTaskInput) (domain.Task, error) {
	if _, err := s.projects.GetProject(ctx, projectID); err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return domain.Task{}, ErrProjectNotFound
		}
		return domain.Task{}, fmt.Errorf("get project: %w", err)
	}
	title := strings.TrimSpace(in.Title)
	if title == "" || len(title) > 200 {
		return domain.Task{}, ErrInvalidInput
	}
	task := domain.Task{ID: s.ids(), ProjectID: projectID, Title: title, Status: "todo", CreatedAt: s.now().UTC()}
	audit := domain.AuditEvent{ProjectID: projectID, TaskID: task.ID, EventType: "task.created", Payload: []byte(`{"source":"workhub"}`), CreatedAt: task.CreatedAt}
	var result domain.Task
	var err error
	if txRepo, ok := s.tasks.(repository.TaskAuditRepository); ok {
		result, err = txRepo.CreateTaskWithAudit(ctx, task, audit)
	} else {
		result, err = s.tasks.CreateTask(ctx, task)
	}
	if err == nil {
		s.invalidateSummary(ctx, projectID)
		if s.publisher != nil {
			event := domain.Event{ID: fmt.Sprintf("task-created-%s-%d", result.ID, result.CreatedAt.UnixNano()), Type: "task.created", AggregateType: "task", AggregateID: result.ID, ProjectID: result.ProjectID, TaskID: result.ID, Payload: []byte(fmt.Sprintf(`{"taskId":"%s","projectId":"%s","title":%q,"status":%q}`, result.ID, result.ProjectID, result.Title, result.Status)), OccurredAt: result.CreatedAt}
			_ = s.publisher.Publish(ctx, event)
		}
	}
	return result, err
}

func (s *WorkHub) GetTask(ctx context.Context, id string) (domain.Task, error) {
	t, err := s.tasks.GetTask(ctx, id)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return domain.Task{}, ErrTaskNotFound
		}
		return domain.Task{}, fmt.Errorf("get task: %w", err)
	}
	return t, nil
}

func (s *WorkHub) ListTasks(ctx context.Context, projectID, status, q, sortBy string, page, limit int) ([]domain.Task, int, error) {
	if _, err := s.projects.GetProject(ctx, projectID); err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, 0, ErrProjectNotFound
		}
		return nil, 0, fmt.Errorf("get project: %w", err)
	}
	items, total, err := s.tasks.ListTasks(ctx, projectID, status, q, sortBy, page, limit)
	if err != nil {
		return nil, 0, fmt.Errorf("list tasks: %w", err)
	}
	return items, total, nil
}

func (s *WorkHub) UpdateTask(ctx context.Context, id string, in domain.UpdateTaskInput) (domain.Task, error) {
	t, err := s.tasks.GetTask(ctx, id)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return domain.Task{}, ErrTaskNotFound
		}
		return domain.Task{}, fmt.Errorf("get task: %w", err)
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
		if status != "todo" && status != "in_progress" && status != "done" {
			return domain.Task{}, ErrInvalidInput
		}
		t.Status = status
	}
	result, err := s.tasks.UpdateTask(ctx, t)
	if err == nil {
		s.invalidateSummary(ctx, t.ProjectID)
	}
	return result, err
}
