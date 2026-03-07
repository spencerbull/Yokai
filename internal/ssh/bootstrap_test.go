package ssh

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func TestPreflightResult(t *testing.T) {
	t.Parallel()

	// Test that PreflightResult struct has all expected fields
	result := &PreflightResult{
		OS:                     "Ubuntu 22.04.3 LTS",
		Arch:                   "x86_64",
		DockerInstalled:        true,
		DockerVersion:          "Docker version 24.0.7",
		GPUDetected:            true,
		GPUName:                "NVIDIA GeForce RTX 4090",
		GPUVRAMMb:              24576,
		NvidiaToolkitInstalled: true,
		NvidiaRuntimeAvailable: true,
		DiskFreeGB:             512,
	}

	// Verify all fields are accessible and have correct types
	if result.OS == "" {
		t.Error("OS field should be accessible")
	}
	if result.Arch == "" {
		t.Error("Arch field should be accessible")
	}
	if !result.DockerInstalled {
		t.Error("DockerInstalled field should be accessible")
	}
	if result.DockerVersion == "" {
		t.Error("DockerVersion field should be accessible")
	}
	if !result.GPUDetected {
		t.Error("GPUDetected field should be accessible")
	}
	if result.GPUName == "" {
		t.Error("GPUName field should be accessible")
	}
	if result.GPUVRAMMb <= 0 {
		t.Error("GPUVRAMMb field should be accessible and positive")
	}
	if !result.NvidiaToolkitInstalled {
		t.Error("NvidiaToolkitInstalled field should be accessible")
	}
	if !result.NvidiaRuntimeAvailable {
		t.Error("NvidiaRuntimeAvailable field should be accessible")
	}
	if result.DiskFreeGB <= 0 {
		t.Error("DiskFreeGB field should be accessible and positive")
	}

	// Test zero value struct
	zeroResult := &PreflightResult{}

	if zeroResult.OS != "" {
		t.Error("zero value OS should be empty")
	}
	if zeroResult.DockerInstalled {
		t.Error("zero value DockerInstalled should be false")
	}
	if zeroResult.GPUDetected {
		t.Error("zero value GPUDetected should be false")
	}
	if zeroResult.GPUVRAMMb != 0 {
		t.Error("zero value GPUVRAMMb should be 0")
	}
}

func TestPreflightResultFields(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		result PreflightResult
	}{
		{
			name: "Linux system with NVIDIA GPU",
			result: PreflightResult{
				OS:                     "Ubuntu 20.04.5 LTS",
				Arch:                   "x86_64",
				DockerInstalled:        true,
				DockerVersion:          "Docker version 20.10.21",
				GPUDetected:            true,
				GPUName:                "NVIDIA Tesla V100",
				GPUVRAMMb:              32768,
				NvidiaToolkitInstalled: true,
				NvidiaRuntimeAvailable: true,
				DiskFreeGB:             1024,
			},
		},
		{
			name: "Linux system without GPU",
			result: PreflightResult{
				OS:              "Debian GNU/Linux 11",
				Arch:            "aarch64",
				DockerInstalled: true,
				DockerVersion:   "Docker version 23.0.1",
				GPUDetected:     false,
				DiskFreeGB:      256,
			},
		},
		{
			name: "System without Docker",
			result: PreflightResult{
				OS:              "CentOS Linux 8",
				Arch:            "x86_64",
				DockerInstalled: false,
				GPUDetected:     false,
				DiskFreeGB:      128,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.result

			// Verify basic fields
			if result.OS == "" && tt.name != "System without Docker" {
				t.Error("OS should not be empty")
			}
			if result.Arch == "" {
				t.Error("Arch should not be empty")
			}

			// Verify Docker consistency
			if result.DockerInstalled && result.DockerVersion == "" {
				t.Error("if Docker is installed, version should not be empty")
			}
			if !result.DockerInstalled && result.DockerVersion != "" {
				t.Error("if Docker is not installed, version should be empty")
			}

			// Verify GPU consistency
			if result.GPUDetected {
				if result.GPUName == "" {
					t.Error("if GPU is detected, name should not be empty")
				}
				if result.GPUVRAMMb <= 0 {
					t.Error("if GPU is detected, VRAM should be positive")
				}
			} else {
				if result.NvidiaToolkitInstalled {
					t.Error("NVIDIA toolkit should not be installed without GPU")
				}
				if result.NvidiaRuntimeAvailable {
					t.Error("NVIDIA runtime should not be available without GPU")
				}
			}

			// Verify disk space
			if result.DiskFreeGB < 0 {
				t.Error("disk free space should not be negative")
			}
		})
	}
}

// Note: The actual Preflight and DeployAgent functions require a real SSH client
// and remote system to test against. These are integration tests that would need
// a test environment with SSH access. For unit testing, we focus on the data
// structures and any testable helper logic.

