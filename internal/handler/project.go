package handler

import (
	"net/http"
	"strconv"
	"strings"

	"workhub-service/internal/domain"
	"workhub-service/internal/middleware"
	"workhub-service/internal/service"
)

func (a *API) projects(w http.ResponseWriter, r *http.Request) {
	authUser, ok := middleware.UserFromContext(r.Context())
	if !ok {
		writeServiceError(w, r, service.ErrUnauthorized)
		return
	}

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
		offset := (page - 1) * limit

		items, err := a.workhub.ListProjects(r.Context(), authUser.ID,
			r.URL.Query().Get("q"), r.URL.Query().Get("sort"), limit, offset)
		if err != nil {
			writeServiceError(w, r, err)
			return
		}
		// ⚠️ total يحتاج count(*) من Service. مؤقتًا نُعيد عدد العناصر.
		//    TODO: أضف ListProjectsCount في Service.
		writeJSON(w, http.StatusOK, envelope{Data: items, Meta: map[string]any{
			"page": page, "limit": limit, "count": len(items)}})

	case http.MethodPost:
		// Rate Limit مخصص لإنشاء المشاريع (10/min per user)
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

	default:
		methodNotAllowed(w, http.MethodGet, http.MethodPost)
	}
}

func (a *API) projectSubresource(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if len(parts) < 4 || parts[0] != "api" || parts[1] != "v1" || parts[2] != "projects" || parts[3] == "" {
		http.NotFound(w, r)
		return
	}
	projectID := parts[3]

	authUser, ok := middleware.UserFromContext(r.Context())
	if !ok {
		writeServiceError(w, r, service.ErrUnauthorized)
		return
	}

	if len(parts) == 4 {
		switch r.Method {
		case http.MethodGet:
			proj, err := a.workhub.GetProject(r.Context(), authUser.ID, projectID)
			if err != nil {
				writeServiceError(w, r, err)
				return
			}
			writeJSON(w, http.StatusOK, envelope{Data: proj})
			return
		case http.MethodDelete:
			err := a.workhub.DeleteProject(r.Context(), authUser.ID, projectID)
			if err != nil {
				writeServiceError(w, r, err)
				return
			}
			w.WriteHeader(http.StatusNoContent)
			return
		default:
			methodNotAllowed(w, http.MethodGet, http.MethodDelete)
			return
		}
	}

	subresource := parts[4]

	switch subresource {
	case "summary":
		if len(parts) != 5 || r.Method != http.MethodGet {
			methodNotAllowed(w, http.MethodGet)
			return
		}
		summary, err := a.workhub.ProjectSummary(r.Context(), authUser.ID, projectID)
		if err != nil {
			writeServiceError(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, envelope{Data: summary})
		return

	case "members":
		if len(parts) == 5 {
			switch r.Method {
			case http.MethodGet:
				members, err := a.workhub.ListProjectMembers(r.Context(), authUser.ID, projectID)
				if err != nil {
					writeServiceError(w, r, err)
					return
				}
				writeJSON(w, http.StatusOK, envelope{Data: members, Meta: map[string]any{
					"total": len(members)}})
			case http.MethodPost:
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
			default:
				methodNotAllowed(w, http.MethodGet, http.MethodPost)
			}
			return
		}
		if len(parts) == 6 && r.Method == http.MethodDelete {
			targetUserID := parts[5]
			if err := a.workhub.RemoveProjectMember(r.Context(), authUser.ID,
				projectID, targetUserID); err != nil {
				writeServiceError(w, r, err)
				return
			}
			w.WriteHeader(http.StatusNoContent)
			return
		}

	case "tasks":
		if len(parts) == 5 {
			a.tasks(w, r, projectID)
			return
		}
	}
	http.NotFound(w, r)
}

/*
================================================================================
 النسخة المقابلة باستخدام إطار عمل Fiber (للمقارنة فقط - معلّقة ومظللة)
 FIBER EQUIVALENT IMPLEMENTATION (FOR COMPARISON ONLY - COMMENTED OUT)
================================================================================

import (
	"strconv"
	"github.com/gofiber/fiber/v2"
	"workhub-service/internal/domain"
	"workhub-service/internal/middleware"
	"workhub-service/internal/service"
)

// In Fiber, routes are mapped cleanly without manual URL parsing:
// projects := api.Group("/projects")
// projects.Get("/", a.listProjectsFiber)
// projects.Post("/", a.createProjectFiber)
// projects.Get("/:id", a.getProjectFiber)
// projects.Delete("/:id", a.deleteProjectFiber)
// projects.Get("/:id/summary", a.getProjectSummaryFiber)
// projects.Get("/:id/members", a.listProjectMembersFiber)
// projects.Post("/:id/members", a.addProjectMemberFiber)
// projects.Delete("/:id/members/:userId", a.removeProjectMemberFiber)

func (a *API) listProjectsFiber(c *fiber.Ctx) error {
	authUser, ok := middleware.UserFromFiberContext(c)
	if !ok {
		return writeServiceErrorFiber(c, service.ErrUnauthorized)
	}

	page, _ := strconv.Atoi(c.Query("page", "1"))
	if page < 1 {
		page = 1
	}
	limit, _ := strconv.Atoi(c.Query("limit", "20"))
	if limit < 1 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	offset := (page - 1) * limit

	items, err := a.workhub.ListProjects(c.UserContext(), authUser.ID,
		c.Query("q"), c.Query("sort"), limit, offset)
	if err != nil {
		return writeServiceErrorFiber(c, err)
	}

	return c.Status(fiber.StatusOK).JSON(envelope{
		Data: items,
		Meta: map[string]any{"page": page, "limit": limit, "count": len(items)},
	})
}

func (a *API) createProjectFiber(c *fiber.Ctx) error {
	authUser, ok := middleware.UserFromFiberContext(c)
	if !ok {
		return writeServiceErrorFiber(c, service.ErrUnauthorized)
	}

	if !a.projectCreationLimiter.Allow("user:" + authUser.ID) {
		c.Set("Retry-After", "60")
		c.Set("X-RateLimit-Limit", "10")
		c.Set("X-RateLimit-Remaining", "0")
		return c.Status(fiber.StatusTooManyRequests).JSON(envelope{
			Error: &apiError{Code: "rate_limit_exceeded", Message: "project creation rate limit exceeded"},
		})
	}

	var in domain.CreateProjectInput
	if err := c.BodyParser(&in); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(envelope{
			Error: &apiError{Code: "invalid_input", Message: "cannot parse request body"},
		})
	}

	p, err := a.workhub.CreateProject(c.UserContext(), authUser.ID, in)
	if err != nil {
		return writeServiceErrorFiber(c, err)
	}

	c.Set("Location", "/api/v1/projects/"+p.ID)
	return c.Status(fiber.StatusCreated).JSON(envelope{Data: p})
}

func (a *API) getProjectFiber(c *fiber.Ctx) error {
	authUser, ok := middleware.UserFromFiberContext(c)
	if !ok {
		return writeServiceErrorFiber(c, service.ErrUnauthorized)
	}

	proj, err := a.workhub.GetProject(c.UserContext(), authUser.ID, c.Params("id"))
	if err != nil {
		return writeServiceErrorFiber(c, err)
	}

	return c.Status(fiber.StatusOK).JSON(envelope{Data: proj})
}

func (a *API) deleteProjectFiber(c *fiber.Ctx) error {
	authUser, ok := middleware.UserFromFiberContext(c)
	if !ok {
		return writeServiceErrorFiber(c, service.ErrUnauthorized)
	}

	if err := a.workhub.DeleteProject(c.UserContext(), authUser.ID, c.Params("id")); err != nil {
		return writeServiceErrorFiber(c, err)
	}

	return c.SendStatus(fiber.StatusNoContent)
}

func (a *API) getProjectSummaryFiber(c *fiber.Ctx) error {
	authUser, ok := middleware.UserFromFiberContext(c)
	if !ok {
		return writeServiceErrorFiber(c, service.ErrUnauthorized)
	}

	summary, err := a.workhub.ProjectSummary(c.UserContext(), authUser.ID, c.Params("id"))
	if err != nil {
		return writeServiceErrorFiber(c, err)
	}

	return c.Status(fiber.StatusOK).JSON(envelope{Data: summary})
}

func (a *API) listProjectMembersFiber(c *fiber.Ctx) error {
	authUser, ok := middleware.UserFromFiberContext(c)
	if !ok {
		return writeServiceErrorFiber(c, service.ErrUnauthorized)
	}

	members, err := a.workhub.ListProjectMembers(c.UserContext(), authUser.ID, c.Params("id"))
	if err != nil {
		return writeServiceErrorFiber(c, err)
	}

	return c.Status(fiber.StatusOK).JSON(envelope{
		Data: members,
		Meta: map[string]any{"total": len(members)},
	})
}

func (a *API) addProjectMemberFiber(c *fiber.Ctx) error {
	authUser, ok := middleware.UserFromFiberContext(c)
	if !ok {
		return writeServiceErrorFiber(c, service.ErrUnauthorized)
	}

	var in domain.AddMemberInput
	if err := c.BodyParser(&in); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(envelope{
			Error: &apiError{Code: "invalid_input", Message: "cannot parse request body"},
		})
	}

	if err := a.workhub.AddProjectMember(c.UserContext(), authUser.ID, c.Params("id"), in.UserID, in.Role); err != nil {
		return writeServiceErrorFiber(c, err)
	}

	return c.Status(fiber.StatusCreated).JSON(envelope{
		Meta: map[string]string{"message": "member added successfully"},
	})
}

func (a *API) removeProjectMemberFiber(c *fiber.Ctx) error {
	authUser, ok := middleware.UserFromFiberContext(c)
	if !ok {
		return writeServiceErrorFiber(c, service.ErrUnauthorized)
	}

	if err := a.workhub.RemoveProjectMember(c.UserContext(), authUser.ID, c.Params("id"), c.Params("userId")); err != nil {
		return writeServiceErrorFiber(c, err)
	}

	return c.SendStatus(fiber.StatusNoContent)
}
*/
