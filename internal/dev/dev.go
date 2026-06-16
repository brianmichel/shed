package dev

import (
	"context"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/brianmichel/shed/internal/compute"
	"github.com/brianmichel/shed/internal/server"
	"github.com/brianmichel/shed/internal/store"
)

type Config struct {
	Addr, WorkspaceRoot string
	UIEnabled           bool
	DefaultCompute      string
	ExternalComputes    []compute.ExternalPluginConfig
}

func Run(ctx context.Context, cfg Config) error {
	if cfg.Addr == "" {
		cfg.Addr = "127.0.0.1:6464"
	}
	if cfg.WorkspaceRoot == "" {
		cfg.WorkspaceRoot = ".shed-dev/workspace"
	}
	abs, _ := filepath.Abs(cfg.WorkspaceRoot)
	if err := os.MkdirAll(abs, 0o755); err != nil {
		return err
	}
	st := store.NewMemoryStore()
	if cfg.DefaultCompute == "" {
		cfg.DefaultCompute = "local"
	}
	mgr := compute.NewManager(compute.ManagerConfig{DefaultCompute: cfg.DefaultCompute})
	_ = mgr.RegisterBuiltin("local", compute.NewLocalCompute(ctx, compute.LocalConfig{WorkspaceRoot: abs, HeartbeatEvery: 5 * time.Second}))
	for _, ext := range cfg.ExternalComputes {
		if err := mgr.RegisterExternal(ext); err != nil {
			return err
		}
	}
	srv := server.New(server.Config{Addr: cfg.Addr, UIEnabled: cfg.UIEnabled, ComputeManager: mgr, DefaultCompute: cfg.DefaultCompute}, st)
	errCh := make(chan error, 1)
	go func() { errCh <- srv.Start(ctx) }()
	for i := 0; i < 100 && srv.Addr() == cfg.Addr; i++ {
		time.Sleep(10 * time.Millisecond)
	}
	sb, sess, err := srv.CreateSandbox(ctx, store.SandboxCreate{Environment: "local", Template: "dev", TTL: 2 * time.Hour, Compute: cfg.DefaultCompute, Metadata: map[string]string{"mode": "dev"}})
	if err != nil {
		return err
	}
	log.Printf("[dev] server: http://%s", srv.Addr())
	log.Printf("[dev] ui:     http://%s/ui/", srv.Addr())
	log.Printf("[dev] sandbox_id=%s session_id=%s workspace=%s", sb.ID, sess.SessionID, sb.ComputeMetadata["workspace_root"])
	select {
	case <-ctx.Done():
		return nil
	case err := <-errCh:
		return err
	}
}
