package store

import (
	"context"
	"crypto/subtle"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/brianmichel/shed/internal/model"
)

type PostgresStore struct {
	db *sql.DB
}

var _ Store = (*PostgresStore)(nil)

func NewPostgresStore(db *sql.DB) (*PostgresStore, error) {
	if db == nil {
		return nil, fmt.Errorf("nil database")
	}
	return &PostgresStore{db: db}, nil
}

func NewMigratedPostgresStore(ctx context.Context, db *sql.DB) (*PostgresStore, error) {
	if err := Migrate(ctx, db); err != nil {
		return nil, err
	}
	return NewPostgresStore(db)
}

func (s *PostgresStore) CreateSandbox(ctx context.Context, in SandboxCreate) (model.Sandbox, model.ClientSession, error) {
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
	key := newID("seedkey")
	sb := model.Sandbox{ID: id, Environment: in.Environment, Template: in.Template, State: model.SandboxPendingClient, Compute: in.Compute, ComputeAPIVersion: in.ComputeAPIVersion, ComputeConfig: cloneStringMap(in.ComputeConfig), Metadata: cloneStringMap(in.Metadata), Capabilities: map[string]bool{"commands": true, "files": true, "pty": false}, Lease: model.Lease{TTLMillis: in.TTL.Milliseconds(), ExpiresAt: now.Add(in.TTL)}, InsertedAt: now, UpdatedAt: now}
	sess := model.ClientSession{SessionID: newID("sess"), AgentTokenHash: tokenHash(key), SandboxID: id, State: model.SessionIssued, InsertedAt: now, UpdatedAt: now}
	err := s.withTx(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `INSERT INTO sandboxes (id, environment, template, state, compute, compute_api_version, compute_config, metadata, capabilities, lease_ttl_ms, lease_expires_at, inserted_at, updated_at) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)`, sb.ID, sb.Environment, sb.Template, sb.State, sb.Compute, sb.ComputeAPIVersion, jsonParam(sb.ComputeConfig), jsonParam(sb.Metadata), jsonParam(sb.Capabilities), sb.Lease.TTLMillis, sb.Lease.ExpiresAt, sb.InsertedAt, sb.UpdatedAt); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO client_sessions (session_id, sandbox_id, agent_token_hash, state, inserted_at, updated_at) VALUES ($1,$2,$3,$4,$5,$6)`, sess.SessionID, sess.SandboxID, sess.AgentTokenHash, sess.State, sess.InsertedAt, sess.UpdatedAt); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO sandbox_event_sequences (sandbox_id, next_seq) VALUES ($1, 0)`, sb.ID); err != nil {
			return err
		}
		_, err := s.appendEventTx(ctx, tx, sb.ID, "", "server.store", "sandbox.pending_client", map[string]any{"state": string(sb.State)})
		return err
	})
	if err != nil {
		return model.Sandbox{}, model.ClientSession{}, err
	}
	sess.AgentToken = key
	return sb, sess, nil
}

func (s *PostgresStore) ListSandboxes(ctx context.Context, opts SandboxListOptions) ([]model.Sandbox, error) {
	query := `SELECT id, environment, template, state, compute, compute_api_version, compute_plugin_version, external_allocation_id, compute_config, compute_metadata, metadata, capabilities, lease_ttl_ms, lease_expires_at, inserted_at, updated_at FROM sandboxes`
	args := []any{}
	if opts.State != "" {
		args = append(args, opts.State)
		query += fmt.Sprintf(" WHERE state = $%d", len(args))
	}
	query += " ORDER BY inserted_at DESC"
	query, args = appendPage(query, args, opts.Page)
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.Sandbox{}
	for rows.Next() {
		sb, err := scanSandbox(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, sb)
	}
	return out, rows.Err()
}

func (s *PostgresStore) GetSandbox(ctx context.Context, sandboxID string) (model.Sandbox, error) {
	row := s.db.QueryRowContext(ctx, `SELECT id, environment, template, state, compute, compute_api_version, compute_plugin_version, external_allocation_id, compute_config, compute_metadata, metadata, capabilities, lease_ttl_ms, lease_expires_at, inserted_at, updated_at FROM sandboxes WHERE id = $1`, sandboxID)
	sb, err := scanSandbox(row)
	if errors.Is(err, sql.ErrNoRows) {
		return model.Sandbox{}, ErrSandboxNotFound
	}
	return sb, err
}

