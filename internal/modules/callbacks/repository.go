package callbacks

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository struct {
	pool *pgxpool.Pool
}

func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

func (r *Repository) FindMemberPaymentCallbackSource(ctx context.Context, qrisTransactionID string) (MemberPaymentCallbackSource, error) {
	var source MemberPaymentCallbackSource
	var storeMemberID *string
	var providerTrxID *string
	err := r.pool.QueryRow(ctx, `
		SELECT
			qt.id,
			qt.store_id,
			qt.store_member_id,
			COALESCE(sm.real_username, ''),
			qt.custom_ref,
			qt.provider_trx_id,
			qt.amount_gross::text,
			qt.platform_fee_amount::text,
			qt.store_credit_amount::text,
			qt.status,
			qt.updated_at
		FROM qris_transactions qt
		INNER JOIN stores s ON s.id = qt.store_id
		LEFT JOIN store_members sm ON sm.id = qt.store_member_id
		WHERE qt.id = $1::uuid
			AND qt.type = 'member_payment'
			AND s.deleted_at IS NULL
		LIMIT 1
	`, strings.TrimSpace(qrisTransactionID)).Scan(
		&source.QRISTransactionID,
		&source.StoreID,
		&storeMemberID,
		&source.RealUsername,
		&source.CustomRef,
		&providerTrxID,
		&source.AmountGross,
		&source.PlatformFeeAmount,
		&source.StoreCreditAmount,
		&source.TransactionStatus,
		&source.TransactionUpdateAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return MemberPaymentCallbackSource{}, ErrNotFound
		}

		return MemberPaymentCallbackSource{}, fmt.Errorf("find member payment callback source: %w", err)
	}

	source.StoreMemberID = storeMemberID
	source.ProviderTrxID = providerTrxID
	return source, nil
}

func (r *Repository) EnqueueOutboundCallback(ctx context.Context, params EnqueueOutboundCallbackParams) (OutboundCallback, error) {
	var callback OutboundCallback
	var payloadRaw []byte
	err := r.pool.QueryRow(ctx, `
		INSERT INTO outbound_callbacks (
			store_id,
			event_type,
			reference_type,
			reference_id,
			payload_json,
			signature,
			status,
			created_at,
			updated_at
		)
		VALUES ($1::uuid, $2, $3, $4::uuid, $5::jsonb, $6, 'pending', $7, $7)
		ON CONFLICT (event_type, reference_type, reference_id)
		DO UPDATE SET
			payload_json = EXCLUDED.payload_json,
			signature = EXCLUDED.signature,
			status = CASE
				WHEN outbound_callbacks.status = 'success' THEN outbound_callbacks.status
				ELSE 'pending'
			END,
			updated_at = EXCLUDED.updated_at
		RETURNING
			id,
			store_id,
			event_type,
			reference_type,
			reference_id::text,
			payload_json,
			signature,
			status,
			created_at,
			updated_at
	`, params.StoreID, params.EventType, params.ReferenceType, params.ReferenceID, string(params.PayloadJSON), params.Signature, params.OccurredAt).Scan(
		&callback.ID,
		&callback.StoreID,
		&callback.EventType,
		&callback.ReferenceType,
		&callback.ReferenceID,
		&payloadRaw,
		&callback.Signature,
		&callback.Status,
		&callback.CreatedAt,
		&callback.UpdatedAt,
	)
	if err != nil {
		return OutboundCallback{}, fmt.Errorf("enqueue outbound callback: %w", err)
	}

	callback.PayloadJSON = payloadRaw
	return callback, nil
}

