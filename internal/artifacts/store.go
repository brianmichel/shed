package artifacts

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type Stored struct {
	URI         string
	ContentHash string
	SizeBytes   int64
}

type Store interface {
	Put(ctx context.Context, name string, content []byte) (Stored, error)
	Get(ctx context.Context, uri string) ([]byte, error)
}

type LocalStore struct {
	root string
}

func NewLocalStore(root string) (*LocalStore, error) {
	if strings.TrimSpace(root) == "" {
		return nil, fmt.Errorf("artifact root is required")
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(abs, 0o755); err != nil {
		return nil, err
	}
	return &LocalStore{root: abs}, nil
}

func (s *LocalStore) Put(_ context.Context, name string, content []byte) (Stored, error) {
	if s == nil || s.root == "" {
		return Stored{}, fmt.Errorf("nil artifact store")
	}
	sum := sha256.Sum256(content)
	hash := "sha256:" + hex.EncodeToString(sum[:])
	path := s.pathFor(hash, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return Stored{}, err
	}
	if err := os.WriteFile(path, content, 0o644); err != nil {
		return Stored{}, err
	}
	return Stored{URI: "file://" + path, ContentHash: hash, SizeBytes: int64(len(content))}, nil
}

func (s *LocalStore) Get(_ context.Context, uri string) ([]byte, error) {
	if s == nil || s.root == "" {
		return nil, fmt.Errorf("nil artifact store")
	}
	path, ok := strings.CutPrefix(uri, "file://")
	if !ok {
		return nil, fmt.Errorf("unsupported artifact uri")
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	rel, err := filepath.Rel(s.root, abs)
	if err != nil || rel == ".." || strings.HasPrefix(rel, "../") {
		return nil, fmt.Errorf("artifact uri escapes root")
	}
	return os.ReadFile(abs)
}

func (s *LocalStore) pathFor(hash, name string) string {
	hexHash := strings.TrimPrefix(hash, "sha256:")
	return filepath.Join(s.root, hexHash[:2], hexHash, safeName(name))
}

func safeName(name string) string {
	name = filepath.Base(strings.TrimSpace(name))
	if name == "." || name == string(filepath.Separator) || name == "" {
		return "artifact"
	}
	return name
}
