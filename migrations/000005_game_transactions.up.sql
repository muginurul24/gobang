CREATE TABLE game_transactions (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  store_id uuid NOT NULL REFERENCES stores(id) ON DELETE CASCADE,
  store_member_id uuid NOT NULL REFERENCES store_members(id) ON DELETE RESTRICT,
  action text NOT NULL CHECK (action IN ('deposit', 'withdraw')),
  trx_id text NOT NULL,
  upstream_user_code text NOT NULL,
  amount numeric(20, 2) NOT NULL CHECK (amount > 0),
  agent_sign text NOT NULL,
  status text NOT NULL CHECK (status IN ('pending', 'success', 'failed')),
  reconcile_status text NULL CHECK (reconcile_status IN ('pending_reconcile', 'resolved')),
  upstream_error_code text NULL,
  upstream_response_masked jsonb NOT NULL DEFAULT '{}'::jsonb,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  CONSTRAINT game_transactions_store_id_trx_id_unique UNIQUE (store_id, trx_id),
  CONSTRAINT game_transactions_agent_sign_unique UNIQUE (agent_sign)
);

CREATE INDEX game_transactions_store_id_created_at_idx ON game_transactions (store_id, created_at DESC);
CREATE INDEX game_transactions_store_member_id_created_at_idx ON game_transactions (store_member_id, created_at DESC);
CREATE INDEX game_transactions_status_idx ON game_transactions (status);
CREATE INDEX game_transactions_reconcile_status_idx ON game_transactions (reconcile_status);
