package daemon

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/spencerbull/yokai/internal/config"
	"github.com/spencerbull/yokai/internal/recipes"
)

func testRecipeDaemon(t *testing.T) (*Daemon, *http.ServeMux) {
	t.Helper()
	d := &Daemon{cfg: config.DefaultConfig()}
	store, err := recipes.OpenStore(filepath.Join(t.TempDir(), "recipes.json"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	d.recipeStore = store
	mux := http.NewServeMux()
	mux.HandleFunc("GET /agent/catalog", d.handleAgentCatalog)
	mux.HandleFunc("GET /agent/recommend", d.handleAgentRecommend)
	mux.HandleFunc("GET /agent/recipes", d.handleAgentListRecipes)
	mux.HandleFunc("POST /agent/recipe", d.handleAgentProposeRecipe)
	mux.HandleFunc("GET /agent/recipe/{recipeID}", d.handleAgentGetRecipe)
	mux.HandleFunc("PATCH /agent/recipe/{recipeID}", d.handleAgentPatchRecipe)
	mux.HandleFunc("POST /agent/recipe/{recipeID}/validate", d.handleAgentValidateRecipe)
	mux.HandleFunc("POST /agent/recipe/{recipeID}/verify", d.handleAgentVerifyRecipe)
	mux.HandleFunc("POST /agent/recipe/{recipeID}/inspect", d.handleAgentInspectRecipe)
	mux.HandleFunc("POST /agent/swap", d.handleAgentSwapRecipe)
	return d, mux
}

func probeJSON(t *testing.T, mux *http.ServeMux, method, path string, body any, code int) map[string]any {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			t.Fatalf("encode body: %v", err)
		}
	}
	req := httptest.NewRequest(method, path, &buf)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != code {
		t.Fatalf("%s %s: expected %d, got %d (%s)", method, path, code, rec.Code, rec.Body.String())
	}
	var out map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	return out
}

func validProposedRecipe() map[string]any {
	return map[string]any{
		"proposed_by": "hermes",
		"config": map[string]any{
			"model_id":            "qwen/qwen3-coder-30b-a3b-instruct",
			"workload":            "vllm",
			"image":               "vllm/vllm-openai@sha256:deadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeef",
			"port":                "8000",
			"min_vram_gb_per_gpu": 24,
			"min_gpu_count":       1,
			"quantization":        "FP8",
			"target_devices":      []string{"rtx-4090"},
		},
		"provenance": map[string]any{
			"agent": "hermes", "source": "upstream/repo @ abc123",
		},
	}
}

func TestProposeRecipeLifecycle(t *testing.T) {
	_, mux := testRecipeDaemon(t)

	created := probeJSON(t, mux, "POST", "/agent/recipe", validProposedRecipe(), http.StatusCreated)
	if created["tier"] != string(recipes.TierCandidate) {
		t.Fatalf("expected tier candidate, got %v", created["tier"])
	}
	if created["status"] != string(recipes.StatusProposed) {
		t.Fatalf("expected status proposed, got %v", created["status"])
	}
	id, _ := created["id"].(string)
	if id == "" {
		t.Fatalf("expected generated id")
	}

	// Get it back.
	got := probeJSON(t, mux, "GET", "/agent/recipe/"+id, nil, http.StatusOK)
	if got["id"] != id {
		t.Fatalf("get returned wrong recipe: %v", got["id"])
	}

	// Duplicate fingerprint -> 409.
	probeJSON(t, mux, "POST", "/agent/recipe", validProposedRecipe(), http.StatusConflict)

	// Invalid (mutable image) -> 422.
	bad := validProposedRecipe()
	bad["config"].(map[string]any)["image"] = "vllm/vllm-openai:latest"
	probeJSON(t, mux, "POST", "/agent/recipe", bad, http.StatusUnprocessableEntity)

	// List shows the candidate.
	list := probeJSON(t, mux, "GET", "/agent/recipes", nil, http.StatusOK)
	if int(list["count"].(float64)) != 1 {
		t.Fatalf("expected 1 candidate, got %v", list["count"])
	}
}

func TestPatchRecipeResetsStatus(t *testing.T) {
	_, mux := testRecipeDaemon(t)
	created := probeJSON(t, mux, "POST", "/agent/recipe", validProposedRecipe(), http.StatusCreated)
	id, _ := created["id"].(string)

	// Try to force status validated via PATCH -> must be reset to proposed.
	patched := probeJSON(t, mux, "PATCH", "/agent/recipe/"+id, map[string]any{
		"config": map[string]any{
			"model_id": "qwen/qwen3-coder-30b-a3b-instruct",
			"workload": "vllm",
			"image":    "vllm/vllm-openai@sha256:deadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeef",
		},
	}, http.StatusOK)
	if patched["status"] != string(recipes.StatusProposed) {
		t.Fatalf("expected PATCH to keep/reset status to proposed, got %v", patched["status"])
	}
}

