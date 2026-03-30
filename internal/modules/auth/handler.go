package auth

import (
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"strings"
)

type Handler struct {
	service Service
}

func NewHandler(service Service) *Handler {
	return &Handler{service: service}
}

func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("POST /v1/auth/login", h.handleLogin)
	mux.HandleFunc("POST /v1/auth/refresh", h.handleRefresh)
	mux.Handle("GET /v1/auth/me", RequireAuth(h.service, http.HandlerFunc(h.handleMe)))
	mux.Handle("POST /v1/auth/logout", RequireAuth(h.service, http.HandlerFunc(h.handleLogout)))
	mux.Handle("POST /v1/auth/logout-all", RequireAuth(h.service, http.HandlerFunc(h.handleLogoutAll)))
	mux.Handle("GET /v1/auth/security", RequireAuth(h.service, http.HandlerFunc(h.handleSecurity)))
	mux.Handle("POST /v1/auth/2fa/enroll", RequireAuth(h.service, http.HandlerFunc(h.handleBeginTOTPEnrollment)))
	mux.Handle("POST /v1/auth/2fa/enable", RequireAuth(h.service, http.HandlerFunc(h.handleEnableTOTP)))
	mux.Handle("POST /v1/auth/2fa/disable", RequireAuth(h.service, http.HandlerFunc(h.handleDisableTOTP)))
	mux.Handle("PUT /v1/auth/ip-allowlist", RequireAuth(h.service, http.HandlerFunc(h.handleUpdateIPAllowlist)))
}

func (h *Handler) handleLogin(w http.ResponseWriter, r *http.Request) {
	var request struct {
		Login        string `json:"login"`
		Password     string `json:"password"`
		TOTPCode     string `json:"totp_code"`
		RecoveryCode string `json:"recovery_code"`
	}

	if err := decodeJSONBody(w, r, &request); err != nil {
		writeEnvelope(w, http.StatusBadRequest, false, "INVALID_REQUEST", nil)
		return
	}

	if strings.TrimSpace(request.Login) == "" || request.Password == "" {
		writeEnvelope(w, http.StatusBadRequest, false, "INVALID_REQUEST", nil)
		return
	}

	session, err := h.service.Login(r.Context(), LoginInput{
		Login:        request.Login,
		Password:     request.Password,
		TOTPCode:     request.TOTPCode,
		RecoveryCode: request.RecoveryCode,
		RequestMetadata: RequestMetadata{
			IPAddress: clientIP(r),
			UserAgent: r.UserAgent(),
		},
	})
	if err != nil {
		var rateLimitError *RateLimitError
		switch {
		case errors.As(err, &rateLimitError):
			writeEnvelope(w, http.StatusTooManyRequests, false, "RATE_LIMITED", map[string]any{
				"scope":               rateLimitError.Scope,
				"retry_after_seconds": rateLimitError.RetryAfter,
			})
		case errors.Is(err, ErrSecondFactorRequired):
			writeEnvelope(w, http.StatusOK, false, "TOTP_REQUIRED", map[string]any{
				"recovery_allowed": true,
			})
		case errors.Is(err, ErrInvalidSecondFactor):
			writeEnvelope(w, http.StatusUnauthorized, false, "INVALID_2FA_CODE", nil)
		case errors.Is(err, ErrInvalidCredentials):
			writeEnvelope(w, http.StatusUnauthorized, false, "INVALID_CREDENTIALS", nil)
		case errors.Is(err, ErrIPNotAllowed):
			writeEnvelope(w, http.StatusForbidden, false, "IP_NOT_ALLOWED", nil)
		default:
			writeEnvelope(w, http.StatusInternalServerError, false, "INTERNAL_ERROR", nil)
		}
		return
	}

	writeEnvelope(w, http.StatusOK, true, "SUCCESS", session)
}

func (h *Handler) handleRefresh(w http.ResponseWriter, r *http.Request) {
	var request struct {
		RefreshToken string `json:"refresh_token"`
	}

	if err := decodeJSONBody(w, r, &request); err != nil {
		writeEnvelope(w, http.StatusBadRequest, false, "INVALID_REQUEST", nil)
		return
	}

	session, err := h.service.Refresh(r.Context(), request.RefreshToken, RequestMetadata{
		IPAddress: clientIP(r),
		UserAgent: r.UserAgent(),
	})
	if err != nil {
		switch {
		case errors.Is(err, ErrInvalidRefreshToken):
			writeEnvelope(w, http.StatusUnauthorized, false, "INVALID_REFRESH_TOKEN", nil)
		case errors.Is(err, ErrUnauthorized):
			writeEnvelope(w, http.StatusUnauthorized, false, "UNAUTHORIZED", nil)
		default:
			writeEnvelope(w, http.StatusInternalServerError, false, "INTERNAL_ERROR", nil)
		}
		return
	}

	writeEnvelope(w, http.StatusOK, true, "SUCCESS", session)
}

