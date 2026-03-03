package openclaw

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	// ProviderKey is the provider key prefix used in openclaw.json for yokai endpoints.
	ProviderKey = "yokai"
)

// Endpoint represents an OpenAI-compatible model endpoint for OpenClaw.
type Endpoint struct {
	BaseURL       string // e.g. "http://192.168.1.100:8000/v1"
	ModelID       string // e.g. "Meta-Llama-3.1-70B-Instruct"
	ModelName     string // e.g. "Llama 3.1 70B (yokai)"
	ContextWindow int    // e.g. 128000
	MaxTokens     int    // e.g. 8192
}

// DetectConfigPath returns the OpenClaw config file path (~/.openclaw/openclaw.json).
func DetectConfigPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("cannot determine home directory: %w", err)
	}

	return filepath.Join(home, ".openclaw", "openclaw.json"), nil
}

// AddEndpoints reads the OpenClaw config and adds yokai model provider(s).
// Creates a backup before modifying.
func AddEndpoints(endpoints []Endpoint) error {
	path, err := DetectConfigPath()
	if err != nil {
		return err
	}

	return AddEndpointsToFile(path, endpoints)
}

// AddEndpointsToFile adds yokai endpoints to a specific config file path.
func AddEndpointsToFile(path string, endpoints []Endpoint) error {
	// Read existing config
	var cfg map[string]interface{}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			cfg = make(map[string]interface{})
		} else {
			return fmt.Errorf("reading config: %w", err)
		}
	} else {
		if err := json.Unmarshal(data, &cfg); err != nil {
			return fmt.Errorf("parsing config: %w", err)
		}
	}

	// Backup
	if len(data) > 0 {
		backupPath := path + ".yokai.bak"
		if err := os.WriteFile(backupPath, data, 0644); err != nil {
			return fmt.Errorf("creating backup: %w", err)
		}
	}

	if len(endpoints) == 0 {
		return fmt.Errorf("no endpoints to add")
	}

	// Group endpoints by base URL to create per-host providers
	byHost := groupByHost(endpoints)

	// Get or create models section
	models, _ := cfg["models"].(map[string]interface{})
	if models == nil {
		models = make(map[string]interface{})
	}

	// Set mode to "merge" so yokai providers coexist with built-in ones
	models["mode"] = "merge"

	// Get or create providers section
	providers, _ := models["providers"].(map[string]interface{})
	if providers == nil {
		providers = make(map[string]interface{})
	}

	// Remove any existing yokai providers first
	for key := range providers {
		if key == ProviderKey || strings.HasPrefix(key, ProviderKey+"-") {
			delete(providers, key)
		}
	}

	// Add new yokai providers
	for providerID, eps := range byHost {
		var modelEntries []interface{}
		for _, ep := range eps {
			entry := map[string]interface{}{
				"id":        ep.ModelID,
				"name":      ep.ModelName,
				"reasoning": false,
				"input":     []string{"text"},
				"cost": map[string]interface{}{
					"input":      0,
					"output":     0,
					"cacheRead":  0,
					"cacheWrite": 0,
				},
			}

			contextWindow := ep.ContextWindow
			if contextWindow == 0 {
				contextWindow = 128000 // sensible default
			}
			entry["contextWindow"] = contextWindow

			maxTokens := ep.MaxTokens
			if maxTokens == 0 {
				maxTokens = 8192 // sensible default
			}
			entry["maxTokens"] = maxTokens

			modelEntries = append(modelEntries, entry)
		}

		providers[providerID] = map[string]interface{}{
			"baseUrl": eps[0].BaseURL,
			"apiKey":  "dummy",
			"api":     "openai-completions",
			"models":  modelEntries,
		}
	}

	models["providers"] = providers
	cfg["models"] = models

	// Write back
	return writeConfig(path, cfg)
}

// RemoveEndpoints removes all yokai-added providers from the OpenClaw config.
func RemoveEndpoints(configPath string) error {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return err
	}

	var cfg map[string]interface{}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return err
	}

	if models, ok := cfg["models"].(map[string]interface{}); ok {
		if providers, ok := models["providers"].(map[string]interface{}); ok {
			for key := range providers {
				if key == ProviderKey || strings.HasPrefix(key, ProviderKey+"-") {
					delete(providers, key)
				}
			}
			models["providers"] = providers
		}
		cfg["models"] = models
	}

	return writeConfig(configPath, cfg)
}

// HasYokaiEndpoints checks if the config file has yokai providers configured.
func HasYokaiEndpoints(configPath string) bool {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return false
	}

	var cfg map[string]interface{}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return false
	}

	if models, ok := cfg["models"].(map[string]interface{}); ok {
		if providers, ok := models["providers"].(map[string]interface{}); ok {
			for key := range providers {
				if key == ProviderKey || strings.HasPrefix(key, ProviderKey+"-") {
					return true
				}
			}
		}
	}

	return false
}

// groupByHost groups endpoints by base URL and returns a map of provider ID to endpoints.
// If all endpoints share the same base URL, they go under "yokai".
// Otherwise, each unique base URL gets "yokai-<n>".
func groupByHost(endpoints []Endpoint) map[string][]Endpoint {
	hostMap := make(map[string][]Endpoint)
	for _, ep := range endpoints {
		hostMap[ep.BaseURL] = append(hostMap[ep.BaseURL], ep)
	}

	if len(hostMap) == 1 {
		// All endpoints share the same base URL
		result := make(map[string][]Endpoint)
		for _, eps := range hostMap {
			result[ProviderKey] = eps
		}
		return result
	}

	// Multiple base URLs — create separate providers
	result := make(map[string][]Endpoint)
	i := 0
	for _, eps := range hostMap {
		id := ProviderKey
		if i > 0 {
			id = fmt.Sprintf("%s-%d", ProviderKey, i)
		}
		result[id] = eps
		i++
	}
	return result
}

func writeConfig(path string, cfg map[string]interface{}) error {
	out, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling config: %w", err)
	}
	out = append(out, '\n')

	// Ensure directory exists
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("creating config dir: %w", err)
	}

	tmpPath := path + ".tmp"
	if err := os.WriteFile(tmpPath, out, 0644); err != nil {
		return fmt.Errorf("writing config: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("renaming config: %w", err)
	}

	return nil
}
