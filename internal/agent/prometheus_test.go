package agent

import (
	"strings"
	"testing"
)

func TestRenderPrometheusMetricsIncludesNormalizedSGLangSeries(t *testing.T) {
	t.Parallel()

	metrics := &SystemMetrics{}
	containers := []Container{{
		Name:   "yokai-sglang-radixark-qwen3-8-27b-nvfp4",
		Image:  "lmsysorg/sglang@sha256:test",
		Status: "running",
		VLLMMetrics: &VLLMMetrics{
			Model:                    "Qwen3.8-27B",
			GenerationTokPerSec:      306.4,
			RequestsRunning:          2,
			GenerationTokensTotal:    2048,
			HasGenerationTokPerSec:   true,
			HasRequestsRunning:       true,
			HasGenerationTokensTotal: true,
		},
	}}

	got := renderPrometheusMetrics(metrics, containers)
	for _, want := range []string{
		`yokai_llm_decode_tokens_per_second{backend="sglang",model="Qwen3.8-27B",service="sglang-radixark-qwen3-8-27b-nvfp4"} 306.4`,
		`yokai_llm_requests_in_flight{backend="sglang",model="Qwen3.8-27B",service="sglang-radixark-qwen3-8-27b-nvfp4"} 2`,
		`yokai_llm_generated_tokens_total{backend="sglang",model="Qwen3.8-27B",service="sglang-radixark-qwen3-8-27b-nvfp4"} 2048`,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("expected normalized SGLang series %q in:\n%s", want, got)
		}
	}
}
