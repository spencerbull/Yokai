package opencode

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	// ProviderKey is the provider key used in .opencode.json for yokai endpoints.
	ProviderKey = "yokai"
	// ModelPrefix is the prefix for local model references in OpenCode.
	ModelPrefix = "local."
)

// Endpoint represents an OpenAI-compatible model endpoint for OpenCode.
type Endpoint struct {
	BaseURL   string // e.g. "http://192.168.1.100:8000/v1"
	ModelID   string // e.g. "Meta-Llama-3.1-70B-Instruct"
	ModelName string // e.g. "Llama 3.1 70B (yokai)"
}

// DetectConfigPath finds the OpenCode .opencode.json config path.
// Checks in order: $XDG_CONFIG_HOME/opencode/.opencode.json, ~/.opencode.json
// Returns the user-level config path (does not check project-local .opencode.json).
func DetectConfigPath() (string, error) {
	// Check XDG config first
	configDir := os.Getenv("XDG_CONFIG_HOME")
	if configDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("cannot determine home directory: %w", err)
		}
		configDir = filepath.Join(home, ".config")
	}

	xdgPath := filepath.Join(configDir, "opencode", ".opencode.json")
	if _, err := os.Stat(xdgPath); err == nil {
		return xdgPath, nil
	}

	// Fallback to ~/.opencode.json
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("cannot determine home directory: %w", err)
	}

	homePath := filepath.Join(home, ".opencode.json")
	if _, err := os.Stat(homePath); err == nil {
		return homePath, nil
	}

	// Default to XDG path for creation
	return xdgPath, nil
}

// AddEndpoints reads the OpenCode config and adds yokai model endpoints.
// It sets LOCAL_ENDPOINT to the first endpoint's base URL and configures
// the coder agent model. Creates a backup before modifying.
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

	// Set up the yokai provider under "providers"
	providers, _ := cfg["providers"].(map[string]interface{})
	if providers == nil {
		providers = make(map[string]interface{})
	}

	providers[ProviderKey] = map[string]interface{}{
		"apiKey":   "none",
		"disabled": false,
	}
	cfg["providers"] = providers

	// Set the first endpoint's model as the coder agent model.
	// OpenCode uses LOCAL_ENDPOINT env var for the base URL, but we can also
	// configure the model reference directly.
	agents, _ := cfg["agents"].(map[string]interface{})
	if agents == nil {
		agents = make(map[string]interface{})
	}

	coder, _ := agents["coder"].(map[string]interface{})
	if coder == nil {
		coder = make(map[string]interface{})
	}

	// Use the first endpoint's model as the coder model
	coder["model"] = ModelPrefix + endpoints[0].ModelID
	agents["coder"] = coder
	cfg["agents"] = agents

	// Store yokai endpoint metadata as a custom section for tracking
	var yokaiModels []interface{}
	for _, ep := range endpoints {
		yokaiModels = append(yokaiModels, map[string]interface{}{
			"id":       ep.ModelID,
			"name":     ep.ModelName,
			"base_url": ep.BaseURL,
		})
	}
	cfg["yokai_endpoints"] = yokaiModels

	// Write back
	return writeConfig(path, cfg)
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

	// Remove yokai provider
	if providers, ok := cfg["providers"].(map[string]interface{}); ok {
		delete(providers, ProviderKey)
		cfg["providers"] = providers
	}

	// Reset coder model if it points to a yokai/local model
	if agents, ok := cfg["agents"].(map[string]interface{}); ok {
		if coder, ok := agents["coder"].(map[string]interface{}); ok {
			if model, ok := coder["model"].(string); ok {
				if strings.HasPrefix(model, ModelPrefix) {
					delete(coder, "model")
				}
			}
			agents["coder"] = coder
		}
		cfg["agents"] = agents
	}

	// Remove yokai endpoint tracking
	delete(cfg, "yokai_endpoints")

	return writeConfig(configPath, cfg)
}

// HasYokaiEndpoints checks if the config file has yokai endpoints configured.
func HasYokaiEndpoints(configPath string) bool {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return false
	}

	var cfg map[string]interface{}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return false
	}

	if providers, ok := cfg["providers"].(map[string]interface{}); ok {
		if _, ok := providers[ProviderKey]; ok {
			return true
		}
	}

	return false
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
