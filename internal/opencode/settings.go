package opencode

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	// ProviderKey is the provider key used in opencode.json for yokai endpoints.
	ProviderKey = "yokai"
	// NPMPackage is the AI SDK package for OpenAI-compatible providers.
	NPMPackage = "@ai-sdk/openai-compatible"
	// ProviderDisplayName is the display name shown in OpenCode's UI.
	ProviderDisplayName = "Yokai"
)

// Endpoint represents an OpenAI-compatible model endpoint for OpenCode.
type Endpoint struct {
	BaseURL   string // e.g. "http://192.168.1.100:8000/v1"
	ModelID   string // e.g. "meta-llama/Llama-3.1-70B-Instruct"
	ModelName string // e.g. "Llama 3.1 70B (yokai)"
}

// DetectConfigPath finds the OpenCode opencode.json config path.
// Checks: $XDG_CONFIG_HOME/opencode/opencode.json, ~/.config/opencode/opencode.json
func DetectConfigPath() (string, error) {
	configDir := os.Getenv("XDG_CONFIG_HOME")
	if configDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("cannot determine home directory: %w", err)
		}
		configDir = filepath.Join(home, ".config")
	}

	return filepath.Join(configDir, "opencode", "opencode.json"), nil
}

// AddEndpoints reads the OpenCode config and adds a yokai provider with models.
// Uses the OpenCode custom provider format with @ai-sdk/openai-compatible.
// Creates a backup before modifying.
func AddEndpoints(endpoints []Endpoint) error {
	path, err := DetectConfigPath()
	if err != nil {
		return err
	}

	return AddEndpointsToFile(path, endpoints)
}

// AddEndpointsToFile adds yokai endpoints to a specific config file path.
// Creates separate provider entries per unique baseURL so each host:port
// gets its own provider with the correct URL.
func AddEndpointsToFile(path string, endpoints []Endpoint) error {
	// Read existing config
	var cfg map[string]interface{}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			cfg = map[string]interface{}{
				"$schema": "https://opencode.ai/config.json",
			}
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

	// Get or create provider section
	provider, _ := cfg["provider"].(map[string]interface{})
	if provider == nil {
		provider = make(map[string]interface{})
	}

	// Remove any existing yokai providers first
	for key := range provider {
		if key == ProviderKey || strings.HasPrefix(key, ProviderKey+"-") {
			delete(provider, key)
		}
	}

	// Add new yokai providers — one per unique baseURL
	for providerID, eps := range byHost {
		models := make(map[string]interface{})
		for _, ep := range eps {
			models[ep.ModelID] = map[string]interface{}{
				"name": ep.ModelName,
			}
		}

		provider[providerID] = map[string]interface{}{
			"npm":  NPMPackage,
			"name": ProviderDisplayName,
			"options": map[string]interface{}{
				"baseURL": eps[0].BaseURL,
				"apiKey":  "none",
			},
			"models": models,
		}
	}

	cfg["provider"] = provider

	// Write back
	return writeConfig(path, cfg)
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

// RemoveEndpoints removes all yokai-added configuration from the OpenCode config.
func RemoveEndpoints(configPath string) error {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return err
	}

	var cfg map[string]interface{}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return err
	}

	// Remove all yokai providers (yokai, yokai-1, yokai-2, ...) from "provider" section
	if provider, ok := cfg["provider"].(map[string]interface{}); ok {
		for key := range provider {
			if key == ProviderKey || strings.HasPrefix(key, ProviderKey+"-") {
				delete(provider, key)
			}
		}
		cfg["provider"] = provider
	}

	return writeConfig(configPath, cfg)
}

// HasYokaiEndpoints checks if the config file has a yokai provider configured.
func HasYokaiEndpoints(configPath string) bool {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return false
	}

	var cfg map[string]interface{}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return false
	}

	if provider, ok := cfg["provider"].(map[string]interface{}); ok {
		for key := range provider {
			if key == ProviderKey || strings.HasPrefix(key, ProviderKey+"-") {
				return true
			}
		}
	}

	// Also check legacy "providers" key
	if providers, ok := cfg["providers"].(map[string]interface{}); ok {
		if _, ok := providers[ProviderKey]; ok {
			return true
		}
	}

	return false
}

// MigrateLegacyConfig checks for the old .opencode.json format and removes
// legacy yokai entries (providers, agents, yokai_endpoints keys).
func MigrateLegacyConfig(configPath string) error {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil // File doesn't exist, nothing to migrate
	}

	var cfg map[string]interface{}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil
	}

	changed := false

	// Remove legacy "providers" yokai entry
	if providers, ok := cfg["providers"].(map[string]interface{}); ok {
		if _, ok := providers[ProviderKey]; ok {
			delete(providers, ProviderKey)
			if len(providers) == 0 {
				delete(cfg, "providers")
			} else {
				cfg["providers"] = providers
			}
			changed = true
		}
	}

	// Remove legacy "agents" coder model if it uses local. prefix
	if agents, ok := cfg["agents"].(map[string]interface{}); ok {
		if coder, ok := agents["coder"].(map[string]interface{}); ok {
			if model, ok := coder["model"].(string); ok {
				if strings.HasPrefix(model, "local.") {
					delete(coder, "model")
					changed = true
				}
			}
		}
	}

	// Remove legacy yokai_endpoints tracking
	if _, ok := cfg["yokai_endpoints"]; ok {
		delete(cfg, "yokai_endpoints")
		changed = true
	}

	if changed {
		return writeConfig(configPath, cfg)
	}

	return nil
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
		_ = os.Remove(tmpPath)
		return fmt.Errorf("renaming config: %w", err)
	}

	return nil
}
