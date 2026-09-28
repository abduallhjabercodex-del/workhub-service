package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"workhub-service/internal/domain"
	"workhub-service/internal/middleware"
)

var (
	ErrInvalidInput       = errors.New("invalid input")
	ErrProjectNotFound    = errors.New("project not found")
	ErrTaskNotFound       = errors.New("task not found")
	ErrUserNotFound       = errors.New("user not found")
	ErrNotFound           = errors.New("not found")
	ErrStorage            = errors.New("storage error")
	ErrUserAlreadyExists  = errors.New("user already exists")
	ErrInvalidCredentials = errors.New("invalid credentials")
	ErrUnauthorized       = errors.New("unauthorized")
	ErrForbidden          = errors.New("forbidden")
	ErrTokenExpired       = errors.New("token expired")
)

type SummaryCache interface {
	Get(context.Context, string) (domain.ProjectSummary, bool)
	Set(context.Context, domain.ProjectSummary)
	Invalidate(context.Context, string) error
	GetOrCompute(context.Context, string, func(context.Context) (domain.ProjectSummary, error)) (domain.ProjectSummary, error)
}

type EventPublisher interface {
	Publish(context.Context, domain.Event) error
}

type AuthConfig struct {
	JWTSecret       string
	AccessTokenTTL  time.Duration
	RefreshTokenTTL time.Duration
	BCryptCost      int
}

type Deps struct {
	Projects             domain.ProjectRepository
	Tasks                domain.TaskRepository
	Summaries            domain.SummaryRepository
	Users                domain.UserRepository
	Auth                 domain.AuthRepository
	Memberships          domain.MembershipRepository
	Quotas               domain.QuotaRepository
	Cache                SummaryCache
	Publisher            EventPublisher
	Config               AuthConfig
	MaxProjectsPerUser   int
	MaxMembersPerProject int
	MaxTasksPerProject   int
}

type WorkHub struct {
	projects             domain.ProjectRepository
	tasks                domain.TaskRepository
	summaries            domain.SummaryRepository
	users                domain.UserRepository
	auth                 domain.AuthRepository
	memberships          domain.MembershipRepository
	quotas               domain.QuotaRepository
	cache                SummaryCache
	publisher            EventPublisher
	jwtSecret            string
	accessTokenTTL       time.Duration
	refreshTokenTTL      time.Duration
	bcryptCost           int
	maxProjectsPerUser   int
	maxMembersPerProject int
	maxTasksPerProject   int
	now                  func() time.Time
}

func NewWorkHub(d Deps) (*WorkHub, error) {
	if d.Projects == nil || d.Tasks == nil || d.Users == nil || d.Auth == nil || d.Memberships == nil {
		return nil, errors.New("missing required repository dependencies in NewWorkHub")
	}
	if len(d.Config.JWTSecret) < 32 {
		return nil, errors.New("JWTSecret must be at least 32 bytes for production security")
	}
	if d.Config.AccessTokenTTL < time.Minute || d.Config.AccessTokenTTL > time.Hour {
		return nil, errors.New("invalid access token TTL: must be between 1 minute and 1 hour")
	}
	if d.Config.RefreshTokenTTL < time.Hour || d.Config.RefreshTokenTTL > 90*24*time.Hour {
		return nil, errors.New("invalid refresh token TTL: must be between 1 hour and 90 days")
	}
	if d.Config.BCryptCost < 10 || d.Config.BCryptCost > 15 {
		return nil, errors.New("invalid bcrypt cost: must be between 10 and 15")
	}

	workhub := &WorkHub{
		projects:             d.Projects,
		tasks:                d.Tasks,
		summaries:            d.Summaries,
		users:                d.Users,
		auth:                 d.Auth,
		memberships:          d.Memberships,
		quotas:               d.Quotas,
		cache:                d.Cache,
		publisher:            d.Publisher,
		jwtSecret:            d.Config.JWTSecret,
		accessTokenTTL:       d.Config.AccessTokenTTL,
		refreshTokenTTL:      d.Config.RefreshTokenTTL,
		bcryptCost:           d.Config.BCryptCost,
		maxProjectsPerUser:   d.MaxProjectsPerUser,
		maxMembersPerProject: d.MaxMembersPerProject,
		maxTasksPerProject:   d.MaxTasksPerProject,
		now:                  time.Now,
	}
	if d.Summaries == nil {
		if summaries, ok := d.Projects.(domain.SummaryRepository); ok {
			workhub.summaries = summaries
		}
	}
	if d.Quotas == nil {
		if qRepo, ok := d.Projects.(domain.QuotaRepository); ok {
			workhub.quotas = qRepo
		}
	}
	if d.MaxProjectsPerUser == 0 {
		workhub.maxProjectsPerUser = 50
	}
	if d.MaxMembersPerProject == 0 {
		workhub.maxMembersPerProject = 100
	}
	if d.MaxTasksPerProject == 0 {
		workhub.maxTasksPerProject = 1000
	}

	return workhub, nil
}

