package store

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"sort"
	"sync"
	"time"

	"github.com/brianmichel/shed/internal/model"
)

var (
	ErrSandboxNotFound = errors.New("sandbox_not_found")
	ErrSessionNotFound = errors.New("session_not_found")
	ErrInvalidSession  = errors.New("invalid_session")
	ErrCommandNotFound = errors.New("command_not_found")
)

type MemoryStore struct {
	mu          sync.Mutex
	sandboxes   map[string]model.Sandbox
	sessions    map[string]model.ClientSession
	commands    map[string]map[string]model.Command
	events      map[string][]model.Event
	nextSeq     map[string]int64
	idempotency map[string]string
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		sandboxes:   map[string]model.Sandbox{},
		sessions:    map[string]model.ClientSession{},
		commands:    map[string]map[string]model.Command{},
		events:      map[string][]model.Event{},
		nextSeq:     map[string]int64{},
		idempotency: map[string]string{},
	}
}

func (s *MemoryStore) CreateSandbox(_ context.Context, in SandboxCreate) (model.Sandbox, model.ClientSession, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now().UTC()
	if in.TTL <= 0 {
		in.TTL = 30 * time.Minute
	}
	if in.Environment == "" {
		in.Environment = "compute"
	}
	if in.Template == "" {
		in.Template = "default"
	}
	id := newID("sbx")
	sb := model.Sandbox{ID: id, Environment: in.Environment, Template: in.Template, State: model.SandboxPendingClient, Metadata: cloneStringMap(in.Metadata), Capabilities: map[string]bool{"commands": true, "files": true, "pty": false}, Lease: model.Lease{TTLMillis: in.TTL.Milliseconds(), ExpiresAt: now.Add(in.TTL)}, InsertedAt: now, UpdatedAt: now}
	sess := model.ClientSession{SessionID: newID("sess"), SessionKey: newID("seedkey"), SandboxID: id, State: model.SessionIssued, InsertedAt: now, UpdatedAt: now}
	s.sandboxes[id] = sb
	s.sessions[sess.SessionID] = sess
	s.appendEventLocked(id, "", "sandbox.pending_client", map[string]any{"state": string(sb.State)})
	return sb, sess, nil
}

func (s *MemoryStore) ListSandboxes(_ context.Context) ([]model.Sandbox, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]model.Sandbox, 0, len(s.sandboxes))
	for _, sb := range s.sandboxes {
		out = append(out, sb)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].InsertedAt.After(out[j].InsertedAt) })
	return out, nil
}

func (s *MemoryStore) GetSandbox(_ context.Context, sandboxID string) (model.Sandbox, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	sb, ok := s.sandboxes[sandboxID]
	if !ok {
		return model.Sandbox{}, ErrSandboxNotFound
	}
	return sb, nil
}

func (s *MemoryStore) UpdateSandboxState(_ context.Context, sandboxID string, state model.SandboxState) (model.Sandbox, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	sb, ok := s.sandboxes[sandboxID]
	if !ok {
		return model.Sandbox{}, ErrSandboxNotFound
	}
	sb.State = state
	sb.UpdatedAt = time.Now().UTC()
	s.sandboxes[sandboxID] = sb
	s.appendEventLocked(sandboxID, "", "sandbox."+string(state), map[string]any{"state": string(state)})
	return sb, nil
}

func (s *MemoryStore) ExtendLease(_ context.Context, sandboxID string, ttl time.Duration) (model.Lease, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	sb, ok := s.sandboxes[sandboxID]
	if !ok {
		return model.Lease{}, ErrSandboxNotFound
	}
	if ttl <= 0 {
		ttl = time.Duration(sb.Lease.TTLMillis) * time.Millisecond
	}
	sb.Lease = model.Lease{TTLMillis: ttl.Milliseconds(), ExpiresAt: time.Now().UTC().Add(ttl)}
	sb.UpdatedAt = time.Now().UTC()
	s.sandboxes[sandboxID] = sb
	s.appendEventLocked(sandboxID, "", "sandbox.lease.extended", map[string]any{"ttl_ms": sb.Lease.TTLMillis, "expires_at": sb.Lease.ExpiresAt})
	return sb.Lease, nil
}

func (s *MemoryStore) AuthenticateSession(_ context.Context, sandboxID, sessionKey string) (model.ClientSession, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, sess := range s.sessions {
		if sess.SandboxID == sandboxID && sess.SessionKey == sessionKey {
			return sess, nil
		}
	}
	return model.ClientSession{}, ErrInvalidSession
}

func (s *MemoryStore) GetSession(_ context.Context, sessionID string) (model.ClientSession, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	sess, ok := s.sessions[sessionID]
	if !ok {
		return model.ClientSession{}, ErrSessionNotFound
	}
	return sess, nil
}

func (s *MemoryStore) FindSessionBySandbox(_ context.Context, sandboxID string) (model.ClientSession, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, sess := range s.sessions {
		if sess.SandboxID == sandboxID {
			return sess, nil
		}
	}
	return model.ClientSession{}, ErrSessionNotFound
}

