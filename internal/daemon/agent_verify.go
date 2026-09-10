package daemon

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/spencerbull/yokai/internal/recipes"
)

// recipeVerifyReport is the HTTP response for an on-hardware verification
// trial. Every field is written by the daemon (server-recorded evidence), not
// by the proposing agent.
type recipeVerifyReport struct {
	RecipeID      string                `json:"recipe_id"`
	DeviceID      string                `json:"device_id"`
	GatePassed    bool                  `json:"gate_passed"`
	GateReasons   []string              `json:"gate_reasons,omitempty"`
	ContainerID   string                `json:"container_id,omitempty"`
	Ready         bool                  `json:"ready"`
	ServedModelID string                `json:"served_model_id,omitempty"`
	Status        string                `json:"status"`
	ValidatedOn   []string              `json:"validated_on,omitempty"`
	LastVerify    *recipes.RecipeVerify `json:"last_verify,omitempty"`
}

// handleAgentVerifyRecipe runs a candidate recipe as a transient, flagged
// container on a live device, probes readiness, then tears it down. It is the
// "agents test on hardware" primitive: only a passing live probe promotes the
// candidate to validated (evidence-based, never an agent's claim). The image
// pull failure case doubles as real digest verification.
func (d *Daemon) handleAgentVerifyRecipe(w http.ResponseWriter, r *http.Request) {
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
	if deviceID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "device_required", "message": "verify requires ?device_id=<configured device id>"})
		return
	}

	report := recipeVerifyReport{RecipeID: id, DeviceID: deviceID, Status: string(rec.Status)}

	// Stage 1: live hardware gate — no container is spent on a candidate that
	// cannot fit, mirrors the read-only validate dry-run.
	live, fits, reasons := d.recipeDeviceFit(deviceID, rec.Config)
	report.GateReasons = reasons
	now := time.Now().UTC().Format(time.RFC3339)
	if !live {
		rec.LastVerify = &recipes.RecipeVerify{DeviceID: deviceID, At: now, Message: "device unreachable for live metrics: " + strings.Join(reasons, "; ")}
		_ = d.recipeStore.Put(rec)
		writeJSON(w, http.StatusUnprocessableEntity, map[string]any{"error": "device_unreachable", "message": "no live metrics for device; cannot verify", "report": report})
		return
	}
	if !fits {
		rec.LastVerify = &recipes.RecipeVerify{DeviceID: deviceID, At: now, Message: "hardware gate failed: " + strings.Join(reasons, "; ")}
		rec.UpdatedAt = now
		_ = d.recipeStore.Put(rec)
		report.LastVerify = rec.LastVerify
		writeJSON(w, http.StatusUnprocessableEntity, map[string]any{"error": "no_fit", "message": "candidate does not fit this device", "report": report})
		return
	}
	report.GatePassed = true

	// Stage 2: transient on-hardware trial. Launch the digest-pinned image,
	// readiness-probe it, then always tear it down (a bad candidate must not
	// linger serving traffic).
	if d.aggregator == nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "aggregator_unavailable", "message": "aggregator is not running"})
		return
	}
	deploy, deployErr := d.aggregator.Deploy(recipeDeployRequest(rec.Config, deviceID))
	if deployErr != nil {
		rec.LastVerify = &recipes.RecipeVerify{DeviceID: deviceID, At: now, Message: "deploy failed: " + deployErr.Error()}
		rec.UpdatedAt = now
		_ = d.recipeStore.Put(rec)
		report.LastVerify = rec.LastVerify
		writeJSON(w, http.StatusUnprocessableEntity, map[string]any{"error": "deploy_failed", "message": deployErr.Error(), "report": report})
		return
	}
	report.ContainerID = deploy.ContainerID
	defer func() { _ = d.aggregator.RemoveContainer(deviceID, deploy.ContainerID) }()

	result, testErr := d.aggregator.TestContainerWithMetrics(deviceID, deploy.ContainerID, d.deviceAgentToken(deviceID))
	verify := &recipes.RecipeVerify{DeviceID: deviceID, At: now, ContainerID: deploy.ContainerID}
	switch {
	case testErr != nil:
		verify.Message = "readiness probe failed: " + testErr.Error()
	case result == nil:
		verify.Message = "readiness probe returned no result"
	case !result.OK:
		verify.Message = result.Message
		if verify.Message == "" {
			verify.Message = "readiness probe reported not ready"
		}
	default:
		verify.OK = true
		verify.MetricsReady = result.MetricsReady
		verify.ServedModelID = result.Model
		verify.Message = result.Message
	}
	rec.LastVerify = verify
	rec.UpdatedAt = now

	if verify.OK {
		// Evidence-based promotion: the candidate actually booted and served the
		// expected model on this device under a live readiness probe.
		rec.Status = recipes.StatusValidated
		rec.ValidatedOn = appendUnique(rec.ValidatedOn, deviceID)
		report.Ready = true
		report.ServedModelID = verify.ServedModelID
	}
	if err := d.recipeStore.Put(rec); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "store_write_failed", "message": err.Error()})
		return
	}
	report.Status = string(rec.Status)
	report.ValidatedOn = rec.ValidatedOn
	report.LastVerify = verify

	status := http.StatusOK
	if !verify.OK {
		status = http.StatusUnprocessableEntity
	}
	writeJSON(w, status, map[string]any{"verified": verify.OK, "report": report})
}

