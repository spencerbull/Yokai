package daemon

import (
	"encoding/json"
	"net/http"
	"os"
	"strings"

	"github.com/spencerbull/yokai/internal/claudecode"
	"github.com/spencerbull/yokai/internal/codex"
	"github.com/spencerbull/yokai/internal/config"
	"github.com/spencerbull/yokai/internal/hf"
	"github.com/spencerbull/yokai/internal/openclaw"
	"github.com/spencerbull/yokai/internal/opencode"
	"github.com/spencerbull/yokai/internal/vscode"
)

type uiSettingsResponse struct {
	HF           hfSettingsStatus   `json:"hf"`
	Preferences  config.Preferences `json:"preferences"`
	History      config.History     `json:"history"`
	Integrations integrationsStatus `json:"integrations"`
}

type hfSettingsStatus struct {
	Configured bool   `json:"configured"`
	Source     string `json:"source"`
	Username   string `json:"username,omitempty"`
}

type integrationsStatus struct {
	VSCode     integrationToolStatus `json:"vscode"`
	OpenCode   integrationToolStatus `json:"opencode"`
	OpenClaw   integrationToolStatus `json:"openclaw"`
	ClaudeCode integrationToolStatus `json:"claudecode"`
	Codex      integrationToolStatus `json:"codex"`
}

type integrationToolStatus struct {
	Available  bool   `json:"available"`
	Configured bool   `json:"configured"`
	Path       string `json:"path,omitempty"`
	Note       string `json:"note,omitempty"`
}

type settingsPatchRequest struct {
	Preferences *preferencesPatch `json:"preferences,omitempty"`
}

type preferencesPatch struct {
	Theme             *string `json:"theme,omitempty"`
	DefaultVLLMImage  *string `json:"default_vllm_image,omitempty"`
	DefaultLlamaImage *string `json:"default_llama_image,omitempty"`
	DefaultComfyImage *string `json:"default_comfyui_image,omitempty"`
}

type hfTokenRequest struct {
	Token string `json:"token"`
}

type hfTokenValidationResponse struct {
	Valid    bool   `json:"valid"`
	Username string `json:"username,omitempty"`
}

type deployHistoryRequest struct {
	Images []string `json:"images"`
	Models []string `json:"models"`
}

func (d *Daemon) handleGetSettings(w http.ResponseWriter, r *http.Request) {
	response, err := d.buildSettingsResponse()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"error":   "settings_load_failed",
			"message": err.Error(),
		})
		return
	}

	writeJSON(w, http.StatusOK, response)
}

func (d *Daemon) handlePatchSettings(w http.ResponseWriter, r *http.Request) {
	var req settingsPatchRequest
	if err := decodeJSONBody(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error":   "bad_request",
			"message": err.Error(),
		})
		return
	}

	if req.Preferences == nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error":   "bad_request",
			"message": "preferences patch is required",
		})
		return
	}

	d.mu.Lock()
	previous := d.cfg.Preferences
	applyPreferencesPatch(&d.cfg.Preferences, req.Preferences)
	if err := config.Save(d.cfg); err != nil {
		d.cfg.Preferences = previous
		d.mu.Unlock()
		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"error":   "settings_save_failed",
			"message": err.Error(),
		})
		return
	}
	d.mu.Unlock()

	response, err := d.buildSettingsResponse()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"error":   "settings_load_failed",
			"message": err.Error(),
		})
		return
	}

	writeJSON(w, http.StatusOK, response)
}

func (d *Daemon) handlePutHFToken(w http.ResponseWriter, r *http.Request) {
	var req hfTokenRequest
	if err := decodeJSONBody(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error":   "bad_request",
			"message": err.Error(),
		})
		return
	}

	token := strings.TrimSpace(req.Token)

	d.mu.Lock()
	previous := d.cfg.HFToken
	d.cfg.HFToken = token
	if err := config.Save(d.cfg); err != nil {
		d.cfg.HFToken = previous
		d.mu.Unlock()
		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"error":   "settings_save_failed",
			"message": err.Error(),
		})
		return
	}
	d.mu.Unlock()

	writeJSON(w, http.StatusOK, currentHFSettings(token))
}

func (d *Daemon) handleValidateHFToken(w http.ResponseWriter, r *http.Request) {
	var req hfTokenRequest
	if err := decodeJSONBody(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error":   "bad_request",
			"message": err.Error(),
		})
		return
	}

	token := strings.TrimSpace(req.Token)
	if token == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error":   "bad_request",
			"message": "token is required",
		})
		return
	}

	username, err := hf.NewClient(token).ValidateToken()
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{
			"error":   "token_validation_failed",
			"message": err.Error(),
		})
		return
	}

	writeJSON(w, http.StatusOK, hfTokenValidationResponse{Valid: true, Username: username})
}

