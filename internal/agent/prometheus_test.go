package agent

import (
	"strings"
	"testing"

	"github.com/spencerbull/yokai/assets"
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

func TestRenderPrometheusMetricsEmitsTensorFoldNativeMetrics(t *testing.T) {
	t.Parallel()

	metrics := &SystemMetrics{}
	containers := []Container{{
		Name:   "yokai-deployment-tf-g1-head",
		Image:  "ghcr.io/miaai-lab/glm-5.3-flash-exl3-2x-dgx-sparks-tensorfold@sha256:abc",
		Status: "running",
		VLLMMetrics: &VLLMMetrics{
			Model:                     "GLM-5.3-Flash-EXL3",
			RequestsRunning:           2,
			HasRequestsRunning:        true,
			TensorFoldRoundsTotal:     1048576,
			HasTensorFoldRoundsTotal:  true,
			HasTensorFoldNativeMetric: true,
		},
	}}

	got := renderPrometheusMetrics(metrics, containers)
	for _, want := range []string{
		`yokai_tensorfold_requests_running{backend="tensorfold",model="GLM-5.3-Flash-EXL3",service="deployment-tf-g1-head"} 2`,
		`yokai_tensorfold_rounds_total{backend="tensorfold",model="GLM-5.3-Flash-EXL3",service="deployment-tf-g1-head"} 1048576`,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("expected TensorFold metric %q in:\n%s", want, got)
		}
	}
}

func TestRenderPrometheusMetricsOmitsTensorFoldMetricsForNonTensorFold(t *testing.T) {
	t.Parallel()

	metrics := &SystemMetrics{}
	containers := []Container{{
		Name:   "yokai-vllm-service",
		Image:  "vllm/vllm@sha256:test",
		Status: "running",
		VLLMMetrics: &VLLMMetrics{
			Model:                    "test-model",
			RequestsRunning:          1,
			HasRequestsRunning:       true,
			TensorFoldRoundsTotal:    100,
			HasTensorFoldRoundsTotal: true,
		},
	}}

	got := renderPrometheusMetrics(metrics, containers)
	if strings.Contains(got, "yokai_tensorfold_requests_running{") {
		t.Fatalf("TensorFold-specific metrics should not be emitted for non-TensorFold services:\n%s", got)
	}
	if strings.Contains(got, "yokai_tensorfold_rounds_total{") {
		t.Fatalf("TensorFold-specific metrics should not be emitted for non-TensorFold services:\n%s", got)
	}
}

func TestTensorFoldAlertRulesYAML(t *testing.T) {
	t.Parallel()

	rules := assets.TensorFoldAlertRulesYAML
	if !strings.Contains(rules, "alert: TensorFoldGenerationHang") {
		t.Fatalf("alert name TensorFoldGenerationHang not found in rules:\n%s", rules)
	}
	if !strings.Contains(rules, "yokai_tensorfold_requests_running") {
		t.Fatal("expr does not reference yokai_tensorfold_requests_running")
	}
	if !strings.Contains(rules, "yokai_tensorfold_rounds_total[2m]") {
		t.Fatal("expr does not reference yokai_tensorfold_rounds_total[2m]")
	}
	if !strings.Contains(rules, "for: 2m") {
		t.Fatal("rule missing 'for: 2m'")
	}
}
