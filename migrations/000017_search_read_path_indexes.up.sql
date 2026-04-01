CREATE EXTENSION IF NOT EXISTS "pg_trgm";

CREATE INDEX stores_owner_created_at_desc_idx
  ON stores (owner_user_id, created_at DESC)
  WHERE deleted_at IS NULL;

CREATE INDEX store_staff_store_created_at_desc_idx
  ON store_staff (store_id, created_at DESC);

CREATE INDEX users_created_by_role_created_at_desc_idx
  ON users (created_by_user_id, created_at DESC)
  WHERE role = 'karyawan';

CREATE INDEX stores_search_trgm_idx
  ON stores
  USING gin (
    (COALESCE(name, '') || ' ' || COALESCE(slug::text, '') || ' ' || COALESCE(callback_url, ''))
    gin_trgm_ops
  )
  WHERE deleted_at IS NULL;

CREATE INDEX users_karyawan_search_trgm_idx
  ON users
  USING gin (
    (COALESCE(username::text, '') || ' ' || COALESCE(email::text, ''))
    gin_trgm_ops
  )
  WHERE role = 'karyawan';

CREATE INDEX store_members_search_trgm_idx
  ON store_members
  USING gin (
    (COALESCE(real_username, '') || ' ' || COALESCE(upstream_user_code, ''))
    gin_trgm_ops
  );

CREATE INDEX provider_catalogs_search_trgm_idx
  ON provider_catalogs
  USING gin (
    (COALESCE(provider_code, '') || ' ' || COALESCE(provider_name, ''))
    gin_trgm_ops
  );

CREATE INDEX provider_games_search_trgm_idx
  ON provider_games
  USING gin (
    (
      COALESCE(provider_code, '')
      || ' '
      || COALESCE(game_code, '')
      || ' '
      || COALESCE(game_name->>'default', '')
    )
    gin_trgm_ops
  );

CREATE INDEX qris_transactions_search_trgm_idx
  ON qris_transactions
  USING gin (
    (
      COALESCE(custom_ref, '')
      || ' '
      || COALESCE(external_username, '')
      || ' '
      || COALESCE(provider_trx_id, '')
    )
    gin_trgm_ops
  );

CREATE INDEX store_bank_accounts_search_trgm_idx
  ON store_bank_accounts
  USING gin (
    (
      COALESCE(bank_code, '')
      || ' '
      || COALESCE(bank_name, '')
      || ' '
      || COALESCE(account_name, '')
      || ' '
      || COALESCE(account_number_masked, '')
    )
    gin_trgm_ops
  );

CREATE INDEX store_withdrawals_search_trgm_idx
  ON store_withdrawals
  USING gin (
    (COALESCE(idempotency_key, '') || ' ' || COALESCE(provider_partner_ref_no, ''))
    gin_trgm_ops
  );

CREATE INDEX notifications_search_trgm_idx
  ON notifications
  USING gin (
    (COALESCE(event_type, '') || ' ' || COALESCE(title, '') || ' ' || COALESCE(body, ''))
    gin_trgm_ops
  );

CREATE INDEX chat_messages_body_trgm_idx
  ON chat_messages
  USING gin (body gin_trgm_ops)
  WHERE deleted_at IS NULL;
