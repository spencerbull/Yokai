package recipes

import (
	"path/filepath"
	"strings"
	"testing"
)

func validConfig() RecipeConfig {
	return RecipeConfig{
		ModelID:         "qwen/qwen3-coder-30b-a3b-instruct",
		Workload:        "vllm",
		Image:           "vllm/vllm-openai@sha256:deadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeef",
		Port:            "8000",
		MinVRAMGBPerGPU: 24,
		MinGPUCount:     1,
		Quantization:    "FP8",
		TargetDevices:   []string{"rtx-4090"},
	}
}

func validProvenance() Provenance {
	return Provenance{Agent: "hermes", Source: "upstream/repo @ abc123", ReportedOn: []string{"rtx-4090"}}
}

func TestValidatorAcceptsGoodRecipe(t *testing.T) {
	t.Parallel()
	res := NewValidator().Validate(validConfig(), validProvenance(), "hermes")
	if !res.OK {
		t.Fatalf("expected OK, got %+v", res)
	}
	if len(res.Errors) != 0 {
		t.Fatalf("expected no errors, got %v", res.Errors)
	}
}

func TestValidatorRejectsMutableImage(t *testing.T) {
	t.Parallel()
	cfg := validConfig()
	cfg.Image = "vllm/vllm-openai:latest"
	res := NewValidator().Validate(cfg, validProvenance(), "hermes")
	if res.OK {
		t.Fatalf("expected rejection of mutable image tag")
	}
	if !strings.Contains(strings.Join(res.Errors, ";"), "digest-pinned") {
		t.Fatalf("expected digest-pinned error, got %v", res.Errors)
	}
}

func TestValidatorRequiresProvenanceAndProposer(t *testing.T) {
	t.Parallel()
	v := NewValidator()
	// No proposer.
	if res := v.Validate(validConfig(), validProvenance(), ""); res.OK {
		t.Fatalf("expected rejection without proposed_by")
	}
	// No source.
	if res := v.Validate(validConfig(), Provenance{Agent: "hermes"}, "hermes"); res.OK {
		t.Fatalf("expected rejection without provenance source")
	}
}

func TestValidatorRejectsUnknownPlugin(t *testing.T) {
	t.Parallel()
	cfg := validConfig()
	cfg.Plugins = []string{"no-such-plugin"}
	res := NewValidator().Validate(cfg, validProvenance(), "hermes")
	if res.OK {
		t.Fatalf("expected rejection of unknown plugin")
	}
}

func TestValidatorAcceptsKnownPlugin(t *testing.T) {
	t.Parallel()
	cfg := validConfig()
	cfg.Plugins = []string{"vllm-reasoning-parser-super-v3"}
	res := NewValidator().Validate(cfg, validProvenance(), "hermes")
	if !res.OK {
		t.Fatalf("expected known plugin to pass, got %+v", res)
	}
}

func TestValidatorRejectsBadWorkload(t *testing.T) {
	t.Parallel()
	cfg := validConfig()
	cfg.Workload = "definitely-not-real"
	res := NewValidator().Validate(cfg, validProvenance(), "hermes")
	if res.OK {
		t.Fatalf("expected rejection of unknown workload")
	}
}

func TestFingerprintStableAndSensitive(t *testing.T) {
	t.Parallel()
	a := validConfig()
	b := validConfig()
	if Fingerprint(a) != Fingerprint(b) {
		t.Fatalf("expected equal fingerprints for identical configs")
	}
	// Changing model id must change the fingerprint.
	c := validConfig()
	c.ModelID = "qwen/qwen3-coder-7b"
	if Fingerprint(a) == Fingerprint(c) {
		t.Fatalf("expected different fingerprints when model changes")
	}
	// Env map order must not matter.
	d := validConfig()
	d.Env = map[string]string{"A": "1", "B": "2"}
	e := validConfig()
	e.Env = map[string]string{"B": "2", "A": "1"}
	if Fingerprint(d) != Fingerprint(e) {
		t.Fatalf("expected fingerprint to be order-independent for env maps")
	}
}

func TestStoreRoundTripAndUpsert(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "recipes.json")
	store, err := OpenStore(path)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	r := Recipe{
		ID: "rec_test", Tier: TierCandidate, Status: StatusProposed,
		Config: validConfig(), Provenance: validProvenance(),
		Fingerprint: Fingerprint(validConfig()), ProposedBy: "hermes",
	}
	if err := store.Put(r); err != nil {
		t.Fatalf("put: %v", err)
	}
	got, ok := store.Get("rec_test")
	if !ok {
		t.Fatalf("expected to find recipe")
	}
	if got.Fingerprint != r.Fingerprint {
		t.Fatalf("fingerprint mismatch")
	}
	if _, found := store.FindFingerprint(r.Fingerprint); !found {
		t.Fatalf("expected fingerprint lookup to succeed")
	}
	// Upsert replaces.
	r.Status = StatusValidated
	if err := store.Put(r); err != nil {
		t.Fatalf("reput: %v", err)
	}
	if len(store.List()) != 1 {
		t.Fatalf("expected single recipe after upsert, got %d", len(store.List()))
	}
	// Reopen from disk.
	reopened, err := OpenStore(path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	if got, ok := reopened.Get("rec_test"); ok && got.Status != StatusValidated {
		t.Fatalf("expected persisted status validated, got %s", got.Status)
	}
}
