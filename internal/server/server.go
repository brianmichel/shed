package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/brianmichel/shed/internal/api"
	"github.com/brianmichel/shed/internal/model"
	"github.com/brianmichel/shed/internal/protocol"
	"github.com/brianmichel/shed/internal/store"
	"github.com/brianmichel/shed/internal/ui"
	"github.com/gorilla/websocket"
)

type Config struct {
	Addr      string
	UIEnabled bool
}

type Server struct {
	cfg      Config
	store    store.Store
	mux      *http.ServeMux
	http     *http.Server
	listener net.Listener
	upgrader websocket.Upgrader
	mu       sync.Mutex
	clients  map[string]*clientConn
}

type clientConn struct {
	session model.ClientSession
	ws      *websocket.Conn
	seq     atomic.Int64
	sent    map[string]protocol.Message
	seen    map[string]bool
	mu      sync.Mutex
}

func New(cfg Config, st store.Store) *Server {
	if cfg.Addr == "" {
		cfg.Addr = "127.0.0.1:8080"
	}
	if st == nil {
		st = store.NewMemoryStore()
	}
	s := &Server{cfg: cfg, store: st, mux: http.NewServeMux(), clients: map[string]*clientConn{}, upgrader: websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}}
	s.routes()
	return s
}

func (s *Server) Store() store.Store { return s.store }
func (s *Server) Addr() string {
	if s.listener != nil {
		return s.listener.Addr().String()
	}
	return s.cfg.Addr
}
func (s *Server) ClientURL() string { return "ws://" + s.Addr() + "/v1/client/connect" }

func (s *Server) Start(ctx context.Context) error {
	ln, err := net.Listen("tcp", s.cfg.Addr)
	if err != nil {
		return err
	}
	s.listener = ln
	s.http = &http.Server{Handler: s.mux, ReadHeaderTimeout: 5 * time.Second}
	go func() { <-ctx.Done(); _ = s.http.Shutdown(context.Background()) }()
	go s.leaseSweeper(ctx)
	log.Printf("[server] listening on http://%s", ln.Addr())
	err = s.http.Serve(ln)
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

func (s *Server) CreateSandbox(ctx context.Context, in store.SandboxCreate) (model.Sandbox, model.ClientSession, error) {
	return s.store.CreateSandbox(ctx, in)
}

func (s *Server) leaseSweeper(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			sandboxes, err := s.store.ListSandboxes(ctx)
			if err != nil {
				continue
			}
			now := time.Now().UTC()
			for _, sb := range sandboxes {
				if sb.State == model.SandboxReleased || sb.State == model.SandboxReleasing || sb.State == model.SandboxFailed {
					continue
				}
				if !sb.Lease.ExpiresAt.IsZero() && now.After(sb.Lease.ExpiresAt) {
					_, _ = s.store.AppendEvent(ctx, sb.ID, "", "sandbox.lease.expired", map[string]any{"expires_at": sb.Lease.ExpiresAt})
					s.closeClientForSandbox(sb.ID)
					_, _ = s.store.UpdateSandboxState(ctx, sb.ID, model.SandboxReleased)
				}
			}
		}
	}
}

