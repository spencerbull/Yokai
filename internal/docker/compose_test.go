package docker

import (
	"strings"
	"testing"
)

func TestGenerateMonitoringCompose(t *testing.T) {
	t.Parallel()

	cfg := MonitoringConfig{
		AgentHost:      "192.168.1.100",
		AgentPort:      7474,
		PrometheusPort: 9090,
		GrafanaPort:    3000,
		HasNvidiaGPU:   false,
	}

	compose := GenerateMonitoringCompose(cfg)

	// Test that all required services are present
	requiredServices := []string{
		"prometheus:",
		"grafana:",
		"node_exporter:",
	}

	for _, service := range requiredServices {
		if !strings.Contains(compose, service) {
			t.Errorf("compose should contain service: %s", service)
		}
	}

	// Test port mappings
	expectedPorts := []string{
		"9090:9090", // prometheus
		"3000:3000", // grafana
	}

	for _, port := range expectedPorts {
		if !strings.Contains(compose, port) {
			t.Errorf("compose should contain port mapping: %s", port)
		}
	}

	// Test container names
	expectedContainerNames := []string{
		"yokai-mon-prometheus",
		"yokai-mon-grafana",
		"yokai-mon-node-exporter",
	}

	for _, name := range expectedContainerNames {
		if !strings.Contains(compose, name) {
			t.Errorf("compose should contain container name: %s", name)
		}
	}

	// Test networks and volumes
	if !strings.Contains(compose, "yokai-monitoring:") {
		t.Error("compose should contain yokai-monitoring network")
	}
	if !strings.Contains(compose, "prometheus_data:") {
		t.Error("compose should contain prometheus_data volume")
	}
	if !strings.Contains(compose, "grafana_data:") {
		t.Error("compose should contain grafana_data volume")
	}

	// Test that dcgm-exporter is NOT included when HasNvidiaGPU is false
	if strings.Contains(compose, "dcgm_exporter:") {
		t.Error("compose should not contain dcgm_exporter when HasNvidiaGPU is false")
	}
}

func TestGenerateMonitoringComposeWithGPU(t *testing.T) {
	t.Parallel()

	cfg := MonitoringConfig{
		AgentHost:      "localhost",
		AgentPort:      7475,
		PrometheusPort: 9091,
		GrafanaPort:    3001,
		HasNvidiaGPU:   true,
	}

	compose := GenerateMonitoringCompose(cfg)

	// Test that dcgm-exporter IS included when HasNvidiaGPU is true
	if !strings.Contains(compose, "dcgm_exporter:") {
		t.Error("compose should contain dcgm_exporter when HasNvidiaGPU is true")
	}

	// Test dcgm-exporter specific configuration
	dcgmRequiredConfig := []string{
		"container_name: yokai-mon-dcgm-exporter",
		"image: nvidia/dcgm-exporter:latest",
		"runtime: nvidia",
		"NVIDIA_VISIBLE_DEVICES=all",
		"9400:9400",
	}

	for _, config := range dcgmRequiredConfig {
		if !strings.Contains(compose, config) {
			t.Errorf("compose should contain dcgm config: %s", config)
		}
	}

	// Test custom ports
	if !strings.Contains(compose, "9091:9090") {
		t.Error("compose should use custom prometheus port 9091")
	}
	if !strings.Contains(compose, "3001:3000") {
		t.Error("compose should use custom grafana port 3001")
	}
}

func TestGenerateMonitoringComposeWithoutGPU(t *testing.T) {
	t.Parallel()

	cfg := MonitoringConfig{
		AgentHost:      "remote.host",
		AgentPort:      8080,
		PrometheusPort: 8090,
		GrafanaPort:    8000,
		HasNvidiaGPU:   false,
	}

	compose := GenerateMonitoringCompose(cfg)

	// Verify dcgm-exporter is NOT included
	gpuRelatedStrings := []string{
		"dcgm_exporter:",
		"nvidia/dcgm-exporter",
		"runtime: nvidia",
		"NVIDIA_VISIBLE_DEVICES",
	}

	for _, str := range gpuRelatedStrings {
		if strings.Contains(compose, str) {
			t.Errorf("compose should not contain GPU-related config when HasNvidiaGPU is false: %s", str)
		}
	}

	// Verify custom ports are used
	if !strings.Contains(compose, "8090:9090") {
		t.Error("compose should use custom prometheus port 8090")
	}
	if !strings.Contains(compose, "8000:3000") {
		t.Error("compose should use custom grafana port 8000")
	}
}

