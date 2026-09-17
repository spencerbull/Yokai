package agent

import (
	"bytes"
	"context"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"hash"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/spencerbull/yokai/internal/bkc"
)

type deploymentPreflightRequest struct {
	FabricAddress     string `json:"fabric_address"`
	FabricInterface   string `json:"fabric_interface,omitempty"`
	FabricHCA         string `json:"fabric_hca,omitempty"`
	FabricGIDIndex    *int   `json:"fabric_gid_index,omitempty"`
	RequireFabric     bool   `json:"require_fabric_config,omitempty"`
	ServiceAddress    string `json:"service_address,omitempty"`
	ServicePort       int    `json:"service_port"`
	RendezvousPort    int    `json:"rendezvous_port"`
	Head              bool   `json:"head"`
	ObservedContainer string `json:"observed_container_id,omitempty"`
	CandidateName     string `json:"candidate_name"`
	LocalModelPath    string `json:"local_model_path,omitempty"`
	BKCID             string `json:"bkc_id,omitempty"`
	ModelRevision     string `json:"model_revision,omitempty"`
}

type deploymentPreflightDeps struct {
	interfaceAddrs   func() ([]net.Addr, error)
	stat             func(string) (os.FileInfo, error)
	open             func(string) (*os.File, error)
	listContainers   func(string) ([]Container, error)
	portAvailable    func(string, int) (bool, error)
	containerOwns    func(string, string, int) (bool, error)
	fabricBinding    func(string, string, int, string) error
	validateSnapshot func(string, string) error
	computeTenants   func(context.Context) ([]gpuComputeTenant, error)
	qwenAdmission    func(context.Context) (func(), error)
}

var liveDeploymentPreflightDeps = deploymentPreflightDeps{
	interfaceAddrs:   net.InterfaceAddrs,
	stat:             os.Stat,
	open:             os.Open,
	listContainers:   listContainersScope,
	portAvailable:    tcpPortAvailable,
	containerOwns:    containerOwnsListeningPort,
	fabricBinding:    validateLocalFabricBinding,
	validateSnapshot: validatePinnedQwenSnapshot,
	computeTenants:   inspectGPUComputeTenants,
	qwenAdmission:    func(ctx context.Context) (func(), error) { return qwenGPUAdmission.acquire(ctx) },
}

var inspectGPUComputeTenantsForLaunch = inspectGPUComputeTenants

type gpuComputeTenant struct {
	PID         int
	ContainerID string
}

type qwen38SnapshotObject struct {
	blob string
	size int64
}

