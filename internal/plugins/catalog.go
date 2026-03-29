package plugins

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/spencerbull/yokai/internal/config"
)

type Asset struct {
	FileName string
	URL      string
}

type Mount struct {
	AssetFile     string
	ContainerPath string
}

type Plugin struct {
	ID        string
	Name      string
	Workload  string
	Assets    []Asset
	Mounts    []Mount
	ExtraArgs string
	Env       map[string]string
	Runtime   config.RuntimeOptions
	Notes     []string
}

var catalog = []Plugin{
	{
		ID:       "vllm-reasoning-parser-super-v3",
		Name:     "Nemotron Super V3 Reasoning Parser",
		Workload: "vllm",
		Assets: []Asset{{
			FileName: "super_v3_reasoning_parser.py",
			URL:      "https://huggingface.co/nvidia/NVIDIA-Nemotron-3-Super-120B-A12B-NVFP4/raw/main/super_v3_reasoning_parser.py",
		}},
		Mounts: []Mount{{
			AssetFile:     "super_v3_reasoning_parser.py",
			ContainerPath: "/plugins/super_v3_reasoning_parser.py",
		}},
		ExtraArgs: "--reasoning-parser-plugin /plugins/super_v3_reasoning_parser.py --reasoning-parser super_v3 --enable-auto-tool-choice --tool-call-parser qwen3_coder",
		Notes: []string{
			"Adds the custom Super V3 reasoning parser required by Nemotron Super.",
			"Does not force fastsafetensors on the stock vllm/vllm-openai image because that image does not ship the extra module.",
		},
	},
}

func Lookup(id string) (Plugin, bool) {
	for _, plugin := range catalog {
		if strings.EqualFold(plugin.ID, strings.TrimSpace(id)) {
			return plugin, true
		}
	}
	return Plugin{}, false
}

func MustLookup(id string) Plugin {
	plugin, _ := Lookup(id)
	return plugin
}

func AssetHostPath(pluginID, fileName string) string {
	return filepath.Join(rootDir(), pluginID, fileName)
}

func rootDir() string {
	if dir := strings.TrimSpace(os.Getenv("YOKAI_PLUGIN_DIR")); dir != "" {
		return dir
	}
	if os.Geteuid() == 0 {
		return "/var/lib/yokai/plugins"
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return filepath.Join(os.TempDir(), "yokai-plugins")
	}
	return filepath.Join(home, ".local", "share", "yokai", "plugins")
}
