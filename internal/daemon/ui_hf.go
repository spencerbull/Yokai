package daemon

import (
	"net/http"
	"sort"
	"strings"

	"github.com/spencerbull/yokai/internal/hf"
)

func (d *Daemon) currentHFToken() string {
	d.mu.RLock()
	token := d.cfg.HFToken
	d.mu.RUnlock()
	if token == "" {
		token = loadHFTokenFromEnv()
	}
	return token
}

func (d *Daemon) handleHFGGUFVariants(w http.ResponseWriter, r *http.Request) {
	model := strings.TrimSpace(r.URL.Query().Get("model"))
	if model == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error":   "bad_request",
			"message": "model query param required",
		})
		return
	}

	variants, err := hf.NewClient(d.currentHFToken()).ListGGUFVariants(model)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{
			"error":   "hf_variants_failed",
			"message": err.Error(),
		})
		return
	}

	if variants == nil {
		variants = []hf.GGUFVariant{}
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"model":    model,
		"variants": variants,
	})
}

func (d *Daemon) handleHFModels(w http.ResponseWriter, r *http.Request) {
	query := strings.TrimSpace(r.URL.Query().Get("query"))
	if len(query) < 2 {
		writeJSON(w, http.StatusOK, map[string]interface{}{"models": []hf.Model{}})
		return
	}

	workload := strings.TrimSpace(r.URL.Query().Get("workload"))
	var filters []string
	if workload == "vllm" || workload == "sglang" {
		// vLLM/SGLang can serve both text-only and multimodal (vision/omni)
		// LLMs, so surface both pipeline tags in the search. HF's `filter`
		// accepts a single pipeline tag, so search each and merge. This makes
		// models tagged image-text-to-text (e.g. RadixArk/Qwen3.8-27B-
		// NVFP4-BF16-LMHead) appear in the deploy search bar.
		filters = []string{"text-generation", "image-text-to-text"}
	}

	models, err := d.searchModelsMerged(query, filters, 30)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{
			"error":   "hf_search_failed",
			"message": err.Error(),
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{"models": models})
}

// searchModelsMerged runs one HF search per pipeline filter and returns the
// results deduplicated by model ID, sorted by likes (descending), capped at
// limit. An empty filters slice performs a single unfiltered search (current
// behavior for non-vLLM/SGLang workloads such as llamacpp/comfyui). Pipeline
// searches degrade independently: a transient 429/5xx/timeout on one pipeline
// does not discard a successful sibling pipeline.
func (d *Daemon) searchModelsMerged(query string, filters []string, limit int) ([]hf.Model, error) {
	if len(filters) == 0 {
		return hf.NewClient(d.currentHFToken()).SearchModelsWithOptions(query, hf.SearchOptions{Limit: limit, Filter: ""})
	}

	groups := make([][]hf.Model, 0, len(filters))
	errs := make([]error, 0, len(filters))
	for _, f := range filters {
		models, err := hf.NewClient(d.currentHFToken()).SearchModelsWithOptions(query, hf.SearchOptions{Limit: limit, Filter: f})
		groups = append(groups, models)
		errs = append(errs, err)
	}
	return mergeModelsBestEffort(groups, errs, limit)
}

// mergeModelsBestEffort merges per-pipeline groups, tolerating failed groups.
// errs is parallel to groups: nil marks a successful pipeline (including one
// that returned zero matches), a non-nil error a failed pipeline. It returns
// the merged successful results (deduped, by likes, capped) unless every
// pipeline failed, in which case it returns the first error.
func mergeModelsBestEffort(groups [][]hf.Model, errs []error, limit int) ([]hf.Model, error) {
	anysuccess := false
	for _, err := range errs {
		if err == nil {
			anysuccess = true
			break
		}
	}
	if !anysuccess {
		if len(errs) > 0 {
			return nil, errs[0]
		}
		return []hf.Model{}, nil
	}
	return mergeModelsByLikes(groups, limit), nil
}

// mergeModelsByLikes merges per-pipeline search results, dedupes by model ID,
// orders by likes (descending), and caps at limit.
func mergeModelsByLikes(results [][]hf.Model, limit int) []hf.Model {
	seen := make(map[string]bool)
	merged := make([]hf.Model, 0, limit)
	for _, group := range results {
		for _, m := range group {
			if seen[m.ID] {
				continue
			}
			seen[m.ID] = true
			merged = append(merged, m)
		}
	}
	sort.SliceStable(merged, func(i, j int) bool { return merged[i].Likes > merged[j].Likes })
	if len(merged) > limit {
		merged = merged[:limit]
	}
	return merged
}