func (s *PostgresStore) UpdateSandboxState(ctx context.Context, sandboxID string, state model.SandboxState) (model.Sandbox, error) {
	var sb model.Sandbox
	err := s.withTx(ctx, func(tx *sql.Tx) error {
		row := tx.QueryRowContext(ctx, `UPDATE sandboxes SET state = $2, updated_at = $3 WHERE id = $1 RETURNING id, environment, template, state, compute, compute_api_version, compute_plugin_version, external_allocation_id, compute_config, compute_metadata, metadata, capabilities, lease_ttl_ms, lease_expires_at, inserted_at, updated_at`, sandboxID, state, time.Now().UTC())
		var err error
		sb, err = scanSandbox(row)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrSandboxNotFound
		}
		if err != nil {
			return err
		}
		_, err = s.appendEventTx(ctx, tx, sandboxID, "", "server.store", "sandbox."+string(state), map[string]any{"state": string(state)})
		return err
	})
	return sb, err
}

func (s *PostgresStore) UpdateSandboxAllocation(ctx context.Context, sandboxID string, in SandboxAllocationUpdate) (model.Sandbox, error) {
	var sb model.Sandbox
	err := s.withTx(ctx, func(tx *sql.Tx) error {
		current, err := scanSandbox(tx.QueryRowContext(ctx, `SELECT id, environment, template, state, compute, compute_api_version, compute_plugin_version, external_allocation_id, compute_config, compute_metadata, metadata, capabilities, lease_ttl_ms, lease_expires_at, inserted_at, updated_at FROM sandboxes WHERE id = $1`, sandboxID))
		if errors.Is(err, sql.ErrNoRows) {
			return ErrSandboxNotFound
		}
		if err != nil {
			return err
		}
		if in.Compute != "" {
			current.Compute = in.Compute
		}
		if in.ComputeAPIVersion != "" {
			current.ComputeAPIVersion = in.ComputeAPIVersion
		}
		if in.ComputePluginVersion != "" {
			current.ComputePluginVersion = in.ComputePluginVersion
		}
		if in.ExternalAllocationID != "" {
			current.ExternalAllocationID = in.ExternalAllocationID
		}
		if in.ComputeMetadata != nil {
			current.ComputeMetadata = cloneStringMap(in.ComputeMetadata)
		}
		current.UpdatedAt = time.Now().UTC()
		row := tx.QueryRowContext(ctx, `UPDATE sandboxes SET compute = $2, compute_api_version = $3, compute_plugin_version = $4, external_allocation_id = $5, compute_metadata = $6, updated_at = $7 WHERE id = $1 RETURNING id, environment, template, state, compute, compute_api_version, compute_plugin_version, external_allocation_id, compute_config, compute_metadata, metadata, capabilities, lease_ttl_ms, lease_expires_at, inserted_at, updated_at`, current.ID, current.Compute, current.ComputeAPIVersion, current.ComputePluginVersion, current.ExternalAllocationID, jsonParam(current.ComputeMetadata), current.UpdatedAt)
		sb, err = scanSandbox(row)
		if err != nil {
			return err
		}
		_, err = s.appendEventTx(ctx, tx, sandboxID, "", "server.store", "sandbox.allocation.updated", map[string]any{"compute": sb.Compute, "compute_api_version": sb.ComputeAPIVersion, "compute_plugin_version": sb.ComputePluginVersion, "external_allocation_id": sb.ExternalAllocationID})
		return err
	})
	return sb, err
}

func (s *PostgresStore) ExtendLease(ctx context.Context, sandboxID string, ttl time.Duration) (model.Lease, error) {
	var lease model.Lease
	err := s.withTx(ctx, func(tx *sql.Tx) error {
		sb, err := scanSandbox(tx.QueryRowContext(ctx, `SELECT id, environment, template, state, compute, compute_api_version, compute_plugin_version, external_allocation_id, compute_config, compute_metadata, metadata, capabilities, lease_ttl_ms, lease_expires_at, inserted_at, updated_at FROM sandboxes WHERE id = $1`, sandboxID))
		if errors.Is(err, sql.ErrNoRows) {
			return ErrSandboxNotFound
		}
		if err != nil {
			return err
		}
		if ttl <= 0 {
			ttl = time.Duration(sb.Lease.TTLMillis) * time.Millisecond
		}
		lease = model.Lease{TTLMillis: ttl.Milliseconds(), ExpiresAt: time.Now().UTC().Add(ttl)}
		if _, err := tx.ExecContext(ctx, `UPDATE sandboxes SET lease_ttl_ms = $2, lease_expires_at = $3, updated_at = $4 WHERE id = $1`, sandboxID, lease.TTLMillis, lease.ExpiresAt, time.Now().UTC()); err != nil {
			return err
		}
		_, err = s.appendEventTx(ctx, tx, sandboxID, "", "server.store", "sandbox.lease.extended", map[string]any{"ttl_ms": lease.TTLMillis, "expires_at": lease.ExpiresAt})
		return err
	})
	return lease, err
}