func (s *MemoryStore) UpdateSession(_ context.Context, sess model.ClientSession) (model.ClientSession, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.sessions[sess.SessionID]; !ok {
		return model.ClientSession{}, ErrSessionNotFound
	}
	sess.UpdatedAt = time.Now().UTC()
	s.sessions[sess.SessionID] = sess
	return sess, nil
}

func (s *MemoryStore) CreateCommand(_ context.Context, sandboxID string, in CommandCreate) (model.Command, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.sandboxes[sandboxID]; !ok {
		return model.Command{}, ErrSandboxNotFound
	}
	now := time.Now().UTC()
	if in.Cwd == "" {
		in.Cwd = "/workspace"
	}
	if in.TimeoutMS == 0 {
		in.TimeoutMS = 60000
	}
	cmd := model.Command{ID: newID("cmd"), SandboxID: sandboxID, State: model.CommandQueued, Command: in.Command, Cwd: in.Cwd, Env: cloneStringMap(in.Env), Stdin: in.Stdin, TimeoutMS: in.TimeoutMS, Metadata: cloneStringMap(in.Metadata), InsertedAt: now, UpdatedAt: now}
	if s.commands[sandboxID] == nil {
		s.commands[sandboxID] = map[string]model.Command{}
	}
	s.commands[sandboxID][cmd.ID] = cmd
	s.appendEventLocked(sandboxID, cmd.ID, "command.queued", map[string]any{"command": cmd.Command})
	return cmd, nil
}

func (s *MemoryStore) ListCommands(_ context.Context, sandboxID string) ([]model.Command, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.sandboxes[sandboxID]; !ok {
		return nil, ErrSandboxNotFound
	}
	out := []model.Command{}
	for _, cmd := range s.commands[sandboxID] {
		out = append(out, cmd)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].InsertedAt.Before(out[j].InsertedAt) })
	return out, nil
}

func (s *MemoryStore) GetCommand(_ context.Context, sandboxID, commandID string) (model.Command, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	cmd, ok := s.commands[sandboxID][commandID]
	if !ok {
		return model.Command{}, ErrCommandNotFound
	}
	return cmd, nil
}

func (s *MemoryStore) UpdateCommand(_ context.Context, cmd model.Command) (model.Command, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.commands[cmd.SandboxID][cmd.ID]; !ok {
		return model.Command{}, ErrCommandNotFound
	}
	cmd.UpdatedAt = time.Now().UTC()
	s.commands[cmd.SandboxID][cmd.ID] = cmd
	return cmd, nil
}

func (s *MemoryStore) AppendEvent(_ context.Context, sandboxID, commandID, eventType string, data map[string]any) (model.Event, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.sandboxes[sandboxID]; !ok {
		return model.Event{}, ErrSandboxNotFound
	}
	return s.appendEventLocked(sandboxID, commandID, eventType, data), nil
}

func (s *MemoryStore) ListSandboxEvents(_ context.Context, sandboxID string, after int64) ([]model.Event, int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.sandboxes[sandboxID]; !ok {
		return nil, after, ErrSandboxNotFound
	}
	return filterEvents(s.events[sandboxID], "", after, false)
}

func (s *MemoryStore) ListCommandEvents(_ context.Context, sandboxID, commandID string, after int64) ([]model.Event, int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.commands[sandboxID][commandID]; !ok {
		return nil, after, ErrCommandNotFound
	}
	return filterEvents(s.events[sandboxID], commandID, after, true)
}

func (s *MemoryStore) RememberIdempotencyKey(_ context.Context, key, value string) (string, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if existing, ok := s.idempotency[key]; ok {
		return existing, false, nil
	}
	s.idempotency[key] = value
	return value, true, nil
}

func (s *MemoryStore) appendEventLocked(sandboxID, commandID, eventType string, data map[string]any) model.Event {
	s.nextSeq[sandboxID]++
	ev := model.Event{ID: newID("evt"), SandboxID: sandboxID, CommandID: commandID, Seq: s.nextSeq[sandboxID], Type: eventType, Timestamp: time.Now().UTC(), Data: data}
	s.events[sandboxID] = append(s.events[sandboxID], ev)
	return ev
}

func filterEvents(events []model.Event, commandID string, after int64, commandOnly bool) ([]model.Event, int64, error) {
	out := []model.Event{}
	next := after
	for _, ev := range events {
		if ev.Seq <= after {
			continue
		}
		if commandOnly && ev.CommandID != commandID {
			continue
		}
		out = append(out, ev)
		if ev.Seq > next {
			next = ev.Seq
		}
	}
	return out, next, nil
}

func cloneStringMap(in map[string]string) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func newID(prefix string) string {
	b := make([]byte, 6)
	_, _ = rand.Read(b)
	return prefix + "_" + hex.EncodeToString(b)
}
