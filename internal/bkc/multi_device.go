package bkc

import (
	"fmt"
	"strings"
)

const (
	GLM53FlashEXL3TensorFoldDualGB10ID       = "glm-5-3-flash-exl3-tensorfold-dual-gb10"
	GLM53FlashEXL3TensorFoldDualGB10V14ID    = "glm-5-3-flash-exl3-tensorfold-dual-gb10-v1-4"
	GLM53FlashEXL3TensorFoldModel            = "Mia-AiLab/GLM-5.3-Flash-EXL3-4bpw-TensorFold"
	GLM53FlashEXL3TensorFoldModelRevision    = "078455ffe6472f9a52fbc1139f58b9db2881b25c"
	GLM53FlashEXL3TensorFoldDrafter          = "incoai/GLM-5.3-Flash-DFlash2"
	GLM53FlashEXL3TensorFoldDrafterRevision  = "bf582e4eacc1810f76656d1811693ff6c6737d2a"
	GLM53FlashEXL3TensorFoldRecipeRepository = "https://github.com/MiaAI-Lab/GLM-5.3-Flash-EXL3-2x-DGX-Sparks-TensorFold.git"
	// v1.10 pins (current)
	GLM53FlashEXL3TensorFoldRecipeCommit    = "549fdc562c477d477244d2df0590d6c14bda4889"
	GLM53FlashEXL3TensorFoldImageRepository = "ghcr.io/miaai-lab/glm-5.3-flash-exl3-2x-dgx-sparks-tensorfold"
	GLM53FlashEXL3TensorFoldImageTag        = "v0.6.0-a1897d591f70"
	GLM53FlashEXL3TensorFoldImagePatchHash  = "a1897d591f70"
	GLM53FlashEXL3TensorFoldImageDigest     = "bc34d7d63f978cf601f42863b284bc95a567c50c10e9adb0866a635be568bf5f"
	GLM53FlashEXL3TensorFoldImage           = GLM53FlashEXL3TensorFoldImageRepository + "@sha256:" + GLM53FlashEXL3TensorFoldImageDigest
	// v1.4 pins (rollback)
	GLM53FlashEXL3TensorFoldRecipeCommitV14   = "cf28cc4f8038be322cdeda220c6f1c8ace8f27d1"
	GLM53FlashEXL3TensorFoldImageTagV14       = "v0.6.0-5e01f1bb74d8"
	GLM53FlashEXL3TensorFoldImagePatchHashV14 = "5e01f1bb74d8"
	GLM53FlashEXL3TensorFoldImageDigestV14    = "14f15591eae5d6a540f09218d3852068962fe5381371bbfefe0e9194cd834529"
	GLM53FlashEXL3TensorFoldImageV14          = GLM53FlashEXL3TensorFoldImageRepository + "@sha256:" + GLM53FlashEXL3TensorFoldImageDigestV14
	GLM53FlashEXL3TensorFoldServedModel       = "GLM-5.3-Flash-EXL3"
	GLM53FlashEXL3TensorFoldServicePort       = 8888
	GLM53FlashEXL3TensorFoldRendezvousPort    = 29551
	MultiDeviceDriverContainers               = "per_rank_containers"
	MultiDeviceDriverHeadRecipe               = "head_recipe"
	MultiDeviceBackendTensorFoldRecipe        = "tensorfold_recipe"
	GLM53FlashNVFP4DualGB10ID                 = "glm-5-3-flash-nvfp4-dual-gb10"
	GLM53FlashNVFP4Model                      = "LibertAIDAI/GLM-5.3-Flash-NVFP4"
	GLM53FlashNVFP4Revision                   = "aa28e1f54130286c95fee10d0705c74ce8743734"
	GLM53FlashNVFP4Image                      = "lmsysorg/sglang@sha256:73f9294b78e38d8cc297bfed16daec8ac192b126a2d1fb9055e259a632c68f00"
	GLM53FlashNVFP4ImageDigest                = "73f9294b78e38d8cc297bfed16daec8ac192b126a2d1fb9055e259a632c68f00"
	MultiDeviceBackendTorch                   = "torch_distributed"
	MultiDeviceBackendVLLM                    = "vllm_multi_node"
	MultiDeviceRoleHead                       = "head"
	MultiDeviceRoleWorker                     = "worker"
	MultiDeviceRoleRankPlaceholder            = "{ROLE_RANK}"
	MultiDeviceHeadAddrPlaceholder            = "{HEAD_FABRIC_ADDRESS}"
	GLM53FlashNVFP4RendezvousPort             = 25000
	GLM53FlashNVFP4ServicePort                = 8000
	GLM53FlashNVFP4GuardSeconds               = 3600
	GLM53FlashRuntimePatchLabel               = "sglang-unbalanced-model-loading-timeout-3600-v1"
	GLM53FlashRuntimePatchPath                = "/sgl-workspace/sglang/python/sglang/srt/model_executor/model_runner_components/load_model_utils.py"
	GLM53FlashRuntimePatchOldLine             = "UNBALANCED_MODEL_LOADING_TIMEOUT_S = 480  # leave more time for post data processing"
	GLM53FlashRuntimePatchNewLine             = "UNBALANCED_MODEL_LOADING_TIMEOUT_S = 3600  # align asymmetric cold loads with Yokai guards"
	GLM53FlashRuntimePatchOldSHA              = "f0193bfaab96053919e3a260f9ccb10e2137ad108cfae03816c867628f611e1f"
	GLM53FlashRuntimePatchNewSHA              = "fc3ae35cce5f712fd3681dc4bcef05157df6d232ac991ce60b78dae492784ff5"
	GLM53FlashDSAGB10TilePatchLabel           = "sglang-dsa-gb10-tile-tp2-v1"
	GLM53FlashDSAGB10TilePatchPath            = "/sgl-workspace/sglang/python/sglang/kernels/ops/attention/dsa/tilelang_kernel.py"
	GLM53FlashDSAGB10TilePatchOldLine         = "            num_heads, d_v, tail_dim, topk, sm_scale=sm_scale, return_lse=return_lse"
	GLM53FlashDSAGB10TilePatchNewLine         = "            num_heads, d_v, tail_dim, topk, sm_scale=sm_scale, return_lse=return_lse, **({'block_I': 32, 'num_stages': 1, 'threads': 128} if tail_dim == 0 else {})"
	GLM53FlashDSAGB10TilePatchOldSHA          = "526988aa5fd8fa61529f2d3cf245d1061e30982d2bd0d6e64ea2e51e31f30f7f"
	GLM53FlashDSAGB10TilePatchNewSHA          = "3150ec691843c84bb5db9ed5bf763c4da03842fde4666489e107bf7a9fcddd7b"
	GLM53FlashTileSharedMemoryBytes           = 100352
)

