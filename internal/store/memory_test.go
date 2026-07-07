package store

import (
	"context"
	"errors"
	"testing"

	"github.com/brianmichel/shed/internal/model"
)

func TestMemoryStoreJobCRUD(t *testing.T) {
	ctx := context.Background()
	st := NewMemoryStore()

	created, err := st.CreateJob(ctx, JobCreate{Repo: "https://example.com/repo.git", BaseRef: "main", Prompt: "do the thing", Model: "lfm2.5-8b-a1b"})
	if err != nil {
		t.Fatal(err)
	}
	if created.State != model.JobQueued {
		t.Fatalf("state=%s want queued", created.State)
	}
	if created.AgentDriver != "pi-rpc" || created.Provider != "lmstudio" || created.ScmDriver != "git" {
		t.Fatalf("unexpected defaults: %#v", created)
	}
	if created.Model != "lfm2.5-8b-a1b" {
		t.Fatalf("model=%q", created.Model)
	}
	if created.Trigger.Source != "manual" {
		t.Fatalf("trigger=%#v want manual default", created.Trigger)
	}

	got, err := st.GetJob(ctx, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != created.ID {
		t.Fatalf("got=%#v", got)
	}

	got.State = model.JobRunningAgent
	got.SandboxID = "sbx_1"
	updated, err := st.UpdateJob(ctx, got)
	if err != nil {
		t.Fatal(err)
	}
	if updated.State != model.JobRunningAgent || updated.SandboxID != "sbx_1" {
		t.Fatalf("updated=%#v", updated)
	}

	if _, err := st.CreateJob(ctx, JobCreate{Repo: "r2", Prompt: "p2"}); err != nil {
		t.Fatal(err)
	}
	all, err := st.ListJobs(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 2 {
		t.Fatalf("len(all)=%d want 2", len(all))
	}

	if _, err := st.GetJob(ctx, "does_not_exist"); !errors.Is(err, ErrJobNotFound) {
		t.Fatalf("err=%v want ErrJobNotFound", err)
	}
	if _, err := st.UpdateJob(ctx, model.Job{ID: "does_not_exist"}); !errors.Is(err, ErrJobNotFound) {
		t.Fatalf("err=%v want ErrJobNotFound", err)
	}
}
