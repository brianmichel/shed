# Shed single-binary architecture

Shed is one portable Go binary that can run in server, client, or dev mode.

## Goals

- One binary, three modes: `server`, `client`, and `dev`.
- Preserve the control-plane vs in-compute execution boundary inside the Go codebase.
- Provide a stable API for the core journey: acquire/register compute, run a command, observe ordered events, release.
- Use in-memory storage first behind interfaces that can later be backed by SQL.
- Serve a minimal operator UI from the binary itself.
- Keep dev mode production-faithful: no shortcuts around auth, sessions, leases, protocol, events, or UI.

## Runtime modes

### `shed server`

Runs only control-plane responsibilities:

- HTTP API.
- Client WebSocket broker.
- Session issuance and authentication.
- Sandbox/allocation, command, lease, and event state machines.
- Lease sweeper.
- Replayable event streams.
- Embedded operator UI under `/ui/`.
- Health and diagnostics endpoints.

The server delegates compute provisioning and optional direct command execution to a compute driver. Drivers may be built in, such as the local workspace/process driver, or external HashiCorp go-plugin processes. The server still treats client WebSocket registration as the preferred readiness source of truth when a driver provisions `shed client`.

### `shed client`

Runs inside the target compute environment. It assumes the VM/container/host already exists and is scoped by its workspace root.

Responsibilities:

- Authenticate to a server using a session key, session ID, and sandbox/allocation ID.
- Register platform, hostname, process, workspace, and capabilities.
- Heartbeat and reconnect/resume.
- Execute commands and stream stdout/stderr/exit events.
- Handle stdin, cancel, and kill.
- Perform scoped file operations in later iterations.

### `shed dev`

Starts server and client on one machine using the same production code paths. By default it listens on `127.0.0.1:6464`.

1. Start server components and HTTP listener.
2. Register the built-in local workspace/process compute driver.
3. Create a sandbox/session through server state paths.
4. Allocate through the same compute manager used by server mode.
5. Start a client with generated credentials.
6. Connect over the same WebSocket protocol.
7. Use the same heartbeat, lease, command, event, release, and UI paths as production.

## Package layout

```text
cmd/shed              CLI dispatch
internal/api          HTTP request/response helpers and future OpenAPI specs
internal/client       In-compute client runtime and command runner
internal/dev          Production-faithful local server+client orchestration
internal/model        Shared resource types and state constants
internal/protocol     Server/client envelope and message validation
internal/server       HTTP server, broker, leases, events, compute wiring, and UI wiring
internal/store        Store interfaces and in-memory implementation
internal/ui           Embedded frontend assets and UI handler
internal/compute      Versioned compute driver interfaces, manager, and go-plugin adapter
pkg/compute           Public SDK aliases for external compute plugin authors
```

## Public API shape

Core endpoints:

- `GET /v1/health`
- `GET /v1/compute/drivers` — list registered compute drivers, server-side configuration, and plugin metadata.
- `POST /v1/sandboxes` — create a logical sandbox/allocation, issue client credentials, and call the selected compute driver.
- `GET /v1/sandboxes`
- `GET /v1/sandboxes/{sandbox_id}`
- `POST /v1/sandboxes/{sandbox_id}/release`
- `POST /v1/sandboxes/{sandbox_id}/lease`
- `GET /v1/sandboxes/{sandbox_id}/events?after=N`
- `GET /v1/sandboxes/{sandbox_id}/files?path=/workspace`
- `GET /v1/sandboxes/{sandbox_id}/files/content?path=/workspace/file.txt`
- `PUT /v1/sandboxes/{sandbox_id}/files/content`
- `POST /v1/sandboxes/{sandbox_id}/commands`
- `GET /v1/sandboxes/{sandbox_id}/commands`
- `GET /v1/sandboxes/{sandbox_id}/commands/{command_id}`
- `POST /v1/sandboxes/{sandbox_id}/commands/{command_id}/stdin`
- `POST /v1/sandboxes/{sandbox_id}/commands/{command_id}/cancel`
- `POST /v1/sandboxes/{sandbox_id}/commands/{command_id}/kill`
- `GET /v1/sandboxes/{sandbox_id}/commands/{command_id}/events?after=N`
- `GET /v1/client/connect` — WebSocket endpoint for `shed client`.

