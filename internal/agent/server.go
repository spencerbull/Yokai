package agent

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/spencerbull/yokai/internal/deployments"
	"github.com/spencerbull/yokai/internal/docker"
)

var startTime = time.Now()
var systemAgentConfigPath = "/etc/yokai/agent.json"

// authConfig holds the bearer token for API authentication.
type authConfig struct {
	Token string `json:"token"`
}

var authToken string
var catalog *docker.Catalog

// Run starts the agent HTTP server on the given port.
func Run(port string, version string) error {
	// Load auth token if available
	loadAuthToken()

	// Initialize Docker catalog
	catalog = docker.NewCatalog()

	mux := http.NewServeMux()

	// Health endpoint (no auth required)
	mux.HandleFunc("GET /health", handleHealth(version))

	// Protected endpoints
	mux.HandleFunc("GET /system/info", requireAuth(handleSystemInfo(version)))
	mux.HandleFunc("GET /metrics", requireAuth(handleMetrics))
	mux.HandleFunc("GET /metrics/prometheus", requireAuth(handlePrometheusMetrics))
	mux.HandleFunc("GET /containers", requireAuth(handleContainers))
	mux.HandleFunc("POST /containers", requireAuth(handleContainerDeploy))
	mux.HandleFunc("POST /containers/{id}/stop", requireAuth(handleContainerStop))
	mux.HandleFunc("DELETE /containers/{id}", requireAuth(handleContainerDelete))
	mux.HandleFunc("POST /containers/{id}/restart", requireAuth(handleContainerRestart))
	mux.HandleFunc("POST /deployments/{deploymentID}/members/{id}/stop", requireAuth(handleDeploymentMemberStop))
	mux.HandleFunc("DELETE /deployments/{deploymentID}/members/{id}", requireAuth(handleDeploymentMemberDelete))
	mux.HandleFunc("POST /deployments/{deploymentID}/members/{id}/restart", requireAuth(handleDeploymentMemberRestart))
	mux.HandleFunc("POST /deployments/{deploymentID}/members/{id}/logs/tail", requireAuth(handleDeploymentMemberLogTail))
	mux.HandleFunc("POST /containers/{id}/test", requireAuth(handleContainerTest))
	mux.HandleFunc("GET /containers/{id}/logs", requireAuth(handleContainerLogs))
	mux.HandleFunc("POST /images/pull", requireAuth(handleImagePull))
	mux.HandleFunc("POST /deployments/preflight", requireAuth(handleDeploymentPreflight))
	mux.HandleFunc("GET /images/tags/{image...}", requireAuth(handleImageTags))

	addr := ":" + port
	log.Printf("yokai agent %s starting on %s", version, addr)
	return http.ListenAndServe(addr, mux)
}

// loadAuthToken loads the bearer token from agent config paths.
func loadAuthToken() {
	authToken = ""

	var configPaths []string
	if p := os.Getenv("YOKAI_AGENT_CONFIG"); p != "" {
		configPaths = append(configPaths, p)
	}
	configPaths = append(configPaths, systemAgentConfigPath)
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		configPaths = append(configPaths, filepath.Join(home, ".config", "yokai", "agent.json"))
	}

	for _, configPath := range configPaths {
		data, err := os.ReadFile(configPath)
		if err != nil {
			continue
		}

		var config authConfig
		if err := json.Unmarshal(data, &config); err != nil {
			log.Printf("Invalid auth config at %s: %v", configPath, err)
			continue
		}

		authToken = config.Token
		log.Printf("Loaded auth token from %s", configPath)
		return
	}

	log.Printf("No auth config found, running without authentication")
}

// requireAuth wraps a handler to require bearer token authentication.
func requireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Skip auth if no token is configured
		if authToken == "" {
			next(w, r)
			return
		}

		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			writeError(w, http.StatusUnauthorized, "missing_auth", "Authorization header required")
			return
		}

		const bearerPrefix = "Bearer "
		if !strings.HasPrefix(authHeader, bearerPrefix) {
			writeError(w, http.StatusUnauthorized, "invalid_auth", "Bearer token required")
			return
		}

		token := strings.TrimPrefix(authHeader, bearerPrefix)
		if token != authToken {
			writeError(w, http.StatusUnauthorized, "invalid_token", "Invalid bearer token")
			return
		}

		next(w, r)
	}
}