func TestGeneratePrometheusConfig(t *testing.T) {
	t.Parallel()

	cfg := MonitoringConfig{
		AgentHost:    "192.168.1.50",
		AgentPort:    7474,
		HasNvidiaGPU: false,
	}

	config := GeneratePrometheusConfig(cfg)

	// Test global configuration
	if !strings.Contains(config, "scrape_interval: 15s") {
		t.Error("config should contain scrape interval")
	}
	if !strings.Contains(config, "evaluation_interval: 15s") {
		t.Error("config should contain evaluation interval")
	}

	// Test scrape configs
	requiredJobs := []string{
		"job_name: 'node'",
		"job_name: 'yokai-agent'",
	}

	for _, job := range requiredJobs {
		if !strings.Contains(config, job) {
			t.Errorf("config should contain job: %s", job)
		}
	}

	// Test agent target
	expectedAgentTarget := "192.168.1.50:7474"
	if !strings.Contains(config, expectedAgentTarget) {
		t.Errorf("config should contain agent target: %s", expectedAgentTarget)
	}

	// Test that dcgm job is NOT included when HasNvidiaGPU is false
	if strings.Contains(config, "job_name: 'dcgm'") {
		t.Error("config should not contain dcgm job when HasNvidiaGPU is false")
	}
}

func TestGeneratePrometheusConfigWithGPU(t *testing.T) {
	t.Parallel()

	cfg := MonitoringConfig{
		AgentHost:    "gpu.server",
		AgentPort:    9999,
		HasNvidiaGPU: true,
	}

	config := GeneratePrometheusConfig(cfg)

	// Test that dcgm job IS included when HasNvidiaGPU is true
	if !strings.Contains(config, "job_name: 'dcgm'") {
		t.Error("config should contain dcgm job when HasNvidiaGPU is true")
	}

	// Test dcgm target
	if !strings.Contains(config, "dcgm_exporter:9400") {
		t.Error("config should contain dcgm_exporter target")
	}

	// Test custom agent configuration
	expectedAgentTarget := "gpu.server:9999"
	if !strings.Contains(config, expectedAgentTarget) {
		t.Errorf("config should contain custom agent target: %s", expectedAgentTarget)
	}
}

func TestMonitoringConfigStructure(t *testing.T) {
	t.Parallel()

	cfg := MonitoringConfig{
		AgentHost:      "test.host",
		AgentPort:      1234,
		PrometheusPort: 5678,
		GrafanaPort:    9012,
		HasNvidiaGPU:   true,
	}

	// Test all fields are accessible
	if cfg.AgentHost != "test.host" {
		t.Errorf("expected AgentHost 'test.host', got %s", cfg.AgentHost)
	}
	if cfg.AgentPort != 1234 {
		t.Errorf("expected AgentPort 1234, got %d", cfg.AgentPort)
	}
	if cfg.PrometheusPort != 5678 {
		t.Errorf("expected PrometheusPort 5678, got %d", cfg.PrometheusPort)
	}
	if cfg.GrafanaPort != 9012 {
		t.Errorf("expected GrafanaPort 9012, got %d", cfg.GrafanaPort)
	}
	if !cfg.HasNvidiaGPU {
		t.Error("expected HasNvidiaGPU true")
	}

	// Test zero value
	var zeroConfig MonitoringConfig
	if zeroConfig.AgentHost != "" {
		t.Error("zero value AgentHost should be empty")
	}
	if zeroConfig.AgentPort != 0 {
		t.Error("zero value AgentPort should be 0")
	}
	if zeroConfig.HasNvidiaGPU {
		t.Error("zero value HasNvidiaGPU should be false")
	}
}

func TestComposeYAMLStructure(t *testing.T) {
	t.Parallel()

	cfg := MonitoringConfig{
		AgentHost:      "localhost",
		AgentPort:      7474,
		PrometheusPort: 9090,
		GrafanaPort:    3000,
		HasNvidiaGPU:   false,
	}

	compose := GenerateMonitoringCompose(cfg)

	// Test YAML structure elements
	yamlElements := []string{
		"version: '3.8'",
		"services:",
		"networks:",
		"volumes:",
	}

	for _, element := range yamlElements {
		if !strings.Contains(compose, element) {
			t.Errorf("compose should contain YAML element: %s", element)
		}
	}

	// Test service configuration elements
	serviceConfigElements := []string{
		"container_name:",
		"image:",
		"ports:",
		"volumes:",
		"restart: unless-stopped",
		"networks:",
	}

	for _, element := range serviceConfigElements {
		if !strings.Contains(compose, element) {
			t.Errorf("compose should contain service config element: %s", element)
		}
	}
}

func TestPrometheusConfigYAMLStructure(t *testing.T) {
	t.Parallel()

	cfg := MonitoringConfig{
		AgentHost:    "localhost",
		AgentPort:    7474,
		HasNvidiaGPU: false,
	}

	config := GeneratePrometheusConfig(cfg)

	// Test YAML structure
	yamlElements := []string{
		"global:",
		"scrape_configs:",
		"static_configs:",
		"targets:",
	}

	for _, element := range yamlElements {
		if !strings.Contains(config, element) {
			t.Errorf("prometheus config should contain YAML element: %s", element)
		}
	}

	// Test required configuration keys
	configKeys := []string{
		"scrape_interval:",
		"evaluation_interval:",
		"metrics_path:",
	}

	for _, key := range configKeys {
		if !strings.Contains(config, key) {
			t.Errorf("prometheus config should contain key: %s", key)
		}
	}
}
