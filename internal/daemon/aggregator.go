package daemon

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/spencerbull/yokai/internal/config"
	"github.com/spencerbull/yokai/internal/docker"
)

// Aggregator polls agents for metrics and forwards commands
type Aggregator struct {
	cfg     *config.Config
	tunnels *TunnelPool
	catalog *docker.Catalog
	metrics map[string]*AgentMetrics // keyed by device ID
	mu      sync.RWMutex
	cancel  context.CancelFunc
	client  *http.Client
}

// AgentMetrics represents metrics from a single device agent
type AgentMetrics struct {
	DeviceID   string          `json:"device_id"`
	Timestamp  time.Time       `json:"timestamp"`
	Online     bool            `json:"online"`
	CPU        json.RawMessage `json:"cpu"`
	RAM        json.RawMessage `json:"ram"`
	Swap       json.RawMessage `json:"swap"`
	Disk       json.RawMessage `json:"disk"`
	GPUs       json.RawMessage `json:"gpus"`
	Containers json.RawMessage `json:"containers"`
}

// NewAggregator creates a new metrics aggregator
func NewAggregator(cfg *config.Config, tunnels *TunnelPool) *Aggregator {
	return &Aggregator{
		cfg:     cfg,
		tunnels: tunnels,
		catalog: docker.NewCatalog(),
		metrics: make(map[string]*AgentMetrics),
		client:  &http.Client{Timeout: 10 * time.Second},
	}
}

// Start begins polling device agents for metrics
func (a *Aggregator) Start() {
	ctx, cancel := context.WithCancel(context.Background())
	a.cancel = cancel

	go a.pollMetrics(ctx)
}

// Stop cancels the polling goroutine
func (a *Aggregator) Stop() {
	if a.cancel != nil {
		a.cancel()
	}
}

// pollMetrics polls each device's agent at regular intervals
func (a *Aggregator) pollMetrics(ctx context.Context) {
	interval := time.Duration(a.cfg.Daemon.MetricsPollInterval) * time.Second
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			a.pollAllDevices()
		}
	}
}

// pollAllDevices polls metrics from all configured devices
func (a *Aggregator) pollAllDevices() {
	for _, device := range a.cfg.Devices {
		go a.pollDevice(device.ID)
	}
}

// pollDevice polls metrics from a single device
func (a *Aggregator) pollDevice(deviceID string) {
	localPort := a.tunnels.LocalPort(deviceID)
	if localPort == 0 {
		a.setDeviceOffline(deviceID)
		return
	}

	url := fmt.Sprintf("http://localhost:%d/metrics", localPort)
	resp, err := a.client.Get(url)
	if err != nil {
		log.Printf("metrics poll %s failed: %v", deviceID, err)
		a.setDeviceOffline(deviceID)
		return
	}
	defer func() {
		_ = resp.Body.Close() // Best-effort close of response body.
	}()

	if resp.StatusCode != http.StatusOK {
		log.Printf("metrics poll %s returned %d", deviceID, resp.StatusCode)
		a.setDeviceOffline(deviceID)
		return
	}

	var metricsData map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&metricsData); err != nil {
		log.Printf("metrics parse %s failed: %v", deviceID, err)
		a.setDeviceOffline(deviceID)
		return
	}

	// Convert to AgentMetrics
	metrics := &AgentMetrics{
		DeviceID:  deviceID,
		Timestamp: time.Now(),
		Online:    true,
	}

	// Extract individual components as raw JSON
	if cpu, ok := metricsData["cpu"]; ok {
		if cpuBytes, err := json.Marshal(cpu); err == nil {
			metrics.CPU = json.RawMessage(cpuBytes)
		}
	}

	if ram, ok := metricsData["ram"]; ok {
		if ramBytes, err := json.Marshal(ram); err == nil {
			metrics.RAM = json.RawMessage(ramBytes)
		}
	}

	if swap, ok := metricsData["swap"]; ok {
		if swapBytes, err := json.Marshal(swap); err == nil {
			metrics.Swap = json.RawMessage(swapBytes)
		}
	}

	if disk, ok := metricsData["disk"]; ok {
		if diskBytes, err := json.Marshal(disk); err == nil {
			metrics.Disk = json.RawMessage(diskBytes)
		}
	}

	if gpus, ok := metricsData["gpus"]; ok {
		if gpuBytes, err := json.Marshal(gpus); err == nil {
			metrics.GPUs = json.RawMessage(gpuBytes)
		}
	}

	if containers, ok := metricsData["containers"]; ok {
		if containerBytes, err := json.Marshal(containers); err == nil {
			metrics.Containers = json.RawMessage(containerBytes)
		}
	}

	a.mu.Lock()
	a.metrics[deviceID] = metrics
	a.mu.Unlock()
}

