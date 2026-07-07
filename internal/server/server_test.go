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

func TestWorkItemAPI(t *testing.T) {
	srv := New(Config{APIToken: "api-secret"}, store.NewMemoryStore())
	repo, err := srv.store.CreateRepository(context.Background(), store.RepositoryCreate{Name: "shed", CloneURL: "https://github.com/brianmichel/shed.git"})
	if err != nil {
		t.Fatal(err)
	}

	createReq := httptest.NewRequest(http.MethodPost, "/v1/work-items", strings.NewReader(`{"title":"Fix bug","source_type":"api","actor":"tester","repository_id":"`+repo.ID+`","repository_ref":"feature/ref","repository_base_branch":"main","metadata":{"repo":"shed"}}`))
	createReq.Header.Set("Authorization", "Bearer api-secret")
	create := httptest.NewRecorder()
	srv.ServeHTTP(create, createReq)
	if create.Code != http.StatusCreated {
		t.Fatalf("create status=%d body=%s", create.Code, create.Body.String())
	}
	var created struct {
		Data model.WorkItem `json:"data"`
	}
	if err := json.NewDecoder(create.Body).Decode(&created); err != nil {
		t.Fatal(err)
	}
	if created.Data.ID == "" || created.Data.State != model.WorkItemQueued || created.Data.RepositoryID != repo.ID || created.Data.RepositoryRef != "feature/ref" {
		t.Fatalf("created work item=%#v", created.Data)
	}

	missingRepoReq := httptest.NewRequest(http.MethodPost, "/v1/work-items", strings.NewReader(`{"title":"Missing repo","repository_id":"repo_missing"}`))
	missingRepoReq.Header.Set("Authorization", "Bearer api-secret")
	missingRepo := httptest.NewRecorder()
	srv.ServeHTTP(missingRepo, missingRepoReq)
	if missingRepo.Code != http.StatusNotFound {
		t.Fatalf("missing repo status=%d body=%s", missingRepo.Code, missingRepo.Body.String())
	}

	getReq := httptest.NewRequest(http.MethodGet, "/v1/work-items/"+created.Data.ID, nil)
	getReq.Header.Set("Authorization", "Bearer api-secret")
	getReq.SetPathValue("work_item_id", created.Data.ID)
	get := httptest.NewRecorder()
	srv.ServeHTTP(get, getReq)
	if get.Code != http.StatusOK {
		t.Fatalf("get status=%d body=%s", get.Code, get.Body.String())
	}

	listReq := httptest.NewRequest(http.MethodGet, "/v1/work-items?state=queued&limit=1", nil)
	listReq.Header.Set("Authorization", "Bearer api-secret")
	list := httptest.NewRecorder()
	srv.ServeHTTP(list, listReq)
	if list.Code != http.StatusOK {
		t.Fatalf("list status=%d body=%s", list.Code, list.Body.String())
	}
	var listed struct {
		Data []model.WorkItem `json:"data"`
	}
	if err := json.NewDecoder(list.Body).Decode(&listed); err != nil {
		t.Fatal(err)
	}
	if len(listed.Data) != 1 || listed.Data[0].ID != created.Data.ID {
		t.Fatalf("listed work items=%#v", listed.Data)
	}

	cancelReq := httptest.NewRequest(http.MethodPost, "/v1/work-items/"+created.Data.ID+"/cancel", nil)
	cancelReq.Header.Set("Authorization", "Bearer api-secret")
	cancelReq.SetPathValue("work_item_id", created.Data.ID)
	cancel := httptest.NewRecorder()
	srv.ServeHTTP(cancel, cancelReq)
	if cancel.Code != http.StatusOK {
		t.Fatalf("cancel status=%d body=%s", cancel.Code, cancel.Body.String())
	}
	var cancelled struct {
		Data model.WorkItem `json:"data"`
	}
	if err := json.NewDecoder(cancel.Body).Decode(&cancelled); err != nil {
		t.Fatal(err)
	}
	if cancelled.Data.State != model.WorkItemCancelled {
		t.Fatalf("cancelled state=%s", cancelled.Data.State)
	}

	eventsReq := httptest.NewRequest(http.MethodGet, "/v1/work-items/"+created.Data.ID+"/events", nil)
	eventsReq.Header.Set("Authorization", "Bearer api-secret")
	eventsReq.SetPathValue("work_item_id", created.Data.ID)
	events := httptest.NewRecorder()
	srv.ServeHTTP(events, eventsReq)
	if events.Code != http.StatusOK {
		t.Fatalf("events status=%d body=%s", events.Code, events.Body.String())
	}
	var replay struct {
		Data []model.Event `json:"data"`
	}
	if err := json.NewDecoder(events.Body).Decode(&replay); err != nil {
		t.Fatal(err)
	}
	if len(replay.Data) != 2 || replay.Data[0].Type != "work_item.created" || replay.Data[1].Type != "work_item.cancelled" {
		t.Fatalf("work item events=%#v", replay.Data)
	}
}

