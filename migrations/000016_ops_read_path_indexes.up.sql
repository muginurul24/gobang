CREATE INDEX audit_logs_created_at_desc_idx
  ON audit_logs (created_at DESC);

CREATE INDEX audit_logs_actor_role_created_at_idx
  ON audit_logs (actor_role, created_at DESC);

CREATE INDEX audit_logs_target_type_created_at_idx
  ON audit_logs (target_type, created_at DESC);

CREATE INDEX chat_messages_active_created_at_desc_idx
  ON chat_messages (created_at DESC)
  WHERE deleted_at IS NULL;
