package job

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/brianmichel/shed/internal/agent"
	"github.com/brianmichel/shed/internal/model"
	"github.com/brianmichel/shed/internal/store"
)

type fakeSandboxManager struct {
	store        store.Store
	createErr    error
	releaseCalls int
}

func (f *fakeSandboxManager) CreateSandbox(ctx context.Context, in store.SandboxCreate) (model.Sandbox, model.ClientSession, error) {
	if f.createErr != nil {
		return model.Sandbox{}, model.ClientSession{}, f.createErr
	}
	sb, sess, err := f.store.CreateSandbox(ctx, in)
	if err != nil {
		return model.Sandbox{}, model.ClientSession{}, err
	}
	sb, err = f.store.UpdateSandboxState(ctx, sb.ID, model.SandboxReady)
	return sb, sess, err
}

func (f *fakeSandboxManager) ReleaseSandbox(ctx context.Context, sandboxID, reason string) (model.Sandbox, error) {
	f.releaseCalls++
	return f.store.UpdateSandboxState(ctx, sandboxID, model.SandboxReleased)
}

// fakeAgentRunner simulates agent.Manager.Run without spawning any real
// process: it reports a fake command id, optionally waits (respecting
// ctx cancellation, to simulate a long-running agent turn), then returns
// a scripted result.
type fakeAgentRunner struct {
	succeeded     bool
	failureReason string
	err           error
	delay         time.Duration
	commandID     string
}

func (f *fakeAgentRunner) Run(ctx context.Context, req agent.RunRequest, onCommandStarted func(commandID string)) (agent.RunResult, error) {
	if f.err != nil {
		return agent.RunResult{}, f.err
	}
	id := f.commandID
	if id == "" {
		id = "cmd_fake"
	}
	if onCommandStarted != nil {
		onCommandStarted(id)
	}
	if f.delay > 0 {
		select {
		case <-time.After(f.delay):
		case <-ctx.Done():
			return agent.RunResult{}, ctx.Err()
		}
	}
	return agent.RunResult{Succeeded: f.succeeded, FailureReason: f.failureReason}, nil
}

