package callbacks

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
	mux.Handle("GET /v1/callbacks/queue", auth.RequireAuth(h.authService, http.HandlerFunc(h.handleListQueue)))
	mux.Handle("GET /v1/callbacks/{callbackID}/attempts", auth.RequireAuth(h.authService, http.HandlerFunc(h.handleListAttempts)))
}

func (h *Handler) handleListQueue(w http.ResponseWriter, r *http.Request) {
	subject, ok := auth.SubjectFromContext(r.Context())
	if !ok {
		writeEnvelope(w, http.StatusUnauthorized, false, "UNAUTHORIZED", nil)
		return
	}

	filter := ListQueueFilter{Limit: 25}
	filter.Query = strings.TrimSpace(r.URL.Query().Get("query"))

	if rawStatus := strings.TrimSpace(r.URL.Query().Get("status")); rawStatus != "" {
		status, err := parseCallbackStatus(rawStatus)
		if err != nil {
			writeEnvelope(w, http.StatusBadRequest, false, "INVALID_REQUEST", nil)
			return
		}
		filter.Status = &status
	}

	if rawStoreID := strings.TrimSpace(r.URL.Query().Get("store_id")); rawStoreID != "" {
		filter.StoreID = &rawStoreID
	}

	if rawLimit := strings.TrimSpace(r.URL.Query().Get("limit")); rawLimit != "" {
		limit, err := strconv.Atoi(rawLimit)
		if err != nil {
			writeEnvelope(w, http.StatusBadRequest, false, "INVALID_REQUEST", nil)
			return
		}
		filter.Limit = limit
	}

	if rawOffset := strings.TrimSpace(r.URL.Query().Get("offset")); rawOffset != "" {
		offset, err := strconv.Atoi(rawOffset)
		if err != nil {
			writeEnvelope(w, http.StatusBadRequest, false, "INVALID_REQUEST", nil)
			return
		}
		filter.Offset = offset
	}

	createdFrom, err := parseFilterTime(r.URL.Query().Get("created_from"))
	if err != nil {
		writeEnvelope(w, http.StatusBadRequest, false, "INVALID_REQUEST", nil)
		return
	}
	createdTo, err := parseFilterTime(r.URL.Query().Get("created_to"))
	if err != nil {
		writeEnvelope(w, http.StatusBadRequest, false, "INVALID_REQUEST", nil)
		return
	}

	filter.CreatedFrom = createdFrom
	filter.CreatedTo = createdTo

	page, err := h.service.ListQueue(r.Context(), subject, filter)
	if err != nil {
		writeCallbackError(w, err)
		return
	}

	writeEnvelope(w, http.StatusOK, true, "SUCCESS", page)
}

func (h *Handler) handleListAttempts(w http.ResponseWriter, r *http.Request) {
	subject, ok := auth.SubjectFromContext(r.Context())
	if !ok {
		writeEnvelope(w, http.StatusUnauthorized, false, "UNAUTHORIZED", nil)
		return
	}

	limit := 25
	offset := 0

	if rawLimit := strings.TrimSpace(r.URL.Query().Get("limit")); rawLimit != "" {
		parsed, err := strconv.Atoi(rawLimit)
		if err != nil {
			writeEnvelope(w, http.StatusBadRequest, false, "INVALID_REQUEST", nil)
			return
		}
		limit = parsed
	}
	if rawOffset := strings.TrimSpace(r.URL.Query().Get("offset")); rawOffset != "" {
		parsed, err := strconv.Atoi(rawOffset)
		if err != nil {
			writeEnvelope(w, http.StatusBadRequest, false, "INVALID_REQUEST", nil)
			return
		}
		offset = parsed
	}

	page, err := h.service.ListAttempts(r.Context(), subject, r.PathValue("callbackID"), limit, offset)
	if err != nil {
		writeCallbackError(w, err)
		return
	}

	writeEnvelope(w, http.StatusOK, true, "SUCCESS", page)
}

func parseCallbackStatus(raw string) (Status, error) {
	switch Status(strings.TrimSpace(raw)) {
	case StatusPending:
		return StatusPending, nil
	case StatusRetrying:
		return StatusRetrying, nil
	case StatusSuccess:
		return StatusSuccess, nil
	case StatusFailed:
		return StatusFailed, nil
	default:
		return "", errors.New("invalid callback status")
	}
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

func writeCallbackError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, auth.ErrUnauthorized):
		writeEnvelope(w, http.StatusForbidden, false, "FORBIDDEN", nil)
	case errors.Is(err, ErrNotFound):
		writeEnvelope(w, http.StatusNotFound, false, "NOT_FOUND", nil)
	default:
		writeEnvelope(w, http.StatusInternalServerError, false, "INTERNAL_ERROR", nil)
	}
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
