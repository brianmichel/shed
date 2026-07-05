package store

import (
	"context"
	"testing"
	"time"

	"github.com/brianmichel/shed/internal/model"
)

func TestMemoryStoreListSandboxesOptions(t *testing.T) {
	ctx := context.Background()
	st := NewMemoryStore()
	first, _, err := st.CreateSandbox(ctx, SandboxCreate{TTL: time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	second, _, err := st.CreateSandbox(ctx, SandboxCreate{TTL: time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.UpdateSandboxState(ctx, first.ID, model.SandboxReleased); err != nil {
		t.Fatal(err)
	}

	released, err := st.ListSandboxes(ctx, SandboxListOptions{State: model.SandboxReleased})
	if err != nil {
		t.Fatal(err)
	}
	if len(released) != 1 || released[0].ID != first.ID {
		t.Fatalf("released sandboxes = %#v, want only %s", released, first.ID)
	}

	paged, err := st.ListSandboxes(ctx, SandboxListOptions{Page: Page{Limit: 1}})
	if err != nil {
		t.Fatal(err)
	}
	if len(paged) != 1 || paged[0].ID != second.ID {
		t.Fatalf("paged sandboxes = %#v, want newest %s", paged, second.ID)
	}
}

func TestMemoryStoreListCommandsAndEventsOptions(t *testing.T) {
	ctx := context.Background()
	st := NewMemoryStore()
	sb, _, err := st.CreateSandbox(ctx, SandboxCreate{TTL: time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	first, err := st.CreateCommand(ctx, sb.ID, CommandCreate{Command: "one"})
	if err != nil {
		t.Fatal(err)
	}
	second, err := st.CreateCommand(ctx, sb.ID, CommandCreate{Command: "two"})
	if err != nil {
		t.Fatal(err)
	}
	second.State = model.CommandExited
	if _, err := st.UpdateCommand(ctx, second); err != nil {
		t.Fatal(err)
	}

	exited, err := st.ListCommands(ctx, sb.ID, CommandListOptions{State: model.CommandExited})
	if err != nil {
		t.Fatal(err)
	}
	if len(exited) != 1 || exited[0].ID != second.ID {
		t.Fatalf("exited commands = %#v, want only %s", exited, second.ID)
	}

	paged, err := st.ListCommands(ctx, sb.ID, CommandListOptions{Page: Page{Limit: 1, Offset: 1}})
	if err != nil {
		t.Fatal(err)
	}
	if len(paged) != 1 || paged[0].ID != second.ID {
		t.Fatalf("paged commands = %#v, want second command %s", paged, second.ID)
	}

	events, next, err := st.ListSandboxEvents(ctx, sb.ID, EventListOptions{After: 1, Limit: 1})
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].Seq <= 1 || next != events[0].Seq {
		t.Fatalf("events=%#v next=%d, want one event after seq 1", events, next)
	}

	commandEvents, _, err := st.ListCommandEvents(ctx, sb.ID, first.ID, EventListOptions{Limit: 1})
	if err != nil {
		t.Fatal(err)
	}
	if len(commandEvents) != 1 || commandEvents[0].CommandID != first.ID {
		t.Fatalf("command events = %#v, want event for %s", commandEvents, first.ID)
	}
}
