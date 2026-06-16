# AGENTS.md

## Shed: repository intent

Shed is a single-binary Go system for provisioning-facing control and in-compute execution of sandbox workloads.

The runtime model is inspired by HashiCorp Nomad:

- `shed server` owns the control plane, API, state, leases, event replay, client session broker, and embedded operator UI.
- `shed client` runs inside the target compute and owns local command/file execution, process control, workspace enforcement, and event reporting.
- `shed dev` runs both roles locally while faithfully exercising production auth, sessions, heartbeats, leases, commands, events, and UI paths.

The architecture source of truth is `docs/single-binary-architecture.md`.

## What we are optimizing for

### 1. Observability first

Every meaningful lifecycle transition should be visible.

Prefer designs that make it easy to answer:

- what sandbox/allocation was created, when, and why
- what client session connected and with which capabilities
- what command ran, under which session, and with what outcome
- what events were emitted, in what order, and whether they can be replayed
- where failures happened: server, transport, or client

Favor:

- explicit state machines
- durable/replayable event streams
- correlation IDs, request IDs, session IDs, sandbox IDs, command IDs, and event sequences
- resumable streams over ephemeral-only behavior
- clear operator logs and machine-readable events

### 2. Security by default

Assume hostile networks, untrusted workloads, and accidental misuse.

Favor:

- least-privilege boundaries between server and client packages
- explicit authentication and scoped session credentials
- narrow, validated command and file-operation surfaces
- strict workspace-root enforcement in client mode
- cancel/kill semantics that do not leave orphaned process groups behind
- no implicit trust between API, broker, protocol, and local execution

### 3. Auditability as a product feature

Shed should explain what happened after the fact.

Favor:

- append-only event histories where practical
- stable schemas for lifecycle and command events
- idempotent acquire/release/start/cancel flows
- timestamps, actor/source attribution, and durable identifiers
- state that can be reconstructed after disconnects or retries

### 4. Power for downstream consumers

Consumers should build rich automation without reverse-engineering internals.

Favor:

- stable contracts over shortcuts
- capability-oriented API/protocol design
- predictable command lifecycle semantics
- transport-aware but resource-oriented APIs
- forward-compatible protocol evolution

## Working rules for agents

When changing this repository:

- preserve the server/client boundary even though both live in one Go module
- keep `shed dev` production-faithful; do not bypass auth/session/lease/event paths for convenience
- route all state mutations through `internal/store.Store`
- make new behavior observable with logs/events/IDs
- treat security and auditability regressions as correctness bugs
- remove dead code rather than carrying compatibility slop
- document cross-cutting contracts in `docs/single-binary-architecture.md` or nearby package docs

## Local documentation map

- Top-level overview: `README.md`
- Go single-binary architecture: `docs/single-binary-architecture.md`
