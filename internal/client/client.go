package client

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/brianmichel/shed/internal/protocol"
	"github.com/gorilla/websocket"
)

type Config struct {
	ServerURL      string
	SessionKey     string
	SessionID      string
	SandboxID      string
	WorkspaceRoot  string
	HeartbeatEvery time.Duration
}

type Client struct {
	cfg      Config
	ws       *websocket.Conn
	seq      atomic.Int64
	lastSeen atomic.Int64
	runners  map[string]*Runner
	mu       sync.Mutex
	writeMu  sync.Mutex
}

func New(cfg Config) (*Client, error) {
	if cfg.ServerURL == "" {
		cfg.ServerURL = "ws://127.0.0.1:6464/v1/client/connect"
	}
	if cfg.WorkspaceRoot == "" {
		cfg.WorkspaceRoot = os.TempDir()
	}
	if cfg.HeartbeatEvery == 0 {
		cfg.HeartbeatEvery = 10 * time.Second
	}
	if cfg.SessionKey == "" || cfg.SessionID == "" || cfg.SandboxID == "" {
		return nil, fmt.Errorf("session-key, session-id, and sandbox-id are required")
	}
	return &Client{cfg: cfg, runners: map[string]*Runner{}}, nil
}

func (c *Client) Run(ctx context.Context) error {
	u, err := normalizeWSURL(c.cfg.ServerURL)
	if err != nil {
		return err
	}
	q := u.Query()
	q.Set("sandbox_id", c.cfg.SandboxID)
	u.RawQuery = q.Encode()
	header := http.Header{}
	header.Set("Authorization", "Bearer "+c.cfg.SessionKey)
	ws, _, err := websocket.DefaultDialer.DialContext(ctx, u.String(), header)
	if err != nil {
		return err
	}
	c.ws = ws
	defer ws.Close()
	log.Printf("[client] connected sandbox_id=%s session_id=%s workspace=%s", c.cfg.SandboxID, c.cfg.SessionID, c.cfg.WorkspaceRoot)
	if err := c.send("seed.hello", map[string]any{"seed_version": "0.1.0", "protocol_version": protocol.Version, "platform": runtime.GOOS, "arch": runtime.GOARCH}); err != nil {
		return err
	}
	go c.heartbeat(ctx)
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		var msg protocol.Message
		if err := ws.ReadJSON(&msg); err != nil {
			return err
		}
		if msg.Seq > 0 {
			c.lastSeen.Store(msg.Seq)
		}
		if err := c.handle(msg); err != nil {
			log.Printf("[client] handle %s: %v", msg.Type, err)
		}
	}
}

func (c *Client) handle(msg protocol.Message) error {
	switch msg.Type {
	case "garden.hello":
		host, _ := os.Hostname()
		return c.send("seed.register", map[string]any{"seed_id": "seed_" + c.cfg.SandboxID, "seed_version": "0.1.0", "platform": runtime.GOOS, "arch": runtime.GOARCH, "hostname": host, "process_id": os.Getpid(), "workspace_root": c.cfg.WorkspaceRoot, "capabilities": map[string]bool{"commands": true, "files": true, "pty": false}})
	case "garden.registered", "garden.heartbeat_ack", "ack":
		return nil
	case "command.start":
		return c.startCommand(msg.Payload)
	case "command.stdin":
		c.withRunner(stringValue(msg.Payload, "command_id"), func(r *Runner) { r.WriteStdin(stringValue(msg.Payload, "data")) })
		return nil
	case "command.cancel":
		grace := time.Duration(floatValue(msg.Payload, "grace_period_ms", 5000)) * time.Millisecond
		c.withRunner(stringValue(msg.Payload, "command_id"), func(r *Runner) { r.Cancel(grace) })
		return nil
	case "command.kill":
		c.withRunner(stringValue(msg.Payload, "command_id"), func(r *Runner) { r.Kill() })
		return nil
	case "file.list":
		return c.handleFileList(msg)
	case "file.read":
		return c.handleFileRead(msg)
	case "file.write":
		return c.handleFileWrite(msg)
	default:
		return nil
	}
}

func (c *Client) startCommand(p map[string]any) error {
	id := stringValue(p, "command_id")
	if id == "" {
		return fmt.Errorf("missing command_id")
	}
	cwd, err := c.resolveCwd(stringValueDefault(p, "cwd", "/workspace"))
	if err != nil {
		c.send("command.failed", map[string]any{"command_id": id, "message": err.Error()})
		return nil
	}
	r := NewRunner(CommandConfig{CommandID: id, Command: stringValue(p, "command"), Cwd: cwd, Env: stringMap(p["env"]), Stdin: boolValue(p, "stdin"), Send: func(t string, payload map[string]any) { _ = c.send(t, payload) }})
	c.mu.Lock()
	c.runners[id] = r
	c.mu.Unlock()
	go func() { r.Run(); c.mu.Lock(); delete(c.runners, id); c.mu.Unlock() }()
	return nil
}

func (c *Client) heartbeat(ctx context.Context) {
	t := time.NewTicker(c.cfg.HeartbeatEvery)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			c.mu.Lock()
			active := len(c.runners)
			c.mu.Unlock()
			_ = c.send("seed.heartbeat", map[string]any{"active_commands": active, "last_garden_seq_seen": c.lastSeen.Load(), "last_seed_seq_sent": c.seq.Load()})
		}
	}
}

