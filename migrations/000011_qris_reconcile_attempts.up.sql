CREATE TABLE qris_reconcile_attempts (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  qris_transaction_id uuid NOT NULL REFERENCES qris_transactions(id) ON DELETE CASCADE,
  attempt_no integer NOT NULL CHECK (attempt_no > 0),
  status text NOT NULL,
  response_masked jsonb NOT NULL DEFAULT '{}'::jsonb,
  created_at timestamptz NOT NULL DEFAULT now(),
  CONSTRAINT qris_reconcile_attempts_qris_transaction_id_attempt_no_unique UNIQUE (qris_transaction_id, attempt_no)
);

CREATE INDEX qris_reconcile_attempts_qris_transaction_id_attempt_no_idx ON qris_reconcile_attempts (qris_transaction_id, attempt_no);
