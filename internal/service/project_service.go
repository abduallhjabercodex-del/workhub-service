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

func (s *WorkHub) CreateProject(ctx context.Context, ownerUserID string, in domain.CreateProjectInput) (domain.Project, error) {
	ctx, span := telemetry.Tracer().Start(ctx, "WorkHub.CreateProject")
	defer span.End()

	if ownerUserID == "" {
		return domain.Project{}, ErrUnauthorized
	}
	name := strings.TrimSpace(in.Name)
	if name == "" || len(name) > 120 {
		return domain.Project{}, ErrInvalidInput
	}

	maxProjects := s.GetEffectiveMaxProjects(ctx, ownerUserID)
	p, err := s.projects.CreateProjectWithQuota(ctx, domain.Project{
		OwnerID:   ownerUserID,
		Name:      name,
		CreatedAt: s.now().UTC(),
	}, maxProjects)
	if err != nil {
		if errors.Is(err, domain.ErrQuotaExceeded) {
			slog.WarnContext(ctx, "quota exceeded for projects",
				"user_id", ownerUserID, "max", maxProjects)
		}
		return domain.Project{}, err
	}

	// ← حذفنا AddProjectMember: الـ Repository يُدرج العضوية داخل نفس Transaction

	slog.InfoContext(ctx, "project created", "project_id", p.ID, "owner_user_id", ownerUserID)

	if s.publisher != nil {
		event := domain.Event{
			ID:            fmt.Sprintf("project-created-%s", p.ID),
			Type:          "project.created",
			AggregateType: "project",
			AggregateID:   p.ID,
			ProjectID:     p.ID,
			OccurredAt:    p.CreatedAt,
			CorrelationID: middleware.RequestIDFromContext(ctx),
		}
		if pubErr := s.publisher.Publish(ctx, event); pubErr != nil {
			slog.ErrorContext(ctx, "publish project.created failed",
				"error", pubErr, "project_id", p.ID)
		}
	}
	return p, nil
}

func (s *WorkHub) ListProjects(ctx context.Context, actorUserID, q, sortBy string, limit, offset int) ([]domain.Project, error) {
	if actorUserID == "" {
		return nil, ErrUnauthorized
	}
	items, err := s.projects.ListProjects(ctx, actorUserID, q, sortBy, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("list projects: %w", err)
	}
	return items, nil
}

func (s *WorkHub) GetProject(ctx context.Context, actorUserID, projectID string) (domain.Project, error) {
	if _, err := s.CheckAccess(ctx, actorUserID, projectID); err != nil {
		return domain.Project{}, err
	}
	p, err := s.projects.GetProject(ctx, projectID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return domain.Project{}, ErrProjectNotFound
		}
		return domain.Project{}, fmt.Errorf("get project: %w", err)
	}
	return p, nil
}

func (s *WorkHub) DeleteProject(ctx context.Context, actorUserID, projectID string) error {
	if _, err := s.CheckAccess(ctx, actorUserID, projectID, domain.RoleOwner); err != nil {
		return err
	}
	if err := s.projects.DeleteProject(ctx, projectID); err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return ErrProjectNotFound
		}
		return fmt.Errorf("delete project: %w", err)
	}
	s.invalidateSummary(ctx, projectID)
	return nil
}

