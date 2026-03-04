package opencode

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
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

	// Should end with opencode.json (no dot prefix)
	if !strings.HasSuffix(path, "opencode/opencode.json") {
		t.Errorf("path should end with opencode/opencode.json, got: %s", path)
	}
}

func TestDetectConfigPathWithXDG(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tempDir)

	path, err := DetectConfigPath()
	if err != nil {
		t.Fatalf("DetectConfigPath with XDG failed: %v", err)
	}

	expected := filepath.Join(tempDir, "opencode", "opencode.json")
	if path != expected {
		t.Errorf("expected path %s, got %s", expected, path)
	}
}

func TestAddEndpointsToNewFile(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "opencode.json")

	endpoints := []Endpoint{
		{
			BaseURL:   "http://192.168.1.100:8000/v1",
			ModelID:   "meta-llama/Llama-3.1-70B-Instruct",
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

	// Verify $schema is set
	if cfg["$schema"] != "https://opencode.ai/config.json" {
		t.Error("$schema should be set")
	}

	// Verify provider was added with correct format
	provider := cfg["provider"].(map[string]interface{})
	yokai, ok := provider[ProviderKey].(map[string]interface{})
	if !ok {
		t.Fatal("yokai provider should exist")
	}

	if yokai["npm"] != NPMPackage {
		t.Errorf("expected npm %s, got %v", NPMPackage, yokai["npm"])
	}

	if yokai["name"] != ProviderDisplayName {
		t.Errorf("expected name %s, got %v", ProviderDisplayName, yokai["name"])
	}

	// Verify options
	options := yokai["options"].(map[string]interface{})
	if options["baseURL"] != "http://192.168.1.100:8000/v1" {
		t.Errorf("unexpected baseURL: %v", options["baseURL"])
	}

	// Verify models
	models := yokai["models"].(map[string]interface{})
	model, ok := models["meta-llama/Llama-3.1-70B-Instruct"].(map[string]interface{})
	if !ok {
		t.Fatal("model should exist in models map")
	}
	if model["name"] != "Llama 3.1 70B (yokai)" {
		t.Errorf("unexpected model name: %v", model["name"])
	}
}

func TestAddEndpointsToExistingFile(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "opencode.json")

	// Create existing config with schema and theme
	existingCfg := map[string]interface{}{
		"$schema":    "https://opencode.ai/config.json",
		"theme":      "system",
		"autoupdate": false,
		"provider": map[string]interface{}{
			"anthropic": map[string]interface{}{
				"options": map[string]interface{}{
					"baseURL": "https://api.anthropic.com/v1",
				},
			},
		},
	}
	if err := writeTestConfig(configPath, existingCfg); err != nil {
		t.Fatal(err)
	}

	endpoints := []Endpoint{
		{
			BaseURL:   "http://192.168.1.100:8000/v1",
			ModelID:   "meta-llama/Llama-3.1-70B-Instruct",
			ModelName: "Llama 3.1 70B (yokai)",
		},
		{
			BaseURL:   "http://192.168.1.100:8000/v1",
			ModelID:   "Qwen/Qwen2.5-Coder-32B",
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
	if cfg["theme"] != "system" {
		t.Error("existing theme setting should be preserved")
	}
	if cfg["autoupdate"] != false {
		t.Error("existing autoupdate setting should be preserved")
	}

	// Verify existing provider preserved
	provider := cfg["provider"].(map[string]interface{})
	if _, ok := provider["anthropic"]; !ok {
		t.Error("existing anthropic provider should be preserved")
	}

	// Verify yokai provider added with both models
	yokai := provider[ProviderKey].(map[string]interface{})
	models := yokai["models"].(map[string]interface{})
	if len(models) != 2 {
		t.Errorf("expected 2 models, got %d", len(models))
	}

	if _, ok := models["meta-llama/Llama-3.1-70B-Instruct"]; !ok {
		t.Error("first model should exist")
	}
	if _, ok := models["Qwen/Qwen2.5-Coder-32B"]; !ok {
		t.Error("second model should exist")
	}
}

func TestAddEndpointsMultipleBaseURLs(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "opencode.json")

	endpoints := []Endpoint{
		{
			BaseURL:   "http://100.96.45.116:8282/v1",
			ModelID:   "Qwen/Qwen3.5-35B-A3B-FP8",
			ModelName: "Qwen 3.5 35B (yokai)",
		},
		{
			BaseURL:   "http://100.96.45.116:8383/v1",
			ModelID:   "meta-llama/Llama-3.1-8B-Instruct",
			ModelName: "Llama 3.1 8B (yokai)",
		},
	}

	if err := AddEndpointsToFile(configPath, endpoints); err != nil {
		t.Fatalf("AddEndpointsToFile failed: %v", err)
	}

	cfg, err := readConfig(configPath)
	if err != nil {
		t.Fatalf("failed to read config: %v", err)
	}

	provider := cfg["provider"].(map[string]interface{})

	// Should have two yokai providers since baseURLs differ
	yokaiCount := 0
	for key := range provider {
		if key == ProviderKey || strings.HasPrefix(key, ProviderKey+"-") {
			yokaiCount++
		}
	}
	if yokaiCount != 2 {
		t.Errorf("expected 2 yokai providers, got %d (keys: %v)", yokaiCount, keysOf(provider))
	}

	// Each provider should have exactly one model and the correct baseURL
	for key, val := range provider {
		if key != ProviderKey && !strings.HasPrefix(key, ProviderKey+"-") {
			continue
		}
		entry := val.(map[string]interface{})
		options := entry["options"].(map[string]interface{})
		baseURL := options["baseURL"].(string)
		models := entry["models"].(map[string]interface{})

		if len(models) != 1 {
			t.Errorf("provider %s: expected 1 model, got %d", key, len(models))
		}

		// Verify the model matches the baseURL
		if _, ok := models["Qwen/Qwen3.5-35B-A3B-FP8"]; ok {
			if baseURL != "http://100.96.45.116:8282/v1" {
				t.Errorf("Qwen model has wrong baseURL: %s", baseURL)
			}
		} else if _, ok := models["meta-llama/Llama-3.1-8B-Instruct"]; ok {
			if baseURL != "http://100.96.45.116:8383/v1" {
				t.Errorf("Llama model has wrong baseURL: %s", baseURL)
			}
		} else {
			t.Errorf("provider %s has unexpected models: %v", key, keysOf(models))
		}
	}
}

func TestAddEndpointsSameBaseURL(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "opencode.json")

	endpoints := []Endpoint{
		{
			BaseURL:   "http://100.96.45.116:8282/v1",
			ModelID:   "Qwen/Qwen3.5-35B-A3B-FP8",
			ModelName: "Qwen 3.5 35B (yokai)",
		},
		{
			BaseURL:   "http://100.96.45.116:8282/v1",
			ModelID:   "meta-llama/Llama-3.1-8B-Instruct",
			ModelName: "Llama 3.1 8B (yokai)",
		},
	}

	if err := AddEndpointsToFile(configPath, endpoints); err != nil {
		t.Fatalf("AddEndpointsToFile failed: %v", err)
	}

	cfg, err := readConfig(configPath)
	if err != nil {
		t.Fatalf("failed to read config: %v", err)
	}

	provider := cfg["provider"].(map[string]interface{})

	// Should have exactly one yokai provider since baseURLs are the same
	yokaiCount := 0
	for key := range provider {
		if key == ProviderKey || strings.HasPrefix(key, ProviderKey+"-") {
			yokaiCount++
		}
	}
	if yokaiCount != 1 {
		t.Errorf("expected 1 yokai provider, got %d", yokaiCount)
	}

	yokai := provider[ProviderKey].(map[string]interface{})
	models := yokai["models"].(map[string]interface{})
	if len(models) != 2 {
		t.Errorf("expected 2 models under single provider, got %d", len(models))
	}
}

