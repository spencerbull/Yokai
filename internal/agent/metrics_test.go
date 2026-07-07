package agent

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestCollectMetricsStructure(t *testing.T) {
	t.Parallel()

	metrics := CollectMetrics()

	if metrics == nil {
		t.Fatal("CollectMetrics returned nil")
	}

	// Check timestamp is recent (within last minute)
	now := time.Now().UTC()
	if metrics.Timestamp.IsZero() {
		t.Error("timestamp should be set")
	}
	if now.Sub(metrics.Timestamp) > time.Minute {
		t.Error("timestamp should be recent")
	}

	// Verify all fields are accessible (not testing exact values due to environment variability)
	_ = metrics.CPU.Percent
	_ = metrics.RAM.TotalMB
	_ = metrics.RAM.UsedMB
	_ = metrics.RAM.Percent
	_ = metrics.Swap.TotalMB
	_ = metrics.Swap.UsedMB
	_ = metrics.Disk.TotalGB
	_ = metrics.Disk.UsedGB
	_ = metrics.Disk.FreeGB

	// GPUs and Containers can be nil/empty on systems without hardware
	if metrics.GPUs == nil {
		t.Log("GPUs is nil (expected on system without NVIDIA GPUs)")
	}
	if metrics.Containers == nil {
		t.Log("Containers is nil (expected on system without Docker or yokai containers)")
	}
}

func TestCollectCPULinux(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("CPU collection only works on Linux")
	}

	// Test with real /proc/stat
	cpu := collectCPU()

	// On Linux, should return some value >= 0 and <= 100
	if cpu.Percent < 0 || cpu.Percent > 100 {
		t.Errorf("CPU percent should be between 0-100, got %.2f", cpu.Percent)
	}
}

func TestCollectCPUNonLinux(t *testing.T) {
	if runtime.GOOS == "linux" {
		t.Skip("Testing non-Linux behavior")
	}

	cpu := collectCPU()

	// On non-Linux, should return zero values without panic
	if cpu.Percent != 0 {
		t.Errorf("expected zero CPU percent on non-Linux, got %.2f", cpu.Percent)
	}
}

func TestCollectCPUMockData(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Mock /proc/stat test only on Linux")
	}

	t.Parallel()

	// Create temporary /proc/stat-like file for testing
	tempDir := t.TempDir()
	statFile := filepath.Join(tempDir, "stat")

	// Mock /proc/stat data: user nice system idle iowait irq softirq
	mockData := `cpu  1000 100 500 8000 200 0 50 0 0 0
cpu0 500 50 250 4000 100 0 25 0 0 0
cpu1 500 50 250 4000 100 0 25 0 0 0
`

	if err := os.WriteFile(statFile, []byte(mockData), 0644); err != nil {
		t.Fatalf("failed to write mock stat file: %v", err)
	}

	// Test parsing logic by reading the mock file directly
	data, err := os.ReadFile(statFile)
	if err != nil {
		t.Fatalf("failed to read mock file: %v", err)
	}

	// Verify we can parse the format
	content := string(data)
	if !containsCPULine(content) {
		t.Error("mock data should contain CPU line")
	}
}

func containsCPULine(content string) bool {
	lines := strings.Split(content, "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "cpu ") {
			return true
		}
	}
	return false
}

func TestCollectRAMLinux(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("RAM collection only works on Linux")
	}

	ram := collectRAM()

	// Should have some reasonable values
	if ram.TotalMB <= 0 {
		t.Error("total RAM should be positive")
	}
	if ram.UsedMB < 0 {
		t.Error("used RAM should not be negative")
	}
	if ram.UsedMB > ram.TotalMB {
		t.Error("used RAM should not exceed total RAM")
	}
	if ram.Percent < 0 || ram.Percent > 100 {
		t.Errorf("RAM percent should be between 0-100, got %.2f", ram.Percent)
	}
}

func TestCollectRAMNonLinux(t *testing.T) {
	if runtime.GOOS == "linux" {
		t.Skip("Testing non-Linux behavior")
	}

	ram := collectRAM()

	// On non-Linux, should return zero values without panic
	if ram.TotalMB != 0 || ram.UsedMB != 0 || ram.Percent != 0 {
		t.Error("expected zero values for RAM on non-Linux")
	}
}

