package domain

import "time"

type Project struct {
	ID        string    `json:"id"`
	OwnerID   string    `json:"ownerId,omitempty"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"createdAt"`
}

type Task struct {
	ID        string    `json:"id"`
	ProjectID string    `json:"projectId"`
	CreatedBy string    `json:"createdBy"` // ← جديد
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
	CorrelationID string    `json:"correlationId,omitempty"`
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

type RegisterInput struct {
	Email       string `json:"email"`
	DisplayName string `json:"displayName"`
	Password    string `json:"password"`
}

type LoginInput struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type RefreshInput struct {
	RefreshToken string `json:"refreshToken"`
}

type LogoutInput struct {
	RefreshToken string `json:"refreshToken"`
}

type AuthResponse struct {
	AccessToken  string   `json:"accessToken"`
	RefreshToken string   `json:"refreshToken"`
	User         AuthUser `json:"user"`
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

type AddMemberInput struct {
	UserID string `json:"userId"`
	Role   Role   `json:"role"`
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
