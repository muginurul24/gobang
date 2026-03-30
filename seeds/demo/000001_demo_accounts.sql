INSERT INTO users (
  id,
  email,
  username,
  password_hash,
  role,
  is_active,
  totp_enabled,
  created_by_user_id,
  created_at,
  updated_at
) VALUES
  (
    'aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa',
    'dev@example.com',
    'dev-demo',
    crypt('DevDemo123!', gen_salt('bf', 12)),
    'dev',
    true,
    false,
    NULL,
    now(),
    now()
  ),
  (
    '99999999-9999-9999-9999-999999999999',
    'superadmin@example.com',
    'superadmin-demo',
    crypt('SuperadminDemo123!', gen_salt('bf', 12)),
    'superadmin',
    true,
    false,
    NULL,
    now(),
    now()
  ),
  (
    'bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb',
    'owner@example.com',
    'owner-demo',
    crypt('OwnerDemo123!', gen_salt('bf', 12)),
    'owner',
    true,
    false,
    NULL,
    now(),
    now()
  ),
  (
    'dddddddd-dddd-dddd-dddd-dddddddddddd',
    'staff@example.com',
    'staff-demo',
    crypt('StaffDemo123!', gen_salt('bf', 12)),
    'karyawan',
    true,
    false,
    'bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb',
    now(),
    now()
  )
ON CONFLICT (email) DO UPDATE
SET
  username = EXCLUDED.username,
  password_hash = EXCLUDED.password_hash,
  role = EXCLUDED.role,
  is_active = EXCLUDED.is_active,
  totp_enabled = EXCLUDED.totp_enabled,
  created_by_user_id = EXCLUDED.created_by_user_id,
  updated_at = now();

INSERT INTO stores (
  id,
  owner_user_id,
  name,
  slug,
  status,
  api_token_hash,
  callback_url,
  current_balance,
  low_balance_threshold,
  created_at,
  updated_at
) VALUES (
  'cccccccc-cccc-cccc-cccc-cccccccccccc',
  'bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb',
  'Demo Store',
  'demo-store',
  'active',
  encode(digest('store_live_demo', 'sha256'), 'hex'),
  'https://merchant.example.com/callback',
  0,
  100000,
  now(),
  now()
)
ON CONFLICT (slug) DO UPDATE
SET
  owner_user_id = EXCLUDED.owner_user_id,
  name = EXCLUDED.name,
  status = EXCLUDED.status,
  api_token_hash = EXCLUDED.api_token_hash,
  callback_url = EXCLUDED.callback_url,
  low_balance_threshold = EXCLUDED.low_balance_threshold,
  deleted_at = NULL,
  updated_at = now();

INSERT INTO ledger_accounts (store_id, currency, created_at)
VALUES (
  'cccccccc-cccc-cccc-cccc-cccccccccccc',
  'IDR',
  now()
)
ON CONFLICT (store_id) DO NOTHING;

INSERT INTO store_members (
  id,
  store_id,
  real_username,
  upstream_user_code,
  status,
  created_at,
  updated_at
) VALUES
  (
    'eeeeeeee-eeee-eeee-eeee-eeeeeeeeeeee',
    'cccccccc-cccc-cccc-cccc-cccccccccccc',
    'member-demo',
    'MEMBER000001',
    'active',
    now(),
    now()
  ),
  (
    'f1f1f1f1-f1f1-f1f1-f1f1-f1f1f1f1f1f1',
    'cccccccc-cccc-cccc-cccc-cccccccccccc',
    'member-alpha',
    'MEMBER000002',
    'active',
    now(),
    now()
  )
ON CONFLICT (store_id, real_username) DO UPDATE
SET
  status = EXCLUDED.status,
  updated_at = now();

INSERT INTO provider_catalogs (
  provider_code,
  provider_name,
  status,
  synced_at,
  created_at,
  updated_at
) VALUES
  (
    'PRAGMATIC',
    'PRAGMATIC',
    1,
    now(),
    now(),
    now()
  ),
  (
    'HACKSAW',
    'HACKSAW',
    1,
    now(),
    now(),
    now()
  )
ON CONFLICT (provider_code) DO UPDATE
SET
  provider_name = EXCLUDED.provider_name,
  status = EXCLUDED.status,
  synced_at = EXCLUDED.synced_at,
  updated_at = now();

INSERT INTO provider_games (
  provider_code,
  game_code,
  game_name,
  banner_url,
  status,
  synced_at,
  created_at,
  updated_at
) VALUES
  (
    'PRAGMATIC',
    'vs20doghouse',
    '{"default":"The Dog House"}'::jsonb,
    NULL,
    1,
    now(),
    now(),
    now()
  ),
  (
    'HACKSAW',
    'wanteddead',
    '{"default":"Wanted Dead or a Wild"}'::jsonb,
    NULL,
    1,
    now(),
    now(),
    now()
  )
ON CONFLICT (provider_code, game_code) DO UPDATE
SET
  game_name = EXCLUDED.game_name,
  banner_url = EXCLUDED.banner_url,
  status = EXCLUDED.status,
  synced_at = EXCLUDED.synced_at,
  updated_at = now();

INSERT INTO store_staff (
  store_id,
  user_id,
  created_by_owner_id,
  created_at
) VALUES (
  'cccccccc-cccc-cccc-cccc-cccccccccccc',
  'dddddddd-dddd-dddd-dddd-dddddddddddd',
  'bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb',
  now()
)
ON CONFLICT (store_id, user_id) DO NOTHING;

INSERT INTO audit_logs (
  id,
  actor_user_id,
  actor_role,
  store_id,
  action,
  target_type,
  target_id,
  payload_masked,
  created_at
) VALUES (
  'abababab-cdcd-efef-abab-cdcdababcdcd',
  'bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb',
  'owner',
  'cccccccc-cccc-cccc-cccc-cccccccccccc',
  'seed.demo',
  'store',
  'cccccccc-cccc-cccc-cccc-cccccccccccc',
  '{"note":"demo seed inserted"}'::jsonb,
  now()
)
ON CONFLICT (id) DO UPDATE
SET
  payload_masked = EXCLUDED.payload_masked,
  created_at = EXCLUDED.created_at;