var qwen38SnapshotObjects = map[string]qwen38SnapshotObject{
	".gitattributes":                   {blob: "a09db2ea4d1bd1fc1c09f6fca45e8e4054953535", size: 1635},
	"README.md":                        {blob: "f3edfc5b8f2cae8ae751162626d4c5aa3ed071a6", size: 12109},
	"chat_template.jinja":              {blob: "c0c686f9c38d70d179fb7b5f5aa7530bc913dda3", size: 8952},
	"config.json":                      {blob: "d9df7636dc5b8bdab82655aba8ce0ebe25b22342", size: 30820},
	"generation_config.json":           {blob: "023756cfadf88e5bf69eefeee3e172f38c448d64", size: 202},
	"hf_quant_config.json":             {blob: "89bb1a48d2358f4b9ab7959bd4357a6d6d04020e", size: 23001},
	"merges.txt":                       {blob: "a494e019ca1502219fd0128658b979e5f05ae8e8", size: 3353259},
	"model-00001-of-00010.safetensors": {blob: "63fde954be6f08b49b876f4f70a0ad0bcfee71aff7b1faa33779b6b32feca2a2", size: 3115991696},
	"model-00002-of-00010.safetensors": {blob: "4dafaef62a908e49e0d92c7c2a3fa99f4d9651fdeeb0ca09f09a9b40095af4e0", size: 10005510304},
	"model-00003-of-00010.safetensors": {blob: "3218ddc129258e91a721a8e81329ca8f00588332ec721bd8d2cf7d20e324bf47", size: 10005364728},
	"model-00004-of-00010.safetensors": {blob: "55e2bdf6a3a1f6e65270787f65c63b2a2a56b308b54eb1fb318dfabc9b48b141", size: 10006060112},
	"model-00005-of-00010.safetensors": {blob: "d3c169d3694bfba846455d01e48f047d770f78d071c5a4d22f129389411d686e", size: 10005362392},
	"model-00006-of-00010.safetensors": {blob: "38f65a9d11090428e139cc60d0587e8e5433ea2ce79820883c583d7cfcc53130", size: 10005375328},
	"model-00007-of-00010.safetensors": {blob: "8cf3fa05c04cb2e060963b69bb01f7a7b9f43c58435fbe380836a0457a19cd3e", size: 10005956544},
	"model-00008-of-00010.safetensors": {blob: "53d1f80746aa0cc0a7c2f4836587002a8a4dd72c827432cd2e43abae3c3899e4", size: 10005375200},
	"model-00009-of-00010.safetensors": {blob: "eaff6a87ece6fbfd6ad0fbaa0a5cf9ff542f8076acba38e62410b1ed55e12cc3", size: 5679113808},
	"model-00010-of-00010.safetensors": {blob: "0d49b0cf7c15bf9b5bd1e17403b36727315860a8f2a2e0f5a53655547084f640", size: 128587536},
	"model-fp8-mtp-ple.safetensors":    {blob: "3525520c8602d850003eb1960aec0b64291dae33b83f8b65dd639a451df78823", size: 53717551730},
	"model.safetensors.index.json":     {blob: "660414e8300728ba80062e6a3c81cb76a65e2ab136ac9e9ceb450ba0c51c3c0d", size: 31275518},
	"preprocessor_config.json":         {blob: "2ea84a437d448ff71b08df68fdd949d5cc4ebb64", size: 390},
	"processor_config.json":            {blob: "33818c7f9e991ad735fd240209f4fa73e6c28c50", size: 1191},
	"tokenizer.json":                   {blob: "0997f410c57a1f4e53b09e4be8f4a172d90edd9564368fb0847030937229b9f3", size: 12809320},
	"tokenizer_config.json":            {blob: "5de744b3fca2129d7186979ae47c06be33903243", size: 17928},
	"video_preprocessor_config.json":   {blob: "3ba673a5ad7d4d13f54155ecd38b2a94a6dac8fe", size: 385},
	"vocab.json":                       {blob: "0aa0ce0658d60ac4a5d609f4eadb0e8e43514176", size: 6722759},
}

type preflightProblem struct {
	status int
	code   string
	err    error
}

func (p *preflightProblem) Error() string { return p.err.Error() }

