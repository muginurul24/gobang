package withdrawals

import (
	"encoding/json"
	"errors"
	"net"
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
	mux.Handle("GET /v1/stores/{storeID}/withdrawals", auth.RequireAuth(h.authService, h.handleListStoreWithdrawals()))
	mux.Handle("POST /v1/stores/{storeID}/withdrawals", auth.RequireAuth(h.authService, h.handleCreateStoreWithdrawal()))
}

func (h *Handler) handleListStoreWithdrawals() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		subject, ok := auth.SubjectFromContext(r.Context())
		if !ok {
			writeEnvelope(w, http.StatusUnauthorized, false, "UNAUTHORIZED", nil)
			return
		}

		filter, err := parseListWithdrawalsFilter(r)
		if err != nil {
			writeEnvelope(w, http.StatusBadRequest, false, "INVALID_REQUEST", nil)
			return
		}

		filter.StoreID = r.PathValue("storeID")
		withdrawals, err := h.service.ListStoreWithdrawals(r.Context(), subject, filter)
		if err != nil {
			writeWithdrawalError(w, err)
			return
		}

		writeEnvelope(w, http.StatusOK, true, "SUCCESS", withdrawals)
	})
}

func parseListWithdrawalsFilter(r *http.Request) (ListWithdrawalsFilter, error) {
	query := r.URL.Query()
	filter := ListWithdrawalsFilter{
		Query:  strings.TrimSpace(query.Get("query")),
		Limit:  12,
		Offset: 0,
	}

	if rawStatus := strings.TrimSpace(query.Get("status")); rawStatus != "" {
		status := WithdrawalStatus(rawStatus)
		switch status {
		case WithdrawalStatusPending, WithdrawalStatusSuccess, WithdrawalStatusFailed:
			filter.Status = &status
		default:
			return ListWithdrawalsFilter{}, errors.New("invalid status")
		}
	}

	if rawLimit := strings.TrimSpace(query.Get("limit")); rawLimit != "" {
		limit, err := strconv.Atoi(rawLimit)
		if err != nil || limit <= 0 {
			return ListWithdrawalsFilter{}, errors.New("invalid limit")
		}
		if limit > 100 {
			limit = 100
		}
		filter.Limit = limit
	}

	if rawOffset := strings.TrimSpace(query.Get("offset")); rawOffset != "" {
		offset, err := strconv.Atoi(rawOffset)
		if err != nil || offset < 0 {
			return ListWithdrawalsFilter{}, errors.New("invalid offset")
		}
		filter.Offset = offset
	}

	createdFrom, err := parseFilterTime(query.Get("created_from"))
	if err != nil {
		return ListWithdrawalsFilter{}, err
	}
	filter.CreatedFrom = createdFrom

	createdTo, err := parseFilterTime(query.Get("created_to"))
	if err != nil {
		return ListWithdrawalsFilter{}, err
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

func (h *Handler) handleCreateStoreWithdrawal() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		subject, ok := auth.SubjectFromContext(r.Context())
		if !ok {
			writeEnvelope(w, http.StatusUnauthorized, false, "UNAUTHORIZED", nil)
			return
		}

		var input CreateWithdrawInput
		if err := decodeJSONBody(w, r, &input); err != nil {
			writeEnvelope(w, http.StatusBadRequest, false, "INVALID_REQUEST", nil)
			return
		}

		withdrawal, created, err := h.service.CreateStoreWithdrawal(r.Context(), subject, r.PathValue("storeID"), input, requestMetadata(r))
		if err != nil {
			writeWithdrawalError(w, err)
			return
		}

		statusCode := http.StatusOK
		if created {
			statusCode = http.StatusCreated
		}

		writeEnvelope(w, statusCode, true, "SUCCESS", withdrawal)
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

func writeWithdrawalError(w http.ResponseWriter, err error) {
	var failure *CreateFailure
	var data any
	if errors.As(err, &failure) && failure.Withdrawal.ID != "" {
		data = failure.Withdrawal
	}

	switch {
	case errors.Is(err, ErrForbidden):
		writeEnvelope(w, http.StatusForbidden, false, "FORBIDDEN", data)
	case errors.Is(err, ErrNotFound):
		writeEnvelope(w, http.StatusNotFound, false, "NOT_FOUND", data)
	case errors.Is(err, ErrStoreInactive):
		writeEnvelope(w, http.StatusForbidden, false, "STORE_INACTIVE", data)
	case errors.Is(err, ErrInvalidAmount):
		writeEnvelope(w, http.StatusBadRequest, false, "INVALID_AMOUNT", data)
	case errors.Is(err, ErrInvalidIdempotencyKey):
		writeEnvelope(w, http.StatusBadRequest, false, "INVALID_IDEMPOTENCY_KEY", data)
	case errors.Is(err, ErrIdempotencyKeyConflict):
		writeEnvelope(w, http.StatusConflict, false, "IDEMPOTENCY_KEY_CONFLICT", data)
	case errors.Is(err, ErrBankAccountInactive):
		writeEnvelope(w, http.StatusConflict, false, "BANK_ACCOUNT_INACTIVE", data)
	case errors.Is(err, ErrInsufficientStoreBalance):
		writeEnvelope(w, http.StatusConflict, false, "INSUFFICIENT_STORE_BALANCE", data)
	case errors.Is(err, ErrInquiryUnavailable):
		writeEnvelope(w, http.StatusServiceUnavailable, false, "WITHDRAW_INQUIRY_UNAVAILABLE", data)
	case errors.Is(err, ErrInquiryFailed):
		writeEnvelope(w, http.StatusBadGateway, false, "WITHDRAW_INQUIRY_FAILED", data)
	case errors.Is(err, ErrTransferUnavailable):
		writeEnvelope(w, http.StatusAccepted, false, "PENDING_RECONCILE", data)
	case errors.Is(err, ErrTransferFailed):
		writeEnvelope(w, http.StatusBadGateway, false, "WITHDRAW_TRANSFER_FAILED", data)
	default:
		writeEnvelope(w, http.StatusInternalServerError, false, "INTERNAL_ERROR", data)
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
