package server

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/brianmichel/shed/internal/compute"
	"github.com/brianmichel/shed/internal/model"
	"github.com/brianmichel/shed/internal/store"
)

func TestCreateSandboxAllocationFailureMarksSandboxFailed(t *testing.T) {
	ctx := context.Background()
	st := store.NewMemoryStore()
	mgr := compute.NewManager(compute.ManagerConfig{DefaultCompute: "fail"})
	if err := mgr.RegisterBuiltin("fail", failingCompute{}); err != nil {
		t.Fatal(err)
	}
	srv := New(Config{Addr: "127.0.0.1:0", ComputeManager: mgr, DefaultCompute: "fail"}, st)
	sb, _, err := srv.CreateSandbox(ctx, store.SandboxCreate{Compute: "fail", TTL: time.Minute})
	if err == nil {
		t.Fatal("expected allocation error")
	}
	got, getErr := st.GetSandbox(ctx, sb.ID)
	if getErr != nil {
		t.Fatal(getErr)
	}
	if got.State != model.SandboxFailed {
		t.Fatalf("state=%s want %s", got.State, model.SandboxFailed)
	}
	events, _, eventErr := st.ListSandboxEvents(ctx, sb.ID, 0)
	if eventErr != nil {
		t.Fatal(eventErr)
	}
	found := false
	for _, ev := range events {
		if ev.Type == "compute.allocate.failed" {
			found = true
		}
	}
	if !found {
		t.Fatalf("compute failure event not found: %#v", events)
	}
}

func TestCreateCommandFallsBackToComputeExec(t *testing.T) {
	ctx := context.Background()
	st := store.NewMemoryStore()
	mgr := compute.NewManager(compute.ManagerConfig{DefaultCompute: "exec"})
	if err := mgr.RegisterBuiltin("exec", execCompute{}); err != nil {
		t.Fatal(err)
	}
	srv := New(Config{Addr: "127.0.0.1:0", ComputeManager: mgr, DefaultCompute: "exec"}, st)
	sb, _, err := srv.CreateSandbox(ctx, store.SandboxCreate{Compute: "exec", TTL: time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/v1/sandboxes/"+sb.ID+"/commands", bytes.NewBufferString(`{"command":"echo hi"}`))
	req.SetPathValue("sandbox_id", sb.ID)
	w := httptest.NewRecorder()
	srv.createCommand(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	var cmds []model.Command
	for i := 0; i < 50; i++ {
		cmds, err = st.ListCommands(ctx, sb.ID)
		if err != nil {
			t.Fatal(err)
		}
		if len(cmds) == 1 && cmds[0].State == model.CommandExited {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if len(cmds) != 1 || cmds[0].State != model.CommandExited {
		t.Fatalf("commands=%#v", cmds)
	}
	events, _, err := st.ListCommandEvents(ctx, sb.ID, cmds[0].ID, 0)
	if err != nil {
		t.Fatal(err)
	}
	var sawStdout bool
	for _, ev := range events {
		if ev.Type == "command.stdout" {
			sawStdout = true
		}
	}
	if !sawStdout {
		t.Fatalf("stdout event missing: %#v", events)
	}
}

type failingCompute struct{}

func (failingCompute) Info(context.Context) (compute.PluginInfo, error) {
	return compute.PluginInfo{Name: "fail", Version: "0.1.0", APIVersions: []string{compute.APIVersionV1}}, nil
}
func (failingCompute) Allocate(context.Context, compute.AllocateRequest) (compute.AllocateResponse, error) {
	return compute.AllocateResponse{}, errors.New("boom")
}
func (failingCompute) Status(context.Context, compute.StatusRequest) (compute.StatusResponse, error) {
	return compute.StatusResponse{State: "failed"}, nil
}
func (failingCompute) Renew(context.Context, compute.RenewRequest) (compute.RenewResponse, error) {
	return compute.RenewResponse{}, nil
}
func (failingCompute) Release(context.Context, compute.ReleaseRequest) (compute.ReleaseResponse, error) {
	return compute.ReleaseResponse{Released: true}, nil
}
func (failingCompute) Exec(context.Context, compute.ExecRequest, compute.ExecEventSink) error {
	return nil
}
func (failingCompute) Stdin(context.Context, compute.ExecStdinRequest) (compute.ExecControlResponse, error) {
	return compute.ExecControlResponse{Accepted: true}, nil
}
func (failingCompute) Cancel(context.Context, compute.ExecSignalRequest) (compute.ExecControlResponse, error) {
	return compute.ExecControlResponse{Accepted: true}, nil
}
func (failingCompute) Kill(context.Context, compute.ExecSignalRequest) (compute.ExecControlResponse, error) {
	return compute.ExecControlResponse{Accepted: true}, nil
}

type execCompute struct{}

func (execCompute) Info(context.Context) (compute.PluginInfo, error) {
	return compute.PluginInfo{Name: "exec", Version: "0.1.0", APIVersions: []string{compute.APIVersionV1}, Capabilities: map[string]bool{"exec": true}}, nil
}
func (execCompute) Allocate(context.Context, compute.AllocateRequest) (compute.AllocateResponse, error) {
	return compute.AllocateResponse{ExternalID: "exec-1", APIVersion: compute.APIVersionV1, PluginName: "exec", PluginVersion: "0.1.0"}, nil
}
func (execCompute) Status(context.Context, compute.StatusRequest) (compute.StatusResponse, error) {
	return compute.StatusResponse{State: "running"}, nil
}
func (execCompute) Renew(context.Context, compute.RenewRequest) (compute.RenewResponse, error) {
	return compute.RenewResponse{}, nil
}
func (execCompute) Release(context.Context, compute.ReleaseRequest) (compute.ReleaseResponse, error) {
	return compute.ReleaseResponse{Released: true}, nil
}
func (execCompute) Exec(_ context.Context, req compute.ExecRequest, sink compute.ExecEventSink) error {
	_ = sink(compute.ExecEvent{CommandID: req.CommandID, Type: "command.accepted", Data: map[string]any{"command_id": req.CommandID}})
	_ = sink(compute.ExecEvent{CommandID: req.CommandID, Type: "command.started", Data: map[string]any{"command_id": req.CommandID, "pid": 123}})
	_ = sink(compute.ExecEvent{CommandID: req.CommandID, Type: "command.stdout", Data: map[string]any{"command_id": req.CommandID, "chunk": "hi\n", "encoding": "utf-8"}})
	return sink(compute.ExecEvent{CommandID: req.CommandID, Type: "command.exit", Data: map[string]any{"command_id": req.CommandID, "exit_code": 0}})
}
func (execCompute) Stdin(context.Context, compute.ExecStdinRequest) (compute.ExecControlResponse, error) {
	return compute.ExecControlResponse{Accepted: true}, nil
}
func (execCompute) Cancel(context.Context, compute.ExecSignalRequest) (compute.ExecControlResponse, error) {
	return compute.ExecControlResponse{Accepted: true}, nil
}
func (execCompute) Kill(context.Context, compute.ExecSignalRequest) (compute.ExecControlResponse, error) {
	return compute.ExecControlResponse{Accepted: true}, nil
}
