package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"workhub-service/internal/domain"
	"workhub-service/internal/infrastructure/telemetry"
	"workhub-service/internal/middleware"
)

// ─────────────────────────────────────────────────────────────
// Sentinel errors
// ─────────────────────────────────────────────────────────────

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

// ─────────────────────────────────────────────────────────────
// Public contracts
// ─────────────────────────────────────────────────────────────

// SummaryCache provides read-through caching for project summaries.
// Implementations must be safe for concurrent use.
type SummaryCache interface {
	Get(context.Context, string) (domain.ProjectSummary, bool)
	Set(context.Context, domain.ProjectSummary)
	Invalidate(context.Context, string) error
	GetOrCompute(
		context.Context,
		string,
		func(context.Context) (domain.ProjectSummary, error),
	) (domain.ProjectSummary, error)
}

// EventPublisher publishes domain events to the message broker.
// Implementations may drop events on transient failures (best-effort).
type EventPublisher interface {
	Publish(context.Context, domain.Event) error
}

// AuthConfig holds authentication-related configuration.
type AuthConfig struct {
	JWTSecret       string
	AccessTokenTTL  time.Duration
	RefreshTokenTTL time.Duration
	BCryptCost      int
}

// Deps is the dependency bundle for constructing a WorkHub.
//
// Required fields (nil will cause NewWorkHub to fail):
//   - Projects, Tasks, Users, Auth, Memberships
//
// Optional fields (nil is acceptable; behavior degrades gracefully):
//   - Summaries, Quotas, Pinger, Cache, Publisher
//
// Zero-valued integer limits are replaced with safe defaults.
type Deps struct {
	// Required
	Projects    domain.ProjectRepository
	Tasks       domain.TaskRepository
	Users       domain.UserRepository
	Auth        domain.AuthRepository
	Memberships domain.MembershipRepository

	// Optional
	Summaries domain.SummaryRepository
	Quotas    domain.QuotaRepository
	Audits    domain.AuditRepository
	Pinger    domain.Pinger
	Cache     SummaryCache
	Publisher EventPublisher

	// Config
	Config AuthConfig

	// Limits (0 = use default)
	MaxProjectsPerUser   int
	MaxMembersPerProject int
	MaxTasksPerProject   int
}

// WorkHub is the top-level service orchestrating domain operations.
type WorkHub struct {
	// Repositories
	projects    domain.ProjectRepository
	tasks       domain.TaskRepository
	summaries   domain.SummaryRepository
	users       domain.UserRepository
	auth        domain.AuthRepository
	memberships domain.MembershipRepository
	quotas      domain.QuotaRepository
	audits      domain.AuditRepository
	pinger      domain.Pinger

	// Infrastructure
	cache     SummaryCache
	publisher EventPublisher

	// Auth configuration
	jwtSecret       string
	accessTokenTTL  time.Duration
	refreshTokenTTL time.Duration
	bcryptCost      int

	// Limits
	maxProjectsPerUser   int
	maxMembersPerProject int
	maxTasksPerProject   int

	// Clock (overridable in tests)
	now func() time.Time
}

// ─────────────────────────────────────────────────────────────
// Constructor
// ─────────────────────────────────────────────────────────────

// Default limits applied when Deps provides zero values.
const (
	defaultMaxProjectsPerUser   = 50
	defaultMaxMembersPerProject = 100
	defaultMaxTasksPerProject   = 1000
)

// NewWorkHub validates the dependency bundle, applies defaults,
// and returns a ready-to-use WorkHub.
func NewWorkHub(d Deps) (*WorkHub, error) {
	if err := validateDeps(d); err != nil {
		return nil, err
	}
	applyDefaults(&d)
	return assemble(d), nil
}

