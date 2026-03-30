package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestIssueBrowserSessionCookiesSetsHardenedBrowserCookies(t *testing.T) {
	recorder := httptest.NewRecorder()
	expiresAt := time.Date(2026, time.March, 31, 10, 0, 0, 0, time.UTC)

	csrfToken, err := issueBrowserSessionCookies(recorder, "https://app.example.com", "refresh-token-1", expiresAt)
	if err != nil {
		t.Fatalf("issueBrowserSessionCookies error = %v", err)
	}

	response := recorder.Result()
	refreshCookie := cookieByName(response.Cookies(), RefreshCookieName)
	if refreshCookie == nil {
		t.Fatalf("refresh cookie %q not found", RefreshCookieName)
	}
	if !refreshCookie.HttpOnly {
		t.Fatal("refresh cookie HttpOnly = false, want true")
	}
	if !refreshCookie.Secure {
		t.Fatal("refresh cookie Secure = false, want true")
	}
	if refreshCookie.SameSite != http.SameSiteLaxMode {
		t.Fatalf("refresh cookie SameSite = %v, want Lax", refreshCookie.SameSite)
	}
	if refreshCookie.Path != "/v1/auth" {
		t.Fatalf("refresh cookie Path = %q, want /v1/auth", refreshCookie.Path)
	}
	if refreshCookie.Domain != "app.example.com" {
		t.Fatalf("refresh cookie Domain = %q, want app.example.com", refreshCookie.Domain)
	}

	csrfCookie := cookieByName(response.Cookies(), CSRFCookieName)
	if csrfCookie == nil {
		t.Fatalf("csrf cookie %q not found", CSRFCookieName)
	}
	if csrfCookie.HttpOnly {
		t.Fatal("csrf cookie HttpOnly = true, want false")
	}
	if csrfCookie.Path != "/" {
		t.Fatalf("csrf cookie Path = %q, want /", csrfCookie.Path)
	}
	if csrfCookie.Value == "" || csrfCookie.Value != csrfToken {
		t.Fatalf("csrf cookie Value = %q, want issued token", csrfCookie.Value)
	}
}

func TestClearBrowserSessionCookiesExpiresBothCookies(t *testing.T) {
	recorder := httptest.NewRecorder()

	clearBrowserSessionCookies(recorder, "https://app.example.com")

	response := recorder.Result()
	refreshCookie := cookieByName(response.Cookies(), RefreshCookieName)
	if refreshCookie == nil {
		t.Fatalf("refresh cookie %q not found", RefreshCookieName)
	}
	if refreshCookie.MaxAge >= 0 {
		t.Fatalf("refresh cookie MaxAge = %d, want expired", refreshCookie.MaxAge)
	}

	csrfCookie := cookieByName(response.Cookies(), CSRFCookieName)
	if csrfCookie == nil {
		t.Fatalf("csrf cookie %q not found", CSRFCookieName)
	}
	if csrfCookie.MaxAge >= 0 {
		t.Fatalf("csrf cookie MaxAge = %d, want expired", csrfCookie.MaxAge)
	}
}

func cookieByName(cookies []*http.Cookie, name string) *http.Cookie {
	for _, cookie := range cookies {
		if cookie.Name == name {
			return cookie
		}
	}

	return nil
}
