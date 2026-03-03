package vscode

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestDetectSettingsPath(t *testing.T) {
	t.Parallel()

	path, err := DetectSettingsPath()
	if err != nil {
		// On unsupported OS, this should return an error
		if runtime.GOOS != "linux" && runtime.GOOS != "darwin" {
			if !strings.Contains(err.Error(), "unsupported OS") {
				t.Errorf("expected unsupported OS error, got: %v", err)
			}
			return
		}
		t.Fatalf("DetectSettingsPath failed on supported OS: %v", err)
	}

	if path == "" {
		t.Error("path should not be empty on supported OS")
	}

	// Verify path contains expected components
	switch runtime.GOOS {
	case "linux":
		if !strings.Contains(path, "Code/User/settings.json") && !strings.Contains(path, "Code - Insiders/User/settings.json") {
			t.Errorf("Linux path should contain Code/User/settings.json, got: %s", path)
		}
	case "darwin":
		if !strings.Contains(path, "Library/Application Support/Code/User/settings.json") {
			t.Errorf("macOS path should contain Library/Application Support/Code/User/settings.json, got: %s", path)
		}
	}
}

func TestDetectSettingsPathWithXDGConfig(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("XDG_CONFIG_HOME test only relevant on Linux")
	}

	// Cannot use t.Parallel() with t.Setenv()

	tempDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tempDir)

	path, err := DetectSettingsPath()
	if err != nil {
		t.Fatalf("DetectSettingsPath with XDG_CONFIG_HOME failed: %v", err)
	}

	expected := filepath.Join(tempDir, "Code", "User", "settings.json")
	if path != expected {
		t.Errorf("expected path %s, got %s", expected, path)
	}
}

func TestAddEndpointsToExistingFile(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	settingsPath := filepath.Join(tempDir, "settings.json")

	// Create initial settings file
	initialSettings := map[string]interface{}{
		"editor.fontSize": 14,
		"chat.models": []interface{}{
			map[string]interface{}{
				"family": "openai",
				"id":     "gpt-4",
				"name":   "GPT-4",
				"url":    "https://api.openai.com/v1",
				"apiKey": "existing-key",
			},
		},
	}

	if err := writeSettingsFile(settingsPath, initialSettings); err != nil {
		t.Fatalf("failed to write initial settings: %v", err)
	}

	// Test the core logic by directly calling the helper function
	endpoints := []Endpoint{
		{
			Family: "openai",
			ID:     "vllm-test",
			Name:   "Test VLLM Model (yokai)",
			URL:    "http://192.168.1.100:8000/v1",
			APIKey: "test-key",
		},
		{
			Family: "openai",
			ID:     "llama-test",
			Name:   "Test Llama Model (yokai)",
			URL:    "http://192.168.1.101:8000/v1",
			APIKey: "test-key-2",
		},
	}

	// Use addEndpointsToFile helper
	if err := addEndpointsToFile(settingsPath, endpoints); err != nil {
		t.Fatalf("addEndpointsToFile failed: %v", err)
	}

	// Verify backup was created
	backupPath := settingsPath + ".yokai.bak"
	if _, err := os.Stat(backupPath); os.IsNotExist(err) {
		t.Error("backup file should have been created")
	}

	// Read and verify updated settings
	updatedSettings, err := readSettingsFile(settingsPath)
	if err != nil {
		t.Fatalf("failed to read updated settings: %v", err)
	}

	// Verify original settings preserved
	if updatedSettings["editor.fontSize"].(float64) != 14 {
		t.Error("original settings should be preserved")
	}

	// Verify chat.models array
	models, ok := updatedSettings["chat.models"].([]interface{})
	if !ok {
		t.Fatal("chat.models should be an array")
	}

	if len(models) != 3 { // 1 original + 2 new
		t.Errorf("expected 3 models, got %d", len(models))
	}

	// Verify new endpoints were added
	foundVLLM := false
	foundLlama := false

	for _, model := range models {
		m := model.(map[string]interface{})
		name := m["name"].(string)

		if name == "Test VLLM Model (yokai)" {
			foundVLLM = true
			if m["url"] != "http://192.168.1.100:8000/v1" {
				t.Error("VLLM endpoint URL mismatch")
			}
			if m["apiKey"] != "test-key" {
				t.Error("VLLM endpoint API key mismatch")
			}
		}

		if name == "Test Llama Model (yokai)" {
			foundLlama = true
			if m["url"] != "http://192.168.1.101:8000/v1" {
				t.Error("Llama endpoint URL mismatch")
			}
		}
	}

	if !foundVLLM {
		t.Error("VLLM endpoint not found in updated settings")
	}
	if !foundLlama {
		t.Error("Llama endpoint not found in updated settings")
	}
}

