# Software factory migration plan

This plan migrates Shed from a sandbox-focused control plane into a self-hostable software factory while preserving the sandbox layer as the execution kernel.

## Assumptions

- Product direction: one integrated factory product, built from composable plugin-backed internals.
- Existing sandbox, command, client, compute, lease, and event paths remain foundational.
- First production storage target is Postgres, with the memory store retained for dev and tests.
- First factory wedge: submit work against a repository, run an agent in a sandbox, produce an auditable patch or pull request.
- Storage remains an internal implementation boundary at first. Do not expose arbitrary external storage plugins until there is concrete demand.

## State values

| State | Meaning |
|---|---|
| `NOT_STARTED` | Ready to scope or assign |
| `IN_PROGRESS` | Actively being worked |
| `BLOCKED` | Waiting on decision or dependency |
| `DONE` | Complete and verified |
| `DEFERRED` | Explicitly postponed |

## Product shape

Shed should feel like one integrated product to users and like composable pieces internally.

The user-facing product should answer:

> Give Shed a task against a repository, run the selected agent safely, validate the result, produce artifacts, and show the full audit trail.

The internal architecture should preserve plugin seams where the market is fragmented or fast-changing:

- Compute backends.
- Agent harnesses.
- Integrations.
- Artifact storage.
- Secret providers.
- Policy engines.
- Memory retrieval.

The core platform should not be a loose toolkit. Work items, agent runs, sandboxes, commands, events, artifacts, approvals, and policy decisions should remain stable first-class concepts.

## Thin waist

The platform flow should converge on this stable path:

```text
Trigger -> WorkItem -> AgentRun -> Sandbox -> Commands/Files -> Artifacts -> Approval -> Publish
```

Core objects:

- `WorkItem`: what the user or integration asked Shed to do.
- `AgentRun`: one execution attempt by a selected harness/model/policy inside one or more sandboxes.
- `Sandbox`: the isolated execution environment.
- `Command`: a low-level process execution inside a sandbox.
- `Event`: append-only, replayable audit stream.
- `Artifact`: output from a run, such as diff, patch, summary, log bundle, test report, commit, or pull request.
- `Approval`: human decision point for risky actions or publishing.
- `Policy`: rule set that decides what a run may do.
- `Memory`: durable knowledge extracted from prior runs and injected into future runs.

## Phase 0: Direction lock

| ID | State | Depends On | Task |
|---|---|---|---|
| `SF-000` | `DONE` | None | Decide product positioning: self-hostable software factory for safe, auditable coding agents. |
| `SF-001` | `DONE` | `SF-000` | Define MVP promise: API or GitHub task in, isolated agent run, diff or PR artifacts out. |
| `SF-002` | `DONE` | `SF-000` | Define non-goals for first release: multi-agent swarms, advanced memory, arbitrary storage plugins, and complex workflow DSL. |
| `SF-003` | `DONE` | `SF-000` | Define product glossary: WorkItem, AgentRun, Sandbox, Command, Artifact, Approval, Workflow, Memory, Policy. |
| `SF-004` | `DONE` | `SF-003` | Decide naming strategy: keep `Sandbox` internally and add factory-level concepts above it. |
| `SF-005` | `DONE` | `SF-003` | Draft architecture decision record for integrated product over plugin substrate. |

Acceptance criteria:

- Shed is not positioned as only a plugin toolkit.
- Sandbox APIs remain lower-level primitives.
- Factory APIs become the main product surface.

## Phase 1: Architecture and contracts

