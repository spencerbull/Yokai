package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"sort"
)

const (
	dashboardUID = "deepseek-v4-flash"
	clusterMatch = `cluster="deepseek-v4-flash"`
	modelMatch   = `model_name=~"$model"`
	hostMatch    = `host=~"$host"`
	// Yokai exposes nvidia-smi GPU-domain power on a fixed five-second scrape.
	// This is not whole-system or wall power.
	gpuPowerMatch           = `cluster="deepseek-v4-flash",host=~"beskar|kyber"`
	gpuPowerScrapeSeconds   = 5
	electricityRateVariable = "$electricity_rate"
)

type apiComparator struct {
	label        string
	intelligence int
	inputPrice   float64
	outputPrice  float64
}

// First-party standard, uncached USD prices per million tokens and Artificial
// Analysis Intelligence Index v4.1.1 scores, captured 2026-08-06.
var apiComparators = []apiComparator{
	{label: "DeepSeek V4 Flash API · AA 52", intelligence: 52, inputPrice: 0.14, outputPrice: 0.28},
	{label: "Gemini 3.6 Flash high · AA 52", intelligence: 52, inputPrice: 1.50, outputPrice: 7.50},
	{label: "GPT-5.6 Terra high · AA 50", intelligence: 50, inputPrice: 2.00, outputPrice: 12.00},
	{label: "Claude Sonnet 5 max · AA 55", intelligence: 55, inputPrice: 2.00, outputPrice: 10.00},
}

var datasource = map[string]any{
	"type": "prometheus",
	"uid":  "yokai-prometheus",
}

type panelTarget struct {
	expr   string
	legend string
	refID  string
	color  string
}

type statStyle struct {
	unit       string
	decimals   int
	baseColor  string
	warnAt     *float64
	criticalAt *float64
	lowIsBad   bool
	min        *float64
	max        *float64
}

func number(value float64) *float64 { return &value }

func target(item panelTarget, instant bool) map[string]any {
	return map[string]any{
		"datasource":   datasource,
		"editorMode":   "code",
		"expr":         item.expr,
		"instant":      instant,
		"legendFormat": item.legend,
		"range":        !instant,
		"refId":        item.refID,
	}
}

func thresholds(style statStyle) map[string]any {
	steps := []any{map[string]any{"color": style.baseColor, "value": nil}}
	if style.lowIsBad {
		steps = []any{map[string]any{"color": "red", "value": nil}}
		if style.warnAt != nil {
			steps = append(steps, map[string]any{"color": "orange", "value": *style.warnAt})
		}
		if style.criticalAt != nil {
			steps = append(steps, map[string]any{"color": style.baseColor, "value": *style.criticalAt})
		}
	} else {
		if style.warnAt != nil {
			steps = append(steps, map[string]any{"color": "orange", "value": *style.warnAt})
		}
		if style.criticalAt != nil {
			steps = append(steps, map[string]any{"color": "red", "value": *style.criticalAt})
		}
	}
	return map[string]any{"mode": "absolute", "steps": steps}
}

func statPanel(id int, title, description string, x, y, w int, query panelTarget, style statStyle) map[string]any {
	defaults := map[string]any{
		"color":      map[string]any{"mode": "thresholds"},
		"decimals":   style.decimals,
		"mappings":   []any{},
		"thresholds": thresholds(style),
		"unit":       style.unit,
	}
	if style.min != nil {
		defaults["min"] = *style.min
	}
	if style.max != nil {
		defaults["max"] = *style.max
	}
	return map[string]any{
		"datasource":  datasource,
		"description": description,
		"fieldConfig": map[string]any{"defaults": defaults, "overrides": []any{}},
		"gridPos":     map[string]any{"h": 4, "w": w, "x": x, "y": y},
		"id":          id,
		"options": map[string]any{
			"colorMode":   "value",
			"graphMode":   "area",
			"justifyMode": "auto",
			"orientation": "horizontal",
			"reduceOptions": map[string]any{
				"calcs":  []any{"lastNotNull"},
				"fields": "",
				"values": false,
			},
			"showPercentChange": false,
			"textMode":          "auto",
			"wideLayout":        true,
		},
		"pluginVersion": "13.0.1",
		"targets":       []any{target(query, true)},
		"title":         title,
		"type":          "stat",
	}
}

func rowPanel(id int, title string, y int) map[string]any {
	return map[string]any{
		"collapsed": false,
		"gridPos":   map[string]any{"h": 1, "w": 24, "x": 0, "y": y},
		"id":        id,
		"panels":    []any{},
		"title":     title,
		"type":      "row",
	}
}

func timeSeriesPanel(id int, title, description string, x, y, w, h int, queries []panelTarget, unit string, min, max *float64) map[string]any {
	defaults := map[string]any{
		"color":    map[string]any{"mode": "palette-classic"},
		"decimals": 2,
		"custom": map[string]any{
			"axisCenteredZero":  false,
			"axisColorMode":     "text",
			"axisLabel":         "",
			"axisPlacement":     "auto",
			"barAlignment":      0,
			"drawStyle":         "line",
			"fillOpacity":       8,
			"gradientMode":      "none",
			"hideFrom":          map[string]any{"legend": false, "tooltip": false, "viz": false},
			"insertNulls":       false,
			"lineInterpolation": "smooth",
			"lineWidth":         2,
			"pointSize":         4,
			"scaleDistribution": map[string]any{"type": "linear"},
			"showPoints":        "never",
			"spanNulls":         true,
			"stacking":          map[string]any{"group": "A", "mode": "none"},
			"thresholdsStyle":   map[string]any{"mode": "off"},
		},
		"mappings":   []any{},
		"thresholds": map[string]any{"mode": "absolute", "steps": []any{map[string]any{"color": "green", "value": nil}}},
		"unit":       unit,
	}
	if min != nil {
		defaults["min"] = *min
	}
	if max != nil {
		defaults["max"] = *max
	}
	targets := make([]any, 0, len(queries))
	overrides := make([]any, 0, len(queries))
	for _, query := range queries {
		targets = append(targets, target(query, false))
		if query.color != "" {
			overrides = append(overrides, map[string]any{
				"matcher": map[string]any{"id": "byName", "options": query.legend},
				"properties": []any{map[string]any{
					"id":    "color",
					"value": map[string]any{"fixedColor": query.color, "mode": "fixed"},
				}},
			})
		}
	}
	return map[string]any{
		"datasource":  datasource,
		"description": description,
		"fieldConfig": map[string]any{"defaults": defaults, "overrides": overrides},
		"gridPos":     map[string]any{"h": h, "w": w, "x": x, "y": y},
		"id":          id,
		"options": map[string]any{
			"legend": map[string]any{
				"calcs":       []any{"lastNotNull", "mean", "max"},
				"displayMode": "table",
				"placement":   "bottom",
				"showLegend":  true,
			},
			"tooltip": map[string]any{"hideZeros": false, "mode": "multi", "sort": "desc"},
		},
		"pluginVersion": "13.0.1",
		"targets":       targets,
		"title":         title,
		"type":          "timeseries",
	}
}

