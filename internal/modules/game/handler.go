package game

import (
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"strings"

	"github.com/mugiew/onixggr/internal/platform/nexusggr"
)

type Handler struct {
	service Service
}

func NewHandler(service Service) *Handler {
	return &Handler{service: service}
}

func (h *Handler) Register(mux *http.ServeMux) {
	mux.Handle("POST /v1/store-api/game/users", h.handleCreateUser())
}

func (h *Handler) handleCreateUser() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token, ok := bearerToken(r.Header.Get("Authorization"))
		if !ok {
			writeEnvelope(w, http.StatusUnauthorized, false, "UNAUTHORIZED", nil)
			return
		}

		var input CreateUserInput
		if err := decodeJSONBody(w, r, &input); err != nil {
			writeEnvelope(w, http.StatusBadRequest, false, "INVALID_REQUEST", nil)
			return
		}

		member, err := h.service.CreateUser(r.Context(), token, input, requestMetadata(r))
		if err != nil {
			writeGameError(w, err)
			return
		}

		writeEnvelope(w, http.StatusCreated, true, "SUCCESS", member)
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

func writeGameError(w http.ResponseWriter, err error) {
	var businessErr *nexusggr.BusinessError

	switch {
	case errors.Is(err, ErrUnauthorized):
		writeEnvelope(w, http.StatusUnauthorized, false, "UNAUTHORIZED", nil)
	case errors.Is(err, ErrStoreInactive):
		writeEnvelope(w, http.StatusForbidden, false, "STORE_INACTIVE", nil)
	case errors.Is(err, ErrInvalidUsername):
		writeEnvelope(w, http.StatusBadRequest, false, "INVALID_USERNAME", nil)
	case errors.Is(err, ErrDuplicateUsername):
		writeEnvelope(w, http.StatusConflict, false, "DUPLICATE_USERNAME", nil)
	case errors.Is(err, nexusggr.ErrNotConfigured):
		writeEnvelope(w, http.StatusServiceUnavailable, false, "UPSTREAM_NOT_CONFIGURED", nil)
	case errors.Is(err, nexusggr.ErrTimeout):
		writeEnvelope(w, http.StatusGatewayTimeout, false, "UPSTREAM_TIMEOUT", nil)
	case errors.Is(err, nexusggr.ErrUpstreamUnavailable):
		writeEnvelope(w, http.StatusBadGateway, false, "UPSTREAM_UNAVAILABLE", nil)
	case errors.Is(err, nexusggr.ErrUnexpectedHTTP), errors.Is(err, nexusggr.ErrInvalidResponse):
		writeEnvelope(w, http.StatusBadGateway, false, "UPSTREAM_INVALID_RESPONSE", nil)
	case errors.As(err, &businessErr):
		writeEnvelope(w, http.StatusBadGateway, false, businessErr.Code, nil)
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

func requestMetadata(r *http.Request) RequestMetadata {
	return RequestMetadata{
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
