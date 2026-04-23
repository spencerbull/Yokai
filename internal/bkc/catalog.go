package bkc

import (
	"regexp"
	"sort"
	"strings"

	"github.com/spencerbull/yokai/internal/config"
)

type Workload string

const (
	WorkloadVLLM     Workload = "vllm"
	WorkloadLlamaCpp Workload = "llamacpp"
)

// Device profile tags used on BKC entries. These identify the NVIDIA/AMD GPUs
// and integrated systems that the config has been validated against, so the
// deploy wizard can filter or hint which configs fit which hardware.
const (
	DeviceGB10        = "gb10"          // NVIDIA DGX Spark GB10 (aarch64, 128 GB unified)
	DeviceJetsonThor  = "jetson-thor"   // NVIDIA Jetson Thor (aarch64)
	DeviceRTXPRO6000  = "rtx-pro-6000"  // NVIDIA RTX PRO 6000 Blackwell, 96 GB
	DeviceRTX5090     = "rtx-5090"      // NVIDIA GeForce RTX 5090, 32 GB
	DeviceRTX4090     = "rtx-4090"      // NVIDIA GeForce RTX 4090, 24 GB
	DeviceL40S        = "l40s"          // NVIDIA L40S, 48 GB
	DeviceA100_80     = "a100-80"       // NVIDIA A100 SXM4/PCIe 80 GB
	DeviceH100_80     = "h100-80"       // NVIDIA H100 SXM5/PCIe 80 GB
	DeviceH100_94     = "h100-94"       // NVIDIA H100 NVL 94 GB
	DeviceH200        = "h200"          // NVIDIA H200, 141 GB
	DeviceH20         = "h20"           // NVIDIA H20, 141 GB (China-market Hopper)
	DeviceB200        = "b200"          // NVIDIA B200, 192 GB
	DeviceGB200       = "gb200"         // NVIDIA GB200 NVL72 node
	DeviceMI300X      = "mi300x"        // AMD Instinct MI300X, 192 GB
	DeviceMI325X      = "mi325x"        // AMD Instinct MI325X, 256 GB
	DeviceMI355X      = "mi355x"        // AMD Instinct MI355X
	DeviceR9700       = "radeon-r9700"  // AMD Radeon AI PRO R9700, 32 GB
)

// Quantization strings used consistently across the catalog.
const (
	QuantBF16   = "BF16"
	QuantFP16   = "FP16"
	QuantFP8    = "FP8"
	QuantFP4    = "FP4"
	QuantNVFP4  = "NVFP4"
	QuantMXFP4  = "MXFP4"
	QuantINT4   = "INT4"
)

// Architecture hints — used for display and future filtering.
const (
	ArchBlackwell = "blackwell"
	ArchHopper    = "hopper"
	ArchAda       = "ada"
	ArchAmpere    = "ampere"
	ArchCDNA3     = "cdna3"
	ArchCDNA4     = "cdna4"
	ArchRDNA4     = "rdna4"
)

type Config struct {
	ID          string
	Name        string
	Workload    Workload
	ModelID     string
	Image       string
	Port        string
	ExtraArgs   string
	Env         map[string]string
	Volumes     map[string]string
	Plugins     []string
	Runtime     config.RuntimeOptions
	Description string
	Source      string
	Notes       []string

	// Device-awareness fields.
	TargetDevices   []string // preferred/validated GPUs (see Device* constants)
	MinVRAMGBPerGPU float64  // minimum VRAM per GPU required to load + serve
	MinGPUCount     int      // minimum number of GPUs required (>=1)
	Quantization    string   // see Quant* constants
	Arch            string   // see Arch* constants (primary architecture this targets)
}

// Catalog returns a copy of the full BKC catalog. Safe to mutate the returned
// slice.
func Catalog() []Config {
	out := make([]Config, len(catalog))
	copy(out, catalog)
	return out
}

// Lookup returns the first BKC entry matching the given workload and model id,
// case-insensitive. When multiple entries target the same model (e.g. one per
// GPU class) this returns the entry that was registered first.
func Lookup(workload Workload, modelID string) (Config, bool) {
	modelID = strings.TrimSpace(modelID)
	for _, cfg := range catalog {
		if cfg.Workload != workload {
			continue
		}
		if strings.EqualFold(cfg.ModelID, modelID) {
			return cfg, true
		}
	}
	return Config{}, false
}

// LookupAll returns every BKC entry that matches the given workload and model
// id. Useful when a single model has multiple validated configs (e.g. FP4 for
// Blackwell vs FP8 for Hopper).
func LookupAll(workload Workload, modelID string) []Config {
	modelID = strings.TrimSpace(modelID)
	if modelID == "" {
		return nil
	}
	var out []Config
	for _, cfg := range catalog {
		if cfg.Workload != workload {
			continue
		}
		if strings.EqualFold(cfg.ModelID, modelID) {
			out = append(out, cfg)
		}
	}
	return out
}

