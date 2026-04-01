package withdrawals

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository struct {
	pool *pgxpool.Pool
}

const withdrawalProcessingLockNamespace = 31031

type ProcessingLock interface {
	Unlock(ctx context.Context) error
}

func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

func (r *Repository) GetStoreScope(ctx context.Context, storeID string) (StoreScope, error) {
	var store StoreScope
	err := r.pool.QueryRow(ctx, `
		SELECT id, owner_user_id, name, slug, status, low_balance_threshold::text, deleted_at
		FROM stores
		WHERE id = $1
		LIMIT 1
	`, strings.TrimSpace(storeID)).Scan(
		&store.ID,
		&store.OwnerUserID,
		&store.Name,
		&store.Slug,
		&store.Status,
		&store.LowBalanceThreshold,
		&store.DeletedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return StoreScope{}, ErrNotFound
		}

		return StoreScope{}, fmt.Errorf("get store scope: %w", err)
	}

	return store, nil
}

func (r *Repository) GetStoreBankAccount(ctx context.Context, storeID string, bankAccountID string) (StoreBankAccount, error) {
	var account StoreBankAccount
	err := r.pool.QueryRow(ctx, `
		SELECT
			id,
			store_id,
			bank_code,
			bank_name,
			account_name,
			account_number_masked,
			account_number_encrypted,
			is_active
		FROM store_bank_accounts
		WHERE store_id = $1 AND id = $2
		LIMIT 1
	`, strings.TrimSpace(storeID), strings.TrimSpace(bankAccountID)).Scan(
		&account.ID,
		&account.StoreID,
		&account.BankCode,
		&account.BankName,
		&account.AccountName,
		&account.AccountNumberMasked,
		&account.AccountNumberEncrypted,
		&account.IsActive,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return StoreBankAccount{}, ErrNotFound
		}

		return StoreBankAccount{}, fmt.Errorf("get store bank account: %w", err)
	}

	return account, nil
}

func (r *Repository) FindByIdempotencyKey(ctx context.Context, storeID string, idempotencyKey string) (StoreWithdrawal, error) {
	return r.getWithdrawal(ctx, `
		SELECT
			w.id,
			w.store_id,
			w.store_bank_account_id,
			w.idempotency_key,
			a.bank_code,
			a.bank_name,
			a.account_name,
			a.account_number_masked,
			w.net_requested_amount::text,
			w.platform_fee_amount::text,
			w.external_fee_amount::text,
			w.total_store_debit::text,
			w.provider_partner_ref_no,
			w.provider_inquiry_id,
			w.status,
			w.created_at,
			w.updated_at
		FROM store_withdrawals w
		INNER JOIN store_bank_accounts a ON a.id = w.store_bank_account_id
		WHERE w.store_id = $1 AND w.idempotency_key = $2
		LIMIT 1
	`, strings.TrimSpace(storeID), strings.TrimSpace(idempotencyKey))
}

func (r *Repository) FindByPartnerRefNo(ctx context.Context, partnerRefNo string) (StoreWithdrawal, error) {
	return r.getWithdrawal(ctx, `
		SELECT
			w.id,
			w.store_id,
			w.store_bank_account_id,
			w.idempotency_key,
			a.bank_code,
			a.bank_name,
			a.account_name,
			a.account_number_masked,
			w.net_requested_amount::text,
			w.platform_fee_amount::text,
			w.external_fee_amount::text,
			w.total_store_debit::text,
			w.provider_partner_ref_no,
			w.provider_inquiry_id,
			w.status,
			w.created_at,
			w.updated_at
		FROM store_withdrawals w
		INNER JOIN store_bank_accounts a ON a.id = w.store_bank_account_id
		WHERE w.provider_partner_ref_no = $1
		LIMIT 1
	`, strings.TrimSpace(partnerRefNo))
}

