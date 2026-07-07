package cli

import (
	"crypto/rand"
	"encoding/hex"
	"flag"
	"fmt"
	"net/http"
	"os"
	"strconv"

	"github.com/spencerbull/yokai/internal/config"
	sshpkg "github.com/spencerbull/yokai/internal/ssh"
)

// RunDevices dispatches devices subcommands.
func RunDevices(args []string) {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "Usage: yokai devices <list|add|remove|test|bootstrap>")
		os.Exit(1)
	}

	switch args[0] {
	case "list":
		runDevicesList(args[1:])
	case "add":
		runDevicesAdd(args[1:])
	case "remove":
		runDevicesRemove(args[1:])
	case "test":
		runDevicesTest(args[1:])
	case "bootstrap":
		runDevicesBootstrap(args[1:])
	default:
		fmt.Fprintf(os.Stderr, "Unknown devices command: %s\n", args[0])
		os.Exit(1)
	}
}

func runDevicesList(_ []string) {
	cfg, err := config.Load()
	if err != nil {
		exitError(fmt.Sprintf("loading config: %v", err))
	}

	client := newDaemonClient(cfg)
	data, err := client.get("/devices")
	if err != nil {
		// Fall back to config-only listing if daemon is unavailable
		type deviceInfo struct {
			config.Device
			Online bool `json:"online"`
		}
		var devices []deviceInfo
		for _, d := range cfg.Devices {
			devices = append(devices, deviceInfo{Device: d, Online: false})
		}
		if devices == nil {
			devices = []deviceInfo{}
		}
		outputJSON(map[string]interface{}{"devices": devices, "source": "config"})
		return
	}

	outputRaw(data)
}

func runDevicesAdd(args []string) {
	fs := flag.NewFlagSet("yokai devices add", flag.ExitOnError)
	host := fs.String("host", "", "Hostname or IP address (required)")
	label := fs.String("label", "", "Human-readable label")
	sshUser := fs.String("ssh-user", "root", "SSH user")
	sshKey := fs.String("ssh-key", "", "Path to SSH private key")
	sshPort := fs.Int("ssh-port", 22, "SSH port")
	connType := fs.String("type", "manual", "Connection type: tailscale, local, manual")
	agentPort := fs.Int("agent-port", 7474, "Agent port on remote device")
	agentToken := fs.String("agent-token", "", "Agent auth token")
	gpuType := fs.String("gpu-type", "", "GPU type: nvidia, amd, apple")
	_ = fs.Parse(args)

	if *host == "" {
		exitError("--host is required")
	}

	if *label == "" {
		*label = *host
	}

	cfg, err := config.Load()
	if err != nil {
		exitError(fmt.Sprintf("loading config: %v", err))
	}

	// Check for duplicate
	if cfg.FindDevice(*host) != nil {
		exitError(fmt.Sprintf("device %q already exists", *host))
	}

	device := config.Device{
		ID:             *host,
		Label:          *label,
		Host:           *host,
		SSHUser:        *sshUser,
		SSHKey:         *sshKey,
		SSHPort:        *sshPort,
		ConnectionType: *connType,
		AgentPort:      *agentPort,
		AgentToken:     *agentToken,
		GPUType:        *gpuType,
	}

	cfg.AddDevice(device)
	if err := config.Save(cfg); err != nil {
		exitError(fmt.Sprintf("saving config: %v", err))
	}

	// Notify daemon to reload
	reloadDaemon(cfg)

	outputJSON(map[string]interface{}{
		"status": "added",
		"device": device,
	})
}

func runDevicesRemove(args []string) {
	if len(args) == 0 {
		exitError("Usage: yokai devices remove <device-id>")
	}
	deviceID := args[0]

	cfg, err := config.Load()
	if err != nil {
		exitError(fmt.Sprintf("loading config: %v", err))
	}

	// Find by ID or label
	found := cfg.FindDevice(deviceID)
	if found == nil {
		// Try finding by label
		for i := range cfg.Devices {
			if cfg.Devices[i].Label == deviceID {
				found = &cfg.Devices[i]
				break
			}
		}
	}

	if found == nil {
		exitError(fmt.Sprintf("device %q not found", deviceID))
	}

	actualID := found.ID
	cfg.RemoveDevice(actualID)
	if err := config.Save(cfg); err != nil {
		exitError(fmt.Sprintf("saving config: %v", err))
	}

	reloadDaemon(cfg)

	outputJSON(map[string]string{
		"status":    "removed",
		"device_id": actualID,
	})
}

