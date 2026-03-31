package middleware

import (
	"net/http"
	"strings"
)

func SecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		headers := w.Header()
		headers.Set("X-Content-Type-Options", "nosniff")
		headers.Set("X-Frame-Options", "DENY")
		headers.Set("Referrer-Policy", "no-referrer")
		headers.Set("Cross-Origin-Opener-Policy", "same-origin")
		headers.Set("Cross-Origin-Resource-Policy", "same-origin")

		if requestIsHTTPS(r) {
			headers.Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		}

		next.ServeHTTP(w, r)
	})
}

func requestIsHTTPS(r *http.Request) bool {
	if r == nil {
		return false
	}

	if r.TLS != nil {
		return true
	}

	return strings.EqualFold(strings.TrimSpace(r.Header.Get("X-Forwarded-Proto")), "https")
}