| ID | State | Depends On | Task |
|---|---|---|---|
| `SF-010` | `DONE` | `SF-003` | Add a factory architecture document covering domain model, plugin seams, APIs, events, and security model. |
| `SF-011` | `DONE` | `SF-010` | Define stable state machines for WorkItem and AgentRun. |
| `SF-012` | `DONE` | `SF-010` | Define factory-level event taxonomy. |
| `SF-013` | `DONE` | `SF-010` | Define artifact taxonomy: patch, diff, summary, test report, log bundle, PR, commit, screenshot, and SARIF. |
| `SF-014` | `DONE` | `SF-010` | Define plugin categories: compute, harness, integration, artifact, memory, secrets, and policy. |
| `SF-015` | `DONE` | `SF-014` | Decide which plugin categories are public in MVP: compute and harness only. |
| `SF-016` | `DONE` | `SF-010` | Define compatibility and versioning policy for factory APIs and harness plugin APIs. |
| `SF-017` | `DONE` | `SF-010` | Define minimum audit fields: actor, source, correlation ID, work item ID, run ID, sandbox ID, and command ID. |

Acceptance criteria:

- New factory architecture has enough detail to split implementation across agents.
- Event and API contracts are explicit before large code changes begin.

## Phase 2: Durable store

| ID | State | Depends On | Task |
|---|---|---|---|
| `SF-020` | `DONE` | `SF-010` | Design Postgres schema for existing sandboxes, sessions, commands, events, API tokens, and idempotency keys. |
| `SF-021` | `DONE` | `SF-020` | Add migration framework. |
| `SF-022` | `DONE` | `SF-021` | Implement Postgres-backed `store.Store`. |
| `SF-023` | `DONE` | `SF-022` | Preserve memory store for dev and tests. |
| `SF-024` | `DONE` | `SF-022` | Add transactional event append with related state mutation. |
| `SF-025` | `DONE` | `SF-022` | Add pagination and filtering support to store interfaces. |
| `SF-026` | `DONE` | `SF-022` | Add durable idempotency for external create operations. |
| `SF-027` | `DONE` | `SF-022` | Add restart recovery tests for sandboxes, sessions, commands, and events. |
| `SF-028` | `DONE` | `SF-022` | Add config flags and env vars for Postgres connection and store selection. |

Acceptance criteria:

- `shed server` can run with Postgres.
- Events remain ordered and replayable after restart.
- Existing sandbox API behavior is preserved.

## Phase 3: Factory domain model

| ID | State | Depends On | Task |
|---|---|---|---|
| `SF-030` | `DONE` | `SF-020` | Add `WorkItem` model. |
| `SF-031` | `DONE` | `SF-030` | Add WorkItem states: `queued`, `running`, `waiting_for_approval`, `completed`, `failed`, and `cancelled`. |
| `SF-032` | `DONE` | `SF-030` | Add `AgentRun` model. |
| `SF-033` | `DONE` | `SF-032` | Add AgentRun states: `queued`, `provisioning`, `preparing`, `running`, `validating`, `publishing`, `completed`, `failed`, `cancelled`, and `timed_out`. |
| `SF-034` | `DONE` | `SF-032` | Link AgentRun to Sandbox without replacing Sandbox. |
| `SF-035` | `DONE` | `SF-030` | Add factory-level events for work item lifecycle. |
| `SF-036` | `DONE` | `SF-032` | Add factory-level events for agent run lifecycle. |
| `SF-037` | `DONE` | `SF-030` | Add API endpoints for creating, listing, reading, and cancelling work items. |
| `SF-038` | `DONE` | `SF-032` | Add API endpoints for creating, listing, reading, and cancelling agent runs. |
| `SF-039` | `DONE` | `SF-038` | Add run-level event replay endpoint. |

Acceptance criteria:

- A user can create a WorkItem.
- A user can start an AgentRun for that WorkItem.
- Every AgentRun is traceable to sandbox, commands, events, and artifacts.

## Phase 4: Scheduler and run orchestration

