package handler

import (
	"net/http"
	"strconv"
	"strings"

	"workhub-service/internal/domain"
	"workhub-service/internal/middleware"
	"workhub-service/internal/service"
)

// taskByID — GET/PATCH/DELETE على /api/v1/tasks/{id}
func (a *API) taskByID(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/api/v1/tasks/")
	if id == "" || strings.Contains(id, "/") {
		http.NotFound(w, r)
		return
	}
	authUser, ok := middleware.UserFromContext(r.Context())
	if !ok {
		writeServiceError(w, r, service.ErrUnauthorized)
		return
	}

	switch r.Method {
	case http.MethodGet:
		t, err := a.workhub.GetTask(r.Context(), authUser.ID, id)
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
		t, err := a.workhub.UpdateTask(r.Context(), authUser.ID, id, in)
		if err != nil {
			writeServiceError(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, envelope{Data: t})
	case http.MethodDelete:
		if err := a.workhub.DeleteTask(r.Context(), authUser.ID, id); err != nil {
			writeServiceError(w, r, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		methodNotAllowed(w, http.MethodGet, http.MethodPatch, http.MethodDelete)
	}
}

// tasks — GET/POST على /api/v1/projects/{id}/tasks
func (a *API) tasks(w http.ResponseWriter, r *http.Request, projectID string) {
	if projectID == "" {
		writeServiceError(w, r, service.ErrInvalidInput)
		return
	}
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
		items, total, err := a.workhub.ListTasks(r.Context(), authUser.ID, projectID,
			r.URL.Query().Get("status"), r.URL.Query().Get("q"),
			r.URL.Query().Get("sort"), page, limit)
		if err != nil {
			writeServiceError(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, envelope{Data: items, Meta: map[string]any{
			"page": page, "limit": limit, "total": total}})
	case http.MethodPost:
		var in domain.CreateTaskInput
		if !decodeJSON(w, r, &in) {
			return
		}
		t, err := a.workhub.CreateTask(r.Context(), authUser.ID, projectID, in)
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

// In Fiber, routes are mapped distinctly:
// api.Get("/tasks/:id", a.getTaskFiber)
// api.Patch("/tasks/:id", a.updateTaskFiber)
// api.Delete("/tasks/:id", a.deleteTaskFiber)
// api.Get("/projects/:id/tasks", a.listTasksFiber)
// api.Post("/projects/:id/tasks", a.createTaskFiber)

func (a *API) getTaskFiber(c *fiber.Ctx) error {
	authUser, ok := middleware.UserFromFiberContext(c)
	if !ok {
		return writeServiceErrorFiber(c, service.ErrUnauthorized)
	}

	t, err := a.workhub.GetTask(c.UserContext(), authUser.ID, c.Params("id"))
	if err != nil {
		return writeServiceErrorFiber(c, err)
	}

	return c.Status(fiber.StatusOK).JSON(envelope{Data: t})
}

func (a *API) updateTaskFiber(c *fiber.Ctx) error {
	authUser, ok := middleware.UserFromFiberContext(c)
	if !ok {
		return writeServiceErrorFiber(c, service.ErrUnauthorized)
	}

	var in domain.UpdateTaskInput
	if err := c.BodyParser(&in); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(envelope{
			Error: &apiError{Code: "invalid_input", Message: "cannot parse request body"},
		})
	}

	t, err := a.workhub.UpdateTask(c.UserContext(), authUser.ID, c.Params("id"), in)
	if err != nil {
		return writeServiceErrorFiber(c, err)
	}

	return c.Status(fiber.StatusOK).JSON(envelope{Data: t})
}

func (a *API) deleteTaskFiber(c *fiber.Ctx) error {
	authUser, ok := middleware.UserFromFiberContext(c)
	if !ok {
		return writeServiceErrorFiber(c, service.ErrUnauthorized)
	}

	if err := a.workhub.DeleteTask(c.UserContext(), authUser.ID, c.Params("id")); err != nil {
		return writeServiceErrorFiber(c, err)
	}

	return c.SendStatus(fiber.StatusNoContent)
}

func (a *API) listTasksFiber(c *fiber.Ctx) error {
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

	projectID := c.Params("id")
	items, total, err := a.workhub.ListTasks(c.UserContext(), authUser.ID, projectID,
		c.Query("status"), c.Query("q"), c.Query("sort"), page, limit)
	if err != nil {
		return writeServiceErrorFiber(c, err)
	}

	return c.Status(fiber.StatusOK).JSON(envelope{
		Data: items,
		Meta: map[string]any{"page": page, "limit": limit, "total": total},
	})
}

func (a *API) createTaskFiber(c *fiber.Ctx) error {
	authUser, ok := middleware.UserFromFiberContext(c)
	if !ok {
		return writeServiceErrorFiber(c, service.ErrUnauthorized)
	}

	var in domain.CreateTaskInput
	if err := c.BodyParser(&in); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(envelope{
			Error: &apiError{Code: "invalid_input", Message: "cannot parse request body"},
		})
	}

	projectID := c.Params("id")
	t, err := a.workhub.CreateTask(c.UserContext(), authUser.ID, projectID, in)
	if err != nil {
		return writeServiceErrorFiber(c, err)
	}

	c.Set("Location", "/api/v1/projects/"+projectID+"/tasks/"+t.ID)
	return c.Status(fiber.StatusCreated).JSON(envelope{Data: t})
}
*/
