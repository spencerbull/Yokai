package daemon

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/spencerbull/yokai/internal/bkc"
	"github.com/spencerbull/yokai/internal/config"
)

func TestHandleAgentCatalog(t *testing.T) {
	t.Parallel()

	d := &Daemon{}
	req := httptest.NewRequest(http.MethodGet, "/agent/catalog", nil)
	recorder := httptest.NewRecorder()
	d.handleAgentCatalog(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", recorder.Code, recorder.Body.String())
	}
	var response struct {
		Count    int               `json:"count"`
		UseCases []string          `json:"use_cases"`
		Catalog  []deployBKCRecord `json:"catalog"`
	}
	if err := json.NewDecoder(recorder.Body).Decode(&response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.Count == 0 || len(response.Catalog) != response.Count {
		t.Fatalf("expected catalog with count %d, got %d records", response.Count, len(response.Catalog))
	}
	if len(response.UseCases) == 0 {
		t.Fatalf("expected non-empty use case list")
	}
	// Every record should carry a use-case tag (defaults at least to chat).
	for _, rec := range response.Catalog {
		if len(rec.UseCases) == 0 {
			t.Fatalf("record %s has no use cases", rec.ID)
		}
	}
}

func TestHandleAgentRecommendFiltersByUseCase(t *testing.T) {
	t.Parallel()

	d := &Daemon{}
	req := httptest.NewRequest(http.MethodGet, "/agent/recommend?use_case=coding&limit=5", nil)
	recorder := httptest.NewRecorder()
	d.handleAgentRecommend(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", recorder.Code, recorder.Body.String())
	}
	var response agentRecommendResponse
	if err := json.NewDecoder(recorder.Body).Decode(&response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.UseCase != "coding" {
		t.Fatalf("expected use_case coding, got %q", response.UseCase)
	}
	if response.Total == 0 || len(response.Options) == 0 {
		t.Fatalf("expected non-empty coding recommendations, total=%d options=%d", response.Total, len(response.Options))
	}
	if len(response.Options) > 5 {
		t.Fatalf("limit not honored: got %d options", len(response.Options))
	}
	for _, opt := range response.Options {
		if len(opt.UseCases) == 0 {
			t.Fatalf("option %s has no use cases", opt.ID)
		}
		if !sliceContains(opt.UseCases, "coding") {
			t.Fatalf("option %s is not tagged coding (use_cases=%v)", opt.ID, opt.UseCases)
		}
	}
}

func TestHandleAgentRecommendRequiresUseCase(t *testing.T) {
	t.Parallel()

	d := &Daemon{}
	req := httptest.NewRequest(http.MethodGet, "/agent/recommend", nil)
	recorder := httptest.NewRecorder()
	d.handleAgentRecommend(recorder, req)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", recorder.Code)
	}
}

func TestHandleAgentRecommendUnknownDevice(t *testing.T) {
	t.Parallel()

	d := &Daemon{cfg: config.DefaultConfig()}
	req := httptest.NewRequest(http.MethodGet, "/agent/recommend?use_case=coding&device_id=missing", nil)
	recorder := httptest.NewRecorder()
	d.handleAgentRecommend(recorder, req)

	if recorder.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for unknown device, got %d: %s", recorder.Code, recorder.Body.String())
	}
}

func TestFitsCandidate(t *testing.T) {
	t.Parallel()

	// No device context -> never over-filter.
	if !fitsCandidate(bkc.Config{MinVRAMGBPerGPU: 24, MinGPUCount: 2}, 0, 0) {
		t.Fatalf("expected no-device context to pass")
	}
	if fitsCandidate(bkc.Config{MinVRAMGBPerGPU: 24}, 8, 1) {
		t.Fatalf("expected 8GB GPU to fail a 24GB recipe")
	}
	if fitsCandidate(bkc.Config{MinGPUCount: 2}, 24, 1) {
		t.Fatalf("expected 1 GPU to fail a 2-GPU recipe")
	}
	if !fitsCandidate(bkc.Config{MinVRAMGBPerGPU: 24, MinGPUCount: 2}, 24, 2) {
		t.Fatalf("expected adequate hardware to pass")
	}
}

func TestUseCaseCoverageAgainstCatalog(t *testing.T) {
	catalog := bkc.Catalog()
	for _, cfg := range catalog {
		if len(cfg.UseCases()) == 0 {
			t.Fatalf("model %s has no use cases", cfg.ModelID)
		}
	}
}

func sliceContains(list []string, want string) bool {
	for _, v := range list {
		if v == want {
			return true
		}
	}
	return false
}
