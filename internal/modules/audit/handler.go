package audit

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

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
	if action := r.URL.Query().Get("action"); action != "" {
		filter.Action = &action
	}
	if actorRole := r.URL.Query().Get("actor_role"); actorRole != "" {
		filter.ActorRole = &actorRole
	}
	if targetType := r.URL.Query().Get("target_type"); targetType != "" {
		filter.TargetType = &targetType
	}

	if limit := r.URL.Query().Get("limit"); limit != "" {
		parsed, err := strconv.Atoi(limit)
		if err == nil {
			filter.Limit = parsed
		}
	}
	if offset := r.URL.Query().Get("offset"); offset != "" {
		parsed, err := strconv.Atoi(offset)
		if err == nil {
			filter.Offset = parsed
		}
	}
	if createdFrom, err := parseFilterTime(r.URL.Query().Get("created_from")); err == nil {
		filter.CreatedFrom = createdFrom
	} else {
		writeEnvelope(w, http.StatusBadRequest, false, "INVALID_REQUEST", nil)
		return
	}
	if createdTo, err := parseFilterTime(r.URL.Query().Get("created_to")); err == nil {
		filter.CreatedTo = createdTo
	} else {
		writeEnvelope(w, http.StatusBadRequest, false, "INVALID_REQUEST", nil)
		return
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

func parseFilterTime(raw string) (*time.Time, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil, nil
	}

	layouts := []string{time.RFC3339, "2006-01-02T15:04", "2006-01-02"}
	for _, layout := range layouts {
		parsed, err := time.Parse(layout, trimmed)
		if err == nil {
			value := parsed.UTC()
			return &value, nil
		}
	}

	return nil, errors.New("invalid time filter")
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
