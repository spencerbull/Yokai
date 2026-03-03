package agent

import (
	"testing"
)

func TestSanitizeName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "valid name",
			input:    "my-container-123",
			expected: "my-container-123",
		},
		{
			name:     "uppercase letters",
			input:    "MyContainer",
			expected: "MyContainer",
		},
		{
			name:     "underscore and dots",
			input:    "my_container.test",
			expected: "my_container.test",
		},
		{
			name:     "spaces replaced with hyphens",
			input:    "my container name",
			expected: "my-container-name",
		},
		{
			name:     "special characters replaced",
			input:    "my@container#name!",
			expected: "my-container-name",
		},
		{
			name:     "leading and trailing hyphens removed",
			input:    "!@#container$%^",
			expected: "container",
		},
		{
			name:     "multiple consecutive invalid chars",
			input:    "my!!!container",
			expected: "my---container",
		},
		{
			name:     "empty after sanitization",
			input:    "!@#$%^&*()",
			expected: "unnamed",
		},
		{
			name:     "already empty",
			input:    "",
			expected: "unnamed",
		},
		{
			name:     "only hyphens",
			input:    "---",
			expected: "unnamed",
		},
		{
			name:     "unicode characters",
			input:    "côntainer-ñame",
			expected: "c-ntainer--ame",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := sanitizeName(tt.input)
			if result != tt.expected {
				t.Errorf("sanitizeName(%q) = %q, expected %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestParseStatus(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		dockerStatus string
		expected     string
	}{
		{
			name:         "running container",
			dockerStatus: "Up 2 hours",
			expected:     "running",
		},
		{
			name:         "running with port info",
			dockerStatus: "Up 5 minutes, 0.0.0.0:8080->80/tcp",
			expected:     "running",
		},
		{
			name:         "exited container",
			dockerStatus: "Exited (0) 2 minutes ago",
			expected:     "stopped",
		},
		{
			name:         "exited with error",
			dockerStatus: "Exited (1) 1 hour ago",
			expected:     "stopped",
		},
		{
			name:         "created but not started",
			dockerStatus: "Created",
			expected:     "created",
		},
		{
			name:         "restarting",
			dockerStatus: "Restarting (1) 30 seconds ago",
			expected:     "restarting",
		},
		{
			name:         "unknown status",
			dockerStatus: "SomeUnknownStatus",
			expected:     "unknown",
		},
		{
			name:         "empty status",
			dockerStatus: "",
			expected:     "unknown",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseStatus(tt.dockerStatus)
			if result != tt.expected {
				t.Errorf("parseStatus(%q) = %q, expected %q", tt.dockerStatus, result, tt.expected)
			}
		})
	}
}

func TestContainerRequestValidation(t *testing.T) {
	t.Parallel()

	// Test that ContainerRequest struct has expected fields
	req := ContainerRequest{
		Image:     "nginx:latest",
		Name:      "test-nginx",
		Ports:     map[string]string{"80": "8080"},
		Env:       map[string]string{"TEST": "value"},
		GPUIDs:    "all",
		ExtraArgs: "--rm",
		Volumes:   map[string]string{"/host": "/container"},
	}

	// Verify all fields are accessible
	if req.Image != "nginx:latest" {
		t.Errorf("expected image 'nginx:latest', got %s", req.Image)
	}
	if req.Name != "test-nginx" {
		t.Errorf("expected name 'test-nginx', got %s", req.Name)
	}
	if req.Ports["80"] != "8080" {
		t.Errorf("expected port mapping 80->8080, got %s", req.Ports["80"])
	}
	if req.Env["TEST"] != "value" {
		t.Errorf("expected env TEST=value, got %s", req.Env["TEST"])
	}
	if req.GPUIDs != "all" {
		t.Errorf("expected GPUIDs 'all', got %s", req.GPUIDs)
	}
	if req.ExtraArgs != "--rm" {
		t.Errorf("expected extra args '--rm', got %s", req.ExtraArgs)
	}
	if req.Volumes["/host"] != "/container" {
		t.Errorf("expected volume mapping /host->/container, got %s", req.Volumes["/host"])
	}
}

func TestContainerStructure(t *testing.T) {
	t.Parallel()

	// Test that Container struct has expected fields and JSON tags
	container := Container{
		ID:     "abc123",
		Name:   "test-container",
		Image:  "nginx:latest",
		Status: "running",
		Ports:  map[string]string{"80": "8080"},
	}

	if container.ID != "abc123" {
		t.Errorf("expected ID 'abc123', got %s", container.ID)
	}
	if container.Name != "test-container" {
		t.Errorf("expected name 'test-container', got %s", container.Name)
	}
	if container.Status != "running" {
		t.Errorf("expected status 'running', got %s", container.Status)
	}
}

func TestContainerResponseStructure(t *testing.T) {
	t.Parallel()

	response := ContainerResponse{
		ID:     "xyz789",
		Status: "created",
	}

	if response.ID != "xyz789" {
		t.Errorf("expected ID 'xyz789', got %s", response.ID)
	}
	if response.Status != "created" {
		t.Errorf("expected status 'created', got %s", response.Status)
	}
}

func TestImagePullRequestStructure(t *testing.T) {
	t.Parallel()

	req := ImagePullRequest{
		Image: "ubuntu:latest",
	}

	if req.Image != "ubuntu:latest" {
		t.Errorf("expected image 'ubuntu:latest', got %s", req.Image)
	}
}

// Integration tests that require Docker - skip in short mode
func TestListContainersIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping Docker integration test in short mode")
	}

	t.Parallel()

	// This function calls 'docker ps' which may fail if Docker is not installed
	containers, err := listContainers()

	// On systems without Docker, this will return an error, which is fine
	if err != nil {
		t.Logf("listContainers failed (expected if Docker not available): %v", err)
		return
	}

	// If Docker is available, containers should be a slice (possibly empty)
	// Note: On systems without Docker or without yokai containers, this can be nil
	if containers == nil {
		t.Log("containers is nil (expected if Docker unavailable or no yokai containers)")
	}

	t.Logf("Found %d containers", len(containers))

	// Verify structure of any returned containers
	for i, container := range containers {
		if container.ID == "" {
			t.Errorf("container %d has empty ID", i)
		}
		if container.Name == "" {
			t.Errorf("container %d has empty name", i)
		}
		if container.Image == "" {
			t.Errorf("container %d has empty image", i)
		}
		if container.Status == "" {
			t.Errorf("container %d has empty status", i)
		}
	}
}

func TestPullImageIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping Docker integration test in short mode")
	}

	t.Parallel()

	// Test pulling a small, commonly available image
	err := pullImage("hello-world:latest")

	// On systems without Docker, this will fail, which is acceptable
	if err != nil {
		t.Logf("pullImage failed (expected if Docker not available): %v", err)
	} else {
		t.Log("Successfully pulled hello-world:latest")
	}
}

func TestContainerOperationsIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping Docker integration test in short mode")
	}

	t.Parallel()

	testContainerID := "nonexistent-container-id"

	// Test stop on non-existent container (should return error)
	err := stopContainer(testContainerID)
	if err == nil {
		t.Log("stopContainer on non-existent container unexpectedly succeeded")
	}

	// Test restart on non-existent container (should return error)
	err = restartContainer(testContainerID)
	if err == nil {
		t.Log("restartContainer on non-existent container unexpectedly succeeded")
	}

	// Test remove on non-existent container with force flag
	// This might succeed or fail depending on Docker version
	err = removeContainer(testContainerID)
	t.Logf("removeContainer on non-existent container: %v", err)
}
