package views

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestResolveOpenAIModelIDsPrefersLiveServedModels(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"id":"nemotron-3-super-nvfp4"}]}`))
	}))
	defer server.Close()

	hostPort := strings.TrimPrefix(server.URL, "http://")
	parts := strings.Split(hostPort, ":")
	if len(parts) != 2 {
		t.Fatalf("unexpected server address %q", hostPort)
	}
	port := atoi(parts[1])

	models, reachable := resolveOpenAIModelIDs(&http.Client{Timeout: time.Second}, parts[0], port, "nvidia/NVIDIA-Nemotron-3-Super-120B-A12B-NVFP4")
	if !reachable {
		t.Fatal("expected endpoint to be reachable")
	}
	if len(models) != 1 || models[0] != "nemotron-3-super-nvfp4" {
		t.Fatalf("expected live served model id, got %#v", models)
	}
}

func TestResolveOpenAIModelIDsFallsBackToConfiguredModel(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[]}`))
	}))
	defer server.Close()

	hostPort := strings.TrimPrefix(server.URL, "http://")
	parts := strings.Split(hostPort, ":")
	port := atoi(parts[1])

	models, reachable := resolveOpenAIModelIDs(&http.Client{Timeout: time.Second}, parts[0], port, "fallback-model")
	if !reachable {
		t.Fatal("expected endpoint to be reachable")
	}
	if len(models) != 1 || models[0] != "fallback-model" {
		t.Fatalf("expected fallback model, got %#v", models)
	}
}

func TestResolveOpenAIModelIDsSkipsUnreachableEndpoint(t *testing.T) {
	t.Parallel()

	models, reachable := resolveOpenAIModelIDs(&http.Client{Timeout: time.Second}, "127.0.0.1", 1, "fallback-model")
	if reachable {
		t.Fatal("expected endpoint to be unreachable")
	}
	if len(models) != 0 {
		t.Fatalf("expected no models for unreachable endpoint, got %#v", models)
	}
}
