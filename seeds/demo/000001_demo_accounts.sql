INSERT INTO users (
  id,
  email,
  username,
  password_hash,
  role,
  is_active,
  totp_enabled,
  created_at,
  updated_at
) VALUES
  (
    'aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa',
    'dev@example.com',
    'dev-demo',
    '$2y$12$placeholderdevhash',
    'dev',
    true,
    false,
    now(),
    now()
  ),
  (
    'bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb',
    'owner@example.com',
    'owner-demo',
    '$2y$12$placeholderownerhash',
    'owner',
    true,
    false,
    now(),
    now()
  )
ON CONFLICT (email) DO NOTHING;

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
