package agent

import (
	"encoding/json"
	"fmt"
	"log"
	"os/exec"
	"path"
	"regexp"
	"runtime"
	"strings"
	"time"
)

// Container represents a running container.
type Container struct {
	ID      string            `json:"id"`
	Name    string            `json:"name"`
	Image   string            `json:"image"`
	Status  string            `json:"status"`
	Ports   map[string]string `json:"ports"`
	Created time.Time         `json:"created"`
	Uptime  int64             `json:"uptime_seconds"`
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

	if isLlamaCppImage(req.Image) && req.Model != "" {
		if req.Volumes == nil {
			req.Volumes = make(map[string]string)
		}
		ensureModelsVolume(req.Volumes)
		req.ExtraArgs = withLlamaModelArg(req.ExtraArgs, req.Model)
	}

	// Build docker run command
	args := []string{"run", "-d", "--name", containerName}

	// Add ports
	for internal, external := range req.Ports {
		args = append(args, "-p", fmt.Sprintf("%s:%s", external, internal))
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
