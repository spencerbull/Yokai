package agent

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"regexp"
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
	// Sanitize container name
	containerName := fmt.Sprintf("yokai-%s", sanitizeName(req.Name))

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

// inspectContainer gets detailed container info.
func inspectContainer(idOrName string) (map[string]interface{}, error) {
	cmd := exec.Command("docker", "inspect", idOrName)
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("docker inspect failed: %w", err)
	}

	var result []map[string]interface{}
	if err := json.Unmarshal(out, &result); err != nil {
		return nil, fmt.Errorf("parsing inspect output: %w", err)
	}

	if len(result) == 0 {
		return nil, fmt.Errorf("container not found")
	}

	return result[0], nil
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
