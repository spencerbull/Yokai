package agent

import (
	"log"
	"net/http"
	"sort"
	"strconv"
	"strings"
)

func handlePrometheusMetrics(w http.ResponseWriter, r *http.Request) {
	metrics := CollectMetrics()
	containers, err := listContainers()
	if err != nil {
		log.Printf("warning: failed to list containers for Prometheus metrics: %v", err)
	}

	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(renderPrometheusMetrics(metrics, containers)))
}

func renderPrometheusMetrics(metrics *SystemMetrics, containers []Container) string {
	var b strings.Builder

	writeMetricHeader(&b, "yokai_cpu_percent", "CPU utilization percentage", "gauge")
	writePrometheusSample(&b, "yokai_cpu_percent", nil, formatFloat(metrics.CPU.Percent))

	writeMetricHeader(&b, "yokai_ram_used_bytes", "RAM used in bytes", "gauge")
	writePrometheusSample(&b, "yokai_ram_used_bytes", nil, formatFloat(float64(metrics.RAM.UsedMB*1024*1024)))

	writeMetricHeader(&b, "yokai_ram_total_bytes", "RAM total in bytes", "gauge")
	writePrometheusSample(&b, "yokai_ram_total_bytes", nil, formatFloat(float64(metrics.RAM.TotalMB*1024*1024)))

	writeMetricHeader(&b, "yokai_ram_percent", "RAM utilization percentage", "gauge")
	writePrometheusSample(&b, "yokai_ram_percent", nil, formatFloat(metrics.RAM.Percent))

	writeMetricHeader(&b, "yokai_disk_free_bytes", "Disk free space in bytes", "gauge")
	writePrometheusSample(&b, "yokai_disk_free_bytes", nil, formatFloat(float64(metrics.Disk.FreeGB*1024*1024*1024)))

	writeMetricHeader(&b, "yokai_gpu_utilization", "GPU utilization percentage", "gauge")
	writeMetricHeader(&b, "yokai_gpu_vram_used_bytes", "GPU VRAM used in bytes", "gauge")
	writeMetricHeader(&b, "yokai_gpu_vram_total_bytes", "GPU VRAM total in bytes", "gauge")
	writeMetricHeader(&b, "yokai_gpu_temperature_celsius", "GPU temperature in Celsius", "gauge")
	writeMetricHeader(&b, "yokai_gpu_power_draw_watts", "GPU power draw in watts", "gauge")
	writeMetricHeader(&b, "yokai_gpu_power_limit_watts", "GPU power limit in watts", "gauge")
	for _, gpu := range metrics.GPUs {
		labels := map[string]string{
			"gpu":  strconv.Itoa(gpu.Index),
			"name": gpu.Name,
		}
		writePrometheusSample(&b, "yokai_gpu_utilization", labels, formatFloat(float64(gpu.UtilPercent)))
		writePrometheusSample(&b, "yokai_gpu_vram_used_bytes", labels, formatFloat(float64(gpu.VRAMUsedMB*1024*1024)))
		writePrometheusSample(&b, "yokai_gpu_vram_total_bytes", labels, formatFloat(float64(gpu.VRAMTotalMB*1024*1024)))
		writePrometheusSample(&b, "yokai_gpu_temperature_celsius", labels, formatFloat(float64(gpu.TempC)))
		writePrometheusSample(&b, "yokai_gpu_power_draw_watts", labels, formatFloat(float64(gpu.PowerDrawW)))
		writePrometheusSample(&b, "yokai_gpu_power_limit_watts", labels, formatFloat(float64(gpu.PowerLimitW)))
	}

	writeMetricHeader(&b, "yokai_service_up", "Whether the inference service is running", "gauge")
	writeMetricHeader(&b, "yokai_service_info", "Static service metadata", "gauge")
	writeMetricHeader(&b, "yokai_llm_prefill_tokens_per_second", "Prompt prefill throughput in tokens per second", "gauge")
	writeMetricHeader(&b, "yokai_llm_decode_tokens_per_second", "Decode throughput in tokens per second", "gauge")
	writeMetricHeader(&b, "yokai_llm_requests_in_flight", "Current in-flight requests", "gauge")
	writeMetricHeader(&b, "yokai_llm_requests_queued", "Current queued requests", "gauge")
	writeMetricHeader(&b, "yokai_llm_prompt_tokens_total", "Total prompt tokens processed", "counter")
	writeMetricHeader(&b, "yokai_llm_generated_tokens_total", "Total generated output tokens", "counter")
	writeMetricHeader(&b, "yokai_llm_ttft_seconds", "Time to first token", "histogram")

	sort.Slice(containers, func(i, j int) bool {
		return containers[i].Name < containers[j].Name
	})

	for _, container := range containers {
		if isMonitoringContainer(container) {
			continue
		}

		backend := inferenceBackend(container)
		if backend == "" {
			continue
		}

		labels := map[string]string{
			"service": trimServiceLabel(container.Name),
			"backend": backend,
			"model":   modelLabel(container),
		}

		writePrometheusSample(&b, "yokai_service_up", labels, formatFloat(serviceUpValue(container)))
		infoLabels := cloneLabels(labels)
		infoLabels["image"] = container.Image
		writePrometheusSample(&b, "yokai_service_info", infoLabels, "1")

		if backend != "vllm" || container.VLLMMetrics == nil {
			continue
		}

		vllm := container.VLLMMetrics
		if vllm.HasPromptTokPerSec {
			writePrometheusSample(&b, "yokai_llm_prefill_tokens_per_second", labels, formatFloat(vllm.PromptTokPerSec))
		}
		if vllm.HasGenerationTokPerSec {
			writePrometheusSample(&b, "yokai_llm_decode_tokens_per_second", labels, formatFloat(vllm.GenerationTokPerSec))
		}
		if vllm.HasRequestsRunning {
			writePrometheusSample(&b, "yokai_llm_requests_in_flight", labels, formatFloat(vllm.RequestsRunning))
		}
		if vllm.HasRequestsWaiting {
			writePrometheusSample(&b, "yokai_llm_requests_queued", labels, formatFloat(vllm.RequestsWaiting))
		}
		if vllm.HasPromptTokensTotal {
			writePrometheusSample(&b, "yokai_llm_prompt_tokens_total", labels, formatFloat(vllm.PromptTokensTotal))
		}
		if vllm.HasGenerationTokensTotal {
			writePrometheusSample(&b, "yokai_llm_generated_tokens_total", labels, formatFloat(vllm.GenerationTokensTotal))
		}
		if vllm.HasTTFT {
			for _, le := range sortedHistogramBounds(vllm.TTFTBuckets) {
				bucketLabels := cloneLabels(labels)
				bucketLabels["le"] = le
				writePrometheusSample(&b, "yokai_llm_ttft_seconds_bucket", bucketLabels, formatFloat(vllm.TTFTBuckets[le]))
			}
			writePrometheusSample(&b, "yokai_llm_ttft_seconds_sum", labels, formatFloat(vllm.TTFTSum))
			writePrometheusSample(&b, "yokai_llm_ttft_seconds_count", labels, formatFloat(vllm.TTFTCount))
		}
	}

	return b.String()
}

