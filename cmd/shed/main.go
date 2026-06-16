package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/brianmichel/shed/internal/client"
	"github.com/brianmichel/shed/internal/dev"
	"github.com/brianmichel/shed/internal/server"
	"github.com/brianmichel/shed/internal/store"
)

func main() {
	log.SetFlags(log.LstdFlags | log.Lmicroseconds)
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	var err error
	switch os.Args[1] {
	case "server":
		err = runServer(ctx, os.Args[2:])
	case "client":
		err = runClient(ctx, os.Args[2:])
	case "dev":
		err = runDev(ctx, os.Args[2:])
	case "help", "-h", "--help":
		usage()
		return
	default:
		usage()
		os.Exit(2)
	}
	if err != nil && err != context.Canceled {
		log.Fatal(err)
	}
}

func usage() { fmt.Fprintln(os.Stderr, "usage: shed <server|client|dev> [flags]") }

func runServer(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("server", flag.ExitOnError)
	addr := fs.String("addr", envOr("SHED_ADDR", "127.0.0.1:6464"), "HTTP listen address")
	uiEnabled := fs.Bool("ui", true, "serve embedded operator UI")
	_ = fs.String("config", "", "config file path (reserved)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	return server.New(server.Config{Addr: *addr, UIEnabled: *uiEnabled}, store.NewMemoryStore()).Start(ctx)
}

func runClient(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("client", flag.ExitOnError)
	serverURL := fs.String("server", envOr("SHED_SERVER_URL", "ws://127.0.0.1:6464/v1/client/connect"), "server websocket URL")
	sessionKey := fs.String("session-key", envOr("SHED_SESSION_KEY", ""), "client session key")
	sessionID := fs.String("session-id", envOr("SHED_SESSION_ID", ""), "client session id")
	sandboxID := fs.String("sandbox-id", envOr("SHED_SANDBOX_ID", ""), "sandbox id")
	workspace := fs.String("workspace-root", envOr("SHED_WORKSPACE_ROOT", "/tmp"), "workspace root")
	_ = fs.String("config", "", "config file path (reserved)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	c, err := client.New(client.Config{ServerURL: *serverURL, SessionKey: *sessionKey, SessionID: *sessionID, SandboxID: *sandboxID, WorkspaceRoot: *workspace, HeartbeatEvery: 10 * time.Second})
	if err != nil {
		return err
	}
	return c.Run(ctx)
}

func runDev(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("dev", flag.ExitOnError)
	addr := fs.String("addr", envOr("SHED_DEV_ADDR", "127.0.0.1:6464"), "HTTP listen address")
	workspace := fs.String("workspace-root", envOr("SHED_DEV_WORKSPACE", ".shed-dev/workspace"), "workspace root")
	uiEnabled := fs.Bool("ui", true, "serve embedded operator UI")
	_ = fs.String("config", "", "config file path (reserved)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	return dev.Run(ctx, dev.Config{Addr: *addr, WorkspaceRoot: *workspace, UIEnabled: *uiEnabled})
}

func envOr(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}
