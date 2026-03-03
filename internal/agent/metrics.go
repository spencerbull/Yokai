package agent

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// SystemMetrics holds a snapshot of system metrics.
type SystemMetrics struct {
	Timestamp  time.Time          `json:"timestamp"`
	CPU        CPUMetrics         `json:"cpu"`
	RAM        RAMMetrics         `json:"ram"`
	Swap       SwapMetrics        `json:"swap"`
	Disk       DiskMetrics        `json:"disk"`
	GPUs       []GPUMetrics       `json:"gpus"`
	Containers []ContainerMetrics `json:"containers"`
}

// CPUMetrics holds CPU utilization.
type CPUMetrics struct {
	Percent float64   `json:"percent"`
	PerCore []float64 `json:"per_core,omitempty"`
}

// RAMMetrics holds memory usage.
type RAMMetrics struct {
	UsedMB  int64   `json:"used_mb"`
	TotalMB int64   `json:"total_mb"`
	Percent float64 `json:"percent"`
}

// SwapMetrics holds swap usage.
type SwapMetrics struct {
	UsedMB  int64 `json:"used_mb"`
	TotalMB int64 `json:"total_mb"`
}

// DiskMetrics holds disk usage.
type DiskMetrics struct {
	UsedGB  int64 `json:"used_gb"`
	TotalGB int64 `json:"total_gb"`
	FreeGB  int64 `json:"free_gb"`
}

// GPUMetrics holds GPU metrics from nvidia-smi.
type GPUMetrics struct {
	Index       int    `json:"index"`
	Name        string `json:"name"`
	UtilPercent int    `json:"utilization_percent"`
	VRAMUsedMB  int64  `json:"vram_used_mb"`
	VRAMTotalMB int64  `json:"vram_total_mb"`
	TempC       int    `json:"temperature_c"`
	PowerDrawW  int    `json:"power_draw_w"`
	PowerLimitW int    `json:"power_limit_w"`
	FanPercent  int    `json:"fan_percent"`
}

// ContainerMetrics holds per-container resource usage.
type ContainerMetrics struct {
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	Image       string  `json:"image,omitempty"`
	Status      string  `json:"status"`
	CPUPercent  float64 `json:"cpu_percent"`
	MemUsedMB   int64   `json:"memory_used_mb"`
	GPUMemoryMB int64   `json:"gpu_memory_mb,omitempty"`
	Uptime      int64   `json:"uptime_seconds"`
}

// CollectMetrics gathers all system metrics.
func CollectMetrics() *SystemMetrics {
	m := &SystemMetrics{
		Timestamp: time.Now().UTC(),
	}

	m.CPU = collectCPU()
	m.RAM = collectRAM()
	m.Swap = collectSwap()
	m.Disk = collectDisk()
	m.GPUs = collectGPUs()
	m.Containers = collectContainers()

	return m
}

// collectCPU reads CPU utilization from /proc/stat.
func collectCPU() CPUMetrics {
	if runtime.GOOS != "linux" {
		return CPUMetrics{Percent: 0}
	}

	data, err := os.ReadFile("/proc/stat")
	if err != nil {
		return CPUMetrics{Percent: 0}
	}

	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "cpu ") {
			fields := strings.Fields(line)
			if len(fields) < 5 {
				continue
			}
			user, _ := strconv.ParseFloat(fields[1], 64)
			nice, _ := strconv.ParseFloat(fields[2], 64)
			system, _ := strconv.ParseFloat(fields[3], 64)
			idle, _ := strconv.ParseFloat(fields[4], 64)
			total := user + nice + system + idle
			if total > 0 {
				used := user + nice + system
				return CPUMetrics{Percent: (used / total) * 100}
			}
		}
	}

	return CPUMetrics{Percent: 0}
}

// collectRAM reads memory info from /proc/meminfo.
func collectRAM() RAMMetrics {
	if runtime.GOOS != "linux" {
		return RAMMetrics{}
	}

	data, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return RAMMetrics{}
	}

	var totalKB, availableKB int64
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, "MemTotal:") {
			if _, err := fmt.Sscanf(line, "MemTotal: %d kB", &totalKB); err != nil {
				continue
			}
		}
		if strings.HasPrefix(line, "MemAvailable:") {
			if _, err := fmt.Sscanf(line, "MemAvailable: %d kB", &availableKB); err != nil {
				continue
			}
		}
	}

	totalMB := totalKB / 1024
	usedMB := (totalKB - availableKB) / 1024
	var percent float64
	if totalMB > 0 {
		percent = float64(usedMB) / float64(totalMB) * 100
	}

	return RAMMetrics{
		UsedMB:  usedMB,
		TotalMB: totalMB,
		Percent: percent,
	}
}

