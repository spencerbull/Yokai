package agent

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os/exec"
	"path"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// VLLMMetrics holds vLLM inference throughput metrics.
type VLLMMetrics struct {
	GenerationTokPerSec float64 `json:"generation_tok_per_s"`
	PromptTokPerSec     float64 `json:"prompt_tok_per_s"`
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
}

// ContainerRequest represents a container deployment request.
type ContainerRequest struct {
	Image     string            `json:"image"`
	Name      string            `json:"name"`
	Model     string            `json:"model"`
	Ports     map[string]string `json:"ports"`
	Env       map[string]string `json:"env"`
	GPUIDs    string            `json:"gpu_ids"`
	ExtraArgs string            `json:"extra_args"`
	Volumes   map[string]string `json:"volumes"`
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

// listContainers returns all yokai-* containers.
func listContainers() ([]Container, error) {
	cmd := exec.Command("docker", "ps", "-a", "--filter", "name=yokai-", "--format", "json")
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

		container := Container{
			ID:      dockerContainer.ID,
			Name:    strings.TrimPrefix(dockerContainer.Names, "/"),
			Image:   dockerContainer.Image,
			Status:  parseStatus(dockerContainer.Status),
			Ports:   ports,
			Created: created,
			Uptime:  uptime,
		}

		// Scrape vLLM metrics for vLLM containers
		if isVLLMImage(container.Image) && container.Status == "running" {
			for _, ext := range container.Ports {
				if ext != "" {
					if vm, err := scrapeVLLMMetrics(ext); err == nil {
						container.VLLMMetrics = vm
					}
					break
				}
			}
		}

		containers = append(containers, container)
	}

	return containers, nil
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
	return "unknown"
}

// runContainer deploys a new container.
func runContainer(req ContainerRequest) (*ContainerResponse, error) {
	if err := validateImagePlatform(req.Image); err != nil {
		return nil, err
	}

	// Sanitize container name
	containerName := fmt.Sprintf("yokai-%s", sanitizeName(req.Name))

	if isLlamaCppImage(req.Image) {
		if req.Model != "" {
			if req.Volumes == nil {
				req.Volumes = make(map[string]string)
			}
			ensureModelsVolume(req.Volumes)
			req.ExtraArgs = withLlamaModelArg(req.ExtraArgs, req.Model)
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
			req.ExtraArgs = withVLLMModelArg(req.ExtraArgs, req.Model)
		}
		req.Ports = normalizeServicePorts(req.Ports, "8000")
		req.ExtraArgs = withHostArg(req.ExtraArgs, "--host", "0.0.0.0")
		req.ExtraArgs = withVLLMToolCallArgs(req.ExtraArgs, req.Model)
	}

	if isComfyUIImage(req.Image) {
		req.Ports = normalizeServicePorts(req.Ports, "8188")
	}

	// Build docker run command
	args := []string{"run", "-d", "--name", containerName}

	// Add ports — bind to 0.0.0.0 explicitly for external access
	for internal, external := range req.Ports {
		args = append(args, "-p", fmt.Sprintf("0.0.0.0:%s:%s", external, internal))
	}

	// Add environment variables
	for key, value := range req.Env {
		args = append(args, "-e", fmt.Sprintf("%s=%s", key, value))
	}

	// Add GPU support
	if req.GPUIDs != "" {
		if req.GPUIDs == "all" {
			args = append(args, "--gpus", "all")
		} else {
			args = append(args, "--gpus", fmt.Sprintf(`"device=%s"`, req.GPUIDs))
		}
	}

	// Add volumes
	for host, container := range req.Volumes {
		args = append(args, "-v", fmt.Sprintf("%s:%s", host, container))
	}

	// Add restart policy
	args = append(args, "--restart", "unless-stopped")

	// Add image
	args = append(args, req.Image)

	// Add extra args
	if req.ExtraArgs != "" {
		extraArgs := strings.Fields(req.ExtraArgs)
		args = append(args, extraArgs...)
	}

	// Run the container
	cmd := exec.Command("docker", args...)
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("docker run failed: %w", err)
	}

	containerID := strings.TrimSpace(string(out))

	return &ContainerResponse{
		ID:     containerID[:12], // Short ID
		Status: "created",
	}, nil
}