func (r *Repository) GetByID(ctx context.Context, withdrawalID string) (StoreWithdrawal, error) {
	return r.getWithdrawal(ctx, `
		SELECT
			w.id,
			w.store_id,
			w.store_bank_account_id,
			w.idempotency_key,
			a.bank_code,
			a.bank_name,
			a.account_name,
			a.account_number_masked,
			w.net_requested_amount::text,
			w.platform_fee_amount::text,
			w.external_fee_amount::text,
			w.total_store_debit::text,
			w.provider_partner_ref_no,
			w.provider_inquiry_id,
			w.status,
			w.created_at,
			w.updated_at
		FROM store_withdrawals w
		INNER JOIN store_bank_accounts a ON a.id = w.store_bank_account_id
		WHERE w.id = $1
		LIMIT 1
	`, strings.TrimSpace(withdrawalID))
}

func (r *Repository) AcquireProcessingLock(ctx context.Context, withdrawalID string) (ProcessingLock, bool, error) {
	conn, err := r.pool.Acquire(ctx)
	if err != nil {
		return nil, false, fmt.Errorf("acquire withdrawal processing connection: %w", err)
	}

	var locked bool
	if err := conn.QueryRow(ctx, `
		SELECT pg_try_advisory_lock($1, hashtext($2))
	`, withdrawalProcessingLockNamespace, strings.TrimSpace(withdrawalID)).Scan(&locked); err != nil {
		conn.Release()
		return nil, false, fmt.Errorf("try withdrawal advisory lock: %w", err)
	}

	if !locked {
		conn.Release()
		return nil, false, nil
	}

	return &repositoryProcessingLock{
		conn:         conn,
		withdrawalID: strings.TrimSpace(withdrawalID),
	}, true, nil
}

