CREATE TABLE store_withdrawals (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  store_id uuid NOT NULL REFERENCES stores(id) ON DELETE CASCADE,
  store_bank_account_id uuid NOT NULL REFERENCES store_bank_accounts(id),
  idempotency_key text NOT NULL,
  net_requested_amount numeric(20,2) NOT NULL,
  platform_fee_amount numeric(20,2) NOT NULL DEFAULT 0,
  external_fee_amount numeric(20,2) NOT NULL DEFAULT 0,
  total_store_debit numeric(20,2) NOT NULL DEFAULT 0,
  provider_partner_ref_no text,
  provider_inquiry_id text,
  status text NOT NULL CHECK (status IN ('pending', 'success', 'failed')),
  request_payload_masked jsonb NOT NULL DEFAULT '{}'::jsonb,
  provider_payload_masked jsonb NOT NULL DEFAULT '{}'::jsonb,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  CONSTRAINT store_withdrawals_store_id_idempotency_key_unique UNIQUE (store_id, idempotency_key),
  CONSTRAINT store_withdrawals_provider_partner_ref_no_unique UNIQUE (provider_partner_ref_no)
);

CREATE INDEX store_withdrawals_store_id_created_at_idx ON store_withdrawals (store_id, created_at DESC);
CREATE INDEX store_withdrawals_status_idx ON store_withdrawals (status);
