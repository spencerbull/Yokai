package bkc

import (
	"fmt"
	"net"
	"strings"
)

const (
	Qwen38FlashNextNVFP4DualGB10ID      = "qwen3-8-flash-next-nvfp4-dual-gb10"
	Qwen38FlashNextNVFP4Model           = "nvidia/Qwen3.8-Flash-Next-NVFP4"
	Qwen38FlashNextNVFP4Revision        = "fc694b54fb0174e0913e6adf86691ef85a4ead47"
	Qwen38FlashNextSourceRevision       = "d2f54b78c0d2f9d74ac61aa56200e3c40fac3f22"
	Qwen38FlashNextNVFP4ImageDigest     = "3b0e188ffceb3d07e09c3cb5215433a0020eacf02d7f882ed3a8bfd15454477e"
	Qwen38FlashNextNVFP4Image           = "vllm/vllm-openai@sha256:" + Qwen38FlashNextNVFP4ImageDigest
	Qwen38FlashNextRendezvousPort       = 50000
	Qwen38FlashNextServicePort          = 8888
	Qwen38FlashNextServedModel          = "qwen3.8-flash-next"
	Qwen38FlashNextHFCacheDirectory     = "models--nvidia--Qwen3.8-Flash-Next-NVFP4"
	Qwen38FlashNextContainerRoot        = "/models/yokai-qwen38-repository"
	Qwen38FlashNextContainerModel       = Qwen38FlashNextContainerRoot + "/snapshots/" + Qwen38FlashNextNVFP4Revision
	Qwen38FlashNextPatchCapability      = "container.runtime_patch.qwen38_flash_next.v1"
	Qwen38LaunchAuthorizationCapability = "deployments.qwen_launch_authorization.v1"
	MultiDeviceFabricCapability         = "deployments.fabric.v1"
	qwen38PatchRoot                     = "/usr/local/lib/python3.12/dist-packages/vllm"
	Qwen38PLEPatchLabel                 = "qwen38-ple-nvfp4-fp8-v1"
	Qwen38ModelOptPatchLabel            = "qwen38-modelopt-mxfp8-fp8-block-moe-v1"
	Qwen38MTPPatchLabel                 = "qwen38-mtp-draft-vocab-tp2-v1"
	Qwen38QSAOpsPatchLabel              = "qwen38-qsa-fp8-kv-ops-v1"
	Qwen38QSAImplPatchLabel             = "qwen38-qsa-fp8-kv-impl-v1"
	Qwen38ConfigPatchLabel              = "qwen38-mtp-config-alias-v1"
	Qwen38LegacyConfigPatchLabel        = "qwen38-mtp-legacy-config-alias-v1"
)

var qwen38RuntimePatches = []MultiDeviceRuntimePatch{
	{Label: Qwen38PLEPatchLabel, SourcePath: qwen38PatchRoot + "/models/qwen3_8_flash_next/nvidia/ple_layer.py", OriginalSHA256: "a71144c1d36e06f22a2da1b1ada900076597fe5e824a911e7ada86249a0993e7", PatchedSHA256: "fae9fd5242748e8cdb314445a25ad628a0ce335cf26f794623f8679497a65186"},
	{Label: Qwen38ModelOptPatchLabel, SourcePath: qwen38PatchRoot + "/model_executor/layers/quantization/modelopt.py", OriginalSHA256: "3f3ca743fd3c66d72be92b7544591a8632f1aa422b73bb50c83e8a3281196e7d", PatchedSHA256: "ed14a21a9331d78deddd78f3c953536c7b827dc35edbb1acabd0f0fe68c64802"},
	{Label: Qwen38MTPPatchLabel, SourcePath: qwen38PatchRoot + "/models/qwen3_8_flash_next/nvidia/mtp.py", OriginalSHA256: "7735cee47d0d1e4776bebd30d907e4a62160409ce4ef2d65611559f8d58af431", PatchedSHA256: "30e3bad24d151f50f318b3836e6a6c5c1979c1b6ef7264a9fef21bdc162e01a2"},
	{Label: Qwen38QSAOpsPatchLabel, SourcePath: qwen38PatchRoot + "/models/qwen3_8_flash_next/nvidia/ops/qsa.py", OriginalSHA256: "c4ffe3674cafa0ce2dabc39a39f0ddbb4b594bc358ad210ffce9d04383350c7f", PatchedSHA256: "0669d6334f58a624c89c15f3e46c90f28e59b0b913507101dec1c5765e3c3b12"},
	{Label: Qwen38QSAImplPatchLabel, SourcePath: qwen38PatchRoot + "/models/qwen3_8_flash_next/nvidia/qsa.py", OriginalSHA256: "748addc85efaa8f7df940d1245bc900192f92e1f17af8fa774625758600751cb", PatchedSHA256: "ee5de40742ad48a6064ea24b99a285ff69c47d57bbb170f57c4eef71567a1df3"},
	{Label: Qwen38ConfigPatchLabel, SourcePath: Qwen38FlashNextContainerModel + "/config.json", OriginalSHA256: "deef67a61f3311faf051b23dc4192f442c7fee4f9cd2f38cbcbe4da55c763a80", PatchedSHA256: "b3ff72b8164d31f902106ad8a54e06e41cf0a8136cc8b75d02781b8a6c30b322"},
	{Label: Qwen38LegacyConfigPatchLabel, SourcePath: Qwen38FlashNextContainerModel + "/hf_quant_config.json", OriginalSHA256: "331ad11d57c8bc374554579977198084e0d4d0933d5b3558f125f6f670aba0e8", PatchedSHA256: "dd8727422cafbb0257d11a7163442bda46421f6e67c78eb9acd58669cb6eb5f8"},
}

