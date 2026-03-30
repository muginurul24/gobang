package middleware

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
)

func RequireCSRF(appURL string, cookieName string, headerName string, next http.Handler) http.Handler {
	appOrigin := originFromURL(appURL)

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !requiresCSRFProtection(r) {
			next.ServeHTTP(w, r)
			return
		}

		if !originAllowed(appOrigin, r) {
			writeCSRFFailure(w)
			return
		}

		cookie, err := r.Cookie(cookieName)
		if err != nil || strings.TrimSpace(cookie.Value) == "" {
			writeCSRFFailure(w)
			return
		}

		headerValue := strings.TrimSpace(r.Header.Get(headerName))
		if headerValue == "" || headerValue != strings.TrimSpace(cookie.Value) {
			writeCSRFFailure(w)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func requiresCSRFProtection(r *http.Request) bool {
	if r == nil {
		return false
	}

	switch r.Method {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
		return false
	}

	path := strings.TrimSpace(r.URL.Path)
	if !strings.HasPrefix(path, "/v1/") {
		return false
	}

	switch {
	case path == "/v1/auth/login":
		return false
	case strings.HasPrefix(path, "/v1/store-api/"):
		return false
	case strings.HasPrefix(path, "/v1/webhooks/"):
		return false
	default:
		return true
	}
}

func originAllowed(appOrigin string, r *http.Request) bool {
	if r == nil {
		return false
	}

	origin := strings.TrimSpace(r.Header.Get("Origin"))
	if origin == "" {
		return true
	}

	if appOrigin == "" {
		return true
	}

	return strings.EqualFold(origin, appOrigin)
}

func originFromURL(raw string) string {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return ""
	}

	if parsed.Scheme == "" || parsed.Host == "" {
		return ""
	}

	return strings.ToLower(parsed.Scheme + "://" + parsed.Host)
}

func writeCSRFFailure(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusForbidden)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"status":  false,
		"message": "CSRF_INVALID",
		"data":    nil,
	})
}