func handleHealth(version string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		hostname, _ := os.Hostname()
		resp := map[string]interface{}{
			"status":         "ok",
			"version":        version,
			"uptime_seconds": int(time.Since(startTime).Seconds()),
			"hostname":       hostname,
			"capabilities":   append([]string(nil), AgentCapabilities...),
		}
		writeJSON(w, http.StatusOK, resp)
	}
}

func handleSystemInfo(version string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		hostname, _ := os.Hostname()

		// Get OS info from /etc/os-release
		osInfo := getOSInfo()

		// Get kernel version
		kernelVersion := getKernelVersion()

		// Get CPU info
		cpuInfo := getCPUInfo()

		// Get GPU info
		gpuInfo := getGPUInfo()

		// Get Docker version
		dockerInfo := getDockerInfo()

		// Get total RAM
		ramInfo := getTotalRAM()

		// Get total disk space
		diskInfo := getTotalDisk()

		resp := map[string]interface{}{
			"hostname": hostname,
			"os":       osInfo,
			"kernel":   kernelVersion,
			"arch":     runtime.GOARCH,
			"cpu":      cpuInfo,
			"gpus":     gpuInfo,
			"docker":   dockerInfo,
			"ram":      ramInfo,
			"disk":     diskInfo,
			"version":  version,
		}
		writeJSON(w, http.StatusOK, resp)
	}
}

func handleMetrics(w http.ResponseWriter, r *http.Request) {
	metrics := CollectMetrics()

	containers, err := listContainers()
	if err != nil {
		log.Printf("warning: failed to list containers for metrics: %v", err)
	} else {
		metrics.Containers = mergeContainerMetrics(metrics.Containers, containers)
	}

	writeJSON(w, http.StatusOK, metrics)
}

func mergeContainerMetrics(metricContainers []ContainerMetrics, dockerContainers []Container) []ContainerMetrics {
	idIndex := make(map[string]int)
	nameIndex := make(map[string]int)

	for i := range metricContainers {
		id := shortContainerID(metricContainers[i].ID)
		if id != "" {
			idIndex[id] = i
		}
		if metricContainers[i].Name != "" {
			nameIndex[metricContainers[i].Name] = i
		}
	}

	for _, container := range dockerContainers {
		id := shortContainerID(container.ID)
		idx, found := idIndex[id]
		if !found {
			idx, found = nameIndex[container.Name]
		}

		if found {
			if metricContainers[idx].ID == "" {
				metricContainers[idx].ID = id
			}
			if metricContainers[idx].Name == "" {
				metricContainers[idx].Name = container.Name
			}
			metricContainers[idx].Status = container.Status
			metricContainers[idx].Image = container.Image
			metricContainers[idx].Uptime = container.Uptime
			metricContainers[idx].Ports = container.Ports
			metricContainers[idx].ServiceAddress = managedServiceAddress(container)
			if container.VLLMMetrics != nil {
				metricContainers[idx].GenerationTokPerSec = container.VLLMMetrics.GenerationTokPerSec
				metricContainers[idx].PromptTokPerSec = container.VLLMMetrics.PromptTokPerSec
				metricContainers[idx].PromptTokensTotal = container.VLLMMetrics.PromptTokensTotal
				metricContainers[idx].GenerationTokensTotal = container.VLLMMetrics.GenerationTokensTotal
				metricContainers[idx].CachedPromptTokensTotal = container.VLLMMetrics.CachedPromptTokensTotal
			}
			continue
		}

		cm := ContainerMetrics{
			ID:             id,
			Name:           container.Name,
			Image:          container.Image,
			Status:         container.Status,
			Uptime:         container.Uptime,
			Ports:          container.Ports,
			ServiceAddress: managedServiceAddress(container),
		}
		if container.VLLMMetrics != nil {
			cm.GenerationTokPerSec = container.VLLMMetrics.GenerationTokPerSec
			cm.PromptTokPerSec = container.VLLMMetrics.PromptTokPerSec
			cm.PromptTokensTotal = container.VLLMMetrics.PromptTokensTotal
			cm.GenerationTokensTotal = container.VLLMMetrics.GenerationTokensTotal
			cm.CachedPromptTokensTotal = container.VLLMMetrics.CachedPromptTokensTotal
		}
		metricContainers = append(metricContainers, cm)

		if id != "" {
			idIndex[id] = len(metricContainers) - 1
		}
		if container.Name != "" {
			nameIndex[container.Name] = len(metricContainers) - 1
		}
	}

	// Probe health for running containers with exposed ports
	for i := range metricContainers {
		if metricContainers[i].Status == "running" && len(metricContainers[i].Ports) > 0 {
			metricContainers[i].Health = probeContainerHealth(metricContainers[i].Ports, metricContainers[i].Image, metricContainers[i].ServiceAddress)
		}
	}

	return metricContainers
}

