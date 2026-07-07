package cli

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"github.com/spencerbull/yokai/internal/config"
)

// RunServices dispatches services subcommands.
func RunServices(args []string) {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "Usage: yokai services <list|deploy|stop|remove|restart|logs>")
		os.Exit(1)
	}

	switch args[0] {
	case "list":
		runServicesList(args[1:])
	case "deploy":
		runServicesDeploy(args[1:])
	case "stop":
		runServicesStop(args[1:])
	case "remove":
		runServicesRemove(args[1:])
	case "restart":
		runServicesRestart(args[1:])
	case "logs":
		runServicesLogs(args[1:])
	default:
		fmt.Fprintf(os.Stderr, "Unknown services command: %s\n", args[0])
		os.Exit(1)
	}
}

func runServicesList(args []string) {
	fs := flag.NewFlagSet("yokai services list", flag.ExitOnError)
	deviceFilter := fs.String("device", "", "Filter by device ID")
	_ = fs.Parse(args)

	cfg, err := config.Load()
	if err != nil {
		exitError(fmt.Sprintf("loading config: %v", err))
	}

	client := newDaemonClient(cfg)

	// Get all metrics which include container info
	data, err := client.get("/metrics")
	if err != nil {
		exitError(fmt.Sprintf("fetching metrics: %v", err))
	}

	// Parse and filter
	var allMetrics map[string]json.RawMessage
	if err := json.Unmarshal(data, &allMetrics); err != nil {
		exitError(fmt.Sprintf("parsing metrics: %v", err))
	}

	type containerInfo struct {
		DeviceID   string          `json:"device_id"`
		Containers json.RawMessage `json:"containers"`
	}

	var result []containerInfo
	for deviceID, metrics := range allMetrics {
		if *deviceFilter != "" && deviceID != *deviceFilter {
			continue
		}
		var m struct {
			Containers json.RawMessage `json:"containers"`
		}
		if err := json.Unmarshal(metrics, &m); err != nil {
			continue
		}
		result = append(result, containerInfo{
			DeviceID:   deviceID,
			Containers: m.Containers,
		})
	}

	if result == nil {
		result = []containerInfo{}
	}

	outputJSON(map[string]interface{}{"services": result})
}

func runServicesDeploy(args []string) {
	fs := flag.NewFlagSet("yokai services deploy", flag.ExitOnError)
	deviceID := fs.String("device", "", "Device ID (required)")
	serviceType := fs.String("type", "", "Service type: vllm, llamacpp, comfyui")
	model := fs.String("model", "", "Model name/path")
	port := fs.String("port", "", "Port mapping (e.g. 8000:8000)")
	image := fs.String("image", "", "Docker image (auto-selected if omitted)")
	name := fs.String("name", "", "Container name")
	extraArgs := fs.String("extra-args", "", "Extra docker arguments")
	gpuIDs := fs.String("gpu-ids", "all", "GPU IDs to use")
	skipPull := fs.Bool("skip-pull", false, "Skip pulling the image (use local/custom image)")
	_ = fs.Parse(args)

	if *deviceID == "" {
		exitError("--device is required")
	}

	cfg, err := config.Load()
	if err != nil {
		exitError(fmt.Sprintf("loading config: %v", err))
	}

	// Auto-select image based on type if not provided
	if *image == "" && *serviceType != "" {
		switch *serviceType {
		case "vllm":
			*image = cfg.Preferences.DefaultVLLMImage
		case "llamacpp":
			*image = cfg.Preferences.DefaultLlamaImage
		case "comfyui":
			*image = cfg.Preferences.DefaultComfyImage
		}
	}

	if *image == "" {
		exitError("--image or --type is required")
	}

	// Build port mapping
	ports := make(map[string]string)
	if *port != "" {
		// Parse "host:container" format
		ports = parsePortMapping(*port, *serviceType)
	} else if *serviceType != "" {
		// Default ports based on service type
		switch *serviceType {
		case "vllm":
			ports["8000"] = "8000"
		case "llamacpp":
			ports["8080"] = "8080"
		case "comfyui":
			ports["8188"] = "8188"
		}
	}

	// Container name
	if *name == "" && *serviceType != "" {
		*name = "yokai-" + *serviceType
	}

	deployReq := map[string]interface{}{
		"device_id":  *deviceID,
		"image":      *image,
		"name":       *name,
		"model":      *model,
		"ports":      ports,
		"gpu_ids":    *gpuIDs,
		"extra_args": *extraArgs,
	}
	if *skipPull {
		deployReq["skip_pull"] = true
	}

	body, _ := json.Marshal(deployReq)
	client := newDaemonClient(cfg)
	data, err := client.post("/deploy", bytes.NewReader(body))
	if err != nil {
		exitError(fmt.Sprintf("deploy failed: %v", err))
	}

	outputRaw(data)
}

