package daemon

import (
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/spencerbull/yokai/internal/bkc"
	"github.com/spencerbull/yokai/internal/hfmem"
)

type deployBKCResponse struct {
	Config *deployBKCRecord `json:"config,omitempty"`
}

type deployBKCRecord struct {
	ID              string            `json:"id"`
	Name            string            `json:"name"`
	Workload        string            `json:"workload"`
	ModelID         string            `json:"model_id"`
	Image           string            `json:"image"`
	Port            string            `json:"port"`
	ExtraArgs       string            `json:"extra_args"`
	Env             map[string]string `json:"env"`
	Volumes         map[string]string `json:"volumes"`
	Plugins         []string          `json:"plugins"`
	Runtime         map[string]any    `json:"runtime"`
	Description     string            `json:"description"`
	MatchType       string            `json:"match_type"`
	Source          string            `json:"source"`
	Notes           []string          `json:"notes"`
	Warning         string            `json:"warning,omitempty"`
	TargetDevices   []string          `json:"target_devices,omitempty"`
	MinVRAMGBPerGPU float64           `json:"min_vram_gb_per_gpu,omitempty"`
	MinGPUCount     int               `json:"min_gpu_count,omitempty"`
	Quantization    string            `json:"quantization,omitempty"`
	Arch            string            `json:"arch,omitempty"`
}

type vllmMemoryEstimateRequest struct {
	ContextLength int    `json:"context_length"`
	DeviceID      string `json:"device_id"`
	Model         string `json:"model"`
	OverheadGB    string `json:"overhead_gb"`
	ExtraArgs     string `json:"extra_args,omitempty"`
}

type vllmMemoryEstimateResponse struct {
	AppliedTPDefault bool    `json:"applied_tp_default"`
	ContextLength    int     `json:"context_length"`
	GPUCount         int     `json:"gpu_count"`
	KVCacheGB        float64 `json:"kv_cache_gb"`
	MinVRAMGB        float64 `json:"min_vram_gb"`
	OverheadGB       float64 `json:"overhead_gb"`
	RequiredPerGPUGB float64 `json:"required_per_gpu_gb"`
	TensorParallel   int     `json:"tensor_parallel"`
	Utilization      float64 `json:"utilization"`
	WeightsGB        float64 `json:"weights_gb"`
}

type deviceMetricsPayload struct {
	GPUs   json.RawMessage `json:"gpus"`
	Online bool            `json:"online"`
}

type deployGPU struct {
	VRAMTotalMB int64 `json:"vram_total_mb"`
}

type liveDeviceMetricsPayload struct {
	GPUs []deployGPU `json:"gpus"`
}

