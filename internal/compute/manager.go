package compute

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"sync"
	"time"

	hplugin "github.com/hashicorp/go-plugin"
	"google.golang.org/grpc"
)

var ErrComputeNotFound = errors.New("compute_not_found")

type EventSink func(ctx context.Context, sandboxID, eventType string, data map[string]any)

type ExternalPluginConfig struct {
	Name       string
	Command    string
	Args       []string
	Env        map[string]string
	APIVersion string
}

type ManagerConfig struct {
	DefaultCompute string
	StartupTimeout time.Duration
	CallTimeout    time.Duration
	EventSink      EventSink
}

type Manager struct {
	cfg       ManagerConfig
	mu        sync.Mutex
	builtins  map[string]ComputeV1
	externals map[string]ExternalPluginConfig
	clients   map[string]*externalClient
}

type externalClient struct {
	client  *hplugin.Client
	compute ComputeV1
	info    PluginInfo
}

func NewManager(cfg ManagerConfig) *Manager {
	if cfg.DefaultCompute == "" {
		cfg.DefaultCompute = "local"
	}
	if cfg.StartupTimeout == 0 {
		cfg.StartupTimeout = 10 * time.Second
	}
	if cfg.CallTimeout == 0 {
		cfg.CallTimeout = 30 * time.Second
	}
	return &Manager{cfg: cfg, builtins: map[string]ComputeV1{}, externals: map[string]ExternalPluginConfig{}, clients: map[string]*externalClient{}}
}

func (m *Manager) DefaultCompute() string { return m.cfg.DefaultCompute }

func (m *Manager) SetEventSink(sink EventSink) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.cfg.EventSink = sink
}