// setDeviceOffline marks a device as offline
func (a *Aggregator) setDeviceOffline(deviceID string) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if existing, exists := a.metrics[deviceID]; exists {
		existing.Online = false
		existing.Timestamp = time.Now()
	} else {
		a.metrics[deviceID] = &AgentMetrics{
			DeviceID:  deviceID,
			Timestamp: time.Now(),
			Online:    false,
		}
	}
}

// AllMetrics returns all cached metrics
func (a *Aggregator) AllMetrics() map[string]*AgentMetrics {
	a.mu.RLock()
	defer a.mu.RUnlock()

	result := make(map[string]*AgentMetrics)
	for k, v := range a.metrics {
		result[k] = v
	}
	return result
}

// DeviceMetrics returns metrics for a specific device
func (a *Aggregator) DeviceMetrics(deviceID string) (*AgentMetrics, bool) {
	a.mu.RLock()
	defer a.mu.RUnlock()

	metrics, exists := a.metrics[deviceID]
	return metrics, exists
}

// Deploy forwards a deploy request to the target device's agent
func (a *Aggregator) Deploy(req DeployRequest) (*DeployResult, error) {
	localPort := a.tunnels.LocalPort(req.DeviceID)
	if localPort == 0 {
		return nil, fmt.Errorf("device %s is not connected", req.DeviceID)
	}

	jsonData, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshaling request: %w", err)
	}

	url := fmt.Sprintf("http://localhost:%d/containers", localPort)
	resp, err := a.client.Post(url, "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("deploy request: %w", err)
	}
	defer func() {
		_ = resp.Body.Close() // Best-effort close of response body.
	}()

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("deploy failed with status %d", resp.StatusCode)
	}

	var result DeployResult
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("parsing deploy result: %w", err)
	}

	return &result, nil
}

// StopContainer forwards a stop request to the agent
func (a *Aggregator) StopContainer(deviceID, containerID string) error {
	localPort := a.tunnels.LocalPort(deviceID)
	if localPort == 0 {
		return fmt.Errorf("device %s is not connected", deviceID)
	}

	url := fmt.Sprintf("http://localhost:%d/containers/%s", localPort, containerID)
	req, err := http.NewRequest("DELETE", url, nil)
	if err != nil {
		return fmt.Errorf("creating request: %w", err)
	}

	resp, err := a.client.Do(req)
	if err != nil {
		return fmt.Errorf("stop request: %w", err)
	}
	defer func() {
		_ = resp.Body.Close() // Best-effort close of response body.
	}()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("stop failed with status %d", resp.StatusCode)
	}

	return nil
}

// RestartContainer forwards a restart request to the agent
func (a *Aggregator) RestartContainer(deviceID, containerID string) error {
	localPort := a.tunnels.LocalPort(deviceID)
	if localPort == 0 {
		return fmt.Errorf("device %s is not connected", deviceID)
	}

	url := fmt.Sprintf("http://localhost:%d/containers/%s/restart", localPort, containerID)
	resp, err := a.client.Post(url, "", nil)
	if err != nil {
		return fmt.Errorf("restart request: %w", err)
	}
	defer func() {
		_ = resp.Body.Close() // Best-effort close of restart response body.
	}()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("restart failed with status %d", resp.StatusCode)
	}

	return nil
}

// StreamLogs connects to agent's SSE log endpoint and relays lines to the channel
func (a *Aggregator) StreamLogs(deviceID, containerID string) (<-chan string, error) {
	localPort := a.tunnels.LocalPort(deviceID)
	if localPort == 0 {
		return nil, fmt.Errorf("device %s is not connected", deviceID)
	}

	url := fmt.Sprintf("http://localhost:%d/logs/%s", localPort, containerID)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("Cache-Control", "no-cache")

	resp, err := a.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("log stream request: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		_ = resp.Body.Close() // Best-effort close on non-OK log stream response.
		return nil, fmt.Errorf("log stream failed with status %d", resp.StatusCode)
	}

	ch := make(chan string, 100)

	go func() {
		defer func() {
			_ = resp.Body.Close() // Best-effort close of log stream response body.
		}()
		defer close(ch)

		scanner := bufio.NewScanner(resp.Body)
		for scanner.Scan() {
			line := scanner.Text()
			if line == "" {
				continue // Skip empty lines
			}

			// SSE format: "data: <content>"
			if len(line) > 6 && line[:6] == "data: " {
				ch <- line[6:]
			} else {
				ch <- line
			}
		}
	}()

	return ch, nil
}

// FetchImageTags uses the docker.Catalog to fetch tags
func (a *Aggregator) FetchImageTags(image string) ([]docker.Tag, error) {
	return a.catalog.FetchTags(image)
}
