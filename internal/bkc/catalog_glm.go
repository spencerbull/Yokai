package bkc

import "strings"

func init() {
	glmCommon := strings.Join([]string{
		"--enable-auto-tool-choice",
	}, " ")

	register(
		// GLM-4.5-Air FP8.
		Config{
			ID:       "glm-4-5-air-fp8",
			Name:     "GLM-4.5-Air FP8",
			Workload: WorkloadVLLM,
			ModelID:  "zai-org/GLM-4.5-Air-FP8",
			Image:    imageVLLMStable,
			Port:     "8000",
			ExtraArgs: strings.Join([]string{
				"--tensor-parallel-size 8",
				"--tool-call-parser glm45",
				"--reasoning-parser glm45",
				glmCommon,
			}, " "),
			Volumes:         hfMountDefault,
			Runtime:         runtimeDefault,
			Description:     "GLM-4.5-Air FP8 on 8x Hopper/Blackwell.",
			Source:          "vllm-project/recipes GLM/GLM.md",
			TargetDevices:   []string{DeviceH100_80, DeviceH200, DeviceB200},
			MinVRAMGBPerGPU: 80,
			MinGPUCount:     8,
			Quantization:    QuantFP8,
			Arch:            ArchHopper,
		},

		// GLM-4.7 FP8 on 4x H200.
		Config{
			ID:       "glm-4-7-fp8",
			Name:     "GLM-4.7 FP8",
			Workload: WorkloadVLLM,
			ModelID:  "zai-org/GLM-4.7-FP8",
			Image:    imageVLLMStable,
			Port:     "8000",
			ExtraArgs: strings.Join([]string{
				"--tensor-parallel-size 4",
				"--speculative-config.method mtp",
				"--speculative-config.num_speculative_tokens 1",
				"--tool-call-parser glm47",
				"--reasoning-parser glm45",
				glmCommon,
			}, " "),
			Volumes:         hfMountDefault,
			Runtime:         runtimeDefault,
			Description:     "GLM-4.7 FP8 with MTP speculation on 4x H200/B200.",
			Source:          "vllm-project/recipes GLM/GLM.md",
			TargetDevices:   []string{DeviceH200, DeviceB200},
			MinVRAMGBPerGPU: 141,
			MinGPUCount:     4,
			Quantization:    QuantFP8,
			Arch:            ArchHopper,
		},

		// GLM-5.1 FP8 on 8x H200.
		Config{
			ID:       "glm-5-1-fp8",
			Name:     "GLM-5.1 FP8",
			Workload: WorkloadVLLM,
			ModelID:  "zai-org/GLM-5.1-FP8",
			Image:    imageVLLMStable,
			Port:     "8000",
			ExtraArgs: strings.Join([]string{
				"--tensor-parallel-size 8",
				"--speculative-config.method mtp",
				"--speculative-config.num_speculative_tokens 3",
				"--tool-call-parser glm47",
				"--reasoning-parser glm45",
				glmCommon,
				"--chat-template-content-format=string",
				"--served-model-name glm-5.1-fp8",
			}, " "),
			Volumes:         hfMountDefault,
			Runtime:         runtimeDefault,
			Description:     "GLM-5.1 FP8 on 8x H200/B200 with MTP speculation.",
			Source:          "vllm-project/recipes GLM/GLM5.md",
			TargetDevices:   []string{DeviceH200, DeviceB200},
			MinVRAMGBPerGPU: 141,
			MinGPUCount:     8,
			Quantization:    QuantFP8,
			Arch:            ArchHopper,
		},

		// GLM-4.5V FP8 on 4x H100.
		Config{
			ID:       "glm-4-5v-fp8",
			Name:     "GLM-4.5V FP8",
			Workload: WorkloadVLLM,
			ModelID:  "zai-org/GLM-4.5V-FP8",
			Image:    imageVLLMStable,
			Port:     "8000",
			ExtraArgs: strings.Join([]string{
				"--tensor-parallel-size 4",
				"--tool-call-parser glm45",
				"--reasoning-parser glm45",
				glmCommon,
				"--enable-expert-parallel",
				"--allowed-local-media-path /",
				"--mm-encoder-tp-mode data",
				"--mm-processor-cache-type shm",
			}, " "),
			Volumes:         hfMountDefault,
			Runtime:         runtimeDefault,
			Description:     "GLM-4.5V FP8 multimodal on 4x H100/H200/B200.",
			Source:          "vllm-project/recipes GLM/GLM-V.md",
			TargetDevices:   []string{DeviceH100_80, DeviceH200, DeviceB200},
			MinVRAMGBPerGPU: 80,
			MinGPUCount:     4,
			Quantization:    QuantFP8,
			Arch:            ArchHopper,
		},

		// GLM-OCR.
		Config{
			ID:       "glm-ocr",
			Name:     "GLM-OCR",
			Workload: WorkloadVLLM,
			ModelID:  "zai-org/GLM-OCR",
			Image:    imageVLLMLatest,
			Port:     "8000",
			ExtraArgs: strings.Join([]string{
				"--tensor-parallel-size 1",
			}, " "),
			Volumes:         hfMountDefault,
			Runtime:         runtimeDefault,
			Description:     "GLM-OCR single-GPU default — use a nightly vLLM build with transformers from source.",
			Source:          "vllm-project/recipes GLM/GLM-OCR.md",
			TargetDevices:   []string{DeviceRTX4090, DeviceRTX5090, DeviceL40S, DeviceA100_80, DeviceH100_80, DeviceH200, DeviceB200, DeviceRTXPRO6000, DeviceGB10},
			MinVRAMGBPerGPU: 16,
			MinGPUCount:     1,
			Quantization:    QuantBF16,
			Arch:            ArchHopper,
		},

		// GLM-ASR.
		Config{
			ID:       "glm-asr-nano-2512",
			Name:     "GLM-ASR Nano 2512",
			Workload: WorkloadVLLM,
			ModelID:  "zai-org/GLM-ASR-Nano-2512",
			Image:    imageVLLMLatest,
			Port:     "8000",
			ExtraArgs: "--tensor-parallel-size 1",
			Volumes:         hfMountDefault,
			Runtime:         runtimeDefault,
			Description:     "Compact GLM ASR — runs on any modern GPU.",
			Source:          "vllm-project/recipes GLM/GLM-ASR.md",
			TargetDevices:   []string{DeviceRTX4090, DeviceRTX5090, DeviceL40S, DeviceA100_80, DeviceH100_80, DeviceH200, DeviceB200, DeviceRTXPRO6000, DeviceGB10, DeviceJetsonThor},
			MinVRAMGBPerGPU: 8,
			MinGPUCount:     1,
			Quantization:    QuantBF16,
			Arch:            ArchHopper,
		},

		// Glyph.
		Config{
			ID:       "glyph",
			Name:     "Glyph",
			Workload: WorkloadVLLM,
			ModelID:  "zai-org/Glyph",
			Image:    imageVLLMStable,
			Port:     "8000",
			ExtraArgs: strings.Join([]string{
				"--no-enable-prefix-caching",
				"--mm-processor-cache-gb 0",
				"--reasoning-parser glm45",
				"--limit-mm-per-prompt.video 0",
			}, " "),
			Volumes:         hfMountDefault,
			Runtime:         runtimeDefault,
			Description:     "Glyph — single-GPU H100/Blackwell serving.",
			Source:          "vllm-project/recipes GLM/Glyph.md",
			TargetDevices:   []string{DeviceH100_80, DeviceH200, DeviceB200, DeviceRTXPRO6000, DeviceMI300X, DeviceMI325X},
			MinVRAMGBPerGPU: 40,
			MinGPUCount:     1,
			Quantization:    QuantBF16,
			Arch:            ArchHopper,
		},
	)
}
