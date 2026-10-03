package agent

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/spencerbull/yokai/internal/bkc"
)

const qwen38PatchContainerDir = "/opt/yokai-qwen38-patches"

type pinnedRuntimeAsset struct {
	name   string
	sha256 string
}

var qwen38RuntimeAssets = []pinnedRuntimeAsset{
	{name: "ple_layer_patched.py", sha256: "fae9fd5242748e8cdb314445a25ad628a0ce335cf26f794623f8679497a65186"},
	{name: "patch_modelopt_mxfp8.py", sha256: "adf72c590969a8a690fdca83e0cb2428df0dc7c7debdd2cb1983013492d9f285"},
	{name: "patch_modelopt_fp8_block_moe.py", sha256: "20b5d81b097f6135a0403d91cdea25e90180f7b7d0aa2fe8d9ba3fe3d4c83acf"},
	{name: "patch_mtp_draft_vocab.py", sha256: "d5a85baaab238917d70448feb57761e61826f4e515bbc098167515d601bb6ea5"},
	{name: "patch_qsa_fp8_kv.py", sha256: "61b7fc7cb64b9ef0d6dc702966385331a27464616cdc3ade22c5d85057a30956"},
	{name: "patch_checkpoint_config.py", sha256: "d727134c3af8db66ee2979fc64049da7895aa0e97408895903d54c52bbdc479b"},
	{name: "draft_vocab_en_code_47k.txt", sha256: "20e36b6e8eae2598019298959a578ef8adc2948bbed7189e43a8da9b9d84a0b1"},
}

var qwen38RuntimeAssetClient = &http.Client{Timeout: 2 * time.Minute}
var ensureQwen38RuntimeAssetsForLaunch = ensureQwen38RuntimeAssets

func applyPinnedQwen38RuntimePatch(ctx context.Context, req *ContainerRequest) error {
	if req.Labels[LabelBKCID] != bkc.Qwen38FlashNextNVFP4DualGB10ID {
		if req.Labels[LabelRuntimePatch] == bkc.Qwen38RuntimePatchSetLabel() {
			return fmt.Errorf("Qwen3.8 runtime patch label is restricted to its pinned BKC")
		}
		return nil
	}
	if req.Labels[LabelRuntimePatch] != bkc.Qwen38RuntimePatchSetLabel() || req.Image != bkc.Qwen38FlashNextNVFP4Image || req.Labels[LabelImageDigest] != bkc.Qwen38FlashNextNVFP4ImageDigest || req.Labels[LabelModelRevision] != bkc.Qwen38FlashNextNVFP4Revision || req.Labels[LabelSourceRevision] != bkc.Qwen38FlashNextSourceRevision {
		return fmt.Errorf("runtime patch provenance does not match the pinned Qwen3.8 BKC")
	}
	if req.Model != bkc.Qwen38FlashNextContainerModel || !hasExactQwen38ModelVolume(req.Volumes) {
		return fmt.Errorf("pinned Qwen3.8 runtime patch requires the read-only fixed Hugging Face repository mount")
	}
	if err := validateQwen38ContainerContract(*req); err != nil {
		return err
	}
	command := append([]string(nil), strings.Fields(req.ExtraArgs)...)
	command = append(command, req.Args...)
	if model, count := tokenFlagValueCount(command, "--model"); count != 1 || model != bkc.Qwen38FlashNextContainerModel {
		return fmt.Errorf("pinned Qwen3.8 runtime patch requires exactly one fixed local model argument")
	}
	if revision, count := tokenFlagValueCount(command, "--revision"); count != 1 || revision != bkc.Qwen38FlashNextNVFP4Revision {
		return fmt.Errorf("pinned Qwen3.8 runtime patch requires exactly one pinned model revision")
	}
	for flag, expected := range map[string]string{"--tensor-parallel-size": "2", "--nnodes": "2", "--master-port": strconv.Itoa(bkc.Qwen38FlashNextRendezvousPort), "--kv-cache-dtype": "fp8", "--mamba-ssm-cache-dtype": "bfloat16"} {
		if value, count := tokenFlagValueCount(command, flag); count != 1 || value != expected {
			return fmt.Errorf("pinned Qwen3.8 runtime patch requires exactly one %s %s setting", flag, expected)
		}
	}
	masterAddress, count := tokenFlagValueCount(command, "--master-addr")
	if count != 1 || net.ParseIP(strings.Trim(masterAddress, "[]")) == nil {
		return fmt.Errorf("pinned Qwen3.8 runtime patch requires one concrete master address")
	}
	role := req.Labels[LabelRole]
	rank, rankCount := tokenFlagValueCount(command, "--node-rank")
	host, hostCount := tokenFlagValueCount(command, "--host")
	_, portCount := tokenFlagValueCount(command, "--port")
	headlessCount := argSequenceCount(command, "--headless")
	apiKeyCount := flagCount(command, "--api-key")
	switch role {
	case bkc.MultiDeviceRoleHead:
		if rankCount != 1 || rank != "0" || hostCount != 1 || net.ParseIP(host) == nil || portCount != 1 || headlessCount != 0 || apiKeyCount != 1 {
			return fmt.Errorf("pinned Qwen3.8 head argv does not match rank-0 API role")
		}
	case bkc.MultiDeviceRoleWorker:
		if rankCount != 1 || rank != "1" || hostCount != 0 || portCount != 0 || headlessCount != 1 || apiKeyCount != 0 {
			return fmt.Errorf("pinned Qwen3.8 worker argv does not match rank-1 headless role")
		}
	default:
		return fmt.Errorf("pinned Qwen3.8 runtime patch requires an exact head or worker role")
	}
	for _, required := range []string{"--enable-expert-parallel", "--enable-chunked-prefill", "--enable-auto-tool-choice"} {
		if argSequenceCount(command, required) != 1 {
			return fmt.Errorf("pinned Qwen3.8 runtime patch requires exactly one %s", required)
		}
	}
	assetDir, err := ensureQwen38RuntimeAssetsForLaunch(ctx)
	if err != nil {
		return err
	}
	if req.Volumes == nil {
		req.Volumes = make(map[string]string)
	}
	req.Volumes[assetDir] = qwen38PatchContainerDir + ":ro"
	if req.Env == nil {
		req.Env = make(map[string]string)
	}
	req.Env["VLLM_MTP_DRAFT_VOCAB"] = qwen38PatchContainerDir + "/draft_vocab_en_code_47k.txt"
	req.ExtraArgs = ""
	req.Entrypoint = "python3"
	req.Args = append([]string{"-c", qwen38BootstrapArg()}, command...)
	return nil
}