func (h *Handler) handleMe(w http.ResponseWriter, r *http.Request) {
	subject, ok := SubjectFromContext(r.Context())
	if !ok {
		writeEnvelope(w, http.StatusUnauthorized, false, "UNAUTHORIZED", nil)
		return
	}

	profile, err := h.service.Me(r.Context(), subject)
	if err != nil {
		switch {
		case errors.Is(err, ErrUnauthorized):
			writeEnvelope(w, http.StatusUnauthorized, false, "UNAUTHORIZED", nil)
		default:
			writeEnvelope(w, http.StatusInternalServerError, false, "INTERNAL_ERROR", nil)
		}
		return
	}

	writeEnvelope(w, http.StatusOK, true, "SUCCESS", profile)
}

func (h *Handler) handleLogout(w http.ResponseWriter, r *http.Request) {
	subject, ok := SubjectFromContext(r.Context())
	if !ok {
		writeEnvelope(w, http.StatusUnauthorized, false, "UNAUTHORIZED", nil)
		return
	}

	if err := h.service.Logout(r.Context(), subject, RequestMetadata{
		IPAddress: clientIP(r),
		UserAgent: r.UserAgent(),
	}); err != nil {
		writeEnvelope(w, http.StatusInternalServerError, false, "INTERNAL_ERROR", nil)
		return
	}

	writeEnvelope(w, http.StatusOK, true, "SUCCESS", nil)
}

func (h *Handler) handleLogoutAll(w http.ResponseWriter, r *http.Request) {
	subject, ok := SubjectFromContext(r.Context())
	if !ok {
		writeEnvelope(w, http.StatusUnauthorized, false, "UNAUTHORIZED", nil)
		return
	}

	revoked, err := h.service.LogoutAll(r.Context(), subject, RequestMetadata{
		IPAddress: clientIP(r),
		UserAgent: r.UserAgent(),
	})
	if err != nil {
		writeEnvelope(w, http.StatusInternalServerError, false, "INTERNAL_ERROR", nil)
		return
	}

	writeEnvelope(w, http.StatusOK, true, "SUCCESS", map[string]int{
		"revoked_sessions": revoked,
	})
}

func (h *Handler) handleSecurity(w http.ResponseWriter, r *http.Request) {
	subject, ok := SubjectFromContext(r.Context())
	if !ok {
		writeEnvelope(w, http.StatusUnauthorized, false, "UNAUTHORIZED", nil)
		return
	}

	settings, err := h.service.GetSecuritySettings(r.Context(), subject)
	if err != nil {
		switch {
		case errors.Is(err, ErrUnauthorized):
			writeEnvelope(w, http.StatusUnauthorized, false, "UNAUTHORIZED", nil)
		default:
			writeEnvelope(w, http.StatusInternalServerError, false, "INTERNAL_ERROR", nil)
		}
		return
	}

	writeEnvelope(w, http.StatusOK, true, "SUCCESS", settings)
}

func (h *Handler) handleBeginTOTPEnrollment(w http.ResponseWriter, r *http.Request) {
	subject, ok := SubjectFromContext(r.Context())
	if !ok {
		writeEnvelope(w, http.StatusUnauthorized, false, "UNAUTHORIZED", nil)
		return
	}

	enrollment, err := h.service.BeginTOTPEnrollment(r.Context(), subject)
	if err != nil {
		switch {
		case errors.Is(err, ErrUnauthorized):
			writeEnvelope(w, http.StatusUnauthorized, false, "UNAUTHORIZED", nil)
		case errors.Is(err, ErrTOTPAlreadyEnabled):
			writeEnvelope(w, http.StatusConflict, false, "TOTP_ALREADY_ENABLED", nil)
		default:
			writeEnvelope(w, http.StatusInternalServerError, false, "INTERNAL_ERROR", nil)
		}
		return
	}

	writeEnvelope(w, http.StatusOK, true, "SUCCESS", enrollment)
}