func handleDeploymentPreflight(w http.ResponseWriter, r *http.Request) {
	var request deploymentPreflightRequest
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_preflight", err.Error())
		return
	}
	if request.BKCID == bkc.Qwen38FlashNextNVFP4DualGB10ID && !coordinatedLaunchAuthorizationAvailable() {
		writeError(w, http.StatusServiceUnavailable, "coordinator_authorization_unavailable", errCoordinatorAuthorizationUnavailable.Error())
		return
	}
	if err := validateDeploymentPreflight(r.Context(), request, liveDeploymentPreflightDeps); err != nil {
		problem, ok := err.(*preflightProblem)
		if !ok {
			writeError(w, http.StatusServiceUnavailable, "preflight_dependency", err.Error())
			return
		}
		writeError(w, problem.status, problem.code, problem.err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func validateDeploymentPreflight(ctx context.Context, request deploymentPreflightRequest, deps deploymentPreflightDeps) error {
	if strings.TrimSpace(request.FabricAddress) == "" || strings.TrimSpace(request.CandidateName) == "" {
		return preflightBadRequest("fabric_address and candidate_name are required")
	}
	if request.BKCID == bkc.Qwen38FlashNextNVFP4DualGB10ID && !request.RequireFabric {
		return preflightBadRequest("pinned Qwen3.8 deployment requires exact fabric configuration")
	}
	if request.ServicePort < 1 || request.ServicePort > 65535 || request.RendezvousPort < 1 || request.RendezvousPort > 65535 {
		return preflightBadRequest("service_port and rendezvous_port must be valid ports")
	}
	requestedAddresses := []string{request.FabricAddress}
	if request.Head {
		if strings.TrimSpace(request.ServiceAddress) == "" {
			return preflightBadRequest("head preflight requires service_address")
		}
		requestedAddresses = append(requestedAddresses, request.ServiceAddress)
	} else if request.ServiceAddress != "" {
		return preflightBadRequest("worker preflight must not define service_address")
	}
	local, err := localIPSet(deps.interfaceAddrs)
	if err != nil {
		return &preflightProblem{status: http.StatusServiceUnavailable, code: "address_inventory_unavailable", err: err}
	}
	for _, address := range requestedAddresses {
		ip := net.ParseIP(strings.TrimSpace(address))
		if ip == nil {
			return preflightBadRequest(fmt.Sprintf("invalid requested address %q", address))
		}
		if _, ok := local[ip.String()]; !ok {
			return preflightConflict(fmt.Sprintf("requested address %s does not exist locally", ip.String()))
		}
	}
	if request.RequireFabric {
		if request.FabricInterface == "" || request.FabricHCA == "" || request.FabricGIDIndex == nil || *request.FabricGIDIndex < 0 || *request.FabricGIDIndex > 255 {
			return preflightBadRequest("fabric_interface, fabric_hca, and a valid fabric_gid_index are required")
		}
		if err := deps.fabricBinding(request.FabricInterface, request.FabricHCA, *request.FabricGIDIndex, request.FabricAddress); err != nil {
			return preflightConflict(err.Error())
		}
	}
	if request.LocalModelPath != "" {
		info, err := deps.stat(request.LocalModelPath)
		if err != nil || !info.IsDir() {
			return preflightConflict("local model path is not an existing directory")
		}
		directory, err := deps.open(request.LocalModelPath)
		if err != nil {
			return preflightConflict("local model directory is not readable")
		}
		_ = directory.Close()
	}
	if request.BKCID == bkc.Qwen38FlashNextNVFP4DualGB10ID {
		if request.LocalModelPath == "" || request.ModelRevision != bkc.Qwen38FlashNextNVFP4Revision {
			return preflightBadRequest("pinned Qwen3.8 deployment requires its exact local snapshot revision")
		}
		if err := deps.validateSnapshot(request.LocalModelPath, request.ModelRevision); err != nil {
			return preflightConflict(err.Error())
		}
	}
	containers, err := deps.listContainers(InventoryScopeAll)
	if err != nil {
		return &preflightProblem{status: http.StatusServiceUnavailable, code: "container_inventory_unavailable", err: err}
	}
	for _, container := range containers {
		if container.Name == request.CandidateName {
			return preflightConflict("deterministic candidate name already exists")
		}
	}
	if request.BKCID == bkc.Qwen38FlashNextNVFP4DualGB10ID {
		acquire := deps.qwenAdmission
		if acquire == nil {
			acquire = func(ctx context.Context) (func(), error) { return qwenGPUAdmission.acquire(ctx) }
		}
		releaseAdmission, err := acquire(ctx)
		if err != nil {
			return &preflightProblem{status: http.StatusServiceUnavailable, code: "gpu_admission_unavailable", err: fmt.Errorf("wait for Qwen3.8 GPU admission: %w", err)}
		}
		defer releaseAdmission()
		tenants, err := deps.computeTenants(ctx)
		if err != nil {
			return &preflightProblem{status: http.StatusServiceUnavailable, code: "gpu_inventory_unavailable", err: fmt.Errorf("inspect active GPU compute tenants: %w", err)}
		}
		if err := validateNoActiveComputeTenants(request.ObservedContainer, containers, tenants); err != nil {
			return preflightConflict(err.Error())
		}
	}
	serviceAddress := request.FabricAddress
	if request.Head {
		serviceAddress = request.ServiceAddress
	}
	available, err := deps.portAvailable(serviceAddress, request.ServicePort)
	if err != nil {
		return &preflightProblem{status: http.StatusServiceUnavailable, code: "port_check_unavailable", err: err}
	}
	if !available {
		if request.ObservedContainer == "" {
			return preflightConflict(fmt.Sprintf("service port %d is occupied", request.ServicePort))
		}
		owned, err := deps.containerOwns(request.ObservedContainer, serviceAddress, request.ServicePort)
		if err != nil {
			return &preflightProblem{status: http.StatusServiceUnavailable, code: "port_owner_unavailable", err: err}
		}
		if !owned {
			return preflightConflict(fmt.Sprintf("service port %d is not owned by the explicitly observed container", request.ServicePort))
		}
	}
	if request.Head {
		available, err := deps.portAvailable(request.FabricAddress, request.RendezvousPort)
		if err != nil {
			return &preflightProblem{status: http.StatusServiceUnavailable, code: "port_check_unavailable", err: err}
		}
		if !available {
			if request.ObservedContainer == "" {
				return preflightConflict(fmt.Sprintf("rendezvous port %d is occupied", request.RendezvousPort))
			}
			owned, ownerErr := deps.containerOwns(request.ObservedContainer, request.FabricAddress, request.RendezvousPort)
			if ownerErr != nil {
				return &preflightProblem{status: http.StatusServiceUnavailable, code: "port_owner_unavailable", err: ownerErr}
			}
			if !owned {
				return preflightConflict(fmt.Sprintf("rendezvous port %d is not owned by the explicitly observed container", request.RendezvousPort))
			}
		}
	}
	return nil
}

func inspectGPUComputeTenants(parent context.Context) ([]gpuComputeTenant, error) {
	ctx, cancel := context.WithTimeout(parent, 5*time.Second)
	defer cancel()
	stdout := boundedCommandOutput{limit: 1 << 20}
	command := exec.CommandContext(ctx, "nvidia-smi", "--query-compute-apps=pid", "--format=csv,noheader,nounits")
	command.Stdout = &stdout
	command.Stderr = io.Discard
	if err := command.Run(); err != nil {
		if ctx.Err() != nil {
			return nil, fmt.Errorf("nvidia-smi exceeded 5 second deadline: %w", ctx.Err())
		}
		return nil, fmt.Errorf("nvidia-smi compute query failed: %w", err)
	}
	if stdout.overflow {
		return nil, fmt.Errorf("nvidia-smi compute output exceeds limit")
	}
	pids, err := parseGPUComputePIDs(stdout.buffer.Bytes())
	if err != nil {
		return nil, err
	}
	tenants := make([]gpuComputeTenant, 0, len(pids))
	for _, pid := range pids {
		cgroup, err := readBoundedFile(filepath.Join("/proc", strconv.Itoa(pid), "cgroup"), 64<<10)
		if err != nil {
			return nil, fmt.Errorf("read compute process %d cgroup: %w", pid, err)
		}
		containerID, err := dockerContainerIDFromCgroup(cgroup)
		if err != nil {
			containerID = ""
		}
		tenants = append(tenants, gpuComputeTenant{PID: pid, ContainerID: containerID})
	}
	return tenants, nil
}

type boundedCommandOutput struct {
	buffer   bytes.Buffer
	limit    int
	overflow bool
}

func (output *boundedCommandOutput) Write(data []byte) (int, error) {
	written := len(data)
	remaining := output.limit - output.buffer.Len()
	if remaining < len(data) {
		output.overflow = true
		if remaining < 0 {
			remaining = 0
		}
		data = data[:remaining]
	}
	_, _ = output.buffer.Write(data)
	return written, nil
}

func parseGPUComputePIDs(output []byte) ([]int, error) {
	if len(output) > 1<<20 {
		return nil, fmt.Errorf("nvidia-smi compute output exceeds limit")
	}
	lines := strings.Split(strings.ReplaceAll(string(output), "\r\n", "\n"), "\n")
	pids := make([]int, 0, len(lines))
	seen := make(map[int]struct{})
	for _, line := range lines {
		if line == "" {
			continue
		}
		if len(pids) >= 1024 || strings.TrimSpace(line) != line || strings.IndexFunc(line, func(r rune) bool { return r < '0' || r > '9' }) >= 0 {
			return nil, fmt.Errorf("nvidia-smi returned malformed compute PID output")
		}
		pid64, err := strconv.ParseInt(line, 10, 31)
		if err != nil || pid64 < 1 {
			return nil, fmt.Errorf("nvidia-smi returned invalid compute PID")
		}
		pid := int(pid64)
		if _, duplicate := seen[pid]; duplicate {
			continue
		}
		seen[pid] = struct{}{}
		pids = append(pids, pid)
	}
	return pids, nil
}

func readBoundedFile(path string, limit int64) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("file exceeds %d byte limit", limit)
	}
	return data, nil
}