func hourlyTokenPanel(id, x, y, w, h int) map[string]any {
	panel := timeSeriesPanel(id, "Rolling hourly token volume", "One-hour rolling increases sampled on one-hour steps. Logical input includes cache hits; local compute isolates newly prefetched tokens.", x, y, w, h, []panelTarget{
		{expr: `sum(increase(vllm:prompt_tokens_by_source_total{` + clusterMatch + `,` + modelMatch + `,source="local_compute"}[1h]))`, legend: "New prefill", refID: "A", color: "blue"},
		{expr: `sum(increase(vllm:prompt_tokens_by_source_total{` + clusterMatch + `,` + modelMatch + `,source="local_cache_hit"}[1h]))`, legend: "Cached input", refID: "B", color: "purple"},
		{expr: `sum(increase(vllm:generation_tokens_total{` + clusterMatch + `,` + modelMatch + `}[1h]))`, legend: "Generated output", refID: "C", color: "orange"},
	}, "locale", number(0), nil)
	panel["interval"] = "1h"
	defaults := panel["fieldConfig"].(map[string]any)["defaults"].(map[string]any)
	custom := defaults["custom"].(map[string]any)
	custom["drawStyle"] = "bars"
	custom["fillOpacity"] = 75
	custom["lineWidth"] = 1
	custom["stacking"] = map[string]any{"group": "tokens", "mode": "normal"}
	return panel
}

func piePanel(id int, title, description string, x, y, w, h int, queries []panelTarget, unit string) map[string]any {
	targets := make([]any, 0, len(queries))
	overrides := make([]any, 0, len(queries))
	for _, query := range queries {
		targets = append(targets, target(query, true))
		if query.color != "" {
			overrides = append(overrides, map[string]any{
				"matcher":    map[string]any{"id": "byName", "options": query.legend},
				"properties": []any{map[string]any{"id": "color", "value": map[string]any{"fixedColor": query.color, "mode": "fixed"}}},
			})
		}
	}
	return map[string]any{
		"datasource":  datasource,
		"description": description,
		"fieldConfig": map[string]any{
			"defaults": map[string]any{
				"color":    map[string]any{"mode": "palette-classic"},
				"mappings": []any{},
				"unit":     unit,
			},
			"overrides": overrides,
		},
		"gridPos": map[string]any{"h": h, "w": w, "x": x, "y": y},
		"id":      id,
		"options": map[string]any{
			"displayLabels": []any{"name", "percent"},
			"legend":        map[string]any{"displayMode": "table", "placement": "bottom", "showLegend": true, "values": []any{"value", "percent"}},
			"pieType":       "donut",
			"reduceOptions": map[string]any{"calcs": []any{"lastNotNull"}, "fields": "", "values": false},
			"tooltip":       map[string]any{"hideZeros": true, "mode": "multi", "sort": "desc"},
		},
		"pluginVersion": "13.0.1",
		"targets":       targets,
		"title":         title,
		"type":          "piechart",
	}
}

func barGaugePanel(id int, title, description string, x, y, w, h int, query panelTarget, unit string, decimals int, max *float64) map[string]any {
	t := target(query, true)
	defaults := map[string]any{
		"color":      map[string]any{"mode": "continuous-BlPu"},
		"decimals":   decimals,
		"mappings":   []any{},
		"min":        0,
		"thresholds": map[string]any{"mode": "absolute", "steps": []any{map[string]any{"color": "blue", "value": nil}}},
		"unit":       unit,
	}
	if max != nil {
		defaults["max"] = *max
	}
	return map[string]any{
		"datasource":  datasource,
		"description": description,
		"fieldConfig": map[string]any{
			"defaults":  defaults,
			"overrides": []any{},
		},
		"gridPos": map[string]any{"h": h, "w": w, "x": x, "y": y},
		"id":      id,
		"options": map[string]any{
			"displayMode":   "lcd",
			"maxVizHeight":  300,
			"minVizHeight":  16,
			"minVizWidth":   8,
			"namePlacement": "left",
			"orientation":   "horizontal",
			"reduceOptions": map[string]any{"calcs": []any{"lastNotNull"}, "fields": "", "values": false},
			"showUnfilled":  true,
			"sizing":        "auto",
			"valueMode":     "color",
		},
		"pluginVersion": "13.0.1",
		"targets":       []any{t},
		"title":         title,
		"type":          "bargauge",
	}
}

func markdownPanel(id int, title, content string, x, y, w, h int) map[string]any {
	return map[string]any{
		"gridPos":       map[string]any{"h": h, "w": w, "x": x, "y": y},
		"id":            id,
		"options":       map[string]any{"code": map[string]any{"language": "plaintext", "showLineNumbers": false, "showMiniMap": false}, "content": content, "mode": "markdown"},
		"pluginVersion": "13.0.1",
		"title":         title,
		"type":          "text",
	}
}

func alertTablePanel(id, x, y, w, h int) map[string]any {
	t := target(panelTarget{expr: `ALERTS{cluster="deepseek-v4-flash",alertstate="firing"}`, legend: "{{severity}} · {{alertname}} · {{host}}", refID: "A"}, true)
	t["format"] = "table"
	return map[string]any{
		"datasource":    datasource,
		"description":   "Prometheus alert rules currently firing. An empty table is healthy; Alertmanager notifications are intentionally not configured here.",
		"fieldConfig":   map[string]any{"defaults": map[string]any{"custom": map[string]any{"align": "auto", "cellOptions": map[string]any{"type": "auto"}, "inspect": false}}, "overrides": []any{}},
		"gridPos":       map[string]any{"h": h, "w": w, "x": x, "y": y},
		"id":            id,
		"options":       map[string]any{"cellHeight": "sm", "footer": map[string]any{"countRows": false, "enablePagination": false, "fields": "", "reducer": []any{"sum"}, "show": false}, "showHeader": true, "sortBy": []any{}},
		"pluginVersion": "13.0.1",
		"targets":       []any{t},
		"title":         "Active alerts",
		"type":          "table",
	}
}

func textPanel(id, x, y, w, h int) map[string]any {
	content := `### How to read this dashboard

- **Logical input** counts every prompt token presented to the model, including cache hits.
- **New prefill** is the prompt work actually computed on the GB10 pair. **Cached input** is reused prefix-cache work.
- **Decode** is generated output throughput. Every tok/s view is a rate-window average using Grafana's dynamic rate interval.
- **Process lifetime** is the current vLLM process counter and resets with the engine. Hourly and selected-range totals are reset-aware after Prometheus began scraping.
- GB10 uses **unified memory**. Host RAM and the Yokai “VRAM” fallback describe the same physical pool and must not be added together.
- Native metrics have model/engine labels, not user or session identity. Per-user accounting requires bounded-label gateway instrumentation.`
	return markdownPanel(id, "Metric definitions and scope", content, x, y, w, h)
}

func selectedPromptTokensExpr() string {
	return `sum(increase(vllm:prompt_tokens_total{` + clusterMatch + `,` + modelMatch + `}[$__range]))`
}

func selectedOutputTokensExpr() string {
	return `sum(increase(vllm:generation_tokens_total{` + clusterMatch + `,` + modelMatch + `}[$__range]))`
}

func apiCostExpr(comparator apiComparator) string {
	return fmt.Sprintf(`((%s) * %g + (%s) * %g) / 1000000`, selectedPromptTokensExpr(), comparator.inputPrice, selectedOutputTokensExpr(), comparator.outputPrice)
}

func gpuEnergyExpr(window string) string {
	return fmt.Sprintf(`sum(sum_over_time(yokai_gpu_power_draw_watts{%s}[%s])) * %d / 3600000`, gpuPowerMatch, window, gpuPowerScrapeSeconds)
}

func gpuElectricityCostExpr(window string) string {
	return `(` + gpuEnergyExpr(window) + `) * ` + electricityRateVariable
}

func powerCoverageExpr() string {
	return fmt.Sprintf(`clamp_max(100 * sum(count_over_time(yokai_gpu_power_draw_watts{%s}[$__range])) / (2 * $__range_s / %d), 100)`, gpuPowerMatch, gpuPowerScrapeSeconds)
}

func projectedDailyGPUElectricityCostExpr() string {
	projection := `sum(avg_over_time(yokai_gpu_power_draw_watts{` + gpuPowerMatch + `}[$__range])) * 24 / 1000 * ` + electricityRateVariable
	return `(` + projection + `) and on() ((` + powerCoverageExpr() + `) >= 99)`
}

