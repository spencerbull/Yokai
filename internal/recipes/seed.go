package recipes

// DefaultRecipes is the shipped baseline of candidate recipes that seed the
// overlay store on first run (when no ~/.config/yokai/recipes.json exists yet).
//
// Deliberate trust-model note: every entry here is TierCandidate / proposed.
// Nothing in this seed is marked validated — promotion to validated remains
// exclusive to a passing live on-hardware readiness probe (verify), never an
// agent's or a seed's claim. Treat these as research findings to be inspected
// and verified against real topology before they ever drive a deploy.
func DefaultRecipes() []Recipe {
	cfg := []RecipeConfig{
		{
			Workload:        "vllm",
			ModelID:         "nvidia/DeepSeek-V4-Flash-NVFP4",
			Image:           "vllm/vllm-openai:v0.29.0-cu129@sha256:7ef5a35d1ef8ce2cf9d671dd91eec6e367c5849262e0362b4d3d4a26be0d87d2",
			TargetDevices:   []string{"b200"},
			MinVRAMGBPerGPU: 180,
			MinGPUCount:     2,
			Quantization:    "NVFP4",
		},
		{
			Workload:        "vllm",
			ModelID:         "deepseek-ai/DeepSeek-V4.1-Flash",
			Image:           "vllm/vllm-openai:deepseekv41-flash-0909-cu129@sha256:4a4431d6a283f7551c3d7a7d5efb905569328c665cb89512d7ba1280c4ca1957",
			TargetDevices:   []string{"h200", "b200"},
			MinVRAMGBPerGPU: 141,
			MinGPUCount:     8,
			Quantization:    "FP8",
		},
		{
			Workload:        "vllm",
			ModelID:         "deepseek-ai/DeepSeek-V4-Pro-0813",
			Image:           "vllm/vllm-openai:deepseekv4-cu129@sha256:c71f14ab0e86b5314521c465a229563c4cd4858d4d8f38e2b787113368df8722",
			TargetDevices:   []string{"b200"},
			MinVRAMGBPerGPU: 192,
			MinGPUCount:     8,
			Quantization:    "FP8",
		},
		{
			Workload:        "vllm",
			ModelID:         "Qwen/Qwen3.8-Flash-Next-FP8",
			Image:           "vllm/vllm-openai:qwen38-flash-next-x86_64-cu129@sha256:f16a96245c0add1a624972ed7a671857369adde6be6193583fca84db77714a48",
			TargetDevices:   []string{"h100-80", "h100-94", "h200", "b200"},
			MinVRAMGBPerGPU: 141,
			MinGPUCount:     2,
			Quantization:    "FP8",
		},
		{
			Workload:        "vllm",
			ModelID:         "nvidia/Qwen3.8-Flash-Next-NVFP4",
			Image:           "vllm/vllm-openai:qwen38-flash-next-x86_64-cu129@sha256:f16a96245c0add1a624972ed7a671857369adde6be6193583fca84db77714a48",
			TargetDevices:   []string{"b200"},
			MinVRAMGBPerGPU: 192,
			MinGPUCount:     1,
			Quantization:    "NVFP4",
		},
		{
			Workload:        "vllm",
			ModelID:         "baidu/Unlimited-OCR",
			Image:           "vllm/vllm-openai:unlimited-ocr-cu129@sha256:e45cf562cbc885af2531f576e79377069e0a6f0af42cdad7658b8494185a1143",
			TargetDevices:   []string{"rtx-4090", "rtx-5090", "l40s", "a100-80", "h100-80", "h200", "b200", "rtx-pro-6000", "gb10", "jetson-thor"},
			MinVRAMGBPerGPU: 16,
			MinGPUCount:     1,
			Quantization:    "BF16",
		},
	}

	provenance := []Provenance{
		{
			Agent:        "hermes",
			Source:       "Hugging Face — nvidia/DeepSeek-V4-Flash-NVFP4 (official NVIDIA Model Optimizer NVFP4 quant of deepseek-ai/DeepSeek-V4-Flash)",
			SourceURL:    "https://huggingface.co/nvidia/DeepSeek-V4-Flash-NVFP4",
			ResearchNote: "Verified NVFP4 exists: moe_quant_algo=NVFP4, routed experts NVFP4 group_size=16, remainder FP8 e4m3, producer=modelopt dsv4-nvfp4-experts. Base DeepSeek-V4-Flash (DeepseekV4ForCausalLM, MoE 284B total / 13B active, 1M ctx, MIT). Weights est ~163GB -> 2x B200 (192GB) TP2; NVFP4/Blackwell needs cu129. Image digest verified via Docker Hub API.",
			ReportedOn:   []string{"2026-09-10"},
		},
		{
			Agent:        "hermes",
			Source:       "Hugging Face — deepseek-ai/DeepSeek-V4.1-Flash (multimodal MoE, 552B backbone, 1M ctx, MIT)",
			SourceURL:    "https://huggingface.co/deepseek-ai/DeepSeek-V4.1-Flash",
			ResearchNote: "Brand new (HF 2026-09-10). Ships native FP8 (~510GB) in-repo; purpose-built official image deepseekv41-flash-0909-cu129, digest verified via Docker Hub API. 8x H200/B200 (141GB/GPU) footprint. Multimodal flash MoE.",
			ReportedOn:   []string{"2026-09-11"},
		},
		{
			Agent:        "hermes",
			Source:       "Hugging Face — deepseek-ai/DeepSeek-V4-Pro-0813 (flagship full-size V4-Pro refresh)",
			SourceURL:    "https://huggingface.co/deepseek-ai/DeepSeek-V4-Pro-0813",
			ResearchNote: "Flagship V4-Pro (refresh 08-13). 384 routed / 6 active experts, 71B layer, 1M ctx, ~893GB FP8. Complements the V4-Flash NVFP4 candidate (different model). deepseekv4-cu129 image digest verified via Docker Hub API. Needs 8x B200 (192GB).",
			ReportedOn:   []string{"2026-09-11"},
		},
		{
			Agent:        "hermes",
			Source:       "Hugging Face — Qwen/Qwen3.8-Flash-Next-FP8 (official Qwen FP8)",
			SourceURL:    "https://huggingface.co/Qwen/Qwen3.8-Flash-Next-FP8",
			ResearchNote: "Official Qwen FP8 (HF created 2026-08-24). Multimodal flash MoE (512 experts / 10 active), 1M ctx, built-in tools, qwen-community-1.0, ~185GB FP8. qwen38-flash-next-x86_64-cu129 image digest verified via Docker Hub API. 2x H100/H200/B200.",
			ReportedOn:   []string{"2026-09-11"},
		},
		{
			Agent:        "hermes",
			Source:       "Hugging Face — nvidia/Qwen3.8-Flash-Next-NVFP4 (NVIDIA Model Optimizer NVFP4)",
			SourceURL:    "https://huggingface.co/nvidia/Qwen3.8-Flash-Next-NVFP4",
			ResearchNote: "NVIDIA Model Optimizer NVFP4 of the same model (created 2026-09-02), Blackwell, ~132GB -> single B200. Pairs with the FP8 variant to cover H100/H200 (FP8) and B200 (NVFP4). Digest verified via Docker Hub API.",
			ReportedOn:   []string{"2026-09-11"},
		},
		{
			Agent:        "hermes",
			Source:       "Hugging Face — baidu/Unlimited-OCR (multilingual OCR VLM, ~3.3B, 32K ctx, MIT)",
			SourceURL:    "https://huggingface.co/baidu/Unlimited-OCR",
			ResearchNote: "Baidu multilingual OCR VLM (~3.3B, 32K ctx, MIT, 4.2k likes / 2.7M downloads). Official vLLM serving command documented in its README. Fills the OCR slot with a non-GLM/Paddle option. unlimited-ocr-cu129 image digest verified via Docker Hub API. Runs on consumer/edge cards incl. GB10/jetson-thor.",
			ReportedOn:   []string{"2026-09-11"},
		},
	}

	ids := []string{"rec_37c6eb9b", "rec_a8530eed", "rec_fcbc736f", "rec_7fb39e9c", "rec_0642f7a0", "rec_7039261d"}
	now := "2026-09-11T00:00:00Z"
	out := make([]Recipe, 0, len(cfg))
	for i, c := range cfg {
		out = append(out, Recipe{
			ID:          ids[i],
			Tier:        TierCandidate,
			Status:      StatusProposed,
			Config:      cfg[i],
			Provenance:  provenance[i],
			Fingerprint: Fingerprint(c),
			ProposedBy:  "hermes",
			CreatedAt:   now,
			UpdatedAt:   now,
		})
	}
	return out
}