var dockerCgroupContainerIDPattern = regexp.MustCompile(`(?:^|[/:.-])([a-f0-9]{64})(?:$|[/.])`)

func dockerContainerIDFromCgroup(data []byte) (string, error) {
	var found string
	for _, line := range strings.Split(string(data), "\n") {
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, ":", 3)
		if len(parts) != 3 {
			return "", fmt.Errorf("malformed process cgroup")
		}
		match := dockerCgroupContainerIDPattern.FindStringSubmatch(parts[2])
		if len(match) != 2 {
			continue
		}
		if found != "" && found != match[1] {
			return "", fmt.Errorf("ambiguous Docker process cgroup")
		}
		found = match[1]
	}
	if found == "" {
		return "", fmt.Errorf("compute process is not in a Docker container")
	}
	return found, nil
}

func validateNoActiveComputeTenants(observedSelector string, containers []Container, tenants []gpuComputeTenant) error {
	if len(tenants) == 0 {
		return nil
	}
	var allowedID string
	if observedSelector != "" {
		for _, container := range containers {
			matches := container.ID == observedSelector || container.Name == observedSelector || (len(observedSelector) >= 12 && strings.HasPrefix(container.ID, observedSelector))
			if !matches {
				continue
			}
			if allowedID != "" {
				return fmt.Errorf("active compute tenant cannot be attributed to an unambiguous rollback owner")
			}
			if container.Labels[LabelManaged] != "true" || container.Labels[LabelOwnership] != OwnershipManaged || container.Labels[LabelDeploymentID] != "" {
				break
			}
			allowedID = container.ID
		}
	}
	for _, tenant := range tenants {
		if allowedID == "" || tenant.ContainerID != allowedID {
			return fmt.Errorf("active compute tenant PID %d is not the exact Yokai-managed rollback owner", tenant.PID)
		}
	}
	return nil
}

