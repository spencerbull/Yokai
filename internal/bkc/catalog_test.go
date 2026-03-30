package bkc

import "testing"

func TestLookupFindsNemotronSuperVLLM(t *testing.T) {
	t.Parallel()

	cfg, ok := Lookup(WorkloadVLLM, "nvidia/NVIDIA-Nemotron-3-Super-120B-A12B-NVFP4")
	if !ok {
		t.Fatal("expected matching BKC")
	}
	if cfg.Port != "8888" {
		t.Fatalf("expected port 8888, got %q", cfg.Port)
	}
	if cfg.Image != "vllm/vllm-openai:v0.17.0-cu130" {
		t.Fatalf("unexpected image %q", cfg.Image)
	}
	if cfg.Volumes["/var/lib/yokai/huggingface"] != "/root/.cache/huggingface" {
		t.Fatalf("expected huggingface cache mount, got %#v", cfg.Volumes)
	}
	if len(cfg.Plugins) != 1 || cfg.Plugins[0] != "vllm-reasoning-parser-super-v3" {
		t.Fatalf("expected nemotron parser plugin, got %#v", cfg.Plugins)
	}
	if cfg.Runtime.IPCMode != "host" || cfg.Runtime.ShmSize != "16g" {
		t.Fatalf("unexpected runtime settings %#v", cfg.Runtime)
	}
}

func TestLookupRejectsWrongWorkload(t *testing.T) {
	t.Parallel()

	if _, ok := Lookup(WorkloadLlamaCpp, "nvidia/NVIDIA-Nemotron-3-Super-120B-A12B-NVFP4"); ok {
		t.Fatal("did not expect vLLM BKC to match llama.cpp workload")
	}
}
