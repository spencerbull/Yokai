package agent

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
)

type deploymentPreflightRequest struct {
	FabricAddress     string `json:"fabric_address"`
	ServiceAddress    string `json:"service_address,omitempty"`
	ServicePort       int    `json:"service_port"`
	RendezvousPort    int    `json:"rendezvous_port"`
	Head              bool   `json:"head"`
	ObservedContainer string `json:"observed_container_id,omitempty"`
	CandidateName     string `json:"candidate_name"`
	LocalModelPath    string `json:"local_model_path,omitempty"`
}

type deploymentPreflightDeps struct {
	interfaceAddrs func() ([]net.Addr, error)
	stat           func(string) (os.FileInfo, error)
	open           func(string) (*os.File, error)
	listContainers func(string) ([]Container, error)
	portAvailable  func(string, int) (bool, error)
	containerOwns  func(string, string, int) (bool, error)
}

var liveDeploymentPreflightDeps = deploymentPreflightDeps{
	interfaceAddrs: net.InterfaceAddrs,
	stat:           os.Stat,
	open:           os.Open,
	listContainers: listContainersScope,
	portAvailable:  tcpPortAvailable,
	containerOwns:  containerOwnsListeningPort,
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
	if err := validateDeploymentPreflight(request, liveDeploymentPreflightDeps); err != nil {
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

func validateDeploymentPreflight(request deploymentPreflightRequest, deps deploymentPreflightDeps) error {
	if strings.TrimSpace(request.FabricAddress) == "" || strings.TrimSpace(request.CandidateName) == "" {
		return preflightBadRequest("fabric_address and candidate_name are required")
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
	containers, err := deps.listContainers(InventoryScopeAll)
	if err != nil {
		return &preflightProblem{status: http.StatusServiceUnavailable, code: "container_inventory_unavailable", err: err}
	}
	for _, container := range containers {
		if container.Name == request.CandidateName {
			return preflightConflict("deterministic candidate name already exists")
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
