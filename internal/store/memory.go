package store

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"sort"
	"sync"
	"time"

	"github.com/brianmichel/shed/internal/model"
)

var (
	ErrSandboxNotFound    = errors.New("sandbox_not_found")
	ErrSessionNotFound    = errors.New("session_not_found")
	ErrInvalidSession     = errors.New("invalid_session")
	ErrInvalidAPIToken    = errors.New("invalid_api_token")
	ErrCommandNotFound    = errors.New("command_not_found")
	ErrWorkItemNotFound   = errors.New("work_item_not_found")
	ErrAgentRunNotFound   = errors.New("agent_run_not_found")
	ErrRepositoryNotFound = errors.New("repository_not_found")
)

type MemoryStore struct {
	mu             sync.Mutex
	sandboxes      map[string]model.Sandbox
	sessions       map[string]model.ClientSession
	commands       map[string]map[string]model.Command
	apiTokens      map[string]model.APIToken
	workItems      map[string]model.WorkItem
	agentRuns      map[string]model.AgentRun
	repositories   map[string]model.Repository
	events         map[string][]model.Event
	nextSeq        map[string]int64
	factoryEvents  map[string][]model.Event
	nextFactorySeq map[string]int64
	idempotency    map[string]string
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		sandboxes:      map[string]model.Sandbox{},
		sessions:       map[string]model.ClientSession{},
		commands:       map[string]map[string]model.Command{},
		apiTokens:      map[string]model.APIToken{},
		workItems:      map[string]model.WorkItem{},
		agentRuns:      map[string]model.AgentRun{},
		repositories:   map[string]model.Repository{},
		events:         map[string][]model.Event{},
		nextSeq:        map[string]int64{},
		factoryEvents:  map[string][]model.Event{},
		nextFactorySeq: map[string]int64{},
		idempotency:    map[string]string{},
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
	sb := model.Sandbox{ID: id, Environment: in.Environment, Template: in.Template, State: model.SandboxPendingClient, Compute: in.Compute, ComputeAPIVersion: in.ComputeAPIVersion, ComputeConfig: cloneStringMap(in.ComputeConfig), Metadata: cloneStringMap(in.Metadata), Capabilities: map[string]bool{"commands": true, "files": true, "pty": false}, Lease: model.Lease{TTLMillis: in.TTL.Milliseconds(), ExpiresAt: now.Add(in.TTL)}, InsertedAt: now, UpdatedAt: now}
	key := newID("seedkey")
	sess := model.ClientSession{SessionID: newID("sess"), AgentTokenHash: tokenHash(key), SandboxID: id, State: model.SessionIssued, InsertedAt: now, UpdatedAt: now}
	s.sandboxes[id] = sb
	s.sessions[sess.SessionID] = sess
	s.appendEventLocked(id, "", "server.store", "sandbox.pending_client", map[string]any{"state": string(sb.State)})
	sess.AgentToken = key
	return sb, sess, nil
}

func (s *MemoryStore) ListSandboxes(_ context.Context, opts SandboxListOptions) ([]model.Sandbox, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]model.Sandbox, 0, len(s.sandboxes))
	for _, sb := range s.sandboxes {
		if opts.State != "" && sb.State != opts.State {
			continue
		}
		out = append(out, sb)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].InsertedAt.After(out[j].InsertedAt) })
	return pageSlice(out, opts.Page), nil
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
	s.appendEventLocked(sandboxID, "", "server.store", "sandbox."+string(state), map[string]any{"state": string(state)})
	return sb, nil
}

