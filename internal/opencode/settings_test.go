package opencode

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDetectConfigPath(t *testing.T) {
	t.Parallel()

	path, err := DetectConfigPath()
	if err != nil {
		t.Fatalf("DetectConfigPath failed: %v", err)
	}

	if path == "" {
		t.Error("path should not be empty")
	}

	// Should end with .opencode.json
	if !strings.HasSuffix(path, ".opencode.json") {
		t.Errorf("path should end with .opencode.json, got: %s", path)
	}
}

func TestDetectConfigPathWithXDG(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tempDir)

	// Create the XDG config file
	xdgDir := filepath.Join(tempDir, "opencode")
	if err := os.MkdirAll(xdgDir, 0755); err != nil {
		t.Fatal(err)
	}
	xdgPath := filepath.Join(xdgDir, ".opencode.json")
	if err := os.WriteFile(xdgPath, []byte("{}"), 0644); err != nil {
		t.Fatal(err)
	}

	path, err := DetectConfigPath()
	if err != nil {
		t.Fatalf("DetectConfigPath with XDG failed: %v", err)
	}

	if path != xdgPath {
		t.Errorf("expected path %s, got %s", xdgPath, path)
	}
}

func TestAddEndpointsToNewFile(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, ".opencode.json")

	endpoints := []Endpoint{
		{
			BaseURL:   "http://192.168.1.100:8000/v1",
			ModelID:   "Meta-Llama-3.1-70B-Instruct",
			ModelName: "Llama 3.1 70B (yokai)",
		},
	}

	if err := AddEndpointsToFile(configPath, endpoints); err != nil {
		t.Fatalf("AddEndpointsToFile failed: %v", err)
	}

	// Verify file was created
	cfg, err := readConfig(configPath)
	if err != nil {
		t.Fatalf("failed to read config: %v", err)
	}

	// Verify provider was added
	providers := cfg["providers"].(map[string]interface{})
	yokai, ok := providers[ProviderKey].(map[string]interface{})
	if !ok {
		t.Fatal("yokai provider should exist")
	}
	if yokai["disabled"] != false {
		t.Error("yokai provider should not be disabled")
	}

	// Verify agent model was set
	agents := cfg["agents"].(map[string]interface{})
	coder := agents["coder"].(map[string]interface{})
	model := coder["model"].(string)
	if model != "local.Meta-Llama-3.1-70B-Instruct" {
		t.Errorf("expected model 'local.Meta-Llama-3.1-70B-Instruct', got '%s'", model)
	}

	// Verify yokai_endpoints tracking
	yokaiEndpoints := cfg["yokai_endpoints"].([]interface{})
	if len(yokaiEndpoints) != 1 {
		t.Errorf("expected 1 yokai endpoint, got %d", len(yokaiEndpoints))
	}

	ep := yokaiEndpoints[0].(map[string]interface{})
	if ep["id"] != "Meta-Llama-3.1-70B-Instruct" {
		t.Errorf("endpoint ID mismatch: %v", ep["id"])
	}
	if ep["base_url"] != "http://192.168.1.100:8000/v1" {
		t.Errorf("endpoint base_url mismatch: %v", ep["base_url"])
	}
}

func TestAddEndpointsToExistingFile(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, ".opencode.json")

	// Create existing config
	existingCfg := map[string]interface{}{
		"debug": true,
		"providers": map[string]interface{}{
			"anthropic": map[string]interface{}{
				"apiKey":   "sk-existing",
				"disabled": false,
			},
		},
		"agents": map[string]interface{}{
			"coder": map[string]interface{}{
				"model":     "claude-3.7-sonnet",
				"maxTokens": 5000,
			},
		},
	}
	if err := writeTestConfig(configPath, existingCfg); err != nil {
		t.Fatal(err)
	}

	endpoints := []Endpoint{
		{
			BaseURL:   "http://192.168.1.100:8000/v1",
			ModelID:   "Meta-Llama-3.1-70B-Instruct",
			ModelName: "Llama 3.1 70B (yokai)",
		},
		{
			BaseURL:   "http://192.168.1.101:8000/v1",
			ModelID:   "Qwen2.5-Coder-32B",
			ModelName: "Qwen 2.5 Coder (yokai)",
		},
	}

	if err := AddEndpointsToFile(configPath, endpoints); err != nil {
		t.Fatalf("AddEndpointsToFile failed: %v", err)
	}

	// Verify backup
	backupPath := configPath + ".yokai.bak"
	if _, err := os.Stat(backupPath); os.IsNotExist(err) {
		t.Error("backup file should have been created")
	}

	cfg, err := readConfig(configPath)
	if err != nil {
		t.Fatal(err)
	}

	// Verify existing settings preserved
	if cfg["debug"] != true {
		t.Error("existing debug setting should be preserved")
	}

	// Verify existing provider preserved
	providers := cfg["providers"].(map[string]interface{})
	if _, ok := providers["anthropic"]; !ok {
		t.Error("existing anthropic provider should be preserved")
	}

	// Verify yokai provider added
	if _, ok := providers[ProviderKey]; !ok {
		t.Error("yokai provider should be added")
	}

	// Verify existing agent maxTokens preserved
	agents := cfg["agents"].(map[string]interface{})
	coder := agents["coder"].(map[string]interface{})
	if coder["maxTokens"].(float64) != 5000 {
		t.Error("existing maxTokens should be preserved")
	}

	// Verify model was updated to first yokai endpoint
	if coder["model"] != "local.Meta-Llama-3.1-70B-Instruct" {
		t.Errorf("model should be updated, got: %v", coder["model"])
	}

	// Verify both endpoints tracked
	yokaiEndpoints := cfg["yokai_endpoints"].([]interface{})
	if len(yokaiEndpoints) != 2 {
		t.Errorf("expected 2 yokai endpoints, got %d", len(yokaiEndpoints))
	}
}