var qwen38CommonArgs = []string{
	"--served-model-name", Qwen38FlashNextServedModel,
	"--tensor-parallel-size", "2",
	"--gpu-memory-utilization", "0.835",
	"--max-num-seqs", "8",
	"--max-num-batched-tokens", "8192",
	"--max-model-len", "262144",
	"--kv-cache-dtype", "fp8",
	"--mamba-ssm-cache-dtype", "bfloat16",
	"--load-format", "safetensors",
	"--safetensors-load-strategy", "lazy",
	"--enable-chunked-prefill",
	"--reasoning-parser", "qwen3",
	"--enable-auto-tool-choice",
	"--tool-call-parser", "qwen3_coder",
	"--distributed-executor-backend", "mp",
	"--mm-encoder-tp-mode", "data",
	"--nnodes", "2",
	"--master-addr", MultiDeviceHeadAddrPlaceholder,
	"--master-port", "50000",
	"--enable-expert-parallel",
	"--all2all-backend", "allgather_reducescatter",
	"--speculative-config", `{"method":"mtp","num_speculative_tokens":3,"use_local_argmax_reduction":true}`,
	"--compilation-config", `{"mode":0,"cudagraph_mode":"FULL_DECODE_ONLY"}`,
	"--hf-overrides", `{"text_config":{"ple_embedding_dtype":"float8_e4m3fn"}}`,
	"--revision", Qwen38FlashNextNVFP4Revision,
}

var qwen38RoleArgs = map[string][]string{
	MultiDeviceRoleHead:   {"--node-rank", "0", "--host", "{SERVICE_ADDRESS}", "--port", "8888"},
	MultiDeviceRoleWorker: {"--node-rank", "1", "--headless"},
}

var qwen38StaticEnv = map[string]string{
	"NCCL_IB_DISABLE":      "0",
	"NCCL_IB_AUTO_DETECT":  "0",
	"NCCL_DEBUG":           "WARN",
	"HF_HUB_OFFLINE":       "1",
	"TRANSFORMERS_OFFLINE": "1",
	"HF_HOME":              "/root/.cache/huggingface",
}

func init() {
	requiredCapabilities := append([]string(nil), multiDeviceCapabilities...)
	requiredCapabilities = append(requiredCapabilities, MultiDeviceFabricCapability, Qwen38FlashNextPatchCapability, Qwen38LaunchAuthorizationCapability)
	register(Config{
		ID: Qwen38FlashNextNVFP4DualGB10ID, Name: "Qwen3.8 Flash Next NVFP4 (dual DGX Spark vLLM TP2+EP+MTP3)",
		Workload: WorkloadVLLM, ModelID: Qwen38FlashNextNVFP4Model, Image: Qwen38FlashNextNVFP4Image, Port: "8888",
		Env: cloneStringMap(qwen38StaticEnv), Runtime: runtimeDefault,
		Description: "Pinned two-node vLLM deployment for one GB10 GPU per DGX Spark with TP2, expert parallelism, and three-token MTP speculation.",
		Source:      "https://github.com/MiaAI-Lab/Qwen3.8-Flash-Next-Dual-DGX-Sparks/tree/" + Qwen38FlashNextSourceRevision,
		Notes: []string{
			"Requires the exact standard Hugging Face cache snapshot revision pre-staged at the same absolute path on both nodes.",
			"Requires explicit per-node fabric IP, interface, HCA, and GID index bindings; no host network identity is stored in the catalog.",
			"Launches rank 1 headless before rank 0 and requires native vLLM metrics plus an exact semantic API response before promotion.",
			"Applies only the source- and hash-pinned upstream PLE, MXFP8, MTP, and FP8-KV adaptations without a shell.",
		},
		TargetDevices: []string{DeviceGB10}, MinVRAMGBPerGPU: 100, MinGPUCount: 1, Quantization: QuantNVFP4, Arch: ArchBlackwell,
		MultiDevice: &MultiDeviceDeployment{
			WorldSize: 2, TensorParallelSize: 2, Backend: MultiDeviceBackendVLLM, GPUsPerNode: 1,
			RendezvousPort: Qwen38FlashNextRendezvousPort, ServicePort: Qwen38FlashNextServicePort,
			ModelRevision: Qwen38FlashNextNVFP4Revision, ServedModelName: Qwen38FlashNextServedModel, SourceRevision: Qwen38FlashNextSourceRevision,
			RequiresLocalModel: true, RequiresFabricConfig: true,
			CommonArgs: append([]string(nil), qwen38CommonArgs...), RoleArgs: cloneStringSliceMap(qwen38RoleArgs),
			LaunchOrder: []string{MultiDeviceRoleWorker, MultiDeviceRoleHead}, RuntimePatches: cloneRuntimePatches(qwen38RuntimePatches),
			Roles:                []MultiDeviceRole{{Name: MultiDeviceRoleHead, Rank: 0, API: true}, {Name: MultiDeviceRoleWorker, Rank: 1, API: false}},
			RequiredCapabilities: requiredCapabilities,
		},
	})
}

