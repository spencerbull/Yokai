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
