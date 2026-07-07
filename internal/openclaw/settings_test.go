package openclaw

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

	if !strings.HasSuffix(path, filepath.Join(".openclaw", "openclaw.json")) {
		t.Errorf("path should end with .openclaw/openclaw.json, got: %s", path)
	}
}

func TestAddEndpointsToNewFile(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "openclaw.json")

	endpoints := []Endpoint{
		{
			BaseURL:       "http://192.168.1.100:8000/v1",
			ModelID:       "Meta-Llama-3.1-70B-Instruct",
			ModelName:     "Llama 3.1 70B (yokai)",
			ContextWindow: 128000,
			MaxTokens:     8192,
		},
	}

	if err := AddEndpointsToFile(configPath, endpoints); err != nil {
		t.Fatalf("AddEndpointsToFile failed: %v", err)
	}

	cfg, err := readConfig(configPath)
	if err != nil {
		t.Fatalf("failed to read config: %v", err)
	}

	// Verify models section
	models := cfg["models"].(map[string]interface{})
	if models["mode"] != "merge" {
		t.Error("mode should be 'merge'")
	}

	// Verify yokai provider
	providers := models["providers"].(map[string]interface{})
	yokai, ok := providers[ProviderKey].(map[string]interface{})
	if !ok {
		t.Fatal("yokai provider should exist")
	}

	if yokai["baseUrl"] != "http://192.168.1.100:8000/v1" {
		t.Errorf("baseUrl mismatch: %v", yokai["baseUrl"])
	}
	if yokai["api"] != "openai-completions" {
		t.Errorf("api should be 'openai-completions', got: %v", yokai["api"])
	}
	if yokai["apiKey"] != "dummy" {
		t.Errorf("apiKey should be 'dummy', got: %v", yokai["apiKey"])
	}

	// Verify model entry
	modelEntries := yokai["models"].([]interface{})
	if len(modelEntries) != 1 {
		t.Fatalf("expected 1 model, got %d", len(modelEntries))
	}

	model := modelEntries[0].(map[string]interface{})
	if model["id"] != "Meta-Llama-3.1-70B-Instruct" {
		t.Errorf("model id mismatch: %v", model["id"])
	}
	if model["name"] != "Llama 3.1 70B (yokai)" {
		t.Errorf("model name mismatch: %v", model["name"])
	}
	if model["contextWindow"].(float64) != 128000 {
		t.Errorf("contextWindow mismatch: %v", model["contextWindow"])
	}
	if model["maxTokens"].(float64) != 8192 {
		t.Errorf("maxTokens mismatch: %v", model["maxTokens"])
	}

	// Verify cost is zero
	cost := model["cost"].(map[string]interface{})
	if cost["input"].(float64) != 0 {
		t.Error("cost.input should be 0")
	}
}

func TestAddEndpointsToExistingFile(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "openclaw.json")

	// Create existing config with other settings
	existingCfg := map[string]interface{}{
		"agents": map[string]interface{}{
			"defaults": map[string]interface{}{
				"workspace": "~/.openclaw/workspace",
			},
		},
		"channels": map[string]interface{}{
			"whatsapp": map[string]interface{}{
				"allowFrom": []string{"+15555550123"},
			},
		},
		"models": map[string]interface{}{
			"mode": "merge",
			"providers": map[string]interface{}{
				"openai": map[string]interface{}{
					"apiKey": "sk-existing",
				},
			},
		},
	}

	if err := writeTestConfig(configPath, existingCfg); err != nil {
		t.Fatal(err)
	}

	endpoints := []Endpoint{
		{
			BaseURL:       "http://192.168.1.100:8000/v1",
			ModelID:       "Meta-Llama-3.1-70B-Instruct",
			ModelName:     "Llama 3.1 70B (yokai)",
			ContextWindow: 128000,
			MaxTokens:     8192,
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
	agents := cfg["agents"].(map[string]interface{})
	defaults := agents["defaults"].(map[string]interface{})
	if defaults["workspace"] != "~/.openclaw/workspace" {
		t.Error("agents.defaults.workspace should be preserved")
	}

	channels := cfg["channels"].(map[string]interface{})
	if _, ok := channels["whatsapp"]; !ok {
		t.Error("channels.whatsapp should be preserved")
	}

	// Verify existing provider preserved
	models := cfg["models"].(map[string]interface{})
	providers := models["providers"].(map[string]interface{})
	if _, ok := providers["openai"]; !ok {
		t.Error("openai provider should be preserved")
	}

	// Verify yokai provider added
	if _, ok := providers[ProviderKey]; !ok {
		t.Error("yokai provider should be added")
	}
}

func TestAddEndpointsMultipleHosts(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "openclaw.json")

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

	cfg, err := readConfig(configPath)
	if err != nil {
		t.Fatal(err)
	}

	models := cfg["models"].(map[string]interface{})
	providers := models["providers"].(map[string]interface{})

	// Should have 2 providers (different base URLs)
	yokaiCount := 0
	for key := range providers {
		if key == ProviderKey || strings.HasPrefix(key, ProviderKey+"-") {
			yokaiCount++
		}
	}

	if yokaiCount != 2 {
		t.Errorf("expected 2 yokai providers for different hosts, got %d", yokaiCount)
	}
}