func TestValidateRecipeWithoutLiveDevice(t *testing.T) {
	_, mux := testRecipeDaemon(t)
	created := probeJSON(t, mux, "POST", "/agent/recipe", validProposedRecipe(), http.StatusCreated)
	id, _ := created["id"].(string)

	report := probeJSON(t, mux, "POST", "/agent/recipe/"+id+"/validate", nil, http.StatusOK)
	if report["live_metrics"] != false {
		t.Fatalf("expected no live metrics, got %v", report["live_metrics"])
	}
	if report["status"] != string(recipes.StatusProposed) {
		t.Fatalf("expected status to remain proposed without evidence, got %v", report["status"])
	}
}

func TestCandidateAppearsInCatalogWithTier(t *testing.T) {
	d, mux := testRecipeDaemon(t)
	_ = d
	probeJSON(t, mux, "POST", "/agent/recipe", validProposedRecipe(), http.StatusCreated)

	catalog := probeJSON(t, mux, "GET", "/agent/catalog", nil, http.StatusOK)
	items := catalog["catalog"].([]any)
	sawCandidate := false
	for _, item := range items {
		rec := item.(map[string]any)
		if rec["tier"] == string(recipes.TierCandidate) {
			sawCandidate = true
			if rec["status"] != string(recipes.StatusProposed) {
				t.Fatalf("candidate should be proposed, got %v", rec["status"])
			}
			if rec["provenance"] == nil {
				t.Fatalf("candidate missing provenance")
			}
		}
	}
	if !sawCandidate {
		t.Fatalf("expected catalog to include the candidate")
	}
}

func TestVerifyRecipeRequiresDevice(t *testing.T) {
	_, mux := testRecipeDaemon(t)
	created := probeJSON(t, mux, "POST", "/agent/recipe", validProposedRecipe(), http.StatusCreated)
	id, _ := created["id"].(string)

	out := probeJSON(t, mux, "POST", "/agent/recipe/"+id+"/verify", nil, http.StatusBadRequest)
	if out["error"] != "device_required" {
		t.Fatalf("expected device_required, got %v", out["error"])
	}
}

func TestVerifyRecipeNotFound(t *testing.T) {
	_, mux := testRecipeDaemon(t)
	out := probeJSON(t, mux, "POST", "/agent/recipe/nope/verify?device_id=dell-pro-max", nil, http.StatusNotFound)
	if out["error"] != "not_found" {
		t.Fatalf("expected not_found, got %v", out["error"])
	}
}

// TestVerifyRecipeDeviceUnreachable exercises the live hardware gate: with no
// reachable device, verify must refuse (and record evidence) rather than
// promote. Promotion is evidence-only, so a candidate must never be validated
// without a live probe passing.
func TestVerifyRecipeDeviceUnreachable(t *testing.T) {
	d, mux := testRecipeDaemon(t)
	created := probeJSON(t, mux, "POST", "/agent/recipe", validProposedRecipe(), http.StatusCreated)
	id, _ := created["id"].(string)

	out := probeJSON(t, mux, "POST", "/agent/recipe/"+id+"/verify?device_id=dell-pro-max", nil, http.StatusUnprocessableEntity)
	if out["error"] != "device_unreachable" {
		t.Fatalf("expected device_unreachable, got %v", out["error"])
	}

	// The failed gate must be recorded as server-side evidence and the
	// candidate must remain proposed (no promotion without a live probe).
	rec := probeJSON(t, mux, "GET", "/agent/recipe/"+id, nil, http.StatusOK)
	if rec["status"] != string(recipes.StatusProposed) {
		t.Fatalf("candidate must stay proposed, got %v", rec["status"])
	}
	if rec["last_verify"] == nil {
		t.Fatalf("expected last_verify evidence to be recorded")
	}
	lv := rec["last_verify"].(map[string]any)
	if lv["ok"] != false || lv["device_id"] != "dell-pro-max" {
		t.Fatalf("unexpected last_verify evidence: %v", lv)
	}
	_ = d
}

