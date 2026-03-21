package hfmem

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"strconv"
)

type Estimate struct {
	WeightsBytes int64
	KVCacheBytes int64
	TotalBytes   int64
}

type cliResult struct {
	Memory      any `json:"memory"`
	KVCache     any `json:"kv_cache"`
	TotalMemory any `json:"total_memory"`
}

var lookPath = exec.LookPath
var command = exec.Command

func EstimateModel(modelID, token string, maxModelLen int) (*Estimate, error) {
	args, err := buildCommandArgs(modelID, token, maxModelLen)
	if err != nil {
		return nil, err
	}

	cmd := command(args[0], args[1:]...)
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("running hf-mem: %w", err)
	}

	var result cliResult
	if err := json.Unmarshal(out, &result); err != nil {
		return nil, fmt.Errorf("parsing hf-mem output: %w", err)
	}

	weights, err := parseJSONInt(result.Memory)
	if err != nil {
		return nil, fmt.Errorf("parsing weights: %w", err)
	}
	kvCache, err := parseJSONInt(result.KVCache)
	if err != nil {
		return nil, fmt.Errorf("parsing kv cache: %w", err)
	}
	total, err := parseJSONInt(result.TotalMemory)
	if err != nil {
		total = weights + kvCache
	}

	return &Estimate{WeightsBytes: weights, KVCacheBytes: kvCache, TotalBytes: total}, nil
}

func buildCommandArgs(modelID, token string, maxModelLen int) ([]string, error) {
	base := []string{"hf-mem", "--model-id", modelID, "--experimental", "--max-model-len", strconv.Itoa(maxModelLen), "--json-output"}
	if token != "" {
		base = append(base, "--hf-token", token)
	}

	if _, err := lookPath("hf-mem"); err == nil {
		return base, nil
	}
	if _, err := lookPath("uvx"); err == nil {
		return append([]string{"uvx"}, base...), nil
	}

	return nil, fmt.Errorf("hf-mem not installed (tried 'hf-mem' and 'uvx hf-mem')")
}

func parseJSONInt(v any) (int64, error) {
	switch x := v.(type) {
	case nil:
		return 0, nil
	case float64:
		return int64(x), nil
	case int64:
		return x, nil
	case int:
		return int64(x), nil
	case json.Number:
		return x.Int64()
	default:
		return 0, fmt.Errorf("unexpected JSON value %T", v)
	}
}
