# Garden ↔ Seed Protocol Draft

This document defines the initial control protocol between Garden (control plane) and Seed (in-sandbox daemon).

## Decisions

- transport: **WebSocket first**
- authentication: **session key issued by Garden for a specific sandbox session**
- sandbox identity: **Garden is source of truth for `sandbox_id`**
- command identity: **Garden is source of truth for `command_id`**
- delivery: **per-message acknowledgements** plus ordered sequencing and replay hooks
- scope: include a broad event surface now and narrow later if needed

## Why WebSocket over long-lived HTTP streaming

WebSocket is the better first transport here because the protocol is inherently bidirectional:
- Seed must initiate a connection back to Garden
- Garden must push commands to Seed
- Seed must stream output, heartbeats, metrics, and events back
- both sides need acknowledgements and reconnect support

Long-lived HTTP streaming works well for one-way event delivery, but becomes awkward when you need:
- server → client commands
- client → server event streams
- correlated acknowledgements in both directions
- resumable multiplexed traffic on a single session

Recommendation:
- **v1 transport:** WebSocket
- **future optional transport:** HTTP SSE for Garden → customer streams, not Garden ↔ Seed

## Connection model

Seed always dials out to Garden.

Example:

```text
wss://garden.example.com/ws/seed
```

Garden authenticates the connection using a short-lived session key minted when sandbox compute is provisioned.

## Authentication model

Garden passes Seed at boot:
- `sandbox_id`
- `session_key`
- `garden_ws_url`
- optional `session_id`

Properties of `session_key`:
- scoped to one sandbox session
- short-lived
- revocable
- not reusable across sandboxes
- not a customer API key

## Protocol goals

- ordered, resumable, bidirectional messaging
- explicit ack semantics
- durable command lifecycle reporting
- capability negotiation
- robust reconnect behavior
- future extensibility for files, PTY, networking, and snapshots

## Envelope

Every message uses the same envelope.

```json
{
  "version": "1",
  "type": "command.start",
  "message_id": "msg_123",
  "ack_id": null,
  "request_id": "req_123",
  "session_id": "sess_123",
  "sandbox_id": "sbx_123",
  "seq": 42,
  "timestamp": "2026-05-08T22:00:00Z",
  "expects_ack": true,
  "payload": {
    "command_id": "cmd_123",
    "command": "npm test",
    "cwd": "/workspace",
    "env": {
      "CI": "1"
    },
    "stdin": true,
    "timeout_ms": 600000
  }
}
```

## Envelope fields

Required:
- `version` — protocol version, initially `"1"`
- `type` — message type
- `message_id` — unique id for this message
- `request_id` — correlation id spanning a logical operation
- `session_id` — logical control session id
- `sandbox_id` — Garden-owned sandbox id
- `seq` — sender-local monotonic sequence number
- `timestamp` — RFC3339 UTC timestamp
- `payload` — typed body

Optional:
- `ack_id` — message id being acknowledged
- `expects_ack` — whether receiver must ack this message
- `reply_to` — prior message id if this is a direct response
- `error` — structured error object if type is an error/negative response

## Acknowledgement model

Every protocol message that mutates state or advances lifecycle should be acked.

Ack message:

```json
{
  "version": "1",
  "type": "ack",
  "message_id": "msg_900",
  "ack_id": "msg_123",
  "request_id": "req_123",
  "session_id": "sess_123",
  "sandbox_id": "sbx_123",
  "seq": 87,
  "timestamp": "2026-05-08T22:00:01Z",
  "expects_ack": false,
  "payload": {
    "status": "accepted"
  }
}
```

Ack payload statuses:
- `accepted`
- `rejected`
- `duplicate`
- `unsupported`

Notes:
- ack confirms receipt and parse/acceptance, not completion
- completion is reported by later lifecycle events
- receivers should deduplicate by `message_id`

## Ordering and replay

Each side maintains its own monotonic `seq`.

Track at least:
- last Garden seq seen by Seed
- last Seed seq seen by Garden

Reconnect should include replay intent:
- last seq seen from peer
- current session id
- current sandbox id

This lets each side decide whether replay is possible, required, or whether the session must be replaced.

## Connection lifecycle

### 1. Seed opens socket
Seed connects to Garden using the session key.

### 2. Handshake
#### `seed.hello`

```json
{
  "type": "seed.hello",
  "payload": {
    "seed_version": "0.1.0",
    "protocol_version": "1",
    "platform": "linux",
    "arch": "amd64",
    "hostname": "sandbox-host",
    "session_key": "redacted"
  }
}
```

#### `garden.hello`

