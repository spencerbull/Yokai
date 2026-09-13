package daemon

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/spencerbull/yokai/internal/config"
	"github.com/spencerbull/yokai/internal/recipes"
)

// agentSwapRequest is the contract for a deploy-on-verified swap of a candidate
// recipe onto a device. The acting agent may declare which model it itself runs
// as (agent_backing_model) so the daemon can refuse to take its own brain off a
// device unless allow_self acknowledges the risk.
type agentSwapRequest struct {
	RecipeID          string `json:"recipe_id"`
	DeviceID          string `json:"device_id"`
	AgentBackingModel string `json:"agent_backing_model,omitempty"`
	AllowSelf         bool   `json:"allow_self,omitempty"`
}

type recipeSwapReport struct {
	RecipeID          string               `json:"recipe_id"`
	DeviceID          string               `json:"device_id"`
	SelfHosted        bool                 `json:"self_hosted"`
	AllowSelf         bool                 `json:"allow_self"`
	PreviousModelID   string               `json:"previous_model_id,omitempty"`
	PreviousContainer string               `json:"previous_container,omitempty"`
	NewContainerID    string               `json:"new_container_id,omitempty"`
	Ready             bool                 `json:"ready"`
	Status            string               `json:"status"`
	SwapHistory       []recipes.SwapRecord `json:"swap_history,omitempty"`
}

