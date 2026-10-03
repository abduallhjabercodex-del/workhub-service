package repository

import (
	"context"
	"errors"
	"strconv"

	"workhub-service/internal/domain"
	"workhub-service/internal/infrastructure/metrics"
	"workhub-service/internal/infrastructure/telemetry"

	"github.com/jackc/pgx/v5"
)

func (r *PostgresRepository) AddProjectMember(ctx context.Context, projectID, userID string, role domain.Role) error {
	pID, err := parseID(projectID)
	if err != nil {
		return ErrInvalidProjectID
	}
	uID, err := parseID(userID)
	if err != nil {
		return ErrInvalidUserID
	}
	if role == "" {
		role = domain.RoleMember
	}
	switch role {
	case domain.RoleOwner, domain.RoleAdmin, domain.RoleMember, domain.RoleViewer:
	default:
		return ErrInvalidRole
	}

	stop := metrics.ObserveDBQuery("INSERT", "memberships")
	_, err = r.pool.Exec(ctx, `
		INSERT INTO memberships (project_id, user_id, role, joined_at, removed_at)
		VALUES ($1, $2, $3, now(), NULL)
		ON CONFLICT (project_id, user_id)
		DO UPDATE SET role = EXCLUDED.role, removed_at = NULL`, pID, uID, role)
	stop(&err)
	if err != nil {
		return mapDBError(err)
	}
	return nil
}

func (r *PostgresRepository) AddProjectMemberWithQuota(
	ctx context.Context,
	projectID, userID string,
	role domain.Role,
	maxMembers int,
) error {
	ctx, span := telemetry.Tracer().Start(ctx, "PostgresRepository.AddProjectMemberWithQuota")
	defer span.End()

	pID, err := parseID(projectID)
	if err != nil {
		return ErrInvalidProjectID
	}
	uID, err := parseID(userID)
	if err != nil {
		return ErrInvalidUserID
	}
	if role == "" {
		role = domain.RoleMember
	}
	switch role {
	case domain.RoleOwner, domain.RoleAdmin, domain.RoleMember, domain.RoleViewer:
	default:
		return ErrInvalidRole
	}

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return mapDBError(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	stopLock := metrics.ObserveDBQuery("SELECT_LOCK", "memberships")
	_, errLock := tx.Exec(ctx,
		`SELECT pg_advisory_xact_lock(hashtext('quota:members')::int, hashtext($1::text)::int)`,
		strconv.FormatInt(pID, 10),
	)
	stopLock(&errLock)
	if errLock != nil {
		return mapDBError(errLock)
	}

	var total, existing int
	stopCount := metrics.ObserveDBQuery("SELECT_COUNT", "memberships")
	errCount := tx.QueryRow(ctx, `
		SELECT
			count(*) AS total,
			count(*) FILTER (WHERE user_id = $2) AS existing
		FROM memberships
		WHERE project_id = $1 AND removed_at IS NULL`,
		pID, uID,
	).Scan(&total, &existing)
	stopCount(&errCount)
	if errCount != nil {
		return mapDBError(errCount)
	}

	// Quota تُطبَّق فقط على إضافة جديدة (ليس تعديل عضو موجود).
	if existing == 0 && maxMembers >= 0 && total >= maxMembers {
		return &domain.QuotaError{Resource: "members", Current: total, Max: maxMembers}
	}

	stopInsert := metrics.ObserveDBQuery("INSERT", "memberships")
	_, errInsert := tx.Exec(ctx, `
		INSERT INTO memberships (project_id, user_id, role, joined_at, removed_at)
		VALUES ($1, $2, $3, now(), NULL)
		ON CONFLICT (project_id, user_id)
		DO UPDATE SET role = EXCLUDED.role, removed_at = NULL`, pID, uID, role,
	)
	stopInsert(&errInsert)
	if errInsert != nil {
		return mapDBError(errInsert)
	}

	return tx.Commit(ctx)
}

func (r *PostgresRepository) CountProjectMembers(ctx context.Context, projectIDStr string) (int, error) {
	pID, err := parseID(projectIDStr)
	if err != nil {
		return 0, ErrInvalidProjectID
	}
	var count int
	stop := metrics.ObserveDBQuery("SELECT_COUNT", "memberships")
	err = r.pool.QueryRow(ctx,
		`SELECT count(*) FROM memberships WHERE project_id = $1 AND removed_at IS NULL`,
		pID,
	).Scan(&count)
	stop(&err)
	if err != nil {
		return 0, mapDBError(err)
	}
	return count, nil
}

func (r *PostgresRepository) GetProjectMember(ctx context.Context, projectID, userID string) (domain.Member, error) {
	pID, err := parseID(projectID)
	if err != nil {
		return domain.Member{}, ErrInvalidProjectID
	}
	uID, err := parseID(userID)
	if err != nil {
		return domain.Member{}, ErrInvalidUserID
	}
	var m domain.Member
	var pNum, uNum int64
	stop := metrics.ObserveDBQuery("SELECT", "memberships")
	err = r.pool.QueryRow(ctx, `
		SELECT m.project_id, m.user_id, u.email, u.display_name, m.role, m.joined_at
		FROM memberships m
		JOIN users u ON u.id = m.user_id
		WHERE m.project_id = $1 AND m.user_id = $2 AND m.removed_at IS NULL`,
		pID, uID,
	).Scan(&pNum, &uNum, &m.Email, &m.DisplayName, &m.Role, &m.JoinedAt)
	stop(&err)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Member{}, ErrNotFound
	}
	if err != nil {
		return domain.Member{}, mapDBError(err)
	}
	m.ProjectID = strconv.FormatInt(pNum, 10)
	m.UserID = strconv.FormatInt(uNum, 10)
	return m, nil
}