| ID | State | Depends On | Task |
|---|---|---|---|
| `SF-040` | `DONE` | `SF-032` | Add durable scheduler package for AgentRun progression. |
| `SF-041` | `DONE` | `SF-040` | Implement run acquisition and locking to avoid duplicate workers. |
| `SF-042` | `DONE` | `SF-040` | Implement run timeout and cancellation handling. |
| `SF-043` | `DONE` | `SF-040` | Implement retry policy primitives. |
| `SF-044` | `DONE` | `SF-040` | Implement run step events. |
| `SF-045` | `DONE` | `SF-040` | Add scheduler recovery after server restart. |
| `SF-046` | `DONE` | `SF-040` | Add single-node scheduler first and defer distributed scheduling. |

Acceptance criteria:

- AgentRuns progress without manual API chaining.
- Server restart does not lose queued or running state.
- Duplicate execution is prevented by store-backed locks.

## Phase 5: Repository lifecycle

| ID | State | Depends On | Task |
|---|---|---|---|
| `SF-050` | `DONE` | `SF-030` | Add `Repository` model. |
| `SF-051` | `DONE` | `SF-050` | Add repository API for registering repositories. |
| `SF-052` | `DONE` | `SF-050` | Add checkout configuration to WorkItem or AgentRun. |
| `SF-053` | `DONE` | `SF-052` | Implement clone and checkout step inside sandbox. |
| `SF-054` | `DONE` | `SF-053` | Add branch and worktree preparation. |
| `SF-055` | `DONE` | `SF-053` | Add dirty state detection. |
| `SF-056` | `NOT_STARTED` | `SF-055` | Add diff generation. |
| `SF-057` | `NOT_STARTED` | `SF-056` | Add commit creation. |
| `SF-058` | `NOT_STARTED` | `SF-057` | Add push branch support behind approval and policy. |
| `SF-059` | `NOT_STARTED` | `SF-056` | Emit repository lifecycle events. |

Acceptance criteria:

- AgentRun can prepare a repository checkout in a sandbox.
- AgentRun can produce a diff artifact.
- Git actions are observable and auditable.

## Phase 6: Artifact system

| ID | State | Depends On | Task |
|---|---|---|---|
| `SF-060` | `NOT_STARTED` | `SF-013` | Add `Artifact` model. |
| `SF-061` | `NOT_STARTED` | `SF-060` | Add artifact store abstraction. |
| `SF-062` | `NOT_STARTED` | `SF-061` | Implement local filesystem artifact store. |
| `SF-063` | `NOT_STARTED` | `SF-061` | Add metadata-only artifact references for external URLs such as GitHub PRs. |
| `SF-064` | `NOT_STARTED` | `SF-060` | Add artifact API list and read endpoints. |
| `SF-065` | `NOT_STARTED` | `SF-060` | Capture diffs as artifacts. |
| `SF-066` | `NOT_STARTED` | `SF-060` | Capture validation logs as artifacts. |
| `SF-067` | `NOT_STARTED` | `SF-060` | Capture final run summary as artifact. |
| `SF-068` | `NOT_STARTED` | `SF-060` | Add artifact content hashing. |

Acceptance criteria:

- Every meaningful run output is stored as an artifact.
- Artifacts are linked to WorkItem, AgentRun, and Sandbox.

## Phase 7: Harness plugin system

| ID | State | Depends On | Task |
|---|---|---|---|
| `SF-070` | `NOT_STARTED` | `SF-014` | Define `agent.v1` harness interface. |
| `SF-071` | `NOT_STARTED` | `SF-070` | Add harness registry and manager. |
| `SF-072` | `NOT_STARTED` | `SF-070` | Add harness plugin process model, likely mirroring compute plugins. |
| `SF-073` | `NOT_STARTED` | `SF-070` | Define harness capabilities: messages, tools, patch, repo, validation, streaming, and cancellation. |
| `SF-074` | `NOT_STARTED` | `SF-071` | Implement built-in shell harness. |
| `SF-075` | `NOT_STARTED` | `SF-074` | Implement one real coding harness, likely OpenCode first if that is the local dogfood path. |
| `SF-076` | `NOT_STARTED` | `SF-071` | Add harness selection field to AgentRun. |
| `SF-077` | `NOT_STARTED` | `SF-071` | Stream harness output into AgentRun events. |
| `SF-078` | `NOT_STARTED` | `SF-071` | Add harness cancellation and timeout propagation. |
| `SF-079` | `NOT_STARTED` | `SF-071` | Document public harness plugin SDK. |

