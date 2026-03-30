package game

import (
	"context"
	"errors"
	"fmt"
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
			id,
			owner_user_id,
			name,
			slug,
			status,
			deleted_at
		FROM stores
		WHERE api_token_hash = $1
		LIMIT 1
	`, tokenHash).Scan(
		&store.ID,
		&store.OwnerUserID,
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

func (r *Repository) FindStoreMemberByUsername(ctx context.Context, storeID string, username string) (StoreMember, error) {
	var member StoreMember
	err := r.pool.QueryRow(ctx, `
		SELECT
			id,
			store_id,
			real_username,
			upstream_user_code,
			status,
			created_at,
			updated_at
		FROM store_members
		WHERE store_id = $1 AND real_username = $2
		LIMIT 1
	`, storeID, username).Scan(
		&member.ID,
		&member.StoreID,
		&member.RealUsername,
		&member.UpstreamUserCode,
		&member.Status,
		&member.CreatedAt,
		&member.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return StoreMember{}, ErrNotFound
		}

		return StoreMember{}, fmt.Errorf("find store member by username: %w", err)
	}

	return member, nil
}

func (r *Repository) HasUpstreamUserCode(ctx context.Context, upstreamUserCode string) (bool, error) {
	var exists bool
	err := r.pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1
			FROM store_members
			WHERE upstream_user_code = $1
		)
	`, upstreamUserCode).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("check upstream user code: %w", err)
	}

	return exists, nil
}

func (r *Repository) CreateStoreMember(ctx context.Context, params CreateStoreMemberParams) (StoreMember, error) {
	var member StoreMember
	err := r.pool.QueryRow(ctx, `
		INSERT INTO store_members (
			store_id,
			real_username,
			upstream_user_code,
			status,
			created_at,
			updated_at
		)
		VALUES ($1, $2, $3, $4, $5, $5)
		RETURNING
			id,
			store_id,
			real_username,
			upstream_user_code,
			status,
			created_at,
			updated_at
	`, params.StoreID, params.RealUsername, params.UpstreamUserCode, params.Status, params.OccurredAt).Scan(
		&member.ID,
		&member.StoreID,
		&member.RealUsername,
		&member.UpstreamUserCode,
		&member.Status,
		&member.CreatedAt,
		&member.UpdatedAt,
	)
	if err != nil {
		switch {
		case duplicateConstraint(err, "store_members_store_id_real_username_unique"):
			return StoreMember{}, ErrDuplicateUsername
		case duplicateConstraint(err, "store_members_upstream_user_code_unique"):
			return StoreMember{}, ErrDuplicateUpstreamUserCode
		}

		return StoreMember{}, fmt.Errorf("create game user mapping: %w", err)
	}

	return member, nil
}

func (r *Repository) FindGameTransactionByTrxID(ctx context.Context, storeID string, trxID string) (GameTransaction, error) {
	var transaction GameTransaction
	var reconcileStatus *string
	var upstreamErrorCode *string
	err := r.pool.QueryRow(ctx, `
		SELECT
			id,
			store_id,
			store_member_id,
			action,
			trx_id,
			upstream_user_code,
			amount::text,
			agent_sign,
			status,
			reconcile_status,
			upstream_error_code,
			upstream_response_masked,
			created_at,
			updated_at
		FROM game_transactions
		WHERE store_id = $1 AND trx_id = $2
		LIMIT 1
	`, storeID, trxID).Scan(
		&transaction.ID,
		&transaction.StoreID,
		&transaction.StoreMemberID,
		&transaction.Action,
		&transaction.TrxID,
		&transaction.UpstreamUserCode,
		&transaction.Amount,
		&transaction.AgentSign,
		&transaction.Status,
		&reconcileStatus,
		&upstreamErrorCode,
		&transaction.UpstreamResponse,
		&transaction.CreatedAt,
		&transaction.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return GameTransaction{}, ErrNotFound
		}

		return GameTransaction{}, fmt.Errorf("find game transaction by trx id: %w", err)
	}

	transaction.ReconcileStatus = reconcileStatusPtr(reconcileStatus)
	transaction.UpstreamErrorCode = upstreamErrorCode

	return transaction, nil
}

