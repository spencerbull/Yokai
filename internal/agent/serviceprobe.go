package agent

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"
)

type ServiceTestResult struct {
	OK           bool   `json:"ok"`
	ServiceType  string `json:"service_type"`
	Message      string `json:"message"`
	Response     string `json:"response,omitempty"`
	Model        string `json:"model,omitempty"`
	PromptID     string `json:"prompt_id,omitempty"`
	MetricsReady bool   `json:"metrics_ready,omitempty"`
}

var serviceTestHTTPClient = &http.Client{Timeout: 45 * time.Second}
var requiredMetricsHTTPClient = &http.Client{Timeout: 30 * time.Second}

func testContainerServiceWithOptions(container Container, requireMetrics bool, apiKey string) (*ServiceTestResult, error) {
	baseURL, err := containerBaseURL(container)
	if err != nil {
		return nil, err
	}

	var result *ServiceTestResult
	switch {
	case isVLLMImage(container.Image), isSGLangImage(container.Image), isLlamaCppImage(container.Image):
		result, err = testOpenAICompatibleServiceWithAPIKey(baseURL, inferServiceKindFromImage(container.Image), apiKey)
	case isComfyUIImage(container.Image):
		result, err = testComfyUIService(baseURL)
	default:
		return nil, fmt.Errorf("service test is not supported for image %s", container.Image)
	}
	if err != nil {
		return nil, err
	}
	if requireMetrics {
		metrics, err := scrapeVLLMMetricsURLWithClient(requiredMetricsHTTPClient, baseURL+"/metrics", apiKey)
		if err != nil {
			return nil, fmt.Errorf("required inference metrics gate failed: %w", err)
		}
		if !metrics.hasNativeFamily(result.ServiceType) {
			return nil, fmt.Errorf("required inference metrics gate failed: no recognized %s native metric family", result.ServiceType)
		}
		result.MetricsReady = true
	}
	return result, nil
}

func (m *VLLMMetrics) hasNativeFamily(serviceType string) bool {
	switch serviceType {
	case "sglang":
		return m.HasSGLangNativeMetric
	case "vllm":
		return m.HasVLLMNativeMetric
	default:
		return m.HasSGLangNativeMetric || m.HasVLLMNativeMetric
	}
}

func containerBaseURL(container Container) (string, error) {
	for _, externalPort := range container.Ports {
		if strings.TrimSpace(externalPort) != "" {
			host := ""
			if container.Ownership == OwnershipManaged || container.Ownership == OwnershipAdopted {
				host = strings.TrimSpace(container.Labels[LabelServiceAddress])
			}
			if host == "" {
				host = "127.0.0.1"
			}
			return "http://" + net.JoinHostPort(host, strings.TrimSpace(externalPort)), nil
		}
	}
	return "", fmt.Errorf("container %s has no exposed ports", container.Name)
}

func inferServiceKindFromImage(image string) string {
	switch {
	case isVLLMImage(image):
		return "vllm"
	case isSGLangImage(image):
		return "sglang"
	case isLlamaCppImage(image):
		return "llamacpp"
	case isComfyUIImage(image):
		return "comfyui"
	default:
		return "unknown"
	}
}

func testOpenAICompatibleService(baseURL, serviceType string) (*ServiceTestResult, error) {
	return testOpenAICompatibleServiceWithAPIKey(baseURL, serviceType, "")
}

func testOpenAICompatibleServiceWithAPIKey(baseURL, serviceType, apiKey string) (*ServiceTestResult, error) {
	modelID, err := fetchServedModelID(baseURL, apiKey)
	if err != nil {
		return nil, err
	}

	body := map[string]interface{}{
		"model": modelID,
		"messages": []map[string]string{
			{"role": "user", "content": "Reply with exactly: ok"},
		},
		"max_tokens":  128,
		"temperature": 0,
	}
	if serviceType == "sglang" {
		// GLM's pinned chat template always emits a reasoning block and defaults
		// reasoning effort to max. Keep its reasoning bounded while the shared
		// 128-token allowance leaves room for reasoning plus the final response.
		body["reasoning_effort"] = "low"
	}

	var response struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := postJSONWithAPIKey(baseURL+"/v1/chat/completions", body, &response, apiKey); err != nil {
		return nil, err
	}

	content := strings.TrimSpace(responseChoiceContent(response.Choices))
	if content == "" {
		return nil, fmt.Errorf("%s test returned no response text", serviceType)
	}
	if !strings.EqualFold(content, "ok") {
		return nil, fmt.Errorf("%s test response must be exactly ok, got %q", serviceType, trimForDisplay(content, 80))
	}

	return &ServiceTestResult{
		OK:          true,
		ServiceType: serviceType,
		Model:       modelID,
		Message:     fmt.Sprintf("%s chat test passed with model %s", serviceType, modelID),
		Response:    content,
	}, nil
}

func responseChoiceContent(choices []struct {
	Message struct {
		Content string `json:"content"`
	} `json:"message"`
}) string {
	if len(choices) == 0 {
		return ""
	}
	return choices[0].Message.Content
}

func fetchServedModelID(baseURL, apiKey string) (string, error) {
	var response struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := getJSONWithAPIKey(baseURL+"/v1/models", &response, apiKey); err != nil {
		return "", fmt.Errorf("fetching served models: %w", err)
	}
	if len(response.Data) == 0 || strings.TrimSpace(response.Data[0].ID) == "" {
		return "", fmt.Errorf("service did not report any served models")
	}
	return strings.TrimSpace(response.Data[0].ID), nil
}

