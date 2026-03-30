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
  '',
  '',
  0,
  100000,
  now(),
  now()
)
ON CONFLICT (slug) DO NOTHING;

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
  actor_user_id,
  actor_role,
  store_id,
  action,
  target_type,
  target_id,
  payload_masked,
  created_at
) VALUES (
  'bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb',
  'owner',
  'cccccccc-cccc-cccc-cccc-cccccccccccc',
  'seed.demo',
  'store',
  'cccccccc-cccc-cccc-cccc-cccccccccccc',
  '{"note":"demo seed inserted"}'::jsonb,
  now()
);
