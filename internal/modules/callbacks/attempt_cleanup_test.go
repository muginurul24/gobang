package callbacks

import (
	"context"
	"testing"
	"time"
)

func TestAttemptCleanupPruneExpiredUsesDefaultRetention(t *testing.T) {
	repository := &stubAttemptCleanupRepository{}
	now := time.Date(2026, 3, 31, 12, 0, 0, 0, time.UTC)
	service := NewAttemptCleanupService(AttemptCleanupOptions{
		Repository: repository,
		Clock:      fixedAttemptCleanupClock{now: now},
	})

	if _, err := service.PruneExpired(context.Background()); err != nil {
		t.Fatalf("PruneExpired() error = %v", err)
	}

	want := now.Add(-30 * 24 * time.Hour)
	if !repository.cutoff.Equal(want) {
		t.Fatalf("cutoff = %v, want %v", repository.cutoff, want)
	}
}

type stubAttemptCleanupRepository struct {
	cutoff time.Time
}

func (s *stubAttemptCleanupRepository) PruneAttemptsBefore(_ context.Context, cutoff time.Time) (int64, error) {
	s.cutoff = cutoff
	return 4, nil
}

type fixedAttemptCleanupClock struct {
	now time.Time
}

func (f fixedAttemptCleanupClock) Now() time.Time {
	return f.now
}