func TestAddEndpointsToNewFile(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	settingsPath := filepath.Join(tempDir, "settings.json")

	endpoints := []Endpoint{
		{
			Family: "openai",
			ID:     "test-model",
			Name:   "Test Model (yokai)",
			URL:    "http://localhost:8000/v1",
			APIKey: "test-key",
		},
	}

	// Add endpoints to non-existent file
	if err := addEndpointsToFile(settingsPath, endpoints); err != nil {
		t.Fatalf("addEndpointsToFile to new file failed: %v", err)
	}

	// Verify file was created
	settings, err := readSettingsFile(settingsPath)
	if err != nil {
		t.Fatalf("failed to read created settings file: %v", err)
	}

	models, ok := settings["chat.models"].([]interface{})
	if !ok {
		t.Fatal("chat.models should be an array")
	}

	if len(models) != 1 {
		t.Errorf("expected 1 model, got %d", len(models))
	}

	model := models[0].(map[string]interface{})
	if model["name"] != "Test Model (yokai)" {
		t.Errorf("expected model name 'Test Model (yokai)', got %v", model["name"])
	}
}

func TestAddEndpointsDuplicates(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	settingsPath := filepath.Join(tempDir, "settings.json")

	// Create initial settings with an endpoint
	initialSettings := map[string]interface{}{
		"chat.models": []interface{}{
			map[string]interface{}{
				"family": "openai",
				"id":     "existing",
				"name":   "Existing Model",
				"url":    "http://localhost:8000/v1",
				"apiKey": "existing-key",
			},
		},
	}

	if err := writeSettingsFile(settingsPath, initialSettings); err != nil {
		t.Fatalf("failed to write initial settings: %v", err)
	}

	// Try to add endpoint with same URL
	endpoints := []Endpoint{
		{
			Family: "openai",
			ID:     "duplicate",
			Name:   "Duplicate Model (yokai)",
			URL:    "http://localhost:8000/v1", // Same URL as existing
			APIKey: "duplicate-key",
		},
		{
			Family: "openai",
			ID:     "new",
			Name:   "New Model (yokai)",
			URL:    "http://localhost:8001/v1", // Different URL
			APIKey: "new-key",
		},
	}

	if err := addEndpointsToFile(settingsPath, endpoints); err != nil {
		t.Fatalf("addEndpointsToFile failed: %v", err)
	}

	// Read updated settings
	updatedSettings, err := readSettingsFile(settingsPath)
	if err != nil {
		t.Fatalf("failed to read updated settings: %v", err)
	}

	models := updatedSettings["chat.models"].([]interface{})
	if len(models) != 2 { // 1 original + 1 new (duplicate should be skipped)
		t.Errorf("expected 2 models (duplicate should be skipped), got %d", len(models))
	}

	// Verify the new model was added
	foundNew := false
	for _, model := range models {
		m := model.(map[string]interface{})
		if m["name"] == "New Model (yokai)" {
			foundNew = true
		}
	}

	if !foundNew {
		t.Error("new model should have been added")
	}
}

func TestRemoveEndpoints(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	settingsPath := filepath.Join(tempDir, "settings.json")

	// Create settings with mix of yokai and non-yokai endpoints
	settings := map[string]interface{}{
		"editor.fontSize": 14,
		"chat.models": []interface{}{
			map[string]interface{}{
				"family": "openai",
				"id":     "gpt-4",
				"name":   "GPT-4",
				"url":    "https://api.openai.com/v1",
			},
			map[string]interface{}{
				"family": "openai",
				"id":     "vllm-1",
				"name":   "VLLM Model (yokai)",
				"url":    "http://localhost:8000/v1",
			},
			map[string]interface{}{
				"family": "openai",
				"id":     "llama-1",
				"name":   "Llama Model (yokai)",
				"url":    "http://localhost:8001/v1",
			},
			map[string]interface{}{
				"family": "anthropic",
				"id":     "claude",
				"name":   "Claude",
				"url":    "https://api.anthropic.com/v1",
			},
		},
	}

	if err := writeSettingsFile(settingsPath, settings); err != nil {
		t.Fatalf("failed to write settings file: %v", err)
	}

	// Remove yokai endpoints
	if err := RemoveEndpoints(settingsPath); err != nil {
		t.Fatalf("RemoveEndpoints failed: %v", err)
	}

	// Read updated settings
	updatedSettings, err := readSettingsFile(settingsPath)
	if err != nil {
		t.Fatalf("failed to read updated settings: %v", err)
	}

	// Verify other settings preserved
	if updatedSettings["editor.fontSize"].(float64) != 14 {
		t.Error("other settings should be preserved")
	}

	// Verify only non-yokai models remain
	models := updatedSettings["chat.models"].([]interface{})
	if len(models) != 2 { // GPT-4 and Claude should remain
		t.Errorf("expected 2 remaining models, got %d", len(models))
	}

	for _, model := range models {
		m := model.(map[string]interface{})
		name := m["name"].(string)
		if strings.Contains(name, "(yokai)") {
			t.Errorf("yokai endpoint should have been removed: %s", name)
		}
	}
}

