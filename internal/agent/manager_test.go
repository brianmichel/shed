package agent

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/brianmichel/shed/internal/model"
	"github.com/brianmichel/shed/internal/store"
)

var errBoom = errors.New("boom")

// fakeCommands simulates a sandbox command driver: DispatchCommand creates a
// real command row, then a background goroutine plays back scripted
// stdout lines (and optionally a terminal command event) as if a real
// process were running.
type fakeCommands struct {
	store         store.Store
	stdoutLines   []string
	terminalEvent string // "", "command.exit", "command.killed", "command.failed"
	stdinWrites   []string
	killCalls     int
	dispatchErr   error
}

func (f *fakeCommands) DispatchCommand(ctx context.Context, sandboxID string, in store.CommandCreate) (model.Command, error) {
	if f.dispatchErr != nil {
		return model.Command{}, f.dispatchErr
	}
	cmd, err := f.store.CreateCommand(ctx, sandboxID, in)
	if err != nil {
		return model.Command{}, err
	}
	go func() {
		time.Sleep(15 * time.Millisecond)
		cmd.State = model.CommandRunning
		_, _ = f.store.UpdateCommand(context.Background(), cmd)
		time.Sleep(15 * time.Millisecond)
		for _, line := range f.stdoutLines {
			_, _ = f.store.AppendEvent(context.Background(), sandboxID, cmd.ID, "test", "command.stdout", map[string]any{"chunk": line + "\n"})
			time.Sleep(5 * time.Millisecond)
		}
		if f.terminalEvent != "" {
			cmd.State = model.CommandExited
			_, _ = f.store.UpdateCommand(context.Background(), cmd)
			_, _ = f.store.AppendEvent(context.Background(), sandboxID, cmd.ID, "test", f.terminalEvent, map[string]any{})
		}
	}()
	return cmd, nil
}

func (f *fakeCommands) SendCommandStdin(ctx context.Context, sandboxID, commandID, data string) error {
	f.stdinWrites = append(f.stdinWrites, data)
	return nil
}

func (f *fakeCommands) KillCommand(ctx context.Context, sandboxID, commandID string) error {
	f.killCalls++
	return nil
}

func TestManagerRunHappyPath(t *testing.T) {
	ctx := context.Background()
	st := store.NewMemoryStore()
	sb, _, err := st.CreateSandbox(ctx, store.SandboxCreate{})
	if err != nil {
		t.Fatal(err)
	}
	fc := &fakeCommands{store: st, stdoutLines: []string{
		`{"type":"agent_start"}`,
		`{"type":"turn_start"}`,
		`{"type":"message_update","assistantMessageEvent":{"type":"text_delta","delta":"pong"}}`,
		`{"type":"agent_end","messages":[]}`,
	}}
	mgr := NewManager(Config{Store: st, Commands: fc, PollInterval: 10 * time.Millisecond})

	var startedID string
	result, err := mgr.Run(ctx, RunRequest{SandboxID: sb.ID, Prompt: "say pong", Provider: "lmstudio", Model: "test-model"}, func(id string) { startedID = id })
	if err != nil {
		t.Fatal(err)
	}
	if !result.Succeeded {
		t.Fatalf("result=%#v", result)
	}
	if startedID == "" {
		t.Fatal("onCommandStarted was not called")
	}
	if fc.killCalls != 1 {
		t.Fatalf("killCalls=%d want 1", fc.killCalls)
	}
	if len(fc.stdinWrites) != 2 {
		t.Fatalf("stdinWrites=%v want [prompt, abort]", fc.stdinWrites)
	}
	var promptMsg map[string]any
	if err := json.Unmarshal([]byte(fc.stdinWrites[0]), &promptMsg); err != nil {
		t.Fatal(err)
	}
	if promptMsg["type"] != "prompt" || promptMsg["message"] != "say pong" {
		t.Fatalf("prompt stdin=%v", promptMsg)
	}
	var abortMsg map[string]any
	if err := json.Unmarshal([]byte(fc.stdinWrites[1]), &abortMsg); err != nil {
		t.Fatal(err)
	}
	if abortMsg["type"] != "abort" {
		t.Fatalf("abort stdin=%v", abortMsg)
	}

	events, _, err := st.ListSandboxEvents(ctx, sb.ID, 0)
	if err != nil {
		t.Fatal(err)
	}
	var sawAgentEnd, sawTextDelta bool
	for _, ev := range events {
		switch ev.Type {
		case "agent.agent_end":
			sawAgentEnd = true
		case "agent.message_update":
			sawTextDelta = true
		}
	}
	if !sawAgentEnd || !sawTextDelta {
		t.Fatalf("expected relayed agent.* events, got: %#v", events)
	}
}

func TestManagerRunFailsIfProcessExitsWithoutAgentEnd(t *testing.T) {
	ctx := context.Background()
	st := store.NewMemoryStore()
	sb, _, err := st.CreateSandbox(ctx, store.SandboxCreate{})
	if err != nil {
		t.Fatal(err)
	}
	fc := &fakeCommands{store: st, stdoutLines: []string{`{"type":"agent_start"}`}, terminalEvent: "command.exit"}
	mgr := NewManager(Config{Store: st, Commands: fc, PollInterval: 10 * time.Millisecond})

	result, err := mgr.Run(ctx, RunRequest{SandboxID: sb.ID, Prompt: "x", Model: "test-model"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if result.Succeeded {
		t.Fatal("expected failure when process exits before agent_end")
	}
	if result.FailureReason == "" {
		t.Fatal("expected a failure reason")
	}
	if fc.killCalls != 0 {
		t.Fatalf("killCalls=%d want 0 (process already exited on its own)", fc.killCalls)
	}
}

func TestManagerRunPropagatesDispatchError(t *testing.T) {
	ctx := context.Background()
	st := store.NewMemoryStore()
	sb, _, err := st.CreateSandbox(ctx, store.SandboxCreate{})
	if err != nil {
		t.Fatal(err)
	}
	fc := &fakeCommands{store: st, dispatchErr: errBoom}
	mgr := NewManager(Config{Store: st, Commands: fc, PollInterval: 10 * time.Millisecond})

	if _, err := mgr.Run(ctx, RunRequest{SandboxID: sb.ID, Prompt: "x", Model: "test-model"}, nil); err == nil {
		t.Fatal("expected dispatch error to propagate")
	}
}