func TestAddEndpointsReplacesExisting(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "opencode.json")

	// First: add two providers
	endpoints1 := []Endpoint{
		{BaseURL: "http://host1:8000/v1", ModelID: "model-a", ModelName: "A (yokai)"},
		{BaseURL: "http://host2:8000/v1", ModelID: "model-b", ModelName: "B (yokai)"},
	}
	if err := AddEndpointsToFile(configPath, endpoints1); err != nil {
		t.Fatal(err)
	}

	// Second: add a single provider — old ones should be removed
	endpoints2 := []Endpoint{
		{BaseURL: "http://host3:8000/v1", ModelID: "model-c", ModelName: "C (yokai)"},
	}
	if err := AddEndpointsToFile(configPath, endpoints2); err != nil {
		t.Fatal(err)
	}

	cfg, err := readConfig(configPath)
	if err != nil {
		t.Fatal(err)
	}

	provider := cfg["provider"].(map[string]interface{})
	yokaiCount := 0
	for key := range provider {
		if key == ProviderKey || strings.HasPrefix(key, ProviderKey+"-") {
			yokaiCount++
		}
	}
	if yokaiCount != 1 {
		t.Errorf("expected 1 yokai provider after replacement, got %d", yokaiCount)
	}

	yokai := provider[ProviderKey].(map[string]interface{})
	options := yokai["options"].(map[string]interface{})
	if options["baseURL"] != "http://host3:8000/v1" {
		t.Errorf("expected new baseURL, got %v", options["baseURL"])
	}
}

func TestAddEndpointsNoEndpoints(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "opencode.json")

	err := AddEndpointsToFile(configPath, nil)
	if err == nil {
		t.Error("should return error for empty endpoints")
	}
}

