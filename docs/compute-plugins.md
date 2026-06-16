# Compute plugins

Shed compute plugins are responsible for turning a logical sandbox record into usable compute. They may allocate a local workspace/process, a container, an SSH host, a VM, or a remote service. Shed uses a Nomad/HashiCorp-style external plugin model: third-party compute code runs in a separate process and communicates with `shed server` over HashiCorp go-plugin using gRPC.

## Lifecycle

1. `POST /v1/sandboxes` creates a sandbox record and client session in the store.
2. Shed calls the selected compute driver's `Allocate` method with:
   - sandbox/session IDs
   - the session key
   - the client WebSocket `connect_url`
   - environment/template fields
   - lease expiry/TTL
   - compute-specific config
3. The compute driver creates or finds a compute resource, prepares the workspace, and usually starts/provisions `shed client` with the supplied credentials.
4. The sandbox becomes `ready` when the client connects and registers through the existing WebSocket protocol. Drivers may also advertise direct `exec` support for computes where Shed should dispatch API commands to the plugin rather than a connected client.
5. Shed calls `Status` on sandbox reads, `Renew` when leases are extended, and `Release` for API releases or lease expiry.

Compute operations are emitted into the sandbox event stream, including `compute.allocate.started`, `compute.allocate.succeeded`, `compute.allocate.failed`, `compute.status`, `compute.renewed`, `compute.exec.started`, `compute.exec.completed`, and `compute.release.succeeded`.

## Versioning

The first compute contract is `compute.v1`. Plugins advertise supported API versions through `Info`:

```go
type PluginInfo struct {
    Name         string
    Version      string
    APIVersions  []string // include "compute.v1"
    Capabilities map[string]bool
}
```

Shed rejects plugins that do not advertise the requested API version. Future API versions should coexist with `compute.v1` instead of changing it in place.

## Go plugin skeleton

Plugin authors should import the public SDK package:

```go
package main

import (
    "context"

    shedcompute "github.com/brianmichel/shed/pkg/compute"
)

type myDriver struct{}

func (myDriver) Info(context.Context) (shedcompute.PluginInfo, error) {
    return shedcompute.PluginInfo{
        Name:        "example",
        Version:     "0.1.0",
        APIVersions: []string{shedcompute.APIVersionV1},
        Capabilities: map[string]bool{"remote": true, "status": true, "renew": true, "release": true, "exec": true},
    }, nil
}

func (myDriver) Allocate(ctx context.Context, req shedcompute.AllocateRequest) (shedcompute.AllocateResponse, error) {
    // Create compute and start/provision shed client with req.ConnectURL,
    // req.SessionID, req.SessionKey, and req.SandboxID.
    return shedcompute.AllocateResponse{
        ExternalID:    "provider-allocation-id",
        APIVersion:    shedcompute.APIVersionV1,
        PluginName:    "example",
        PluginVersion: "0.1.0",
        Metadata:      map[string]string{"region": "local"},
    }, nil
}

func (myDriver) Status(context.Context, shedcompute.StatusRequest) (shedcompute.StatusResponse, error) {
    return shedcompute.StatusResponse{State: "running"}, nil
}
func (myDriver) Renew(context.Context, shedcompute.RenewRequest) (shedcompute.RenewResponse, error) {
    return shedcompute.RenewResponse{}, nil
}
func (myDriver) Release(context.Context, shedcompute.ReleaseRequest) (shedcompute.ReleaseResponse, error) {
    return shedcompute.ReleaseResponse{Released: true}, nil
}

func (myDriver) Exec(ctx context.Context, req shedcompute.ExecRequest, sink shedcompute.ExecEventSink) error {
    // Execute req.Command on the compute and stream standard command events.
    _ = sink(shedcompute.ExecEvent{CommandID: req.CommandID, Type: "command.started", Data: map[string]any{"command_id": req.CommandID}})
    return sink(shedcompute.ExecEvent{CommandID: req.CommandID, Type: "command.exit", Data: map[string]any{"command_id": req.CommandID, "exit_code": 0}})
}
func (myDriver) Stdin(context.Context, shedcompute.ExecStdinRequest) (shedcompute.ExecControlResponse, error) {
    return shedcompute.ExecControlResponse{Accepted: true}, nil
}
func (myDriver) Cancel(context.Context, shedcompute.ExecSignalRequest) (shedcompute.ExecControlResponse, error) {
    return shedcompute.ExecControlResponse{Accepted: true}, nil
}
func (myDriver) Kill(context.Context, shedcompute.ExecSignalRequest) (shedcompute.ExecControlResponse, error) {
    return shedcompute.ExecControlResponse{Accepted: true}, nil
}

func main() {
    shedcompute.ServePlugin(myDriver{})
}
```

Build it as a normal executable, not a Go `.so` plugin.

## Operational configuration

`shed server` supports:

```sh
shed server \
  -compute-driver=my-remote \
  -compute-plugin my-remote=/usr/local/bin/shed-compute-my-remote
```

Multiple `-compute-plugin name=/path` flags may be supplied. `SHED_COMPUTE_PLUGINS` accepts a comma-separated list using the same `name=/path` form. `shed dev` supports the same `-compute-driver` and `-compute-plugin` flags.

The built-in `local` compute is always registered by `shed server` and `shed dev`. It creates a workspace under `-compute-workspace-root` in server mode, or under `-workspace-root` in dev mode, and starts an in-process `shed client` using the normal session/WebSocket path.

## Security boundaries

Compute plugins are treated as untrusted:

- Shed launches plugins as child processes instead of loading code into the server process.
- Plugins receive a restricted environment, not the full host environment.
- Shed invokes plugins directly without shell interpolation.
- Startup and per-call timeouts bound plugin hangs.
- The manager kills plugin processes when Shed shuts down.
- All lifecycle calls emit structured events.

Process isolation is not a complete sandbox. Operators should run Shed and plugin executables with appropriate OS/container sandboxing, filesystem permissions, network policy, and secret scoping for their deployment.
