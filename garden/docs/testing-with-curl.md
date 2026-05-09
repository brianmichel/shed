# Testing Garden with curl

Assume Garden runs on `http://localhost:4000`.

## Acquire a sandbox

```bash
curl -sS -X POST http://localhost:4000/api/v1/sandboxes \
  -H 'content-type: application/json' \
  -d '{
    "environment": "linux",
    "template": "default",
    "ttl_ms": 1800000,
    "metadata": {"session_id": "demo"}
  }' | jq
```

## List sandboxes

```bash
curl -sS http://localhost:4000/api/v1/sandboxes | jq
```

## Get a sandbox

```bash
curl -sS http://localhost:4000/api/v1/sandboxes/$SANDBOX_ID | jq
```

## Extend a lease

```bash
curl -sS -X POST http://localhost:4000/api/v1/sandboxes/$SANDBOX_ID/lease \
  -H 'content-type: application/json' \
  -d '{"ttl_ms": 1800000, "reason": "manual_test"}' | jq
```

## Release a sandbox

```bash
curl -sS -X POST http://localhost:4000/api/v1/sandboxes/$SANDBOX_ID/release \
  -H 'content-type: application/json' \
  -d '{"reason": "done"}' | jq
```

## Replay sandbox events

```bash
curl -sS "http://localhost:4000/api/v1/sandboxes/$SANDBOX_ID/events?after=0" | jq
```

## Start a command

```bash
curl -sS -X POST http://localhost:4000/api/v1/sandboxes/$SANDBOX_ID/commands \
  -H 'content-type: application/json' \
  -d '{"command": "echo hello", "stdin": true}' | jq
```

## List commands

```bash
curl -sS http://localhost:4000/api/v1/sandboxes/$SANDBOX_ID/commands | jq
```

## Get a command

```bash
curl -sS http://localhost:4000/api/v1/sandboxes/$SANDBOX_ID/commands/$COMMAND_ID | jq
```

## Send stdin

```bash
curl -sS -X POST http://localhost:4000/api/v1/sandboxes/$SANDBOX_ID/commands/$COMMAND_ID/stdin \
  -H 'content-type: application/json' \
  -d '{"data": "hello from stdin\n", "encoding": "utf-8"}' | jq
```

## Cancel a command

```bash
curl -sS -X POST http://localhost:4000/api/v1/sandboxes/$SANDBOX_ID/commands/$COMMAND_ID/cancel \
  -H 'content-type: application/json' \
  -d '{"grace_period_ms": 100, "escalation": "kill"}' | jq
```

## Kill a command

```bash
curl -sS -X POST http://localhost:4000/api/v1/sandboxes/$SANDBOX_ID/commands/$COMMAND_ID/kill \
  -H 'content-type: application/json' \
  -d '{}' | jq
```

## Replay command events

```bash
curl -sS "http://localhost:4000/api/v1/sandboxes/$SANDBOX_ID/commands/$COMMAND_ID/events?after=0" | jq
```

## List files in `/workspace`

```bash
curl -sS "http://localhost:4000/api/v1/sandboxes/$SANDBOX_ID/files?path=/workspace" | jq
```

## Read a file

```bash
curl -sS "http://localhost:4000/api/v1/sandboxes/$SANDBOX_ID/files/content?path=/workspace/README.txt" | jq
```

## Write a file

```bash
curl -sS -X PUT http://localhost:4000/api/v1/sandboxes/$SANDBOX_ID/files/content \
  -H 'content-type: application/json' \
  -d '{"path": "/workspace/README.txt", "content": "updated from curl\n"}' | jq
```

## Fetch OpenAPI spec

```bash
curl -sS http://localhost:4000/api/openapi.json | jq
```

## SSE sandbox event stream

```bash
curl -N -H 'accept: text/event-stream' \
  http://localhost:4000/api/v1/sandboxes/$SANDBOX_ID/events
```

## SSE command event stream

```bash
curl -N -H 'accept: text/event-stream' \
  http://localhost:4000/api/v1/sandboxes/$SANDBOX_ID/commands/$COMMAND_ID/events
```
