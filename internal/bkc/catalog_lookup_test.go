package bkc

import "testing"

func TestLookupBestSuggestedNemotronSuperVariant(t *testing.T) {
	cfg, match, ok := LookupBest(WorkloadVLLM, "nvidia/NVIDIA-Nemotron-3-Super-49B-v1")
	if !ok {
		t.Fatal("expected a suggested BKC match")
	}
	if match != MatchSuggested {
		t.Fatalf("expected suggested match, got %q", match)
	}
	if cfg.ID != "nemotron-3-super-nvfp4" {
		t.Fatalf("expected nemotron BKC, got %q", cfg.ID)
	}
}
