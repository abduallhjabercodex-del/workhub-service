package domain

import (
	"context"
	"errors"
)

var (
	ErrNotFound      = errors.New("not found")
	ErrAlreadyExists = errors.New("already exists")
	ErrTokenReplay   = errors.New("token replay detected")
	ErrLastOwner     = errors.New("cannot remove last owner")
)

type ProjectRepository interface {
	CreateProject(context.Context, Project) (Project, error)
	CreateProjectWithQuota(ctx context.Context, p Project, maxProjects int) (Project, error)
	CountUserProjects(ctx context.Context, ownerID string) (int, error)
	ListProjects(ctx context.Context, userID, q, sortBy string, limit, offset int) ([]Project, error)
	GetProject(context.Context, string) (Project, error)
	DeleteProject(context.Context, string) error
}

type TaskRepository interface {
	CreateTask(context.Context, Task) (Task, error)
	CreateTaskWithQuota(ctx context.Context, t Task, audit AuditEvent, maxTasks int) (Task, error)
	CountProjectTasks(ctx context.Context, projectID string) (int, error)
	ListTasks(ctx context.Context, projectID, status, q, sortBy string, page, limit int) ([]Task, int, error)
	GetTask(context.Context, string) (Task, error)
	UpdateTask(context.Context, Task) (Task, error)
	DeleteTask(context.Context, string) error
}

type SummaryRepository interface {
	GetProjectSummary(context.Context, string) (ProjectSummary, error)
}

type UserRepository interface {
	CreateUser(ctx context.Context, email, displayName, passwordHash string) (User, error)
	GetUserByEmail(ctx context.Context, email string) (User, error)
	GetUserByID(ctx context.Context, id string) (User, error)
}

type AuthRepository interface {
	SaveRefreshToken(ctx context.Context, session RefreshTokenSession) error
	GetRefreshToken(ctx context.Context, tokenHash string) (RefreshTokenSession, error)
	RevokeRefreshToken(ctx context.Context, tokenHash string, replacedByHash *string) error
	RevokeAllUserRefreshTokens(ctx context.Context, userID string) error
	RotateRefreshToken(ctx context.Context, oldHash string, newSession RefreshTokenSession) (RefreshTokenSession, error)
}

type MembershipRepository interface {
	AddProjectMember(ctx context.Context, projectID, userID string, role Role) error
	AddProjectMemberWithQuota(ctx context.Context, projectID, userID string, role Role, maxMembers int) error
	CountProjectMembers(ctx context.Context, projectID string) (int, error)
	GetProjectMember(ctx context.Context, projectID, userID string) (Member, error)
	ListProjectMembers(ctx context.Context, projectID string) ([]Member, error)
	RemoveProjectMember(ctx context.Context, projectID, userID string) error
	RemoveProjectMemberSafely(ctx context.Context, projectID, userID string) error // ← جديد: ذرّي
}

// QuotaRepository — تغيّر التوقيع: (quota, found, err)
type QuotaRepository interface {
	GetUserQuota(ctx context.Context, userID string) (UserQuota, bool, error) // ← bool جديد
	SetUserQuota(ctx context.Context, quota UserQuota) error
}

type TaskAuditRepository interface {
	TaskRepository
	CreateTaskWithAudit(context.Context, Task, AuditEvent) (Task, error)
}

// Pinger — نُقل إلى domain (كان في service)
type Pinger interface {
	Ping(context.Context) error
}
