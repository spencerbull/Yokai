package agent

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/spencerbull/yokai/internal/config"
	"github.com/spencerbull/yokai/internal/deployments"
	"github.com/spencerbull/yokai/internal/plugins"
)

// VLLMMetrics holds OpenAI-compatible inference throughput metrics. The JSON
// field name is retained for API compatibility, but values may come from vLLM
// or SGLang.
type VLLMMetrics struct {
	Model                    string             `json:"model,omitempty"`
	GenerationTokPerSec      float64            `json:"generation_tok_per_s"`
	PromptTokPerSec          float64            `json:"prompt_tok_per_s"`
	RequestsRunning          float64            `json:"requests_running,omitempty"`
	RequestsWaiting          float64            `json:"requests_waiting,omitempty"`
	PromptTokensTotal        float64            `json:"prompt_tokens_total,omitempty"`
	GenerationTokensTotal    float64            `json:"generation_tokens_total,omitempty"`
	CachedPromptTokensTotal  float64            `json:"cached_prompt_tokens_total,omitempty"`
	TTFTBuckets              map[string]float64 `json:"-"`
	TTFTSum                  float64            `json:"-"`
	TTFTCount                float64            `json:"-"`
	HasGenerationTokPerSec   bool               `json:"-"`
	HasPromptTokPerSec       bool               `json:"-"`
	HasRequestsRunning       bool               `json:"-"`
	HasRequestsWaiting       bool               `json:"-"`
	HasPromptTokensTotal     bool               `json:"-"`
	HasGenerationTokensTotal bool               `json:"-"`
	HasCachedPromptTokens    bool               `json:"-"`
	HasTTFT                  bool               `json:"-"`
	HasSGLangNativeMetric    bool               `json:"-"`
	HasVLLMNativeMetric      bool               `json:"-"`
}

// Container represents a running container.
type Container struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	Image       string            `json:"image"`
	Status      string            `json:"status"`
	Ports       map[string]string `json:"ports"`
	Created     time.Time         `json:"created"`
	Uptime      int64             `json:"uptime_seconds"`
	VLLMMetrics *VLLMMetrics      `json:"vllm_metrics,omitempty"`
	Labels      map[string]string `json:"labels,omitempty"`
	Ownership   string            `json:"ownership"`
}

const (
	InventoryScopeManaged = "managed"
	InventoryScopeAll     = "all"
	OwnershipManaged      = "managed"
	OwnershipAdopted      = "adopted"
	OwnershipObserved     = "observed"

	LabelManaged        = "io.yokai.managed"
	LabelOwnership      = "io.yokai.ownership"
	LabelServiceAddress = "io.yokai.service.address"
	LabelServicePort    = "io.yokai.service.port"
	LabelDeploymentID   = "io.yokai.deployment.id"
	LabelGeneration     = "io.yokai.deployment.generation"
	LabelRole           = "io.yokai.deployment.role"
	LabelBKCID          = "io.yokai.bkc.id"
	LabelModelRevision  = "io.yokai.model.revision"
	LabelImageDigest    = "io.yokai.image.digest"
	LabelRuntimePatch   = "io.yokai.runtime.patch"
	LabelLaunchNonce    = "io.yokai.launch.nonce"
)

var AgentCapabilities = []string{
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

// ContainerRequest represents a container deployment request.
type ContainerRequest struct {
	Image       string                `json:"image"`
	Name        string                `json:"name"`
	Model       string                `json:"model"`
	GGUFVariant string                `json:"gguf_variant,omitempty"`
	GGUFFiles   []string              `json:"gguf_files,omitempty"`
	HFToken     string                `json:"hf_token,omitempty"`
	Ports       map[string]string     `json:"ports"`
	Env         map[string]string     `json:"env"`
	GPUIDs      string                `json:"gpu_ids"`
	ExtraArgs   string                `json:"extra_args"`
	Args        []string              `json:"args,omitempty"`
	Volumes     map[string]string     `json:"volumes"`
	Plugins     []string              `json:"plugins"`
	Runtime     config.RuntimeOptions `json:"runtime"`
	SkipPull    bool                  `json:"skip_pull,omitempty"`
	Labels      map[string]string     `json:"labels,omitempty"`
	NetworkMode string                `json:"network_mode,omitempty"`
	Devices     []string              `json:"devices,omitempty"`
	CapAdd      []string              `json:"cap_add,omitempty"`
}

// ContainerResponse represents a container deployment response.
type ContainerResponse struct {
	ID     string `json:"id"`
	Status string `json:"status"`
}

// ImagePullRequest represents an image pull request.
type ImagePullRequest struct {
	Image string `json:"image"`
}

// listContainers retains the existing managed-only behavior used by metrics
// and GET /containers without an explicit scope.
func listContainers() ([]Container, error) {
	return listContainersScope(InventoryScopeManaged)
}

// listContainersScope returns a sanitized inventory. Docker environment data
// is never requested or represented by Container.
func listContainersScope(scope string) ([]Container, error) {
	if scope != InventoryScopeManaged && scope != InventoryScopeAll {
		return nil, fmt.Errorf("invalid inventory scope %q", scope)
	}
	args := []string{"ps", "-a", "--no-trunc"}
	if scope == InventoryScopeManaged {
		args = append(args, "--filter", "name=yokai-")
	}
	args = append(args, "--format", "json")
	cmd := exec.Command("docker", args...)
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("docker ps failed: %w", err)
	}

	var containers []Container
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line == "" {
			continue
		}

		var dockerContainer struct {
			ID         string `json:"ID"`
			Names      string `json:"Names"`
			Image      string `json:"Image"`
			Status     string `json:"Status"`
			Ports      string `json:"Ports"`
			CreatedAt  string `json:"CreatedAt"`
			RunningFor string `json:"RunningFor"`
			Labels     string `json:"Labels"`
		}

		if err := json.Unmarshal([]byte(line), &dockerContainer); err != nil {
			continue // Skip malformed lines
		}

		// Parse ports
		ports := make(map[string]string)
		if dockerContainer.Ports != "" {
			// Parse format like "0.0.0.0:8000->8000/tcp"
			portRegex := regexp.MustCompile(`0\.0\.0\.0:(\d+)->(\d+)/(tcp|udp)`)
			matches := portRegex.FindAllStringSubmatch(dockerContainer.Ports, -1)
			for _, match := range matches {
				if len(match) >= 3 {
					ports[match[2]] = match[1] // internal:external
				}
			}
		}

		// Parse created time
		created, _ := time.Parse("2006-01-02 15:04:05 -0700 MST", dockerContainer.CreatedAt)

		// Calculate uptime
		var uptime int64
		if strings.Contains(dockerContainer.Status, "Up ") {
			uptime = int64(time.Since(created).Seconds())
		}

		labels := sanitizeInventoryLabels(parseDockerLabels(dockerContainer.Labels))
		ownership := inventoryOwnership(dockerContainer.Names, labels)
		if scope == InventoryScopeManaged && ownership == OwnershipObserved {
			continue
		}
		container := Container{
			ID:        dockerContainer.ID,
			Name:      strings.TrimPrefix(dockerContainer.Names, "/"),
			Image:     dockerContainer.Image,
			Status:    parseStatus(dockerContainer.Status),
			Ports:     ports,
			Created:   created,
			Uptime:    uptime,
			Labels:    labels,
			Ownership: ownership,
		}
		if port := labels[LabelServicePort]; port != "" && container.Ports[port] == "" {
			container.Ports[port] = port
		}

		// The all-scope inventory is an identity path used by deployment
		// operations. Keep inference scrapes on the managed metrics/UI path.
		if scope == InventoryScopeManaged && (isVLLMImage(container.Image) || isSGLangImage(container.Image)) && container.Status == "running" {
			if baseURL, err := containerBaseURL(container); err == nil {
				if vm, err := scrapeVLLMMetricsURL(baseURL+"/metrics", ""); err == nil {
					container.VLLMMetrics = vm
				}
			}
		}

		containers = append(containers, container)
	}

	return containers, nil
}