// collectSwap reads swap info from /proc/meminfo.
func collectSwap() SwapMetrics {
	if runtime.GOOS != "linux" {
		return SwapMetrics{}
	}

	data, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return SwapMetrics{}
	}

	var totalKB, freeKB int64
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, "SwapTotal:") {
			if _, err := fmt.Sscanf(line, "SwapTotal: %d kB", &totalKB); err != nil {
				continue
			}
		}
		if strings.HasPrefix(line, "SwapFree:") {
			if _, err := fmt.Sscanf(line, "SwapFree: %d kB", &freeKB); err != nil {
				continue
			}
		}
	}

	return SwapMetrics{
		UsedMB:  (totalKB - freeKB) / 1024,
		TotalMB: totalKB / 1024,
	}
}

// collectDisk reads disk usage from df.
func collectDisk() DiskMetrics {
	out, err := exec.Command("df", "-BG", "--output=size,used,avail", "/").Output()
	if err != nil {
		return DiskMetrics{}
	}

	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(lines) < 2 {
		return DiskMetrics{}
	}

	fields := strings.Fields(lines[1])
	if len(fields) < 3 {
		return DiskMetrics{}
	}

	var total, used, avail int64
	if _, err := fmt.Sscanf(strings.TrimSuffix(fields[0], "G"), "%d", &total); err != nil {
		return DiskMetrics{}
	}
	if _, err := fmt.Sscanf(strings.TrimSuffix(fields[1], "G"), "%d", &used); err != nil {
		return DiskMetrics{}
	}
	if _, err := fmt.Sscanf(strings.TrimSuffix(fields[2], "G"), "%d", &avail); err != nil {
		return DiskMetrics{}
	}

	return DiskMetrics{
		TotalGB: total,
		UsedGB:  used,
		FreeGB:  avail,
	}
}

// collectGPUs uses nvidia-smi to gather GPU metrics.
func collectGPUs() []GPUMetrics {
	out, err := exec.Command("nvidia-smi",
		"--query-gpu=index,name,utilization.gpu,memory.used,memory.total,temperature.gpu,power.draw,power.limit,fan.speed",
		"--format=csv,noheader,nounits",
	).Output()
	if err != nil {
		return nil
	}

	var gpus []GPUMetrics
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line == "" {
			continue
		}
		fields := strings.Split(line, ",")
		if len(fields) < 9 {
			continue
		}

		for i := range fields {
			fields[i] = strings.TrimSpace(fields[i])
		}

		index, _ := strconv.Atoi(fields[0])
		util, _ := strconv.Atoi(fields[2])
		vramUsed, _ := strconv.ParseInt(fields[3], 10, 64)
		vramTotal, _ := strconv.ParseInt(fields[4], 10, 64)
		temp, _ := strconv.Atoi(fields[5])
		powerDraw, _ := strconv.ParseFloat(fields[6], 64)
		powerLimit, _ := strconv.ParseFloat(fields[7], 64)
		fan, _ := strconv.Atoi(fields[8])

		gpus = append(gpus, GPUMetrics{
			Index:       index,
			Name:        fields[1],
			UtilPercent: util,
			VRAMUsedMB:  vramUsed,
			VRAMTotalMB: vramTotal,
			TempC:       temp,
			PowerDrawW:  int(powerDraw),
			PowerLimitW: int(powerLimit),
			FanPercent:  fan,
		})
	}

	return gpus
}

// collectContainers uses docker stats to gather container metrics.
func collectContainers() []ContainerMetrics {
	out, err := exec.Command("docker", "stats", "--no-stream",
		"--format", "{{.ID}}\t{{.Name}}\t{{.CPUPerc}}\t{{.MemUsage}}",
		"--filter", "name=yokai-",
	).Output()
	if err != nil {
		return nil
	}

	var containers []ContainerMetrics
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line == "" {
			continue
		}
		fields := strings.Split(line, "\t")
		if len(fields) < 4 {
			continue
		}

		cpuStr := strings.TrimSuffix(fields[2], "%")
		cpuPercent, _ := strconv.ParseFloat(cpuStr, 64)

		// Parse memory: "1.2GiB / 32GiB" or "512MiB / 32GiB"
		var memUsedMB int64
		memParts := strings.Split(fields[3], "/")
		if len(memParts) >= 1 {
			memStr := strings.TrimSpace(memParts[0])
			if strings.HasSuffix(memStr, "GiB") {
				val, _ := strconv.ParseFloat(strings.TrimSuffix(memStr, "GiB"), 64)
				memUsedMB = int64(val * 1024)
			} else if strings.HasSuffix(memStr, "MiB") {
				val, _ := strconv.ParseFloat(strings.TrimSuffix(memStr, "MiB"), 64)
				memUsedMB = int64(val)
			}
		}

		containers = append(containers, ContainerMetrics{
			ID:         fields[0][:12],
			Name:       fields[1],
			Status:     "running",
			CPUPercent: cpuPercent,
			MemUsedMB:  memUsedMB,
		})
	}

	return containers
}