// LookupForDevice returns the BKC that best fits the provided device profile,
// preferring configs whose TargetDevices include the profile, and otherwise
// the first config whose MinVRAMGBPerGPU/MinGPUCount the device can satisfy.
// deviceProfile may be empty, in which case this is equivalent to Lookup.
func LookupForDevice(workload Workload, modelID, deviceProfile string, vramGBPerGPU float64, gpuCount int) (Config, bool) {
	matches := LookupAll(workload, modelID)
	if len(matches) == 0 {
		return Config{}, false
	}

	deviceProfile = strings.ToLower(strings.TrimSpace(deviceProfile))

	// First, prefer an entry that explicitly lists the device profile. When
	// multiple entries list it, prefer the one with the fewest TargetDevices
	// — that's the most specialised (e.g. a GB10-only NGC aarch64 variant).
	if deviceProfile != "" {
		var best Config
		bestTagCount := -1
		found := false
		for _, cfg := range matches {
			for _, tag := range cfg.TargetDevices {
				if !strings.EqualFold(tag, deviceProfile) {
					continue
				}
				count := len(cfg.TargetDevices)
				if !found || count < bestTagCount {
					best = cfg
					bestTagCount = count
					found = true
				}
				break
			}
		}
		if found {
			return best, true
		}
	}

	// Next, filter by resource constraints.
	for _, cfg := range matches {
		minGPUs := cfg.MinGPUCount
		if minGPUs <= 0 {
			minGPUs = 1
		}
		if gpuCount > 0 && gpuCount < minGPUs {
			continue
		}
		if cfg.MinVRAMGBPerGPU > 0 && vramGBPerGPU > 0 && vramGBPerGPU < cfg.MinVRAMGBPerGPU {
			continue
		}
		return cfg, true
	}

	// Fall back to the first registered config for the model.
	return matches[0], true
}

type MatchType string

const (
	MatchExact     MatchType = "exact"
	MatchSuggested MatchType = "suggested"
)

func LookupBest(workload Workload, modelID string) (Config, MatchType, bool) {
	if cfg, ok := Lookup(workload, modelID); ok {
		return cfg, MatchExact, true
	}

	modelID = strings.TrimSpace(modelID)
	if modelID == "" {
		return Config{}, "", false
	}

	owner, tokens := normalizedModelParts(modelID)
	if owner == "" || len(tokens) == 0 {
		return Config{}, "", false
	}

	type scored struct {
		cfg   Config
		score int
	}
	var matches []scored
	for _, cfg := range catalog {
		if cfg.Workload != workload {
			continue
		}
		cfgOwner, cfgTokens := normalizedModelParts(cfg.ModelID)
		if cfgOwner == "" || cfgOwner != owner {
			continue
		}
		overlap := tokenOverlap(tokens, cfgTokens)
		if overlap < 3 {
			continue
		}
		score := overlap*10 - abs(len(tokens)-len(cfgTokens))
		matches = append(matches, scored{cfg: cfg, score: score})
	}

	if len(matches) == 0 {
		return Config{}, "", false
	}

	sort.Slice(matches, func(i, j int) bool {
		if matches[i].score != matches[j].score {
			return matches[i].score > matches[j].score
		}
		return matches[i].cfg.Name < matches[j].cfg.Name
	})

	return matches[0].cfg, MatchSuggested, true
}

var splitNonAlphaNum = regexp.MustCompile(`[^a-z0-9]+`)

func normalizedModelParts(modelID string) (string, []string) {
	modelID = strings.TrimSpace(strings.ToLower(modelID))
	if modelID == "" {
		return "", nil
	}
	owner := ""
	rest := modelID
	if slash := strings.Index(modelID, "/"); slash >= 0 {
		owner = modelID[:slash]
		rest = modelID[slash+1:]
	}
	parts := splitNonAlphaNum.Split(rest, -1)
	tokens := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		tokens = append(tokens, part)
	}
	return owner, tokens
}

func tokenOverlap(a, b []string) int {
	if len(a) == 0 || len(b) == 0 {
		return 0
	}
	set := make(map[string]struct{}, len(a))
	for _, token := range a {
		set[token] = struct{}{}
	}
	overlap := 0
	seen := make(map[string]struct{})
	for _, token := range b {
		if _, ok := set[token]; !ok {
			continue
		}
		if _, dup := seen[token]; dup {
			continue
		}
		seen[token] = struct{}{}
		overlap++
	}
	return overlap
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

// Common Docker image tags used by the BKC catalog.
const (
	imageVLLMStable    = "vllm/vllm-openai:v0.12.0"
	imageVLLMLatest    = "vllm/vllm-openai:latest"
	imageVLLMCU130     = "vllm/vllm-openai:v0.14.1-cu130"
	imageVLLMNGC       = "nvcr.io/nvidia/vllm:25.12.post1-py3"
	imageVLLMJetson    = "ghcr.io/nvidia-ai-iot/vllm:latest-jetson-thor"
	imageVLLMGemma4    = "vllm/vllm-openai:gemma4"
	imageVLLMGemma4Cu  = "vllm/vllm-openai:gemma4-cu130"
	imageVLLMROCmGemma = "vllm/vllm-openai-rocm:gemma4"
)

// hfMountDefault is the default Hugging Face cache mount shared by every
// vLLM recipe so weights are cached on the host between deployments.
var hfMountDefault = map[string]string{
	"/var/lib/yokai/huggingface": "/root/.cache/huggingface",
}

// runtimeDefault matches the ipc=host / shm=16g settings NVIDIA recommends for
// every multi-process vLLM worker.
var runtimeDefault = config.RuntimeOptions{
	IPCMode: "host",
	ShmSize: "16g",
	Ulimits: map[string]string{
		"memlock": "-1",
		"stack":   "67108864",
	},
}
