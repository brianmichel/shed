# Plugin System Plan

## Context

We want to design a Nomad-like plugin system for Shed so third-party Go modules can describe how sandboxes are allocated at runtime. Sandboxes may be local or remote and may be created using different allocation strategies.

Initial repository scan:
- Shed is a single Go binary with `server`, `client`, and `dev` modes.
- Current core journey: server creates a logical sandbox/allocation, issues client credentials, client connects/registers, then commands run through the client.
- The codebase is small and organized under `internal/{server,client,dev,model,store,protocol}`.

## Approach

Use a HashiCorp/Nomad-style **external plugin process over RPC** model rather than Go `.so` plugins. Shed should never load third-party compute code into the server process. Plugins are configured as executable commands, launched/supervised by Shed, and communicate through a versioned RPC protocol, preferably using `github.com/hashicorp/go-plugin` with a gRPC transport.

Treat sandbox creation as two phases:
1. **Persist intent and issue credentials** via `store.CreateSandbox`, producing the canonical `model.Sandbox` and `model.ClientSession`.
2. **Delegate allocation lifecycle** to the selected compute plugin. The plugin receives `sandbox_id`, `session_id`, `agent_token`, `connect_url`, lease information, template/environment, and plugin-specific config so it can create local/remote compute, prepare a workspace, start/provision `shed client`, perform health checks, renew/interpret lease policy, and clean up on release.

Core design points:
- Add a small versioned compute API with explicit protocol and capability negotiation. The first stable contract should be `compute.v1`; future versions can coexist.
- Add an compute manager in the server process that resolves compute names, launches external plugin processes, performs handshake/version checks, calls lifecycle methods, enforces timeouts, records events, and terminates plugin processes on shutdown.
- Keep `internal/store.Store` as the state boundary; plugin orchestration lives above it in server code. Store types only gain compute selection/config/result metadata needed for auditability.
- Preserve the current client/session WebSocket handshake as the stable server-to-sandbox execution contract. A plugin allocates compute, but a sandbox is not command-ready until the `shed client` registers and the server marks it `ready`.
- Assume plugins are untrusted: execute them out-of-process with restricted inherited environment, no shell interpolation, configured executable allowlists/paths, per-call deadlines, process kill on cancellation, bounded RPC payloads, and audit events for every plugin call. Document that stronger isolation may require running Shed/plugin workers under OS/container sandboxing.
- Replace `server.Config.OnSandboxCreated` with a real compute manager. Dev mode should use the same manager and register a built-in local workspace/process compute.

Initial versioned Go interface shape to implement/adapt through RPC:

```go
type PluginInfo struct {
    Name              string
    Version           string
    APIVersions       []string // e.g. ["compute.v1"]
    Capabilities      map[string]bool // health, renew, release, remote, local
}

type ComputeV1 interface {
    Info(context.Context) (PluginInfo, error)
    Allocate(context.Context, AllocateRequest) (AllocateResponse, error)
    Status(context.Context, StatusRequest) (StatusResponse, error)
    Renew(context.Context, RenewRequest) (RenewResponse, error)
    Release(context.Context, ReleaseRequest) (ReleaseResponse, error)
}
```

The concrete RPC DTOs should use JSON-compatible fields first (`map[string]string`, `map[string]any` only at API boundaries if unavoidable) so external plugin authors have a stable schema.

## Files to modify

Likely candidates:
- `go.mod` / `go.sum` — add HashiCorp plugin/RPC dependency.
- `internal/model/model.go` — add compute identity/config/result fields, e.g. compute name, compute version/API version, external allocation ID, allocation metadata, and last compute status if needed.
- `internal/store/store.go` — extend `SandboxCreate` for compute selection, template/environment parameters, and plugin config; potentially add a method to persist compute results/status without bypassing state events.
- `internal/store/memory.go` — persist new compute fields and emit/update allocation metadata.
- `internal/server/server.go` — route create/status/release/lease-expiry paths through the compute manager; emit plugin lifecycle events; remove or retire `OnSandboxCreated`.
- `internal/dev/dev.go` — use the compute manager and built-in local compute instead of directly calling `startClient` from a callback.
- `cmd/shed/main.go` — expose config/flags/env for plugin executable paths/directories, default compute, plugin startup timeout, and local compute settings.
- New `internal/compute` package — versioned interfaces, DTOs, registry, manager, plugin client wrapper, built-in local compute, and tests.
- New `internal/compute/hashiplugin` or similar package — HashiCorp go-plugin handshake, gRPC broker/client/server shims, and protocol version constants.
- `docs/single-binary-architecture.md` and new `docs/sandbox-compute-plugins.md` — document lifecycle, security model, versioning, plugin authoring, and operational config.