func (r *Repository) ListStoreWithdrawalsPage(ctx context.Context, filter ListWithdrawalsFilter) (StoreWithdrawalPage, error) {
	whereClause, args := buildWithdrawalListWhere(filter)

	var summary StoreWithdrawalSummary
	summaryQuery := `
		SELECT
			COUNT(*)::int,
			COUNT(*) FILTER (WHERE status = 'pending')::int,
			COUNT(*) FILTER (WHERE status = 'success')::int,
			COUNT(*) FILTER (WHERE status = 'failed')::int,
			COALESCE(SUM(net_requested_amount), 0)::text,
			COALESCE(SUM(platform_fee_amount), 0)::text,
			COALESCE(SUM(external_fee_amount), 0)::text
		FROM store_withdrawals w
		INNER JOIN store_bank_accounts a ON a.id = w.store_bank_account_id
	` + whereClause
	if err := r.pool.QueryRow(ctx, summaryQuery, args...).Scan(
		&summary.TotalCount,
		&summary.PendingCount,
		&summary.SuccessCount,
		&summary.FailedCount,
		&summary.TotalNetAmount,
		&summary.TotalPlatformFee,
		&summary.TotalExternalFee,
	); err != nil {
		return StoreWithdrawalPage{}, fmt.Errorf("summarize store withdrawals: %w", err)
	}

	page := StoreWithdrawalPage{
		Items:   []StoreWithdrawal{},
		Summary: summary,
		Limit:   filter.Limit,
		Offset:  filter.Offset,
	}
	if summary.TotalCount == 0 {
		return page, nil
	}

	queryArgs := append([]any{}, args...)
	limitPlaceholder := len(queryArgs) + 1
	queryArgs = append(queryArgs, filter.Limit)
	offsetPlaceholder := len(queryArgs) + 1
	queryArgs = append(queryArgs, filter.Offset)

	rows, err := r.pool.Query(ctx, `
		SELECT
			w.id,
			w.store_id,
			w.store_bank_account_id,
			w.idempotency_key,
			a.bank_code,
			a.bank_name,
			a.account_name,
			a.account_number_masked,
			w.net_requested_amount::text,
			w.platform_fee_amount::text,
			w.external_fee_amount::text,
			w.total_store_debit::text,
			w.provider_partner_ref_no,
			w.provider_inquiry_id,
			w.status,
			w.created_at,
			w.updated_at
		FROM store_withdrawals w
		INNER JOIN store_bank_accounts a ON a.id = w.store_bank_account_id
	`+whereClause+`
		ORDER BY w.created_at DESC
		LIMIT $`+fmt.Sprint(limitPlaceholder)+` OFFSET $`+fmt.Sprint(offsetPlaceholder), queryArgs...)
	if err != nil {
		return StoreWithdrawalPage{}, fmt.Errorf("list store withdrawals: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		withdrawal, err := scanWithdrawal(rows)
		if err != nil {
			return StoreWithdrawalPage{}, err
		}

		page.Items = append(page.Items, withdrawal)
	}

	if err := rows.Err(); err != nil {
		return StoreWithdrawalPage{}, fmt.Errorf("iterate store withdrawals: %w", err)
	}

	return page, nil
}

func buildWithdrawalListWhere(filter ListWithdrawalsFilter) (string, []any) {
	clauses := []string{"w.store_id = $1"}
	args := []any{strings.TrimSpace(filter.StoreID)}

	if filter.Status != nil {
		clauses = append(clauses, fmt.Sprintf("w.status = $%d", len(args)+1))
		args = append(args, *filter.Status)
	}

	if query := strings.TrimSpace(filter.Query); query != "" {
		placeholder := fmt.Sprintf("$%d", len(args)+1)
		args = append(args, "%"+query+"%")
		clauses = append(clauses, "("+strings.Join([]string{
			"a.bank_code ILIKE " + placeholder,
			"a.bank_name ILIKE " + placeholder,
			"a.account_name ILIKE " + placeholder,
			"COALESCE(w.provider_partner_ref_no, '') ILIKE " + placeholder,
			"w.idempotency_key ILIKE " + placeholder,
		}, " OR ")+")")
	}

	if filter.CreatedFrom != nil {
		clauses = append(clauses, fmt.Sprintf("w.created_at >= $%d", len(args)+1))
		args = append(args, *filter.CreatedFrom)
	}

	if filter.CreatedTo != nil {
		clauses = append(clauses, fmt.Sprintf("w.created_at <= $%d", len(args)+1))
		args = append(args, *filter.CreatedTo)
	}

	return " WHERE " + strings.Join(clauses, " AND "), args
}

func (r *Repository) NextStatusCheckAttemptNo(ctx context.Context, withdrawalID string) (int, error) {
	var nextAttempt int
	err := r.pool.QueryRow(ctx, `
		SELECT COALESCE(MAX(attempt_no), 0) + 1
		FROM withdrawal_status_checks
		WHERE store_withdrawal_id = $1
	`, strings.TrimSpace(withdrawalID)).Scan(&nextAttempt)
	if err != nil {
		return 0, fmt.Errorf("next withdrawal status check attempt no: %w", err)
	}

	return nextAttempt, nil
}

func (r *Repository) ListDueStatusCheckWithdrawals(ctx context.Context, cutoff time.Time, limit int) ([]StatusCheckCandidate, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT
			w.id,
			w.store_id,
			w.store_bank_account_id,
			w.idempotency_key,
			a.bank_code,
			a.bank_name,
			a.account_name,
			a.account_number_masked,
			w.net_requested_amount::text,
			w.platform_fee_amount::text,
			w.external_fee_amount::text,
			w.total_store_debit::text,
			w.provider_partner_ref_no,
			w.provider_inquiry_id,
			w.status,
			w.created_at,
			w.updated_at,
			COALESCE(last_attempt.attempt_no, 0) AS attempt_no,
			last_attempt.created_at AS last_attempt_at
		FROM store_withdrawals w
		INNER JOIN stores s ON s.id = w.store_id
		INNER JOIN store_bank_accounts a ON a.id = w.store_bank_account_id
		LEFT JOIN LATERAL (
			SELECT attempt_no, created_at
			FROM withdrawal_status_checks
			WHERE store_withdrawal_id = w.id
			ORDER BY attempt_no DESC
			LIMIT 1
		) AS last_attempt ON TRUE
		WHERE w.status = 'pending'
			AND w.provider_partner_ref_no IS NOT NULL
			AND s.deleted_at IS NULL
			AND (last_attempt.created_at IS NULL OR last_attempt.created_at <= $1)
		ORDER BY COALESCE(last_attempt.created_at, w.updated_at) ASC, w.created_at ASC
		LIMIT $2
	`, cutoff.UTC(), limit)
	if err != nil {
		return nil, fmt.Errorf("list due withdrawal status checks: %w", err)
	}
	defer rows.Close()

	var candidates []StatusCheckCandidate
	for rows.Next() {
		var candidate StatusCheckCandidate
		if err := rows.Scan(
			&candidate.Withdrawal.ID,
			&candidate.Withdrawal.StoreID,
			&candidate.Withdrawal.StoreBankAccountID,
			&candidate.Withdrawal.IdempotencyKey,
			&candidate.Withdrawal.BankCode,
			&candidate.Withdrawal.BankName,
			&candidate.Withdrawal.AccountName,
			&candidate.Withdrawal.AccountNumberMasked,
			&candidate.Withdrawal.NetRequestedAmount,
			&candidate.Withdrawal.PlatformFeeAmount,
			&candidate.Withdrawal.ExternalFeeAmount,
			&candidate.Withdrawal.TotalStoreDebit,
			&candidate.Withdrawal.ProviderPartnerRefNo,
			&candidate.Withdrawal.ProviderInquiryID,
			&candidate.Withdrawal.Status,
			&candidate.Withdrawal.CreatedAt,
			&candidate.Withdrawal.UpdatedAt,
			&candidate.AttemptNo,
			&candidate.LastAttemptAt,
		); err != nil {
			return nil, fmt.Errorf("scan withdrawal status check candidate: %w", err)
		}

		candidates = append(candidates, candidate)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate withdrawal status checks: %w", err)
	}

	return candidates, nil
}

func (r *Repository) CreateStoreWithdrawal(ctx context.Context, params CreateStoreWithdrawalParams) (StoreWithdrawal, error) {
	var withdrawalID string
	err := r.pool.QueryRow(ctx, `
		INSERT INTO store_withdrawals (
			store_id,
			store_bank_account_id,
			idempotency_key,
			net_requested_amount,
			platform_fee_amount,
			external_fee_amount,
			total_store_debit,
			provider_partner_ref_no,
			provider_inquiry_id,
			status,
			request_payload_masked,
			provider_payload_masked,
			created_at,
			updated_at
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11::jsonb, $12::jsonb, $13, $13)
		RETURNING id
	`, params.StoreID, params.StoreBankAccountID, params.IdempotencyKey, params.NetRequestedAmount, params.PlatformFeeAmount, params.ExternalFeeAmount, params.TotalStoreDebit, params.ProviderPartnerRefNo, params.ProviderInquiryID, params.Status, toJSON(params.RequestPayload), toJSON(params.ProviderPayload), params.OccurredAt).Scan(&withdrawalID)
	if err != nil {
		return StoreWithdrawal{}, fmt.Errorf("create store withdrawal: %w", err)
	}

	return r.GetByID(ctx, withdrawalID)
}

func (r *Repository) UpdateStoreWithdrawal(ctx context.Context, params UpdateStoreWithdrawalParams) (StoreWithdrawal, error) {
	var providerPayload *string
	if params.ProviderPayload != nil {
		encoded := toJSON(params.ProviderPayload)
		providerPayload = &encoded
	}

	_, err := r.pool.Exec(ctx, `
		UPDATE store_withdrawals
		SET
			platform_fee_amount = COALESCE($2, platform_fee_amount),
			external_fee_amount = COALESCE($3, external_fee_amount),
			total_store_debit = COALESCE($4, total_store_debit),
			provider_partner_ref_no = COALESCE($5, provider_partner_ref_no),
			provider_inquiry_id = COALESCE($6, provider_inquiry_id),
			status = COALESCE($7, status),
			provider_payload_masked = COALESCE($8::jsonb, provider_payload_masked),
			updated_at = $9
		WHERE id = $1
	`, params.WithdrawalID, params.PlatformFeeAmount, params.ExternalFeeAmount, params.TotalStoreDebit, params.ProviderPartnerRefNo, params.ProviderInquiryID, params.Status, providerPayload, params.OccurredAt)
	if err != nil {
		return StoreWithdrawal{}, fmt.Errorf("update store withdrawal: %w", err)
	}

	return r.GetByID(ctx, params.WithdrawalID)
}

func (r *Repository) RecordStatusCheck(ctx context.Context, params RecordStatusCheckParams) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO withdrawal_status_checks (
			store_withdrawal_id,
			attempt_no,
			status,
			response_masked,
			created_at
		)
		VALUES ($1, $2, $3, $4::jsonb, $5)
	`, strings.TrimSpace(params.WithdrawalID), params.AttemptNo, strings.TrimSpace(params.Status), toJSON(params.ResponseMasked), params.OccurredAt)
	if err != nil {
		return fmt.Errorf("record withdrawal status check: %w", err)
	}

	return nil
}

