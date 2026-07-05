package store

import (
	"strings"
	"testing"
)

func TestMigrationsIncludesInitialStoreSchema(t *testing.T) {
	migrations, err := Migrations()
	if err != nil {
		t.Fatalf("Migrations() error = %v", err)
	}
	if len(migrations) == 0 {
		t.Fatal("expected embedded migrations")
	}
	first := migrations[0]
	if first.Version != "001" {
		t.Fatalf("first migration version = %q, want 001", first.Version)
	}
	if first.Name != "001_initial_store.sql" {
		t.Fatalf("first migration name = %q, want 001_initial_store.sql", first.Name)
	}
	for _, want := range []string{"CREATE TABLE sandboxes", "CREATE TABLE events", "CREATE TABLE idempotency_keys"} {
		if !strings.Contains(first.SQL, want) {
			t.Fatalf("initial migration missing %q", want)
		}
	}
}

func TestMigrationsAreSorted(t *testing.T) {
	migrations, err := Migrations()
	if err != nil {
		t.Fatalf("Migrations() error = %v", err)
	}
	for i := 1; i < len(migrations); i++ {
		if migrations[i-1].Name > migrations[i].Name {
			t.Fatalf("migrations are not sorted: %q before %q", migrations[i-1].Name, migrations[i].Name)
		}
	}
}
