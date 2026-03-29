package httpserver

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/mugiew/onixggr/internal/platform/config"
	"github.com/mugiew/onixggr/internal/platform/health"
)

func TestHealthzReturnsOK(t *testing.T) {
	handler := NewHandler(
		config.Config{
			App: config.AppConfig{
				Name: "onixggr",
				Env:  "test",
			},
		},
		Dependencies{
			Health: health.New("onixggr", "test", time.Second),
		},
	)

	request := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("Code = %d, want 200", recorder.Code)
	}
}

func TestReadyzReturnsServiceUnavailableWhenDependencyFails(t *testing.T) {
	handler := NewHandler(
		config.Config{
			App: config.AppConfig{
				Name: "onixggr",
				Env:  "test",
			},
		},
		Dependencies{
			Health: health.New(
				"onixggr",
				"test",
				time.Second,
				health.Checker{
					Name: "postgres",
					Check: func(context.Context) error {
						return errors.New("database unavailable")
					},
				},
			),
		},
	)

	request := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("Code = %d, want 503", recorder.Code)
	}
}
