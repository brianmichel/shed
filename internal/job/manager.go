// Package job implements the software-factory orchestrator: it turns a repo
// + prompt into an allocated sandbox running one long-lived agent command,
// and tracks that as a Job through to success, failure, or cancellation.
package job

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/brianmichel/shed/internal/model"
	"github.com/brianmichel/shed/internal/store"
)

// CommandDispatcher dispatches a command against a sandbox. *server.Server
// satisfies this via Server.DispatchCommand.
type CommandDispatcher interface {
	DispatchCommand(ctx context.Context, sandboxID string, in store.CommandCreate) (model.Command, error)
}

// SandboxManager creates and releases the sandbox a job runs in.
// *server.Server satisfies this via Server.CreateSandbox/Server.ReleaseSandbox.
type SandboxManager interface {
	CreateSandbox(ctx context.Context, in store.SandboxCreate) (model.Sandbox, model.ClientSession, error)
	ReleaseSandbox(ctx context.Context, sandboxID, reason string) (model.Sandbox, error)
}

// ErrJobNotCancellable is returned when Cancel is called on a job that has
// already reached a terminal state.
var ErrJobNotCancellable = errors.New("job_not_cancellable")

type Config struct {
	Store        store.Store
	Sandboxes    SandboxManager
	Commands     CommandDispatcher
	PollInterval time.Duration
}

type Manager struct {
	cfg     Config
	mu      sync.Mutex
	cancels map[string]context.CancelFunc
}

func NewManager(cfg Config) *Manager {
	if cfg.PollInterval <= 0 {
		cfg.PollInterval = 200 * time.Millisecond
	}
	return &Manager{cfg: cfg, cancels: map[string]context.CancelFunc{}}
}

type CreateJobRequest struct {
	Repo         string
	BaseRef      string
	WorkBranch   string
	Prompt       string
	ComputeClass string
	AgentDriver  string
	ScmDriver    string
	Trigger      model.JobTrigger
	Metadata     map[string]string
}

// Start persists a new Job and begins running its state machine in the
// background, returning as soon as the job is queued.
func (m *Manager) Start(ctx context.Context, req CreateJobRequest) (model.Job, error) {
	j, err := m.cfg.Store.CreateJob(ctx, store.JobCreate{
		Repo:         req.Repo,
		BaseRef:      req.BaseRef,
		WorkBranch:   req.WorkBranch,
		Prompt:       req.Prompt,
		ComputeClass: req.ComputeClass,
		AgentDriver:  req.AgentDriver,
		ScmDriver:    req.ScmDriver,
		Trigger:      req.Trigger,
		Metadata:     req.Metadata,
	})
	if err != nil {
		return model.Job{}, err
	}
	runCtx, cancel := context.WithCancel(context.Background())
	m.mu.Lock()
	m.cancels[j.ID] = cancel
	m.mu.Unlock()
	go m.run(runCtx, j)
	return j, nil
}

// Cancel requests cancellation of a running job. It is a no-op error if the
// job has already reached a terminal state.
func (m *Manager) Cancel(ctx context.Context, jobID string) (model.Job, error) {
	m.mu.Lock()
	cancel, ok := m.cancels[jobID]
	m.mu.Unlock()
	j, err := m.cfg.Store.GetJob(ctx, jobID)
	if err != nil {
		return model.Job{}, err
	}
	if !ok || isTerminalJobState(j.State) {
		return model.Job{}, ErrJobNotCancellable
	}
	cancel()
	return j, nil
}

func (m *Manager) clearCancel(jobID string) {
	m.mu.Lock()
	delete(m.cancels, jobID)
	m.mu.Unlock()
}

