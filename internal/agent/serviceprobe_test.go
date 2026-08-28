package agent

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestRequiredMetricsGateUsesPatientBoundedTimeout(t *testing.T) {
	if requiredMetricsHTTPClient.Timeout != 30*time.Second {
		t.Fatalf("required metrics scrape timeout = %s, want 30s", requiredMetricsHTTPClient.Timeout)
	}
	if inventoryMetricsHTTPClient.Timeout >= requiredMetricsHTTPClient.Timeout {
		t.Fatalf("best-effort inventory timeout %s must remain below required gate %s", inventoryMetricsHTTPClient.Timeout, requiredMetricsHTTPClient.Timeout)
	}
}

func TestTestOpenAICompatibleService(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/models":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"data":[{"id":"test-model"}]}`))
		case "/v1/chat/completions":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"ok"}}]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	result, err := testOpenAICompatibleService(server.URL, "vllm")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Model != "test-model" {
		t.Fatalf("expected model test-model, got %q", result.Model)
	}
	if !result.OK || result.Response != "ok" {
		t.Fatalf("expected exact semantic success, got %#v", result)
	}
}

func TestOpenAICompatibleProbeShapesRequestsByBackend(t *testing.T) {
	tests := []struct {
		serviceType       string
		wantMaxTokens     float64
		wantReasoningMode bool
	}{
		{serviceType: "sglang", wantMaxTokens: 128, wantReasoningMode: true},
		{serviceType: "vllm", wantMaxTokens: 128},
		{serviceType: "llamacpp", wantMaxTokens: 128},
	}

	for _, tt := range tests {
		t.Run(tt.serviceType, func(t *testing.T) {
			var requestBody map[string]interface{}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/v1/models":
					_, _ = w.Write([]byte(`{"data":[{"id":"test-model"}]}`))
				case "/v1/chat/completions":
					if err := json.NewDecoder(r.Body).Decode(&requestBody); err != nil {
						http.Error(w, err.Error(), http.StatusBadRequest)
						return
					}
					_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"ok"}}]}`))
				default:
					http.NotFound(w, r)
				}
			}))
			defer server.Close()

			if _, err := testOpenAICompatibleService(server.URL, tt.serviceType); err != nil {
				t.Fatal(err)
			}
			if got := requestBody["max_tokens"]; got != tt.wantMaxTokens {
				t.Fatalf("max_tokens = %#v, want %v", got, tt.wantMaxTokens)
			}
			reasoningEffort, present := requestBody["reasoning_effort"]
			if present != tt.wantReasoningMode {
				t.Fatalf("reasoning_effort presence = %v, want %v (body %#v)", present, tt.wantReasoningMode, requestBody)
			}
			if present && reasoningEffort != "low" {
				t.Fatalf("reasoning_effort = %#v, want low", reasoningEffort)
			}
		})
	}
}

func TestOpenAICompatibleServiceUsesFinalContentNotReasoning(t *testing.T) {
	tests := []struct {
		name         string
		finalContent string
		wantError    bool
	}{
		{name: "exact final ok", finalContent: "ok"},
		{name: "reasoning ok cannot mask wrong final", finalContent: "not ok", wantError: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/v1/models" {
					_, _ = w.Write([]byte(`{"data":[{"id":"glm-test"}]}`))
					return
				}
				_, _ = w.Write([]byte(`{"choices":[{"message":{"reasoning_content":"ok","content":` + strconv.Quote(tt.finalContent) + `}}]}`))
			}))
			defer server.Close()

			result, err := testOpenAICompatibleService(server.URL, "sglang")
			if tt.wantError {
				if err == nil {
					t.Fatalf("non-exact final content passed: %#v", result)
				}
				return
			}
			if err != nil || result.Response != "ok" {
				t.Fatalf("reasoning plus exact final content failed: result=%#v err=%v", result, err)
			}
		})
	}
}

func TestOpenAICompatibleServiceRejectsNonExactOK(t *testing.T) {
	for _, response := range []string{"okay", "ok.", "prefix ok", "ok suffix"} {
		t.Run(response, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/v1/models" {
					_, _ = w.Write([]byte(`{"data":[{"id":"test-model"}]}`))
					return
				}
				_, _ = w.Write([]byte(`{"choices":[{"message":{"content":` + strconv.Quote(response) + `}}]}`))
			}))
			defer server.Close()
			if _, err := testOpenAICompatibleService(server.URL, "sglang"); err == nil {
				t.Fatalf("non-exact response %q passed", response)
			}
		})
	}
}

func TestOpenAICompatibleServiceErrorsDoNotEchoAPIKey(t *testing.T) {
	const secret = "request-only-secret"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "credential "+secret+" rejected", http.StatusUnauthorized)
	}))
	defer server.Close()

	_, err := testOpenAICompatibleServiceWithAPIKey(server.URL, "sglang", secret)
	if err == nil {
		t.Fatal("expected authentication failure")
	}
	if strings.Contains(err.Error(), secret) {
		t.Fatalf("request-time API key leaked through error: %v", err)
	}
}

func TestTestComfyUIService(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/object_info/CheckpointLoaderSimple":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"CheckpointLoaderSimple":{"input":{"required":{"ckpt_name":[["flux.safetensors"]]}}}}`))
		case "/prompt":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"prompt_id":"prompt-123"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	result, err := testComfyUIService(server.URL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.PromptID != "prompt-123" {
		t.Fatalf("expected prompt-123, got %q", result.PromptID)
	}
	if !strings.Contains(result.Message, "flux.safetensors") {
		t.Fatalf("expected checkpoint in message, got %q", result.Message)
	}
}