func (r *Repository) CreateGameTransaction(ctx context.Context, params CreateGameTransactionParams) (GameTransaction, error) {
	var transaction GameTransaction
	var reconcileStatus *string
	var upstreamErrorCode *string
	err := r.pool.QueryRow(ctx, `
		INSERT INTO game_transactions (
			store_id,
			store_member_id,
			action,
			trx_id,
			upstream_user_code,
			amount,
			agent_sign,
			status,
			created_at,
			updated_at
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $9)
		RETURNING
			id,
			store_id,
			store_member_id,
			action,
			trx_id,
			upstream_user_code,
			amount::text,
			agent_sign,
			status,
			reconcile_status,
			upstream_error_code,
			upstream_response_masked,
			created_at,
			updated_at
	`, params.StoreID, params.StoreMemberID, params.Action, params.TrxID, params.UpstreamUserCode, params.Amount, params.AgentSign, params.Status, params.OccurredAt).Scan(
		&transaction.ID,
		&transaction.StoreID,
		&transaction.StoreMemberID,
		&transaction.Action,
		&transaction.TrxID,
		&transaction.UpstreamUserCode,
		&transaction.Amount,
		&transaction.AgentSign,
		&transaction.Status,
		&reconcileStatus,
		&upstreamErrorCode,
		&transaction.UpstreamResponse,
		&transaction.CreatedAt,
		&transaction.UpdatedAt,
	)
	if err != nil {
		switch {
		case duplicateConstraint(err, "game_transactions_store_id_trx_id_unique"):
			return GameTransaction{}, ErrDuplicateTransactionID
		case duplicateConstraint(err, "game_transactions_agent_sign_unique"):
			return GameTransaction{}, ErrAgentSignExhausted
		}

		return GameTransaction{}, fmt.Errorf("create game transaction: %w", err)
	}

	transaction.ReconcileStatus = reconcileStatusPtr(reconcileStatus)
	transaction.UpstreamErrorCode = upstreamErrorCode

	return transaction, nil
}

func (r *Repository) UpdateGameTransaction(ctx context.Context, params UpdateGameTransactionParams) (GameTransaction, error) {
	var transaction GameTransaction
	var reconcileStatus *string
	var upstreamErrorCode *string
	err := r.pool.QueryRow(ctx, `
		UPDATE game_transactions
		SET
			status = $2,
			reconcile_status = $3,
			upstream_error_code = $4,
			upstream_response_masked = $5::jsonb,
			updated_at = $6
		WHERE id = $1
		RETURNING
			id,
			store_id,
			store_member_id,
			action,
			trx_id,
			upstream_user_code,
			amount::text,
			agent_sign,
			status,
			reconcile_status,
			upstream_error_code,
			upstream_response_masked,
			created_at,
			updated_at
	`, params.GameTransactionID, params.Status, nullableReconcileStatus(params.ReconcileStatus), params.UpstreamErrorCode, toJSON(params.UpstreamResponseMasked), params.OccurredAt).Scan(
		&transaction.ID,
		&transaction.StoreID,
		&transaction.StoreMemberID,
		&transaction.Action,
		&transaction.TrxID,
		&transaction.UpstreamUserCode,
		&transaction.Amount,
		&transaction.AgentSign,
		&transaction.Status,
		&reconcileStatus,
		&upstreamErrorCode,
		&transaction.UpstreamResponse,
		&transaction.CreatedAt,
		&transaction.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return GameTransaction{}, ErrNotFound
		}

		return GameTransaction{}, fmt.Errorf("update game transaction: %w", err)
	}

	transaction.ReconcileStatus = reconcileStatusPtr(reconcileStatus)
	transaction.UpstreamErrorCode = upstreamErrorCode

	return transaction, nil
}

func (r *Repository) InsertAuditLog(
	ctx context.Context,
	storeID string,
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
		VALUES ('store_api', $1, $2, $3, $4, $5::jsonb, $6, $7, $8)
	`, storeID, action, targetType, targetID, toJSON(payload), nullableString(ipAddress), nullableString(userAgent), occurredAt)
	if err != nil {
		return fmt.Errorf("insert game audit log: %w", err)
	}

	return nil
}

func duplicateConstraint(err error, constraint string) bool {
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		return false
	}

	return pgErr.Code == "23505" && pgErr.ConstraintName == constraint
}

func nullableString(value string) *string {
	if value == "" {
		return nil
	}

	return &value
}

func nullableReconcileStatus(status *ReconcileStatus) *string {
	if status == nil {
		return nil
	}

	value := string(*status)
	return &value
}

func reconcileStatusPtr(value *string) *ReconcileStatus {
	if value == nil || *value == "" {
		return nil
	}

	status := ReconcileStatus(*value)
	return &status
}