var safeInventoryLabelKeys = map[string]struct{}{
	LabelManaged:        {},
	LabelOwnership:      {},
	LabelServiceAddress: {},
	LabelServicePort:    {},
	LabelDeploymentID:   {},
	LabelGeneration:     {},
	LabelRole:           {},
	LabelBKCID:          {},
	LabelModelRevision:  {},
	LabelImageDigest:    {},
	LabelRuntimePatch:   {},
}

func parseDockerLabels(raw string) map[string]string {
	labels := make(map[string]string)
	for _, item := range strings.Split(raw, ",") {
		key, value, ok := strings.Cut(item, "=")
		if !ok {
			continue
		}
		labels[strings.TrimSpace(key)] = strings.TrimSpace(value)
	}
	return labels
}

func sanitizeInventoryLabels(labels map[string]string) map[string]string {
	clean := make(map[string]string)
	for key, value := range labels {
		if _, ok := safeInventoryLabelKeys[key]; ok {
			clean[key] = value
		}
	}
	if len(clean) == 0 {
		return nil
	}
	return clean
}

func inventoryOwnership(name string, labels map[string]string) string {
	switch labels[LabelOwnership] {
	case OwnershipAdopted:
		return OwnershipAdopted
	case OwnershipManaged:
		return OwnershipManaged
	}
	if labels[LabelManaged] == "true" || strings.HasPrefix(strings.TrimPrefix(name, "/"), "yokai-") {
		return OwnershipManaged
	}
	return OwnershipObserved
}

// parseStatus converts Docker status to simplified format.
func parseStatus(dockerStatus string) string {
	if strings.HasPrefix(dockerStatus, "Up ") {
		return "running"
	}
	if strings.HasPrefix(dockerStatus, "Exited ") {
		return "stopped"
	}
	if strings.Contains(dockerStatus, "Created") {
		return "created"
	}
	if strings.Contains(dockerStatus, "Restarting") {
		return "restarting"
	}
	if strings.Contains(dockerStatus, "Dead") {
		return "dead"
	}
	return "unknown"
}

// runContainer deploys a new container.
func runContainer(req ContainerRequest) (*ContainerResponse, error) {
	return runContainerWithContext(context.Background(), req, liveFailedRunCleanupDeps)
}

