package ssh

import (
	"fmt"
	"strings"
)

// PreflightResult holds the results of remote pre-flight checks.
type PreflightResult struct {
	OS                    string
	Arch                  string
	DockerInstalled       bool
	DockerVersion         string
	GPUDetected           bool
	GPUName               string
	GPUVRAMMb             int
	NvidiaToolkitInstalled bool
	NvidiaRuntimeAvailable bool
	DiskFreeGB            int
}

// Preflight runs pre-flight checks on a remote device via SSH.
func Preflight(client *Client) (*PreflightResult, error) {
	result := &PreflightResult{}

	// OS info
	if out, err := client.Exec("cat /etc/os-release 2>/dev/null | grep PRETTY_NAME | cut -d= -f2 | tr -d '\"'"); err == nil {
		result.OS = strings.TrimSpace(out)
	}

	// Arch
	if out, err := client.Exec("uname -m"); err == nil {
		result.Arch = strings.TrimSpace(out)
	}

	// Docker
	if out, err := client.Exec("docker --version 2>/dev/null"); err == nil {
		result.DockerInstalled = true
		result.DockerVersion = strings.TrimSpace(out)
	}

	// NVIDIA GPU
	if out, err := client.Exec("nvidia-smi --query-gpu=name,memory.total --format=csv,noheader,nounits 2>/dev/null"); err == nil {
		out = strings.TrimSpace(out)
		if out != "" {
			result.GPUDetected = true
			parts := strings.SplitN(out, ",", 2)
			if len(parts) >= 1 {
				result.GPUName = strings.TrimSpace(parts[0])
			}
			if len(parts) >= 2 {
				fmt.Sscanf(strings.TrimSpace(parts[1]), "%d", &result.GPUVRAMMb)
			}
		}
	}

	// nvidia-container-toolkit
	if _, err := client.Exec("which nvidia-container-toolkit 2>/dev/null"); err == nil {
		result.NvidiaToolkitInstalled = true
	}

	// Docker nvidia runtime
	if out, err := client.Exec("docker info --format '{{.Runtimes}}' 2>/dev/null"); err == nil {
		if strings.Contains(out, "nvidia") {
			result.NvidiaRuntimeAvailable = true
		}
	}

	// Disk free
	if out, err := client.Exec("df -BG --output=avail / 2>/dev/null | tail -1"); err == nil {
		fmt.Sscanf(strings.TrimSpace(out), "%dG", &result.DiskFreeGB)
	}

	return result, nil
}

// DeployAgent uploads the yokai binary and installs it as a systemd service.
func DeployAgent(client *Client, localBinaryPath string, agentToken string) error {
	remoteBinPath := "/usr/local/bin/yokai"
	remoteConfigDir := "/etc/yokai"

	// Upload binary
	if err := client.Upload(localBinaryPath, remoteBinPath); err != nil {
		return fmt.Errorf("uploading binary: %w", err)
	}

	// Make executable
	if _, err := client.Exec(fmt.Sprintf("chmod +x %s", remoteBinPath)); err != nil {
		return fmt.Errorf("chmod: %w", err)
	}

	// Create config dir and agent config
	cmds := []string{
		fmt.Sprintf("sudo mkdir -p %s", remoteConfigDir),
		fmt.Sprintf(`sudo tee %s/agent.json > /dev/null << 'EOF'
{
  "token": "%s"
}
EOF`, remoteConfigDir, agentToken),
	}

	for _, cmd := range cmds {
		if _, err := client.Exec(cmd); err != nil {
			return fmt.Errorf("running %q: %w", cmd, err)
		}
	}

	// Install systemd service
	serviceUnit := `[Unit]
Description=yokai agent
After=network.target docker.service
Wants=docker.service

[Service]
Type=simple
ExecStart=/usr/local/bin/yokai agent
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
`
	writeCmd := fmt.Sprintf("sudo tee /etc/systemd/system/yokai-agent.service > /dev/null << 'EOF'\n%sEOF", serviceUnit)
	if _, err := client.Exec(writeCmd); err != nil {
		return fmt.Errorf("writing systemd unit: %w", err)
	}

	// Enable and start
	startCmds := []string{
		"sudo systemctl daemon-reload",
		"sudo systemctl enable yokai-agent",
		"sudo systemctl restart yokai-agent",
	}
	for _, cmd := range startCmds {
		if _, err := client.Exec(cmd); err != nil {
			return fmt.Errorf("running %q: %w", cmd, err)
		}
	}

	return nil
}