func shortContainerID(id string) string {
	id = strings.TrimSpace(id)
	if len(id) > 12 {
		return id[:12]
	}
	return id
}

func handleContainers(w http.ResponseWriter, r *http.Request) {
	scope := strings.TrimSpace(r.URL.Query().Get("scope"))
	if scope == "" {
		scope = InventoryScopeManaged
	}
	if scope != InventoryScopeManaged && scope != InventoryScopeAll {
		writeError(w, http.StatusBadRequest, "invalid_scope", "scope must be managed or all")
		return
	}
	containers, err := listContainersScope(scope)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "docker_error", err.Error())
		return
	}

	resp := map[string]interface{}{
		"containers": containers,
	}
	writeJSON(w, http.StatusOK, resp)
}

func handleContainerDeploy(w http.ResponseWriter, r *http.Request) {
	handleContainerDeployWithLaunchTimeout(w, r, deployments.DefaultCandidateLaunchCommandTimeout)
}

func handleContainerDeployWithLaunchTimeout(w http.ResponseWriter, r *http.Request, launchTimeout time.Duration) {
	var req ContainerRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "Invalid JSON body")
		return
	}

	// Validate required fields
	if req.Image == "" {
		writeError(w, http.StatusBadRequest, "missing_image", "Image is required")
		return
	}
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "missing_name", "Name is required")
		return
	}
	if err := validateContainerRuntime(req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_runtime", err.Error())
		return
	}
	if req.Labels == nil {
		req.Labels = make(map[string]string)
	}
	if req.Labels[LabelManaged] == "" {
		req.Labels[LabelManaged] = "true"
	}
	if req.Labels[LabelOwnership] == "" {
		req.Labels[LabelOwnership] = OwnershipManaged
	}

	// Register coordinated ownership before any Docker/image command. Those
	// validation commands are deliberately not trusted to finish before the
	// coordinator's request deadline, so rollback deletion must already see the
	// deterministic-name barrier while they run. The detached command deadline
	// also starts here, preventing a handler that resumes after slow validation
	// from later issuing docker run.
	launchCtx := r.Context()
	if hasCompleteManagedCandidateProvenance(req) {
		finishLaunch, beginErr := coordinatedCandidateLaunches.begin(normalizedContainerName(req.Name))
		if beginErr != nil {
			writeError(w, http.StatusConflict, "launch_in_progress", beginErr.Error())
			return
		}
		defer finishLaunch()
		var cancel context.CancelFunc
		launchCtx, cancel = detachedCandidateLaunchContext(r.Context(), launchTimeout)
		defer cancel()
	}

	if req.SkipPull {
		log.Printf("Skipping image pull (--skip-pull): %s", req.Image)
	} else {
		if err := validateImagePlatform(launchCtx, req.Image); err != nil {
			writeError(w, http.StatusBadRequest, "unsupported_platform", err.Error())
			return
		}

		log.Printf("Pulling image: %s", req.Image)
		if err := pullImageWithContext(launchCtx, req.Image); err != nil {
			writeError(w, http.StatusInternalServerError, "pull_failed", err.Error())
			return
		}
	}

	if err := validatePulledImageArchitecture(launchCtx, req.Image); err != nil {
		writeError(w, http.StatusBadRequest, "unsupported_platform", err.Error())
		return
	}

	// Deploy container
	log.Printf("Deploying container: %s", req.Name)
	resp, err := runContainerWithContext(launchCtx, req, liveFailedRunCleanupDeps)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "deploy_failed", err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, resp)
}

