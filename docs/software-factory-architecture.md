# Shed software factory architecture

Shed's software factory direction keeps the existing sandbox control plane as the execution kernel and adds a higher-level product layer for agentic software work.

The target product is a self-hostable system that can receive work from APIs or integrations, provision isolated compute, run a selected agent harness, validate the result, produce artifacts, request approval when needed, and publish changes with a complete audit trail.

## Product boundary

Shed should feel like one integrated product to users:

```text
task in -> isolated agent run -> validated artifact or pull request out
```

Internally, Shed should remain composable:

```text
trigger -> work item -> agent run -> sandbox -> commands/files -> artifacts -> approval -> publish
```

The sandbox layer is not being replaced. It remains the boundary for compute allocation, client sessions, command execution, workspace enforcement, and low-level event capture.

## Core objects

### WorkItem

A `WorkItem` is the user- or integration-facing unit of requested work.

Examples:

- Fix a GitHub issue.
- Review a pull request.
- Investigate a failing CI job.
- Migrate a package across a repository.
- Reproduce and patch a bug report.

Initial states:

- `queued`
- `running`
- `waiting_for_approval`
- `completed`
- `failed`
- `cancelled`

Required audit fields:

- work item ID
- actor
- source type
- source external ID
- correlation ID
- inserted and updated timestamps

### AgentRun

An `AgentRun` is one execution attempt for a work item using a selected harness, policy, and sandbox.

A WorkItem may have multiple AgentRuns, for example after retry, fallback, repair, or human-directed rerun.

Initial states:

- `queued`
- `provisioning`
- `preparing`
- `running`
- `validating`
- `publishing`
- `completed`
- `failed`
- `cancelled`
- `timed_out`

Required audit fields:

- agent run ID
- work item ID
- sandbox ID when allocated
- harness name and version
- model or runtime selection when applicable
- actor
- correlation ID
- inserted, started, and completed timestamps

### Sandbox

A `Sandbox` remains the execution environment managed by Shed's existing control plane.

Responsibilities:

- Compute allocation.
- Client session authentication.
- Workspace root enforcement.
- Command execution.
- File operations.
- Lease lifecycle.
- Low-level event replay.

Factory-level code should use sandboxes through existing server, store, compute, and protocol boundaries rather than bypassing them.

### Command

A `Command` is a low-level process execution in a sandbox.

Agent harnesses, repository preparation, validation, and publication should all use command execution paths when work happens inside a sandbox.

### Artifact

An `Artifact` is a durable output from a WorkItem or AgentRun.

Initial artifact types:

- `diff`
- `patch`
- `summary`
- `validation_log`
- `command_log`
- `commit`
- `pull_request`
- `sarif`
- `external_url`

Artifacts should record:

- artifact ID
- work item ID
- agent run ID
- sandbox ID when applicable
- type
- URI or storage key
- content hash when content is stored by Shed
- metadata
- inserted timestamp

### Approval

An `Approval` is a human decision gate for actions that should not happen automatically under the active policy.

Initial approval reasons:

- Push branch.
- Open pull request.
- Use sensitive secret.
- Run high-risk command.
- Allow unrestricted network access.
- Accept proposed memory.

### Policy

A `Policy` decides what a run may do.

Initial policy dimensions:

- Allowed repositories.
- Allowed harnesses.
- Allowed commands.
- Network mode.
- Secret access.
- Publication rules.
- Required approvals.

Policy decisions must emit events. Security policy failures should be treated as correctness failures, not advisory warnings.

### Memory

`Memory` is durable knowledge extracted from prior runs and injected into future runs.

Initial memory scopes:

- global
- organization
- repository
- workflow
- harness
- user

Memory must be auditable. The system should record when memory is proposed, accepted, rejected, retired, and used.

## Plugin boundaries

Use plugins where the market is fragmented or fast-changing.

### Compute plugins

Compute plugins already exist and remain responsible for turning a logical sandbox into usable compute.

Examples:

- local process/workspace
- Docker
- Kubernetes
- Firecracker or VM
- remote worker service

### Harness plugins

Harness plugins run coding agents or agent-like tools inside prepared sandboxes.

Examples:

- shell harness
- OpenCode
- Claude Code
- Codex
- Aider
- custom organization agent

The initial public harness contract should be `agent.v1`.

Expected methods:

```go
type HarnessV1 interface {
    Info(context.Context) (HarnessInfo, error)
    Prepare(context.Context, PrepareRequest) (PrepareResponse, error)
    Start(context.Context, StartRequest, EventSink) error
    Stdin(context.Context, StdinRequest) (ControlResponse, error)
    Cancel(context.Context, CancelRequest) (ControlResponse, error)
}
```

The exact Go interface can evolve before implementation, but the product contract should remain stable: the server owns lifecycle, audit, policy, and artifacts; the harness owns agent-specific execution behavior.

### Integration plugins

Integration plugins are deferred from the MVP public plugin surface. GitHub should be built in first to prove the domain.

Future integration targets:

- GitHub
- GitLab
- Bitbucket
- Slack
- Linear
- Jira
- generic webhooks

### Artifact storage

