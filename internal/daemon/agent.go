package daemon

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/spencerbull/yokai/internal/bkc"
	"github.com/spencerbull/yokai/internal/config"
	"github.com/spencerbull/yokai/internal/recipes"
)

// agent.go implements the external-agent read contract. A provider-agnostic
// agent (pi, opencode, hermes, openclaw, ...) calls these endpoints — either
// directly over HTTP or through the MCP shim — to discover recipes and
// hardware without knowing model ids up front.
//
//   - GET /agent/catalog    -> full machine-readable BKC catalog dump
//   - GET /agent/topology   -> fleet devices + GPUs + running services
//   - GET /agent/recommend  -> use-case -> ranked recipe suggestions
//
// These are read-only. The apply side (deploy/swap) reuses the existing
// /deploy and /deployments engine endpoints.

type agentGPU struct {
	Name        string `json:"name,omitempty"`
	MemoryType  string `json:"memory_type,omitempty"`
	VRAMTotalMB int64  `json:"vram_total_mb"`
}

type agentTopologyService struct {
	ID       string `json:"id"`
	DeviceID string `json:"device_id"`
	Type     string `json:"type"`
	Model    string `json:"model,omitempty"`
	Port     int    `json:"port"`
	Image    string `json:"image,omitempty"`
	Endpoint string `json:"endpoint,omitempty"`
}

type agentTopologyDevice struct {
	ID           string                 `json:"id"`
	Label        string                 `json:"label,omitempty"`
	Host         string                 `json:"host"`
	GPUType      string                 `json:"gpu_type,omitempty"`
	Online       bool                   `json:"online"`
	GPUs         []agentGPU             `json:"gpus,omitempty"`
	GPUCount     int                    `json:"gpu_count"`
	SmallestVRAM float64                `json:"smallest_vram_gb,omitempty"`
	TunnelPort   int                    `json:"tunnel_port"`
	Services     []agentTopologyService `json:"services,omitempty"`
}

type agentTopologyResponse struct {
	Devices []agentTopologyDevice `json:"devices"`
}

type agentRecipeRecord struct {
	deployBKCRecord
	Tier        string              `json:"tier,omitempty"`
	Status      string              `json:"status,omitempty"`
	Fingerprint string              `json:"fingerprint,omitempty"`
	Provenance  *recipes.Provenance `json:"provenance,omitempty"`
}

type agentRecommendRecord struct {
	agentRecipeRecord
	FitsDevice bool   `json:"fits_device,omitempty"`
	FitReason  string `json:"fit_reason,omitempty"`
}

type agentRecommendResponse struct {
	UseCase  string                 `json:"use_case"`
	DeviceID string                 `json:"device_id,omitempty"`
	Total    int                    `json:"total"`
	Options  []agentRecommendRecord `json:"options"`
}

// handleAgentCatalog dumps the curated + candidate recipe union as
// machine-readable JSON with hardware affinity, use-case metadata, and each
// record's trust tier and status.
func (d *Daemon) handleAgentCatalog(w http.ResponseWriter, r *http.Request) {
	records := d.allAgentRecipes()
	writeJSON(w, http.StatusOK, map[string]any{
		"count":     len(records),
		"use_cases": bkc.AllUseCases(),
		"catalog":   records,
	})
}

// allAgentRecipes joins the immutable curated BKC catalog (tier=curated,
// status=validated) with the mutable candidate store (tier=candidate), so
// recommendation and catalog see a single union while preserving each
// record's trust level.
func (d *Daemon) allAgentRecipes() []agentRecipeRecord {
	var records []agentRecipeRecord
	for _, cfg := range bkc.Catalog() {
		rec := agentRecipeRecord{
			deployBKCRecord: deployBKCRecordFromConfig(cfg, bkc.MatchExact, ""),
			Tier:            string(recipes.TierCurated),
			Status:          string(recipes.StatusValidated),
		}
		records = append(records, rec)
	}
	if d.recipeStore == nil {
		return records
	}
	for _, cand := range d.recipeStore.List() {
		rc := cand.Config
		cfg := rc.ToBKC()
		cfg.ID = cand.ID
		deploy := deployBKCRecordFromConfig(cfg, bkc.MatchExact, "")
		records = append(records, agentRecipeRecord{
			deployBKCRecord: deploy,
			Tier:            string(cand.Tier),
			Status:          string(cand.Status),
			Fingerprint:     cand.Fingerprint,
			Provenance:      &cand.Provenance,
		})
	}
	sort.Slice(records, func(i, j int) bool {
		li, lj := records[i].Tier, records[j].Tier
		if li != lj {
			return li > lj // curated before candidate
		}
		return records[i].ModelID < records[j].ModelID
	})
	return records
}