func Qwen38RuntimePatchSetLabel() string { return RuntimePatchSetLabel(qwen38RuntimePatches) }

func Qwen38RuntimePatches() []MultiDeviceRuntimePatch {
	return cloneRuntimePatches(qwen38RuntimePatches)
}

func validateQwen38FlashNextRecipe(cfg Config) error {
	md := cfg.MultiDevice
	expectedSource := "https://github.com/MiaAI-Lab/Qwen3.8-Flash-Next-Dual-DGX-Sparks/tree/" + Qwen38FlashNextSourceRevision
	if cfg.Workload != WorkloadVLLM || md.Backend != MultiDeviceBackendVLLM || cfg.ModelID != Qwen38FlashNextNVFP4Model || cfg.Image != Qwen38FlashNextNVFP4Image || cfg.Source != expectedSource || md.ModelRevision != Qwen38FlashNextNVFP4Revision || md.SourceRevision != Qwen38FlashNextSourceRevision {
		return fmt.Errorf("bkc %s immutable provenance mismatch", cfg.ID)
	}
	if !equalStrings(cfg.TargetDevices, []string{DeviceGB10}) || cfg.MinVRAMGBPerGPU != 100 || cfg.MinGPUCount != 1 || cfg.Quantization != QuantNVFP4 || cfg.Arch != ArchBlackwell {
		return fmt.Errorf("bkc %s hardware contract mismatch", cfg.ID)
	}
	if cfg.Port != "8888" || md.ServicePort != Qwen38FlashNextServicePort || md.RendezvousPort != Qwen38FlashNextRendezvousPort || md.ServedModelName != Qwen38FlashNextServedModel || !md.RequiresLocalModel || !md.RequiresFabricConfig {
		return fmt.Errorf("bkc %s topology or staging contract mismatch", cfg.ID)
	}
	if !equalRuntimePatches(md.RuntimePatches, qwen38RuntimePatches) || !equalStrings(md.CommonArgs, qwen38CommonArgs) || !equalStringSliceMaps(md.RoleArgs, qwen38RoleArgs) || !equalStrings(md.LaunchOrder, []string{MultiDeviceRoleWorker, MultiDeviceRoleHead}) {
		return fmt.Errorf("bkc %s argv, launch order, or patch provenance mismatch", cfg.ID)
	}
	requiredCapabilities := append([]string(nil), multiDeviceCapabilities...)
	requiredCapabilities = append(requiredCapabilities, MultiDeviceFabricCapability, Qwen38FlashNextPatchCapability, Qwen38LaunchAuthorizationCapability)
	if !equalStrings(md.RequiredCapabilities, requiredCapabilities) || !equalStringMaps(cfg.Env, qwen38StaticEnv) || len(cfg.Volumes) != 0 || cfg.ExtraArgs != "" || cfg.Runtime.IPCMode != "host" {
		return fmt.Errorf("bkc %s capability or environment mismatch", cfg.ID)
	}
	for _, values := range [][]string{md.CommonArgs, md.RoleArgs[MultiDeviceRoleHead], md.RoleArgs[MultiDeviceRoleWorker]} {
		for _, value := range values {
			for _, token := range strings.Fields(value) {
				if net.ParseIP(strings.Trim(token, "[],:;()")) != nil {
					return fmt.Errorf("bkc %s embeds a host-specific address", cfg.ID)
				}
			}
		}
	}
	return nil
}

func equalStringSliceMaps(left, right map[string][]string) bool {
	if len(left) != len(right) {
		return false
	}
	for key, values := range left {
		if !equalStrings(values, right[key]) {
			return false
		}
	}
	return true
}

func equalStringMaps(left, right map[string]string) bool {
	if len(left) != len(right) {
		return false
	}
	for key, value := range left {
		if right[key] != value {
			return false
		}
	}
	return true
}
