package daemon

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/spencerbull/yokai/internal/claudecode"
	"github.com/spencerbull/yokai/internal/codex"
	"github.com/spencerbull/yokai/internal/config"
	"github.com/spencerbull/yokai/internal/openclaw"
	"github.com/spencerbull/yokai/internal/opencode"
	"github.com/spencerbull/yokai/internal/vscode"
)

type openAIEndpointRecord struct {
	ServiceID   string   `json:"service_id"`
	DeviceID    string   `json:"device_id"`
	DeviceLabel string   `json:"device_label"`
	Host        string   `json:"host"`
	Port        int      `json:"port"`
	BaseURL     string   `json:"base_url"`
	ServiceType string   `json:"service_type"`
	ModelIDs    []string `json:"model_ids"`
	DisplayName string   `json:"display_name"`
	Reachable   bool     `json:"reachable"`
}

type configureIntegrationsRequest struct {
	Tools []string `json:"tools"`
}

type configureIntegrationResult struct {
	Name string `json:"name"`
	OK   bool   `json:"ok"`
	Err  string `json:"err,omitempty"`
}

type configureIntegrationsResponse struct {
	Results []configureIntegrationResult `json:"results"`
}

func (d *Daemon) handleGetOpenAIEndpoints(w http.ResponseWriter, r *http.Request) {
	endpoints, err := d.discoverOpenAIEndpoints()
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{
			"error":   "endpoint_discovery_failed",
			"message": err.Error(),
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{"endpoints": endpoints})
}

func (d *Daemon) handleConfigureIntegrations(w http.ResponseWriter, r *http.Request) {
	var req configureIntegrationsRequest
	if err := decodeJSONBody(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error":   "bad_request",
			"message": err.Error(),
		})
		return
	}

	selected := normalizeToolNames(req.Tools)
	if len(selected) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error":   "bad_request",
			"message": "at least one tool must be selected",
		})
		return
	}

	endpoints, err := d.discoverOpenAIEndpoints()
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{
			"error":   "endpoint_discovery_failed",
			"message": err.Error(),
		})
		return
	}
	if len(endpoints) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error":   "no_endpoints",
			"message": "no OpenAI-compatible services running",
		})
		return
	}

	var results []configureIntegrationResult
	for _, tool := range selected {
		switch tool {
		case "vscode":
			results = append(results, configureVSCode(endpoints))
		case "claudecode":
			results = append(results, configureClaudeCode(endpoints))
		case "codex":
			results = append(results, configureCodex(endpoints))
		case "opencode":
			results = append(results, configureOpenCode(endpoints))
		case "openclaw":
			results = append(results, configureOpenClaw(endpoints))
		}
	}

	writeJSON(w, http.StatusOK, configureIntegrationsResponse{Results: results})
}

func (d *Daemon) discoverOpenAIEndpoints() ([]openAIEndpointRecord, error) {
	d.mu.RLock()
	services := append([]config.Service(nil), d.cfg.Services...)
	devices := append([]config.Device(nil), d.cfg.Devices...)
	d.mu.RUnlock()

	deviceByID := make(map[string]config.Device, len(devices))
	for _, device := range devices {
		deviceByID[device.ID] = device
	}

	httpClient := &http.Client{Timeout: 5 * time.Second}
	endpoints := make([]openAIEndpointRecord, 0)
	for _, service := range services {
		if service.Type == "comfyui" {
			continue
		}
		device, ok := deviceByID[service.DeviceID]
		if !ok {
			continue
		}
		modelName := strings.TrimSpace(service.Model)
		if modelName == "" {
			modelName = service.Type
		}
		models, reachable := resolveOpenAIModelIDs(httpClient, device.Host, service.Port, modelName)
		if !reachable {
			continue
		}
		endpoints = append(endpoints, openAIEndpointRecord{
			ServiceID:   service.ID,
			DeviceID:    device.ID,
			DeviceLabel: firstNonEmpty(device.Label, device.ID),
			Host:        device.Host,
			Port:        service.Port,
			BaseURL:     fmt.Sprintf("http://%s:%d/v1", device.Host, service.Port),
			ServiceType: service.Type,
			ModelIDs:    models,
			DisplayName: fmt.Sprintf("%s / %s", firstNonEmpty(device.Label, device.ID), modelName),
			Reachable:   true,
		})
	}

	return endpoints, nil
}

func resolveOpenAIModelIDs(client *http.Client, host string, port int, fallback string) ([]string, bool) {
	fallback = strings.TrimSpace(fallback)
	req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("http://%s:%d/v1/models", host, port), nil)
	if err != nil {
		return nil, false
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, false
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, false
	}

	var payload struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil || len(payload.Data) == 0 {
		if fallback == "" {
			return nil, true
		}
		return []string{fallback}, true
	}

	seen := make(map[string]struct{}, len(payload.Data))
	models := make([]string, 0, len(payload.Data))
	for _, model := range payload.Data {
		id := strings.TrimSpace(model.ID)
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		models = append(models, id)
	}
	if len(models) == 0 && fallback != "" {
		return []string{fallback}, true
	}
	return models, true
}

