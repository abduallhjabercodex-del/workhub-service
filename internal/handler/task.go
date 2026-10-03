package handler

import (
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"workhub-service/internal/domain"
	"workhub-service/internal/middleware"
	"workhub-service/internal/service"
)

func (a *API) listProjectTasks(w http.ResponseWriter, r *http.Request) {
	projectID := pathParam(r, "id", "/api/v1/projects/")
	if projectID == "" {
		writeServiceError(w, r, service.ErrInvalidInput)
		return
	}
	authUser, ok := middleware.UserFromContext(r.Context())
	if !ok {
		writeServiceError(w, r, service.ErrUnauthorized)
		return
	}

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
	items, total, err := a.workhub.ListTasks(r.Context(), authUser.ID, projectID,
		r.URL.Query().Get("status"), r.URL.Query().Get("q"),
		r.URL.Query().Get("sort"), page, limit)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, envelope{Data: items, Meta: map[string]any{
		"page": page, "limit": limit, "total": total}})
}

func (a *API) createProjectTask(w http.ResponseWriter, r *http.Request) {
	projectID := pathParam(r, "id", "/api/v1/projects/")
	if projectID == "" {
		writeServiceError(w, r, service.ErrInvalidInput)
		return
	}
	authUser, ok := middleware.UserFromContext(r.Context())
	if !ok {
		writeServiceError(w, r, service.ErrUnauthorized)
		return
	}
	var in domain.CreateTaskInput
	if !decodeJSON(w, r, &in) {
		return
	}
	t, err := a.workhub.CreateTask(r.Context(), authUser.ID, projectID, in)
	if err != nil {
		slog.ErrorContext(r.Context(), "createProjectTask failed", "error", err)
		writeServiceError(w, r, err)
		return
	}
	w.Header().Set("Location", "/api/v1/projects/"+projectID+"/tasks/"+t.ID)
	writeJSON(w, http.StatusCreated, envelope{Data: t})
}

func (a *API) getTask(w http.ResponseWriter, r *http.Request) {
	id := pathParam(r, "id", "/api/v1/tasks/")
	if id == "" || strings.Contains(id, "/") {
		http.NotFound(w, r)
		return
	}
	authUser, ok := middleware.UserFromContext(r.Context())
	if !ok {
		writeServiceError(w, r, service.ErrUnauthorized)
		return
	}
	t, err := a.workhub.GetTask(r.Context(), authUser.ID, id)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, envelope{Data: t})
}

func (a *API) updateTask(w http.ResponseWriter, r *http.Request) {
	id := pathParam(r, "id", "/api/v1/tasks/")
	if id == "" || strings.Contains(id, "/") {
		http.NotFound(w, r)
		return
	}
	authUser, ok := middleware.UserFromContext(r.Context())
	if !ok {
		writeServiceError(w, r, service.ErrUnauthorized)
		return
	}
	var in domain.UpdateTaskInput
	if !decodeJSON(w, r, &in) {
		return
	}
	t, err := a.workhub.UpdateTask(r.Context(), authUser.ID, id, in)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, envelope{Data: t})
}

func (a *API) deleteTask(w http.ResponseWriter, r *http.Request) {
	id := pathParam(r, "id", "/api/v1/tasks/")
	if id == "" || strings.Contains(id, "/") {
		http.NotFound(w, r)
		return
	}
	authUser, ok := middleware.UserFromContext(r.Context())
	if !ok {
		writeServiceError(w, r, service.ErrUnauthorized)
		return
	}
	if err := a.workhub.DeleteTask(r.Context(), authUser.ID, id); err != nil {
		writeServiceError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// Backward-compatibility wrappers (Unused / Dead code)
// func (a *API) taskByID(w http.ResponseWriter, r *http.Request) {
// 	switch r.Method {
// 	case http.MethodGet:
// 		a.getTask(w, r)
// 	case http.MethodPatch:
// 		a.updateTask(w, r)
// 	case http.MethodDelete:
// 		a.deleteTask(w, r)
// 	default:
// 		methodNotAllowed(w, http.MethodGet, http.MethodPatch, http.MethodDelete)
// 	}
// }
//
// func (a *API) tasks(w http.ResponseWriter, r *http.Request, projectID string) {
// 	switch r.Method {
// 	case http.MethodGet:
// 		a.listProjectTasks(w, r)
// 	case http.MethodPost:
// 		a.createProjectTask(w, r)
// 	default:
// 		methodNotAllowed(w, http.MethodGet, http.MethodPost)
// 	}
// }