func waitForTerminal(t *testing.T, st store.Store, jobID string) model.Job {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		j, err := st.GetJob(context.Background(), jobID)
		if err != nil {
			t.Fatal(err)
		}
		if isTerminalJobState(j.State) {
			return j
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("job %s did not reach a terminal state in time", jobID)
	return model.Job{}
}

func waitForState(t *testing.T, st store.Store, jobID string, want model.JobState) model.Job {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		j, err := st.GetJob(context.Background(), jobID)
		if err != nil {
			t.Fatal(err)
		}
		if j.State == want && j.AgentCommandID != "" {
			return j
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("job %s did not reach state %s in time", jobID, want)
	return model.Job{}
}

func TestManagerHappyPath(t *testing.T) {
	st := store.NewMemoryStore()
	sbMgr := &fakeSandboxManager{store: st}
	agentMgr := &fakeAgentRunner{succeeded: true}
	mgr := NewManager(Config{Store: st, Sandboxes: sbMgr, Agent: agentMgr, PollInterval: 10 * time.Millisecond})

	j, err := mgr.Start(context.Background(), CreateJobRequest{Repo: "https://example.com/repo.git", BaseRef: "main", Prompt: "do the thing", Model: "test-model"})
	if err != nil {
		t.Fatal(err)
	}
	if j.State != model.JobQueued {
		t.Fatalf("state=%s want queued", j.State)
	}

	final := waitForTerminal(t, st, j.ID)
	if final.State != model.JobSucceeded {
		t.Fatalf("state=%s reason=%s", final.State, final.FailureReason)
	}
	if final.SandboxID == "" || final.AgentCommandID == "" {
		t.Fatalf("missing sandbox/agent command id: %#v", final)
	}
	if sbMgr.releaseCalls != 1 {
		t.Fatalf("expected sandbox to be released on success, got %d release calls", sbMgr.releaseCalls)
	}
}

func TestManagerCancelMidFlight(t *testing.T) {
	st := store.NewMemoryStore()
	sbMgr := &fakeSandboxManager{store: st}
	agentMgr := &fakeAgentRunner{succeeded: true, delay: 2 * time.Second}
	mgr := NewManager(Config{Store: st, Sandboxes: sbMgr, Agent: agentMgr, PollInterval: 10 * time.Millisecond})

	j, err := mgr.Start(context.Background(), CreateJobRequest{Repo: "repo", Prompt: "prompt", Model: "test-model"})
	if err != nil {
		t.Fatal(err)
	}
	waitForState(t, st, j.ID, model.JobRunningAgent)

	if _, err := mgr.Cancel(context.Background(), j.ID); err != nil {
		t.Fatal(err)
	}

	final := waitForTerminal(t, st, j.ID)
	if final.State != model.JobCancelled {
		t.Fatalf("state=%s want cancelled", final.State)
	}
	if sbMgr.releaseCalls != 0 {
		t.Fatalf("expected sandbox left for the lease sweeper on cancel, got %d release calls", sbMgr.releaseCalls)
	}
}

func TestManagerCancelAlreadyTerminalFails(t *testing.T) {
	st := store.NewMemoryStore()
	sbMgr := &fakeSandboxManager{store: st}
	agentMgr := &fakeAgentRunner{succeeded: true}
	mgr := NewManager(Config{Store: st, Sandboxes: sbMgr, Agent: agentMgr, PollInterval: 10 * time.Millisecond})

	j, err := mgr.Start(context.Background(), CreateJobRequest{Repo: "repo", Prompt: "prompt", Model: "test-model"})
	if err != nil {
		t.Fatal(err)
	}
	waitForTerminal(t, st, j.ID)

	if _, err := mgr.Cancel(context.Background(), j.ID); !errors.Is(err, ErrJobNotCancellable) {
		t.Fatalf("err=%v want ErrJobNotCancellable", err)
	}
}

func TestManagerAllocateFailureMarksJobFailed(t *testing.T) {
	st := store.NewMemoryStore()
	sbMgr := &fakeSandboxManager{store: st, createErr: errors.New("allocate boom")}
	agentMgr := &fakeAgentRunner{succeeded: true}
	mgr := NewManager(Config{Store: st, Sandboxes: sbMgr, Agent: agentMgr, PollInterval: 10 * time.Millisecond})

	j, err := mgr.Start(context.Background(), CreateJobRequest{Repo: "repo", Prompt: "prompt", Model: "test-model"})
	if err != nil {
		t.Fatal(err)
	}
	final := waitForTerminal(t, st, j.ID)
	if final.State != model.JobFailed {
		t.Fatalf("state=%s want failed", final.State)
	}
	if final.FailureReason == "" {
		t.Fatal("expected a failure reason to be recorded")
	}
	if sbMgr.releaseCalls != 0 {
		t.Fatalf("expected no release call on allocate failure, got %d", sbMgr.releaseCalls)
	}
}

func TestManagerAgentRunFailureMarksJobFailed(t *testing.T) {
	st := store.NewMemoryStore()
	sbMgr := &fakeSandboxManager{store: st}
	agentMgr := &fakeAgentRunner{succeeded: false, failureReason: "agent process crashed"}
	mgr := NewManager(Config{Store: st, Sandboxes: sbMgr, Agent: agentMgr, PollInterval: 10 * time.Millisecond})

	j, err := mgr.Start(context.Background(), CreateJobRequest{Repo: "repo", Prompt: "prompt", Model: "test-model"})
	if err != nil {
		t.Fatal(err)
	}
	final := waitForTerminal(t, st, j.ID)
	if final.State != model.JobFailed {
		t.Fatalf("state=%s want failed", final.State)
	}
	if final.FailureReason != "agent process crashed" {
		t.Fatalf("failure_reason=%q", final.FailureReason)
	}
	if sbMgr.releaseCalls != 0 {
		t.Fatalf("expected no release call on agent run failure, got %d", sbMgr.releaseCalls)
	}
}

func TestManagerAgentRunErrorMarksJobFailed(t *testing.T) {
	st := store.NewMemoryStore()
	sbMgr := &fakeSandboxManager{store: st}
	agentMgr := &fakeAgentRunner{err: errors.New("dispatch boom")}
	mgr := NewManager(Config{Store: st, Sandboxes: sbMgr, Agent: agentMgr, PollInterval: 10 * time.Millisecond})

	j, err := mgr.Start(context.Background(), CreateJobRequest{Repo: "repo", Prompt: "prompt", Model: "test-model"})
	if err != nil {
		t.Fatal(err)
	}
	final := waitForTerminal(t, st, j.ID)
	if final.State != model.JobFailed {
		t.Fatalf("state=%s want failed", final.State)
	}
	if sbMgr.releaseCalls != 0 {
		t.Fatalf("expected no release call on agent run error, got %d", sbMgr.releaseCalls)
	}
}
