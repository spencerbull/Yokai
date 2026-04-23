package bkc

import "strings"

func init() {
	llamaCommonBlackwell := strings.Join([]string{
		"--kv-cache-dtype fp8",
		"--async-scheduling",
		"--no-enable-prefix-caching",
		"--max-num-batched-tokens 8192",
		"--compilation-config " + `'{"pass_config":{"fuse_allreduce_rms":true,"fuse_attn_quant":true,"eliminate_noops":true}}'`,
	}, " ")

	llamaCommonHopper := strings.Join([]string{
		"--kv-cache-dtype fp8",
		"--async-scheduling",
		"--no-enable-prefix-caching",
		"--max-num-batched-tokens 8192",
	}, " ")

	register(
		// Llama 3.3 70B FP8 — fits a single 80GB+ GPU, recommended by NVIDIA
		// for Hopper (H100/H200) and as a fallback on Blackwell.
		Config{
			ID:       "llama-3-3-70b-fp8-hopper",
			Name:     "Llama 3.3 70B Instruct FP8 (Hopper)",
			Workload: WorkloadVLLM,
			ModelID:  "nvidia/Llama-3.3-70B-Instruct-FP8",
			Image:    imageVLLMStable,
			Port:     "8000",
			ExtraArgs: strings.Join([]string{
				llamaCommonHopper,
				"--tensor-parallel-size 1",
			}, " "),
			Volumes:         hfMountDefault,
			Runtime:         runtimeDefault,
			Description:     "FP8 Llama 3.3 70B on a single Hopper GPU with async scheduling.",
			Source:          "vllm-project/recipes Llama/Llama3.3-70B.md",
			TargetDevices:   []string{DeviceH100_80, DeviceH100_94, DeviceH200, DeviceRTXPRO6000, DeviceB200},
			MinVRAMGBPerGPU: 80,
			MinGPUCount:     1,
			Quantization:    QuantFP8,
			Arch:            ArchHopper,
			Notes: []string{
				"FP8 weights ship with an ~70 GB footprint; requires a single 80 GB-class GPU.",
				"Add --tensor-parallel-size 2/4/8 on multi-GPU systems for lower per-user latency.",
			},
		},

		// Llama 3.3 70B FP4 — NVFP4 Blackwell-only variant fitting RTX PRO
		// 6000 (96 GB), DGX Spark GB10, and larger data-centre Blackwell GPUs.
		Config{
			ID:       "llama-3-3-70b-fp4-blackwell",
			Name:     "Llama 3.3 70B Instruct FP4 (Blackwell)",
			Workload: WorkloadVLLM,
			ModelID:  "nvidia/Llama-3.3-70B-Instruct-FP4",
			Image:    imageVLLMStable,
			Port:     "8000",
			ExtraArgs: strings.Join([]string{
				llamaCommonBlackwell,
				"--tensor-parallel-size 1",
			}, " "),
			Volumes:         hfMountDefault,
			Runtime:         runtimeDefault,
			Description:     "NVFP4 Llama 3.3 70B tuned for single-GPU Blackwell with all-reduce/RMS fusions enabled.",
			Source:          "vllm-project/recipes Llama/Llama3.3-70B.md",
			TargetDevices:   []string{DeviceB200, DeviceRTXPRO6000, DeviceGB10},
			MinVRAMGBPerGPU: 48,
			MinGPUCount:     1,
			Quantization:    QuantFP4,
			Arch:            ArchBlackwell,
			Notes: []string{
				"NVFP4 weights are ~38 GB and leave ample KV-cache budget on 96 GB+ Blackwell GPUs.",
				"Fits DGX Spark (GB10) unified memory; use NGC container image for aarch64 hosts.",
			},
		},

		// Base meta-llama Llama 3.3 70B — suggest the FP8 NVIDIA repo as the
		// operational variant unless the user switches explicitly.
		Config{
			ID:       "llama-3-3-70b-meta-suggested-fp8",
			Name:     "Llama 3.3 70B Instruct (default suggestion)",
			Workload: WorkloadVLLM,
			ModelID:  "meta-llama/Llama-3.3-70B-Instruct",
			Image:    imageVLLMStable,
			Port:     "8000",
			ExtraArgs: strings.Join([]string{
				"--tensor-parallel-size 2",
				"--async-scheduling",
				"--max-num-batched-tokens 8192",
			}, " "),
			Volumes:         hfMountDefault,
			Runtime:         runtimeDefault,
			Description:     "Base Meta checkpoint. FP8 or FP4 NVIDIA checkpoints are strongly recommended on single-GPU targets.",
			Source:          "vllm-project/recipes Llama/Llama3.3-70B.md",
			TargetDevices:   []string{DeviceH200, DeviceB200},
			MinVRAMGBPerGPU: 80,
			MinGPUCount:     2,
			Quantization:    QuantBF16,
			Arch:            ArchHopper,
			Notes: []string{
				"BF16 base weights are ~140 GB — requires TP>=2 on 80 GB GPUs.",
				"Consider switching to nvidia/Llama-3.3-70B-Instruct-FP8 for single-GPU deployments.",
			},
		},

		// Llama 4 Scout FP8.
		Config{
			ID:       "llama-4-scout-fp8-hopper",
			Name:     "Llama 4 Scout 17B/16E FP8 (Hopper)",
			Workload: WorkloadVLLM,
			ModelID:  "nvidia/Llama-4-Scout-17B-16E-Instruct-FP8",
			Image:    imageVLLMStable,
			Port:     "8000",
			ExtraArgs: strings.Join([]string{
				llamaCommonHopper,
				"--tensor-parallel-size 1",
			}, " "),
			Volumes:         hfMountDefault,
			Runtime:         runtimeDefault,
			Description:     "FP8 Llama 4 Scout for single-GPU Hopper deployments.",
			Source:          "vllm-project/recipes Llama/Llama4-Scout.md",
			TargetDevices:   []string{DeviceH100_94, DeviceH200, DeviceB200, DeviceRTXPRO6000},
			MinVRAMGBPerGPU: 80,
			MinGPUCount:     1,
			Quantization:    QuantFP8,
			Arch:            ArchHopper,
		},

		// Llama 4 Scout FP4.
		Config{
			ID:       "llama-4-scout-fp4-blackwell",
			Name:     "Llama 4 Scout 17B/16E FP4 (Blackwell)",
			Workload: WorkloadVLLM,
			ModelID:  "nvidia/Llama-4-Scout-17B-16E-Instruct-FP4",
			Image:    imageVLLMStable,
			Port:     "8000",
			ExtraArgs: strings.Join([]string{
				llamaCommonBlackwell,
				"--tensor-parallel-size 1",
			}, " "),
			Volumes:         hfMountDefault,
			Runtime:         runtimeDefault,
			Description:     "NVFP4 Llama 4 Scout, fits RTX PRO 6000 or DGX Spark GB10.",
			Source:          "vllm-project/recipes Llama/Llama4-Scout.md",
			TargetDevices:   []string{DeviceB200, DeviceRTXPRO6000, DeviceGB10},
			MinVRAMGBPerGPU: 48,
			MinGPUCount:     1,
			Quantization:    QuantFP4,
			Arch:            ArchBlackwell,
		},

		// Llama 3.1 8B Instruct — fits almost anything from RTX 4090 up.
		Config{
			ID:       "llama-3-1-8b-instruct",
			Name:     "Llama 3.1 8B Instruct",
			Workload: WorkloadVLLM,
			ModelID:  "meta-llama/Llama-3.1-8B-Instruct",
			Image:    imageVLLMStable,
			Port:     "8000",
			ExtraArgs: strings.Join([]string{
				"--tensor-parallel-size 1",
				"--max-model-len 131072",
				"--async-scheduling",
			}, " "),
			Volumes:         hfMountDefault,
			Runtime:         runtimeDefault,
			Description:     "General-purpose Llama 3.1 8B Instruct config suitable for any 24 GB+ NVIDIA GPU.",
			Source:          "vllm-project/recipes Llama/Llama3.1.md",
			TargetDevices:   []string{DeviceRTX4090, DeviceRTX5090, DeviceL40S, DeviceA100_80, DeviceH100_80, DeviceH200, DeviceB200, DeviceRTXPRO6000, DeviceGB10},
			MinVRAMGBPerGPU: 24,
			MinGPUCount:     1,
			Quantization:    QuantBF16,
			Arch:            ArchHopper,
		},
	)
}
