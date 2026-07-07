package bkc

import "strings"

func init() {
	kimiCommon := strings.Join([]string{
		"--tool-call-parser kimi_k2",
		"--reasoning-parser kimi_k2",
		"--enable-auto-tool-choice",
		"--trust-remote-code",
	}, " ")

	register(
		// Kimi-K2-Instruct FP8 quantised on 16x H200 (2 nodes of 8) — we keep
		// the simpler 8x TP config as the single-node baseline.
		Config{
			ID:       "kimi-k2-instruct-tp8pp2",
			Name:     "Kimi-K2 Instruct FP8 (TP8 PP2)",
			Workload: WorkloadVLLM,
			ModelID:  "moonshotai/Kimi-K2-Instruct",
			Image:    imageVLLMStable,
			Port:     "8000",
			ExtraArgs: strings.Join([]string{
				"--trust-remote-code",
				"--tokenizer-mode auto",
				"--tensor-parallel-size 8",
				"--pipeline-parallel-size 2",
				"--dtype bfloat16",
				"--quantization fp8",
				"--max-model-len 2048",
				"--max-num-seqs 1",
				"--max-num-batched-tokens 1024",
				"--enable-chunked-prefill",
				"--disable-log-requests",
				"--kv-cache-dtype fp8",
				"-dcp 8",
			}, " "),
			Volumes:         hfMountDefault,
			Runtime:         runtimeDefault,
			Description:     "Kimi-K2 Instruct FP8 on 16x H200 across 2 nodes.",
			Source:          "vllm-project/recipes moonshotai/Kimi-K2.md",
			TargetDevices:   []string{DeviceH200, DeviceB200},
			MinVRAMGBPerGPU: 141,
			MinGPUCount:     16,
			Quantization:    QuantFP8,
			Arch:            ArchHopper,
		},

		// Kimi-K2.5 BF16 on 8x H200/B200.
		Config{
			ID:       "kimi-k2-5-tp8",
			Name:     "Kimi-K2.5",
			Workload: WorkloadVLLM,
			ModelID:  "moonshotai/Kimi-K2.5",
			Image:    imageVLLMStable,
			Port:     "8000",
			ExtraArgs: kimiCommon + " -tp 8 --mm-encoder-tp-mode data",
			Volumes:         hfMountDefault,
			Runtime:         runtimeDefault,
			Description:     "Kimi-K2.5 multimodal on 8x H200/B200.",
			Source:          "vllm-project/recipes moonshotai/Kimi-K2.5.md",
			TargetDevices:   []string{DeviceH200, DeviceB200},
			MinVRAMGBPerGPU: 141,
			MinGPUCount:     8,
			Quantization:    QuantBF16,
			Arch:            ArchHopper,
		},

		// Kimi-K2.5 NVFP4 (Blackwell).
		Config{
			ID:       "kimi-k2-5-nvfp4",
			Name:     "Kimi-K2.5 NVFP4",
			Workload: WorkloadVLLM,
			ModelID:  "nvidia/Kimi-K2.5-NVFP4",
			Image:    imageVLLMStable,
			Port:     "8000",
			ExtraArgs: strings.Join([]string{
				kimiCommon,
				"-tp 4",
				"--mm-encoder-tp-mode data",
				"--compilation_config.pass_config.fuse_allreduce_rms true",
			}, " "),
			Volumes:         hfMountDefault,
			Runtime:         runtimeDefault,
			Description:     "NVFP4 Kimi-K2.5 on 4x B200.",
			Source:          "vllm-project/recipes moonshotai/Kimi-K2.5.md",
			TargetDevices:   []string{DeviceB200},
			MinVRAMGBPerGPU: 180,
			MinGPUCount:     4,
			Quantization:    QuantNVFP4,
			Arch:            ArchBlackwell,
		},

		// Kimi-K2 Thinking BF16.
		Config{
			ID:       "kimi-k2-thinking-tp8",
			Name:     "Kimi-K2 Thinking",
			Workload: WorkloadVLLM,
			ModelID:  "moonshotai/Kimi-K2-Thinking",
			Image:    imageVLLMStable,
			Port:     "8000",
			ExtraArgs: kimiCommon + " --tensor-parallel-size 8",
			Volumes:         hfMountDefault,
			Runtime:         runtimeDefault,
			Description:     "Kimi-K2 Thinking — 8x H200/B200 with reasoning parser.",
			Source:          "vllm-project/recipes moonshotai/Kimi-K2-Think.md",
			TargetDevices:   []string{DeviceH200, DeviceB200},
			MinVRAMGBPerGPU: 141,
			MinGPUCount:     8,
			Quantization:    QuantBF16,
			Arch:            ArchHopper,
		},

		// Kimi-Linear 48B A3B Instruct (TP4).
		Config{
			ID:       "kimi-linear-48b-a3b-tp4",
			Name:     "Kimi-Linear 48B A3B Instruct (TP4)",
			Workload: WorkloadVLLM,
			ModelID:  "moonshotai/Kimi-Linear-48B-A3B-Instruct",
			Image:    imageVLLMStable,
			Port:     "8000",
			ExtraArgs: strings.Join([]string{
				"--tensor-parallel-size 4",
				"--max-model-len 1048576",
				"--trust-remote-code",
			}, " "),
			Volumes:         hfMountDefault,
			Runtime:         runtimeDefault,
			Description:     "Kimi-Linear 48B A3B on 4x Hopper/Blackwell with 1M context.",
			Source:          "vllm-project/recipes moonshotai/Kimi-Linear.md",
			TargetDevices:   []string{DeviceH100_94, DeviceH200, DeviceB200, DeviceRTXPRO6000},
			MinVRAMGBPerGPU: 80,
			MinGPUCount:     4,
			Quantization:    QuantBF16,
			Arch:            ArchHopper,
		},
	)
}