// GetEffectiveMaxProjects — يستخدم bool الجديد
func (s *WorkHub) GetEffectiveMaxProjects(ctx context.Context, userID string) int {
	if s.quotas != nil {
		q, found, err := s.quotas.GetUserQuota(ctx, userID)
		if err == nil && found && q.MaxProjects != nil {
			return *q.MaxProjects
		}
	}
	return s.maxProjectsPerUser
}

// GetEffectiveMaxMembers — يستخدم bool الجديد
func (s *WorkHub) GetEffectiveMaxMembers(ctx context.Context, userID string) int {
	if s.quotas != nil {
		q, found, err := s.quotas.GetUserQuota(ctx, userID)
		if err == nil && found && q.MaxMembersPerProject != nil {
			return *q.MaxMembersPerProject
		}
	}
	return s.maxMembersPerProject
}

// GetEffectiveMaxTasks — يستخدم bool الجديد
func (s *WorkHub) GetEffectiveMaxTasks(ctx context.Context, userID string) int {
	if s.quotas != nil {
		q, found, err := s.quotas.GetUserQuota(ctx, userID)
		if err == nil && found && q.MaxTasksPerProject != nil {
			return *q.MaxTasksPerProject
		}
	}
	return s.maxTasksPerProject
}

// GetUserQuota — توقيع جديد
func (s *WorkHub) GetUserQuota(ctx context.Context, actorUserID, userID string) (domain.UserQuota, error) {
	if actorUserID == "" {
		return domain.UserQuota{}, ErrUnauthorized
	}
	if s.quotas == nil {
		return domain.UserQuota{UserID: userID}, nil
	}
	// المستخدم يرى quota نفسه فقط (إلا Super Admin — يُضاف لاحقًا)
	if actorUserID != userID {
		return domain.UserQuota{}, ErrForbidden
	}
	quota, _, err := s.quotas.GetUserQuota(ctx, userID)
	return quota, err
}

// SetUserQuota — مع actor و Super Admin check
func (s *WorkHub) SetUserQuota(ctx context.Context, actorUserID string, quota domain.UserQuota) error {
	if actorUserID == "" {
		return ErrUnauthorized
	}
	if s.quotas == nil {
		return ErrStorage
	}
	// TODO: isSuperAdmin check
	// if !s.isSuperAdmin(actorUserID) { return ErrForbidden }

	quota.SetBy = actorUserID
	if quota.SetAt.IsZero() {
		quota.SetAt = s.now().UTC()
	}

	if err := s.quotas.SetUserQuota(ctx, quota); err != nil {
		return err
	}

	slog.InfoContext(ctx, "user quota updated",
		"actor_user_id", actorUserID, "target_user_id", quota.UserID)

	// Audit event
	if s.publisher != nil {
		event := domain.Event{
			ID:            fmt.Sprintf("quota-updated-%s-%d", quota.UserID, s.now().UnixNano()),
			Type:          "quota.updated",
			AggregateType: "user",
			AggregateID:   quota.UserID,
			OccurredAt:    s.now().UTC(),
			CorrelationID: middleware.RequestIDFromContext(ctx),
		}
		_ = s.publisher.Publish(ctx, event)
	}
	return nil
}

type Pinger interface {
	Ping(context.Context) error
}

func (s *WorkHub) Ping(ctx context.Context) error {
	if pinger, ok := s.projects.(Pinger); ok {
		return pinger.Ping(ctx)
	}
	return nil
}

func (s *WorkHub) CheckAccess(ctx context.Context, actorUserID, projectID string, requiredRoles ...domain.Role) (domain.Member, error) {
	if actorUserID == "" {
		return domain.Member{}, ErrUnauthorized
	}
	if s.memberships == nil {
		return domain.Member{}, ErrStorage
	}
	member, err := s.memberships.GetProjectMember(ctx, projectID, actorUserID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return domain.Member{}, ErrForbidden
		}
		return domain.Member{}, fmt.Errorf("get project member: %w", err)
	}

	if len(requiredRoles) > 0 {
		if !domain.HasMinimumRole(member.Role, requiredRoles) {
			return domain.Member{}, ErrForbidden
		}
	}
	return member, nil
}
