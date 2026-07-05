CREATE TABLE sandboxes (
    id text PRIMARY KEY,
    environment text NOT NULL,
    template text NOT NULL,
    state text NOT NULL,
    compute text NOT NULL DEFAULT '',
    compute_api_version text NOT NULL DEFAULT '',
    compute_plugin_version text NOT NULL DEFAULT '',
    external_allocation_id text NOT NULL DEFAULT '',
    compute_config jsonb,
    compute_metadata jsonb,
    metadata jsonb,
    capabilities jsonb,
    lease_ttl_ms bigint NOT NULL,
    lease_expires_at timestamptz NOT NULL,
    inserted_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL
);

CREATE INDEX sandboxes_state_idx ON sandboxes (state);
CREATE INDEX sandboxes_inserted_at_idx ON sandboxes (inserted_at DESC);
CREATE INDEX sandboxes_lease_expires_at_idx ON sandboxes (lease_expires_at);

CREATE TABLE client_sessions (
    session_id text PRIMARY KEY,
    sandbox_id text NOT NULL REFERENCES sandboxes (id) ON DELETE CASCADE,
    agent_token_hash text NOT NULL,
    state text NOT NULL,
    capabilities jsonb,
    metadata jsonb,
    last_client_seq_seen bigint NOT NULL DEFAULT 0,
    last_server_seq_sent bigint NOT NULL DEFAULT 0,
    inserted_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL
);

CREATE UNIQUE INDEX client_sessions_sandbox_id_idx ON client_sessions (sandbox_id);
CREATE INDEX client_sessions_state_idx ON client_sessions (state);

CREATE TABLE api_tokens (
    id text PRIMARY KEY,
    name text NOT NULL,
    token_hash text NOT NULL UNIQUE,
    token_prefix text NOT NULL,
    metadata jsonb,
    last_used_at timestamptz,
    inserted_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL
);

CREATE INDEX api_tokens_inserted_at_idx ON api_tokens (inserted_at DESC);

CREATE TABLE commands (
    id text PRIMARY KEY,
    sandbox_id text NOT NULL REFERENCES sandboxes (id) ON DELETE CASCADE,
    state text NOT NULL,
    command text NOT NULL,
    cwd text NOT NULL,
    env jsonb,
    stdin boolean NOT NULL DEFAULT false,
    timeout_ms bigint NOT NULL,
    metadata jsonb,
    pid integer NOT NULL DEFAULT 0,
    exit_code integer,
    signal text NOT NULL DEFAULT '',
    started_at timestamptz,
    completed_at timestamptz,
    inserted_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL
);

CREATE INDEX commands_sandbox_id_inserted_at_idx ON commands (sandbox_id, inserted_at ASC);
CREATE INDEX commands_sandbox_id_state_idx ON commands (sandbox_id, state);

CREATE TABLE sandbox_event_sequences (
    sandbox_id text PRIMARY KEY REFERENCES sandboxes (id) ON DELETE CASCADE,
    next_seq bigint NOT NULL
);

CREATE TABLE events (
    id text PRIMARY KEY,
    sandbox_id text NOT NULL REFERENCES sandboxes (id) ON DELETE CASCADE,
    command_id text REFERENCES commands (id) ON DELETE SET NULL,
    seq bigint NOT NULL,
    type text NOT NULL,
    source text NOT NULL DEFAULT '',
    timestamp timestamptz NOT NULL,
    data jsonb,
    UNIQUE (sandbox_id, seq)
);

CREATE INDEX events_sandbox_id_seq_idx ON events (sandbox_id, seq ASC);
CREATE INDEX events_sandbox_id_command_id_seq_idx ON events (sandbox_id, command_id, seq ASC);
CREATE INDEX events_type_idx ON events (type);

CREATE TABLE idempotency_keys (
    key text PRIMARY KEY,
    value text NOT NULL,
    inserted_at timestamptz NOT NULL DEFAULT now()
);
