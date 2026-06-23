package store

import (
	"context"
	"time"

	"github.com/brianmichel/shed/internal/model"
)

type SandboxCreate struct {
	Environment       string
	Template          string
	TTL               time.Duration
	Compute           string
	ComputeAPIVersion string
	ComputeConfig     map[string]string
	Metadata          map[string]string
}

type SandboxAllocationUpdate struct {
	Compute              string
	ComputeAPIVersion    string
	ComputePluginVersion string
	ExternalAllocationID string
	ComputeMetadata      map[string]string
}

type CommandCreate struct {
	Command   string
	Cwd       string
	Env       map[string]string
	Stdin     bool
	TimeoutMS int64
	Metadata  map[string]string
}

type APITokenCreate struct {
	Name     string
	Metadata map[string]string
}

type APITokenCreateResult struct {
	Token  model.APIToken
	Secret string
}

type Store interface {
	CreateSandbox(ctx context.Context, in SandboxCreate) (model.Sandbox, model.ClientSession, error)
	ListSandboxes(ctx context.Context) ([]model.Sandbox, error)
	GetSandbox(ctx context.Context, sandboxID string) (model.Sandbox, error)
	UpdateSandboxState(ctx context.Context, sandboxID string, state model.SandboxState) (model.Sandbox, error)
	UpdateSandboxAllocation(ctx context.Context, sandboxID string, in SandboxAllocationUpdate) (model.Sandbox, error)
	ExtendLease(ctx context.Context, sandboxID string, ttl time.Duration) (model.Lease, error)

	AuthenticateSession(ctx context.Context, sandboxID, agentToken string) (model.ClientSession, error)
	GetSession(ctx context.Context, sessionID string) (model.ClientSession, error)
	FindSessionBySandbox(ctx context.Context, sandboxID string) (model.ClientSession, error)
	UpdateSession(ctx context.Context, session model.ClientSession) (model.ClientSession, error)

	CreateAPIToken(ctx context.Context, in APITokenCreate) (APITokenCreateResult, error)
	ListAPITokens(ctx context.Context) ([]model.APIToken, error)
	AuthenticateAPIToken(ctx context.Context, token string) (model.APIToken, error)

	CreateCommand(ctx context.Context, sandboxID string, in CommandCreate) (model.Command, error)
	ListCommands(ctx context.Context, sandboxID string) ([]model.Command, error)
	GetCommand(ctx context.Context, sandboxID, commandID string) (model.Command, error)
	UpdateCommand(ctx context.Context, command model.Command) (model.Command, error)

	AppendEvent(ctx context.Context, sandboxID, commandID, source, eventType string, data map[string]any) (model.Event, error)
	ListSandboxEvents(ctx context.Context, sandboxID string, after int64) ([]model.Event, int64, error)
	ListCommandEvents(ctx context.Context, sandboxID, commandID string, after int64) ([]model.Event, int64, error)

	RememberIdempotencyKey(ctx context.Context, key, value string) (string, bool, error)
}
