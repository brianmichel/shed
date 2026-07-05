package scheduler

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/brianmichel/shed/internal/model"
	"github.com/brianmichel/shed/internal/store"
)

var ErrCancelled = errors.New("agent_run_cancelled")

type Processor interface {
	ProcessAgentRun(context.Context, model.AgentRun) error
}

type ProcessorFunc func(context.Context, model.AgentRun) error

func (fn ProcessorFunc) ProcessAgentRun(ctx context.Context, run model.AgentRun) error {
	return fn(ctx, run)
}

type Config struct {
	PollEvery     time.Duration
	RunTimeout    time.Duration
	RecoveryAfter time.Duration
	BatchSize     int
	MaxAttempts   int
}

type Scheduler struct {
	store     store.Store
	processor Processor
	cfg       Config
}

func New(st store.Store, processor Processor, cfg Config) (*Scheduler, error) {
	if st == nil {
		return nil, fmt.Errorf("store is required")
	}
	if processor == nil {
		return nil, fmt.Errorf("processor is required")
	}
	if cfg.PollEvery <= 0 {
		cfg.PollEvery = 5 * time.Second
	}
	if cfg.BatchSize <= 0 {
		cfg.BatchSize = 10
	}
	if cfg.MaxAttempts <= 0 {
		cfg.MaxAttempts = 1
	}
	return &Scheduler{store: st, processor: processor, cfg: cfg}, nil
}

func (s *Scheduler) Run(ctx context.Context) error {
	ticker := time.NewTicker(s.cfg.PollEvery)
	defer ticker.Stop()
	for {
		if _, err := s.RunOnce(ctx); err != nil {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func (s *Scheduler) RunOnce(ctx context.Context) (int, error) {
	if s.cfg.RecoveryAfter > 0 {
		if _, err := s.store.RecoverStaleAgentRuns(ctx, time.Now().UTC().Add(-s.cfg.RecoveryAfter)); err != nil {
			return 0, err
		}
	}
	runs, err := s.store.AcquireQueuedAgentRuns(ctx, s.cfg.BatchSize)
	if err != nil {
		return 0, err
	}
	processed := 0
	for _, run := range runs {
		select {
		case <-ctx.Done():
			return processed, ctx.Err()
		default:
		}
		if err := s.processOne(ctx, run); err != nil {
			return processed, err
		}
		processed++
	}
	return processed, nil
}

func (s *Scheduler) processOne(ctx context.Context, run model.AgentRun) error {
	if _, err := s.store.UpdateWorkItemState(ctx, run.WorkItemID, model.WorkItemRunning); err != nil {
		return err
	}
	_, _ = s.store.AppendFactoryEvent(ctx, run.WorkItemID, run.ID, "server.scheduler", "agent_run.step.started", map[string]any{"step": "processor", "attempt": run.Attempt})
	processCtx := ctx
	var cancel context.CancelFunc
	if s.cfg.RunTimeout > 0 {
		processCtx, cancel = context.WithTimeout(ctx, s.cfg.RunTimeout)
		defer cancel()
	}
	if err := s.processor.ProcessAgentRun(processCtx, run); err != nil {
		if errors.Is(err, context.DeadlineExceeded) || processCtx.Err() == context.DeadlineExceeded {
			if s.shouldRetry(run) {
				return s.requeue(ctx, run, "agent_run.processor.retry_scheduled", map[string]any{"reason": "timeout", "attempt": run.Attempt, "max_attempts": s.cfg.MaxAttempts})
			}
			_, _ = s.store.UpdateAgentRunState(ctx, run.ID, model.AgentRunTimedOut)
			_, _ = s.store.UpdateWorkItemState(ctx, run.WorkItemID, model.WorkItemFailed)
			_, _ = s.store.AppendFactoryEvent(ctx, run.WorkItemID, run.ID, "server.scheduler", "agent_run.processor.timed_out", map[string]any{"timeout_ms": s.cfg.RunTimeout.Milliseconds()})
			return nil
		}
		if errors.Is(err, ErrCancelled) {
			_, _ = s.store.UpdateAgentRunState(ctx, run.ID, model.AgentRunCancelled)
			_, _ = s.store.UpdateWorkItemState(ctx, run.WorkItemID, model.WorkItemCancelled)
			_, _ = s.store.AppendFactoryEvent(ctx, run.WorkItemID, run.ID, "server.scheduler", "agent_run.processor.cancelled", map[string]any{})
			return nil
		}
		if s.shouldRetry(run) {
			return s.requeue(ctx, run, "agent_run.processor.retry_scheduled", map[string]any{"reason": "failure", "attempt": run.Attempt, "max_attempts": s.cfg.MaxAttempts, "message": err.Error()})
		}
		_, _ = s.store.UpdateAgentRunState(ctx, run.ID, model.AgentRunFailed)
		_, _ = s.store.UpdateWorkItemState(ctx, run.WorkItemID, model.WorkItemFailed)
		_, _ = s.store.AppendFactoryEvent(ctx, run.WorkItemID, run.ID, "server.scheduler", "agent_run.processor.failed", map[string]any{"message": err.Error()})
		return nil
	}
	if _, err := s.store.UpdateAgentRunState(ctx, run.ID, model.AgentRunCompleted); err != nil {
		return err
	}
	if _, err := s.store.UpdateWorkItemState(ctx, run.WorkItemID, model.WorkItemCompleted); err != nil {
		return err
	}
	_, _ = s.store.AppendFactoryEvent(ctx, run.WorkItemID, run.ID, "server.scheduler", "agent_run.step.completed", map[string]any{"step": "processor", "attempt": run.Attempt})
	return nil
}

func (s *Scheduler) shouldRetry(run model.AgentRun) bool {
	return run.Attempt > 0 && run.Attempt < s.cfg.MaxAttempts
}

func (s *Scheduler) requeue(ctx context.Context, run model.AgentRun, eventType string, data map[string]any) error {
	if _, err := s.store.UpdateAgentRunState(ctx, run.ID, model.AgentRunQueued); err != nil {
		return err
	}
	_, _ = s.store.AppendFactoryEvent(ctx, run.WorkItemID, run.ID, "server.scheduler", eventType, data)
	return nil
}
