package repository

import (
	"strconv"
	"strings"
)

var (
	allowedProjectSort = map[string]string{
		"created_at": "p.created_at",
		"createdAt":  "p.created_at",
		"name":       "p.name",
	}
	allowedTaskSort = map[string]string{
		"created_at": "created_at",
		"createdAt":  "created_at",
		"title":      "title",
	}
)

// parseID converts a raw string ID to a positive int64.
// Returns ErrInvalidID on failure.
func parseID(raw string) (int64, error) {
	id, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
	if err != nil || id < 1 {
		return 0, ErrInvalidID
	}
	return id, nil
}

// escapeLike escapes special SQL LIKE characters (\, %, _).
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
