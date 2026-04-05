package claudecode

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/tailscale/hujson"
)

type Endpoint struct {
	BaseURL   string
	ModelID   string
	ModelName string
}

func DetectSettingsPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("cannot determine home directory: %w", err)
	}
	return filepath.Join(home, ".claude", "settings.json"), nil
}

func AddEndpoints(endpoints []Endpoint) error {
	path, err := DetectSettingsPath()
	if err != nil {
		return err
	}
	return AddEndpointsToFile(path, endpoints)
}

func AddEndpointsToFile(path string, endpoints []Endpoint) error {
	if len(endpoints) == 0 {
		return fmt.Errorf("no endpoints to add")
	}

	settings, original, err := loadSettings(path)
	if err != nil {
		return err
	}
	if len(original) > 0 {
		backupPath := path + ".yokai.bak"
		if err := os.WriteFile(backupPath, original, 0644); err != nil {
			return fmt.Errorf("creating backup: %w", err)
		}
	}

	ep := endpoints[0]
	env := ensureObject(settings, "env")
	env["ANTHROPIC_BASE_URL"] = trimV1(ep.BaseURL)
	env["ANTHROPIC_CUSTOM_MODEL_OPTION"] = ep.ModelID
	env["ANTHROPIC_CUSTOM_MODEL_OPTION_NAME"] = ep.ModelName
	env["ANTHROPIC_MODEL"] = ep.ModelID
	settings["env"] = env
	settings["model"] = ep.ModelID

	return writeSettings(path, settings)
}

func HasYokaiConfig(path string) bool {
	settings, _, err := loadSettings(path)
	if err != nil {
		return false
	}
	env, _ := settings["env"].(map[string]interface{})
	if env == nil {
		return false
	}
	_, hasBaseURL := env["ANTHROPIC_BASE_URL"]
	_, hasModel := env["ANTHROPIC_MODEL"]
	_, hasCustom := env["ANTHROPIC_CUSTOM_MODEL_OPTION"]
	return hasBaseURL && (hasModel || hasCustom)
}

func loadSettings(path string) (map[string]interface{}, []byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return make(map[string]interface{}), nil, nil
		}
		return nil, nil, fmt.Errorf("reading settings: %w", err)
	}
	ast, err := hujson.Parse(data)
	if err != nil {
		return nil, nil, fmt.Errorf("parsing settings: %w", err)
	}
	ast.Standardize()
	var settings map[string]interface{}
	if err := json.Unmarshal(ast.Pack(), &settings); err != nil {
		return nil, nil, fmt.Errorf("decoding settings: %w", err)
	}
	return settings, data, nil
}

func writeSettings(path string, settings map[string]interface{}) error {
	out, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling settings: %w", err)
	}
	out = append(out, '\n')
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("creating settings dir: %w", err)
	}
	tmpPath := path + ".tmp"
	if err := os.WriteFile(tmpPath, out, 0644); err != nil {
		return fmt.Errorf("writing settings: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("renaming settings: %w", err)
	}
	return nil
}

func ensureObject(settings map[string]interface{}, key string) map[string]interface{} {
	if existing, ok := settings[key].(map[string]interface{}); ok && existing != nil {
		return existing
	}
	created := make(map[string]interface{})
	settings[key] = created
	return created
}

func trimV1(baseURL string) string {
	if len(baseURL) >= 3 && baseURL[len(baseURL)-3:] == "/v1" {
		return baseURL[:len(baseURL)-3]
	}
	return baseURL
}
