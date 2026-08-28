package bkc

import (
	"encoding/json"
	"net"
	"strings"
	"testing"
)

func TestGLM53FlashNVFP4DualGB10Recipe(t *testing.T) {
	cfg, ok := LookupID(GLM53FlashNVFP4DualGB10ID)
	if !ok {
		t.Fatal("pinned multi-device BKC not found")
	}
	if err := ValidateMultiDeviceRecipe(cfg); err != nil {
		t.Fatalf("validate pinned recipe: %v", err)
	}
	if cfg.MinGPUCount != 1 {
		t.Fatalf("MinGPUCount must remain a per-node gate, got %d", cfg.MinGPUCount)
	}
	if cfg.MultiDevice == nil || cfg.MultiDevice.WorldSize != 2 || cfg.MultiDevice.TensorParallelSize != 2 {
		t.Fatalf("unexpected multi-device metadata: %#v", cfg.MultiDevice)
	}
	if !equalRuntimePatches(cfg.MultiDevice.RuntimePatches, glm53FlashRuntimePatches) {
		t.Fatalf("ordered runtime patch provenance drifted: %#v", cfg.MultiDevice.RuntimePatches)
	}
	apiJSON, err := json.Marshal(cfg.MultiDevice)
	if err != nil {
		t.Fatal(err)
	}
	firstLabel := strings.Index(string(apiJSON), GLM53FlashRuntimePatchLabel)
	secondLabel := strings.Index(string(apiJSON), GLM53FlashDSAGB10TilePatchLabel)
	if firstLabel < 0 || secondLabel <= firstLabel || !strings.Contains(string(apiJSON), GLM53FlashRuntimePatchOldSHA) || !strings.Contains(string(apiJSON), GLM53FlashDSAGB10TilePatchNewSHA) {
		t.Fatalf("BKC API JSON omitted or reordered exact runtime patches: %s", apiJSON)
	}
	if cfg.Port != "8000" || cfg.MultiDevice.ServicePort != GLM53FlashNVFP4ServicePort {
		t.Fatalf("rank-0 endpoint port drifted: port=%q metadata=%d", cfg.Port, cfg.MultiDevice.ServicePort)
	}
	if strings.Contains(cfg.ExtraArgs, "--host") || strings.Contains(cfg.ExtraArgs, "--port") {
		t.Fatalf("service bind must be rendered from the request, not catalog args: %q", cfg.ExtraArgs)
	}
	for _, field := range append([]string{cfg.Name, cfg.Description}, cfg.Notes...) {
		for _, token := range strings.Fields(field) {
			if net.ParseIP(strings.Trim(token, "[],:;()")) != nil {
				t.Fatalf("catalog embeds host-specific address %q", token)
			}
		}
	}
	for _, required := range []string{
		"--dist-timeout 3600",
		"--watchdog-timeout 3600",
		"--log-level warning",
		"--enable-metrics",
	} {
		if !containsArgSequence(cfg.ExtraArgs, required) {
			t.Fatalf("recipe missing %q: %q", required, cfg.ExtraArgs)
		}
	}
	if strings.Contains(strings.ToLower(cfg.ExtraArgs), "mtp") {
		t.Fatalf("first canary must not enable MTP: %q", cfg.ExtraArgs)
	}
	if len(cfg.Env) != len(glm53FabricEnv) {
		t.Fatalf("unexpected fabric environment: %#v", cfg.Env)
	}
	for key, value := range glm53FabricEnv {
		if cfg.Env[key] != value {
			t.Fatalf("fabric environment %s drifted: %q", key, cfg.Env[key])
		}
	}
}