func hasExactQwen38ModelVolume(volumes map[string]string) bool {
	if len(volumes) != 1 {
		return false
	}
	for host, target := range volumes {
		return filepath.Base(filepath.Clean(host)) == bkc.Qwen38FlashNextHFCacheDirectory && target == bkc.Qwen38FlashNextContainerRoot+":ro"
	}
	return false
}

func validateQwen38ContainerContract(req ContainerRequest) error {
	if req.NetworkMode != "host" || req.GPUIDs != "0" || len(req.Devices) != 1 || req.Devices[0] != "/dev/infiniband:/dev/infiniband" || len(req.CapAdd) != 1 || req.CapAdd[0] != "SYS_NICE" {
		return fmt.Errorf("pinned Qwen3.8 runtime or device contract mismatch")
	}
	if req.Runtime.IPCMode != "host" || req.Runtime.ShmSize != "" || string(req.Runtime.RestartPolicy) != "no" || len(req.Runtime.Ulimits) != 2 || req.Runtime.Ulimits["memlock"] != "-1" || req.Runtime.Ulimits["stack"] != "67108864" {
		return fmt.Errorf("pinned Qwen3.8 IPC, ulimit, or restart contract mismatch")
	}
	if len(req.Ports) != 1 || req.Ports["8888"] != "8888" {
		return fmt.Errorf("pinned Qwen3.8 service port contract mismatch")
	}
	required := map[string]string{
		"NCCL_IB_DISABLE": "0", "NCCL_IB_AUTO_DETECT": "0", "NCCL_DEBUG": "WARN",
		"HF_HUB_OFFLINE": "1", "TRANSFORMERS_OFFLINE": "1", "HF_HOME": "/root/.cache/huggingface",
	}
	if len(req.Env) != len(required)+6 {
		return fmt.Errorf("pinned Qwen3.8 environment contains unexpected settings")
	}
	for key, value := range required {
		if req.Env[key] != value {
			return fmt.Errorf("pinned Qwen3.8 environment %s mismatch", key)
		}
	}
	interfaceName := req.Env["NCCL_SOCKET_IFNAME"]
	if !localFabricNamePattern.MatchString(interfaceName) || req.Env["GLOO_SOCKET_IFNAME"] != interfaceName || req.Env["TP_SOCKET_IFNAME"] != interfaceName {
		return fmt.Errorf("pinned Qwen3.8 fabric interface environment mismatch")
	}
	hca := strings.TrimPrefix(req.Env["NCCL_IB_HCA"], "=")
	if req.Env["NCCL_IB_HCA"] != "="+hca || !localFabricNamePattern.MatchString(hca) {
		return fmt.Errorf("pinned Qwen3.8 fabric HCA environment mismatch")
	}
	gid, err := strconv.Atoi(req.Env["NCCL_IB_GID_INDEX"])
	if err != nil || gid < 0 || gid > 255 || net.ParseIP(req.Env["VLLM_HOST_IP"]) == nil {
		return fmt.Errorf("pinned Qwen3.8 fabric address or GID environment mismatch")
	}
	return nil
}

