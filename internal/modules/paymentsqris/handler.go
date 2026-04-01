package paymentsqris

import (
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/mugiew/onixggr/internal/modules/auth"
	"github.com/mugiew/onixggr/internal/platform/qris"
)

type Handler struct {
	service     Service
	authService auth.Service
	observer    WebhookObserver
}

type WebhookObserver interface {
	ObserveWebhook(provider string, kind string, result string)
}

func NewHandler(service Service, authService auth.Service, observer WebhookObserver) *Handler {
	return &Handler{service: service, authService: authService, observer: observer}
}

func (h *Handler) Register(mux *http.ServeMux) {
	mux.Handle("GET /v1/stores/{storeID}/topups/qris", auth.RequireAuth(h.authService, h.handleListStoreTopups()))
	mux.Handle("POST /v1/stores/{storeID}/topups/qris", auth.RequireAuth(h.authService, h.handleCreateStoreTopup()))
	mux.Handle("POST /v1/store-api/qris/member-payments", h.handleCreateMemberPayment())
	mux.Handle("POST /v1/webhooks/qris", h.handleIncomingWebhook())
}

func (h *Handler) handleListStoreTopups() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		subject, ok := auth.SubjectFromContext(r.Context())
		if !ok {
			writeEnvelope(w, http.StatusUnauthorized, false, "UNAUTHORIZED", nil)
			return
		}

		filter, err := parseListTransactionsFilter(r)
		if err != nil {
			writeEnvelope(w, http.StatusBadRequest, false, "INVALID_REQUEST", nil)
			return
		}

		filter.StoreID = r.PathValue("storeID")
		transactions, err := h.service.ListStoreTopups(r.Context(), subject, filter)
		if err != nil {
			writePaymentError(w, err)
			return
		}

		writeEnvelope(w, http.StatusOK, true, "SUCCESS", transactions)
	})
}

func parseListTransactionsFilter(r *http.Request) (ListTransactionsFilter, error) {
	query := r.URL.Query()
	filter := ListTransactionsFilter{
		Type:   TransactionTypeStoreTopup,
		Query:  strings.TrimSpace(query.Get("query")),
		Limit:  12,
		Offset: 0,
	}

	if rawType := strings.TrimSpace(query.Get("type")); rawType != "" {
		filter.Type = TransactionType(rawType)
		if filter.Type != TransactionTypeStoreTopup && filter.Type != TransactionTypeMemberPayment {
			return ListTransactionsFilter{}, errors.New("invalid type")
		}
	}

	if rawStatus := strings.TrimSpace(query.Get("status")); rawStatus != "" {
		status := TransactionStatus(rawStatus)
		switch status {
		case TransactionStatusPending, TransactionStatusSuccess, TransactionStatusExpired, TransactionStatusFailed:
			filter.Status = &status
		default:
			return ListTransactionsFilter{}, errors.New("invalid status")
		}
	}

	if rawLimit := strings.TrimSpace(query.Get("limit")); rawLimit != "" {
		limit, err := strconv.Atoi(rawLimit)
		if err != nil || limit <= 0 {
			return ListTransactionsFilter{}, errors.New("invalid limit")
		}
		if limit > 100 {
			limit = 100
		}
		filter.Limit = limit
	}

	if rawOffset := strings.TrimSpace(query.Get("offset")); rawOffset != "" {
		offset, err := strconv.Atoi(rawOffset)
		if err != nil || offset < 0 {
			return ListTransactionsFilter{}, errors.New("invalid offset")
		}
		filter.Offset = offset
	}

	createdFrom, err := parseFilterTime(query.Get("created_from"))
	if err != nil {
		return ListTransactionsFilter{}, err
	}
	filter.CreatedFrom = createdFrom

	createdTo, err := parseFilterTime(query.Get("created_to"))
	if err != nil {
		return ListTransactionsFilter{}, err
	}
	filter.CreatedTo = createdTo

	return filter, nil
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

func (h *Handler) handleCreateMemberPayment() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token, ok := bearerToken(r.Header.Get("Authorization"))
		if !ok {
			writeEnvelope(w, http.StatusUnauthorized, false, "UNAUTHORIZED", nil)
			return
		}

		var input CreateMemberPaymentInput
		if err := decodeJSONBody(w, r, &input); err != nil {
			writeEnvelope(w, http.StatusBadRequest, false, "INVALID_REQUEST", nil)
			return
		}

		transaction, err := h.service.CreateMemberPayment(r.Context(), token, input, requestMetadata(r))
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

func (h *Handler) handleIncomingWebhook() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
		raw, err := io.ReadAll(r.Body)
		if err != nil {
			h.observeWebhook("unknown", "invalid")
			writeEnvelope(w, http.StatusBadRequest, false, "INVALID_REQUEST", nil)
			return
		}

		if payment, parseErr := qris.ParsePaymentWebhook(raw); parseErr == nil {
			result, serviceErr := h.service.HandlePaymentWebhook(r.Context(), payment, requestMetadata(r))
			if serviceErr != nil {
				h.observeWebhook(string(WebhookKindPayment), webhookResultLabel(serviceErr))
				writeWebhookError(w, serviceErr)
				return
			}

			h.observeWebhook(string(WebhookKindPayment), "success")
			writeEnvelope(w, http.StatusOK, true, "SUCCESS", result)
			return
		}

		if transfer, parseErr := qris.ParseTransferWebhook(raw); parseErr == nil {
			result, serviceErr := h.service.HandleTransferWebhook(r.Context(), transfer, requestMetadata(r))
			if serviceErr != nil {
				h.observeWebhook(string(WebhookKindWithdrawalStatus), webhookResultLabel(serviceErr))
				writeWebhookError(w, serviceErr)
				return
			}

			h.observeWebhook(string(WebhookKindWithdrawalStatus), "success")
			writeEnvelope(w, http.StatusOK, true, "SUCCESS", result)
			return
		}

		h.observeWebhook("unknown", "invalid")
		writeEnvelope(w, http.StatusBadRequest, false, "INVALID_REQUEST", nil)
	})
}

