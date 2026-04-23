package bkc

import "strings"

func init() {
	register(
		Config{
			ID:       "phi-4-mini-instruct",
			Name:     "Phi-4 Mini Instruct",
			Workload: WorkloadVLLM,
			ModelID:  "microsoft/Phi-4-mini-instruct",
			Image:    imageVLLMStable,
			Port:     "8000",
			ExtraArgs: strings.Join([]string{
				"--host 0.0.0.0",
				"--max-model-len 4000",
			}, " "),
			Volumes:         hfMountDefault,
			Runtime:         runtimeDefault,
			Description:     "Compact Phi-4 Mini — single-GPU, runs anywhere.",
			Source:          "vllm-project/recipes Microsoft/Phi-4.md",
			TargetDevices:   []string{DeviceRTX4090, DeviceRTX5090, DeviceL40S, DeviceA100_80, DeviceH100_80, DeviceH200, DeviceB200, DeviceRTXPRO6000, DeviceGB10, DeviceJetsonThor},
			MinVRAMGBPerGPU: 8,
			MinGPUCount:     1,
			Quantization:    QuantBF16,
			Arch:            ArchHopper,
		},
	)
}
