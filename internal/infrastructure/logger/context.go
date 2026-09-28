package logger

import (
	"context"
	"log/slog"

	"workhub-service/internal/middleware"
)

// ContextHandler is an slog.Handler that extracts context values and appends them to log records.
type ContextHandler struct {
	slog.Handler
}

// Handle adds fields from context.
func (h ContextHandler) Handle(ctx context.Context, r slog.Record) error {
	if reqID := middleware.RequestIDFromContext(ctx); reqID != "" {
		r.AddAttrs(slog.String("request_id", reqID))
	}
	if user, ok := middleware.UserFromContext(ctx); ok {
		r.AddAttrs(slog.String("user_id", user.ID))
	}
	return h.Handler.Handle(ctx, r)
}

// WithAttrs implements slog.Handler.
func (h ContextHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return ContextHandler{h.Handler.WithAttrs(attrs)}
}

// WithGroup implements slog.Handler.
func (h ContextHandler) WithGroup(name string) slog.Handler {
	return ContextHandler{h.Handler.WithGroup(name)}
}
