package handler

import (
	"net/http"
	"strings"

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
	// body is optional — read only if present.
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
	if strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
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

