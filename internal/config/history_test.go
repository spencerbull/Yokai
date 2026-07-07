package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestHistoryAddImage(t *testing.T) {
	t.Parallel()

	h := &History{}

	// Add first image
	h.AddImage("vllm/vllm-openai:latest")
	if len(h.Images) != 1 {
		t.Fatalf("expected 1 image, got %d", len(h.Images))
	}
	if h.Images[0] != "vllm/vllm-openai:latest" {
		t.Errorf("expected vllm/vllm-openai:latest, got %s", h.Images[0])
	}

	// Add second image — goes to front
	h.AddImage("ghcr.io/ggml-org/llama.cpp:server-cuda")
	if len(h.Images) != 2 {
		t.Fatalf("expected 2 images, got %d", len(h.Images))
	}
	if h.Images[0] != "ghcr.io/ggml-org/llama.cpp:server-cuda" {
		t.Errorf("expected llama.cpp at front, got %s", h.Images[0])
	}

	// Re-add first image — should move to front, no duplicates
	h.AddImage("vllm/vllm-openai:latest")
	if len(h.Images) != 2 {
		t.Fatalf("expected 2 images after dedup, got %d", len(h.Images))
	}
	if h.Images[0] != "vllm/vllm-openai:latest" {
		t.Errorf("expected vllm at front after re-add, got %s", h.Images[0])
	}

	// Empty string should be ignored
	h.AddImage("")
	if len(h.Images) != 2 {
		t.Errorf("expected 2 images after adding empty, got %d", len(h.Images))
	}
}

func TestHistoryAddModel(t *testing.T) {
	t.Parallel()

	h := &History{}

	h.AddModel("meta-llama/Llama-3.1-8B-Instruct")
	h.AddModel("mistralai/Mistral-7B-Instruct-v0.3")
	h.AddModel("meta-llama/Llama-3.1-8B-Instruct") // duplicate

	if len(h.Models) != 2 {
		t.Fatalf("expected 2 models, got %d", len(h.Models))
	}
	if h.Models[0] != "meta-llama/Llama-3.1-8B-Instruct" {
		t.Errorf("expected llama at front, got %s", h.Models[0])
	}
}

func TestHistoryMaxItems(t *testing.T) {
	t.Parallel()

	h := &History{}
	for i := 0; i < MaxHistoryItems+5; i++ {
		h.AddImage("image-" + string(rune('A'+i)))
	}

	if len(h.Images) != MaxHistoryItems {
		t.Errorf("expected %d images after cap, got %d", MaxHistoryItems, len(h.Images))
	}
}

func TestHistorySaveAndLoad(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tempDir)

	h := &History{}
	h.AddImage("vllm/vllm-openai:latest")
	h.AddImage("ghcr.io/ggml-org/llama.cpp:server-cuda")
	h.AddModel("meta-llama/Llama-3.1-8B-Instruct")

	if err := SaveHistory(h); err != nil {
		t.Fatalf("failed to save history: %v", err)
	}

	// Verify file exists
	path := filepath.Join(tempDir, ConfigDirName, HistoryFile)
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("history file not found: %v", err)
	}

	// Load it back
	loaded, err := LoadHistory()
	if err != nil {
		t.Fatalf("failed to load history: %v", err)
	}

	if len(loaded.Images) != 2 {
		t.Fatalf("expected 2 images, got %d", len(loaded.Images))
	}
	if loaded.Images[0] != "ghcr.io/ggml-org/llama.cpp:server-cuda" {
		t.Errorf("expected llama.cpp first, got %s", loaded.Images[0])
	}
	if loaded.Images[1] != "vllm/vllm-openai:latest" {
		t.Errorf("expected vllm second, got %s", loaded.Images[1])
	}

	if len(loaded.Models) != 1 {
		t.Fatalf("expected 1 model, got %d", len(loaded.Models))
	}
	if loaded.Models[0] != "meta-llama/Llama-3.1-8B-Instruct" {
		t.Errorf("expected llama model, got %s", loaded.Models[0])
	}
}

func TestHistoryLoadNonExistent(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tempDir)

	h, err := LoadHistory()
	if err != nil {
		t.Fatalf("expected no error for missing file, got: %v", err)
	}
	if len(h.Images) != 0 {
		t.Errorf("expected empty images, got %d", len(h.Images))
	}
	if len(h.Models) != 0 {
		t.Errorf("expected empty models, got %d", len(h.Models))
	}
}