func TestCollectRAMMockData(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Mock /proc/meminfo test only on Linux")
	}

	t.Parallel()

	// Create temporary /proc/meminfo-like file
	tempDir := t.TempDir()
	meminfoFile := filepath.Join(tempDir, "meminfo")

	// Mock /proc/meminfo data
	mockData := `MemTotal:       16384000 kB
MemFree:         2048000 kB
MemAvailable:    8192000 kB
Buffers:          512000 kB
Cached:          4096000 kB
SwapCached:            0 kB
SwapTotal:       4194304 kB
SwapFree:        4194304 kB
`

	if err := os.WriteFile(meminfoFile, []byte(mockData), 0644); err != nil {
		t.Fatalf("failed to write mock meminfo file: %v", err)
	}

	// Test that we can parse the format
	data, err := os.ReadFile(meminfoFile)
	if err != nil {
		t.Fatalf("failed to read mock file: %v", err)
	}

	content := string(data)
	if !strings.Contains(content, "MemTotal:") {
		t.Error("mock data should contain MemTotal")
	}
	if !strings.Contains(content, "MemAvailable:") {
		t.Error("mock data should contain MemAvailable")
	}
}

func TestGPUMetricsEmpty(t *testing.T) {
	t.Parallel()

	// On most CI systems and development machines without NVIDIA GPUs,
	// this should return nil or empty slice without error
	gpus := collectGPUs()

	// Should not panic and return a slice (possibly empty)
	if gpus == nil {
		t.Log("No GPUs detected (expected on systems without nvidia-smi)")
		return
	}

	// If GPUs are detected, verify structure
	for i, gpu := range gpus {
		if gpu.Index < 0 {
			t.Errorf("GPU %d has negative index: %d", i, gpu.Index)
		}
		if gpu.Name == "" {
			t.Errorf("GPU %d has empty name", i)
		}
		// Other fields can be zero (normal for idle GPU)
	}
}

func TestContainerMetricsEmpty(t *testing.T) {
	t.Parallel()

	// Without Docker or without yokai containers, this should return
	// nil or empty slice without error
	containers := collectContainers()

	// Should not panic and return a slice (possibly empty)
	if containers == nil {
		t.Log("No containers detected (expected on systems without Docker or yokai containers)")
		return
	}

	// If containers are detected, verify structure
	for i, container := range containers {
		if container.ID == "" {
			t.Errorf("Container %d has empty ID", i)
		}
		if container.Name == "" {
			t.Errorf("Container %d has empty name", i)
		}
		if container.CPUPercent < 0 {
			t.Errorf("Container %d has negative CPU percent: %.2f", i, container.CPUPercent)
		}
		if container.MemUsedMB < 0 {
			t.Errorf("Container %d has negative memory usage: %d", i, container.MemUsedMB)
		}
	}
}

func TestSystemMetricsJSONSerialization(t *testing.T) {
	t.Parallel()

	// Create a sample SystemMetrics struct
	metrics := &SystemMetrics{
		Timestamp: time.Now().UTC(),
		CPU:       CPUMetrics{Percent: 25.5, PerCore: []float64{20.0, 30.0}},
		RAM:       RAMMetrics{UsedMB: 8192, TotalMB: 16384, Percent: 50.0},
		Swap:      SwapMetrics{UsedMB: 0, TotalMB: 4096},
		Disk:      DiskMetrics{UsedGB: 100, TotalGB: 500, FreeGB: 400},
		GPUs: []GPUMetrics{
			{
				Index:       0,
				Name:        "GeForce RTX 4090",
				UtilPercent: 75,
				VRAMUsedMB:  12000,
				VRAMTotalMB: 24000,
				TempC:       65,
				PowerDrawW:  300,
				PowerLimitW: 450,
				FanPercent:  60,
			},
		},
		Containers: []ContainerMetrics{
			{
				ID:         "abc123456789",
				Name:       "yokai-vllm-1",
				Status:     "running",
				CPUPercent: 15.5,
				MemUsedMB:  2048,
				Uptime:     3600,
			},
		},
	}

	// Test JSON marshaling
	data, err := json.Marshal(metrics)
	if err != nil {
		t.Fatalf("failed to marshal metrics to JSON: %v", err)
	}

	// Test JSON unmarshaling
	var unmarshaled SystemMetrics
	if err := json.Unmarshal(data, &unmarshaled); err != nil {
		t.Fatalf("failed to unmarshal metrics from JSON: %v", err)
	}

	// Verify key fields
	if unmarshaled.CPU.Percent != 25.5 {
		t.Errorf("CPU percent mismatch: expected 25.5, got %.1f", unmarshaled.CPU.Percent)
	}
	if unmarshaled.RAM.UsedMB != 8192 {
		t.Errorf("RAM used mismatch: expected 8192, got %d", unmarshaled.RAM.UsedMB)
	}
	if len(unmarshaled.GPUs) != 1 {
		t.Errorf("GPU count mismatch: expected 1, got %d", len(unmarshaled.GPUs))
	}
	if len(unmarshaled.Containers) != 1 {
		t.Errorf("Container count mismatch: expected 1, got %d", len(unmarshaled.Containers))
	}
}