func flagCount(args []string, flag string) int {
	count := 0
	for _, arg := range args {
		if arg == flag || strings.HasPrefix(arg, flag+"=") {
			count++
		}
	}
	return count
}

func qwen38RuntimeAssetDir() string {
	if root := strings.TrimSpace(os.Getenv("YOKAI_RUNTIME_PATCH_DIR")); root != "" {
		return filepath.Join(root, bkc.Qwen38FlashNextSourceRevision)
	}
	if os.Geteuid() == 0 {
		return filepath.Join("/var/lib/yokai/runtime-patches", bkc.Qwen38FlashNextSourceRevision)
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return filepath.Join(os.TempDir(), "yokai-runtime-patches", bkc.Qwen38FlashNextSourceRevision)
	}
	return filepath.Join(home, ".local", "share", "yokai", "runtime-patches", bkc.Qwen38FlashNextSourceRevision)
}

func ensureQwen38RuntimeAssets(ctx context.Context) (string, error) {
	directory := qwen38RuntimeAssetDir()
	if err := os.MkdirAll(directory, 0o755); err != nil {
		return "", fmt.Errorf("create pinned Qwen3.8 patch directory: %w", err)
	}
	baseURL := "https://raw.githubusercontent.com/MiaAI-Lab/Qwen3.8-Flash-Next-Dual-DGX-Sparks/" + bkc.Qwen38FlashNextSourceRevision + "/files/"
	for _, asset := range qwen38RuntimeAssets {
		path := filepath.Join(directory, asset.name)
		if data, err := os.ReadFile(path); err == nil && fmt.Sprintf("%x", sha256.Sum256(data)) == asset.sha256 {
			continue
		}
		request, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+asset.name, nil)
		if err != nil {
			return "", fmt.Errorf("build pinned runtime asset request: %w", err)
		}
		response, err := qwen38RuntimeAssetClient.Do(request)
		if err != nil {
			return "", fmt.Errorf("download pinned runtime asset %s: %w", asset.name, err)
		}
		data, readErr := io.ReadAll(io.LimitReader(response.Body, 4<<20))
		closeErr := response.Body.Close()
		if response.StatusCode != http.StatusOK {
			return "", fmt.Errorf("download pinned runtime asset %s: status %d", asset.name, response.StatusCode)
		}
		if readErr != nil || closeErr != nil {
			return "", fmt.Errorf("read pinned runtime asset %s", asset.name)
		}
		if len(data) == 0 || len(data) >= 4<<20 || fmt.Sprintf("%x", sha256.Sum256(data)) != asset.sha256 {
			return "", fmt.Errorf("pinned runtime asset %s hash mismatch", asset.name)
		}
		temporary, err := os.CreateTemp(directory, ".asset-*")
		if err != nil {
			return "", fmt.Errorf("stage pinned runtime asset %s: %w", asset.name, err)
		}
		temporaryPath := temporary.Name()
		cleanup := func() { _ = temporary.Close(); _ = os.Remove(temporaryPath) }
		if _, err := temporary.Write(data); err != nil {
			cleanup()
			return "", fmt.Errorf("write pinned runtime asset %s: %w", asset.name, err)
		}
		if err := temporary.Sync(); err != nil {
			cleanup()
			return "", fmt.Errorf("sync pinned runtime asset %s: %w", asset.name, err)
		}
		if err := temporary.Close(); err != nil {
			cleanup()
			return "", fmt.Errorf("close pinned runtime asset %s: %w", asset.name, err)
		}
		if err := os.Chmod(temporaryPath, 0o644); err != nil {
			cleanup()
			return "", fmt.Errorf("chmod pinned runtime asset %s: %w", asset.name, err)
		}
		if err := os.Rename(temporaryPath, path); err != nil {
			cleanup()
			return "", fmt.Errorf("install pinned runtime asset %s: %w", asset.name, err)
		}
	}
	return directory, nil
}

