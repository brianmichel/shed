package compute

import "errors"

// SandboxClass is an operator-defined compute shape users may request by name.
// It describes what kind of compute is available while keeping provider-specific
// provisioning details behind the operator boundary.
type SandboxClass struct {
	Name             string         `json:"name"`
	Driver           string         `json:"driver"`
	Description      string         `json:"description,omitempty"`
	Capabilities     map[string]any `json:"capabilities,omitempty"`
	Defaults         ClassDefaults  `json:"defaults,omitempty"`
	ParametersSchema map[string]any `json:"parameters_schema,omitempty"`
	DriverConfig     map[string]any `json:"driver_config,omitempty"`
}

type ClassDefaults struct {
	TTLMillis int64 `json:"ttl_ms,omitempty"`
}

var (
	ErrComputeClassNotFound         = errors.New("compute_class_not_found")
	ErrInvalidComputeClassParameter = errors.New("invalid_compute_class_parameter")
)
