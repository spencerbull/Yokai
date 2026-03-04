package docker

import (
	"fmt"
	"strings"
)

// MonitoringConfig holds parameters for the monitoring stack.
type MonitoringConfig struct {
	AgentHost      string // hostname/IP of the yokai agent
	AgentPort      int    // agent port (default 7474)
	PrometheusPort int    // expose prometheus on this port (default 9090)
	GrafanaPort    int    // expose grafana on this port (default 3000)
	HasNvidiaGPU   bool   // whether to include dcgm-exporter
}

// GenerateMonitoringCompose returns a docker-compose.yml string for the monitoring stack.
func GenerateMonitoringCompose(cfg MonitoringConfig) string {
	var services strings.Builder

	services.WriteString(`services:
  prometheus:
    container_name: yokai-mon-prometheus
    image: prom/prometheus:latest
    ports:
      - "`)
	services.WriteString(fmt.Sprintf("%d:9090", cfg.PrometheusPort))
	services.WriteString(`"
    volumes:
      - ./prometheus.yml:/etc/prometheus/prometheus.yml:ro
      - prometheus_data:/prometheus
    command:
      - '--config.file=/etc/prometheus/prometheus.yml'
      - '--storage.tsdb.path=/prometheus'
      - '--web.console.libraries=/etc/prometheus/console_libraries'
      - '--web.console.templates=/etc/prometheus/consoles'
      - '--storage.tsdb.retention.time=30d'
      - '--web.enable-lifecycle'
    networks:
      - yokai-monitoring
    restart: unless-stopped

  grafana:
    container_name: yokai-mon-grafana
    image: grafana/grafana:latest
    ports:
      - "`)
	services.WriteString(fmt.Sprintf("%d:3000", cfg.GrafanaPort))
	services.WriteString(`"
    environment:
      - GF_SECURITY_ADMIN_PASSWORD=admin
      - GF_AUTH_ANONYMOUS_ENABLED=true
      - GF_AUTH_ANONYMOUS_ORG_ROLE=Viewer
    volumes:
      - grafana_data:/var/lib/grafana
      - ./grafana/provisioning:/etc/grafana/provisioning:ro
      - ./grafana/dashboards:/var/lib/grafana/dashboards:ro
    networks:
      - yokai-monitoring
    restart: unless-stopped

  node_exporter:
    container_name: yokai-mon-node-exporter
    image: prom/node-exporter:latest
    network_mode: host
    pid: host
    volumes:
      - /proc:/host/proc:ro
      - /sys:/host/sys:ro
      - /:/rootfs:ro
    command:
      - '--path.procfs=/host/proc'
      - '--path.rootfs=/rootfs'
      - '--path.sysfs=/host/sys'
      - '--collector.filesystem.mount-points-exclude=^/(sys|proc|dev|host|etc)($$|/)'
    restart: unless-stopped
`)

	if cfg.HasNvidiaGPU {
		services.WriteString(`
  dcgm_exporter:
    container_name: yokai-mon-dcgm-exporter
    image: nvidia/dcgm-exporter:latest
    runtime: nvidia
    environment:
      - NVIDIA_VISIBLE_DEVICES=all
    ports:
      - "9400:9400"
    networks:
      - yokai-monitoring
    restart: unless-stopped
`)
	}

	services.WriteString(`
networks:
  yokai-monitoring:
    driver: bridge

volumes:
  prometheus_data:
    driver: local
  grafana_data:
    driver: local
`)

	return services.String()
}

// GeneratePrometheusConfig returns a prometheus.yml configuration.
func GeneratePrometheusConfig(cfg MonitoringConfig) string {
	var config strings.Builder

	config.WriteString(`global:
  scrape_interval: 15s
  evaluation_interval: 15s

scrape_configs:
  - job_name: 'node'
    static_configs:
      - targets: ['host.docker.internal:9100']

  - job_name: 'yokai-agent'
    metrics_path: '/metrics'
    static_configs:
      - targets: ['`)
	config.WriteString(fmt.Sprintf("%s:%d", cfg.AgentHost, cfg.AgentPort))
	config.WriteString(`']
`)

	if cfg.HasNvidiaGPU {
		config.WriteString(`
  - job_name: 'dcgm'
    static_configs:
      - targets: ['dcgm_exporter:9400']
`)
	}

	return config.String()
}
