package domain

import "time"

type Project struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"createdAt"`
}

type Task struct {
	ID        string    `json:"id"`
	ProjectID string    `json:"projectId"`
	Title     string    `json:"title"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"createdAt"`
}

type ProjectSummary struct {
	ProjectID      string `json:"projectId"`
	TaskCount      int    `json:"taskCount"`
	CompletedCount int    `json:"completedCount"`
	OpenCount      int    `json:"openCount"`
}

type Event struct {
	ID            string    `json:"id"`
	Type          string    `json:"type"`
	AggregateType string    `json:"aggregateType"`
	AggregateID   string    `json:"aggregateId"`
	ProjectID     string    `json:"projectId"`
	TaskID        string    `json:"taskId"`
	Payload       []byte    `json:"payload"`
	OccurredAt    time.Time `json:"occurredAt"`
}

type AuditEvent struct {
	ActorUserID string
	ProjectID   string
	TaskID      string
	EventType   string
	Payload     []byte
	CreatedAt   time.Time
}

type CreateProjectInput struct {
	Name string `json:"name"`
}
type CreateTaskInput struct {
	Title string `json:"title"`
}
type UpdateTaskInput struct {
	Title  *string `json:"title"`
	Status *string `json:"status"`
}