func testComfyUIService(baseURL string) (*ServiceTestResult, error) {
	ckptName, err := fetchComfyCheckpoint(baseURL)
	if err != nil {
		return nil, err
	}

	prompt := map[string]interface{}{
		"4": map[string]interface{}{
			"inputs":     map[string]interface{}{"ckpt_name": ckptName},
			"class_type": "CheckpointLoaderSimple",
		},
		"5": map[string]interface{}{
			"inputs":     map[string]interface{}{"width": 512, "height": 512, "batch_size": 1},
			"class_type": "EmptyLatentImage",
		},
		"6": map[string]interface{}{
			"inputs":     map[string]interface{}{"text": "a simple grey square", "clip": []interface{}{"4", 1}},
			"class_type": "CLIPTextEncode",
		},
		"7": map[string]interface{}{
			"inputs":     map[string]interface{}{"text": "", "clip": []interface{}{"4", 1}},
			"class_type": "CLIPTextEncode",
		},
		"3": map[string]interface{}{
			"inputs": map[string]interface{}{
				"seed":         1,
				"steps":        1,
				"cfg":          1,
				"sampler_name": "euler",
				"scheduler":    "normal",
				"denoise":      1,
				"model":        []interface{}{"4", 0},
				"positive":     []interface{}{"6", 0},
				"negative":     []interface{}{"7", 0},
				"latent_image": []interface{}{"5", 0},
			},
			"class_type": "KSampler",
		},
		"8": map[string]interface{}{
			"inputs":     map[string]interface{}{"samples": []interface{}{"3", 0}, "vae": []interface{}{"4", 2}},
			"class_type": "VAEDecode",
		},
		"9": map[string]interface{}{
			"inputs":     map[string]interface{}{"filename_prefix": "yokai_test", "images": []interface{}{"8", 0}},
			"class_type": "SaveImage",
		},
	}

	body := map[string]interface{}{
		"prompt":    prompt,
		"client_id": "yokai-smoketest",
	}

	var response struct {
		PromptID string `json:"prompt_id"`
	}
	if err := postJSON(baseURL+"/prompt", body, &response); err != nil {
		return nil, err
	}
	if strings.TrimSpace(response.PromptID) == "" {
		return nil, fmt.Errorf("ComfyUI test was accepted without a prompt_id")
	}

	return &ServiceTestResult{
		OK:          true,
		ServiceType: "comfyui",
		PromptID:    response.PromptID,
		Message:     fmt.Sprintf("ComfyUI workflow test queued with checkpoint %s (prompt %s)", ckptName, response.PromptID),
	}, nil
}

func fetchComfyCheckpoint(baseURL string) (string, error) {
	var response map[string]struct {
		Input struct {
			Required map[string][]interface{} `json:"required"`
		} `json:"input"`
	}
	if err := getJSON(baseURL+"/object_info/CheckpointLoaderSimple", &response); err != nil {
		return "", fmt.Errorf("fetching ComfyUI checkpoint metadata: %w", err)
	}
	node, ok := response["CheckpointLoaderSimple"]
	if !ok {
		return "", fmt.Errorf("ComfyUI did not return CheckpointLoaderSimple metadata")
	}
	values, ok := node.Input.Required["ckpt_name"]
	if !ok || len(values) == 0 {
		return "", fmt.Errorf("ComfyUI did not report any checkpoint names")
	}
	options, ok := values[0].([]interface{})
	if !ok || len(options) == 0 {
		return "", fmt.Errorf("ComfyUI checkpoint metadata was malformed")
	}
	ckptName, ok := options[0].(string)
	if !ok || strings.TrimSpace(ckptName) == "" {
		return "", fmt.Errorf("ComfyUI reported an empty checkpoint name")
	}
	return ckptName, nil
}

func getJSON(url string, out interface{}) error {
	return getJSONWithAPIKey(url, out, "")
}

func getJSONWithAPIKey(url string, out interface{}, apiKey string) error {
	request, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	if apiKey != "" {
		request.Header.Set("Authorization", "Bearer "+apiKey)
	}
	resp, err := serviceTestHTTPClient.Do(request)
	if err != nil {
		return redactSecretError(err, apiKey)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return redactSecretError(decodeHTTPError(resp), apiKey)
	}
	return redactSecretError(json.NewDecoder(resp.Body).Decode(out), apiKey)
}

func postJSON(url string, body, out interface{}) error {
	return postJSONWithAPIKey(url, body, out, "")
}

func postJSONWithAPIKey(url string, body, out interface{}, apiKey string) error {
	payload, err := json.Marshal(body)
	if err != nil {
		return err
	}
	request, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/json")
	if apiKey != "" {
		request.Header.Set("Authorization", "Bearer "+apiKey)
	}
	resp, err := serviceTestHTTPClient.Do(request)
	if err != nil {
		return redactSecretError(err, apiKey)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return redactSecretError(decodeHTTPError(resp), apiKey)
	}
	return redactSecretError(json.NewDecoder(resp.Body).Decode(out), apiKey)
}

func redactSecretError(err error, secret string) error {
	if err == nil || secret == "" {
		return err
	}
	return fmt.Errorf("%s", strings.ReplaceAll(err.Error(), secret, "[REDACTED]"))
}

func decodeHTTPError(resp *http.Response) error {
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
	trimmed := strings.TrimSpace(string(body))
	if trimmed == "" {
		return fmt.Errorf("service returned status %d", resp.StatusCode)
	}
	return fmt.Errorf("service returned status %d: %s", resp.StatusCode, trimmed)
}

func trimForDisplay(s string, max int) string {
	s = strings.Join(strings.Fields(s), " ")
	if max <= 0 || len(s) <= max {
		return s
	}
	if max <= 3 {
		return s[:max]
	}
	return s[:max-3] + "..."
}