func (d *Daemon) handleDeployBKC(w http.ResponseWriter, r *http.Request) {
	workload := strings.TrimSpace(r.URL.Query().Get("workload"))
	model := strings.TrimSpace(r.URL.Query().Get("model"))
	deviceID := strings.TrimSpace(r.URL.Query().Get("device_id"))
	deviceProfile := strings.TrimSpace(r.URL.Query().Get("device_profile"))
	if workload == "" || model == "" {
		writeJSON(w, http.StatusOK, deployBKCResponse{})
		return
	}

	var target bkc.Workload
	switch workload {
	case string(bkc.WorkloadVLLM):
		target = bkc.WorkloadVLLM
	case string(bkc.WorkloadLlamaCpp):
		target = bkc.WorkloadLlamaCpp
	default:
		writeJSON(w, http.StatusOK, deployBKCResponse{})
		return
	}

	cfg, matchType, ok := bkc.LookupBest(target, model)
	if !ok {
		writeJSON(w, http.StatusOK, deployBKCResponse{})
		return
	}

	// When the UI supplied a device, prefer a device-aware variant so GB10 and
	// RTX PRO 6000 users get the right container image / flag set.
	if matchType == bkc.MatchExact {
		vramGB, gpuCount := 0.0, 0
		if deviceID != "" {
			if gpus, err := d.fetchDeployGPUs(deviceID); err == nil && len(gpus) > 0 {
				vramGB = smallestVRAMGB(gpus)
				gpuCount = len(gpus)
			}
		}
		if picked, found := bkc.LookupForDevice(target, model, deviceProfile, vramGB, gpuCount); found {
			cfg = picked
		}
	}

	warning := ""
	if matchType == bkc.MatchSuggested {
		warning = "Suggested BKC for a similar model. Review image, port, and flags before deploying."
	}

	writeJSON(w, http.StatusOK, deployBKCResponse{Config: &deployBKCRecord{
		ID:        cfg.ID,
		Name:      cfg.Name,
		Workload:  string(cfg.Workload),
		ModelID:   cfg.ModelID,
		Image:     cfg.Image,
		Port:      cfg.Port,
		ExtraArgs: cfg.ExtraArgs,
		Env:       cfg.Env,
		Volumes:   cfg.Volumes,
		Plugins:   cfg.Plugins,
		Runtime: map[string]any{
			"ipc_mode": cfg.Runtime.IPCMode,
			"shm_size": cfg.Runtime.ShmSize,
			"ulimits":  cfg.Runtime.Ulimits,
		},
		Description:     cfg.Description,
		MatchType:       string(matchType),
		Source:          cfg.Source,
		Notes:           cfg.Notes,
		Warning:         warning,
		TargetDevices:   cfg.TargetDevices,
		MinVRAMGBPerGPU: cfg.MinVRAMGBPerGPU,
		MinGPUCount:     cfg.MinGPUCount,
		Quantization:    cfg.Quantization,
		Arch:            cfg.Arch,
	}})
}

func (d *Daemon) handleVLLMMemoryEstimate(w http.ResponseWriter, r *http.Request) {
	var req vllmMemoryEstimateRequest
	if err := decodeJSONBody(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad_request", "message": err.Error()})
		return
	}

	req.Model = strings.TrimSpace(req.Model)
	if req.Model == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad_request", "message": "model is required"})
		return
	}
	if req.ContextLength <= 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad_request", "message": "context length must be positive"})
		return
	}
	overheadGB, err := strconv.ParseFloat(strings.TrimSpace(req.OverheadGB), 64)
	if err != nil || overheadGB <= 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad_request", "message": "overhead_gb must be a positive number"})
		return
	}

	gpus, err := d.fetchDeployGPUs(req.DeviceID)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "device_metrics_failed", "message": err.Error()})
		return
	}

	tensorParallel, usedDefault := detectTensorParallelSize(req.ExtraArgs, len(gpus))
	if tensorParallel > len(gpus) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad_request", "message": fmt.Sprintf("tensor parallel size %d exceeds detected GPU count %d", tensorParallel, len(gpus))})
		return
	}

	token := ""
	d.mu.RLock()
	token = d.cfg.HFToken
	d.mu.RUnlock()
	if token == "" {
		token = loadHFTokenFromEnv()
	}

	kvCacheDType, _ := argValue(req.ExtraArgs, "--kv-cache-dtype")
	if kvCacheDType == "" {
		if cfg, _, ok := bkc.LookupBest(bkc.WorkloadVLLM, req.Model); ok {
			kvCacheDType, _ = argValue(cfg.ExtraArgs, "--kv-cache-dtype")
		}
	}
	estimate, err := hfmem.EstimateModel(req.Model, token, req.ContextLength, kvCacheDType)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "hf_mem_failed", "message": err.Error()})
		return
	}

	minVRAMGB := smallestVRAMGB(gpus)
	requiredPerGPUGB := (bytesToGB(estimate.WeightsBytes) + bytesToGB(estimate.KVCacheBytes)) / float64(tensorParallel)
	requiredPerGPUGB += overheadGB
	utilization := roundUpHundredth(requiredPerGPUGB / minVRAMGB)
	if utilization > 0.99 {
		utilization = 0.99
	}

	writeJSON(w, http.StatusOK, vllmMemoryEstimateResponse{
		AppliedTPDefault: usedDefault,
		ContextLength:    req.ContextLength,
		GPUCount:         len(gpus),
		KVCacheGB:        bytesToGB(estimate.KVCacheBytes),
		MinVRAMGB:        minVRAMGB,
		OverheadGB:       overheadGB,
		RequiredPerGPUGB: requiredPerGPUGB,
		TensorParallel:   tensorParallel,
		Utilization:      utilization,
		WeightsGB:        bytesToGB(estimate.WeightsBytes),
	})
}