func runContainerWithContext(ctx context.Context, req ContainerRequest, cleanupDeps failedRunCleanupDeps) (*ContainerResponse, error) {
	if err := validateContainerRuntime(req); err != nil {
		return nil, err
	}
	if err := validateImagePlatform(ctx, req.Image); err != nil {
		return nil, err
	}
	if err := applyPlugins(&req); err != nil {
		return nil, err
	}

	// Sanitize container name
	containerName := normalizedContainerName(req.Name)

	// If the deploy targets a specific GGUF variant, pre-download every shard
	// to the agent's shared models directory so the container can mmap the
	// file(s) directly. The returned path is container-local (/models/...).
	ggufPath, err := ensureGGUFFiles(&req)
	if err != nil {
		return nil, err
	}
	if ggufPath != "" {
		if req.Volumes == nil {
			req.Volumes = make(map[string]string)
		}
		ensureGGUFVolume(req.Volumes)
	}

	if isLlamaCppImage(req.Image) {
		if req.Model != "" {
			if req.Volumes == nil {
				req.Volumes = make(map[string]string)
			}
			ensureModelsVolume(req.Volumes)
			modelArg := req.Model
			if ggufPath != "" {
				modelArg = ggufPath
			}
			req.ExtraArgs = withLlamaModelArg(req.ExtraArgs, modelArg)
		}
		req.Ports = normalizeServicePorts(req.Ports, "8080")
		req.ExtraArgs = withHostArg(req.ExtraArgs, "--host", "0.0.0.0")
	}

	if isVLLMImage(req.Image) {
		if req.Model != "" {
			if req.Volumes == nil {
				req.Volumes = make(map[string]string)
			}
			ensureHFCacheVolume(req.Volumes)
			modelArg := req.Model
			if ggufPath != "" {
				// vLLM 0.6+ loads GGUF directly when --model points at the
				// on-disk file. The tokenizer still resolves from the original
				// HF repo unless the user overrode it in extra args.
				modelArg = ggufPath
				req.ExtraArgs = withVLLMTokenizerArg(req.ExtraArgs, req.Model)
			}
			req.ExtraArgs = withVLLMModelArg(req.ExtraArgs, modelArg)
		}
		req.Ports = normalizeServicePorts(req.Ports, "8000")
		req.ExtraArgs = withHostArg(req.ExtraArgs, "--host", "0.0.0.0")
		req.ExtraArgs = withVLLMToolCallArgs(req.ExtraArgs, req.Model)
	}

	sglangImage := isSGLangImage(req.Image)
	if sglangImage {
		if req.Model != "" {
			if req.Volumes == nil {
				req.Volumes = make(map[string]string)
			}
			ensureHFCacheVolume(req.Volumes)
		}
		req.ExtraArgs = withSGLangServeArgs(req.ExtraArgs, req.Model)
		req.Ports = normalizeServicePorts(req.Ports, "30000")
		req.ExtraArgs = withHostArg(req.ExtraArgs, "--host", "0.0.0.0")
	}
	if err := applyPinnedSGLangRuntimePatch(&req); err != nil {
		return nil, err
	}
	if sglangImage {
		if err := validateContainerRuntime(req); err != nil {
			return nil, err
		}
	}

	if isComfyUIImage(req.Image) {
		req.Ports = normalizeServicePorts(req.Ports, "8188")
	}
	if err := prepareLegacyLaunchNonce(&req); err != nil {
		return nil, err
	}

	args := buildDockerRunArgs(req, containerName)

	// Run the container
	cmd := exec.CommandContext(ctx, "docker", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		if hasCompleteManagedCandidateProvenance(req) {
			cleanupFailedManagedRunEventually(ctx, req, containerName, cleanupDeps, deployments.DefaultCandidateLaunchSettleTimeout, 250*time.Millisecond)
		} else {
			cleanupCtx, cancelCleanup := context.WithTimeout(context.WithoutCancel(ctx), deployments.DefaultCandidateLaunchSettleTimeout)
			cleanupFailedManagedRun(cleanupCtx, req, containerName, cleanupDeps)
			cancelCleanup()
		}
		// Docker output is not safe to return here: launch arguments can include
		// request-time credentials such as SGLang's rank-0 API key.
		return nil, fmt.Errorf("docker run failed: %w", err)
	}

	containerID := strings.TrimSpace(string(out))

	return &ContainerResponse{
		ID:     containerID[:12], // Short ID
		Status: "created",
	}, nil
}

func normalizedContainerName(name string) string {
	name = sanitizeName(name)
	if !strings.HasPrefix(name, "yokai-") {
		name = "yokai-" + name
	}
	return name
}

func hasCompleteManagedCandidateProvenance(req ContainerRequest) bool {
	for _, key := range []string{LabelDeploymentID, LabelGeneration, LabelRole} {
		if strings.TrimSpace(req.Labels[key]) == "" {
			return false
		}
	}
	return req.Labels[LabelManaged] == "true" && req.Labels[LabelOwnership] == OwnershipManaged
}

func prepareLegacyLaunchNonce(req *ContainerRequest) error {
	if req == nil || req.Labels[LabelManaged] != "true" || req.Labels[LabelOwnership] != OwnershipManaged {
		return nil
	}
	for _, key := range []string{LabelDeploymentID, LabelGeneration, LabelRole} {
		if strings.TrimSpace(req.Labels[key]) != "" {
			return nil
		}
	}
	random := make([]byte, 16)
	if _, err := rand.Read(random); err != nil {
		return fmt.Errorf("generate legacy launch identity: %w", err)
	}
	req.Labels[LabelLaunchNonce] = hex.EncodeToString(random)
	return nil
}

func cleanupFailedManagedRunEventually(ctx context.Context, req ContainerRequest, containerName string, deps failedRunCleanupDeps, timeout, interval time.Duration) bool {
	if timeout <= 0 || interval <= 0 {
		return cleanupFailedManagedRun(ctx, req, containerName, deps)
	}
	settleCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), timeout)
	defer cancel()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		if cleanupFailedManagedRun(settleCtx, req, containerName, deps) {
			return true
		}
		select {
		case <-settleCtx.Done():
			return false
		case <-ticker.C:
		}
	}
}

type failedRunCleanupDeps struct {
	inspect func(context.Context, string) (string, string, map[string]string, error)
	remove  func(context.Context, string) error
}

var liveFailedRunCleanupDeps = failedRunCleanupDeps{
	inspect: inspectContainerIdentityWithContext,
	remove:  removeContainerWithContext,
}

// cleanupFailedManagedRun handles Docker's ambiguous run failure: the daemon
// may have created the requested container even though the client returned an
// error. It removes only the exact Yokai-managed container whose ownership
// labels match the request, never an unrelated same-name container. Grouped
// requests additionally require their complete deployment provenance; partial
// grouped provenance cannot fall back to the legacy cleanup path.
func cleanupFailedManagedRun(ctx context.Context, req ContainerRequest, containerName string, deps failedRunCleanupDeps) bool {
	expected := map[string]string{
		LabelManaged:   "true",
		LabelOwnership: OwnershipManaged,
	}
	if req.Labels[LabelManaged] != expected[LabelManaged] || req.Labels[LabelOwnership] != expected[LabelOwnership] {
		return false
	}
	provenanceKeys := []string{LabelDeploymentID, LabelGeneration, LabelRole}
	provenanceCount := 0
	for _, key := range provenanceKeys {
		if strings.TrimSpace(req.Labels[key]) != "" {
			provenanceCount++
		}
	}
	if provenanceCount != 0 && provenanceCount != len(provenanceKeys) {
		return false
	}
	if provenanceCount == len(provenanceKeys) {
		for _, key := range provenanceKeys {
			expected[key] = req.Labels[key]
		}
	} else {
		expected[LabelLaunchNonce] = req.Labels[LabelLaunchNonce]
	}
	for _, value := range expected {
		if strings.TrimSpace(value) == "" {
			return false
		}
	}
	id, name, labels, err := deps.inspect(ctx, containerName)
	if err != nil || name != containerName {
		return false
	}
	for key, value := range expected {
		if labels[key] != value {
			return false
		}
	}
	return deps.remove(ctx, id) == nil
}

