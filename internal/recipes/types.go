// Package recipes implements a mutable, versioned recipe store that overlays
// the immutable curated BKC catalog. Agents (pi, hermes, opencode, ...) propose
// candidate recipes into the store; recommendation reads the union of curated
// + candidates. Candidates are always second-class: they carry provenance and
// a status, are schema/affinity-validated at ingest, and are never treated as
// deployable truth until they earn a validated status (e.g. a passing live
// probe on real hardware).
package recipes

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"
	"strings"

	"github.com/spencerbull/yokai/internal/bkc"
)

// Tier distinguishes shipped, human-validated recipes from agent-researched,
// unverified candidates. The store only persists candidates; curated entries
// remain embedded in the binary and are joined in at read time.
type Tier string

const (
	TierCurated   Tier = "curated"
	TierCandidate Tier = "candidate"
)

// Status tracks a candidate recipe's lifecycle. Promotion to validated is only
// earned by evidence (a passing live readiness probe on reported hardware),
// never by an agent's claim.
type Status string

const (
	StatusProposed   Status = "proposed"
	StatusValidated  Status = "validated"
	StatusSuperseded Status = "superseded"
)

// Provenance records where a candidate came from and what the research claims,
// so recommendation can show its trust level. Free-text fields are treated as
// untrusted input and never executed or parsed as configuration.
type Provenance struct {
	Agent        string   `json:"agent,omitempty"`         // e.g. "hermes"
	Source       string   `json:"source,omitempty"`        // e.g. "upstream repo @ commit"
	SourceURL    string   `json:"source_url,omitempty"`    // e.g. HF model card
	ResearchNote string   `json:"research_note,omitempty"` // free text, non-executable
	ReportedOn   []string `json:"reported_on,omitempty"`   // device profiles the agent claims tested on
}

// RecipeConfig is the JSON-serializable deployable content of a recipe. It
// mirrors bkc.Config (which deliberately has no json tags) so the store is
// self-contained and daemon-independent.
type RecipeConfig struct {
	ID              string                     `json:"id,omitempty"`
	Name            string                     `json:"name,omitempty"`
	Workload        string                     `json:"workload"`
	ModelID         string                     `json:"model_id"`
	Image           string                     `json:"image"`
	Port            string                     `json:"port,omitempty"`
	ExtraArgs       string                     `json:"extra_args,omitempty"`
	Env             map[string]string          `json:"env,omitempty"`
	Volumes         map[string]string          `json:"volumes,omitempty"`
	Plugins         []string                   `json:"plugins,omitempty"`
	Description     string                     `json:"description,omitempty"`
	TargetDevices   []string                   `json:"target_devices,omitempty"`
	MinVRAMGBPerGPU float64                    `json:"min_vram_gb_per_gpu,omitempty"`
	MinGPUCount     int                        `json:"min_gpu_count,omitempty"`
	Quantization    string                     `json:"quantization,omitempty"`
	Arch            string                     `json:"arch,omitempty"`
	MultiDevice     *bkc.MultiDeviceDeployment `json:"multi_device,omitempty"`
}

// ToBKC converts the stored form back to a bkc.Config so candidates flow
// through the same lookup/deploy primitives as curated recipes.
func (c RecipeConfig) ToBKC() bkc.Config {
	return bkc.Config{
		ID:              c.ID,
		Name:            c.Name,
		Workload:        bkc.Workload(c.Workload),
		ModelID:         c.ModelID,
		Image:           c.Image,
		Port:            c.Port,
		ExtraArgs:       c.ExtraArgs,
		Env:             c.Env,
		Volumes:         c.Volumes,
		Plugins:         c.Plugins,
		Description:     c.Description,
		TargetDevices:   c.TargetDevices,
		MinVRAMGBPerGPU: c.MinVRAMGBPerGPU,
		MinGPUCount:     c.MinGPUCount,
		Quantization:    c.Quantization,
		Arch:            c.Arch,
		MultiDevice:     c.MultiDevice,
	}
}

// RecipeVerify records the outcome of the most recent on-hardware trial. It
// is written only by the daemon's verification path (server-recorded evidence),
// never by a proposing agent. A passing trial is the sole path to validated.
type RecipeVerify struct {
	DeviceID      string `json:"device_id"`
	At            string `json:"at"`
	OK            bool   `json:"ok"`
	ContainerID   string `json:"container_id,omitempty"`
	ServedModelID string `json:"served_model_id,omitempty"`
	MetricsReady  bool   `json:"metrics_ready"`
	Message       string `json:"message,omitempty"`
}

// SwapRecord is a server-written, fleet-level record of a swap of a recipe
// onto a device (deploy-on-verified). It captures the previous deployment that
// became the rollback target, the new container, and whether the self-host
// guard was acknowledged. Written only by the daemon.
type SwapRecord struct {
	DeviceID          string `json:"device_id"`
	At                string `json:"at"`
	PreviousModelID   string `json:"previous_model_id,omitempty"`
	PreviousContainer string `json:"previous_container,omitempty"`
	NewContainerID    string `json:"new_container_id,omitempty"`
	AllowSelf         bool   `json:"allow_self"`
	OK                bool   `json:"ok"`
	Message           string `json:"message,omitempty"`
}

// Recipe is a stored candidate recipe plus its metadata.
type Recipe struct {
	ID          string        `json:"id"`
	Tier        Tier          `json:"tier"`
	Status      Status        `json:"status"`
	Config      RecipeConfig  `json:"config"`
	Provenance  Provenance    `json:"provenance"`
	ValidatedOn []string      `json:"validated_on,omitempty"`
	LastVerify  *RecipeVerify `json:"last_verify,omitempty"`
	SwapHistory []SwapRecord  `json:"swap_history,omitempty"`
	Fingerprint string        `json:"fingerprint"`
	ProposedBy  string        `json:"proposed_by,omitempty"`
	Supersedes  string        `json:"supersedes,omitempty"`
	CreatedAt   string        `json:"created_at,omitempty"`
	UpdatedAt   string        `json:"updated_at,omitempty"`
}

// Fingerprint returns a canonical hash of a recipe config so near-duplicate
// proposals can be deduplicated/merged regardless of who wrote them.
func Fingerprint(c RecipeConfig) string {
	h := sha256.New()
	write := func(parts ...string) { _, _ = h.Write([]byte(strings.Join(parts, "\x00") + "\x1f")) }
	write(c.ModelID, c.Workload, c.Image, c.Port, c.ExtraArgs)

	envKeys := make([]string, 0, len(c.Env))
	for k := range c.Env {
		envKeys = append(envKeys, k)
	}
	sort.Strings(envKeys)
	for _, k := range envKeys {
		write("env", k, c.Env[k])
	}
	volKeys := make([]string, 0, len(c.Volumes))
	for k := range c.Volumes {
		volKeys = append(volKeys, k)
	}
	sort.Strings(volKeys)
	for _, k := range volKeys {
		write("vol", k, c.Volumes[k])
	}
	plugins := append([]string(nil), c.Plugins...)
	sort.Strings(plugins)
	write("plugins", strings.Join(plugins, ","))
	write("targets", strings.Join(c.TargetDevices, ","))
	write("quant", c.Quantization, "arch", c.Arch)
	if c.MultiDevice != nil {
		raw, _ := json.Marshal(c.MultiDevice)
		write("md", string(raw))
	}
	sum := h.Sum(nil)
	return hex.EncodeToString(sum[:])
}
