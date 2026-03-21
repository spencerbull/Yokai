package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultConfig(t *testing.T) {
	t.Parallel()

	cfg := DefaultConfig()

	// Verify non-zero values are set
	if cfg.Version != ConfigVersion {
		t.Errorf("expected version %d, got %d", ConfigVersion, cfg.Version)
	}

	// Test daemon defaults
	if cfg.Daemon.Listen != "127.0.0.1:7473" {
		t.Errorf("expected listen address '127.0.0.1:7473', got '%s'", cfg.Daemon.Listen)
	}
	if cfg.Daemon.MetricsPollInterval != 2 {
		t.Errorf("expected metrics poll interval 2, got %d", cfg.Daemon.MetricsPollInterval)
	}
	if cfg.Daemon.ReconnectInterval != 30 {
		t.Errorf("expected reconnect interval 30, got %d", cfg.Daemon.ReconnectInterval)
	}

	// Test preferences defaults
	if cfg.Preferences.Theme != "tokyonight" {
		t.Errorf("expected theme 'tokyonight', got '%s'", cfg.Preferences.Theme)
	}
	if cfg.Preferences.DefaultVLLMImage != "vllm/vllm-openai:latest" {
		t.Errorf("expected vllm image 'vllm/vllm-openai:latest', got '%s'", cfg.Preferences.DefaultVLLMImage)
	}
	if cfg.Preferences.DefaultLlamaImage != "ghcr.io/ggml-org/llama.cpp:server-cuda" {
		t.Errorf("expected llama image 'ghcr.io/ggml-org/llama.cpp:server-cuda', got '%s'", cfg.Preferences.DefaultLlamaImage)
	}
	if cfg.Preferences.DefaultComfyImage != "spencerbull/yokai-comfyui:latest" {
		t.Errorf("expected comfy image 'spencerbull/yokai-comfyui:latest', got '%s'", cfg.Preferences.DefaultComfyImage)
	}

	// Test slices are initialized
	if cfg.Devices == nil {
		t.Error("expected devices slice to be initialized")
	}
	if len(cfg.Devices) != 0 {
		t.Errorf("expected empty devices slice, got %d items", len(cfg.Devices))
	}

	if cfg.Services == nil {
		t.Error("expected services slice to be initialized")
	}
	if len(cfg.Services) != 0 {
		t.Errorf("expected empty services slice, got %d items", len(cfg.Services))
	}
}

func TestLoadNonExistent(t *testing.T) {
	// Cannot use t.Parallel() with t.Setenv()

	// Override config dir for test
	tempDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tempDir)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("expected no error when file doesn't exist, got: %v", err)
	}

	// Should return default config
	defaultCfg := DefaultConfig()
	if cfg.Version != defaultCfg.Version {
		t.Errorf("expected version %d, got %d", defaultCfg.Version, cfg.Version)
	}
	if cfg.Daemon.Listen != defaultCfg.Daemon.Listen {
		t.Errorf("expected listen %s, got %s", defaultCfg.Daemon.Listen, cfg.Daemon.Listen)
	}
}

func TestSaveAndLoad(t *testing.T) {
	// Cannot use t.Parallel() with t.Setenv()

	tempDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tempDir)

	// Create a test config
	original := &Config{
		Version: 1,
		HFToken: "test-token",
		Daemon: DaemonConfig{
			Listen:              "0.0.0.0:8080",
			MetricsPollInterval: 5,
			ReconnectInterval:   60,
		},
		Devices: []Device{
			{
				ID:             "test-device",
				Label:          "Test Device",
				Host:           "192.168.1.100",
				SSHUser:        "testuser",
				ConnectionType: "local",
				AgentPort:      8080,
				GPUType:        "nvidia",
				Tags:           []string{"test", "gpu"},
			},
		},
		Services: []Service{
			{
				ID:       "test-service",
				DeviceID: "test-device",
				Type:     "vllm",
				Image:    "test/image:latest",
				Model:    "test-model",
				Port:     8000,
			},
		},
		Preferences: Preferences{
			Theme:             "dark",
			DefaultVLLMImage:  "custom/vllm:latest",
			DefaultLlamaImage: "custom/llama:latest",
			DefaultComfyImage: "custom/comfy:latest",
		},
	}

	// Save the config
	if err := Save(original); err != nil {
		t.Fatalf("failed to save config: %v", err)
	}

	// Load it back
	loaded, err := Load()
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}

	// Compare important fields
	if loaded.Version != original.Version {
		t.Errorf("version mismatch: expected %d, got %d", original.Version, loaded.Version)
	}
	if loaded.HFToken != original.HFToken {
		t.Errorf("HFToken mismatch: expected %s, got %s", original.HFToken, loaded.HFToken)
	}
	if loaded.Daemon.Listen != original.Daemon.Listen {
		t.Errorf("daemon listen mismatch: expected %s, got %s", original.Daemon.Listen, loaded.Daemon.Listen)
	}

	if len(loaded.Devices) != 1 {
		t.Fatalf("expected 1 device, got %d", len(loaded.Devices))
	}
	device := loaded.Devices[0]
	if device.ID != "test-device" {
		t.Errorf("device ID mismatch: expected test-device, got %s", device.ID)
	}
	if device.Label != "Test Device" {
		t.Errorf("device label mismatch: expected Test Device, got %s", device.Label)
	}

	if len(loaded.Services) != 1 {
		t.Fatalf("expected 1 service, got %d", len(loaded.Services))
	}
	service := loaded.Services[0]
	if service.ID != "test-service" {
		t.Errorf("service ID mismatch: expected test-service, got %s", service.ID)
	}
}

