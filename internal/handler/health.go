package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"sync/atomic"
	"time"

	"workhub-service/internal/infrastructure/version"
)

type HealthResponse struct {
	Status  string `json:"status"`
	Version string `json:"version"`
	Time    string `json:"time"`
}

type ReadyResponse struct {
	Status string `json:"status"`
	Time   string `json:"time"`
	Error  string `json:"error,omitempty"`
}

type AppHealth struct {
	isReady           atomic.Bool
	isStartupComplete atomic.Bool
}

func NewAppHealth() *AppHealth {
	return &AppHealth{}
}

func (h *AppHealth) MarkReady(ready bool) {
	h.isReady.Store(ready)
}

func (h *AppHealth) MarkStartupComplete(complete bool) {
	h.isStartupComplete.Store(complete)
}

func (h *AppHealth) IsReady() bool {
	return h.isReady.Load()
}

func (h *AppHealth) IsStartupComplete() bool {
	return h.isStartupComplete.Load()
}

func Health(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(HealthResponse{
		Status:  "ok",
		Version: version.Version,
		Time:    time.Now().UTC().Format(time.RFC3339),
	})
}

func (a *API) Ready(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	w.Header().Set("Content-Type", "application/json")

	if !a.health.IsReady() {
		w.WriteHeader(http.StatusServiceUnavailable)
		_ = json.NewEncoder(w).Encode(ReadyResponse{
			Status: "unavailable",
			Time:   time.Now().UTC().Format(time.RFC3339),
			Error:  "server is shutting down or not ready",
		})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()

	if err := a.workhub.Ping(ctx); err != nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		_ = json.NewEncoder(w).Encode(ReadyResponse{
			Status: "unavailable",
			Time:   time.Now().UTC().Format(time.RFC3339),
			Error:  err.Error(),
		})
		return
	}

	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(ReadyResponse{
		Status: "ok",
		Time:   time.Now().UTC().Format(time.RFC3339),
	})
}

func (a *API) Startup(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	w.Header().Set("Content-Type", "application/json")

	if !a.health.IsStartupComplete() {
		w.WriteHeader(http.StatusServiceUnavailable)
		_ = json.NewEncoder(w).Encode(HealthResponse{
			Status: "starting",
			Time:   time.Now().UTC().Format(time.RFC3339),
		})
		return
	}

	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(HealthResponse{
		Status: "ok",
		Time:   time.Now().UTC().Format(time.RFC3339),
	})
}

