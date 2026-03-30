package paymentsqris

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

func (r *Repository) AuthenticateStore(ctx context.Context, tokenHash string) (StoreScope, error) {
	var store StoreScope
	err := r.pool.QueryRow(ctx, `
		SELECT
			s.id,
			s.owner_user_id,
			u.username,
			s.name,
			s.slug,
			s.status,
			s.deleted_at
		FROM stores s
		INNER JOIN users u ON u.id = s.owner_user_id
		WHERE s.api_token_hash = $1
		LIMIT 1
	`, strings.TrimSpace(tokenHash)).Scan(
		&store.ID,
		&store.OwnerUserID,
		&store.OwnerUsername,
		&store.Name,
		&store.Slug,
		&store.Status,
		&store.DeletedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return StoreScope{}, ErrUnauthorized
		}

		return StoreScope{}, fmt.Errorf("authenticate store token: %w", err)
	}

	return store, nil
}

func (r *Repository) GetStoreScope(ctx context.Context, storeID string) (StoreScope, error) {
	var store StoreScope
	err := r.pool.QueryRow(ctx, `
		SELECT
			s.id,
			s.owner_user_id,
			u.username,
			s.name,
			s.slug,
			s.status,
			s.deleted_at
		FROM stores s
		INNER JOIN users u ON u.id = s.owner_user_id
		WHERE s.id = $1
		LIMIT 1
	`, strings.TrimSpace(storeID)).Scan(
		&store.ID,
		&store.OwnerUserID,
		&store.OwnerUsername,
		&store.Name,
		&store.Slug,
		&store.Status,
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

func (r *Repository) FindStoreMemberByUsername(ctx context.Context, storeID string, username string) (StoreMember, error) {
	var member StoreMember
	err := r.pool.QueryRow(ctx, `
		SELECT
			id,
			store_id,
			real_username,
			upstream_user_code,
			status
		FROM store_members
		WHERE store_id = $1 AND real_username = $2
		LIMIT 1
	`, storeID, strings.TrimSpace(username)).Scan(
		&member.ID,
		&member.StoreID,
		&member.RealUsername,
		&member.UpstreamUserCode,
		&member.Status,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return StoreMember{}, ErrNotFound
		}

		return StoreMember{}, fmt.Errorf("find store member by username: %w", err)
	}

	return member, nil
}

func (r *Repository) CreateQRISTransaction(ctx context.Context, params CreateQRISTransactionParams) (QRISTransaction, error) {
	var transaction QRISTransaction
	var storeMemberID *string
	var providerTrxID *string
	var payloadRaw []byte
	err := r.pool.QueryRow(ctx, `
		INSERT INTO qris_transactions (
			store_id,
			store_member_id,
			type,
			provider_trx_id,
			custom_ref,
			external_username,
			amount_gross,
			platform_fee_amount,
			store_credit_amount,
			status,
			expires_at,
			provider_payload_masked,
			created_at,
			updated_at
		)
		VALUES ($1, $2, $3, NULL, $4, $5, $6, $7, $8, $9, $10, $11::jsonb, $12, $12)
		RETURNING
			id,
			store_id,
			store_member_id,
			type,
			provider_trx_id,
			custom_ref,
			external_username,
			amount_gross::text,
			platform_fee_amount::text,
			store_credit_amount::text,
			status,
			expires_at,
			provider_payload_masked,
			created_at,
			updated_at
	`, params.StoreID, params.StoreMemberID, params.Type, params.CustomRef, params.ExternalUsername, params.AmountGross, params.PlatformFeeAmount, params.StoreCreditAmount, params.Status, params.ExpiresAt, toJSON(params.ProviderPayload), params.OccurredAt).Scan(
		&transaction.ID,
		&transaction.StoreID,
		&storeMemberID,
		&transaction.Type,
		&providerTrxID,
		&transaction.CustomRef,
		&transaction.ExternalUsername,
		&transaction.AmountGross,
		&transaction.PlatformFeeAmount,
		&transaction.StoreCreditAmount,
		&transaction.Status,
		&transaction.ExpiresAt,
		&payloadRaw,
		&transaction.CreatedAt,
		&transaction.UpdatedAt,
	)
	if err != nil {
		if duplicateConstraint(err, "qris_transactions_type_custom_ref_unique") {
			return QRISTransaction{}, ErrDuplicateCustomRef
		}

		return QRISTransaction{}, fmt.Errorf("create qris transaction: %w", err)
	}

	transaction.StoreMemberID = storeMemberID
	transaction.ProviderTrxID = providerTrxID
	applyPayloadFields(&transaction, payloadRaw)

	return transaction, nil
}

func (r *Repository) UpdateGeneratedTransaction(ctx context.Context, params UpdateGeneratedTransactionParams) (QRISTransaction, error) {
	var transaction QRISTransaction
	var storeMemberID *string
	var providerTrxID *string
	var payloadRaw []byte
	err := r.pool.QueryRow(ctx, `
		UPDATE qris_transactions
		SET
			provider_trx_id = $2,
			expires_at = $3,
			provider_payload_masked = $4::jsonb,
			updated_at = $5
		WHERE id = $1
		RETURNING
			id,
			store_id,
			store_member_id,
			type,
			provider_trx_id,
			custom_ref,
			external_username,
			amount_gross::text,
			platform_fee_amount::text,
			store_credit_amount::text,
			status,
			expires_at,
			provider_payload_masked,
			created_at,
			updated_at
	`, params.TransactionID, params.ProviderTrxID, params.ExpiresAt, toJSON(params.ProviderPayload), params.OccurredAt).Scan(
		&transaction.ID,
		&transaction.StoreID,
		&storeMemberID,
		&transaction.Type,
		&providerTrxID,
		&transaction.CustomRef,
		&transaction.ExternalUsername,
		&transaction.AmountGross,
		&transaction.PlatformFeeAmount,
		&transaction.StoreCreditAmount,
		&transaction.Status,
		&transaction.ExpiresAt,
		&payloadRaw,
		&transaction.CreatedAt,
		&transaction.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return QRISTransaction{}, ErrNotFound
		}

		return QRISTransaction{}, fmt.Errorf("update generated qris transaction: %w", err)
	}

	transaction.StoreMemberID = storeMemberID
	transaction.ProviderTrxID = providerTrxID
	applyPayloadFields(&transaction, payloadRaw)

	return transaction, nil
}

