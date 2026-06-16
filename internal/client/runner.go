package client

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

type SendFunc func(msgType string, payload map[string]any)

type CommandConfig struct {
	CommandID string
	Command   string
	Cwd       string
	Env       map[string]string
	Stdin     bool
	Send      SendFunc
}

type Runner struct {
	cfg       CommandConfig
	cmd       *exec.Cmd
	stdin     io.WriteCloser
	mu        sync.Mutex
	done      chan struct{}
	stdoutSeq atomic.Int64
	stderrSeq atomic.Int64
}

func NewRunner(cfg CommandConfig) *Runner { return &Runner{cfg: cfg, done: make(chan struct{})} }

func (r *Runner) Run() {
	r.cfg.Send("command.accepted", map[string]any{"command_id": r.cfg.CommandID, "state": "starting"})
	cmd := exec.Command("/bin/sh", "-c", r.cfg.Command)
	cmd.Dir = r.cfg.Cwd
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	for k, v := range r.cfg.Env {
		cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", k, v))
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		r.fail(err)
		return
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		r.fail(err)
		return
	}
	if r.cfg.Stdin {
		r.stdin, _ = cmd.StdinPipe()
	}
	if err := cmd.Start(); err != nil {
		r.fail(err)
		return
	}
	r.mu.Lock()
	r.cmd = cmd
	r.mu.Unlock()
	started := time.Now().UTC()
	r.cfg.Send("command.started", map[string]any{"command_id": r.cfg.CommandID, "pid": cmd.Process.Pid, "started_at": started.Format(time.RFC3339)})
	var wg sync.WaitGroup
	wg.Add(2)
	go r.stream(stdout, "command.stdout", &r.stdoutSeq, &wg)
	go r.stream(stderr, "command.stderr", &r.stderrSeq, &wg)
	wg.Wait()
	completed := time.Now().UTC().Format(time.RFC3339)
	if err := cmd.Wait(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			r.cfg.Send("command.exit", map[string]any{"command_id": r.cfg.CommandID, "exit_code": exitErr.ExitCode(), "completed_at": completed})
		} else {
			r.cfg.Send("command.failed", map[string]any{"command_id": r.cfg.CommandID, "message": err.Error(), "completed_at": completed})
		}
	} else {
		r.cfg.Send("command.exit", map[string]any{"command_id": r.cfg.CommandID, "exit_code": 0, "completed_at": completed})
	}
	close(r.done)
}

func (r *Runner) stream(reader io.Reader, msgType string, seq *atomic.Int64, wg *sync.WaitGroup) {
	defer wg.Done()
	buf := make([]byte, 4096)
	for {
		n, err := reader.Read(buf)
		if n > 0 {
			r.cfg.Send(msgType, map[string]any{"command_id": r.cfg.CommandID, "chunk": string(buf[:n]), "encoding": "utf-8", "stream_seq": seq.Add(1)})
		}
		if err != nil {
			return
		}
	}
}

func (r *Runner) fail(err error) {
	r.cfg.Send("command.failed", map[string]any{"command_id": r.cfg.CommandID, "message": err.Error(), "completed_at": time.Now().UTC().Format(time.RFC3339)})
}

func (r *Runner) Cancel(grace time.Duration) {
	r.mu.Lock()
	cmd := r.cmd
	r.mu.Unlock()
	if cmd == nil || cmd.Process == nil {
		return
	}
	log.Printf("[client] cancelling command %s", r.cfg.CommandID)
	_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGTERM)
	select {
	case <-r.done:
	case <-time.After(grace):
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
	log.Printf("[client] killing command %s", r.cfg.CommandID)
	_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	r.cfg.Send("command.killed", map[string]any{"command_id": r.cfg.CommandID, "signal": "KILL", "completed_at": time.Now().UTC().Format(time.RFC3339)})
}

func (r *Runner) WriteStdin(data string) {
	r.mu.Lock()
	stdin := r.stdin
	r.mu.Unlock()
	if stdin != nil {
		_, _ = fmt.Fprint(stdin, data)
		r.cfg.Send("command.stdin.accepted", map[string]any{"command_id": r.cfg.CommandID, "bytes": len(data)})
	}
}
