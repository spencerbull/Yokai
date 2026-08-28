package bkc

import (
	"fmt"
	"strings"
)

const (
	GLM53FlashNVFP4DualGB10ID         = "glm-5-3-flash-nvfp4-dual-gb10"
	GLM53FlashNVFP4Model              = "LibertAIDAI/GLM-5.3-Flash-NVFP4"
	GLM53FlashNVFP4Revision           = "aa28e1f54130286c95fee10d0705c74ce8743734"
	GLM53FlashNVFP4Image              = "lmsysorg/sglang@sha256:73f9294b78e38d8cc297bfed16daec8ac192b126a2d1fb9055e259a632c68f00"
	GLM53FlashNVFP4ImageDigest        = "73f9294b78e38d8cc297bfed16daec8ac192b126a2d1fb9055e259a632c68f00"
	MultiDeviceBackendTorch           = "torch_distributed"
	MultiDeviceRoleHead               = "head"
	MultiDeviceRoleWorker             = "worker"
	MultiDeviceRoleRankPlaceholder    = "{ROLE_RANK}"
	MultiDeviceHeadAddrPlaceholder    = "{HEAD_FABRIC_ADDRESS}"
	GLM53FlashNVFP4RendezvousPort     = 25000
	GLM53FlashNVFP4ServicePort        = 8000
	GLM53FlashNVFP4GuardSeconds       = 3600
	GLM53FlashRuntimePatchLabel       = "sglang-unbalanced-model-loading-timeout-3600-v1"
	GLM53FlashRuntimePatchPath        = "/sgl-workspace/sglang/python/sglang/srt/model_executor/model_runner_components/load_model_utils.py"
	GLM53FlashRuntimePatchOldLine     = "UNBALANCED_MODEL_LOADING_TIMEOUT_S = 480  # leave more time for post data processing"
	GLM53FlashRuntimePatchNewLine     = "UNBALANCED_MODEL_LOADING_TIMEOUT_S = 3600  # align asymmetric cold loads with Yokai guards"
	GLM53FlashRuntimePatchOldSHA      = "f0193bfaab96053919e3a260f9ccb10e2137ad108cfae03816c867628f611e1f"
	GLM53FlashRuntimePatchNewSHA      = "fc3ae35cce5f712fd3681dc4bcef05157df6d232ac991ce60b78dae492784ff5"
	GLM53FlashDSAGB10TilePatchLabel   = "sglang-dsa-gb10-tile-tp2-v1"
	GLM53FlashDSAGB10TilePatchPath    = "/sgl-workspace/sglang/python/sglang/kernels/ops/attention/dsa/tilelang_kernel.py"
	GLM53FlashDSAGB10TilePatchOldLine = "            num_heads, d_v, tail_dim, topk, sm_scale=sm_scale, return_lse=return_lse"
	GLM53FlashDSAGB10TilePatchNewLine = "            num_heads, d_v, tail_dim, topk, sm_scale=sm_scale, return_lse=return_lse, **({'block_I': 32, 'num_stages': 1, 'threads': 128} if tail_dim == 0 else {})"
	GLM53FlashDSAGB10TilePatchOldSHA  = "526988aa5fd8fa61529f2d3cf245d1061e30982d2bd0d6e64ea2e51e31f30f7f"
	GLM53FlashDSAGB10TilePatchNewSHA  = "3150ec691843c84bb5db9ed5bf763c4da03842fde4666489e107bf7a9fcddd7b"
	GLM53FlashTileSharedMemoryBytes   = 100352
)

var glm53FlashRuntimePatches = []MultiDeviceRuntimePatch{
	{
		Label:           GLM53FlashRuntimePatchLabel,
		SourcePath:      GLM53FlashRuntimePatchPath,
		OriginalLine:    GLM53FlashRuntimePatchOldLine,
		ReplacementLine: GLM53FlashRuntimePatchNewLine,
		OriginalSHA256:  GLM53FlashRuntimePatchOldSHA,
		PatchedSHA256:   GLM53FlashRuntimePatchNewSHA,
	},
	{
		Label:           GLM53FlashDSAGB10TilePatchLabel,
		SourcePath:      GLM53FlashDSAGB10TilePatchPath,
		OriginalLine:    GLM53FlashDSAGB10TilePatchOldLine,
		ReplacementLine: GLM53FlashDSAGB10TilePatchNewLine,
		OriginalSHA256:  GLM53FlashDSAGB10TilePatchOldSHA,
		PatchedSHA256:   GLM53FlashDSAGB10TilePatchNewSHA,
	},
}

