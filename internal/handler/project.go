package handler

import (
	"net/http"
	"strconv"
	"strings"

	"workhub-service/internal/domain"
	"workhub-service/internal/middleware"
	"workhub-service/internal/service"
)

func (a *API) listProjects(w http.ResponseWriter, r *http.Request) {
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
	offset := (page - 1) * limit

	items, err := a.workhub.ListProjects(r.Context(), authUser.ID,
		r.URL.Query().Get("q"), r.URL.Query().Get("sort"), limit, offset)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	total, _ := a.workhub.CountUserProjects(r.Context(), authUser.ID)
	writeJSON(w, http.StatusOK, envelope{Data: items, Meta: map[string]any{
		"page": page, "limit": limit, "count": len(items), "total": total}})
}

func (a *API) createProject(w http.ResponseWriter, r *http.Request) {
	authUser, ok := middleware.UserFromContext(r.Context())
	if !ok {
		writeServiceError(w, r, service.ErrUnauthorized)
		return
	}

	// Dedicated rate limit for project creation (10/min per user)
	if !a.projectCreationLimiter.Allow("user:" + authUser.ID) {
		w.Header().Set("Retry-After", "60")
		w.Header().Set("X-RateLimit-Limit", "10")
		w.Header().Set("X-RateLimit-Remaining", "0")
		writeJSON(w, http.StatusTooManyRequests, envelope{Error: &apiError{
			Code: "rate_limit_exceeded", Message: "project creation rate limit exceeded"}})
		return
	}
	var in domain.CreateProjectInput
	if !decodeJSON(w, r, &in) {
		return
	}
	p, err := a.workhub.CreateProject(r.Context(), authUser.ID, in)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	w.Header().Set("Location", "/api/v1/projects/"+p.ID)
	writeJSON(w, http.StatusCreated, envelope{Data: p})
}

func (a *API) getProject(w http.ResponseWriter, r *http.Request) {
	projectID := pathParam(r, "id", "/api/v1/projects/")
	if projectID == "" {
		http.NotFound(w, r)
		return
	}
	authUser, ok := middleware.UserFromContext(r.Context())
	if !ok {
		writeServiceError(w, r, service.ErrUnauthorized)
		return
	}
	proj, err := a.workhub.GetProject(r.Context(), authUser.ID, projectID)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, envelope{Data: proj})
}

func (a *API) deleteProject(w http.ResponseWriter, r *http.Request) {
	projectID := pathParam(r, "id", "/api/v1/projects/")
	if projectID == "" {
		http.NotFound(w, r)
		return
	}
	authUser, ok := middleware.UserFromContext(r.Context())
	if !ok {
		writeServiceError(w, r, service.ErrUnauthorized)
		return
	}
	if err := a.workhub.DeleteProject(r.Context(), authUser.ID, projectID); err != nil {
		writeServiceError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) getProjectSummary(w http.ResponseWriter, r *http.Request) {
	projectID := pathParam(r, "id", "/api/v1/projects/")
	if projectID == "" {
		http.NotFound(w, r)
		return
	}
	authUser, ok := middleware.UserFromContext(r.Context())
	if !ok {
		writeServiceError(w, r, service.ErrUnauthorized)
		return
	}
	summary, err := a.workhub.ProjectSummary(r.Context(), authUser.ID, projectID)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, envelope{Data: summary})
}

func (a *API) listProjectMembers(w http.ResponseWriter, r *http.Request) {
	projectID := pathParam(r, "id", "/api/v1/projects/")
	if projectID == "" {
		http.NotFound(w, r)
		return
	}
	authUser, ok := middleware.UserFromContext(r.Context())
	if !ok {
		writeServiceError(w, r, service.ErrUnauthorized)
		return
	}
	members, err := a.workhub.ListProjectMembers(r.Context(), authUser.ID, projectID)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, envelope{Data: members, Meta: map[string]any{
		"total": len(members)}})
}