var localFabricNamePattern = regexp.MustCompile(`^[A-Za-z0-9_.:-]{1,64}$`)

func validateLocalFabricBinding(interfaceName, hcaName string, gidIndex int, address string) error {
	if !localFabricNamePattern.MatchString(interfaceName) || !localFabricNamePattern.MatchString(hcaName) || strings.HasPrefix(hcaName, "=") {
		return fmt.Errorf("invalid fabric interface or HCA name")
	}
	iface, err := net.InterfaceByName(interfaceName)
	if err != nil {
		return fmt.Errorf("fabric interface %s does not exist", interfaceName)
	}
	addresses, err := iface.Addrs()
	if err != nil {
		return fmt.Errorf("inspect fabric interface %s: %w", interfaceName, err)
	}
	want := net.ParseIP(address)
	found := false
	for _, candidate := range addresses {
		value := strings.SplitN(candidate.String(), "/", 2)[0]
		if ip := net.ParseIP(value); ip != nil && ip.Equal(want) {
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("fabric address %s is not assigned to interface %s", address, interfaceName)
	}
	if _, err := os.Stat(filepath.Join("/sys/class/infiniband", hcaName, "device", "net", interfaceName)); err != nil {
		return fmt.Errorf("fabric HCA %s is not associated with interface %s", hcaName, interfaceName)
	}
	gids, err := filepath.Glob(filepath.Join("/sys/class/infiniband", hcaName, "ports", "*", "gids", strconv.Itoa(gidIndex)))
	if err != nil || len(gids) == 0 {
		return fmt.Errorf("fabric HCA %s has no GID index %d", hcaName, gidIndex)
	}
	for _, gid := range gids {
		gidData, gidErr := os.ReadFile(gid)
		portDirectory := filepath.Dir(filepath.Dir(gid))
		ndevData, ndevErr := os.ReadFile(filepath.Join(portDirectory, "gid_attrs", "ndevs", strconv.Itoa(gidIndex)))
		if gidErr == nil && ndevErr == nil && validateFabricGIDValue(interfaceName, address, gidData, ndevData) == nil {
			return nil
		}
	}
	return fmt.Errorf("fabric HCA %s GID index %d does not map address %s to interface %s", hcaName, gidIndex, address, interfaceName)
}

func validateFabricGIDValue(interfaceName, address string, gidData, ndevData []byte) error {
	if strings.TrimSpace(string(ndevData)) != interfaceName {
		return fmt.Errorf("GID network device does not match interface %s", interfaceName)
	}
	want := net.ParseIP(strings.TrimSpace(address))
	got := net.ParseIP(strings.TrimSpace(string(gidData)))
	if want == nil || got == nil || !got.Equal(want) {
		return fmt.Errorf("GID does not encode fabric address %s", address)
	}
	return nil
}

func validatePinnedQwenSnapshot(snapshotPath, revision string) error {
	clean := filepath.Clean(snapshotPath)
	repositoryRoot := filepath.Dir(filepath.Dir(clean))
	if filepath.Base(clean) != revision || filepath.Base(filepath.Dir(clean)) != "snapshots" || filepath.Base(repositoryRoot) != bkc.Qwen38FlashNextHFCacheDirectory {
		return fmt.Errorf("local snapshot is not the standard pinned Hugging Face cache path")
	}
	resolvedRoot, err := filepath.EvalSymlinks(repositoryRoot)
	if err != nil {
		return fmt.Errorf("resolve pinned Hugging Face repository: %w", err)
	}
	if err := validateQwen38SnapshotObjectSet(clean); err != nil {
		return err
	}
	for name, object := range qwen38SnapshotObjects {
		if err := validateQwen38SnapshotObject(clean, resolvedRoot, name, object); err != nil {
			return err
		}
	}
	for _, patch := range bkc.Qwen38RuntimePatches() {
		name := filepath.Base(patch.SourcePath)
		if name != "config.json" && name != "hf_quant_config.json" {
			continue
		}
		data, err := os.ReadFile(filepath.Join(snapshotPath, name))
		if err != nil {
			return fmt.Errorf("read pinned snapshot %s: %w", name, err)
		}
		if fmt.Sprintf("%x", sha256.Sum256(data)) != patch.OriginalSHA256 {
			return fmt.Errorf("pinned snapshot %s hash mismatch", name)
		}
	}
	indexData, err := os.ReadFile(filepath.Join(clean, "model.safetensors.index.json"))
	if err != nil {
		return fmt.Errorf("read pinned snapshot weight index: %w", err)
	}
	var index struct {
		WeightMap map[string]string `json:"weight_map"`
	}
	if err := json.Unmarshal(indexData, &index); err != nil || len(index.WeightMap) == 0 {
		return fmt.Errorf("pinned snapshot has an invalid weight index")
	}
	seen := make(map[string]struct{})
	for _, relative := range index.WeightMap {
		if _, ok := seen[relative]; ok {
			continue
		}
		seen[relative] = struct{}{}
		clean := filepath.Clean(relative)
		if clean != relative || filepath.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
			return fmt.Errorf("pinned snapshot weight index contains an unsafe path")
		}
		if _, expected := qwen38SnapshotObjects[clean]; !expected || !strings.HasSuffix(clean, ".safetensors") {
			return fmt.Errorf("pinned snapshot weight index references unexpected shard %s", clean)
		}
	}
	expectedShardCount := 0
	for name := range qwen38SnapshotObjects {
		if strings.HasSuffix(name, ".safetensors") {
			expectedShardCount++
			if _, ok := seen[name]; !ok {
				return fmt.Errorf("pinned snapshot weight index omits shard %s", name)
			}
		}
	}
	if len(seen) != expectedShardCount {
		return fmt.Errorf("pinned snapshot weight index shard set mismatch")
	}
	return nil
}

