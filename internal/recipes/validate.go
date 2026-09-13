package recipes

import (
	"strings"

	"github.com/spencerbull/yokai/internal/bkc"
	"github.com/spencerbull/yokai/internal/plugins"
)

// ValidationResult reports whether a proposed recipe is safe to store as a
// candidate, plus human-readable errors/warnings. Errors block ingest;
// warnings are informational and never block.
type ValidationResult struct {
	OK       bool     `json:"ok"`
	Errors   []string `json:"errors,omitempty"`
	Warnings []string `json:"warnings,omitempty"`
}

// Validator enforces the ingest gate for candidate recipes. The rules are
// intentionally conservative: a bad candidate is cheaper to reject than a
// fleet-wide misconfiguration. Trust tiers are not consulted here — every
// writer (human or agent) gets the same gate.
type Validator struct {
	pluginExists func(string) bool
}

func NewValidator() *Validator {
	return &Validator{pluginExists: func(id string) bool { _, ok := plugins.Lookup(id); return ok }}
}

func (v *Validator) Validate(c RecipeConfig, prov Provenance, proposedBy string) ValidationResult {
	res := ValidationResult{OK: true}

	if strings.TrimSpace(c.ModelID) == "" {
		res.Errors = append(res.Errors, "config.model_id is required")
	}
	if strings.TrimSpace(c.Workload) == "" {
		res.Errors = append(res.Errors, "config.workload is required")
	} else if !knownWorkload(c.Workload) {
		res.Errors = append(res.Errors, "config.workload must be one of vllm, sglang, llamacpp, comfyui (got "+c.Workload+")")
	}

	// Image must be digest-pinned for candidates so an agent can never point a
	// deployment at a mutable tag that silently moves underneath validation.
	if strings.TrimSpace(c.Image) == "" {
		res.Errors = append(res.Errors, "config.image is required")
	} else if !isDigestPinned(c.Image) {
		res.Errors = append(res.Errors, "config.image must be digest-pinned (reference like repo/img@sha256:...) so validation is reproducible")
	}

	if c.MinVRAMGBPerGPU < 0 {
		res.Errors = append(res.Errors, "config.min_vram_gb_per_gpu cannot be negative")
	}
	if c.MinGPUCount < 0 {
		res.Errors = append(res.Errors, "config.min_gpu_count cannot be negative")
	}
	if c.MinGPUCount > 1 && strings.TrimSpace(c.ExtraArgs) == "" && c.MultiDevice == nil {
		res.Warnings = append(res.Warnings, "min_gpu_count > 1 but no tensor-parallel args or multi-device plan declared")
	}

	if c.Quantization != "" && !knownQuantization(c.Quantization) {
		res.Warnings = append(res.Warnings, "config.quantization is not a recognized value: "+c.Quantization)
	}
	for _, target := range c.TargetDevices {
		if !knownDevice(target) {
			res.Warnings = append(res.Warnings, "config.target_devices contains unrecognized profile "+target)
		}
	}
	if c.Port != "" && !isNumeric(c.Port) {
		res.Warnings = append(res.Warnings, "config.port should be a numeric string: "+c.Port)
	}

	for _, pluginID := range c.Plugins {
		if !v.pluginExists(pluginID) {
			res.Errors = append(res.Errors, "config.plugins references unknown plugin "+pluginID)
		}
	}

	if strings.TrimSpace(proposedBy) == "" {
		res.Errors = append(res.Errors, "proposed_by is required (the writing agent/human identity)")
	}
	if prov.Agent == "" {
		res.Warnings = append(res.Warnings, "provenance.agent is empty")
	}
	if strings.TrimSpace(prov.Source) == "" && strings.TrimSpace(prov.SourceURL) == "" {
		res.Errors = append(res.Errors, "provenance must include at least one of source or source_url")
	}

	res.OK = len(res.Errors) == 0
	return res
}

func knownWorkload(w string) bool {
	switch strings.ToLower(strings.TrimSpace(w)) {
	case string(bkc.WorkloadVLLM), string(bkc.WorkloadSGLang), string(bkc.WorkloadLlamaCpp), "comfyui":
		return true
	}
	return false
}

func knownQuantization(q string) bool {
	switch strings.ToUpper(strings.TrimSpace(q)) {
	case bkc.QuantBF16, bkc.QuantFP16, bkc.QuantFP8, bkc.QuantFP4, bkc.QuantNVFP4, bkc.QuantMXFP4, bkc.QuantINT4:
		return true
	}
	return false
}

// knownDevice mirrors the bkc device-profile constants so candidates are
// tagged with profiles the rest of yokai understands.
func knownDevice(d string) bool {
	switch strings.ToLower(strings.TrimSpace(d)) {
	case bkc.DeviceGB10, bkc.DeviceJetsonThor, bkc.DeviceRTXPRO6000, bkc.DeviceRTX5090,
		bkc.DeviceRTX4090, bkc.DeviceL40S, bkc.DeviceA100_80, bkc.DeviceH100_80,
		bkc.DeviceH100_94, bkc.DeviceH200, bkc.DeviceH20, bkc.DeviceB200, bkc.DeviceGB200,
		bkc.DeviceMI300X, bkc.DeviceMI325X, bkc.DeviceMI355X, bkc.DeviceR9700:
		return true
	}
	return false
}

func isDigestPinned(image string) bool {
	return strings.Contains(image, "@sha256:")
}

func isNumeric(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
