# AGENTS.md

## Seed mission

Seed is the Shed in-sandbox daemon.

It is responsible for:

- booting inside a provisioned sandbox
- authenticating back to Garden
- receiving control-plane instructions
- executing commands and streaming results
- enforcing local workspace and process boundaries
- exposing host-local capabilities safely

Seed should be small, reliable, and explicit.

## What Seed optimizes for

### Observability

Seed is the source of truth for what actually happened inside the sandbox.

Favor:

- accurate command lifecycle reporting
- stdout/stderr delivery with stable sequencing
- heartbeat and status updates that reflect real sandbox state
- logs that distinguish transport issues from execution issues
- explicit acknowledgements for accepted work

### Security

Seed runs closest to untrusted workloads and must behave defensively.

Favor:

- strict workspace-root enforcement
- narrow execution and file-operation surfaces
- rejecting implicit path trust
- careful environment propagation and secret handling
- clean cancel/kill behavior that avoids orphaned processes

### Auditability

Seed should emit enough structured information for Garden to reconstruct execution later.

Favor:

- stable message shapes
- timestamps and command IDs on every meaningful event
- deterministic reporting for start, output, exit, cancel, and kill flows
- behavior that remains intelligible across reconnects or retries

### Downstream power

Seed enables downstream capabilities by being dependable and protocol-driven.

Favor:

- explicit capabilities over hidden side effects
- predictable execution semantics
- transport messages that can evolve without ambiguity
- local behavior that matches Garden's external contract

## Seed owns

Seed is the execution authority for:

- process spawning and termination inside the sandbox
- command stdin/stdout/stderr handling
- workspace path resolution
- local environment shaping for subprocesses
- sandbox-local capability implementation

Garden remains the control-plane source of truth; Seed should not invent external policy.

## Working rules for agents in `seed/`

When changing Seed:

- preserve the distinction between transport, protocol handling, and command execution
- make every new capability explicit in the protocol and event stream
- default to safer workspace and process behavior
- treat execution/reporting mismatches as serious bugs
- keep the implementation understandable under failure, reconnect, and cancellation conditions

## Implementation guidance

- Keep command lifecycle transitions explicit and easy to reason about
- Prefer small, composable packages with narrow responsibilities
- Ensure new filesystem behavior is scoped to the sandbox workspace unless intentionally broader
- Be careful with process groups, signal handling, and child cleanup
- Keep message payloads stable and machine-readable

## Local references

- Repository principles: `../AGENTS.md`
- Repository overview: `../README.md`
- Seed entrypoint: `main.go`
- Connection/session logic: `internal/connection/connection.go`
- Command execution: `internal/runner/runner.go`
- Transport: `internal/transport/transport.go`
