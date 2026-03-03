package vscode

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

// Endpoint represents an OpenAI-compatible model endpoint.
type Endpoint struct {
	Family string `json:"family"`
	ID     string `json:"id"`
	Name   string `json:"name"`
	URL    string `json:"url"`
	APIKey string `json:"apiKey"`
}

// DetectSettingsPath finds the VS Code settings.json path.
func DetectSettingsPath() (string, error) {
	var base string
	switch runtime.GOOS {
	case "linux":
		configDir := os.Getenv("XDG_CONFIG_HOME")
		if configDir == "" {
			home, _ := os.UserHomeDir()
			configDir = filepath.Join(home, ".config")
		}
		base = filepath.Join(configDir, "Code", "User")
	case "darwin":
		home, _ := os.UserHomeDir()
		base = filepath.Join(home, "Library", "Application Support", "Code", "User")
	default:
		return "", fmt.Errorf("unsupported OS: %s", runtime.GOOS)
	}

	path := filepath.Join(base, "settings.json")

	// Also check Insiders
	if _, err := os.Stat(path); os.IsNotExist(err) {
		insidersBase := filepath.Join(filepath.Dir(base), "Code - Insiders", "User")
		insidersPath := filepath.Join(insidersBase, "settings.json")
		if _, err := os.Stat(insidersPath); err == nil {
			return insidersPath, nil
		}
	}

	return path, nil
}

// AddEndpoints reads VS Code settings.json and adds yokai model endpoints.
// It creates a backup before modifying.
func AddEndpoints(endpoints []Endpoint) error {
	path, err := DetectSettingsPath()
	if err != nil {
		return err
	}

	// Read existing settings
	var settings map[string]interface{}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			settings = make(map[string]interface{})
		} else {
			return fmt.Errorf("reading settings: %w", err)
		}
	} else {
		if err := json.Unmarshal(data, &settings); err != nil {
			return fmt.Errorf("parsing settings: %w", err)
		}
	}

	// Backup
	if len(data) > 0 {
		backupPath := path + ".yokai.bak"
		if err := os.WriteFile(backupPath, data, 0644); err != nil {
			return fmt.Errorf("creating backup: %w", err)
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
		return fmt.Errorf("marshaling settings: %w", err)
	}
	out = append(out, '\n')

	// Ensure directory exists
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("creating settings dir: %w", err)
	}

	tmpPath := path + ".tmp"
	if err := os.WriteFile(tmpPath, out, 0644); err != nil {
		return fmt.Errorf("writing settings: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		_ = os.Remove(tmpPath) // Best-effort cleanup of temporary settings file.
		return fmt.Errorf("renaming settings: %w", err)
	}

	return nil
}

// RemoveEndpoints removes all yokai-added endpoints from settings.
func RemoveEndpoints(settingsPath string) error {
	data, err := os.ReadFile(settingsPath)
	if err != nil {
		return err
	}

	var settings map[string]interface{}
	if err := json.Unmarshal(data, &settings); err != nil {
		return err
	}

	if existing, ok := settings["chat.models"]; ok {
		if arr, ok := existing.([]interface{}); ok {
			var filtered []interface{}
			for _, item := range arr {
				if m, ok := item.(map[string]interface{}); ok {
					if name, ok := m["name"].(string); ok {
						if len(name) < 7 || name[len(name)-7:] != "(yokai)" {
							filtered = append(filtered, item)
						}
						continue
					}
				}
				filtered = append(filtered, item)
			}
			settings["chat.models"] = filtered
		}
	}

	out, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return err
	}
	out = append(out, '\n')
	return os.WriteFile(settingsPath, out, 0644)
}

func endpointExists(models []interface{}, url string) bool {
	for _, item := range models {
		if m, ok := item.(map[string]interface{}); ok {
			if existingURL, ok := m["url"].(string); ok && existingURL == url {
				return true
			}
		}
	}
	return false
}