func (s *MemoryStore) UpdateSandboxAllocation(_ context.Context, sandboxID string, in SandboxAllocationUpdate) (model.Sandbox, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	sb, ok := s.sandboxes[sandboxID]
	if !ok {
		return model.Sandbox{}, ErrSandboxNotFound
	}
	if in.Compute != "" {
		sb.Compute = in.Compute
	}
	if in.ComputeAPIVersion != "" {
		sb.ComputeAPIVersion = in.ComputeAPIVersion
	}
	if in.ComputePluginVersion != "" {
		sb.ComputePluginVersion = in.ComputePluginVersion
	}
	if in.ExternalAllocationID != "" {
		sb.ExternalAllocationID = in.ExternalAllocationID
	}
	if in.ComputeMetadata != nil {
		sb.ComputeMetadata = cloneStringMap(in.ComputeMetadata)
	}
	sb.UpdatedAt = time.Now().UTC()
	s.sandboxes[sandboxID] = sb
	s.appendEventLocked(sandboxID, "", "server.store", "sandbox.allocation.updated", map[string]any{"compute": sb.Compute, "compute_api_version": sb.ComputeAPIVersion, "compute_plugin_version": sb.ComputePluginVersion, "external_allocation_id": sb.ExternalAllocationID})
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
	s.appendEventLocked(sandboxID, "", "server.store", "sandbox.lease.extended", map[string]any{"ttl_ms": sb.Lease.TTLMillis, "expires_at": sb.Lease.ExpiresAt})
	return sb.Lease, nil
}

func (s *MemoryStore) AuthenticateSession(_ context.Context, sandboxID, agentToken string) (model.ClientSession, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, sess := range s.sessions {
		if sess.SandboxID == sandboxID && subtle.ConstantTimeCompare([]byte(sess.AgentTokenHash), []byte(tokenHash(agentToken))) == 1 {
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
	current := s.sessions[sess.SessionID]
	if sess.AgentTokenHash == "" {
		sess.AgentTokenHash = current.AgentTokenHash
	}
	sess.AgentToken = ""
	sess.UpdatedAt = time.Now().UTC()
	s.sessions[sess.SessionID] = sess
	return sess, nil
}

func (s *MemoryStore) CreateAPIToken(_ context.Context, in APITokenCreate) (APITokenCreateResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now().UTC()
	secret := newID("shd")
	if in.Name == "" {
		in.Name = "api-token"
	}
	tok := model.APIToken{ID: newID("atok"), Name: in.Name, TokenHash: tokenHash(secret), TokenPrefix: tokenPrefix(secret), Metadata: cloneStringMap(in.Metadata), InsertedAt: now, UpdatedAt: now}
	s.apiTokens[tok.ID] = tok
	return APITokenCreateResult{Token: tok, Secret: secret}, nil
}

func (s *MemoryStore) ListAPITokens(_ context.Context) ([]model.APIToken, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]model.APIToken, 0, len(s.apiTokens))
	for _, tok := range s.apiTokens {
		out = append(out, tok)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].InsertedAt.After(out[j].InsertedAt) })
	return out, nil
}

func (s *MemoryStore) AuthenticateAPIToken(_ context.Context, token string) (model.APIToken, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	hash := tokenHash(token)
	for _, tok := range s.apiTokens {
		if subtle.ConstantTimeCompare([]byte(tok.TokenHash), []byte(hash)) == 1 {
			now := time.Now().UTC()
			tok.LastUsedAt = &now
			tok.UpdatedAt = now
			s.apiTokens[tok.ID] = tok
			return tok, nil
		}
	}
	return model.APIToken{}, ErrInvalidAPIToken
}

func (s *MemoryStore) CreateWorkItem(_ context.Context, in WorkItemCreate) (model.WorkItem, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if in.RepositoryID != "" {
		if _, ok := s.repositories[in.RepositoryID]; !ok {
			return model.WorkItem{}, ErrRepositoryNotFound
		}
	}
	now := time.Now().UTC()
	item := model.WorkItem{ID: newID("work"), Title: in.Title, Description: in.Description, SourceType: in.SourceType, SourceID: in.SourceID, RepositoryID: in.RepositoryID, RepositoryRef: in.RepositoryRef, RepositoryBaseBranch: in.RepositoryBaseBranch, Actor: in.Actor, State: model.WorkItemQueued, Priority: in.Priority, Metadata: cloneStringMap(in.Metadata), InsertedAt: now, UpdatedAt: now}
	s.workItems[item.ID] = item
	s.appendFactoryEventLocked(item.ID, "", "server.store", "work_item.created", map[string]any{"state": string(item.State)})
	return item, nil
}

func (s *MemoryStore) ListWorkItems(_ context.Context, opts WorkItemListOptions) ([]model.WorkItem, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]model.WorkItem, 0, len(s.workItems))
	for _, item := range s.workItems {
		if opts.State != "" && item.State != opts.State {
			continue
		}
		out = append(out, item)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].InsertedAt.After(out[j].InsertedAt) })
	return pageSlice(out, opts.Page), nil
}

