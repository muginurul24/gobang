package paymentsqris

import (
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"strings"

	"github.com/mugiew/onixggr/internal/modules/auth"
	"github.com/mugiew/onixggr/internal/platform/qris"
)

type Handler struct {
	service     Service
	authService auth.Service
}

func NewHandler(service Service, authService auth.Service) *Handler {
	return &Handler{service: service, authService: authService}
}

func (h *Handler) Register(mux *http.ServeMux) {
	mux.Handle("GET /v1/stores/{storeID}/topups/qris", auth.RequireAuth(h.authService, h.handleListStoreTopups()))
	mux.Handle("POST /v1/stores/{storeID}/topups/qris", auth.RequireAuth(h.authService, h.handleCreateStoreTopup()))
}

func (h *Handler) handleListStoreTopups() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		subject, ok := auth.SubjectFromContext(r.Context())
		if !ok {
			writeEnvelope(w, http.StatusUnauthorized, false, "UNAUTHORIZED", nil)
			return
		}

		transactions, err := h.service.ListStoreTopups(r.Context(), subject, r.PathValue("storeID"))
		if err != nil {
			writePaymentError(w, err)
			return
		}

		writeEnvelope(w, http.StatusOK, true, "SUCCESS", transactions)
	})
}

func (h *Handler) handleCreateStoreTopup() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		subject, ok := auth.SubjectFromContext(r.Context())
		if !ok {
			writeEnvelope(w, http.StatusUnauthorized, false, "UNAUTHORIZED", nil)
			return
		}

		var input CreateStoreTopupInput
		if err := decodeJSONBody(w, r, &input); err != nil {
			writeEnvelope(w, http.StatusBadRequest, false, "INVALID_REQUEST", nil)
			return
		}

		transaction, err := h.service.CreateStoreTopup(r.Context(), subject, r.PathValue("storeID"), input, requestMetadata(r))
		if err != nil {
			writePaymentError(w, err)
			return
		}

		if transaction.ProviderState != nil && *transaction.ProviderState == ProviderStatePendingProviderAnswer {
			writeEnvelope(w, http.StatusAccepted, true, "PENDING_PROVIDER", transaction)
			return
		}

		writeEnvelope(w, http.StatusCreated, true, "SUCCESS", transaction)
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

func writePaymentError(w http.ResponseWriter, err error) {
	var businessErr *qris.BusinessError

	switch {
	case errors.Is(err, ErrForbidden):
		writeEnvelope(w, http.StatusForbidden, false, "FORBIDDEN", nil)
	case errors.Is(err, ErrNotFound):
		writeEnvelope(w, http.StatusNotFound, false, "NOT_FOUND", nil)
	case errors.Is(err, ErrStoreInactive):
		writeEnvelope(w, http.StatusConflict, false, "STORE_INACTIVE", nil)
	case errors.Is(err, ErrInvalidAmount):
		writeEnvelope(w, http.StatusBadRequest, false, "INVALID_AMOUNT", nil)
	case errors.Is(err, qris.ErrNotConfigured):
		writeEnvelope(w, http.StatusServiceUnavailable, false, "UPSTREAM_NOT_CONFIGURED", nil)
	case errors.Is(err, qris.ErrTimeout), errors.Is(err, qris.ErrUpstreamUnavailable), errors.Is(err, qris.ErrUnexpectedHTTP):
		writeEnvelope(w, http.StatusAccepted, false, "PENDING_PROVIDER", nil)
	case errors.As(err, &businessErr):
		writeEnvelope(w, http.StatusBadGateway, false, businessErr.Code, nil)
	default:
		writeEnvelope(w, http.StatusInternalServerError, false, "INTERNAL_ERROR", nil)
	}
}

func decodeJSONBody(w http.ResponseWriter, r *http.Request, target any) error {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)

	decoder := json.NewDecoder(r.Body)
	decoder.UseNumber()
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
