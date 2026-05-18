package bkc

import (
	"strings"
	"testing"
)

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

func TestLookupFindsNemotronNanoOmniNVFP4(t *testing.T) {
	t.Parallel()

	cfg, ok := Lookup(WorkloadVLLM, "nvidia/Nemotron-3-Nano-Omni-30B-A3B-Reasoning-NVFP4")
	if !ok {
		t.Fatal("expected matching BKC")
	}
	if cfg.Image != imageVLLM020 {
		t.Fatalf("expected vLLM 0.20.0 image, got %q", cfg.Image)
	}
	if cfg.Quantization != QuantNVFP4 {
		t.Fatalf("expected NVFP4 quantization, got %q", cfg.Quantization)
	}
	if cfg.Arch != ArchBlackwell {
		t.Fatalf("expected Blackwell arch, got %q", cfg.Arch)
	}
	for _, want := range []string{
		"--max-model-len 131072",
		"--max-num-seqs 384",
		"--video-pruning-rate 0.5",
		`--media-io-kwargs {"video":{"fps":2,"num_frames":256}}`,
		"--reasoning-parser nemotron_v3",
		"--tool-call-parser qwen3_coder",
		"--kv-cache-dtype fp8",
	} {
		if !strings.Contains(cfg.ExtraArgs, want) {
			t.Fatalf("expected %q in extra args, got %q", want, cfg.ExtraArgs)
		}
	}
}

func TestLookupRejectsWrongWorkload(t *testing.T) {
	t.Parallel()

	if _, ok := Lookup(WorkloadLlamaCpp, "nvidia/NVIDIA-Nemotron-3-Super-120B-A12B-NVFP4"); ok {
		t.Fatal("did not expect vLLM BKC to match llama.cpp workload")
	}
}

func TestLookupFindsNvidiaGemma4NVFP4(t *testing.T) {
	t.Parallel()

	cfg, ok := Lookup(WorkloadVLLM, "nvidia/Gemma-4-26B-A4B-NVFP4")
	if !ok {
		t.Fatal("expected matching BKC")
	}
	if cfg.Quantization != QuantNVFP4 {
		t.Fatalf("expected NVFP4 quantization, got %q", cfg.Quantization)
	}
	if cfg.Arch != ArchBlackwell {
		t.Fatalf("expected Blackwell arch, got %q", cfg.Arch)
	}
	if !strings.Contains(cfg.ExtraArgs, "--tool-call-parser gemma4") {
		t.Fatalf("expected Gemma 4 tool parser flag, got %q", cfg.ExtraArgs)
	}
	if !strings.Contains(cfg.ExtraArgs, "--tensor-parallel-size 1") {
		t.Fatalf("expected TP=1 flag, got %q", cfg.ExtraArgs)
	}
	if !strings.Contains(cfg.ExtraArgs, "--max-model-len 262144") {
		t.Fatalf("expected 256K context flag, got %q", cfg.ExtraArgs)
	}
	if !strings.Contains(cfg.ExtraArgs, "--max-num-seqs 30") {
		t.Fatalf("expected concurrency flag, got %q", cfg.ExtraArgs)
	}
	if !strings.Contains(cfg.ExtraArgs, "--limit-mm-per-prompt image=4,audio=0") {
		t.Fatalf("expected image multimodal limit flag, got %q", cfg.ExtraArgs)
	}
	if !strings.Contains(cfg.ExtraArgs, "--kv-cache-dtype fp8") {
		t.Fatalf("expected FP8 KV cache flag, got %q", cfg.ExtraArgs)
	}
}
