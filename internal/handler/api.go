package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"workhub-service/internal/domain"
	"workhub-service/internal/service"
)

type API struct{ workhub *service.WorkHub }

func NewAPI(workhub *service.WorkHub) *API { return &API{workhub: workhub} }

func (a *API) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", Health)
	// Swagger UI & OpenAPI spec
	mux.HandleFunc("/api/v1/docs", ServeSwaggerUI)
	mux.HandleFunc("/api/v1/docs/openapi.yaml", ServeOpenAPISpec)
	// Business endpoints
	mux.HandleFunc("/api/v1/projects", a.projects)
	mux.HandleFunc("/api/v1/projects/", a.projectSubresource)
	mux.HandleFunc("/api/v1/tasks/", a.taskByID)
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

func (a *API) projects(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		items, err := a.workhub.ListProjects(r.Context(), r.URL.Query().Get("q"), r.URL.Query().Get("sort"))
		if err != nil {
			writeServiceError(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, envelope{Data: items, Meta: map[string]any{"total": len(items)}})
	case http.MethodPost:
		var in domain.CreateProjectInput
		if !decodeJSON(w, r, &in) {
			return
		}
		p, err := a.workhub.CreateProject(r.Context(), in)
		if err != nil {
			writeServiceError(w, r, err)
			return
		}
		w.Header().Set("Location", "/api/v1/projects/"+p.ID)
		writeJSON(w, http.StatusCreated, envelope{Data: p})
	default:
		methodNotAllowed(w, http.MethodGet, http.MethodPost)
	}
}

func (a *API) projectSubresource(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if len(parts) == 5 && parts[0] == "api" && parts[1] == "v1" && parts[2] == "projects" && parts[4] == "summary" {
		if r.Method != http.MethodGet {
			methodNotAllowed(w, http.MethodGet)
			return
		}
		summary, err := a.workhub.ProjectSummary(r.Context(), parts[3])
		if err != nil {
			writeServiceError(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, envelope{Data: summary})
		return
	}
	if len(parts) != 5 || parts[0] != "api" || parts[1] != "v1" || parts[2] != "projects" || parts[3] == "" || parts[4] != "tasks" {
		http.NotFound(w, r)
		return
	}
	projectID := parts[3]
	if r.URL.Path == "/api/v1/projects/"+projectID+"/tasks" {
		a.tasks(w, r, projectID)
		return
	}
	http.NotFound(w, r)
}

func (a *API) taskByID(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/api/v1/tasks/")
	if id == "" || strings.Contains(id, "/") {
		http.NotFound(w, r)
		return
	}
	switch r.Method {
	case http.MethodGet:
		t, err := a.workhub.GetTask(r.Context(), id)
		if err != nil {
			writeServiceError(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, envelope{Data: t})
	case http.MethodPatch:
		var in domain.UpdateTaskInput
		if !decodeJSON(w, r, &in) {
			return
		}
		t, err := a.workhub.UpdateTask(r.Context(), id, in)
		if err != nil {
			writeServiceError(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, envelope{Data: t})
	default:
		methodNotAllowed(w, http.MethodGet, http.MethodPatch)
	}
}

func (a *API) tasks(w http.ResponseWriter, r *http.Request, projectID string) {
	switch r.Method {
	case http.MethodGet:
		page, _ := strconv.Atoi(r.URL.Query().Get("page"))
		if page < 1 {
			page = 1
		}
		limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
		if limit < 1 {
			limit = 20
		}
		if limit > 100 {
			limit = 100
		}
		items, total, err := a.workhub.ListTasks(r.Context(), projectID, r.URL.Query().Get("status"), r.URL.Query().Get("q"), r.URL.Query().Get("sort"), page, limit)
		if err != nil {
			writeServiceError(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, envelope{Data: items, Meta: map[string]any{"page": page, "limit": limit, "total": total}})
	case http.MethodPost:
		var in domain.CreateTaskInput
		if !decodeJSON(w, r, &in) {
			return
		}
		t, err := a.workhub.CreateTask(r.Context(), projectID, in)
		if err != nil {
			writeServiceError(w, r, err)
			return
		}
		w.Header().Set("Location", "/api/v1/projects/"+projectID+"/tasks/"+t.ID)
		writeJSON(w, http.StatusCreated, envelope{Data: t})
	default:
		methodNotAllowed(w, http.MethodGet, http.MethodPost)
	}
}

func writeServiceError(w http.ResponseWriter, r *http.Request, err error) {
	status, code, message := http.StatusInternalServerError, "internal_error", "internal server error"
	switch {
	case errors.Is(err, service.ErrInvalidInput):
		status, code, message = http.StatusBadRequest, "invalid_input", "request validation failed"
	case errors.Is(err, service.ErrProjectNotFound):
		status, code, message = http.StatusNotFound, "project_not_found", "project not found"
	case errors.Is(err, service.ErrTaskNotFound):
		status, code, message = http.StatusNotFound, "task_not_found", "task not found"
	}
	writeJSON(w, status, envelope{Error: &apiError{Code: code, Message: message, RequestID: requestID(r)}})
}

func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) bool {
	if ct := r.Header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		writeJSON(w, http.StatusUnsupportedMediaType, envelope{Error: &apiError{Code: "unsupported_media_type", Message: "Content-Type must be application/json"}})
		return false
	}
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		writeJSON(w, http.StatusBadRequest, envelope{Error: &apiError{Code: "invalid_json", Message: "request body must be valid JSON"}})
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
	http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
}
func requestID(r *http.Request) string { return r.Header.Get("X-Request-ID") }