Acceptance criteria:

- AgentRun can execute through a selected harness.
- Harnesses are isolated from core server logic.
- Harness output is replayable through events.

## Phase 8: Validation pipeline

| ID | State | Depends On | Task |
|---|---|---|---|
| `SF-080` | `NOT_STARTED` | `SF-040` | Add validation step model. |
| `SF-081` | `NOT_STARTED` | `SF-080` | Support validation commands configured per run. |
| `SF-082` | `NOT_STARTED` | `SF-081` | Run validation commands through existing sandbox command path. |
| `SF-083` | `NOT_STARTED` | `SF-082` | Capture validation stdout, stderr, and exit codes as events and artifacts. |
| `SF-084` | `NOT_STARTED` | `SF-080` | Mark AgentRun failed when required validation fails. |
| `SF-085` | `NOT_STARTED` | `SF-080` | Allow validation failures to be sent back to harness for repair loop. |
| `SF-086` | `NOT_STARTED` | `SF-085` | Add max repair attempts. |

Acceptance criteria:

- The platform does not trust agent success claims.
- A run can be marked successful only after configured validation passes.

## Phase 9: Approval and policy

| ID | State | Depends On | Task |
|---|---|---|---|
| `SF-090` | `NOT_STARTED` | `SF-030` | Add `Approval` model. |
| `SF-091` | `NOT_STARTED` | `SF-090` | Add approval request, grant, and deny APIs. |
| `SF-092` | `NOT_STARTED` | `SF-090` | Add approval gates before push, PR creation, and secret use. |
| `SF-093` | `NOT_STARTED` | `SF-090` | Emit approval events. |
| `SF-094` | `NOT_STARTED` | `SF-090` | Add basic policy model. |
| `SF-095` | `NOT_STARTED` | `SF-094` | Implement built-in policy checks for allowed repositories, allowed harnesses, allowed commands, and network mode. |
| `SF-096` | `NOT_STARTED` | `SF-094` | Add policy decision events. |
| `SF-097` | `DEFERRED` | `SF-094` | Add external policy plugin or OPA integration after MVP. |

Acceptance criteria:

- Human approval can pause and resume a run.
- Dangerous publication steps require explicit approval when configured.
- Policy decisions are auditable.

## Phase 10: Secrets and identity

| ID | State | Depends On | Task |
|---|---|---|---|
| `SF-100` | `NOT_STARTED` | `SF-020` | Add user and actor identity model. |
| `SF-101` | `NOT_STARTED` | `SF-100` | Add actor attribution to WorkItems, AgentRuns, approvals, and events. |
| `SF-102` | `DEFERRED` | `SF-100` | Add organization and team model after MVP if multi-tenancy is required. |
| `SF-103` | `NOT_STARTED` | `SF-100` | Add scoped API tokens. |
| `SF-104` | `NOT_STARTED` | `SF-103` | Add token scopes for work, runs, repos, approvals, and admin. |
| `SF-105` | `NOT_STARTED` | `SF-100` | Add secret reference model. |
| `SF-106` | `NOT_STARTED` | `SF-105` | Implement env-based and dev secret provider. |
| `SF-107` | `DEFERRED` | `SF-105` | Add Vault, 1Password, or AWS provider after MVP. |
| `SF-108` | `NOT_STARTED` | `SF-105` | Ensure secrets are never stored in events, metadata, command logs, or artifacts. |

Acceptance criteria:

- Every external action has an actor.
- Tokens have explicit scopes.
- Secrets are referenced and injected, not casually persisted.

## Phase 11: GitHub integration

