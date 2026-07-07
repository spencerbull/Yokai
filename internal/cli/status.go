package cli

import (
	"encoding/json"
	"flag"
	"fmt"

	"github.com/spencerbull/yokai/internal/config"
)

// RunStatus outputs fleet overview.
func RunStatus(_ []string) {
	cfg, err := config.Load()
	if err != nil {
		exitError(fmt.Sprintf("loading config: %v", err))
	}

	client := newDaemonClient(cfg)

	// Get devices
	devicesData, err := client.get("/devices")
	if err != nil {
		exitError(fmt.Sprintf("fetching devices: %v", err))
	}

	// Get metrics
	metricsData, err := client.get("/metrics")
	if err != nil {
		exitError(fmt.Sprintf("fetching metrics: %v", err))
	}

	// Parse devices
	var devicesResp struct {
		Devices []json.RawMessage `json:"devices"`
	}
	if err := json.Unmarshal(devicesData, &devicesResp); err != nil {
		exitError(fmt.Sprintf("parsing devices: %v", err))
	}

	// Count online/offline
	online := 0
	offline := 0
	for _, d := range devicesResp.Devices {
		var dev struct {
			Online bool `json:"online"`
		}
		if json.Unmarshal(d, &dev) == nil {
			if dev.Online {
				online++
			} else {
				offline++
			}
		}
	}

	// Count containers and GPU usage from metrics
	var allMetrics map[string]json.RawMessage
	containerCount := 0
	if json.Unmarshal(metricsData, &allMetrics) == nil {
		for _, m := range allMetrics {
			var metrics struct {
				Containers json.RawMessage `json:"containers"`
			}
			if json.Unmarshal(m, &metrics) == nil && len(metrics.Containers) > 2 {
				var containers []json.RawMessage
				if json.Unmarshal(metrics.Containers, &containers) == nil {
					containerCount += len(containers)
				}
			}
		}
	}

	outputJSON(map[string]interface{}{
		"devices_total":   len(devicesResp.Devices),
		"devices_online":  online,
		"devices_offline": offline,
		"containers":      containerCount,
		"devices":         devicesResp.Devices,
		"metrics":         allMetrics,
	})
}

// RunMetrics outputs detailed metrics.
func RunMetrics(args []string) {
	fs := flag.NewFlagSet("yokai metrics", flag.ExitOnError)
	deviceID := fs.String("device", "", "Filter by device ID")
	_ = fs.Parse(args)

	cfg, err := config.Load()
	if err != nil {
		exitError(fmt.Sprintf("loading config: %v", err))
	}

	client := newDaemonClient(cfg)

	if *deviceID != "" {
		data, err := client.get(fmt.Sprintf("/metrics/%s", *deviceID))
		if err != nil {
			exitError(fmt.Sprintf("fetching metrics for %s: %v", *deviceID, err))
		}
		outputRaw(data)
	} else {
		data, err := client.get("/metrics")
		if err != nil {
			exitError(fmt.Sprintf("fetching metrics: %v", err))
		}
		outputRaw(data)
	}
}
