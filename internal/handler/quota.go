package handler

import (
	"net/http"
	"strings"

	"workhub-service/internal/middleware"
	"workhub-service/internal/service"
)

func (a *API) getUserQuota(w http.ResponseWriter, r *http.Request) {
	userID := pathParam(r, "id", "/api/v1/quotas/")
	userID = strings.TrimSpace(userID)
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

	// Users can only read their own quota.
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
}

func (a *API) setUserQuota(w http.ResponseWriter, r *http.Request) {
	// Only Super Admin can modify quotas.
	// For now, no Super Admin role exists -> always reject.
	// Later: replace this check with a.workhub.IsSuperAdmin(authUser.ID).
	writeServiceError(w, r, service.ErrForbidden)
}

// Backward-compatibility wrapper
func (a *API) userQuotas(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		a.getUserQuota(w, r)
	case http.MethodPut:
		a.setUserQuota(w, r)
	default:
		methodNotAllowed(w, http.MethodGet, http.MethodPut)
	}
}

