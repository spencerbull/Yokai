package daemon

import (
	"net/http"
	"strings"

	"github.com/spencerbull/yokai/internal/hf"
)

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

	token := ""
	d.mu.RLock()
	token = d.cfg.HFToken
	d.mu.RUnlock()
	if token == "" {
		token = loadHFTokenFromEnv()
	}

	models, err := hf.NewClient(token).SearchModelsWithOptions(query, hf.SearchOptions{
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
