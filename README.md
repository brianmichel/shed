# Shed

Shed is a single portable Go binary for controlling sandbox compute and running commands inside it.

Runtime modes:

- `shed server` — control plane, API, client session broker, lease/event state, and embedded operator UI.
- `shed client` — in-compute worker that connects to a server, registers capabilities, executes commands, and streams events.
- `shed dev` — production-faithful local mode that runs server and client together while exercising real auth, sessions, heartbeats, leases, commands, events, and UI paths.

See [`docs/single-binary-architecture.md`](docs/single-binary-architecture.md) for the architecture.

## Quick start

```sh
mise run test
mise run build
mise run run:dev
```

Then open `http://127.0.0.1:6464/ui/` or use the API under `http://127.0.0.1:6464/v1`.

## Current status

The root Go module contains the single-binary foundation:

- CLI for `server`, `client`, and `dev`.
- In-memory store behind interfaces.
- Versioned compute driver interface with HashiCorp go-plugin/gRPC adapter.
- Built-in local workspace/process compute driver.
- Server/client WebSocket protocol envelope.
- Command execution, stdout/stderr events, stdin/cancel/kill dispatch path.
- Replayable JSON/SSE event endpoints.
- Embedded minimal UI served from the binary.

## Core journey

1. Server creates a logical sandbox/allocation and issues client credentials.
2. A compute driver allocates local or remote compute and may provision `shed client`.
3. Server marks the sandbox ready after client registration, when a client path exists.
4. API/UI starts commands and dispatches them to the client or to a compute driver that advertises direct exec support.
5. The execution path streams command events.
6. Consumers replay ordered events by cursor.
7. Releasing the sandbox calls compute cleanup, closes the client path, and marks state terminal.

## Tooling

Root `mise.toml` pins Go and defines repeatable tasks:

- `mise run fmt`
- `mise run test`
- `mise run build`
- `mise run run:server`
- `mise run run:client`
- `mise run run:dev` — run dev mode with Air hot reload
- `mise run run:dev:once` — run dev mode once without hot reload
- `mise run ui:check`

## Repository layout

```text
cmd/shed/          CLI entrypoint
internal/api/      JSON response/error helpers
internal/compute/  compute driver interfaces, manager, and plugin adapter
internal/client/   in-compute client runtime
internal/dev/       local production-faithful orchestration
internal/model/     shared resource models
internal/protocol/  protocol envelope
internal/server/    control plane and broker
internal/store/     store interfaces and memory implementation
internal/ui/        embedded frontend
pkg/compute/       public compute plugin SDK aliases
```
