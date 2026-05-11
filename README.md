# Garden + Seed

Garden + Seed is a two-part system for orchestrating remote sandbox workloads and controlling them through a unified, bidirectional interface.

## Components

### `garden`
Control plane and API for provisioning and managing sandbox compute.

Garden is responsible for:
- starting workloads in Linux containers and macOS VMs
- tracking sandbox lifecycle and metadata
- issuing actions to running sandboxes through a unified interface
- brokering communication with the in-sandbox agent
- exposing higher-level APIs for automation, task execution, and environment control

### `seed`
In-sandbox daemon written in Go.

Seed is responsible for:
- booting inside each Linux container or macOS VM
- authenticating with Garden using an API key and API URL
- establishing a bidirectional control channel back to Garden
- executing commands and streaming output
- cancelling or killing processes
- searching for, reading, and editing files
- exposing additional host-local capabilities needed for sandbox automation

## How the system fits together

1. Garden provisions a unit of compute.
2. The workload boots with `seed` running inside it.
3. Seed connects back to Garden using its configured credentials.
4. Garden issues actions over the control channel.
5. Seed performs those actions inside the sandbox and streams results back.

This model avoids requiring a direct inbound connection to the container or VM while still allowing rich remote control.

## Goals

- one control model for Linux containers and macOS VMs
- secure, authenticated communication between control plane and agent
- real-time command execution and output streaming
- process lifecycle control
- file system inspection and mutation
- extensible protocol for future sandbox capabilities

## Repository layout

```text
.
├── garden/   # API and control plane
├── seed/     # Go daemon running inside each sandbox
└── spec/     # Shared protocol and API specifications
```

## Spec-driven generation

Garden and Seed now share top-level specs as the source of truth for:
- WebSocket protocol messages
- Garden API / OpenAPI output

Specs live in:
- `spec/protocol/messages.json`
- `spec/api/openapi.json`

Generated code is produced from those specs for both apps.

### Generate code

```bash
mise run generate
```

Or generate only one side:

```bash
mise run generate:protocol
mise run generate:api
```

### Check generated files are up to date

```bash
mise run check:generated
```

This should be run in CI and before committing changes to shared specs.

## Expected configuration

At a minimum, `seed` will be configured with:
- API key
- Garden API / control plane URL

Additional environment-specific configuration will likely be added within each component.

## Development notes

This repository is intentionally split into two components:
- `garden` owns orchestration, lifecycle, APIs, and coordination
- `seed` owns execution inside the target environment

Component-specific setup, architecture, and agent instructions should live in:
- `garden/README.md`
- `garden/AGENT.md`
- `seed/README.md`
- `seed/AGENT.md`

## Future areas to define

- wire protocol / message schema between Garden and Seed
- authentication and key rotation model
- reconnect and heartbeat semantics
- sandbox capability negotiation
- audit logging and observability
- execution isolation and permission model
- artifact upload/download flows
- session multiplexing and concurrency rules