func (a *API) addProjectMember(w http.ResponseWriter, r *http.Request) {
	projectID := pathParam(r, "id", "/api/v1/projects/")
	if projectID == "" {
		http.NotFound(w, r)
		return
	}
	authUser, ok := middleware.UserFromContext(r.Context())
	if !ok {
		writeServiceError(w, r, service.ErrUnauthorized)
		return
	}
	var in domain.AddMemberInput
	if !decodeJSON(w, r, &in) {
		return
	}
	if err := a.workhub.AddProjectMember(r.Context(), authUser.ID,
		projectID, in.UserID, in.Role); err != nil {
		writeServiceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, envelope{Meta: map[string]string{
		"message": "member added successfully"}})
}

func (a *API) removeProjectMember(w http.ResponseWriter, r *http.Request) {
	projectID := pathParam(r, "id", "/api/v1/projects/")
	targetUserID := r.PathValue("userId")
	if targetUserID == "" {
		parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
		if len(parts) >= 6 {
			targetUserID = parts[5]
		}
	}
	if projectID == "" || targetUserID == "" {
		http.NotFound(w, r)
		return
	}
	authUser, ok := middleware.UserFromContext(r.Context())
	if !ok {
		writeServiceError(w, r, service.ErrUnauthorized)
		return
	}
	if err := a.workhub.RemoveProjectMember(r.Context(), authUser.ID,
		projectID, targetUserID); err != nil {
		writeServiceError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// Backward-compatibility wrappers (Unused / Dead code)
// func (a *API) projects(w http.ResponseWriter, r *http.Request) {
// 	switch r.Method {
// 	case http.MethodGet:
// 		a.listProjects(w, r)
// 	case http.MethodPost:
// 		a.createProject(w, r)
// 	default:
// 		methodNotAllowed(w, http.MethodGet, http.MethodPost)
// 	}
// }
//
// func (a *API) projectSubresource(w http.ResponseWriter, r *http.Request) {
// 	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
// 	if len(parts) < 4 || parts[0] != "api" || parts[1] != "v1" || parts[2] != "projects" || parts[3] == "" {
// 		http.NotFound(w, r)
// 		return
// 	}
//
// 	if len(parts) == 4 {
// 		switch r.Method {
// 		case http.MethodGet:
// 			a.getProject(w, r)
// 		case http.MethodDelete:
// 			a.deleteProject(w, r)
// 		default:
// 			methodNotAllowed(w, http.MethodGet, http.MethodDelete)
// 		}
// 		return
// 	}
//
// 	switch parts[4] {
// 	case "summary":
// 		if len(parts) != 5 || r.Method != http.MethodGet {
// 			methodNotAllowed(w, http.MethodGet)
// 			return
// 		}
// 		a.getProjectSummary(w, r)
// 	case "members":
// 		if len(parts) == 5 {
// 			switch r.Method {
// 			case http.MethodGet:
// 				a.listProjectMembers(w, r)
// 			case http.MethodPost:
// 				a.addProjectMember(w, r)
// 			default:
// 				methodNotAllowed(w, http.MethodGet, http.MethodPost)
// 			}
// 			return
// 		}
// 		if len(parts) == 6 && r.Method == http.MethodDelete {
// 			a.removeProjectMember(w, r)
// 			return
// 		}
// 	case "audit":
// 		if len(parts) != 5 || r.Method != http.MethodGet {
// 			methodNotAllowed(w, http.MethodGet)
// 			return
// 		}
// 		a.listProjectAudit(w, r)
// 	case "tasks":
// 		if len(parts) == 5 {
// 			switch r.Method {
// 			case http.MethodGet:
// 				a.listProjectTasks(w, r)
// 			case http.MethodPost:
// 				a.createProjectTask(w, r)
// 			default:
// 				methodNotAllowed(w, http.MethodGet, http.MethodPost)
// 			}
// 			return
// 		}
// 		http.NotFound(w, r)
// 	default:
// 		http.NotFound(w, r)
// 	}
// }

func (a *API) listProjectAudit(w http.ResponseWriter, r *http.Request) {
	projectID := pathParam(r, "id", "/api/v1/projects/")
	if projectID == "" {
		http.NotFound(w, r)
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
		limit = 50
	}
	if limit > 100 {
		limit = 100
	}
	offset := (page - 1) * limit

	events, total, err := a.workhub.ListProjectAudit(r.Context(), authUser.ID, projectID, limit, offset)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, envelope{
		Data: events,
		Meta: map[string]any{
			"page":  page,
			"limit": limit,
			"count": len(events),
			"total": total,
		},
	})
}