func handleContainerStop(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "missing_id", "Container ID is required")
		return
	}
	if rejectLegacyGroupedMutation(w, id) {
		return
	}
	finishStop, beginErr := containerStops.begin(strings.TrimSpace(id))
	if beginErr != nil {
		writeError(w, http.StatusConflict, "container_stop_in_progress", "container stop is already in progress")
		return
	}
	defer finishStop()

	if err := stopContainer(id); err != nil {
		writeError(w, http.StatusInternalServerError, "stop_failed", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"status": "stopped",
	})
}

func handleContainerDelete(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "missing_id", "Container ID is required")
		return
	}
	if rejectLegacyGroupedMutation(w, id) {
		return
	}
	if err := containerStops.wait(r.Context(), strings.TrimSpace(id)); err != nil {
		writeError(w, http.StatusServiceUnavailable, "container_stop_in_progress", "container stop has not settled")
		return
	}

	if !containerExists(id) {
		writeError(w, http.StatusNotFound, "container_not_found", fmt.Sprintf("Container %s not found", id))
		return
	}

	// Stop the container first
	if err := stopContainer(id); err != nil {
		log.Printf("Warning: failed to stop container %s: %v", id, err)
	}

	// Remove the container
	if err := removeContainer(id); err != nil {
		writeError(w, http.StatusInternalServerError, "remove_failed", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"status": "removed",
	})
}

func handleContainerRestart(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "missing_id", "Container ID is required")
		return
	}
	if rejectLegacyGroupedMutation(w, id) {
		return
	}
	if err := containerStops.wait(r.Context(), strings.TrimSpace(id)); err != nil {
		writeError(w, http.StatusServiceUnavailable, "container_stop_in_progress", "container stop has not settled")
		return
	}

	if !containerExists(id) {
		writeError(w, http.StatusNotFound, "container_not_found", fmt.Sprintf("Container %s not found", id))
		return
	}

	if err := restartContainer(id); err != nil {
		writeError(w, http.StatusInternalServerError, "restart_failed", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"status": "restarted",
	})
}

func rejectLegacyGroupedMutation(w http.ResponseWriter, id string) bool {
	_, _, labels, err := inspectContainerIdentity(id)
	if err != nil {
		if errors.Is(err, errContainerIdentityNotFound) {
			return false
		}
		writeError(w, http.StatusServiceUnavailable, "container_identity_unavailable", "container identity could not be verified safely")
		return true
	}
	if labels[LabelDeploymentID] == "" {
		return false
	}
	writeError(w, http.StatusConflict, "grouped_deployment_required", "deployment-managed containers require the deployment-member lifecycle API")
	return true
}

func deploymentMemberProvenance(r *http.Request) (map[string]string, error) {
	_, name, labels, err := inspectContainerIdentityWithContext(r.Context(), r.PathValue("id"))
	if err != nil {
		return nil, err
	}
	if err := validateDeploymentMemberProvenance(name, labels, r.PathValue("deploymentID"), r.URL.Query().Get("generation"), r.URL.Query().Get("role"), r.URL.Query().Get("name")); err != nil {
		return nil, err
	}
	return labels, nil
}

func validateDeploymentMemberProvenance(name string, labels map[string]string, deploymentID, generation, role, expectedName string) error {
	expected := map[string]string{
		LabelManaged:      "true",
		LabelOwnership:    OwnershipManaged,
		LabelDeploymentID: deploymentID,
		LabelGeneration:   generation,
		LabelRole:         role,
	}
	for key, value := range expected {
		if value == "" || labels[key] != value {
			return fmt.Errorf("container provenance does not match expected deployment member")
		}
	}
	if expectedName == "" || name != expectedName {
		return fmt.Errorf("container name does not match expected deployment member")
	}
	return nil
}

func authorizeDeploymentMember(w http.ResponseWriter, r *http.Request) bool {
	if _, err := deploymentMemberProvenance(r); err != nil {
		writeError(w, http.StatusConflict, "deployment_member_mismatch", err.Error())
		return false
	}
	return true
}