Artifact storage should start with a local filesystem implementation and a narrow internal interface. S3/GCS-compatible backends can be added once artifact lifecycle requirements settle.

### Storage

The store interface remains internal. Production should target Postgres first. Memory store remains for dev and tests.

Arbitrary external storage plugins are intentionally not part of the MVP because storage controls transactions, event ordering, idempotency, and recovery.

### Secrets

Secret providers should be pluggable after the core reference model is stable.

Initial providers:

- env/dev provider

Likely future providers:

- Vault
- AWS Secrets Manager
- 1Password
- Doppler

Secrets should be referenced by ID and injected at execution time. Secrets must not be persisted in events, logs, metadata, or artifacts.

## API shape

Existing sandbox APIs remain as lower-level control-plane APIs.

New factory APIs should be added above them:

```text
POST /v1/work-items
GET  /v1/work-items
GET  /v1/work-items/{work_item_id}
POST /v1/work-items/{work_item_id}/cancel

POST /v1/work-items/{work_item_id}/runs
GET  /v1/work-items/{work_item_id}/runs
GET  /v1/agent-runs
GET  /v1/agent-runs/{agent_run_id}
POST /v1/agent-runs/{agent_run_id}/cancel
POST /v1/agent-runs/{agent_run_id}/message
GET  /v1/agent-runs/{agent_run_id}/events

GET  /v1/artifacts/{artifact_id}
GET  /v1/agent-runs/{agent_run_id}/artifacts

GET  /v1/approvals
POST /v1/approvals/{approval_id}/approve
POST /v1/approvals/{approval_id}/deny
```

Factory APIs must support idempotency keys for externally triggered creates.

## Event taxonomy

Events remain append-only and replayable. Factory-level events should include work item and run IDs when available.

Initial event families:

```text
work_item.created
work_item.started
work_item.waiting_for_approval
work_item.completed
work_item.failed
work_item.cancelled

agent_run.created
agent_run.provisioning
agent_run.preparing
agent_run.started
agent_run.message
agent_run.tool.started
agent_run.tool.completed
agent_run.validating
agent_run.publishing
agent_run.completed
agent_run.failed
agent_run.cancelled
agent_run.timed_out

repo.clone.started
repo.clone.succeeded
repo.clone.failed
repo.checkout.succeeded
repo.diff.generated
repo.commit.created
repo.push.succeeded
repo.pr.opened

artifact.created
artifact.updated

approval.requested
approval.granted
approval.denied

policy.allowed
policy.denied

memory.proposed
memory.accepted
memory.rejected
memory.used
memory.retired
```

Event records should include:

- stable ID
- sequence
- timestamp
- source
- actor when applicable
- correlation ID
- work item ID when applicable
- agent run ID when applicable
- sandbox ID when applicable
- command ID when applicable
- type
- JSON data

## Scheduler

The scheduler is responsible for durable AgentRun progression.

MVP scheduler constraints:

- Single-node scheduling first.
- Store-backed run acquisition and locking.
- Recovery after server restart.
- Timeouts and cancellation.
- Retry primitives.
- Step events.

Distributed scheduling should be deferred until there is operational need.

## Repository lifecycle

Repository actions are part of the factory layer but should execute through sandbox command/file paths where possible.

Initial lifecycle:

1. Register repository.
2. Create WorkItem with repository target.
3. Allocate sandbox.
4. Clone and checkout target branch, SHA, or PR.
5. Run harness.
6. Detect dirty state.
7. Generate diff artifact.
8. Run validation.
9. Commit changes if configured.
10. Push and open PR after policy and approval.

Git credentials must use secret references and must not appear in events or command output.

## Validation

Shed should not trust agent success claims.

Validation commands should run through existing sandbox command execution and emit normal command events.

An AgentRun can be marked successful only when required validation passes or policy explicitly allows validation failure.

Optional repair loops may feed validation failures back into the harness with a bounded attempt count.

## Security model

Security and auditability are product requirements.

Required rules:

- Preserve the server/client boundary.
- Route state mutations through `internal/store.Store`.
- Authenticate all customer-facing factory APIs.
- Scope API tokens.
- Verify integration webhook signatures.
- Store raw secrets nowhere except approved secret providers.
- Enforce workspace-root boundaries in client mode.
- Emit policy decision events.
- Require approvals for configured risky actions.
- Avoid bypassing sandbox command paths for convenience.

## UI direction

The UI should present factory concepts first and keep sandbox views as drilldowns.

Initial screens:

- Work item list.
- Agent run detail.
- Live run event stream.
- Artifact browser.
- Diff viewer.
- Approval inbox.
- Repository configuration.
- Sandbox and command drilldown.

The most important MVP screen is AgentRun detail because it proves observability and control.

## MVP definition

MVP is complete when this works end-to-end:

1. User creates a WorkItem from API or GitHub.
2. Shed creates an AgentRun.
3. AgentRun provisions a sandbox.
4. Shed checks out a repository.
5. Selected harness runs in the sandbox.
6. Harness changes files.
7. Shed captures diff and logs as artifacts.
8. Shed runs validation commands.
9. Human approval is requested if configured.
10. Shed opens or prepares a pull request.
11. UI shows complete event history, commands, artifacts, and outcome.