func (s *Server) routes() {
	s.mux.HandleFunc("GET /v1/health", func(w http.ResponseWriter, r *http.Request) {
		api.WriteJSON(w, 200, map[string]any{"status": "ok", "time": time.Now().UTC()})
	})
	s.mux.HandleFunc("GET /v1/sandboxes", s.listSandboxes)
	s.mux.HandleFunc("POST /v1/sandboxes", s.createSandbox)
	s.mux.HandleFunc("GET /v1/sandboxes/{sandbox_id}", s.getSandbox)
	s.mux.HandleFunc("POST /v1/sandboxes/{sandbox_id}/release", s.releaseSandbox)
	s.mux.HandleFunc("POST /v1/sandboxes/{sandbox_id}/lease", s.extendLease)
	s.mux.HandleFunc("GET /v1/sandboxes/{sandbox_id}/events", s.sandboxEvents)
	s.mux.HandleFunc("GET /v1/sandboxes/{sandbox_id}/commands", s.listCommands)
	s.mux.HandleFunc("POST /v1/sandboxes/{sandbox_id}/commands", s.createCommand)
	s.mux.HandleFunc("GET /v1/sandboxes/{sandbox_id}/commands/{command_id}", s.getCommand)
	s.mux.HandleFunc("POST /v1/sandboxes/{sandbox_id}/commands/{command_id}/stdin", s.stdinCommand)
	s.mux.HandleFunc("POST /v1/sandboxes/{sandbox_id}/commands/{command_id}/cancel", s.cancelCommand)
	s.mux.HandleFunc("POST /v1/sandboxes/{sandbox_id}/commands/{command_id}/kill", s.killCommand)
	s.mux.HandleFunc("GET /v1/sandboxes/{sandbox_id}/commands/{command_id}/events", s.commandEvents)
	s.mux.HandleFunc("GET /v1/client/connect", s.clientConnect)
	s.mux.Handle("/ui/", ui.Handler(s.cfg.UIEnabled))
	s.mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			http.Redirect(w, r, "/ui/", http.StatusTemporaryRedirect)
			return
		}
		http.NotFound(w, r)
	})
}

func (s *Server) createSandbox(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Environment, Template string
		Lease                 struct {
			TTLMS int64 `json:"ttl_ms"`
		} `json:"lease"`
		Metadata map[string]string `json:"metadata"`
	}
	_ = json.NewDecoder(r.Body).Decode(&in)
	ttl := time.Duration(in.Lease.TTLMS) * time.Millisecond
	sb, sess, err := s.store.CreateSandbox(r.Context(), store.SandboxCreate{Environment: in.Environment, Template: in.Template, TTL: ttl, Metadata: in.Metadata})
	if err != nil {
		writeStoreErr(w, err)
		return
	}
	api.WriteJSON(w, http.StatusCreated, map[string]any{"data": sb, "client_session": sess, "connect_url": s.ClientURL()})
}
func (s *Server) listSandboxes(w http.ResponseWriter, r *http.Request) {
	xs, err := s.store.ListSandboxes(r.Context())
	if err != nil {
		writeStoreErr(w, err)
		return
	}
	api.WriteJSON(w, 200, map[string]any{"data": xs})
}
func (s *Server) getSandbox(w http.ResponseWriter, r *http.Request) {
	sb, err := s.store.GetSandbox(r.Context(), r.PathValue("sandbox_id"))
	if err != nil {
		writeStoreErr(w, err)
		return
	}
	api.WriteJSON(w, 200, map[string]any{"data": sb})
}
func (s *Server) releaseSandbox(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("sandbox_id")
	sb, err := s.store.UpdateSandboxState(r.Context(), id, model.SandboxReleasing)
	if err != nil {
		writeStoreErr(w, err)
		return
	}
	s.closeClientForSandbox(id)
	sb, err = s.store.UpdateSandboxState(r.Context(), id, model.SandboxReleased)
	if err != nil {
		writeStoreErr(w, err)
		return
	}
	api.WriteJSON(w, 200, map[string]any{"data": sb})
}
func (s *Server) extendLease(w http.ResponseWriter, r *http.Request) {
	var in struct {
		TTLMS int64 `json:"ttl_ms"`
	}
	_ = json.NewDecoder(r.Body).Decode(&in)
	lease, err := s.store.ExtendLease(r.Context(), r.PathValue("sandbox_id"), time.Duration(in.TTLMS)*time.Millisecond)
	if err != nil {
		writeStoreErr(w, err)
		return
	}
	api.WriteJSON(w, 200, map[string]any{"data": lease})
}
func (s *Server) sandboxEvents(w http.ResponseWriter, r *http.Request) {
	after := parseAfter(r)
	events, next, err := s.store.ListSandboxEvents(r.Context(), r.PathValue("sandbox_id"), after)
	if err != nil {
		writeStoreErr(w, err)
		return
	}
	writeEvents(w, r, events, next)
}

