package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/spencerbull/yokai/internal/config"
)

// RunConfig dispatches config subcommands.
func RunConfig(args []string) {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "Usage: yokai config <show|set|path>")
		os.Exit(1)
	}

	switch args[0] {
	case "show":
		runConfigShow()
	case "set":
		runConfigSet(args[1:])
	case "path":
		runConfigPath()
	default:
		fmt.Fprintf(os.Stderr, "Unknown config command: %s\n", args[0])
		os.Exit(1)
	}
}

func runConfigShow() {
	cfg, err := config.Load()
	if err != nil {
		exitError(fmt.Sprintf("loading config: %v", err))
	}

	// Redact sensitive values
	redacted := *cfg
	if redacted.HFToken != "" {
		redacted.HFToken = redacted.HFToken[:4] + "..." + redacted.HFToken[len(redacted.HFToken)-4:]
	}
	for i := range redacted.Devices {
		if redacted.Devices[i].AgentToken != "" {
			t := redacted.Devices[i].AgentToken
			redacted.Devices[i].AgentToken = t[:4] + "..." + t[len(t)-4:]
		}
	}

	outputJSON(redacted)
}

func runConfigSet(args []string) {
	if len(args) < 2 {
		exitError("Usage: yokai config set <key> <value>")
	}
	key := args[0]
	value := args[1]

	cfg, err := config.Load()
	if err != nil {
		exitError(fmt.Sprintf("loading config: %v", err))
	}

	switch strings.ToLower(key) {
	case "hf_token":
		cfg.HFToken = value
	case "daemon.listen":
		cfg.Daemon.Listen = value
	case "daemon.metrics_poll_interval":
		n, err := strconv.Atoi(value)
		if err != nil {
			exitError(fmt.Sprintf("invalid integer: %s", value))
		}
		cfg.Daemon.MetricsPollInterval = n
	case "daemon.reconnect_interval":
		n, err := strconv.Atoi(value)
		if err != nil {
			exitError(fmt.Sprintf("invalid integer: %s", value))
		}
		cfg.Daemon.ReconnectInterval = n
	case "preferences.theme":
		cfg.Preferences.Theme = value
	case "preferences.default_vllm_image":
		cfg.Preferences.DefaultVLLMImage = value
	case "preferences.default_llama_image":
		cfg.Preferences.DefaultLlamaImage = value
	case "preferences.default_comfyui_image":
		cfg.Preferences.DefaultComfyImage = value
	default:
		exitError(fmt.Sprintf("unknown config key: %s\nValid keys: hf_token, daemon.listen, daemon.metrics_poll_interval, daemon.reconnect_interval, preferences.theme, preferences.default_vllm_image, preferences.default_llama_image, preferences.default_comfyui_image", key))
	}

	if err := config.Save(cfg); err != nil {
		exitError(fmt.Sprintf("saving config: %v", err))
	}

	// Show the raw value that was set
	var raw json.RawMessage
	raw, _ = json.Marshal(value)

	outputJSON(map[string]interface{}{
		"status": "updated",
		"key":    key,
		"value":  json.RawMessage(raw),
	})
}

func runConfigPath() {
	path, err := config.ConfigPath()
	if err != nil {
		exitError(fmt.Sprintf("getting config path: %v", err))
	}
	outputJSON(map[string]string{"path": path})
}
