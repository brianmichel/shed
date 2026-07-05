package store

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/brianmichel/shed/internal/model"
	_ "github.com/jackc/pgx/v5/stdlib"
)

func TestStoreContractMemory(t *testing.T) {
	runStoreContract(t, func(t *testing.T, ctx context.Context) Store {
		t.Helper()
		return NewMemoryStore()
	}, nil)
}

func TestStoreContractPostgresRestartRecovery(t *testing.T) {
	url := os.Getenv("SHED_TEST_POSTGRES_URL")
	if url == "" {
		t.Skip("set SHED_TEST_POSTGRES_URL to a dedicated Postgres test database to run")
	}
	ctx := context.Background()
	db, schema := openIsolatedPostgresTestDB(t, ctx, url)
	defer db.Close()
	defer func() { _, _ = db.ExecContext(context.Background(), `DROP SCHEMA IF EXISTS `+schema+` CASCADE`) }()

	var sameDB *sql.DB
	runStoreContract(t, func(t *testing.T, ctx context.Context) Store {
		t.Helper()
		st, err := NewMigratedPostgresStore(ctx, db)
		if err != nil {
			t.Fatal(err)
		}
		sameDB = db
		return st
	}, func(t *testing.T, ctx context.Context, snapshot contractSnapshot) {
		t.Helper()
		restarted, err := NewPostgresStore(sameDB)
		if err != nil {
			t.Fatal(err)
		}
		assertContractSnapshotRecovered(t, ctx, restarted, snapshot)
	})
}

type contractSnapshot struct {
	SandboxID string
	SessionID string
	CommandID string
	APIToken  string
	Events    int
}

