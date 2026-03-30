package auth

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestHandleLoginIssuesBrowserCookiesAndSanitizesResponse(t *testing.T) {
	session := fixtureAuthSession()
	service := &stubHandlerService{loginSession: session}
	mux := http.NewServeMux()
	NewHandler(service, "https://app.example.com").Register(mux)

	request := httptest.NewRequest(http.MethodPost, "/v1/auth/login", strings.NewReader(`{"login":"owner@example.com","password":"OwnerDemo123!"}`))
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()

	mux.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", recorder.Code)
	}
	if recorder.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("Cache-Control = %q, want no-store", recorder.Header().Get("Cache-Control"))
	}

	response := decodeEnvelopeMap(t, recorder.Body.Bytes())
	data := response.Data
	if _, ok := data["refresh_token"]; ok {
		t.Fatal("response unexpectedly includes refresh_token")
	}
	if _, ok := data["refresh_token_expires_at"]; ok {
		t.Fatal("response unexpectedly includes refresh_token_expires_at")
	}
	if data["access_token"] != session.AccessToken {
		t.Fatalf("access_token = %v, want %q", data["access_token"], session.AccessToken)
	}

	cookies := recorder.Result().Cookies()
	if cookieByName(cookies, RefreshCookieName) == nil {
		t.Fatalf("refresh cookie %q not found", RefreshCookieName)
	}
	if cookieByName(cookies, CSRFCookieName) == nil {
		t.Fatalf("csrf cookie %q not found", CSRFCookieName)
	}
}

func TestHandleRefreshReadsRefreshTokenFromCookieWhenBodyEmpty(t *testing.T) {
	session := fixtureAuthSession()
	service := &stubHandlerService{refreshSession: session}
	mux := http.NewServeMux()
	NewHandler(service, "https://app.example.com").Register(mux)

	request := httptest.NewRequest(http.MethodPost, "/v1/auth/refresh", nil)
	request.AddCookie(&http.Cookie{
		Name:  RefreshCookieName,
		Value: "refresh-from-cookie",
	})
	recorder := httptest.NewRecorder()

	mux.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", recorder.Code)
	}
	if service.refreshToken != "refresh-from-cookie" {
		t.Fatalf("refreshToken = %q, want refresh-from-cookie", service.refreshToken)
	}

	response := decodeEnvelopeMap(t, recorder.Body.Bytes())
	if _, ok := response.Data["refresh_token"]; ok {
		t.Fatal("response unexpectedly includes refresh_token")
	}
}

func TestHandleLogoutClearsBrowserCookies(t *testing.T) {
	service := &stubHandlerService{}
	handler := NewHandler(service, "https://app.example.com")

	request := httptest.NewRequest(http.MethodPost, "/v1/auth/logout", nil)
	request = request.WithContext(WithSubject(request.Context(), Subject{
		UserID:     "user-1",
		Role:       RoleOwner,
		SessionJTI: "session-1",
	}))
	recorder := httptest.NewRecorder()

	handler.handleLogout(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", recorder.Code)
	}
	if recorder.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("Cache-Control = %q, want no-store", recorder.Header().Get("Cache-Control"))
	}

	refreshCookie := cookieByName(recorder.Result().Cookies(), RefreshCookieName)
	if refreshCookie == nil || refreshCookie.MaxAge >= 0 {
		t.Fatalf("refresh cookie = %#v, want expired cookie", refreshCookie)
	}
	csrfCookie := cookieByName(recorder.Result().Cookies(), CSRFCookieName)
	if csrfCookie == nil || csrfCookie.MaxAge >= 0 {
		t.Fatalf("csrf cookie = %#v, want expired cookie", csrfCookie)
	}
}

type handlerEnvelope struct {
	Status  bool                   `json:"status"`
	Message string                 `json:"message"`
	Data    map[string]interface{} `json:"data"`
}

func decodeEnvelopeMap(t *testing.T, payload []byte) handlerEnvelope {
	t.Helper()

	var response handlerEnvelope
	if err := json.Unmarshal(payload, &response); err != nil {
		t.Fatalf("json.Unmarshal error = %v", err)
	}

	return response
}

type stubHandlerService struct {
	loginSession   AuthSession
	refreshSession AuthSession
	refreshToken   string
}

func (s *stubHandlerService) Login(context.Context, LoginInput) (AuthSession, error) {
	return s.loginSession, nil
}

func (s *stubHandlerService) Refresh(_ context.Context, refreshToken string, _ RequestMetadata) (AuthSession, error) {
	s.refreshToken = refreshToken
	return s.refreshSession, nil
}

func (s *stubHandlerService) Logout(context.Context, Subject, RequestMetadata) error {
	return nil
}

func (s *stubHandlerService) LogoutAll(context.Context, Subject, RequestMetadata) (int, error) {
	return 0, nil
}

func (s *stubHandlerService) Me(context.Context, Subject) (UserProfile, error) {
	return UserProfile{}, nil
}

func (s *stubHandlerService) GetSecuritySettings(context.Context, Subject) (SecuritySettings, error) {
	return SecuritySettings{}, nil
}

func (s *stubHandlerService) BeginTOTPEnrollment(context.Context, Subject) (TOTPEnrollment, error) {
	return TOTPEnrollment{}, nil
}

func (s *stubHandlerService) EnableTOTP(context.Context, Subject, EnableTOTPInput) (RecoveryCodesPayload, error) {
	return RecoveryCodesPayload{}, nil
}

func (s *stubHandlerService) DisableTOTP(context.Context, Subject, DisableTOTPInput) error {
	return nil
}

func (s *stubHandlerService) UpdateIPAllowlist(context.Context, Subject, UpdateIPAllowlistInput) (SecuritySettings, error) {
	return SecuritySettings{}, nil
}

func (s *stubHandlerService) AuthenticateAccessToken(context.Context, string) (Subject, error) {
	return Subject{}, nil
}

func fixtureAuthSession() AuthSession {
	expiresAt := time.Date(2026, time.March, 31, 10, 0, 0, 0, time.UTC)

	return AuthSession{
		User: UserProfile{
			ID:       "user-1",
			Email:    "owner@example.com",
			Username: "owner-demo",
			Role:     RoleOwner,
		},
		TokenType:             "Bearer",
		AccessToken:           "access-token-1",
		AccessTokenExpiresAt:  expiresAt,
		RefreshToken:          "refresh-token-1",
		RefreshTokenExpiresAt: expiresAt.Add(24 * time.Hour),
		SessionJTI:            "session-1",
	}
}