// handleAgentTopology reports each device's identity, online status, GPU
// shape (count + per-GPU VRAM), and the inference services running on it.
func (d *Daemon) handleAgentTopology(w http.ResponseWriter, r *http.Request) {
	statuses := d.deviceStatuses()
	services := d.runningServices()

	out := agentTopologyResponse{Devices: make([]agentTopologyDevice, 0, len(statuses))}
	for _, status := range statuses {
		gpus, _ := d.fetchAgentGPUs(status.ID)
		dev := agentTopologyDevice{
			ID:         status.ID,
			Label:      status.Label,
			Host:       status.Host,
			GPUType:    status.GPUType,
			Online:     status.Online,
			GPUs:       gpus,
			GPUCount:   len(gpus),
			TunnelPort: status.TunnelPort,
		}
		if len(gpus) > 0 {
			dev.SmallestVRAM = float64(gpus[0].VRAMTotalMB) / 1024.0
			for _, gpu := range gpus[1:] {
				if v := float64(gpu.VRAMTotalMB) / 1024.0; v < dev.SmallestVRAM {
					dev.SmallestVRAM = v
				}
			}
		}
		for _, svc := range services {
			if svc.DeviceID == status.ID {
				dev.Services = append(dev.Services, svc)
			}
		}
		out.Devices = append(out.Devices, dev)
	}
	writeJSON(w, http.StatusOK, out)
}

// handleAgentRecommend answers "what can I run for <use_case> on <device>?"
// filtering the catalog by use case and ranking options by hardware fit when a
// device is supplied.
func (d *Daemon) handleAgentRecommend(w http.ResponseWriter, r *http.Request) {
	useCase := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("use_case")))
	deviceID := strings.TrimSpace(r.URL.Query().Get("device_id"))
	workload := strings.TrimSpace(r.URL.Query().Get("workload"))
	limit := 10
	if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 {
			limit = parsed
		}
	}
	if useCase == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad_request", "message": "use_case is required"})
		return
	}

	// Device context for fit ranking (optional). Only meaningful when live GPU
	// metrics are available for the selected device.
	vramGB, gpuCount, hasMetrics := 0.0, 0, false
	var device *config.Device
	if deviceID != "" {
		device = d.lookupDevice(deviceID)
		if device == nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "device_not_found", "message": fmt.Sprintf("device %q was not found", deviceID)})
			return
		}
		if gpus, err := d.fetchAgentGPUs(deviceID); err == nil && len(gpus) > 0 {
			vramGB = float64(gpus[0].VRAMTotalMB) / 1024.0
			for _, gpu := range gpus[1:] {
				if v := float64(gpu.VRAMTotalMB) / 1024.0; v < vramGB {
					vramGB = v
				}
			}
			gpuCount = len(gpus)
			hasMetrics = true
		}
	}

	all := d.allAgentRecipes()
	var candidates []agentRecommendRecord
	for _, rec := range all {
		if !hasUseCase(rec.UseCases, useCase) {
			continue
		}
		if workload != "" && !strings.EqualFold(workload, rec.Workload) {
			continue
		}
		candidates = append(candidates, agentRecommendRecord{agentRecipeRecord: rec})
	}

	// Sort: hardware-fit first, then curated-before-candidate, then by name.
	sort.SliceStable(candidates, func(i, j int) bool {
		fi := fitsRecord(candidates[i], vramGB, gpuCount)
		fj := fitsRecord(candidates[j], vramGB, gpuCount)
		if hasMetrics && fi != fj {
			return fi
		}
		if candidates[i].Tier != candidates[j].Tier {
			return candidates[i].Tier > candidates[j].Tier // curated before candidate
		}
		if candidates[i].ModelID != candidates[j].ModelID {
			return candidates[i].ModelID < candidates[j].ModelID
		}
		return candidates[i].ID < candidates[j].ID
	})

	if limit > len(candidates) {
		limit = len(candidates)
	}
	if limit > 50 {
		limit = 50
	}

	options := make([]agentRecommendRecord, 0, limit)
	for _, rec := range candidates[:limit] {
		fits := fitsRecord(rec, vramGB, gpuCount)
		reason := ""
		if hasMetrics {
			switch {
			case !fits:
				reason = fitMissReason(rec, vramGB, gpuCount)
			case rec.MinGPUCount > 0 && gpuCount >= rec.MinGPUCount && rec.MinVRAMGBPerGPU <= 0:
				reason = fmt.Sprintf("fits %d GPU(s)", gpuCount)
			default:
				reason = "fits this device"
			}
		}
		rec.FitsDevice = fits && hasMetrics
		rec.FitReason = reason
		options = append(options, rec)
	}

	writeJSON(w, http.StatusOK, agentRecommendResponse{
		UseCase:  useCase,
		DeviceID: deviceID,
		Total:    len(candidates),
		Options:  options,
	})
}

