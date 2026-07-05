# Postgres store schema

This document defines the first durable schema for Shed's existing `internal/store.Store` contract. It covers the current sandbox control-plane objects before the higher-level software factory objects are added.

Task coverage: `SF-020`.

## Goals

- Preserve existing sandbox API behavior.
- Keep all state mutations behind `internal/store.Store`.
- Make sandbox, session, command, token, event, and idempotency state durable.
- Preserve replayable per-sandbox and per-command event streams.
- Support transactional state changes and related event appends.
- Avoid persisting raw agent tokens, API tokens, or secrets.

## Conventions

- IDs remain application-generated strings with stable prefixes such as `sbx_`, `sess_`, `cmd_`, `evt_`, and `atok_`.
- Timestamps use `timestamptz` and should be written in UTC by the application.
- Flexible maps use `jsonb`.
- Token hashes are stored as hex strings.
- Raw API tokens and raw agent tokens are never stored.
- Event ordering is per sandbox using a monotonic `seq`.

## Tables

### `schema_migrations`

Tracks applied migrations.

```sql
CREATE TABLE schema_migrations (
    version text PRIMARY KEY,
    applied_at timestamptz NOT NULL DEFAULT now()
);
```

### `sandboxes`

Stores logical sandbox/allocation state.

```sql
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
```

Notes:

- `compute_config` can contain provider configuration and must be treated as sensitive in API responses.
- `capabilities` follows the existing `map[string]bool` shape.
- The lease sweeper should use `state` and `lease_expires_at` indexes.

### `client_sessions`

Stores client session state and hashed agent credentials.

```sql
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
```

Notes:

- The current model assumes one active session per sandbox.
- `agent_token_hash` must be compared in application code with constant-time comparison after lookup by sandbox ID.

### `api_tokens`

Stores customer-facing API tokens by hash.

```sql
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
```

Notes:

- `token_prefix` is safe for operator display.
- Authentication updates `last_used_at` and `updated_at`.
- Future scoped tokens should extend this table or add a related `api_token_scopes` table.

### `commands`

Stores command lifecycle state.

```sql
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
```

Notes:

- `env` may contain sensitive data and should not be broadly exposed without redaction.
- Command rows are updated as client or compute events arrive.

### `sandbox_event_sequences`

Stores the next event sequence per sandbox. This table exists to allocate event sequence numbers transactionally.

```sql
CREATE TABLE sandbox_event_sequences (
    sandbox_id text PRIMARY KEY REFERENCES sandboxes (id) ON DELETE CASCADE,
    next_seq bigint NOT NULL
);
```

Sequence allocation should occur in the same transaction as event insertion:

```sql
UPDATE sandbox_event_sequences
SET next_seq = next_seq + 1
WHERE sandbox_id = $1
RETURNING next_seq;
```

The returned `next_seq` is the event's `seq`.

### `events`

Stores append-only sandbox and command events.

```sql
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
```

Notes:

- Cursors are sequence numbers. `after=N` returns rows with `seq > N`.
- Command event queries should filter by `(sandbox_id, command_id, seq)`.
- `command_id` allows `NULL` for sandbox-level events.

### `idempotency_keys`

Stores external idempotency key results.

```sql
CREATE TABLE idempotency_keys (
    key text PRIMARY KEY,
    value text NOT NULL,
    inserted_at timestamptz NOT NULL DEFAULT now()
);
```

Notes:

- `RememberIdempotencyKey` maps directly to an insert with conflict handling.
- Factory-level creates should use scoped keys that include actor/source/resource intent.

## Store operation mapping

### `CreateSandbox`

Transaction:

1. Insert `sandboxes` row.
2. Insert `client_sessions` row with hashed agent token.
3. Insert `sandbox_event_sequences` row with `next_seq = 0`.
4. Append `sandbox.pending_client` event.
5. Return sandbox and one-time raw agent token.

### `UpdateSandboxState`

Transaction:

1. Update `sandboxes.state` and `updated_at`.
2. Append `sandbox.<state>` event.

### `UpdateSandboxAllocation`

Transaction:

1. Update compute allocation fields.
2. Append `sandbox.allocation.updated` event.

### `ExtendLease`

Transaction:

1. Update `lease_ttl_ms`, `lease_expires_at`, and `updated_at`.
2. Append `sandbox.lease.extended` event.

### `CreateCommand`

Transaction:

1. Confirm sandbox exists.
2. Insert `commands` row.
3. Append `command.queued` event.

### `UpdateCommand`

Transaction:

1. Update command fields and `updated_at`.
2. Do not append an event automatically unless the caller explicitly appends one, preserving current store behavior.

### `AppendEvent`

Transaction:

1. Confirm sandbox exists.
2. Allocate next sandbox sequence.
3. Insert event row.

### `RememberIdempotencyKey`

Transaction:

1. Insert `(key, value)`.
2. On conflict, return existing value with `created=false`.

## Migration ordering

The first migration should create tables in dependency order:

1. `schema_migrations`
2. `sandboxes`
3. `client_sessions`
4. `api_tokens`
5. `commands`
6. `sandbox_event_sequences`
7. `events`
8. `idempotency_keys`

Factory tables should be added in later migrations after the current store contract is durable.

## Open implementation choices

- Use `database/sql` with `pgx` driver or use `pgx` directly. Prefer the smallest dependency surface that still supports context, transactions, and reliable JSON handling.
- Keep enum-like states as `text` initially to avoid migration churn while state machines are still evolving.
- Consider a store contract test suite before adding the Postgres implementation so memory and Postgres behavior remain aligned.