func (h *Handler) observeWebhook(kind string, result string) {
	if h == nil || h.observer == nil {
		return
	}

	h.observer.ObserveWebhook("qris", kind, result)
}

func webhookResultLabel(err error) string {
	switch {
	case errors.Is(err, ErrNotFound):
		return "ignored"
	case errors.Is(err, ErrDuplicateProvider):
		return "duplicate"
	default:
		return "error"
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

func writePaymentError(w http.ResponseWriter, err error) {
	var businessErr *qris.BusinessError

	switch {
	case errors.Is(err, ErrUnauthorized):
		writeEnvelope(w, http.StatusUnauthorized, false, "UNAUTHORIZED", nil)
	case errors.Is(err, ErrForbidden):
		writeEnvelope(w, http.StatusForbidden, false, "FORBIDDEN", nil)
	case errors.Is(err, ErrNotFound):
		writeEnvelope(w, http.StatusNotFound, false, "NOT_FOUND", nil)
	case errors.Is(err, ErrStoreInactive):
		writeEnvelope(w, http.StatusConflict, false, "STORE_INACTIVE", nil)
	case errors.Is(err, ErrInvalidUsername):
		writeEnvelope(w, http.StatusBadRequest, false, "INVALID_USERNAME", nil)
	case errors.Is(err, ErrInvalidAmount):
		writeEnvelope(w, http.StatusBadRequest, false, "INVALID_AMOUNT", nil)
	case errors.Is(err, ErrMemberInactive):
		writeEnvelope(w, http.StatusConflict, false, "MEMBER_INACTIVE", nil)
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

func writeWebhookError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrNotFound):
		writeEnvelope(w, http.StatusOK, true, "IGNORED", nil)
	case errors.Is(err, ErrDuplicateProvider):
		writeEnvelope(w, http.StatusOK, true, "SUCCESS", nil)
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

func bearerToken(header string) (string, bool) {
	if header == "" {
		return "", false
	}

	prefix, token, found := strings.Cut(header, " ")
	if !found || !strings.EqualFold(prefix, "Bearer") {
		return "", false
	}

	token = strings.TrimSpace(token)
	if token == "" {
		return "", false
	}

	return token, true
}