// handleAgentSwapRecipe deploys a verified candidate onto a device: pull +
// readiness-probe, then cut over from the previous deployment (which becomes
// the rollback target). It is gated on (a) the candidate being evidence-validated
// on exactly this device, and (b) the self-host guard: if acting would disrupt the
// fleet's own control plane on that device, we refuse unless allow_self=true.
func (d *Daemon) handleAgentSwapRecipe(w http.ResponseWriter, r *http.Request) {
	var req agentSwapRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_json", "message": err.Error()})
		return
	}
	req.RecipeID = strings.TrimSpace(req.RecipeID)
	req.DeviceID = strings.TrimSpace(req.DeviceID)
	if req.RecipeID == "" || req.DeviceID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "recipe_and_device_required", "message": "recipe_id and device_id are required"})
		return
	}
	if d.recipeStore == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not_found", "message": "recipe store is not loaded"})
		return
	}
	rec, ok := d.recipeStore.Get(req.RecipeID)
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not_found", "message": "no candidate recipe with that id"})
		return
	}

	report := recipeSwapReport{
		RecipeID:  req.RecipeID,
		DeviceID:  req.DeviceID,
		AllowSelf: req.AllowSelf,
		Status:    string(rec.Status),
	}

	// (a) Self-host guard: never swap a device that currently serves the acting
	// agent's own model (or is a control device) without explicit acknowledgment.
	if _, colliding := d.selfHostedCollision(req.DeviceID, req.AgentBackingModel); colliding && !req.AllowSelf {
		writeJSON(w, http.StatusConflict, map[string]any{
			"error":   "self_hosted_swap",
			"message": "this device currently serves the active agent's own model (or is marked control); swapping would take the fleet's brain offline. Set allow_self=true to acknowledge you accept disrupting the controller.",
			"report":  report,
		})
		return
	}
	report.SelfHosted = req.AllowSelf && d.controlDevice(req.DeviceID)

	// (b) Evidence gate: only a candidate validated on exactly this device may
	// drive a swap (deploy-on-verified). No claim substitutes for the probe.
	if rec.Status != recipes.StatusValidated || !containsString(rec.ValidatedOn, req.DeviceID) {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]any{
			"error":   "require_verify",
			"message": "candidate must be validated (passing live probe) on this device before swap; run /agent/recipe/{id}/verify first",
			"report":  report,
		})
		return
	}

	// Previous deployment on the device becomes the rollback target.
	prev := d.serviceOnDevice(req.DeviceID)
	if prev != nil {
		report.PreviousModelID = prev.Model
		report.PreviousContainer = prev.ContainerID
	}

	now := time.Now().UTC().Format(time.RFC3339)
	record := func(ok bool, msg string) {
		rec.SwapHistory = append(rec.SwapHistory, recipes.SwapRecord{
			DeviceID: req.DeviceID, At: now, PreviousModelID: report.PreviousModelID,
			PreviousContainer: report.PreviousContainer, AllowSelf: req.AllowSelf, OK: ok, Message: msg,
		})
	}

	// Transient cutover trial: deploy + readiness-probe the new recipe.
	if d.aggregator == nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "aggregator_unavailable", "message": "aggregator is not running"})
		return
	}
	deploy, deployErr := d.aggregator.Deploy(recipeDeployRequest(rec.Config, req.DeviceID))
	if deployErr != nil {
		record(false, "deploy failed: "+deployErr.Error())
		report.SwapHistory = rec.SwapHistory
		rec.UpdatedAt = now
		_ = d.recipeStore.Put(rec)
		writeJSON(w, http.StatusUnprocessableEntity, map[string]any{"error": "deploy_failed", "message": deployErr.Error(), "report": report})
		return
	}
	report.NewContainerID = deploy.ContainerID

	result, testErr := d.aggregator.TestContainerWithMetrics(req.DeviceID, deploy.ContainerID, d.deviceAgentToken(req.DeviceID))
	ready := testErr == nil && result != nil && result.OK
	if ready {
		// Cutover: adopt the new container as the active service, then stop (not
		// remove) the previous one so it remains a rollback target.
		if err := d.persistSwappedService(rec.Config, req.DeviceID, deploy.ContainerID); err != nil {
			record(false, "persist new service failed: "+err.Error())
		} else if prev != nil && prev.ContainerID != "" && prev.ContainerID != deploy.ContainerID {
			_ = d.aggregator.StopContainer(req.DeviceID, prev.ContainerID)
		}
		if len(rec.SwapHistory) == 0 || !rec.SwapHistory[len(rec.SwapHistory)-1].OK {
			record(true, "swapped in; previous deployment retained as rollback target")
		}
		report.Ready = true
		report.Status = "active"
	} else {
		// Failed: tear down the trial container and keep the previous deployment.
		_ = d.aggregator.RemoveContainer(req.DeviceID, deploy.ContainerID)
		msg := "readiness probe failed"
		if testErr != nil {
			msg = "readiness probe failed: " + testErr.Error()
		} else if result != nil && result.Message != "" {
			msg = result.Message
		}
		record(false, msg)
		report.Status = string(rec.Status)
	}

	rec.UpdatedAt = now
	if err := d.recipeStore.Put(rec); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "store_write_failed", "message": err.Error()})
		return
	}
	report.SwapHistory = rec.SwapHistory

	status := http.StatusOK
	if !ready {
		status = http.StatusUnprocessableEntity
	}
	writeJSON(w, status, map[string]any{"swapped": ready, "report": report})
}

// persistSwappedService adopts the verified container as the active service for
// the device, mirroring the trusted deploy-persistence path.
func (d *Daemon) persistSwappedService(cfg recipes.RecipeConfig, deviceID, containerID string) error {
	return d.persistDeployResult(recipeDeployRequest(cfg, deviceID), &DeployResult{ContainerID: containerID})
}

// serviceOnDevice returns the current active service for a device, if any.
func (d *Daemon) serviceOnDevice(deviceID string) *config.Service {
	d.mu.RLock()
	defer d.mu.RUnlock()
	for i := range d.cfg.Services {
		if d.cfg.Services[i].DeviceID == deviceID {
			svc := d.cfg.Services[i]
			return &svc
		}
	}
	return nil
}

// controlDevice reports whether a device is marked as a control-plane device.
func (d *Daemon) controlDevice(deviceID string) bool {
	dev := d.lookupDevice(deviceID)
	return dev != nil && dev.Control
}

func containsString(xs []string, want string) bool {
	for _, x := range xs {
		if x == want {
			return true
		}
	}
	return false
}
