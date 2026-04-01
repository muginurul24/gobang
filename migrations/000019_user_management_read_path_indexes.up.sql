CREATE INDEX users_role_is_active_created_at_desc_idx
  ON users (role, is_active, created_at DESC)
  WHERE role IN ('dev', 'superadmin', 'owner');

CREATE INDEX users_platform_search_trgm_idx
  ON users
  USING gin (
    (COALESCE(username::text, '') || ' ' || COALESCE(email::text, ''))
    gin_trgm_ops
  )
  WHERE role IN ('dev', 'superadmin', 'owner');

