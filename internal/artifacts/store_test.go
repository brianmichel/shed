package artifacts

import (
	"context"
	"strings"
	"testing"
)

func TestLocalStorePutGet(t *testing.T) {
	st, err := NewLocalStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	stored, err := st.Put(context.Background(), "diff.patch", []byte("patch data"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(stored.URI, "file://") {
		t.Fatalf("uri=%q", stored.URI)
	}
	if !strings.HasPrefix(stored.ContentHash, "sha256:") {
		t.Fatalf("content hash=%q", stored.ContentHash)
	}
	if stored.SizeBytes != int64(len("patch data")) {
		t.Fatalf("size=%d", stored.SizeBytes)
	}
	got, err := st.Get(context.Background(), stored.URI)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "patch data" {
		t.Fatalf("content=%q", got)
	}
}

func TestLocalStoreRejectsEscapingURI(t *testing.T) {
	st, err := NewLocalStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.Get(context.Background(), "file:///etc/passwd"); err == nil {
		t.Fatal("expected escape error")
	}
}

func TestLocalStoreUsesSafeBaseName(t *testing.T) {
	st, err := NewLocalStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	stored, err := st.Put(context.Background(), "../../secret.txt", []byte("data"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(stored.URI, "..") || !strings.HasSuffix(stored.URI, "/secret.txt") {
		t.Fatalf("unsafe uri=%q", stored.URI)
	}
}