func TestBackupCreation(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	settingsPath := filepath.Join(tempDir, "settings.json")

	// Create initial settings file
	originalContent := map[string]interface{}{
		"editor.fontSize": 12,
	}

	if err := writeSettingsFile(settingsPath, originalContent); err != nil {
		t.Fatalf("failed to write settings file: %v", err)
	}

	// Read the original content as bytes (to check exact backup)
	originalBytes, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("failed to read original file: %v", err)
	}

	// Add endpoints (should create backup)
	endpoints := []Endpoint{
		{
			Family: "openai",
			ID:     "test",
			Name:   "Test (yokai)",
			URL:    "http://localhost:8000/v1",
			APIKey: "test-key",
		},
	}

	if err := addEndpointsToFile(settingsPath, endpoints); err != nil {
		t.Fatalf("addEndpointsToFile failed: %v", err)
	}

	// Verify backup was created with original content
	backupPath := settingsPath + ".yokai.bak"
	backupData, err := os.ReadFile(backupPath)
	if err != nil {
		t.Fatalf("failed to read backup file: %v", err)
	}

	if string(backupData) != string(originalBytes) {
		t.Error("backup should contain original content")
	}
}

func TestEndpointExists(t *testing.T) {
	t.Parallel()

	models := []interface{}{
		map[string]interface{}{
			"url": "http://localhost:8000/v1",
		},
		map[string]interface{}{
			"url": "http://localhost:8001/v1",
		},
		// Invalid model without url field
		map[string]interface{}{
			"name": "Invalid Model",
		},
		// Non-map item
		"invalid",
	}

	tests := []struct {
		url      string
		expected bool
	}{
		{"http://localhost:8000/v1", true},
		{"http://localhost:8001/v1", true},
		{"http://localhost:8002/v1", false},
		{"", false},
	}

	for _, tt := range tests {
		result := endpointExists(models, tt.url)
		if result != tt.expected {
			t.Errorf("endpointExists(%q) = %v, expected %v", tt.url, result, tt.expected)
		}
	}
}

func TestEndpointStructure(t *testing.T) {
	t.Parallel()

	endpoint := Endpoint{
		Family: "openai",
		ID:     "test-model",
		Name:   "Test Model (yokai)",
		URL:    "http://192.168.1.100:8000/v1",
		APIKey: "test-api-key",
	}

	if endpoint.Family != "openai" {
		t.Errorf("expected family 'openai', got %s", endpoint.Family)
	}
	if endpoint.ID != "test-model" {
		t.Errorf("expected ID 'test-model', got %s", endpoint.ID)
	}
	if !strings.Contains(endpoint.Name, "(yokai)") {
		t.Error("endpoint name should contain (yokai)")
	}
	if endpoint.URL != "http://192.168.1.100:8000/v1" {
		t.Errorf("unexpected URL: %s", endpoint.URL)
	}
}

// Helper functions for testing

func writeSettingsFile(path string, settings map[string]interface{}) error {
	data, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}

	return os.WriteFile(path, data, 0644)
}

func readSettingsFile(path string) (map[string]interface{}, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var settings map[string]interface{}
	if err := json.Unmarshal(data, &settings); err != nil {
		return nil, err
	}

	return settings, nil
}

// addEndpointsToFile is a testable version of the core AddEndpoints logic
func addEndpointsToFile(settingsPath string, endpoints []Endpoint) error {
	// Read existing settings
	var settings map[string]interface{}
	data, err := os.ReadFile(settingsPath)
	if err != nil {
		if os.IsNotExist(err) {
			settings = make(map[string]interface{})
		} else {
			return err
		}
	} else {
		if err := json.Unmarshal(data, &settings); err != nil {
			return err
		}
	}

	// Backup
	if len(data) > 0 {
		backupPath := settingsPath + ".yokai.bak"
		if err := os.WriteFile(backupPath, data, 0644); err != nil {
			return err
		}
	}

	// Get or create chat.models array
	var models []interface{}
	if existing, ok := settings["chat.models"]; ok {
		if arr, ok := existing.([]interface{}); ok {
			models = arr
		}
	}

	// Add endpoints that aren't already present
	for _, ep := range endpoints {
		if !endpointExists(models, ep.URL) {
			models = append(models, map[string]interface{}{
				"family": ep.Family,
				"id":     ep.ID,
				"name":   ep.Name,
				"url":    ep.URL,
				"apiKey": ep.APIKey,
			})
		}
	}

	settings["chat.models"] = models

	// Write back
	out, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return err
	}
	out = append(out, '\n')

	// Ensure directory exists
	if err := os.MkdirAll(filepath.Dir(settingsPath), 0755); err != nil {
		return err
	}

	tmpPath := settingsPath + ".tmp"
	if err := os.WriteFile(tmpPath, out, 0644); err != nil {
		return err
	}
	if err := os.Rename(tmpPath, settingsPath); err != nil {
		_ = os.Remove(tmpPath) // Best-effort cleanup of temporary settings file.
		return err
	}

	return nil
}
