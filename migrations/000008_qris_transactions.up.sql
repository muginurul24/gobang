CREATE TABLE qris_transactions (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  store_id uuid NOT NULL REFERENCES stores(id) ON DELETE CASCADE,
  store_member_id uuid NULL REFERENCES store_members(id) ON DELETE RESTRICT,
  type text NOT NULL CHECK (type IN ('store_topup', 'member_payment')),
  provider_trx_id text NULL,
  custom_ref text NOT NULL,
  external_username text NOT NULL,
  amount_gross numeric(20, 2) NOT NULL CHECK (amount_gross > 0),
  platform_fee_amount numeric(20, 2) NOT NULL DEFAULT 0 CHECK (platform_fee_amount >= 0),
  store_credit_amount numeric(20, 2) NOT NULL CHECK (store_credit_amount >= 0),
  status text NOT NULL CHECK (status IN ('pending', 'success', 'expired', 'failed')),
  expires_at timestamptz NULL,
  provider_payload_masked jsonb NOT NULL DEFAULT '{}'::jsonb,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  CONSTRAINT qris_transactions_provider_trx_id_unique UNIQUE (provider_trx_id),
  CONSTRAINT qris_transactions_type_custom_ref_unique UNIQUE (type, custom_ref)
);

CREATE INDEX qris_transactions_store_id_created_at_idx ON qris_transactions (store_id, created_at DESC);
CREATE INDEX qris_transactions_store_member_id_created_at_idx ON qris_transactions (store_member_id, created_at DESC);
CREATE INDEX qris_transactions_status_idx ON qris_transactions (status);
CREATE INDEX qris_transactions_type_idx ON qris_transactions (type);