| ID | State | Depends On | Task |
|---|---|---|---|
| `SF-110` | `NOT_STARTED` | `SF-050` | Add GitHub integration configuration. |
| `SF-111` | `NOT_STARTED` | `SF-110` | Add GitHub webhook receiver with signature verification. |
| `SF-112` | `NOT_STARTED` | `SF-111` | Convert issue comment or label events into WorkItems. |
| `SF-113` | `NOT_STARTED` | `SF-111` | Convert PR review requests into WorkItems. |
| `SF-114` | `NOT_STARTED` | `SF-110` | Add GitHub branch push support. |
| `SF-115` | `NOT_STARTED` | `SF-114` | Add PR creation support. |
| `SF-116` | `NOT_STARTED` | `SF-115` | Add PR comment and status update support. |
| `SF-117` | `NOT_STARTED` | `SF-111` | Add webhook idempotency by delivery ID. |
| `SF-118` | `NOT_STARTED` | `SF-110` | Emit GitHub integration events. |

Acceptance criteria:

- A GitHub trigger can create a WorkItem.
- A completed AgentRun can open or update a PR.
- Webhook retries do not duplicate work.

## Phase 12: Workflow engine

| ID | State | Depends On | Task |
|---|---|---|---|
| `SF-120` | `NOT_STARTED` | `SF-040` | Add `WorkflowDefinition` model. |
| `SF-121` | `NOT_STARTED` | `SF-120` | Add `WorkflowRun` model. |
| `SF-122` | `NOT_STARTED` | `SF-121` | Implement fixed built-in workflow: checkout, run agent, validate, summarize, approval, and publish. |
| `SF-123` | `NOT_STARTED` | `SF-122` | Add workflow dispatch API. |
| `SF-124` | `NOT_STARTED` | `SF-122` | Add workflow step events. |
| `SF-125` | `NOT_STARTED` | `SF-122` | Add basic retry and timeout per step. |
| `SF-126` | `DEFERRED` | `SF-122` | Defer broad YAML DSL until several built-in workflows are proven. |
| `SF-127` | `NOT_STARTED` | `SF-122` | Add workflow templates for issue fix, PR review, and CI failure investigation. |

Acceptance criteria:

- Users can run a repeatable factory process.
- MVP does not get blocked by an over-designed DSL.

## Phase 13: Memory

| ID | State | Depends On | Task |
|---|---|---|---|
| `SF-130` | `NOT_STARTED` | `SF-067` | Add `Memory` model. |
| `SF-131` | `NOT_STARTED` | `SF-130` | Add memory scopes: global, org, repo, workflow, harness, and user. |
| `SF-132` | `NOT_STARTED` | `SF-130` | Generate run summaries suitable for memory extraction. |
| `SF-133` | `NOT_STARTED` | `SF-132` | Propose memories after completed runs. |
| `SF-134` | `NOT_STARTED` | `SF-133` | Require approval before memory acceptance in MVP. |
| `SF-135` | `NOT_STARTED` | `SF-130` | Inject accepted memories into future harness context. |
| `SF-136` | `NOT_STARTED` | `SF-130` | Emit memory proposed, accepted, used, and retired events. |
| `SF-137` | `DEFERRED` | `SF-130` | Defer vector retrieval until plain scoped retrieval is insufficient. |

Acceptance criteria:

- Memory is auditable and portable.
- Agents can benefit from prior runs without hidden state.

## Phase 14: UI migration

| ID | State | Depends On | Task |
|---|---|---|---|
| `SF-140` | `NOT_STARTED` | `SF-030` | Add WorkItem list screen. |
| `SF-141` | `NOT_STARTED` | `SF-032` | Add AgentRun detail screen. |
| `SF-142` | `NOT_STARTED` | `SF-039` | Add live event stream for AgentRun. |
| `SF-143` | `NOT_STARTED` | `SF-060` | Add artifact browser. |
| `SF-144` | `NOT_STARTED` | `SF-065` | Add diff viewer. |
| `SF-145` | `NOT_STARTED` | `SF-090` | Add approval inbox. |
| `SF-146` | `NOT_STARTED` | `SF-050` | Add repository configuration screen. |
| `SF-147` | `NOT_STARTED` | `SF-120` | Add workflow dispatch and history screen. |
| `SF-148` | `NOT_STARTED` | Existing UI | Keep sandbox and command detail screens as drilldowns. |
| `SF-149` | `NOT_STARTED` | `SF-141` | Add run status dashboard for active, completed, and failed runs. |