func (r *Repository) UpdateTransactionStatus(ctx context.Context, params UpdateTransactionStatusParams) (QRISTransaction, error) {
	var transaction QRISTransaction
	var storeMemberID *string
	var providerTrxID *string
	var payloadRaw []byte
	err := r.pool.QueryRow(ctx, `
		UPDATE qris_transactions
		SET
			status = $2,
			expires_at = $3,
			provider_payload_masked = $4::jsonb,
			updated_at = $5
		WHERE id = $1
		RETURNING
			id,
			store_id,
			store_member_id,
			type,
			provider_trx_id,
			custom_ref,
			external_username,
			amount_gross::text,
			platform_fee_amount::text,
			store_credit_amount::text,
			status,
			expires_at,
			provider_payload_masked,
			created_at,
			updated_at
	`, params.TransactionID, params.Status, params.ExpiresAt, toJSON(params.ProviderPayload), params.OccurredAt).Scan(
		&transaction.ID,
		&transaction.StoreID,
		&storeMemberID,
		&transaction.Type,
		&providerTrxID,
		&transaction.CustomRef,
		&transaction.ExternalUsername,
		&transaction.AmountGross,
		&transaction.PlatformFeeAmount,
		&transaction.StoreCreditAmount,
		&transaction.Status,
		&transaction.ExpiresAt,
		&payloadRaw,
		&transaction.CreatedAt,
		&transaction.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return QRISTransaction{}, ErrNotFound
		}

		return QRISTransaction{}, fmt.Errorf("update qris transaction status: %w", err)
	}

	transaction.StoreMemberID = storeMemberID
	transaction.ProviderTrxID = providerTrxID
	applyPayloadFields(&transaction, payloadRaw)

	return transaction, nil
}

func (r *Repository) ListQRISTransactions(ctx context.Context, storeID string, transactionType TransactionType) ([]QRISTransaction, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT
			id,
			store_id,
			store_member_id,
			type,
			provider_trx_id,
			custom_ref,
			external_username,
			amount_gross::text,
			platform_fee_amount::text,
			store_credit_amount::text,
			status,
			expires_at,
			provider_payload_masked,
			created_at,
			updated_at
		FROM qris_transactions
		WHERE store_id = $1 AND type = $2
		ORDER BY created_at DESC
	`, storeID, transactionType)
	if err != nil {
		return nil, fmt.Errorf("list qris transactions: %w", err)
	}
	defer rows.Close()

	var transactions []QRISTransaction
	for rows.Next() {
		var transaction QRISTransaction
		var storeMemberID *string
		var providerTrxID *string
		var payloadRaw []byte

		if err := rows.Scan(
			&transaction.ID,
			&transaction.StoreID,
			&storeMemberID,
			&transaction.Type,
			&providerTrxID,
			&transaction.CustomRef,
			&transaction.ExternalUsername,
			&transaction.AmountGross,
			&transaction.PlatformFeeAmount,
			&transaction.StoreCreditAmount,
			&transaction.Status,
			&transaction.ExpiresAt,
			&payloadRaw,
			&transaction.CreatedAt,
			&transaction.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan qris transaction: %w", err)
		}

		transaction.StoreMemberID = storeMemberID
		transaction.ProviderTrxID = providerTrxID
		applyPayloadFields(&transaction, payloadRaw)
		transactions = append(transactions, transaction)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate qris transactions: %w", err)
	}

	return transactions, nil
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
	_, err := r.pool.Exec(ctx, `
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
	`, actorUserID, actorRole, storeID, action, targetType, targetID, toJSON(payload), nullableString(ipAddress), nullableString(userAgent), occurredAt)
	if err != nil {
		return fmt.Errorf("insert audit log: %w", err)
	}

	return nil
}

func applyPayloadFields(transaction *QRISTransaction, raw []byte) {
	if transaction == nil || len(raw) == 0 {
		return
	}

	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		return
	}

	transaction.ProviderState = payloadFieldProviderState(payload)
	if transaction.Status == TransactionStatusPending {
		transaction.QRCodeValue = payloadFieldString(payload, "qr_code_value")
	}
}

func duplicateConstraint(err error, name string) bool {
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		return false
	}

	return pgErr.Code == "23505" && pgErr.ConstraintName == name
}

func nullableString(value string) *string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil
	}

	return &trimmed
}