func (s *Server) createCommand(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("sandbox_id")
	var in store.CommandCreate
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil || in.Command == "" {
		api.WriteError(w, 422, "invalid_request", "command is required", false)
		return
	}
	sess, err := s.store.FindSessionBySandbox(r.Context(), id)
	if err != nil {
		writeStoreErr(w, err)
		return
	}
	cc := s.getClient(sess.SessionID)
	if cc == nil {
		api.WriteError(w, 409, "client_not_connected", "Client is not connected", true)
		return
	}
	cmd, err := s.store.CreateCommand(r.Context(), id, in)
	if err != nil {
		writeStoreErr(w, err)
		return
	}
	if err := cc.send("command.start", map[string]any{"command_id": cmd.ID, "command": cmd.Command, "cwd": cmd.Cwd, "env": cmd.Env, "stdin": cmd.Stdin, "timeout_ms": cmd.TimeoutMS}); err != nil {
		api.WriteError(w, 502, "dispatch_failed", err.Error(), true)
		return
	}
	api.WriteJSON(w, http.StatusCreated, map[string]any{"data": cmd})
}
func (s *Server) listCommands(w http.ResponseWriter, r *http.Request) {
	xs, err := s.store.ListCommands(r.Context(), r.PathValue("sandbox_id"))
	if err != nil {
		writeStoreErr(w, err)
		return
	}
	api.WriteJSON(w, 200, map[string]any{"data": xs})
}
func (s *Server) getCommand(w http.ResponseWriter, r *http.Request) {
	cmd, err := s.store.GetCommand(r.Context(), r.PathValue("sandbox_id"), r.PathValue("command_id"))
	if err != nil {
		writeStoreErr(w, err)
		return
	}
	api.WriteJSON(w, 200, map[string]any{"data": cmd})
}
func (s *Server) stdinCommand(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Data string `json:"data"`
	}
	_ = json.NewDecoder(r.Body).Decode(&in)
	s.dispatchCommandControl(w, r, "command.stdin", map[string]any{"command_id": r.PathValue("command_id"), "data": in.Data, "encoding": "utf-8"})
}
func (s *Server) cancelCommand(w http.ResponseWriter, r *http.Request) {
	var in struct {
		GraceMS int64 `json:"grace_period_ms"`
	}
	_ = json.NewDecoder(r.Body).Decode(&in)
	if in.GraceMS == 0 {
		in.GraceMS = 5000
	}
	s.dispatchCommandControl(w, r, "command.cancel", map[string]any{"command_id": r.PathValue("command_id"), "grace_period_ms": in.GraceMS})
}
func (s *Server) killCommand(w http.ResponseWriter, r *http.Request) {
	s.dispatchCommandControl(w, r, "command.kill", map[string]any{"command_id": r.PathValue("command_id")})
}
func (s *Server) commandEvents(w http.ResponseWriter, r *http.Request) {
	events, next, err := s.store.ListCommandEvents(r.Context(), r.PathValue("sandbox_id"), r.PathValue("command_id"), parseAfter(r))
	if err != nil {
		writeStoreErr(w, err)
		return
	}
	writeEvents(w, r, events, next)
}

func (s *Server) dispatchCommandControl(w http.ResponseWriter, r *http.Request, typ string, payload map[string]any) {
	sess, err := s.store.FindSessionBySandbox(r.Context(), r.PathValue("sandbox_id"))
	if err != nil {
		writeStoreErr(w, err)
		return
	}
	cc := s.getClient(sess.SessionID)
	if cc == nil {
		api.WriteError(w, 409, "client_not_connected", "Client is not connected", true)
		return
	}
	if err := cc.send(typ, payload); err != nil {
		api.WriteError(w, 502, "dispatch_failed", err.Error(), true)
		return
	}
	api.WriteJSON(w, 200, map[string]any{"data": map[string]bool{"accepted": true}})
}

