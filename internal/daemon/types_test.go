package daemon

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestDeployRequestJSON(t *testing.T) {
	t.Parallel()

	// Test JSON marshal/unmarshal roundtrip
	original := DeployRequest{
		DeviceID: "test-device-123",
		Image:    "vllm/vllm-openai:latest",
		Name:     "test-vllm-container",
		Model:    "microsoft/DialoGPT-medium",
		Ports: map[string]string{
			"8000": "8080",
			"8001": "8081",
		},
		Env: map[string]string{
			"MODEL_NAME":           "microsoft/DialoGPT-medium",
			"MAX_MODEL_LEN":        "2048",
			"TENSOR_PARALLEL_SIZE": "1",
		},
		GPUIDs:    "all",
		ExtraArgs: "--max-model-len 2048 --tensor-parallel-size 1",
		Volumes: map[string]string{
			"/host/models": "/app/models",
			"/host/cache":  "/app/cache",
		},
	}

	// Marshal to JSON
	jsonData, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("failed to marshal DeployRequest: %v", err)
	}

	// Verify JSON contains expected fields
	jsonStr := string(jsonData)
	expectedFields := []string{
		`"device_id":"test-device-123"`,
		`"image":"vllm/vllm-openai:latest"`,
		`"name":"test-vllm-container"`,
		`"model":"microsoft/DialoGPT-medium"`,
		`"gpu_ids":"all"`,
		`"extra_args":"--max-model-len 2048 --tensor-parallel-size 1"`,
	}

	for _, field := range expectedFields {
		if !contains(jsonStr, field) {
			t.Errorf("JSON should contain field: %s", field)
		}
	}

	// Unmarshal from JSON
	var unmarshaled DeployRequest
	if err := json.Unmarshal(jsonData, &unmarshaled); err != nil {
		t.Fatalf("failed to unmarshal DeployRequest: %v", err)
	}

	// Verify fields match
	if unmarshaled.DeviceID != original.DeviceID {
		t.Errorf("DeviceID mismatch: expected %s, got %s", original.DeviceID, unmarshaled.DeviceID)
	}
	if unmarshaled.Image != original.Image {
		t.Errorf("Image mismatch: expected %s, got %s", original.Image, unmarshaled.Image)
	}
	if unmarshaled.Name != original.Name {
		t.Errorf("Name mismatch: expected %s, got %s", original.Name, unmarshaled.Name)
	}
	if unmarshaled.Model != original.Model {
		t.Errorf("Model mismatch: expected %s, got %s", original.Model, unmarshaled.Model)
	}
	if unmarshaled.GPUIDs != original.GPUIDs {
		t.Errorf("GPUIDs mismatch: expected %s, got %s", original.GPUIDs, unmarshaled.GPUIDs)
	}
	if unmarshaled.ExtraArgs != original.ExtraArgs {
		t.Errorf("ExtraArgs mismatch: expected %s, got %s", original.ExtraArgs, unmarshaled.ExtraArgs)
	}

	// Verify maps
	if len(unmarshaled.Ports) != len(original.Ports) {
		t.Errorf("Ports length mismatch: expected %d, got %d", len(original.Ports), len(unmarshaled.Ports))
	}
	for k, v := range original.Ports {
		if unmarshaled.Ports[k] != v {
			t.Errorf("Ports[%s] mismatch: expected %s, got %s", k, v, unmarshaled.Ports[k])
		}
	}

	if len(unmarshaled.Env) != len(original.Env) {
		t.Errorf("Env length mismatch: expected %d, got %d", len(original.Env), len(unmarshaled.Env))
	}
	for k, v := range original.Env {
		if unmarshaled.Env[k] != v {
			t.Errorf("Env[%s] mismatch: expected %s, got %s", k, v, unmarshaled.Env[k])
		}
	}

	if len(unmarshaled.Volumes) != len(original.Volumes) {
		t.Errorf("Volumes length mismatch: expected %d, got %d", len(original.Volumes), len(unmarshaled.Volumes))
	}
	for k, v := range original.Volumes {
		if unmarshaled.Volumes[k] != v {
			t.Errorf("Volumes[%s] mismatch: expected %s, got %s", k, v, unmarshaled.Volumes[k])
		}
	}
}

func TestDeployRequestStructure(t *testing.T) {
	t.Parallel()

	// Test struct field accessibility
	req := DeployRequest{
		DeviceID:  "device-1",
		Image:     "nginx:latest",
		Name:      "test-nginx",
		Model:     "TheBloke/test.gguf",
		Ports:     map[string]string{"80": "8080"},
		Env:       map[string]string{"NGINX_HOST": "localhost"},
		GPUIDs:    "0,1",
		ExtraArgs: "--privileged",
		Volumes:   map[string]string{"/host": "/container"},
	}

	// Verify all fields are accessible
	if req.DeviceID != "device-1" {
		t.Errorf("expected DeviceID 'device-1', got %s", req.DeviceID)
	}
	if req.Image != "nginx:latest" {
		t.Errorf("expected Image 'nginx:latest', got %s", req.Image)
	}
	if req.Name != "test-nginx" {
		t.Errorf("expected Name 'test-nginx', got %s", req.Name)
	}
	if req.Model != "TheBloke/test.gguf" {
		t.Errorf("expected Model 'TheBloke/test.gguf', got %s", req.Model)
	}
	if req.GPUIDs != "0,1" {
		t.Errorf("expected GPUIDs '0,1', got %s", req.GPUIDs)
	}
	if req.ExtraArgs != "--privileged" {
		t.Errorf("expected ExtraArgs '--privileged', got %s", req.ExtraArgs)
	}

	// Test map fields
	if req.Ports["80"] != "8080" {
		t.Errorf("expected Ports['80'] = '8080', got %s", req.Ports["80"])
	}
	if req.Env["NGINX_HOST"] != "localhost" {
		t.Errorf("expected Env['NGINX_HOST'] = 'localhost', got %s", req.Env["NGINX_HOST"])
	}
	if req.Volumes["/host"] != "/container" {
		t.Errorf("expected Volumes['/host'] = '/container', got %s", req.Volumes["/host"])
	}
}