func (c *Client) send(msgType string, payload map[string]any) error {
	m := protocol.New(msgType, c.cfg.SessionID, c.cfg.SandboxID, c.seq.Add(1), payload)
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	return c.ws.WriteJSON(m)
}

func (c *Client) reply(to protocol.Message, msgType string, payload map[string]any) error {
	m := protocol.New(msgType, c.cfg.SessionID, c.cfg.SandboxID, c.seq.Add(1), payload)
	m.ReplyTo = to.MessageID
	m.RequestID = to.RequestID
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	return c.ws.WriteJSON(m)
}

func (c *Client) withRunner(id string, fn func(*Runner)) {
	c.mu.Lock()
	r := c.runners[id]
	c.mu.Unlock()
	if r != nil {
		fn(r)
	}
}

func (c *Client) resolveCwd(cwd string) (string, error) {
	return c.resolveWorkspacePath(cwd)
}

func (c *Client) resolveWorkspacePath(p string) (string, error) {
	root, err := filepath.Abs(c.cfg.WorkspaceRoot)
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

func (c *Client) virtualPath(realPath string) string {
	root, err := filepath.Abs(c.cfg.WorkspaceRoot)
	if err != nil {
		return "/workspace"
	}
	rel, err := filepath.Rel(root, realPath)
	if err != nil || rel == "." {
		return "/workspace"
	}
	return "/workspace/" + filepath.ToSlash(rel)
}

func (c *Client) handleFileList(msg protocol.Message) error {
	path := stringValueDefault(msg.Payload, "path", "/workspace")
	realPath, err := c.resolveWorkspacePath(path)
	if err != nil {
		return c.reply(msg, "file.error", map[string]any{"message": err.Error(), "path": path})
	}
	entries, err := os.ReadDir(realPath)
	if err != nil {
		return c.reply(msg, "file.error", map[string]any{"message": err.Error(), "path": path})
	}
	out := make([]map[string]any, 0, len(entries))
	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil {
			continue
		}
		kind := "file"
		if entry.IsDir() {
			kind = "dir"
		}
		out = append(out, map[string]any{
			"name":        entry.Name(),
			"path":        c.virtualPath(filepath.Join(realPath, entry.Name())),
			"type":        kind,
			"size":        info.Size(),
			"mode":        info.Mode().String(),
			"modified_at": info.ModTime().UTC().Format(time.RFC3339),
		})
	}
	return c.reply(msg, "file.list.result", map[string]any{"path": path, "entries": out})
}

func (c *Client) handleFileRead(msg protocol.Message) error {
	path := stringValue(msg.Payload, "path")
	if path == "" {
		return c.reply(msg, "file.error", map[string]any{"message": "path is required"})
	}
	realPath, err := c.resolveWorkspacePath(path)
	if err != nil {
		return c.reply(msg, "file.error", map[string]any{"message": err.Error(), "path": path})
	}
	info, err := os.Stat(realPath)
	if err != nil {
		return c.reply(msg, "file.error", map[string]any{"message": err.Error(), "path": path})
	}
	if info.IsDir() {
		return c.reply(msg, "file.error", map[string]any{"message": "path is a directory", "path": path})
	}
	content, err := os.ReadFile(realPath)
	if err != nil {
		return c.reply(msg, "file.error", map[string]any{"message": err.Error(), "path": path})
	}
	return c.reply(msg, "file.read.result", map[string]any{"path": path, "content": string(content), "bytes": len(content), "mode": info.Mode().String(), "modified_at": info.ModTime().UTC().Format(time.RFC3339)})
}

func (c *Client) handleFileWrite(msg protocol.Message) error {
	path := stringValue(msg.Payload, "path")
	content := stringValue(msg.Payload, "content")
	if path == "" {
		return c.reply(msg, "file.error", map[string]any{"message": "path is required"})
	}
	realPath, err := c.resolveWorkspacePath(path)
	if err != nil {
		return c.reply(msg, "file.error", map[string]any{"message": err.Error(), "path": path})
	}
	if err := os.MkdirAll(filepath.Dir(realPath), 0o755); err != nil {
		return c.reply(msg, "file.error", map[string]any{"message": err.Error(), "path": path})
	}
	if err := os.WriteFile(realPath, []byte(content), fs.FileMode(0o644)); err != nil {
		return c.reply(msg, "file.error", map[string]any{"message": err.Error(), "path": path})
	}
	return c.reply(msg, "file.write.result", map[string]any{"path": path, "bytes": len(content)})
}

func normalizeWSURL(raw string) (*url.URL, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return nil, err
	}
	if u.Scheme == "http" {
		u.Scheme = "ws"
	}
	if u.Scheme == "https" {
		u.Scheme = "wss"
	}
	return u, nil
}
func stringValue(m map[string]any, k string) string { s, _ := m[k].(string); return s }
func stringValueDefault(m map[string]any, k, d string) string {
	if s := stringValue(m, k); s != "" {
		return s
	}
	return d
}
func boolValue(m map[string]any, k string) bool { b, _ := m[k].(bool); return b }
func floatValue(m map[string]any, k string, d float64) float64 {
	switch v := m[k].(type) {
	case float64:
		return v
	case int:
		return float64(v)
	case json.Number:
		f, _ := v.Float64()
		return f
	default:
		return d
	}
}
func stringMap(v any) map[string]string {
	out := map[string]string{}
	if m, ok := v.(map[string]any); ok {
		for k, v := range m {
			if s, ok := v.(string); ok {
				out[k] = s
			}
		}
	}
	return out
}
