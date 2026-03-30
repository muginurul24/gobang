package realtime

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mugiew/onixggr/internal/modules/auth"
)

func TestWebSocketHandlerRejectsUnauthorizedWithoutAccessToken(t *testing.T) {
	handler := NewHandler(stubRealtimeService{}, nil, "")
	mux := http.NewServeMux()
	handler.Register(mux)

	request := httptest.NewRequest(http.MethodGet, "/v1/realtime/ws", nil)
	response := httptest.NewRecorder()

	mux.ServeHTTP(response, request)

	if response.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", response.Code)
	}
}

func TestWebSocketHandlerRejectsUnauthorizedFromService(t *testing.T) {
	handler := NewHandler(stubRealtimeService{err: auth.ErrUnauthorized}, nil, "")
	mux := http.NewServeMux()
	handler.Register(mux)

	request := httptest.NewRequest(http.MethodGet, "/v1/realtime/ws?access_token=test-token", nil)
	response := httptest.NewRecorder()

	mux.ServeHTTP(response, request)

	if response.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", response.Code)
	}
}

func TestWebSocketHandlerReturnsInternalErrorOnServiceFailure(t *testing.T) {
	handler := NewHandler(stubRealtimeService{err: errors.New("boom")}, nil, "")
	mux := http.NewServeMux()
	handler.Register(mux)

	request := httptest.NewRequest(http.MethodGet, "/v1/realtime/ws?access_token=test-token", nil)
	response := httptest.NewRecorder()

	mux.ServeHTTP(response, request)

	if response.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", response.Code)
	}
}

func TestCheckOriginAllowsSameHost(t *testing.T) {
	handler := NewHandler(stubRealtimeService{}, nil, "https://app.example.com")
	request := httptest.NewRequest(http.MethodGet, "/v1/realtime/ws", nil)
	request.Host = "api.example.com"
	request.Header.Set("Origin", "https://api.example.com")

	if !handler.checkOrigin(request) {
		t.Fatal("checkOrigin() = false, want true")
	}
}

func TestCheckOriginAllowsConfiguredOrigin(t *testing.T) {
	handler := NewHandler(stubRealtimeService{}, nil, "https://app.example.com")
	request := httptest.NewRequest(http.MethodGet, "/v1/realtime/ws", nil)
	request.Host = "api.example.com"
	request.Header.Set("Origin", "https://app.example.com")

	if !handler.checkOrigin(request) {
		t.Fatal("checkOrigin() = false, want true")
	}
}

func TestCheckOriginRejectsUnknownOrigin(t *testing.T) {
	handler := NewHandler(stubRealtimeService{}, nil, "https://app.example.com")
	request := httptest.NewRequest(http.MethodGet, "/v1/realtime/ws", nil)
	request.Host = "api.example.com"
	request.Header.Set("Origin", "https://evil.example.com")

	if handler.checkOrigin(request) {
		t.Fatal("checkOrigin() = true, want false")
	}
}

type stubRealtimeService struct {
	session ConnectionSession
	err     error
}

func (s stubRealtimeService) AuthorizeConnection(context.Context, string) (ConnectionSession, error) {
	if s.err != nil {
		return ConnectionSession{}, s.err
	}

	return s.session, nil
}

func TestWebSocketTokenFromAuthorizationHeader(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/v1/realtime/ws", nil)
	request.Header.Set("Authorization", "Bearer token-123")

	token, ok := websocketToken(request)
	if !ok || token != "token-123" {
		t.Fatalf("token = %q, ok = %v, want token-123 true", token, ok)
	}
}

func TestWebSocketTokenFromQueryString(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/v1/realtime/ws?access_token=token-456", nil)

	token, ok := websocketToken(request)
	if !ok || token != "token-456" {
		t.Fatalf("token = %q, ok = %v, want token-456 true", token, ok)
	}
}

func TestHelloFrameJSONShape(t *testing.T) {
	payload, err := json.Marshal(HelloFrame{
		Kind:         "hello",
		ConnectionID: "conn-1",
		UserID:       "user-1",
		Role:         "owner",
		Channels:     []string{"user:user-1"},
	})
	if err != nil {
		t.Fatalf("Marshal hello frame error = %v", err)
	}

	if !strings.Contains(string(payload), "\"kind\":\"hello\"") {
		t.Fatalf("payload = %s, want hello kind", string(payload))
	}
}
