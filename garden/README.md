# Garden

Garden is the control plane and API surface for provisioning fresh sandbox compute and coordinating work performed by the in-sandbox `seed` daemon.

This document captures the initial Phoenix-oriented API draft for:
- acquiring a sandbox for a customer session
- managing the sandbox lease / lifecycle
- starting, observing, and controlling commands
- streaming durable events over unreliable networks

## Design constraints

The v1 API assumes:
- **fresh sandbox per acquisition** to avoid cross-session data leakage
- customers generally **hold a sandbox for a session**
- many operations are **async by default**
- command and lifecycle activity may traverse multiple hops:
  - compute ↔ seed
  - seed ↔ garden
  - garden ↔ customer
- network interruptions are expected, so streams must be **resumable**
- command output should be **durably stored** for replay / reconnect
- sandbox lifetime should be **lease-based**, with TTL extension driven by activity
- `cancel` means graceful termination first, then forced termination if needed
- **PTY / full terminal support is not part of v1**, but the API should leave room for it later

## Resource model

Primary resources:
- `sandbox`
- `lease`
- `command`
- `event`
- `operation` (optional server-side async work tracker)

### Sandbox states

Suggested states:
- `provisioning`
- `booting`
- `ready`
- `degraded`
- `releasing`
- `released`
- `failed`

### Command states

Suggested states:
- `queued`
- `starting`
- `running`
- `cancelling`
- `exited`
- `killed`
- `failed`

## API shape

Versioned JSON API under `/api/v1`.

### Sandboxes

- `POST /api/v1/sandboxes` — acquire a fresh sandbox
- `GET /api/v1/sandboxes` — list sandboxes
- `GET /api/v1/sandboxes/:sandbox_id` — fetch sandbox details
- `POST /api/v1/sandboxes/:sandbox_id/release` — release / terminate sandbox
- `POST /api/v1/sandboxes/:sandbox_id/lease` — extend sandbox TTL
- `GET /api/v1/sandboxes/:sandbox_id/events` — durable sandbox event stream / replay

### Commands

- `POST /api/v1/sandboxes/:sandbox_id/commands` — start a command
- `GET /api/v1/sandboxes/:sandbox_id/commands` — list commands for sandbox
- `GET /api/v1/sandboxes/:sandbox_id/commands/:command_id` — fetch command details
- `POST /api/v1/sandboxes/:sandbox_id/commands/:command_id/stdin` — send stdin bytes / text
- `POST /api/v1/sandboxes/:sandbox_id/commands/:command_id/cancel` — graceful stop, escalating if needed
- `POST /api/v1/sandboxes/:sandbox_id/commands/:command_id/kill` — hard stop
- `GET /api/v1/sandboxes/:sandbox_id/commands/:command_id/events` — durable command event stream / replay

## Phoenix router draft

```elixir
scope "/api/v1", GardenWeb.Api.V1 do
  pipe_through :api

  resources "/sandboxes", SandboxController, only: [:index, :create, :show], param: "sandbox_id" do
    post "/release", SandboxController, :release
    post "/lease", SandboxController, :lease
    get "/events", SandboxController, :events

    resources "/commands", CommandController, only: [:index, :create, :show], param: "command_id" do
      post "/stdin", CommandController, :stdin
      post "/cancel", CommandController, :cancel
      post "/kill", CommandController, :kill
      get "/events", CommandController, :events
    end
  end
end
```

## Request / response draft

### Acquire sandbox

`POST /api/v1/sandboxes`

```json
{
  "environment": "linux",
  "template": "ubuntu-dev",
  "lease": {
    "ttl_ms": 1800000
  },
  "metadata": {
    "project_id": "proj_123",
    "session_id": "sess_123"
  }
}
```

Response:

```json
{
  "data": {
    "id": "sbx_123",
    "environment": "linux",
    "template": "ubuntu-dev",
    "state": "provisioning",
    "lease": {
      "ttl_ms": 1800000,
      "expires_at": "2026-05-08T20:00:00Z"
    },
    "metadata": {
      "project_id": "proj_123",
      "session_id": "sess_123"
    }
  },
  "operation": {
    "id": "op_123",
    "state": "pending"
  }
}
```

Notes:
- acquisition is async-first
- clients should expect a sandbox to move through `provisioning` / `booting` before `ready`
- use an idempotency key for retries

Suggested headers:
- `Idempotency-Key`
- `X-Request-Id`

### Get sandbox

`GET /api/v1/sandboxes/:sandbox_id`

```json
{
  "data": {
    "id": "sbx_123",
    "state": "ready",
    "environment": "linux",
    "template": "ubuntu-dev",
    "lease": {
      "ttl_ms": 1800000,
      "expires_at": "2026-05-08T20:00:00Z"
    },
    "capabilities": {
      "commands": true,
      "files": true,
      "pty": false
    }
  }
}
```

### Extend lease

`POST /api/v1/sandboxes/:sandbox_id/lease`

```json
{
  "ttl_ms": 1800000,
  "reason": "customer_activity"
}
```

Response:

```json
{
  "data": {
    "sandbox_id": "sbx_123",
    "ttl_ms": 1800000,
    "expires_at": "2026-05-08T20:30:00Z"
  }
}
```

### Release sandbox

`POST /api/v1/sandboxes/:sandbox_id/release`

```json
{
  "reason": "session_complete"
}
```

Response:

```json
{
  "data": {
    "id": "sbx_123",
    "state": "releasing"
  }
}
```

## Command APIs

### Start command

`POST /api/v1/sandboxes/:sandbox_id/commands`