func handleDeploymentMemberStop(w http.ResponseWriter, r *http.Request) {
	if !authorizeDeploymentMember(w, r) {
		return
	}
	finishStop, beginErr := containerStops.begin(strings.TrimSpace(r.PathValue("id")))
	if beginErr != nil {
		writeError(w, http.StatusConflict, "container_stop_in_progress", "container stop is already in progress")
		return
	}
	defer finishStop()
	if err := stopContainerWithContext(r.Context(), r.PathValue("id")); err != nil {
		writeError(w, http.StatusInternalServerError, "stop_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "stopped"})
}

func handleDeploymentMemberDelete(w http.ResponseWriter, r *http.Request) {
	// If an ambiguous launch request is still executing, deletion is the
	// rollback barrier: do not inspect an as-yet unmaterialized name and certify
	// it absent before the launch's owned cleanup has completed.
	if err := coordinatedCandidateLaunches.wait(r.Context(), r.URL.Query().Get("name")); err != nil {
		writeError(w, http.StatusServiceUnavailable, "deployment_launch_in_progress", "candidate launch cleanup is still in progress")
		return
	}
	if err := containerStops.wait(r.Context(), strings.TrimSpace(r.PathValue("id"))); err != nil {
		writeError(w, http.StatusServiceUnavailable, "container_stop_in_progress", "container stop has not settled")
		return
	}
	if _, err := deploymentMemberProvenance(r); err != nil {
		if errors.Is(err, errContainerIdentityNotFound) {
			writeError(w, http.StatusNotFound, "container_not_found", "deployment member is already absent")
			return
		}
		writeError(w, http.StatusConflict, "deployment_member_mismatch", err.Error())
		return
	}
	if err := removeContainerWithContext(r.Context(), r.PathValue("id")); err != nil {
		writeError(w, http.StatusInternalServerError, "remove_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "removed"})
}

func handleDeploymentMemberRestart(w http.ResponseWriter, r *http.Request) {
	if !authorizeDeploymentMember(w, r) {
		return
	}
	if err := containerStops.wait(r.Context(), strings.TrimSpace(r.PathValue("id"))); err != nil {
		writeError(w, http.StatusServiceUnavailable, "container_stop_in_progress", "container stop has not settled")
		return
	}
	if err := restartContainerWithContext(r.Context(), r.PathValue("id")); err != nil {
		writeError(w, http.StatusInternalServerError, "restart_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "restarted"})
}

func handleDeploymentMemberLogTail(w http.ResponseWriter, r *http.Request) {
	if err := coordinatedCandidateLaunches.wait(r.Context(), r.URL.Query().Get("name")); err != nil {
		writeError(w, http.StatusServiceUnavailable, "deployment_launch_in_progress", "candidate launch has not settled")
		return
	}
	if _, err := deploymentMemberProvenance(r); err != nil {
		if errors.Is(err, errContainerIdentityNotFound) {
			writeError(w, http.StatusNotFound, "container_not_found", "deployment member is absent")
			return
		}
		writeError(w, http.StatusConflict, "deployment_member_mismatch", err.Error())
		return
	}
	var request struct {
		Redact string `json:"redact,omitempty"`
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 2048))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil && !errors.Is(err, io.EOF) {
		writeError(w, http.StatusBadRequest, "invalid_json", "Invalid JSON body")
		return
	}
	if request.Redact != "" && (len(request.Redact) > 512 || strings.IndexFunc(request.Redact, unicode.IsSpace) >= 0 || strings.IndexFunc(request.Redact, unicode.IsControl) >= 0) {
		writeError(w, http.StatusBadRequest, "invalid_redaction", "Redaction value must be a single non-control token of at most 512 characters")
		return
	}
	capture, err := captureContainerLogTail(r.Context(), r.PathValue("id"), request.Redact, runDockerLogTail)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "logs_failed", "bounded container log capture failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"tail": capture.Tail, "truncated": capture.Truncated})
}

func handleContainerTest(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "missing_id", "Container ID is required")
		return
	}

	containers, err := listContainersScope(InventoryScopeAll)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "docker_error", err.Error())
		return
	}

	var target *Container
	for i := range containers {
		if containers[i].ID == id || strings.HasPrefix(containers[i].ID, id) || containers[i].Name == id {
			target = &containers[i]
			break
		}
	}
	if target == nil {
		writeError(w, http.StatusNotFound, "container_not_found", fmt.Sprintf("Container %s not found", id))
		return
	}

	var request struct {
		APIKey string `json:"api_key,omitempty"`
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil && !errors.Is(err, io.EOF) {
		writeError(w, http.StatusBadRequest, "invalid_json", "Invalid JSON body")
		return
	}
	if request.APIKey != "" && (len(request.APIKey) > 512 || strings.IndexFunc(request.APIKey, unicode.IsSpace) >= 0 || strings.IndexFunc(request.APIKey, unicode.IsControl) >= 0) {
		writeError(w, http.StatusBadRequest, "invalid_api_key", "API key must be a single non-control token of at most 512 characters")
		return
	}
	result, err := testContainerServiceWithOptions(*target, r.URL.Query().Get("require_metrics") == "true", request.APIKey)
	if err != nil {
		if isServiceAuthorizationError(err) {
			writeError(w, http.StatusBadGateway, "service_unauthorized", "model service rejected the API key")
			return
		}
		writeError(w, http.StatusInternalServerError, "service_test_failed", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, result)
}

func handleContainerLogs(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "missing_id", "Container ID is required")
		return
	}

	// Set headers for Server-Sent Events
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	// Get flusher
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "no_flusher", "Streaming not supported")
		return
	}

	// Start docker logs command
	cmd := exec.Command("docker", "logs", "-f", "--tail", "100", id)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "pipe_error", err.Error())
		return
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "pipe_error", err.Error())
		return
	}

	if err := cmd.Start(); err != nil {
		writeError(w, http.StatusInternalServerError, "logs_failed", err.Error())
		return
	}

	type logEvent struct {
		line string
	}
	logCh := make(chan logEvent, 64)
	scanDone := make(chan struct{}, 2)

	startScan := func(reader io.Reader, prefix string) {
		go func() {
			defer func() { scanDone <- struct{}{} }()
			scanner := bufio.NewScanner(reader)
			scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
			for scanner.Scan() {
				line := scanner.Text()
				if prefix != "" {
					line = prefix + line
				}
				select {
				case logCh <- logEvent{line: line}:
				case <-r.Context().Done():
					return
				}
			}
			if err := scanner.Err(); err != nil {
				select {
				case logCh <- logEvent{line: "[stderr] scanner error: " + err.Error()}:
				case <-r.Context().Done():
				}
			}
		}()
	}

	startScan(stdout, "")
	startScan(stderr, "[stderr] ")

	// Wait for command to finish or client disconnect
	done := make(chan error, 1)
	go func() {
		done <- cmd.Wait()
	}()
	completedScans := 0

	select {
	case <-r.Context().Done():
		if cmd.Process != nil {
			if err := cmd.Process.Kill(); err != nil && !errors.Is(err, os.ErrProcessDone) {
				log.Printf("failed to kill docker logs process: %v", err)
			}
		}
		return
	default:
	}

	for {
		select {
		case <-r.Context().Done():
			if cmd.Process != nil {
				if err := cmd.Process.Kill(); err != nil && !errors.Is(err, os.ErrProcessDone) {
					log.Printf("failed to kill docker logs process: %v", err)
				}
			}
			return
		case event := <-logCh:
			if event.line != "" {
				_, _ = fmt.Fprintf(w, "data: %s\n\n", event.line)
				flusher.Flush()
			}
		case <-scanDone:
			completedScans++
			if completedScans == 2 {
				if cmd.ProcessState == nil || !cmd.ProcessState.Exited() {
					<-done
				}
				return
			}
		case <-done:
			if completedScans >= 2 {
				return
			}
		}
	}
}

