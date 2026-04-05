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
}

var catalog = []Config{
	{
		ID:       "nemotron-3-super-nvfp4",
		Name:     "Nemotron 3 Super NVFP4",
		Workload: WorkloadVLLM,
		ModelID:  "nvidia/NVIDIA-Nemotron-3-Super-120B-A12B-NVFP4",
		Image:    "vllm/vllm-openai:v0.17.0-cu130",
		Port:     "8888",
		ExtraArgs: strings.Join([]string{
			"--tokenizer nvidia/NVIDIA-Nemotron-3-Super-120B-A12B-NVFP4",
			"--served-model-name nemotron-3-super-nvfp4",
			"--tensor-parallel-size 1",
			"--gpu-memory-utilization 0.90",
			"--max-model-len 262144",
			"--max-num-seqs 8",
			"--max-num-batched-tokens 16384",
			"--kv-cache-dtype fp8",
			"--enable-prefix-caching",
			"--enable-chunked-prefill",
			"--trust-remote-code",
		}, " "),
		Env: map[string]string{
			"VLLM_ATTENTION_BACKEND":     "FLASHINFER",
			"VLLM_NVFP4_GEMM_BACKEND":    "marlin",
			"VLLM_TEST_FORCE_FP8_MARLIN": "1",
			"VLLM_MARLIN_USE_ATOMIC_ADD": "1",
			"VLLM_FP8_BACKEND":           "marlin",
			"VLLM_SCALED_MM_BACKEND":     "marlin",
		},
		Volumes: map[string]string{
			"/var/lib/yokai/huggingface": "/root/.cache/huggingface",
		},
		Plugins: []string{"vllm-reasoning-parser-super-v3"},
		Runtime: config.RuntimeOptions{
			IPCMode: "host",
			ShmSize: "16g",
			Ulimits: map[string]string{
				"memlock": "-1",
				"stack":   "67108864",
			},
		},
		Description: "Best known vLLM config derived from the local vllm-model-template for Nemotron Super.",
		Source:      "~/genai/vllm-model-template/.envrc",
		Notes: []string{
			"Prefills image, port, core vLLM flags, and required runtime env vars.",
			"Includes a persistent Hugging Face cache mount so weights are reused on the host.",
			"Adds the Super V3 reasoning parser plugin and matching runtime settings.",
		},
	},
}

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
