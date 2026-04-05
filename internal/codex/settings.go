package codex

import (
	"fmt"
	"os"
	"path/filepath"

	toml "github.com/pelletier/go-toml/v2"
)

const ProviderKey = "yokai"

type Endpoint struct {
	BaseURL   string
	ModelID   string
	ModelName string
}

func DetectConfigPath() (string, error) {
	base := os.Getenv("CODEX_HOME")
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("cannot determine home directory: %w", err)
		}
		base = filepath.Join(home, ".codex")
	}
	return filepath.Join(base, "config.toml"), nil
}

func AddEndpoints(endpoints []Endpoint) error {
	path, err := DetectConfigPath()
	if err != nil {
		return err
	}
	return AddEndpointsToFile(path, endpoints)
}

func AddEndpointsToFile(path string, endpoints []Endpoint) error {
	if len(endpoints) == 0 {
		return fmt.Errorf("no endpoints to add")
	}

	var cfg map[string]interface{}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			cfg = make(map[string]interface{})
		} else {
			return fmt.Errorf("reading config: %w", err)
		}
	} else {
		if err := toml.Unmarshal(data, &cfg); err != nil {
			return fmt.Errorf("parsing config: %w", err)
		}
	}

	if len(data) > 0 {
		backupPath := path + ".yokai.bak"
		if err := os.WriteFile(backupPath, data, 0644); err != nil {
			return fmt.Errorf("creating backup: %w", err)
		}
	}

	ep := endpoints[0]
	cfg["model"] = ep.ModelID
	cfg["model_provider"] = ProviderKey

	providers, _ := cfg["model_providers"].(map[string]interface{})
	if providers == nil {
		providers = make(map[string]interface{})
	}
	providers[ProviderKey] = map[string]interface{}{
		"name":     "Yokai",
		"base_url": ep.BaseURL,
	}
	cfg["model_providers"] = providers

	return writeConfig(path, cfg)
}

func HasYokaiConfig(path string) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}

	var cfg map[string]interface{}
	if err := toml.Unmarshal(data, &cfg); err != nil {
		return false
	}

	provider, _ := cfg["model_provider"].(string)
	if provider == ProviderKey {
		return true
	}
	providers, _ := cfg["model_providers"].(map[string]interface{})
	_, ok := providers[ProviderKey]
	return ok
}

func writeConfig(path string, cfg map[string]interface{}) error {
	out, err := toml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshaling config: %w", err)
	}
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
