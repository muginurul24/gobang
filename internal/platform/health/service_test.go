package health

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestLiveness(t *testing.T) {
	service := New("onixggr", "test", time.Second)

	report := service.Liveness()
	if report.Status != "ok" {
		t.Fatalf("Status = %q, want ok", report.Status)
	}

	if report.Service != "onixggr" {
		t.Fatalf("Service = %q, want onixggr", report.Service)
	}

	if len(report.Dependencies) != 0 {
		t.Fatalf("Dependencies len = %d, want 0", len(report.Dependencies))
	}
}

func TestReadinessReturnsNotReadyWhenDependencyFails(t *testing.T) {
	service := New(
		"onixggr",
		"test",
		time.Second,
		Checker{Name: "postgres", Check: func(context.Context) error { return nil }},
		Checker{Name: "redis", Check: func(context.Context) error { return errors.New("boom") }},
	)

	report := service.Readiness(context.Background())
	if report.Status != "not_ready" {
		t.Fatalf("Status = %q, want not_ready", report.Status)
	}

	if len(report.Dependencies) != 2 {
		t.Fatalf("Dependencies len = %d, want 2", len(report.Dependencies))
	}

	if report.Dependencies[1].Status != "error" {
		t.Fatalf("Dependencies[1].Status = %q, want error", report.Dependencies[1].Status)
	}

	if report.Dependencies[1].Error == "" {
		t.Fatal("Dependencies[1].Error = empty, want failure message")
	}
}

func TestReadinessReturnsDegradedWhenOptionalDependencyFails(t *testing.T) {
	service := New("onixggr", "test", time.Second, Checker{
		Name:     "nexusggr",
		Severity: SeverityDegraded,
		Check: func(context.Context) error {
			return errors.New("missing credentials")
		},
	})

	report := service.Readiness(context.Background())
	if report.Status != "degraded" {
		t.Fatalf("Status = %q, want degraded", report.Status)
	}

	if len(report.Dependencies) != 1 || report.Dependencies[0].Status != "degraded" {
		t.Fatalf("Dependencies = %#v, want degraded dependency", report.Dependencies)
	}
}