func TestAgentRunAPI(t *testing.T) {
	srv := New(Config{APIToken: "api-secret"}, store.NewMemoryStore())
	item, err := srv.store.CreateWorkItem(context.Background(), store.WorkItemCreate{Title: "Fix bug"})
	if err != nil {
		t.Fatal(err)
	}

	createReq := httptest.NewRequest(http.MethodPost, "/v1/work-items/"+item.ID+"/runs", strings.NewReader(`{"harness":"shell","model":"local","prompt":"fix it","actor":"tester"}`))
	createReq.Header.Set("Authorization", "Bearer api-secret")
	createReq.SetPathValue("work_item_id", item.ID)
	create := httptest.NewRecorder()
	srv.ServeHTTP(create, createReq)
	if create.Code != http.StatusCreated {
		t.Fatalf("create status=%d body=%s", create.Code, create.Body.String())
	}
	var created struct {
		Data model.AgentRun `json:"data"`
	}
	if err := json.NewDecoder(create.Body).Decode(&created); err != nil {
		t.Fatal(err)
	}
	if created.Data.ID == "" || created.Data.WorkItemID != item.ID || created.Data.State != model.AgentRunQueued {
		t.Fatalf("created agent run=%#v", created.Data)
	}

	getReq := httptest.NewRequest(http.MethodGet, "/v1/agent-runs/"+created.Data.ID, nil)
	getReq.Header.Set("Authorization", "Bearer api-secret")
	getReq.SetPathValue("agent_run_id", created.Data.ID)
	get := httptest.NewRecorder()
	srv.ServeHTTP(get, getReq)
	if get.Code != http.StatusOK {
		t.Fatalf("get status=%d body=%s", get.Code, get.Body.String())
	}

	listReq := httptest.NewRequest(http.MethodGet, "/v1/work-items/"+item.ID+"/runs?state=queued", nil)
	listReq.Header.Set("Authorization", "Bearer api-secret")
	listReq.SetPathValue("work_item_id", item.ID)
	list := httptest.NewRecorder()
	srv.ServeHTTP(list, listReq)
	if list.Code != http.StatusOK {
		t.Fatalf("list status=%d body=%s", list.Code, list.Body.String())
	}
	var listed struct {
		Data []model.AgentRun `json:"data"`
	}
	if err := json.NewDecoder(list.Body).Decode(&listed); err != nil {
		t.Fatal(err)
	}
	if len(listed.Data) != 1 || listed.Data[0].ID != created.Data.ID {
		t.Fatalf("listed agent runs=%#v", listed.Data)
	}

	cancelReq := httptest.NewRequest(http.MethodPost, "/v1/agent-runs/"+created.Data.ID+"/cancel", nil)
	cancelReq.Header.Set("Authorization", "Bearer api-secret")
	cancelReq.SetPathValue("agent_run_id", created.Data.ID)
	cancel := httptest.NewRecorder()
	srv.ServeHTTP(cancel, cancelReq)
	if cancel.Code != http.StatusOK {
		t.Fatalf("cancel status=%d body=%s", cancel.Code, cancel.Body.String())
	}
	var cancelled struct {
		Data model.AgentRun `json:"data"`
	}
	if err := json.NewDecoder(cancel.Body).Decode(&cancelled); err != nil {
		t.Fatal(err)
	}
	if cancelled.Data.State != model.AgentRunCancelled || cancelled.Data.CompletedAt == nil {
		t.Fatalf("cancelled agent run=%#v", cancelled.Data)
	}

	eventsReq := httptest.NewRequest(http.MethodGet, "/v1/agent-runs/"+created.Data.ID+"/events?after=7", nil)
	eventsReq.Header.Set("Authorization", "Bearer api-secret")
	eventsReq.SetPathValue("agent_run_id", created.Data.ID)
	events := httptest.NewRecorder()
	srv.ServeHTTP(events, eventsReq)
	if events.Code != http.StatusOK {
		t.Fatalf("events status=%d body=%s", events.Code, events.Body.String())
	}
	var replay struct {
		Data       []model.Event `json:"data"`
		NextCursor int64         `json:"next_cursor"`
	}
	if err := json.NewDecoder(events.Body).Decode(&replay); err != nil {
		t.Fatal(err)
	}
	if len(replay.Data) != 0 || replay.NextCursor != 7 {
		t.Fatalf("replay=%#v, want empty at cursor 7", replay)
	}

	allEventsReq := httptest.NewRequest(http.MethodGet, "/v1/agent-runs/"+created.Data.ID+"/events", nil)
	allEventsReq.Header.Set("Authorization", "Bearer api-secret")
	allEventsReq.SetPathValue("agent_run_id", created.Data.ID)
	allEvents := httptest.NewRecorder()
	srv.ServeHTTP(allEvents, allEventsReq)
	if allEvents.Code != http.StatusOK {
		t.Fatalf("all events status=%d body=%s", allEvents.Code, allEvents.Body.String())
	}
	var allReplay struct {
		Data []model.Event `json:"data"`
	}
	if err := json.NewDecoder(allEvents.Body).Decode(&allReplay); err != nil {
		t.Fatal(err)
	}
	if len(allReplay.Data) != 2 || allReplay.Data[0].Type != "agent_run.created" || allReplay.Data[1].Type != "agent_run.cancelled" {
		t.Fatalf("agent run events=%#v", allReplay.Data)
	}
}

