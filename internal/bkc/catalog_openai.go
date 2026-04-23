package bkc

import "strings"

func init() {
	gptOSSBlackwell := strings.Join([]string{
		"--kv-cache-dtype fp8",
		"--no-enable-prefix-caching",
		"--max-cudagraph-capture-size 2048",
		"--max-num-batched-tokens 8192",
		"--stream-interval 20",
	}, " ")

	gptOSSHopper := strings.Join([]string{
		"--no-enable-prefix-caching",
		"--max-cudagraph-capture-size 2048",
		"--max-num-batched-tokens 8192",
		"--stream-interval 20",
	}, " ")

	register(
		// GPT-OSS 20B — native MXFP4, ~11 GB weights, fits 24 GB GPUs and up.
		Config{
			ID:       "gpt-oss-20b",
			Name:     "GPT-OSS 20B (MXFP4)",
			Workload: WorkloadVLLM,
			ModelID:  "openai/gpt-oss-20b",
			Image:    imageVLLMLatest,
			Port:     "8000",
			ExtraArgs: strings.Join([]string{
				gptOSSHopper,
				"--tensor-parallel-size 1",
			}, " "),
			Volumes:         hfMountDefault,
			Runtime:         runtimeDefault,
			Description:     "GPT-OSS 20B native MXFP4 — single-GPU default for Hopper, Ada, and workstation Blackwell.",
			Source:          "vllm-project/recipes OpenAI/GPT-OSS.md",
			TargetDevices:   []string{DeviceA100_80, DeviceH100_80, DeviceH200, DeviceB200, DeviceRTXPRO6000, DeviceRTX5090, DeviceL40S, DeviceGB10},
			MinVRAMGBPerGPU: 24,
			MinGPUCount:     1,
			Quantization:    QuantMXFP4,
			Arch:            ArchHopper,
			Notes: []string{
				"Streams tokens every 20 iterations to maximise throughput.",
				"Disables prefix caching for consistent benchmark numbers — flip for production.",
			},
		},

		// GPT-OSS 20B Blackwell — same flags plus fp8 KV cache and FlashInfer MoE.
		Config{
			ID:       "gpt-oss-20b-blackwell",
			Name:     "GPT-OSS 20B (MXFP4, Blackwell)",
			Workload: WorkloadVLLM,
			ModelID:  "openai/gpt-oss-20b",
			Image:    imageVLLMLatest,
			Port:     "8000",
			ExtraArgs: strings.Join([]string{
				gptOSSBlackwell,
				"--tensor-parallel-size 1",
			}, " "),
			Env: map[string]string{
				"VLLM_USE_FLASHINFER_MOE_MXFP4_MXFP8": "1",
			},
			Volumes:         hfMountDefault,
			Runtime:         runtimeDefault,
			Description:     "GPT-OSS 20B tuned for Blackwell with FlashInfer MXFP4+MXFP8 MoE kernels.",
			Source:          "vllm-project/recipes OpenAI/GPT-OSS.md",
			TargetDevices:   []string{DeviceB200, DeviceRTXPRO6000, DeviceGB10},
			MinVRAMGBPerGPU: 24,
			MinGPUCount:     1,
			Quantization:    QuantMXFP4,
			Arch:            ArchBlackwell,
		},

		// GPT-OSS 120B — MXFP4, ~65 GB weights; fits a single 80 GB+ GPU.
		Config{
			ID:       "gpt-oss-120b",
			Name:     "GPT-OSS 120B (MXFP4)",
			Workload: WorkloadVLLM,
			ModelID:  "openai/gpt-oss-120b",
			Image:    imageVLLMLatest,
			Port:     "8000",
			ExtraArgs: strings.Join([]string{
				gptOSSHopper,
				"--tensor-parallel-size 1",
			}, " "),
			Volumes:         hfMountDefault,
			Runtime:         runtimeDefault,
			Description:     "GPT-OSS 120B single-GPU default — validated on H100/H200/B200/MI300X.",
			Source:          "vllm-project/recipes OpenAI/GPT-OSS.md",
			TargetDevices:   []string{DeviceH100_80, DeviceH200, DeviceB200, DeviceRTXPRO6000, DeviceA100_80},
			MinVRAMGBPerGPU: 80,
			MinGPUCount:     1,
			Quantization:    QuantMXFP4,
			Arch:            ArchHopper,
			Notes: []string{
				"On H100 TP1, raise --gpu-memory-utilization to 0.95 and drop batched tokens to 1024 to avoid OOM.",
				"Scale --tensor-parallel-size to 2/4/8 for lower per-user latencies.",
			},
		},

		// GPT-OSS 120B Blackwell — explicit Blackwell recipe with FlashInfer MoE.
		Config{
			ID:       "gpt-oss-120b-blackwell",
			Name:     "GPT-OSS 120B (MXFP4, Blackwell)",
			Workload: WorkloadVLLM,
			ModelID:  "openai/gpt-oss-120b",
			Image:    imageVLLMLatest,
			Port:     "8000",
			ExtraArgs: strings.Join([]string{
				gptOSSBlackwell,
				"--tensor-parallel-size 1",
			}, " "),
			Env: map[string]string{
				"VLLM_USE_FLASHINFER_MOE_MXFP4_MXFP8": "1",
			},
			Volumes:         hfMountDefault,
			Runtime:         runtimeDefault,
			Description:     "GPT-OSS 120B tuned for Blackwell with FlashInfer MoE and FP8 KV cache.",
			Source:          "vllm-project/recipes OpenAI/GPT-OSS.md",
			TargetDevices:   []string{DeviceB200, DeviceRTXPRO6000, DeviceGB10},
			MinVRAMGBPerGPU: 80,
			MinGPUCount:     1,
			Quantization:    QuantMXFP4,
			Arch:            ArchBlackwell,
			Notes: []string{
				"Requires compute capability 10.0 (B200, RTX PRO 6000 Blackwell, GB10).",
			},
		},

		// AMD MI355x variant of 120B (Quark MXFP4+FP8 activation checkpoint).
		Config{
			ID:       "gpt-oss-120b-amd-mi355x",
			Name:     "GPT-OSS 120B (AMD MI355X)",
			Workload: WorkloadVLLM,
			ModelID:  "amd/gpt-oss-120b-w-mxfp4-a-fp8",
			Image:    imageVLLMLatest,
			Port:     "8000",
			ExtraArgs: strings.Join([]string{
				"--tensor-parallel-size 8",
				"--attention-backend ROCM_AITER_UNIFIED_ATTN",
				"-cc.pass_config.fuse_rope_kvcache=True",
				"-cc.use_inductor_graph_partition=True",
				"--gpu-memory-utilization 0.95",
				"--block-size 64",
			}, " "),
			Env: map[string]string{
				"HSA_NO_SCRATCH_RECLAIM":              "1",
				"AMDGCN_USE_BUFFER_OPS":               "0",
				"VLLM_ROCM_USE_AITER":                 "1",
				"VLLM_ROCM_QUICK_REDUCE_QUANTIZATION": "INT4",
			},
			Volumes:         hfMountDefault,
			Runtime:         runtimeDefault,
			Description:     "Quark-quantised GPT-OSS 120B with FP8 activations for AMD MI355X (8x TP).",
			Source:          "vllm-project/recipes OpenAI/GPT-OSS.md",
			TargetDevices:   []string{DeviceMI355X},
			MinVRAMGBPerGPU: 128,
			MinGPUCount:     8,
			Quantization:    QuantMXFP4,
			Arch:            ArchCDNA4,
		},
	)
}
