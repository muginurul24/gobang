package stores

import (
	"context"
	"testing"
	"time"

	"github.com/mugiew/onixggr/internal/modules/ledger"
)

func TestLowBalanceMonitorSweepEmitsNotification(t *testing.T) {
	threshold := "100000.00"
	repository := &stubLowBalanceRepository{
		locked: true,
		stores: []Store{
			{
				ID:                  "store-1",
				Name:                "Demo Store",
				LowBalanceThreshold: &threshold,
			},
		},
	}
	ledgerStub := &stubLowBalanceLedger{
		balances: map[string]ledger.BalanceSnapshot{
			"store-1": {AvailableBalance: "95000.00"},
		},
	}
	notifier := &stubLowBalanceNotifier{}
	monitor := NewLowBalanceMonitor(LowBalanceMonitorOptions{
		Repository:    repository,
		Ledger:        ledgerStub,
		Notifications: notifier,
		Clock:         stubMonitorClock{now: time.Date(2026, 3, 31, 10, 0, 0, 0, time.UTC)},
		Cooldown:      6 * time.Hour,
	})

	result, err := monitor.Sweep(context.Background())
	if err != nil {
		t.Fatalf("Sweep() error = %v", err)
	}

	if result.Alerted != 1 {
		t.Fatalf("Alerted = %d, want 1", result.Alerted)
	}
	if len(notifier.calls) != 1 {
		t.Fatalf("notifications = %d, want 1", len(notifier.calls))
	}
	if notifier.calls[0].eventType != "store.low_balance" {
		t.Fatalf("eventType = %q, want store.low_balance", notifier.calls[0].eventType)
	}
}

func TestLowBalanceMonitorSweepSkipsRecentNotification(t *testing.T) {
	threshold := "100000.00"
	repository := &stubLowBalanceRepository{
		locked: true,
		stores: []Store{
			{
				ID:                  "store-1",
				Name:                "Demo Store",
				LowBalanceThreshold: &threshold,
			},
		},
		recentStores: map[string]bool{"store-1": true},
	}
	ledgerStub := &stubLowBalanceLedger{
		balances: map[string]ledger.BalanceSnapshot{
			"store-1": {AvailableBalance: "50000.00"},
		},
	}
	notifier := &stubLowBalanceNotifier{}
	monitor := NewLowBalanceMonitor(LowBalanceMonitorOptions{
		Repository:    repository,
		Ledger:        ledgerStub,
		Notifications: notifier,
		Clock:         stubMonitorClock{now: time.Date(2026, 3, 31, 10, 0, 0, 0, time.UTC)},
		Cooldown:      6 * time.Hour,
	})

	result, err := monitor.Sweep(context.Background())
	if err != nil {
		t.Fatalf("Sweep() error = %v", err)
	}

	if result.SkippedCooldown != 1 {
		t.Fatalf("SkippedCooldown = %d, want 1", result.SkippedCooldown)
	}
	if len(notifier.calls) != 0 {
		t.Fatalf("notifications = %d, want 0", len(notifier.calls))
	}
}

func TestLowBalanceMonitorSweepSkipsHealthyStore(t *testing.T) {
	threshold := "100000.00"
	repository := &stubLowBalanceRepository{
		locked: true,
		stores: []Store{
			{
				ID:                  "store-1",
				Name:                "Demo Store",
				LowBalanceThreshold: &threshold,
			},
		},
	}
	ledgerStub := &stubLowBalanceLedger{
		balances: map[string]ledger.BalanceSnapshot{
			"store-1": {AvailableBalance: "150000.00"},
		},
	}
	monitor := NewLowBalanceMonitor(LowBalanceMonitorOptions{
		Repository: repository,
		Ledger:     ledgerStub,
		Clock:      stubMonitorClock{now: time.Date(2026, 3, 31, 10, 0, 0, 0, time.UTC)},
		Cooldown:   6 * time.Hour,
	})

	result, err := monitor.Sweep(context.Background())
	if err != nil {
		t.Fatalf("Sweep() error = %v", err)
	}

	if result.SkippedHealthy != 1 {
		t.Fatalf("SkippedHealthy = %d, want 1", result.SkippedHealthy)
	}
}

func TestLowBalanceMonitorSweepReturnsSkippedLocked(t *testing.T) {
	monitor := NewLowBalanceMonitor(LowBalanceMonitorOptions{
		Repository: &stubLowBalanceRepository{locked: false},
		Ledger:     &stubLowBalanceLedger{},
		Clock:      stubMonitorClock{now: time.Date(2026, 3, 31, 10, 0, 0, 0, time.UTC)},
		Cooldown:   6 * time.Hour,
	})

	result, err := monitor.Sweep(context.Background())
	if err != nil {
		t.Fatalf("Sweep() error = %v", err)
	}

	if !result.SkippedLocked {
		t.Fatal("SkippedLocked = false, want true")
	}
}

type stubLowBalanceRepository struct {
	stores        []Store
	recentStores  map[string]bool
	locked        bool
	acquiredSince time.Time
}

func (r *stubLowBalanceRepository) AcquireLowBalanceSweepLock(context.Context) (LowBalanceSweepLock, bool, error) {
	if !r.locked {
		return nil, false, nil
	}

	return stubLowBalanceLock{}, true, nil
}

func (r *stubLowBalanceRepository) ListStoresForLowBalanceSweep(context.Context) ([]Store, error) {
	return r.stores, nil
}

func (r *stubLowBalanceRepository) HasRecentLowBalanceNotification(_ context.Context, storeID string, since time.Time) (bool, error) {
	r.acquiredSince = since
	return r.recentStores[storeID], nil
}

type stubLowBalanceLedger struct {
	balances map[string]ledger.BalanceSnapshot
}

func (s *stubLowBalanceLedger) GetBalance(_ context.Context, storeID string) (ledger.BalanceSnapshot, error) {
	balance, ok := s.balances[storeID]
	if !ok {
		return ledger.BalanceSnapshot{}, ledger.ErrNotFound
	}

	return balance, nil
}

type stubLowBalanceNotifier struct {
	calls []stubLowBalanceNotificationCall
}

type stubLowBalanceNotificationCall struct {
	storeID   string
	eventType string
	title     string
	body      string
}

func (s *stubLowBalanceNotifier) Emit(storeID string, eventType string, title string, body string) {
	s.calls = append(s.calls, stubLowBalanceNotificationCall{
		storeID:   storeID,
		eventType: eventType,
		title:     title,
		body:      body,
	})
}

type stubLowBalanceLock struct{}

func (stubLowBalanceLock) Unlock(context.Context) error { return nil }

type stubMonitorClock struct {
	now time.Time
}

func (s stubMonitorClock) Now() time.Time {
	return s.now
}
