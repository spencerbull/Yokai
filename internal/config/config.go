package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

const (
	ConfigVersion = 1
	ConfigDirName = "yokai"
	ConfigFile    = "config.json"
)

type Config struct {
	Version     int          `json:"version"`
	HFToken     string       `json:"hf_token,omitempty"`
	Daemon      DaemonConfig `json:"daemon"`
	Devices     []Device     `json:"devices"`
	Services    []Service    `json:"services"`
	Preferences Preferences  `json:"preferences"`
}

type DaemonConfig struct {
	Listen              string `json:"listen"`
	MetricsPollInterval int    `json:"metrics_poll_interval_s"`
	ReconnectInterval   int    `json:"reconnect_interval_s"`
}

type Device struct {
	ID             string   `json:"id"`
	Label          string   `json:"label,omitempty"`
	Host           string   `json:"host"`
	SSHUser        string   `json:"ssh_user,omitempty"`
	SSHKey         string   `json:"ssh_key,omitempty"`
	SSHPort        int      `json:"ssh_port,omitempty"` // default 22
	ConnectionType string   `json:"connection_type"`    // "tailscale", "local", "manual"
	AgentPort      int      `json:"agent_port"`
	AgentToken     string   `json:"agent_token,omitempty"`
	GPUType        string   `json:"gpu_type,omitempty"` // "nvidia", "amd", "apple", ""
	Tags           []string `json:"tags,omitempty"`
}

// SSHPortOrDefault returns the device's SSH port, defaulting to 22.
func (d Device) SSHPortOrDefault() int {
	if d.SSHPort <= 0 {
		return 22
	}
	return d.SSHPort
}

type Service struct {
	ID          string            `json:"id"`
	DeviceID    string            `json:"device_id"`
	Type        string            `json:"type"` // "vllm", "llamacpp", "comfyui"
	Image       string            `json:"image"`
	Model       string            `json:"model,omitempty"`
	Port        int               `json:"port"`
	ExtraArgs   string            `json:"extra_args,omitempty"`
	Env         map[string]string `json:"env,omitempty"`
	Volumes     map[string]string `json:"volumes,omitempty"`
	Plugins     []string          `json:"plugins,omitempty"`
	Runtime     RuntimeOptions    `json:"runtime,omitempty"`
	ContainerID string            `json:"container_id,omitempty"`
}

type RuntimeOptions struct {
	IPCMode string            `json:"ipc_mode,omitempty"`
	ShmSize string            `json:"shm_size,omitempty"`
	Ulimits map[string]string `json:"ulimits,omitempty"`
}

type Preferences struct {
	Theme             string `json:"theme"`
	DefaultVLLMImage  string `json:"default_vllm_image"`
	DefaultLlamaImage string `json:"default_llama_image"`
	DefaultComfyImage string `json:"default_comfyui_image"`
}

// DefaultConfig returns a config with sensible defaults.
func DefaultConfig() *Config {
	return &Config{
		Version: ConfigVersion,
		Daemon: DaemonConfig{
			Listen:              "127.0.0.1:7473",
			MetricsPollInterval: 2,
			ReconnectInterval:   30,
		},
		Devices:  []Device{},
		Services: []Service{},
		Preferences: Preferences{
			Theme:             "tokyonight",
			DefaultVLLMImage:  "vllm/vllm-openai:latest",
			DefaultLlamaImage: "ghcr.io/ggml-org/llama.cpp:server-cuda",
			DefaultComfyImage: "spencerbull/yokai-comfyui:latest",
		},
	}
}

// ConfigDir returns the path to ~/.config/yokai/
func ConfigDir() (string, error) {
	configHome := os.Getenv("XDG_CONFIG_HOME")
	if configHome == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("cannot determine home directory: %w", err)
		}
		configHome = filepath.Join(home, ".config")
	}
	return filepath.Join(configHome, ConfigDirName), nil
}

// ConfigPath returns the full path to config.json
func ConfigPath() (string, error) {
	dir, err := ConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, ConfigFile), nil
}