func (s *PostgresStore) AuthenticateSession(ctx context.Context, sandboxID, agentToken string) (model.ClientSession, error) {
	sess, err := scanSession(s.db.QueryRowContext(ctx, `SELECT session_id, sandbox_id, agent_token_hash, state, capabilities, metadata, last_client_seq_seen, last_server_seq_sent, inserted_at, updated_at FROM client_sessions WHERE sandbox_id = $1`, sandboxID))
	if errors.Is(err, sql.ErrNoRows) {
		return model.ClientSession{}, ErrInvalidSession
	}
	if err != nil {
		return model.ClientSession{}, err
	}
	if subtleCompare(sess.AgentTokenHash, tokenHash(agentToken)) {
		return sess, nil
	}
	return model.ClientSession{}, ErrInvalidSession
}

func (s *PostgresStore) GetSession(ctx context.Context, sessionID string) (model.ClientSession, error) {
	sess, err := scanSession(s.db.QueryRowContext(ctx, `SELECT session_id, sandbox_id, agent_token_hash, state, capabilities, metadata, last_client_seq_seen, last_server_seq_sent, inserted_at, updated_at FROM client_sessions WHERE session_id = $1`, sessionID))
	if errors.Is(err, sql.ErrNoRows) {
		return model.ClientSession{}, ErrSessionNotFound
	}
	return sess, err
}

func (s *PostgresStore) FindSessionBySandbox(ctx context.Context, sandboxID string) (model.ClientSession, error) {
	sess, err := scanSession(s.db.QueryRowContext(ctx, `SELECT session_id, sandbox_id, agent_token_hash, state, capabilities, metadata, last_client_seq_seen, last_server_seq_sent, inserted_at, updated_at FROM client_sessions WHERE sandbox_id = $1`, sandboxID))
	if errors.Is(err, sql.ErrNoRows) {
		return model.ClientSession{}, ErrSessionNotFound
	}
	return sess, err
}

func (s *PostgresStore) UpdateSession(ctx context.Context, sess model.ClientSession) (model.ClientSession, error) {
	current, err := s.GetSession(ctx, sess.SessionID)
	if err != nil {
		return model.ClientSession{}, err
	}
	if sess.AgentTokenHash == "" {
		sess.AgentTokenHash = current.AgentTokenHash
	}
	sess.AgentToken = ""
	sess.UpdatedAt = time.Now().UTC()
	row := s.db.QueryRowContext(ctx, `UPDATE client_sessions SET sandbox_id = $2, agent_token_hash = $3, state = $4, capabilities = $5, metadata = $6, last_client_seq_seen = $7, last_server_seq_sent = $8, updated_at = $9 WHERE session_id = $1 RETURNING session_id, sandbox_id, agent_token_hash, state, capabilities, metadata, last_client_seq_seen, last_server_seq_sent, inserted_at, updated_at`, sess.SessionID, sess.SandboxID, sess.AgentTokenHash, sess.State, jsonParam(sess.Capabilities), jsonParam(sess.Metadata), sess.LastClientSeqSeen, sess.LastServerSeqSent, sess.UpdatedAt)
	return scanSession(row)
}

func (s *PostgresStore) CreateAPIToken(ctx context.Context, in APITokenCreate) (APITokenCreateResult, error) {
	now := time.Now().UTC()
	secret := newID("shd")
	if in.Name == "" {
		in.Name = "api-token"
	}
	tok := model.APIToken{ID: newID("atok"), Name: in.Name, TokenHash: tokenHash(secret), TokenPrefix: tokenPrefix(secret), Metadata: cloneStringMap(in.Metadata), InsertedAt: now, UpdatedAt: now}
	_, err := s.db.ExecContext(ctx, `INSERT INTO api_tokens (id, name, token_hash, token_prefix, metadata, inserted_at, updated_at) VALUES ($1,$2,$3,$4,$5,$6,$7)`, tok.ID, tok.Name, tok.TokenHash, tok.TokenPrefix, jsonParam(tok.Metadata), tok.InsertedAt, tok.UpdatedAt)
	return APITokenCreateResult{Token: tok, Secret: secret}, err
}

func (s *PostgresStore) ListAPITokens(ctx context.Context) ([]model.APIToken, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, name, token_hash, token_prefix, metadata, last_used_at, inserted_at, updated_at FROM api_tokens ORDER BY inserted_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.APIToken{}
	for rows.Next() {
		tok, err := scanAPIToken(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, tok)
	}
	return out, rows.Err()
}

