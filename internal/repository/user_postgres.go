package repository

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"

	"workhub-service/internal/domain"
	"workhub-service/internal/infrastructure/metrics"

	"github.com/jackc/pgx/v5"
)

func (r *PostgresRepository) CreateUser(ctx context.Context, email, displayName, passwordHash string) (domain.User, error) {
	lowerEmail := strings.ToLower(strings.TrimSpace(email))
	var id int64
	var createdAt time.Time
	stop := metrics.ObserveDBQuery("INSERT", "users")
	err := r.pool.QueryRow(ctx, `
		INSERT INTO users (email, display_name, password_hash, created_at, updated_at)
		VALUES ($1, $2, $3, now(), now())
		RETURNING id, created_at`, lowerEmail, displayName, passwordHash).Scan(&id, &createdAt)
	stop(&err)
	if err != nil {
		return domain.User{}, mapDBError(err)
	}
	return domain.User{
		ID:           strconv.FormatInt(id, 10),
		Email:        lowerEmail,
		DisplayName:  displayName,
		PasswordHash: passwordHash,
		CreatedAt:    createdAt,
	}, nil
}

func (r *PostgresRepository) GetUserByEmail(ctx context.Context, email string) (domain.User, error) {
	lowerEmail := strings.ToLower(strings.TrimSpace(email))
	var u domain.User
	var id int64
	stop := metrics.ObserveDBQuery("SELECT", "users")
	err := r.pool.QueryRow(ctx, `
		SELECT id, email, display_name, password_hash, created_at
		FROM users WHERE lower(email) = $1 AND deleted_at IS NULL`, lowerEmail).
		Scan(&id, &u.Email, &u.DisplayName, &u.PasswordHash, &u.CreatedAt)
	stop(&err)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.User{}, ErrNotFound
	}
	if err != nil {
		return domain.User{}, mapDBError(err)
	}
	u.ID = strconv.FormatInt(id, 10)
	return u, nil
}

func (r *PostgresRepository) GetUserByID(ctx context.Context, id string) (domain.User, error) {
	numericID, err := parseID(id)
	if err != nil {
		return domain.User{}, ErrInvalidUserID
	}
	var u domain.User
	var numID int64
	stop := metrics.ObserveDBQuery("SELECT", "users")
	err = r.pool.QueryRow(ctx, `
		SELECT id, email, display_name, password_hash, created_at
		FROM users WHERE id = $1 AND deleted_at IS NULL`, numericID).
		Scan(&numID, &u.Email, &u.DisplayName, &u.PasswordHash, &u.CreatedAt)
	stop(&err)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.User{}, ErrNotFound
	}
	if err != nil {
		return domain.User{}, mapDBError(err)
	}
	u.ID = strconv.FormatInt(numID, 10)
	return u, nil
}