func TestFindDevice(t *testing.T) {
	t.Parallel()

	cfg := &Config{
		Devices: []Device{
			{ID: "device1", Label: "Device 1"},
			{ID: "device2", Label: "Device 2"},
		},
	}

	tests := []struct {
		name     string
		id       string
		expected *Device
	}{
		{
			name:     "find existing device",
			id:       "device1",
			expected: &Device{ID: "device1", Label: "Device 1"},
		},
		{
			name:     "find another existing device",
			id:       "device2",
			expected: &Device{ID: "device2", Label: "Device 2"},
		},
		{
			name:     "find non-existent device",
			id:       "device3",
			expected: nil,
		},
		{
			name:     "find with empty string",
			id:       "",
			expected: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := cfg.FindDevice(tt.id)

			if tt.expected == nil {
				if result != nil {
					t.Errorf("expected nil, got device with ID %s", result.ID)
				}
			} else {
				if result == nil {
					t.Errorf("expected device with ID %s, got nil", tt.expected.ID)
				} else if result.ID != tt.expected.ID {
					t.Errorf("expected device ID %s, got %s", tt.expected.ID, result.ID)
				}
			}
		})
	}
}

func TestAddDevice(t *testing.T) {
	t.Parallel()

	cfg := &Config{
		Devices: []Device{},
	}

	device1 := Device{ID: "device1", Label: "Device 1"}
	device2 := Device{ID: "device2", Label: "Device 2"}

	// Add first device
	cfg.AddDevice(device1)
	if len(cfg.Devices) != 1 {
		t.Errorf("expected 1 device after adding, got %d", len(cfg.Devices))
	}
	if cfg.Devices[0].ID != "device1" {
		t.Errorf("expected first device ID 'device1', got '%s'", cfg.Devices[0].ID)
	}

	// Add second device
	cfg.AddDevice(device2)
	if len(cfg.Devices) != 2 {
		t.Errorf("expected 2 devices after adding, got %d", len(cfg.Devices))
	}
	if cfg.Devices[1].ID != "device2" {
		t.Errorf("expected second device ID 'device2', got '%s'", cfg.Devices[1].ID)
	}

	// Verify can find added devices
	found := cfg.FindDevice("device1")
	if found == nil {
		t.Error("could not find device1 after adding")
	}
	found = cfg.FindDevice("device2")
	if found == nil {
		t.Error("could not find device2 after adding")
	}
}

func TestUpsertDeviceAddsWhenMissing(t *testing.T) {
	t.Parallel()

	cfg := &Config{Devices: []Device{}}
	dev := Device{ID: "device1", Label: "Device 1", AgentToken: "token-a"}

	cfg.UpsertDevice(dev)

	if len(cfg.Devices) != 1 {
		t.Fatalf("expected 1 device, got %d", len(cfg.Devices))
	}
	if cfg.Devices[0].ID != "device1" {
		t.Fatalf("expected ID device1, got %s", cfg.Devices[0].ID)
	}
	if cfg.Devices[0].AgentToken != "token-a" {
		t.Fatalf("expected token token-a, got %s", cfg.Devices[0].AgentToken)
	}
}

