package main

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func dashboardPanels(t *testing.T) []any {
	t.Helper()
	panels, ok := buildDashboard()["panels"].([]any)
	if !ok {
		t.Fatal("dashboard panels have the wrong type")
	}
	return panels
}

func TestDashboardIdentityAndBreadth(t *testing.T) {
	dashboard := buildDashboard()
	if dashboard["uid"] != dashboardUID {
		t.Fatalf("uid = %v, want %q", dashboard["uid"], dashboardUID)
	}
	if dashboard["title"] != "DeepSeek V4 Flash · Beskar + Kyber" {
		t.Fatalf("unexpected dashboard title: %v", dashboard["title"])
	}

	panels := dashboardPanels(t)
	if len(panels) < 80 {
		t.Fatalf("dashboard has %d panels, want at least 80", len(panels))
	}

	rowTitles := make(map[string]bool)
	panelTypes := make(map[string]int)
	ids := make(map[int]bool)
	for _, raw := range panels {
		panel := raw.(map[string]any)
		id := panel["id"].(int)
		if ids[id] {
			t.Fatalf("duplicate panel id %d", id)
		}
		ids[id] = true
		panelType := panel["type"].(string)
		panelTypes[panelType]++
		if panelType == "row" {
			rowTitles[panel["title"].(string)] = true
		}
	}

	wantRows := []string{
		"01 · Model pulse",
		"02 · Tokens, requests, and throughput",
		"03 · Latency and user experience",
		"04 · Cache, scheduler, and speculative decoding",
		"05 · GB10 unified memory and host pressure",
		"06 · GPU, thermals, power, and efficiency",
		"07 · Cost compare · local vs comparable APIs",
		"08 · Network, storage, and platform reliability",
	}
	for _, title := range wantRows {
		if !rowTitles[title] {
			t.Errorf("missing row %q", title)
		}
	}
	for _, panelType := range []string{"stat", "timeseries", "piechart", "bargauge", "table", "text"} {
		if panelTypes[panelType] == 0 {
			t.Errorf("missing panel type %q", panelType)
		}
	}
}

func TestDashboardQueriesUseLiveMetricSemantics(t *testing.T) {
	queries := make(map[string]struct{})
	collectQueries(buildDashboard(), queries)
	joined := strings.Join(mapKeys(queries), "\n")

	wantFragments := []string{
		`count(up{cluster="deepseek-v4-flash",job=~"deepseek-vllm|yokai-agent-.*|node"}) == bool 5`,
		"vllm:prompt_tokens_total",
		`vllm:prompt_tokens_by_source_total`,
		`source="local_compute"`,
		`source="local_cache_hit"`,
		"vllm:generation_tokens_total",
		"vllm:time_to_first_token_seconds_bucket",
		"vllm:request_time_per_output_token_seconds_bucket",
		"vllm:e2e_request_latency_seconds_bucket",
		"vllm:prefix_cache_hits_total",
		"vllm:spec_decode_num_accepted_tokens_total",
		"node_pressure_memory_waiting_seconds_total",
		"node_vmstat_oom_kill",
		"yokai_gpu_power_draw_watts",
		"sum_over_time(yokai_gpu_power_draw_watts",
		"count_over_time(yokai_gpu_power_draw_watts",
		"$electricity_rate",
		"label_replace",
		"$__rate_interval",
		"$__range",
	}
	for _, fragment := range wantFragments {
		if !strings.Contains(joined, fragment) {
			t.Errorf("dashboard queries are missing %q", fragment)
		}
	}

	for _, forbidden := range []string{
		"avg_generation_throughput_toks_per_s",
		"avg_prompt_throughput_toks_per_s",
		"[$__interval]",
		"[1m]",
		"[5m]",
		`user=`,
		`session=`,
	} {
		if strings.Contains(joined, forbidden) {
			t.Errorf("dashboard query contains forbidden pattern %q", forbidden)
		}
	}
}

