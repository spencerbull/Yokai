package daemon

import (
	"net/http"
	"strings"

	"github.com/spencerbull/yokai/internal/config"
)

// recipeInspectReport is a read-only pre-flight: it checks a candidate against
// live topology (and its own claimed requirements) WITHOUT spending hardware
// and WITHOUT promoting status. Agents can use it to de-risk before verify.
type recipeInspectReport struct {
	RecipeID       string   `json:"recipe_id"`
	DeviceID       string   `json:"device_id,omitempty"`
	LiveMetrics    bool     `json:"live_metrics"`
	DeviceVRAMGB   float64  `json:"device_vram_gb,omitempty"`
	DeviceGPUCount int      `json:"device_gpu_count,omitempty"`
	ClaimedMinVRAM float64  `json:"claimed_min_vram_gb,omitempty"`
	ClaimedMinGPUs int      `json:"claimed_min_gpu_count,omitempty"`
	TargetDevices  []string `json:"target_devices,omitempty"`
	Fits           bool     `json:"fits"`
	Reasons        []string `json:"reasons,omitempty"`
	Status         string   `json:"status"`
}

// handleAgentInspectRecipe is the offline/read-only pre-gate (Phase 2). It
// reports whether a candidate can plausibly fit a device before any hardare is
// spent in verify/swap. Inspection never promotes: validated remains exclusive
// to a passing live readiness probe.
func (d *Daemon) handleAgentInspectRecipe(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("recipeID")
	deviceID := strings.TrimSpace(r.URL.Query().Get("device_id"))
	if d.recipeStore == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not_found", "message": "recipe store is not loaded"})
		return
	}
	rec, ok := d.recipeStore.Get(id)
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not_found", "message": "no candidate recipe with that id"})
		return
	}

	report := recipeInspectReport{
		RecipeID:       id,
		DeviceID:       deviceID,
		ClaimedMinVRAM: rec.Config.MinVRAMGBPerGPU,
		ClaimedMinGPUs: rec.Config.MinGPUCount,
		TargetDevices:  rec.Config.TargetDevices,
		Status:         string(rec.Status),
	}

	if deviceID == "" {
		report.Reasons = append(report.Reasons, "no device_id supplied; only claimed requirements reported")
		writeJSON(w, http.StatusOK, report)
		return
	}

	live, fits, reasons := d.recipeDeviceFit(deviceID, rec.Config)
	report.LiveMetrics = live
	report.Fits = fits
	report.Reasons = reasons
	if live {
		gpus, err := d.fetchAgentGPUs(deviceID)
		if err == nil && len(gpus) > 0 {
			report.DeviceGPUCount = len(gpus)
			report.DeviceVRAMGB = float64(gpus[0].VRAMTotalMB) / 1024.0
			for _, g := range gpus[1:] {
				if v := float64(g.VRAMTotalMB) / 1024.0; v < report.DeviceVRAMGB {
					report.DeviceVRAMGB = v
				}
			}
		}
	}
	writeJSON(w, http.StatusOK, report)
}

// selfHostedCollision reports whether acting on a device would disrupt the
// fleet's own control plane — i.e., that device currently serves the acting
// agent's backing model, or is a device explicitly marked Control in config.
// This is the guard for the "agent is backed by the model running on the device
// undergoing the swap" case: we refuse to cut the wire to our own brain unless
// the operator explicitly acknowledges (allow_self).
func (d *Daemon) selfHostedCollision(deviceID, backingModel string) (config.Service, bool) {
	d.mu.RLock()
	services := append([]config.Service(nil), d.cfg.Services...)
	d.mu.RUnlock()

	backing := strings.ToLower(strings.TrimSpace(backingModel))
	var onDevice []config.Service
	for _, svc := range services {
		if svc.DeviceID != deviceID {
			continue
		}
		onDevice = append(onDevice, svc)
		if backing != "" && svc.Model != "" && strings.Contains(strings.ToLower(svc.Model), backing) {
			return svc, true
		}
	}
	if dev := d.lookupDevice(deviceID); dev != nil && dev.Control && len(onDevice) > 0 {
		return onDevice[0], true
	}
	return config.Service{}, false
}