func TestContainerBaseURLRequiresPort(t *testing.T) {
	t.Parallel()

	_, err := containerBaseURL(Container{Name: "yokai-empty", Ports: map[string]string{}})
	if err == nil {
		t.Fatal("expected error when container has no ports")
	}
}

func TestContainerBaseURLUsesExplicitManagedServiceAddress(t *testing.T) {
	baseURL, err := containerBaseURL(Container{
		Name: "yokai-head", Ownership: OwnershipManaged, Ports: map[string]string{"8000": "8000"}, Labels: map[string]string{LabelServiceAddress: "100.96.0.20"},
	})
	if err != nil || baseURL != "http://100.96.0.20:8000" {
		t.Fatalf("unexpected explicit service URL %q: %v", baseURL, err)
	}
}

func TestContainerBaseURLIgnoresObservedServiceAddressLabel(t *testing.T) {
	baseURL, err := containerBaseURL(Container{
		Name: "external", Ownership: OwnershipObserved, Ports: map[string]string{"8000": "8000"}, Labels: map[string]string{LabelServiceAddress: "203.0.113.20"},
	})
	if err != nil || baseURL != "http://127.0.0.1:8000" {
		t.Fatalf("observed container redirected service probe to %q: %v", baseURL, err)
	}
}

func TestInferServiceKindDetectsSGLang(t *testing.T) {
	t.Parallel()

	if got := inferServiceKindFromImage("lmsysorg/sglang:latest"); got != "sglang" {
		t.Fatalf("expected sglang, got %q", got)
	}
}

func TestContainerServiceRequiredMetricsGate(t *testing.T) {
	const apiKey = "request-only-secret"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+apiKey {
			http.Error(w, "missing bearer token", http.StatusUnauthorized)
			return
		}
		switch r.URL.Path {
		case "/v1/models":
			_, _ = w.Write([]byte(`{"data":[{"id":"glm-test"}]}`))
		case "/v1/chat/completions":
			_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"ok"}}]}`))
		case "/metrics":
			_, _ = w.Write([]byte("sglang:num_running_reqs 0\n"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	port := strings.TrimPrefix(server.URL, "http://127.0.0.1:")
	result, err := testContainerServiceWithOptions(Container{
		Name: "yokai-glm", Image: "lmsysorg/sglang@sha256:test", Ports: map[string]string{"8000": port},
	}, true, apiKey)
	if err != nil {
		t.Fatalf("required metrics gate failed: %v", err)
	}
	if !result.MetricsReady {
		t.Fatalf("metrics gate did not mark result ready: %#v", result)
	}
}

func TestContainerServiceRequiredMetricsGateRejectsUnrecognizedBodies(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		body string
	}{
		{name: "empty"},
		{name: "html", body: "<html>ok</html>"},
		{name: "unrelated prometheus", body: "go_goroutines 12\n"},
		{name: "unknown sglang family", body: "sglang:unrelated_metric 1\n"},
		{name: "wrong native family", body: "vllm:num_requests_running 1\n"},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/v1/models":
					_, _ = w.Write([]byte(`{"data":[{"id":"glm-test"}]}`))
				case "/v1/chat/completions":
					_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"ok"}}]}`))
				case "/metrics":
					_, _ = w.Write([]byte(tt.body))
				default:
					http.NotFound(w, r)
				}
			}))
			defer server.Close()

			port := strings.TrimPrefix(server.URL, "http://127.0.0.1:")
			_, err := testContainerServiceWithOptions(Container{
				Name: "yokai-glm", Image: "lmsysorg/sglang@sha256:test", Ports: map[string]string{"8000": port},
			}, true, "")
			if err == nil || !strings.Contains(err.Error(), "no recognized sglang native metric family") {
				t.Fatalf("expected native-metric rejection, got %v", err)
			}
		})
	}
}

func TestContainerServiceRequiredMetricsGateAcceptsVLLMNativeFamily(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/models":
			_, _ = w.Write([]byte(`{"data":[{"id":"vllm-test"}]}`))
		case "/v1/chat/completions":
			_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"ok"}}]}`))
		case "/metrics":
			_, _ = w.Write([]byte("vllm:num_requests_running 0\n"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	port := strings.TrimPrefix(server.URL, "http://127.0.0.1:")
	result, err := testContainerServiceWithOptions(Container{
		Name: "yokai-vllm", Image: "vllm/vllm-openai@sha256:test", Ports: map[string]string{"8000": port},
	}, true, "")
	if err != nil || !result.MetricsReady {
		t.Fatalf("vLLM native family did not pass metrics gate: result=%#v err=%v", result, err)
	}
}
