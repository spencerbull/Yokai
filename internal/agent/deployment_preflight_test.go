package agent

import (
	"net"
	"os"
	"strings"
	"testing"
)

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
	if err := validateDeploymentPreflight(request, deps); err != nil {
		t.Fatalf("valid preflight failed: %v", err)
	}

	request.LocalModelPath = modelDir + "/missing"
	if err := validateDeploymentPreflight(request, deps); err == nil {
		t.Fatal("missing model directory passed preflight")
	}
	request.LocalModelPath = modelDir
	request.ServiceAddress = "100.96.0.21"
	if err := validateDeploymentPreflight(request, deps); err == nil {
		t.Fatal("non-local service address passed preflight")
	}
}

func TestDeploymentPreflightRequiresObservedContainerToOwnOccupiedServicePort(t *testing.T) {
	request := deploymentPreflightRequest{
		FabricAddress: "192.168.201.2", ServicePort: 8000, RendezvousPort: 25000,
		CandidateName: "yokai-deployment-test-worker", ObservedContainer: "1234567890ab",
	}
	owned := false
	deps := deploymentPreflightDeps{
		interfaceAddrs: func() ([]net.Addr, error) { return []net.Addr{testAddr("192.168.201.2/24")}, nil },
		stat:           os.Stat, open: os.Open,
		listContainers: func(string) ([]Container, error) { return nil, nil },
		portAvailable:  func(string, int) (bool, error) { return false, nil },
		containerOwns: func(id, address string, port int) (bool, error) {
			return owned && id == "1234567890ab" && address == "192.168.201.2" && port == 8000, nil
		},
	}
	if err := validateDeploymentPreflight(request, deps); err == nil {
		t.Fatal("unowned occupied service port passed preflight")
	}
	owned = true
	if err := validateDeploymentPreflight(request, deps); err != nil {
		t.Fatalf("explicitly owned service port was rejected: %v", err)
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
	if err := validateDeploymentPreflight(request, deps); err == nil {
		t.Fatal("candidate name collision passed preflight")
	}
	deps.listContainers = func(string) ([]Container, error) { return nil, nil }
	deps.portAvailable = func(_ string, port int) (bool, error) { return port != 25000, nil }
	if err := validateDeploymentPreflight(request, deps); err == nil {
		t.Fatal("occupied rendezvous port passed preflight")
	}
}

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
	if err := validateDeploymentPreflight(request, deps); err != nil {
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