func TestAddEndpointsNoEndpoints(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, ".opencode.json")

	err := AddEndpointsToFile(configPath, nil)
	if err == nil {
		t.Error("should return error for empty endpoints")
	}
}

func TestRemoveEndpoints(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, ".opencode.json")

	// Create config with yokai + other settings
	cfg := map[string]interface{}{
		"debug": true,
		"providers": map[string]interface{}{
			"anthropic": map[string]interface{}{
				"apiKey": "sk-existing",
			},
			ProviderKey: map[string]interface{}{
				"apiKey":   "none",
				"disabled": false,
			},
		},
		"agents": map[string]interface{}{
			"coder": map[string]interface{}{
				"model":     "local.Meta-Llama-3.1-70B-Instruct",
				"maxTokens": 5000,
			},
		},
		"yokai_endpoints": []interface{}{
			map[string]interface{}{
				"id":       "Meta-Llama-3.1-70B-Instruct",
				"name":     "Llama 3.1 70B (yokai)",
				"base_url": "http://192.168.1.100:8000/v1",
			},
		},
	}

	if err := writeTestConfig(configPath, cfg); err != nil {
		t.Fatal(err)
	}

	if err := RemoveEndpoints(configPath); err != nil {
		t.Fatalf("RemoveEndpoints failed: %v", err)
	}

	updated, err := readConfig(configPath)
	if err != nil {
		t.Fatal(err)
	}

	// Verify non-yokai settings preserved
	if updated["debug"] != true {
		t.Error("debug setting should be preserved")
	}

	// Verify anthropic provider preserved
	providers := updated["providers"].(map[string]interface{})
	if _, ok := providers["anthropic"]; !ok {
		t.Error("anthropic provider should be preserved")
	}

	// Verify yokai provider removed
	if _, ok := providers[ProviderKey]; ok {
		t.Error("yokai provider should be removed")
	}

	// Verify local model reference removed
	agents := updated["agents"].(map[string]interface{})
	coder := agents["coder"].(map[string]interface{})
	if _, ok := coder["model"]; ok {
		t.Error("local model reference should be removed")
	}

	// Verify maxTokens preserved
	if coder["maxTokens"].(float64) != 5000 {
		t.Error("maxTokens should be preserved")
	}

	// Verify yokai_endpoints removed
	if _, ok := updated["yokai_endpoints"]; ok {
		t.Error("yokai_endpoints should be removed")
	}
}

func TestBackupCreation(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, ".opencode.json")

	originalCfg := map[string]interface{}{"debug": true}
	if err := writeTestConfig(configPath, originalCfg); err != nil {
		t.Fatal(err)
	}

	originalBytes, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}

	endpoints := []Endpoint{
		{
			BaseURL:   "http://localhost:8000/v1",
			ModelID:   "test-model",
			ModelName: "Test (yokai)",
		},
	}

	if err := AddEndpointsToFile(configPath, endpoints); err != nil {
		t.Fatal(err)
	}

	backupPath := configPath + ".yokai.bak"
	backupData, err := os.ReadFile(backupPath)
	if err != nil {
		t.Fatalf("failed to read backup: %v", err)
	}

	if string(backupData) != string(originalBytes) {
		t.Error("backup should contain original content")
	}
}

func TestHasYokaiEndpoints(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()

	// Test with yokai provider
	withYokai := filepath.Join(tempDir, "with-yokai.json")
	if err := writeTestConfig(withYokai, map[string]interface{}{
		"providers": map[string]interface{}{
			ProviderKey: map[string]interface{}{"apiKey": "none"},
		},
	}); err != nil {
		t.Fatal(err)
	}

	if !HasYokaiEndpoints(withYokai) {
		t.Error("should detect yokai endpoints")
	}

	// Test without yokai provider
	withoutYokai := filepath.Join(tempDir, "without-yokai.json")
	if err := writeTestConfig(withoutYokai, map[string]interface{}{
		"providers": map[string]interface{}{
			"anthropic": map[string]interface{}{"apiKey": "sk-test"},
		},
	}); err != nil {
		t.Fatal(err)
	}

	if HasYokaiEndpoints(withoutYokai) {
		t.Error("should not detect yokai endpoints")
	}

	// Test with missing file
	if HasYokaiEndpoints(filepath.Join(tempDir, "nonexistent.json")) {
		t.Error("should not detect yokai endpoints for missing file")
	}
}

// Helper functions

func writeTestConfig(path string, cfg map[string]interface{}) error {
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

func readConfig(path string) (map[string]interface{}, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cfg map[string]interface{}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}