func (m *Manager) RegisterBuiltin(name string, alloc ComputeV1) error {
	if name == "" || alloc == nil {
		return fmt.Errorf("compute name and implementation are required")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.builtins[name] = alloc
	return nil
}

func (m *Manager) RegisterExternal(cfg ExternalPluginConfig) error {
	if cfg.Name == "" || cfg.Command == "" {
		return fmt.Errorf("external compute name and command are required")
	}
	if cfg.APIVersion == "" {
		cfg.APIVersion = APIVersionV1
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.externals[cfg.Name] = cfg
	return nil
}

func (m *Manager) Allocate(ctx context.Context, req AllocateRequest) (AllocateResponse, error) {
	name := req.ComputeDriver
	if name == "" {
		name = m.cfg.DefaultCompute
	}
	req.ComputeDriver = name
	if req.APIVersion == "" {
		req.APIVersion = APIVersionV1
	}
	m.emit(ctx, req.SandboxID, "compute.allocate.started", map[string]any{"compute": name, "api_version": req.APIVersion})
	start := time.Now()
	alloc, info, err := m.resolve(ctx, name, req.APIVersion)
	if err != nil {
		m.emit(ctx, req.SandboxID, "compute.allocate.failed", map[string]any{"compute": name, "error": err.Error()})
		return AllocateResponse{}, err
	}
	callCtx, cancel := m.withCallTimeout(ctx)
	defer cancel()
	resp, err := alloc.Allocate(callCtx, req)
	if err != nil {
		m.emit(ctx, req.SandboxID, "compute.allocate.failed", map[string]any{"compute": name, "plugin_name": info.Name, "plugin_version": info.Version, "error": err.Error(), "duration_ms": time.Since(start).Milliseconds()})
		return AllocateResponse{}, err
	}
	if resp.APIVersion == "" {
		resp.APIVersion = req.APIVersion
	}
	if resp.PluginName == "" {
		resp.PluginName = info.Name
	}
	if resp.PluginVersion == "" {
		resp.PluginVersion = info.Version
	}
	m.emit(ctx, req.SandboxID, "compute.allocate.succeeded", map[string]any{"compute": name, "plugin_name": resp.PluginName, "plugin_version": resp.PluginVersion, "external_id": resp.ExternalID, "duration_ms": time.Since(start).Milliseconds()})
	return resp, nil
}

func (m *Manager) Status(ctx context.Context, computeName string, req StatusRequest) (StatusResponse, error) {
	if computeName == "" {
		computeName = m.cfg.DefaultCompute
	}
	if req.APIVersion == "" {
		req.APIVersion = APIVersionV1
	}
	alloc, info, err := m.resolve(ctx, computeName, req.APIVersion)
	if err != nil {
		return StatusResponse{}, err
	}
	callCtx, cancel := m.withCallTimeout(ctx)
	defer cancel()
	resp, err := alloc.Status(callCtx, req)
	if err == nil {
		m.emit(ctx, req.SandboxID, "compute.status", map[string]any{"compute": computeName, "plugin_name": info.Name, "state": resp.State, "message": resp.Message})
	}
	return resp, err
}

func (m *Manager) Renew(ctx context.Context, computeName string, req RenewRequest) (RenewResponse, error) {
	if computeName == "" {
		computeName = m.cfg.DefaultCompute
	}
	if req.APIVersion == "" {
		req.APIVersion = APIVersionV1
	}
	alloc, info, err := m.resolve(ctx, computeName, req.APIVersion)
	if err != nil {
		return RenewResponse{}, err
	}
	callCtx, cancel := m.withCallTimeout(ctx)
	defer cancel()
	resp, err := alloc.Renew(callCtx, req)
	if err != nil {
		m.emit(ctx, req.SandboxID, "compute.renew.failed", map[string]any{"compute": computeName, "plugin_name": info.Name, "error": err.Error()})
		return RenewResponse{}, err
	}
	m.emit(ctx, req.SandboxID, "compute.renewed", map[string]any{"compute": computeName, "plugin_name": info.Name, "lease_expires_at": resp.LeaseExpiresAt})
	return resp, nil
}

func (m *Manager) Release(ctx context.Context, computeName string, req ReleaseRequest) (ReleaseResponse, error) {
	if computeName == "" {
		computeName = m.cfg.DefaultCompute
	}
	if req.APIVersion == "" {
		req.APIVersion = APIVersionV1
	}
	m.emit(ctx, req.SandboxID, "compute.release.started", map[string]any{"compute": computeName, "reason": req.Reason})
	alloc, info, err := m.resolve(ctx, computeName, req.APIVersion)
	if err != nil {
		m.emit(ctx, req.SandboxID, "compute.release.failed", map[string]any{"compute": computeName, "error": err.Error()})
		return ReleaseResponse{}, err
	}
	callCtx, cancel := m.withCallTimeout(ctx)
	defer cancel()
	resp, err := alloc.Release(callCtx, req)
	if err != nil {
		m.emit(ctx, req.SandboxID, "compute.release.failed", map[string]any{"compute": computeName, "plugin_name": info.Name, "error": err.Error()})
		return ReleaseResponse{}, err
	}
	m.emit(ctx, req.SandboxID, "compute.release.succeeded", map[string]any{"compute": computeName, "plugin_name": info.Name, "released": resp.Released})
	return resp, nil
}

func (m *Manager) SupportsExec(ctx context.Context, computeName, apiVersion string) bool {
	if computeName == "" {
		computeName = m.cfg.DefaultCompute
	}
	if apiVersion == "" {
		apiVersion = APIVersionV1
	}
	_, info, err := m.resolve(ctx, computeName, apiVersion)
	return err == nil && info.Capabilities["exec"]
}

func (m *Manager) Exec(ctx context.Context, computeName string, req ExecRequest, sink ExecEventSink) error {
	if computeName == "" {
		computeName = m.cfg.DefaultCompute
	}
	if req.APIVersion == "" {
		req.APIVersion = APIVersionV1
	}
	m.emit(ctx, req.SandboxID, "compute.exec.started", map[string]any{"compute": computeName, "command_id": req.CommandID})
	alloc, info, err := m.resolve(ctx, computeName, req.APIVersion)
	if err != nil {
		m.emit(ctx, req.SandboxID, "compute.exec.failed", map[string]any{"compute": computeName, "command_id": req.CommandID, "error": err.Error()})
		return err
	}
	if !info.Capabilities["exec"] {
		err := fmt.Errorf("compute %q does not support exec", computeName)
		m.emit(ctx, req.SandboxID, "compute.exec.failed", map[string]any{"compute": computeName, "command_id": req.CommandID, "error": err.Error()})
		return err
	}
	err = alloc.Exec(ctx, req, sink)
	if err != nil {
		m.emit(ctx, req.SandboxID, "compute.exec.failed", map[string]any{"compute": computeName, "plugin_name": info.Name, "command_id": req.CommandID, "error": err.Error()})
		return err
	}
	m.emit(ctx, req.SandboxID, "compute.exec.completed", map[string]any{"compute": computeName, "plugin_name": info.Name, "command_id": req.CommandID})
	return nil
}

func (m *Manager) Stdin(ctx context.Context, computeName string, req ExecStdinRequest) (ExecControlResponse, error) {
	if computeName == "" {
		computeName = m.cfg.DefaultCompute
	}
	if req.APIVersion == "" {
		req.APIVersion = APIVersionV1
	}
	alloc, _, err := m.resolve(ctx, computeName, req.APIVersion)
	if err != nil {
		return ExecControlResponse{}, err
	}
	return alloc.Stdin(ctx, req)
}

func (m *Manager) Cancel(ctx context.Context, computeName string, req ExecSignalRequest) (ExecControlResponse, error) {
	if computeName == "" {
		computeName = m.cfg.DefaultCompute
	}
	if req.APIVersion == "" {
		req.APIVersion = APIVersionV1
	}
	alloc, _, err := m.resolve(ctx, computeName, req.APIVersion)
	if err != nil {
		return ExecControlResponse{}, err
	}
	return alloc.Cancel(ctx, req)
}

func (m *Manager) Kill(ctx context.Context, computeName string, req ExecSignalRequest) (ExecControlResponse, error) {
	if computeName == "" {
		computeName = m.cfg.DefaultCompute
	}
	if req.APIVersion == "" {
		req.APIVersion = APIVersionV1
	}
	alloc, _, err := m.resolve(ctx, computeName, req.APIVersion)
	if err != nil {
		return ExecControlResponse{}, err
	}
	return alloc.Kill(ctx, req)
}

func (m *Manager) Close() error {
	m.mu.Lock()
	clients := m.clients
	builtins := m.builtins
	m.clients = map[string]*externalClient{}
	m.mu.Unlock()
	for _, c := range clients {
		c.client.Kill()
	}
	for _, b := range builtins {
		if c, ok := b.(closableCompute); ok {
			_ = c.Close()
		}
	}
	return nil
}

func (m *Manager) resolve(ctx context.Context, name, apiVersion string) (ComputeV1, PluginInfo, error) {
	m.mu.Lock()
	if b := m.builtins[name]; b != nil {
		m.mu.Unlock()
		callCtx, cancel := m.withCallTimeout(ctx)
		defer cancel()
		info, err := b.Info(callCtx)
		if err != nil {
			return nil, PluginInfo{}, err
		}
		if !HasAPIVersion(info, apiVersion) {
			return nil, info, fmt.Errorf("compute %q does not support %s", name, apiVersion)
		}
		return b, info, nil
	}
	if c := m.clients[name]; c != nil {
		m.mu.Unlock()
		if !HasAPIVersion(c.info, apiVersion) {
			return nil, c.info, fmt.Errorf("compute %q does not support %s", name, apiVersion)
		}
		return c.compute, c.info, nil
	}
	cfg, ok := m.externals[name]
	m.mu.Unlock()
	if !ok {
		return nil, PluginInfo{}, fmt.Errorf("%w: %s", ErrComputeNotFound, name)
	}
	return m.startExternal(ctx, cfg, apiVersion)
}

func (m *Manager) startExternal(ctx context.Context, cfg ExternalPluginConfig, apiVersion string) (ComputeV1, PluginInfo, error) {
	m.mu.Lock()
	if c := m.clients[cfg.Name]; c != nil {
		m.mu.Unlock()
		return c.compute, c.info, nil
	}
	m.mu.Unlock()

	cmd := exec.Command(cfg.Command, cfg.Args...)
	cmd.Env = restrictedEnv(cfg.Env)
	client := hplugin.NewClient(&hplugin.ClientConfig{
		HandshakeConfig:  Handshake,
		Plugins:          PluginSet(nil),
		Cmd:              cmd,
		AllowedProtocols: []hplugin.Protocol{hplugin.ProtocolGRPC},
		StartTimeout:     m.cfg.StartupTimeout,
		SkipHostEnv:      true,
		SyncStdout:       io.Discard,
		SyncStderr:       io.Discard,
		GRPCDialOptions:  []grpc.DialOption{grpc.WithDefaultCallOptions(grpc.ForceCodec(jsonCodec{}))},
	})
	rpcClient, err := client.Client()
	if err != nil {
		client.Kill()
		return nil, PluginInfo{}, err
	}
	raw, err := rpcClient.Dispense(PluginMapKey)
	if err != nil {
		client.Kill()
		return nil, PluginInfo{}, err
	}
	alloc, ok := raw.(ComputeV1)
	if !ok {
		client.Kill()
		return nil, PluginInfo{}, fmt.Errorf("plugin %q did not expose ComputeV1", cfg.Name)
	}
	callCtx, cancel := m.withCallTimeout(ctx)
	defer cancel()
	info, err := alloc.Info(callCtx)
	if err != nil {
		client.Kill()
		return nil, PluginInfo{}, err
	}
	if !HasAPIVersion(info, apiVersion) {
		client.Kill()
		return nil, info, fmt.Errorf("compute %q does not support %s", cfg.Name, apiVersion)
	}
	m.mu.Lock()
	m.clients[cfg.Name] = &externalClient{client: client, compute: alloc, info: info}
	m.mu.Unlock()
	return alloc, info, nil
}

func (m *Manager) withCallTimeout(ctx context.Context) (context.Context, context.CancelFunc) {
	if m.cfg.CallTimeout <= 0 {
		return context.WithCancel(ctx)
	}
	return context.WithTimeout(ctx, m.cfg.CallTimeout)
}

func (m *Manager) emit(ctx context.Context, sandboxID, eventType string, data map[string]any) {
	if m.cfg.EventSink != nil && sandboxID != "" {
		m.cfg.EventSink(ctx, sandboxID, eventType, data)
	}
}

func restrictedEnv(env map[string]string) []string {
	out := []string{"PATH=/usr/bin:/bin"}
	for k, v := range env {
		out = append(out, k+"="+v)
	}
	return out
}
