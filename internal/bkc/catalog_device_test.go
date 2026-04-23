package bkc

import (
	"strings"
	"testing"
)

func TestCatalogIsPopulated(t *testing.T) {
	t.Parallel()
	all := Catalog()
	if len(all) < 40 {
		t.Fatalf("expected at least 40 BKC entries, got %d", len(all))
	}
}

func TestCatalogIDsAreUnique(t *testing.T) {
	t.Parallel()
	seen := make(map[string]struct{})
	for _, cfg := range Catalog() {
		if _, dup := seen[cfg.ID]; dup {
			t.Fatalf("duplicate BKC id: %s", cfg.ID)
		}
		seen[cfg.ID] = struct{}{}
	}
}

func TestCatalogRequiredFields(t *testing.T) {
	t.Parallel()
	for _, cfg := range Catalog() {
		if cfg.Workload == "" {
			t.Errorf("BKC %q is missing Workload", cfg.ID)
		}
		if cfg.ModelID == "" {
			t.Errorf("BKC %q is missing ModelID", cfg.ID)
		}
		if cfg.Image == "" {
			t.Errorf("BKC %q is missing Image", cfg.ID)
		}
		if cfg.Port == "" {
			t.Errorf("BKC %q is missing Port", cfg.ID)
		}
		if cfg.MinGPUCount < 0 {
			t.Errorf("BKC %q has negative MinGPUCount", cfg.ID)
		}
		if cfg.MinVRAMGBPerGPU < 0 {
			t.Errorf("BKC %q has negative MinVRAMGBPerGPU", cfg.ID)
		}
	}
}

func TestCatalogTargetDevicesAreKnown(t *testing.T) {
	t.Parallel()
	known := map[string]struct{}{
		DeviceGB10: {}, DeviceJetsonThor: {}, DeviceRTXPRO6000: {},
		DeviceRTX5090: {}, DeviceRTX4090: {}, DeviceL40S: {},
		DeviceA100_80: {}, DeviceH100_80: {}, DeviceH100_94: {},
		DeviceH200: {}, DeviceH20: {}, DeviceB200: {}, DeviceGB200: {},
		DeviceMI300X: {}, DeviceMI325X: {}, DeviceMI355X: {}, DeviceR9700: {},
	}
	for _, cfg := range Catalog() {
		for _, tag := range cfg.TargetDevices {
			if _, ok := known[tag]; !ok {
				t.Errorf("BKC %q references unknown device tag %q", cfg.ID, tag)
			}
		}
	}
}

func TestGB10HasCoverage(t *testing.T) {
	t.Parallel()
	count := 0
	for _, cfg := range Catalog() {
		for _, tag := range cfg.TargetDevices {
			if tag == DeviceGB10 {
				count++
				break
			}
		}
	}
	if count < 10 {
		t.Fatalf("expected at least 10 BKCs flagged for GB10, got %d", count)
	}
}

func TestRTXPRO6000HasCoverage(t *testing.T) {
	t.Parallel()
	count := 0
	for _, cfg := range Catalog() {
		for _, tag := range cfg.TargetDevices {
			if tag == DeviceRTXPRO6000 {
				count++
				break
			}
		}
	}
	if count < 10 {
		t.Fatalf("expected at least 10 BKCs flagged for RTX PRO 6000, got %d", count)
	}
}

func TestLookupAllReturnsLlamaVariants(t *testing.T) {
	t.Parallel()

	// meta base checkpoint resolves to exactly one entry (the 70B suggestion).
	if got := LookupAll(WorkloadVLLM, "meta-llama/Llama-3.3-70B-Instruct"); len(got) != 1 {
		t.Fatalf("expected 1 LookupAll result for meta Llama 3.3 70B, got %d", len(got))
	}

	// FP8 NVIDIA checkpoint resolves to exactly one entry.
	if got := LookupAll(WorkloadVLLM, "nvidia/Llama-3.3-70B-Instruct-FP8"); len(got) != 1 {
		t.Fatalf("expected 1 LookupAll result for FP8 Llama 3.3 70B, got %d", len(got))
	}
}

func TestLookupForDevicePicksGB10Variant(t *testing.T) {
	t.Parallel()

	cfg, ok := LookupForDevice(WorkloadVLLM, "nvidia/NVIDIA-Nemotron-3-Nano-30B-A3B-BF16", DeviceGB10, 0, 0)
	if !ok {
		t.Fatal("expected a GB10 BKC for Nemotron 3 Nano BF16")
	}
	if !strings.Contains(cfg.ID, "gb10") {
		t.Fatalf("expected GB10-specific config, got %q", cfg.ID)
	}
	if cfg.Image != imageVLLMNGC {
		t.Fatalf("expected NGC aarch64 image for GB10, got %q", cfg.Image)
	}
}

func TestLookupForDeviceFallsBackByVRAM(t *testing.T) {
	t.Parallel()

	// A 24 GB GPU can't run the 70B FP8 config (needs 80 GB), but it can run
	// the 70B FP4 config (48 GB min). LookupForDevice should pick the FP4 one.
	cfg, ok := LookupForDevice(WorkloadVLLM, "nvidia/Llama-3.3-70B-Instruct-FP4", "", 96, 1)
	if !ok {
		t.Fatal("expected a BKC match for FP4 Llama 3.3 70B")
	}
	if cfg.Quantization != QuantFP4 {
		t.Fatalf("expected FP4 config, got %q", cfg.Quantization)
	}
}

func TestLookupForDeviceFallsBackWhenNothingFits(t *testing.T) {
	t.Parallel()

	// 24 GB single GPU cannot satisfy any 70B FP8 config (80 GB min). We still
	// fall back to the first registered match so the UI can surface a warning.
	cfg, ok := LookupForDevice(WorkloadVLLM, "nvidia/Llama-3.3-70B-Instruct-FP8", "", 24, 1)
	if !ok {
		t.Fatal("expected fallback BKC match even when VRAM is insufficient")
	}
	if cfg.ID == "" {
		t.Fatal("expected non-empty fallback config")
	}
}

func TestCatalogReturnsIndependentCopy(t *testing.T) {
	t.Parallel()
	snapshot := Catalog()
	before := len(snapshot)
	snapshot = append(snapshot, Config{ID: "zzz-test-only"})
	if len(Catalog()) != before {
		t.Fatalf("Catalog() returned a slice that shares backing memory with internal state")
	}
}
