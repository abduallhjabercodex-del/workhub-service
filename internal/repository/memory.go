package repository

import (
	"context"
	"errors"
	"sort"
	"sync"

	"workhub-service/internal/domain"
)

var ErrNotFound = errors.New("not found")

// ProjectRepository is the persistence boundary for project data.
type ProjectRepository interface {
	CreateProject(context.Context, domain.Project) (domain.Project, error)
	ListProjects(context.Context, string, string) ([]domain.Project, error)
	GetProject(context.Context, string) (domain.Project, error)
}

// TaskRepository is the persistence boundary for task data.
type TaskRepository interface {
	CreateTask(context.Context, domain.Task) (domain.Task, error)
	ListTasks(context.Context, string, string, string, string, int, int) ([]domain.Task, int, error)
	GetTask(context.Context, string) (domain.Task, error)
	UpdateTask(context.Context, domain.Task) (domain.Task, error)
}

type SummaryRepository interface {
	GetProjectSummary(context.Context, string) (domain.ProjectSummary, error)
}

type MemoryRepository struct {
	mu       sync.RWMutex
	projects map[string]domain.Project
	tasks    map[string]domain.Task
}

func NewMemoryRepository() *MemoryRepository {
	return &MemoryRepository{projects: map[string]domain.Project{}, tasks: map[string]domain.Task{}}
}

func (r *MemoryRepository) CreateProject(_ context.Context, p domain.Project) (domain.Project, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.projects[p.ID] = p
	return p, nil
}

func (r *MemoryRepository) ListProjects(_ context.Context, q, sortBy string) ([]domain.Project, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	items := make([]domain.Project, 0, len(r.projects))
	for _, p := range r.projects {
		if q == "" || containsFold(p.Name, q) {
			items = append(items, p)
		}
	}
	sort.Slice(items, func(i, j int) bool {
		if sortBy == "name" {
			return items[i].Name < items[j].Name
		}
		return items[i].CreatedAt.Before(items[j].CreatedAt)
	})
	return items, nil
}

func (r *MemoryRepository) GetProject(_ context.Context, id string) (domain.Project, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	p, ok := r.projects[id]
	if !ok {
		return domain.Project{}, ErrNotFound
	}
	return p, nil
}

func (r *MemoryRepository) CreateTask(_ context.Context, t domain.Task) (domain.Task, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.tasks[t.ID] = t
	return t, nil
}

func (r *MemoryRepository) ListTasks(_ context.Context, projectID, status, q, sortBy string, page, limit int) ([]domain.Task, int, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	items := make([]domain.Task, 0)
	for _, t := range r.tasks {
		if t.ProjectID != projectID || (status != "" && t.Status != status) || (q != "" && !containsFold(t.Title, q)) {
			continue
		}
		items = append(items, t)
	}
	sort.Slice(items, func(i, j int) bool {
		if sortBy == "title" {
			return items[i].Title < items[j].Title
		}
		return items[i].CreatedAt.Before(items[j].CreatedAt)
	})
	total := len(items)
	if page < 1 {
		page = 1
	}
	if limit < 1 {
		limit = 20
	}
	start := (page - 1) * limit
	if start >= total {
		return []domain.Task{}, total, nil
	}
	end := start + limit
	if end > total {
		end = total
	}
	return items[start:end], total, nil
}

func (r *MemoryRepository) GetProjectSummary(_ context.Context, projectID string) (domain.ProjectSummary, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if _, ok := r.projects[projectID]; !ok {
		return domain.ProjectSummary{}, ErrNotFound
	}
	var summary domain.ProjectSummary
	summary.ProjectID = projectID
	for _, task := range r.tasks {
		if task.ProjectID != projectID {
			continue
		}
		summary.TaskCount++
		if task.Status == "done" {
			summary.CompletedCount++
		} else {
			summary.OpenCount++
		}
	}
	return summary, nil
}

func (r *MemoryRepository) GetTask(_ context.Context, id string) (domain.Task, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	t, ok := r.tasks[id]
	if !ok {
		return domain.Task{}, ErrNotFound
	}
	return t, nil
}

func (r *MemoryRepository) UpdateTask(_ context.Context, t domain.Task) (domain.Task, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.tasks[t.ID]; !ok {
		return domain.Task{}, ErrNotFound
	}
	r.tasks[t.ID] = t
	return t, nil
}

func containsFold(value, query string) bool {
	return len(query) == 0 || len(value) >= len(query) && indexFold(value, query)
}

func indexFold(value, query string) bool {
	for i := 0; i+len(query) <= len(value); i++ {
		match := true
		for j := range query {
			if lower(value[i+j]) != lower(query[j]) {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}

func lower(b byte) byte {
	if b >= 'A' && b <= 'Z' {
		return b + ('a' - 'A')
	}
	return b
}