func (d *Daemon) fetchDeployGPUs(deviceID string) ([]deployGPU, error) {
	metrics, ok := d.aggregator.DeviceMetrics(deviceID)
	if !ok {
		return d.fetchDeployGPUsDirect(deviceID)
	}

	payload := deviceMetricsPayload{
		GPUs:   metrics.GPUs,
		Online: metrics.Online,
	}
	if !payload.Online {
		return d.fetchDeployGPUsDirect(deviceID)
	}
	if len(payload.GPUs) == 0 {
		return d.fetchDeployGPUsDirect(deviceID)
	}

	var gpus []deployGPU
	if err := json.Unmarshal(payload.GPUs, &gpus); err != nil {
		return nil, fmt.Errorf("parsing GPU metrics: %w", err)
	}
	if len(gpus) == 0 {
		return d.fetchDeployGPUsDirect(deviceID)
	}
	return gpus, nil
}

func (d *Daemon) fetchDeployGPUsDirect(deviceID string) ([]deployGPU, error) {
	device := d.lookupDevice(deviceID)
	if device == nil {
		return nil, fmt.Errorf("device not found")
	}

	localPort := d.tunnels.LocalPort(deviceID)
	if localPort == 0 {
		return nil, fmt.Errorf("fetching device metrics: not available yet")
	}

	req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("http://localhost:%d/metrics", localPort), nil)
	if err != nil {
		return nil, fmt.Errorf("building metrics request: %w", err)
	}
	if device.AgentToken != "" {
		req.Header.Set("Authorization", "Bearer "+device.AgentToken)
	}

	resp, err := (&http.Client{Timeout: 5 * time.Second}).Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetching device metrics: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("device metrics returned %d", resp.StatusCode)
	}

	var payload liveDeviceMetricsPayload
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, fmt.Errorf("parsing live device metrics: %w", err)
	}
	if len(payload.GPUs) == 0 {
		return nil, fmt.Errorf("selected device reports no GPUs")
	}
	return payload.GPUs, nil
}

func detectTensorParallelSize(extraArgs string, gpuCount int) (int, bool) {
	if value, ok := argValue(extraArgs, "--tensor-parallel-size"); ok {
		if parsed, err := strconv.Atoi(value); err == nil && parsed > 0 {
			return parsed, false
		}
	}
	if gpuCount <= 0 {
		return 1, true
	}
	return gpuCount, true
}

func argValue(args, flag string) (string, bool) {
	tokens := strings.Fields(args)
	for i := 0; i < len(tokens); i++ {
		token := tokens[i]
		if token == flag {
			if i+1 < len(tokens) {
				return tokens[i+1], true
			}
			return "", true
		}
		prefix := flag + "="
		if strings.HasPrefix(token, prefix) {
			return strings.TrimPrefix(token, prefix), true
		}
	}
	return "", false
}

func bytesToGB(v int64) float64 {
	return float64(v) / (1024 * 1024 * 1024)
}

func smallestVRAMGB(gpus []deployGPU) float64 {
	minVRAMGB := float64(gpus[0].VRAMTotalMB) / 1024.0
	for _, gpu := range gpus[1:] {
		vramGB := float64(gpu.VRAMTotalMB) / 1024.0
		if vramGB < minVRAMGB {
			minVRAMGB = vramGB
		}
	}
	return minVRAMGB
}

func roundUpHundredth(v float64) float64 {
	return math.Ceil(v*100) / 100
}