func (r *PostgresRepository) ListProjectMembers(ctx context.Context, projectID string) ([]domain.Member, error) {
	pID, err := parseID(projectID)
	if err != nil {
		return nil, ErrInvalidProjectID
	}
	stop := metrics.ObserveDBQuery("SELECT", "memberships")
	rows, err := r.pool.Query(ctx, `
		SELECT m.project_id, m.user_id, u.email, u.display_name, m.role, m.joined_at
		FROM memberships m
		JOIN users u ON u.id = m.user_id
		WHERE m.project_id = $1 AND m.removed_at IS NULL
		ORDER BY m.joined_at ASC, m.user_id ASC`, pID)
	stop(&err)
	if err != nil {
		return nil, mapDBError(err)
	}
	defer rows.Close()

	members := make([]domain.Member, 0)
	for rows.Next() {
		var m domain.Member
		var pNum, uNum int64
		if err := rows.Scan(&pNum, &uNum, &m.Email, &m.DisplayName, &m.Role, &m.JoinedAt); err != nil {
			return nil, mapDBError(err)
		}
		m.ProjectID = strconv.FormatInt(pNum, 10)
		m.UserID = strconv.FormatInt(uNum, 10)
		members = append(members, m)
	}
	return members, mapDBError(rows.Err())
}

// RemoveProjectMember — نسخة بسيطة بدون فحص last-owner.
func (r *PostgresRepository) RemoveProjectMember(ctx context.Context, projectID, userID string) error {
	pID, err := parseID(projectID)
	if err != nil {
		return ErrInvalidProjectID
	}
	uID, err := parseID(userID)
	if err != nil {
		return ErrInvalidUserID
	}
	stop := metrics.ObserveDBQuery("UPDATE", "memberships")
	cmd, err := r.pool.Exec(ctx, `
		UPDATE memberships
		SET removed_at = now()
		WHERE project_id = $1 AND user_id = $2 AND removed_at IS NULL`, pID, uID)
	stop(&err)
	if err != nil {
		return mapDBError(err)
	}
	if cmd.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// RemoveProjectMemberSafely — ذرّي: يمنع حذف آخر Owner.
func (r *PostgresRepository) RemoveProjectMemberSafely(
	ctx context.Context,
	projectID, userID string,
) error {
	ctx, span := telemetry.Tracer().Start(ctx, "PostgresRepository.RemoveProjectMemberSafely")
	defer span.End()

	pID, err := parseID(projectID)
	if err != nil {
		return ErrInvalidProjectID
	}
	uID, err := parseID(userID)
	if err != nil {
		return ErrInvalidUserID
	}

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return mapDBError(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	stopLock := metrics.ObserveDBQuery("SELECT_LOCK", "memberships")
	_, errLock := tx.Exec(ctx,
		`SELECT pg_advisory_xact_lock(hashtext('member:remove')::int, hashtext($1::text)::int)`,
		strconv.FormatInt(pID, 10),
	)
	stopLock(&errLock)
	if errLock != nil {
		return mapDBError(errLock)
	}

	var targetRole string
	stopSelect := metrics.ObserveDBQuery("SELECT", "memberships")
	err = tx.QueryRow(ctx, `
		SELECT role FROM memberships
		WHERE project_id = $1 AND user_id = $2 AND removed_at IS NULL
		FOR UPDATE`, pID, uID).Scan(&targetRole)
	stopSelect(&err)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return mapDBError(err)
	}

	if targetRole == string(domain.RoleOwner) {
		var ownerCount int
		stopOwner := metrics.ObserveDBQuery("SELECT_COUNT", "memberships")
		errOwner := tx.QueryRow(ctx, `
			SELECT count(*) FROM memberships
			WHERE project_id = $1 AND role = 'owner' AND removed_at IS NULL`,
			pID,
		).Scan(&ownerCount)
		stopOwner(&errOwner)
		if errOwner != nil {
			return mapDBError(errOwner)
		}
		if ownerCount <= 1 {
			return domain.ErrLastOwner
		}
	}

	stopUpdate := metrics.ObserveDBQuery("UPDATE", "memberships")
	cmd, errUpdate := tx.Exec(ctx, `
		UPDATE memberships SET removed_at = now()
		WHERE project_id = $1 AND user_id = $2 AND removed_at IS NULL`, pID, uID)
	stopUpdate(&errUpdate)
	if errUpdate != nil {
		return mapDBError(errUpdate)
	}
	if cmd.RowsAffected() == 0 {
		return ErrNotFound
	}

	return tx.Commit(ctx)
}