// fitsRecord applies the same hardware-affinity gates the deploy wizard uses,
// so the agent's suggestion can never outrank an unfit recipe.
func fitsRecord(rec agentRecommendRecord, vramGB float64, gpuCount int) bool {
	if vramGB <= 0 {
		return true // no device context; don't over-filter
	}
	if rec.MinVRAMGBPerGPU > 0 && vramGB < rec.MinVRAMGBPerGPU {
		return false
	}
	if rec.MinGPUCount > 0 && gpuCount < rec.MinGPUCount {
		return false
	}
	return true
}

func fitMissReason(rec agentRecommendRecord, vramGB float64, gpuCount int) string {
	if rec.MinVRAMGBPerGPU > 0 && vramGB > 0 && vramGB < rec.MinVRAMGBPerGPU {
		return fmt.Sprintf("needs %.0fGB/GPU, device has %.0fGB", rec.MinVRAMGBPerGPU, vramGB)
	}
	if rec.MinGPUCount > 0 && gpuCount > 0 && gpuCount < rec.MinGPUCount {
		return fmt.Sprintf("needs %d GPU(s), device has %d", rec.MinGPUCount, gpuCount)
	}
	return "does not fit device"
}

// fetchAgentGPUs returns per-GPU shape for a device, preferring cached
// aggregator data and falling back to a direct agent query.
func (d *Daemon) fetchAgentGPUs(deviceID string) ([]agentGPU, error) {
	if d.aggregator != nil {
		if device, ok := d.aggregator.DeviceMetrics(deviceID); ok {
			var gpus []agentGPU
			if err := json.Unmarshal(device.GPUs, &gpus); err == nil && len(gpus) > 0 {
				return gpus, nil
			}
		}
	}

	dev := d.lookupDevice(deviceID)
	if dev == nil {
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
	if dev.AgentToken != "" {
		req.Header.Set("Authorization", "Bearer "+dev.AgentToken)
	}
	resp, err := (&http.Client{Timeout: 5 * time.Second}).Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetching device metrics: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("device metrics returned %d", resp.StatusCode)
	}
	var payload struct {
		GPUs []agentGPU `json:"gpus"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, fmt.Errorf("parsing live device metrics: %w", err)
	}
	return payload.GPUs, nil
}

// runningServices returns lightweight records for every configured service.
func (d *Daemon) runningServices() []agentTopologyService {
	d.mu.RLock()
	services := append([]config.Service(nil), d.cfg.Services...)
	d.mu.RUnlock()

	out := make([]agentTopologyService, 0, len(services))
	for _, svc := range services {
		out = append(out, agentTopologyService{
			ID:       svc.ID,
			DeviceID: svc.DeviceID,
			Type:     svc.Type,
			Model:    svc.Model,
			Port:     svc.Port,
			Image:    svc.Image,
			Endpoint: fmt.Sprintf("http://127.0.0.1:%d/v1", svc.Port),
		})
	}
	return out
}

// hasUseCase reports whether a record's populated use-case list includes the
// requested use case, case-insensitively.
func hasUseCase(useCases []string, want string) bool {
	for _, uc := range useCases {
		if strings.EqualFold(uc, want) {
			return true
		}
	}
	return false
}
