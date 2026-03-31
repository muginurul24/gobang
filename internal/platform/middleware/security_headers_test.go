package middleware

import (
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSecurityHeadersAddsDefaultHeaders(t *testing.T) {
	handler := SecurityHeaders(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	request := httptest.NewRequest(http.MethodGet, "/v1/ping", nil)
	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, request)

	if recorder.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatalf("X-Content-Type-Options = %q, want nosniff", recorder.Header().Get("X-Content-Type-Options"))
	}
	if recorder.Header().Get("X-Frame-Options") != "DENY" {
		t.Fatalf("X-Frame-Options = %q, want DENY", recorder.Header().Get("X-Frame-Options"))
	}
	if recorder.Header().Get("Referrer-Policy") != "no-referrer" {
		t.Fatalf("Referrer-Policy = %q, want no-referrer", recorder.Header().Get("Referrer-Policy"))
	}
	if recorder.Header().Get("Strict-Transport-Security") != "" {
		t.Fatalf("Strict-Transport-Security = %q, want empty for non-https", recorder.Header().Get("Strict-Transport-Security"))
	}
}

func TestSecurityHeadersAddsHSTSForHTTPS(t *testing.T) {
	handler := SecurityHeaders(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	request := httptest.NewRequest(http.MethodGet, "/v1/ping", nil)
	request.TLS = &tls.ConnectionState{}
	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, request)

	if recorder.Header().Get("Strict-Transport-Security") != "max-age=31536000; includeSubDomains" {
		t.Fatalf("Strict-Transport-Security = %q", recorder.Header().Get("Strict-Transport-Security"))
	}
}