func TestCostComparisonAssumptionsAreExplicitAndCurrent(t *testing.T) {
	if len(apiComparators) != 4 {
		t.Fatalf("api comparators = %d, want 4", len(apiComparators))
	}
	want := []apiComparator{
		{label: "DeepSeek V4 Flash API · AA 52", intelligence: 52, inputPrice: 0.14, outputPrice: 0.28},
		{label: "Gemini 3.6 Flash high · AA 52", intelligence: 52, inputPrice: 1.50, outputPrice: 7.50},
		{label: "GPT-5.6 Terra high · AA 50", intelligence: 50, inputPrice: 2.00, outputPrice: 12.00},
		{label: "Claude Sonnet 5 max · AA 55", intelligence: 55, inputPrice: 2.00, outputPrice: 10.00},
	}
	for index := range want {
		if apiComparators[index] != want[index] {
			t.Errorf("api comparator %d = %+v, want %+v", index, apiComparators[index], want[index])
		}
	}

	encoded, err := json.Marshal(buildDashboard())
	if err != nil {
		t.Fatal(err)
	}
	text := string(encoded)
	for _, required := range []string{
		`"name":"electricity_rate"`,
		`"query":"0.1615"`,
		"standard **uncached** list rates",
		"token-for-token counterfactual",
		"observed NVIDIA GPU-domain watts",
		"not total cost of ownership",
		"including idle",
		"year-to-date average through May 2026",
		"not a North Houston tariff",
		"Requires at least 99% telemetry coverage",
		"local quantization is not independently re-benchmarked",
		"EIA Texas statewide residential year-to-date average",
		"captured Aug 6, 2026",
	} {
		if !strings.Contains(text, required) {
			t.Errorf("dashboard cost copy is missing %q", required)
		}
	}
}

func TestIntelligenceComparisonUsesFixedScale(t *testing.T) {
	for _, raw := range dashboardPanels(t) {
		panel := raw.(map[string]any)
		if panel["id"] != 138 {
			continue
		}
		defaults := panel["fieldConfig"].(map[string]any)["defaults"].(map[string]any)
		if defaults["min"] != 0 || defaults["max"] != float64(100) {
			t.Fatalf("intelligence gauge scale = %v..%v, want 0..100", defaults["min"], defaults["max"])
		}
		return
	}
	t.Fatal("intelligence comparison panel not found")
}

func TestDailyProjectionRequiresPowerCoverage(t *testing.T) {
	expr := projectedDailyGPUElectricityCostExpr()
	for _, required := range []string{">= 99", "and on()", "count_over_time(yokai_gpu_power_draw_watts"} {
		if !strings.Contains(expr, required) {
			t.Errorf("daily projection is missing coverage gate %q: %s", required, expr)
		}
	}
}

func TestDashboardIsSerializableAndProvisionedReadOnly(t *testing.T) {
	dashboard := buildDashboard()
	if dashboard["editable"] != false {
		t.Fatal("provisioned dashboard must be read-only")
	}
	encoded, err := json.Marshal(dashboard)
	if err != nil {
		t.Fatalf("marshal dashboard: %v", err)
	}
	for _, secretish := range []string{"VLLM_API_KEY", "agent-token", "Bearer "} {
		if strings.Contains(string(encoded), secretish) {
			t.Errorf("serialized dashboard contains secret-like value %q", secretish)
		}
	}
}

func TestPanelGridDoesNotOverlapWithinRows(t *testing.T) {
	type cell struct{ x, y, w, h, id int }
	var cells []cell
	for _, raw := range dashboardPanels(t) {
		panel := raw.(map[string]any)
		if panel["type"] == "row" {
			continue
		}
		pos := panel["gridPos"].(map[string]any)
		cells = append(cells, cell{
			x: pos["x"].(int), y: pos["y"].(int),
			w: pos["w"].(int), h: pos["h"].(int),
			id: panel["id"].(int),
		})
	}
	for i, left := range cells {
		if left.x < 0 || left.w <= 0 || left.x+left.w > 24 || left.h <= 0 {
			t.Errorf("panel %d has invalid grid geometry: %+v", left.id, left)
		}
		for _, right := range cells[i+1:] {
			overlapsX := left.x < right.x+right.w && right.x < left.x+left.w
			overlapsY := left.y < right.y+right.h && right.y < left.y+left.h
			if overlapsX && overlapsY {
				t.Errorf("panels %d and %d overlap", left.id, right.id)
			}
		}
	}
}

func mapKeys(values map[string]struct{}) []string {
	result := make([]string, 0, len(values))
	for value := range values {
		result = append(result, value)
	}
	return result
}

func TestDescriptionsDoNotOverclaimLifetimeOrDedicatedVRAM(t *testing.T) {
	encoded, err := json.Marshal(buildDashboard())
	if err != nil {
		t.Fatal(err)
	}
	text := string(encoded)
	for _, required := range []string{"process lifetime", "resets when vLLM restarts", "unified memory", "not wall-system power"} {
		if !strings.Contains(strings.ToLower(text), strings.ToLower(required)) {
			t.Errorf("dashboard copy is missing required caveat %q", required)
		}
	}
	for _, forbidden := range []string{"lifetime since forever", "dedicated vram"} {
		if strings.Contains(strings.ToLower(text), forbidden) {
			t.Errorf("dashboard copy contains misleading phrase %q", forbidden)
		}
	}
}

func Example_buildDashboard() {
	dashboard := buildDashboard()
	fmt.Println(dashboard["uid"])
	// Output: deepseek-v4-flash
}
