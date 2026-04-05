package daemon

import "github.com/spencerbull/yokai/internal/config"

// DeployRequest represents a request to deploy a container to a device
type DeployRequest struct {
	DeviceID    string                `json:"device_id"`
	ServiceType string                `json:"service_type,omitempty"`
	Image       string                `json:"image"`
	Name        string                `json:"name"`
	Model       string                `json:"model"`
	Ports       map[string]string     `json:"ports"`
	Env         map[string]string     `json:"env"`
	GPUIDs      string                `json:"gpu_ids"`
	ExtraArgs   string                `json:"extra_args"`
	Volumes     map[string]string     `json:"volumes"`
	Plugins     []string              `json:"plugins"`
	Runtime     config.RuntimeOptions `json:"runtime"`
}

// DeployResult represents the result of a container deployment
type DeployResult struct {
	ContainerID string            `json:"container_id"`
	Status      string            `json:"status"`
	Ports       map[string]string `json:"ports"`
}

// ServiceTestResult represents the result of a service smoke test.
type ServiceTestResult struct {
	ServiceType string `json:"service_type"`
	Message     string `json:"message"`
	Model       string `json:"model,omitempty"`
	PromptID    string `json:"prompt_id,omitempty"`
}