func TestDeployResultJSON(t *testing.T) {
	t.Parallel()

	// Test JSON marshal/unmarshal roundtrip
	original := DeployResult{
		ContainerID: "abc123def456",
		Status:      "running",
		Ports: map[string]string{
			"8000": "30001",
			"8001": "30002",
		},
	}

	// Marshal to JSON
	jsonData, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("failed to marshal DeployResult: %v", err)
	}

	// Verify JSON structure
	jsonStr := string(jsonData)
	expectedFields := []string{
		`"container_id":"abc123def456"`,
		`"status":"running"`,
	}

	for _, field := range expectedFields {
		if !contains(jsonStr, field) {
			t.Errorf("JSON should contain field: %s", field)
		}
	}

	// Unmarshal from JSON
	var unmarshaled DeployResult
	if err := json.Unmarshal(jsonData, &unmarshaled); err != nil {
		t.Fatalf("failed to unmarshal DeployResult: %v", err)
	}

	// Verify fields match
	if unmarshaled.ContainerID != original.ContainerID {
		t.Errorf("ContainerID mismatch: expected %s, got %s", original.ContainerID, unmarshaled.ContainerID)
	}
	if unmarshaled.Status != original.Status {
		t.Errorf("Status mismatch: expected %s, got %s", original.Status, unmarshaled.Status)
	}

	// Verify ports map
	if len(unmarshaled.Ports) != len(original.Ports) {
		t.Errorf("Ports length mismatch: expected %d, got %d", len(original.Ports), len(unmarshaled.Ports))
	}
	for k, v := range original.Ports {
		if unmarshaled.Ports[k] != v {
			t.Errorf("Ports[%s] mismatch: expected %s, got %s", k, v, unmarshaled.Ports[k])
		}
	}
}

func TestDeployResultStructure(t *testing.T) {
	t.Parallel()

	// Test struct field accessibility
	result := DeployResult{
		ContainerID: "container-xyz789",
		Status:      "created",
		Ports:       map[string]string{"3000": "8080", "3001": "8081"},
	}

	// Verify all fields are accessible
	if result.ContainerID != "container-xyz789" {
		t.Errorf("expected ContainerID 'container-xyz789', got %s", result.ContainerID)
	}
	if result.Status != "created" {
		t.Errorf("expected Status 'created', got %s", result.Status)
	}

	// Test ports map
	if result.Ports["3000"] != "8080" {
		t.Errorf("expected Ports['3000'] = '8080', got %s", result.Ports["3000"])
	}
	if result.Ports["3001"] != "8081" {
		t.Errorf("expected Ports['3001'] = '8081', got %s", result.Ports["3001"])
	}
}

func TestEmptyMapsInJSON(t *testing.T) {
	t.Parallel()

	// Test with empty/nil maps
	req := DeployRequest{
		DeviceID:  "device-1",
		Image:     "ubuntu:latest",
		Name:      "test-ubuntu",
		Model:     "",
		Ports:     nil,
		Env:       map[string]string{},
		GPUIDs:    "",
		ExtraArgs: "",
		Volumes:   nil,
	}

	// Marshal to JSON
	jsonData, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("failed to marshal DeployRequest with empty maps: %v", err)
	}

	// Unmarshal from JSON
	var unmarshaled DeployRequest
	if err := json.Unmarshal(jsonData, &unmarshaled); err != nil {
		t.Fatalf("failed to unmarshal DeployRequest with empty maps: %v", err)
	}

	// Verify basic fields
	if unmarshaled.DeviceID != "device-1" {
		t.Errorf("DeviceID mismatch with empty maps: expected device-1, got %s", unmarshaled.DeviceID)
	}

	// Verify empty/nil maps are handled correctly
	if len(unmarshaled.Ports) > 0 {
		t.Error("Ports should be empty after unmarshaling nil map")
	}
	if len(unmarshaled.Env) > 0 {
		t.Error("Env should be empty after unmarshaling empty map")
	}
	if len(unmarshaled.Volumes) > 0 {
		t.Error("Volumes should be empty after unmarshaling nil map")
	}
}

func TestJSONOmitEmpty(t *testing.T) {
	t.Parallel()

	// Test what happens with minimal DeployRequest
	minimal := DeployRequest{
		DeviceID: "device-minimal",
		Image:    "alpine:latest",
		Name:     "minimal-test",
	}

	jsonData, err := json.Marshal(minimal)
	if err != nil {
		t.Fatalf("failed to marshal minimal DeployRequest: %v", err)
	}

	jsonStr := string(jsonData)

	// Even with empty values, all fields should be present in JSON
	// since the struct doesn't use omitempty tags
	requiredFields := []string{
		`"device_id"`,
		`"image"`,
		`"name"`,
		`"model"`,
		`"ports"`,
		`"env"`,
		`"gpu_ids"`,
		`"extra_args"`,
		`"volumes"`,
	}

	for _, field := range requiredFields {
		if !contains(jsonStr, field) {
			t.Errorf("minimal JSON should contain field: %s", field)
		}
	}
}

// Helper function for substring checking
func contains(s, substr string) bool {
	return strings.Contains(s, substr)
}