func TestInspectRecipeNoDevice(t *testing.T) {
	_, mux := testRecipeDaemon(t)
	created := probeJSON(t, mux, "POST", "/agent/recipe", validProposedRecipe(), http.StatusCreated)
	id, _ := created["id"].(string)

	out := probeJSON(t, mux, "POST", "/agent/recipe/"+id+"/inspect", nil, http.StatusOK)
	if out["status"] != string(recipes.StatusProposed) {
		t.Fatalf("inspect must not change status, got %v", out["status"])
	}
	if out["claimed_min_vram_gb"] == nil {
		t.Fatalf("expected claimed requirements reported")
	}
}

// TestInspectDoesNotPromote ensures the read-only inspect never promotes even
// with an unreachable/failed device gate (promotion is verify-exclusive).
func TestInspectDoesNotPromote(t *testing.T) {
	d, mux := testRecipeDaemon(t)
	created := probeJSON(t, mux, "POST", "/agent/recipe", validProposedRecipe(), http.StatusCreated)
	id, _ := created["id"].(string)

	probeJSON(t, mux, "POST", "/agent/recipe/"+id+"/inspect?device_id=some-device", nil, http.StatusOK)
	rec := probeJSON(t, mux, "GET", "/agent/recipe/"+id, nil, http.StatusOK)
	if rec["status"] != string(recipes.StatusProposed) {
		t.Fatalf("inspect promoted a candidate to %v; must stay proposed", rec["status"])
	}
	_ = d
}

func TestSwapRecipeMissingFields(t *testing.T) {
	_, mux := testRecipeDaemon(t)
	out := probeJSON(t, mux, "POST", "/agent/swap", map[string]any{"recipe_id": "rec_x"}, http.StatusBadRequest)
	if out["error"] != "recipe_and_device_required" {
		t.Fatalf("expected recipe_and_device_required, got %v", out["error"])
	}
}

func TestSwapRecipeRequiresValidated(t *testing.T) {
	d, mux := testRecipeDaemon(t)
	created := probeJSON(t, mux, "POST", "/agent/recipe", validProposedRecipe(), http.StatusCreated)
	id, _ := created["id"].(string)

	out := probeJSON(t, mux, "POST", "/agent/swap", map[string]any{
		"recipe_id": id, "device_id": "dell-pro-max",
	}, http.StatusUnprocessableEntity)
	if out["error"] != "require_verify" {
		t.Fatalf("expected require_verify, got %v", out["error"])
	}
	_ = d
}

// TestSwapRecipeSelfHostedBlocked is the user's concern: the acting agent is
// backed by the model running on the device undergoing the swap. The guard must
// refuse (self_hosted_swap) unless allow_self=true — and it runs BEFORE the
// validated gate, so no swap can silently cut its own brain off.
func TestSwapRecipeSelfHostedBlocked(t *testing.T) {
	d := &Daemon{cfg: config.DefaultConfig()}
	store, err := recipes.OpenStore(filepath.Join(t.TempDir(), "recipes.json"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	d.recipeStore = store
	// The target device currently serves the agent's own model.
	d.cfg.Services = append(d.cfg.Services, config.Service{ID: "brain", DeviceID: "dell-pro-max", Model: "TRAVIS_deepseek"})
	mux := http.NewServeMux()
	mux.HandleFunc("POST /agent/recipe", d.handleAgentProposeRecipe)
	mux.HandleFunc("POST /agent/swap", d.handleAgentSwapRecipe)

	created := probeJSON(t, mux, "POST", "/agent/recipe", validProposedRecipe(), http.StatusCreated)
	id, _ := created["id"].(string)

	// Without acknowledgment: refused, even though the recipe is only proposed
	// (the self-host guard runs before the validated gate).
	out := probeJSON(t, mux, "POST", "/agent/swap", map[string]any{
		"recipe_id": id, "device_id": "dell-pro-max", "agent_backing_model": "TRAVIS_deepseek",
	}, http.StatusConflict)
	if out["error"] != "self_hosted_swap" {
		t.Fatalf("expected self_hosted_swap, got %v", out["error"])
	}

	// With acknowledgment: passes the guard and proceeds to the validated gate.
	out2 := probeJSON(t, mux, "POST", "/agent/swap", map[string]any{
		"recipe_id": id, "device_id": "dell-pro-max", "agent_backing_model": "TRAVIS_deepseek", "allow_self": true,
	}, http.StatusUnprocessableEntity)
	if out2["error"] != "require_verify" {
		t.Fatalf("expected require_verify after self-host acknowledged, got %v", out2["error"])
	}
}
