package repository

import (
	"context"
	"errors"
	"strconv"
	"time"

	"workhub-service/internal/domain"
	"workhub-service/internal/infrastructure/metrics"

	"github.com/jackc/pgx/v5"
)

func (r *PostgresRepository) GetUserQuota(
	ctx context.Context,
	userIDStr string,
) (domain.UserQuota, bool, error) {
	uID, err := parseID(userIDStr)
	if err != nil {
		return domain.UserQuota{}, false, ErrInvalidUserID
	}

	var quota domain.UserQuota
	var numID int64
	var maxProjects, maxMembers, maxTasks *int
	var setBy *int64
	var reason *string

	stop := metrics.ObserveDBQuery("SELECT", "user_quotas")
	err = r.pool.QueryRow(ctx, `
		SELECT user_id, max_projects, max_members_per_project, max_tasks_per_project,
		       set_by, set_at, updated_at, reason
		FROM user_quotas WHERE user_id = $1`, uID).
		Scan(&numID, &maxProjects, &maxMembers, &maxTasks,
			&setBy, &quota.SetAt, &quota.UpdatedAt, &reason)
	stop(&err)

	if errors.Is(err, pgx.ErrNoRows) {
		return domain.UserQuota{UserID: userIDStr}, false, nil
	}
	if err != nil {
		return domain.UserQuota{}, false, mapDBError(err)
	}

	quota.UserID = strconv.FormatInt(numID, 10)
	quota.MaxProjects = maxProjects
	quota.MaxMembersPerProject = maxMembers
	quota.MaxTasksPerProject = maxTasks
	if setBy != nil {
		quota.SetBy = strconv.FormatInt(*setBy, 10)
	}
	if reason != nil {
		quota.Reason = *reason
	}
	return quota, true, nil
}

func (r *PostgresRepository) SetUserQuota(ctx context.Context, quota domain.UserQuota) error {
	uID, err := parseID(quota.UserID)
	if err != nil {
		return ErrInvalidUserID
	}
	if quota.UpdatedAt.IsZero() {
		quota.UpdatedAt = time.Now().UTC()
	}

	var setBy *int64
	if quota.SetBy != "" {
		sid, err := parseID(quota.SetBy)
		if err != nil {
			return ErrInvalidUserID
		}
		setBy = &sid
	}

	stop := metrics.ObserveDBQuery("INSERT_UPDATE", "user_quotas")
	_, err = r.pool.Exec(ctx, `
		INSERT INTO user_quotas (
			user_id, max_projects, max_members_per_project, max_tasks_per_project,
			set_by, set_at, updated_at, reason
		)
		VALUES ($1, $2, $3, $4, $5, now(), $6, $7)
		ON CONFLICT (user_id) DO UPDATE SET
			max_projects = EXCLUDED.max_projects,
			max_members_per_project = EXCLUDED.max_members_per_project,
			max_tasks_per_project = EXCLUDED.max_tasks_per_project,
			set_by = EXCLUDED.set_by,
			set_at = now(),
			updated_at = EXCLUDED.updated_at,
			reason = EXCLUDED.reason`,
		uID, quota.MaxProjects, quota.MaxMembersPerProject, quota.MaxTasksPerProject,
		setBy, quota.UpdatedAt, quota.Reason)
	stop(&err)

	if err != nil {
		return mapDBError(err)
	}
	return nil
}
