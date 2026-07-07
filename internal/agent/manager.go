// Package agent implements Shed's coding-agent execution: turning a job's
// prompt into a running agent process inside its sandbox, and relaying that
// agent's activity into the sandbox's event log.
//
// The only built-in driver for now is "pi-rpc": it drives
// (https://github.com/earendil-works/pi) in RPC mode, a JSONL protocol over
// the stdin/stdout of one long-lived process. Shed's existing sandbox
// command primitive already supports exactly that transport (stdin writes,
// line-buffered stdout streaming), so no new sandbox-side execution
// machinery is needed — only the orchestration in this package.
package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/brianmichel/shed/internal/model"
	"github.com/brianmichel/shed/internal/store"
)

// CommandController is the sandbox-command surface the agent runner needs:
// dispatch a command, write to its stdin, and kill it. *server.Server
// satisfies this via Server.DispatchCommand/SendCommandStdin/KillCommand.
type CommandController interface {
	DispatchCommand(ctx context.Context, sandboxID string, in store.CommandCreate) (model.Command, error)
	SendCommandStdin(ctx context.Context, sandboxID, commandID, data string) error
	KillCommand(ctx context.Context, sandboxID, commandID string) error
}

type Config struct {
	Store        store.Store
	Commands     CommandController
	PollInterval time.Duration
}

type Manager struct {
	cfg Config
}

func NewManager(cfg Config) *Manager {
	if cfg.PollInterval <= 0 {
		cfg.PollInterval = 200 * time.Millisecond
	}
	return &Manager{cfg: cfg}
}

type RunRequest struct {
	SandboxID string
	Prompt    string
	Provider  string
	Model     string
}

type RunResult struct {
	Succeeded     bool
	FailureReason string
}

// Run drives one "pi-rpc" agent turn to completion: it starts `pi --mode
// rpc` as a long-running sandbox command, sends the job's prompt over its
// stdin, relays pi's JSONL events into the sandbox's event log as agent.*
// events, and terminates the process once pi reports agent_end.
// onCommandStarted is invoked with the dispatched command's ID as soon as
// it's known, so the caller can record it (e.g. Job.AgentCommandID) before
// the (potentially long) run completes.
func (m *Manager) Run(ctx context.Context, req RunRequest, onCommandStarted func(commandID string)) (RunResult, error) {
	cmd, err := m.cfg.Commands.DispatchCommand(ctx, req.SandboxID, m.renderCommand(req))
	if err != nil {
		return RunResult{}, err
	}
	if onCommandStarted != nil {
		onCommandStarted(cmd.ID)
	}

	if err := m.waitCommandRunning(ctx, req.SandboxID, cmd.ID); err != nil {
		return RunResult{}, fmt.Errorf("agent process did not start: %w", err)
	}

	promptLine, err := json.Marshal(map[string]any{"type": "prompt", "message": req.Prompt})
	if err != nil {
		return RunResult{}, err
	}
	if err := m.cfg.Commands.SendCommandStdin(ctx, req.SandboxID, cmd.ID, string(promptLine)+"\n"); err != nil {
		return RunResult{}, fmt.Errorf("send prompt: %w", err)
	}

	return m.relayUntilDone(ctx, req.SandboxID, cmd.ID)
}

func (m *Manager) renderCommand(req RunRequest) store.CommandCreate {
	parts := []string{"pi", "--mode", "rpc", "--no-session", "--no-extensions"}
	if req.Provider != "" {
		parts = append(parts, "--provider", shellQuote(req.Provider))
	}
	if req.Model != "" {
		parts = append(parts, "--model", shellQuote(req.Model))
	}
	return store.CommandCreate{
		Command:   strings.Join(parts, " "),
		Cwd:       "/workspace",
		Stdin:     true,
		TimeoutMS: 30 * 60 * 1000,
		Metadata:  map[string]string{"kind": "agent", "driver": "pi-rpc"},
	}
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

func (m *Manager) waitCommandRunning(ctx context.Context, sandboxID, commandID string) error {
	ticker := time.NewTicker(m.cfg.PollInterval)
	defer ticker.Stop()
	for {
		cmd, err := m.cfg.Store.GetCommand(ctx, sandboxID, commandID)
		if err == nil {
			switch cmd.State {
			case model.CommandRunning:
				return nil
			case model.CommandExited, model.CommandKilled, model.CommandFailed:
				return fmt.Errorf("command reached terminal state %s before running", cmd.State)
			}
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

// relayUntilDone tails the command's event log, decoding pi's JSONL stdout
// as it arrives and re-emitting each recognized event into the sandbox's
// own event log (as agent.<type>), until pi reports agent_end. It then
// aborts and kills the pi process to end the session cleanly.
func (m *Manager) relayUntilDone(ctx context.Context, sandboxID, commandID string) (RunResult, error) {
	ticker := time.NewTicker(m.cfg.PollInterval)
	defer ticker.Stop()
	var after int64
	var buf strings.Builder
	sawAgentEnd := false
	for {
		events, next, err := m.cfg.Store.ListCommandEvents(ctx, sandboxID, commandID, after)
		if err != nil {
			return RunResult{}, err
		}
		after = next
		for _, ev := range events {
			switch ev.Type {
			case "command.stdout":
				chunk, _ := ev.Data["chunk"].(string)
				buf.WriteString(chunk)
				for {
					raw := buf.String()
					idx := strings.IndexByte(raw, '\n')
					if idx < 0 {
						break
					}
					line := raw[:idx]
					buf.Reset()
					buf.WriteString(raw[idx+1:])
					if m.relayLine(ctx, sandboxID, commandID, line) {
						sawAgentEnd = true
					}
				}
			case "command.stderr":
				if chunk, ok := ev.Data["chunk"].(string); ok && strings.TrimSpace(chunk) != "" {
					_, _ = m.cfg.Store.AppendEvent(ctx, sandboxID, commandID, "agent.pi-rpc", "agent.stderr", map[string]any{"text": chunk})
				}
			case "command.exit", "command.killed", "command.failed":
				if !sawAgentEnd {
					return RunResult{Succeeded: false, FailureReason: fmt.Sprintf("agent process ended (%s) before reporting completion", ev.Type)}, nil
				}
			}
		}
		if sawAgentEnd {
			_ = m.cfg.Commands.SendCommandStdin(ctx, sandboxID, commandID, `{"type":"abort"}`+"\n")
			_ = m.cfg.Commands.KillCommand(ctx, sandboxID, commandID)
			return RunResult{Succeeded: true}, nil
		}
		select {
		case <-ctx.Done():
			return RunResult{}, ctx.Err()
		case <-ticker.C:
		}
	}
}

// relayLine decodes one line of pi's RPC stdout as JSON and re-emits it as
// an agent.<type> event. It returns true if the line was an agent_end
// event. Non-JSON or untyped lines (extension noise, blank lines) are
// ignored rather than treated as errors.
func (m *Manager) relayLine(ctx context.Context, sandboxID, commandID, line string) bool {
	line = strings.TrimRight(line, "\r")
	if line == "" {
		return false
	}
	var ev map[string]any
	if err := json.Unmarshal([]byte(line), &ev); err != nil {
		return false
	}
	evType, _ := ev["type"].(string)
	if evType == "" {
		return false
	}
	_, _ = m.cfg.Store.AppendEvent(ctx, sandboxID, commandID, "agent.pi-rpc", "agent."+evType, ev)
	return evType == "agent_end"
}
