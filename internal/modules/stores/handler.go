package stores

import (
	"encoding/json"
	"errors"
	"net"
	"net/http"
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
	mux.Handle("GET /v1/stores", auth.RequireAuth(h.authService, h.handleListStores()))
	mux.Handle("POST /v1/stores", auth.RequireAuth(h.authService, h.handleCreateStore()))
	mux.Handle("GET /v1/stores/{storeID}", auth.RequireAuth(h.authService, h.handleGetStore()))
	mux.Handle("PATCH /v1/stores/{storeID}", auth.RequireAuth(h.authService, h.handleUpdateStore()))
	mux.Handle("DELETE /v1/stores/{storeID}", auth.RequireAuth(h.authService, h.handleDeleteStore()))
	mux.Handle("POST /v1/stores/{storeID}/token", auth.RequireAuth(h.authService, h.handleRotateStoreToken()))
	mux.Handle("PUT /v1/stores/{storeID}/callback-url", auth.RequireAuth(h.authService, h.handleUpdateCallbackURL()))
	mux.Handle("GET /v1/stores/{storeID}/staff", auth.RequireAuth(h.authService, h.handleListStoreStaff()))
	mux.Handle("POST /v1/stores/{storeID}/staff", auth.RequireAuth(h.authService, h.handleAssignStoreStaff()))
	mux.Handle("DELETE /v1/stores/{storeID}/staff/{userID}", auth.RequireAuth(h.authService, h.handleUnassignStoreStaff()))
	mux.Handle("GET /v1/staff/users", auth.RequireAuth(h.authService, h.handleListEmployees()))
	mux.Handle("POST /v1/staff/users", auth.RequireAuth(h.authService, h.handleCreateEmployee()))
}

func (h *Handler) handleListStores() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		subject, ok := auth.SubjectFromContext(r.Context())
		if !ok {
			writeEnvelope(w, http.StatusUnauthorized, false, "UNAUTHORIZED", nil)
			return
		}

		stores, err := h.service.ListStores(r.Context(), subject)
		if err != nil {
			writeStoreError(w, err)
			return
		}

		writeEnvelope(w, http.StatusOK, true, "SUCCESS", stores)
	})
}

func (h *Handler) handleGetStore() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		subject, ok := auth.SubjectFromContext(r.Context())
		if !ok {
			writeEnvelope(w, http.StatusUnauthorized, false, "UNAUTHORIZED", nil)
			return
		}

		store, err := h.service.GetStore(r.Context(), subject, r.PathValue("storeID"))
		if err != nil {
			writeStoreError(w, err)
			return
		}

		writeEnvelope(w, http.StatusOK, true, "SUCCESS", store)
	})
}

func (h *Handler) handleCreateStore() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		subject, ok := auth.SubjectFromContext(r.Context())
		if !ok {
			writeEnvelope(w, http.StatusUnauthorized, false, "UNAUTHORIZED", nil)
			return
		}

		var input CreateStoreInput
		if err := decodeJSONBody(w, r, &input); err != nil {
			writeEnvelope(w, http.StatusBadRequest, false, "INVALID_REQUEST", nil)
			return
		}

		store, err := h.service.CreateStore(r.Context(), subject, input, requestMetadata(r))
		if err != nil {
			writeStoreError(w, err)
			return
		}

		writeEnvelope(w, http.StatusCreated, true, "SUCCESS", store)
	})
}

func (h *Handler) handleUpdateStore() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		subject, ok := auth.SubjectFromContext(r.Context())
		if !ok {
			writeEnvelope(w, http.StatusUnauthorized, false, "UNAUTHORIZED", nil)
			return
		}

		var input UpdateStoreInput
		if err := decodeJSONBody(w, r, &input); err != nil {
			writeEnvelope(w, http.StatusBadRequest, false, "INVALID_REQUEST", nil)
			return
		}

		store, err := h.service.UpdateStore(r.Context(), subject, r.PathValue("storeID"), input, requestMetadata(r))
		if err != nil {
			writeStoreError(w, err)
			return
		}

		writeEnvelope(w, http.StatusOK, true, "SUCCESS", store)
	})
}