func labeledValue(expr, labelName, labelValue string) string {
	return fmt.Sprintf(`label_replace((%s), %q, %q, "", "")`, expr, labelName, labelValue)
}

func apiCostComparisonExpr() string {
	expr := labeledValue(gpuElectricityCostExpr("$__range"), "comparison", "Observed GPU electricity · includes idle")
	for _, comparator := range apiComparators {
		expr += ` or ` + labeledValue(apiCostExpr(comparator), "comparison", comparator.label)
	}
	return expr
}

func intelligenceComparisonExpr() string {
	expr := ""
	for index, comparator := range apiComparators {
		if index > 0 {
			expr += ` or `
		}
		expr += labeledValue(fmt.Sprintf("vector(%d)", comparator.intelligence), "comparison", comparator.label)
	}
	return expr
}

func costNotesPanel(id, x, y, w, h int) map[string]any {
	content := `### What this comparison means

- **Intelligence band:** Artificial Analysis Intelligence Index v4.1.1 snapshot captured Aug 6, 2026. DeepSeek V4 Flash 0731 max scores 52; Gemini 3.6 Flash high 52; GPT-5.6 Terra high 50; Claude Sonnet 5 max 55. Differences of up to three points should be treated as the same broad tier, not a task-level guarantee.
- **API equivalent:** selected-range prompt and output tokens are repriced at current first-party, standard **uncached** list rates. This is a token-for-token counterfactual; models differ in token use, cache eligibility, tools, and task success. Terra uses its base tier; prompts above 272K cost more. Sonnet 5 uses introductory pricing through Aug 31, 2026; refresh the price snapshot before Sep 1, 2026.
- **Local electricity:** all observed NVIDIA GPU-domain watts, including idle, integrated from five-second samples and multiplied by the editable electricity-rate assumption. It excludes CPU/SoC, memory, PSU loss, networking, cooling, fixed fees, and hardware amortization—so it is a lower bound, not total cost of ownership. Yokai currently publishes whole-watt readings.
- **North Houston fallback:** the default **0.1615 USD/kWh** is the EIA Texas statewide residential year-to-date average through May 2026. It is not a North Houston tariff. Replace it with the variable portion of your actual plan or bill for a better estimate.

[Artificial Analysis · DeepSeek](https://artificialanalysis.ai/models/deepseek-v4-flash) · [Gemini](https://artificialanalysis.ai/models/gemini-3-6-flash) · [Terra](https://artificialanalysis.ai/models/gpt-5-6-terra-high) · [Sonnet](https://artificialanalysis.ai/models/claude-sonnet-5)

[Pricing · DeepSeek](https://api-docs.deepseek.com/quick_start/pricing) · [Google](https://ai.google.dev/gemini-api/docs/pricing) · [OpenAI](https://developers.openai.com/api/docs/models/gpt-5.6-terra) · [Anthropic](https://platform.claude.com/docs/en/about-claude/pricing) · [EIA Texas electricity](https://www.eia.gov/electricity/monthly/epm_table_grapher.php?t=epmt_5_06_b)`
	return markdownPanel(id, "Assumptions, limitations, and sources", content, x, y, w, h)
}

