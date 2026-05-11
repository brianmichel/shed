package runner

import (
	"fmt"
	"io"
	"log"
	"os/exec"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
)

// SendFunc is the callback a Runner uses to push messages back to Garden.
type SendFunc func(msgType string, payload map[string]any)

// Config holds the parameters for a single command execution.
type Config struct {
	CommandID string
	Command   string
	Cwd       string
	Env       map[string]string
	Stdin     bool
	Send      SendFunc
}

// Runner executes a single OS command and streams output back via Send.
type Runner struct {
	cfg        Config
	cmd        *exec.Cmd
	stdin      io.WriteCloser
	mu         sync.Mutex
	done       chan struct{}
	startedAt  time.Time
	stdoutSeq  atomic.Int64
	stderrSeq  atomic.Int64
}

func New(cfg Config) *Runner {
	return &Runner{cfg: cfg, done: make(chan struct{})}
}

func (r *Runner) Run() {
	r.cfg.Send("command.accepted", map[string]any{
		"command_id": r.cfg.CommandID,
		"state":      "starting",
	})

	cmd := exec.Command("/bin/sh", "-c", r.cfg.Command)
	cmd.Dir = r.cfg.Cwd
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	for k, v := range r.cfg.Env {
		cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", k, v))
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		r.cfg.Send("command.failed", map[string]any{
			"command_id":   r.cfg.CommandID,
			"message":      err.Error(),
			"completed_at": now(),
		})
		return
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		r.cfg.Send("command.failed", map[string]any{
			"command_id":   r.cfg.CommandID,
			"message":      err.Error(),
			"completed_at": now(),
		})
		return
	}

	if r.cfg.Stdin {
		r.stdin, _ = cmd.StdinPipe()
	}

	if err := cmd.Start(); err != nil {
		r.cfg.Send("command.failed", map[string]any{
			"command_id":   r.cfg.CommandID,
			"message":      err.Error(),
			"completed_at": now(),
		})
		return
	}

	r.mu.Lock()
	r.cmd = cmd
	r.startedAt = time.Now()
	r.mu.Unlock()

	r.cfg.Send("command.started", map[string]any{
		"command_id": r.cfg.CommandID,
		"pid":        cmd.Process.Pid,
		"started_at": r.startedAt.UTC().Format(time.RFC3339),
	})

	var wg sync.WaitGroup
	wg.Add(2)
	go r.streamOutput(stdout, "command.stdout", &r.stdoutSeq, &wg)
	go r.streamOutput(stderr, "command.stderr", &r.stderrSeq, &wg)
	wg.Wait()

	completedAt := now()
	if err := cmd.Wait(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			r.cfg.Send("command.exit", map[string]any{
				"command_id":   r.cfg.CommandID,
				"exit_code":    exitErr.ExitCode(),
				"completed_at": completedAt,
			})
		} else {
			r.cfg.Send("command.failed", map[string]any{
				"command_id":   r.cfg.CommandID,
				"message":      err.Error(),
				"completed_at": completedAt,
			})
		}
	} else {
		r.cfg.Send("command.exit", map[string]any{
			"command_id":   r.cfg.CommandID,
			"exit_code":    0,
			"completed_at": completedAt,
		})
	}

	close(r.done)
}

func (r *Runner) streamOutput(reader io.Reader, msgType string, seq *atomic.Int64, wg *sync.WaitGroup) {
	defer wg.Done()
	buf := make([]byte, 4096)
	for {
		n, err := reader.Read(buf)
		if n > 0 {
			r.cfg.Send(msgType, map[string]any{
				"command_id": r.cfg.CommandID,
				"chunk":      string(buf[:n]),
				"encoding":   "utf-8",
				"stream_seq": seq.Add(1),
			})
		}
		if err != nil {
			break
		}
	}
}

func (r *Runner) Cancel(gracePeriod time.Duration) {
	r.mu.Lock()
	cmd := r.cmd
	r.mu.Unlock()

	if cmd == nil || cmd.Process == nil {
		return
	}

	log.Printf("[runner] cancelling command %s with grace=%v", r.cfg.CommandID, gracePeriod)
	syscall.Kill(-cmd.Process.Pid, syscall.SIGTERM)

	select {
	case <-r.done:
	case <-time.After(gracePeriod):
		r.Kill()
	}
}

func (r *Runner) Kill() {
	r.mu.Lock()
	cmd := r.cmd
	r.mu.Unlock()

	if cmd == nil || cmd.Process == nil {
		return
	}

	log.Printf("[runner] killing command %s", r.cfg.CommandID)
	syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	r.cfg.Send("command.killed", map[string]any{
		"command_id":   r.cfg.CommandID,
		"signal":       "KILL",
		"completed_at": now(),
	})
}

func (r *Runner) WriteStdin(data string) {
	r.mu.Lock()
	stdin := r.stdin
	r.mu.Unlock()

	if stdin == nil {
		return
	}
	_, err := fmt.Fprint(stdin, data)
	if err != nil {
		log.Printf("[runner] stdin write error: %v", err)
		return
	}
	r.cfg.Send("command.stdin.accepted", map[string]any{
		"command_id": r.cfg.CommandID,
		"bytes":      len(data),
	})
}

func now() string {
	return time.Now().UTC().Format(time.RFC3339)
}
