package connection

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log"
	"os"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/brianmichel/shed/seed/internal/protocol"
	"github.com/brianmichel/shed/seed/internal/runner"
	"github.com/brianmichel/shed/seed/internal/transport"
)

const defaultHeartbeatInterval = 10 * time.Second

// Conn manages the Garden session: handshake, heartbeat, and command dispatch.
type Conn struct {
	sandboxID     string
	sessionID     string
	sessionKey    string
	workspaceRoot string
	t             *transport.Transport
	seq           atomic.Int64
	lastGardenSeq atomic.Int64
	heartbeat     time.Duration
	startTime     time.Time
	runners       map[string]*runner.Runner
	runnersMu     sync.Mutex
}

func New(wsURL, sessionKey, sandboxID, sessionID, workspaceRoot string) (*Conn, error) {
	t, err := transport.New(wsURL, sessionKey, sandboxID)
	if err != nil {
		return nil, err
	}
	return &Conn{
		sandboxID:     sandboxID,
		sessionID:     sessionID,
		sessionKey:    sessionKey,
		workspaceRoot: workspaceRoot,
		t:             t,
		heartbeat:     defaultHeartbeatInterval,
		startTime:     time.Now(),
		runners:       make(map[string]*runner.Runner),
	}, nil
}

func (c *Conn) Run() error {
	log.Printf("[seed] connected to Garden, sandbox_id=%s session_id=%s", c.sandboxID, c.sessionID)

	hostname, _ := os.Hostname()
	if err := c.send(protocol.SeedHello, map[string]any{
		"seed_version":     "0.1.0",
		"protocol_version": protocol.Version,
		"platform":         runtime.GOOS,
		"arch":             runtime.GOARCH,
		"hostname":         hostname,
		"session_key":      c.sessionKey,
	}); err != nil {
		return err
	}

	go c.t.ReadLoop()

	for msg := range c.t.Messages() {
		if msg.Seq > 0 {
			c.lastGardenSeq.Store(msg.Seq)
		}
		if err := c.handle(msg); err != nil {
			log.Printf("[seed] error handling %s: %v", msg.Type, err)
		}
	}
	return fmt.Errorf("connection closed")
}

func (c *Conn) handle(msg transport.Message) error {
	p := msg.Payload
	if err := protocol.ValidatePayload(msg.Type, p); err != nil {
		return err
	}

	switch msg.Type {
	case protocol.GardenHello:
		if ms, ok := p["heartbeat_interval_ms"].(float64); ok {
			c.heartbeat = time.Duration(ms) * time.Millisecond
		}
		log.Printf("[seed] garden.hello heartbeat_interval=%v", c.heartbeat)
		go c.heartbeatLoop()

		hostname, _ := os.Hostname()
		return c.send(protocol.SeedRegister, map[string]any{
			"seed_id":        fmt.Sprintf("seed_%s", c.sandboxID),
			"seed_version":   "0.1.0",
			"platform":       runtime.GOOS,
			"arch":           runtime.GOARCH,
			"hostname":       hostname,
			"boot_time":      c.startTime.UTC().Format(time.RFC3339),
			"process_id":     os.Getpid(),
			"workspace_root": c.workspaceRoot,
		})

	case protocol.GardenRegistered:
		log.Printf("[seed] registered")
		hostname, _ := os.Hostname()
		return c.send(protocol.SeedStatus, map[string]any{
			"state":        "ready",
			"seed_version": "0.1.0",
			"platform":     runtime.GOOS,
			"arch":         runtime.GOARCH,
			"hostname":     hostname,
		})

	case protocol.GardenHeartbeatAck:
		return nil

	case protocol.GardenLeaseWarning:
		log.Printf("[seed] lease warning: %ds remaining", msToSec(p, "remaining_ms"))
		return nil

	case protocol.GardenLeaseExpiring:
		log.Printf("[seed] lease expiring: %ds remaining", msToSec(p, "remaining_ms"))
		return nil

	case protocol.CommandStart:
		return c.startCommand(p)

	case protocol.CommandCancel:
		cmdID, _ := p["command_id"].(string)
		grace := 5000.0
		if g, ok := p["grace_period_ms"].(float64); ok {
			grace = g
		}
		c.withRunner(cmdID, func(r *runner.Runner) { r.Cancel(time.Duration(grace) * time.Millisecond) })
		return nil

	case protocol.CommandKill:
		cmdID, _ := p["command_id"].(string)
		c.withRunner(cmdID, func(r *runner.Runner) { r.Kill() })
		return nil

	case protocol.CommandStdin:
		cmdID, _ := p["command_id"].(string)
		data, _ := p["data"].(string)
		c.withRunner(cmdID, func(r *runner.Runner) { r.WriteStdin(data) })
		return nil

	default:
		log.Printf("[seed] unhandled message type=%s", msg.Type)
		return nil
	}
}