## Reuse

Existing code and patterns to preserve:
- `store.CreateSandbox` already creates the canonical `model.Sandbox` + `model.ClientSession` pair and emits `sandbox.pending_client`.
- `server.createSandbox` already has the API entry point and returns `connect_url` with session credentials.
- `server.Config.OnSandboxCreated` is an existing proof-of-concept allocation hook used by dev mode; use it as evidence for the extension point, but replace it with the compute manager.
- `dev.startClient` already demonstrates the built-in local workspace/process compute behavior: prepare workspace, instantiate `client.New`, and run it with generated credentials.
- Client registration (`seed.hello` -> `seed.register`) in `internal/server/server.go` is already the boundary that marks a sandbox `ready`.
- Lease sweeper and release paths already centralize terminal lifecycle changes and can call compute cleanup.
- Existing event stream model can audit compute operations with new event types such as `compute.allocate.started`, `compute.allocate.succeeded`, `compute.allocate.failed`, `compute.status`, `compute.renewed`, and `compute.release.succeeded`.

## Steps

- [x] Identify current sandbox/allocation lifecycle and extension points.
- [x] Confirm runtime loading model and plugin process/security expectations: external HashiCorp-style RPC plugins, treated as untrusted and isolated out-of-process.
- [x] Add `internal/compute` with versioned DTOs and `ComputeV1` lifecycle interface (`Info`, `Allocate`, `Status`, `Renew`, `Release`).
- [x] Add HashiCorp go-plugin/gRPC adapter and handshake constants for `compute.v1`, including plugin name/version/API-version negotiation.
- [x] Add compute manager/registry that supports built-in computes and external executable plugins, with startup/call timeouts, process supervision, restricted env, and structured lifecycle events.
- [x] Extend sandbox create API/model/store fields for compute selection, compute config, plugin API version, external allocation ID, and compute metadata.
- [x] Wire `POST /v1/sandboxes` through the compute manager after `store.CreateSandbox`; on allocation failure, emit an event and mark sandbox `failed`.
- [x] Wire release and lease expiry through compute `Release`; call `Renew`/`Status` where appropriate without replacing the existing client heartbeat readiness model.
- [x] Implement the built-in local workspace/process compute as the example compute, reusing current `dev.startClient` logic and keeping auth/session/WebSocket paths intact.
- [x] Update `shed server` and `shed dev` flags/config for default compute and external plugin executable configuration.
- [x] Add unit tests for manager lifecycle, version mismatch, allocate failure -> failed sandbox, release cleanup, and local compute behavior.
- [x] Document plugin authoring, versioning, security boundaries, lifecycle semantics, example local compute, and operational configuration.

## Verification

Planned checks:
- `go test ./...`
- `mise run fmt`
- `mise run build`
- Unit test fake external plugin: manager launches plugin, negotiates `compute.v1`, calls `Allocate`, `Status`, `Renew`, and `Release`, and kills the process on shutdown.
- Unit test version mismatch: unsupported plugin API version is rejected with an audit event and no in-process loading.
- Unit test allocation failure: sandbox emits compute failure event and transitions to `failed`.
- Manual dev flow: `mise run run:dev:once`, create/release a sandbox, verify built-in local compute starts `shed client` and cleanup runs.
- API-level create flow: create sandbox with default compute, verify events progress `sandbox.pending_client` -> `compute.allocate.succeeded` -> `client.connected` -> `sandbox.ready`.
- Release/expiry flow: verify compute `Release` is called and sandbox ends in `released`.
