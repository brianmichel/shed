package config

import (
	"encoding/json"
	"os"

	"github.com/brianmichel/shed/internal/compute"
)

type File struct {
	Compute        ComputeConfig          `json:"compute"`
	ComputeClasses []compute.SandboxClass `json:"compute_classes"`
}

type ComputeConfig struct {
	DefaultDriver string                          `json:"default_driver"`
	Plugins       map[string]ExternalPluginConfig `json:"plugins"`
}

type ExternalPluginConfig struct {
	Command string            `json:"command"`
	Args    []string          `json:"args"`
	Env     map[string]string `json:"env"`
}

func Load(path string) (File, error) {
	if path == "" {
		return File{}, nil
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return File{}, err
	}
	var cfg File
	if err := json.Unmarshal(b, &cfg); err != nil {
		return File{}, err
	}
	return cfg, nil
}

func (f File) ExternalComputes() []compute.ExternalPluginConfig {
	out := make([]compute.ExternalPluginConfig, 0, len(f.Compute.Plugins))
	for name, plugin := range f.Compute.Plugins {
		out = append(out, compute.ExternalPluginConfig{Name: name, Command: plugin.Command, Args: plugin.Args, Env: plugin.Env})
	}
	return out
}
