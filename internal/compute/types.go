package compute

import (
	"context"
	"time"
)

const (
	APIVersionV1 = "compute.v1"
)

type PluginInfo struct {
	Name         string          `json:"name"`
	Version      string          `json:"version"`
	APIVersions  []string        `json:"api_versions"`
	Capabilities map[string]bool `json:"capabilities,omitempty"`
}

// DriverDescriptor summarizes a registered compute driver for operator surfaces.
type DriverDescriptor struct {
	Name       string            `json:"name"`
	Kind       string            `json:"kind"`
	Default    bool              `json:"default"`
	Command    string            `json:"command,omitempty"`
	Args       []string          `json:"args,omitempty"`
	EnvKeys    []string          `json:"env_keys,omitempty"`
	APIVersion string            `json:"api_version,omitempty"`
	Config     map[string]string `json:"config,omitempty"`
	Plugin     *PluginInfo       `json:"plugin,omitempty"`
	Loaded     bool              `json:"loaded"`
	Error      string            `json:"error,omitempty"`
}

type AllocateRequest struct {
	APIVersion     string            `json:"api_version"`
	ComputeDriver  string            `json:"compute_driver"`
	SandboxID      string            `json:"sandbox_id"`
	SessionID      string            `json:"session_id"`
	SessionKey     string            `json:"session_key"`
	ConnectURL     string            `json:"connect_url"`
	Environment    string            `json:"environment"`
	Template       string            `json:"template"`
	LeaseTTLMillis int64             `json:"lease_ttl_ms"`
	LeaseExpiresAt time.Time         `json:"lease_expires_at"`
	Config         map[string]string `json:"config,omitempty"`
	Metadata       map[string]string `json:"metadata,omitempty"`
}

type AllocateResponse struct {
	ExternalID    string            `json:"external_id,omitempty"`
	APIVersion    string            `json:"api_version"`
	PluginName    string            `json:"plugin_name"`
	PluginVersion string            `json:"plugin_version"`
	Metadata      map[string]string `json:"metadata,omitempty"`
}

type StatusRequest struct {
	APIVersion string            `json:"api_version"`
	SandboxID  string            `json:"sandbox_id"`
	ExternalID string            `json:"external_id,omitempty"`
	Config     map[string]string `json:"config,omitempty"`
}

type StatusResponse struct {
	State    string            `json:"state"`
	Message  string            `json:"message,omitempty"`
	Metadata map[string]string `json:"metadata,omitempty"`
}

type RenewRequest struct {
	APIVersion     string            `json:"api_version"`
	SandboxID      string            `json:"sandbox_id"`
	ExternalID     string            `json:"external_id,omitempty"`
	LeaseTTLMillis int64             `json:"lease_ttl_ms"`
	LeaseExpiresAt time.Time         `json:"lease_expires_at"`
	Config         map[string]string `json:"config,omitempty"`
}

type RenewResponse struct {
	LeaseExpiresAt time.Time         `json:"lease_expires_at"`
	Metadata       map[string]string `json:"metadata,omitempty"`
}

type ReleaseRequest struct {
	APIVersion string            `json:"api_version"`
	SandboxID  string            `json:"sandbox_id"`
	ExternalID string            `json:"external_id,omitempty"`
	Config     map[string]string `json:"config,omitempty"`
	Reason     string            `json:"reason,omitempty"`
}

type ReleaseResponse struct {
	Released bool              `json:"released"`
	Metadata map[string]string `json:"metadata,omitempty"`
}

type ExecRequest struct {
	APIVersion string            `json:"api_version"`
	SandboxID  string            `json:"sandbox_id"`
	ExternalID string            `json:"external_id,omitempty"`
	CommandID  string            `json:"command_id"`
	Command    string            `json:"command"`
	Cwd        string            `json:"cwd"`
	Env        map[string]string `json:"env,omitempty"`
	Stdin      bool              `json:"stdin"`
	TimeoutMS  int64             `json:"timeout_ms"`
	Config     map[string]string `json:"config,omitempty"`
	Metadata   map[string]string `json:"metadata,omitempty"`
}

type ExecEvent struct {
	CommandID string         `json:"command_id"`
	Type      string         `json:"type"`
	Data      map[string]any `json:"data,omitempty"`
}

type ExecStdinRequest struct {
	APIVersion string `json:"api_version"`
	SandboxID  string `json:"sandbox_id"`
	ExternalID string `json:"external_id,omitempty"`
	CommandID  string `json:"command_id"`
	Data       string `json:"data"`
}

type ExecSignalRequest struct {
	APIVersion    string `json:"api_version"`
	SandboxID     string `json:"sandbox_id"`
	ExternalID    string `json:"external_id,omitempty"`
	CommandID     string `json:"command_id"`
	GracePeriodMS int64  `json:"grace_period_ms,omitempty"`
	Signal        string `json:"signal,omitempty"`
}

type ExecControlResponse struct {
	Accepted bool              `json:"accepted"`
	Metadata map[string]string `json:"metadata,omitempty"`
}

type ExecEventSink func(ExecEvent) error

type ComputeV1 interface {
	Info(context.Context) (PluginInfo, error)
	Allocate(context.Context, AllocateRequest) (AllocateResponse, error)
	Status(context.Context, StatusRequest) (StatusResponse, error)
	Renew(context.Context, RenewRequest) (RenewResponse, error)
	Release(context.Context, ReleaseRequest) (ReleaseResponse, error)
	Exec(context.Context, ExecRequest, ExecEventSink) error
	Stdin(context.Context, ExecStdinRequest) (ExecControlResponse, error)
	Cancel(context.Context, ExecSignalRequest) (ExecControlResponse, error)
	Kill(context.Context, ExecSignalRequest) (ExecControlResponse, error)
}

type closableCompute interface {
	Close() error
}

func HasAPIVersion(info PluginInfo, apiVersion string) bool {
	for _, v := range info.APIVersions {
		if v == apiVersion {
			return true
		}
	}
	return false
}