func configureVSCode(endpoints []openAIEndpointRecord) configureIntegrationResult {
	vscodeEndpoints := make([]vscode.Endpoint, 0, len(endpoints))
	for _, endpoint := range endpoints {
		for _, modelID := range endpoint.ModelIDs {
			vscodeEndpoints = append(vscodeEndpoints, vscode.Endpoint{
				Family: "openai",
				ID:     endpoint.ServiceID,
				Name:   fmt.Sprintf("%s (yokai)", modelID),
				URL:    endpoint.BaseURL,
				APIKey: "none",
			})
		}
	}
	if err := vscode.AddEndpoints(vscodeEndpoints); err != nil {
		return configureIntegrationResult{Name: "VS Code Copilot", OK: false, Err: err.Error()}
	}
	return configureIntegrationResult{Name: "VS Code Copilot", OK: true}
}

func configureOpenCode(endpoints []openAIEndpointRecord) configureIntegrationResult {
	if legacyPath, err := opencode.DetectConfigPath(); err == nil {
		_ = opencode.MigrateLegacyConfig(legacyPath)
	}

	opencodeEndpoints := make([]opencode.Endpoint, 0, len(endpoints))
	for _, endpoint := range endpoints {
		for _, modelID := range endpoint.ModelIDs {
			opencodeEndpoints = append(opencodeEndpoints, opencode.Endpoint{
				BaseURL:   endpoint.BaseURL,
				ModelID:   modelID,
				ModelName: fmt.Sprintf("%s (yokai)", modelID),
			})
		}
	}
	if err := opencode.AddEndpoints(opencodeEndpoints); err != nil {
		return configureIntegrationResult{Name: "OpenCode", OK: false, Err: err.Error()}
	}
	return configureIntegrationResult{Name: "OpenCode", OK: true}
}

func configureClaudeCode(endpoints []openAIEndpointRecord) configureIntegrationResult {
	if len(endpoints) == 0 {
		return configureIntegrationResult{Name: "Claude Code", OK: false, Err: "no OpenAI-compatible endpoints available"}
	}
	chosen := endpoints[0]
	modelID := firstModelID(chosen.ModelIDs, chosen.ServiceType)
	if err := claudecode.AddEndpoints([]claudecode.Endpoint{{
		BaseURL:   chosen.BaseURL,
		ModelID:   modelID,
		ModelName: fmt.Sprintf("%s (yokai)", modelID),
	}}); err != nil {
		return configureIntegrationResult{Name: "Claude Code", OK: false, Err: err.Error()}
	}
	return configureIntegrationResult{Name: "Claude Code", OK: true}
}

func configureCodex(endpoints []openAIEndpointRecord) configureIntegrationResult {
	if len(endpoints) == 0 {
		return configureIntegrationResult{Name: "Codex", OK: false, Err: "no OpenAI-compatible endpoints available"}
	}
	chosen := endpoints[0]
	modelID := firstModelID(chosen.ModelIDs, chosen.ServiceType)
	if err := codex.AddEndpoints([]codex.Endpoint{{
		BaseURL:   chosen.BaseURL,
		ModelID:   modelID,
		ModelName: fmt.Sprintf("%s (yokai)", modelID),
	}}); err != nil {
		return configureIntegrationResult{Name: "Codex", OK: false, Err: err.Error()}
	}
	return configureIntegrationResult{Name: "Codex", OK: true}
}

func configureOpenClaw(endpoints []openAIEndpointRecord) configureIntegrationResult {
	openclawEndpoints := make([]openclaw.Endpoint, 0, len(endpoints))
	for _, endpoint := range endpoints {
		for _, modelID := range endpoint.ModelIDs {
			openclawEndpoints = append(openclawEndpoints, openclaw.Endpoint{
				BaseURL:   endpoint.BaseURL,
				ModelID:   modelID,
				ModelName: fmt.Sprintf("%s (yokai)", modelID),
			})
		}
	}
	if err := openclaw.AddEndpoints(openclawEndpoints); err != nil {
		return configureIntegrationResult{Name: "OpenClaw", OK: false, Err: err.Error()}
	}
	return configureIntegrationResult{Name: "OpenClaw", OK: true}
}

func normalizeToolNames(tools []string) []string {
	seen := make(map[string]struct{}, len(tools))
	var normalized []string
	for _, tool := range tools {
		switch strings.ToLower(strings.TrimSpace(tool)) {
		case "vscode", "vscode copilot", "vs code copilot":
			if _, ok := seen["vscode"]; !ok {
				seen["vscode"] = struct{}{}
				normalized = append(normalized, "vscode")
			}
		case "claudecode", "claude code", "claude":
			if _, ok := seen["claudecode"]; !ok {
				seen["claudecode"] = struct{}{}
				normalized = append(normalized, "claudecode")
			}
		case "codex":
			if _, ok := seen["codex"]; !ok {
				seen["codex"] = struct{}{}
				normalized = append(normalized, "codex")
			}
		case "opencode":
			if _, ok := seen["opencode"]; !ok {
				seen["opencode"] = struct{}{}
				normalized = append(normalized, "opencode")
			}
		case "openclaw":
			if _, ok := seen["openclaw"]; !ok {
				seen["openclaw"] = struct{}{}
				normalized = append(normalized, "openclaw")
			}
		}
	}
	return normalized
}

func firstModelID(models []string, fallback string) string {
	for _, model := range models {
		if strings.TrimSpace(model) != "" {
			return strings.TrimSpace(model)
		}
	}
	return strings.TrimSpace(fallback)
}
