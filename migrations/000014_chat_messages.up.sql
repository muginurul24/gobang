CREATE TABLE chat_messages (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  sender_user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  body text NOT NULL,
  deleted_by_dev_user_id uuid NULL REFERENCES users(id) ON DELETE SET NULL,
  deleted_at timestamptz NULL,
  created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX chat_messages_created_at_desc_idx
  ON chat_messages (created_at DESC);

CREATE INDEX chat_messages_sender_user_id_idx
  ON chat_messages (sender_user_id);
