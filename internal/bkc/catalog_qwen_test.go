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

func TestLookupFindsQwen38SGLangDSpark(t *testing.T) {
	t.Parallel()

	cfg, ok := Lookup(WorkloadSGLang, "RadixArk/Qwen3.8-27B-NVFP4")
	if !ok {
		t.Fatal("expected matching SGLang BKC")
	}
	if cfg.ID != "qwen3-8-27b-nvfp4-sglang-dspark" {
		t.Fatalf("unexpected BKC %q", cfg.ID)
	}
	if cfg.Port != "30000" || cfg.MinGPUCount != 1 || cfg.MinVRAMGBPerGPU != 90 {
		t.Fatalf("unexpected capacity metadata: %#v", cfg)
	}
	if cfg.Image != imageSGLangQwen38 {
		t.Fatalf("expected pinned SGLang image, got %q", cfg.Image)
	}
	for _, want := range []string{
		"sglang serve",
		"--revision 554ebba9b5f1b79dc11246341960360e6ef05ef4",
		"--context-length 262144",
		"--max-running-requests 3",
		"--kv-cache-dtype fp8_e4m3",
		"--speculative-algorithm DSPARK",
		"--speculative-draft-model-path RadixArk/Qwen3.8-27B-DSpark",
		"--speculative-draft-model-revision 85ef153be924f17ce4bf62726954eeaa4a73e854",
		"--enable-metrics",
	} {
		if !strings.Contains(cfg.ExtraArgs, want) {
			t.Fatalf("expected %q in extra args, got %q", want, cfg.ExtraArgs)
		}
	}
	if _, ok := Lookup(WorkloadVLLM, cfg.ModelID); ok {
		t.Fatal("did not expect SGLang BKC to match the vLLM workload")
	}
}

func TestLookupFindsQwen36TextNVFP4MTP(t *testing.T) {
	t.Parallel()

	cfg, ok := Lookup(WorkloadVLLM, "sakamakismile/Qwen3.6-27B-Text-NVFP4-MTP")
	if !ok {
		t.Fatal("expected matching BKC")
	}
	if cfg.ID != "qwen3-6-27b-text-nvfp4-mtp" {
		t.Fatalf("expected qwen3.6 Text NVFP4 MTP BKC, got %q", cfg.ID)
	}
	if cfg.Quantization != QuantNVFP4 {
		t.Fatalf("expected NVFP4 quantization, got %q", cfg.Quantization)
	}
	if cfg.Arch != ArchBlackwell {
		t.Fatalf("expected Blackwell arch, got %q", cfg.Arch)
	}
	for _, want := range []string{
		"--trust-remote-code",
		"--quantization modelopt",
		"--language-model-only",
		"--tensor-parallel-size 1",
		"--max-model-len 262144",
		"--max-num-seqs 2",
		"--kv-cache-dtype fp8",
		"--gpu-memory-utilization 0.90",
		"--reasoning-parser qwen3",
		"--speculative-config.method qwen3_5_mtp",
		"--speculative-config.num_speculative_tokens 3",
	} {
		if !strings.Contains(cfg.ExtraArgs, want) {
			t.Fatalf("expected %q in extra args, got %q", want, cfg.ExtraArgs)
		}
	}
	for _, want := range []string{DeviceGB10, DeviceRTXPRO6000, DeviceB200} {
		found := false
		for _, got := range cfg.TargetDevices {
			if got == want {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("expected target device %q in %#v", want, cfg.TargetDevices)
		}
	}
	for _, unwanted := range []string{
		"--speculative-config {",
		"--limit-mm-per-prompt",
	} {
		if strings.Contains(cfg.ExtraArgs, unwanted) {
			t.Fatalf("did not expect %q in extra args: %q", unwanted, cfg.ExtraArgs)
		}
	}
}