func qwen38BootstrapArg() string {
	return "import base64;exec(compile(base64.b64decode(" + strconv.Quote(base64.StdEncoding.EncodeToString([]byte(qwen38BootstrapPython))) + "),'<yokai-qwen38-bootstrap>','exec'))"
}

const qwen38BootstrapPython = `import hashlib,os,pathlib,shutil,subprocess,sys
asset=pathlib.Path("/opt/yokai-qwen38-patches")
work=pathlib.Path("/tmp/yokai-qwen38-patches")
originals=pathlib.Path("/tmp/yokai-qwen38-originals")
model=pathlib.Path("/models/yokai-qwen38-repository/snapshots/fc694b54fb0174e0913e6adf86691ef85a4ead47")
overlay=pathlib.Path("/tmp/yokai-qwen38-model")
def digest(path): return hashlib.sha256(path.read_bytes()).hexdigest()
assets={"ple_layer_patched.py":"fae9fd5242748e8cdb314445a25ad628a0ce335cf26f794623f8679497a65186","patch_modelopt_mxfp8.py":"adf72c590969a8a690fdca83e0cb2428df0dc7c7debdd2cb1983013492d9f285","patch_modelopt_fp8_block_moe.py":"20b5d81b097f6135a0403d91cdea25e90180f7b7d0aa2fe8d9ba3fe3d4c83acf","patch_mtp_draft_vocab.py":"d5a85baaab238917d70448feb57761e61826f4e515bbc098167515d601bb6ea5","patch_qsa_fp8_kv.py":"61b7fc7cb64b9ef0d6dc702966385331a27464616cdc3ade22c5d85057a30956","patch_checkpoint_config.py":"d727134c3af8db66ee2979fc64049da7895aa0e97408895903d54c52bbdc479b","draft_vocab_en_code_47k.txt":"20e36b6e8eae2598019298959a578ef8adc2948bbed7189e43a8da9b9d84a0b1"}
for name,want in assets.items():
 p=asset/name
 if not p.is_file() or digest(p)!=want: raise SystemExit("refusing unverified Qwen3.8 patch asset: "+name)
sources=[("/usr/local/lib/python3.12/dist-packages/vllm/models/qwen3_8_flash_next/nvidia/ple_layer.py","a71144c1d36e06f22a2da1b1ada900076597fe5e824a911e7ada86249a0993e7","fae9fd5242748e8cdb314445a25ad628a0ce335cf26f794623f8679497a65186"),("/usr/local/lib/python3.12/dist-packages/vllm/model_executor/layers/quantization/modelopt.py","3f3ca743fd3c66d72be92b7544591a8632f1aa422b73bb50c83e8a3281196e7d","ed14a21a9331d78deddd78f3c953536c7b827dc35edbb1acabd0f0fe68c64802"),("/usr/local/lib/python3.12/dist-packages/vllm/models/qwen3_8_flash_next/nvidia/mtp.py","7735cee47d0d1e4776bebd30d907e4a62160409ce4ef2d65611559f8d58af431","30e3bad24d151f50f318b3836e6a6c5c1979c1b6ef7264a9fef21bdc162e01a2"),("/usr/local/lib/python3.12/dist-packages/vllm/models/qwen3_8_flash_next/nvidia/ops/qsa.py","c4ffe3674cafa0ce2dabc39a39f0ddbb4b594bc358ad210ffce9d04383350c7f","0669d6334f58a624c89c15f3e46c90f28e59b0b913507101dec1c5765e3c3b12"),("/usr/local/lib/python3.12/dist-packages/vllm/models/qwen3_8_flash_next/nvidia/qsa.py","748addc85efaa8f7df940d1245bc900192f92e1f17af8fa774625758600751cb","ee5de40742ad48a6064ea24b99a285ff69c47d57bbb170f57c4eef71567a1df3")]
for index,(name,pristine,patched) in enumerate(sources):
 p=pathlib.Path(name)
 live=digest(p) if p.is_file() else ""
 backup=originals/(str(index)+"-"+p.name)
 if live not in (pristine,patched): raise SystemExit("refusing unexpected pinned vLLM source: "+name)
 if backup.exists() and (not backup.is_file() or digest(backup)!=pristine): raise SystemExit("refusing unexpected Qwen3.8 pristine backup: "+name)
 if live==patched and not backup.is_file(): raise SystemExit("patched Qwen3.8 source has no verified pristine backup: "+name)
for name,want in (("config.json","deef67a61f3311faf051b23dc4192f442c7fee4f9cd2f38cbcbe4da55c763a80"),("hf_quant_config.json","331ad11d57c8bc374554579977198084e0d4d0933d5b3558f125f6f670aba0e8")):
 p=model/name
 if not p.is_file() or digest(p)!=want: raise SystemExit("refusing unexpected pinned model source: "+name)
originals.mkdir(mode=0o700,exist_ok=True)
for index,(name,pristine,patched) in enumerate(sources):
 backup=originals/(str(index)+"-"+pathlib.Path(name).name)
 if not backup.exists(): shutil.copyfile(name,backup)
 if digest(backup)!=pristine: raise SystemExit("Qwen3.8 pristine backup verification failed: "+name)
shutil.rmtree(work,ignore_errors=True);work.mkdir(mode=0o700)
for name in assets:
 if name.endswith(".py"): shutil.copyfile(asset/name,work/name)
ple=pathlib.Path(sources[0][0]);shutil.copyfile(asset/"ple_layer_patched.py",ple)
if digest(ple)!="fae9fd5242748e8cdb314445a25ad628a0ce335cf26f794623f8679497a65186": raise SystemExit("PLE patch verification failed")
modelopt=pathlib.Path(sources[1][0]);shutil.copyfile(originals/("1-"+modelopt.name),work/"modelopt_patched.py.orig")
subprocess.run([sys.executable,str(work/"patch_modelopt_mxfp8.py")],check=True,cwd=work)
subprocess.run([sys.executable,str(work/"patch_modelopt_fp8_block_moe.py")],check=True,cwd=work)
if digest(work/"modelopt_patched.py")!="ed14a21a9331d78deddd78f3c953536c7b827dc35edbb1acabd0f0fe68c64802": raise SystemExit("ModelOpt patch verification failed")
shutil.copyfile(work/"modelopt_patched.py",modelopt)
mtp=pathlib.Path(sources[2][0]);shutil.copyfile(originals/("2-"+mtp.name),work/"mtp_patched.py.orig")
subprocess.run([sys.executable,str(work/"patch_mtp_draft_vocab.py")],check=True,cwd=work)
if digest(work/"mtp_patched.py")!="30e3bad24d151f50f318b3836e6a6c5c1979c1b6ef7264a9fef21bdc162e01a2": raise SystemExit("MTP patch verification failed")
shutil.copyfile(work/"mtp_patched.py",mtp)
qops=pathlib.Path(sources[3][0]);qimpl=pathlib.Path(sources[4][0]);shutil.copyfile(originals/("3-"+qops.name),work/"qsa_ops_patched.py.orig");shutil.copyfile(originals/("4-"+qimpl.name),work/"qsa_nvidia_patched.py.orig")
subprocess.run([sys.executable,str(work/"patch_qsa_fp8_kv.py")],check=True,cwd=work)
if digest(work/"qsa_ops_patched.py")!="0669d6334f58a624c89c15f3e46c90f28e59b0b913507101dec1c5765e3c3b12" or digest(work/"qsa_nvidia_patched.py")!="ee5de40742ad48a6064ea24b99a285ff69c47d57bbb170f57c4eef71567a1df3": raise SystemExit("FP8 KV patch verification failed")
shutil.copyfile(work/"qsa_ops_patched.py",qops);shutil.copyfile(work/"qsa_nvidia_patched.py",qimpl)
shutil.rmtree(overlay,ignore_errors=True);overlay.mkdir(mode=0o700)
for child in model.iterdir():
 if child.name not in ("config.json","hf_quant_config.json"): (overlay/child.name).symlink_to(child)
subprocess.run([sys.executable,str(work/"patch_checkpoint_config.py"),str(model),str(work)],check=True,cwd=work)
for source_name,patched_name,want in (("config.json","config_patched.json","b3ff72b8164d31f902106ad8a54e06e41cf0a8136cc8b75d02781b8a6c30b322"),("hf_quant_config.json","hf_quant_config_patched.json","dd8727422cafbb0257d11a7163442bda46421f6e67c78eb9acd58669cb6eb5f8")):
 p=work/patched_name
 if not p.is_file() or digest(p)!=want: raise SystemExit("checkpoint config patch verification failed: "+source_name)
 shutil.copyfile(p,overlay/source_name)
args=sys.argv[1:]
positions=[i for i,value in enumerate(args) if value=="--model"]
if len(positions)!=1 or positions[0]+1>=len(args) or args[positions[0]+1]!=str(model): raise SystemExit("invalid model argv for Qwen3.8 bootstrap")
args[positions[0]+1]=str(overlay)
os.execvp("vllm",["vllm","serve",*args])
`
