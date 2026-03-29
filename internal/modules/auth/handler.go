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
}

func (h *Handler) handleLogin(w http.ResponseWriter, r *http.Request) {
	var request struct {
		Login    string `json:"login"`
		Password string `json:"password"`
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
		Login:    request.Login,
		Password: request.Password,
		RequestMetadata: RequestMetadata{
			IPAddress: clientIP(r),
			UserAgent: r.UserAgent(),
		},
	})
	if err != nil {
		switch {
		case errors.Is(err, ErrInvalidCredentials):
			writeEnvelope(w, http.StatusUnauthorized, false, "INVALID_CREDENTIALS", nil)
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
