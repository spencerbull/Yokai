package daemon

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/spencerbull/yokai/internal/config"
	"github.com/spencerbull/yokai/internal/recipes"
)

func recipeStorePath() (string, error) {
	dir, err := config.ConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, recipes.StoreFile), nil
}

// agentRecipeCreateRequest is the write contract for proposing a candidate
// recipe. configured content is validated server-side regardless of writer.
type agentRecipeCreateRequest struct {
	ProposedBy string               `json:"proposed_by"`
	ID         string               `json:"id,omitempty"` // optional client-supplied id
	Config     recipes.RecipeConfig `json:"config"`
	Provenance recipes.Provenance   `json:"provenance"`
	Supersedes string               `json:"supersedes,omitempty"`
}

// handleAgentListRecipes lists the candidate tier from the store.
func (d *Daemon) handleAgentListRecipes(w http.ResponseWriter, r *http.Request) {
	list := []recipes.Recipe{}
	if d.recipeStore != nil {
		list = d.recipeStore.List()
	}
	writeJSON(w, http.StatusOK, map[string]any{"count": len(list), "recipes": list})
}

// handleAgentProposeRecipe ingests a candidate. It validates, dedups by config
// fingerprint, and persists with tier=candidate/status=proposed.
func (d *Daemon) handleAgentProposeRecipe(w http.ResponseWriter, r *http.Request) {
	var req agentRecipeCreateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_json", "message": err.Error()})
		return
	}
	proposedBy := strings.TrimSpace(req.ProposedBy)
	res := recipes.NewValidator().Validate(req.Config, req.Provenance, proposedBy)
	if !res.OK {
		writeJSON(w, http.StatusUnprocessableEntity, res)
		return
	}

	fp := recipes.Fingerprint(req.Config)
	// Dedup: a matching fingerprint (same model/workload/image/flags) already
	// exists regardless of writer, so agents can't spam near-duplicates.
	if d.recipeStore != nil {
		if existing, ok := d.recipeStore.FindFingerprint(fp); ok {
			writeJSON(w, http.StatusConflict, map[string]any{
				"error":   "duplicate_fingerprint",
				"message": "a candidate with this config fingerprint already exists; PATCH it to update",
				"recipe":  existing,
			})
			return
		}
	}

	id := req.ID
	if strings.TrimSpace(id) == "" {
		id = "rec_" + randHex(8)
	}
	now := time.Now().UTC().Format(time.RFC3339)
	rec := recipes.Recipe{
		ID:          id,
		Tier:        recipes.TierCandidate,
		Status:      recipes.StatusProposed,
		Config:      req.Config,
		Provenance:  req.Provenance,
		Fingerprint: fp,
		ProposedBy:  proposedBy,
		Supersedes:  req.Supersedes,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if d.recipeStore == nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "store_unavailable", "message": "recipe store is not loaded"})
		return
	}
	if err := d.recipeStore.Put(rec); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "store_write_failed", "message": err.Error()})
		return
	}
	writeJSON(w, http.StatusCreated, rec)
}

// handleAgentGetRecipe fetches a single candidate.
func (d *Daemon) handleAgentGetRecipe(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("recipeID")
	if d.recipeStore == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not_found", "message": "recipe store is not loaded"})
		return
	}
	rec, ok := d.recipeStore.Get(id)
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not_found", "message": "no candidate recipe with that id"})
		return
	}
	writeJSON(w, http.StatusOK, rec)
}

