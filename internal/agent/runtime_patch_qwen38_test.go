package agent

import (
	"context"
	"crypto/sha256"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/spencerbull/yokai/internal/bkc"
	"github.com/spencerbull/yokai/internal/config"
)

func pinnedQwen38PatchRequest(role string) ContainerRequest {
	args := []string{
		"--served-model-name", bkc.Qwen38FlashNextServedModel, "--tensor-parallel-size", "2", "--gpu-memory-utilization", "0.835",
		"--max-num-seqs", "8", "--max-num-batched-tokens", "8192", "--max-model-len", "262144", "--kv-cache-dtype", "fp8",
		"--mamba-ssm-cache-dtype", "bfloat16", "--load-format", "safetensors", "--safetensors-load-strategy", "lazy",
		"--enable-chunked-prefill", "--reasoning-parser", "qwen3", "--enable-auto-tool-choice", "--tool-call-parser", "qwen3_coder",
		"--distributed-executor-backend", "mp", "--mm-encoder-tp-mode", "data", "--nnodes", "2", "--master-addr", "10.20.0.1",
		"--master-port", "50000", "--enable-expert-parallel", "--all2all-backend", "allgather_reducescatter",
		"--speculative-config", `{"method":"mtp","num_speculative_tokens":3,"use_local_argmax_reduction":true}`,
		"--compilation-config", `{"mode":0,"cudagraph_mode":"FULL_DECODE_ONLY"}`,
		"--hf-overrides", `{"text_config":{"ple_embedding_dtype":"float8_e4m3fn"}}`, "--revision", bkc.Qwen38FlashNextNVFP4Revision,
	}
	if role == bkc.MultiDeviceRoleHead {
		args = append(args, "--node-rank", "0", "--host", "100.96.0.20", "--port", "8888", "--api-key=secret")
	} else {
		args = append(args, "--node-rank", "1", "--headless")
	}
	return ContainerRequest{
		Image: bkc.Qwen38FlashNextNVFP4Image, Model: bkc.Qwen38FlashNextContainerModel,
		ExtraArgs: "--model " + bkc.Qwen38FlashNextContainerModel, Args: args,
		Volumes: map[string]string{"/srv/hf/models--nvidia--Qwen3.8-Flash-Next-NVFP4": bkc.Qwen38FlashNextContainerRoot + ":ro"},
		Ports:   map[string]string{"8888": "8888"}, GPUIDs: "0", NetworkMode: "host",
		Devices: []string{"/dev/infiniband:/dev/infiniband"}, CapAdd: []string{"SYS_NICE"},
		Runtime: config.RuntimeOptions{IPCMode: "host", Ulimits: map[string]string{"memlock": "-1", "stack": "67108864"}, RestartPolicy: config.RestartPolicyNo},
		Env: map[string]string{
			"NCCL_IB_DISABLE": "0", "NCCL_IB_AUTO_DETECT": "0", "NCCL_DEBUG": "WARN", "HF_HUB_OFFLINE": "1", "TRANSFORMERS_OFFLINE": "1", "HF_HOME": "/root/.cache/huggingface",
			"GLOO_SOCKET_IFNAME": "enp1s0f0np0", "NCCL_SOCKET_IFNAME": "enp1s0f0np0", "TP_SOCKET_IFNAME": "enp1s0f0np0", "NCCL_IB_HCA": "=rocep1s0f0", "NCCL_IB_GID_INDEX": "3", "VLLM_HOST_IP": "10.20.0.1",
		},
		Labels: map[string]string{
			LabelBKCID: bkc.Qwen38FlashNextNVFP4DualGB10ID, LabelRole: role,
			LabelRuntimePatch: bkc.Qwen38RuntimePatchSetLabel(), LabelImageDigest: bkc.Qwen38FlashNextNVFP4ImageDigest,
			LabelModelRevision: bkc.Qwen38FlashNextNVFP4Revision, LabelSourceRevision: bkc.Qwen38FlashNextSourceRevision,
		},
	}
}