Acceptance criteria:

- The UI presents factory concepts first.
- Sandbox and command views remain available for debugging.

## Phase 15: CLI and developer experience

| ID | State | Depends On | Task |
|---|---|---|---|
| `SF-150` | `NOT_STARTED` | `SF-037` | Add CLI command to create WorkItem. |
| `SF-151` | `NOT_STARTED` | `SF-038` | Add CLI command to start AgentRun. |
| `SF-152` | `NOT_STARTED` | `SF-039` | Add CLI command to follow AgentRun events. |
| `SF-153` | `NOT_STARTED` | `SF-064` | Add CLI command to fetch artifacts. |
| `SF-154` | `NOT_STARTED` | `SF-091` | Add CLI approval commands. |
| `SF-155` | `NOT_STARTED` | `SF-075` | Add dev-mode quickstart for local factory run. |
| `SF-156` | `NOT_STARTED` | `SF-155` | Add sample repository and task fixture for demos. |

Acceptance criteria:

- A developer can try the factory locally from the CLI.
- `shed dev` remains production-faithful.

## Phase 16: Observability and audit

| ID | State | Depends On | Task |
|---|---|---|---|
| `SF-160` | `NOT_STARTED` | `SF-017` | Add correlation IDs across API, scheduler, compute, harness, commands, and events. |
| `SF-161` | `NOT_STARTED` | `SF-160` | Add structured logs. |
| `SF-162` | `NOT_STARTED` | `SF-160` | Add metrics endpoint. |
| `SF-163` | `NOT_STARTED` | `SF-160` | Add audit export endpoint. |
| `SF-164` | `NOT_STARTED` | `SF-160` | Add event schema and version field if needed. |
| `SF-165` | `NOT_STARTED` | `SF-160` | Add operator diagnostics for stuck runs, disconnected clients, failed harnesses, and plugin errors. |

Acceptance criteria:

- Operators can answer who started what, what ran, what changed, and why it failed.
- Audit history survives restarts.

## Phase 17: Compute evolution

| ID | State | Depends On | Task |
|---|---|---|---|
| `SF-170` | `NOT_STARTED` | Existing compute plugins | Keep local compute as default dev path. |
| `SF-171` | `NOT_STARTED` | `SF-020` | Add Docker compute plugin. |
| `SF-172` | `NOT_STARTED` | `SF-171` | Add container resource limits. |
| `SF-173` | `NOT_STARTED` | `SF-171` | Add network policy modes: disabled, restricted, and full. |
| `SF-174` | `NOT_STARTED` | `SF-171` | Add workspace volume lifecycle. |
| `SF-175` | `DEFERRED` | `SF-171` | Add Kubernetes compute plugin after Docker path is proven. |
| `SF-176` | `DEFERRED` | `SF-171` | Add Firecracker or VM isolation if market requires stronger isolation. |

Acceptance criteria:

- MVP can run locally and in a containerized environment.
- Stronger compute backends can be added without changing factory domain logic.

## Phase 18: Security hardening