var errContainerIdentityNotFound = errors.New("container identity not found")

func inspectContainerIdentity(containerName string) (string, string, map[string]string, error) {
	return inspectContainerIdentityWithContext(context.Background(), containerName)
}

func inspectContainerIdentityWithContext(ctx context.Context, containerName string) (string, string, map[string]string, error) {
	output, err := exec.CommandContext(ctx, "docker", "inspect", "--format={{json .Id}}\n{{json .Name}}\n{{json .Config.Labels}}", containerName).CombinedOutput()
	if err != nil {
		if contextErr := ctx.Err(); contextErr != nil {
			return "", "", nil, fmt.Errorf("docker inspect canceled: %w", contextErr)
		}
		message := strings.ToLower(string(output))
		if strings.Contains(message, "no such object") || strings.Contains(message, "no such container") {
			return "", "", nil, fmt.Errorf("%w: %s", errContainerIdentityNotFound, containerName)
		}
		// Docker output is intentionally omitted because this identity check may
		// run beside secret-bearing coordinated containers.
		return "", "", nil, fmt.Errorf("docker inspect failed: %w", err)
	}
	parts := strings.SplitN(strings.TrimSpace(string(output)), "\n", 3)
	if len(parts) != 3 {
		return "", "", nil, fmt.Errorf("docker inspect returned incomplete identity")
	}
	var id string
	if err := json.Unmarshal([]byte(parts[0]), &id); err != nil || !dockerContainerIDPattern.MatchString(id) {
		return "", "", nil, fmt.Errorf("decode docker container ID")
	}
	var name string
	if err := json.Unmarshal([]byte(parts[1]), &name); err != nil {
		return "", "", nil, fmt.Errorf("decode docker container name: %w", err)
	}
	labels := make(map[string]string)
	if err := json.Unmarshal([]byte(parts[2]), &labels); err != nil {
		return "", "", nil, fmt.Errorf("decode docker container labels: %w", err)
	}
	return id, strings.TrimPrefix(name, "/"), labels, nil
}

func buildDockerRunArgs(req ContainerRequest, containerName string) []string {
	args := []string{"run", "-d", "--name", containerName}
	if req.NetworkMode != "" {
		args = append(args, "--network", req.NetworkMode)
	}
	if req.NetworkMode != "host" {
		for _, internal := range sortedMapKeys(req.Ports) {
			args = append(args, "-p", fmt.Sprintf("0.0.0.0:%s:%s", req.Ports[internal], internal))
		}
	}
	for _, key := range sortedMapKeys(req.Labels) {
		args = append(args, "--label", fmt.Sprintf("%s=%s", key, req.Labels[key]))
	}
	for _, key := range sortedMapKeys(req.Env) {
		args = append(args, "-e", fmt.Sprintf("%s=%s", key, req.Env[key]))
	}
	if req.GPUIDs != "" {
		if req.GPUIDs == "all" {
			args = append(args, "--gpus", "all")
		} else {
			args = append(args, "--gpus", fmt.Sprintf(`"device=%s"`, req.GPUIDs))
		}
	}
	for _, host := range sortedMapKeys(req.Volumes) {
		args = append(args, "-v", fmt.Sprintf("%s:%s", host, req.Volumes[host]))
	}
	for _, device := range req.Devices {
		args = append(args, "--device", device)
	}
	for _, capability := range req.CapAdd {
		args = append(args, "--cap-add", capability)
	}
	if req.Runtime.IPCMode != "" {
		args = append(args, "--ipc", req.Runtime.IPCMode)
	}
	if req.Runtime.ShmSize != "" {
		args = append(args, "--shm-size", req.Runtime.ShmSize)
	}
	for _, name := range sortedMapKeys(req.Runtime.Ulimits) {
		args = append(args, "--ulimit", fmt.Sprintf("%s=%s", name, req.Runtime.Ulimits[name]))
	}
	restartPolicy := req.Runtime.RestartPolicy
	if restartPolicy == config.RestartPolicyDefault {
		restartPolicy = config.RestartPolicyUnlessStopped
	}
	args = append(args, "--restart", string(restartPolicy), req.Image)
	if req.ExtraArgs != "" {
		args = append(args, strings.Fields(req.ExtraArgs)...)
	}
	args = append(args, req.Args...)
	return args
}

func sortedMapKeys[V any](values map[string]V) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

var capabilityPattern = regexp.MustCompile(`^[A-Z][A-Z0-9_]*$`)

func validateContainerRuntime(req ContainerRequest) error {
	if req.NetworkMode != "" && req.NetworkMode != "host" {
		return fmt.Errorf("unsupported network mode %q", req.NetworkMode)
	}
	for _, device := range req.Devices {
		hostPath := strings.SplitN(device, ":", 2)[0]
		if !strings.HasPrefix(path.Clean(hostPath), "/dev/") {
			return fmt.Errorf("device mount must be under /dev: %q", device)
		}
	}
	for _, capability := range req.CapAdd {
		if !capabilityPattern.MatchString(capability) {
			return fmt.Errorf("invalid Linux capability %q", capability)
		}
	}
	switch req.Runtime.RestartPolicy {
	case config.RestartPolicyDefault, config.RestartPolicyNo, config.RestartPolicyUnlessStopped:
	default:
		return fmt.Errorf("unsupported restart policy %q", req.Runtime.RestartPolicy)
	}
	for _, arg := range req.Args {
		if arg == "" || strings.ContainsAny(arg, "\x00\r\n") {
			return fmt.Errorf("invalid structured container argument")
		}
	}
	for key, value := range req.Labels {
		if strings.TrimSpace(key) == "" || strings.ContainsAny(key+value, "\r\n") {
			return fmt.Errorf("invalid container label")
		}
	}
	return nil
}