func writeMetricHeader(b *strings.Builder, name, help, metricType string) {
	b.WriteString("# HELP ")
	b.WriteString(name)
	b.WriteString(" ")
	b.WriteString(help)
	b.WriteString("\n# TYPE ")
	b.WriteString(name)
	b.WriteString(" ")
	b.WriteString(metricType)
	b.WriteString("\n")
}

func writePrometheusSample(b *strings.Builder, name string, labels map[string]string, value string) {
	b.WriteString(name)
	if len(labels) > 0 {
		keys := make([]string, 0, len(labels))
		for key := range labels {
			keys = append(keys, key)
		}
		sort.Strings(keys)

		b.WriteString("{")
		for i, key := range keys {
			if i > 0 {
				b.WriteString(",")
			}
			b.WriteString(key)
			b.WriteString(`="`)
			b.WriteString(escapePrometheusLabelValue(labels[key]))
			b.WriteString(`"`)
		}
		b.WriteString("}")
	}
	b.WriteString(" ")
	b.WriteString(value)
	b.WriteString("\n")
}

func escapePrometheusLabelValue(value string) string {
	value = strings.ReplaceAll(value, `\`, `\\`)
	value = strings.ReplaceAll(value, `"`, `\"`)
	return strings.ReplaceAll(value, "\n", `\n`)
}

func formatFloat(value float64) string {
	return strconv.FormatFloat(value, 'f', -1, 64)
}

func trimServiceLabel(name string) string {
	trimmed := strings.TrimPrefix(name, "yokai-")
	if trimmed == "" {
		return name
	}
	return trimmed
}

func inferenceBackend(container Container) string {
	switch {
	case isVLLMImage(container.Image):
		return "vllm"
	case isLlamaCppImage(container.Image):
		return "llamacpp"
	case isComfyUIImage(container.Image):
		return "comfyui"
	default:
		return ""
	}
}

func isMonitoringContainer(container Container) bool {
	name := strings.ToLower(container.Name)
	image := strings.ToLower(container.Image)
	if strings.HasPrefix(name, "yokai-mon-") {
		return true
	}
	return strings.Contains(image, "grafana") || strings.Contains(image, "prometheus") || strings.Contains(image, "node-exporter") || strings.Contains(image, "dcgm-exporter")
}

func modelLabel(container Container) string {
	if container.VLLMMetrics != nil && strings.TrimSpace(container.VLLMMetrics.Model) != "" {
		return strings.TrimSpace(container.VLLMMetrics.Model)
	}
	return ""
}

func serviceUpValue(container Container) float64 {
	if container.Status == "running" {
		return 1
	}
	return 0
}

func cloneLabels(labels map[string]string) map[string]string {
	cloned := make(map[string]string, len(labels))
	for key, value := range labels {
		cloned[key] = value
	}
	return cloned
}

func sortedHistogramBounds(buckets map[string]float64) []string {
	if len(buckets) == 0 {
		return nil
	}

	bounds := make([]string, 0, len(buckets))
	for bound := range buckets {
		bounds = append(bounds, bound)
	}

	sort.Slice(bounds, func(i, j int) bool {
		if bounds[i] == "+Inf" {
			return false
		}
		if bounds[j] == "+Inf" {
			return true
		}
		left, errLeft := strconv.ParseFloat(bounds[i], 64)
		right, errRight := strconv.ParseFloat(bounds[j], 64)
		if errLeft != nil || errRight != nil {
			return bounds[i] < bounds[j]
		}
		return left < right
	})

	return bounds
}
