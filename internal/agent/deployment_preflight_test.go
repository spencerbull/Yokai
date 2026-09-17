package agent

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/spencerbull/yokai/internal/bkc"
)

func TestQwenDeploymentPreflightFailsClosedWithoutLaunchAuthorizationState(t *testing.T) {
	originalPublicKey := coordinatorVerificationKey
	originalDeviceID := agentDeviceID
	originalReplayStore := launchAuthorizationReplayStore
	coordinatorVerificationKey = nil
	agentDeviceID = ""
	launchAuthorizationReplayStore = nil
	defer func() {
		coordinatorVerificationKey = originalPublicKey
		agentDeviceID = originalDeviceID
		launchAuthorizationReplayStore = originalReplayStore
	}()

	body, err := json.Marshal(deploymentPreflightRequest{BKCID: bkc.Qwen38FlashNextNVFP4DualGB10ID})
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/deployments/preflight", bytes.NewReader(body))
	recorder := httptest.NewRecorder()
	handleDeploymentPreflight(recorder, request)
	if recorder.Code != http.StatusServiceUnavailable || !strings.Contains(recorder.Body.String(), "coordinator_authorization_unavailable") {
		t.Fatalf("Qwen preflight did not fail closed on absent authorization state: status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestDeploymentPreflightValidatesLocalPathAddressesAndPorts(t *testing.T) {
	modelDir := t.TempDir()
	request := deploymentPreflightRequest{
		FabricAddress: "192.168.201.1", ServiceAddress: "100.96.0.20", ServicePort: 8000,
		RendezvousPort: 25000, Head: true, CandidateName: "yokai-deployment-test-head", LocalModelPath: modelDir,
	}
	deps := deploymentPreflightDeps{
		interfaceAddrs: func() ([]net.Addr, error) {
			return []net.Addr{testAddr("192.168.201.1/24"), testAddr("100.96.0.20/32")}, nil
		},
		stat: os.Stat, open: os.Open,
		listContainers: func(string) ([]Container, error) { return nil, nil },
		portAvailable:  func(string, int) (bool, error) { return true, nil },
		containerOwns:  func(string, string, int) (bool, error) { return false, nil },
	}
	if err := validateDeploymentPreflight(context.Background(), request, deps); err != nil {
		t.Fatalf("valid preflight failed: %v", err)
	}

	request.LocalModelPath = modelDir + "/missing"
	if err := validateDeploymentPreflight(context.Background(), request, deps); err == nil {
		t.Fatal("missing model directory passed preflight")
	}
	request.LocalModelPath = modelDir
	request.ServiceAddress = "100.96.0.21"
	if err := validateDeploymentPreflight(context.Background(), request, deps); err == nil {
		t.Fatal("non-local service address passed preflight")
	}
}

func TestDeploymentPreflightRequiresObservedContainerToOwnOccupiedServicePort(t *testing.T) {
	request := deploymentPreflightRequest{
		FabricAddress: "192.168.201.2", ServiceAddress: "100.96.0.20", ServicePort: 8000, RendezvousPort: 25000,
		Head: true, CandidateName: "yokai-deployment-test-head", ObservedContainer: "1234567890ab",
	}
	owned := false
	deps := deploymentPreflightDeps{
		interfaceAddrs: func() ([]net.Addr, error) {
			return []net.Addr{testAddr("192.168.201.2/24"), testAddr("100.96.0.20/32")}, nil
		},
		stat: os.Stat, open: os.Open,
		listContainers: func(string) ([]Container, error) { return nil, nil },
		portAvailable:  func(_ string, port int) (bool, error) { return port == 25000, nil },
		containerOwns: func(id, address string, port int) (bool, error) {
			return owned && id == "1234567890ab" && address == "100.96.0.20" && port == 8000, nil
		},
	}
	if err := validateDeploymentPreflight(context.Background(), request, deps); err == nil {
		t.Fatal("unowned occupied service port passed preflight")
	}
	owned = true
	if err := validateDeploymentPreflight(context.Background(), request, deps); err != nil {
		t.Fatalf("explicitly owned service port was rejected: %v", err)
	}
}

func TestWorkerPreflightValidatesOnlyRendezvousPort(t *testing.T) {
	request := deploymentPreflightRequest{
		FabricAddress: "192.168.201.2", ServicePort: 8888, RendezvousPort: 50000,
		CandidateName: "yokai-deployment-test-worker",
	}
	available := true
	owned := false
	var portChecks []string
	var ownershipChecks []string
	deps := deploymentPreflightDeps{
		interfaceAddrs: func() ([]net.Addr, error) { return []net.Addr{testAddr("192.168.201.2/24")}, nil },
		stat:           os.Stat, open: os.Open,
		listContainers: func(string) ([]Container, error) { return nil, nil },
		portAvailable: func(address string, port int) (bool, error) {
			portChecks = append(portChecks, fmt.Sprintf("%s:%d", address, port))
			return available, nil
		},
		containerOwns: func(id, address string, port int) (bool, error) {
			ownershipChecks = append(ownershipChecks, fmt.Sprintf("%s@%s:%d", id, address, port))
			return owned, nil
		},
	}

	if err := validateDeploymentPreflight(context.Background(), request, deps); err != nil {
		t.Fatalf("worker with available rendezvous port failed preflight: %v", err)
	}
	if len(portChecks) != 1 || portChecks[0] != "192.168.201.2:50000" {
		t.Fatalf("worker port checks = %v, want only its fabric rendezvous address and port", portChecks)
	}

	available = false
	portChecks = nil
	if err := validateDeploymentPreflight(context.Background(), request, deps); err == nil || !strings.Contains(err.Error(), "rendezvous port 50000 is occupied") {
		t.Fatalf("worker accepted occupied rendezvous port without observed ownership: %v", err)
	}
	if len(portChecks) != 1 || portChecks[0] != "192.168.201.2:50000" {
		t.Fatalf("worker occupied-port checks = %v, want only its fabric rendezvous address and port", portChecks)
	}
	if len(ownershipChecks) != 0 {
		t.Fatalf("worker checked ownership without an explicitly observed container: %v", ownershipChecks)
	}

	request.ObservedContainer = "1234567890ab"
	if err := validateDeploymentPreflight(context.Background(), request, deps); err == nil || !strings.Contains(err.Error(), "rendezvous port 50000 is not owned") {
		t.Fatalf("worker accepted occupied rendezvous port owned by another process: %v", err)
	}
	if len(ownershipChecks) != 1 || ownershipChecks[0] != "1234567890ab@192.168.201.2:50000" {
		t.Fatalf("worker ownership checks = %v, want observed container on its fabric rendezvous address and port", ownershipChecks)
	}

	owned = true
	if err := validateDeploymentPreflight(context.Background(), request, deps); err != nil {
		t.Fatalf("worker rejected rendezvous port owned by the explicitly observed container: %v", err)
	}
}

func TestDeploymentPreflightRejectsCandidateCollisionAndOccupiedRendezvous(t *testing.T) {
	request := deploymentPreflightRequest{
		FabricAddress: "192.168.201.1", ServiceAddress: "100.96.0.20", ServicePort: 8000,
		RendezvousPort: 25000, Head: true, CandidateName: "yokai-deployment-test-head",
	}
	deps := deploymentPreflightDeps{
		interfaceAddrs: func() ([]net.Addr, error) {
			return []net.Addr{testAddr("192.168.201.1/24"), testAddr("100.96.0.20/32")}, nil
		},
		stat: os.Stat, open: os.Open,
		listContainers: func(string) ([]Container, error) { return []Container{{Name: request.CandidateName}}, nil },
		portAvailable:  func(string, int) (bool, error) { return true, nil },
		containerOwns:  func(string, string, int) (bool, error) { return false, nil },
	}
	if err := validateDeploymentPreflight(context.Background(), request, deps); err == nil {
		t.Fatal("candidate name collision passed preflight")
	}
	deps.listContainers = func(string) ([]Container, error) { return nil, nil }
	deps.portAvailable = func(_ string, port int) (bool, error) { return port != 25000, nil }
	if err := validateDeploymentPreflight(context.Background(), request, deps); err == nil {
		t.Fatal("occupied rendezvous port passed preflight")
	}
}

func TestPinnedSnapshotRejectsSameSizeTamperedLFSBlob(t *testing.T) {
	repositoryRoot := t.TempDir()
	snapshotPath := filepath.Join(repositoryRoot, "snapshots", "revision")
	blobsPath := filepath.Join(repositoryRoot, "blobs")
	if err := os.MkdirAll(snapshotPath, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(blobsPath, 0700); err != nil {
		t.Fatal(err)
	}
	original := []byte("original-weight-bytes")
	tampered := []byte("tampered-weight-bytes")
	if len(original) != len(tampered) {
		t.Fatal("test fixture must preserve blob size")
	}
	digest := fmt.Sprintf("%x", sha256.Sum256(original))
	blobPath := filepath.Join(blobsPath, digest)
	if err := os.WriteFile(blobPath, tampered, 0600); err != nil {
		t.Fatal(err)
	}
	name := "model.safetensors"
	if err := os.Symlink(filepath.Join("..", "..", "blobs", digest), filepath.Join(snapshotPath, name)); err != nil {
		t.Fatal(err)
	}
	err := validateQwen38SnapshotObject(snapshotPath, repositoryRoot, name, qwen38SnapshotObject{blob: digest, size: int64(len(original))})
	if err == nil || !strings.Contains(err.Error(), "hash mismatch") {
		t.Fatalf("same-size tampered LFS blob was accepted: %v", err)
	}
}

func TestPinnedQwenSnapshotManifestExactlyMatchesUpstreamRevision(t *testing.T) {
	expected := []string{
		".gitattributes",
		"README.md",
		"chat_template.jinja",
		"config.json",
		"generation_config.json",
		"hf_quant_config.json",
		"merges.txt",
		"model-00001-of-00010.safetensors",
		"model-00002-of-00010.safetensors",
		"model-00003-of-00010.safetensors",
		"model-00004-of-00010.safetensors",
		"model-00005-of-00010.safetensors",
		"model-00006-of-00010.safetensors",
		"model-00007-of-00010.safetensors",
		"model-00008-of-00010.safetensors",
		"model-00009-of-00010.safetensors",
		"model-00010-of-00010.safetensors",
		"model-fp8-mtp-ple.safetensors",
		"model.safetensors.index.json",
		"preprocessor_config.json",
		"processor_config.json",
		"tokenizer.json",
		"tokenizer_config.json",
		"video_preprocessor_config.json",
		"vocab.json",
	}
	if len(qwen38SnapshotObjects) != len(expected) {
		t.Fatalf("pinned manifest has %d objects, upstream revision has %d", len(qwen38SnapshotObjects), len(expected))
	}
	for _, name := range expected {
		if _, ok := qwen38SnapshotObjects[name]; !ok {
			t.Errorf("pinned manifest omits upstream object %s", name)
		}
	}
	if object := qwen38SnapshotObjects[".gitattributes"]; object != (qwen38SnapshotObject{blob: "a09db2ea4d1bd1fc1c09f6fca45e8e4054953535", size: 1635}) {
		t.Errorf(".gitattributes provenance mismatch: %#v", object)
	}
	if object := qwen38SnapshotObjects["README.md"]; object != (qwen38SnapshotObject{blob: "f3edfc5b8f2cae8ae751162626d4c5aa3ed071a6", size: 12109}) {
		t.Errorf("README.md provenance mismatch: %#v", object)
	}
}

func TestPinnedQwenSnapshotObjectSetRejectsMissingAndUnexpectedEntries(t *testing.T) {
	snapshot := t.TempDir()
	for name := range qwen38SnapshotObjects {
		if err := os.WriteFile(filepath.Join(snapshot, name), nil, 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err := validateQwen38SnapshotObjectSet(snapshot); err != nil {
		t.Fatalf("exact pinned object set was rejected: %v", err)
	}
	if err := os.Remove(filepath.Join(snapshot, "README.md")); err != nil {
		t.Fatal(err)
	}
	if err := validateQwen38SnapshotObjectSet(snapshot); err == nil || !strings.Contains(err.Error(), "object set") {
		t.Fatalf("missing pinned object passed exact-set validation: %v", err)
	}
	if err := os.WriteFile(filepath.Join(snapshot, "README.md"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(snapshot, "unexpected.txt"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	if err := validateQwen38SnapshotObjectSet(snapshot); err == nil || !strings.Contains(err.Error(), "object set") {
		t.Fatalf("unexpected snapshot object passed exact-set validation: %v", err)
	}
}

func TestQwenPreflightRejectsActiveComputeTenantAndAllowsExactManagedRollbackOwner(t *testing.T) {
	containerID := strings.Repeat("a", 64)
	request := deploymentPreflightRequest{
		FabricAddress: "192.168.201.2", FabricInterface: "enp1s0", FabricHCA: "roce0", FabricGIDIndex: intPointer(3), RequireFabric: true,
		ServicePort: 8888, RendezvousPort: 50000, CandidateName: "candidate-worker", LocalModelPath: t.TempDir(),
		BKCID: bkc.Qwen38FlashNextNVFP4DualGB10ID, ModelRevision: bkc.Qwen38FlashNextNVFP4Revision,
	}
	containers := []Container{{ID: containerID, Name: "old-yokai", Labels: map[string]string{LabelManaged: "true", LabelOwnership: OwnershipManaged}, Ownership: OwnershipManaged}}
	deps := deploymentPreflightDeps{
		interfaceAddrs: func() ([]net.Addr, error) { return []net.Addr{testAddr("192.168.201.2/24")}, nil },
		stat:           os.Stat, open: os.Open,
		listContainers: func(string) ([]Container, error) { return containers, nil },
		portAvailable:  func(string, int) (bool, error) { return true, nil },
		containerOwns:  func(string, string, int) (bool, error) { return false, nil },
		fabricBinding:  func(string, string, int, string) error { return nil },
		validateSnapshot: func(string, string) error {
			return nil
		},
		computeTenants: func(context.Context) ([]gpuComputeTenant, error) {
			return []gpuComputeTenant{{PID: 4242, ContainerID: containerID}}, nil
		},
	}
	if err := validateDeploymentPreflight(context.Background(), request, deps); err == nil || !strings.Contains(err.Error(), "active compute tenant") {
		t.Fatalf("unselected active compute tenant passed Qwen preflight: %v", err)
	}
	request.ObservedContainer = containerID
	if err := validateDeploymentPreflight(context.Background(), request, deps); err != nil {
		t.Fatalf("exact Yokai-managed rollback owner was rejected: %v", err)
	}
	containers[0].Labels[LabelOwnership] = OwnershipObserved
	containers[0].Ownership = OwnershipObserved
	if err := validateDeploymentPreflight(context.Background(), request, deps); err == nil || !strings.Contains(err.Error(), "active compute tenant") {
		t.Fatalf("external active compute tenant passed as rollback owner: %v", err)
	}
}

func TestQwenPreflightUsesAgentAdmissionLockForTenantCheck(t *testing.T) {
	release, err := qwenGPUAdmission.acquire(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	request := deploymentPreflightRequest{
		FabricAddress: "192.168.201.2", FabricInterface: "enp1s0", FabricHCA: "roce0", FabricGIDIndex: intPointer(3), RequireFabric: true,
		ServicePort: 8888, RendezvousPort: 50000, CandidateName: "candidate-worker", LocalModelPath: t.TempDir(),
		BKCID: bkc.Qwen38FlashNextNVFP4DualGB10ID, ModelRevision: bkc.Qwen38FlashNextNVFP4Revision,
	}
	checkEntered := make(chan struct{}, 1)
	deps := deploymentPreflightDeps{
		interfaceAddrs: func() ([]net.Addr, error) { return []net.Addr{testAddr("192.168.201.2/24")}, nil },
		stat:           os.Stat,
		open:           os.Open,
		listContainers: func(string) ([]Container, error) { return nil, nil },
		portAvailable:  func(string, int) (bool, error) { return true, nil },
		containerOwns:  func(string, string, int) (bool, error) { return false, nil },
		fabricBinding:  func(string, string, int, string) error { return nil },
		validateSnapshot: func(string, string) error {
			return nil
		},
		computeTenants: func(context.Context) ([]gpuComputeTenant, error) {
			checkEntered <- struct{}{}
			return nil, nil
		},
	}
	done := make(chan error, 1)
	go func() { done <- validateDeploymentPreflight(context.Background(), request, deps) }()
	select {
	case <-checkEntered:
		t.Fatal("Qwen preflight tenant check bypassed the agent admission lock")
	case <-time.After(30 * time.Millisecond):
	}
	release()
	select {
	case <-checkEntered:
	case <-time.After(time.Second):
		t.Fatal("Qwen preflight did not resume after admission was released")
	}
	if err := <-done; err != nil {
		t.Fatalf("valid Qwen preflight failed after admission release: %v", err)
	}
}

func TestGPUComputeTenantParsingIsStrict(t *testing.T) {
	pids, err := parseGPUComputePIDs([]byte("17\n2048\r\n"))
	if err != nil || len(pids) != 2 || pids[0] != 17 || pids[1] != 2048 {
		t.Fatalf("valid nvidia-smi PID output failed: pids=%v err=%v", pids, err)
	}
	for _, unsafe := range [][]byte{[]byte("17, python\n"), []byte("pid\n"), []byte("-1\n"), []byte("0\n"), []byte("17\x00\n")} {
		if _, err := parseGPUComputePIDs(unsafe); err == nil {
			t.Fatalf("unsafe nvidia-smi output was accepted: %q", unsafe)
		}
	}
	exactID := strings.Repeat("b", 64)
	got, err := dockerContainerIDFromCgroup([]byte("0::/system.slice/docker-" + exactID + ".scope\n"))
	if err != nil || got != exactID {
		t.Fatalf("valid Docker cgroup was not resolved: id=%q err=%v", got, err)
	}
	if _, err := dockerContainerIDFromCgroup([]byte("0::/user.slice/user-1000.slice\n")); err == nil {
		t.Fatal("host process cgroup was accepted as a Docker tenant")
	}
}

func TestGPUComputeTenantQueryHonorsCallerDeadline(t *testing.T) {
	binDir := t.TempDir()
	commandPath := filepath.Join(binDir, "nvidia-smi")
	if err := os.WriteFile(commandPath, []byte("#!/bin/sh\nexec sleep 30\n"), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	started := time.Now()
	if _, err := inspectGPUComputeTenants(ctx); err == nil || !strings.Contains(err.Error(), "deadline") {
		t.Fatalf("bounded nvidia-smi query did not report its deadline: %v", err)
	}
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Fatalf("nvidia-smi query exceeded bounded execution: %s", elapsed)
	}
}

func TestFabricGIDMustEncodeConfiguredAddressOnConfiguredInterface(t *testing.T) {
	if err := validateFabricGIDValue("enp1s0", "192.168.201.1", []byte("0000:0000:0000:0000:0000:ffff:c0a8:c901\n"), []byte("enp1s0\n")); err != nil {
		t.Fatalf("matching IPv4-mapped RoCE GID was rejected: %v", err)
	}
	for name, test := range map[string]struct {
		address string
		gid     string
		ndev    string
	}{
		"different address": {address: "192.168.201.1", gid: "0000:0000:0000:0000:0000:ffff:c0a8:c902", ndev: "enp1s0"},
		"different device":  {address: "192.168.201.1", gid: "0000:0000:0000:0000:0000:ffff:c0a8:c901", ndev: "enp2s0"},
		"non-IP GID":        {address: "192.168.201.1", gid: "fe80:0000:0000:0000:1234:5678:90ab:cdef", ndev: "enp1s0"},
	} {
		t.Run(name, func(t *testing.T) {
			if err := validateFabricGIDValue("enp1s0", test.address, []byte(test.gid+"\n"), []byte(test.ndev+"\n")); err == nil {
				t.Fatal("mismatched fabric GID binding was accepted")
			}
		})
	}
}

func intPointer(value int) *int { return &value }

func TestContainerOwnsListeningPortUsesDockerCgroup(t *testing.T) {
	exactID := strings.Repeat("51f2311d0ce3abcd", 4)
	deps := containerPortOwnershipDeps{
		inspectExactID: func(selector string) (string, error) {
			if selector != "observed-head" {
				t.Fatalf("unexpected selector %q", selector)
			}
			return exactID, nil
		},
		containerPIDs: func(id string) (map[string]struct{}, error) {
			t.Fatalf("matching cgroup should not fall back to docker top for %q", id)
			return nil, nil
		},
		listeningSocketData: func(address string, port int) ([]byte, error) {
			if address != "100.96.0.20" || port != 8000 {
				t.Fatalf("unexpected endpoint %s:%d", address, port)
			}
			return []byte("LISTEN 0 4096 100.96.0.20:8000 0.0.0.0:* ino:1 sk:1 cgroup:/system.slice/docker-" + exactID + ".scope\n"), nil
		},
		prefixIsUnambiguous: func(string, string) (bool, error) {
			t.Fatal("full cgroup ID should not require prefix resolution")
			return false, nil
		},
	}
	owned, err := containerOwnsListeningPortWithDeps("observed-head", "100.96.0.20", 8000, deps)
	if err != nil || !owned {
		t.Fatalf("matching Docker cgroup was rejected: owned=%v err=%v", owned, err)
	}
}

func TestSocketOwnershipFiltersExactRequestedAddress(t *testing.T) {
	exactID := strings.Repeat("e", 64)
	output := []byte("LISTEN 0 4096 100.96.0.21:8000 0.0.0.0:* cgroup:/system.slice/docker-" + exactID + ".scope\n")
	filtered := filterSocketDataByEndpoint(output, "100.96.0.20", 8000)
	if len(filtered) != 0 {
		t.Fatalf("different-address listener survived endpoint filter: %s", filtered)
	}
	if filtered = filterSocketDataByEndpoint(output, "100.96.0.21", 8000); len(filtered) == 0 {
		t.Fatal("exact-address listener was removed by endpoint filter")
	}
}

func TestDeploymentPreflightAllowsObservedHeadToOwnRendezvous(t *testing.T) {
	request := deploymentPreflightRequest{FabricAddress: "192.168.201.1", ServiceAddress: "100.96.0.20", ServicePort: 8000, RendezvousPort: 25000, Head: true, CandidateName: "candidate", ObservedContainer: "observed-head"}
	deps := deploymentPreflightDeps{
		interfaceAddrs: func() ([]net.Addr, error) {
			return []net.Addr{testAddr("192.168.201.1/24"), testAddr("100.96.0.20/32")}, nil
		},
		stat: os.Stat, open: os.Open, listContainers: func(string) ([]Container, error) { return nil, nil },
		portAvailable: func(address string, port int) (bool, error) { return port == 8000, nil },
		containerOwns: func(id, address string, port int) (bool, error) {
			return id == "observed-head" && address == "192.168.201.1" && port == 25000, nil
		},
	}
	if err := validateDeploymentPreflight(context.Background(), request, deps); err != nil {
		t.Fatalf("Kyber-style observed rendezvous owner was rejected: %v", err)
	}
}

func TestSocketDataOwnershipRejectsMismatchedDockerCgroup(t *testing.T) {
	exactID := strings.Repeat("a", 64)
	otherID := strings.Repeat("b", 64)
	output := []byte("LISTEN 0 4096 100.96.0.20:8000 0.0.0.0:* cgroup:/system.slice/docker-" + otherID + ".scope\n")
	owned, err := socketDataOwnedByContainer(output, exactID, nil, func(string, string) (bool, error) {
		return false, nil
	})
	if err != nil || owned {
		t.Fatalf("mismatched Docker cgroup was accepted: owned=%v err=%v", owned, err)
	}
}

func TestSocketDataOwnershipFallsBackToContainerPID(t *testing.T) {
	output := []byte(`LISTEN 0 4096 100.96.0.20:8000 0.0.0.0:* users:(("python",pid=4321,fd=7))`)
	owned, err := socketDataOwnedByContainer(output, strings.Repeat("c", 64), map[string]struct{}{"4321": {}}, func(string, string) (bool, error) {
		return false, nil
	})
	if err != nil || !owned {
		t.Fatalf("PID fallback did not recognize observed container process: owned=%v err=%v", owned, err)
	}
}

func TestSocketDataOwnershipRequiresUnambiguousLongCgroupPrefix(t *testing.T) {
	exactID := strings.Repeat("d", 64)
	prefix := exactID[:12]
	output := []byte("LISTEN 0 4096 100.96.0.20:8000 0.0.0.0:* cgroup:/system.slice/docker-" + prefix + ".scope\n")
	for _, unambiguous := range []bool{false, true} {
		owned, err := socketDataOwnedByContainer(output, exactID, nil, func(gotPrefix, gotExact string) (bool, error) {
			if gotPrefix != prefix || gotExact != exactID {
				t.Fatalf("unexpected prefix resolution %q against %q", gotPrefix, gotExact)
			}
			return unambiguous, nil
		})
		if err != nil || owned != unambiguous {
			t.Fatalf("unambiguous=%v produced owned=%v err=%v", unambiguous, owned, err)
		}
	}
}

type testAddr string

func (a testAddr) Network() string { return "ip" }
func (a testAddr) String() string  { return string(a) }