func handleImagePull(w http.ResponseWriter, r *http.Request) {
	var req ImagePullRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "Invalid JSON body")
		return
	}

	if req.Image == "" {
		writeError(w, http.StatusBadRequest, "missing_image", "Image is required")
		return
	}

	if err := pullImageWithContext(r.Context(), req.Image); err != nil {
		writeError(w, http.StatusInternalServerError, "pull_failed", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"status": "pulled",
		"image":  req.Image,
	})
}

func handleImageTags(w http.ResponseWriter, r *http.Request) {
	image := r.PathValue("image")
	if image == "" {
		writeError(w, http.StatusBadRequest, "missing_image", "Image name is required")
		return
	}

	tags, err := catalog.FetchTags(image)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "fetch_failed", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"image": image,
		"tags":  tags,
	})
}

func writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	if err := enc.Encode(data); err != nil {
		fmt.Fprintf(os.Stderr, "json encode error: %v\n", err)
	}
}

func writeError(w http.ResponseWriter, status int, code string, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	resp := map[string]interface{}{
		"error":   code,
		"message": message,
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	if err := enc.Encode(resp); err != nil {
		fmt.Fprintf(os.Stderr, "json encode error: %v\n", err)
	}
}

// System info helper functions

func getOSInfo() string {
	data, err := os.ReadFile("/etc/os-release")
	if err != nil {
		return runtime.GOOS
	}

	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, "PRETTY_NAME=") {
			return strings.Trim(strings.TrimPrefix(line, "PRETTY_NAME="), `"`)
		}
	}
	return runtime.GOOS
}