// handleAgentPatchRecipe updates a candidate (a "new version"). Config and
// provenance are re-validated; status is never flipped to validated here —
// only the live-probe validation endpoint may do that.
func (d *Daemon) handleAgentPatchRecipe(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("recipeID")
	if d.recipeStore == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not_found", "message": "recipe store is not loaded"})
		return
	}
	existing, ok := d.recipeStore.Get(id)
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not_found", "message": "no candidate recipe with that id"})
		return
	}
	var req struct {
		Config     *recipes.RecipeConfig `json:"config,omitempty"`
		Provenance *recipes.Provenance   `json:"provenance,omitempty"`
		Supersedes *string               `json:"supersedes,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_json", "message": err.Error()})
		return
	}
	next := existing
	if req.Config != nil {
		next.Config = *req.Config
	}
	if req.Provenance != nil {
		next.Provenance = *req.Provenance
	}
	if req.Supersedes != nil {
		next.Supersedes = *req.Supersedes
	}
	// Re-validate as if freshly proposed. Status stays as-is unless superseded.
	if res := recipes.NewValidator().Validate(next.Config, next.Provenance, next.ProposedBy); !res.OK {
		writeJSON(w, http.StatusUnprocessableEntity, res)
		return
	}
	next.Fingerprint = recipes.Fingerprint(next.Config)
	if next.Status != recipes.StatusValidated {
		next.Status = recipes.StatusProposed
	}
	next.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	if err := d.recipeStore.Put(next); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "store_write_failed", "message": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, next)
}

type recipeValidationReport struct {
	RecipeID    string   `json:"recipe_id"`
	DeviceID    string   `json:"device_id,omitempty"`
	LiveMetrics bool     `json:"live_metrics"`
	Fits        bool     `json:"fits"`
	Reasons     []string `json:"reasons,omitempty"`
	Status      string   `json:"status"`
	ValidatedOn []string `json:"validated_on,omitempty"`
}

// handleAgentValidateRecipe is a dry-run that checks a candidate against a
// live device's topology (the seed of "agents testing on hardware"). When the
// gate passes against real metrics, the candidate earns validated status —
// evidence-based promotion, never a claim.
func (d *Daemon) handleAgentValidateRecipe(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("recipeID")
	deviceID := r.URL.Query().Get("device_id")
	if d.recipeStore == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not_found", "message": "recipe store is not loaded"})
		return
	}
	rec, ok := d.recipeStore.Get(id)
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not_found", "message": "no candidate recipe with that id"})
		return
	}

	report := recipeValidationReport{RecipeID: id, DeviceID: deviceID, Status: string(rec.Status)}
	var vramGB float64
	var gpuCount int
	if deviceID != "" {
		if gpus, err := d.fetchAgentGPUs(deviceID); err == nil && len(gpus) > 0 {
			report.LiveMetrics = true
			vramGB = float64(gpus[0].VRAMTotalMB) / 1024.0
			gpuCount = len(gpus)
			for _, gpu := range gpus[1:] {
				if v := float64(gpu.VRAMTotalMB) / 1024.0; v < vramGB {
					vramGB = v
				}
			}
		} else if err != nil {
			report.Reasons = append(report.Reasons, "device not reachable for live metrics: "+err.Error())
		}
	}

	if report.LiveMetrics {
		if rec.Config.MinVRAMGBPerGPU > 0 && vramGB < rec.Config.MinVRAMGBPerGPU {
			report.Fits = false
			report.Reasons = append(report.Reasons, fmt.Sprintf("needs %.0fGB/GPU, device has %.0fGB", rec.Config.MinVRAMGBPerGPU, vramGB))
		} else if rec.Config.MinGPUCount > 0 && gpuCount < rec.Config.MinGPUCount {
			report.Fits = false
			report.Reasons = append(report.Reasons, fmt.Sprintf("needs %d GPU(s), device has %d", rec.Config.MinGPUCount, gpuCount))
		} else {
			report.Fits = true
		}
		if len(rec.Config.TargetDevices) > 0 {
			report.Reasons = append(report.Reasons, "target_devices declared: "+strings.Join(rec.Config.TargetDevices, ", "))
		}
		if report.Fits {
			// Evidence-based promotion: real metrics passed the hardware gate.
			rec.Status = recipes.StatusValidated
			rec.ValidatedOn = appendUnique(rec.ValidatedOn, deviceID)
			rec.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
			if err := d.recipeStore.Put(rec); err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "store_write_failed", "message": err.Error()})
				return
			}
			report.Status = string(rec.Status)
			report.ValidatedOn = rec.ValidatedOn
			report.Reasons = append(report.Reasons, "hardware gate passed against live metrics; promoted to validated")
		}
	} else {
		report.Reasons = append(report.Reasons, "no live device metrics; affinity unverified (must pass a live probe before promotion)")
	}
	writeJSON(w, http.StatusOK, report)
}

func appendUnique(in []string, v string) []string {
	for _, s := range in {
		if s == v {
			return in
		}
	}
	return append(in, v)
}

// randHex returns n random hex characters for recipe id generation.
func randHex(n int) string {
	const hexDigits = "0123456789abcdef"
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		// Fallback to time-based entropy so id generation never blocks.
		seed := uint64(time.Now().UnixNano())
		for i := range b {
			b[i] = hexDigits[seed%16]
			seed /= 16
		}
		return string(b)
	}
	for i := range b {
		b[i] = hexDigits[int(b[i])%16]
	}
	return string(b)
}