func buildDashboard() map[string]any {
	green := statStyle{unit: "short", decimals: 0, baseColor: "green"}
	count := statStyle{unit: "locale", decimals: 0, baseColor: "blue"}
	tokensPerSecond := statStyle{unit: "suffix: tok/s", decimals: 1, baseColor: "blue", min: number(0)}
	percentHighBad := statStyle{unit: "percent", decimals: 1, baseColor: "green", warnAt: number(85), criticalAt: number(95), min: number(0), max: number(100)}
	seconds := statStyle{unit: "s", decimals: 2, baseColor: "blue", min: number(0)}

	panels := []any{
		rowPanel(1, "01 · Model pulse", 0),
		statPanel(2, "Telemetry ready", "One only when all five expected model, Yokai-agent, and node-exporter targets are present and healthy.", 0, 1, 3, panelTarget{expr: `(count(up{` + clusterMatch + `,job=~"deepseek-vllm|yokai-agent-.*|node"}) == bool 5) * (sum(up{` + clusterMatch + `,job=~"deepseek-vllm|yokai-agent-.*|node"}) == bool 5)`, legend: "Ready", refID: "A"}, statStyle{unit: "short", decimals: 0, baseColor: "green", warnAt: number(1), criticalAt: number(1), lowIsBad: true, min: number(0), max: number(1)}),
		statPanel(3, "Requests running", "Requests currently executing in the distributed vLLM engine.", 3, 1, 3, panelTarget{expr: `sum(vllm:num_requests_running{` + clusterMatch + `,` + modelMatch + `})`, legend: "Running", refID: "A"}, green),
		statPanel(4, "Queue depth", "Requests currently waiting for scheduling capacity or a transient engine constraint.", 6, 1, 3, panelTarget{expr: `sum(vllm:num_requests_waiting{` + clusterMatch + `,` + modelMatch + `})`, legend: "Waiting", refID: "A"}, statStyle{unit: "short", decimals: 0, baseColor: "green", warnAt: number(1), criticalAt: number(3), min: number(0)}),
		statPanel(5, "Decode throughput", "Generated output tokens per second across the distributed engine.", 9, 1, 3, panelTarget{expr: `sum(rate(vllm:generation_tokens_total{` + clusterMatch + `,` + modelMatch + `}[$__rate_interval]))`, legend: "Output", refID: "A"}, tokensPerSecond),
		statPanel(6, "New prefill throughput", "Only prompt tokens locally computed; cached prefix tokens are excluded.", 12, 1, 3, panelTarget{expr: `sum(rate(vllm:prompt_tokens_by_source_total{` + clusterMatch + `,` + modelMatch + `,source="local_compute"}[$__rate_interval]))`, legend: "New prefill", refID: "A"}, tokensPerSecond),
		statPanel(7, "Cached input throughput", "Prompt tokens served by the local prefix cache rather than recomputed.", 15, 1, 3, panelTarget{expr: `sum(rate(vllm:prompt_tokens_by_source_total{` + clusterMatch + `,` + modelMatch + `,source="local_cache_hit"}[$__rate_interval]))`, legend: "Cached input", refID: "A"}, tokensPerSecond),
		statPanel(8, "KV cache used", "Current vLLM KV-cache allocation ratio.", 18, 1, 3, panelTarget{expr: `100 * max(vllm:kv_cache_usage_perc{` + clusterMatch + `,` + modelMatch + `})`, legend: "KV cache", refID: "A"}, percentHighBad),
		statPanel(9, "Max unified memory", "Highest host unified-memory utilization across Beskar and Kyber.", 21, 1, 3, panelTarget{expr: `100 * max(deepseek:host_memory_used_ratio{` + hostMatch + `})`, legend: "Memory", refID: "A"}, percentHighBad),

		rowPanel(20, "02 · Tokens, requests, and throughput", 5),
		statPanel(21, "Tokens · last hour", "Logical prompt plus generated tokens observed in the rolling last hour.", 0, 6, 4, panelTarget{expr: `sum(increase(vllm:prompt_tokens_total{` + clusterMatch + `,` + modelMatch + `}[1h])) + sum(increase(vllm:generation_tokens_total{` + clusterMatch + `,` + modelMatch + `}[1h]))`, legend: "Tokens", refID: "A"}, count),
		statPanel(22, "Tokens · selected range", "Reset-aware logical prompt plus generated tokens in the selected dashboard window.", 4, 6, 4, panelTarget{expr: `sum(increase(vllm:prompt_tokens_total{` + clusterMatch + `,` + modelMatch + `}[$__range])) + sum(increase(vllm:generation_tokens_total{` + clusterMatch + `,` + modelMatch + `}[$__range]))`, legend: "Tokens", refID: "A"}, count),
		statPanel(23, "Tokens · process lifetime", "Current vLLM process counters. This includes activity before Prometheus began scraping, but resets when vLLM restarts.", 8, 6, 4, panelTarget{expr: `sum(vllm:prompt_tokens_total{` + clusterMatch + `,` + modelMatch + `}) + sum(vllm:generation_tokens_total{` + clusterMatch + `,` + modelMatch + `})`, legend: "Tokens", refID: "A"}, count),
		statPanel(24, "Requests · last hour", "Completed vLLM requests in the rolling last hour, across all finish reasons.", 12, 6, 4, panelTarget{expr: `sum(increase(vllm:request_success_total{` + clusterMatch + `,` + modelMatch + `}[1h]))`, legend: "Requests", refID: "A"}, count),
		statPanel(25, "Avg prompt · selected", "Logical prompt tokens divided by completed requests in the selected range.", 16, 6, 4, panelTarget{expr: `sum(increase(vllm:request_prompt_tokens_sum{` + clusterMatch + `,` + modelMatch + `}[$__range])) / clamp_min(sum(increase(vllm:request_prompt_tokens_count{` + clusterMatch + `,` + modelMatch + `}[$__range])), 1)`, legend: "Prompt tokens/request", refID: "A"}, count),
		statPanel(26, "Avg output · selected", "Generated tokens divided by completed requests in the selected range.", 20, 6, 4, panelTarget{expr: `sum(increase(vllm:request_generation_tokens_sum{` + clusterMatch + `,` + modelMatch + `}[$__range])) / clamp_min(sum(increase(vllm:request_generation_tokens_count{` + clusterMatch + `,` + modelMatch + `}[$__range])), 1)`, legend: "Output tokens/request", refID: "A"}, count),
		timeSeriesPanel(27, "Token throughput", "Logical input, newly computed prefill, cache-reused input, and generated output. Rate windows follow the dashboard interval.", 0, 10, 12, 9, []panelTarget{
			{expr: `sum(rate(vllm:prompt_tokens_total{` + clusterMatch + `,` + modelMatch + `}[$__rate_interval]))`, legend: "Logical input", refID: "A", color: "light-blue"},
			{expr: `sum(rate(vllm:prompt_tokens_by_source_total{` + clusterMatch + `,` + modelMatch + `,source="local_compute"}[$__rate_interval]))`, legend: "New prefill", refID: "B", color: "blue"},
			{expr: `sum(rate(vllm:prompt_tokens_by_source_total{` + clusterMatch + `,` + modelMatch + `,source="local_cache_hit"}[$__rate_interval]))`, legend: "Cached input", refID: "C", color: "purple"},
			{expr: `sum(rate(vllm:generation_tokens_total{` + clusterMatch + `,` + modelMatch + `}[$__rate_interval]))`, legend: "Generated output", refID: "D", color: "orange"},
		}, "suffix: tok/s", number(0), nil),
		hourlyTokenPanel(28, 12, 10, 12, 9),
		timeSeriesPanel(29, "Concurrency and queue", "Current executing and waiting requests. Waiting is split by reason where the runtime exposes it.", 0, 19, 12, 8, []panelTarget{
			{expr: `sum(vllm:num_requests_running{` + clusterMatch + `,` + modelMatch + `})`, legend: "Running", refID: "A", color: "blue"},
			{expr: `sum(vllm:num_requests_waiting_by_reason{` + clusterMatch + `,` + modelMatch + `,reason="capacity"})`, legend: "Waiting · capacity", refID: "B", color: "orange"},
			{expr: `sum(vllm:num_requests_waiting_by_reason{` + clusterMatch + `,` + modelMatch + `,reason="deferred"})`, legend: "Waiting · deferred", refID: "C", color: "purple"},
		}, "short", number(0), nil),
		piePanel(30, "Prompt source mix", "Selected-range logical input split into newly computed tokens, local cache hits, and external KV transfer.", 12, 19, 6, 8, []panelTarget{
			{expr: `sum(increase(vllm:prompt_tokens_by_source_total{` + clusterMatch + `,` + modelMatch + `,source="local_compute"}[$__range]))`, legend: "New prefill", refID: "A", color: "blue"},
			{expr: `sum(increase(vllm:prompt_tokens_by_source_total{` + clusterMatch + `,` + modelMatch + `,source="local_cache_hit"}[$__range]))`, legend: "Local cache", refID: "B", color: "purple"},
			{expr: `sum(increase(vllm:prompt_tokens_by_source_total{` + clusterMatch + `,` + modelMatch + `,source="external_kv_transfer"}[$__range]))`, legend: "External KV", refID: "C", color: "yellow"},
		}, "locale"),
		barGaugePanel(31, "Request outcomes", "Completed requests in the selected range, grouped by vLLM finish reason.", 18, 19, 6, 8, panelTarget{expr: `sum by (finished_reason) (increase(vllm:request_success_total{` + clusterMatch + `,` + modelMatch + `}[$__range]))`, legend: "{{finished_reason}}", refID: "A"}, "locale", 0, nil),

		rowPanel(40, "03 · Latency and user experience", 27),
		statPanel(41, "TTFT p95", "95th-percentile time to first token over the selected range of completed requests.", 0, 28, 4, panelTarget{expr: `histogram_quantile(0.95, sum by (le) (increase(vllm:time_to_first_token_seconds_bucket{` + clusterMatch + `,` + modelMatch + `}[$__range])))`, legend: "TTFT p95", refID: "A"}, seconds),
		statPanel(42, "TPOT p95", "95th-percentile per-request time per output token over the selected range.", 4, 28, 4, panelTarget{expr: `histogram_quantile(0.95, sum by (le) (increase(vllm:request_time_per_output_token_seconds_bucket{` + clusterMatch + `,` + modelMatch + `}[$__range])))`, legend: "TPOT p95", refID: "A"}, seconds),
		statPanel(43, "E2E p95", "95th-percentile end-to-end vLLM request latency over the selected range.", 8, 28, 4, panelTarget{expr: `histogram_quantile(0.95, sum by (le) (increase(vllm:e2e_request_latency_seconds_bucket{` + clusterMatch + `,` + modelMatch + `}[$__range])))`, legend: "E2E p95", refID: "A"}, seconds),
		statPanel(44, "Failure share", "Errors and aborts divided by all completed requests in the selected range.", 12, 28, 4, panelTarget{expr: `100 * sum(increase(vllm:request_success_total{` + clusterMatch + `,` + modelMatch + `,finished_reason=~"error|abort"}[$__range])) / clamp_min(sum(increase(vllm:request_success_total{` + clusterMatch + `,` + modelMatch + `}[$__range])), 1)`, legend: "Failure", refID: "A"}, statStyle{unit: "percent", decimals: 2, baseColor: "green", warnAt: number(1), criticalAt: number(5), min: number(0), max: number(100)}),
		statPanel(45, "Request rate", "Completed vLLM requests per second over the dynamic rate interval.", 16, 28, 4, panelTarget{expr: `sum(rate(vllm:request_success_total{` + clusterMatch + `,` + modelMatch + `}[$__rate_interval]))`, legend: "Requests/s", refID: "A"}, statStyle{unit: "reqps", decimals: 3, baseColor: "blue", min: number(0)}),
		statPanel(46, "Preemptions · 24h", "Engine preemptions in the rolling last 24 hours.", 20, 28, 4, panelTarget{expr: `sum(increase(vllm:num_preemptions_total{` + clusterMatch + `,` + modelMatch + `}[24h]))`, legend: "Preemptions", refID: "A"}, statStyle{unit: "short", decimals: 0, baseColor: "green", warnAt: number(1), criticalAt: number(5), min: number(0)}),
		timeSeriesPanel(47, "Time to first token", "TTFT percentiles from completed requests. Histogram rates use the dashboard's dynamic rate interval.", 0, 32, 12, 9, []panelTarget{
			{expr: `histogram_quantile(0.50, sum by (le) (rate(vllm:time_to_first_token_seconds_bucket{` + clusterMatch + `,` + modelMatch + `}[$__rate_interval])))`, legend: "p50", refID: "A", color: "light-blue"},
			{expr: `histogram_quantile(0.95, sum by (le) (rate(vllm:time_to_first_token_seconds_bucket{` + clusterMatch + `,` + modelMatch + `}[$__rate_interval])))`, legend: "p95", refID: "B", color: "blue"},
			{expr: `histogram_quantile(0.99, sum by (le) (rate(vllm:time_to_first_token_seconds_bucket{` + clusterMatch + `,` + modelMatch + `}[$__rate_interval])))`, legend: "p99", refID: "C", color: "orange"},
		}, "s", number(0), nil),
		timeSeriesPanel(48, "Inter-token latency and TPOT", "ITL measures time between output events; speculative decoding can emit several accepted tokens per event. TPOT is the per-request user-experience complement.", 12, 32, 12, 9, []panelTarget{
			{expr: `histogram_quantile(0.50, sum by (le) (rate(vllm:inter_token_latency_seconds_bucket{` + clusterMatch + `,` + modelMatch + `}[$__rate_interval])))`, legend: "ITL p50", refID: "A", color: "light-blue"},
			{expr: `histogram_quantile(0.95, sum by (le) (rate(vllm:inter_token_latency_seconds_bucket{` + clusterMatch + `,` + modelMatch + `}[$__rate_interval])))`, legend: "ITL p95", refID: "B", color: "blue"},
			{expr: `histogram_quantile(0.95, sum by (le) (rate(vllm:request_time_per_output_token_seconds_bucket{` + clusterMatch + `,` + modelMatch + `}[$__rate_interval])))`, legend: "TPOT p95", refID: "C", color: "orange"},
		}, "s", number(0), nil),
		timeSeriesPanel(49, "Request phase latency · p95", "Queue, prefill, and decode phase p95 for completed requests.", 0, 41, 12, 9, []panelTarget{
			{expr: `histogram_quantile(0.95, sum by (le) (rate(vllm:request_queue_time_seconds_bucket{` + clusterMatch + `,` + modelMatch + `}[$__rate_interval])))`, legend: "Queue", refID: "A", color: "purple"},
			{expr: `histogram_quantile(0.95, sum by (le) (rate(vllm:request_prefill_time_seconds_bucket{` + clusterMatch + `,` + modelMatch + `}[$__rate_interval])))`, legend: "Prefill", refID: "B", color: "blue"},
			{expr: `histogram_quantile(0.95, sum by (le) (rate(vllm:request_decode_time_seconds_bucket{` + clusterMatch + `,` + modelMatch + `}[$__rate_interval])))`, legend: "Decode", refID: "C", color: "orange"},
		}, "s", number(0), nil),
		timeSeriesPanel(50, "End-to-end request latency", "Completed-request end-to-end latency percentiles.", 12, 41, 12, 9, []panelTarget{
			{expr: `histogram_quantile(0.50, sum by (le) (rate(vllm:e2e_request_latency_seconds_bucket{` + clusterMatch + `,` + modelMatch + `}[$__rate_interval])))`, legend: "p50", refID: "A", color: "light-blue"},
			{expr: `histogram_quantile(0.95, sum by (le) (rate(vllm:e2e_request_latency_seconds_bucket{` + clusterMatch + `,` + modelMatch + `}[$__rate_interval])))`, legend: "p95", refID: "B", color: "blue"},
			{expr: `histogram_quantile(0.99, sum by (le) (rate(vllm:e2e_request_latency_seconds_bucket{` + clusterMatch + `,` + modelMatch + `}[$__rate_interval])))`, legend: "p99", refID: "C", color: "orange"},
		}, "s", number(0), nil),

		rowPanel(60, "04 · Cache, scheduler, and speculative decoding", 50),
		statPanel(61, "Prefix-cache hit rate", "Prefix-cache hit tokens divided by queried tokens over the dynamic rate interval.", 0, 51, 4, panelTarget{expr: `100 * sum(rate(vllm:prefix_cache_hits_total{` + clusterMatch + `,` + modelMatch + `}[$__rate_interval])) / clamp_min(sum(rate(vllm:prefix_cache_queries_total{` + clusterMatch + `,` + modelMatch + `}[$__rate_interval])), 1e-9)`, legend: "Hit rate", refID: "A"}, statStyle{unit: "percent", decimals: 1, baseColor: "blue", min: number(0), max: number(100)}),
		statPanel(62, "Speculative acceptance", "Accepted speculative tokens divided by all draft tokens over the dynamic rate interval.", 4, 51, 4, panelTarget{expr: `100 * sum(rate(vllm:spec_decode_num_accepted_tokens_total{` + clusterMatch + `,` + modelMatch + `}[$__rate_interval])) / clamp_min(sum(rate(vllm:spec_decode_num_draft_tokens_total{` + clusterMatch + `,` + modelMatch + `}[$__rate_interval])), 1e-9)`, legend: "Acceptance", refID: "A"}, statStyle{unit: "percent", decimals: 1, baseColor: "blue", min: number(0), max: number(100)}),
		statPanel(63, "Draft tokens · selected", "Speculative draft tokens proposed in the selected dashboard range.", 8, 51, 4, panelTarget{expr: `sum(increase(vllm:spec_decode_num_draft_tokens_total{` + clusterMatch + `,` + modelMatch + `}[$__range]))`, legend: "Draft", refID: "A"}, count),
		statPanel(64, "Accepted tokens · selected", "Speculative draft tokens accepted in the selected dashboard range.", 12, 51, 4, panelTarget{expr: `sum(increase(vllm:spec_decode_num_accepted_tokens_total{` + clusterMatch + `,` + modelMatch + `}[$__range]))`, legend: "Accepted", refID: "A"}, count),
		statPanel(65, "Computed prefill / request", "Average uncached KV tokens computed per completed request in the selected range.", 16, 51, 4, panelTarget{expr: `sum(increase(vllm:request_prefill_kv_computed_tokens_sum{` + clusterMatch + `,` + modelMatch + `}[$__range])) / clamp_min(sum(increase(vllm:request_prefill_kv_computed_tokens_count{` + clusterMatch + `,` + modelMatch + `}[$__range])), 1)`, legend: "Computed tokens", refID: "A"}, count),
		statPanel(66, "HTTP errors · selected", "Non-2xx inference API responses in the selected dashboard range, including failures before engine admission.", 20, 51, 4, panelTarget{expr: `sum(increase(http_requests_total{` + clusterMatch + `,handler=~"/v1/(chat/completions|completions|responses)",status!~"2xx"}[$__range])) or vector(0)`, legend: "HTTP errors", refID: "A"}, statStyle{unit: "short", decimals: 0, baseColor: "green", warnAt: number(1), criticalAt: number(5), min: number(0)}),
		timeSeriesPanel(67, "KV-cache and prefix-cache effectiveness", "Current KV allocation and rolling prefix-cache hit ratio. Percentages share one axis.", 0, 55, 12, 9, []panelTarget{
			{expr: `100 * max(vllm:kv_cache_usage_perc{` + clusterMatch + `,` + modelMatch + `})`, legend: "KV cache used", refID: "A", color: "orange"},
			{expr: `100 * sum(rate(vllm:prefix_cache_hits_total{` + clusterMatch + `,` + modelMatch + `}[$__rate_interval])) / clamp_min(sum(rate(vllm:prefix_cache_queries_total{` + clusterMatch + `,` + modelMatch + `}[$__rate_interval])), 1e-9)`, legend: "Prefix hit rate", refID: "B", color: "blue"},
		}, "percent", number(0), number(100)),
		timeSeriesPanel(68, "Speculative decoding flow", "Draft and accepted token rates; the acceptance ratio is summarized in the stat above.", 12, 55, 12, 9, []panelTarget{
			{expr: `sum(rate(vllm:spec_decode_num_draft_tokens_total{` + clusterMatch + `,` + modelMatch + `}[$__rate_interval]))`, legend: "Draft tok/s", refID: "A", color: "light-blue"},
			{expr: `sum(rate(vllm:spec_decode_num_accepted_tokens_total{` + clusterMatch + `,` + modelMatch + `}[$__rate_interval]))`, legend: "Accepted tok/s", refID: "B", color: "blue"},
		}, "suffix: tok/s", number(0), nil),

		rowPanel(80, "05 · GB10 unified memory and host pressure", 64),
		statPanel(81, "Beskar memory", "Host memory utilization on worker rank 1. GB10 shares this pool with GPU allocations.", 0, 65, 4, panelTarget{expr: `100 * deepseek:host_memory_used_ratio{host="beskar"}`, legend: "Beskar", refID: "A"}, percentHighBad),
		statPanel(82, "Kyber memory", "Host memory utilization on API rank 0. GB10 shares this pool with GPU allocations.", 4, 65, 4, panelTarget{expr: `100 * deepseek:host_memory_used_ratio{host="kyber"}`, legend: "Kyber", refID: "A"}, percentHighBad),
		statPanel(83, "Swap used · max host", "Largest swap allocation across the selected hosts.", 8, 65, 4, panelTarget{expr: `max(node_memory_SwapTotal_bytes{` + clusterMatch + `,` + hostMatch + `} - node_memory_SwapFree_bytes{` + clusterMatch + `,` + hostMatch + `})`, legend: "Swap", refID: "A"}, statStyle{unit: "bytes", decimals: 1, baseColor: "orange", min: number(0)}),
		statPanel(84, "Memory PSI some · max", "Highest fraction of wall time with at least one task stalled on memory.", 12, 65, 4, panelTarget{expr: `100 * max(rate(node_pressure_memory_waiting_seconds_total{` + clusterMatch + `,` + hostMatch + `}[$__rate_interval]))`, legend: "PSI some", refID: "A"}, statStyle{unit: "percent", decimals: 2, baseColor: "green", warnAt: number(2), criticalAt: number(10), min: number(0), max: number(100)}),
		statPanel(85, "OOM kills · selected", "Kernel out-of-memory kills across both hosts in the selected range.", 16, 65, 4, panelTarget{expr: `sum(increase(node_vmstat_oom_kill{` + clusterMatch + `,` + hostMatch + `}[$__range]))`, legend: "OOM kills", refID: "A"}, statStyle{unit: "short", decimals: 0, baseColor: "green", warnAt: number(1), criticalAt: number(2), min: number(0)}),
		statPanel(86, "Major faults / s", "Current major page-fault rate across the selected hosts.", 20, 65, 4, panelTarget{expr: `sum(rate(node_vmstat_pgmajfault{` + clusterMatch + `,` + hostMatch + `}[$__rate_interval]))`, legend: "Major faults", refID: "A"}, statStyle{unit: "ops", decimals: 2, baseColor: "green", warnAt: number(10), criticalAt: number(100), min: number(0)}),
		timeSeriesPanel(87, "Unified-memory utilization", "Host available-memory calculation and Yokai's GB10 unified-memory fallback. These are two views of the same physical pool.", 0, 69, 12, 9, []panelTarget{
			{expr: `100 * deepseek:host_memory_used_ratio{` + hostMatch + `}`, legend: "{{host}} · host", refID: "A", color: "blue"},
			{expr: `100 * deepseek:unified_memory_used_ratio{` + hostMatch + `}`, legend: "{{host}} · GPU view", refID: "B", color: "orange"},
		}, "percent", number(0), number(100)),
		timeSeriesPanel(88, "Available memory and swap", "Available unified memory and swap allocation by host.", 12, 69, 12, 9, []panelTarget{
			{expr: `node_memory_MemAvailable_bytes{` + clusterMatch + `,` + hostMatch + `}`, legend: "{{host}} · available", refID: "A", color: "blue"},
			{expr: `node_memory_SwapTotal_bytes{` + clusterMatch + `,` + hostMatch + `} - node_memory_SwapFree_bytes{` + clusterMatch + `,` + hostMatch + `}`, legend: "{{host}} · swap used", refID: "B", color: "orange"},
		}, "bytes", number(0), nil),
		timeSeriesPanel(89, "Memory pressure stalls", "PSI some means at least one task stalled; PSI full means all non-idle tasks stalled.", 0, 78, 12, 9, []panelTarget{
			{expr: `100 * rate(node_pressure_memory_waiting_seconds_total{` + clusterMatch + `,` + hostMatch + `}[$__rate_interval])`, legend: "{{host}} · some", refID: "A", color: "orange"},
			{expr: `100 * rate(node_pressure_memory_stalled_seconds_total{` + clusterMatch + `,` + hostMatch + `}[$__rate_interval])`, legend: "{{host}} · full", refID: "B", color: "purple"},
		}, "percent", number(0), number(100)),
		timeSeriesPanel(90, "CPU utilization and load", "CPU busy percentage and one-minute load normalized by logical CPU count.", 12, 78, 12, 9, []panelTarget{
			{expr: `100 * (1 - avg by (host) (rate(node_cpu_seconds_total{` + clusterMatch + `,` + hostMatch + `,mode="idle"}[$__rate_interval])))`, legend: "{{host}} · CPU", refID: "A", color: "blue"},
			{expr: `100 * node_load1{` + clusterMatch + `,` + hostMatch + `} / count by (host) (node_cpu_seconds_total{` + clusterMatch + `,` + hostMatch + `,mode="idle"})`, legend: "{{host}} · load / CPU", refID: "B", color: "orange"},
		}, "percent", number(0), nil),

		rowPanel(100, "06 · GPU, thermals, power, and efficiency", 87),
		statPanel(101, "Cluster GPU power", "Combined NVIDIA-reported GPU-domain power across both workers; this is not wall-system power.", 0, 88, 4, panelTarget{expr: `sum(yokai_gpu_power_draw_watts{` + gpuPowerMatch + `})`, legend: "GPU power", refID: "A"}, statStyle{unit: "watt", decimals: 1, baseColor: "blue", min: number(0)}),
		statPanel(102, "GPU output efficiency", "Generated output tok/s divided by reported GPU-domain watts: output tokens per GPU joule, not whole-system efficiency.", 4, 88, 4, panelTarget{expr: `sum(rate(vllm:generation_tokens_total{` + clusterMatch + `,` + modelMatch + `}[$__rate_interval])) / clamp_min(sum(yokai_gpu_power_draw_watts{` + gpuPowerMatch + `}), 1)`, legend: "Efficiency", refID: "A"}, statStyle{unit: "suffix: tok/GPU J", decimals: 3, baseColor: "blue", min: number(0)}),
		statPanel(103, "Max GB10 temperature", "Highest reported GB10 temperature across the selected hosts.", 8, 88, 4, panelTarget{expr: `max(yokai_gpu_temperature_celsius{` + clusterMatch + `,` + hostMatch + `})`, legend: "Temperature", refID: "A"}, statStyle{unit: "celsius", decimals: 0, baseColor: "green", warnAt: number(75), criticalAt: number(85), min: number(0)}),
		statPanel(104, "Max GPU utilization", "Highest current GPU utilization across the two GB10 workers.", 12, 88, 4, panelTarget{expr: `max(yokai_gpu_utilization{` + clusterMatch + `,` + hostMatch + `})`, legend: "GPU", refID: "A"}, statStyle{unit: "percent", decimals: 1, baseColor: "blue", min: number(0), max: number(100)}),
		statPanel(105, "Model process uptime", "Seconds since the vLLM API process started on Kyber.", 16, 88, 4, panelTarget{expr: `time() - max(process_start_time_seconds{` + clusterMatch + `,job="deepseek-vllm"})`, legend: "Uptime", refID: "A"}, statStyle{unit: "s", decimals: 0, baseColor: "blue", min: number(0)}),
		statPanel(106, "Active Prometheus alerts", "Number of currently firing DeepSeek observability alerts.", 20, 88, 4, panelTarget{expr: `count(ALERTS{` + clusterMatch + `,alertstate="firing"}) or vector(0)`, legend: "Alerts", refID: "A"}, statStyle{unit: "short", decimals: 0, baseColor: "green", warnAt: number(1), criticalAt: number(2), min: number(0)}),
		timeSeriesPanel(107, "GPU utilization", "Native Yokai/nvidia-smi utilization by GB10 host.", 0, 92, 8, 9, []panelTarget{{expr: `yokai_gpu_utilization{` + clusterMatch + `,` + hostMatch + `}`, legend: "{{host}}", refID: "A", color: "blue"}}, "percent", number(0), number(100)),
		timeSeriesPanel(108, "GPU temperature", "GB10 temperature by host.", 8, 92, 8, 9, []panelTarget{{expr: `yokai_gpu_temperature_celsius{` + clusterMatch + `,` + hostMatch + `}`, legend: "{{host}}", refID: "A", color: "orange"}}, "celsius", number(0), nil),
		timeSeriesPanel(109, "GPU power draw", "NVIDIA-reported GPU-domain power by host; excludes the rest of each system and power-conversion losses.", 16, 92, 8, 9, []panelTarget{{expr: `yokai_gpu_power_draw_watts{` + clusterMatch + `,` + hostMatch + `}`, legend: "{{host}}", refID: "A", color: "purple"}}, "watt", number(0), nil),
		statPanel(110, "GPU energy · selected", "Observed five-second GPU-domain power samples integrated over the selected range. Missed samples undercount.", 0, 101, 6, panelTarget{expr: gpuEnergyExpr("$__range"), legend: "GPU energy", refID: "A"}, statStyle{unit: "suffix: kWh", decimals: 3, baseColor: "blue", min: number(0)}),
		statPanel(111, "GPU electricity · selected", "Selected-range GPU-domain energy multiplied by the dashboard electricity-rate assumption.", 6, 101, 6, panelTarget{expr: gpuElectricityCostExpr("$__range"), legend: "GPU electricity", refID: "A"}, statStyle{unit: "currencyUSD", decimals: 3, baseColor: "green", min: number(0)}),
		statPanel(112, "Power telemetry coverage", "Observed five-second samples divided by the two-host sample count expected in the selected range.", 12, 101, 6, panelTarget{expr: powerCoverageExpr(), legend: "Coverage", refID: "A"}, statStyle{unit: "percent", decimals: 1, baseColor: "green", warnAt: number(95), criticalAt: number(99), lowIsBad: true, min: number(0), max: number(100)}),
		statPanel(113, "Projected GPU cost / day", "Available selected-range GPU-domain samples projected across 24 hours at the configured electricity rate. Requires at least 99% telemetry coverage.", 18, 101, 6, panelTarget{expr: projectedDailyGPUElectricityCostExpr(), legend: "Daily GPU electricity", refID: "A"}, statStyle{unit: "currencyUSD", decimals: 2, baseColor: "blue", min: number(0)}),
		timeSeriesPanel(114, "GPU energy · rolling hour", "Observed GPU-domain energy from five-second samples in each rolling hour, by host.", 0, 105, 12, 9, []panelTarget{{expr: fmt.Sprintf(`sum by (host) (sum_over_time(yokai_gpu_power_draw_watts{%s}[1h])) * %d / 3600000`, gpuPowerMatch, gpuPowerScrapeSeconds), legend: "{{host}}", refID: "A", color: "purple"}}, "suffix: kWh", number(0), nil),
		timeSeriesPanel(115, "GPU electricity · rolling hour", "Rolling-hour GPU-domain energy multiplied by the configured electricity rate, by host.", 12, 105, 12, 9, []panelTarget{{expr: fmt.Sprintf(`sum by (host) (sum_over_time(yokai_gpu_power_draw_watts{%s}[1h])) * %d / 3600000 * %s`, gpuPowerMatch, gpuPowerScrapeSeconds, electricityRateVariable), legend: "{{host}}", refID: "A", color: "green"}}, "currencyUSD", number(0), nil),

		rowPanel(130, "07 · Cost compare · local vs comparable APIs", 114),
		statPanel(131, "AA Index · upstream 0731", "Artificial Analysis Intelligence Index v4.1.1 score for the upstream DeepSeek V4 Flash 0731 checkpoint at max effort, captured Aug 6, 2026; local quantization is not independently re-benchmarked.", 0, 115, 4, panelTarget{expr: `vector(52)`, legend: "AA Index", refID: "A"}, statStyle{unit: "short", decimals: 0, baseColor: "blue", min: number(0)}),
		statPanel(132, "Observed GPU electricity", "Selected-range GPU-domain electricity estimate, including idle; excludes whole-system power and hardware cost.", 4, 115, 4, panelTarget{expr: gpuElectricityCostExpr("$__range"), legend: "Local", refID: "A"}, statStyle{unit: "currencyUSD", decimals: 3, baseColor: "green", min: number(0)}),
		statPanel(133, "DeepSeek API equivalent", "Selected-range token mix at DeepSeek V4 Flash standard uncached list prices: 0.14 USD/M input and 0.28 USD/M output.", 8, 115, 4, panelTarget{expr: apiCostExpr(apiComparators[0]), legend: "DeepSeek API", refID: "A"}, statStyle{unit: "currencyUSD", decimals: 2, baseColor: "blue", min: number(0)}),
		statPanel(134, "Gemini 3.6 equivalent", "Selected-range token mix at Gemini 3.6 Flash standard uncached list prices: 1.50 USD/M input and 7.50 USD/M output.", 12, 115, 4, panelTarget{expr: apiCostExpr(apiComparators[1]), legend: "Gemini API", refID: "A"}, statStyle{unit: "currencyUSD", decimals: 2, baseColor: "blue", min: number(0)}),
		statPanel(135, "GPT-5.6 Terra equivalent", "Selected-range token mix at Terra base-tier standard uncached list prices: 2.00 USD/M input and 12.00 USD/M output.", 16, 115, 4, panelTarget{expr: apiCostExpr(apiComparators[2]), legend: "OpenAI API", refID: "A"}, statStyle{unit: "currencyUSD", decimals: 2, baseColor: "blue", min: number(0)}),
		statPanel(136, "Claude Sonnet 5 equivalent", "Selected-range token mix at Sonnet 5 introductory uncached list prices through Aug 31, 2026: 2.00 USD/M input and 10.00 USD/M output.", 20, 115, 4, panelTarget{expr: apiCostExpr(apiComparators[3]), legend: "Claude API", refID: "A"}, statStyle{unit: "currencyUSD", decimals: 2, baseColor: "blue", min: number(0)}),
		barGaugePanel(137, "Selected-range variable-cost comparison", "Observed GPU-domain electricity, including idle, alongside token-for-token standard uncached API list-price equivalents.", 0, 119, 12, 9, panelTarget{expr: apiCostComparisonExpr(), legend: "{{comparison}}", refID: "A"}, "currencyUSD", 3, nil),
		barGaugePanel(138, "Artificial Analysis intelligence band", "Independent AA Intelligence Index v4.1.1 snapshot captured Aug 6, 2026. Small gaps are not task-level guarantees; fixed 0–100 scale avoids exaggerating them.", 12, 119, 12, 9, panelTarget{expr: intelligenceComparisonExpr(), legend: "{{comparison}}", refID: "A"}, "short", 0, number(100)),
		costNotesPanel(139, 0, 128, 24, 8),

		rowPanel(120, "08 · Network, storage, and platform reliability", 136),
		timeSeriesPanel(121, "Network throughput", "Receive and transmit traffic by host, excluding loopback and virtual bridge interfaces.", 0, 137, 12, 9, []panelTarget{
			{expr: `sum by (host) (rate(node_network_receive_bytes_total{` + clusterMatch + `,` + hostMatch + `,device!~"lo|veth.*|docker.*|br-.*"}[$__rate_interval]))`, legend: "{{host}} · receive", refID: "A", color: "blue"},
			{expr: `sum by (host) (rate(node_network_transmit_bytes_total{` + clusterMatch + `,` + hostMatch + `,device!~"lo|veth.*|docker.*|br-.*"}[$__rate_interval]))`, legend: "{{host}} · transmit", refID: "B", color: "orange"},
		}, "Bps", number(0), nil),
		timeSeriesPanel(122, "Disk throughput", "Physical block-device read and write throughput by host.", 12, 137, 12, 9, []panelTarget{
			{expr: `sum by (host) (rate(node_disk_read_bytes_total{` + clusterMatch + `,` + hostMatch + `,device!~"loop.*|ram.*|dm-.*"}[$__rate_interval]))`, legend: "{{host}} · read", refID: "A", color: "blue"},
			{expr: `sum by (host) (rate(node_disk_written_bytes_total{` + clusterMatch + `,` + hostMatch + `,device!~"loop.*|ram.*|dm-.*"}[$__rate_interval]))`, legend: "{{host}} · write", refID: "B", color: "orange"},
		}, "Bps", number(0), nil),
		timeSeriesPanel(123, "Root filesystem free", "Available bytes for the root filesystem by host.", 0, 146, 12, 9, []panelTarget{
			{expr: `node_filesystem_avail_bytes{` + clusterMatch + `,` + hostMatch + `,mountpoint="/",fstype!="rootfs"}`, legend: "{{host}} · available", refID: "A", color: "blue"},
		}, "bytes", number(0), nil),
		timeSeriesPanel(124, "Target health and scrape latency", "Health is one when a target is reachable. Scrape duration exposes exporter or network slowdown.", 12, 146, 12, 9, []panelTarget{
			{expr: `up{` + clusterMatch + `}`, legend: "{{job}} · {{host}} · up", refID: "A", color: "blue"},
			{expr: `scrape_duration_seconds{` + clusterMatch + `}`, legend: "{{job}} · {{host}} · scrape s", refID: "B", color: "orange"},
		}, "short", number(0), nil),
		alertTablePanel(125, 0, 155, 12, 8),
		textPanel(126, 12, 155, 12, 8),
	}

	return map[string]any{
		"annotations": map[string]any{
			"list": []any{map[string]any{
				"builtIn":    1,
				"datasource": map[string]any{"type": "grafana", "uid": "-- Grafana --"},
				"enable":     true,
				"hide":       true,
				"iconColor":  "rgba(0, 211, 255, 1)",
				"name":       "Annotations & Alerts",
				"type":       "dashboard",
			}},
		},
		"description":          "DeepSeek-V4-Flash-0731 observability across the Beskar and Kyber GB10 cluster: tokens, latency, cache, scheduling, unified memory, thermals, GPU-domain power, cost comparison, and reliability.",
		"editable":             false,
		"fiscalYearStartMonth": 0,
		"graphTooltip":         1,
		"id":                   nil,
		"links": []any{
			map[string]any{"asDropdown": false, "icon": "external link", "includeVars": true, "keepTime": true, "tags": []any{}, "targetBlank": true, "title": "Prometheus targets", "tooltip": "Inspect scrape health", "type": "link", "url": "http://beskar:9090/targets"},
		},
		"liveNow":       true,
		"panels":        panels,
		"preload":       false,
		"refresh":       "5s",
		"schemaVersion": 41,
		"tags":          []any{"yokai", "deepseek", "vllm", "gb10", "beskar", "kyber"},
		"templating": map[string]any{
			"list": []any{
				map[string]any{
					"current":    map[string]any{"text": "All", "value": "$__all"},
					"datasource": datasource,
					"definition": `label_values(vllm:num_requests_running{cluster="deepseek-v4-flash"}, model_name)`,
					"includeAll": true,
					"allValue":   ".+",
					"label":      "Model",
					"name":       "model",
					"options":    []any{},
					"query":      map[string]any{"query": `label_values(vllm:num_requests_running{cluster="deepseek-v4-flash"}, model_name)`, "refId": "PrometheusVariableQueryEditor-VariableQuery"},
					"refresh":    1,
					"regex":      "",
					"sort":       1,
					"type":       "query",
				},
				map[string]any{
					"current":    map[string]any{"text": "All", "value": "$__all"},
					"datasource": datasource,
					"definition": `label_values(node_uname_info{cluster="deepseek-v4-flash"}, host)`,
					"includeAll": true,
					"allValue":   ".+",
					"label":      "Host",
					"name":       "host",
					"options":    []any{},
					"query":      map[string]any{"query": `label_values(node_uname_info{cluster="deepseek-v4-flash"}, host)`, "refId": "PrometheusVariableQueryEditor-VariableQuery"},
					"refresh":    1,
					"regex":      "",
					"sort":       1,
					"type":       "query",
				},
				map[string]any{
					"current": map[string]any{"selected": true, "text": "0.1615", "value": "0.1615"},
					"hide":    0,
					"label":   "Electricity rate · USD/kWh",
					"name":    "electricity_rate",
					"options": []any{},
					"query":   "0.1615",
					"type":    "textbox",
				},
			},
		},
		"time": map[string]any{"from": "now-6h", "to": "now"},
		"timepicker": map[string]any{
			"refresh_intervals": []any{"5s", "10s", "30s", "1m", "5m", "15m", "30m", "1h"},
			"time_options":      []any{"15m", "30m", "1h", "3h", "6h", "12h", "24h", "2d", "7d", "30d", "90d", "1y"},
		},
		"timezone":  "browser",
		"title":     "DeepSeek V4 Flash · Beskar + Kyber",
		"uid":       dashboardUID,
		"version":   1,
		"weekStart": "",
	}
}

func collectQueries(value any, queries map[string]struct{}) {
	switch item := value.(type) {
	case map[string]any:
		if expr, ok := item["expr"].(string); ok && expr != "" {
			queries[expr] = struct{}{}
		}
		for _, child := range item {
			collectQueries(child, queries)
		}
	case []any:
		for _, child := range item {
			collectQueries(child, queries)
		}
	}
}

func main() {
	queriesOnly := flag.Bool("queries", false, "emit the unique dashboard PromQL expressions as JSON")
	compact := flag.Bool("compact", false, "emit compact JSON")
	flag.Parse()

	dashboard := buildDashboard()
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetEscapeHTML(false)
	if !*compact {
		encoder.SetIndent("", "  ")
	}

	if *queriesOnly {
		set := make(map[string]struct{})
		collectQueries(dashboard, set)
		queries := make([]string, 0, len(set))
		for query := range set {
			queries = append(queries, query)
		}
		sort.Strings(queries)
		if err := encoder.Encode(queries); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}

	if err := encoder.Encode(dashboard); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
