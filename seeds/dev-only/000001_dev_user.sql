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
) VALUES (
  '11111111-1111-1111-1111-111111111111',
  'dev@example.com',
  'dev',
  crypt('DevDemo123!', gen_salt('bf', 12)),
  'dev',
  true,
  false,
  NULL,
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

