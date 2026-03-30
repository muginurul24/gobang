CREATE TABLE outbound_callbacks (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  store_id uuid NOT NULL REFERENCES stores(id) ON DELETE CASCADE,
  event_type text NOT NULL,
  reference_type text NOT NULL,
  reference_id uuid NOT NULL,
  payload_json jsonb NOT NULL DEFAULT '{}'::jsonb,
  signature text NOT NULL,
  status text NOT NULL CHECK (status IN ('pending', 'success', 'failed', 'retrying')),
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  CONSTRAINT outbound_callbacks_event_type_reference_unique UNIQUE (event_type, reference_type, reference_id)
);

CREATE INDEX outbound_callbacks_store_id_created_at_idx ON outbound_callbacks (store_id, created_at DESC);
CREATE INDEX outbound_callbacks_status_idx ON outbound_callbacks (status);
CREATE INDEX outbound_callbacks_reference_type_reference_id_idx ON outbound_callbacks (reference_type, reference_id);

CREATE TABLE outbound_callback_attempts (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  outbound_callback_id uuid NOT NULL REFERENCES outbound_callbacks(id) ON DELETE CASCADE,
  attempt_no integer NOT NULL CHECK (attempt_no > 0),
  http_status integer NULL,
  status text NOT NULL CHECK (status IN ('success', 'failed')),
  response_body_masked text NOT NULL DEFAULT '',
  next_retry_at timestamptz NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  CONSTRAINT outbound_callback_attempts_outbound_callback_id_attempt_no_unique UNIQUE (outbound_callback_id, attempt_no)
);

CREATE INDEX outbound_callback_attempts_outbound_callback_id_attempt_no_idx ON outbound_callback_attempts (outbound_callback_id, attempt_no);
