package handler

import (
	"net/http"

	"workhub-service/internal/domain"
	"workhub-service/internal/middleware"
	"workhub-service/internal/service"
)

func (a *API) register(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w, http.MethodPost)
		return
	}
	var in domain.RegisterInput
	if !decodeJSON(w, r, &in) {
		return
	}
	res, err := a.workhub.Register(r.Context(), in)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, envelope{Data: res})
}

func (a *API) login(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w, http.MethodPost)
		return
	}
	var in domain.LoginInput
	if !decodeJSON(w, r, &in) {
		return
	}
	res, err := a.workhub.Login(r.Context(), in)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, envelope{Data: res})
}

func (a *API) refresh(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w, http.MethodPost)
		return
	}
	var in domain.RefreshInput
	// body اختياري — نقرأ فقط إذا وُجد.
	if r.Header.Get("Content-Type") != "" {
		if !decodeJSON(w, r, &in) {
			return
		}
	}
	if in.RefreshToken == "" {
		if cookie, err := r.Cookie("refresh_token"); err == nil {
			in.RefreshToken = cookie.Value
		}
	}
	if in.RefreshToken == "" {
		writeServiceError(w, r, service.ErrInvalidInput)
		return
	}
	res, err := a.workhub.RefreshToken(r.Context(), in.RefreshToken)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, envelope{Data: res})
}

func (a *API) logout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w, http.MethodPost)
		return
	}
	var in domain.LogoutInput
	_ = decodeJSON(w, r, &in)
	if in.RefreshToken == "" {
		if cookie, err := r.Cookie("refresh_token"); err == nil {
			in.RefreshToken = cookie.Value
		}
	}
	if err := a.workhub.Logout(r.Context(), in.RefreshToken); err != nil {
		writeServiceError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) me(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w, http.MethodGet)
		return
	}
	authUser, ok := middleware.UserFromContext(r.Context())
	if !ok {
		writeServiceError(w, r, service.ErrUnauthorized)
		return
	}
	user, err := a.workhub.GetUserByID(r.Context(), authUser.ID, authUser.ID)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, envelope{Data: domain.AuthUser{
		ID:          user.ID,
		Email:       user.Email,
		DisplayName: user.DisplayName,
	}})
}

/*
================================================================================
 النسخة المقابلة باستخدام إطار عمل Fiber (للمقارنة فقط - معلّقة ومظللة)
 FIBER EQUIVALENT IMPLEMENTATION (FOR COMPARISON ONLY - COMMENTED OUT)
================================================================================

import (
	"github.com/gofiber/fiber/v2"
	"workhub-service/internal/domain"
	"workhub-service/internal/middleware"
	"workhub-service/internal/service"
)

func (a *API) registerFiber(c *fiber.Ctx) error {
	var in domain.RegisterInput
	if err := c.BodyParser(&in); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(envelope{
			Error: &apiError{Code: "invalid_input", Message: "cannot parse request body"},
		})
	}

	res, err := a.workhub.Register(c.UserContext(), in)
	if err != nil {
		return writeServiceErrorFiber(c, err)
	}

	return c.Status(fiber.StatusCreated).JSON(envelope{Data: res})
}

func (a *API) loginFiber(c *fiber.Ctx) error {
	var in domain.LoginInput
	if err := c.BodyParser(&in); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(envelope{
			Error: &apiError{Code: "invalid_input", Message: "cannot parse request body"},
		})
	}

	res, err := a.workhub.Login(c.UserContext(), in)
	if err != nil {
		return writeServiceErrorFiber(c, err)
	}

	return c.Status(fiber.StatusOK).JSON(envelope{Data: res})
}

func (a *API) refreshFiber(c *fiber.Ctx) error {
	var in domain.RefreshInput
	_ = c.BodyParser(&in)

	if in.RefreshToken == "" {
		in.RefreshToken = c.Cookies("refresh_token")
	}

	if in.RefreshToken == "" {
		return writeServiceErrorFiber(c, service.ErrInvalidInput)
	}

	res, err := a.workhub.RefreshToken(c.UserContext(), in.RefreshToken)
	if err != nil {
		return writeServiceErrorFiber(c, err)
	}

	return c.Status(fiber.StatusOK).JSON(envelope{Data: res})
}

func (a *API) logoutFiber(c *fiber.Ctx) error {
	var in domain.LogoutInput
	_ = c.BodyParser(&in)

	if in.RefreshToken == "" {
		in.RefreshToken = c.Cookies("refresh_token")
	}

	if err := a.workhub.Logout(c.UserContext(), in.RefreshToken); err != nil {
		return writeServiceErrorFiber(c, err)
	}

	return c.SendStatus(fiber.StatusNoContent)
}

func (a *API) meFiber(c *fiber.Ctx) error {
	authUser, ok := middleware.UserFromFiberContext(c)
	if !ok {
		return writeServiceErrorFiber(c, service.ErrUnauthorized)
	}

	user, err := a.workhub.GetUserByID(c.UserContext(), authUser.ID, authUser.ID)
	if err != nil {
		return writeServiceErrorFiber(c, err)
	}

	return c.Status(fiber.StatusOK).JSON(envelope{Data: domain.AuthUser{
		ID:          user.ID,
		Email:       user.Email,
		DisplayName: user.DisplayName,
	}})
}
*/
