package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/brianmichel/shed/seed/internal/connection"
)

func main() {
	url := flag.String("url", envOr("GARDEN_URL", "ws://localhost:4000/ws/seed"), "Garden WebSocket URL")
	sessionKey := flag.String("session-key", envOr("GARDEN_SESSION_KEY", ""), "Garden session key")
	sandboxID := flag.String("sandbox-id", envOr("GARDEN_SANDBOX_ID", ""), "Sandbox ID")
	sessionID := flag.String("session-id", envOr("GARDEN_SESSION_ID", ""), "Session ID assigned by Garden")
	workspaceRoot := flag.String("workspace-root", envOr("GARDEN_WORKSPACE_ROOT", "/tmp"), "Absolute path to the sandbox workspace on disk")
	flag.Parse()

	if *sessionKey == "" {
		fmt.Fprintln(os.Stderr, "error: --session-key or GARDEN_SESSION_KEY is required")
		os.Exit(1)
	}
	if *sandboxID == "" {
		fmt.Fprintln(os.Stderr, "error: --sandbox-id or GARDEN_SANDBOX_ID is required")
		os.Exit(1)
	}
	if *sessionID == "" {
		fmt.Fprintln(os.Stderr, "error: --session-id or GARDEN_SESSION_ID is required")
		os.Exit(1)
	}

	conn, err := connection.New(*url, *sessionKey, *sandboxID, *sessionID, *workspaceRoot)
	if err != nil {
		log.Fatalf("failed to connect: %v", err)
	}

	if err := conn.Run(); err != nil {
		log.Fatalf("connection error: %v", err)
	}
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