// containerExists checks whether a container with the given ID or name exists.
func containerExists(idOrName string) bool {
	cmd := exec.Command("docker", "inspect", "--format={{.State.Status}}", idOrName)
	return cmd.Run() == nil
}

// stopContainer stops a container by ID or name.
func stopContainer(idOrName string) error {
	return stopContainerWithContext(context.Background(), idOrName)
}

func stopContainerWithContext(ctx context.Context, idOrName string) error {
	cmd := exec.CommandContext(ctx, "docker", "stop", idOrName)
	if err := cmd.Run(); err != nil {
		if contextErr := ctx.Err(); contextErr != nil {
			return fmt.Errorf("docker stop canceled: %w", contextErr)
		}
		return fmt.Errorf("docker stop failed: %w", err)
	}
	return nil
}

// removeContainer removes a stopped container.
func removeContainer(idOrName string) error {
	return removeContainerWithContext(context.Background(), idOrName)
}

func removeContainerWithContext(ctx context.Context, idOrName string) error {
	cmd := exec.CommandContext(ctx, "docker", "rm", "-f", idOrName)
	if err := cmd.Run(); err != nil {
		if contextErr := ctx.Err(); contextErr != nil {
			return fmt.Errorf("docker rm canceled: %w", contextErr)
		}
		return fmt.Errorf("docker rm failed: %w", err)
	}
	return nil
}

// restartContainer restarts a container.
func restartContainer(idOrName string) error {
	return restartContainerWithContext(context.Background(), idOrName)
}

func restartContainerWithContext(ctx context.Context, idOrName string) error {
	cmd := exec.CommandContext(ctx, "docker", "restart", idOrName)
	if err := cmd.Run(); err != nil {
		if contextErr := ctx.Err(); contextErr != nil {
			return fmt.Errorf("docker restart canceled: %w", contextErr)
		}
		return fmt.Errorf("docker restart failed: %w", err)
	}
	return nil
}

// pullImage pulls a Docker image.
func pullImage(image string) error {
	return pullImageWithContext(context.Background(), image)
}

func pullImageWithContext(ctx context.Context, image string) error {
	cmd := exec.CommandContext(ctx, "docker", "pull", image)
	if err := cmd.Run(); err != nil {
		if contextErr := ctx.Err(); contextErr != nil {
			return fmt.Errorf("docker pull canceled: %w", contextErr)
		}
		return fmt.Errorf("docker pull failed: %w", err)
	}
	return nil
}

// sanitizeName sanitizes a container name.
func sanitizeName(name string) string {
	// Replace invalid characters with hyphens
	reg := regexp.MustCompile(`[^a-zA-Z0-9_.-]`)
	sanitized := reg.ReplaceAllString(name, "-")

	// Remove leading/trailing hyphens
	sanitized = strings.Trim(sanitized, "-")

	// Ensure it's not empty
	if sanitized == "" {
		sanitized = "unnamed"
	}

	return sanitized
}

func isVLLMImage(image string) bool {
	return strings.Contains(strings.ToLower(image), "vllm")
}

func isSGLangImage(image string) bool {
	return strings.Contains(strings.ToLower(image), "sglang")
}

func isLlamaCppImage(image string) bool {
	return strings.Contains(strings.ToLower(image), "llama.cpp")
}

func ensureModelsVolume(volumes map[string]string) {
	for _, containerPath := range volumes {
		if containerPath == "/models" {
			return
		}
	}
	volumes["/var/lib/yokai/models"] = "/models"
}

func ensureHFCacheVolume(volumes map[string]string) {
	for _, containerPath := range volumes {
		if containerPath == "/root/.cache/huggingface" {
			return
		}
	}
	volumes["/var/lib/yokai/huggingface"] = "/root/.cache/huggingface"
}

func applyPlugins(req *ContainerRequest) error {
	if len(req.Plugins) == 0 {
		return nil
	}
	if req.Env == nil {
		req.Env = make(map[string]string)
	}
	if req.Volumes == nil {
		req.Volumes = make(map[string]string)
	}
	if req.Runtime.Ulimits == nil {
		req.Runtime.Ulimits = make(map[string]string)
	}

	for _, pluginID := range req.Plugins {
		plugin, ok := plugins.Lookup(pluginID)
		if !ok {
			return fmt.Errorf("unknown plugin %s", pluginID)
		}
		for _, asset := range plugin.Assets {
			if err := ensurePluginAsset(plugin.ID, asset); err != nil {
				return err
			}
		}
		for _, mount := range plugin.Mounts {
			req.Volumes[plugins.AssetHostPath(plugin.ID, mount.AssetFile)] = mount.ContainerPath
		}
		for key, value := range plugin.Env {
			req.Env[key] = value
		}
		req.ExtraArgs = appendArg(req.ExtraArgs, plugin.ExtraArgs)
		if req.Runtime.IPCMode == "" {
			req.Runtime.IPCMode = plugin.Runtime.IPCMode
		}
		if req.Runtime.ShmSize == "" {
			req.Runtime.ShmSize = plugin.Runtime.ShmSize
		}
		for name, value := range plugin.Runtime.Ulimits {
			if _, exists := req.Runtime.Ulimits[name]; !exists {
				req.Runtime.Ulimits[name] = value
			}
		}
	}

	return nil
}

