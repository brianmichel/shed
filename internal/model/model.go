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
	State                SandboxState      `json:"state"`
	Compute              string            `json:"compute_driver,omitempty"`
	ComputeAPIVersion    string            `json:"compute_api_version,omitempty"`
	ComputePluginVersion string            `json:"compute_plugin_version,omitempty"`
	ExternalAllocationID string            `json:"external_allocation_id,omitempty"`
	ComputeConfig        map[string]string `json:"compute_config,omitempty"`
	ComputeMetadata      map[string]string `json:"compute_metadata,omitempty"`
	Metadata             map[string]string `json:"metadata,omitempty"`
	Capabilities         map[string]bool   `json:"capabilities,omitempty"`
	Lease                Lease             `json:"lease"`
	InsertedAt           time.Time         `json:"inserted_at"`
	UpdatedAt            time.Time         `json:"updated_at"`
}

type ClientSession struct {
	SessionID         string            `json:"session_id"`
	AgentToken        string            `json:"agent_token,omitempty"`
	AgentTokenHash    string            `json:"-"`
	SandboxID         string            `json:"sandbox_id"`
	State             SessionState      `json:"state"`
	Capabilities      map[string]bool   `json:"capabilities,omitempty"`
	Metadata          map[string]string `json:"metadata,omitempty"`
	LastClientSeqSeen int64             `json:"last_client_seq_seen"`
	LastServerSeqSent int64             `json:"last_server_seq_sent"`
	InsertedAt        time.Time         `json:"inserted_at"`
	UpdatedAt         time.Time         `json:"updated_at"`
}

type APIToken struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	TokenHash   string            `json:"-"`
	TokenPrefix string            `json:"token_prefix"`
	Metadata    map[string]string `json:"metadata,omitempty"`
	LastUsedAt  *time.Time        `json:"last_used_at,omitempty"`
	InsertedAt  time.Time         `json:"inserted_at"`
	UpdatedAt   time.Time         `json:"updated_at"`
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