func getKernelVersion() string {
	cmd := exec.Command("uname", "-r")
	out, err := cmd.Output()
	if err != nil {
		return "unknown"
	}
	return strings.TrimSpace(string(out))
}

func getCPUInfo() map[string]interface{} {
	cores := runtime.NumCPU()
	model := getCPUModel()

	return map[string]interface{}{
		"cores":   cores,
		"model":   model,
		"threads": cores, // In Go, NumCPU returns logical CPUs (threads)
	}
}

func getCPUModel() string {
	data, err := os.ReadFile("/proc/cpuinfo")
	if err != nil {
		return "unknown"
	}

	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, "model name") {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 {
				return strings.TrimSpace(parts[1])
			}
		}
	}
	return "unknown"
}

func getGPUInfo() []map[string]interface{} {
	cmd := exec.Command("nvidia-smi", "--query-gpu=index,name,memory.total", "--format=csv,noheader,nounits")
	out, err := cmd.Output()
	if err != nil {
		return []map[string]interface{}{}
	}

	var gpus []map[string]interface{}
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line == "" {
			continue
		}
		fields := strings.Split(line, ",")
		if len(fields) >= 3 {
			for i := range fields {
				fields[i] = strings.TrimSpace(fields[i])
			}

			index, _ := strconv.Atoi(fields[0])
			memoryMB, _ := strconv.ParseInt(fields[2], 10, 64)

			gpus = append(gpus, map[string]interface{}{
				"index":     index,
				"name":      fields[1],
				"memory_mb": memoryMB,
			})
		}
	}
	return gpus
}

func getDockerInfo() map[string]interface{} {
	cmd := exec.Command("docker", "version", "--format", "{{.Server.Version}}")
	out, err := cmd.Output()
	if err != nil {
		return map[string]interface{}{
			"available": false,
			"error":     err.Error(),
		}
	}

	version := strings.TrimSpace(string(out))
	return map[string]interface{}{
		"available": true,
		"version":   version,
	}
}

func getTotalRAM() map[string]interface{} {
	data, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return map[string]interface{}{
			"total_mb": 0,
			"error":    err.Error(),
		}
	}

	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, "MemTotal:") {
			var totalKB int64
			if _, err := fmt.Sscanf(line, "MemTotal: %d kB", &totalKB); err != nil {
				continue
			}
			return map[string]interface{}{
				"total_mb": totalKB / 1024,
			}
		}
	}
	return map[string]interface{}{"total_mb": 0}
}

func getTotalDisk() map[string]interface{} {
	cmd := exec.Command("df", "-BG", "--output=size", "/")
	out, err := cmd.Output()
	if err != nil {
		return map[string]interface{}{
			"total_gb": 0,
			"error":    err.Error(),
		}
	}

	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(lines) >= 2 {
		sizeStr := strings.TrimSuffix(strings.TrimSpace(lines[1]), "G")
		size, _ := strconv.ParseInt(sizeStr, 10, 64)
		return map[string]interface{}{
			"total_gb": size,
		}
	}
	return map[string]interface{}{"total_gb": 0}
}
