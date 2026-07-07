package job

import (
	"context"
	"errors"
	"testing"
	"time"

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

type fakeCommandDispatcher struct {
	store        store.Store
	exitCode     int
	delay        time.Duration
	dispatchErr  error
	dispatchedID string
}

func (f *fakeCommandDispatcher) DispatchCommand(ctx context.Context, sandboxID string, in store.CommandCreate) (model.Command, error) {
	if f.dispatchErr != nil {
		return model.Command{}, f.dispatchErr
	}
	cmd, err := f.store.CreateCommand(ctx, sandboxID, in)
	if err != nil {
		return model.Command{}, err
	}
	f.dispatchedID = cmd.ID
	go func() {
		if f.delay > 0 {
			time.Sleep(f.delay)
		}
		code := f.exitCode
		cmd.ExitCode = &code
		cmd.State = model.CommandExited
		_, _ = f.store.UpdateCommand(context.Background(), cmd)
	}()
	return cmd, nil
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
	cmdMgr := &fakeCommandDispatcher{store: st, exitCode: 0}
	mgr := NewManager(Config{Store: st, Sandboxes: sbMgr, Commands: cmdMgr, PollInterval: 10 * time.Millisecond})

	j, err := mgr.Start(context.Background(), CreateJobRequest{Repo: "https://example.com/repo.git", BaseRef: "main", Prompt: "do the thing"})
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
	cmdMgr := &fakeCommandDispatcher{store: st, exitCode: 0, delay: 2 * time.Second}
	mgr := NewManager(Config{Store: st, Sandboxes: sbMgr, Commands: cmdMgr, PollInterval: 10 * time.Millisecond})

	j, err := mgr.Start(context.Background(), CreateJobRequest{Repo: "repo", Prompt: "prompt"})
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
	cmdMgr := &fakeCommandDispatcher{store: st, exitCode: 0}
	mgr := NewManager(Config{Store: st, Sandboxes: sbMgr, Commands: cmdMgr, PollInterval: 10 * time.Millisecond})

	j, err := mgr.Start(context.Background(), CreateJobRequest{Repo: "repo", Prompt: "prompt"})
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
	cmdMgr := &fakeCommandDispatcher{store: st}
	mgr := NewManager(Config{Store: st, Sandboxes: sbMgr, Commands: cmdMgr, PollInterval: 10 * time.Millisecond})

	j, err := mgr.Start(context.Background(), CreateJobRequest{Repo: "repo", Prompt: "prompt"})
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

func TestManagerAgentCommandFailureMarksJobFailed(t *testing.T) {
	st := store.NewMemoryStore()
	sbMgr := &fakeSandboxManager{store: st}
	cmdMgr := &fakeCommandDispatcher{store: st, exitCode: 1}
	mgr := NewManager(Config{Store: st, Sandboxes: sbMgr, Commands: cmdMgr, PollInterval: 10 * time.Millisecond})

	j, err := mgr.Start(context.Background(), CreateJobRequest{Repo: "repo", Prompt: "prompt"})
	if err != nil {
		t.Fatal(err)
	}
	final := waitForTerminal(t, st, j.ID)
	if final.State != model.JobFailed {
		t.Fatalf("state=%s want failed", final.State)
	}
	if sbMgr.releaseCalls != 0 {
		t.Fatalf("expected no release call on agent command failure, got %d", sbMgr.releaseCalls)
	}
}
