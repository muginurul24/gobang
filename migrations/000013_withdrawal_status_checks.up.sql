CREATE TABLE withdrawal_status_checks (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  store_withdrawal_id uuid NOT NULL REFERENCES store_withdrawals(id) ON DELETE CASCADE,
  attempt_no integer NOT NULL,
  status text NOT NULL,
  response_masked jsonb NOT NULL DEFAULT '{}'::jsonb,
  created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX withdrawal_status_checks_store_withdrawal_id_attempt_no_idx
  ON withdrawal_status_checks (store_withdrawal_id, attempt_no);