// recipeDeployRequest maps a candidate's deployable config to a transient
// per-device container run. Ports and workload image service defaults are
// resolved by the device agent (vLLM => 8000, etc.).
func recipeDeployRequest(cfg recipes.RecipeConfig, deviceID string) DeployRequest {
	name := cfg.Name
	if name == "" {
		name = "recipe-verify-" + cfg.ModelID
	}
	req := DeployRequest{
		DeviceID:    deviceID,
		ServiceType: string(cfg.Workload),
		Image:       cfg.Image,
		Name:        name,
		Model:       cfg.ModelID,
		Env:         cfg.Env,
		ExtraArgs:   cfg.ExtraArgs,
		Volumes:     cfg.Volumes,
		Plugins:     cfg.Plugins,
		Ports:       map[string]string{},
	}
	if cfg.Port != "" {
		req.Ports[cfg.Port] = cfg.Port
	}
	return req
}

// recipeDeviceFit is the live hardware gate shared by validate and verify. It
// returns whether live metrics were available and whether the candidate fits
// the device's VRAM/GPU-count/TargetDevices.
func (d *Daemon) recipeDeviceFit(deviceID string, cfg recipes.RecipeConfig) (live bool, fits bool, reasons []string) {
	gpus, err := d.fetchAgentGPUs(deviceID)
	if err != nil || len(gpus) == 0 {
		if err != nil {
			reasons = append(reasons, "device not reachable for live metrics: "+err.Error())
		} else {
			reasons = append(reasons, "device reports no GPUs")
		}
		return false, false, reasons
	}
	live = true

	vramGB := float64(gpus[0].VRAMTotalMB) / 1024.0
	for _, gpu := range gpus[1:] {
		if v := float64(gpu.VRAMTotalMB) / 1024.0; v < vramGB {
			vramGB = v
		}
	}
	gpuCount := len(gpus)

	if cfg.MinVRAMGBPerGPU > 0 && vramGB < cfg.MinVRAMGBPerGPU {
		reasons = append(reasons, fmt.Sprintf("needs %.0fGB/GPU, device has %.0fGB", cfg.MinVRAMGBPerGPU, vramGB))
	}
	if cfg.MinGPUCount > 0 && gpuCount < cfg.MinGPUCount {
		reasons = append(reasons, fmt.Sprintf("needs %d GPU(s), device has %d", cfg.MinGPUCount, gpuCount))
	}
	if len(cfg.TargetDevices) > 0 {
		name := strings.ToLower(gpus[0].Name)
		matched := false
		for _, t := range cfg.TargetDevices {
			if name != "" && (strings.Contains(name, strings.ToLower(t)) || strings.Contains(strings.ToLower(t), name)) {
				matched = true
				break
			}
		}
		if !matched && name != "" {
			reasons = append(reasons, fmt.Sprintf("declared target_devices %v, device GPU is %q", cfg.TargetDevices, gpus[0].Name))
		}
	}

	fits = len(reasons) == 0
	return live, fits, reasons
}

// deviceAgentToken returns the shared key for the device's agent (used to
// authenticate metrics-bearing readiness probes).
func (d *Daemon) deviceAgentToken(deviceID string) string {
	dev := d.lookupDevice(deviceID)
	if dev == nil {
		return ""
	}
	return dev.AgentToken
}