```json
{
  "command": "npm test",
  "cwd": "/workspace",
  "env": {
    "CI": "1"
  },
  "stdin": true,
  "timeout_ms": 600000,
  "metadata": {
    "initiator": "customer"
  }
}
```

Response:

```json
{
  "data": {
    "id": "cmd_123",
    "sandbox_id": "sbx_123",
    "state": "queued",
    "command": "npm test",
    "cwd": "/workspace",
    "stdin": true,
    "timeout_ms": 600000,
    "started_at": null,
    "completed_at": null
  }
}
```

### Get command

`GET /api/v1/sandboxes/:sandbox_id/commands/:command_id`

```json
{
  "data": {
    "id": "cmd_123",
    "sandbox_id": "sbx_123",
    "state": "running",
    "command": "npm test",
    "pid": 4242,
    "exit_code": null,
    "signal": null,
    "started_at": "2026-05-08T19:02:00Z",
    "completed_at": null
  }
}
```

### List commands

`GET /api/v1/sandboxes/:sandbox_id/commands`

```json
{
  "data": [
    {
      "id": "cmd_123",
      "state": "running",
      "command": "npm test"
    }
  ]
}
```

### Send stdin

`POST /api/v1/sandboxes/:sandbox_id/commands/:command_id/stdin`

```json
{
  "data": "yes\n",
  "encoding": "utf-8"
}
```

Response:

```json
{
  "data": {
    "accepted": true
  }
}
```

### Cancel command

`POST /api/v1/sandboxes/:sandbox_id/commands/:command_id/cancel`

```json
{
  "grace_period_ms": 5000,
  "escalation": "kill"
}
```

Response:

```json
{
  "data": {
    "id": "cmd_123",
    "state": "cancelling"
  }
}
```

Semantics:
- send graceful stop first (for example `SIGTERM`)
- if the process does not exit inside `grace_period_ms`, escalate to hard stop
- final disposition appears in command events and command state

### Kill command

`POST /api/v1/sandboxes/:sandbox_id/commands/:command_id/kill`

```json
{}
```

Response:

```json
{
  "data": {
    "id": "cmd_123",
    "state": "killed"
  }
}
```

## Durable events

Event streaming is central to the design.

Instead of exposing output only as a raw transient socket stream, Garden should persist ordered event records and allow replay / resume.

### Why

This helps with:
- client reconnects
- Garden restarts
- seed reconnects
- replaying stdout / stderr after a disconnect
- auditability
- future fan-out to multiple consumers

### Event fields

Suggested common envelope:

```json
{
  "id": "evt_123",
  "seq": 44,
  "cursor": "44",
  "type": "command.stdout",
  "sandbox_id": "sbx_123",
  "command_id": "cmd_123",
  "timestamp": "2026-05-08T19:03:12Z",
  "data": {
    "chunk": "running tests...\n"
  }
}
```

### Event types

Sandbox examples:
- `sandbox.provisioning`
- `sandbox.booting`
- `sandbox.ready`
- `sandbox.lease.extended`
- `sandbox.degraded`
- `sandbox.release.requested`
- `sandbox.released`
- `sandbox.failed`

Command examples:
- `command.queued`
- `command.started`
- `command.stdout`
- `command.stderr`
- `command.stdin.accepted`
- `command.cancel.requested`
- `command.kill.requested`
- `command.exit`
- `command.killed`
- `command.failed`

### Replay and resume

`GET /api/v1/sandboxes/:sandbox_id/events?after=42`

`GET /api/v1/sandboxes/:sandbox_id/commands/:command_id/events?after=88`

Response for paginated replay:

```json
{
  "data": [
    {
      "seq": 89,
      "type": "command.stdout",
      "data": {
        "chunk": "ok\n"
      }
    }
  ],
  "next_cursor": "89"
}
```

For live streaming, the same route can support SSE when the client requests:

```http
Accept: text/event-stream
```

SSE event example:

```text
event: command.stdout
id: 89
data: {"seq":89,"type":"command.stdout","data":{"chunk":"ok\n"}}

```

## Lease model

Recommended behavior:
- sandbox gets an initial TTL on acquisition
- Garden extends lease on meaningful session activity
- command activity may also extend lease automatically
- lease extension should be bounded by policy / max lifetime
- idle sandboxes eventually expire and are released

Possible auto-extension triggers:
- command stdout / stderr flowing to active client
- stdin sent by customer
- explicit customer heartbeat
- file operations / future API activity

Suggested policy note:
- treat user-visible session activity as eligible to extend the lease
- do not extend indefinitely without a max cap

## Idempotency and retries

Mutating endpoints should accept an idempotency key, especially:
- `POST /api/v1/sandboxes`
- `POST /api/v1/sandboxes/:sandbox_id/release`
- `POST /api/v1/sandboxes/:sandbox_id/lease`
- `POST /api/v1/sandboxes/:sandbox_id/commands`
- `POST /api/v1/sandboxes/:sandbox_id/commands/:command_id/cancel`
- `POST /api/v1/sandboxes/:sandbox_id/commands/:command_id/kill`

This is important because retries are expected across unreliable networks.

## Error shape

Suggested JSON error envelope:

```json
{
  "error": {
    "code": "sandbox_not_ready",
    "message": "Sandbox sbx_123 is not ready",
    "retryable": true,
    "details": {
      "state": "booting"
    }
  }
}
```

## Suggested next steps

1. add route + controller skeletons for the draft API
2. define JSON payload validation / changesets for request bodies
3. choose event persistence model:
   - append-only database table
   - topic / stream abstraction over DB + pubsub
4. define seed ↔ garden protocol for command lifecycle and output chunks
5. define lease extension policy and max lifetime rules
6. add OpenAPI generation once the payloads settle