// validateDeps checks required fields and value ranges.
// It does NOT mutate d.
func validateDeps(d Deps) error {
	// ─── Required dependencies ─────────────────────────────
	switch {
	case d.Projects == nil:
		return errors.New("service: Deps.Projects is required")
	case d.Tasks == nil:
		return errors.New("service: Deps.Tasks is required")
	case d.Users == nil:
		return errors.New("service: Deps.Users is required")
	case d.Auth == nil:
		return errors.New("service: Deps.Auth is required")
	case d.Memberships == nil:
		return errors.New("service: Deps.Memberships is required")
	}

	// ─── Auth configuration ────────────────────────────────
	if len(d.Config.JWTSecret) < 32 {
		return errors.New("service: Config.JWTSecret must be at least 32 bytes")
	}
	if d.Config.AccessTokenTTL < time.Minute || d.Config.AccessTokenTTL > time.Hour {
		return errors.New("service: Config.AccessTokenTTL must be between 1 minute and 1 hour")
	}
	if d.Config.RefreshTokenTTL < time.Hour || d.Config.RefreshTokenTTL > 90*24*time.Hour {
		return errors.New("service: Config.RefreshTokenTTL must be between 1 hour and 90 days")
	}
	if d.Config.BCryptCost < 10 || d.Config.BCryptCost > 15 {
		return errors.New("service: Config.BCryptCost must be between 10 and 15")
	}

	// ─── Optional dependency coherence ─────────────────────
	// No checks needed: nil is explicitly allowed and handled
	// at use sites via defensive nil checks.

	return nil
}

// applyDefaults fills zero-valued limits with safe defaults.
// It mutates d in place.
func applyDefaults(d *Deps) {
	if d.MaxProjectsPerUser == 0 {
		d.MaxProjectsPerUser = defaultMaxProjectsPerUser
	}
	if d.MaxMembersPerProject == 0 {
		d.MaxMembersPerProject = defaultMaxMembersPerProject
	}
	if d.MaxTasksPerProject == 0 {
		d.MaxTasksPerProject = defaultMaxTasksPerProject
	}
}

// assemble wires validated dependencies into a WorkHub.
// It assumes validateDeps and applyDefaults have already run.
func assemble(d Deps) *WorkHub {
	return &WorkHub{
		// Repositories
		projects:    d.Projects,
		tasks:       d.Tasks,
		summaries:   d.Summaries,
		users:       d.Users,
		auth:        d.Auth,
		memberships: d.Memberships,
		quotas:      d.Quotas,
		audits:      d.Audits,
		pinger:      d.Pinger,

		// Infrastructure
		cache:     d.Cache,
		publisher: d.Publisher,

		// Auth
		jwtSecret:       d.Config.JWTSecret,
		accessTokenTTL:  d.Config.AccessTokenTTL,
		refreshTokenTTL: d.Config.RefreshTokenTTL,
		bcryptCost:      d.Config.BCryptCost,

		// Limits
		maxProjectsPerUser:   d.MaxProjectsPerUser,
		maxMembersPerProject: d.MaxMembersPerProject,
		maxTasksPerProject:   d.MaxTasksPerProject,

		// Clock
		now: time.Now,
	}
}

// ─────────────────────────────────────────────────────────────
// Quota helpers
// ─────────────────────────────────────────────────────────────

// GetEffectiveMaxProjects returns the project limit for a user,
// preferring a per-user override if one exists.
func (s *WorkHub) GetEffectiveMaxProjects(ctx context.Context, userID string) int {
	if s.quotas != nil {
		q, found, err := s.quotas.GetUserQuota(ctx, userID)
		if err == nil && found && q.MaxProjects != nil {
			return *q.MaxProjects
		}
	}
	return s.maxProjectsPerUser
}

// GetEffectiveMaxMembers returns the member limit for a user,
// preferring a per-user override if one exists.
func (s *WorkHub) GetEffectiveMaxMembers(ctx context.Context, userID string) int {
	if s.quotas != nil {
		q, found, err := s.quotas.GetUserQuota(ctx, userID)
		if err == nil && found && q.MaxMembersPerProject != nil {
			return *q.MaxMembersPerProject
		}
	}
	return s.maxMembersPerProject
}

// GetEffectiveMaxTasks returns the task limit for a user,
// preferring a per-user override if one exists.
func (s *WorkHub) GetEffectiveMaxTasks(ctx context.Context, userID string) int {
	if s.quotas != nil {
		q, found, err := s.quotas.GetUserQuota(ctx, userID)
		if err == nil && found && q.MaxTasksPerProject != nil {
			return *q.MaxTasksPerProject
		}
	}
	return s.maxTasksPerProject
}

// GetUserQuota returns the quota for a target user.
// A user may only read their own quota (super-admin bypass pending).
func (s *WorkHub) GetUserQuota(
	ctx context.Context,
	actorUserID, userID string,
) (domain.UserQuota, error) {
	if actorUserID == "" {
		return domain.UserQuota{}, ErrUnauthorized
	}
	if s.quotas == nil {
		return domain.UserQuota{UserID: userID}, nil
	}
	if actorUserID != userID {
		return domain.UserQuota{}, ErrForbidden
	}
	quota, _, err := s.quotas.GetUserQuota(ctx, userID)
	return quota, err
}

