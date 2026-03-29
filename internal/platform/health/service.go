package health

import (
	"context"
	"time"
)

type Checker struct {
	Name  string
	Check func(context.Context) error
}

type DependencyStatus struct {
	Name   string `json:"name"`
	Status string `json:"status"`
	Error  string `json:"error,omitempty"`
}

type Report struct {
	Status       string             `json:"status"`
	Service      string             `json:"service"`
	Environment  string             `json:"environment"`
	Timestamp    string             `json:"timestamp"`
	Dependencies []DependencyStatus `json:"dependencies,omitempty"`
}

type Service struct {
	serviceName string
	environment string
	timeout     time.Duration
	checkers    []Checker
	now         func() time.Time
}

func New(serviceName string, environment string, timeout time.Duration, checkers ...Checker) Service {
	if timeout <= 0 {
		timeout = 2 * time.Second
	}

	registered := make([]Checker, len(checkers))
	copy(registered, checkers)

	return Service{
		serviceName: serviceName,
		environment: environment,
		timeout:     timeout,
		checkers:    registered,
		now:         time.Now,
	}
}

func (s Service) Liveness() Report {
	return Report{
		Status:      "ok",
		Service:     s.serviceName,
		Environment: s.environment,
		Timestamp:   s.timestamp(),
	}
}

func (s Service) Readiness(ctx context.Context) Report {
	report := Report{
		Status:       "ready",
		Service:      s.serviceName,
		Environment:  s.environment,
		Timestamp:    s.timestamp(),
		Dependencies: make([]DependencyStatus, 0, len(s.checkers)),
	}

	for _, checker := range s.checkers {
		dependency := DependencyStatus{
			Name:   checker.Name,
			Status: "ok",
		}

		checkCtx, cancel := context.WithTimeout(ctx, s.timeout)
		err := checker.Check(checkCtx)
		cancel()
		if err != nil {
			dependency.Status = "error"
			dependency.Error = err.Error()
			report.Status = "not_ready"
		}

		report.Dependencies = append(report.Dependencies, dependency)
	}

	return report
}

func (s Service) timestamp() string {
	now := s.now
	if now == nil {
		now = time.Now
	}

	return now().UTC().Format(time.RFC3339)
}
