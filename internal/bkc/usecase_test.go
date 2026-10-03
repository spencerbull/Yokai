package bkc

import (
	"strings"
	"testing"
)

func TestUseCasesForModelCurated(t *testing.T) {
	t.Parallel()

	cases := []struct {
		model string
		want  string // any one expected use case present
	}{
		{"Qwen/Qwen3-Coder-480B-A35B-Instruct", UseCaseCoding},
		{"Qwen/Qwen2.5-VL-72B-Instruct", UseCaseVision},
		{"jinaai/jina-reranker-m0", UseCaseRerank},
		{"deepseek-ai/DeepSeek-R1-0528", UseCaseReasoning},
		{"google/translategemma-27b-it", UseCaseTranslation},
		{"deepseek-ai/DeepSeek-OCR", UseCaseOCR},
		{"zai-org/GLM-ASR-Nano-2512", UseCaseSpeech},
		{"Qwen/Qwen3Guard-Gen-0.6B", UseCaseGuardrails},
		{"zai-org/Glyph", UseCaseOCR},
		{"moonshotai/Kimi-K2-Thinking", UseCaseReasoning},
	}
	for _, tc := range cases {
		ucs := UseCasesForModel(tc.model)
		found := false
		for _, uc := range ucs {
			if uc == tc.want {
				found = true
			}
		}
		if !found {
			t.Errorf("model %s: expected %s in use cases, got %v", tc.model, tc.want, ucs)
		}
	}
}

func TestUseCasesForModelCaseInsensitive(t *testing.T) {
	t.Parallel()
	upper := UseCasesForModel("Qwen/Qwen3-Coder-480B-A35B-Instruct")
	lower := UseCasesForModel("qwen/qwen3-coder-480b-a35b-instruct")
	if strings.Join(upper, ",") != strings.Join(lower, ",") {
		t.Fatalf("expected case-insensitive lookup: %v vs %v", upper, lower)
	}
	if !contains(upper, UseCaseCoding) {
		t.Fatalf("expected coding, got %v", upper)
	}
}

func TestUseCasesForModelHeuristicFallback(t *testing.T) {
	t.Parallel()
	// Uncurated reasoning model should still be tagged via heuristics.
	ucs := UseCasesForModel("whoever/atomic-thinker-r1-vl")
	if !contains(ucs, UseCaseChat) {
		t.Fatalf("expected chat default, got %v", ucs)
	}
	if !contains(ucs, UseCaseReasoning) {
		t.Fatalf("expected reasoning from heuristic, got %v", ucs)
	}
	if !contains(ucs, UseCaseVision) {
		t.Fatalf("expected vision from heuristic, got %v", ucs)
	}
}

func TestEveryCatalogModelHasUseCases(t *testing.T) {
	t.Parallel()
	for _, cfg := range Catalog() {
		if len(cfg.UseCases()) == 0 {
			t.Fatalf("model %s has zero use cases", cfg.ModelID)
		}
	}
}

func TestAllUseCasesSortedUnique(t *testing.T) {
	t.Parallel()
	list := AllUseCases()
	if len(list) < 5 {
		t.Fatalf("expected several use cases, got %v", list)
	}
	for i := 1; i < len(list); i++ {
		if list[i] <= list[i-1] {
			t.Fatalf("use cases not sorted/deduped: %v", list)
		}
	}
}

func contains(list []string, want string) bool {
	for _, v := range list {
		if v == want {
			return true
		}
	}
	return false
}
