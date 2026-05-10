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

func TestDefaultGrafanaDashboardUsesSelectedRangeForStatTiles(t *testing.T) {
	t.Parallel()

	var dashboard struct {
		Panels []struct {
			Type    string `json:"type"`
			Title   string `json:"title"`
			Targets []struct {
				Expr    string `json:"expr"`
				Instant bool   `json:"instant"`
			} `json:"targets"`
		} `json:"panels"`
	}
	if err := json.Unmarshal([]byte(DefaultGrafanaDashboard), &dashboard); err != nil {
		t.Fatalf("dashboard JSON should be valid: %v", err)
	}

	rangeDrivenStatPanels := map[string]bool{
		"Active Avg Decode Tok/s":    false,
		"Active Avg Prefill Tok/s":   false,
		"Avg Nonzero Sessions":       false,
		"Avg Nonzero Queue":          false,
		"Active Avg Generated Tok/s": false,
		"Avg Services Up":            false,
		"Active Avg Prompt Tok/s":    false,
	}

	for _, panel := range dashboard.Panels {
		if panel.Type != "stat" {
			continue
		}
		if len(panel.Targets) == 0 {
			t.Fatalf("stat panel %q should have a target", panel.Title)
		}
		expr := panel.Targets[0].Expr
		if strings.Contains(panel.Title, "1h") || strings.Contains(expr, "[1h]") {
			t.Fatalf("stat panel %q should not hard-code a 1h range: %s", panel.Title, expr)
		}
		if _, ok := rangeDrivenStatPanels[panel.Title]; !ok {
			continue
		}
		if !strings.Contains(expr, "$__range") {
			t.Fatalf("stat panel %q should use Grafana's selected range: %s", panel.Title, expr)
		}
		if !panel.Targets[0].Instant {
			t.Fatalf("stat panel %q should query one selected-range value instead of a graph series", panel.Title)
		}
		rangeDrivenStatPanels[panel.Title] = true
	}

	for title, seen := range rangeDrivenStatPanels {
		if !seen {
			t.Fatalf("dashboard should include range-driven stat panel %q", title)
		}
	}
}

func TestDefaultGrafanaDashboardStatTilesAverageSelectedRange(t *testing.T) {
	t.Parallel()

	var dashboard struct {
		Panels []struct {
			Type    string `json:"type"`
			Title   string `json:"title"`
			Targets []struct {
				Expr string `json:"expr"`
			} `json:"targets"`
		} `json:"panels"`
	}
	if err := json.Unmarshal([]byte(DefaultGrafanaDashboard), &dashboard); err != nil {
		t.Fatalf("dashboard JSON should be valid: %v", err)
	}

	averagedPanels := map[string]bool{
		"Active Avg Decode Tok/s":    false,
		"Active Avg Prefill Tok/s":   false,
		"Avg Nonzero Sessions":       false,
		"Avg Nonzero Queue":          false,
		"Active Avg Generated Tok/s": false,
		"Avg Services Up":            false,
		"Active Avg Prompt Tok/s":    false,
	}
	for _, panel := range dashboard.Panels {
		if _, ok := averagedPanels[panel.Title]; !ok || panel.Type != "stat" {
			continue
		}
		if len(panel.Targets) == 0 {
			t.Fatalf("stat panel %q should have a target", panel.Title)
		}
		expr := panel.Targets[0].Expr
		if !strings.Contains(expr, "avg_over_time(") {
			t.Fatalf("stat panel %q should average over Grafana's selected range: %s", panel.Title, expr)
		}
		averagedPanels[panel.Title] = true
	}
	for title, seen := range averagedPanels {
		if !seen {
			t.Fatalf("dashboard should include averaged stat panel %q", title)
		}
	}
}

func TestDefaultGrafanaDashboardWorkloadStatTilesIgnoreZeroSamples(t *testing.T) {
	t.Parallel()

	var dashboard struct {
		Panels []struct {
			Type    string `json:"type"`
			Title   string `json:"title"`
			Targets []struct {
				Expr string `json:"expr"`
			} `json:"targets"`
		} `json:"panels"`
	}
	if err := json.Unmarshal([]byte(DefaultGrafanaDashboard), &dashboard); err != nil {
		t.Fatalf("dashboard JSON should be valid: %v", err)
	}

	nonzeroPanels := map[string]bool{
		"Active Avg Decode Tok/s":    false,
		"Active Avg Prefill Tok/s":   false,
		"Avg Nonzero Sessions":       false,
		"Avg Nonzero Queue":          false,
		"Active Avg Generated Tok/s": false,
		"Active Avg Prompt Tok/s":    false,
	}

	for _, panel := range dashboard.Panels {
		if _, ok := nonzeroPanels[panel.Title]; !ok || panel.Type != "stat" {
			continue
		}
		if len(panel.Targets) == 0 {
			t.Fatalf("stat panel %q should have a target", panel.Title)
		}
		expr := panel.Targets[0].Expr
		if !strings.Contains(expr, " > 0)") {
			t.Fatalf("stat panel %q should ignore zero samples: %s", panel.Title, expr)
		}
		if !strings.Contains(expr, "or vector(0)") {
			t.Fatalf("stat panel %q should return zero for fully idle ranges: %s", panel.Title, expr)
		}
		nonzeroPanels[panel.Title] = true
	}

	for title, seen := range nonzeroPanels {
		if !seen {
			t.Fatalf("dashboard should include nonzero-filtered stat panel %q", title)
		}
	}
}

func TestDefaultGrafanaDashboardServicesUpStatTileIncludesZeroSamples(t *testing.T) {
	t.Parallel()

	var dashboard struct {
		Panels []struct {
			Type    string `json:"type"`
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
		if panel.Type != "stat" || panel.Title != "Avg Services Up" {
			continue
		}
		if len(panel.Targets) == 0 {
			t.Fatal("Avg Services Up should have a target")
		}
		expr := panel.Targets[0].Expr
		if strings.Contains(expr, " > 0") {
			t.Fatalf("Avg Services Up should include zero samples so downtime is visible: %s", expr)
		}
		return
	}

	t.Fatal("dashboard should include Avg Services Up panel")
}
