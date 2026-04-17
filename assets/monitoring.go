package assets

import _ "embed"

var (
	//go:embed grafana/provisioning/datasources/prometheus.yml
	GrafanaDatasourceProvisioning string

	//go:embed grafana/provisioning/dashboards/dashboard.yml
	GrafanaDashboardProvisioning string

	//go:embed grafana/dashboards/gpu-dashboard.json
	DefaultGrafanaDashboard string
)
