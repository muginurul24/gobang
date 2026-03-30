CREATE TABLE provider_catalogs (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  provider_code text NOT NULL,
  provider_name text NOT NULL,
  status integer NOT NULL,
  synced_at timestamptz NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  CONSTRAINT provider_catalogs_provider_code_unique UNIQUE (provider_code)
);

CREATE TABLE provider_games (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  provider_code text NOT NULL,
  game_code text NOT NULL,
  game_name jsonb NOT NULL DEFAULT '{}'::jsonb,
  banner_url text NULL,
  status integer NOT NULL,
  synced_at timestamptz NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  CONSTRAINT provider_games_provider_code_game_code_unique UNIQUE (provider_code, game_code)
);

CREATE INDEX provider_games_provider_code_idx ON provider_games (provider_code);
CREATE INDEX provider_games_status_idx ON provider_games (status);

CREATE TABLE game_launch_logs (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  store_id uuid NOT NULL REFERENCES stores(id) ON DELETE CASCADE,
  store_member_id uuid NOT NULL REFERENCES store_members(id) ON DELETE RESTRICT,
  provider_code text NOT NULL,
  game_code text NOT NULL,
  lang text NOT NULL DEFAULT 'id',
  status text NOT NULL CHECK (status IN ('success', 'failed')),
  upstream_payload_masked jsonb NOT NULL DEFAULT '{}'::jsonb,
  created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX game_launch_logs_store_id_created_at_idx ON game_launch_logs (store_id, created_at DESC);
CREATE INDEX game_launch_logs_store_member_id_created_at_idx ON game_launch_logs (store_member_id, created_at DESC);