func TestUpsertDeviceReplacesAndDedupesByID(t *testing.T) {
	t.Parallel()

	cfg := &Config{
		Devices: []Device{
			{ID: "device1", Label: "Old", AgentToken: "old-token"},
			{ID: "device2", Label: "Two", AgentToken: "token-2"},
			{ID: "device1", Label: "Old Duplicate", AgentToken: "stale-token"},
		},
	}

	cfg.UpsertDevice(Device{ID: "device1", Label: "New", AgentToken: "new-token"})

	if len(cfg.Devices) != 2 {
		t.Fatalf("expected duplicates to be removed, got %d devices", len(cfg.Devices))
	}

	dev1 := cfg.FindDevice("device1")
	if dev1 == nil {
		t.Fatal("expected device1 to exist")
	}
	if dev1.Label != "New" {
		t.Fatalf("expected updated label New, got %s", dev1.Label)
	}
	if dev1.AgentToken != "new-token" {
		t.Fatalf("expected updated token new-token, got %s", dev1.AgentToken)
	}

	dev2 := cfg.FindDevice("device2")
	if dev2 == nil {
		t.Fatal("expected device2 to remain")
	}
}

func TestRemoveDevice(t *testing.T) {
	t.Parallel()

	cfg := &Config{
		Devices: []Device{
			{ID: "device1", Label: "Device 1"},
			{ID: "device2", Label: "Device 2"},
			{ID: "device3", Label: "Device 3"},
		},
	}

	// Remove middle device
	cfg.RemoveDevice("device2")
	if len(cfg.Devices) != 2 {
		t.Errorf("expected 2 devices after removing, got %d", len(cfg.Devices))
	}

	// Verify device2 is gone
	if cfg.FindDevice("device2") != nil {
		t.Error("device2 should be removed but was found")
	}

	// Verify other devices remain
	if cfg.FindDevice("device1") == nil {
		t.Error("device1 should remain but was not found")
	}
	if cfg.FindDevice("device3") == nil {
		t.Error("device3 should remain but was not found")
	}

	// Remove non-existent device (should not panic)
	cfg.RemoveDevice("non-existent")
	if len(cfg.Devices) != 2 {
		t.Errorf("removing non-existent device changed device count: got %d", len(cfg.Devices))
	}

	// Remove first device
	cfg.RemoveDevice("device1")
	if len(cfg.Devices) != 1 {
		t.Errorf("expected 1 device after removing device1, got %d", len(cfg.Devices))
	}
	if cfg.Devices[0].ID != "device3" {
		t.Errorf("expected remaining device to be device3, got %s", cfg.Devices[0].ID)
	}
}

func TestRemoveServicesByDevice(t *testing.T) {
	t.Parallel()

	cfg := &Config{
		Services: []Service{
			{ID: "svc-a", DeviceID: "dev-1"},
			{ID: "svc-b", DeviceID: "dev-2"},
			{ID: "svc-c", DeviceID: "dev-1"},
		},
	}

	removed := cfg.RemoveServicesByDevice("dev-1")
	if removed != 2 {
		t.Fatalf("expected 2 services removed, got %d", removed)
	}
	if len(cfg.Services) != 1 {
		t.Fatalf("expected 1 service remaining, got %d", len(cfg.Services))
	}
	if cfg.Services[0].ID != "svc-b" {
		t.Fatalf("expected svc-b to remain, got %s", cfg.Services[0].ID)
	}
}

func TestRemoveServiceByContainerID(t *testing.T) {
	t.Parallel()

	cfg := &Config{
		Services: []Service{
			{ID: "svc-a", ContainerID: "cont-1"},
			{ID: "svc-b", ContainerID: "cont-2"},
			{ID: "svc-c", ContainerID: "cont-1"},
		},
	}

	removed := cfg.RemoveServiceByContainerID("cont-1")
	if removed != 2 {
		t.Fatalf("expected 2 services removed, got %d", removed)
	}
	if len(cfg.Services) != 1 {
		t.Fatalf("expected 1 service remaining, got %d", len(cfg.Services))
	}
	if cfg.Services[0].ID != "svc-b" {
		t.Fatalf("expected svc-b to remain, got %s", cfg.Services[0].ID)
	}
}

func TestRemoveServiceByID(t *testing.T) {
	t.Parallel()

	cfg := &Config{
		Services: []Service{
			{ID: "svc-a"},
			{ID: "svc-b"},
			{ID: "svc-a"},
		},
	}

	removed := cfg.RemoveServiceByID("svc-a")
	if removed != 2 {
		t.Fatalf("expected 2 services removed, got %d", removed)
	}
	if len(cfg.Services) != 1 {
		t.Fatalf("expected 1 service remaining, got %d", len(cfg.Services))
	}
	if cfg.Services[0].ID != "svc-b" {
		t.Fatalf("expected svc-b to remain, got %s", cfg.Services[0].ID)
	}
}