func (s *Server) clientConnect(w http.ResponseWriter, r *http.Request) {
	sandboxID, key := r.URL.Query().Get("sandbox_id"), r.URL.Query().Get("session_key")
	sess, err := s.store.AuthenticateSession(r.Context(), sandboxID, key)
	if err != nil {
		api.WriteError(w, 401, "invalid_session", "Invalid client session", false)
		return
	}
	ws, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	cc := &clientConn{session: sess, ws: ws, sent: map[string]protocol.Message{}, seen: map[string]bool{}}
	s.mu.Lock()
	s.clients[sess.SessionID] = cc
	s.mu.Unlock()
	sess.State = model.SessionConnected
	_, _ = s.store.UpdateSession(context.Background(), sess)
	_, _ = s.store.AppendEvent(context.Background(), sandboxID, "", "client.connected", map[string]any{"session_id": sess.SessionID})
	go s.readClient(cc)
}

func (s *Server) readClient(cc *clientConn) {
	defer func() {
		cc.ws.Close()
		s.mu.Lock()
		delete(s.clients, cc.session.SessionID)
		s.mu.Unlock()
		sess := cc.session
		sess.State = model.SessionDisconnected
		_, _ = s.store.UpdateSession(context.Background(), sess)
		_, _ = s.store.UpdateSandboxState(context.Background(), sess.SandboxID, model.SandboxDegraded)
	}()
	for {
		var msg protocol.Message
		if err := cc.ws.ReadJSON(&msg); err != nil {
			return
		}
		if err := protocol.Validate(msg); err != nil {
			log.Printf("[server] invalid message: %v", err)
			continue
		}
		if msg.SessionID != cc.session.SessionID || msg.SandboxID != cc.session.SandboxID {
			continue
		}
		cc.mu.Lock()
		if cc.seen[msg.MessageID] {
			cc.mu.Unlock()
			continue
		}
		cc.seen[msg.MessageID] = true
		cc.mu.Unlock()
		s.handleClientMessage(cc, msg)
	}
}

func (s *Server) handleClientMessage(cc *clientConn, msg protocol.Message) {
	ctx := context.Background()
	sess := cc.session
	sess.LastClientSeqSeen = msg.Seq
	switch msg.Type {
	case "seed.hello":
		_, _ = s.store.UpdateSession(ctx, sess)
		_ = cc.send("garden.hello", map[string]any{"protocol_version": protocol.Version, "session_id": sess.SessionID, "sandbox_id": sess.SandboxID, "heartbeat_interval_ms": 10000})
	case "seed.register":
		sess.State = model.SessionRegistered
		sess.Capabilities = boolMap(msg.Payload["capabilities"])
		cc.session = sess
		_, _ = s.store.UpdateSession(ctx, sess)
		_, _ = s.store.UpdateSandboxState(ctx, sess.SandboxID, model.SandboxReady)
		_ = cc.send("garden.registered", map[string]any{"session_id": sess.SessionID, "sandbox_id": sess.SandboxID})
	case "seed.heartbeat":
		_, _ = s.store.UpdateSession(ctx, sess)
		_ = cc.send("garden.heartbeat_ack", map[string]any{"server_time": time.Now().UTC(), "status": "ok"})
	case "command.accepted":
		s.updateCommandFromPayload(ctx, msg, model.CommandStarting, "command.accepted")
	case "command.started":
		s.updateCommandFromPayload(ctx, msg, model.CommandRunning, "command.started")
	case "command.stdout", "command.stderr", "command.stdin.accepted", "command.killed", "command.failed", "command.exit":
		s.updateCommandFromPayload(ctx, msg, "", msg.Type)
	}
	if msg.ExpectsAck {
		_ = cc.sendMessage(protocol.Ack(msg, cc.seq.Add(1)))
	}
}