// RuntimePatchSetLabel derives the immutable composite label carried into the
// agent. The agent accepts only patch sets compiled into the matching BKC.
func RuntimePatchSetLabel(patches []MultiDeviceRuntimePatch) string {
	labels := make([]string, len(patches))
	for index, patch := range patches {
		labels[index] = patch.Label
	}
	return strings.Join(labels, "+")
}

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
	return RuntimePatchSetLabel(glm53FlashRuntimePatches)
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

var tensorFoldRecipeCapabilities = []string{
	"deployments.v1",
	"deployments.preflight.v1",
	"deployments.recipe.tensorfold.v1",
	"deployments.recipe.reservations.v1",
	"deployments.recipe.logs.v1",
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
// inconsistent. Pinned recipes receive additional provenance and flag checks
// so catalog drift cannot mutate devices.
func ValidateMultiDeviceRecipe(cfg Config) error {
	md := cfg.MultiDevice
	if md == nil {
		return fmt.Errorf("bkc %s is not a multi-device recipe", cfg.ID)
	}
	if cfg.Workload == WorkloadTensorFold {
		return validateTensorFoldRecipe(cfg)
	}
	if (cfg.Workload != WorkloadSGLang || md.Backend != MultiDeviceBackendTorch) &&
		(cfg.Workload != WorkloadVLLM || md.Backend != MultiDeviceBackendVLLM) {
		return fmt.Errorf("bkc %s has an unsupported coordinated runtime", cfg.ID)
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
	if len(md.LaunchOrder) != 0 && !sameRoleSet(md.LaunchOrder, []string{MultiDeviceRoleHead, MultiDeviceRoleWorker}) {
		return fmt.Errorf("bkc %s launch order must contain head and worker exactly once", cfg.ID)
	}
	if cfg.ID == Qwen38FlashNextNVFP4DualGB10ID {
		return validateQwen38FlashNextRecipe(cfg)
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

func sameRoleSet(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	seen := make(map[string]bool, len(left))
	for _, value := range left {
		if seen[value] {
			return false
		}
		seen[value] = true
	}
	for _, value := range right {
		if !seen[value] {
			return false
		}
	}
	return true
}

type tensorFoldPins struct {
	id           string
	image        string
	commit       string
	imageTag     string
	imageDigest  string
	watchdogExit string
	watchdogS    string
}

var tensorFoldPinsByID = map[string]tensorFoldPins{
	GLM53FlashEXL3TensorFoldDualGB10ID: {
		id:           GLM53FlashEXL3TensorFoldDualGB10ID,
		image:        GLM53FlashEXL3TensorFoldImage,
		commit:       GLM53FlashEXL3TensorFoldRecipeCommit,
		imageTag:     GLM53FlashEXL3TensorFoldImageTag,
		imageDigest:  GLM53FlashEXL3TensorFoldImageDigest,
		watchdogExit: "1",
		watchdogS:    "120",
	},
	GLM53FlashEXL3TensorFoldDualGB10V14ID: {
		id:           GLM53FlashEXL3TensorFoldDualGB10V14ID,
		image:        GLM53FlashEXL3TensorFoldImageV14,
		commit:       GLM53FlashEXL3TensorFoldRecipeCommitV14,
		imageTag:     GLM53FlashEXL3TensorFoldImageTagV14,
		imageDigest:  GLM53FlashEXL3TensorFoldImageDigestV14,
		watchdogExit: "0",
	},
}

// TensorFoldWatchdogForCommit returns the pinned watchdog policy (the value
// rendered as TF_GLM_MULTI_WATCHDOG_EXIT and, when set, TF_GLM_MULTI_WATCHDOG_S)
// for a recipe commit. ok is false for commits the BKC does not pin; callers
// must fail closed.
func TensorFoldWatchdogForCommit(commit string) (exit, seconds string, ok bool) {
	for _, pins := range tensorFoldPinsByID {
		if pins.commit == commit {
			return pins.watchdogExit, pins.watchdogS, true
		}
	}
	return "", "", false
}

func validateTensorFoldRecipe(cfg Config) error {
	md := cfg.MultiDevice
	pins, ok := tensorFoldPinsByID[cfg.ID]
	if !ok {
		return fmt.Errorf("bkc %s immutable TensorFold provenance mismatch", cfg.ID)
	}
	if cfg.ModelID != GLM53FlashEXL3TensorFoldModel || cfg.Image != pins.image {
		return fmt.Errorf("bkc %s immutable TensorFold provenance mismatch", cfg.ID)
	}
	if md.Driver != MultiDeviceDriverHeadRecipe || md.Backend != MultiDeviceBackendTensorFoldRecipe {
		return fmt.Errorf("bkc %s requires the TensorFold head-recipe driver", cfg.ID)
	}
	if md.WorldSize != 2 || md.TensorParallelSize != 2 || md.GPUsPerNode != 1 {
		return fmt.Errorf("bkc %s requires two one-GPU nodes with TP=2", cfg.ID)
	}
	if md.RendezvousPort != GLM53FlashEXL3TensorFoldRendezvousPort || md.ServicePort != GLM53FlashEXL3TensorFoldServicePort || cfg.Port != "8888" {
		return fmt.Errorf("bkc %s TensorFold ports drifted", cfg.ID)
	}
	if md.ModelRevision != GLM53FlashEXL3TensorFoldModelRevision || md.ServedModelName != GLM53FlashEXL3TensorFoldServedModel || len(md.RuntimePatches) != 0 {
		return fmt.Errorf("bkc %s TensorFold model provenance drifted", cfg.ID)
	}
	if !equalStrings(md.LaunchOrder, []string{MultiDeviceRoleWorker, MultiDeviceRoleHead}) {
		return fmt.Errorf("bkc %s must reserve the worker before launching the head recipe", cfg.ID)
	}
	if md.SourceRevision != "" || md.RequiresLocalModel || md.RequiresFabricConfig || len(md.CommonArgs) != 0 || len(md.RoleArgs) != 0 {
		return fmt.Errorf("bkc %s must leave execution inputs to the pinned upstream recipe", cfg.ID)
	}
	if len(md.Roles) != 2 || md.Roles[0] != (MultiDeviceRole{Name: MultiDeviceRoleHead, Rank: 0, API: true}) || md.Roles[1] != (MultiDeviceRole{Name: MultiDeviceRoleWorker, Rank: 1, API: false}) {
		return fmt.Errorf("bkc %s must define ordered head and worker roles", cfg.ID)
	}
	if !equalStrings(md.RequiredCapabilities, tensorFoldRecipeCapabilities) {
		return fmt.Errorf("bkc %s TensorFold capabilities mismatch", cfg.ID)
	}
	r := md.Recipe
	if r == nil || r.Repository != GLM53FlashEXL3TensorFoldRecipeRepository || r.Commit != pins.commit || r.ImageTag != pins.imageTag || r.ImageDigest != pins.imageDigest {
		return fmt.Errorf("bkc %s immutable TensorFold recipe source mismatch", cfg.ID)
	}
	if r.DrafterModel != GLM53FlashEXL3TensorFoldDrafter || r.DrafterRevision != GLM53FlashEXL3TensorFoldDrafterRevision {
		return fmt.Errorf("bkc %s immutable DFlash2 provenance mismatch", cfg.ID)
	}
	if r.ContextTokens != 1048576 || r.ParallelRequests != 4 || r.KVCache != "fp8" || r.Dense != "q4" || r.Drafter != "dflash2" || !r.Vision {
		return fmt.Errorf("bkc %s TensorFold serving policy drifted", cfg.ID)
	}
	if r.MemoryReserveGiB != "14.5" || r.KVPoolGiB != "12.5" || r.NCCLRails != 1 || r.ReadinessTimeoutSec != 14400 {
		return fmt.Errorf("bkc %s TensorFold memory, rail, or readiness policy drifted", cfg.ID)
	}
	if r.WatchdogExit != pins.watchdogExit || r.WatchdogSeconds != pins.watchdogS {
		return fmt.Errorf("bkc %s TensorFold watchdog policy drifted", cfg.ID)
	}
	if strings.TrimSpace(cfg.ExtraArgs) != "" || len(cfg.Env) != 0 || len(cfg.Volumes) != 0 {
		return fmt.Errorf("bkc %s must leave execution inputs to the pinned upstream recipe", cfg.ID)
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
