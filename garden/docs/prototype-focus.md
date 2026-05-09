# Prototype focus: what matters vs what can wait

This document proposes a simpler mental model for the Garden prototype.

## Core user journey (must keep)

1. Acquire a sandbox.
2. Run a command in that sandbox.
3. Observe command output and exit status.
4. Release the sandbox.

If a feature does not strengthen one of those four steps, it is probably optional for prototype phase.

## Thin-slice architecture (must keep)

Keep only these stable boundaries:

- **HTTP API** (`GardenWeb.Api.V1`) as the single client entrypoint.
- **Sandbox backend behaviour** (`Garden.SandboxBackend`) with one concrete backend enabled at a time.
- **In-memory stores** for sandboxes / commands / sessions.
- **Durable event stream endpoint shape** (even if backed by simple append-only storage).

Everything else should be judged by whether it helps verify product risk quickly.

## Candidate cuts right now

### 1) Dual guardrail namespaces

You currently have both:

- `Garden.Guardrails.*`
- `Garden.LocalSandbox.Guardrails.*`

For prototype speed, collapse to one namespace (prefer `Garden.Guardrails.*`) and keep one active OS implementation plus a safe default.

### 2) Parallel runtime paths

Both mock compute and localhost runtime are supervised at app start. For prototype simplicity:

- gate with config and run **one runtime mode per boot** (`:mock` or `:local_host`), not both.
- keep the other implementation in code but unsupervised.

This reduces moving parts and debugging ambiguity.

### 3) Optional channels/live surfaces

You expose API + LiveViews + channels. If external integration is API-first, keep UI/channel surfaces minimal:

- retain only one operator screen (or none).
- defer non-critical realtime channel features when SSE/event replay already covers observability.

### 4) Persistence breadth

You already have multiple record schemas for durable history. During prototyping, consider a narrower persistence contract:

- persist only sandbox lifecycle + command lifecycle + output chunks needed for replay.
- defer rich metadata/event taxonomies until query needs become real.

### 5) Dependency surface

Some dependencies are convenience/production-facing (dashboard, mailer, clustering). Keep if actively used in prototype demos; otherwise defer wiring/usage complexity.

## Non-negotiables (do not cut)

- idempotent sandbox acquire/release semantics.
- explicit command state machine (`queued` -> `running` -> terminal states).
- replayable event stream with stable ordering per resource.
- consistent error schema for API clients.

These are the contract pieces that are expensive to change later.

## Suggested next refactor order

1. Choose one runtime mode by config and stop supervising the other.
2. Unify guardrails namespace.
3. Reduce UI/channel scope to minimum needed for demos.
4. Trim persistence fields to replay-critical data only.
5. Re-run tests and update API docs with the simplified contract.
