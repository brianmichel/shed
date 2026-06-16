package dev

import (
	"context"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/brianmichel/shed/internal/client"
	"github.com/brianmichel/shed/internal/server"
	"github.com/brianmichel/shed/internal/store"
)

type Config struct {
	Addr, WorkspaceRoot string
	UIEnabled           bool
}

func Run(ctx context.Context, cfg Config) error {
	if cfg.Addr == "" {
		cfg.Addr = "127.0.0.1:0"
	}
	if cfg.WorkspaceRoot == "" {
		cfg.WorkspaceRoot = ".shed-dev/workspace"
	}
	abs, _ := filepath.Abs(cfg.WorkspaceRoot)
	if err := os.MkdirAll(abs, 0o755); err != nil {
		return err
	}
	st := store.NewMemoryStore()
	srv := server.New(server.Config{Addr: cfg.Addr, UIEnabled: cfg.UIEnabled}, st)
	errCh := make(chan error, 1)
	go func() { errCh <- srv.Start(ctx) }()
	for i := 0; i < 100 && srv.Addr() == cfg.Addr; i++ {
		time.Sleep(10 * time.Millisecond)
	}
	sb, sess, err := srv.CreateSandbox(ctx, store.SandboxCreate{Environment: "local", Template: "dev", TTL: 2 * time.Hour, Metadata: map[string]string{"mode": "dev"}})
	if err != nil {
		return err
	}
	cli, err := client.New(client.Config{ServerURL: srv.ClientURL(), SessionKey: sess.SessionKey, SessionID: sess.SessionID, SandboxID: sb.ID, WorkspaceRoot: abs, HeartbeatEvery: 5 * time.Second})
	if err != nil {
		return err
	}
	go func() {
		if err := cli.Run(ctx); err != nil && ctx.Err() == nil {
			log.Printf("[dev] client stopped: %v", err)
		}
	}()
	log.Printf("[dev] server: http://%s", srv.Addr())
	log.Printf("[dev] ui:     http://%s/ui/", srv.Addr())
	log.Printf("[dev] sandbox_id=%s session_id=%s workspace=%s", sb.ID, sess.SessionID, abs)
	select {
	case <-ctx.Done():
		return nil
	case err := <-errCh:
		return err
	}
}