func ensurePluginAsset(pluginID string, asset plugins.Asset) error {
	hostPath := plugins.AssetHostPath(pluginID, asset.FileName)
	if info, err := os.Stat(hostPath); err == nil && info.Size() > 0 {
		return nil
	}
	if err := os.MkdirAll(path.Dir(hostPath), 0755); err != nil {
		return fmt.Errorf("creating plugin dir: %w", err)
	}
	resp, err := http.Get(asset.URL)
	if err != nil {
		return fmt.Errorf("downloading plugin asset %s: %w", asset.URL, err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("downloading plugin asset %s: status %d", asset.URL, resp.StatusCode)
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("reading plugin asset %s: %w", asset.URL, err)
	}
	if len(data) == 0 {
		return fmt.Errorf("downloaded empty plugin asset %s", asset.URL)
	}
	if err := os.WriteFile(hostPath, data, 0644); err != nil {
		return fmt.Errorf("writing plugin asset %s: %w", hostPath, err)
	}
	return nil
}

// withVLLMTokenizerArg injects `--tokenizer <repo>` when the model arg is a
// local GGUF path. Without this, vLLM tries to auto-load the tokenizer from
// the file path, which fails since GGUF files do not ship a HF tokenizer
// config. Callers should only invoke this when a GGUF variant is deployed.
func withVLLMTokenizerArg(extraArgs, modelRepo string) string {
	if modelRepo == "" {
		return extraArgs
	}
	tokens := strings.Fields(extraArgs)
	for _, t := range tokens {
		if hasFlag(t, "--tokenizer") {
			return extraArgs
		}
	}
	return appendArg(extraArgs, fmt.Sprintf("--tokenizer %s", modelRepo))
}

func withVLLMModelArg(extraArgs, model string) string {
	if model == "" {
		return extraArgs
	}

	tokens := strings.Fields(extraArgs)
	for i := range tokens {
		if hasFlag(tokens[i], "--model") {
			return extraArgs
		}
	}

	if strings.TrimSpace(extraArgs) == "" {
		return fmt.Sprintf("--model %s", model)
	}

	return fmt.Sprintf("--model %s %s", model, extraArgs)
}

// withSGLangServeArgs makes the stock lmsysorg/sglang image launch its OpenAI
// compatible server and injects the selected Hugging Face model unless the
// caller already supplied --model-path/--model.
func withSGLangServeArgs(extraArgs, model string) string {
	tokens := strings.Fields(extraArgs)
	hasServeCommand := len(tokens) >= 2 && tokens[0] == "sglang" && tokens[1] == "serve"
	hasModel := false
	for _, token := range tokens {
		if hasFlag(token, "--model-path") || hasFlag(token, "--model") {
			hasModel = true
			break
		}
	}

	args := strings.TrimSpace(extraArgs)
	if !hasServeCommand {
		args = appendArg("sglang serve", args)
	}
	if model == "" || hasModel {
		return args
	}

	tokens = strings.Fields(args)
	if len(tokens) >= 2 && tokens[0] == "sglang" && tokens[1] == "serve" {
		rest := strings.Join(tokens[2:], " ")
		return appendArg(fmt.Sprintf("sglang serve --model-path %s", model), rest)
	}
	return appendArg(args, "--model-path "+model)
}

func withLlamaModelArg(extraArgs, model string) string {
	if model == "" {
		return extraArgs
	}

	tokens := strings.Fields(extraArgs)
	for i := range tokens {
		if hasFlag(tokens[i], "-m") || hasFlag(tokens[i], "--model") {
			return extraArgs
		}
	}

	modelPath := model
	if !strings.HasPrefix(modelPath, "/") {
		modelPath = path.Join("/models", path.Base(modelPath))
	}

	if strings.TrimSpace(extraArgs) == "" {
		return fmt.Sprintf("-m %s", modelPath)
	}

	return fmt.Sprintf("-m %s %s", modelPath, extraArgs)
}

func validateImagePlatform(ctx context.Context, image string) error {
	cmd := exec.CommandContext(ctx, "docker", "manifest", "inspect", image)
	out, err := cmd.Output()
	if err != nil {
		if contextErr := ctx.Err(); contextErr != nil {
			return fmt.Errorf("docker manifest inspect canceled: %w", contextErr)
		}
		log.Printf("warning: unable to inspect image platform for %s: %v", image, err)
		return nil
	}

	supported, platforms, err := imageSupportsPlatform(out, runtime.GOOS, runtime.GOARCH)
	if err != nil {
		log.Printf("warning: unable to parse image platform for %s: %v", image, err)
		return nil
	}

	if !supported {
		return fmt.Errorf("image %s does not support host platform %s/%s (supported: %s)", image, runtime.GOOS, runtime.GOARCH, strings.Join(platforms, ", "))
	}

	return nil
}

func validatePulledImageArchitecture(ctx context.Context, image string) error {
	cmd := exec.CommandContext(ctx, "docker", "inspect", image, "--format", "{{.Architecture}}")
	out, err := cmd.Output()
	if err != nil {
		if contextErr := ctx.Err(); contextErr != nil {
			return fmt.Errorf("docker inspect canceled: %w", contextErr)
		}
		return fmt.Errorf("docker inspect failed: %w", err)
	}

	imageArch := normalizeArch(strings.TrimSpace(string(out)))
	hostArch := normalizeArch(runtime.GOARCH)
	if imageArch == "" {
		return fmt.Errorf("docker inspect returned empty architecture for %s", image)
	}

	if imageArch != hostArch {
		return fmt.Errorf("image %s architecture %s does not match host architecture %s", image, imageArch, hostArch)
	}

	return nil
}

func imageSupportsPlatform(manifestJSON []byte, hostOS, hostArch string) (bool, []string, error) {
	var manifest struct {
		Manifests []struct {
			Platform struct {
				Architecture string `json:"architecture"`
				OS           string `json:"os"`
				Variant      string `json:"variant"`
			} `json:"platform"`
		} `json:"manifests"`
		Architecture string `json:"architecture"`
		OS           string `json:"os"`
	}

	if err := json.Unmarshal(manifestJSON, &manifest); err != nil {
		return false, nil, err
	}

	var platforms []string
	normalizedHostArch := normalizeArch(hostArch)

	if len(manifest.Manifests) > 0 {
		for _, entry := range manifest.Manifests {
			platformOS := entry.Platform.OS
			platformArch := normalizeArch(entry.Platform.Architecture)
			if platformOS == "" || platformArch == "" {
				continue
			}

			platform := platformOS + "/" + platformArch
			if entry.Platform.Variant != "" {
				platform += "/" + entry.Platform.Variant
			}
			platforms = append(platforms, platform)

			if platformOS == hostOS && platformArch == normalizedHostArch {
				return true, platforms, nil
			}
		}

		if len(platforms) == 0 {
			return false, nil, fmt.Errorf("manifest list has no platform entries")
		}
		return false, platforms, nil
	}

	if manifest.OS == "" || manifest.Architecture == "" {
		return false, nil, fmt.Errorf("manifest missing os/architecture")
	}

	platform := manifest.OS + "/" + normalizeArch(manifest.Architecture)
	platforms = append(platforms, platform)
	if manifest.OS == hostOS && normalizeArch(manifest.Architecture) == normalizedHostArch {
		return true, platforms, nil
	}

	return false, platforms, nil
}

// probeContainerHealth checks if a container's service is responding.
// It tries the first external port it finds. For inference servers it hits /health,
// for other services it does a simple TCP dial.
// Returns "healthy", "unhealthy", or "starting".
func probeContainerHealth(ports map[string]string, image, serviceAddress string) string {
	if len(ports) == 0 {
		return ""
	}

	// Find the first external port
	var externalPort string
	for _, ext := range ports {
		externalPort = ext
		break
	}
	if externalPort == "" {
		return ""
	}

	host := strings.TrimSpace(serviceAddress)
	if host == "" {
		host = "127.0.0.1"
	}
	addr := net.JoinHostPort(host, externalPort)

	// For known inference servers, try their /health endpoint
	imageLower := strings.ToLower(image)
	if strings.Contains(imageLower, "vllm") || strings.Contains(imageLower, "sglang") || strings.Contains(imageLower, "llama") {
		client := &http.Client{Timeout: 2 * time.Second}
		resp, err := client.Get("http://" + addr + "/health")
		if err != nil {
			return "starting"
		}
		_ = resp.Body.Close()
		if resp.StatusCode == http.StatusOK {
			return "healthy"
		}
		return "unhealthy"
	}

	// For other services, check if the port is accepting connections
	conn, err := net.DialTimeout("tcp", addr, 2*time.Second)
	if err != nil {
		return "starting"
	}
	_ = conn.Close()
	return "healthy"
}

// scrapeVLLMMetrics fetches Prometheus metrics from a vLLM container's /metrics
// endpoint and parses generation and prompt throughput.
func scrapeVLLMMetrics(port string) (*VLLMMetrics, error) {
	return scrapeVLLMMetricsURL("http://127.0.0.1:"+port+"/metrics", "")
}

var inventoryMetricsHTTPClient = &http.Client{Timeout: 2 * time.Second}

func scrapeVLLMMetricsURL(metricsURL, apiKey string) (*VLLMMetrics, error) {
	return scrapeVLLMMetricsURLWithClient(inventoryMetricsHTTPClient, metricsURL, apiKey)
}

func scrapeVLLMMetricsURLWithClient(client *http.Client, metricsURL, apiKey string) (*VLLMMetrics, error) {
	request, err := http.NewRequest(http.MethodGet, metricsURL, nil)
	if err != nil {
		return nil, err
	}
	if apiKey != "" {
		request.Header.Set("Authorization", "Bearer "+apiKey)
	}
	resp, err := client.Do(request)
	if err != nil {
		return nil, redactSecretError(err, apiKey)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode != http.StatusOK {
		return nil, redactSecretError(&serviceHTTPStatusError{
			status:  resp.StatusCode,
			message: fmt.Sprintf("inference metrics returned %d", resp.StatusCode),
		}, apiKey)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 256*1024))
	if err != nil {
		return nil, err
	}

	m := &VLLMMetrics{}
	cachedPromptTokensFallback := 0.0
	for _, line := range strings.Split(string(body), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "#") || line == "" {
			continue
		}

		name, labels, value, ok := parsePrometheusMetricLine(line)
		if !ok {
			continue
		}

		if m.Model == "" {
			if model := firstMetricLabel(labels, "model_name", "model"); model != "" {
				m.Model = model
			}
		}

		switch name {
		case "vllm:avg_generation_throughput_toks_per_s", "sglang:gen_throughput":
			m.GenerationTokPerSec += value
			m.HasGenerationTokPerSec = true
			m.markNativeMetric(name)
		case "vllm:avg_prompt_throughput_toks_per_s":
			m.PromptTokPerSec += value
			m.HasPromptTokPerSec = true
			m.markNativeMetric(name)
		case "vllm:num_requests_running", "sglang:num_running_reqs":
			m.RequestsRunning += value
			m.HasRequestsRunning = true
			m.markNativeMetric(name)
		case "vllm:num_requests_waiting", "sglang:num_queue_reqs":
			m.RequestsWaiting += value
			m.HasRequestsWaiting = true
			m.markNativeMetric(name)
		case "vllm:prompt_tokens_total", "vllm:prompt_tokens", "sglang:prompt_tokens_total":
			m.PromptTokensTotal += value
			m.HasPromptTokensTotal = true
			m.markNativeMetric(name)
		case "vllm:generation_tokens_total", "vllm:generation_tokens", "sglang:generation_tokens_total":
			m.GenerationTokensTotal += value
			m.HasGenerationTokensTotal = true
			m.markNativeMetric(name)
		case "vllm:prompt_tokens_cached_total", "vllm:prompt_tokens_cached", "sglang:cached_tokens_total":
			m.CachedPromptTokensTotal += value
			m.HasCachedPromptTokens = true
			m.markNativeMetric(name)
		case "vllm:prefix_cache_hits_total", "vllm:prefix_cache_hits", "vllm:external_prefix_cache_hits_total", "vllm:external_prefix_cache_hits":
			cachedPromptTokensFallback += value
			m.markNativeMetric(name)
		case "vllm:time_to_first_token_seconds_bucket", "sglang:time_to_first_token_seconds_bucket":
			le := labels["le"]
			if le == "" {
				continue
			}
			if m.TTFTBuckets == nil {
				m.TTFTBuckets = make(map[string]float64)
			}
			m.TTFTBuckets[le] += value
			m.HasTTFT = true
			m.markNativeMetric(name)
		case "vllm:time_to_first_token_seconds_sum", "sglang:time_to_first_token_seconds_sum":
			m.TTFTSum += value
			m.HasTTFT = true
			m.markNativeMetric(name)
		case "vllm:time_to_first_token_seconds_count", "sglang:time_to_first_token_seconds_count":
			m.TTFTCount += value
			m.HasTTFT = true
			m.markNativeMetric(name)
		}
	}
	if !m.HasCachedPromptTokens && cachedPromptTokensFallback > 0 {
		m.CachedPromptTokensTotal = cachedPromptTokensFallback
		m.HasCachedPromptTokens = true
	}
	return m, nil
}