func (s *PostgresStore) AuthenticateAPIToken(ctx context.Context, token string) (model.APIToken, error) {
	now := time.Now().UTC()
	tok, err := scanAPIToken(s.db.QueryRowContext(ctx, `UPDATE api_tokens SET last_used_at = $2, updated_at = $2 WHERE token_hash = $1 RETURNING id, name, token_hash, token_prefix, metadata, last_used_at, inserted_at, updated_at`, tokenHash(token), now))
	if errors.Is(err, sql.ErrNoRows) {
		return model.APIToken{}, ErrInvalidAPIToken
	}
	return tok, err
}

func (s *PostgresStore) CreateWorkItem(ctx context.Context, in WorkItemCreate) (model.WorkItem, error) {
	now := time.Now().UTC()
	item := model.WorkItem{ID: newID("work"), Title: in.Title, Description: in.Description, SourceType: in.SourceType, SourceID: in.SourceID, Actor: in.Actor, State: model.WorkItemQueued, Priority: in.Priority, Metadata: cloneStringMap(in.Metadata), InsertedAt: now, UpdatedAt: now}
	_, err := s.db.ExecContext(ctx, `INSERT INTO work_items (id, title, description, source_type, source_id, actor, state, priority, metadata, inserted_at, updated_at) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)`, item.ID, item.Title, item.Description, item.SourceType, item.SourceID, item.Actor, item.State, item.Priority, jsonParam(item.Metadata), item.InsertedAt, item.UpdatedAt)
	return item, err
}