func (s *MemoryStore) GetWorkItem(_ context.Context, workItemID string) (model.WorkItem, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	item, ok := s.workItems[workItemID]
	if !ok {
		return model.WorkItem{}, ErrWorkItemNotFound
	}
	return item, nil
}

func (s *MemoryStore) UpdateWorkItemState(_ context.Context, workItemID string, state model.WorkItemState) (model.WorkItem, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	item, ok := s.workItems[workItemID]
	if !ok {
		return model.WorkItem{}, ErrWorkItemNotFound
	}
	item.State = state
	item.UpdatedAt = time.Now().UTC()
	s.workItems[workItemID] = item
	s.appendFactoryEventLocked(workItemID, "", "server.store", "work_item."+string(state), map[string]any{"state": string(state)})
	return item, nil
}

func (s *MemoryStore) CreateAgentRun(_ context.Context, workItemID string, in AgentRunCreate) (model.AgentRun, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.workItems[workItemID]; !ok {
		return model.AgentRun{}, ErrWorkItemNotFound
	}
	if in.SandboxID != "" {
		if _, ok := s.sandboxes[in.SandboxID]; !ok {
			return model.AgentRun{}, ErrSandboxNotFound
		}
	}
	now := time.Now().UTC()
	run := model.AgentRun{ID: newID("run"), WorkItemID: workItemID, SandboxID: in.SandboxID, Harness: in.Harness, Model: in.Model, State: model.AgentRunQueued, Prompt: in.Prompt, Actor: in.Actor, Metadata: cloneStringMap(in.Metadata), InsertedAt: now, UpdatedAt: now}
	s.agentRuns[run.ID] = run
	s.appendFactoryEventLocked(workItemID, run.ID, "server.store", "agent_run.created", map[string]any{"state": string(run.State), "harness": run.Harness, "model": run.Model})
	return run, nil
}

func (s *MemoryStore) AcquireQueuedAgentRuns(_ context.Context, limit int) ([]model.AgentRun, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if limit <= 0 {
		limit = 1
	}
	queued := make([]model.AgentRun, 0, len(s.agentRuns))
	for _, run := range s.agentRuns {
		if run.State == model.AgentRunQueued {
			queued = append(queued, run)
		}
	}
	sort.Slice(queued, func(i, j int) bool { return queued[i].InsertedAt.Before(queued[j].InsertedAt) })
	if len(queued) > limit {
		queued = queued[:limit]
	}
	now := time.Now().UTC()
	for i, run := range queued {
		run.State = model.AgentRunRunning
		run.Attempt++
		run.UpdatedAt = now
		if run.StartedAt == nil {
			run.StartedAt = &now
		}
		s.agentRuns[run.ID] = run
		s.appendFactoryEventLocked(run.WorkItemID, run.ID, "server.store", "agent_run.running", map[string]any{"state": string(run.State), "attempt": run.Attempt})
		queued[i] = run
	}
	return queued, nil
}

func (s *MemoryStore) RecoverStaleAgentRuns(_ context.Context, olderThan time.Time) ([]model.AgentRun, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	recovered := []model.AgentRun{}
	now := time.Now().UTC()
	for _, run := range s.agentRuns {
		if run.State != model.AgentRunRunning || !run.UpdatedAt.Before(olderThan) {
			continue
		}
		run.State = model.AgentRunQueued
		run.UpdatedAt = now
		s.agentRuns[run.ID] = run
		s.appendFactoryEventLocked(run.WorkItemID, run.ID, "server.store", "agent_run.recovered", map[string]any{"state": string(run.State), "attempt": run.Attempt})
		recovered = append(recovered, run)
	}
	sort.Slice(recovered, func(i, j int) bool { return recovered[i].InsertedAt.Before(recovered[j].InsertedAt) })
	return recovered, nil
}

