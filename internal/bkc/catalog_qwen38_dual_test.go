package bkc

import (
	"net"
	"strings"
	"testing"
)

func TestQwen38FlashNextDualGB10Recipe(t *testing.T) {
	cfg, ok := LookupID(Qwen38FlashNextNVFP4DualGB10ID)
	if !ok {
		t.Fatal("pinned Qwen3.8 multi-device BKC not found")
	}
	if err := ValidateMultiDeviceRecipe(cfg); err != nil {
		t.Fatalf("validate pinned recipe: %v", err)
	}
	if cfg.Workload != WorkloadVLLM || cfg.Image != Qwen38FlashNextNVFP4Image || cfg.MultiDevice.ModelRevision != Qwen38FlashNextNVFP4Revision || cfg.MultiDevice.SourceRevision != Qwen38FlashNextSourceRevision {
		t.Fatalf("immutable provenance drifted: %#v", cfg)
	}
	if !equalStrings(cfg.MultiDevice.LaunchOrder, []string{MultiDeviceRoleWorker, MultiDeviceRoleHead}) || !equalStrings(cfg.MultiDevice.RoleArgs[MultiDeviceRoleWorker], []string{"--node-rank", "1", "--headless"}) {
		t.Fatalf("rank launch roles drifted: order=%#v args=%#v", cfg.MultiDevice.LaunchOrder, cfg.MultiDevice.RoleArgs)
	}
	for _, required := range []string{"--tensor-parallel-size", "2", "--enable-expert-parallel", "--speculative-config", `{"method":"mtp","num_speculative_tokens":3,"use_local_argmax_reduction":true}`, "--kv-cache-dtype", "fp8"} {
		found := false
		for _, arg := range cfg.MultiDevice.CommonArgs {
			if arg == required {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("missing exact structured argv element %q", required)
		}
	}
	if len(cfg.MultiDevice.RuntimePatches) != 7 || cfg.MultiDevice.RuntimePatches[0].Label != Qwen38PLEPatchLabel || cfg.MultiDevice.RuntimePatches[6].Label != Qwen38LegacyConfigPatchLabel {
		t.Fatalf("runtime patch provenance drifted: %#v", cfg.MultiDevice.RuntimePatches)
	}
	if !containsString(cfg.MultiDevice.RequiredCapabilities, Qwen38LaunchAuthorizationCapability) {
		t.Fatal("Qwen recipe does not require an authorization-capable agent")
	}
	for _, field := range append([]string{cfg.Name, cfg.Description, cfg.Source}, cfg.Notes...) {
		for _, token := range strings.Fields(field) {
			if net.ParseIP(strings.Trim(token, "[],:;()")) != nil {
				t.Fatalf("catalog embeds host-specific address %q", token)
			}
		}
	}
}

func containsString(values []string, expected string) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}

func TestQwen38RecipeValidationFailsClosedOnDrift(t *testing.T) {
	tests := map[string]func(*Config){
		"source revision": func(cfg *Config) { cfg.MultiDevice.SourceRevision = "main" },
		"source URL":      func(cfg *Config) { cfg.Source = "https://example.invalid" },
		"model revision":  func(cfg *Config) { cfg.MultiDevice.ModelRevision = "main" },
		"image digest":    func(cfg *Config) { cfg.Image = "vllm/vllm-openai:qwen38-flash-next" },
		"launch order":    func(cfg *Config) { cfg.MultiDevice.LaunchOrder = []string{MultiDeviceRoleHead, MultiDeviceRoleWorker} },
		"worker role":     func(cfg *Config) { cfg.MultiDevice.RoleArgs[MultiDeviceRoleWorker] = []string{"--node-rank", "1"} },
		"structured args": func(cfg *Config) { cfg.MultiDevice.CommonArgs[1] = "other" },
		"patch hash":      func(cfg *Config) { cfg.MultiDevice.RuntimePatches[3].PatchedSHA256 = "bad" },
		"fabric contract": func(cfg *Config) { cfg.MultiDevice.RequiresFabricConfig = false },
		"local snapshot":  func(cfg *Config) { cfg.MultiDevice.RequiresLocalModel = false },
		"host volume":     func(cfg *Config) { cfg.Volumes = map[string]string{"/host": "/container"} },
		"hardware":        func(cfg *Config) { cfg.TargetDevices = []string{DeviceH100_80} },
		"IPC":             func(cfg *Config) { cfg.Runtime.IPCMode = "private" },
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			cfg, _ := LookupID(Qwen38FlashNextNVFP4DualGB10ID)
			mutate(&cfg)
			if err := ValidateMultiDeviceRecipe(cfg); err == nil {
				t.Fatal("expected pinned recipe drift to fail closed")
			}
		})
	}
}
