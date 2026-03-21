package agent

import (
	"strings"
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

	req := ContainerRequest{
		Image:     "nginx:latest",
		Name:      "test-nginx",
		Model:     "TheBloke/test.gguf",
		Ports:     map[string]string{"80": "8080"},
		Env:       map[string]string{"TEST": "value"},
		GPUIDs:    "all",
		ExtraArgs: "--rm",
		Volumes:   map[string]string{"/host": "/container"},
	}

	if req.Image != "nginx:latest" {
		t.Errorf("expected image 'nginx:latest', got %s", req.Image)
	}
	if req.Name != "test-nginx" {
		t.Errorf("expected name 'test-nginx', got %s", req.Name)
	}
	if req.Model != "TheBloke/test.gguf" {
		t.Errorf("expected model 'TheBloke/test.gguf', got %s", req.Model)
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

func TestDefaultArgsRespectUserOverrides(t *testing.T) {
	tests := []struct {
		name  string
		got   string
		wants []string
	}{
		{name: "vllm model equals form", got: withVLLMModelArg("--model=custom/repo", "default/repo"), wants: []string{"--model=custom/repo"}},
		{name: "llama model equals form", got: withLlamaModelArg("--model=/tmp/model.gguf", "foo/bar.gguf"), wants: []string{"--model=/tmp/model.gguf"}},
		{name: "host equals form", got: withHostArg("--host=127.0.0.1", "--host", "0.0.0.0"), wants: []string{"--host=127.0.0.1"}},
		{name: "tool parser equals form", got: withVLLMToolCallArgs("--tool-call-parser=hermes", "meta-llama/Llama-3.1-8B-Instruct"), wants: []string{"--enable-auto-tool-choice", "--tool-call-parser=hermes"}},
	}

	for _, tt := range tests {
		for _, want := range tt.wants {
			if !strings.Contains(tt.got, want) {
				t.Fatalf("%s: expected %q in %q", tt.name, want, tt.got)
			}
		}
	}
}

// Integration tests that require Docker - skip in short mode
func TestListContainersIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping Docker integration test in short mode")
	}

	t.Parallel()

	containers, err := listContainers()
	if err != nil {
		t.Logf("listContainers failed (expected if Docker not available): %v", err)
		return
	}

	if containers == nil {
		t.Log("containers is nil (expected if Docker unavailable or no yokai containers)")
	}

	t.Logf("Found %d containers", len(containers))

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

	err := pullImage("hello-world:latest")
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

	err := stopContainer(testContainerID)
	if err == nil {
		t.Log("stopContainer on non-existent container unexpectedly succeeded")
	}

	err = restartContainer(testContainerID)
	if err == nil {
		t.Log("restartContainer on non-existent container unexpectedly succeeded")
	}

	err = removeContainer(testContainerID)
	t.Logf("removeContainer on non-existent container: %v", err)
}
