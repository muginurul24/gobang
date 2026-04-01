CREATE INDEX IF NOT EXISTS outbound_callbacks_created_at_desc_idx
	ON outbound_callbacks (created_at DESC);

CREATE INDEX IF NOT EXISTS outbound_callbacks_status_created_at_desc_idx
	ON outbound_callbacks (status, created_at DESC);

CREATE INDEX IF NOT EXISTS outbound_callback_attempts_outbound_callback_id_created_at_desc_idx
	ON outbound_callback_attempts (outbound_callback_id, created_at DESC);
