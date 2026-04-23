package daemon

import (
	"net/http"
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
	filter := ""
	if workload == "vllm" {
		filter = "text-generation"
	}

	models, err := hf.NewClient(d.currentHFToken()).SearchModelsWithOptions(query, hf.SearchOptions{
		Limit:  30,
		Filter: filter,
	})
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{
			"error":   "hf_search_failed",
			"message": err.Error(),
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{"models": models})
}