func TestPinnedQwen38RuntimePatchBuildsFixedPythonEntrypoint(t *testing.T) {
	originalEnsure := ensureQwen38RuntimeAssetsForLaunch
	defer func() { ensureQwen38RuntimeAssetsForLaunch = originalEnsure }()
	assetDir := t.TempDir()
	ensureQwen38RuntimeAssetsForLaunch = func(context.Context) (string, error) { return assetDir, nil }
	req := pinnedQwen38PatchRequest(bkc.MultiDeviceRoleHead)
	original := append(strings.Fields(req.ExtraArgs), req.Args...)
	if err := applyPinnedQwen38RuntimePatch(context.Background(), &req); err != nil {
		t.Fatal(err)
	}
	if req.ExtraArgs != "" || req.Entrypoint != "python3" || len(req.Args) < 3 || req.Args[0] != "-c" || strings.ContainsAny(req.Args[1], "\x00\r\n") {
		t.Fatalf("unexpected bootstrap command: entrypoint=%q extra=%q args=%#v", req.Entrypoint, req.ExtraArgs, req.Args[:2])
	}
	if strings.Contains(req.Args[1], "/bin/sh") || strings.Contains(req.Args[1], "shell=True") || !reflect.DeepEqual(req.Args[2:], original) {
		t.Fatalf("bootstrap introduced a shell or changed structured argv")
	}
	if !strings.Contains(qwen38BootstrapPython, "patched Qwen3.8 source has no verified pristine backup") || !strings.Contains(qwen38BootstrapPython, "os.execvp") {
		t.Fatal("bootstrap is not restart-safe or does not directly exec vLLM")
	}
	if req.Volumes[assetDir] != qwen38PatchContainerDir+":ro" || req.Env["VLLM_MTP_DRAFT_VOCAB"] == "" {
		t.Fatalf("patch assets were not mounted read-only: %#v %#v", req.Volumes, req.Env)
	}
	dockerArgs := buildDockerRunArgs(req, "yokai-qwen38-head")
	joined := strings.Join(dockerArgs, " ")
	if !strings.Contains(joined, "--entrypoint python3 "+bkc.Qwen38FlashNextNVFP4Image+" -c") {
		t.Fatalf("docker argv omitted fixed entrypoint: %s", joined)
	}
}

func TestPinnedQwen38RuntimePatchFailsClosedBeforeAssetFetch(t *testing.T) {
	originalEnsure := ensureQwen38RuntimeAssetsForLaunch
	defer func() { ensureQwen38RuntimeAssetsForLaunch = originalEnsure }()
	fetches := 0
	ensureQwen38RuntimeAssetsForLaunch = func(context.Context) (string, error) { fetches++; return t.TempDir(), nil }
	tests := map[string]func(*ContainerRequest){
		"source revision": func(req *ContainerRequest) { req.Labels[LabelSourceRevision] = "main" },
		"image":           func(req *ContainerRequest) { req.Image = "vllm/vllm-openai:qwen38-flash-next" },
		"model":           func(req *ContainerRequest) { req.Model = bkc.Qwen38FlashNextNVFP4Model },
		"worker host":     func(req *ContainerRequest) { req.Args = append(req.Args, "--host", "0.0.0.0") },
		"duplicate rank":  func(req *ContainerRequest) { req.Args = append(req.Args, "--node-rank", "1") },
		"writable volume": func(req *ContainerRequest) { req.Volumes["/extra"] = "/tmp" },
		"extra env":       func(req *ContainerRequest) { req.Env["PYTHONPATH"] = "/tmp" },
		"runtime":         func(req *ContainerRequest) { req.CapAdd = append(req.CapAdd, "SYS_ADMIN") },
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			req := pinnedQwen38PatchRequest(bkc.MultiDeviceRoleWorker)
			mutate(&req)
			if err := applyPinnedQwen38RuntimePatch(context.Background(), &req); err == nil {
				t.Fatal("expected provenance or argv drift rejection")
			}
		})
	}
	if fetches != 0 {
		t.Fatalf("invalid requests fetched runtime assets %d times", fetches)
	}
}

