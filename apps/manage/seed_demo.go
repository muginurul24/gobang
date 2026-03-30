package main

import (
	"context"
	"fmt"

	"github.com/mugiew/onixggr/internal/platform/crypto"
	"github.com/mugiew/onixggr/internal/platform/db"
)

const (
	demoStoreID                 = "cccccccc-cccc-cccc-cccc-cccccccccccc"
	demoStoreBankAccountID      = "abababab-abab-abab-abab-abababababab"
	demoOpeningBalanceReference = "12121212-1212-1212-1212-121212121212"
	demoBankCode                = "542"
	demoBankName                = "PT. BANK ARTOS INDONESIA (Bank Jago)"
	demoBankAccountNumber       = "000000009749"
	demoBankAccountNumberMasked = "********9749"
	demoBankAccountName         = "DEMO OWNER"
	demoOpeningBalanceAmount    = "2500000.00"
)

func applyDemoSeed(ctx context.Context, pool *db.Pool, authEncryptionKey string) (int, error) {
	applied, err := db.ApplySQLDir(ctx, pool, "seeds/demo")
	if err != nil {
		return 0, fmt.Errorf("apply demo sql seeds: %w", err)
	}

	if err := upsertDemoBankAccount(ctx, pool, authEncryptionKey); err != nil {
		return 0, fmt.Errorf("upsert demo bank account: %w", err)
	}

	if err := ensureDemoOpeningBalance(ctx, pool); err != nil {
		return 0, fmt.Errorf("ensure demo opening balance: %w", err)
	}

	return applied, nil
}

func upsertDemoBankAccount(ctx context.Context, pool *db.Pool, authEncryptionKey string) error {
	sealedAccountNumber, err := crypto.NewSealer(authEncryptionKey).Seal(demoBankAccountNumber)
	if err != nil {
		return fmt.Errorf("seal demo bank account number: %w", err)
	}

	_, err = pool.Exec(ctx, `
		INSERT INTO store_bank_accounts (
			id,
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
		) VALUES (
			$1,
			$2,
			$3,
			$4,
			$5,
			$6,
			$7,
			now(),
			true,
			now(),
			now()
		)
		ON CONFLICT (id) DO UPDATE
		SET
			store_id = EXCLUDED.store_id,
			bank_code = EXCLUDED.bank_code,
			bank_name = EXCLUDED.bank_name,
			account_number_encrypted = EXCLUDED.account_number_encrypted,
			account_number_masked = EXCLUDED.account_number_masked,
			account_name = EXCLUDED.account_name,
			verified_at = EXCLUDED.verified_at,
			is_active = EXCLUDED.is_active,
			updated_at = now()
	`,
		demoStoreBankAccountID,
		demoStoreID,
		demoBankCode,
		demoBankName,
		sealedAccountNumber,
		demoBankAccountNumberMasked,
		demoBankAccountName,
	)
	if err != nil {
		return fmt.Errorf("insert demo bank account: %w", err)
	}

	return nil
}

func ensureDemoOpeningBalance(ctx context.Context, pool *db.Pool) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	var ledgerAccountID string
	if err := tx.QueryRow(ctx, `
		SELECT id
		FROM ledger_accounts
		WHERE store_id = $1
	`, demoStoreID).Scan(&ledgerAccountID); err != nil {
		return fmt.Errorf("load demo ledger account: %w", err)
	}

	tag, err := tx.Exec(ctx, `
		INSERT INTO ledger_entries (
			id,
			store_id,
			ledger_account_id,
			direction,
			entry_type,
			amount,
			balance_after,
			reference_type,
			reference_id,
			metadata_json,
			created_at
		) VALUES (
			$1,
			$2,
			$3,
			'credit',
			'store_topup',
			$4,
			$4,
			'seed_demo',
			$1,
			'{"note":"demo opening balance"}'::jsonb,
			now()
		)
		ON CONFLICT (id) DO NOTHING
	`,
		demoOpeningBalanceReference,
		demoStoreID,
		ledgerAccountID,
		demoOpeningBalanceAmount,
	)
	if err != nil {
		return fmt.Errorf("insert demo opening balance entry: %w", err)
	}

	if tag.RowsAffected() > 0 {
		if _, err := tx.Exec(ctx, `
			UPDATE stores
			SET current_balance = $2, updated_at = now()
			WHERE id = $1
		`, demoStoreID, demoOpeningBalanceAmount); err != nil {
			return fmt.Errorf("update demo store current balance: %w", err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit demo opening balance: %w", err)
	}

	return nil
}