func runDevicesTest(args []string) {
	if len(args) == 0 {
		exitError("Usage: yokai devices test <device-id>")
	}
	deviceID := args[0]

	cfg, err := config.Load()
	if err != nil {
		exitError(fmt.Sprintf("loading config: %v", err))
	}

	dev := cfg.FindDevice(deviceID)
	if dev == nil {
		// Try label
		for i := range cfg.Devices {
			if cfg.Devices[i].Label == deviceID {
				dev = &cfg.Devices[i]
				break
			}
		}
	}
	if dev == nil {
		exitError(fmt.Sprintf("device %q not found in config", deviceID))
	}

	result := map[string]interface{}{
		"device_id": dev.ID,
		"host":      dev.Host,
	}

	// Test SSH
	client, err := sshpkg.Connect(sshpkg.ClientConfig{
		Host:    dev.Host,
		Port:    strconv.Itoa(dev.SSHPortOrDefault()),
		User:    dev.SSHUser,
		KeyPath: dev.SSHKey,
	})
	if err != nil {
		result["ssh"] = map[string]interface{}{"ok": false, "error": err.Error()}
		result["agent"] = map[string]interface{}{"ok": false, "error": "skipped (SSH failed)"}
		outputJSON(result)
		os.Exit(1)
	}
	defer func() { _ = client.Close() }()
	result["ssh"] = map[string]interface{}{"ok": true}

	// Test agent via daemon tunnel if available
	daemonClient := newDaemonClient(cfg)
	data, err := daemonClient.get(fmt.Sprintf("/metrics/%s", dev.ID))
	if err != nil {
		// Try direct SSH check for agent
		out, execErr := client.Exec("curl -sf http://localhost:" + strconv.Itoa(dev.AgentPort) + "/health 2>/dev/null || echo AGENT_UNREACHABLE")
		if execErr != nil || out == "AGENT_UNREACHABLE\n" || out == "AGENT_UNREACHABLE" {
			result["agent"] = map[string]interface{}{"ok": false, "error": "agent not responding on port " + strconv.Itoa(dev.AgentPort)}
		} else {
			result["agent"] = map[string]interface{}{"ok": true, "method": "ssh_direct"}
		}
	} else {
		_ = data
		result["agent"] = map[string]interface{}{"ok": true, "method": "daemon_tunnel"}
	}

	outputJSON(result)
}

func runDevicesBootstrap(args []string) {
	if len(args) == 0 {
		exitError("Usage: yokai devices bootstrap <device-id>")
	}
	deviceID := args[0]

	cfg, err := config.Load()
	if err != nil {
		exitError(fmt.Sprintf("loading config: %v", err))
	}

	dev := cfg.FindDevice(deviceID)
	if dev == nil {
		for i := range cfg.Devices {
			if cfg.Devices[i].Label == deviceID {
				dev = &cfg.Devices[i]
				break
			}
		}
	}
	if dev == nil {
		exitError(fmt.Sprintf("device %q not found in config", deviceID))
	}

	// Connect via SSH
	client, err := sshpkg.Connect(sshpkg.ClientConfig{
		Host:    dev.Host,
		Port:    strconv.Itoa(dev.SSHPortOrDefault()),
		User:    dev.SSHUser,
		KeyPath: dev.SSHKey,
	})
	if err != nil {
		exitError(fmt.Sprintf("SSH connect failed: %v", err))
	}
	defer func() { _ = client.Close() }()

	// Preflight
	pf, err := sshpkg.Preflight(client)
	if err != nil {
		exitError(fmt.Sprintf("pre-flight failed: %v", err))
	}
	if !pf.DockerInstalled {
		exitError(fmt.Sprintf("docker not installed on %s", dev.Host))
	}

	// Generate agent token
	tokenBytes := make([]byte, 32)
	if _, err := rand.Read(tokenBytes); err != nil {
		exitError(fmt.Sprintf("generating token: %v", err))
	}
	agentToken := hex.EncodeToString(tokenBytes)

	// Get binary path
	binaryPath, err := os.Executable()
	if err != nil {
		exitError(fmt.Sprintf("getting binary path: %v", err))
	}

	// Deploy agent
	if err := sshpkg.DeployAgent(client, binaryPath, agentToken); err != nil {
		exitError(fmt.Sprintf("deploying agent: %v", err))
	}

	// Update device config with token and GPU info
	dev.AgentToken = agentToken
	if pf.GPUDetected {
		dev.GPUType = "nvidia"
	}
	if err := config.Save(cfg); err != nil {
		exitError(fmt.Sprintf("saving config: %v", err))
	}

	reloadDaemon(cfg)

	outputJSON(map[string]interface{}{
		"status":    "bootstrapped",
		"device_id": dev.ID,
		"preflight": map[string]interface{}{
			"os":              pf.OS,
			"arch":            pf.Arch,
			"docker_version":  pf.DockerVersion,
			"gpu_detected":    pf.GPUDetected,
			"gpu_name":        pf.GPUName,
			"gpu_vram_mb":     pf.GPUVRAMMb,
			"nvidia_toolkit":  pf.NvidiaToolkitInstalled,
			"nvidia_runtime":  pf.NvidiaRuntimeAvailable,
			"disk_free_gb":    pf.DiskFreeGB,
		},
	})
}

// reloadDaemon tells the daemon to reload config. Best-effort.
func reloadDaemon(cfg *config.Config) {
	addr := cfg.Daemon.Listen
	if addr == "" {
		addr = "127.0.0.1:7473"
	}
	resp, err := http.Post("http://"+addr+"/reload", "application/json", nil)
	if err != nil {
		return // daemon may not be running
	}
	_ = resp.Body.Close()
}
