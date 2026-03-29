CREATE TABLE users (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  email citext NOT NULL UNIQUE,
  username citext NOT NULL UNIQUE,
  password_hash text NOT NULL,
  role text NOT NULL CHECK (role IN ('dev', 'superadmin', 'owner', 'karyawan')),
  is_active boolean NOT NULL DEFAULT true,
  totp_enabled boolean NOT NULL DEFAULT false,
  totp_secret_encrypted text NULL,
  ip_allowlist inet NULL,
  created_by_user_id uuid NULL REFERENCES users(id) ON DELETE SET NULL,
  last_login_at timestamptz NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX users_role_idx ON users (role);
CREATE INDEX users_created_by_user_id_idx ON users (created_by_user_id);

CREATE TABLE user_recovery_codes (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  code_hash text NOT NULL,
  used_at timestamptz NULL,
  created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX user_recovery_codes_user_id_idx ON user_recovery_codes (user_id);

CREATE TABLE user_sessions (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  session_jti text NOT NULL UNIQUE,
  refresh_hash text NOT NULL,
  ip_address inet NOT NULL,
  user_agent text NOT NULL,
  expires_at timestamptz NOT NULL,
  revoked_at timestamptz NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX user_sessions_user_id_idx ON user_sessions (user_id);
CREATE INDEX user_sessions_expires_at_idx ON user_sessions (expires_at);

CREATE TABLE stores (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  owner_user_id uuid NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
  name text NOT NULL,
  slug citext NOT NULL UNIQUE,
  status text NOT NULL CHECK (status IN ('active', 'inactive', 'banned', 'deleted')),
  api_token_hash text NOT NULL DEFAULT '',
  callback_url text NOT NULL DEFAULT '',
  current_balance numeric(20, 2) NOT NULL DEFAULT 0,
  low_balance_threshold numeric(20, 2) NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  deleted_at timestamptz NULL
);

CREATE INDEX stores_owner_user_id_idx ON stores (owner_user_id);
CREATE INDEX stores_status_idx ON stores (status);
CREATE INDEX stores_active_idx ON stores (id) WHERE deleted_at IS NULL;

CREATE TABLE store_staff (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  store_id uuid NOT NULL REFERENCES stores(id) ON DELETE CASCADE,
  user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  created_by_owner_id uuid NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
  created_at timestamptz NOT NULL DEFAULT now(),
  CONSTRAINT store_staff_store_id_user_id_unique UNIQUE (store_id, user_id)
);

CREATE INDEX store_staff_user_id_idx ON store_staff (user_id);
CREATE INDEX store_staff_store_id_idx ON store_staff (store_id);

CREATE TABLE store_bank_accounts (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  store_id uuid NOT NULL REFERENCES stores(id) ON DELETE CASCADE,
  bank_code text NOT NULL,
  bank_name text NOT NULL,
  account_number_encrypted text NOT NULL,
  account_number_masked text NOT NULL,
  account_name text NOT NULL,
  verified_at timestamptz NULL,
  is_active boolean NOT NULL DEFAULT true,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX store_bank_accounts_store_id_idx ON store_bank_accounts (store_id);
CREATE INDEX store_bank_accounts_bank_code_idx ON store_bank_accounts (bank_code);
CREATE INDEX store_bank_accounts_store_id_is_active_idx
  ON store_bank_accounts (store_id, is_active) WHERE is_active IS TRUE;

CREATE TABLE audit_logs (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  actor_user_id uuid NULL REFERENCES users(id) ON DELETE SET NULL,
  actor_role text NOT NULL,
  store_id uuid NULL REFERENCES stores(id) ON DELETE SET NULL,
  action text NOT NULL,
  target_type text NOT NULL,
  target_id uuid NULL,
  payload_masked jsonb NOT NULL DEFAULT '{}'::jsonb,
  ip_address inet NULL,
  user_agent text NULL,
  created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX audit_logs_store_id_created_at_idx ON audit_logs (store_id, created_at DESC);
CREATE INDEX audit_logs_actor_user_id_created_at_idx ON audit_logs (actor_user_id, created_at DESC);
CREATE INDEX audit_logs_action_idx ON audit_logs (action);
