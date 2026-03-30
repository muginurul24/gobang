CREATE TABLE notifications (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  scope_type text NOT NULL,
  scope_id text NOT NULL,
  event_type text NOT NULL,
  title text NOT NULL,
  body text NOT NULL,
  read_at timestamptz NULL,
  created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX notifications_scope_type_scope_id_created_at_idx ON notifications (scope_type, scope_id, created_at DESC);
CREATE INDEX notifications_read_at_idx ON notifications (read_at);
