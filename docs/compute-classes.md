# Compute classes

Shed users request operator-defined compute classes rather than provider-specific jobs or language-specific development environments. A class describes the shape of compute that should be provisioned: operating system, architecture, accelerator shape, and bounded resource knobs.

Examples:

- `linux-x86_64`
- `linux-arm64`
- `linux-x86_64-gpu`
- `darwin-arm64`
- `windows-x86_64`

## Naming model

Keep these concepts separate:

- **Driver** — the implementation mechanism, such as `local`, `nomad`, `daytona`, or `ssh`.
- **Compute class** — the user-facing compute shape, such as `linux-arm64`.
- **Allocation** — the concrete sandbox instance, such as a Shed sandbox ID and a provider allocation ID.

Operators define classes. Users select a class and provide validated parameters.

## Create API

```json
POST /v1/sandboxes
{
  "compute_class": "linux-arm64",
  "parameters": {
    "cpu": 2000,
    "memory_mb": 4096
  },
  "metadata": {
    "purpose": "agent-task"
  }
}
```

Shed resolves the class to a driver and driver configuration, validates parameters, creates a sandbox/session record, and calls the selected compute driver.

Lower-level `compute_driver` and `compute_config` fields still exist for direct/admin usage, but compute classes are the preferred public contract.

## Discovery API

```http
GET /v1/compute/classes
```

Returns the configured classes, their driver, descriptions, capabilities, parameter schema, defaults, and operator-controlled driver config.

## Server config

`shed server -config shed.json` loads JSON configuration. The current file format is intentionally small and stdlib-only:

```json
{
  "compute": {
    "default_driver": "nomad",
    "plugins": {
      "nomad": {
        "command": "/usr/local/bin/shed-compute-nomad",
        "args": [],
        "env": {
          "NOMAD_ADDR": "https://nomad.service.consul:4646"
        }
      }
    }
  },
  "compute_classes": [
    {
      "name": "linux-arm64",
      "driver": "nomad",
      "description": "Linux ARM64 sandbox",
      "capabilities": {
        "os": "linux",
        "arch": "arm64",
        "gpu": false,
        "exec": true,
        "files": true
      },
      "defaults": {
        "ttl_ms": 3600000
      },
      "parameters_schema": {
        "type": "object",
        "required": ["cpu"],
        "properties": {
          "cpu": {"type": "integer", "default": 1000},
          "memory_mb": {"type": "integer", "default": 2048}
        }
      },
      "driver_config": {
        "job_template": "/etc/shed/jobs/linux-arm64.nomad.hcl",
        "node_pool": "arm64"
      }
    }
  ]
}
```

The built-in local driver registers a default `local` class automatically.

## Parameter validation

Shed currently supports a small JSON-schema-shaped subset for class parameters:

- `properties`
- `required`
- primitive `type`: `string`, `boolean`, `number`, `integer`
- `default`

Unknown parameters are rejected when a class declares `properties`. This keeps provider-specific details behind the operator boundary and avoids letting callers submit arbitrary Nomad jobs by default.

## Nomad direction

A Nomad compute plugin should consume resolved class config rather than arbitrary user-submitted job specs. The operator owns job templates, node pools, image policy, resource bounds, volume policy, and secrets. The caller supplies only validated parameters. The plugin renders/submits a Nomad job, starts or provisions `shed client`, and returns Nomad identifiers in compute metadata.
