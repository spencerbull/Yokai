package assets

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestGrafanaDatasourceProvisioningReplacesStalePrometheus(t *testing.T) {
	t.Parallel()

	checks := []string{
		"deleteDatasources:",
		"uid: yokai-prometheus",
		"url: http://prometheus:9090",
	}
	for _, check := range checks {
		if !strings.Contains(GrafanaDatasourceProvisioning, check) {
			t.Fatalf("datasource provisioning should contain %q", check)
		}
	}
	if strings.Contains(GrafanaDatasourceProvisioning, "localhost:9090") {
		t.Fatal("datasource provisioning should not point Grafana at localhost")
	}
}

func TestDefaultGrafanaDashboardIncludesTotalGeneratedTokens(t *testing.T) {
	t.Parallel()

	var dashboard struct {
		Panels []struct {
			Title   string `json:"title"`
			Targets []struct {
				Expr string `json:"expr"`
			} `json:"targets"`
		} `json:"panels"`
	}
	if err := json.Unmarshal([]byte(DefaultGrafanaDashboard), &dashboard); err != nil {
		t.Fatalf("dashboard JSON should be valid: %v", err)
	}

	for _, panel := range dashboard.Panels {
		if panel.Title != "Total Generated Tokens" || len(panel.Targets) == 0 {
			continue
		}
		if panel.Targets[0].Expr != "sum(yokai_llm_generated_tokens_total)" {
			t.Fatalf("unexpected total generated tokens expr: %s", panel.Targets[0].Expr)
		}
		return
	}
	t.Fatal("dashboard should include Total Generated Tokens panel")
}