// Load reads the config from disk. Returns default config if file doesn't exist.
func Load() (*Config, error) {
	path, err := ConfigPath()
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return DefaultConfig(), nil
		}
		return nil, fmt.Errorf("reading config: %w", err)
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing config: %w", err)
	}

	// Migrate if needed
	if cfg.Version < ConfigVersion {
		cfg.Version = ConfigVersion
	}

	// Fill defaults for missing fields
	if cfg.Daemon.Listen == "" {
		cfg.Daemon.Listen = "127.0.0.1:7473"
	}
	if cfg.Daemon.MetricsPollInterval == 0 {
		cfg.Daemon.MetricsPollInterval = 2
	}
	if cfg.Daemon.ReconnectInterval == 0 {
		cfg.Daemon.ReconnectInterval = 30
	}
	if cfg.Preferences.DefaultVLLMImage == "" {
		cfg.Preferences.DefaultVLLMImage = "vllm/vllm-openai:latest"
	}
	if cfg.Preferences.DefaultLlamaImage == "" {
		cfg.Preferences.DefaultLlamaImage = "ghcr.io/ggml-org/llama.cpp:server-cuda"
	}
	if cfg.Preferences.DefaultComfyImage == "" {
		cfg.Preferences.DefaultComfyImage = "spencerbull/yokai-comfyui:latest"
	}
	if cfg.Devices == nil {
		cfg.Devices = []Device{}
	}
	if cfg.Services == nil {
		cfg.Services = []Service{}
	}

	return &cfg, nil
}

// Save writes the config to disk atomically.
func Save(cfg *Config) error {
	dir, err := ConfigDir()
	if err != nil {
		return err
	}

	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("creating config dir: %w", err)
	}

	path, err := ConfigPath()
	if err != nil {
		return err
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling config: %w", err)
	}
	data = append(data, '\n')

	// Atomic write: write to tmp, then rename
	tmpPath := path + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0600); err != nil {
		return fmt.Errorf("writing config: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		if removeErr := os.Remove(tmpPath); removeErr != nil && !os.IsNotExist(removeErr) {
			return fmt.Errorf("renaming config: %w (cleanup tmp failed: %v)", err, removeErr)
		}
		return fmt.Errorf("renaming config: %w", err)
	}

	return nil
}

// HasDevices returns true if at least one device is configured.
func (c *Config) HasDevices() bool {
	return len(c.Devices) > 0
}

// FindDevice returns a device by ID, or nil if not found.
func (c *Config) FindDevice(id string) *Device {
	for i := range c.Devices {
		if c.Devices[i].ID == id {
			return &c.Devices[i]
		}
	}
	return nil
}

// AddDevice appends a device to the config.
func (c *Config) AddDevice(d Device) {
	c.Devices = append(c.Devices, d)
}

// UpsertDevice inserts or replaces a device by ID.
// If duplicate IDs exist, only one entry is kept.
func (c *Config) UpsertDevice(d Device) {
	filtered := c.Devices[:0]
	replaced := false

	for _, existing := range c.Devices {
		if existing.ID == d.ID {
			if !replaced {
				filtered = append(filtered, d)
				replaced = true
			}
			continue
		}
		filtered = append(filtered, existing)
	}

	if !replaced {
		filtered = append(filtered, d)
	}

	c.Devices = filtered
}

// RemoveDevice removes a device by ID.
func (c *Config) RemoveDevice(id string) {
	filtered := c.Devices[:0]
	for _, d := range c.Devices {
		if d.ID != id {
			filtered = append(filtered, d)
		}
	}
	c.Devices = filtered
}

// RemoveServicesByDevice removes all services for a device and returns count.
func (c *Config) RemoveServicesByDevice(deviceID string) int {
	filtered := c.Services[:0]
	removed := 0
	for _, s := range c.Services {
		if s.DeviceID == deviceID {
			removed++
			continue
		}
		filtered = append(filtered, s)
	}
	c.Services = filtered
	return removed
}

// RemoveServiceByContainerID removes services with the given container ID.
// Returns the number of removed services.
func (c *Config) RemoveServiceByContainerID(containerID string) int {
	filtered := c.Services[:0]
	removed := 0
	for _, s := range c.Services {
		if s.ContainerID == containerID {
			removed++
			continue
		}
		filtered = append(filtered, s)
	}
	c.Services = filtered
	return removed
}

// RemoveServiceByID removes services with the given ID and returns count.
func (c *Config) RemoveServiceByID(serviceID string) int {
	filtered := c.Services[:0]
	removed := 0
	for _, s := range c.Services {
		if s.ID == serviceID {
			removed++
			continue
		}
		filtered = append(filtered, s)
	}
	c.Services = filtered
	return removed
}