func runServicesStop(args []string) {
	if len(args) < 2 {
		exitError("Usage: yokai services stop <device-id> <container-id>")
	}
	deviceID := args[0]
	containerID := args[1]

	cfg, err := config.Load()
	if err != nil {
		exitError(fmt.Sprintf("loading config: %v", err))
	}

	client := newDaemonClient(cfg)
	data, err := client.post(fmt.Sprintf("/containers/%s/%s/stop", deviceID, containerID), nil)
	if err != nil {
		exitError(fmt.Sprintf("stop failed: %v", err))
	}

	outputRaw(data)
}

func runServicesRemove(args []string) {
	if len(args) < 2 {
		exitError("Usage: yokai services remove <device-id> <container-id>")
	}
	deviceID := args[0]
	containerID := args[1]

	cfg, err := config.Load()
	if err != nil {
		exitError(fmt.Sprintf("loading config: %v", err))
	}

	client := newDaemonClient(cfg)
	data, err := client.doDelete(fmt.Sprintf("/containers/%s/%s/remove", deviceID, containerID))
	if err != nil {
		exitError(fmt.Sprintf("remove failed: %v", err))
	}

	outputRaw(data)
}

func runServicesRestart(args []string) {
	if len(args) < 2 {
		exitError("Usage: yokai services restart <device-id> <container-id>")
	}
	deviceID := args[0]
	containerID := args[1]

	cfg, err := config.Load()
	if err != nil {
		exitError(fmt.Sprintf("loading config: %v", err))
	}

	client := newDaemonClient(cfg)
	data, err := client.post(fmt.Sprintf("/containers/%s/%s/restart", deviceID, containerID), nil)
	if err != nil {
		exitError(fmt.Sprintf("restart failed: %v", err))
	}

	outputRaw(data)
}

func runServicesLogs(args []string) {
	fs := flag.NewFlagSet("yokai services logs", flag.ExitOnError)
	follow := fs.Bool("follow", false, "Follow log output")
	_ = fs.Parse(args)

	remaining := fs.Args()
	if len(remaining) < 2 {
		exitError("Usage: yokai services logs [--follow] <device-id> <container-id>")
	}
	deviceID := remaining[0]
	containerID := remaining[1]

	cfg, err := config.Load()
	if err != nil {
		exitError(fmt.Sprintf("loading config: %v", err))
	}

	client := newDaemonClient(cfg)

	if *follow {
		// Stream logs via SSE
		ch, err := client.getSSE(fmt.Sprintf("/logs/%s/%s", deviceID, containerID))
		if err != nil {
			exitError(fmt.Sprintf("streaming logs: %v", err))
		}
		for line := range ch {
			fmt.Println(line)
		}
	} else {
		// Get snapshot of logs via SSE, read what's available
		ch, err := client.getSSE(fmt.Sprintf("/logs/%s/%s", deviceID, containerID))
		if err != nil {
			exitError(fmt.Sprintf("fetching logs: %v", err))
		}
		for line := range ch {
			fmt.Println(line)
		}
	}
}

// parsePortMapping parses "host:container" or just "port" into a map.
func parsePortMapping(spec string, serviceType string) map[string]string {
	ports := make(map[string]string)
	// Simple "host:container" parsing
	for i := 0; i < len(spec); i++ {
		if spec[i] == ':' {
			ports[spec[i+1:]] = spec[:i]
			return ports
		}
	}
	// Single port: use as both
	ports[spec] = spec
	return ports
}