func (h *Handler) handleEnableTOTP(w http.ResponseWriter, r *http.Request) {
	subject, ok := SubjectFromContext(r.Context())
	if !ok {
		writeEnvelope(w, http.StatusUnauthorized, false, "UNAUTHORIZED", nil)
		return
	}

	var request struct {
		Code string `json:"code"`
	}

	if err := decodeJSONBody(w, r, &request); err != nil {
		writeEnvelope(w, http.StatusBadRequest, false, "INVALID_REQUEST", nil)
		return
	}

	recoveryCodes, err := h.service.EnableTOTP(r.Context(), subject, EnableTOTPInput{
		Code: request.Code,
		RequestMetadata: RequestMetadata{
			IPAddress: clientIP(r),
			UserAgent: r.UserAgent(),
		},
	})
	if err != nil {
		switch {
		case errors.Is(err, ErrUnauthorized):
			writeEnvelope(w, http.StatusUnauthorized, false, "UNAUTHORIZED", nil)
		case errors.Is(err, ErrTOTPAlreadyEnabled):
			writeEnvelope(w, http.StatusConflict, false, "TOTP_ALREADY_ENABLED", nil)
		case errors.Is(err, ErrNoPendingEnrollment):
			writeEnvelope(w, http.StatusBadRequest, false, "NO_PENDING_ENROLLMENT", nil)
		case errors.Is(err, ErrInvalidSecondFactor):
			writeEnvelope(w, http.StatusBadRequest, false, "INVALID_2FA_CODE", nil)
		default:
			writeEnvelope(w, http.StatusInternalServerError, false, "INTERNAL_ERROR", nil)
		}
		return
	}

	writeEnvelope(w, http.StatusOK, true, "SUCCESS", recoveryCodes)
}

func (h *Handler) handleDisableTOTP(w http.ResponseWriter, r *http.Request) {
	subject, ok := SubjectFromContext(r.Context())
	if !ok {
		writeEnvelope(w, http.StatusUnauthorized, false, "UNAUTHORIZED", nil)
		return
	}

	var request struct {
		TOTPCode     string `json:"totp_code"`
		RecoveryCode string `json:"recovery_code"`
	}

	if err := decodeJSONBody(w, r, &request); err != nil {
		writeEnvelope(w, http.StatusBadRequest, false, "INVALID_REQUEST", nil)
		return
	}

	err := h.service.DisableTOTP(r.Context(), subject, DisableTOTPInput{
		TOTPCode:     request.TOTPCode,
		RecoveryCode: request.RecoveryCode,
		RequestMetadata: RequestMetadata{
			IPAddress: clientIP(r),
			UserAgent: r.UserAgent(),
		},
	})
	if err != nil {
		switch {
		case errors.Is(err, ErrUnauthorized):
			writeEnvelope(w, http.StatusUnauthorized, false, "UNAUTHORIZED", nil)
		case errors.Is(err, ErrTOTPNotEnabled):
			writeEnvelope(w, http.StatusBadRequest, false, "TOTP_NOT_ENABLED", nil)
		case errors.Is(err, ErrSecondFactorRequired):
			writeEnvelope(w, http.StatusBadRequest, false, "TOTP_REQUIRED", nil)
		case errors.Is(err, ErrInvalidSecondFactor):
			writeEnvelope(w, http.StatusBadRequest, false, "INVALID_2FA_CODE", nil)
		default:
			writeEnvelope(w, http.StatusInternalServerError, false, "INTERNAL_ERROR", nil)
		}
		return
	}

	writeEnvelope(w, http.StatusOK, true, "SUCCESS", nil)
}

func (h *Handler) handleUpdateIPAllowlist(w http.ResponseWriter, r *http.Request) {
	subject, ok := SubjectFromContext(r.Context())
	if !ok {
		writeEnvelope(w, http.StatusUnauthorized, false, "UNAUTHORIZED", nil)
		return
	}

	var request struct {
		IPAllowlist *string `json:"ip_allowlist"`
	}

	if err := decodeJSONBody(w, r, &request); err != nil {
		writeEnvelope(w, http.StatusBadRequest, false, "INVALID_REQUEST", nil)
		return
	}

	settings, err := h.service.UpdateIPAllowlist(r.Context(), subject, UpdateIPAllowlistInput{
		IPAllowlist: request.IPAllowlist,
		RequestMetadata: RequestMetadata{
			IPAddress: clientIP(r),
			UserAgent: r.UserAgent(),
		},
	})
	if err != nil {
		switch {
		case errors.Is(err, ErrUnauthorized):
			writeEnvelope(w, http.StatusUnauthorized, false, "UNAUTHORIZED", nil)
		case errors.Is(err, ErrInvalidIPAllowlist):
			writeEnvelope(w, http.StatusBadRequest, false, "INVALID_IP_ALLOWLIST", nil)
		default:
			writeEnvelope(w, http.StatusInternalServerError, false, "INTERNAL_ERROR", nil)
		}
		return
	}

	writeEnvelope(w, http.StatusOK, true, "SUCCESS", settings)
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

func decodeJSONBody(w http.ResponseWriter, r *http.Request, target any) error {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)

	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()

	return decoder.Decode(target)
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
