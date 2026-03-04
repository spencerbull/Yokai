package ssh

import (
	"fmt"
	"strings"
)

// PreflightResult holds the results of remote pre-flight checks.
type PreflightResult struct {
	OS                     string
	Arch                   string
	DockerInstalled        bool
	DockerVersion          string
	GPUDetected            bool
	GPUName                string
	GPUVRAMMb              int
	NvidiaToolkitInstalled bool
	NvidiaRuntimeAvailable bool
	DiskFreeGB             int
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
				if _, err := fmt.Sscanf(strings.TrimSpace(parts[1]), "%d", &result.GPUVRAMMb); err != nil {
					result.GPUVRAMMb = 0
				}
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
		if _, err := fmt.Sscanf(strings.TrimSpace(out), "%dG", &result.DiskFreeGB); err != nil {
			result.DiskFreeGB = 0
		}
	}

	return result, nil
}

// UpgradeAgent replaces the agent binary on a remote device and restarts it.
// Detects the running agent's binary path, uploads the new binary to /tmp,
// sudo-moves it into place, and restarts via systemd if available.
func UpgradeAgent(client *Client, localBinaryPath string, agentPort int) error {
	// Find the running agent process to determine the remote binary path
	out, err := client.Exec("pgrep -a -f 'yokai agent'")
	if err != nil {
		return fmt.Errorf("agent not running on remote (pgrep failed): %w", err)
	}

	remoteBinPath := ""
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		if line == "" {
			continue
		}
		// Format: "PID /path/to/yokai agent 7474"
		fields := strings.Fields(line)
		if len(fields) >= 2 {
			remoteBinPath = fields[1]
			break
		}
	}

	if remoteBinPath == "" {
		return fmt.Errorf("could not determine remote agent binary path")
	}

	// Upload to user-writable /tmp first, then sudo mv into place
	tmpPath := "/tmp/yokai.new"
	if err := client.Upload(localBinaryPath, tmpPath); err != nil {
		return fmt.Errorf("uploading binary: %w", err)
	}

	cmds := []string{
		fmt.Sprintf("chmod +x %s", tmpPath),
		fmt.Sprintf("sudo mv -f %s %s", tmpPath, remoteBinPath),
	}
	for _, cmd := range cmds {
		if _, err := client.Exec(cmd); err != nil {
			return fmt.Errorf("running %q: %w", cmd, err)
		}
	}

	// Restart: prefer systemd if the service exists, otherwise manual restart
	if _, err := client.Exec("systemctl list-unit-files yokai-agent.service 2>/dev/null | grep -q yokai-agent"); err == nil {
		if _, err := client.Exec("sudo systemctl restart yokai-agent"); err != nil {
			return fmt.Errorf("restarting agent via systemd: %w", err)
		}
	} else {
		// Fallback: manual kill + setsid restart
		_, _ = client.Exec("pkill -f 'yokai agent'")
		_, _ = client.Exec("sleep 0.5")
		startCmd := fmt.Sprintf("setsid %s agent %d > /tmp/yokai-agent.log 2>&1 < /dev/null &", remoteBinPath, agentPort)
		if _, err := client.Exec(startCmd); err != nil {
			return fmt.Errorf("restarting agent: %w", err)
		}
	}

	// Verify it came back up
	_, _ = client.Exec("sleep 1.5")
	verifyCmd := fmt.Sprintf("curl -sf http://127.0.0.1:%d/health", agentPort)
	if _, err := client.Exec(verifyCmd); err != nil {
		return fmt.Errorf("agent failed to start after upgrade (health check failed)")
	}

	return nil
}

// DeployAgent uploads the yokai binary and installs it as a systemd service.
func DeployAgent(client *Client, localBinaryPath string, agentToken string) error {
	remoteBinPath := "/usr/local/bin/yokai"
	remoteConfigDir := "/etc/yokai"
	tmpUploadPath := "/tmp/yokai.new"

	// Upload binary to a user-writable temp path first, then sudo mv into place.
	// SCP runs as the SSH user who may not have write access to /usr/local/bin.
	if err := client.Upload(localBinaryPath, tmpUploadPath); err != nil {
		return fmt.Errorf("uploading binary: %w", err)
	}

	// Move into place and make executable
	moveCmds := []string{
		fmt.Sprintf("chmod +x %s", tmpUploadPath),
		fmt.Sprintf("sudo mv -f %s %s", tmpUploadPath, remoteBinPath),
	}
	for _, cmd := range moveCmds {
		if _, err := client.Exec(cmd); err != nil {
			return fmt.Errorf("running %q: %w", cmd, err)
		}
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
