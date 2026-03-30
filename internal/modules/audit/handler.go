package audit

import (
	"encoding/json"
	"errors"
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
	mux.Handle("GET /v1/audit/logs", auth.RequireAuth(h.authService, http.HandlerFunc(h.handleListLogs)))
}

func (h *Handler) handleListLogs(w http.ResponseWriter, r *http.Request) {
	subject, ok := auth.SubjectFromContext(r.Context())
	if !ok {
		writeEnvelope(w, http.StatusUnauthorized, false, "UNAUTHORIZED", nil)
		return
	}

	filter := Filter{Limit: 50}
	if storeID := r.URL.Query().Get("store_id"); storeID != "" {
		filter.StoreID = &storeID
	}

	if limit := r.URL.Query().Get("limit"); limit != "" {
		parsed, err := strconv.Atoi(limit)
		if err == nil {
			filter.Limit = parsed
		}
	}

	logs, err := h.service.ListLogs(r.Context(), subject, filter)
	if err != nil {
		switch {
		case errors.Is(err, auth.ErrUnauthorized):
			writeEnvelope(w, http.StatusForbidden, false, "FORBIDDEN", nil)
		default:
			writeEnvelope(w, http.StatusInternalServerError, false, "INTERNAL_ERROR", nil)
		}
		return
	}

	writeEnvelope(w, http.StatusOK, true, "SUCCESS", logs)
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
