CREATE INDEX qris_transactions_store_type_status_created_at_idx
  ON qris_transactions (store_id, type, status, created_at DESC);

CREATE INDEX store_withdrawals_store_status_created_at_idx
  ON store_withdrawals (store_id, status, created_at DESC);