func TestRepositoryAPI(t *testing.T) {
	srv := New(Config{APIToken: "api-secret"}, store.NewMemoryStore())

	invalidReq := httptest.NewRequest(http.MethodPost, "/v1/repositories", strings.NewReader(`{"name":"shed"}`))
	invalidReq.Header.Set("Authorization", "Bearer api-secret")
	invalid := httptest.NewRecorder()
	srv.ServeHTTP(invalid, invalidReq)
	if invalid.Code != http.StatusUnprocessableEntity {
		t.Fatalf("invalid status=%d body=%s", invalid.Code, invalid.Body.String())
	}

	createReq := httptest.NewRequest(http.MethodPost, "/v1/repositories", strings.NewReader(`{"name":"shed","provider":"github","clone_url":"https://github.com/brianmichel/shed.git","default_branch":"main","credential_ref":"secret/github","metadata":{"owner":"brianmichel"}}`))
	createReq.Header.Set("Authorization", "Bearer api-secret")
	create := httptest.NewRecorder()
	srv.ServeHTTP(create, createReq)
	if create.Code != http.StatusCreated {
		t.Fatalf("create status=%d body=%s", create.Code, create.Body.String())
	}
	var created struct {
		Data model.Repository `json:"data"`
	}
	if err := json.NewDecoder(create.Body).Decode(&created); err != nil {
		t.Fatal(err)
	}
	if created.Data.ID == "" || created.Data.Provider != "github" || created.Data.CloneURL == "" {
		t.Fatalf("created repository=%#v", created.Data)
	}

	getReq := httptest.NewRequest(http.MethodGet, "/v1/repositories/"+created.Data.ID, nil)
	getReq.Header.Set("Authorization", "Bearer api-secret")
	getReq.SetPathValue("repository_id", created.Data.ID)
	get := httptest.NewRecorder()
	srv.ServeHTTP(get, getReq)
	if get.Code != http.StatusOK {
		t.Fatalf("get status=%d body=%s", get.Code, get.Body.String())
	}

	listReq := httptest.NewRequest(http.MethodGet, "/v1/repositories?provider=github&limit=1", nil)
	listReq.Header.Set("Authorization", "Bearer api-secret")
	list := httptest.NewRecorder()
	srv.ServeHTTP(list, listReq)
	if list.Code != http.StatusOK {
		t.Fatalf("list status=%d body=%s", list.Code, list.Body.String())
	}
	var listed struct {
		Data []model.Repository `json:"data"`
	}
	if err := json.NewDecoder(list.Body).Decode(&listed); err != nil {
		t.Fatal(err)
	}
	if len(listed.Data) != 1 || listed.Data[0].ID != created.Data.ID {
		t.Fatalf("listed repositories=%#v", listed.Data)
	}
}

