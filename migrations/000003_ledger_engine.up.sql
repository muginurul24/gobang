CREATE TABLE ledger_accounts (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  store_id uuid NOT NULL REFERENCES stores(id) ON DELETE CASCADE,
  currency text NOT NULL DEFAULT 'IDR',
  created_at timestamptz NOT NULL DEFAULT now(),
  CONSTRAINT ledger_accounts_store_id_unique UNIQUE (store_id)
);

CREATE TABLE ledger_entries (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  store_id uuid NOT NULL REFERENCES stores(id) ON DELETE CASCADE,
  ledger_account_id uuid NOT NULL REFERENCES ledger_accounts(id) ON DELETE CASCADE,
  direction text NOT NULL CHECK (direction IN ('debit', 'credit')),
  entry_type text NOT NULL CHECK (
    entry_type IN (
      'game_deposit',
      'game_withdraw',
      'store_topup',
      'member_payment_credit',
      'member_payment_fee',
      'withdraw_reserve',
      'withdraw_commit',
      'withdraw_release',
      'withdraw_platform_fee',
      'withdraw_external_fee'
    )
  ),
  amount numeric(20, 2) NOT NULL CHECK (amount > 0),
  balance_after numeric(20, 2) NOT NULL CHECK (balance_after >= 0),
  reference_type text NOT NULL,
  reference_id uuid NOT NULL,
  metadata_json jsonb NOT NULL DEFAULT '{}'::jsonb,
  created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX ledger_entries_store_id_created_at_idx ON ledger_entries (store_id, created_at DESC);
CREATE INDEX ledger_entries_reference_type_reference_id_idx ON ledger_entries (reference_type, reference_id);
CREATE INDEX ledger_entries_entry_type_idx ON ledger_entries (entry_type);

CREATE TABLE ledger_reservations (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  store_id uuid NOT NULL REFERENCES stores(id) ON DELETE CASCADE,
  reference_type text NOT NULL,
  reference_id uuid NOT NULL,
  amount numeric(20, 2) NOT NULL CHECK (amount > 0),
  status text NOT NULL CHECK (status IN ('pending', 'committed', 'released')),
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  CONSTRAINT ledger_reservations_reference_type_reference_id_unique UNIQUE (reference_type, reference_id)
);

CREATE INDEX ledger_reservations_store_id_idx ON ledger_reservations (store_id);
CREATE INDEX ledger_reservations_status_idx ON ledger_reservations (status);

INSERT INTO ledger_accounts (store_id, currency, created_at)
SELECT id, 'IDR', created_at
FROM stores
ON CONFLICT (store_id) DO NOTHING;
