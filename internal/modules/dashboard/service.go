package dashboard

import (
	"context"
	"time"

	"github.com/mugiew/onixggr/internal/modules/auth"
	"github.com/mugiew/onixggr/internal/platform/clock"
)

type Service interface {
	GetSummary(ctx context.Context, subject auth.Subject) (Summary, error)
}

type Options struct {
	Repository RepositoryContract
	Clock      clock.Clock
}

type service struct {
	repository RepositoryContract
	clock      clock.Clock
}

func NewService(options Options) Service {
	now := options.Clock
	if now == nil {
		now = clock.SystemClock{}
	}

	return &service{
		repository: options.Repository,
		clock:      now,
	}
}

func (s *service) GetSummary(ctx context.Context, subject auth.Subject) (Summary, error) {
	now := s.clock.Now()
	dayStart, dayEnd := currentDayWindow(now)
	monthStart, monthEnd := currentMonthWindow(now)

	switch subject.Role {
	case auth.RoleOwner, auth.RoleKaryawan:
		metrics, err := s.repository.GetStoreMetricsForUser(ctx, subject.UserID, dayStart, dayEnd, monthStart, monthEnd)
		if err != nil {
			return Summary{}, err
		}

		return Summary{
			Role:         string(subject.Role),
			StoreMetrics: &metrics,
		}, nil
	case auth.RoleDev, auth.RoleSuperadmin:
		metrics, err := s.repository.GetPlatformMetrics(ctx, dayStart, dayEnd, monthStart, monthEnd, now.Add(-24*time.Hour))
		if err != nil {
			return Summary{}, err
		}

		return Summary{
			Role:            string(subject.Role),
			PlatformMetrics: &metrics,
		}, nil
	default:
		return Summary{}, ErrForbidden
	}
}

func currentDayWindow(now time.Time) (time.Time, time.Time) {
	location := now.Location()
	start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, location)
	return start, start.AddDate(0, 0, 1)
}

func currentMonthWindow(now time.Time) (time.Time, time.Time) {
	location := now.Location()
	start := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, location)
	return start, start.AddDate(0, 1, 0)
}
