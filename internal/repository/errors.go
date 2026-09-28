package repository

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"workhub-service/internal/domain"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

var (
	ErrNotFound      = domain.ErrNotFound
	ErrAlreadyExists = domain.ErrAlreadyExists
	ErrConstraint    = errors.New("storage constraint violation")
	ErrConflict      = errors.New("storage conflict")

	ErrInvalidID        = errors.New("invalid id")
	ErrInvalidUserID    = errors.New("invalid user id")
	ErrInvalidProjectID = errors.New("invalid project id")
	ErrInvalidSort      = errors.New("invalid sort field")
	ErrInvalidRole      = errors.New("invalid role")

	allowedProjectSort = map[string]string{
		"created_at": "p.created_at",
		"name":       "p.name",
	}
	allowedTaskSort = map[string]string{
		"created_at": "created_at",
		"title":      "title",
	}
)

// parseID — يحوّل string إلى int64 موجب. يعيد ErrInvalidID عند الفشل.
// على المستدعي أن يُعيد تعيينه إلى خطأ مُحدَّد (ErrInvalidUserID, ErrInvalidProjectID).
func parseID(raw string) (int64, error) {
	id, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
	if err != nil || id < 1 {
		return 0, ErrInvalidID
	}
	return id, nil
}

func escapeLike(s string) string {
	if s == "" {
		return ""
	}
	out := make([]byte, 0, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch c {
		case '\\', '%', '_':
			out = append(out, '\\', c)
		default:
			out = append(out, c)
		}
	}
	return string(out)
}

// mapDBError — يحوّل أخطاء pgx/pgconn إلى أخطاء domain،
// مع الحفاظ على الخطأ الأصلي عبر %w (مهم لـ errors.Is/As).
func mapDBError(err error) error {
	if err == nil {
		return nil
	}

	// QuotaError — يُحافَظ عليه كما هو (يُترجم في handler إلى 403).
	var quotaErr *domain.QuotaError
	if errors.As(err, &quotaErr) || errors.Is(err, domain.ErrQuotaExceeded) {
		return err
	}

	// Context errors — لا تُغلَّف، يحتفظ بها الـ caller.
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}

	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}

	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "23505": // unique_violation
			return fmt.Errorf("%w: %s", ErrAlreadyExists, pgErr.Message)
		case "23503", "23514", "23502", "22P02": // FK, check, not-null, invalid input
			return fmt.Errorf("%w: %s", ErrConstraint, pgErr.Message)
		case "40001": // serialization_failure
			return fmt.Errorf("%w: %s", ErrConflict, pgErr.Message)
		}
	}

	// fallback: احتفظ بالخطأ الأصلي عبر %w.
	return fmt.Errorf("database operation failed: %w", err)
}