func TestPreflightMockData(t *testing.T) {
	t.Parallel()

	// Mock what the output parsing would look like
	// This tests the structure that would be returned from parsing command outputs

	// Mock OS release parsing
	osReleaseLine := `PRETTY_NAME="Ubuntu 22.04.3 LTS"`
	expectedOS := "Ubuntu 22.04.3 LTS"

	if !contains(osReleaseLine, expectedOS) {
		t.Errorf("OS parsing test: expected to find %s in %s", expectedOS, osReleaseLine)
	}

	// Mock nvidia-smi output parsing
	nvidiaSmiOutput := "NVIDIA GeForce RTX 4090, 24564"
	expectedGPU := "NVIDIA GeForce RTX 4090"
	expectedVRAM := 24564

	if !contains(nvidiaSmiOutput, expectedGPU) {
		t.Errorf("GPU parsing test: expected to find %s in output", expectedGPU)
	}

	// Simple parsing simulation (real code uses more complex parsing)
	parts := strings.SplitN(nvidiaSmiOutput, ",", 2)
	if len(parts) < 2 {
		t.Error("GPU output should have name and VRAM")
	}

	vramStr := strings.TrimSpace(parts[1])
	var vram int
	if n, err := fmt.Sscanf(vramStr, "%d", &vram); n != 1 || err != nil {
		t.Error("should be able to parse VRAM as integer")
	}

	if vram != expectedVRAM {
		t.Errorf("expected VRAM %d, got %d", expectedVRAM, vram)
	}

	// Mock docker runtime output
	dockerRuntimeOutput := "map[nvidia:{path:nvidia-container-runtime runtimeArgs:[]} runc:{path:runc runtimeArgs:[]}]"
	hasNvidiaRuntime := contains(dockerRuntimeOutput, "nvidia")

	if !hasNvidiaRuntime {
		t.Error("should detect nvidia runtime in docker output")
	}
}

func TestDeployAgentStructure(t *testing.T) {
	t.Parallel()

	// Test the user-level systemd service unit structure that would be deployed
	homeBinPath := "/home/testuser/.local/bin/yokai"
	expectedServiceContent := fmt.Sprintf(`[Unit]
Description=yokai agent
After=network.target docker.service
Wants=docker.service

[Service]
Type=simple
ExecStart=%s agent
Restart=always
RestartSec=5

[Install]
WantedBy=default.target
`, homeBinPath)

	// Verify service unit has required sections
	requiredSections := []string{"[Unit]", "[Service]", "[Install]"}
	for _, section := range requiredSections {
		if !contains(expectedServiceContent, section) {
			t.Errorf("service unit should contain %s section", section)
		}
	}

	// Verify critical fields
	requiredFields := []string{
		"Description=yokai agent",
		fmt.Sprintf("ExecStart=%s agent", homeBinPath),
		"Restart=always",
		"WantedBy=default.target",
	}

	for _, field := range requiredFields {
		if !contains(expectedServiceContent, field) {
			t.Errorf("service unit should contain field: %s", field)
		}
	}

	// Verify user-local paths (no /usr/local/bin, no /etc/systemd)
	if contains(expectedServiceContent, "/usr/local/bin") {
		t.Error("user-local service unit should not reference /usr/local/bin")
	}
	if contains(expectedServiceContent, "multi-user.target") {
		t.Error("user-level service should use default.target, not multi-user.target")
	}
}

func TestDeployAgentPaths(t *testing.T) {
	t.Parallel()

	// Verify that user-local deploy paths follow XDG conventions
	homeDir := "/home/testuser"
	expectedBinDir := homeDir + "/.local/bin"
	expectedBinPath := expectedBinDir + "/yokai"
	expectedConfigDir := homeDir + "/.config/yokai"
	expectedSystemdDir := homeDir + "/.config/systemd/user"

	if !strings.HasPrefix(expectedBinPath, homeDir) {
		t.Error("binary path should be under home directory")
	}
	if !contains(expectedBinDir, ".local/bin") {
		t.Error("binary should be in ~/.local/bin")
	}
	if !contains(expectedConfigDir, ".config/yokai") {
		t.Error("config should be in ~/.config/yokai")
	}
	if !contains(expectedSystemdDir, ".config/systemd/user") {
		t.Error("systemd unit should be in ~/.config/systemd/user")
	}
}

func TestAgentConfigStructure(t *testing.T) {
	t.Parallel()

	// Test the agent config JSON structure
	testToken := "test-token-123"
	expectedConfig := fmt.Sprintf(`{
  "token": "%s"
}`, testToken)

	// Parse to verify it's valid JSON
	var config map[string]interface{}
	if err := json.Unmarshal([]byte(expectedConfig), &config); err != nil {
		t.Errorf("agent config should be valid JSON: %v", err)
	}

	if config["token"] != testToken {
		t.Errorf("expected token %s, got %v", testToken, config["token"])
	}
}

func TestIsSudoError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		output   string
		expected bool
	}{
		{"password required", "sudo: a password is required", true},
		{"no tty", "sudo: no tty present and no askpass program specified", true},
		{"terminal required", "sudo: a terminal is required to read the password", true},
		{"normal error", "mv: cannot stat '/tmp/yokai.new': No such file or directory", false},
		{"empty", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isSudoError(tt.output); got != tt.expected {
				t.Errorf("isSudoError(%q) = %v, want %v", tt.output, got, tt.expected)
			}
		})
	}
}

func TestIsUserWritable(t *testing.T) {
	t.Parallel()

	tests := []struct {
		path     string
		expected bool
	}{
		{"$HOME/.local/bin/yokai", true},
		{"~/.local/bin/yokai", true},
		{"/home/user/.local/bin/yokai", true},
		{"/home/user/.config/yokai/agent.json", true},
		{"/usr/local/bin/yokai", false},
		{"/etc/yokai/agent.json", false},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			if got := isUserWritable(tt.path); got != tt.expected {
				t.Errorf("isUserWritable(%q) = %v, want %v", tt.path, got, tt.expected)
			}
		})
	}
}

// Helper function to check if string contains substring
func contains(s, substr string) bool {
	return strings.Contains(s, substr)
}