func (h *Handler) handleDeleteStore() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		subject, ok := auth.SubjectFromContext(r.Context())
		if !ok {
			writeEnvelope(w, http.StatusUnauthorized, false, "UNAUTHORIZED", nil)
			return
		}

		if err := h.service.DeleteStore(r.Context(), subject, r.PathValue("storeID"), requestMetadata(r)); err != nil {
			writeStoreError(w, err)
			return
		}

		writeEnvelope(w, http.StatusOK, true, "SUCCESS", nil)
	})
}

func (h *Handler) handleRotateStoreToken() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		subject, ok := auth.SubjectFromContext(r.Context())
		if !ok {
			writeEnvelope(w, http.StatusUnauthorized, false, "UNAUTHORIZED", nil)
			return
		}

		token, err := h.service.RotateStoreToken(r.Context(), subject, r.PathValue("storeID"), requestMetadata(r))
		if err != nil {
			writeStoreError(w, err)
			return
		}

		writeEnvelope(w, http.StatusOK, true, "SUCCESS", token)
	})
}

func (h *Handler) handleUpdateCallbackURL() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		subject, ok := auth.SubjectFromContext(r.Context())
		if !ok {
			writeEnvelope(w, http.StatusUnauthorized, false, "UNAUTHORIZED", nil)
			return
		}

		var input UpdateCallbackInput
		if err := decodeJSONBody(w, r, &input); err != nil {
			writeEnvelope(w, http.StatusBadRequest, false, "INVALID_REQUEST", nil)
			return
		}

		store, err := h.service.UpdateCallbackURL(r.Context(), subject, r.PathValue("storeID"), input, requestMetadata(r))
		if err != nil {
			writeStoreError(w, err)
			return
		}

		writeEnvelope(w, http.StatusOK, true, "SUCCESS", store)
	})
}

func (h *Handler) handleListStoreStaff() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		subject, ok := auth.SubjectFromContext(r.Context())
		if !ok {
			writeEnvelope(w, http.StatusUnauthorized, false, "UNAUTHORIZED", nil)
			return
		}

		staff, err := h.service.ListStoreStaff(r.Context(), subject, r.PathValue("storeID"))
		if err != nil {
			writeStoreError(w, err)
			return
		}

		writeEnvelope(w, http.StatusOK, true, "SUCCESS", staff)
	})
}

func (h *Handler) handleAssignStoreStaff() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		subject, ok := auth.SubjectFromContext(r.Context())
		if !ok {
			writeEnvelope(w, http.StatusUnauthorized, false, "UNAUTHORIZED", nil)
			return
		}

		var input AssignStaffInput
		if err := decodeJSONBody(w, r, &input); err != nil {
			writeEnvelope(w, http.StatusBadRequest, false, "INVALID_REQUEST", nil)
			return
		}

		staff, err := h.service.AssignStoreStaff(r.Context(), subject, r.PathValue("storeID"), input, requestMetadata(r))
		if err != nil {
			writeStoreError(w, err)
			return
		}

		writeEnvelope(w, http.StatusOK, true, "SUCCESS", staff)
	})
}

func (h *Handler) handleUnassignStoreStaff() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		subject, ok := auth.SubjectFromContext(r.Context())
		if !ok {
			writeEnvelope(w, http.StatusUnauthorized, false, "UNAUTHORIZED", nil)
			return
		}

		staff, err := h.service.UnassignStoreStaff(r.Context(), subject, r.PathValue("storeID"), r.PathValue("userID"), requestMetadata(r))
		if err != nil {
			writeStoreError(w, err)
			return
		}

		writeEnvelope(w, http.StatusOK, true, "SUCCESS", staff)
	})
}