func TestPinnedRecipeValidationFailsClosedOnDrift(t *testing.T) {
	tests := map[string]func(*Config){
		"id":             func(cfg *Config) { cfg.ID = "other" },
		"workload":       func(cfg *Config) { cfg.Workload = WorkloadVLLM },
		"model":          func(cfg *Config) { cfg.ModelID = "other/model" },
		"image":          func(cfg *Config) { cfg.Image = "lmsysorg/sglang:latest" },
		"model revision": func(cfg *Config) { cfg.MultiDevice.ModelRevision = "other" },
		"patch missing":  func(cfg *Config) { cfg.MultiDevice.RuntimePatches = nil },
		"patch order": func(cfg *Config) {
			cfg.MultiDevice.RuntimePatches[0], cfg.MultiDevice.RuntimePatches[1] = cfg.MultiDevice.RuntimePatches[1], cfg.MultiDevice.RuntimePatches[0]
		},
		"second patch label":   func(cfg *Config) { cfg.MultiDevice.RuntimePatches[1].Label = "other" },
		"second original hash": func(cfg *Config) { cfg.MultiDevice.RuntimePatches[1].OriginalSHA256 = "bad" },
		"second patched hash":  func(cfg *Config) { cfg.MultiDevice.RuntimePatches[1].PatchedSHA256 = "bad" },
		"world size":           func(cfg *Config) { cfg.MultiDevice.WorldSize = 1 },
		"metadata tp size":     func(cfg *Config) { cfg.MultiDevice.TensorParallelSize = 1 },
		"gpus per node":        func(cfg *Config) { cfg.MultiDevice.GPUsPerNode = 2 },
		"capability":           func(cfg *Config) { cfg.MultiDevice.RequiredCapabilities[0] = "other" },
		"tp near match":        func(cfg *Config) { cfg.ExtraArgs = strings.Replace(cfg.ExtraArgs, "--tp-size 2", "--tp-size 20", 1) },
		"missing tp":           func(cfg *Config) { cfg.ExtraArgs = strings.Replace(cfg.ExtraArgs, " --tp-size 2", "", 1) },
		"duplicate tp":         func(cfg *Config) { cfg.ExtraArgs += " --tp-size=2" },
		"conflicting tp":       func(cfg *Config) { cfg.ExtraArgs += " --tp-size=4" },
		"missing nnodes":       func(cfg *Config) { cfg.ExtraArgs = strings.Replace(cfg.ExtraArgs, " --nnodes 2", "", 1) },
		"duplicate nnodes":     func(cfg *Config) { cfg.ExtraArgs += " --nnodes=2" },
		"conflicting nnodes":   func(cfg *Config) { cfg.ExtraArgs += " --nnodes=4" },
		"timeout near match": func(cfg *Config) {
			cfg.ExtraArgs = strings.Replace(cfg.ExtraArgs, "--dist-timeout 3600", "--dist-timeout 36000", 1)
		},
		"duplicate revision":   func(cfg *Config) { cfg.ExtraArgs += " --revision " + GLM53FlashNVFP4Revision },
		"conflicting revision": func(cfg *Config) { cfg.ExtraArgs += " --revision other" },
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			cfg, _ := LookupID(GLM53FlashNVFP4DualGB10ID)
			mutate(&cfg)
			if err := ValidateMultiDeviceRecipe(cfg); err == nil {
				t.Fatal("expected pinned recipe drift to fail closed")
			}
		})
	}
}

func TestOnlyPinnedGLMRecipeCarriesRuntimePatch(t *testing.T) {
	carriers := 0
	for _, cfg := range Catalog() {
		if cfg.MultiDevice == nil || len(cfg.MultiDevice.RuntimePatches) == 0 {
			continue
		}
		carriers++
		if cfg.ID != GLM53FlashNVFP4DualGB10ID || !equalRuntimePatches(cfg.MultiDevice.RuntimePatches, glm53FlashRuntimePatches) {
			t.Fatalf("unexpected runtime patch carrier: %s %#v", cfg.ID, cfg.MultiDevice.RuntimePatches)
		}
	}
	if carriers != 1 {
		t.Fatalf("expected exactly one runtime patch carrier, got %d", carriers)
	}
}

func TestLookupIDReturnsIndependentMultiDeviceMetadata(t *testing.T) {
	first, _ := LookupID(GLM53FlashNVFP4DualGB10ID)
	first.MultiDevice.ServicePort = 9999
	first.MultiDevice.Roles[0].Name = "mutated"
	first.MultiDevice.RuntimePatches[0].Label = "mutated"
	first.MultiDevice.RuntimePatches = append(first.MultiDevice.RuntimePatches, MultiDeviceRuntimePatch{Label: "extra"})
	first.Env["NCCL_NET"] = "Socket"
	second, _ := LookupID(GLM53FlashNVFP4DualGB10ID)
	if second.MultiDevice.ServicePort != GLM53FlashNVFP4ServicePort || second.MultiDevice.Roles[0].Name != MultiDeviceRoleHead {
		t.Fatalf("lookup leaked metadata mutation: %#v", second.MultiDevice)
	}
	if !equalRuntimePatches(second.MultiDevice.RuntimePatches, glm53FlashRuntimePatches) {
		t.Fatalf("lookup leaked runtime patch mutation: %#v", second.MultiDevice.RuntimePatches)
	}
	if second.Env["NCCL_NET"] != "IB" {
		t.Fatalf("lookup leaked environment mutation: %#v", second.Env)
	}
}

func TestLookupBestReturnsIndependentRuntimePatchMetadata(t *testing.T) {
	first, match, ok := LookupBest(WorkloadSGLang, "LibertAIDAI/GLM-5-3-Flash-NVFP4-build")
	if !ok || match != MatchSuggested || first.MultiDevice == nil || len(first.MultiDevice.RuntimePatches) != 2 {
		t.Fatalf("suggested lookup did not find pinned recipe: match=%q cfg=%#v", match, first)
	}
	first.MultiDevice.RuntimePatches[1].PatchedSHA256 = "mutated"
	second, _ := LookupID(GLM53FlashNVFP4DualGB10ID)
	if second.MultiDevice.RuntimePatches[1].PatchedSHA256 != GLM53FlashDSAGB10TilePatchNewSHA {
		t.Fatalf("LookupBest leaked runtime patch mutation: %#v", second.MultiDevice.RuntimePatches)
	}
}
