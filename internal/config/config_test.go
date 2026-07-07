package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadConfigFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "shed.json")
	content := `{
  "compute": {
    "default_driver": "nomad",
    "plugins": {
      "nomad": {"command": "/usr/local/bin/shed-compute-nomad", "args": ["-log"], "env": {"NOMAD_ADDR": "http://127.0.0.1:4646"}}
    }
  },
  "compute_classes": [
    {"name": "linux-arm64", "driver": "nomad", "driver_config": {"node_pool": "arm64"}}
  ]
}`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Compute.DefaultDriver != "nomad" {
		t.Fatalf("default_driver=%q", cfg.Compute.DefaultDriver)
	}
	ext := cfg.ExternalComputes()
	if len(ext) != 1 || ext[0].Name != "nomad" || ext[0].Command == "" || ext[0].Env["NOMAD_ADDR"] == "" {
		t.Fatalf("externals=%#v", ext)
	}
	if len(cfg.ComputeClasses) != 1 || cfg.ComputeClasses[0].Name != "linux-arm64" || cfg.ComputeClasses[0].DriverConfig["node_pool"] != "arm64" {
		t.Fatalf("classes=%#v", cfg.ComputeClasses)
	}
}
