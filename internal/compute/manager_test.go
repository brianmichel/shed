package compute

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestMain(m *testing.M) {
	if os.Getenv("SHED_TEST_COMPUTE_PLUGIN") == "1" {
		apiVersions := []string{APIVersionV1}
		if v := os.Getenv("SHED_TEST_COMPUTE_API"); v != "" {
			apiVersions = []string{v}
		}
		ServePlugin(&fakeCompute{name: "external-test", version: "1.2.3", apiVersions: apiVersions})
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func TestManagerListDrivers(t *testing.T) {
	ctx := context.Background()
	mgr := NewManager(ManagerConfig{DefaultCompute: "local"})
	root := t.TempDir()
	if err := mgr.RegisterBuiltin("local", NewLocalCompute(ctx, LocalConfig{WorkspaceRoot: root})); err != nil {
		t.Fatal(err)
	}
	if err := mgr.RegisterExternal(ExternalPluginConfig{Name: "external", Command: "/bin/false", APIVersion: APIVersionV1, Env: map[string]string{"FOO": "bar"}}); err != nil {
		t.Fatal(err)
	}
	drivers := mgr.ListDrivers(ctx)
	if len(drivers) != 2 {
		t.Fatalf("drivers=%#v", drivers)
	}
	byName := map[string]DriverDescriptor{}
	for _, d := range drivers {
		byName[d.Name] = d
	}
	local := byName["local"]
	if local.Kind != "builtin" || !local.Default || !local.Loaded || local.Plugin == nil || local.Plugin.Name != "local" {
		t.Fatalf("local=%#v", local)
	}
	if local.Config["workspace_root"] != root {
		t.Fatalf("local config=%#v", local.Config)
	}
	external := byName["external"]
	if external.Kind != "external" || external.Loaded || external.Command != "/bin/false" || len(external.EnvKeys) != 1 || external.EnvKeys[0] != "FOO" {
		t.Fatalf("external=%#v", external)
	}
}

func TestManagerBuiltinLifecycle(t *testing.T) {
	ctx := context.Background()
	fake := &fakeCompute{name: "builtin-test", version: "0.1.0", apiVersions: []string{APIVersionV1}}
	var events []string
	mgr := NewManager(ManagerConfig{DefaultCompute: "test", EventSink: func(_ context.Context, _ string, eventType string, _ map[string]any) {
		events = append(events, eventType)
	}})
	if err := mgr.RegisterBuiltin("test", fake); err != nil {
		t.Fatal(err)
	}
	resp, err := mgr.Allocate(ctx, AllocateRequest{SandboxID: "sbx_1", SessionID: "sess_1", SessionKey: "key", ConnectURL: "ws://127.0.0.1/client"})
	if err != nil {
		t.Fatal(err)
	}
	if resp.ExternalID != "external-sbx_1" || fake.allocateCalls != 1 {
		t.Fatalf("unexpected allocate response/calls: %#v calls=%d", resp, fake.allocateCalls)
	}
	if _, err := mgr.Status(ctx, "test", StatusRequest{SandboxID: "sbx_1", ExternalID: resp.ExternalID}); err != nil {
		t.Fatal(err)
	}
	if _, err := mgr.Renew(ctx, "test", RenewRequest{SandboxID: "sbx_1", ExternalID: resp.ExternalID, LeaseExpiresAt: time.Now().Add(time.Hour)}); err != nil {
		t.Fatal(err)
	}
	var execEvents []ExecEvent
	if err := mgr.Exec(ctx, "test", ExecRequest{SandboxID: "sbx_1", ExternalID: resp.ExternalID, CommandID: "cmd_1", Command: "true"}, func(ev ExecEvent) error {
		execEvents = append(execEvents, ev)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if len(execEvents) != 1 || execEvents[0].Type != "command.exit" {
		t.Fatalf("execEvents=%#v", execEvents)
	}
	if _, err := mgr.Release(ctx, "test", ReleaseRequest{SandboxID: "sbx_1", ExternalID: resp.ExternalID}); err != nil {
		t.Fatal(err)
	}
	want := []string{"compute.allocate.started", "compute.allocate.succeeded", "compute.status", "compute.renewed", "compute.exec.started", "compute.exec.completed", "compute.release.started", "compute.release.succeeded"}
	if len(events) != len(want) {
		t.Fatalf("events=%v want=%v", events, want)
	}
	for i := range want {
		if events[i] != want[i] {
			t.Fatalf("events=%v want=%v", events, want)
		}
	}
}

func TestManagerExternalPluginLifecycle(t *testing.T) {
	ctx := context.Background()
	mgr := NewManager(ManagerConfig{DefaultCompute: "external", StartupTimeout: 5 * time.Second, CallTimeout: 5 * time.Second})
	if err := mgr.RegisterExternal(ExternalPluginConfig{Name: "external", Command: os.Args[0], Env: map[string]string{"SHED_TEST_COMPUTE_PLUGIN": "1"}}); err != nil {
		t.Fatal(err)
	}
	defer mgr.Close()
	resp, err := mgr.Allocate(ctx, AllocateRequest{SandboxID: "sbx_ext", SessionID: "sess", SessionKey: "key", ConnectURL: "ws://127.0.0.1/client"})
	if err != nil {
		t.Fatal(err)
	}
	if resp.PluginName != "external-test" || resp.PluginVersion != "1.2.3" || resp.ExternalID != "external-sbx_ext" {
		t.Fatalf("unexpected external response: %#v", resp)
	}
	var execEvents []ExecEvent
	if err := mgr.Exec(ctx, "external", ExecRequest{SandboxID: "sbx_ext", ExternalID: resp.ExternalID, CommandID: "cmd_ext", Command: "true"}, func(ev ExecEvent) error {
		execEvents = append(execEvents, ev)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if len(execEvents) != 1 || execEvents[0].Type != "command.exit" {
		t.Fatalf("execEvents=%#v", execEvents)
	}
	if _, err := mgr.Release(ctx, "external", ReleaseRequest{SandboxID: "sbx_ext", ExternalID: resp.ExternalID}); err != nil {
		t.Fatal(err)
	}
}

func TestManagerVersionMismatch(t *testing.T) {
	ctx := context.Background()
	mgr := NewManager(ManagerConfig{DefaultCompute: "external", StartupTimeout: 5 * time.Second, CallTimeout: 5 * time.Second})
	if err := mgr.RegisterExternal(ExternalPluginConfig{Name: "external", Command: os.Args[0], Env: map[string]string{"SHED_TEST_COMPUTE_PLUGIN": "1", "SHED_TEST_COMPUTE_API": "compute.v0"}}); err != nil {
		t.Fatal(err)
	}
	defer mgr.Close()
	_, err := mgr.Allocate(ctx, AllocateRequest{APIVersion: APIVersionV1, SandboxID: "sbx_bad", SessionID: "sess", SessionKey: "key", ConnectURL: "ws://127.0.0.1/client"})
	if err == nil {
		t.Fatal("expected version mismatch")
	}
}

func TestLocalComputeReleaseCleanup(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	root := t.TempDir()
	alloc := NewLocalCompute(ctx, LocalConfig{WorkspaceRoot: root, HeartbeatEvery: time.Hour})
	resp, err := alloc.Allocate(ctx, AllocateRequest{SandboxID: "sbx_local", SessionID: "sess", SessionKey: "key", ConnectURL: "ws://127.0.0.1:1/v1/client/connect"})
	if err != nil {
		t.Fatal(err)
	}
	workspace := filepath.Join(root, "sbx_local")
	if resp.Metadata["workspace_root"] != workspace {
		t.Fatalf("workspace=%q want %q", resp.Metadata["workspace_root"], workspace)
	}
	if _, err := os.Stat(workspace); err != nil {
		t.Fatal(err)
	}
	if _, err := alloc.Release(ctx, ReleaseRequest{SandboxID: "sbx_local", ExternalID: resp.ExternalID}); err != nil {
		t.Fatal(err)
	}
	status, err := alloc.Status(ctx, StatusRequest{SandboxID: "sbx_local"})
	if err != nil {
		t.Fatal(err)
	}
	if status.State == "running" {
		t.Fatal("expected released local sandbox not to report running")
	}
}

type fakeCompute struct {
	name          string
	version       string
	apiVersions   []string
	allocateErr   error
	allocateCalls int
}

func (f *fakeCompute) Info(context.Context) (PluginInfo, error) {
	return PluginInfo{Name: f.name, Version: f.version, APIVersions: f.apiVersions, Capabilities: map[string]bool{"status": true, "renew": true, "release": true, "exec": true}}, nil
}
func (f *fakeCompute) Allocate(_ context.Context, req AllocateRequest) (AllocateResponse, error) {
	f.allocateCalls++
	if f.allocateErr != nil {
		return AllocateResponse{}, f.allocateErr
	}
	return AllocateResponse{ExternalID: "external-" + req.SandboxID, APIVersion: req.APIVersion, PluginName: f.name, PluginVersion: f.version, Metadata: map[string]string{"allocated": "true"}}, nil
}
func (f *fakeCompute) Status(context.Context, StatusRequest) (StatusResponse, error) {
	return StatusResponse{State: "running"}, nil
}
func (f *fakeCompute) Renew(_ context.Context, req RenewRequest) (RenewResponse, error) {
	return RenewResponse{LeaseExpiresAt: req.LeaseExpiresAt}, nil
}
func (f *fakeCompute) Release(context.Context, ReleaseRequest) (ReleaseResponse, error) {
	return ReleaseResponse{Released: true}, nil
}
func (f *fakeCompute) Exec(_ context.Context, req ExecRequest, sink ExecEventSink) error {
	return sink(ExecEvent{CommandID: req.CommandID, Type: "command.exit", Data: map[string]any{"command_id": req.CommandID, "exit_code": 0}})
}
func (f *fakeCompute) Stdin(context.Context, ExecStdinRequest) (ExecControlResponse, error) {
	return ExecControlResponse{Accepted: true}, nil
}
func (f *fakeCompute) Cancel(context.Context, ExecSignalRequest) (ExecControlResponse, error) {
	return ExecControlResponse{Accepted: true}, nil
}
func (f *fakeCompute) Kill(context.Context, ExecSignalRequest) (ExecControlResponse, error) {
	return ExecControlResponse{Accepted: true}, nil
}
