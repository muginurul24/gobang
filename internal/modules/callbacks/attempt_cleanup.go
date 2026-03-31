package callbacks

import (
	"context"
	"time"

	"github.com/mugiew/onixggr/internal/platform/clock"
)

type AttemptCleanupRepository interface {
	PruneAttemptsBefore(ctx context.Context, cutoff time.Time) (int64, error)
}

type AttemptCleanupService interface {
	PruneExpired(ctx context.Context) (int64, error)
}

type AttemptCleanupOptions struct {
	Repository      AttemptCleanupRepository
	Clock           clock.Clock
	RetentionPeriod time.Duration
}

type attemptCleanupService struct {
	repository      AttemptCleanupRepository
	clock           clock.Clock
	retentionPeriod time.Duration
}

func NewAttemptCleanupService(options AttemptCleanupOptions) AttemptCleanupService {
	now := options.Clock
	if now == nil {
		now = clock.SystemClock{}
	}

	retention := options.RetentionPeriod
	if retention <= 0 {
		retention = 30 * 24 * time.Hour
	}

	return &attemptCleanupService{
		repository:      options.Repository,
		clock:           now,
		retentionPeriod: retention,
	}
}

func (s *attemptCleanupService) PruneExpired(ctx context.Context) (int64, error) {
	return s.repository.PruneAttemptsBefore(ctx, s.clock.Now().UTC().Add(-s.retentionPeriod))
}
