package server

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/brianmichel/shed/internal/agent"
	"github.com/brianmichel/shed/internal/compute"
	"github.com/brianmichel/shed/internal/job"
	"github.com/brianmichel/shed/internal/model"
	"github.com/brianmichel/shed/internal/store"
)

// fakeAgentRunner stubs out the agent step so this test exercises the real
// server/job/sandbox integration (compute allocation, sandbox readiness,
// event plumbing) without depending on a real pi/LM Studio installation.
type fakeAgentRunner struct{}

func (fakeAgentRunner) Run(ctx context.Context, req agent.RunRequest, onCommandStarted func(commandID string)) (agent.RunResult, error) {
	if onCommandStarted != nil {
		onCommandStarted("cmd_fake_agent")
	}
	return agent.RunResult{Succeeded: true}, nil
}

func TestCreateJobWithoutManagerReturnsNotImplemented(t *testing.T) {
	st := store.NewMemoryStore()
	srv := New(Config{Addr: "127.0.0.1:0", DefaultCompute: "local"}, st)
	req := httptest.NewRequest(http.MethodPost, "/v1/jobs", bytes.NewBufferString(`{"repo":"r","prompt":"p"}`))
	w := httptest.NewRecorder()
	srv.createJob(w, req)
	if w.Code != http.StatusNotImplemented {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
}

func TestGetJobNotFound(t *testing.T) {
	st := store.NewMemoryStore()
	srv := New(Config{Addr: "127.0.0.1:0", DefaultCompute: "local"}, st)
	req := httptest.NewRequest(http.MethodGet, "/v1/jobs/job_missing", nil)
	req.SetPathValue("job_id", "job_missing")
	w := httptest.NewRecorder()
	srv.getJob(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
}

func TestJobEndToEndWithLocalCompute(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	st := store.NewMemoryStore()
	root := t.TempDir()
	mgr := compute.NewManager(compute.ManagerConfig{DefaultCompute: "local"})
	if err := mgr.RegisterBuiltin("local", compute.NewLocalCompute(ctx, compute.LocalConfig{WorkspaceRoot: root, HeartbeatEvery: time.Hour})); err != nil {
		t.Fatal(err)
	}
	srv := New(Config{Addr: "127.0.0.1:0", ComputeManager: mgr, DefaultCompute: "local"}, st)
	jobMgr := job.NewManager(job.Config{Store: st, Sandboxes: srv, Agent: fakeAgentRunner{}, PollInterval: 20 * time.Millisecond})
	srv.SetJobManager(jobMgr)

	errCh := make(chan error, 1)
	go func() { errCh <- srv.Start(ctx) }()
	for i := 0; i < 200 && srv.Addr() == "127.0.0.1:0"; i++ {
		time.Sleep(5 * time.Millisecond)
	}

	req := httptest.NewRequest(http.MethodPost, "/v1/jobs", bytes.NewBufferString(`{"repo":"https://example.com/repo.git","base_ref":"main","prompt":"do the thing","model":"stub-model"}`))
	w := httptest.NewRecorder()
	srv.createJob(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	var created struct {
		Data model.Job `json:"data"`
	}
	if err := json.NewDecoder(w.Body).Decode(&created); err != nil {
		t.Fatal(err)
	}
	if created.Data.State != model.JobQueued {
		t.Fatalf("initial state=%s want queued", created.Data.State)
	}

	var final model.Job
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		j, err := st.GetJob(ctx, created.Data.ID)
		if err != nil {
			t.Fatal(err)
		}
		if j.State == model.JobSucceeded || j.State == model.JobFailed {
			final = j
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if final.State != model.JobSucceeded {
		t.Fatalf("state=%s reason=%s", final.State, final.FailureReason)
	}
	if final.SandboxID == "" || final.AgentCommandID == "" {
		t.Fatalf("missing sandbox/agent command id: %#v", final)
	}

	eventsReq := httptest.NewRequest(http.MethodGet, "/v1/jobs/"+final.ID+"/events", nil)
	eventsReq.SetPathValue("job_id", final.ID)
	eventsW := httptest.NewRecorder()
	srv.jobEvents(eventsW, eventsReq)
	if eventsW.Code != http.StatusOK {
		t.Fatalf("events status=%d body=%s", eventsW.Code, eventsW.Body.String())
	}
	var eventsBody struct {
		Data []model.Event `json:"data"`
	}
	if err := json.NewDecoder(eventsW.Body).Decode(&eventsBody); err != nil {
		t.Fatal(err)
	}
	if len(eventsBody.Data) == 0 {
		t.Fatal("expected at least one job/sandbox event")
	}

	listReq := httptest.NewRequest(http.MethodGet, "/v1/jobs", nil)
	listW := httptest.NewRecorder()
	srv.listJobs(listW, listReq)
	if listW.Code != http.StatusOK {
		t.Fatalf("list status=%d body=%s", listW.Code, listW.Body.String())
	}
}
