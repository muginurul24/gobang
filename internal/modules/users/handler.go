package users

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
	mux.Handle("GET /v1/users/directory", auth.RequireAuth(h.authService, http.HandlerFunc(h.handleListDirectory)))
	mux.Handle("POST /v1/users", auth.RequireAuth(h.authService, http.HandlerFunc(h.handleCreateUser)))
	mux.Handle("PATCH /v1/users/{userID}/status", auth.RequireAuth(h.authService, http.HandlerFunc(h.handleUpdateUserStatus)))
}

func (h *Handler) handleListDirectory(w http.ResponseWriter, r *http.Request) {
	subject, ok := auth.SubjectFromContext(r.Context())
	if !ok {
		writeEnvelope(w, http.StatusUnauthorized, false, "UNAUTHORIZED", nil)
		return
	}

	filter := ListFilter{Limit: 12}
	if query := strings.TrimSpace(r.URL.Query().Get("query")); query != "" {
		filter.Query = query
	}
	if role := strings.TrimSpace(r.URL.Query().Get("role")); role != "" {
		parsedRole := auth.Role(strings.ToLower(role))
		filter.Role = &parsedRole
	}
	if rawIsActive := strings.TrimSpace(r.URL.Query().Get("is_active")); rawIsActive != "" {
		value, err := strconv.ParseBool(rawIsActive)
		if err != nil {
			writeEnvelope(w, http.StatusBadRequest, false, "INVALID_REQUEST", nil)
			return
		}
		filter.IsActive = &value
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

	page, err := h.service.ListDirectory(r.Context(), subject, filter)
	if err != nil {
		writeUserError(w, err)
		return
	}

	writeEnvelope(w, http.StatusOK, true, "SUCCESS", page)
}

func (h *Handler) handleCreateUser(w http.ResponseWriter, r *http.Request) {
	subject, ok := auth.SubjectFromContext(r.Context())
	if !ok {
		writeEnvelope(w, http.StatusUnauthorized, false, "UNAUTHORIZED", nil)
		return
	}

	var input CreateUserInput
	if err := decodeJSONBody(r, &input); err != nil {
		writeEnvelope(w, http.StatusBadRequest, false, "INVALID_REQUEST", nil)
		return
	}

	user, err := h.service.CreateUser(r.Context(), subject, input, requestMetadata(r))
	if err != nil {
		writeUserError(w, err)
		return
	}

	writeEnvelope(w, http.StatusCreated, true, "SUCCESS", user)
}

func (h *Handler) handleUpdateUserStatus(w http.ResponseWriter, r *http.Request) {
	subject, ok := auth.SubjectFromContext(r.Context())
	if !ok {
		writeEnvelope(w, http.StatusUnauthorized, false, "UNAUTHORIZED", nil)
		return
	}

	var input UpdateUserStatusInput
	if err := decodeJSONBody(r, &input); err != nil {
		writeEnvelope(w, http.StatusBadRequest, false, "INVALID_REQUEST", nil)
		return
	}

	user, err := h.service.UpdateUserStatus(r.Context(), subject, r.PathValue("userID"), input, requestMetadata(r))
	if err != nil {
		writeUserError(w, err)
		return
	}

	writeEnvelope(w, http.StatusOK, true, "SUCCESS", user)
}

func requestMetadata(r *http.Request) auth.RequestMetadata {
	ipAddress := strings.TrimSpace(r.Header.Get("X-Forwarded-For"))
	if ipAddress == "" {
		ipAddress = strings.TrimSpace(r.RemoteAddr)
	}

	return auth.RequestMetadata{
		IPAddress: ipAddress,
		UserAgent: strings.TrimSpace(r.UserAgent()),
	}
}

func decodeJSONBody(r *http.Request, dst any) error {
	defer r.Body.Close()
	return json.NewDecoder(r.Body).Decode(dst)
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

func writeUserError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrForbidden):
		writeEnvelope(w, http.StatusForbidden, false, "FORBIDDEN", nil)
	case errors.Is(err, ErrNotFound):
		writeEnvelope(w, http.StatusNotFound, false, "NOT_FOUND", nil)
	case errors.Is(err, ErrInvalidInput):
		writeEnvelope(w, http.StatusBadRequest, false, "INVALID_INPUT", nil)
	case errors.Is(err, ErrInvalidRole):
		writeEnvelope(w, http.StatusBadRequest, false, "INVALID_ROLE", nil)
	case errors.Is(err, ErrRoleProvisionForbidden):
		writeEnvelope(w, http.StatusForbidden, false, "ROLE_PROVISION_FORBIDDEN", nil)
	case errors.Is(err, ErrDuplicateIdentity):
		writeEnvelope(w, http.StatusConflict, false, "DUPLICATE_IDENTITY", nil)
	case errors.Is(err, ErrStatusUpdateForbidden), errors.Is(err, ErrProtectedPlatformTarget):
		writeEnvelope(w, http.StatusForbidden, false, "STATUS_UPDATE_FORBIDDEN", nil)
	case errors.Is(err, ErrCannotDeactivateSelf):
		writeEnvelope(w, http.StatusConflict, false, "CANNOT_DEACTIVATE_SELF", nil)
	default:
		writeEnvelope(w, http.StatusInternalServerError, false, "INTERNAL_ERROR", nil)
	}
}
