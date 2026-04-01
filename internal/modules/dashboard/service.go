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
	Cache      SummaryCache
	Clock      clock.Clock
}

type service struct {
	repository RepositoryContract
	cache      SummaryCache
	clock      clock.Clock
}

func NewService(options Options) Service {
	now := options.Clock
	if now == nil {
		now = clock.SystemClock{}
	}

	cache := options.Cache
	if cache == nil {
		cache = noopSummaryCache{}
	}

	return &service{
		repository: options.Repository,
		cache:      cache,
		clock:      now,
	}
}

func (s *service) GetSummary(ctx context.Context, subject auth.Subject) (Summary, error) {
	now := s.clock.Now()
	dayStart, dayEnd := currentDayWindow(now)
	monthStart, monthEnd := currentMonthWindow(now)
	cacheKey := buildCacheKey(subject, dayStart, monthStart)

	if cached, found, err := s.cache.Get(ctx, cacheKey); err == nil && found {
		return cached, nil
	}

	var summary Summary
	switch subject.Role {
	case auth.RoleOwner, auth.RoleKaryawan:
		metrics, err := s.repository.GetStoreMetricsForUser(ctx, subject.UserID, dayStart, dayEnd, monthStart, monthEnd)
		if err != nil {
			return Summary{}, err
		}

		summary = Summary{
			Role:         string(subject.Role),
			StoreMetrics: &metrics,
		}
	case auth.RoleDev, auth.RoleSuperadmin:
		metrics, err := s.repository.GetPlatformMetrics(ctx, dayStart, dayEnd, monthStart, monthEnd, now.Add(-24*time.Hour))
		if err != nil {
			return Summary{}, err
		}

		summary = Summary{
			Role:            string(subject.Role),
			PlatformMetrics: &metrics,
		}
	default:
		return Summary{}, ErrForbidden
	}

	_ = s.cache.Set(ctx, cacheKey, summary, 15*time.Second)
	return summary, nil
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

func buildCacheKey(subject auth.Subject, dayStart time.Time, monthStart time.Time) string {
	return "dashboard:summary:" + string(subject.Role) + ":" + subject.UserID + ":" + dayStart.Format("2006-01-02") + ":" + monthStart.Format("2006-01")
}
