package auth

import (
	"context"
	"time"

	"github.com/mugiew/onixggr/internal/platform/clock"
)

type SessionCleanupRepository interface {
	PruneSessionsExpiredBefore(ctx context.Context, cutoff time.Time) (int64, error)
}

type SessionCleanupService interface {
	PruneExpired(ctx context.Context) (int64, error)
}

type SessionCleanupOptions struct {
	Repository SessionCleanupRepository
	Clock      clock.Clock
}

type sessionCleanupService struct {
	repository SessionCleanupRepository
	clock      clock.Clock
}

func NewSessionCleanupService(options SessionCleanupOptions) SessionCleanupService {
	now := options.Clock
	if now == nil {
		now = clock.SystemClock{}
	}

	return &sessionCleanupService{
		repository: options.Repository,
		clock:      now,
	}
}

func (s *sessionCleanupService) PruneExpired(ctx context.Context) (int64, error) {
	return s.repository.PruneSessionsExpiredBefore(ctx, s.clock.Now().UTC())
}
