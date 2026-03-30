package observability

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type RuntimeSnapshot struct {
	CallbackQueueDepth       int
	GameReconcileBacklog     int
	QRISReconcileBacklog     int
	WithdrawalReconcileDepth int
}

type Snapshotter struct {
	pool *pgxpool.Pool
}

func NewSnapshotter(pool *pgxpool.Pool) *Snapshotter {
	return &Snapshotter{pool: pool}
}

func (s *Snapshotter) Snapshot(ctx context.Context) (RuntimeSnapshot, error) {
	if s == nil || s.pool == nil {
		return RuntimeSnapshot{}, nil
	}

	snapshot := RuntimeSnapshot{}

	if err := s.pool.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM outbound_callbacks
		WHERE status IN ('pending', 'retrying')
	`).Scan(&snapshot.CallbackQueueDepth); err != nil {
		return RuntimeSnapshot{}, fmt.Errorf("count callback queue depth: %w", err)
	}

	if err := s.pool.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM game_transactions
		WHERE status = 'pending'
			AND reconcile_status = 'pending_reconcile'
	`).Scan(&snapshot.GameReconcileBacklog); err != nil {
		return RuntimeSnapshot{}, fmt.Errorf("count game reconcile backlog: %w", err)
	}

	if err := s.pool.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM qris_transactions
		WHERE status = 'pending'
			AND provider_trx_id IS NOT NULL
	`).Scan(&snapshot.QRISReconcileBacklog); err != nil {
		return RuntimeSnapshot{}, fmt.Errorf("count qris reconcile backlog: %w", err)
	}

	if err := s.pool.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM store_withdrawals
		WHERE status = 'pending'
			AND provider_partner_ref_no IS NOT NULL
	`).Scan(&snapshot.WithdrawalReconcileDepth); err != nil {
		return RuntimeSnapshot{}, fmt.Errorf("count withdrawal reconcile backlog: %w", err)
	}

	return snapshot, nil
}

func RunSnapshotLoop(ctx context.Context, metrics *Metrics, snapshotter *Snapshotter, websocketConnections func() int, interval time.Duration) {
	if metrics == nil || snapshotter == nil {
		return
	}

	if interval <= 0 {
		interval = 15 * time.Second
	}

	runOnce := func() {
		if websocketConnections != nil {
			metrics.SetWebsocketConnections(websocketConnections())
		}

		snapshotCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
		defer cancel()

		snapshot, err := snapshotter.Snapshot(snapshotCtx)
		if err != nil {
			return
		}

		metrics.SetCallbackQueueDepth(snapshot.CallbackQueueDepth)
		metrics.SetReconcileBacklog("game", snapshot.GameReconcileBacklog)
		metrics.SetReconcileBacklog("qris", snapshot.QRISReconcileBacklog)
		metrics.SetReconcileBacklog("withdraw", snapshot.WithdrawalReconcileDepth)
	}

	runOnce()

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			runOnce()
		}
	}
}