func TestRemoveEndpoints(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "opencode.json")

	// Create config with multiple yokai providers + other settings
	cfg := map[string]interface{}{
		"$schema": "https://opencode.ai/config.json",
		"theme":   "system",
		"provider": map[string]interface{}{
			"anthropic": map[string]interface{}{
				"options": map[string]interface{}{"baseURL": "https://api.anthropic.com/v1"},
			},
			ProviderKey: map[string]interface{}{
				"npm":  NPMPackage,
				"name": ProviderDisplayName,
				"options": map[string]interface{}{
					"baseURL": "http://192.168.1.100:8000/v1",
					"apiKey":  "none",
				},
				"models": map[string]interface{}{
					"meta-llama/Llama-3.1-70B-Instruct": map[string]interface{}{
						"name": "Llama 3.1 70B (yokai)",
					},
				},
			},
			ProviderKey + "-1": map[string]interface{}{
				"npm":  NPMPackage,
				"name": ProviderDisplayName,
				"options": map[string]interface{}{
					"baseURL": "http://192.168.1.100:8383/v1",
					"apiKey":  "none",
				},
				"models": map[string]interface{}{
					"Qwen/Qwen3.5-35B-A3B-FP8": map[string]interface{}{
						"name": "Qwen 3.5 35B (yokai)",
					},
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
	if updated["theme"] != "system" {
		t.Error("theme setting should be preserved")
	}

	// Verify anthropic provider preserved
	provider := updated["provider"].(map[string]interface{})
	if _, ok := provider["anthropic"]; !ok {
		t.Error("anthropic provider should be preserved")
	}

	// Verify all yokai providers removed
	for key := range provider {
		if key == ProviderKey || strings.HasPrefix(key, ProviderKey+"-") {
			t.Errorf("yokai provider %s should be removed", key)
		}
	}
}

func TestBackupCreation(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "opencode.json")

	originalCfg := map[string]interface{}{
		"$schema": "https://opencode.ai/config.json",
		"theme":   "system",
	}
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

	// Test with yokai provider (new format)
	withYokai := filepath.Join(tempDir, "with-yokai.json")
	if err := writeTestConfig(withYokai, map[string]interface{}{
		"provider": map[string]interface{}{
			ProviderKey: map[string]interface{}{
				"npm":  NPMPackage,
				"name": ProviderDisplayName,
			},
		},
	}); err != nil {
		t.Fatal(err)
	}

	if !HasYokaiEndpoints(withYokai) {
		t.Error("should detect yokai endpoints")
	}

	// Test with legacy format (providers plural)
	withLegacy := filepath.Join(tempDir, "with-legacy.json")
	if err := writeTestConfig(withLegacy, map[string]interface{}{
		"providers": map[string]interface{}{
			ProviderKey: map[string]interface{}{"apiKey": "none"},
		},
	}); err != nil {
		t.Fatal(err)
	}

	if !HasYokaiEndpoints(withLegacy) {
		t.Error("should detect yokai endpoints in legacy format")
	}

	// Test without yokai provider
	withoutYokai := filepath.Join(tempDir, "without-yokai.json")
	if err := writeTestConfig(withoutYokai, map[string]interface{}{
		"provider": map[string]interface{}{
			"anthropic": map[string]interface{}{"options": map[string]interface{}{}},
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

func TestMigrateLegacyConfig(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "opencode.json")

	// Create legacy config
	legacyCfg := map[string]interface{}{
		"providers": map[string]interface{}{
			"anthropic": map[string]interface{}{"apiKey": "sk-test"},
			ProviderKey: map[string]interface{}{
				"apiKey":   "none",
				"disabled": false,
			},
		},
		"agents": map[string]interface{}{
			"coder": map[string]interface{}{
				"model":     "local.meta-llama/Llama-3.1-70B-Instruct",
				"maxTokens": 5000,
			},
		},
		"yokai_endpoints": []interface{}{
			map[string]interface{}{
				"id":       "meta-llama/Llama-3.1-70B-Instruct",
				"base_url": "http://192.168.1.100:8000/v1",
			},
		},
	}
	if err := writeTestConfig(configPath, legacyCfg); err != nil {
		t.Fatal(err)
	}

	if err := MigrateLegacyConfig(configPath); err != nil {
		t.Fatalf("MigrateLegacyConfig failed: %v", err)
	}

	cfg, err := readConfig(configPath)
	if err != nil {
		t.Fatal(err)
	}

	// Verify yokai provider removed from legacy "providers"
	providers := cfg["providers"].(map[string]interface{})
	if _, ok := providers[ProviderKey]; ok {
		t.Error("yokai should be removed from legacy providers")
	}
	// Anthropic should be preserved
	if _, ok := providers["anthropic"]; !ok {
		t.Error("anthropic should be preserved")
	}

	// Verify local model reference removed from agents
	agents := cfg["agents"].(map[string]interface{})
	coder := agents["coder"].(map[string]interface{})
	if _, ok := coder["model"]; ok {
		t.Error("local model reference should be removed")
	}
	// maxTokens preserved
	if coder["maxTokens"].(float64) != 5000 {
		t.Error("maxTokens should be preserved")
	}

	// Verify yokai_endpoints removed
	if _, ok := cfg["yokai_endpoints"]; ok {
		t.Error("yokai_endpoints tracking should be removed")
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

func keysOf(m map[string]interface{}) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