```json
{
  "type": "garden.hello",
  "payload": {
    "protocol_version": "1",
    "session_id": "sess_123",
    "sandbox_id": "sbx_123",
    "heartbeat_interval_ms": 10000,
    "ack_timeout_ms": 5000
  }
}
```

### 3. Registration
#### `seed.register`
Sent once Seed has accepted Garden’s identity and wants to bind runtime details.

Payload:
- `seed_id` — ephemeral runtime id
- `seed_version`
- `platform`
- `arch`
- `hostname`
- `boot_time`
- `workspace_root`
- `process_id`

#### `garden.registered`
Confirms the session is active.

Payload:
- `session_id`
- `sandbox_id`
- `lease_expires_at`
- `max_session_duration_ms`

### 4. Capability advertisement
#### `seed.capabilities`

Payload example:

```json
{
  "commands": true,
  "stdin": true,
  "cancel": true,
  "kill": true,
  "files": {
    "read": true,
    "write": true,
    "edit": true,
    "search": true,
    "stat": true,
    "list": true
  },
  "pty": false,
  "metrics": true,
  "ports": false,
  "snapshots": false
}
```

Garden should not assume unsupported features exist.

## Heartbeats and liveness

#### `seed.heartbeat`
Sent periodically.

Payload:
- `uptime_ms`
- `active_commands`
- `last_garden_seq_seen`
- `last_seed_seq_sent`
- `load_avg` if available
- `memory_used_bytes`
- `connection_generation`

#### `garden.heartbeat_ack`
Payload:
- `lease_expires_at`
- `server_time`
- `status`

Potential statuses:
- `ok`
- `draining`
- `reauth_required`

## Status and metrics

### `seed.status`
A structured runtime snapshot.

Suggested payload:
- `state` — `booting | ready | degraded | draining`
- `hostname`
- `platform`
- `arch`
- `seed_version`
- `uptime_ms`
- `workspace_root`
- `active_commands`
- `network` summary

### `seed.metrics`
Suggested payload:
- `cpu_percent`
- `load_avg_1m`
- `memory_total_bytes`
- `memory_used_bytes`
- `disk_total_bytes`
- `disk_used_bytes`
- `fs_available_bytes`
- `rx_bytes`
- `tx_bytes`
- `sampled_at`

## Activity and lease signaling

### `seed.activity`
Used to signal meaningful customer-visible activity.

Payload:
- `kind` — `command_output | stdin | file_op | session_active`
- `command_id` optional
- `at`

### `garden.lease_extended`
Payload:
- `lease_expires_at`
- `reason`

### `garden.lease_warning`
Payload:
- `lease_expires_at`
- `remaining_ms`

### `garden.lease_expiring`
Payload:
- `lease_expires_at`
- `remaining_ms`
- `action` — typically `release`

## Command protocol

Garden is source of truth for `command_id`.

### Start command
#### `command.start`

Payload:
- `command_id`
- `command`
- `cwd`
- `env`
- `stdin`
- `timeout_ms`
- `metadata`

#### `command.accepted`
Seed has accepted the request and queued/spawned work.

Payload:
- `command_id`
- `state` — `queued | starting`

#### `command.started`
Payload:
- `command_id`
- `pid`
- `started_at`

#### `command.stdout`
Payload:
- `command_id`
- `chunk`
- `encoding` — usually `utf-8`
- `stream_seq`

#### `command.stderr`
Payload:
- `command_id`
- `chunk`
- `encoding`
- `stream_seq`

#### `command.exit`
Payload:
- `command_id`
- `exit_code`
- `completed_at`

#### `command.failed`
Payload:
- `command_id`
- `exit_code` optional
- `message`
- `completed_at`

#### `command.cancelled`
Payload:
- `command_id`
- `completed_at`

#### `command.killed`
Payload:
- `command_id`
- `signal`
- `completed_at`

### Send stdin
#### `command.stdin`
Payload:
- `command_id`
- `data`
- `encoding`

#### `command.stdin.accepted`
Payload:
- `command_id`
- `bytes`

### Cancel command
#### `command.cancel`
Payload:
- `command_id`
- `grace_period_ms`
- `escalation` — `kill`

Seed behavior:
- send graceful signal first
- if process does not exit within grace period, escalate to kill

### Kill command
#### `command.kill`
Payload:
- `command_id`
- `signal` optional

## File operation protocol

These may not all be implemented in v1, but reserve them now.

Garden → Seed:
- `file.read`
- `file.write`
- `file.edit`
- `file.stat`
- `file.search`
- `file.list`
- `file.delete`
- `file.mkdir`

Seed → Garden:
- `file.result`
- `file.chunk`
- `file.error`

