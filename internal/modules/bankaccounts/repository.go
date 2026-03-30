package bankaccounts

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

func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

func (r *Repository) GetStoreScope(ctx context.Context, storeID string) (StoreScope, error) {
	var store StoreScope
	err := r.pool.QueryRow(ctx, `
		SELECT id, owner_user_id, name, slug, deleted_at
		FROM stores
		WHERE id = $1
		LIMIT 1
	`, storeID).Scan(
		&store.ID,
		&store.OwnerUserID,
		&store.Name,
		&store.Slug,
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

func (r *Repository) ListBankAccounts(ctx context.Context, storeID string) ([]BankAccount, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT
			id,
			store_id,
			bank_code,
			bank_name,
			account_number_masked,
			account_name,
			verified_at,
			is_active,
			created_at,
			updated_at
		FROM store_bank_accounts
		WHERE store_id = $1
		ORDER BY created_at DESC
	`, storeID)
	if err != nil {
		return nil, fmt.Errorf("list bank accounts: %w", err)
	}
	defer rows.Close()

	return collectBankAccounts(rows)
}

func (r *Repository) CreateBankAccount(ctx context.Context, params CreateBankAccountParams) (BankAccount, error) {
	var bankAccount BankAccount
	err := r.pool.QueryRow(ctx, `
		INSERT INTO store_bank_accounts (
			store_id,
			bank_code,
			bank_name,
			account_number_encrypted,
			account_number_masked,
			account_name,
			verified_at,
			is_active,
			created_at,
			updated_at
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $9)
		RETURNING
			id,
			store_id,
			bank_code,
			bank_name,
			account_number_masked,
			account_name,
			verified_at,
			is_active,
			created_at,
			updated_at
	`, params.StoreID, params.BankCode, params.BankName, params.AccountNumberEncrypted, params.AccountNumberMasked, params.AccountName, params.VerifiedAt, params.IsActive, params.OccurredAt).Scan(
		&bankAccount.ID,
		&bankAccount.StoreID,
		&bankAccount.BankCode,
		&bankAccount.BankName,
		&bankAccount.AccountNumberMasked,
		&bankAccount.AccountName,
		&bankAccount.VerifiedAt,
		&bankAccount.IsActive,
		&bankAccount.CreatedAt,
		&bankAccount.UpdatedAt,
	)
	if err != nil {
		return BankAccount{}, fmt.Errorf("create bank account: %w", err)
	}

	return bankAccount, nil
}

func (r *Repository) GetBankAccountByID(ctx context.Context, bankAccountID string) (BankAccount, error) {
	var bankAccount BankAccount
	err := r.pool.QueryRow(ctx, `
		SELECT
			id,
			store_id,
			bank_code,
			bank_name,
			account_number_masked,
			account_name,
			verified_at,
			is_active,
			created_at,
			updated_at
		FROM store_bank_accounts
		WHERE id = $1
		LIMIT 1
	`, bankAccountID).Scan(
		&bankAccount.ID,
		&bankAccount.StoreID,
		&bankAccount.BankCode,
		&bankAccount.BankName,
		&bankAccount.AccountNumberMasked,
		&bankAccount.AccountName,
		&bankAccount.VerifiedAt,
		&bankAccount.IsActive,
		&bankAccount.CreatedAt,
		&bankAccount.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return BankAccount{}, ErrNotFound
		}

		return BankAccount{}, fmt.Errorf("get bank account by id: %w", err)
	}

	return bankAccount, nil
}

func (r *Repository) UpdateBankAccountStatus(ctx context.Context, params UpdateBankAccountStatusParams) (BankAccount, error) {
	var bankAccount BankAccount
	err := r.pool.QueryRow(ctx, `
		UPDATE store_bank_accounts
		SET is_active = $2, updated_at = $3
		WHERE id = $1
		RETURNING
			id,
			store_id,
			bank_code,
			bank_name,
			account_number_masked,
			account_name,
			verified_at,
			is_active,
			created_at,
			updated_at
	`, params.BankAccountID, params.IsActive, params.OccurredAt).Scan(
		&bankAccount.ID,
		&bankAccount.StoreID,
		&bankAccount.BankCode,
		&bankAccount.BankName,
		&bankAccount.AccountNumberMasked,
		&bankAccount.AccountName,
		&bankAccount.VerifiedAt,
		&bankAccount.IsActive,
		&bankAccount.CreatedAt,
		&bankAccount.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return BankAccount{}, ErrNotFound
		}

		return BankAccount{}, fmt.Errorf("update bank account status: %w", err)
	}

	return bankAccount, nil
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
		return fmt.Errorf("insert bank account audit log: %w", err)
	}

	return nil
}

func collectBankAccounts(rows pgx.Rows) ([]BankAccount, error) {
	var bankAccounts []BankAccount
	for rows.Next() {
		var bankAccount BankAccount
		if err := rows.Scan(
			&bankAccount.ID,
			&bankAccount.StoreID,
			&bankAccount.BankCode,
			&bankAccount.BankName,
			&bankAccount.AccountNumberMasked,
			&bankAccount.AccountName,
			&bankAccount.VerifiedAt,
			&bankAccount.IsActive,
			&bankAccount.CreatedAt,
			&bankAccount.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan bank account: %w", err)
		}

		bankAccounts = append(bankAccounts, bankAccount)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate bank accounts: %w", err)
	}

	return bankAccounts, nil
}

func nullableString(value string) any {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil
	}

	return trimmed
}