func (s *MemoryStore) ListAgentRuns(_ context.Context, opts AgentRunListOptions) ([]model.AgentRun, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]model.AgentRun, 0, len(s.agentRuns))
	for _, run := range s.agentRuns {
		if opts.WorkItemID != "" && run.WorkItemID != opts.WorkItemID {
			continue
		}
		if opts.State != "" && run.State != opts.State {
			continue
		}
		out = append(out, run)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].InsertedAt.After(out[j].InsertedAt) })
	return pageSlice(out, opts.Page), nil
}

func (s *MemoryStore) GetAgentRun(_ context.Context, agentRunID string) (model.AgentRun, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	run, ok := s.agentRuns[agentRunID]
	if !ok {
		return model.AgentRun{}, ErrAgentRunNotFound
	}
	return run, nil
}

func (s *MemoryStore) UpdateAgentRunState(_ context.Context, agentRunID string, state model.AgentRunState) (model.AgentRun, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	run, ok := s.agentRuns[agentRunID]
	if !ok {
		return model.AgentRun{}, ErrAgentRunNotFound
	}
	now := time.Now().UTC()
	run.State = state
	run.UpdatedAt = now
	if state == model.AgentRunRunning && run.StartedAt == nil {
		run.StartedAt = &now
	}
	if isTerminalAgentRunState(state) {
		run.CompletedAt = &now
	}
	s.agentRuns[agentRunID] = run
	s.appendFactoryEventLocked(run.WorkItemID, run.ID, "server.store", "agent_run."+string(state), map[string]any{"state": string(state)})
	return run, nil
}

func (s *MemoryStore) CreateRepository(_ context.Context, in RepositoryCreate) (model.Repository, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now().UTC()
	if in.Provider == "" {
		in.Provider = "git"
	}
	if in.DefaultBranch == "" {
		in.DefaultBranch = "main"
	}
	repo := model.Repository{ID: newID("repo"), Name: in.Name, Provider: in.Provider, CloneURL: in.CloneURL, DefaultBranch: in.DefaultBranch, CredentialRef: in.CredentialRef, Metadata: cloneStringMap(in.Metadata), InsertedAt: now, UpdatedAt: now}
	s.repositories[repo.ID] = repo
	return repo, nil
}

func (s *MemoryStore) ListRepositories(_ context.Context, opts RepositoryListOptions) ([]model.Repository, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]model.Repository, 0, len(s.repositories))
	for _, repo := range s.repositories {
		if opts.Provider != "" && repo.Provider != opts.Provider {
			continue
		}
		out = append(out, repo)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].InsertedAt.After(out[j].InsertedAt) })
	return pageSlice(out, opts.Page), nil
}

func (s *MemoryStore) GetRepository(_ context.Context, repositoryID string) (model.Repository, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	repo, ok := s.repositories[repositoryID]
	if !ok {
		return model.Repository{}, ErrRepositoryNotFound
	}
	return repo, nil
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
	s.appendEventLocked(sandboxID, cmd.ID, "server.store", "command.queued", map[string]any{"command": cmd.Command})
	return cmd, nil
}

func (s *MemoryStore) ListCommands(_ context.Context, sandboxID string, opts CommandListOptions) ([]model.Command, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.sandboxes[sandboxID]; !ok {
		return nil, ErrSandboxNotFound
	}
	out := []model.Command{}
	for _, cmd := range s.commands[sandboxID] {
		if opts.State != "" && cmd.State != opts.State {
			continue
		}
		out = append(out, cmd)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].InsertedAt.Before(out[j].InsertedAt) })
	return pageSlice(out, opts.Page), nil
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

func (s *MemoryStore) AppendEvent(_ context.Context, sandboxID, commandID, source, eventType string, data map[string]any) (model.Event, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.sandboxes[sandboxID]; !ok {
		return model.Event{}, ErrSandboxNotFound
	}
	return s.appendEventLocked(sandboxID, commandID, source, eventType, data), nil
}

func (s *MemoryStore) ListSandboxEvents(_ context.Context, sandboxID string, opts EventListOptions) ([]model.Event, int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.sandboxes[sandboxID]; !ok {
		return nil, opts.After, ErrSandboxNotFound
	}
	return filterEvents(s.events[sandboxID], "", opts, false)
}

func (s *MemoryStore) ListCommandEvents(_ context.Context, sandboxID, commandID string, opts EventListOptions) ([]model.Event, int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.commands[sandboxID][commandID]; !ok {
		return nil, opts.After, ErrCommandNotFound
	}
	return filterEvents(s.events[sandboxID], commandID, opts, true)
}

