package bankaccounts

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
	mux.Handle("GET /v1/banks", auth.RequireAuth(h.authService, h.handleSearchBanks()))
	mux.Handle("GET /v1/stores/{storeID}/bank-accounts", auth.RequireAuth(h.authService, h.handleListBankAccounts()))
	mux.Handle("POST /v1/stores/{storeID}/bank-accounts", auth.RequireAuth(h.authService, h.handleCreateBankAccount()))
	mux.Handle("PATCH /v1/stores/{storeID}/bank-accounts/{bankAccountID}", auth.RequireAuth(h.authService, h.handleUpdateBankAccountStatus()))
}

func (h *Handler) handleSearchBanks() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		subject, ok := auth.SubjectFromContext(r.Context())
		if !ok {
			writeEnvelope(w, http.StatusUnauthorized, false, "UNAUTHORIZED", nil)
			return
		}

		filter := SearchFilter{
			Query: strings.TrimSpace(r.URL.Query().Get("query")),
			Limit: 20,
		}
		if limit := strings.TrimSpace(r.URL.Query().Get("limit")); limit != "" {
			parsed, err := strconv.Atoi(limit)
			if err == nil {
				filter.Limit = parsed
			}
		}

		results, err := h.service.SearchBanks(r.Context(), subject, filter)
		if err != nil {
			writeBankAccountError(w, err)
			return
		}

		writeEnvelope(w, http.StatusOK, true, "SUCCESS", results)
	})
}

func (h *Handler) handleListBankAccounts() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		subject, ok := auth.SubjectFromContext(r.Context())
		if !ok {
			writeEnvelope(w, http.StatusUnauthorized, false, "UNAUTHORIZED", nil)
			return
		}

		filter := ListBankAccountsFilter{
			StoreID: r.PathValue("storeID"),
			Query:   strings.TrimSpace(r.URL.Query().Get("query")),
			Limit:   20,
		}
		if rawStatus := strings.TrimSpace(r.URL.Query().Get("status")); rawStatus != "" {
			isActive, ok := parseOptionalAccountStatus(rawStatus)
			if !ok {
				writeEnvelope(w, http.StatusBadRequest, false, "INVALID_REQUEST", nil)
				return
			}
			filter.IsActive = isActive
		}
		if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
			limit, err := strconv.Atoi(raw)
			if err != nil {
				writeEnvelope(w, http.StatusBadRequest, false, "INVALID_REQUEST", nil)
				return
			}
			filter.Limit = limit
		}
		if raw := strings.TrimSpace(r.URL.Query().Get("offset")); raw != "" {
			offset, err := strconv.Atoi(raw)
			if err != nil {
				writeEnvelope(w, http.StatusBadRequest, false, "INVALID_REQUEST", nil)
				return
			}
			filter.Offset = offset
		}
		verifiedFrom, err := parseFilterTime(r.URL.Query().Get("verified_from"))
		if err != nil {
			writeEnvelope(w, http.StatusBadRequest, false, "INVALID_REQUEST", nil)
			return
		}
		verifiedTo, err := parseFilterTime(r.URL.Query().Get("verified_to"))
		if err != nil {
			writeEnvelope(w, http.StatusBadRequest, false, "INVALID_REQUEST", nil)
			return
		}
		filter.VerifiedFrom = verifiedFrom
		filter.VerifiedTo = verifiedTo

		results, err := h.service.ListBankAccounts(r.Context(), subject, filter)
		if err != nil {
			writeBankAccountError(w, err)
			return
		}

		writeEnvelope(w, http.StatusOK, true, "SUCCESS", results)
	})
}

func (h *Handler) handleCreateBankAccount() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		subject, ok := auth.SubjectFromContext(r.Context())
		if !ok {
			writeEnvelope(w, http.StatusUnauthorized, false, "UNAUTHORIZED", nil)
			return
		}

		var input CreateBankAccountInput
		if err := decodeJSONBody(w, r, &input); err != nil {
			writeEnvelope(w, http.StatusBadRequest, false, "INVALID_REQUEST", nil)
			return
		}

		bankAccount, err := h.service.CreateBankAccount(r.Context(), subject, r.PathValue("storeID"), input, requestMetadata(r))
		if err != nil {
			writeBankAccountError(w, err)
			return
		}

		writeEnvelope(w, http.StatusCreated, true, "SUCCESS", bankAccount)
	})
}

func (h *Handler) handleUpdateBankAccountStatus() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		subject, ok := auth.SubjectFromContext(r.Context())
		if !ok {
			writeEnvelope(w, http.StatusUnauthorized, false, "UNAUTHORIZED", nil)
			return
		}

		var input UpdateBankAccountStatusInput
		if err := decodeJSONBody(w, r, &input); err != nil {
			writeEnvelope(w, http.StatusBadRequest, false, "INVALID_REQUEST", nil)
			return
		}

		bankAccount, err := h.service.UpdateBankAccountStatus(r.Context(), subject, r.PathValue("storeID"), r.PathValue("bankAccountID"), input, requestMetadata(r))
		if err != nil {
			writeBankAccountError(w, err)
			return
		}

		writeEnvelope(w, http.StatusOK, true, "SUCCESS", bankAccount)
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

func writeBankAccountError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrForbidden):
		writeEnvelope(w, http.StatusForbidden, false, "FORBIDDEN", nil)
	case errors.Is(err, ErrNotFound):
		writeEnvelope(w, http.StatusNotFound, false, "NOT_FOUND", nil)
	case errors.Is(err, ErrInvalidBankCode):
		writeEnvelope(w, http.StatusBadRequest, false, "INVALID_BANK_CODE", nil)
	case errors.Is(err, ErrInvalidAccountNumber):
		writeEnvelope(w, http.StatusBadRequest, false, "INVALID_ACCOUNT_NUMBER", nil)
	case errors.Is(err, ErrInquiryUnavailable):
		writeEnvelope(w, http.StatusServiceUnavailable, false, "BANK_INQUIRY_UNAVAILABLE", nil)
	case errors.Is(err, ErrInquiryFailed):
		writeEnvelope(w, http.StatusBadRequest, false, "BANK_INQUIRY_FAILED", nil)
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

func parseOptionalAccountStatus(raw string) (*bool, bool) {
	switch strings.TrimSpace(raw) {
	case "active":
		value := true
		return &value, true
	case "inactive":
		value := false
		return &value, true
	default:
		return nil, false
	}
}

func parseFilterTime(raw string) (*time.Time, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return nil, nil
	}

	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return nil, err
	}

	parsed = parsed.UTC()
	return &parsed, nil
}
