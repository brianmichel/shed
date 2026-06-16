package compute

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/brianmichel/shed/internal/client"
)

type LocalConfig struct {
	WorkspaceRoot  string
	HeartbeatEvery time.Duration
}

type LocalCompute struct {
	cfg LocalConfig
	ctx context.Context
	mu  sync.Mutex
	run map[string]localRun
}

type localRun struct {
	cancel        context.CancelFunc
	workspaceRoot string
	externalID    string
	runners       map[string]*client.Runner
}

func NewLocalCompute(ctx context.Context, cfg LocalConfig) *LocalCompute {
	if cfg.WorkspaceRoot == "" {
		cfg.WorkspaceRoot = filepath.Join(os.TempDir(), "shed-workspaces")
	}
	if cfg.HeartbeatEvery == 0 {
		cfg.HeartbeatEvery = 5 * time.Second
	}
	return &LocalCompute{cfg: cfg, ctx: ctx, run: map[string]localRun{}}
}

func (a *LocalCompute) Info(context.Context) (PluginInfo, error) {
	return PluginInfo{Name: "local", Version: "0.1.0", APIVersions: []string{APIVersionV1}, Capabilities: map[string]bool{"local": true, "status": true, "renew": true, "release": true, "exec": true}}, nil
}

func (a *LocalCompute) Allocate(ctx context.Context, req AllocateRequest) (AllocateResponse, error) {
	workspaceRoot := req.Config["workspace_root"]
	if workspaceRoot == "" {
		workspaceRoot = filepath.Join(a.cfg.WorkspaceRoot, req.SandboxID)
	}
	abs, err := filepath.Abs(workspaceRoot)
	if err != nil {
		return AllocateResponse{}, err
	}
	if err := os.MkdirAll(abs, 0o755); err != nil {
		return AllocateResponse{}, err
	}

	runCtx, cancel := context.WithCancel(a.ctx)
	cli, err := client.New(client.Config{ServerURL: req.ConnectURL, SessionKey: req.SessionKey, SessionID: req.SessionID, SandboxID: req.SandboxID, WorkspaceRoot: abs, HeartbeatEvery: a.cfg.HeartbeatEvery})
	if err != nil {
		cancel()
		return AllocateResponse{}, err
	}
	externalID := "local:" + req.SandboxID
	a.mu.Lock()
	if existing := a.run[req.SandboxID]; existing.cancel != nil {
		a.mu.Unlock()
		cancel()
		return AllocateResponse{}, fmt.Errorf("local sandbox %s is already allocated", req.SandboxID)
	}
	a.run[req.SandboxID] = localRun{cancel: cancel, workspaceRoot: abs, externalID: externalID, runners: map[string]*client.Runner{}}
	a.mu.Unlock()

	go func() {
		if err := cli.Run(runCtx); err != nil && runCtx.Err() == nil {
			log.Printf("[compute/local] client stopped sandbox_id=%s: %v", req.SandboxID, err)
		}
	}()
	return AllocateResponse{ExternalID: externalID, APIVersion: APIVersionV1, PluginName: "local", PluginVersion: "0.1.0", Metadata: map[string]string{"workspace_root": abs}}, nil
}

func (a *LocalCompute) Status(_ context.Context, req StatusRequest) (StatusResponse, error) {
	a.mu.Lock()
	_, ok := a.run[req.SandboxID]
	a.mu.Unlock()
	if ok {
		return StatusResponse{State: "running"}, nil
	}
	return StatusResponse{State: "released"}, nil
}

func (a *LocalCompute) Renew(_ context.Context, req RenewRequest) (RenewResponse, error) {
	return RenewResponse{LeaseExpiresAt: req.LeaseExpiresAt}, nil
}

