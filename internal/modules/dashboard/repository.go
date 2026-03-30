package dashboard

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type RepositoryContract interface {
	GetStoreMetricsForUser(ctx context.Context, userID string, dayStart time.Time, dayEnd time.Time, monthStart time.Time, monthEnd time.Time) (StoreMetrics, error)
	GetPlatformMetrics(ctx context.Context, dayStart time.Time, dayEnd time.Time, monthStart time.Time, monthEnd time.Time, since time.Time) (PlatformMetrics, error)
}

type Repository struct {
	pool *pgxpool.Pool
}

func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

func (r *Repository) GetStoreMetricsForUser(ctx context.Context, userID string, dayStart time.Time, dayEnd time.Time, monthStart time.Time, monthEnd time.Time) (StoreMetrics, error) {
	var metrics StoreMetrics
	err := r.pool.QueryRow(ctx, `
		WITH accessible_stores AS (
			SELECT s.id, s.current_balance
			FROM stores s
			WHERE s.owner_user_id = $1
				AND s.deleted_at IS NULL

			UNION

			SELECT s.id, s.current_balance
			FROM store_staff ss
			INNER JOIN stores s ON s.id = ss.store_id
			WHERE ss.user_id = $1
				AND s.deleted_at IS NULL
		)
		SELECT
			(SELECT COUNT(*)::int FROM accessible_stores) AS accessible_store_count,
			COALESCE((SELECT SUM(current_balance) FROM accessible_stores), 0)::text AS balance_total,
			(
				SELECT COUNT(*)::int
				FROM qris_transactions qt
				INNER JOIN accessible_stores s ON s.id = qt.store_id
				WHERE qt.status = 'pending'
			) AS pending_qris_count,
			(
				SELECT COUNT(*)::int
				FROM qris_transactions qt
				INNER JOIN accessible_stores s ON s.id = qt.store_id
				WHERE qt.status = 'success'
					AND qt.updated_at >= $2
					AND qt.updated_at < $3
			) AS success_today_count,
			(
				SELECT COUNT(*)::int
				FROM qris_transactions qt
				INNER JOIN accessible_stores s ON s.id = qt.store_id
				WHERE qt.status = 'expired'
					AND qt.updated_at >= $2
					AND qt.updated_at < $3
			) AS expired_today_count,
			COALESCE((
				SELECT SUM(qt.store_credit_amount)
				FROM qris_transactions qt
				INNER JOIN accessible_stores s ON s.id = qt.store_id
				WHERE qt.type = 'member_payment'
					AND qt.status = 'success'
					AND qt.updated_at >= $4
					AND qt.updated_at < $5
			), 0)::text AS monthly_store_income
	`, userID, dayStart, dayEnd, monthStart, monthEnd).Scan(
		&metrics.AccessibleStoreCount,
		&metrics.BalanceTotal,
		&metrics.PendingQRISCount,
		&metrics.SuccessTodayCount,
		&metrics.ExpiredTodayCount,
		&metrics.MonthlyStoreIncome,
	)
	if err != nil {
		return StoreMetrics{}, fmt.Errorf("get store dashboard metrics: %w", err)
	}

	return metrics, nil
}

func (r *Repository) GetPlatformMetrics(ctx context.Context, dayStart time.Time, dayEnd time.Time, monthStart time.Time, monthEnd time.Time, since time.Time) (PlatformMetrics, error) {
	var metrics PlatformMetrics
	err := r.pool.QueryRow(ctx, `
		WITH upstream_attempts AS (
			SELECT status, created_at
			FROM qris_reconcile_attempts
			WHERE created_at >= $5

			UNION ALL

			SELECT status, created_at
			FROM withdrawal_status_checks
			WHERE created_at >= $5
		)
		SELECT
			(
				COALESCE((
					SELECT SUM(platform_fee_amount)
					FROM qris_transactions
					WHERE type = 'member_payment'
						AND status = 'success'
						AND updated_at >= $1
						AND updated_at < $2
				), 0)
				+
				COALESCE((
					SELECT SUM(platform_fee_amount)
					FROM store_withdrawals
					WHERE status = 'success'
						AND updated_at >= $1
						AND updated_at < $2
				), 0)
			)::text AS platform_income_today,
			(
				COALESCE((
					SELECT SUM(platform_fee_amount)
					FROM qris_transactions
					WHERE type = 'member_payment'
						AND status = 'success'
						AND updated_at >= $3
						AND updated_at < $4
				), 0)
				+
				COALESCE((
					SELECT SUM(platform_fee_amount)
					FROM store_withdrawals
					WHERE status = 'success'
						AND updated_at >= $3
						AND updated_at < $4
				), 0)
			)::text AS platform_income_month,
			(SELECT COUNT(*)::int FROM stores WHERE deleted_at IS NULL) AS total_store_count,
			(SELECT COUNT(*)::int FROM store_withdrawals WHERE status = 'pending') AS pending_withdraw_count,
			COALESCE((
				SELECT ROUND(
					100.0 * COUNT(*) FILTER (WHERE status = 'upstream_error') / NULLIF(COUNT(*), 0),
					2
				)::float8
				FROM upstream_attempts
			), 0)::float8 AS upstream_error_rate_24h,
			COALESCE((
				SELECT ROUND(
					100.0 * COUNT(*) FILTER (WHERE status = 'failed') / NULLIF(COUNT(*), 0),
					2
				)::float8
				FROM outbound_callback_attempts
				WHERE created_at >= $5
			), 0)::float8 AS callback_failure_rate_24h
	`, dayStart, dayEnd, monthStart, monthEnd, since).Scan(
		&metrics.PlatformIncomeToday,
		&metrics.PlatformIncomeMonth,
		&metrics.TotalStoreCount,
		&metrics.PendingWithdrawCount,
		&metrics.UpstreamErrorRate24h,
		&metrics.CallbackFailureRate24h,
	)
	if err != nil {
		return PlatformMetrics{}, fmt.Errorf("get platform dashboard metrics: %w", err)
	}

	return metrics, nil
}