// GLM53FlashRuntimePatchSetLabel derives the composite Docker label from the
// ordered patch definitions that the BKC actually carries.
func GLM53FlashRuntimePatchSetLabel() string {
	labels := make([]string, len(glm53FlashRuntimePatches))
	for index, patch := range glm53FlashRuntimePatches {
		labels[index] = patch.Label
	}
	return strings.Join(labels, "+")
}

var multiDeviceCapabilities = []string{
	"deployments.v1",
	"deployments.preflight.v1",
	"deployments.members.v1",
	"deployments.logs.tail.v1",
	"container.inventory.all",
	"container.labels",
	"container.network.host",
	"container.device_mounts",
	"container.cap_add",
}

var glm53FabricEnv = map[string]string{
	"GLOO_SOCKET_IFNAME":       "enP2p1s0f0np0",
	"NCCL_SOCKET_IFNAME":       "enP2p1s0f0np0",
	"NCCL_IB_HCA":              "roceP2p1s0f0",
	"NCCL_IB_GID_INDEX":        "3",
	"NCCL_IB_ADDR_FAMILY":      "AF_INET",
	"NCCL_IB_ROCE_VERSION_NUM": "2",
	"NCCL_IB_DISABLE":          "0",
	"NCCL_NET":                 "IB",
	"NCCL_CROSS_NIC":           "1",
	"NCCL_CUMEM_ENABLE":        "0",
	"NCCL_IGNORE_CPU_AFFINITY": "1",
	"NCCL_NVLS_ENABLE":         "0",
	"NCCL_DEBUG":               "INFO",
}