func (m *Manager) run(ctx context.Context, j model.Job) {
	defer m.clearCancel(j.ID)

	fail := func(reason string) {
		j.State = model.JobFailed
		j.FailureReason = reason
		updated, err := m.cfg.Store.UpdateJob(context.Background(), j)
		if err == nil {
			j = updated
		}
		if j.SandboxID != "" {
			_, _ = m.cfg.Store.AppendEvent(context.Background(), j.SandboxID, "", "server.job", "job.failed", map[string]any{"job_id": j.ID, "reason": reason})
		}
	}

	j.State = model.JobAllocatingCompute
	j, err := m.cfg.Store.UpdateJob(ctx, j)
	if err != nil {
		return
	}

	sb, _, err := m.cfg.Sandboxes.CreateSandbox(ctx, store.SandboxCreate{
		Environment:  "job",
		Template:     "software-factory",
		ComputeClass: j.ComputeClass,
		Metadata:     j.Metadata,
	})
	if err != nil {
		fail("allocate_compute: " + err.Error())
		return
	}
	j.SandboxID = sb.ID
	if j, err = m.cfg.Store.UpdateJob(ctx, j); err != nil {
		fail(err.Error())
		return
	}
	_, _ = m.cfg.Store.AppendEvent(ctx, j.SandboxID, "", "server.job", "job.allocating_compute", map[string]any{"job_id": j.ID})

	if err := m.waitSandboxReady(ctx, sb.ID); err != nil {
		if ctx.Err() != nil {
			m.markCancelled(j)
			return
		}
		fail("sandbox_not_ready: " + err.Error())
		return
	}

	j.State = model.JobRunningAgent
	if j, err = m.cfg.Store.UpdateJob(ctx, j); err != nil {
		fail(err.Error())
		return
	}
	_, _ = m.cfg.Store.AppendEvent(ctx, j.SandboxID, "", "server.job", "job.running_agent", map[string]any{"job_id": j.ID})

	cmd, err := m.cfg.Commands.DispatchCommand(ctx, j.SandboxID, m.renderAgentCommand(j))
	if err != nil {
		if ctx.Err() != nil {
			m.markCancelled(j)
			return
		}
		fail("dispatch_agent: " + err.Error())
		return
	}
	j.AgentCommandID = cmd.ID
	if j, err = m.cfg.Store.UpdateJob(ctx, j); err != nil {
		fail(err.Error())
		return
	}

	final, err := m.waitCommandTerminal(ctx, j.SandboxID, cmd.ID)
	if err != nil {
		if ctx.Err() != nil {
			m.markCancelled(j)
			return
		}
		fail("agent_wait: " + err.Error())
		return
	}
	if final.State != model.CommandExited || (final.ExitCode != nil && *final.ExitCode != 0) {
		fail("agent command did not exit successfully")
		return
	}

	_, _ = m.cfg.Sandboxes.ReleaseSandbox(context.Background(), j.SandboxID, "job_succeeded")
	j.State = model.JobSucceeded
	if j, err = m.cfg.Store.UpdateJob(context.Background(), j); err == nil {
		_, _ = m.cfg.Store.AppendEvent(context.Background(), j.SandboxID, "", "server.job", "job.succeeded", map[string]any{"job_id": j.ID})
	}
}

func (m *Manager) markCancelled(j model.Job) {
	j.State = model.JobCancelled
	j.FailureReason = "cancelled"
	updated, err := m.cfg.Store.UpdateJob(context.Background(), j)
	if err == nil {
		j = updated
	}
	if j.SandboxID != "" {
		_, _ = m.cfg.Store.AppendEvent(context.Background(), j.SandboxID, "", "server.job", "job.cancelled", map[string]any{"job_id": j.ID})
	}
}

// renderAgentCommand is a stub for milestone 1; milestone 2 replaces this
// with agent.Manager.Render output.
func (m *Manager) renderAgentCommand(j model.Job) store.CommandCreate {
	return store.CommandCreate{
		Command:   "sleep 1 && echo done",
		Cwd:       "/workspace",
		TimeoutMS: 60000,
		Metadata:  map[string]string{"job_id": j.ID, "kind": "agent"},
	}
}

func (m *Manager) waitSandboxReady(ctx context.Context, sandboxID string) error {
	ticker := time.NewTicker(m.cfg.PollInterval)
	defer ticker.Stop()
	for {
		sb, err := m.cfg.Store.GetSandbox(ctx, sandboxID)
		if err == nil {
			switch sb.State {
			case model.SandboxReady:
				return nil
			case model.SandboxFailed, model.SandboxReleased:
				return errors.New("sandbox reached terminal state before becoming ready")
			}
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func (m *Manager) waitCommandTerminal(ctx context.Context, sandboxID, commandID string) (model.Command, error) {
	ticker := time.NewTicker(m.cfg.PollInterval)
	defer ticker.Stop()
	for {
		cmd, err := m.cfg.Store.GetCommand(ctx, sandboxID, commandID)
		if err == nil && isCommandTerminal(cmd.State) {
			return cmd, nil
		}
		select {
		case <-ctx.Done():
			return model.Command{}, ctx.Err()
		case <-ticker.C:
		}
	}
}

func isCommandTerminal(state model.CommandState) bool {
	switch state {
	case model.CommandExited, model.CommandKilled, model.CommandFailed:
		return true
	default:
		return false
	}
}

func isTerminalJobState(state model.JobState) bool {
	switch state {
	case model.JobSucceeded, model.JobFailed, model.JobCancelled:
		return true
	default:
		return false
	}
}
