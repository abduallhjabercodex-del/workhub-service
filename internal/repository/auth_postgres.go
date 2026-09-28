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

func (r *PostgresRepository) SaveRefreshToken(ctx context.Context, session domain.RefreshTokenSession) error {
	userID, err := parseID(session.UserID)
	if err != nil {
		return ErrInvalidUserID
	}
	stop := metrics.ObserveDBQuery("INSERT", "refresh_tokens")
	_, err = r.pool.Exec(ctx, `
		INSERT INTO refresh_tokens (token_hash, user_id, expires_at, created_at)
		VALUES ($1, $2, $3, now())`, session.TokenHash, userID, session.ExpiresAt)
	stop(&err)
	if err != nil {
		return mapDBError(err)
	}
	return nil
}

func (r *PostgresRepository) GetRefreshToken(ctx context.Context, tokenHash string) (domain.RefreshTokenSession, error) {
	var s domain.RefreshTokenSession
	var numericID, userID int64
	stop := metrics.ObserveDBQuery("SELECT", "refresh_tokens")
	err := r.pool.QueryRow(ctx, `
		SELECT id, token_hash, user_id, expires_at, revoked_at, replaced_by_token_hash, created_at
		FROM refresh_tokens WHERE token_hash = $1`, tokenHash).
		Scan(&numericID, &s.TokenHash, &userID, &s.ExpiresAt, &s.RevokedAt, &s.ReplacedByTokenHash, &s.CreatedAt)
	stop(&err)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.RefreshTokenSession{}, ErrNotFound
	}
	if err != nil {
		return domain.RefreshTokenSession{}, mapDBError(err)
	}
	s.ID = strconv.FormatInt(numericID, 10)
	s.UserID = strconv.FormatInt(userID, 10)
	return s, nil
}

func (r *PostgresRepository) RevokeRefreshToken(ctx context.Context, tokenHash string, replacedByHash *string) error {
	stop := metrics.ObserveDBQuery("UPDATE", "refresh_tokens")
	command, err := r.pool.Exec(ctx, `
		UPDATE refresh_tokens
		SET revoked_at = now(), replaced_by_token_hash = $2
		WHERE token_hash = $1 AND revoked_at IS NULL`, tokenHash, replacedByHash)
	stop(&err)
	if err != nil {
		return mapDBError(err)
	}
	if command.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *PostgresRepository) RevokeAllUserRefreshTokens(ctx context.Context, userID string) error {
	numericID, err := parseID(userID)
	if err != nil {
		return ErrInvalidUserID
	}
	stop := metrics.ObserveDBQuery("UPDATE", "refresh_tokens")
	_, err = r.pool.Exec(ctx, `
		UPDATE refresh_tokens
		SET revoked_at = now()
		WHERE user_id = $1 AND revoked_at IS NULL`, numericID)
	stop(&err)
	return mapDBError(err)
}

// RotateRefreshToken — ذرّي: Revoke القديم + Insert الجديد في Transaction واحدة.
// عند اكتشاف Replay: إبطال كل جلسات المستخدم.
func (r *PostgresRepository) RotateRefreshToken(
	ctx context.Context,
	oldHash string,
	newSession domain.RefreshTokenSession,
) (domain.RefreshTokenSession, error) {
	ctx, span := telemetry.Tracer().Start(ctx, "PostgresRepository.RotateRefreshToken")
	defer span.End()

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return domain.RefreshTokenSession{}, mapDBError(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	userIDNum, err := parseID(newSession.UserID)
	if err != nil {
		return domain.RefreshTokenSession{}, ErrInvalidUserID
	}

	var existingUserID int64
	stop := metrics.ObserveDBQuery("UPDATE", "refresh_tokens")
	err = tx.QueryRow(ctx, `
		UPDATE refresh_tokens
		SET revoked_at = now(), replaced_by_token_hash = $2
		WHERE token_hash = $1 AND revoked_at IS NULL AND expires_at > now()
		RETURNING user_id`, oldHash, newSession.TokenHash).Scan(&existingUserID)
	stop(&err)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return r.handleTokenReplay(ctx, tx, oldHash)
		}
		return domain.RefreshTokenSession{}, mapDBError(err)
	}

	var newNumericID int64
	stopInsert := metrics.ObserveDBQuery("INSERT", "refresh_tokens")
	err = tx.QueryRow(ctx, `
		INSERT INTO refresh_tokens (token_hash, user_id, expires_at, created_at)
		VALUES ($1, $2, $3, now())
		RETURNING id`, newSession.TokenHash, userIDNum, newSession.ExpiresAt).Scan(&newNumericID)
	stopInsert(&err)
	if err != nil {
		return domain.RefreshTokenSession{}, mapDBError(err)
	}

	if err := tx.Commit(ctx); err != nil {
		return domain.RefreshTokenSession{}, mapDBError(err)
	}

	newSession.ID = strconv.FormatInt(newNumericID, 10)
	return newSession, nil
}

// handleTokenReplay — يُستدعى عندما يكون التوكن مُبطَل مسبقًا.
func (r *PostgresRepository) handleTokenReplay(ctx context.Context, tx pgx.Tx, oldHash string) (domain.RefreshTokenSession, error) {
	var revokedUserID int64
	stopCheck := metrics.ObserveDBQuery("SELECT", "refresh_tokens")
	checkErr := tx.QueryRow(ctx,
		`SELECT user_id FROM refresh_tokens WHERE token_hash = $1 AND revoked_at IS NOT NULL`,
		oldHash,
	).Scan(&revokedUserID)
	stopCheck(&checkErr)

	if checkErr != nil {
		return domain.RefreshTokenSession{}, ErrNotFound
	}

	stopExec := metrics.ObserveDBQuery("UPDATE", "refresh_tokens")
	_, execErr := tx.Exec(ctx, `
		UPDATE refresh_tokens SET revoked_at = now()
		WHERE user_id = $1 AND revoked_at IS NULL`, revokedUserID)
	stopExec(&execErr)
	if execErr != nil {
		return domain.RefreshTokenSession{}, mapDBError(execErr)
	}

	if commitErr := tx.Commit(ctx); commitErr != nil {
		return domain.RefreshTokenSession{}, mapDBError(commitErr)
	}
	return domain.RefreshTokenSession{}, domain.ErrTokenReplay
}