func validateQwen38SnapshotObjectSet(snapshotPath string) error {
	entries, err := os.ReadDir(snapshotPath)
	if err != nil {
		return fmt.Errorf("read pinned snapshot object set: %w", err)
	}
	if len(entries) != len(qwen38SnapshotObjects) {
		return fmt.Errorf("pinned snapshot object set has %d entries, expected %d", len(entries), len(qwen38SnapshotObjects))
	}
	for _, entry := range entries {
		if _, ok := qwen38SnapshotObjects[entry.Name()]; !ok {
			return fmt.Errorf("pinned snapshot contains unexpected object %s", entry.Name())
		}
	}
	return nil
}

func validateQwen38SnapshotObject(snapshotPath, resolvedRoot, name string, object qwen38SnapshotObject) error {
	resolved, err := resolveQwen38SnapshotObject(snapshotPath, resolvedRoot, name, object)
	if err != nil {
		return err
	}
	if err := verifyQwen38SnapshotObjectDigest(resolved, object); err != nil {
		return fmt.Errorf("pinned snapshot object %s hash mismatch: %w", name, err)
	}
	return nil
}

func resolveQwen38SnapshotObject(snapshotPath, resolvedRoot, name string, object qwen38SnapshotObject) (string, error) {
	path := filepath.Join(snapshotPath, name)
	info, err := os.Lstat(path)
	if err != nil || info.Mode()&os.ModeSymlink == 0 {
		return "", fmt.Errorf("pinned snapshot object %s is not a Hugging Face cache symlink", name)
	}
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", fmt.Errorf("resolve pinned snapshot object %s: %w", name, err)
	}
	blobRoot := filepath.Join(resolvedRoot, "blobs")
	relative, err := filepath.Rel(blobRoot, resolved)
	if err != nil || filepath.Dir(relative) != "." || filepath.Base(relative) != object.blob {
		return "", fmt.Errorf("pinned snapshot object %s does not reference expected cache blob", name)
	}
	target, err := os.Lstat(resolved)
	if err != nil || !target.Mode().IsRegular() || target.Size() != object.size {
		return "", fmt.Errorf("pinned snapshot object %s has unexpected size or type", name)
	}
	return resolved, nil
}

