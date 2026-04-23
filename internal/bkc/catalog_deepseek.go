package bkc

import "strings"

func init() {
	register(
		// DeepSeek-R1-0528 (V3 generation) FP8 on 8x H200.
		Config{
			ID:       "deepseek-r1-0528-tp8",
			Name:     "DeepSeek-R1 0528",
			Workload: WorkloadVLLM,
			ModelID:  "deepseek-ai/DeepSeek-R1-0528",
			Image:    imageVLLMStable,
			Port:     "8000",
			ExtraArgs: strings.Join([]string{
				"--trust-remote-code",
				"--tensor-parallel-size 8",
				"--enable-expert-parallel",
			}, " "),
			Volumes:         hfMountDefault,
			Runtime:         runtimeDefault,
			Description:     "DeepSeek-R1 0528 on 8x H200 (141 GB each) with expert parallel.",
			Source:          "vllm-project/recipes DeepSeek/DeepSeek-V3.md",
			TargetDevices:   []string{DeviceH200, DeviceB200},
			MinVRAMGBPerGPU: 141,
			MinGPUCount:     8,
			Quantization:    QuantFP8,
			Arch:            ArchHopper,
		},

		// DeepSeek-V3.1.
		Config{
			ID:       "deepseek-v3-1-tp8",
			Name:     "DeepSeek-V3.1",
			Workload: WorkloadVLLM,
			ModelID:  "deepseek-ai/DeepSeek-V3.1",
			Image:    imageVLLMStable,
			Port:     "8000",
			ExtraArgs: strings.Join([]string{
				"--enable-expert-parallel",
				"--tensor-parallel-size 8",
				"--served-model-name ds31",
			}, " "),
			Volumes:         hfMountDefault,
			Runtime:         runtimeDefault,
			Description:     "DeepSeek-V3.1 on 8x H200 / B200 with expert parallel.",
			Source:          "vllm-project/recipes DeepSeek/DeepSeek-V3_1.md",
			TargetDevices:   []string{DeviceH200, DeviceB200, DeviceH20},
			MinVRAMGBPerGPU: 141,
			MinGPUCount:     8,
			Quantization:    QuantFP8,
			Arch:            ArchHopper,
		},

		// DeepSeek-V3.2-Exp.
		Config{
			ID:       "deepseek-v3-2-exp-dp8",
			Name:     "DeepSeek-V3.2-Exp",
			Workload: WorkloadVLLM,
			ModelID:  "deepseek-ai/DeepSeek-V3.2-Exp",
			Image:    imageVLLMStable,
			Port:     "8000",
			ExtraArgs: strings.Join([]string{
				"-dp 8",
				"--enable-expert-parallel",
			}, " "),
			Volumes:         hfMountDefault,
			Runtime:         runtimeDefault,
			Description:     "DeepSeek-V3.2-Exp on 8x H200 with DP + expert parallel.",
			Source:          "vllm-project/recipes DeepSeek/DeepSeek-V3_2-Exp.md",
			TargetDevices:   []string{DeviceH200, DeviceB200},
			MinVRAMGBPerGPU: 141,
			MinGPUCount:     8,
			Quantization:    QuantFP8,
			Arch:            ArchHopper,
		},

		// DeepSeek-V3.2.
		Config{
			ID:       "deepseek-v3-2-dp8",
			Name:     "DeepSeek-V3.2",
			Workload: WorkloadVLLM,
			ModelID:  "deepseek-ai/DeepSeek-V3.2",
			Image:    imageVLLMStable,
			Port:     "8000",
			ExtraArgs: strings.Join([]string{
				"-dp 8",
				"--enable-expert-parallel",
			}, " "),
			Volumes:         hfMountDefault,
			Runtime:         runtimeDefault,
			Description:     "DeepSeek-V3.2 on 8x H200 with DP + expert parallel.",
			Source:          "vllm-project/recipes DeepSeek/DeepSeek-V3_2.md",
			TargetDevices:   []string{DeviceH200, DeviceB200},
			MinVRAMGBPerGPU: 141,
			MinGPUCount:     8,
			Quantization:    QuantFP8,
			Arch:            ArchHopper,
		},

		// DeepSeek-OCR (tiny vision-language model).
		Config{
			ID:       "deepseek-ocr",
			Name:     "DeepSeek-OCR",
			Workload: WorkloadVLLM,
			ModelID:  "deepseek-ai/DeepSeek-OCR",
			Image:    imageVLLMStable,
			Port:     "8000",
			ExtraArgs: strings.Join([]string{
				"--logits_processors vllm.model_executor.models.deepseek_ocr:NGramPerReqLogitsProcessor",
				"--no-enable-prefix-caching",
				"--mm-processor-cache-gb 0",
			}, " "),
			Volumes:         hfMountDefault,
			Runtime:         runtimeDefault,
			Description:     "DeepSeek OCR online serving config — single-GPU default.",
			Source:          "vllm-project/recipes DeepSeek/DeepSeek-OCR.md",
			TargetDevices:   []string{DeviceRTX4090, DeviceRTX5090, DeviceL40S, DeviceA100_80, DeviceH100_80, DeviceH200, DeviceB200, DeviceRTXPRO6000, DeviceGB10, DeviceJetsonThor},
			MinVRAMGBPerGPU: 16,
			MinGPUCount:     1,
			Quantization:    QuantBF16,
			Arch:            ArchHopper,
		},

		// DeepSeek-OCR-2.
		Config{
			ID:       "deepseek-ocr-2",
			Name:     "DeepSeek-OCR-2",
			Workload: WorkloadVLLM,
			ModelID:  "deepseek-ai/DeepSeek-OCR-2",
			Image:    imageVLLMStable,
			Port:     "8000",
			ExtraArgs: strings.Join([]string{
				"--logits_processors vllm.model_executor.models.deepseek_ocr:NGramPerReqLogitsProcessor",
				"--no-enable-prefix-caching",
				"--mm-processor-cache-gb 0",
			}, " "),
			Volumes:         hfMountDefault,
			Runtime:         runtimeDefault,
			Description:     "DeepSeek-OCR-2 online serving config.",
			Source:          "vllm-project/recipes DeepSeek/DeepSeek-OCR-2.md",
			TargetDevices:   []string{DeviceRTX4090, DeviceRTX5090, DeviceL40S, DeviceA100_80, DeviceH100_80, DeviceH200, DeviceB200, DeviceRTXPRO6000, DeviceGB10, DeviceJetsonThor},
			MinVRAMGBPerGPU: 16,
			MinGPUCount:     1,
			Quantization:    QuantBF16,
			Arch:            ArchHopper,
		},
	)
}
