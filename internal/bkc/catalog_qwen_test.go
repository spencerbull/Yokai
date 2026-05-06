package bkc

import (
	"strings"
	"testing"
)

func TestLookupFindsQwen36NVFP4(t *testing.T) {
	t.Parallel()

	cfg, ok := Lookup(WorkloadVLLM, "sakamakismile/Qwen3.6-27B-NVFP4")
	if !ok {
		t.Fatal("expected matching BKC")
	}
	if cfg.ID != "qwen3-6-27b-nvfp4" {
		t.Fatalf("expected qwen3.6 NVFP4 BKC, got %q", cfg.ID)
	}
	if cfg.Name != "Qwen3.6-27B" {
		t.Fatalf("expected alias Qwen3.6-27B, got %q", cfg.Name)
	}
	if cfg.Image != imageVLLMCU130_019 {
		t.Fatalf("expected vLLM 0.19 CUDA 13 image, got %q", cfg.Image)
	}
	if cfg.Quantization != QuantNVFP4 {
		t.Fatalf("expected NVFP4 quantization, got %q", cfg.Quantization)
	}
	if cfg.Arch != ArchBlackwell {
		t.Fatalf("expected Blackwell arch, got %q", cfg.Arch)
	}
	for _, want := range []string{
		"--served-model-name Qwen3.6-27B",
		"--max-model-len 262144",
		"--max-num-seqs 2",
		"--kv-cache-dtype fp8",
		"--gpu-memory-utilization 0.90",
		"--limit-mm-per-prompt {\"image\":4,\"video\":0}",
		"--reasoning-parser qwen3",
		"--enable-auto-tool-choice",
		"--tool-call-parser qwen3_xml",
	} {
		if !strings.Contains(cfg.ExtraArgs, want) {
			t.Fatalf("expected %q in extra args, got %q", want, cfg.ExtraArgs)
		}
	}
	for _, unwanted := range []string{
		"--quantization modelopt",
		"--speculative-config",
	} {
		if strings.Contains(cfg.ExtraArgs, unwanted) {
			t.Fatalf("did not expect %q in extra args: %q", unwanted, cfg.ExtraArgs)
		}
	}
	if strings.Contains(cfg.ExtraArgs, "--language-model-only") {
		t.Fatalf("did not expect text-only flag in VLM config: %q", cfg.ExtraArgs)
	}
}
