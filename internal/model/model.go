package model

import "time"

type SandboxState string

const (
	SandboxPendingClient SandboxState = "pending_client"
	SandboxReady         SandboxState = "ready"
	SandboxDegraded      SandboxState = "degraded"
	SandboxReleasing     SandboxState = "releasing"
	SandboxReleased      SandboxState = "released"
	SandboxFailed        SandboxState = "failed"
)

type SessionState string

const (
	SessionIssued       SessionState = "issued"
	SessionConnected    SessionState = "connected"
	SessionRegistered   SessionState = "registered"
	SessionDisconnected SessionState = "disconnected"
	SessionExpired      SessionState = "expired"
	SessionClosed       SessionState = "closed"
)

type CommandState string

const (
	CommandQueued     CommandState = "queued"
	CommandStarting   CommandState = "starting"
	CommandRunning    CommandState = "running"
	CommandCancelling CommandState = "cancelling"
	CommandExited     CommandState = "exited"
	CommandKilled     CommandState = "killed"
	CommandFailed     CommandState = "failed"
)

type Lease struct {
	TTLMillis int64     `json:"ttl_ms"`
	ExpiresAt time.Time `json:"expires_at"`
}

type Sandbox struct {
	ID                   string            `json:"id"`
	Environment          string            `json:"environment"`
	Template             string            `json:"template"`
	ComputeClass         string            `json:"compute_class,omitempty"`
	State                SandboxState      `json:"state"`
	Compute              string            `json:"compute_driver,omitempty"`
	ComputeAPIVersion    string            `json:"compute_api_version,omitempty"`
	ComputePluginVersion string            `json:"compute_plugin_version,omitempty"`
	ExternalAllocationID string            `json:"external_allocation_id,omitempty"`
	Parameters           map[string]any    `json:"parameters,omitempty"`
	ComputeConfig        map[string]any    `json:"compute_config,omitempty"`
	ComputeMetadata      map[string]string `json:"compute_metadata,omitempty"`
	Metadata             map[string]string `json:"metadata,omitempty"`
	Capabilities         map[string]bool   `json:"capabilities,omitempty"`
	Lease                Lease             `json:"lease"`
	InsertedAt           time.Time         `json:"inserted_at"`
	UpdatedAt            time.Time         `json:"updated_at"`
}

type ClientSession struct {
	SessionID         string            `json:"session_id"`
	SessionKey        string            `json:"session_key,omitempty"`
	SandboxID         string            `json:"sandbox_id"`
	State             SessionState      `json:"state"`
	Capabilities      map[string]bool   `json:"capabilities,omitempty"`
	Metadata          map[string]string `json:"metadata,omitempty"`
	LastClientSeqSeen int64             `json:"last_client_seq_seen"`
	LastServerSeqSent int64             `json:"last_server_seq_sent"`
	InsertedAt        time.Time         `json:"inserted_at"`
	UpdatedAt         time.Time         `json:"updated_at"`
}

type Command struct {
	ID          string            `json:"id"`
	SandboxID   string            `json:"sandbox_id"`
	State       CommandState      `json:"state"`
	Command     string            `json:"command"`
	Cwd         string            `json:"cwd"`
	Env         map[string]string `json:"env,omitempty"`
	Stdin       bool              `json:"stdin"`
	TimeoutMS   int64             `json:"timeout_ms"`
	Metadata    map[string]string `json:"metadata,omitempty"`
	PID         int               `json:"pid,omitempty"`
	ExitCode    *int              `json:"exit_code,omitempty"`
	Signal      string            `json:"signal,omitempty"`
	StartedAt   *time.Time        `json:"started_at,omitempty"`
	CompletedAt *time.Time        `json:"completed_at,omitempty"`
	InsertedAt  time.Time         `json:"inserted_at"`
	UpdatedAt   time.Time         `json:"updated_at"`
}

type JobState string

const (
	JobQueued            JobState = "queued"
	JobAllocatingCompute JobState = "allocating_compute"
	JobRunningAgent      JobState = "running_agent"
	JobSucceeded         JobState = "succeeded"
	JobFailed            JobState = "failed"
	JobCancelled         JobState = "cancelled"
)

type JobTrigger struct {
	Source      string `json:"source"`
	Actor       string `json:"actor,omitempty"`
	ExternalRef string `json:"external_ref,omitempty"`
}

type JobResult struct {
	Branch   string `json:"branch,omitempty"`
	PRURL    string `json:"pr_url,omitempty"`
	PRNumber int    `json:"pr_number,omitempty"`
}

type Job struct {
	ID             string            `json:"id"`
	Repo           string            `json:"repo"`
	BaseRef        string            `json:"base_ref"`
	WorkBranch     string            `json:"work_branch"`
	Prompt         string            `json:"prompt"`
	ComputeClass   string            `json:"compute_class,omitempty"`
	AgentDriver    string            `json:"agent_driver"`
	Provider       string            `json:"provider,omitempty"`
	Model          string            `json:"model,omitempty"`
	ScmDriver      string            `json:"scm_driver"`
	SandboxID      string            `json:"sandbox_id,omitempty"`
	AgentCommandID string            `json:"agent_command_id,omitempty"`
	State          JobState          `json:"state"`
	FailureReason  string            `json:"failure_reason,omitempty"`
	Trigger        JobTrigger        `json:"trigger"`
	Result         JobResult         `json:"result"`
	Metadata       map[string]string `json:"metadata,omitempty"`
	InsertedAt     time.Time         `json:"inserted_at"`
	UpdatedAt      time.Time         `json:"updated_at"`
}

type Event struct {
	ID        string         `json:"id"`
	SandboxID string         `json:"sandbox_id"`
	CommandID string         `json:"command_id,omitempty"`
	Seq       int64          `json:"seq"`
	Type      string         `json:"type"`
	Source    string         `json:"source,omitempty"`
	Timestamp time.Time      `json:"timestamp"`
	Data      map[string]any `json:"data,omitempty"`
}