func (r *Repository) ListDueOutboundCallbacks(ctx context.Context, now time.Time, limit int) ([]DueOutboundCallback, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT
			c.id,
			c.store_id,
			s.callback_url,
			c.event_type,
			c.reference_type,
			c.reference_id::text,
			c.payload_json,
			c.signature,
			c.status,
			c.created_at,
			c.updated_at,
			COALESCE(last_attempt.attempt_no, 0) AS attempt_no
		FROM outbound_callbacks c
		INNER JOIN stores s ON s.id = c.store_id
		LEFT JOIN LATERAL (
			SELECT attempt_no, next_retry_at
			FROM outbound_callback_attempts
			WHERE outbound_callback_id = c.id
			ORDER BY attempt_no DESC
			LIMIT 1
		) AS last_attempt ON TRUE
		WHERE c.status IN ('pending', 'retrying')
			AND s.deleted_at IS NULL
			AND NULLIF(BTRIM(s.callback_url), '') IS NOT NULL
			AND (
				c.status = 'pending'
				OR last_attempt.next_retry_at IS NULL
				OR last_attempt.next_retry_at <= $1
			)
		ORDER BY c.created_at ASC
		LIMIT $2
	`, now.UTC(), limit)
	if err != nil {
		return nil, fmt.Errorf("list due outbound callbacks: %w", err)
	}
	defer rows.Close()

	var callbacks []DueOutboundCallback
	for rows.Next() {
		var callback DueOutboundCallback
		var payloadRaw []byte
		if err := rows.Scan(
			&callback.ID,
			&callback.StoreID,
			&callback.CallbackURL,
			&callback.EventType,
			&callback.ReferenceType,
			&callback.ReferenceID,
			&payloadRaw,
			&callback.Signature,
			&callback.Status,
			&callback.CreatedAt,
			&callback.UpdatedAt,
			&callback.AttemptNo,
		); err != nil {
			return nil, fmt.Errorf("scan due outbound callback: %w", err)
		}

		callback.PayloadJSON = payloadRaw
		callbacks = append(callbacks, callback)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate due outbound callbacks: %w", err)
	}

	return callbacks, nil
}

func (r *Repository) RecordAttempt(ctx context.Context, params RecordAttemptParams) error {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("begin record callback attempt tx: %w", err)
	}
	defer tx.Rollback(ctx)

	_, err = tx.Exec(ctx, `
		INSERT INTO outbound_callback_attempts (
			outbound_callback_id,
			attempt_no,
			http_status,
			status,
			response_body_masked,
			next_retry_at,
			created_at
		)
		VALUES ($1::uuid, $2, $3, $4, $5, $6, $7)
	`, params.OutboundCallbackID, params.AttemptNo, params.HTTPStatus, params.Status, params.ResponseBodyMasked, params.NextRetryAt, params.OccurredAt)
	if err != nil {
		if isUniqueViolation(err, "outbound_callback_attempts_outbound_callback_id_attempt_no_unique") {
			return ErrDuplicateAttempt
		}

		return fmt.Errorf("insert outbound callback attempt: %w", err)
	}

	_, err = tx.Exec(ctx, `
		UPDATE outbound_callbacks
		SET status = $2, updated_at = $3
		WHERE id = $1::uuid
	`, params.OutboundCallbackID, params.CallbackStatus, params.OccurredAt)
	if err != nil {
		return fmt.Errorf("update outbound callback status: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit record callback attempt tx: %w", err)
	}

	return nil
}

func (r *Repository) ListQueue(ctx context.Context, filter ListQueueFilter) (QueuePage, error) {
	filter = normalizeQueueFilter(filter)

	summary, err := r.listQueueSummary(ctx, filter)
	if err != nil {
		return QueuePage{}, err
	}

	rows, err := r.pool.Query(ctx, `
		SELECT
			c.id,
			c.store_id,
			s.name,
			s.slug,
			s.callback_url,
			c.event_type,
			c.reference_type,
			c.reference_id::text,
			c.status,
			c.created_at,
			c.updated_at,
			COALESCE(last_attempt.attempt_no, 0) AS latest_attempt_no,
			last_attempt.http_status,
			last_attempt.status,
			COALESCE(last_attempt.response_body_masked, ''),
			last_attempt.next_retry_at,
			last_attempt.created_at
		FROM outbound_callbacks c
		INNER JOIN stores s ON s.id = c.store_id
		LEFT JOIN LATERAL (
			SELECT
				attempt_no,
				http_status,
				status,
				response_body_masked,
				next_retry_at,
				created_at
			FROM outbound_callback_attempts
			WHERE outbound_callback_id = c.id
			ORDER BY attempt_no DESC
			LIMIT 1
		) AS last_attempt ON TRUE
		WHERE s.deleted_at IS NULL
			AND ($1 = '' OR (
				c.id::text ILIKE '%' || $1 || '%'
				OR c.event_type ILIKE '%' || $1 || '%'
				OR c.reference_type ILIKE '%' || $1 || '%'
				OR c.reference_id::text ILIKE '%' || $1 || '%'
				OR s.name ILIKE '%' || $1 || '%'
				OR s.slug ILIKE '%' || $1 || '%'
				OR COALESCE(s.callback_url, '') ILIKE '%' || $1 || '%'
			))
			AND ($2 = '' OR c.status = $2)
			AND (NULLIF($3, '')::uuid IS NULL OR c.store_id = NULLIF($3, '')::uuid)
			AND ($4::timestamptz IS NULL OR c.created_at >= $4)
			AND ($5::timestamptz IS NULL OR c.created_at <= $5)
		ORDER BY c.created_at DESC, c.id DESC
		LIMIT $6 OFFSET $7
	`, filter.Query, stringValue(filter.Status), stringValue(filter.StoreID), filter.CreatedFrom, filter.CreatedTo, filter.Limit, filter.Offset)
	if err != nil {
		return QueuePage{}, fmt.Errorf("list callback queue: %w", err)
	}
	defer rows.Close()

	items := make([]QueueItem, 0, filter.Limit)
	for rows.Next() {
		var item QueueItem
		var latestAttemptStatus *AttemptStatus
		if err := rows.Scan(
			&item.ID,
			&item.StoreID,
			&item.StoreName,
			&item.StoreSlug,
			&item.CallbackURL,
			&item.EventType,
			&item.ReferenceType,
			&item.ReferenceID,
			&item.Status,
			&item.CreatedAt,
			&item.UpdatedAt,
			&item.LatestAttemptNo,
			&item.LatestHTTPStatus,
			&latestAttemptStatus,
			&item.LatestResponseBodyMasked,
			&item.LatestNextRetryAt,
			&item.LatestAttemptAt,
		); err != nil {
			return QueuePage{}, fmt.Errorf("scan callback queue item: %w", err)
		}

		item.LatestAttemptStatus = latestAttemptStatus
		items = append(items, item)
	}

	if err := rows.Err(); err != nil {
		return QueuePage{}, fmt.Errorf("iterate callback queue: %w", err)
	}

	return QueuePage{
		Items:   items,
		Summary: summary,
		Limit:   filter.Limit,
		Offset:  filter.Offset,
	}, nil
}

func (r *Repository) ListAttempts(ctx context.Context, callbackID string, limit int, offset int) (AttemptPage, error) {
	trimmedCallbackID := strings.TrimSpace(callbackID)
	if trimmedCallbackID == "" {
		return AttemptPage{}, ErrNotFound
	}

	limit = clampLimit(limit, 50, 200)
	offset = clampOffset(offset)

	var exists bool
	if err := r.pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1
			FROM outbound_callbacks
			WHERE id = $1::uuid
		)
	`, trimmedCallbackID).Scan(&exists); err != nil {
		return AttemptPage{}, fmt.Errorf("check callback attempts parent: %w", err)
	}
	if !exists {
		return AttemptPage{}, ErrNotFound
	}

	rows, err := r.pool.Query(ctx, `
		SELECT
			id,
			outbound_callback_id,
			attempt_no,
			http_status,
			status,
			response_body_masked,
			next_retry_at,
			created_at
		FROM outbound_callback_attempts
		WHERE outbound_callback_id = $1::uuid
		ORDER BY attempt_no DESC
		LIMIT $2 OFFSET $3
	`, trimmedCallbackID, limit, offset)
	if err != nil {
		return AttemptPage{}, fmt.Errorf("list callback attempts: %w", err)
	}
	defer rows.Close()

	items := make([]AttemptRecord, 0, limit)
	for rows.Next() {
		var item AttemptRecord
		if err := rows.Scan(
			&item.ID,
			&item.OutboundCallbackID,
			&item.AttemptNo,
			&item.HTTPStatus,
			&item.Status,
			&item.ResponseBodyMasked,
			&item.NextRetryAt,
			&item.CreatedAt,
		); err != nil {
			return AttemptPage{}, fmt.Errorf("scan callback attempt: %w", err)
		}

		items = append(items, item)
	}

	if err := rows.Err(); err != nil {
		return AttemptPage{}, fmt.Errorf("iterate callback attempts: %w", err)
	}

	var totalCount int
	if err := r.pool.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM outbound_callback_attempts
		WHERE outbound_callback_id = $1::uuid
	`, trimmedCallbackID).Scan(&totalCount); err != nil {
		return AttemptPage{}, fmt.Errorf("count callback attempts: %w", err)
	}

	return AttemptPage{
		CallbackID: trimmedCallbackID,
		Items:      items,
		TotalCount: totalCount,
		Limit:      limit,
		Offset:     offset,
	}, nil
}

func (r *Repository) PruneAttemptsBefore(ctx context.Context, cutoff time.Time) (int64, error) {
	commandTag, err := r.pool.Exec(ctx, `
		DELETE FROM outbound_callback_attempts attempts
		USING outbound_callbacks callbacks
		WHERE attempts.outbound_callback_id = callbacks.id
			AND callbacks.status IN ('success', 'failed')
			AND attempts.created_at < $1
	`, cutoff.UTC())
	if err != nil {
		return 0, fmt.Errorf("prune outbound callback attempts: %w", err)
	}

	return commandTag.RowsAffected(), nil
}

func isUniqueViolation(err error, constraintName string) bool {
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		return false
	}

	return pgErr.Code == "23505" && pgErr.ConstraintName == constraintName
}

func toJSON(payload any) string {
	encoded, err := json.Marshal(payload)
	if err != nil {
		return "{}"
	}

	return string(encoded)
}

func (r *Repository) listQueueSummary(ctx context.Context, filter ListQueueFilter) (QueueSummary, error) {
	var summary QueueSummary
	err := r.pool.QueryRow(ctx, `
		SELECT
			COUNT(*) AS total_count,
			COUNT(*) FILTER (WHERE c.status = 'pending') AS pending_count,
			COUNT(*) FILTER (WHERE c.status = 'retrying') AS retrying_count,
			COUNT(*) FILTER (WHERE c.status = 'success') AS success_count,
			COUNT(*) FILTER (WHERE c.status = 'failed') AS failed_count
		FROM outbound_callbacks c
		INNER JOIN stores s ON s.id = c.store_id
		WHERE s.deleted_at IS NULL
			AND ($1 = '' OR (
				c.id::text ILIKE '%' || $1 || '%'
				OR c.event_type ILIKE '%' || $1 || '%'
				OR c.reference_type ILIKE '%' || $1 || '%'
				OR c.reference_id::text ILIKE '%' || $1 || '%'
				OR s.name ILIKE '%' || $1 || '%'
				OR s.slug ILIKE '%' || $1 || '%'
				OR COALESCE(s.callback_url, '') ILIKE '%' || $1 || '%'
			))
			AND ($2 = '' OR c.status = $2)
			AND (NULLIF($3, '')::uuid IS NULL OR c.store_id = NULLIF($3, '')::uuid)
			AND ($4::timestamptz IS NULL OR c.created_at >= $4)
			AND ($5::timestamptz IS NULL OR c.created_at <= $5)
	`, filter.Query, stringValue(filter.Status), stringValue(filter.StoreID), filter.CreatedFrom, filter.CreatedTo).Scan(
		&summary.TotalCount,
		&summary.PendingCount,
		&summary.RetryingCount,
		&summary.SuccessCount,
		&summary.FailedCount,
	)
	if err != nil {
		return QueueSummary{}, fmt.Errorf("list callback queue summary: %w", err)
	}

	return summary, nil
}

func normalizeQueueFilter(filter ListQueueFilter) ListQueueFilter {
	filter.Query = strings.TrimSpace(filter.Query)
	filter.StoreID = normalizeOptionalString(filter.StoreID)
	filter.Status = normalizeOptionalStatus(filter.Status)
	filter.Limit = clampLimit(filter.Limit, 25, 200)
	filter.Offset = clampOffset(filter.Offset)
	return filter
}

func normalizeOptionalString(value *string) *string {
	if value == nil {
		return nil
	}

	trimmed := strings.TrimSpace(*value)
	if trimmed == "" {
		return nil
	}

	return &trimmed
}

func normalizeOptionalStatus(value *Status) *Status {
	if value == nil {
		return nil
	}

	trimmed := Status(strings.TrimSpace(string(*value)))
	if trimmed == "" {
		return nil
	}

	return &trimmed
}

func clampLimit(value int, defaultValue int, maxValue int) int {
	switch {
	case value <= 0:
		return defaultValue
	case value > maxValue:
		return maxValue
	default:
		return value
	}
}

func clampOffset(value int) int {
	if value < 0 {
		return 0
	}

	return value
}

func stringValue[T ~string](value *T) string {
	if value == nil {
		return ""
	}

	return string(*value)
}
