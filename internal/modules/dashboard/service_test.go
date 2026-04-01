package dashboard

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/mugiew/onixggr/internal/modules/auth"
)

func TestGetSummaryReturnsStoreMetricsForOwner(t *testing.T) {
	repository := &stubRepository{
		storeMetrics: StoreMetrics{
			AccessibleStoreCount: 2,
			ActiveStoreCount:     2,
			LowBalanceStoreCount: 1,
			BalanceTotal:         "150000.00",
			PendingQRISCount:     3,
			SuccessTodayCount:    7,
			ExpiredTodayCount:    1,
			MonthlyStoreIncome:   "420000.00",
		},
	}

	service := NewService(Options{
		Repository: repository,
		Clock:      fixedClock{now: time.Date(2026, time.March, 31, 10, 0, 0, 0, time.FixedZone("WIB", 7*60*60))},
	})

	summary, err := service.GetSummary(context.Background(), auth.Subject{
		UserID: "owner-1",
		Role:   auth.RoleOwner,
	})
	if err != nil {
		t.Fatalf("GetSummary error = %v", err)
	}

	if summary.StoreMetrics == nil {
		t.Fatal("StoreMetrics = nil, want value")
	}
	if summary.PlatformMetrics != nil {
		t.Fatal("PlatformMetrics != nil, want nil")
	}
	if repository.storeUserID != "owner-1" {
		t.Fatalf("store user id = %q, want owner-1", repository.storeUserID)
	}
}

func TestGetSummaryReturnsPlatformMetricsForDev(t *testing.T) {
	repository := &stubRepository{
		platformMetrics: PlatformMetrics{
			PlatformIncomeToday:    "15000.00",
			PlatformIncomeMonth:    "300000.00",
			TotalStoreCount:        8,
			ActiveStoreCount:       7,
			LowBalanceStoreCount:   2,
			PendingWithdrawCount:   2,
			UpstreamErrorRate24h:   4.5,
			CallbackFailureRate24h: 1.25,
		},
	}

	service := NewService(Options{
		Repository: repository,
		Clock:      fixedClock{now: time.Date(2026, time.March, 31, 10, 0, 0, 0, time.UTC)},
	})

	summary, err := service.GetSummary(context.Background(), auth.Subject{
		UserID: "dev-1",
		Role:   auth.RoleDev,
	})
	if err != nil {
		t.Fatalf("GetSummary error = %v", err)
	}

	if summary.PlatformMetrics == nil {
		t.Fatal("PlatformMetrics = nil, want value")
	}
	if summary.StoreMetrics != nil {
		t.Fatal("StoreMetrics != nil, want nil")
	}
}

func TestGetSummaryRejectsUnknownRole(t *testing.T) {
	service := NewService(Options{
		Repository: &stubRepository{},
		Clock:      fixedClock{now: time.Now()},
	})

	_, err := service.GetSummary(context.Background(), auth.Subject{
		UserID: "user-1",
		Role:   auth.Role("guest"),
	})
	if !errors.Is(err, ErrForbidden) {
		t.Fatalf("error = %v, want ErrForbidden", err)
	}
}

type stubRepository struct {
	storeMetrics    StoreMetrics
	platformMetrics PlatformMetrics
	storeUserID     string
}

func (s *stubRepository) GetStoreMetricsForUser(_ context.Context, userID string, _ time.Time, _ time.Time, _ time.Time, _ time.Time) (StoreMetrics, error) {
	s.storeUserID = userID
	return s.storeMetrics, nil
}

func (s *stubRepository) GetPlatformMetrics(context.Context, time.Time, time.Time, time.Time, time.Time, time.Time) (PlatformMetrics, error) {
	return s.platformMetrics, nil
}

type fixedClock struct {
	now time.Time
}

func (f fixedClock) Now() time.Time {
	return f.now
}