func TestAddEndpointsSameHost(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "openclaw.json")

	endpoints := []Endpoint{
		{
			BaseURL:   "http://192.168.1.100:8000/v1",
			ModelID:   "Meta-Llama-3.1-70B-Instruct",
			ModelName: "Llama 3.1 70B (yokai)",
		},
		{
			BaseURL:   "http://192.168.1.100:8000/v1",
			ModelID:   "Qwen2.5-Coder-32B",
			ModelName: "Qwen 2.5 Coder (yokai)",
		},
	}

	if err := AddEndpointsToFile(configPath, endpoints); err != nil {
		t.Fatalf("AddEndpointsToFile failed: %v", err)
	}

	cfg, err := readConfig(configPath)
	if err != nil {
		t.Fatal(err)
	}

	models := cfg["models"].(map[string]interface{})
	providers := models["providers"].(map[string]interface{})

	// Should have 1 provider (same base URL) with 2 models
	yokai, ok := providers[ProviderKey].(map[string]interface{})
	if !ok {
		t.Fatal("single yokai provider should exist for same-host endpoints")
	}

	modelEntries := yokai["models"].([]interface{})
	if len(modelEntries) != 2 {
		t.Errorf("expected 2 models under single provider, got %d", len(modelEntries))
	}
}

func TestAddEndpointsNoEndpoints(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "openclaw.json")

	err := AddEndpointsToFile(configPath, nil)
	if err == nil {
		t.Error("should return error for empty endpoints")
	}
}

func TestRemoveEndpoints(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "openclaw.json")

	// Create config with yokai + other providers
	cfg := map[string]interface{}{
		"agents": map[string]interface{}{
			"defaults": map[string]interface{}{
				"workspace": "~/.openclaw/workspace",
			},
		},
		"models": map[string]interface{}{
			"mode": "merge",
			"providers": map[string]interface{}{
				"openai": map[string]interface{}{
					"apiKey": "sk-test",
				},
				ProviderKey: map[string]interface{}{
					"baseUrl": "http://192.168.1.100:8000/v1",
					"apiKey":  "dummy",
					"api":     "openai-completions",
					"models":  []interface{}{},
				},
				ProviderKey + "-1": map[string]interface{}{
					"baseUrl": "http://192.168.1.101:8000/v1",
					"apiKey":  "dummy",
					"api":     "openai-completions",
					"models":  []interface{}{},
				},
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
	agents := updated["agents"].(map[string]interface{})
	defaults := agents["defaults"].(map[string]interface{})
	if defaults["workspace"] != "~/.openclaw/workspace" {
		t.Error("workspace should be preserved")
	}

	// Verify openai provider preserved
	models := updated["models"].(map[string]interface{})
	providers := models["providers"].(map[string]interface{})
	if _, ok := providers["openai"]; !ok {
		t.Error("openai provider should be preserved")
	}

	// Verify yokai providers removed
	for key := range providers {
		if key == ProviderKey || strings.HasPrefix(key, ProviderKey+"-") {
			t.Errorf("yokai provider '%s' should be removed", key)
		}
	}
}

func TestBackupCreation(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "openclaw.json")

	originalCfg := map[string]interface{}{"agents": map[string]interface{}{}}
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
		"models": map[string]interface{}{
			"providers": map[string]interface{}{
				ProviderKey: map[string]interface{}{"baseUrl": "http://localhost:8000/v1"},
			},
		},
	}); err != nil {
		t.Fatal(err)
	}

	if !HasYokaiEndpoints(withYokai) {
		t.Error("should detect yokai endpoints")
	}

	// Test with yokai-N provider
	withYokaiN := filepath.Join(tempDir, "with-yokai-n.json")
	if err := writeTestConfig(withYokaiN, map[string]interface{}{
		"models": map[string]interface{}{
			"providers": map[string]interface{}{
				ProviderKey + "-1": map[string]interface{}{"baseUrl": "http://localhost:8000/v1"},
			},
		},
	}); err != nil {
		t.Fatal(err)
	}

	if !HasYokaiEndpoints(withYokaiN) {
		t.Error("should detect yokai-N endpoints")
	}

	// Test without yokai provider
	withoutYokai := filepath.Join(tempDir, "without-yokai.json")
	if err := writeTestConfig(withoutYokai, map[string]interface{}{
		"models": map[string]interface{}{
			"providers": map[string]interface{}{
				"openai": map[string]interface{}{"apiKey": "sk-test"},
			},
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

func TestDefaultContextWindowAndMaxTokens(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "openclaw.json")

	// Endpoints with zero values should get defaults
	endpoints := []Endpoint{
		{
			BaseURL:   "http://localhost:8000/v1",
			ModelID:   "test-model",
			ModelName: "Test (yokai)",
			// ContextWindow and MaxTokens left as 0
		},
	}

	if err := AddEndpointsToFile(configPath, endpoints); err != nil {
		t.Fatal(err)
	}

	cfg, err := readConfig(configPath)
	if err != nil {
		t.Fatal(err)
	}

	models := cfg["models"].(map[string]interface{})
	providers := models["providers"].(map[string]interface{})
	yokai := providers[ProviderKey].(map[string]interface{})
	modelEntries := yokai["models"].([]interface{})
	model := modelEntries[0].(map[string]interface{})

	if model["contextWindow"].(float64) != 128000 {
		t.Errorf("default contextWindow should be 128000, got %v", model["contextWindow"])
	}
	if model["maxTokens"].(float64) != 8192 {
		t.Errorf("default maxTokens should be 8192, got %v", model["maxTokens"])
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
