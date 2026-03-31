package stores

import (
	"context"
	"fmt"
	"math/big"
	"strings"
	"time"

	"github.com/mugiew/onixggr/internal/modules/ledger"
	"github.com/mugiew/onixggr/internal/platform/clock"
)

const lowBalanceSweepLockNamespace = 32032

type LowBalanceSweepLock interface {
	Unlock(ctx context.Context) error
}

type LowBalanceMonitorRepository interface {
	AcquireLowBalanceSweepLock(ctx context.Context) (LowBalanceSweepLock, bool, error)
	ListStoresForLowBalanceSweep(ctx context.Context) ([]Store, error)
	HasRecentLowBalanceNotification(ctx context.Context, storeID string, since time.Time) (bool, error)
}

type LowBalanceLedger interface {
	GetBalance(ctx context.Context, storeID string) (ledger.BalanceSnapshot, error)
}

type LowBalanceNotificationEmitter interface {
	Emit(storeID string, eventType string, title string, body string)
}

type LowBalanceSweepResult struct {
	Scanned         int
	Alerted         int
	SkippedHealthy  int
	SkippedCooldown int
	Errors          int
	SkippedLocked   bool
}

type LowBalanceMonitor interface {
	Sweep(ctx context.Context) (LowBalanceSweepResult, error)
}

type LowBalanceMonitorOptions struct {
	Repository    LowBalanceMonitorRepository
	Ledger        LowBalanceLedger
	Notifications LowBalanceNotificationEmitter
	Clock         clock.Clock
	Cooldown      time.Duration
}

type lowBalanceMonitor struct {
	repository    LowBalanceMonitorRepository
	ledger        LowBalanceLedger
	notifications LowBalanceNotificationEmitter
	clock         clock.Clock
	cooldown      time.Duration
}

func NewLowBalanceMonitor(options LowBalanceMonitorOptions) LowBalanceMonitor {
	now := options.Clock
	if now == nil {
		now = clock.SystemClock{}
	}

	cooldown := options.Cooldown
	if cooldown <= 0 {
		cooldown = 6 * time.Hour
	}

	notifications := options.Notifications
	if notifications == nil {
		notifications = noopLowBalanceNotificationEmitter{}
	}

	return &lowBalanceMonitor{
		repository:    options.Repository,
		ledger:        options.Ledger,
		notifications: notifications,
		clock:         now,
		cooldown:      cooldown,
	}
}

func (m *lowBalanceMonitor) Sweep(ctx context.Context) (LowBalanceSweepResult, error) {
	if m.repository == nil || m.ledger == nil {
		return LowBalanceSweepResult{}, nil
	}

	lock, locked, err := m.repository.AcquireLowBalanceSweepLock(ctx)
	if err != nil {
		return LowBalanceSweepResult{}, err
	}
	if !locked {
		return LowBalanceSweepResult{SkippedLocked: true}, nil
	}
	defer func() {
		_ = lock.Unlock(ctx)
	}()

	stores, err := m.repository.ListStoresForLowBalanceSweep(ctx)
	if err != nil {
		return LowBalanceSweepResult{}, err
	}

	now := m.clock.Now().UTC()
	since := now.Add(-m.cooldown)
	result := LowBalanceSweepResult{}

	for _, store := range stores {
		result.Scanned++

		thresholdValue, ok := parseLowBalanceAmount(store.LowBalanceThreshold)
		if !ok {
			result.Errors++
			continue
		}

		balance, err := m.ledger.GetBalance(ctx, store.ID)
		if err != nil {
			result.Errors++
			continue
		}

		availableValue, ok := parseAmount(balance.AvailableBalance)
		if !ok {
			result.Errors++
			continue
		}

		if availableValue.Cmp(thresholdValue) > 0 {
			result.SkippedHealthy++
			continue
		}

		recent, err := m.repository.HasRecentLowBalanceNotification(ctx, store.ID, since)
		if err != nil {
			result.Errors++
			continue
		}
		if recent {
			result.SkippedCooldown++
			continue
		}

		thresholdText := strings.TrimSpace(*store.LowBalanceThreshold)
		availableText := strings.TrimSpace(balance.AvailableBalance)
		m.notifications.Emit(store.ID, "store.low_balance",
			"Saldo toko rendah",
			fmt.Sprintf("Saldo tersedia toko %s tersisa %s dan sudah menyentuh threshold %s.", store.Name, availableText, thresholdText),
		)
		result.Alerted++
	}

	return result, nil
}

func parseLowBalanceAmount(raw *string) (*big.Rat, bool) {
	if raw == nil {
		return nil, false
	}

	threshold, ok := parseAmount(*raw)
	if !ok || threshold.Sign() <= 0 {
		return nil, false
	}

	return threshold, true
}

func parseAmount(raw string) (*big.Rat, bool) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return nil, false
	}

	rat := new(big.Rat)
	if _, ok := rat.SetString(value); !ok {
		return nil, false
	}

	return rat, true
}

type noopLowBalanceNotificationEmitter struct{}

func (noopLowBalanceNotificationEmitter) Emit(string, string, string, string) {}
