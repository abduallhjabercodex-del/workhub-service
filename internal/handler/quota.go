package handler

import (
	"net/http"
	"strings"

	"workhub-service/internal/middleware"
	"workhub-service/internal/service"
)

func (a *API) userQuotas(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/quotas/")
	userID := strings.TrimSpace(path)
	if userID == "" {
		writeJSON(w, http.StatusBadRequest, envelope{Error: &apiError{
			Code: "invalid_input", Message: "user ID required"}})
		return
	}

	authUser, ok := middleware.UserFromContext(r.Context())
	if !ok {
		writeServiceError(w, r, service.ErrUnauthorized)
		return
	}

	switch r.Method {
	case http.MethodGet:
		// المستخدم يقرأ quota نفسه فقط.
		if authUser.ID != userID {
			writeServiceError(w, r, service.ErrForbidden)
			return
		}
		quota, err := a.workhub.GetUserQuota(r.Context(), authUser.ID, userID)
		if err != nil {
			writeServiceError(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, envelope{Data: quota})

	case http.MethodPut:
		// ⚠️ فقط Super Admin يمكنه تعديل quotas.
		//    حتى الآن، لا يوجد Super Admin → ارفض دائمًا.
		//    لاحقًا: استبدل هذا الفحص بـ a.workhub.IsSuperAdmin(authUser.ID).
		writeServiceError(w, r, service.ErrForbidden)
		return

		// الكود المستقبلي:
		// var in domain.UserQuota
		// if !decodeJSON(w, r, &in) { return }
		// in.UserID = userID
		// if err := a.workhub.SetUserQuota(r.Context(), authUser.ID, in); err != nil {
		//     writeServiceError(w, r, err)
		//     return
		// }
		// writeJSON(w, http.StatusOK, envelope{Data: in})

	default:
		methodNotAllowed(w, http.MethodGet, http.MethodPut)
	}
}

/*
================================================================================
 النسخة المقابلة باستخدام إطار عمل Fiber (للمقارنة فقط - معلّقة ومظللة)
 FIBER EQUIVALENT IMPLEMENTATION (FOR COMPARISON ONLY - COMMENTED OUT)
================================================================================

import (
	"strings"
	"github.com/gofiber/fiber/v2"
	"workhub-service/internal/middleware"
	"workhub-service/internal/service"
)

// In Fiber, routes are registered distinctly:
// router.Get("/quotas/:id", a.getUserQuotaFiber)
// router.Put("/quotas/:id", a.putUserQuotaFiber)

func (a *API) getUserQuotaFiber(c *fiber.Ctx) error {
	userID := strings.TrimSpace(c.Params("id"))
	if userID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(envelope{
			Error: &apiError{Code: "invalid_input", Message: "user ID required"},
		})
	}

	authUser, ok := middleware.UserFromFiberContext(c)
	if !ok {
		return writeServiceErrorFiber(c, service.ErrUnauthorized)
	}

	if authUser.ID != userID {
		return writeServiceErrorFiber(c, service.ErrForbidden)
	}

	quota, err := a.workhub.GetUserQuota(c.UserContext(), authUser.ID, userID)
	if err != nil {
		return writeServiceErrorFiber(c, err)
	}

	return c.Status(fiber.StatusOK).JSON(envelope{Data: quota})
}

func (a *API) putUserQuotaFiber(c *fiber.Ctx) error {
	return writeServiceErrorFiber(c, service.ErrForbidden)
}
*/
