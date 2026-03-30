package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRequireCSRFRejectsProtectedMutationWithoutMatchingTokens(t *testing.T) {
	nextCalled := false
	handler := RequireCSRF(
		"https://app.example.com",
		"csrf_cookie",
		"X-CSRF-Token",
		http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			nextCalled = true
			w.WriteHeader(http.StatusNoContent)
		}),
	)

	request := httptest.NewRequest(http.MethodPost, "/v1/stores/store-1/withdrawals", nil)
	request.Header.Set("Origin", "https://app.example.com")
	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", recorder.Code)
	}
	if nextCalled {
		t.Fatal("next handler unexpectedly called")
	}
	if !strings.Contains(recorder.Body.String(), "CSRF_INVALID") {
		t.Fatalf("body = %q, want CSRF_INVALID", recorder.Body.String())
	}
}

func TestRequireCSRFAcceptsProtectedMutationWithMatchingCookieAndHeader(t *testing.T) {
	nextCalled := false
	handler := RequireCSRF(
		"https://app.example.com",
		"csrf_cookie",
		"X-CSRF-Token",
		http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			nextCalled = true
			w.WriteHeader(http.StatusNoContent)
		}),
	)

	request := httptest.NewRequest(http.MethodPost, "/v1/stores/store-1/withdrawals", nil)
	request.Header.Set("Origin", "https://app.example.com")
	request.Header.Set("X-CSRF-Token", "csrf-value")
	request.AddCookie(&http.Cookie{
		Name:  "csrf_cookie",
		Value: "csrf-value",
	})
	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", recorder.Code)
	}
	if !nextCalled {
		t.Fatal("next handler was not called")
	}
}

func TestRequireCSRFSkipsExemptMutationRoutes(t *testing.T) {
	paths := []string{
		"/v1/auth/login",
		"/v1/store-api/game/deposits",
		"/v1/webhooks/qris",
	}

	for _, path := range paths {
		t.Run(path, func(t *testing.T) {
			nextCalled := false
			handler := RequireCSRF(
				"https://app.example.com",
				"csrf_cookie",
				"X-CSRF-Token",
				http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					nextCalled = true
					w.WriteHeader(http.StatusNoContent)
				}),
			)

			request := httptest.NewRequest(http.MethodPost, path, nil)
			recorder := httptest.NewRecorder()

			handler.ServeHTTP(recorder, request)

			if recorder.Code != http.StatusNoContent {
				t.Fatalf("status = %d, want 204", recorder.Code)
			}
			if !nextCalled {
				t.Fatal("next handler was not called")
			}
		})
	}
}