func (s *PostgresStore) ListWorkItems(ctx context.Context, opts WorkItemListOptions) ([]model.WorkItem, error) {
	query := `SELECT id, title, description, source_type, source_id, actor, state, priority, metadata, inserted_at, updated_at FROM work_items`
	args := []any{}
	if opts.State != "" {
		args = append(args, opts.State)
		query += fmt.Sprintf(" WHERE state = $%d", len(args))
	}
	query += " ORDER BY inserted_at DESC"
	query, args = appendPage(query, args, opts.Page)
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.WorkItem{}
	for rows.Next() {
		item, err := scanWorkItem(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *PostgresStore) GetWorkItem(ctx context.Context, workItemID string) (model.WorkItem, error) {
	item, err := scanWorkItem(s.db.QueryRowContext(ctx, `SELECT id, title, description, source_type, source_id, actor, state, priority, metadata, inserted_at, updated_at FROM work_items WHERE id = $1`, workItemID))
	if errors.Is(err, sql.ErrNoRows) {
		return model.WorkItem{}, ErrWorkItemNotFound
	}
	return item, err
}

func (s *PostgresStore) UpdateWorkItemState(ctx context.Context, workItemID string, state model.WorkItemState) (model.WorkItem, error) {
	item, err := scanWorkItem(s.db.QueryRowContext(ctx, `UPDATE work_items SET state = $2, updated_at = $3 WHERE id = $1 RETURNING id, title, description, source_type, source_id, actor, state, priority, metadata, inserted_at, updated_at`, workItemID, state, time.Now().UTC()))
	if errors.Is(err, sql.ErrNoRows) {
		return model.WorkItem{}, ErrWorkItemNotFound
	}
	return item, err
}

func (s *PostgresStore) CreateAgentRun(ctx context.Context, workItemID string, in AgentRunCreate) (model.AgentRun, error) {
	now := time.Now().UTC()
	run := model.AgentRun{ID: newID("run"), WorkItemID: workItemID, SandboxID: in.SandboxID, Harness: in.Harness, Model: in.Model, State: model.AgentRunQueued, Prompt: in.Prompt, Actor: in.Actor, Metadata: cloneStringMap(in.Metadata), InsertedAt: now, UpdatedAt: now}
	var sandboxID any
	if run.SandboxID != "" {
		sandboxID = run.SandboxID
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO agent_runs (id, work_item_id, sandbox_id, harness, model, state, prompt, actor, metadata, inserted_at, updated_at) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)`, run.ID, run.WorkItemID, sandboxID, run.Harness, run.Model, run.State, run.Prompt, run.Actor, jsonParam(run.Metadata), run.InsertedAt, run.UpdatedAt)
	if err != nil {
		if strings.Contains(err.Error(), "agent_runs_work_item_id_fkey") {
			return model.AgentRun{}, ErrWorkItemNotFound
		}
		if strings.Contains(err.Error(), "agent_runs_sandbox_id_fkey") {
			return model.AgentRun{}, ErrSandboxNotFound
		}
	}
	return run, err
}

func (s *PostgresStore) ListAgentRuns(ctx context.Context, opts AgentRunListOptions) ([]model.AgentRun, error) {
	query := `SELECT id, work_item_id, sandbox_id, harness, model, state, prompt, actor, metadata, started_at, completed_at, inserted_at, updated_at FROM agent_runs`
	where := []string{}
	args := []any{}
	if opts.WorkItemID != "" {
		args = append(args, opts.WorkItemID)
		where = append(where, fmt.Sprintf("work_item_id = $%d", len(args)))
	}
	if opts.State != "" {
		args = append(args, opts.State)
		where = append(where, fmt.Sprintf("state = $%d", len(args)))
	}
	if len(where) > 0 {
		query += " WHERE " + strings.Join(where, " AND ")
	}
	query += " ORDER BY inserted_at DESC"
	query, args = appendPage(query, args, opts.Page)
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.AgentRun{}
	for rows.Next() {
		run, err := scanAgentRun(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, run)
	}
	return out, rows.Err()
}

func (s *PostgresStore) GetAgentRun(ctx context.Context, agentRunID string) (model.AgentRun, error) {
	run, err := scanAgentRun(s.db.QueryRowContext(ctx, `SELECT id, work_item_id, sandbox_id, harness, model, state, prompt, actor, metadata, started_at, completed_at, inserted_at, updated_at FROM agent_runs WHERE id = $1`, agentRunID))
	if errors.Is(err, sql.ErrNoRows) {
		return model.AgentRun{}, ErrAgentRunNotFound
	}
	return run, err
}

func (s *PostgresStore) UpdateAgentRunState(ctx context.Context, agentRunID string, state model.AgentRunState) (model.AgentRun, error) {
	now := time.Now().UTC()
	query := `UPDATE agent_runs SET state = $2, updated_at = $3`
	args := []any{agentRunID, state, now}
	if state == model.AgentRunRunning {
		query += `, started_at = COALESCE(started_at, $3)`
	}
	if isTerminalAgentRunState(state) {
		query += `, completed_at = $3`
	}
	query += ` WHERE id = $1 RETURNING id, work_item_id, sandbox_id, harness, model, state, prompt, actor, metadata, started_at, completed_at, inserted_at, updated_at`
	run, err := scanAgentRun(s.db.QueryRowContext(ctx, query, args...))
	if errors.Is(err, sql.ErrNoRows) {
		return model.AgentRun{}, ErrAgentRunNotFound
	}
	return run, err
}

func (s *PostgresStore) CreateCommand(ctx context.Context, sandboxID string, in CommandCreate) (model.Command, error) {
	now := time.Now().UTC()
	if in.Cwd == "" {
		in.Cwd = "/workspace"
	}
	if in.TimeoutMS == 0 {
		in.TimeoutMS = 60000
	}
	cmd := model.Command{ID: newID("cmd"), SandboxID: sandboxID, State: model.CommandQueued, Command: in.Command, Cwd: in.Cwd, Env: cloneStringMap(in.Env), Stdin: in.Stdin, TimeoutMS: in.TimeoutMS, Metadata: cloneStringMap(in.Metadata), InsertedAt: now, UpdatedAt: now}
	err := s.withTx(ctx, func(tx *sql.Tx) error {
		var exists bool
		if err := tx.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM sandboxes WHERE id = $1)`, sandboxID).Scan(&exists); err != nil {
			return err
		}
		if !exists {
			return ErrSandboxNotFound
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO commands (id, sandbox_id, state, command, cwd, env, stdin, timeout_ms, metadata, inserted_at, updated_at) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)`, cmd.ID, cmd.SandboxID, cmd.State, cmd.Command, cmd.Cwd, jsonParam(cmd.Env), cmd.Stdin, cmd.TimeoutMS, jsonParam(cmd.Metadata), cmd.InsertedAt, cmd.UpdatedAt); err != nil {
			return err
		}
		_, err := s.appendEventTx(ctx, tx, sandboxID, cmd.ID, "server.store", "command.queued", map[string]any{"command": cmd.Command})
		return err
	})
	return cmd, err
}

func (s *PostgresStore) ListCommands(ctx context.Context, sandboxID string, opts CommandListOptions) ([]model.Command, error) {
	if _, err := s.GetSandbox(ctx, sandboxID); err != nil {
		return nil, err
	}
	query := `SELECT id, sandbox_id, state, command, cwd, env, stdin, timeout_ms, metadata, pid, exit_code, signal, started_at, completed_at, inserted_at, updated_at FROM commands WHERE sandbox_id = $1`
	args := []any{sandboxID}
	if opts.State != "" {
		args = append(args, opts.State)
		query += fmt.Sprintf(" AND state = $%d", len(args))
	}
	query += " ORDER BY inserted_at ASC"
	query, args = appendPage(query, args, opts.Page)
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.Command{}
	for rows.Next() {
		cmd, err := scanCommand(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, cmd)
	}
	return out, rows.Err()
}

func (s *PostgresStore) GetCommand(ctx context.Context, sandboxID, commandID string) (model.Command, error) {
	cmd, err := scanCommand(s.db.QueryRowContext(ctx, `SELECT id, sandbox_id, state, command, cwd, env, stdin, timeout_ms, metadata, pid, exit_code, signal, started_at, completed_at, inserted_at, updated_at FROM commands WHERE sandbox_id = $1 AND id = $2`, sandboxID, commandID))
	if errors.Is(err, sql.ErrNoRows) {
		return model.Command{}, ErrCommandNotFound
	}
	return cmd, err
}

func (s *PostgresStore) UpdateCommand(ctx context.Context, cmd model.Command) (model.Command, error) {
	cmd.UpdatedAt = time.Now().UTC()
	row := s.db.QueryRowContext(ctx, `UPDATE commands SET state = $3, command = $4, cwd = $5, env = $6, stdin = $7, timeout_ms = $8, metadata = $9, pid = $10, exit_code = $11, signal = $12, started_at = $13, completed_at = $14, updated_at = $15 WHERE sandbox_id = $1 AND id = $2 RETURNING id, sandbox_id, state, command, cwd, env, stdin, timeout_ms, metadata, pid, exit_code, signal, started_at, completed_at, inserted_at, updated_at`, cmd.SandboxID, cmd.ID, cmd.State, cmd.Command, cmd.Cwd, jsonParam(cmd.Env), cmd.Stdin, cmd.TimeoutMS, jsonParam(cmd.Metadata), cmd.PID, cmd.ExitCode, cmd.Signal, cmd.StartedAt, cmd.CompletedAt, cmd.UpdatedAt)
	updated, err := scanCommand(row)
	if errors.Is(err, sql.ErrNoRows) {
		return model.Command{}, ErrCommandNotFound
	}
	return updated, err
}

func (s *PostgresStore) AppendEvent(ctx context.Context, sandboxID, commandID, source, eventType string, data map[string]any) (model.Event, error) {
	var ev model.Event
	err := s.withTx(ctx, func(tx *sql.Tx) error {
		var exists bool
		if err := tx.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM sandboxes WHERE id = $1)`, sandboxID).Scan(&exists); err != nil {
			return err
		}
		if !exists {
			return ErrSandboxNotFound
		}
		var err error
		ev, err = s.appendEventTx(ctx, tx, sandboxID, commandID, source, eventType, data)
		return err
	})
	return ev, err
}

