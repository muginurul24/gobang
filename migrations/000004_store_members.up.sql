CREATE TABLE store_members (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  store_id uuid NOT NULL REFERENCES stores(id) ON DELETE CASCADE,
  real_username citext NOT NULL,
  upstream_user_code text NOT NULL,
  status text NOT NULL CHECK (status IN ('active', 'inactive')),
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  CONSTRAINT store_members_store_id_real_username_unique UNIQUE (store_id, real_username),
  CONSTRAINT store_members_upstream_user_code_unique UNIQUE (upstream_user_code)
);

CREATE INDEX store_members_store_id_idx ON store_members (store_id);
CREATE INDEX store_members_real_username_idx ON store_members (real_username);
