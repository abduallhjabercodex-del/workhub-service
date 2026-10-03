package domain

import (
	"encoding/json"
	"time"
)

// Core Domain Entities and Aggregates.

type Project struct {
	ID        string    `json:"id"`
	OwnerID   string    `json:"ownerId,omitempty"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"createdAt"`
}

type Task struct {
	ID         string    `json:"id"`
	ProjectID  string    `json:"projectId"`
	CreatedBy  string    `json:"createdBy"`
	AssigneeID *string   `json:"assigneeId,omitempty"`
	Title      string    `json:"title"`
	Status     string    `json:"status"`
	CreatedAt  time.Time `json:"createdAt"`
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
	CorrelationID string    `json:"correlationId,omitempty"`
}

type OutboxRecord struct {
	ID            int64      `json:"id"`
	EventID       string     `json:"eventId"`
	EventType     string     `json:"eventType"`
	AggregateType string     `json:"aggregateType"`
	AggregateID   string     `json:"aggregateId"`
	Payload       []byte     `json:"payload"`
	Attempts      int        `json:"attempts"`
	AvailableAt   time.Time  `json:"availableAt"`
	PublishedAt   *time.Time `json:"publishedAt,omitempty"`
	LastError     *string    `json:"lastError,omitempty"`
	CreatedAt     time.Time  `json:"createdAt"`
}

type AuditEvent struct {
	ID               string          `json:"id"`
	ActorUserID      string          `json:"actorUserId"`
	ActorDisplayName string          `json:"actorDisplayName,omitempty"`
	ActorEmail       string          `json:"actorEmail,omitempty"`
	ProjectID        string          `json:"projectId"`
	TaskID           string          `json:"taskId,omitempty"`
	EventType        string          `json:"eventType"`
	Payload          json.RawMessage `json:"payload"`
	CreatedAt        time.Time       `json:"createdAt"`
}

type User struct {
	ID           string    `json:"id"`
	Email        string    `json:"email"`
	DisplayName  string    `json:"displayName"`
	PasswordHash string    `json:"-"`
	CreatedAt    time.Time `json:"createdAt"`
}

type AuthUser struct {
	ID          string `json:"id"`
	Email       string `json:"email"`
	DisplayName string `json:"displayName"`
}

type RefreshTokenSession struct {
	ID                  string
	TokenHash           string
	UserID              string
	ExpiresAt           time.Time
	RevokedAt           *time.Time
	ReplacedByTokenHash *string
	CreatedAt           time.Time
}

type Member struct {
	ProjectID   string    `json:"projectId"`
	UserID      string    `json:"userId"`
	Email       string    `json:"email,omitempty"`
	DisplayName string    `json:"displayName,omitempty"`
	Role        Role      `json:"role"`
	JoinedAt    time.Time `json:"joinedAt"`
}

type UserQuota struct {
	UserID               string    `json:"userId"`
	MaxProjects          *int      `json:"maxProjects,omitempty"`
	MaxMembersPerProject *int      `json:"maxMembersPerProject,omitempty"`
	MaxTasksPerProject   *int      `json:"maxTasksPerProject,omitempty"`
	SetBy                string    `json:"setBy,omitempty"`
	SetAt                time.Time `json:"setAt"`
	UpdatedAt            time.Time `json:"updatedAt"`
	Reason               string    `json:"reason,omitempty"`
}