func (s *Server) updateCommandFromPayload(ctx context.Context, msg protocol.Message, state model.CommandState, eventType string) {
	cmdID, _ := msg.Payload["command_id"].(string)
	if cmdID == "" {
		return
	}
	cmd, err := s.store.GetCommand(ctx, msg.SandboxID, cmdID)
	if err == nil {
		now := time.Now().UTC()
		if state != "" {
			cmd.State = state
		}
		if eventType == "command.started" {
			if pid, ok := number(msg.Payload["pid"]); ok {
				cmd.PID = int(pid)
			}
			cmd.StartedAt = &now
		}
		if eventType == "command.exit" {
			code := 0
			if n, ok := number(msg.Payload["exit_code"]); ok {
				code = int(n)
			}
			cmd.ExitCode = &code
			cmd.State = model.CommandExited
			cmd.CompletedAt = &now
		}
		if eventType == "command.killed" {
			cmd.State = model.CommandKilled
			cmd.Signal = "KILL"
			cmd.CompletedAt = &now
		}
		if eventType == "command.failed" {
			cmd.State = model.CommandFailed
			cmd.CompletedAt = &now
		}
		_, _ = s.store.UpdateCommand(ctx, cmd)
	}
	_, _ = s.store.AppendEvent(ctx, msg.SandboxID, cmdID, eventType, msg.Payload)
}

func (cc *clientConn) send(typ string, payload map[string]any) error {
	return cc.sendMessage(protocol.New(typ, cc.session.SessionID, cc.session.SandboxID, cc.seq.Add(1), payload))
}
func (cc *clientConn) sendMessage(msg protocol.Message) error {
	cc.mu.Lock()
	defer cc.mu.Unlock()
	cc.sent[msg.MessageID] = msg
	return cc.ws.WriteJSON(msg)
}
func (s *Server) getClient(sessionID string) *clientConn {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.clients[sessionID]
}
func (s *Server) closeClientForSandbox(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, cc := range s.clients {
		if cc.session.SandboxID == id {
			_ = cc.ws.Close()
		}
	}
}

func parseAfter(r *http.Request) int64 {
	n, _ := strconv.ParseInt(r.URL.Query().Get("after"), 10, 64)
	return n
}
func writeEvents(w http.ResponseWriter, r *http.Request, events []model.Event, next int64) {
	if r.Header.Get("Accept") == "text/event-stream" {
		w.Header().Set("Content-Type", "text/event-stream")
		for _, ev := range events {
			b, _ := json.Marshal(ev)
			fmt.Fprintf(w, "id: %d\nevent: %s\ndata: %s\n\n", ev.Seq, ev.Type, b)
		}
		return
	}
	api.WriteJSON(w, 200, map[string]any{"data": events, "next_cursor": next})
}
func writeStoreErr(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, store.ErrSandboxNotFound):
		api.WriteError(w, 404, "sandbox_not_found", "Sandbox not found", false)
	case errors.Is(err, store.ErrCommandNotFound):
		api.WriteError(w, 404, "command_not_found", "Command not found", false)
	case errors.Is(err, store.ErrSessionNotFound):
		api.WriteError(w, 404, "session_not_found", "Session not found", false)
	default:
		api.WriteError(w, 422, err.Error(), "Request failed", false)
	}
}
func boolMap(v any) map[string]bool {
	out := map[string]bool{}
	if m, ok := v.(map[string]any); ok {
		for k, v := range m {
			if b, ok := v.(bool); ok {
				out[k] = b
			}
		}
	}
	if m, ok := v.(map[string]bool); ok {
		return m
	}
	return out
}
func number(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	default:
		return 0, false
	}
}
