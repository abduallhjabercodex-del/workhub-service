package repository

import (
	"context"
	"errors"
	"testing"

	"workhub-service/internal/domain"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

func TestParseID(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    int64
		wantErr bool
	}{
		{"valid id", "123", 123, false},
		{"valid with spaces", "  456  ", 456, false},
		{"zero", "0", 0, true},
		{"negative", "-5", 0, true},
		{"empty", "", 0, true},
		{"non-numeric", "abc", 0, true},
		{"max int64", "9223372036854775807", 9223372036854775807, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseID(tt.input)
			if tt.wantErr {
				if !errors.Is(err, ErrInvalidID) {
					t.Fatalf("parseID(%q) error = %v, want ErrInvalidID", tt.input, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseID(%q) unexpected error: %v", tt.input, err)
			}
			if got != tt.want {
				t.Fatalf("parseID(%q) = %d, want %d", tt.input, got, tt.want)
			}
		})
	}
}

func TestEscapeLike(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"", ""},
		{"normal", "normal"},
		{"test%name", `test\%name`},
		{"test_name", `test\_name`},
		{`test\name`, `test\\name`},
		{`%_\`, `\%\_\\`},
	}

	for _, tt := range tests {
		got := escapeLike(tt.input)
		if got != tt.want {
			t.Errorf("escapeLike(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestAllowedSortMapsMatchContract(t *testing.T) {
	// Must support camelCase (OpenAPI) and snake_case (legacy)
	if _, ok := allowedProjectSort["createdAt"]; !ok {
		t.Errorf("allowedProjectSort missing 'createdAt'")
	}
	if _, ok := allowedProjectSort["created_at"]; !ok {
		t.Errorf("allowedProjectSort missing 'created_at'")
	}
	if _, ok := allowedProjectSort["name"]; !ok {
		t.Errorf("allowedProjectSort missing 'name'")
	}

	if _, ok := allowedTaskSort["createdAt"]; !ok {
		t.Errorf("allowedTaskSort missing 'createdAt'")
	}
	if _, ok := allowedTaskSort["created_at"]; !ok {
		t.Errorf("allowedTaskSort missing 'created_at'")
	}
	if _, ok := allowedTaskSort["title"]; !ok {
		t.Errorf("allowedTaskSort missing 'title'")
	}
}

func TestMapDBError(t *testing.T) {
	if err := mapDBError(nil); err != nil {
		t.Errorf("mapDBError(nil) = %v, want nil", err)
	}

	if err := mapDBError(context.Canceled); !errors.Is(err, context.Canceled) {
		t.Errorf("mapDBError(Canceled) = %v, want context.Canceled", err)
	}

	if err := mapDBError(pgx.ErrNoRows); !errors.Is(err, ErrNotFound) {
		t.Errorf("mapDBError(ErrNoRows) = %v, want ErrNotFound", err)
	}

	// 23505 -> ErrAlreadyExists
	fkUnique := &pgconn.PgError{Code: "23505", Message: "duplicate key"}
	if err := mapDBError(fkUnique); !errors.Is(err, ErrAlreadyExists) {
		t.Errorf("mapDBError(23505) = %v, want ErrAlreadyExists", err)
	}

	// 23503 -> ErrNotFound AND ErrConstraint
	fkNotFound := &pgconn.PgError{Code: "23503", Message: "foreign key violation"}
	errFk := mapDBError(fkNotFound)
	if !errors.Is(errFk, ErrNotFound) {
		t.Errorf("mapDBError(23503) = %v, want ErrNotFound", errFk)
	}
	if !errors.Is(errFk, ErrConstraint) {
		t.Errorf("mapDBError(23503) = %v, want ErrConstraint", errFk)
	}

	// 23514 -> ErrConstraint
	checkErr := &pgconn.PgError{Code: "23514", Message: "check violation"}
	if err := mapDBError(checkErr); !errors.Is(err, ErrConstraint) {
		t.Errorf("mapDBError(23514) = %v, want ErrConstraint", err)
	}

	// 40001 -> ErrConflict
	conflictErr := &pgconn.PgError{Code: "40001", Message: "serialization failure"}
	if err := mapDBError(conflictErr); !errors.Is(err, ErrConflict) {
		t.Errorf("mapDBError(40001) = %v, want ErrConflict", err)
	}

	// QuotaError preservation
	quotaErr := &domain.QuotaError{Resource: "projects", Current: 5, Max: 5}
	if err := mapDBError(quotaErr); !errors.Is(err, quotaErr) {
		t.Errorf("mapDBError(QuotaError) = %v, want QuotaError", err)
	}
}