func TestQwen38SnapshotObjectRequiresPinnedContentAddressedSymlink(t *testing.T) {
	repository := filepath.Join(t.TempDir(), bkc.Qwen38FlashNextHFCacheDirectory)
	blobs := filepath.Join(repository, "blobs")
	snapshot := filepath.Join(repository, "snapshots", bkc.Qwen38FlashNextNVFP4Revision)
	if err := os.MkdirAll(blobs, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(snapshot, 0o755); err != nil {
		t.Fatal(err)
	}
	content := []byte("pinned")
	blob := fmt.Sprintf("%x", sha256.Sum256(content))
	if err := os.WriteFile(filepath.Join(blobs, blob), content, 0o644); err != nil {
		t.Fatal(err)
	}
	objectPath := filepath.Join(snapshot, "object.bin")
	if err := os.Symlink(filepath.Join("..", "..", "blobs", blob), objectPath); err != nil {
		t.Fatal(err)
	}
	resolvedRoot, err := filepath.EvalSymlinks(repository)
	if err != nil {
		t.Fatal(err)
	}
	object := qwen38SnapshotObject{blob: blob, size: int64(len("pinned"))}
	if err := validateQwen38SnapshotObject(snapshot, resolvedRoot, "object.bin", object); err != nil {
		t.Fatalf("valid content-addressed object rejected: %v", err)
	}
	if err := validateQwen38SnapshotObject(snapshot, resolvedRoot, "object.bin", qwen38SnapshotObject{blob: "other", size: object.size}); err == nil {
		t.Fatal("expected wrong content address rejection")
	}
}

func TestQwen38PreflightChecksExactFabricAndSnapshotBeforeMutation(t *testing.T) {
	modelDir := t.TempDir()
	gidIndex := 3
	request := deploymentPreflightRequest{
		FabricAddress: "10.20.0.1", FabricInterface: "enp1s0f0np0", FabricHCA: "rocep1s0f0", FabricGIDIndex: &gidIndex, RequireFabric: true,
		ServiceAddress: "100.96.0.20", ServicePort: 8888, RendezvousPort: 50000, Head: true, CandidateName: "candidate",
		LocalModelPath: modelDir, BKCID: bkc.Qwen38FlashNextNVFP4DualGB10ID, ModelRevision: bkc.Qwen38FlashNextNVFP4Revision,
	}
	fabricCalls, snapshotCalls := 0, 0
	deps := deploymentPreflightDeps{
		interfaceAddrs: func() ([]net.Addr, error) {
			return []net.Addr{testAddr("10.20.0.1/24"), testAddr("100.96.0.20/32")}, nil
		},
		stat: os.Stat, open: os.Open, listContainers: func(string) ([]Container, error) { return nil, nil },
		portAvailable: func(string, int) (bool, error) { return true, nil }, containerOwns: func(string, string, int) (bool, error) { return false, nil },
		fabricBinding: func(iface, hca string, gid int, address string) error {
			fabricCalls++
			if iface != request.FabricInterface || hca != request.FabricHCA || gid != 3 || address != request.FabricAddress {
				t.Fatalf("fabric preflight mismatch")
			}
			return nil
		},
		validateSnapshot: func(path, revision string) error {
			snapshotCalls++
			if path != modelDir || revision != bkc.Qwen38FlashNextNVFP4Revision {
				t.Fatalf("snapshot preflight mismatch")
			}
			return nil
		},
		computeTenants: func(context.Context) ([]gpuComputeTenant, error) { return nil, nil },
	}
	if err := validateDeploymentPreflight(context.Background(), request, deps); err != nil {
		t.Fatalf("valid Qwen preflight failed: %v", err)
	}
	if fabricCalls != 1 || snapshotCalls != 1 {
		t.Fatalf("missing exact preflights: fabric=%d snapshot=%d", fabricCalls, snapshotCalls)
	}
	request.RequireFabric = false
	if err := validateDeploymentPreflight(context.Background(), request, deps); err == nil {
		t.Fatal("pinned Qwen3.8 preflight accepted an omitted fabric contract")
	}
}
