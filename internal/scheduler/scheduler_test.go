package scheduler

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/brianmichel/shed/internal/model"
	"github.com/brianmichel/shed/internal/store"
)

func TestRunOnceCompletesQueuedAgentRun(t *testing.T) {
	ctx := context.Background()
	st := store.NewMemoryStore()
	item, err := st.CreateWorkItem(ctx, store.WorkItemCreate{Title: "test"})
	if err != nil {
		t.Fatal(err)
	}
	run, err := st.CreateAgentRun(ctx, item.ID, store.AgentRunCreate{Harness: "test"})
	if err != nil {
		t.Fatal(err)
	}
	var processed []string
	s, err := New(st, ProcessorFunc(func(_ context.Context, run model.AgentRun) error {
		processed = append(processed, run.ID)
		return nil
	}), Config{BatchSize: 5})
	if err != nil {
		t.Fatal(err)
	}

	count, err := s.RunOnce(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 || len(processed) != 1 || processed[0] != run.ID {
		t.Fatalf("count=%d processed=%v, want one run %s", count, processed, run.ID)
	}
	updatedRun, err := st.GetAgentRun(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if updatedRun.State != model.AgentRunCompleted || updatedRun.StartedAt == nil || updatedRun.CompletedAt == nil {
		t.Fatalf("updated run=%#v, want completed with timestamps", updatedRun)
	}
	updatedItem, err := st.GetWorkItem(ctx, item.ID)
	if err != nil {
		t.Fatal(err)
	}
	if updatedItem.State != model.WorkItemCompleted {
		t.Fatalf("work item state=%s want completed", updatedItem.State)
	}
}

func TestRunOnceMarksFailureAndContinues(t *testing.T) {
	ctx := context.Background()
	st := store.NewMemoryStore()
	item, err := st.CreateWorkItem(ctx, store.WorkItemCreate{Title: "test"})
	if err != nil {
		t.Fatal(err)
	}
	run, err := st.CreateAgentRun(ctx, item.ID, store.AgentRunCreate{Harness: "test"})
	if err != nil {
		t.Fatal(err)
	}
	s, err := New(st, ProcessorFunc(func(context.Context, model.AgentRun) error { return errors.New("boom") }), Config{})
	if err != nil {
		t.Fatal(err)
	}

	count, err := s.RunOnce(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("count=%d want 1", count)
	}
	updatedRun, err := st.GetAgentRun(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if updatedRun.State != model.AgentRunFailed || updatedRun.CompletedAt == nil {
		t.Fatalf("updated run=%#v, want failed with completed_at", updatedRun)
	}
	events, _, err := st.ListAgentRunEvents(ctx, run.ID, store.EventListOptions{})
	if err != nil {
		t.Fatal(err)
	}
	var foundFailure bool
	for _, ev := range events {
		if ev.Type == "agent_run.processor.failed" {
			foundFailure = true
		}
	}
	if !foundFailure {
		t.Fatalf("processor failure event missing: %#v", events)
	}
}

func TestRunOnceMarksTimeout(t *testing.T) {
	ctx := context.Background()
	st := store.NewMemoryStore()
	item, err := st.CreateWorkItem(ctx, store.WorkItemCreate{Title: "test"})
	if err != nil {
		t.Fatal(err)
	}
	run, err := st.CreateAgentRun(ctx, item.ID, store.AgentRunCreate{})
	if err != nil {
		t.Fatal(err)
	}
	s, err := New(st, ProcessorFunc(func(ctx context.Context, _ model.AgentRun) error {
		<-ctx.Done()
		return ctx.Err()
	}), Config{RunTimeout: time.Nanosecond})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.RunOnce(ctx); err != nil {
		t.Fatal(err)
	}
	updatedRun, err := st.GetAgentRun(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if updatedRun.State != model.AgentRunTimedOut || updatedRun.CompletedAt == nil {
		t.Fatalf("updated run=%#v, want timed out", updatedRun)
	}
	updatedItem, err := st.GetWorkItem(ctx, item.ID)
	if err != nil {
		t.Fatal(err)
	}
	if updatedItem.State != model.WorkItemFailed {
		t.Fatalf("work item state=%s want failed", updatedItem.State)
	}
}

func TestRunOnceMarksCancelled(t *testing.T) {
	ctx := context.Background()
	st := store.NewMemoryStore()
	item, err := st.CreateWorkItem(ctx, store.WorkItemCreate{Title: "test"})
	if err != nil {
		t.Fatal(err)
	}
	run, err := st.CreateAgentRun(ctx, item.ID, store.AgentRunCreate{})
	if err != nil {
		t.Fatal(err)
	}
	s, err := New(st, ProcessorFunc(func(context.Context, model.AgentRun) error { return ErrCancelled }), Config{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.RunOnce(ctx); err != nil {
		t.Fatal(err)
	}
	updatedRun, err := st.GetAgentRun(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if updatedRun.State != model.AgentRunCancelled || updatedRun.CompletedAt == nil {
		t.Fatalf("updated run=%#v, want cancelled", updatedRun)
	}
	updatedItem, err := st.GetWorkItem(ctx, item.ID)
	if err != nil {
		t.Fatal(err)
	}
	if updatedItem.State != model.WorkItemCancelled {
		t.Fatalf("work item state=%s want cancelled", updatedItem.State)
	}
}

func TestRunOnceSkipsQueuedRunCancelledBeforeAcquire(t *testing.T) {
	ctx := context.Background()
	st := store.NewMemoryStore()
	item, err := st.CreateWorkItem(ctx, store.WorkItemCreate{Title: "test"})
	if err != nil {
		t.Fatal(err)
	}
	run, err := st.CreateAgentRun(ctx, item.ID, store.AgentRunCreate{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.UpdateAgentRunState(ctx, run.ID, model.AgentRunCancelled); err != nil {
		t.Fatal(err)
	}
	var processed bool
	s, err := New(st, ProcessorFunc(func(context.Context, model.AgentRun) error { processed = true; return nil }), Config{})
	if err != nil {
		t.Fatal(err)
	}
	count, err := s.RunOnce(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if count != 0 || processed {
		t.Fatalf("count=%d processed=%v, want skipped", count, processed)
	}
}

func TestRunOnceHonorsBatchSize(t *testing.T) {
	ctx := context.Background()
	st := store.NewMemoryStore()
	for i := 0; i < 3; i++ {
		item, err := st.CreateWorkItem(ctx, store.WorkItemCreate{Title: "test"})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := st.CreateAgentRun(ctx, item.ID, store.AgentRunCreate{}); err != nil {
			t.Fatal(err)
		}
	}
	s, err := New(st, ProcessorFunc(func(context.Context, model.AgentRun) error { return nil }), Config{BatchSize: 2})
	if err != nil {
		t.Fatal(err)
	}
	count, err := s.RunOnce(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatalf("count=%d want 2", count)
	}
	queued, err := st.ListAgentRuns(ctx, store.AgentRunListOptions{State: model.AgentRunQueued})
	if err != nil {
		t.Fatal(err)
	}
	if len(queued) != 1 {
		t.Fatalf("queued=%d want 1", len(queued))
	}
}

func TestAcquireQueuedAgentRunsPreventsDuplicateProcessing(t *testing.T) {
	ctx := context.Background()
	st := store.NewMemoryStore()
	item, err := st.CreateWorkItem(ctx, store.WorkItemCreate{Title: "test"})
	if err != nil {
		t.Fatal(err)
	}
	run, err := st.CreateAgentRun(ctx, item.ID, store.AgentRunCreate{})
	if err != nil {
		t.Fatal(err)
	}
	first, err := st.AcquireQueuedAgentRuns(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(first) != 1 || first[0].ID != run.ID || first[0].State != model.AgentRunRunning {
		t.Fatalf("first acquire=%#v", first)
	}
	second, err := st.AcquireQueuedAgentRuns(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(second) != 0 {
		t.Fatalf("second acquire=%#v, want none", second)
	}
}

func TestNewValidationAndDefaults(t *testing.T) {
	if _, err := New(nil, ProcessorFunc(func(context.Context, model.AgentRun) error { return nil }), Config{}); err == nil {
		t.Fatal("expected nil store error")
	}
	if _, err := New(store.NewMemoryStore(), nil, Config{}); err == nil {
		t.Fatal("expected nil processor error")
	}
	s, err := New(store.NewMemoryStore(), ProcessorFunc(func(context.Context, model.AgentRun) error { return nil }), Config{})
	if err != nil {
		t.Fatal(err)
	}
	if s.cfg.PollEvery != 5*time.Second || s.cfg.BatchSize != 10 {
		t.Fatalf("defaults=%#v", s.cfg)
	}
}
