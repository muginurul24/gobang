package nexusggr

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestUserCreateSuccess(t *testing.T) {
	client := NewClient(Config{
		BaseURL:    "https://nexusggr.test",
		AgentCode:  "demo-agent",
		AgentToken: "demo-token",
		Timeout:    time.Second,
	}, slog.New(slog.NewTextHandler(io.Discard, nil)), stubHTTPClient(func(req *http.Request) (*http.Response, error) {
		payload, err := io.ReadAll(req.Body)
		if err != nil {
			t.Fatalf("ReadAll returned error: %v", err)
		}

		if !strings.Contains(string(payload), `"method":"user_create"`) {
			t.Fatalf("payload = %s, want user_create method", string(payload))
		}

		return jsonResponse(http.StatusOK, `{"status":1,"msg":"SUCCESS","user_code":"ABCDEF123456","user_balance":0}`), nil
	}))

	result, err := client.UserCreate(context.Background(), UserCreateInput{
		UserCode: "ABCDEF123456",
	})
	if err != nil {
		t.Fatalf("UserCreate returned error: %v", err)
	}

	if result.UserCode != "ABCDEF123456" {
		t.Fatalf("UserCode = %s, want ABCDEF123456", result.UserCode)
	}
}

func TestBusinessFailureNormalizesMsg(t *testing.T) {
	client := NewClient(Config{
		BaseURL:    "https://nexusggr.test",
		AgentCode:  "demo-agent",
		AgentToken: "demo-token",
		Timeout:    time.Second,
	}, slog.New(slog.NewTextHandler(io.Discard, nil)), stubHTTPClient(func(*http.Request) (*http.Response, error) {
		return jsonResponse(http.StatusOK, `{"status":0,"msg":"Invalid Agent."}`), nil
	}))

	_, err := client.ProviderList(context.Background())
	var businessErr *BusinessError
	if !errors.As(err, &businessErr) {
		t.Fatalf("ProviderList error = %v, want BusinessError", err)
	}

	if businessErr.Code != "INVALID_AGENT" {
		t.Fatalf("BusinessError.Code = %s, want INVALID_AGENT", businessErr.Code)
	}
}

func TestBusinessFailureNormalizesErrorField(t *testing.T) {
	client := NewClient(Config{
		BaseURL:    "https://nexusggr.test",
		AgentCode:  "demo-agent",
		AgentToken: "demo-token",
		Timeout:    time.Second,
	}, slog.New(slog.NewTextHandler(io.Discard, nil)), stubHTTPClient(func(*http.Request) (*http.Response, error) {
		return jsonResponse(http.StatusOK, `{"status":0,"error":"INVALID_PARAMETER"}`), nil
	}))

	_, err := client.ProviderList(context.Background())
	var businessErr *BusinessError
	if !errors.As(err, &businessErr) {
		t.Fatalf("ProviderList error = %v, want BusinessError", err)
	}

	if businessErr.Code != "INVALID_PARAMETER" {
		t.Fatalf("BusinessError.Code = %s, want INVALID_PARAMETER", businessErr.Code)
	}
}

func TestTimeoutIsNormalized(t *testing.T) {
	client := NewClient(Config{
		BaseURL:    "https://nexusggr.test",
		AgentCode:  "demo-agent",
		AgentToken: "demo-token",
		Timeout:    10 * time.Millisecond,
	}, slog.New(slog.NewTextHandler(io.Discard, nil)), stubHTTPClient(func(*http.Request) (*http.Response, error) {
		return nil, context.DeadlineExceeded
	}))

	_, err := client.ProviderList(context.Background())
	if !errors.Is(err, ErrTimeout) {
		t.Fatalf("ProviderList error = %v, want ErrTimeout", err)
	}
}

func TestLogsMaskSensitiveFields(t *testing.T) {
	var buffer bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buffer, nil))

	client := NewClient(Config{
		BaseURL:    "https://nexusggr.test",
		AgentCode:  "demo-agent",
		AgentToken: "super-secret-token",
		Timeout:    time.Second,
	}, logger, stubHTTPClient(func(*http.Request) (*http.Response, error) {
		return jsonResponse(http.StatusOK, `{"status":1,"msg":"SUCCESS","agent_balance":1000,"user_balance":500}`), nil
	}))

	_, err := client.UserDeposit(context.Background(), TransferInput{
		UserCode:  "ABCDEF123456",
		Amount:    1000,
		AgentSign: "sign-1234567890",
	})
	if err != nil {
		t.Fatalf("UserDeposit returned error: %v", err)
	}

	logOutput := buffer.String()
	if strings.Contains(logOutput, "super-secret-token") {
		t.Fatal("log output leaked agent token")
	}

	if strings.Contains(logOutput, "ABCDEF123456") {
		t.Fatal("log output leaked full user code")
	}

	if !strings.Contains(logOutput, "AB********56") {
		t.Fatalf("log output = %s, want masked user code", logOutput)
	}
}

type stubHTTPClient func(req *http.Request) (*http.Response, error)

func (f stubHTTPClient) Do(req *http.Request) (*http.Response, error) {
	return f(req)
}

func jsonResponse(statusCode int, body string) *http.Response {
	return &http.Response{
		StatusCode: statusCode,
		Header: http.Header{
			"Content-Type": []string{"application/json"},
		},
		Body: io.NopCloser(strings.NewReader(body)),
	}
}