| ID | State | Depends On | Task |
|---|---|---|---|
| `SF-180` | `NOT_STARTED` | `SF-095` | Add command allow and deny policy. |
| `SF-181` | `NOT_STARTED` | `SF-095` | Add path enforcement for repository and workspace operations. |
| `SF-182` | `NOT_STARTED` | `SF-108` | Add secret redaction tests. |
| `SF-183` | `NOT_STARTED` | `SF-173` | Add network egress policy enforcement for supported compute. |
| `SF-184` | `NOT_STARTED` | `SF-103` | Add scoped token tests. |
| `SF-185` | `NOT_STARTED` | `SF-111` | Add webhook signature verification tests. |
| `SF-186` | `NOT_STARTED` | `SF-090` | Add approval bypass tests. |
| `SF-187` | `NOT_STARTED` | `SF-160` | Add audit completeness tests. |

Acceptance criteria:

- Security and auditability are treated as correctness.
- Common bypasses are covered by tests.

## Phase 19: Packaging and deployment

| ID | State | Depends On | Task |
|---|---|---|---|
| `SF-190` | `NOT_STARTED` | `SF-028` | Add production config file support. |
| `SF-191` | `NOT_STARTED` | `SF-028` | Add Docker image. |
| `SF-192` | `NOT_STARTED` | `SF-191` | Add docker-compose for server plus Postgres. |
| `SF-193` | `DEFERRED` | `SF-191` | Add Helm chart if Kubernetes demand exists. |
| `SF-194` | `NOT_STARTED` | `SF-160` | Add health and readiness endpoints for factory dependencies. |
| `SF-195` | `NOT_STARTED` | `SF-020` | Add backup and restore documentation. |
| `SF-196` | `NOT_STARTED` | `SF-191` | Add deployment guide for self-hosted MVP. |

Acceptance criteria:

- A team can run Shed Factory with one binary or docker-compose.
- Production dependencies are explicit.

## Phase 20: Testing strategy

| ID | State | Depends On | Task |
|---|---|---|---|
| `SF-200` | `DONE` | `SF-020` | Add store contract tests that run against memory and Postgres. |
| `SF-201` | `NOT_STARTED` | `SF-030` | Add WorkItem API tests. |
| `SF-202` | `NOT_STARTED` | `SF-032` | Add AgentRun API tests. |
| `SF-203` | `NOT_STARTED` | `SF-040` | Add scheduler recovery tests. |
| `SF-204` | `NOT_STARTED` | `SF-053` | Add repository lifecycle tests. |
| `SF-205` | `NOT_STARTED` | `SF-070` | Add harness manager tests. |
| `SF-206` | `NOT_STARTED` | `SF-080` | Add validation loop tests. |
| `SF-207` | `NOT_STARTED` | `SF-090` | Add approval gate tests. |
| `SF-208` | `NOT_STARTED` | `SF-110` | Add GitHub webhook idempotency tests. |
| `SF-209` | `NOT_STARTED` | `SF-140` | Add UI smoke tests. |
| `SF-210` | `NOT_STARTED` | `SF-155` | Add end-to-end demo test: task to patch artifact. |

Acceptance criteria:

- Core factory behavior has automated coverage.
- Store behavior is consistent across memory and Postgres.

## Recommended execution order

1. `SF-000` through `SF-017`.
2. `SF-020` through `SF-028`.
3. `SF-030` through `SF-046`.
4. `SF-050` through `SF-068`.
5. `SF-070` through `SF-086`.
6. `SF-090` through `SF-118`.
7. `SF-120` through `SF-149`.
8. `SF-150` through `SF-210`.

Critical path:

```text
Architecture -> Postgres store -> WorkItem/AgentRun -> Scheduler -> Repository lifecycle -> Harness -> Artifacts -> Validation -> Approval -> GitHub PR
```

## MVP completion definition

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

## Fan-out guidance

Use task IDs as stable work identifiers in branches, issues, commits, and PR titles.

Suggested branch format:

```text
sf-030-work-item-model
sf-070-agent-v1-interface
sf-110-github-integration-config
```

Suggested PR title format:

```text
SF-030 Add WorkItem model
```

Do not start implementation work from deferred tasks unless their dependencies have been revisited and the state is changed from `DEFERRED` to `NOT_STARTED`.
