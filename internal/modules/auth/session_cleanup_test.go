package auth

import (
	"context"
	"testing"
	"time"
)

func TestSessionCleanupPruneExpiredUsesCurrentTimeAsCutoff(t *testing.T) {
	repository := &stubSessionCleanupRepository{}
	now := time.Date(2026, 3, 31, 11, 0, 0, 0, time.UTC)
	service := NewSessionCleanupService(SessionCleanupOptions{
		Repository: repository,
		Clock:      fixedSessionCleanupClock{now: now},
	})

	if _, err := service.PruneExpired(context.Background()); err != nil {
		t.Fatalf("PruneExpired() error = %v", err)
	}

	if !repository.cutoff.Equal(now) {
		t.Fatalf("cutoff = %v, want %v", repository.cutoff, now)
	}
}

type stubSessionCleanupRepository struct {
	cutoff time.Time
}

func (s *stubSessionCleanupRepository) PruneSessionsExpiredBefore(_ context.Context, cutoff time.Time) (int64, error) {
	s.cutoff = cutoff
	return 2, nil
}

type fixedSessionCleanupClock struct {
	now time.Time
}

func (f fixedSessionCleanupClock) Now() time.Time {
	return f.now
}