func (s *PostgresStore) ListSandboxEvents(ctx context.Context, sandboxID string, opts EventListOptions) ([]model.Event, int64, error) {
	if _, err := s.GetSandbox(ctx, sandboxID); err != nil {
		return nil, opts.After, err
	}
	return s.listEvents(ctx, `SELECT id, sandbox_id, command_id, seq, type, source, timestamp, data FROM events WHERE sandbox_id = $1 AND seq > $2 ORDER BY seq ASC`, sandboxID, "", opts)
}

func (s *PostgresStore) ListCommandEvents(ctx context.Context, sandboxID, commandID string, opts EventListOptions) ([]model.Event, int64, error) {
	if _, err := s.GetCommand(ctx, sandboxID, commandID); err != nil {
		return nil, opts.After, err
	}
	return s.listEvents(ctx, `SELECT id, sandbox_id, command_id, seq, type, source, timestamp, data FROM events WHERE sandbox_id = $1 AND command_id = $2 AND seq > $3 ORDER BY seq ASC`, sandboxID, commandID, opts)
}

func (s *PostgresStore) RememberIdempotencyKey(ctx context.Context, key, value string) (string, bool, error) {
	var out string
	var created bool
	err := s.db.QueryRowContext(ctx, `WITH inserted AS (
    INSERT INTO idempotency_keys (key, value)
    VALUES ($1, $2)
    ON CONFLICT (key) DO NOTHING
    RETURNING value, true AS created
)
SELECT value, created FROM inserted
UNION ALL
SELECT value, false AS created FROM idempotency_keys WHERE key = $1 AND NOT EXISTS (SELECT 1 FROM inserted)`, key, value).Scan(&out, &created)
	if err != nil {
		return "", false, err
	}
	return out, created, nil
}

