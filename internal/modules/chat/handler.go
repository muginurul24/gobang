package chat

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
	mux.Handle("GET /v1/chat/messages", auth.RequireAuth(h.authService, http.HandlerFunc(h.handleListMessages)))
	mux.Handle("POST /v1/chat/messages", auth.RequireAuth(h.authService, http.HandlerFunc(h.handleSendMessage)))
	mux.Handle("DELETE /v1/chat/messages/{messageID}", auth.RequireAuth(h.authService, http.HandlerFunc(h.handleDeleteMessage)))
}

func (h *Handler) handleListMessages(w http.ResponseWriter, r *http.Request) {
	subject, ok := auth.SubjectFromContext(r.Context())
	if !ok {
		writeEnvelope(w, http.StatusUnauthorized, false, "UNAUTHORIZED", nil)
		return
	}

	limit := 50
	if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 {
			limit = parsed
		}
	}

	offset := 0
	if raw := strings.TrimSpace(r.URL.Query().Get("offset")); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed >= 0 {
			offset = parsed
		}
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

	messages, err := h.service.ListMessages(r.Context(), subject, ListMessagesFilter{
		Query:       strings.TrimSpace(r.URL.Query().Get("query")),
		Role:        strings.TrimSpace(r.URL.Query().Get("role")),
		CreatedFrom: createdFrom,
		CreatedTo:   createdTo,
		Limit:       limit,
		Offset:      offset,
	})
	if err != nil {
		writeChatError(w, err)
		return
	}

	writeEnvelope(w, http.StatusOK, true, "SUCCESS", messages)
}

func (h *Handler) handleSendMessage(w http.ResponseWriter, r *http.Request) {
	subject, ok := auth.SubjectFromContext(r.Context())
	if !ok {
		writeEnvelope(w, http.StatusUnauthorized, false, "UNAUTHORIZED", nil)
		return
	}

	var input SendMessageInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeEnvelope(w, http.StatusBadRequest, false, "INVALID_REQUEST", nil)
		return
	}

	message, err := h.service.SendMessage(r.Context(), subject, input)
	if err != nil {
		writeChatError(w, err)
		return
	}

	writeEnvelope(w, http.StatusCreated, true, "SUCCESS", message)
}

func (h *Handler) handleDeleteMessage(w http.ResponseWriter, r *http.Request) {
	subject, ok := auth.SubjectFromContext(r.Context())
	if !ok {
		writeEnvelope(w, http.StatusUnauthorized, false, "UNAUTHORIZED", nil)
		return
	}

	message, err := h.service.DeleteMessage(r.Context(), subject, r.PathValue("messageID"))
	if err != nil {
		writeChatError(w, err)
		return
	}

	writeEnvelope(w, http.StatusOK, true, "SUCCESS", message)
}

func writeChatError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrForbidden):
		writeEnvelope(w, http.StatusForbidden, false, "FORBIDDEN", nil)
	case errors.Is(err, ErrNotFound):
		writeEnvelope(w, http.StatusNotFound, false, "NOT_FOUND", nil)
	case errors.Is(err, ErrInvalidBody):
		writeEnvelope(w, http.StatusBadRequest, false, "INVALID_MESSAGE_BODY", nil)
	default:
		writeEnvelope(w, http.StatusInternalServerError, false, "INTERNAL_ERROR", nil)
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
