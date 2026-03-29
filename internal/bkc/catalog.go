package bkc

import (
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
