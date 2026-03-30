package providercatalog

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/mugiew/onixggr/internal/modules/auth"
)

type Handler struct {
	service     Service
	authService auth.Service
}

func NewHandler(service Service, authService auth.Service) *Handler {
	return &Handler{service: service, authService: authService}
}

func (h *Handler) Register(mux *http.ServeMux) {
	mux.Handle("GET /v1/catalog/providers", auth.RequireAuth(h.authService, h.handleListProviders()))
	mux.Handle("GET /v1/catalog/games", auth.RequireAuth(h.authService, h.handleListGames()))
}

func (h *Handler) handleListProviders() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		providers, err := h.service.ListProviders(r.Context(), ListProvidersFilter{
			Query:  r.URL.Query().Get("query"),
			Status: parseOptionalStatus(r.URL.Query().Get("status")),
			Limit:  parseLimit(r.URL.Query().Get("limit")),
		})
		if err != nil {
			writeEnvelope(w, http.StatusInternalServerError, false, "INTERNAL_ERROR", nil)
			return
		}

		writeEnvelope(w, http.StatusOK, true, "SUCCESS", providers)
	})
}

func (h *Handler) handleListGames() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		games, err := h.service.ListGames(r.Context(), ListGamesFilter{
			ProviderCode: r.URL.Query().Get("provider_code"),
			Query:        r.URL.Query().Get("query"),
			Status:       parseOptionalStatus(r.URL.Query().Get("status")),
			Limit:        parseLimit(r.URL.Query().Get("limit")),
		})
		if err != nil {
			writeEnvelope(w, http.StatusInternalServerError, false, "INTERNAL_ERROR", nil)
			return
		}

		writeEnvelope(w, http.StatusOK, true, "SUCCESS", games)
	})
}

type envelope struct {
	Status  bool   `json:"status"`
	Message string `json:"message"`
	Data    any    `json:"data"`
}

func writeEnvelope(w http.ResponseWriter, status int, ok bool, message string, data any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(envelope{
		Status:  ok,
		Message: message,
		Data:    data,
	})
}

func parseOptionalStatus(raw string) *int {
	if raw == "" {
		return nil
	}

	parsed, err := strconv.Atoi(raw)
	if err != nil {
		return nil
	}

	return &parsed
}

func parseLimit(raw string) int {
	if raw == "" {
		return 0
	}

	parsed, err := strconv.Atoi(raw)
	if err != nil {
		return 0
	}

	return parsed
}
