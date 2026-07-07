// Package cli implements Shed's command-line client on top of Cobra: a thin
// wrapper over the public /v1 HTTP API, used the same way against a local
// `shed dev` or a remote `shed server`.
package cli

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/brianmichel/shed/internal/apiclient"
	"github.com/brianmichel/shed/internal/model"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// NewJobCommand builds the `shed job` command tree: run, status, logs, stop.
func NewJobCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "job",
		Short: "Create and observe software-factory jobs",
		Long: `Create and observe software-factory jobs.

A job allocates a sandbox, runs an agent command against a repo, and
(optionally) publishes the result — the same lifecycle whether it's created
here or through the /v1/jobs API directly.`,
		// Cobra's default Args validation ("legacyArgs") only rejects unknown
		// subcommands for the root command — a group command like this one
		// otherwise accepts stray args silently and just shows help. Reject
		// them explicitly so `shed job bogus` errors instead of looking like
		// a no-op `shed job`.
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				return fmt.Errorf("unknown command %q for %q", args[0], cmd.CommandPath())
			}
			return cmd.Help()
		},
	}
	cmd.AddCommand(newJobRunCmd(), newJobStatusCmd(), newJobLogsCmd(), newJobStopCmd())
	return cmd
}

func bindShedAddr(fs *pflag.FlagSet, addr *string) {
	fs.StringVar(addr, "shed-addr", envOr("SHED_ADDR", "http://127.0.0.1:6464"), "shed server API base URL")
}

func envOr(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}

func newJobRunCmd() *cobra.Command {
	var (
		addr                                                                                     string
		repo, baseRef, workBranch, prompt, computeClass, agentDriver, provider, model, scmDriver string
		detach                                                                                   bool
	)
	cmd := &cobra.Command{
		Use:   "run",
		Short: "Create and run a new job, then monitor it until it finishes",
		Args:  cobra.NoArgs,
		// Runtime/API failures (e.g. a 4xx from the server) aren't usage
		// mistakes, so don't dump the flag list underneath them.
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			c := apiclient.New(addr)
			j, err := c.CreateJob(cmd.Context(), apiclient.CreateJobRequest{
				Repo:         repo,
				BaseRef:      baseRef,
				WorkBranch:   workBranch,
				Prompt:       prompt,
				ComputeClass: computeClass,
				AgentDriver:  agentDriver,
				Provider:     provider,
				Model:        model,
				ScmDriver:    scmDriver,
			})
			if err != nil {
				return err
			}
			if detach {
				fmt.Printf("job %q queued\n", j.ID)
				return nil
			}
			return monitorJob(cmd.Context(), c, j.ID)
		},
	}
	fs := cmd.Flags()
	bindShedAddr(fs, &addr)
	fs.StringVar(&repo, "repo", "", "repository to operate on (required)")
	fs.StringVar(&baseRef, "base-ref", "main", "base ref to branch from")
	fs.StringVar(&workBranch, "work-branch", "", "branch name the agent should push to")
	fs.StringVar(&prompt, "prompt", "", "prompt describing the work (required)")
	fs.StringVar(&computeClass, "compute-class", "", "compute class to allocate")
	fs.StringVar(&agentDriver, "agent-driver", "", "agent driver to use")
	fs.StringVar(&provider, "provider", "lmstudio", "model provider for the agent driver")
	fs.StringVar(&model, "model", "", "model id for the agent driver (required)")
	fs.StringVar(&scmDriver, "scm-driver", "", "scm driver to use")
	fs.BoolVar(&detach, "detach", false, "return immediately instead of monitoring the job until it finishes")
	_ = cmd.MarkFlagRequired("repo")
	_ = cmd.MarkFlagRequired("prompt")
	_ = cmd.MarkFlagRequired("model")
	return cmd
}

// monitorJob polls a job until it reaches a terminal state, printing each
// state transition as it's observed, streaming the agent's rendered
// transcript live as soon as it starts, and printing the full detail once
// it finishes — the same "watch it happen" behavior as `nomad job run`'s
// post-submit evaluation monitor, extended with `job logs -f` folded in.
func monitorJob(ctx context.Context, c *apiclient.Client, jobID string) error {
	fmt.Printf("==> Monitoring job %q\n", jobID)
	mon := &jobMonitor{renderer: newLogRenderer(false)}
	for {
		j, err := c.GetJob(ctx, jobID)
		if err != nil {
			return err
		}
		// Drain any pending transcript output before announcing a state
		// change, so a transition line never lands mid-word inside a
		// still-streaming line of agent text.
		if j.AgentCommandID != "" {
			if err := mon.streamTranscript(ctx, c, jobID, j.AgentCommandID); err != nil {
				return err
			}
		}
		mon.printStateChange(jobID, j.State)
		if isTerminalJobState(j.State) {
			fmt.Printf("\n==> Job %q finished with status %q\n\n", jobID, j.State)
			printJobDetail(j)
			if j.State == model.JobFailed || j.State == model.JobCancelled {
				return fmt.Errorf("job %s did not succeed (state=%s)", jobID, j.State)
			}
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(500 * time.Millisecond):
		}
	}
}

