package daemon

// DeployRequest represents a request to deploy a container to a device
type DeployRequest struct {
	DeviceID  string            `json:"device_id"`
	Image     string            `json:"image"`
	Name      string            `json:"name"`
	Ports     map[string]string `json:"ports"`
	Env       map[string]string `json:"env"`
	GPUIDs    string            `json:"gpu_ids"`
	ExtraArgs string            `json:"extra_args"`
	Volumes   map[string]string `json:"volumes"`
}

// DeployResult represents the result of a container deployment
type DeployResult struct {
	ContainerID string            `json:"container_id"`
	Status      string            `json:"status"`
	Ports       map[string]string `json:"ports"`
}