func (m *VLLMMetrics) markNativeMetric(name string) {
	switch {
	case strings.HasPrefix(name, "sglang:"):
		m.HasSGLangNativeMetric = true
	case strings.HasPrefix(name, "vllm:"):
		m.HasVLLMNativeMetric = true
	}
}

var prometheusMetricLinePattern = regexp.MustCompile(`^([^\s{]+)(?:\{([^}]*)\})?\s+([-+]?(?:\d+\.?\d*|\.\d+)(?:[eE][-+]?\d+)?)$`)

func parsePrometheusMetricLine(line string) (string, map[string]string, float64, bool) {
	matches := prometheusMetricLinePattern.FindStringSubmatch(line)
	if len(matches) != 4 {
		return "", nil, 0, false
	}

	value, err := strconv.ParseFloat(matches[3], 64)
	if err != nil {
		return "", nil, 0, false
	}

	return matches[1], parsePrometheusLabels(matches[2]), value, true
}

func parsePrometheusLabels(raw string) map[string]string {
	labels := make(map[string]string)
	if raw == "" {
		return labels
	}

	for _, match := range regexp.MustCompile(`([a-zA-Z_][a-zA-Z0-9_]*)="((?:\\.|[^"])*)"`).FindAllStringSubmatch(raw, -1) {
		if len(match) != 3 {
			continue
		}
		labels[match[1]] = strings.ReplaceAll(match[2], `\"`, `"`)
	}

	return labels
}

