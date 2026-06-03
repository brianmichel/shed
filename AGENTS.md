# AGENTS.md

## Shed: repository intent

Shed is a two-part system for provisioning sandbox compute and operating it through a secure, observable, and auditable control plane.

- `garden/` is the control plane, API surface, and orchestration layer.
- `seed/` is the in-sandbox daemon that executes work and reports back to Garden.

This repository exists to give downstream consumers a powerful remote execution model **without sacrificing observability, security, or auditability**.

## What we are optimizing for

### 1. Observability first

Every meaningful lifecycle transition should be visible.

Prefer designs that make it easy to answer:

- what sandbox was created, when, and why
- what command ran, under which session, and with what outcome
- what events were emitted, in what order, and whether they can be replayed
- where failures happened: Garden, transport, or Seed

When making changes, favor:

- explicit state machines
- durable event streams
- correlation IDs, request IDs, session IDs, and sandbox IDs
- resumable/replayable streams over ephemeral in-memory-only behavior
- clear operator-facing logs and machine-readable events

### 2. Security by default

The system should assume hostile networks, untrusted workloads, and accidental misuse.

Favor:

- least-privilege boundaries between Garden and Seed
- explicit authentication and scoped credentials
- narrow, validated command and file-operation surfaces
- safe defaults for workspace and process isolation
- avoiding implicit trust between control plane, transport, and sandbox

Security-sensitive changes should preserve or improve:

- tenant/session isolation
- command authorization boundaries
- secret handling
- workspace-root enforcement
- kill/cancel semantics that do not leave orphaned processes behind

### 3. Auditability as a product feature

We are not just executing remote work; we are building a system that can explain what happened afterward.

Favor:

- append-only event histories where practical
- stable schemas for lifecycle and command events
- idempotent APIs for acquire/release/start/cancel flows
- timestamps, actor/source attribution, and durable identifiers
- behavior that can be reconstructed after disconnects or retries

### 4. Power for downstream consumers

Consumers should be able to build rich automation on top of Shed without needing undocumented behavior.

Favor:

- stable contracts over convenience shortcuts
- capability-oriented APIs
- predictable command lifecycle semantics
- transport-agnostic resource models
- forward-compatible protocol evolution

## Sub-project roles

### `garden/`

Garden owns:

- sandbox acquisition and release
- lease/lifecycle management
- API contracts and external integration
- event durability and replay surfaces
- coordination with connected Seed instances

Garden should be the source of truth for:

- resource state
- control-plane policy
- externally visible IDs and event ordering contracts

### `seed/`

Seed owns:

- booting inside the sandbox workload
- authenticating back to Garden
- command execution and process control
- stdout/stderr streaming
- local file and workspace interactions
- host-local capabilities exposed to Garden

Seed should be optimized for:

- reliable execution
- strict workspace scoping
- clean process lifecycle handling
- accurate status/event reporting back to Garden

## Working rules for agents

When changing this repository:

- preserve the Garden/Seed boundary; do not blur control-plane and in-sandbox responsibilities
- prefer explicit protocol messages and state transitions over implicit coupling
- make new behaviors observable with logs/events/IDs
- treat security and auditability regressions as correctness bugs
- document new cross-cutting contracts near the top level or in the owning sub-project

## Directional questions to keep asking

A good change should make it easier to answer:

- Can an operator see what happened?
- Can a customer trust the isolation boundary?
- Can we reconstruct the sequence of events later?
- Can downstream consumers build against this without reverse-engineering internals?

## Local documentation map

- Top-level overview: `README.md`
- Garden details: `garden/README.md`
- Garden prototype scope: `garden/docs/prototype-focus.md`
- Garden-specific agent guidance: `garden/AGENTS.md`

If `seed/` grows its own README or AGENTS document, keep it aligned with the principles above.
