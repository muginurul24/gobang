package dashboard

import (
	"encoding/json"
	"errors"
	"net/http"

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
	mux.Handle("GET /v1/dashboard/cards", auth.RequireAuth(h.authService, http.HandlerFunc(h.handleGetCards)))
}

func (h *Handler) handleGetCards(w http.ResponseWriter, r *http.Request) {
	subject, ok := auth.SubjectFromContext(r.Context())
	if !ok {
		writeEnvelope(w, http.StatusUnauthorized, false, "UNAUTHORIZED", nil)
		return
	}

	summary, err := h.service.GetSummary(r.Context(), subject)
	if err != nil {
		switch {
		case errors.Is(err, ErrForbidden):
			writeEnvelope(w, http.StatusForbidden, false, "FORBIDDEN", nil)
		default:
			writeEnvelope(w, http.StatusInternalServerError, false, "INTERNAL_ERROR", nil)
		}
		return
	}

	writeEnvelope(w, http.StatusOK, true, "SUCCESS", summary)
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