func firstMetricLabel(labels map[string]string, keys ...string) string {
	for _, key := range keys {
		if value := strings.TrimSpace(labels[key]); value != "" {
			return value
		}
	}
	return ""
}

// normalizeServicePorts remaps user-specified ports so the container-internal
// port is the service's default. For example, if the user chose host port 8253
// for vLLM (which listens on 8000 inside the container), this produces
// {"8000": "8253"} so Docker maps host:8253 → container:8000.
func normalizeServicePorts(ports map[string]string, defaultContainerPort string) map[string]string {
	if len(ports) == 0 {
		return map[string]string{defaultContainerPort: defaultContainerPort}
	}

	normalized := make(map[string]string, len(ports))
	for internal, external := range ports {
		// If the internal port matches the default, keep as-is
		if internal == defaultContainerPort {
			normalized[internal] = external
			continue
		}
		// The user likely set both sides to the same host port (e.g. "8253":"8253").
		// Remap so the container side uses the service's default port.
		if internal == external {
			normalized[defaultContainerPort] = external
		} else {
			// User explicitly set different internal/external — respect it
			normalized[internal] = external
		}
	}

	return normalized
}

// withHostArg injects a host bind flag (e.g. --host 0.0.0.0) into the extra
// args string if it's not already present.
// withVLLMToolCallArgs adds --enable-auto-tool-choice and --tool-call-parser
// to vLLM extra args if not already present. The parser is inferred from the
// model name so the right format is used for each model family.
func withVLLMToolCallArgs(extraArgs, model string) string {
	tokens := strings.Fields(extraArgs)
	hasAutoTool := false
	hasParser := false
	for _, t := range tokens {
		if hasFlag(t, "--enable-auto-tool-choice") {
			hasAutoTool = true
		}
		if hasFlag(t, "--tool-call-parser") {
			hasParser = true
		}
	}

	if !hasAutoTool {
		extraArgs = appendArg(extraArgs, "--enable-auto-tool-choice")
	}
	if !hasParser {
		parser := inferToolCallParser(model)
		extraArgs = appendArg(extraArgs, "--tool-call-parser "+parser)
	}
	return extraArgs
}

// inferToolCallParser returns the best vLLM --tool-call-parser value for a model.
func inferToolCallParser(model string) string {
	m := strings.ToLower(model)

	switch {
	case strings.Contains(m, "llama"):
		return "llama3_json"
	case strings.Contains(m, "mistral"), strings.Contains(m, "mixtral"):
		return "mistral"
	case strings.Contains(m, "jamba"):
		return "jamba"
	case strings.Contains(m, "internlm"):
		return "internlm"
	case strings.Contains(m, "granite"):
		return "granite"
	case strings.Contains(m, "qwen3-coder"), strings.Contains(m, "qwen3coder"):
		return "qwen3_xml"
	case strings.Contains(m, "qwen3.5"), strings.Contains(m, "qwen3_5"):
		return "qwen3_coder"
	case strings.Contains(m, "qwen"), strings.Contains(m, "hermes"):
		return "hermes"
	default:
		// hermes is the most broadly compatible fallback
		return "hermes"
	}
}

func appendArg(extraArgs, arg string) string {
	if strings.TrimSpace(extraArgs) == "" {
		return arg
	}
	return extraArgs + " " + arg
}

func withHostArg(extraArgs, flag, value string) string {
	tokens := strings.Fields(extraArgs)
	for _, t := range tokens {
		if hasFlag(t, flag) {
			return extraArgs // already present
		}
	}

	hostArg := fmt.Sprintf("%s %s", flag, value)
	if strings.TrimSpace(extraArgs) == "" {
		return hostArg
	}
	return fmt.Sprintf("%s %s", extraArgs, hostArg)
}

func hasFlag(token, flag string) bool {
	return token == flag || strings.HasPrefix(token, flag+"=")
}

// isComfyUIImage checks if the image is a ComfyUI image.
func isComfyUIImage(image string) bool {
	return strings.Contains(strings.ToLower(image), "comfyui") ||
		strings.Contains(strings.ToLower(image), "comfy-ui")
}

func normalizeArch(arch string) string {
	switch strings.ToLower(arch) {
	case "x86_64":
		return "amd64"
	case "aarch64":
		return "arm64"
	default:
		return strings.ToLower(arch)
	}
}
