package handler

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
	"log/slog"
	"workhub-service/internal/domain"
	"workhub-service/internal/infrastructure/cache"
	"workhub-service/internal/middleware"
	"workhub-service/internal/service"

	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type Config struct {
	AuthRateLimit            int
	ProjectCreationRateLimit int
}

type API struct {
	workhub                *service.WorkHub
	trustedProxies         map[string]struct{}
	projectCreationLimiter *middleware.RateLimiterStore
	authLimiter            *middleware.RateLimiterStore
	idempotencyStore       cache.IdempotencyStore
	logger                 *slog.Logger
	health                 *AppHealth
}

func NewAPI(workhub *service.WorkHub, trustedProxies map[string]struct{}, idempotencyStore cache.IdempotencyStore, logger *slog.Logger) *API {
	return NewAPIWithConfig(workhub, trustedProxies, idempotencyStore, logger, Config{
		AuthRateLimit:            5,
		ProjectCreationRateLimit: 10,
	})
}

func NewAPIWithConfig(workhub *service.WorkHub, trustedProxies map[string]struct{}, idempotencyStore cache.IdempotencyStore, logger *slog.Logger, cfg Config) *API {
	projLimit := cfg.ProjectCreationRateLimit
	if projLimit <= 0 {
		projLimit = 10
	}
	authLimit := cfg.AuthRateLimit
	if authLimit <= 0 {
		authLimit = 5
	}

	projInterval := time.Minute / time.Duration(projLimit)
	authInterval := time.Minute / time.Duration(authLimit)

	return &API{
		workhub:                workhub,
		trustedProxies:         trustedProxies,
		projectCreationLimiter: middleware.NewRateLimiter(projLimit, projInterval),
		authLimiter:            middleware.NewRateLimiter(authLimit, authInterval),
		idempotencyStore:       idempotencyStore,
		logger:                 logger,
		health:                 NewAppHealth(),
	}
}

func (a *API) AppHealth() *AppHealth {
	return a.health
}

func (a *API) Routes() http.Handler {
	mux := http.NewServeMux()

	// Public endpoints
	mux.Handle("/metrics", promhttp.Handler())
	mux.HandleFunc("/health", Health)
	mux.HandleFunc("/ready", a.Ready)
	mux.HandleFunc("/startup", a.Startup)
	mux.HandleFunc("/api/v1/docs", ServeSwaggerUI)
	mux.HandleFunc("/api/v1/docs/openapi.yaml", ServeOpenAPISpec)
	mux.Handle("/api/v1/docs/assets/", http.StripPrefix("/api/v1/docs/", ServeSwaggerAssets()))

	// Auth endpoints (public with rate limit)
	authHandler := middleware.RateLimit(a.authLimiter, a.trustedProxies)

	mux.Handle("/api/v1/auth/register", authHandler(http.HandlerFunc(a.register)))
	mux.Handle("/api/v1/auth/login", authHandler(http.HandlerFunc(a.login)))
	mux.Handle("/api/v1/auth/refresh", authHandler(http.HandlerFunc(a.refresh)))
	mux.Handle("/api/v1/auth/logout", authHandler(http.HandlerFunc(a.logout)))

	// Protected endpoints — apply RequireAuth for each route
	requireAuth := middleware.RequireAuth
	idempotency := middleware.Idempotency(a.idempotencyStore, a.logger)

	protect := func(h http.HandlerFunc) http.Handler {
		return requireAuth(idempotency(h))
	}

	mux.Handle("/api/v1/auth/me", requireAuth(http.HandlerFunc(a.me)))

	// Projects
	mux.Handle("GET /api/v1/projects", protect(a.listProjects))
	mux.Handle("POST /api/v1/projects", protect(a.createProject))
	mux.Handle("GET /api/v1/projects/{id}", protect(a.getProject))
	mux.Handle("DELETE /api/v1/projects/{id}", protect(a.deleteProject))

	// Project Subresources
	mux.Handle("GET /api/v1/projects/{id}/summary", protect(a.getProjectSummary))
	mux.Handle("GET /api/v1/projects/{id}/audit", protect(a.listProjectAudit))
	mux.Handle("GET /api/v1/projects/{id}/members", protect(a.listProjectMembers))
	mux.Handle("POST /api/v1/projects/{id}/members", protect(a.addProjectMember))
	mux.Handle("DELETE /api/v1/projects/{id}/members/{userId}", protect(a.removeProjectMember))

	// Project Tasks
	mux.Handle("GET /api/v1/projects/{id}/tasks", protect(a.listProjectTasks))
	mux.Handle("POST /api/v1/projects/{id}/tasks", protect(a.createProjectTask))

	// Tasks
	mux.Handle("GET /api/v1/tasks/{id}", protect(a.getTask))
	mux.Handle("PATCH /api/v1/tasks/{id}", protect(a.updateTask))
	mux.Handle("DELETE /api/v1/tasks/{id}", protect(a.deleteTask))

	// Quotas
	mux.Handle("GET /api/v1/quotas/{id}", requireAuth(http.HandlerFunc(a.getUserQuota)))
	mux.Handle("PUT /api/v1/quotas/{id}", requireAuth(http.HandlerFunc(a.setUserQuota)))

	return mux
}

