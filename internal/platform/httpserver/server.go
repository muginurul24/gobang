package httpserver

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/mugiew/onixggr/internal/platform/config"
	"github.com/mugiew/onixggr/internal/platform/health"
	"github.com/mugiew/onixggr/internal/platform/middleware"
)

type Dependencies struct {
	Health health.Service
	Logger *slog.Logger
}

type infoResponse struct {
	Name  string   `json:"name"`
	Apps  []string `json:"apps"`
	Docs  []string `json:"docs"`
	Ready bool     `json:"ready"`
}

func NewHandler(cfg config.Config, deps Dependencies) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, deps.Health.Liveness())
	})

	mux.HandleFunc("GET /health/live", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, deps.Health.Liveness())
	})

	mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, r *http.Request) {
		report := deps.Health.Readiness(r.Context())
		status := http.StatusOK
		if report.Status != "ready" {
			status = http.StatusServiceUnavailable
		}

		writeJSON(w, status, report)
	})

	mux.HandleFunc("GET /health/ready", func(w http.ResponseWriter, r *http.Request) {
		report := deps.Health.Readiness(r.Context())
		status := http.StatusOK
		if report.Status != "ready" {
			status = http.StatusServiceUnavailable
		}

		writeJSON(w, status, report)
	})

	mux.HandleFunc("GET /v1/ping", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{
			"message": "pong",
		})
	})

	mux.HandleFunc("GET /", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, infoResponse{
			Name:  cfg.App.Name,
			Apps:  []string{"apps/api", "apps/worker", "apps/scheduler", "apps/web"},
			Docs:  []string{"docs/blueprint.md", "docs/database-final.md", "docs/nexusggr-openapi-3.1.yaml"},
			Ready: true,
		})
	})

	return middleware.RequestID(middleware.Logging(deps.Logger, mux))
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}
