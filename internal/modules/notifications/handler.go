package notifications

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

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
	mux.Handle("GET /v1/notifications", auth.RequireAuth(h.authService, http.HandlerFunc(h.handleList)))
	mux.Handle("POST /v1/notifications/{id}/read", auth.RequireAuth(h.authService, http.HandlerFunc(h.handleMarkRead)))
	mux.Handle("GET /v1/notifications/unread-count", auth.RequireAuth(h.authService, http.HandlerFunc(h.handleUnreadCount)))
}

func (h *Handler) handleList(w http.ResponseWriter, r *http.Request) {
	subject, ok := auth.SubjectFromContext(r.Context())
	if !ok {
		writeEnvelope(w, http.StatusUnauthorized, false, "UNAUTHORIZED", nil)
		return
	}

	scopeType, scopeID, ok := resolveScope(r, subject)
	if !ok {
		writeEnvelope(w, http.StatusBadRequest, false, "INVALID_SCOPE", nil)
		return
	}

	limit := 50
	if v := r.URL.Query().Get("limit"); v != "" {
		if parsed, err := strconv.Atoi(v); err == nil && parsed > 0 {
			limit = parsed
		}
	}
	offset := 0
	if v := r.URL.Query().Get("offset"); v != "" {
		if parsed, err := strconv.Atoi(v); err == nil && parsed >= 0 {
			offset = parsed
		}
	}

	list, err := h.service.ListByScope(r.Context(), ListParams{
		ScopeType: scopeType,
		ScopeID:   scopeID,
		Limit:     limit,
		Offset:    offset,
	})
	if err != nil {
		writeEnvelope(w, http.StatusInternalServerError, false, "INTERNAL_ERROR", nil)
		return
	}

	writeEnvelope(w, http.StatusOK, true, "SUCCESS", list)
}

func (h *Handler) handleMarkRead(w http.ResponseWriter, r *http.Request) {
	_, ok := auth.SubjectFromContext(r.Context())
	if !ok {
		writeEnvelope(w, http.StatusUnauthorized, false, "UNAUTHORIZED", nil)
		return
	}

	id := strings.TrimSpace(r.PathValue("id"))
	if id == "" {
		writeEnvelope(w, http.StatusBadRequest, false, "MISSING_ID", nil)
		return
	}

	err := h.service.MarkRead(r.Context(), id)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			writeEnvelope(w, http.StatusNotFound, false, "NOT_FOUND", nil)
			return
		}
		writeEnvelope(w, http.StatusInternalServerError, false, "INTERNAL_ERROR", nil)
		return
	}

	writeEnvelope(w, http.StatusOK, true, "MARKED_READ", nil)
}

func (h *Handler) handleUnreadCount(w http.ResponseWriter, r *http.Request) {
	subject, ok := auth.SubjectFromContext(r.Context())
	if !ok {
		writeEnvelope(w, http.StatusUnauthorized, false, "UNAUTHORIZED", nil)
		return
	}

	scopeType, scopeID, ok := resolveScope(r, subject)
	if !ok {
		writeEnvelope(w, http.StatusBadRequest, false, "INVALID_SCOPE", nil)
		return
	}

	count, err := h.service.CountUnread(r.Context(), scopeType, scopeID)
	if err != nil {
		writeEnvelope(w, http.StatusInternalServerError, false, "INTERNAL_ERROR", nil)
		return
	}

	writeEnvelope(w, http.StatusOK, true, "SUCCESS", map[string]int{"unread_count": count})
}

func resolveScope(r *http.Request, subject auth.Subject) (ScopeType, string, bool) {
	switch subject.Role {
	case auth.RoleDev, auth.RoleSuperadmin:
		scopeType := ScopeType(r.URL.Query().Get("scope_type"))
		scopeID := r.URL.Query().Get("scope_id")
		if scopeType == "" || scopeID == "" {
			scopeType = ScopeRole
			scopeID = string(subject.Role)
		}
		return scopeType, scopeID, true
	case auth.RoleOwner, auth.RoleKaryawan:
		storeID := r.URL.Query().Get("store_id")
		if storeID == "" {
			return "", "", false
		}
		return ScopeStore, storeID, true
	default:
		return "", "", false
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