func (s *WorkHub) AddProjectMember(ctx context.Context, actorUserID, projectID, targetUserID string, role domain.Role) error {
	switch role {
	case domain.RoleOwner, domain.RoleAdmin, domain.RoleMember, domain.RoleViewer:
	default:
		return ErrInvalidInput
	}
	if s.memberships == nil {
		return ErrStorage
	}

	// ← استخدام العضو من CheckAccess بدل استدعاء مكرر
	actorMember, err := s.CheckAccess(ctx, actorUserID, projectID, domain.RoleAdmin)
	if err != nil {
		return err
	}
	if (role == domain.RoleOwner || role == domain.RoleAdmin) && actorMember.Role != domain.RoleOwner {
		return ErrForbidden
	}

	// ← معالجة خطأ GetProject
	proj, err := s.projects.GetProject(ctx, projectID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return ErrProjectNotFound
		}
		return fmt.Errorf("get project: %w", err)
	}
	maxMembers := s.GetEffectiveMaxMembers(ctx, proj.OwnerID)

	if err := s.memberships.AddProjectMemberWithQuota(ctx, projectID, targetUserID, role, maxMembers); err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return ErrNotFound
		}
		if errors.Is(err, domain.ErrQuotaExceeded) {
			slog.WarnContext(ctx, "quota exceeded for members",
				"project_id", projectID, "max", maxMembers)
		}
		return err
	}

	s.invalidateSummary(ctx, projectID)
	slog.InfoContext(ctx, "project member added",
		"project_id", projectID, "target_user_id", targetUserID,
		"role", role, "actor_user_id", actorUserID)

	if s.publisher != nil {
		event := domain.Event{
			ID:            fmt.Sprintf("member-added-%s-%s", projectID, targetUserID),
			Type:          "member.added",
			AggregateType: "member",
			AggregateID:   targetUserID,
			ProjectID:     projectID,
			OccurredAt:    s.now().UTC(),
			CorrelationID: middleware.RequestIDFromContext(ctx),
		}
		_ = s.publisher.Publish(ctx, event)
	}
	return nil
}

func (s *WorkHub) ListProjectMembers(ctx context.Context, actorUserID, projectID string) ([]domain.Member, error) {
	if s.memberships == nil {
		return nil, ErrStorage
	}
	if _, err := s.CheckAccess(ctx, actorUserID, projectID); err != nil {
		return nil, err
	}
	return s.memberships.ListProjectMembers(ctx, projectID)
}

func (s *WorkHub) RemoveProjectMember(ctx context.Context, actorUserID, projectID, targetUserID string) error {
	if s.memberships == nil {
		return ErrStorage
	}
	if _, err := s.CheckAccess(ctx, actorUserID, projectID, domain.RoleAdmin); err != nil {
		return err
	}

	// ← استخدام الدالة الذرّية الجديدة
	if err := s.memberships.RemoveProjectMemberSafely(ctx, projectID, targetUserID); err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return ErrNotFound
		}
		if errors.Is(err, domain.ErrLastOwner) {
			return ErrForbidden
		}
		return fmt.Errorf("remove project member: %w", err)
	}

	s.invalidateSummary(ctx, projectID)
	slog.InfoContext(ctx, "project member removed",
		"project_id", projectID, "target_user_id", targetUserID,
		"actor_user_id", actorUserID)

	if s.publisher != nil {
		event := domain.Event{
			ID:            fmt.Sprintf("member-removed-%s-%s", projectID, targetUserID),
			Type:          "member.removed",
			AggregateType: "member",
			AggregateID:   targetUserID,
			ProjectID:     projectID,
			OccurredAt:    s.now().UTC(),
			CorrelationID: middleware.RequestIDFromContext(ctx),
		}
		_ = s.publisher.Publish(ctx, event)
	}
	return nil
}

func (s *WorkHub) ProjectSummary(ctx context.Context, actorUserID, projectID string) (domain.ProjectSummary, error) {
	if s.summaries == nil {
		return domain.ProjectSummary{}, ErrStorage
	}
	if _, err := s.CheckAccess(ctx, actorUserID, projectID); err != nil {
		return domain.ProjectSummary{}, err
	}
	if s.cache != nil {
		return s.cache.GetOrCompute(ctx, projectID, func(ctx context.Context) (domain.ProjectSummary, error) {
			summary, err := s.summaries.GetProjectSummary(ctx, projectID)
			if err != nil {
				if errors.Is(err, domain.ErrNotFound) {
					return domain.ProjectSummary{}, ErrProjectNotFound
				}
				return domain.ProjectSummary{}, fmt.Errorf("get project summary: %w", err)
			}
			return summary, nil
		})
	}
	summary, err := s.summaries.GetProjectSummary(ctx, projectID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return domain.ProjectSummary{}, ErrProjectNotFound
		}
		return domain.ProjectSummary{}, fmt.Errorf("get project summary: %w", err)
	}
	return summary, nil
}

func (s *WorkHub) invalidateSummary(ctx context.Context, projectID string) {
	if s.cache != nil {
		if err := s.cache.Invalidate(ctx, projectID); err != nil {
			slog.WarnContext(ctx, "cache invalidation failed",
				"error", err, "project_id", projectID)
		}
	}
}