// jobMonitor tracks the incremental state needed to render `job run`'s live
// view: the last-seen job state (to print only transitions) and the event
// cursor for the agent transcript stream.
type jobMonitor struct {
	renderer      *logRenderer
	lastState     model.JobState
	afterEvents   int64
	streamStarted bool
}

func (mon *jobMonitor) printStateChange(jobID string, state model.JobState) {
	if state == mon.lastState {
		return
	}
	if mon.lastState != "" {
		fmt.Printf("    %s: %s -> %s\n", jobID, mon.lastState, state)
	} else {
		fmt.Printf("    %s: %s\n", jobID, state)
	}
	mon.lastState = state
}

func (mon *jobMonitor) streamTranscript(ctx context.Context, c *apiclient.Client, jobID, agentCommandID string) error {
	if !mon.streamStarted {
		fmt.Println()
		mon.streamStarted = true
	}
	next, err := c.StreamJobEvents(ctx, jobID, mon.afterEvents, func(ev model.Event) {
		if ev.CommandID != agentCommandID {
			return
		}
		mon.renderer.handle(ev)
	})
	if err != nil {
		return err
	}
	mon.afterEvents = next
	return nil
}

func newJobStatusCmd() *cobra.Command {
	var addr string
	cmd := &cobra.Command{
		Use:          "status [job_id]",
		Short:        "Display status information about a job, or list all jobs",
		Args:         cobra.MaximumNArgs(1),
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			c := apiclient.New(addr)
			if len(args) == 1 {
				j, err := c.GetJob(cmd.Context(), args[0])
				if err != nil {
					return err
				}
				printJobDetail(j)
				return nil
			}
			jobs, err := c.ListJobs(cmd.Context())
			if err != nil {
				return err
			}
			printJobTable(jobs)
			return nil
		},
	}
	bindShedAddr(cmd.Flags(), &addr)
	return cmd
}

func newJobLogsCmd() *cobra.Command {
	var (
		addr   string
		follow bool
		raw    bool
	)
	cmd := &cobra.Command{
		Use:          "logs <job_id>",
		Short:        "Stream a job's agent activity",
		Args:         cobra.ExactArgs(1),
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runJobLogs(cmd.Context(), addr, args[0], follow, raw)
		},
	}
	fs := cmd.Flags()
	bindShedAddr(fs, &addr)
	fs.BoolVarP(&follow, "follow", "f", false, "follow the log stream until the job finishes")
	fs.BoolVar(&raw, "raw", false, "print the agent's raw JSONL stdout instead of rendering it")
	return cmd
}

func runJobLogs(ctx context.Context, addr, jobID string, follow, raw bool) error {
	c := apiclient.New(addr)
	j, err := c.GetJob(ctx, jobID)
	if err != nil {
		return err
	}
	for j.AgentCommandID == "" && !isTerminalJobState(j.State) {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(300 * time.Millisecond):
		}
		if j, err = c.GetJob(ctx, jobID); err != nil {
			return err
		}
	}
	if j.AgentCommandID == "" {
		return fmt.Errorf("job %s never started an agent command (state=%s)", jobID, j.State)
	}
	renderer := newLogRenderer(raw)
	onEvent := func(ev model.Event) {
		if ev.CommandID != j.AgentCommandID {
			return
		}
		renderer.handle(ev)
	}
	var after int64
	for {
		next, err := c.StreamJobEvents(ctx, jobID, after, onEvent)
		if err != nil {
			return err
		}
		after = next
		if !follow {
			return nil
		}
		if j, err = c.GetJob(ctx, jobID); err != nil {
			return err
		}
		if isTerminalJobState(j.State) {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(500 * time.Millisecond):
		}
	}
}

func newJobStopCmd() *cobra.Command {
	var addr string
	cmd := &cobra.Command{
		Use:          "stop <job_id>",
		Short:        "Cancel a running job",
		Args:         cobra.ExactArgs(1),
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			c := apiclient.New(addr)
			j, err := c.CancelJob(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			fmt.Printf("job %s %s\n", j.ID, j.State)
			return nil
		},
	}
	bindShedAddr(cmd.Flags(), &addr)
	return cmd
}

func isTerminalJobState(state model.JobState) bool {
	switch state {
	case model.JobSucceeded, model.JobFailed, model.JobCancelled:
		return true
	default:
		return false
	}
}
