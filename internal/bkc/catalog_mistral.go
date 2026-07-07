package bkc

import "strings"

func init() {
	mistralCommon := strings.Join([]string{
		"--tokenizer_mode mistral",
		"--config_format mistral",
		"--load_format mistral",
		"--enable-auto-tool-choice",
		"--tool-call-parser mistral",
	}, " ")

	register(
		// Ministral-3 14B Instruct.
		Config{
			ID:       "ministral-3-14b-instruct",
			Name:     "Ministral 3 14B Instruct",
			Workload: WorkloadVLLM,
			ModelID:  "mistralai/Ministral-3-14B-Instruct-2512",
			Image:    imageVLLMStable,
			Port:     "8000",
			ExtraArgs: mistralCommon + " --tensor-parallel-size 1",
			Volumes:         hfMountDefault,
			Runtime:         runtimeDefault,
			Description:     "Ministral 3 14B Instruct, single-GPU default.",
			Source:          "vllm-project/recipes Mistral/Ministral-3-Instruct.md",
			TargetDevices:   []string{DeviceRTX5090, DeviceL40S, DeviceA100_80, DeviceH100_80, DeviceH200, DeviceB200, DeviceRTXPRO6000, DeviceGB10},
			MinVRAMGBPerGPU: 32,
			MinGPUCount:     1,
			Quantization:    QuantBF16,
			Arch:            ArchHopper,
		},

		// Ministral-3 8B Reasoning.
		Config{
			ID:       "ministral-3-8b-reasoning",
			Name:     "Ministral 3 8B Reasoning",
			Workload: WorkloadVLLM,
			ModelID:  "mistralai/Ministral-3-8B-Reasoning-2512",
			Image:    imageVLLMStable,
			Port:     "8000",
			ExtraArgs: mistralCommon + " --reasoning-parser mistral --tensor-parallel-size 1",
			Volumes:         hfMountDefault,
			Runtime:         runtimeDefault,
			Description:     "Ministral 3 8B Reasoning — single-GPU default.",
			Source:          "vllm-project/recipes Mistral/Ministral-3-Reasoning.md",
			TargetDevices:   []string{DeviceRTX4090, DeviceRTX5090, DeviceL40S, DeviceA100_80, DeviceH100_80, DeviceH200, DeviceB200, DeviceRTXPRO6000, DeviceGB10},
			MinVRAMGBPerGPU: 20,
			MinGPUCount:     1,
			Quantization:    QuantBF16,
			Arch:            ArchHopper,
		},

		// Ministral-3 14B Reasoning.
		Config{
			ID:       "ministral-3-14b-reasoning",
			Name:     "Ministral 3 14B Reasoning",
			Workload: WorkloadVLLM,
			ModelID:  "mistralai/Ministral-3-14B-Reasoning-2512",
			Image:    imageVLLMStable,
			Port:     "8000",
			ExtraArgs: mistralCommon + " --reasoning-parser mistral --tensor-parallel-size 2",
			Volumes:         hfMountDefault,
			Runtime:         runtimeDefault,
			Description:     "Ministral 3 14B Reasoning with 2-way TP for low-latency reasoning.",
			Source:          "vllm-project/recipes Mistral/Ministral-3-Reasoning.md",
			TargetDevices:   []string{DeviceA100_80, DeviceH100_80, DeviceH200, DeviceB200, DeviceRTXPRO6000},
			MinVRAMGBPerGPU: 32,
			MinGPUCount:     2,
			Quantization:    QuantBF16,
			Arch:            ArchHopper,
		},

		// Mistral Large 3 675B BF16.
		Config{
			ID:       "mistral-large-3-675b",
			Name:     "Mistral Large 3 675B Instruct",
			Workload: WorkloadVLLM,
			ModelID:  "mistralai/Mistral-Large-3-675B-Instruct-2512",
			Image:    imageVLLMStable,
			Port:     "8000",
			ExtraArgs: mistralCommon + " --tensor-parallel-size 8",
			Volumes:         hfMountDefault,
			Runtime:         runtimeDefault,
			Description:     "Mistral Large 3 BF16 on 8x H200/B200.",
			Source:          "vllm-project/recipes Mistral/Mistral-Large-3.md",
			TargetDevices:   []string{DeviceH200, DeviceB200},
			MinVRAMGBPerGPU: 141,
			MinGPUCount:     8,
			Quantization:    QuantBF16,
			Arch:            ArchHopper,
		},

		// Mistral Large 3 675B NVFP4 (Blackwell).
		Config{
			ID:       "mistral-large-3-675b-nvfp4",
			Name:     "Mistral Large 3 675B NVFP4",
			Workload: WorkloadVLLM,
			ModelID:  "mistralai/Mistral-Large-3-675B-Instruct-2512-NVFP4",
			Image:    imageVLLMStable,
			Port:     "8000",
			ExtraArgs: mistralCommon + " --tensor-parallel-size 4",
			Volumes:         hfMountDefault,
			Runtime:         runtimeDefault,
			Description:     "NVFP4 Mistral Large 3 on 4x B200.",
			Source:          "vllm-project/recipes Mistral/Mistral-Large-3.md",
			TargetDevices:   []string{DeviceB200},
			MinVRAMGBPerGPU: 180,
			MinGPUCount:     4,
			Quantization:    QuantNVFP4,
			Arch:            ArchBlackwell,
		},
	)
}