// stopContainer stops a container by ID or name.
func stopContainer(idOrName string) error {
	cmd := exec.Command("docker", "stop", idOrName)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("docker stop failed: %w", err)
	}
	return nil
}

// removeContainer removes a stopped container.
func removeContainer(idOrName string) error {
	cmd := exec.Command("docker", "rm", "-f", idOrName)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("docker rm failed: %w", err)
	}
	return nil
}

// restartContainer restarts a container.
func restartContainer(idOrName string) error {
	cmd := exec.Command("docker", "restart", idOrName)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("docker restart failed: %w", err)
	}
	return nil
}

// pullImage pulls a Docker image.
func pullImage(image string) error {
	cmd := exec.Command("docker", "pull", image)
	if err := cmd.Run(); err != nil {
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

func withVLLMModelArg(extraArgs, model string) string {
	if model == "" {
		return extraArgs
	}

	tokens := strings.Fields(extraArgs)
	for i := range tokens {
		if tokens[i] == "--model" {
			return extraArgs
		}
	}

	if strings.TrimSpace(extraArgs) == "" {
		return fmt.Sprintf("--model %s", model)
	}

	return fmt.Sprintf("--model %s %s", model, extraArgs)
}

func withLlamaModelArg(extraArgs, model string) string {
	if model == "" {
		return extraArgs
	}

	tokens := strings.Fields(extraArgs)
	for i := range tokens {
		if tokens[i] == "-m" || tokens[i] == "--model" {
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

func validateImagePlatform(image string) error {
	cmd := exec.Command("docker", "manifest", "inspect", image)
	out, err := cmd.Output()
	if err != nil {
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

func validatePulledImageArchitecture(image string) error {
	cmd := exec.Command("docker", "inspect", image, "--format", "{{.Architecture}}")
	out, err := cmd.Output()
	if err != nil {
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
// It tries the first external port it finds. For vLLM/llama.cpp it hits /health,
// for other services it does a simple TCP dial.
// Returns "healthy", "unhealthy", or "starting".
func probeContainerHealth(ports map[string]string, image string) string {
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

	addr := "127.0.0.1:" + externalPort

	// For known inference servers, try their /health endpoint
	imageLower := strings.ToLower(image)
	if strings.Contains(imageLower, "vllm") || strings.Contains(imageLower, "llama") {
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
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get("http://127.0.0.1:" + port + "/metrics")
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("vllm metrics returned %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 256*1024))
	if err != nil {
		return nil, err
	}

	m := &VLLMMetrics{}
	for _, line := range strings.Split(string(body), "\n") {
		if strings.HasPrefix(line, "#") || line == "" {
			continue
		}
		if strings.HasPrefix(line, "vllm:avg_generation_throughput_toks_per_s") {
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				m.GenerationTokPerSec, _ = strconv.ParseFloat(parts[len(parts)-1], 64)
			}
		} else if strings.HasPrefix(line, "vllm:avg_prompt_throughput_toks_per_s") {
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				m.PromptTokPerSec, _ = strconv.ParseFloat(parts[len(parts)-1], 64)
			}
		}
	}
	return m, nil
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
		if t == "--enable-auto-tool-choice" {
			hasAutoTool = true
		}
		if t == "--tool-call-parser" {
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
		if t == flag {
			return extraArgs // already present
		}
	}

	hostArg := fmt.Sprintf("%s %s", flag, value)
	if strings.TrimSpace(extraArgs) == "" {
		return hostArg
	}
	return fmt.Sprintf("%s %s", extraArgs, hostArg)
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