func (h *Handler) handleCreateEmployee() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		subject, ok := auth.SubjectFromContext(r.Context())
		if !ok {
			writeEnvelope(w, http.StatusUnauthorized, false, "UNAUTHORIZED", nil)
			return
		}

		var input CreateEmployeeInput
		if err := decodeJSONBody(w, r, &input); err != nil {
			writeEnvelope(w, http.StatusBadRequest, false, "INVALID_REQUEST", nil)
			return
		}

		user, err := h.service.CreateEmployee(r.Context(), subject, input, requestMetadata(r))
		if err != nil {
			writeStoreError(w, err)
			return
		}

		writeEnvelope(w, http.StatusCreated, true, "SUCCESS", user)
	})
}

func (h *Handler) handleListEmployees() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		subject, ok := auth.SubjectFromContext(r.Context())
		if !ok {
			writeEnvelope(w, http.StatusUnauthorized, false, "UNAUTHORIZED", nil)
			return
		}

		users, err := h.service.ListEmployees(r.Context(), subject)
		if err != nil {
			writeStoreError(w, err)
			return
		}

		writeEnvelope(w, http.StatusOK, true, "SUCCESS", users)
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

func writeStoreError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrForbidden):
		writeEnvelope(w, http.StatusForbidden, false, "FORBIDDEN", nil)
	case errors.Is(err, ErrNotFound), errors.Is(err, ErrEmployeeNotFound):
		writeEnvelope(w, http.StatusNotFound, false, "NOT_FOUND", nil)
	case errors.Is(err, ErrInvalidStoreName):
		writeEnvelope(w, http.StatusBadRequest, false, "INVALID_STORE_NAME", nil)
	case errors.Is(err, ErrInvalidSlug):
		writeEnvelope(w, http.StatusBadRequest, false, "INVALID_SLUG", nil)
	case errors.Is(err, ErrInvalidThreshold):
		writeEnvelope(w, http.StatusBadRequest, false, "INVALID_THRESHOLD", nil)
	case errors.Is(err, ErrInvalidStatus):
		writeEnvelope(w, http.StatusBadRequest, false, "INVALID_STATUS", nil)
	case errors.Is(err, ErrInvalidCallbackURL):
		writeEnvelope(w, http.StatusBadRequest, false, "INVALID_CALLBACK_URL", nil)
	case errors.Is(err, ErrInvalidEmployeeInput):
		writeEnvelope(w, http.StatusBadRequest, false, "INVALID_EMPLOYEE_INPUT", nil)
	case errors.Is(err, ErrEmployeeScopeMismatch):
		writeEnvelope(w, http.StatusBadRequest, false, "CROSS_OWNER_RELATION_FORBIDDEN", nil)
	case errors.Is(err, ErrDuplicateSlug):
		writeEnvelope(w, http.StatusConflict, false, "DUPLICATE_SLUG", nil)
	case errors.Is(err, ErrDuplicateIdentity):
		writeEnvelope(w, http.StatusConflict, false, "DUPLICATE_IDENTITY", nil)
	case errors.Is(err, ErrDuplicateStaff):
		writeEnvelope(w, http.StatusConflict, false, "STAFF_ALREADY_ASSIGNED", nil)
	default:
		writeEnvelope(w, http.StatusInternalServerError, false, "INTERNAL_ERROR", nil)
	}
}

func decodeJSONBody(w http.ResponseWriter, r *http.Request, target any) error {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)

	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()

	return decoder.Decode(target)
}

func requestMetadata(r *http.Request) auth.RequestMetadata {
	return auth.RequestMetadata{
		IPAddress: clientIP(r),
		UserAgent: r.UserAgent(),
	}
}

func clientIP(r *http.Request) string {
	if forwarded := strings.TrimSpace(r.Header.Get("X-Forwarded-For")); forwarded != "" {
		ip, _, _ := strings.Cut(forwarded, ",")
		parsed := net.ParseIP(strings.TrimSpace(ip))
		if parsed != nil {
			return parsed.String()
		}
	}

	host, _, err := net.SplitHostPort(strings.TrimSpace(r.RemoteAddr))
	if err == nil {
		parsed := net.ParseIP(host)
		if parsed != nil {
			return parsed.String()
		}
	}

	parsed := net.ParseIP(strings.TrimSpace(r.RemoteAddr))
	if parsed != nil {
		return parsed.String()
	}

	return "0.0.0.0"
}