func verifyQwen38SnapshotObjectDigest(path string, object qwen38SnapshotObject) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	return verifyQwen38SnapshotObjectFile(file, object)
}

func verifyQwen38SnapshotObjectFile(file *os.File, object qwen38SnapshotObject) error {
	var digest hash.Hash
	switch len(object.blob) {
	case sha1.Size * 2:
		digest = sha1.New() // #nosec G505 -- Hugging Face uses Git blob object IDs for non-LFS files.
		_, _ = fmt.Fprintf(digest, "blob %d\x00", object.size)
	case sha256.Size * 2:
		digest = sha256.New()
	default:
		return fmt.Errorf("unsupported expected digest")
	}
	if _, err := io.Copy(digest, file); err != nil {
		return err
	}
	if fmt.Sprintf("%x", digest.Sum(nil)) != object.blob {
		return fmt.Errorf("content digest does not match %s", object.blob)
	}
	return nil
}

func preflightBadRequest(message string) error {
	return &preflightProblem{status: http.StatusBadRequest, code: "invalid_preflight", err: fmt.Errorf("%s", message)}
}

func preflightConflict(message string) error {
	return &preflightProblem{status: http.StatusConflict, code: "preflight_conflict", err: fmt.Errorf("%s", message)}
}

func localIPSet(get func() ([]net.Addr, error)) (map[string]struct{}, error) {
	addresses, err := get()
	if err != nil {
		return nil, err
	}
	result := make(map[string]struct{})
	for _, address := range addresses {
		value := address.String()
		if host, _, splitErr := net.SplitHostPort(value); splitErr == nil {
			value = host
		} else if slash := strings.IndexByte(value, '/'); slash >= 0 {
			value = value[:slash]
		}
		if ip := net.ParseIP(strings.Trim(value, "[]")); ip != nil {
			result[ip.String()] = struct{}{}
		}
	}
	return result, nil
}

func tcpPortAvailable(address string, port int) (bool, error) {
	listener, err := net.Listen("tcp", net.JoinHostPort(address, strconv.Itoa(port)))
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "address already in use") {
			return false, nil
		}
		return false, err
	}
	return true, listener.Close()
}

var (
	socketPIDPattern         = regexp.MustCompile(`pid=(\d+)`)
	dockerScopeCgroupPattern = regexp.MustCompile(`(?:^|[/\s])docker-([[:xdigit:]]{12,64})\.scope(?:\s|$)`)
	dockerContainerIDPattern = regexp.MustCompile(`^[[:xdigit:]]{64}$`)
)

type containerPortOwnershipDeps struct {
	inspectExactID      func(string) (string, error)
	containerPIDs       func(string) (map[string]struct{}, error)
	listeningSocketData func(string, int) ([]byte, error)
	prefixIsUnambiguous func(string, string) (bool, error)
}

var liveContainerPortOwnershipDeps = containerPortOwnershipDeps{
	inspectExactID:      inspectExactContainerID,
	containerPIDs:       containerProcessPIDs,
	listeningSocketData: listeningSocketData,
	prefixIsUnambiguous: dockerIDPrefixIsUnambiguous,
}

func containerOwnsListeningPort(containerID, address string, port int) (bool, error) {
	return containerOwnsListeningPortWithDeps(containerID, address, port, liveContainerPortOwnershipDeps)
}

func containerOwnsListeningPortWithDeps(containerID, address string, port int, deps containerPortOwnershipDeps) (bool, error) {
	exactID, err := deps.inspectExactID(containerID)
	if err != nil {
		return false, fmt.Errorf("inspect observed container identity: %w", err)
	}
	output, err := deps.listeningSocketData(address, port)
	if err != nil {
		return false, fmt.Errorf("inspect listening port owner: %w", err)
	}
	owned, err := socketCgroupOwnedByContainer(output, exactID, deps.prefixIsUnambiguous)
	if err != nil || owned {
		return owned, err
	}
	pids, err := deps.containerPIDs(exactID)
	if err != nil {
		return false, fmt.Errorf("inspect observed container processes: %w", err)
	}
	return socketPIDOwnedByContainer(output, pids), nil
}

