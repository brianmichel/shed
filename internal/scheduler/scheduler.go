package scheduler

import (
	"context"
	"fmt"
	"time"

	"github.com/brianmichel/shed/internal/model"
	"github.com/brianmichel/shed/internal/store"
)

type Processor interface {
	ProcessAgentRun(context.Context, model.AgentRun) error
}

type ProcessorFunc func(context.Context, model.AgentRun) error

func (fn ProcessorFunc) ProcessAgentRun(ctx context.Context, run model.AgentRun) error {
	return fn(ctx, run)
}

type Config struct {
	PollEvery time.Duration
	BatchSize int
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
	runs, err := s.store.ListAgentRuns(ctx, store.AgentRunListOptions{State: model.AgentRunQueued, Page: store.Page{Limit: s.cfg.BatchSize}})
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
	if _, err := s.store.UpdateAgentRunState(ctx, run.ID, model.AgentRunRunning); err != nil {
		return err
	}
	if err := s.processor.ProcessAgentRun(ctx, run); err != nil {
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
	return nil
}