func (s *MemoryStore) AppendFactoryEvent(_ context.Context, workItemID, agentRunID, source, eventType string, data map[string]any) (model.Event, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.workItems[workItemID]; !ok {
		return model.Event{}, ErrWorkItemNotFound
	}
	if agentRunID != "" {
		if _, ok := s.agentRuns[agentRunID]; !ok {
			return model.Event{}, ErrAgentRunNotFound
		}
	}
	return s.appendFactoryEventLocked(workItemID, agentRunID, source, eventType, data), nil
}

func (s *MemoryStore) ListWorkItemEvents(_ context.Context, workItemID string, opts EventListOptions) ([]model.Event, int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.workItems[workItemID]; !ok {
		return nil, opts.After, ErrWorkItemNotFound
	}
	return filterFactoryEvents(s.factoryEvents[workItemID], "", opts, false)
}

func (s *MemoryStore) ListAgentRunEvents(_ context.Context, agentRunID string, opts EventListOptions) ([]model.Event, int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	run, ok := s.agentRuns[agentRunID]
	if !ok {
		return nil, opts.After, ErrAgentRunNotFound
	}
	return filterFactoryEvents(s.factoryEvents[run.WorkItemID], agentRunID, opts, true)
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

func (s *MemoryStore) appendEventLocked(sandboxID, commandID, source, eventType string, data map[string]any) model.Event {
	s.nextSeq[sandboxID]++
	ev := model.Event{ID: newID("evt"), SandboxID: sandboxID, CommandID: commandID, Seq: s.nextSeq[sandboxID], Type: eventType, Source: source, Timestamp: time.Now().UTC(), Data: data}
	s.events[sandboxID] = append(s.events[sandboxID], ev)
	return ev
}

func (s *MemoryStore) appendFactoryEventLocked(workItemID, agentRunID, source, eventType string, data map[string]any) model.Event {
	s.nextFactorySeq[workItemID]++
	ev := model.Event{ID: newID("evt"), WorkItemID: workItemID, AgentRunID: agentRunID, Seq: s.nextFactorySeq[workItemID], Type: eventType, Source: source, Timestamp: time.Now().UTC(), Data: data}
	s.factoryEvents[workItemID] = append(s.factoryEvents[workItemID], ev)
	return ev
}

func filterEvents(events []model.Event, commandID string, opts EventListOptions, commandOnly bool) ([]model.Event, int64, error) {
	out := []model.Event{}
	next := opts.After
	for _, ev := range events {
		if ev.Seq <= opts.After {
			continue
		}
		if commandOnly && ev.CommandID != commandID {
			continue
		}
		out = append(out, ev)
		if ev.Seq > next {
			next = ev.Seq
		}
		if opts.Limit > 0 && len(out) >= opts.Limit {
			break
		}
	}
	return out, next, nil
}

func filterFactoryEvents(events []model.Event, agentRunID string, opts EventListOptions, agentRunOnly bool) ([]model.Event, int64, error) {
	out := []model.Event{}
	next := opts.After
	for _, ev := range events {
		if ev.Seq <= opts.After {
			continue
		}
		if agentRunOnly && ev.AgentRunID != agentRunID {
			continue
		}
		out = append(out, ev)
		if ev.Seq > next {
			next = ev.Seq
		}
		if opts.Limit > 0 && len(out) >= opts.Limit {
			break
		}
	}
	return out, next, nil
}

func pageSlice[T any](in []T, page Page) []T {
	if page.Offset > 0 {
		if page.Offset >= len(in) {
			return []T{}
		}
		in = in[page.Offset:]
	}
	if page.Limit > 0 && page.Limit < len(in) {
		return in[:page.Limit]
	}
	return in
}

func isTerminalAgentRunState(state model.AgentRunState) bool {
	return state == model.AgentRunCompleted || state == model.AgentRunFailed || state == model.AgentRunCancelled || state == model.AgentRunTimedOut
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

func tokenHash(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func tokenPrefix(token string) string {
	if len(token) <= 12 {
		return token
	}
	return token[:12]
}