// SetUserQuota updates a target user's quota.
// Only super-admins may call this (currently fail-closed).
func (s *WorkHub) SetUserQuota(
	ctx context.Context,
	actorUserID string,
	quota domain.UserQuota,
) error {
	if actorUserID == "" {
		return ErrUnauthorized
	}
	if s.quotas == nil {
		return ErrStorage
	}
	if !s.IsSuperAdmin(actorUserID) {
		return ErrForbidden
	}

	quota.SetBy = actorUserID
	if quota.SetAt.IsZero() {
		quota.SetAt = s.now().UTC()
	}

	if err := s.quotas.SetUserQuota(ctx, quota); err != nil {
		return err
	}

	slog.InfoContext(ctx, "user quota updated",
		"actor_user_id", actorUserID,
		"target_user_id", quota.UserID)

	s.publishQuotaUpdated(ctx, quota)
	return nil
}

// publishQuotaUpdated emits a domain event. Failures are logged but
// do not propagate (best-effort semantics).
func (s *WorkHub) publishQuotaUpdated(ctx context.Context, quota domain.UserQuota) {
	if s.publisher == nil {
		return
	}
	event := domain.Event{
		ID:            fmt.Sprintf("quota-updated-%s-%d", quota.UserID, s.now().UnixNano()),
		Type:          "quota.updated",
		AggregateType: "user",
		AggregateID:   quota.UserID,
		OccurredAt:    s.now().UTC(),
		CorrelationID: middleware.RequestIDFromContext(ctx),
	}
	if err := s.publisher.Publish(ctx, event); err != nil {
		slog.ErrorContext(ctx, "publish quota.updated failed",
			"error", err,
			"user_id", quota.UserID)
	}
}

// ─────────────────────────────────────────────────────────────
// Authorization helpers
// ─────────────────────────────────────────────────────────────

// IsSuperAdmin reports whether the given user has super-admin privileges.
// Fail-closed: returns false until a proper admin model is added.
func (s *WorkHub) IsSuperAdmin(actorUserID string) bool {
	return false
}

// CheckAccess verifies that actorUserID is a member of projectID
// with at least one of requiredRoles (if any are provided).
func (s *WorkHub) CheckAccess(
	ctx context.Context,
	actorUserID, projectID string,
	requiredRoles ...domain.Role,
) (domain.Member, error) {
	if actorUserID == "" {
		return domain.Member{}, ErrUnauthorized
	}
	if projectID == "" {
		return domain.Member{}, ErrForbidden
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

	if len(requiredRoles) > 0 && !domain.HasMinimumRole(member.Role, requiredRoles) {
		return domain.Member{}, ErrForbidden
	}
	return member, nil
}

// ─────────────────────────────────────────────────────────────
// Health
// ─────────────────────────────────────────────────────────────

// Ping checks downstream dependencies. Returns nil if no pinger is
// configured (degraded mode).
func (s *WorkHub) Ping(ctx context.Context) error {
	if s.pinger != nil {
		return s.pinger.Ping(ctx)
	}
	return nil
}

// ─────────────────────────────────────────────────────────────
// Audit Log
// ─────────────────────────────────────────────────────────────

func (s *WorkHub) ListProjectAudit(ctx context.Context, actorUserID, projectID string, limit, offset int) ([]domain.AuditEvent, int, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, span := telemetry.Tracer().Start(ctx, "WorkHub.ListProjectAudit")
	defer span.End()

	if strings.TrimSpace(projectID) == "" {
		return nil, 0, ErrProjectNotFound
	}
	// Any member with at least RoleViewer can view project audit log
	if _, err := s.CheckAccess(ctx, actorUserID, projectID); err != nil {
		return nil, 0, err
	}
	if s.audits == nil {
		return []domain.AuditEvent{}, 0, nil
	}
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}
	return s.audits.ListProjectAuditEvents(ctx, projectID, limit, offset)
}

func (s *WorkHub) recordAudit(ctx context.Context, event domain.AuditEvent) {
	if s.audits == nil {
		return
	}
	if event.CreatedAt.IsZero() {
		event.CreatedAt = s.now().UTC()
	}
	if err := s.audits.RecordAuditEvent(ctx, event); err != nil {
		slog.WarnContext(ctx, "failed to record audit event", "error", err, "type", event.EventType)
	}
}