func TestConfigMigration(t *testing.T) {
	// Cannot use t.Parallel() with t.Setenv()

	tempDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tempDir)

	// Create config dir
	configDir := filepath.Join(tempDir, ConfigDirName)
	if err := os.MkdirAll(configDir, 0700); err != nil {
		t.Fatalf("failed to create config dir: %v", err)
	}

	// Write old version config
	oldConfig := map[string]interface{}{
		"version": 0, // Old version
		"daemon": map[string]interface{}{
			"listen": "0.0.0.0:9000",
		},
	}

	configPath := filepath.Join(configDir, ConfigFile)
	data, err := json.MarshalIndent(oldConfig, "", "  ")
	if err != nil {
		t.Fatalf("failed to marshal old config: %v", err)
	}

	if err := os.WriteFile(configPath, data, 0600); err != nil {
		t.Fatalf("failed to write old config: %v", err)
	}

	// Load config (should migrate)
	cfg, err := Load()
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}

	// Verify migration
	if cfg.Version != ConfigVersion {
		t.Errorf("expected version to be migrated to %d, got %d", ConfigVersion, cfg.Version)
	}

	// Verify loaded values are preserved
	if cfg.Daemon.Listen != "0.0.0.0:9000" {
		t.Errorf("expected preserved listen address, got %s", cfg.Daemon.Listen)
	}
}

func TestFillDefaults(t *testing.T) {
	// Cannot use t.Parallel() with t.Setenv()

	tempDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tempDir)

	// Create config dir
	configDir := filepath.Join(tempDir, ConfigDirName)
	if err := os.MkdirAll(configDir, 0700); err != nil {
		t.Fatalf("failed to create config dir: %v", err)
	}

	// Write minimal config (missing many fields)
	minimalConfig := map[string]interface{}{
		"version": ConfigVersion,
		"daemon":  map[string]interface{}{}, // Empty daemon config
		"preferences": map[string]interface{}{
			"theme": "custom", // Only set theme
		},
	}

	configPath := filepath.Join(configDir, ConfigFile)
	data, err := json.MarshalIndent(minimalConfig, "", "  ")
	if err != nil {
		t.Fatalf("failed to marshal minimal config: %v", err)
	}

	if err := os.WriteFile(configPath, data, 0600); err != nil {
		t.Fatalf("failed to write minimal config: %v", err)
	}

	// Load config (should fill defaults)
	cfg, err := Load()
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}

	// Verify defaults were filled
	if cfg.Daemon.Listen != "127.0.0.1:7473" {
		t.Errorf("expected default listen address, got %s", cfg.Daemon.Listen)
	}
	if cfg.Daemon.MetricsPollInterval != 2 {
		t.Errorf("expected default metrics poll interval, got %d", cfg.Daemon.MetricsPollInterval)
	}
	if cfg.Daemon.ReconnectInterval != 30 {
		t.Errorf("expected default reconnect interval, got %d", cfg.Daemon.ReconnectInterval)
	}

	// Verify preserved values
	if cfg.Preferences.Theme != "custom" {
		t.Errorf("expected preserved theme 'custom', got %s", cfg.Preferences.Theme)
	}

	// Verify default preferences were filled
	if cfg.Preferences.DefaultVLLMImage == "" {
		t.Error("expected default VLLM image to be filled")
	}
	if cfg.Preferences.DefaultLlamaImage == "" {
		t.Error("expected default Llama image to be filled")
	}
	if cfg.Preferences.DefaultComfyImage == "" {
		t.Error("expected default ComfyUI image to be filled")
	}

	// Verify slices are initialized
	if cfg.Devices == nil {
		t.Error("expected devices slice to be initialized")
	}
	if cfg.Services == nil {
		t.Error("expected services slice to be initialized")
	}
}

func TestHasDevices(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		devices  []Device
		expected bool
	}{
		{
			name:     "no devices",
			devices:  []Device{},
			expected: false,
		},
		{
			name:     "nil devices slice",
			devices:  nil,
			expected: false,
		},
		{
			name: "one device",
			devices: []Device{
				{ID: "device1"},
			},
			expected: true,
		},
		{
			name: "multiple devices",
			devices: []Device{
				{ID: "device1"},
				{ID: "device2"},
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{Devices: tt.devices}
			result := cfg.HasDevices()
			if result != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}
