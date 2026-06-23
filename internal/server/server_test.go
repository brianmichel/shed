package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/brianmichel/shed/internal/compute"
	"github.com/brianmichel/shed/internal/model"
	"github.com/brianmichel/shed/internal/store"
	"github.com/gorilla/websocket"
)

func TestAPIRequiresBearerToken(t *testing.T) {
	srv := New(Config{APIToken: "api-secret"}, store.NewMemoryStore())

	unauthorized := httptest.NewRecorder()
	srv.ServeHTTP(unauthorized, httptest.NewRequest(http.MethodGet, "/v1/sandboxes", nil))
	if unauthorized.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d body=%s", unauthorized.Code, unauthorized.Body.String())
	}

	health := httptest.NewRecorder()
	srv.ServeHTTP(health, httptest.NewRequest(http.MethodGet, "/v1/health", nil))
	if health.Code != http.StatusOK {
		t.Fatalf("health status=%d body=%s", health.Code, health.Body.String())
	}
}

func TestCreateSandboxReturnsOneTimeAgentTokenAndRedactsSecrets(t *testing.T) {
	ctx := context.Background()
	st := store.NewMemoryStore()
	mgr := compute.NewManager(compute.ManagerConfig{DefaultCompute: "exec"})
	if err := mgr.RegisterBuiltin("exec", execCompute{}); err != nil {
		t.Fatal(err)
	}
	srv := New(Config{APIToken: "api-secret", ComputeManager: mgr, DefaultCompute: "exec"}, st)
	req := httptest.NewRequest(http.MethodPost, "/v1/sandboxes", strings.NewReader(`{"compute_driver":"exec","compute_config":{"provider_token":"secret"}}`))
	req.Header.Set("Authorization", "Bearer api-secret")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	var body struct {
		Data          model.Sandbox       `json:"data"`
		ClientSession model.ClientSession `json:"client_session"`
		AgentToken    string              `json:"agent_token"`
	}
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body.AgentToken == "" {
		t.Fatal("agent_token missing")
	}
	if body.ClientSession.SessionKey != "" {
		t.Fatalf("session key leaked in client_session: %#v", body.ClientSession)
	}
	if body.Data.ComputeConfig != nil {
		t.Fatalf("compute config leaked: %#v", body.Data.ComputeConfig)
	}
	sess, err := st.FindSessionBySandbox(ctx, body.Data.ID)
	if err != nil {
		t.Fatal(err)
	}
	if sess.SessionKey != "" || sess.SessionKeyHash == "" {
		t.Fatalf("stored session should only retain hash: %#v", sess)
	}
}

func TestClientConnectRequiresBearerSessionToken(t *testing.T) {
	ctx := context.Background()
	st := store.NewMemoryStore()
	mgr := compute.NewManager(compute.ManagerConfig{DefaultCompute: "exec"})
	if err := mgr.RegisterBuiltin("exec", execCompute{}); err != nil {
		t.Fatal(err)
	}
	srv := New(Config{APIToken: "api-secret", ComputeManager: mgr, DefaultCompute: "exec"}, st)
	sb, sess, err := srv.CreateSandbox(ctx, store.SandboxCreate{Compute: "exec", TTL: time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	httpSrv := httptest.NewServer(srv)
	defer httpSrv.Close()
	wsURL := "ws" + strings.TrimPrefix(httpSrv.URL, "http") + "/v1/client/connect?sandbox_id=" + sb.ID

	_, resp, err := websocket.DefaultDialer.Dial(wsURL+"&session_key="+sess.SessionKey, nil)
	if err == nil {
		t.Fatal("expected missing header auth to fail")
	}
	if resp == nil || resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("response=%#v err=%v", resp, err)
	}

	header := http.Header{}
	header.Set("Authorization", "Bearer "+sess.SessionKey)
	ws, _, err := websocket.DefaultDialer.Dial(wsURL, header)
	if err != nil {
		t.Fatal(err)
	}
	_ = ws.Close()
}

func TestListComputeDrivers(t *testing.T) {
	ctx := context.Background()
	st := store.NewMemoryStore()
	root := t.TempDir()
	mgr := compute.NewManager(compute.ManagerConfig{DefaultCompute: "local"})
	if err := mgr.RegisterBuiltin("local", compute.NewLocalCompute(ctx, compute.LocalConfig{WorkspaceRoot: root})); err != nil {
		t.Fatal(err)
	}
	if err := mgr.RegisterExternal(compute.ExternalPluginConfig{Name: "cloud", Command: "/opt/shed/cloud-plugin", APIVersion: compute.APIVersionV1, Env: map[string]string{"PROVIDER_TOKEN": "secret"}}); err != nil {
		t.Fatal(err)
	}
	srv := New(Config{Addr: "127.0.0.1:0", ComputeManager: mgr, DefaultCompute: "local"}, st)
	req := httptest.NewRequest(http.MethodGet, "/v1/compute/drivers", nil)
	w := httptest.NewRecorder()
	srv.listComputeDrivers(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	var body struct {
		Data          []compute.DriverDescriptor `json:"data"`
		DefaultDriver string                     `json:"default_driver"`
	}
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body.DefaultDriver != "local" || len(body.Data) != 2 {
		t.Fatalf("body=%#v", body)
	}
	for _, driver := range body.Data {
		if driver.Name == "cloud" && (len(driver.EnvKeys) != 1 || driver.EnvKeys[0] != "PROVIDER_TOKEN") {
			t.Fatalf("expected env key redaction, got %#v", driver)
		}
	}
}

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