func inspectExactContainerID(containerID string) (string, error) {
	output, err := exec.Command("docker", "inspect", "--format={{.Id}}", containerID).Output()
	if err != nil {
		return "", err
	}
	exactID := strings.ToLower(strings.TrimSpace(string(output)))
	if !dockerContainerIDPattern.MatchString(exactID) {
		return "", fmt.Errorf("docker inspect returned invalid container identity")
	}
	return exactID, nil
}

func containerProcessPIDs(containerID string) (map[string]struct{}, error) {
	top, err := exec.Command("docker", "top", containerID, "-eo", "pid").Output()
	if err != nil {
		return nil, err
	}
	pids := make(map[string]struct{})
	for index, field := range strings.Fields(string(top)) {
		if index == 0 && strings.EqualFold(field, "pid") {
			continue
		}
		if _, err := strconv.Atoi(field); err == nil {
			pids[field] = struct{}{}
		}
	}
	return pids, nil
}

func listeningSocketData(address string, port int) ([]byte, error) {
	output, err := exec.Command("ss", "-H", "-ltnpe", "sport = :"+strconv.Itoa(port)).CombinedOutput()
	if err != nil {
		return nil, err
	}
	return filterSocketDataByEndpoint(output, address, port), nil
}

func filterSocketDataByEndpoint(output []byte, address string, port int) []byte {
	var matched []string
	for _, line := range strings.Split(string(output), "\n") {
		if socketLineMatchesEndpoint(line, address, port) {
			matched = append(matched, line)
		}
	}
	return []byte(strings.Join(matched, "\n"))
}

func socketLineMatchesEndpoint(line, address string, port int) bool {
	fields := strings.Fields(line)
	if len(fields) < 4 {
		return false
	}
	local := fields[3]
	wantPort := strconv.Itoa(port)
	separator := strings.LastIndex(local, ":")
	if separator < 0 || local[separator+1:] != wantPort {
		return false
	}
	host := strings.Trim(local[:separator], "[]")
	if host == "*" || host == "0.0.0.0" || host == "::" {
		return true
	}
	got := net.ParseIP(host)
	want := net.ParseIP(strings.Trim(address, "[]"))
	return got != nil && want != nil && got.Equal(want)
}

func socketDataOwnedByContainer(output []byte, exactID string, pids map[string]struct{}, prefixIsUnambiguous func(string, string) (bool, error)) (bool, error) {
	owned, err := socketCgroupOwnedByContainer(output, exactID, prefixIsUnambiguous)
	if err != nil || owned {
		return owned, err
	}
	return socketPIDOwnedByContainer(output, pids), nil
}

func socketCgroupOwnedByContainer(output []byte, exactID string, prefixIsUnambiguous func(string, string) (bool, error)) (bool, error) {
	exactID = strings.ToLower(strings.TrimSpace(exactID))
	for _, match := range dockerScopeCgroupPattern.FindAllSubmatch(output, -1) {
		candidate := strings.ToLower(string(match[1]))
		if candidate == exactID {
			return true, nil
		}
		if len(candidate) < 12 || !strings.HasPrefix(exactID, candidate) {
			continue
		}
		matched, err := prefixIsUnambiguous(candidate, exactID)
		if err != nil {
			return false, err
		}
		if matched {
			return true, nil
		}
	}
	return false, nil
}

func socketPIDOwnedByContainer(output []byte, pids map[string]struct{}) bool {
	// Some ss builds expose users/pid but not cgroup metadata. Keep this as a
	// fallback only after all extended cgroup identities have been checked.
	for _, match := range socketPIDPattern.FindAllStringSubmatch(string(output), -1) {
		if _, ok := pids[match[1]]; ok {
			return true
		}
	}
	return false
}

func dockerIDPrefixIsUnambiguous(prefix, exactID string) (bool, error) {
	if len(prefix) < 12 || !strings.HasPrefix(exactID, prefix) {
		return false, nil
	}
	output, err := exec.Command("docker", "ps", "-aq", "--no-trunc", "--filter", "id="+prefix).Output()
	if err != nil {
		return false, err
	}
	matches := strings.Fields(strings.ToLower(string(output)))
	return len(matches) == 1 && matches[0] == exactID, nil
}