func (c *Conn) startCommand(p map[string]any) error {
	cmdID, _ := p["command_id"].(string)
	if cmdID == "" {
		return fmt.Errorf("command.start missing command_id")
	}

	r := runner.New(runner.Config{
		CommandID:     cmdID,
		Command:       p["command"].(string),
		Cwd:           c.resolveCwd(stringOrDefault(p, "cwd", "/workspace")),
		Env:           toStringMap(p["env"]),
		Stdin:         toBool(p["stdin"]),
		Send:          func(msgType string, payload map[string]any) { _ = c.send(msgType, payload) },
	})

	c.runnersMu.Lock()
	c.runners[cmdID] = r
	c.runnersMu.Unlock()

	go func() {
		r.Run()
		c.runnersMu.Lock()
		delete(c.runners, cmdID)
		c.runnersMu.Unlock()
	}()

	return nil
}

func (c *Conn) heartbeatLoop() {
	for {
		time.Sleep(c.heartbeat)
		c.runnersMu.Lock()
		active := len(c.runners)
		c.runnersMu.Unlock()

		if err := c.send(protocol.SeedHeartbeat, map[string]any{
			"uptime_ms":             time.Since(c.startTime).Milliseconds(),
			"active_commands":       active,
			"last_garden_seq_seen":  c.lastGardenSeq.Load(),
			"last_seed_seq_sent":    c.seq.Load(),
			"connection_generation": 0,
		}); err != nil {
			log.Printf("[seed] heartbeat error: %v", err)
			return
		}
	}
}

func (c *Conn) send(msgType string, payload map[string]any) error {
	if err := protocol.ValidatePayload(msgType, payload); err != nil {
		return err
	}

	seq := c.seq.Add(1)
	return c.t.Send(map[string]any{
		"version":     protocol.Version,
		"seq":         seq,
		"session_id":  c.sessionID,
		"sandbox_id":  c.sandboxID,
		"message_id":  "msg_" + randHex(8),
		"request_id":  randHex(8),
		"timestamp":   time.Now().UTC().Format(time.RFC3339),
		"type":        msgType,
		"payload":     payload,
		"expects_ack": false,
	})
}

func (c *Conn) withRunner(cmdID string, fn func(*runner.Runner)) {
	c.runnersMu.Lock()
	r, ok := c.runners[cmdID]
	c.runnersMu.Unlock()
	if ok {
		fn(r)
	} else {
		log.Printf("[seed] no runner for command_id=%s", cmdID)
	}
}

func randHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// resolveCwd maps virtual /workspace paths to the real workspace root on disk.
func (c *Conn) resolveCwd(cwd string) string {
	if cwd == "/workspace" {
		return c.workspaceRoot
	}
	if after, ok := strings.CutPrefix(cwd, "/workspace/"); ok {
		return c.workspaceRoot + "/" + after
	}
	return cwd
}

func msToSec(p map[string]any, key string) int64 {
	ms, _ := p[key].(float64)
	return int64(ms) / 1000
}

func stringOrDefault(m map[string]any, key, def string) string {
	if v, ok := m[key].(string); ok && v != "" {
		return v
	}
	return def
}

func toBool(v any) bool {
	b, _ := v.(bool)
	return b
}

func toStringMap(v any) map[string]string {
	out := make(map[string]string)
	m, ok := v.(map[string]any)
	if !ok {
		return out
	}
	for k, val := range m {
		if s, ok := val.(string); ok {
			out[k] = s
		}
	}
	return out
}