type envelope struct {
	Data  any       `json:"data,omitempty"`
	Error *apiError `json:"error,omitempty"`
	Meta  any       `json:"meta,omitempty"`
}

type apiError struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	RequestID string `json:"requestId,omitempty"`
}

func writeServiceError(w http.ResponseWriter, r *http.Request, err error) {
	var quotaErr *domain.QuotaError
	if errors.As(err, &quotaErr) || errors.Is(err, domain.ErrQuotaExceeded) {
		message := "Resource quota exceeded"
		if quotaErr != nil {
			message = fmt.Sprintf("Quota exceeded for %s: current count %d reaches maximum allowed %d",
				quotaErr.Resource, quotaErr.Current, quotaErr.Max)
		}
		writeJSON(w, http.StatusForbidden, envelope{Error: &apiError{
			Code: "QUOTA_EXCEEDED", Message: message, RequestID: requestID(r)}})
		return
	}

	status, code, message := http.StatusInternalServerError, "internal_error", "internal server error"
	switch {
	case errors.Is(err, service.ErrInvalidInput):
		status, code, message = http.StatusBadRequest, "invalid_input", "request validation failed"
	case errors.Is(err, service.ErrProjectNotFound):
		status, code, message = http.StatusNotFound, "project_not_found", "project not found"
	case errors.Is(err, service.ErrTaskNotFound):
		status, code, message = http.StatusNotFound, "task_not_found", "task not found"
	case errors.Is(err, service.ErrUserNotFound):
		status, code, message = http.StatusNotFound, "user_not_found", "user not found"
	case errors.Is(err, service.ErrNotFound):
		status, code, message = http.StatusNotFound, "not_found", "resource not found"
	case errors.Is(err, service.ErrUserAlreadyExists):
		status, code, message = http.StatusConflict, "user_already_exists", "user with this email already exists"
	case errors.Is(err, service.ErrInvalidCredentials):
		status, code, message = http.StatusUnauthorized, "invalid_credentials", "invalid email or password"
	case errors.Is(err, service.ErrUnauthorized):
		status, code, message = http.StatusUnauthorized, "unauthorized", "unauthorized"
	case errors.Is(err, service.ErrForbidden):
		status, code, message = http.StatusForbidden, "forbidden", "forbidden: insufficient permissions or membership"
	case errors.Is(err, service.ErrTokenExpired):
		status, code, message = http.StatusUnauthorized, "token_expired", "token has expired"
	}
	writeJSON(w, status, envelope{Error: &apiError{Code: code, Message: message, RequestID: requestID(r)}})
}

func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) bool {
	if ct := r.Header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		writeJSON(w, http.StatusUnsupportedMediaType, envelope{Error: &apiError{
			Code: "unsupported_media_type", Message: "Content-Type must be application/json"}})
		return false
	}
	r.Body = http.MaxBytesReader(w, r.Body, 1024*1024)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		writeJSON(w, http.StatusBadRequest, envelope{Error: &apiError{
			Code: "invalid_json", Message: "request body must be valid JSON or payload too large"}})
		return false
	}
	return true
}

func writeJSON(w http.ResponseWriter, status int, body envelope) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func methodNotAllowed(w http.ResponseWriter, methods ...string) {
	w.Header().Set("Allow", strings.Join(methods, ", "))
	writeJSON(w, http.StatusMethodNotAllowed, envelope{Error: &apiError{
		Code: "method_not_allowed", Message: "method not allowed"}})
}

// requestID — reads from context, not from header.
func requestID(r *http.Request) string {
	return middleware.RequestIDFromContext(r.Context())
}

func pathParam(r *http.Request, key string, fallbackPrefix string) string {
	if val := r.PathValue(key); val != "" {
		return val
	}
	if fallbackPrefix != "" {
		trimmed := strings.TrimPrefix(r.URL.Path, fallbackPrefix)
		if idx := strings.Index(trimmed, "/"); idx != -1 {
			trimmed = trimmed[:idx]
		}
		return strings.TrimSpace(trimmed)
	}
	return ""
}