All JSON errors use a stable machine-readable shape:

```json
{"error":{"code":"sandbox_not_found","message":"Sandbox not found","retryable":false}}
```

## State model

### Sandbox/allocation states

- `pending_client` — created and awaiting client connection.
- `ready` — client registered and command-capable.
- `degraded` — client disconnected before release.
- `releasing` — release requested.
- `released` — terminal successful release.
- `failed` — terminal failed setup or protocol state.

### Client session states

- `issued` — credentials created.
- `connected` — WebSocket connected.
- `registered` — client registered capabilities.
- `disconnected` — transport lost.
- `expired` — lease/session expired.
- `closed` — terminal after release.

### Command states

- `queued`
- `starting`
- `running`
- `cancelling`
- `exited`
- `killed`
- `failed`

### Compute lifecycle

Each sandbox records compute driver identity, compute API version, external compute/allocation ID, compute config, and compute metadata. `POST /v1/sandboxes` persists the sandbox/session first, then calls compute `Allocate`. If allocation fails, Shed emits a compute failure event and marks the sandbox `failed`. The sandbox becomes `ready` when a provisioned `shed client` connects and registers, but drivers may also advertise direct `exec` support for command dispatch without a connected client.

Compute plugins implement the versioned `compute.v1` lifecycle:

- `Info` — plugin name/version/API-version/capability negotiation.
- `Allocate` — create or attach to compute and start/provision `shed client`.
- `Status` — report compute-side health/status without replacing client heartbeat readiness.
- `Renew` — observe/perform compute-specific lease extension work.
- `Release` — clean up compute-owned resources.
- `Exec` — execute an API-created command and stream standard `command.*` events.
- `Stdin`, `Cancel`, `Kill` — command control operations for plugin-executed commands.

External compute drivers run as isolated child processes through HashiCorp go-plugin/gRPC. See [`compute-plugins.md`](compute-plugins.md).

### Lease lifecycle

Each sandbox has a lease with TTL and expiry. Activity may extend the lease up to policy limits. Lease extension calls compute `Renew`. A server sweeper emits expiration events, calls compute `Release`, and releases expired sandboxes.

### Event model

Events are append-only per sandbox and per command. Each event has:

- stable `id`
- `sandbox_id`
- optional `command_id`
- monotonic `seq`
- `type`
- RFC3339 `timestamp`
- JSON `data`

Cursors are sequence numbers. `after=N` returns events with `seq > N`.

## Store design

`internal/store.Store` is the boundary for all mutations and reads. The initial implementation is `MemoryStore`, protected by a mutex and suitable for dev/prototype use.

Future SQL stores should preserve:

- transactional command/sandbox state updates with event append.
- uniqueness for IDs and idempotency keys.
- ordered event queries by `(sandbox_id, seq)` and `(sandbox_id, command_id, seq)`.
- session key lookup without exposing raw keys in broad list operations.

## Server/client protocol

The protocol envelope:

```json
{
  "version":"1",
  "type":"command.start",
  "message_id":"msg_...",
  "request_id":"req_...",
  "session_id":"sess_...",
  "sandbox_id":"sbx_...",
  "seq":1,
  "timestamp":"2026-01-01T00:00:00Z",
  "expects_ack":false,
  "reply_to":"msg_...",
  "payload":{}
}
```

Required protocol behaviors:

- monotonic sequence per sender/session.
- duplicate message detection by `message_id`.
- hello/register/status/heartbeat handshake.
- explicit ack support for messages that request it.
- reconnect/resume support using last seen sequence.
- capability negotiation for commands, files, pty, ports, and future features.

## Embedded frontend

Shed includes a minimal UI in the Go binary:

- API routes and UI routes live on one HTTP server.
- UI is mounted at `/ui/`.
- `/` redirects to `/ui/`.
- deep SPA paths fall back to `index.html`.
- Content Security Policy is set for UI responses.
- disabled UI builds show a clear stub page.

The implementation uses native `embed.FS`.

Initial screens:

- overview / health.
- sandboxes and connected clients.
- command list and command detail/log output.
- raw event stream/cursor view.
- basic diagnostics.

## Tooling

Root `mise.toml` is the developer entrypoint for:

- Go version pinning.
- formatting.
- tests.
- builds.
- running server/client/dev modes.
- UI asset build/embed checks.
