package bkc

import (
	"strings"
	"testing"
)

func TestLookupFindsQwen3CoderNextNVFP4ForRTXPro6000(t *testing.T) {
	t.Parallel()

	cfg, ok := LookupForDevice(WorkloadVLLM, "RedHatAI/Qwen3-Coder-Next-NVFP4", DeviceRTXPRO6000, 96, 1)
	if !ok {
		t.Fatal("expected RTX PRO 6000 Qwen3-Coder-Next BKC")
	}
	if cfg.ID != "qwen3-coder-next-80b-a3b-nvfp4-rtx-pro-6000" {
		t.Fatalf("expected RTX PRO 6000 BKC, got %q", cfg.ID)
	}
	if cfg.Image != "vllm/vllm-openai:v0.14.1-x86_64-cu130" {
		t.Fatalf("unexpected image %q", cfg.Image)
	}
	for _, want := range []string{
		"--max-model-len 262144",
		"--kv-cache-dtype fp8",
		"--gpu-memory-utilization 0.85",
		"--tensor-parallel-size 1",
		"--enable-auto-tool-choice",
		"--tool-call-parser qwen3_coder",
	} {
		if !strings.Contains(cfg.ExtraArgs, want) {
			t.Fatalf("expected %q in extra args, got %q", want, cfg.ExtraArgs)
		}
	}
}

func TestLookupFindsQwen3CoderNextNVFP4ForGB10(t *testing.T) {
	t.Parallel()

	cfg, ok := LookupForDevice(WorkloadVLLM, "RedHatAI/Qwen3-Coder-Next-NVFP4", DeviceGB10, 128, 1)
	if !ok {
		t.Fatal("expected GB10 Qwen3-Coder-Next BKC")
	}
	if cfg.ID != "qwen3-coder-next-80b-a3b-nvfp4-gb10" {
		t.Fatalf("expected GB10 BKC, got %q", cfg.ID)
	}
	if cfg.Image != "vllm/vllm-openai:v0.14.1-aarch64-cu130" {
		t.Fatalf("unexpected image %q", cfg.Image)
	}
	if !strings.Contains(cfg.ExtraArgs, "--gpu-memory-utilization 0.75") {
		t.Fatalf("expected GB10 memory utilization in extra args, got %q", cfg.ExtraArgs)
	}
}