func (r *Repository) InsertAuditLog(
	ctx context.Context,
	actorUserID *string,
	actorRole string,
	storeID *string,
	action string,
	targetType string,
	targetID *string,
	payload map[string]any,
	ipAddress string,
	userAgent string,
	occurredAt time.Time,
) error {
	encoded, err := json.Marshal(payload)
	if err != nil {
		encoded = []byte("{}")
	}

	_, err = r.pool.Exec(ctx, `
		INSERT INTO audit_logs (
			actor_user_id,
			actor_role,
			store_id,
			action,
			target_type,
			target_id,
			payload_masked,
			ip_address,
			user_agent,
			created_at
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7::jsonb, $8, $9, $10)
	`, actorUserID, actorRole, storeID, action, targetType, targetID, string(encoded), nullableString(ipAddress), nullableString(userAgent), occurredAt)
	if err != nil {
		return fmt.Errorf("insert withdrawal audit log: %w", err)
	}

	return nil
}

func (r *Repository) getWithdrawal(ctx context.Context, query string, args ...any) (StoreWithdrawal, error) {
	row := r.pool.QueryRow(ctx, query, args...)
	withdrawal, err := scanWithdrawal(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return StoreWithdrawal{}, ErrNotFound
		}

		return StoreWithdrawal{}, err
	}

	return withdrawal, nil
}