func runStoreContract(t *testing.T, newStore func(*testing.T, context.Context) Store, assertRecovered func(*testing.T, context.Context, contractSnapshot)) {
	t.Helper()
	ctx := context.Background()
	st := newStore(t, ctx)

	sb, sess, err := st.CreateSandbox(ctx, SandboxCreate{Environment: "test", Template: "contract", TTL: time.Minute, Compute: "local", ComputeAPIVersion: "compute.v1", ComputeConfig: map[string]string{"region": "test"}, Metadata: map[string]string{"suite": "contract"}})
	if err != nil {
		t.Fatal(err)
	}
	if sess.AgentToken == "" {
		t.Fatal("expected one-time agent token")
	}
	if _, err := st.AuthenticateSession(ctx, sb.ID, sess.AgentToken); err != nil {
		t.Fatalf("authenticate session: %v", err)
	}

	if _, err := st.UpdateSandboxAllocation(ctx, sb.ID, SandboxAllocationUpdate{ComputePluginVersion: "test-plugin", ExternalAllocationID: "alloc-1", ComputeMetadata: map[string]string{"workspace_root": "/tmp/shed-test"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.UpdateSandboxState(ctx, sb.ID, model.SandboxReady); err != nil {
		t.Fatal(err)
	}
	if _, err := st.ExtendLease(ctx, sb.ID, time.Minute); err != nil {
		t.Fatal(err)
	}

	cmd, err := st.CreateCommand(ctx, sb.ID, CommandCreate{Command: "echo contract", Env: map[string]string{"A": "B"}, Metadata: map[string]string{"kind": "contract"}})
	if err != nil {
		t.Fatal(err)
	}
	cmd.State = model.CommandExited
	exitCode := 0
	cmd.ExitCode = &exitCode
	now := time.Now().UTC()
	cmd.StartedAt = &now
	cmd.CompletedAt = &now
	if _, err := st.UpdateCommand(ctx, cmd); err != nil {
		t.Fatal(err)
	}
	if _, err := st.AppendEvent(ctx, sb.ID, cmd.ID, "test", "command.stdout", map[string]any{"data": "ok"}); err != nil {
		t.Fatal(err)
	}

	tok, err := st.CreateAPIToken(ctx, APITokenCreate{Name: "contract"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.AuthenticateAPIToken(ctx, tok.Secret); err != nil {
		t.Fatalf("authenticate api token: %v", err)
	}

	value, created, err := st.RememberIdempotencyKey(ctx, "contract-key", "first")
	if err != nil || !created || value != "first" {
		t.Fatalf("first idempotency remember value=%q created=%v err=%v", value, created, err)
	}
	value, created, err = st.RememberIdempotencyKey(ctx, "contract-key", "second")
	if err != nil || created || value != "first" {
		t.Fatalf("second idempotency remember value=%q created=%v err=%v", value, created, err)
	}

	assertStoreContractState(t, ctx, st, sb.ID, sess.SessionID, cmd.ID)
	if assertRecovered != nil {
		events, _, err := st.ListSandboxEvents(ctx, sb.ID, EventListOptions{})
		if err != nil {
			t.Fatal(err)
		}
		assertRecovered(t, ctx, contractSnapshot{SandboxID: sb.ID, SessionID: sess.SessionID, CommandID: cmd.ID, APIToken: tok.Token.ID, Events: len(events)})
	}
}

func assertContractSnapshotRecovered(t *testing.T, ctx context.Context, st Store, snapshot contractSnapshot) {
	t.Helper()
	assertStoreContractState(t, ctx, st, snapshot.SandboxID, snapshot.SessionID, snapshot.CommandID)
	events, _, err := st.ListSandboxEvents(ctx, snapshot.SandboxID, EventListOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != snapshot.Events {
		t.Fatalf("recovered event count=%d want %d", len(events), snapshot.Events)
	}
	tokens, err := st.ListAPITokens(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, token := range tokens {
		if token.ID == snapshot.APIToken {
			return
		}
	}
	t.Fatalf("api token %s not recovered", snapshot.APIToken)
}

func assertStoreContractState(t *testing.T, ctx context.Context, st Store, sandboxID, sessionID, commandID string) {
	t.Helper()
	sb, err := st.GetSandbox(ctx, sandboxID)
	if err != nil {
		t.Fatal(err)
	}
	if sb.State != model.SandboxReady {
		t.Fatalf("sandbox state=%s want %s", sb.State, model.SandboxReady)
	}
	sess, err := st.GetSession(ctx, sessionID)
	if err != nil {
		t.Fatal(err)
	}
	if sess.SandboxID != sandboxID {
		t.Fatalf("session sandbox_id=%s want %s", sess.SandboxID, sandboxID)
	}
	cmd, err := st.GetCommand(ctx, sandboxID, commandID)
	if err != nil {
		t.Fatal(err)
	}
	if cmd.State != model.CommandExited || cmd.ExitCode == nil || *cmd.ExitCode != 0 {
		t.Fatalf("command=%#v, want exited with code 0", cmd)
	}
	commands, err := st.ListCommands(ctx, sandboxID, CommandListOptions{State: model.CommandExited})
	if err != nil {
		t.Fatal(err)
	}
	if len(commands) != 1 || commands[0].ID != commandID {
		t.Fatalf("commands=%#v, want command %s", commands, commandID)
	}
	events, next, err := st.ListCommandEvents(ctx, sandboxID, commandID, EventListOptions{Limit: 1})
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].CommandID != commandID || next != events[0].Seq {
		t.Fatalf("events=%#v next=%d, want one command event", events, next)
	}
}

func openIsolatedPostgresTestDB(t *testing.T, ctx context.Context, url string) (*sql.DB, string) {
	t.Helper()
	db, err := sql.Open("pgx", url)
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	schema := fmt.Sprintf("shed_test_%d", time.Now().UnixNano())
	if !strings.HasPrefix(schema, "shed_test_") {
		t.Fatalf("unsafe schema name %q", schema)
	}
	if _, err := db.ExecContext(ctx, `CREATE SCHEMA `+schema); err != nil {
		_ = db.Close()
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `SET search_path TO `+schema); err != nil {
		_ = db.Close()
		t.Fatal(err)
	}
	return db, schema
}
