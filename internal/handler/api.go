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

type API struct {
	workhub                *service.WorkHub
	trustedProxies         map[string]struct{}
	projectCreationLimiter *middleware.RateLimiterStore
	idempotencyStore       cache.IdempotencyStore
	logger                 *slog.Logger
	health                 *AppHealth
}

func NewAPI(workhub *service.WorkHub, trustedProxies map[string]struct{}, idempotencyStore cache.IdempotencyStore, logger *slog.Logger) *API {
	return &API{
		workhub:                workhub,
		trustedProxies:         trustedProxies,
		projectCreationLimiter: middleware.NewRateLimiter(10, 6*time.Second), // 10 per minute
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
	authLimiter := middleware.NewRateLimiter(5, 12*time.Second)
	authHandler := middleware.RateLimit(authLimiter, a.trustedProxies)

	mux.Handle("/api/v1/auth/register", authHandler(http.HandlerFunc(a.register)))
	mux.Handle("/api/v1/auth/login", authHandler(http.HandlerFunc(a.login)))
	mux.Handle("/api/v1/auth/refresh", authHandler(http.HandlerFunc(a.refresh)))
	mux.Handle("/api/v1/auth/logout", authHandler(http.HandlerFunc(a.logout)))

	// Protected endpoints — نطبّق RequireAuth لكل مسار
	requireAuth := middleware.RequireAuth

	mux.Handle("/api/v1/auth/me", requireAuth(http.HandlerFunc(a.me)))

	idempotency := middleware.Idempotency(a.idempotencyStore, a.logger)

	projectsHandler := requireAuth(idempotency(http.HandlerFunc(a.projects)))
	mux.Handle("/api/v1/projects", projectsHandler)

	projectSubresourceHandler := requireAuth(idempotency(http.HandlerFunc(a.projectSubresource)))
	mux.Handle("/api/v1/projects/", projectSubresourceHandler)

	tasksHandler := requireAuth(idempotency(http.HandlerFunc(a.taskByID)))
	mux.Handle("/api/v1/tasks/", tasksHandler)
	// globalTasks: أُزيل لأن ListTasks يفرض projectID.

	mux.Handle("/api/v1/quotas/", requireAuth(http.HandlerFunc(a.userQuotas)))

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

// requestID — يقرأ من context، لا من header.
func requestID(r *http.Request) string {
	return middleware.RequestIDFromContext(r.Context())
}

/*
================================================================================
 النسخة المقابلة باستخدام إطار عمل Fiber (للمقارنة فقط - معلّقة ومظللة)
 FIBER EQUIVALENT IMPLEMENTATION (FOR COMPARISON ONLY - COMMENTED OUT)
================================================================================

import (
	"errors"
	"fmt"
	"time"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/filesystem"
	"workhub-service/internal/domain"
	"workhub-service/internal/middleware"
	"workhub-service/internal/service"
)

// RegisterRoutesFiber configures all application routes on a *fiber.App
func (a *API) RegisterRoutesFiber(app *fiber.App) {
	// Public & Health
	app.Get("/health", HealthFiber)
	app.Get("/ready", a.ReadyFiber)
	app.Get("/startup", a.StartupFiber)
	app.Get("/api/v1/docs", ServeSwaggerUIFiber)
	app.Get("/api/v1/docs/openapi.yaml", ServeOpenAPISpecFiber)
	app.Use("/api/v1/docs/assets", filesystem.New(filesystem.Config{
		Root:   http.FS(swaggerAssets),
		PathPrefix: "assets",
	}))

	// API Group
	api := app.Group("/api/v1")

	// Auth (Public with rate limiter)
	auth := api.Group("/auth", middleware.RateLimitFiber(5, 12*time.Second, a.trustedProxies))
	auth.Post("/register", a.registerFiber)
	auth.Post("/login", a.loginFiber)
	auth.Post("/refresh", a.refreshFiber)
	auth.Post("/logout", a.logoutFiber)

	// Protected routes
	protected := api.Group("", middleware.RequireAuthFiber)
	protected.Get("/auth/me", a.meFiber)

	// Projects
	idempotency := middleware.IdempotencyFiber(a.idempotencyStore, a.logger)
	projects := protected.Group("/projects", idempotency)
	projects.Get("/", a.listProjectsFiber)
	projects.Post("/", a.createProjectFiber)
	projects.Get("/:id", a.getProjectFiber)
	projects.Delete("/:id", a.deleteProjectFiber)
	projects.Get("/:id/summary", a.getProjectSummaryFiber)
	projects.Get("/:id/members", a.listProjectMembersFiber)
	projects.Post("/:id/members", a.addProjectMemberFiber)
	projects.Delete("/:id/members/:userId", a.removeProjectMemberFiber)

	// Project Tasks
	projects.Get("/:id/tasks", a.listTasksFiber)
	projects.Post("/:id/tasks", a.createTaskFiber)

	// Standalone Task Operations
	tasks := protected.Group("/tasks", idempotency)
	tasks.Get("/:id", a.getTaskFiber)
	tasks.Patch("/:id", a.updateTaskFiber)
	tasks.Delete("/:id", a.deleteTaskFiber)

	// Quotas
	protected.Get("/quotas/:id", a.getUserQuotaFiber)
	protected.Put("/quotas/:id", a.putUserQuotaFiber)
}

func writeServiceErrorFiber(c *fiber.Ctx, err error) error {
	reqID := middleware.RequestIDFromFiber(c)
	var quotaErr *domain.QuotaError
	if errors.As(err, &quotaErr) || errors.Is(err, domain.ErrQuotaExceeded) {
		message := "Resource quota exceeded"
		if quotaErr != nil {
			message = fmt.Sprintf("Quota exceeded for %s: current count %d reaches maximum allowed %d",
				quotaErr.Resource, quotaErr.Current, quotaErr.Max)
		}
		return c.Status(fiber.StatusForbidden).JSON(envelope{
			Error: &apiError{Code: "QUOTA_EXCEEDED", Message: message, RequestID: reqID},
		})
	}

	status, code, message := fiber.StatusInternalServerError, "internal_error", "internal server error"
	switch {
	case errors.Is(err, service.ErrInvalidInput):
		status, code, message = fiber.StatusBadRequest, "invalid_input", "request validation failed"
	case errors.Is(err, service.ErrProjectNotFound):
		status, code, message = fiber.StatusNotFound, "project_not_found", "project not found"
	case errors.Is(err, service.ErrTaskNotFound):
		status, code, message = fiber.StatusNotFound, "task_not_found", "task not found"
	case errors.Is(err, service.ErrUserAlreadyExists):
		status, code, message = fiber.StatusConflict, "user_already_exists", "user with this email already exists"
	case errors.Is(err, service.ErrInvalidCredentials):
		status, code, message = fiber.StatusUnauthorized, "invalid_credentials", "invalid email or password"
	case errors.Is(err, service.ErrUnauthorized):
		status, code, message = fiber.StatusUnauthorized, "unauthorized", "unauthorized"
	case errors.Is(err, service.ErrForbidden):
		status, code, message = fiber.StatusForbidden, "forbidden", "forbidden: insufficient permissions or membership"
	case errors.Is(err, service.ErrTokenExpired):
		status, code, message = fiber.StatusUnauthorized, "token_expired", "token has expired"
	}

	return c.Status(status).JSON(envelope{
		Error: &apiError{Code: code, Message: message, RequestID: reqID},
	})
}
*/