type scanner interface {
	Scan(dest ...any) error
}

func scanWithdrawal(row scanner) (StoreWithdrawal, error) {
	var withdrawal StoreWithdrawal
	err := row.Scan(
		&withdrawal.ID,
		&withdrawal.StoreID,
		&withdrawal.StoreBankAccountID,
		&withdrawal.IdempotencyKey,
		&withdrawal.BankCode,
		&withdrawal.BankName,
		&withdrawal.AccountName,
		&withdrawal.AccountNumberMasked,
		&withdrawal.NetRequestedAmount,
		&withdrawal.PlatformFeeAmount,
		&withdrawal.ExternalFeeAmount,
		&withdrawal.TotalStoreDebit,
		&withdrawal.ProviderPartnerRefNo,
		&withdrawal.ProviderInquiryID,
		&withdrawal.Status,
		&withdrawal.CreatedAt,
		&withdrawal.UpdatedAt,
	)
	if err != nil {
		return StoreWithdrawal{}, err
	}

	return withdrawal, nil
}

type repositoryProcessingLock struct {
	conn         *pgxpool.Conn
	withdrawalID string
}

func (l *repositoryProcessingLock) Unlock(ctx context.Context) error {
	if l == nil || l.conn == nil {
		return nil
	}

	var unlocked bool
	err := l.conn.QueryRow(ctx, `
		SELECT pg_advisory_unlock($1, hashtext($2))
	`, withdrawalProcessingLockNamespace, l.withdrawalID).Scan(&unlocked)
	l.conn.Release()
	l.conn = nil
	if err != nil {
		return fmt.Errorf("unlock withdrawal advisory lock: %w", err)
	}
	if !unlocked {
		return fmt.Errorf("unlock withdrawal advisory lock: lock not held")
	}

	return nil
}
