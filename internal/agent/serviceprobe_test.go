package agent

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

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
	if !strings.Contains(result.Message, "ok") {
		t.Fatalf("expected success message to include response text, got %q", result.Message)
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