// ValidateMultiDeviceRecipe fails closed if a coordinated recipe is internally
// inconsistent. The pinned GLM recipe receives additional provenance and flag
// checks so catalog drift cannot mutate devices.
func ValidateMultiDeviceRecipe(cfg Config) error {
	md := cfg.MultiDevice
	if md == nil {
		return fmt.Errorf("bkc %s is not a multi-device recipe", cfg.ID)
	}
	if cfg.Workload != WorkloadSGLang || md.Backend != MultiDeviceBackendTorch {
		return fmt.Errorf("bkc %s requires SGLang torch distributed", cfg.ID)
	}
	// These dimensions are correctness-bearing for the GB10 DSA override: the
	// 32/1/128 tile fits the shared-memory budget only with TP=2.
	if md.WorldSize != 2 || md.TensorParallelSize != 2 || md.GPUsPerNode != 1 {
		return fmt.Errorf("bkc %s requires two one-GPU nodes with TP=2", cfg.ID)
	}
	if md.RendezvousPort < 1 || md.RendezvousPort > 65535 {
		return fmt.Errorf("bkc %s has invalid rendezvous port", cfg.ID)
	}
	if md.ServicePort < 1 || md.ServicePort > 65535 || cfg.Port != fmt.Sprintf("%d", md.ServicePort) {
		return fmt.Errorf("bkc %s has inconsistent service port", cfg.ID)
	}
	if len(md.Roles) != 2 || md.Roles[0] != (MultiDeviceRole{Name: MultiDeviceRoleHead, Rank: 0, API: true}) || md.Roles[1] != (MultiDeviceRole{Name: MultiDeviceRoleWorker, Rank: 1, API: false}) {
		return fmt.Errorf("bkc %s must define ordered head and worker roles", cfg.ID)
	}
	if cfg.ID != GLM53FlashNVFP4DualGB10ID {
		if len(md.RuntimePatches) != 0 {
			return fmt.Errorf("bkc %s must not declare the GLM runtime patches", cfg.ID)
		}
		return nil
	}
	if cfg.ModelID != GLM53FlashNVFP4Model || cfg.Image != GLM53FlashNVFP4Image || md.ModelRevision != GLM53FlashNVFP4Revision {
		return fmt.Errorf("bkc %s immutable provenance mismatch", cfg.ID)
	}
	if !equalRuntimePatches(md.RuntimePatches, glm53FlashRuntimePatches) {
		return fmt.Errorf("bkc %s ordered runtime patch provenance mismatch", cfg.ID)
	}
	if !equalStrings(md.RequiredCapabilities, multiDeviceCapabilities) {
		return fmt.Errorf("bkc %s required capabilities mismatch", cfg.ID)
	}
	if md.ServicePort != GLM53FlashNVFP4ServicePort {
		return fmt.Errorf("bkc %s must preserve the rank-0 API/metrics port %d", cfg.ID, GLM53FlashNVFP4ServicePort)
	}
	if strings.Contains(cfg.ExtraArgs, "--host") || strings.Contains(cfg.ExtraArgs, "--port") {
		return fmt.Errorf("bkc %s service bind must come from the explicit head binding", cfg.ID)
	}
	required := []string{
		"--trust-remote-code",
		"--tp-size 2",
		"--nnodes 2",
		"--node-rank " + MultiDeviceRoleRankPlaceholder,
		"--dist-init-addr " + MultiDeviceHeadAddrPlaceholder + ":25000",
		fmt.Sprintf("--dist-timeout %d", GLM53FlashNVFP4GuardSeconds),
		fmt.Sprintf("--watchdog-timeout %d", GLM53FlashNVFP4GuardSeconds),
		"--attention-backend dsa",
		"--dsa-prefill-backend tilelang",
		"--dsa-decode-backend tilelang",
		"--moe-runner-backend flashinfer_cutlass",
		"--kv-cache-dtype bfloat16",
		"--disable-shared-experts-fusion",
		"--reasoning-parser glm45",
		"--tool-call-parser glm47",
		"--mem-fraction-static 0.84",
		"--context-length 65536",
		"--max-running-requests 2",
		"--log-level warning",
		"--enable-metrics",
		"--revision " + GLM53FlashNVFP4Revision,
	}
	for _, flag := range required {
		if !containsArgSequence(cfg.ExtraArgs, flag) {
			return fmt.Errorf("bkc %s missing pinned flag %q", cfg.ID, flag)
		}
	}
	if revision, count := flagValueCount(cfg.ExtraArgs, "--revision"); count != 1 || revision != GLM53FlashNVFP4Revision {
		return fmt.Errorf("bkc %s must contain exactly one pinned model revision", cfg.ID)
	}
	if tpSize, count := flagValueCount(cfg.ExtraArgs, "--tp-size"); count != 1 || tpSize != "2" {
		return fmt.Errorf("bkc %s must contain exactly one TP=2 setting", cfg.ID)
	}
	if nodeCount, count := flagValueCount(cfg.ExtraArgs, "--nnodes"); count != 1 || nodeCount != "2" {
		return fmt.Errorf("bkc %s must contain exactly one nnodes=2 setting", cfg.ID)
	}
	if len(cfg.Env) != len(glm53FabricEnv) {
		return fmt.Errorf("bkc %s fabric environment mismatch", cfg.ID)
	}
	for key, value := range glm53FabricEnv {
		if cfg.Env[key] != value {
			return fmt.Errorf("bkc %s fabric environment %s mismatch", cfg.ID, key)
		}
	}
	lowerArgs := strings.ToLower(cfg.ExtraArgs)
	if strings.Contains(lowerArgs, "mtp") || strings.Contains(lowerArgs, "--speculative") {
		return fmt.Errorf("bkc %s must not enable MTP or speculative decoding", cfg.ID)
	}
	return nil
}

func equalStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func equalRuntimePatches(left, right []MultiDeviceRuntimePatch) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func flagValueCount(args, flag string) (string, int) {
	tokens := strings.Fields(args)
	value := ""
	count := 0
	for index := 0; index < len(tokens); index++ {
		token := tokens[index]
		switch {
		case token == flag:
			count++
			if index+1 < len(tokens) {
				value = tokens[index+1]
				index++
			}
		case strings.HasPrefix(token, flag+"="):
			count++
			value = strings.TrimPrefix(token, flag+"=")
		}
	}
	return value, count
}

func containsArgSequence(args, required string) bool {
	tokens := strings.Fields(args)
	want := strings.Fields(required)
	for start := 0; start+len(want) <= len(tokens); start++ {
		matched := true
		for offset := range want {
			if tokens[start+offset] != want[offset] {
				matched = false
				break
			}
		}
		if matched {
			return true
		}
	}
	return false
}