func TestPrepareAgentRunRepositoryDispatchesCloneCommand(t *testing.T) {
	ctx := context.Background()
	st := store.NewMemoryStore()
	mgr := compute.NewManager(compute.ManagerConfig{DefaultCompute: "exec"})
	if err := mgr.RegisterBuiltin("exec", execCompute{}); err != nil {
		t.Fatal(err)
	}
	srv := New(Config{APIToken: "api-secret", ComputeManager: mgr, DefaultCompute: "exec"}, st)
	sb, _, err := srv.CreateSandbox(ctx, store.SandboxCreate{Compute: "exec", TTL: time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	repo, err := st.CreateRepository(ctx, store.RepositoryCreate{Name: "shed", Provider: "github", CloneURL: "https://github.com/brianmichel/shed.git", DefaultBranch: "main"})
	if err != nil {
		t.Fatal(err)
	}
	item, err := st.CreateWorkItem(ctx, store.WorkItemCreate{Title: "Fix bug", RepositoryID: repo.ID, RepositoryRef: "feature/ref", RepositoryBaseBranch: "main"})
	if err != nil {
		t.Fatal(err)
	}
	run, err := st.CreateAgentRun(ctx, item.ID, store.AgentRunCreate{SandboxID: sb.ID})
	if err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodPost, "/v1/agent-runs/"+run.ID+"/prepare-repository", nil)
	req.Header.Set("Authorization", "Bearer api-secret")
	req.SetPathValue("agent_run_id", run.ID)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	var body struct {
		Data model.Command `json:"data"`
	}
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(body.Data.Command, "git clone") || !strings.Contains(body.Data.Command, "git -C '/workspace/repo' checkout 'feature/ref'") || !strings.Contains(body.Data.Command, "git -C '/workspace/repo' checkout -B 'shed/"+run.ID+"'") {
		t.Fatalf("prepare command=%q", body.Data.Command)
	}
	events, _, err := st.ListAgentRunEvents(ctx, run.ID, store.EventListOptions{})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, ev := range events {
		if ev.Type == "repo.prepare.started" {
			found = true
		}
	}
	if !found {
		t.Fatalf("repo prepare event missing: %#v", events)
	}
}

func TestDetectAgentRunDirtyStateDispatchesStatusCommand(t *testing.T) {
	ctx := context.Background()
	st := store.NewMemoryStore()
	mgr := compute.NewManager(compute.ManagerConfig{DefaultCompute: "exec"})
	if err := mgr.RegisterBuiltin("exec", execCompute{}); err != nil {
		t.Fatal(err)
	}
	srv := New(Config{APIToken: "api-secret", ComputeManager: mgr, DefaultCompute: "exec"}, st)
	sb, _, err := srv.CreateSandbox(ctx, store.SandboxCreate{Compute: "exec", TTL: time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	item, err := st.CreateWorkItem(ctx, store.WorkItemCreate{Title: "Fix bug"})
	if err != nil {
		t.Fatal(err)
	}
	run, err := st.CreateAgentRun(ctx, item.ID, store.AgentRunCreate{SandboxID: sb.ID})
	if err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodPost, "/v1/agent-runs/"+run.ID+"/detect-dirty", nil)
	req.Header.Set("Authorization", "Bearer api-secret")
	req.SetPathValue("agent_run_id", run.ID)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	var body struct {
		Data model.Command `json:"data"`
	}
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body.Data.Command != "git -C '/workspace/repo' status --porcelain=v1" {
		t.Fatalf("dirty command=%q", body.Data.Command)
	}
	events, _, err := st.ListAgentRunEvents(ctx, run.ID, store.EventListOptions{})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, ev := range events {
		if ev.Type == "repo.dirty_check.started" {
			found = true
		}
	}
	if !found {
		t.Fatalf("dirty check event missing: %#v", events)
	}
}

func TestCreateSandboxReturnsOneTimeAgentTokenAndRedactsSecrets(t *testing.T) {
	ctx := context.Background()
	st := store.NewMemoryStore()
	capture := &captureAllocateCompute{}
	mgr := compute.NewManager(compute.ManagerConfig{DefaultCompute: "exec"})
	if err := mgr.RegisterBuiltin("exec", capture); err != nil {
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
	if capture.agentToken != body.AgentToken {
		t.Fatalf("compute saw agent token %q, response token %q", capture.agentToken, body.AgentToken)
	}
	if body.ClientSession.AgentToken != "" {
		t.Fatalf("agent token leaked in client_session: %#v", body.ClientSession)
	}
	if body.Data.ComputeConfig != nil {
		t.Fatalf("compute config leaked: %#v", body.Data.ComputeConfig)
	}
	sess, err := st.FindSessionBySandbox(ctx, body.Data.ID)
	if err != nil {
		t.Fatal(err)
	}
	if sess.AgentToken != "" || sess.AgentTokenHash == "" {
		t.Fatalf("stored session should only retain hash: %#v", sess)
	}
}

func TestCreateAPITokenCanAuthenticateAPI(t *testing.T) {
	srv := New(Config{APIToken: "bootstrap"}, store.NewMemoryStore())
	createReq := httptest.NewRequest(http.MethodPost, "/v1/api-tokens", strings.NewReader(`{"name":"ci"}`))
	createReq.Header.Set("Authorization", "Bearer bootstrap")
	createW := httptest.NewRecorder()
	srv.ServeHTTP(createW, createReq)
	if createW.Code != http.StatusCreated {
		t.Fatalf("status=%d body=%s", createW.Code, createW.Body.String())
	}
	var created struct {
		Data     model.APIToken `json:"data"`
		APIToken string         `json:"api_token"`
	}
	if err := json.NewDecoder(createW.Body).Decode(&created); err != nil {
		t.Fatal(err)
	}
	if created.APIToken == "" || created.Data.TokenHash != "" || created.Data.TokenPrefix == "" {
		t.Fatalf("created token response=%#v", created)
	}

	listReq := httptest.NewRequest(http.MethodGet, "/v1/api-tokens", nil)
	listReq.Header.Set("Authorization", "Bearer "+created.APIToken)
	listW := httptest.NewRecorder()
	srv.ServeHTTP(listW, listReq)
	if listW.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", listW.Code, listW.Body.String())
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

	_, resp, err := websocket.DefaultDialer.Dial(wsURL+"&agent_token="+sess.AgentToken, nil)
	if err == nil {
		t.Fatal("expected missing header auth to fail")
	}
	if resp == nil || resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("response=%#v err=%v", resp, err)
	}

	header := http.Header{}
	header.Set("Authorization", "Bearer "+sess.AgentToken)
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
	events, _, eventErr := st.ListSandboxEvents(ctx, sb.ID, store.EventListOptions{})
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
		cmds, err = st.ListCommands(ctx, sb.ID, store.CommandListOptions{})
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
	events, _, err := st.ListCommandEvents(ctx, sb.ID, cmds[0].ID, store.EventListOptions{})
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

type captureAllocateCompute struct {
	agentToken string
}

func (c *captureAllocateCompute) Info(context.Context) (compute.PluginInfo, error) {
	return execCompute{}.Info(context.Background())
}
func (c *captureAllocateCompute) Allocate(_ context.Context, req compute.AllocateRequest) (compute.AllocateResponse, error) {
	c.agentToken = req.AgentToken
	return execCompute{}.Allocate(context.Background(), req)
}
func (c *captureAllocateCompute) Status(ctx context.Context, req compute.StatusRequest) (compute.StatusResponse, error) {
	return execCompute{}.Status(ctx, req)
}
func (c *captureAllocateCompute) Renew(ctx context.Context, req compute.RenewRequest) (compute.RenewResponse, error) {
	return execCompute{}.Renew(ctx, req)
}
func (c *captureAllocateCompute) Release(ctx context.Context, req compute.ReleaseRequest) (compute.ReleaseResponse, error) {
	return execCompute{}.Release(ctx, req)
}
func (c *captureAllocateCompute) Exec(ctx context.Context, req compute.ExecRequest, sink compute.ExecEventSink) error {
	return execCompute{}.Exec(ctx, req, sink)
}
func (c *captureAllocateCompute) Stdin(ctx context.Context, req compute.ExecStdinRequest) (compute.ExecControlResponse, error) {
	return execCompute{}.Stdin(ctx, req)
}
func (c *captureAllocateCompute) Cancel(ctx context.Context, req compute.ExecSignalRequest) (compute.ExecControlResponse, error) {
	return execCompute{}.Cancel(ctx, req)
}
func (c *captureAllocateCompute) Kill(ctx context.Context, req compute.ExecSignalRequest) (compute.ExecControlResponse, error) {
	return execCompute{}.Kill(ctx, req)
}