Notes:
- large responses may need chunking
- every response should reference `request_id`

## PTY protocol

Not in v1, but reserve namespace.

Garden → Seed:
- `pty.create`
- `pty.input`
- `pty.resize`
- `pty.close`

Seed → Garden:
- `pty.created`
- `pty.output`
- `pty.exit`
- `pty.error`

## Port / service protocol

Reserve for future service exposure.

Garden → Seed:
- `port.open`
- `port.close`
- `port.describe`

Seed → Garden:
- `port.opened`
- `port.closed`
- `port.status`

## Snapshot / artifact protocol

Reserve namespace now.

Garden → Seed:
- `artifact.upload`
- `artifact.download`
- `snapshot.create`

Seed → Garden:
- `artifact.ready`
- `artifact.failed`
- `snapshot.created`

## Error protocol

### `error`
Use for structured request failures.

Envelope `payload`:
- `code`
- `message`
- `retryable`
- `details`
- `failed_message_id`

Example codes:
- `auth_failed`
- `invalid_message`
- `unsupported_type`
- `sandbox_mismatch`
- `session_expired`
- `command_not_found`
- `command_not_running`
- `spawn_failed`
- `permission_denied`
- `internal_error`

### `seed.warning`
For non-fatal issues.

Examples:
- low disk
- high memory
- degraded network
- partial metrics unavailable

## Resume and reconnect

Seed should reconnect using the same `sandbox_id` and a new socket if the prior transport is interrupted.

### `seed.resume`
Payload:
- `session_id`
- `sandbox_id`
- `last_garden_seq_seen`
- `last_seed_seq_sent`
- `seed_id`
- `connection_generation`

### `garden.resume`
Payload:
- `status` — `ok | replace_session | rejected`
- `session_id`
- `replay_from_seed_seq` optional
- `replay_from_garden_seq` optional

Possible behavior:
- if Garden still has active session state, resume
- if session is stale or replaced, reject and require fresh register

## Drain and shutdown

### `garden.drain`
Garden tells Seed to stop accepting new work.

Payload:
- `reason`
- `deadline`

### `seed.draining`
Seed acknowledges drain mode.

Payload:
- `active_commands`
- `estimated_completion_ms` optional

### `garden.shutdown`
Garden instructs Seed to terminate.

Payload:
- `reason`
- `deadline`

### `seed.goodbye`
Seed’s final best-effort message before exit.

Payload:
- `reason`
- `final_command_count`
- `sent_at`

## Delivery semantics

For v1:
- at-least-once delivery across reconnects
- deduplicate by `message_id`
- ordered processing per sender `seq`
- ack each message that requests acknowledgement
- lifecycle completion is separate from ack

## Recommended implementation notes

### On Garden side
Track:
- session by `sandbox_id`
- active socket by `session_id`
- last Seed seq seen
- last Garden seq acknowledged
- sent-but-unacked messages
- dedupe window for incoming Seed `message_id`s

### On Seed side
Track:
- current `session_id`
- last Garden seq seen
- last Seed seq sent
- sent-but-unacked messages
- running commands by Garden-issued `command_id`

## Minimal v1 required message types

### Handshake/session
- `seed.hello`
- `garden.hello`
- `seed.register`
- `garden.registered`
- `seed.resume`
- `garden.resume`
- `ack`
- `error`

### Runtime health
- `seed.capabilities`
- `seed.status`
- `seed.heartbeat`
- `garden.heartbeat_ack`
- `seed.metrics`
- `seed.warning`
- `seed.activity`
- `garden.lease_extended`
- `garden.lease_warning`
- `garden.lease_expiring`

### Commands
- `command.start`
- `command.accepted`
- `command.started`
- `command.stdout`
- `command.stderr`
- `command.stdin`
- `command.stdin.accepted`
- `command.cancel`
- `command.kill`
- `command.exit`
- `command.failed`
- `command.cancelled`
- `command.killed`

### Control lifecycle
- `garden.drain`
- `seed.draining`
- `garden.shutdown`
- `seed.goodbye`

## Open questions

- should message payloads include explicit tenant / account scoping beyond `sandbox_id`?
- should command output chunks support binary framing later?
- does Garden persist unacked outbound messages durably or only in memory at first?
- should metrics be periodic only, or also request/response driven?
- do file operations return inline bodies, chunked bodies, or references for large content?

## Suggested next steps

1. define JSON schemas for the envelope and each message payload
2. add Phoenix channel / WebSocket endpoint shape for Seed connections
3. define session key minting and validation rules
4. model Garden-side session state machine
5. build a mock Seed client against this protocol before implementing the real daemon
