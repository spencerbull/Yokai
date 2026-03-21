package ssh

import (
	"fmt"
	"strings"
)

type remoteRunner interface {
	Exec(cmd string) (string, error)
	Upload(localPath, remotePath string) error
}

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

// isSudoError returns true if the error output indicates sudo requires a password or tty.
func isSudoError(output string) bool {
	lower := strings.ToLower(output)
	return strings.Contains(lower, "a password is required") ||
		strings.Contains(lower, "no tty present") ||
		strings.Contains(lower, "sudo: a terminal is required")
}

// isUserWritable returns true if the path is under the user's home directory.
func isUserWritable(path string) bool {
	return strings.HasPrefix(path, "$HOME/") ||
		strings.HasPrefix(path, "~/") ||
		strings.Contains(path, "/.local/") ||
		strings.Contains(path, "/.config/")
}

// UpgradeAgent replaces the agent binary on a remote device and restarts it.
// Detects the running agent's binary path, uploads the new binary to /tmp,
// moves it into place (no sudo if user-local), and restarts the service.
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

	// Upload to user-writable /tmp first
	tmpPath := "/tmp/yokai.new"
	if err := client.Upload(localBinaryPath, tmpPath); err != nil {
		return fmt.Errorf("uploading binary: %w", err)
	}

	if _, err := client.Exec(fmt.Sprintf("chmod +x %s", tmpPath)); err != nil {
		return fmt.Errorf("chmod binary: %w", err)
	}

	// Determine if we need sudo to replace the binary
	homeDir := getRemoteHome(client)
	needsSudo := !strings.HasPrefix(remoteBinPath, homeDir+"/")

	if needsSudo {
		// System-level install — try sudo, give clear error on failure
		mvCmd := fmt.Sprintf("sudo mv -f %s %s", tmpPath, remoteBinPath)
		if mvOut, err := client.Exec(mvCmd); err != nil {
			if isSudoError(mvOut) {
				return fmt.Errorf("agent is installed system-wide at %s but sudo requires a password. "+
					"Run `sudo mv /tmp/yokai.new %s` on the device, or reinstall with user-local mode: %s",
					remoteBinPath, remoteBinPath, strings.TrimSpace(mvOut))
			}
			return fmt.Errorf("running %q: %w — stderr: %s", mvCmd, err, strings.TrimSpace(mvOut))
		}
	} else {
		// User-local install — no sudo needed
		mvCmd := fmt.Sprintf("mv -f %s %s", tmpPath, remoteBinPath)
		if mvOut, err := client.Exec(mvCmd); err != nil {
			return fmt.Errorf("running %q: %w — stderr: %s", mvCmd, err, strings.TrimSpace(mvOut))
		}
	}

	// Restart: try user-level systemd first, then system-level, then setsid
	restarted := false

	// 1. Try user-level systemd
	if _, err := client.Exec("systemctl --user list-unit-files yokai-agent.service 2>/dev/null | grep -q yokai-agent"); err == nil {
		if restartOut, err := client.Exec("systemctl --user restart yokai-agent"); err == nil {
			restarted = true
		} else {
			return fmt.Errorf("restarting agent via systemd --user: %w — stderr: %s", err, strings.TrimSpace(restartOut))
		}
	}

	// 2. Try system-level systemd
	if !restarted {
		if _, err := client.Exec("systemctl list-unit-files yokai-agent.service 2>/dev/null | grep -q yokai-agent"); err == nil {
			if restartOut, err := client.Exec("sudo systemctl restart yokai-agent"); err == nil {
				restarted = true
			} else if isSudoError(restartOut) {
				return fmt.Errorf("agent uses system-level systemd but sudo requires a password. "+
					"Run `sudo systemctl restart yokai-agent` on the device: %s", strings.TrimSpace(restartOut))
			} else {
				return fmt.Errorf("restarting agent via systemd: %w — stderr: %s", err, strings.TrimSpace(restartOut))
			}
		}
	}

	// 3. Fallback: manual kill + setsid restart
	if !restarted {
		_, _ = client.Exec("pkill -f 'yokai agent'")
		_, _ = client.Exec("sleep 0.5")
		startCmd := fmt.Sprintf("setsid %s agent %d > /tmp/yokai-agent.log 2>&1 < /dev/null &", remoteBinPath, agentPort)
		if startOut, err := client.Exec(startCmd); err != nil {
			return fmt.Errorf("restarting agent: %w — stderr: %s", err, strings.TrimSpace(startOut))
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

// getRemoteHome returns the remote user's home directory.
func getRemoteHome(client remoteRunner) string {
	if out, err := client.Exec("echo $HOME"); err == nil {
		return strings.TrimSpace(out)
	}
	return "/home/unknown"
}

func hasSystemService(client remoteRunner) bool {
	if _, err := client.Exec("systemctl is-active --quiet yokai-agent 2>/dev/null"); err == nil {
		return true
	}
	if _, err := client.Exec("systemctl list-unit-files yokai-agent.service 2>/dev/null | grep -q yokai-agent"); err == nil {
		return true
	}
	return false
}

func remoteIsRoot(client remoteRunner) bool {
	out, err := client.Exec("id -u 2>/dev/null")
	if err != nil {
		return false
	}
	return strings.TrimSpace(out) == "0"
}

func ensureSudoNonInteractive(client remoteRunner) error {
	out, err := client.Exec("sudo -n true 2>&1")
	if err == nil {
		return nil
	}

	trimmed := strings.TrimSpace(out)
	if isSudoError(trimmed) {
		return fmt.Errorf("existing system-level yokai-agent detected, but sudo is required to update /etc/yokai/agent.json and restart the service: %s", trimmed)
	}
	return fmt.Errorf("checking sudo access: %w — stderr: %s", err, trimmed)
}

func parseSystemExecBinaryPath(output string) string {
	for _, field := range strings.Fields(output) {
		field = strings.Trim(field, "{};,")
		if strings.HasPrefix(field, "path=") {
			field = strings.TrimPrefix(field, "path=")
		}
		if strings.HasPrefix(field, "/") && strings.HasSuffix(field, "/yokai") {
			return field
		}
	}
	return ""
}

func systemAgentBinaryPath(client remoteRunner) string {
	out, err := client.Exec("systemctl show -p ExecStart --value yokai-agent")
	if err == nil {
		if parsed := parseSystemExecBinaryPath(out); parsed != "" {
			return parsed
		}
	}
	return "/usr/local/bin/yokai"
}

func verifyAgentAuth(client remoteRunner, port int, agentToken string) error {
	verifyCmd := fmt.Sprintf("curl -sf -H 'Authorization: Bearer %s' http://127.0.0.1:%d/system/info >/dev/null", agentToken, port)
	if out, err := client.Exec(verifyCmd); err != nil {
		return fmt.Errorf("agent auth verification failed (token mismatch or agent unavailable): %w — stderr: %s", err, strings.TrimSpace(out))
	}
	return nil
}

func sudoPrefix(useSudo bool) string {
	if useSudo {
		return "sudo -n "
	}
	return ""
}

func deploySystemService(client remoteRunner, tmpUploadPath, agentToken string) error {
	// Avoid conflicts on :7474 if a stale user-level service exists.
	_, _ = client.Exec("systemctl --user disable --now yokai-agent 2>/dev/null || true")

	useSudo := !remoteIsRoot(client)
	if useSudo {
		if err := ensureSudoNonInteractive(client); err != nil {
			return err
		}
	}

	binaryPath := systemAgentBinaryPath(client)
	installCmd := fmt.Sprintf("%sinstall -m 0755 %s %s", sudoPrefix(useSudo), tmpUploadPath, binaryPath)
	if installOut, err := client.Exec(installCmd); err != nil {
		return fmt.Errorf("running %q: %w — stderr: %s", installCmd, err, strings.TrimSpace(installOut))
	}

	writeTokenCmd := fmt.Sprintf(`%ssh -c 'mkdir -p /etc/yokai && cat > /etc/yokai/agent.json << "EOF"
{
  "token": "%s"
}
EOF'`, sudoPrefix(useSudo), agentToken)
	if tokenOut, err := client.Exec(writeTokenCmd); err != nil {
		return fmt.Errorf("writing /etc/yokai/agent.json: %w — stderr: %s", err, strings.TrimSpace(tokenOut))
	}

	for _, cmd := range []string{
		fmt.Sprintf("%ssystemctl daemon-reload", sudoPrefix(useSudo)),
		fmt.Sprintf("%ssystemctl enable yokai-agent", sudoPrefix(useSudo)),
		fmt.Sprintf("%ssystemctl restart yokai-agent", sudoPrefix(useSudo)),
	} {
		if cmdOut, err := client.Exec(cmd); err != nil {
			return fmt.Errorf("running %q: %w — stderr: %s", cmd, err, strings.TrimSpace(cmdOut))
		}
	}

	_, _ = client.Exec("sleep 1.5")
	if err := verifyAgentAuth(client, 7474, agentToken); err != nil {
		return err
	}

	return nil
}

func deployUserService(client remoteRunner, tmpUploadPath, agentToken string) error {
	homeDir := getRemoteHome(client)
	remoteBinDir := homeDir + "/.local/bin"
	remoteBinPath := remoteBinDir + "/yokai"
	remoteConfigDir := homeDir + "/.config/yokai"
	remoteSystemdDir := homeDir + "/.config/systemd/user"

	// Create ~/.local/bin and install binary
	installCmds := []string{
		fmt.Sprintf("mkdir -p %s", remoteBinDir),
		fmt.Sprintf("chmod +x %s", tmpUploadPath),
		fmt.Sprintf("mv -f %s %s", tmpUploadPath, remoteBinPath),
	}
	for _, cmd := range installCmds {
		if cmdOut, err := client.Exec(cmd); err != nil {
			return fmt.Errorf("running %q: %w — stderr: %s", cmd, err, strings.TrimSpace(cmdOut))
		}
	}

	// Ensure ~/.local/bin is in PATH (add to .bashrc and .profile if missing)
	for _, rcFile := range []string{homeDir + "/.bashrc", homeDir + "/.profile"} {
		checkCmd := fmt.Sprintf("grep -q '%s' %s 2>/dev/null", remoteBinDir, rcFile)
		if _, err := client.Exec(checkCmd); err != nil {
			appendCmd := fmt.Sprintf(`echo 'export PATH="%s:$PATH"' >> %s`, remoteBinDir, rcFile)
			_, _ = client.Exec(appendCmd)
		}
	}

	// Create config directory and write agent config
	configCmds := []string{
		fmt.Sprintf("mkdir -p %s", remoteConfigDir),
		fmt.Sprintf(`cat > %s/agent.json << 'EOF'
{
  "token": "%s"
}
EOF`, remoteConfigDir, agentToken),
	}
	for _, cmd := range configCmds {
		if cmdOut, err := client.Exec(cmd); err != nil {
			return fmt.Errorf("running %q: %w — stderr: %s", cmd, err, strings.TrimSpace(cmdOut))
		}
	}

	// Check if user-level systemd is available.
	// systemctl --user status may return non-zero even when working, so we
	// test with daemon-reload which reliably fails without a user session.
	_, userSystemdErr := client.Exec("systemctl --user daemon-reload 2>&1")

	if userSystemdErr == nil {
		// User-level systemd available — install service
		serviceUnit := fmt.Sprintf(`[Unit]
Description=yokai agent
After=network.target docker.service
Wants=docker.service

[Service]
Type=simple
ExecStart=%s agent
Environment=YOKAI_AGENT_CONFIG=%s/agent.json
Restart=always
RestartSec=5

[Install]
WantedBy=default.target
`, remoteBinPath, remoteConfigDir)

		setupCmds := []string{
			fmt.Sprintf("mkdir -p %s", remoteSystemdDir),
			fmt.Sprintf("cat > %s/yokai-agent.service << 'SERVICEEOF'\n%sSERVICEEOF", remoteSystemdDir, serviceUnit),
			"systemctl --user daemon-reload",
			"systemctl --user enable yokai-agent",
			"systemctl --user restart yokai-agent",
		}
		for _, cmd := range setupCmds {
			if cmdOut, err := client.Exec(cmd); err != nil {
				return fmt.Errorf("running %q: %w — stderr: %s", cmd, err, strings.TrimSpace(cmdOut))
			}
		}

		// Enable lingering so the service runs without an active login session.
		// This may require sudo on some systems — warn but don't fail.
		if lingerOut, err := client.Exec("loginctl enable-linger $(whoami) 2>&1"); err != nil {
			if isSudoError(lingerOut) {
				fmt.Printf("warning: could not enable lingering (sudo required). The agent service may stop when you log out. Run `sudo loginctl enable-linger %s` on the device.\n", getUserName(client))
			}
			// Non-fatal: the service will still work while the user session is active
		}
	} else {
		// No user-level systemd — fall back to setsid background process
		_, _ = client.Exec("pkill -f 'yokai agent'")
		_, _ = client.Exec("sleep 0.5")
		startCmd := fmt.Sprintf("setsid %s agent > /tmp/yokai-agent.log 2>&1 < /dev/null &", remoteBinPath)
		if startOut, err := client.Exec(startCmd); err != nil {
			return fmt.Errorf("starting agent: %w — stderr: %s", err, strings.TrimSpace(startOut))
		}
	}

	_, _ = client.Exec("sleep 1.5")
	if err := verifyAgentAuth(client, 7474, agentToken); err != nil {
		return err
	}

	return nil
}

func deployAgent(client remoteRunner, localBinaryPath string, agentToken string) error {
	tmpUploadPath := "/tmp/yokai.new"

	// Upload binary to /tmp first, then install according to service mode.
	if err := client.Upload(localBinaryPath, tmpUploadPath); err != nil {
		return fmt.Errorf("uploading binary: %w", err)
	}

	if hasSystemService(client) {
		return deploySystemService(client, tmpUploadPath, agentToken)
	}

	return deployUserService(client, tmpUploadPath, agentToken)
}

// DeployAgent uploads the yokai binary and installs it as a user-level or
// existing system-level service depending on what is already present.
func DeployAgent(client *Client, localBinaryPath string, agentToken string) error {
	return deployAgent(client, localBinaryPath, agentToken)
}

// getUserName returns the remote username.
func getUserName(client remoteRunner) string {
	if out, err := client.Exec("whoami"); err == nil {
		return strings.TrimSpace(out)
	}
	return "$(whoami)"
}