func (s *PostgresStore) withTx(ctx context.Context, fn func(*sql.Tx) error) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := fn(tx); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *PostgresStore) appendEventTx(ctx context.Context, tx *sql.Tx, sandboxID, commandID, source, eventType string, data map[string]any) (model.Event, error) {
	var seq int64
	if err := tx.QueryRowContext(ctx, `UPDATE sandbox_event_sequences SET next_seq = next_seq + 1 WHERE sandbox_id = $1 RETURNING next_seq`, sandboxID).Scan(&seq); err != nil {
		return model.Event{}, err
	}
	ev := model.Event{ID: newID("evt"), SandboxID: sandboxID, CommandID: commandID, Seq: seq, Type: eventType, Source: source, Timestamp: time.Now().UTC(), Data: data}
	var commandIDParam any
	if commandID != "" {
		commandIDParam = commandID
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO events (id, sandbox_id, command_id, seq, type, source, timestamp, data) VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`, ev.ID, ev.SandboxID, commandIDParam, ev.Seq, ev.Type, ev.Source, ev.Timestamp, jsonParam(ev.Data)); err != nil {
		return model.Event{}, err
	}
	return ev, nil
}

func (s *PostgresStore) listEvents(ctx context.Context, query, sandboxID, commandID string, opts EventListOptions) ([]model.Event, int64, error) {
	var rows *sql.Rows
	var err error
	if commandID == "" {
		args := []any{sandboxID, opts.After}
		query, args = appendLimit(query, args, opts.Limit)
		rows, err = s.db.QueryContext(ctx, query, args...)
	} else {
		args := []any{sandboxID, commandID, opts.After}
		query, args = appendLimit(query, args, opts.Limit)
		rows, err = s.db.QueryContext(ctx, query, args...)
	}
	if err != nil {
		return nil, opts.After, err
	}
	defer rows.Close()
	out := []model.Event{}
	next := opts.After
	for rows.Next() {
		ev, err := scanEvent(rows)
		if err != nil {
			return nil, opts.After, err
		}
		out = append(out, ev)
		if ev.Seq > next {
			next = ev.Seq
		}
	}
	return out, next, rows.Err()
}

func appendPage(query string, args []any, page Page) (string, []any) {
	query, args = appendLimit(query, args, page.Limit)
	if page.Offset > 0 {
		args = append(args, page.Offset)
		query += fmt.Sprintf(" OFFSET $%d", len(args))
	}
	return query, args
}

func appendLimit(query string, args []any, limit int) (string, []any) {
	if limit <= 0 {
		return query, args
	}
	args = append(args, limit)
	if strings.Contains(query, " LIMIT ") {
		return query, args
	}
	return query + fmt.Sprintf(" LIMIT $%d", len(args)), args
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanSandbox(row rowScanner) (model.Sandbox, error) {
	var sb model.Sandbox
	var state string
	var computeConfig, computeMetadata, metadata, capabilities []byte
	err := row.Scan(&sb.ID, &sb.Environment, &sb.Template, &state, &sb.Compute, &sb.ComputeAPIVersion, &sb.ComputePluginVersion, &sb.ExternalAllocationID, &computeConfig, &computeMetadata, &metadata, &capabilities, &sb.Lease.TTLMillis, &sb.Lease.ExpiresAt, &sb.InsertedAt, &sb.UpdatedAt)
	if err != nil {
		return model.Sandbox{}, err
	}
	sb.State = model.SandboxState(state)
	if err := scanJSON(computeConfig, &sb.ComputeConfig); err != nil {
		return model.Sandbox{}, err
	}
	if err := scanJSON(computeMetadata, &sb.ComputeMetadata); err != nil {
		return model.Sandbox{}, err
	}
	if err := scanJSON(metadata, &sb.Metadata); err != nil {
		return model.Sandbox{}, err
	}
	if err := scanJSON(capabilities, &sb.Capabilities); err != nil {
		return model.Sandbox{}, err
	}
	return sb, nil
}

func scanSession(row rowScanner) (model.ClientSession, error) {
	var sess model.ClientSession
	var state string
	var capabilities, metadata []byte
	err := row.Scan(&sess.SessionID, &sess.SandboxID, &sess.AgentTokenHash, &state, &capabilities, &metadata, &sess.LastClientSeqSeen, &sess.LastServerSeqSent, &sess.InsertedAt, &sess.UpdatedAt)
	if err != nil {
		return model.ClientSession{}, err
	}
	sess.State = model.SessionState(state)
	if err := scanJSON(capabilities, &sess.Capabilities); err != nil {
		return model.ClientSession{}, err
	}
	if err := scanJSON(metadata, &sess.Metadata); err != nil {
		return model.ClientSession{}, err
	}
	return sess, nil
}

func scanAPIToken(row rowScanner) (model.APIToken, error) {
	var tok model.APIToken
	var metadata []byte
	var lastUsed sql.NullTime
	err := row.Scan(&tok.ID, &tok.Name, &tok.TokenHash, &tok.TokenPrefix, &metadata, &lastUsed, &tok.InsertedAt, &tok.UpdatedAt)
	if err != nil {
		return model.APIToken{}, err
	}
	if lastUsed.Valid {
		tok.LastUsedAt = &lastUsed.Time
	}
	if err := scanJSON(metadata, &tok.Metadata); err != nil {
		return model.APIToken{}, err
	}
	return tok, nil
}

func scanCommand(row rowScanner) (model.Command, error) {
	var cmd model.Command
	var state string
	var env, metadata []byte
	var exitCode sql.NullInt64
	var startedAt, completedAt sql.NullTime
	err := row.Scan(&cmd.ID, &cmd.SandboxID, &state, &cmd.Command, &cmd.Cwd, &env, &cmd.Stdin, &cmd.TimeoutMS, &metadata, &cmd.PID, &exitCode, &cmd.Signal, &startedAt, &completedAt, &cmd.InsertedAt, &cmd.UpdatedAt)
	if err != nil {
		return model.Command{}, err
	}
	cmd.State = model.CommandState(state)
	if exitCode.Valid {
		v := int(exitCode.Int64)
		cmd.ExitCode = &v
	}
	if startedAt.Valid {
		cmd.StartedAt = &startedAt.Time
	}
	if completedAt.Valid {
		cmd.CompletedAt = &completedAt.Time
	}
	if err := scanJSON(env, &cmd.Env); err != nil {
		return model.Command{}, err
	}
	if err := scanJSON(metadata, &cmd.Metadata); err != nil {
		return model.Command{}, err
	}
	return cmd, nil
}

func scanWorkItem(row rowScanner) (model.WorkItem, error) {
	var item model.WorkItem
	var state string
	var metadata []byte
	err := row.Scan(&item.ID, &item.Title, &item.Description, &item.SourceType, &item.SourceID, &item.Actor, &state, &item.Priority, &metadata, &item.InsertedAt, &item.UpdatedAt)
	if err != nil {
		return model.WorkItem{}, err
	}
	item.State = model.WorkItemState(state)
	if err := scanJSON(metadata, &item.Metadata); err != nil {
		return model.WorkItem{}, err
	}
	return item, nil
}

func scanAgentRun(row rowScanner) (model.AgentRun, error) {
	var run model.AgentRun
	var sandboxID sql.NullString
	var state string
	var metadata []byte
	var startedAt, completedAt sql.NullTime
	err := row.Scan(&run.ID, &run.WorkItemID, &sandboxID, &run.Harness, &run.Model, &state, &run.Prompt, &run.Actor, &metadata, &startedAt, &completedAt, &run.InsertedAt, &run.UpdatedAt)
	if err != nil {
		return model.AgentRun{}, err
	}
	if sandboxID.Valid {
		run.SandboxID = sandboxID.String
	}
	run.State = model.AgentRunState(state)
	if startedAt.Valid {
		run.StartedAt = &startedAt.Time
	}
	if completedAt.Valid {
		run.CompletedAt = &completedAt.Time
	}
	if err := scanJSON(metadata, &run.Metadata); err != nil {
		return model.AgentRun{}, err
	}
	return run, nil
}

func scanEvent(row rowScanner) (model.Event, error) {
	var ev model.Event
	var commandID sql.NullString
	var data []byte
	err := row.Scan(&ev.ID, &ev.SandboxID, &commandID, &ev.Seq, &ev.Type, &ev.Source, &ev.Timestamp, &data)
	if err != nil {
		return model.Event{}, err
	}
	if commandID.Valid {
		ev.CommandID = commandID.String
	}
	if err := scanJSON(data, &ev.Data); err != nil {
		return model.Event{}, err
	}
	return ev, nil
}

func jsonParam(v any) any {
	if v == nil {
		return nil
	}
	b, err := json.Marshal(v)
	if err != nil {
		return nil
	}
	if string(b) == "null" {
		return nil
	}
	return b
}

func scanJSON(data []byte, out any) error {
	if len(data) == 0 {
		return nil
	}
	return json.Unmarshal(data, out)
}

func subtleCompare(a, b string) bool {
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}