func (d *Daemon) handleGetDeployHistory(w http.ResponseWriter, r *http.Request) {
	history, err := config.LoadHistory()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"error":   "history_load_failed",
			"message": err.Error(),
		})
		return
	}

	writeJSON(w, http.StatusOK, history)
}

func (d *Daemon) handlePutDeployHistory(w http.ResponseWriter, r *http.Request) {
	var req deployHistoryRequest
	if err := decodeJSONBody(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error":   "bad_request",
			"message": err.Error(),
		})
		return
	}

	history := &config.History{
		Images: normalizeHistoryItems(req.Images),
		Models: normalizeHistoryItems(req.Models),
	}

	if err := config.SaveHistory(history); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"error":   "history_save_failed",
			"message": err.Error(),
		})
		return
	}

	writeJSON(w, http.StatusOK, history)
}

func (d *Daemon) buildSettingsResponse() (uiSettingsResponse, error) {
	d.mu.RLock()
	preferences := d.cfg.Preferences
	hfToken := d.cfg.HFToken
	d.mu.RUnlock()

	history, err := config.LoadHistory()
	if err != nil {
		return uiSettingsResponse{}, err
	}

	return uiSettingsResponse{
		HF:          currentHFSettings(hfToken),
		Preferences: preferences,
		History:     *history,
		Integrations: integrationsStatus{
			VSCode:     detectIntegrationStatus(vscode.DetectSettingsPath, hasVSCodeEndpoints),
			OpenCode:   detectIntegrationStatus(opencode.DetectConfigPath, opencode.HasYokaiEndpoints),
			OpenClaw:   detectIntegrationStatus(openclaw.DetectConfigPath, openclaw.HasYokaiEndpoints),
			ClaudeCode: detectIntegrationStatus(claudecode.DetectSettingsPath, claudecode.HasYokaiConfig),
			Codex:      detectIntegrationStatus(codex.DetectConfigPath, codex.HasYokaiConfig),
		},
	}, nil
}

func applyPreferencesPatch(preferences *config.Preferences, patch *preferencesPatch) {
	if patch.Theme != nil {
		preferences.Theme = strings.TrimSpace(*patch.Theme)
	}
	if patch.DefaultVLLMImage != nil {
		preferences.DefaultVLLMImage = strings.TrimSpace(*patch.DefaultVLLMImage)
	}
	if patch.DefaultLlamaImage != nil {
		preferences.DefaultLlamaImage = strings.TrimSpace(*patch.DefaultLlamaImage)
	}
	if patch.DefaultComfyImage != nil {
		preferences.DefaultComfyImage = strings.TrimSpace(*patch.DefaultComfyImage)
	}
}

func currentHFSettings(configToken string) hfSettingsStatus {
	if token := strings.TrimSpace(loadHFTokenFromEnv()); token != "" {
		return hfSettingsStatus{Configured: true, Source: "env"}
	}

	if strings.TrimSpace(configToken) != "" {
		return hfSettingsStatus{Configured: true, Source: "config"}
	}

	return hfSettingsStatus{Configured: false, Source: "none"}
}

func loadHFTokenFromEnv() string {
	if token := strings.TrimSpace(os.Getenv("HF_TOKEN")); token != "" {
		return token
	}

	if token := strings.TrimSpace(os.Getenv("HUGGING_FACE_HUB_TOKEN")); token != "" {
		return token
	}

	return ""
}

func detectIntegrationStatus(detect func() (string, error), hasYokaiEndpoints func(string) bool) integrationToolStatus {
	path, err := detect()
	if err != nil {
		return integrationToolStatus{}
	}

	return integrationToolStatus{
		Available:  true,
		Configured: hasYokaiEndpoints(path),
		Path:       path,
	}
}

func hasVSCodeEndpoints(settingsPath string) bool {
	return strings.TrimSpace(settingsPath) != "" && vscodeHasYokaiEndpoints(settingsPath)
}

func normalizeHistoryItems(items []string) []string {
	seen := make(map[string]struct{}, len(items))
	result := make([]string, 0, min(len(items), config.MaxHistoryItems))

	for _, item := range items {
		normalized := strings.TrimSpace(item)
		if normalized == "" {
			continue
		}
		if _, ok := seen[normalized]; ok {
			continue
		}
		seen[normalized] = struct{}{}
		result = append(result, normalized)
		if len(result) >= config.MaxHistoryItems {
			break
		}
	}

	return result
}

func decodeJSONBody(r *http.Request, dst interface{}) error {
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	return decoder.Decode(dst)
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func vscodeHasYokaiEndpoints(settingsPath string) bool {
	data, err := os.ReadFile(settingsPath)
	if err != nil {
		return false
	}

	var settings map[string]interface{}
	if err := json.Unmarshal(data, &settings); err != nil {
		return false
	}

	models, ok := settings["chat.models"].([]interface{})
	if !ok {
		return false
	}

	for _, item := range models {
		model, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		name, _ := model["name"].(string)
		if strings.HasSuffix(name, "(yokai)") {
			return true
		}
	}

	return false
}