func (a *LocalCompute) Release(_ context.Context, req ReleaseRequest) (ReleaseResponse, error) {
	a.mu.Lock()
	run := a.run[req.SandboxID]
	delete(a.run, req.SandboxID)
	a.mu.Unlock()
	if run.cancel != nil {
		run.cancel()
	}
	for _, r := range run.runners {
		r.Kill()
	}
	return ReleaseResponse{Released: true, Metadata: map[string]string{"external_id": run.externalID, "workspace_root": run.workspaceRoot}}, nil
}

func (a *LocalCompute) Exec(_ context.Context, req ExecRequest, sink ExecEventSink) error {
	a.mu.Lock()
	run := a.run[req.SandboxID]
	a.mu.Unlock()
	if run.workspaceRoot == "" {
		return fmt.Errorf("local sandbox %s is not allocated", req.SandboxID)
	}
	cwd, err := resolveWorkspacePath(run.workspaceRoot, req.Cwd)
	if err != nil {
		return err
	}
	runner := client.NewRunner(client.CommandConfig{CommandID: req.CommandID, Command: req.Command, Cwd: cwd, Env: req.Env, Stdin: req.Stdin, Send: func(t string, payload map[string]any) {
		if sink != nil {
			_ = sink(ExecEvent{CommandID: req.CommandID, Type: t, Data: payload})
		}
	}})
	a.mu.Lock()
	run = a.run[req.SandboxID]
	if run.runners == nil {
		run.runners = map[string]*client.Runner{}
	}
	run.runners[req.CommandID] = runner
	a.run[req.SandboxID] = run
	a.mu.Unlock()
	runner.Run()
	a.mu.Lock()
	run = a.run[req.SandboxID]
	delete(run.runners, req.CommandID)
	a.run[req.SandboxID] = run
	a.mu.Unlock()
	return nil
}

func (a *LocalCompute) Stdin(_ context.Context, req ExecStdinRequest) (ExecControlResponse, error) {
	r := a.runner(req.SandboxID, req.CommandID)
	if r == nil {
		return ExecControlResponse{}, fmt.Errorf("command %s is not running", req.CommandID)
	}
	r.WriteStdin(req.Data)
	return ExecControlResponse{Accepted: true}, nil
}

func (a *LocalCompute) Cancel(_ context.Context, req ExecSignalRequest) (ExecControlResponse, error) {
	r := a.runner(req.SandboxID, req.CommandID)
	if r == nil {
		return ExecControlResponse{}, fmt.Errorf("command %s is not running", req.CommandID)
	}
	grace := time.Duration(req.GracePeriodMS) * time.Millisecond
	if grace <= 0 {
		grace = 5 * time.Second
	}
	go r.Cancel(grace)
	return ExecControlResponse{Accepted: true}, nil
}

func (a *LocalCompute) Kill(_ context.Context, req ExecSignalRequest) (ExecControlResponse, error) {
	r := a.runner(req.SandboxID, req.CommandID)
	if r == nil {
		return ExecControlResponse{}, fmt.Errorf("command %s is not running", req.CommandID)
	}
	r.Kill()
	return ExecControlResponse{Accepted: true}, nil
}

func (a *LocalCompute) runner(sandboxID, commandID string) *client.Runner {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.run[sandboxID].runners[commandID]
}

func resolveWorkspacePath(root, p string) (string, error) {
	root, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	var target string
	if p == "" || p == "/workspace" {
		target = root
	} else if rest, ok := strings.CutPrefix(p, "/workspace/"); ok {
		target = filepath.Join(root, rest)
	} else {
		return "", fmt.Errorf("path must be under /workspace")
	}
	target, err = filepath.Abs(target)
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(root, target)
	if err != nil || rel == ".." || strings.HasPrefix(rel, "../") {
		return "", fmt.Errorf("path escapes workspace")
	}
	return target, nil
}

func (a *LocalCompute) Close() error {
	a.mu.Lock()
	runs := a.run
	a.run = map[string]localRun{}
	a.mu.Unlock()
	for _, run := range runs {
		if run.cancel != nil {
			run.cancel()
		}
		for _, r := range run.runners {
			r.Kill()
		}
	}
	return nil
}
