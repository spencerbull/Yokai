package hfmem

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
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

const defaultUVVersion = "0.11.3"

func EstimateModel(modelID, token string, maxModelLen int, kvCacheDType string) (*Estimate, error) {
	args, err := buildCommandArgs(modelID, token, maxModelLen, kvCacheDType)
	if err != nil {
		return nil, err
	}

	cmd := command(args[0], args[1:]...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return nil, formatRunError(err, stderr.String())
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

func buildCommandArgs(modelID, token string, maxModelLen int, kvCacheDType string) ([]string, error) {
	base := []string{"hf-mem", "--model-id", modelID, "--experimental", "--max-model-len", strconv.Itoa(maxModelLen), "--json-output"}
	if strings.TrimSpace(kvCacheDType) != "" {
		base = append(base, "--kv-cache-dtype", strings.TrimSpace(kvCacheDType))
	}
	if token != "" {
		base = append(base, "--hf-token", token)
	}

	if _, err := lookPath("hf-mem"); err == nil {
		return base, nil
	}
	if _, err := lookPath("uvx"); err == nil {
		return append([]string{"uvx"}, base...), nil
	}
	if _, err := lookPath("mise"); err == nil {
		return append([]string{"mise", "exec", "uv@" + defaultUVVersion, "--", "uvx"}, base...), nil
	}

	return nil, fmt.Errorf("hf-mem not installed (tried 'hf-mem', 'uvx hf-mem', and 'mise exec uv@%s -- uvx hf-mem')", defaultUVVersion)
}

func formatRunError(err error, stderr string) error {
	stderr = strings.TrimSpace(stderr)
	if stderr == "" {
		return fmt.Errorf("running hf-mem: %w", err)
	}

	switch {
	case strings.Contains(stderr, "401 Unauthorized"):
		return fmt.Errorf("running hf-mem: Hugging Face returned 401 Unauthorized; check your HF token and access to the selected model")
	case strings.Contains(stderr, "403 Forbidden"):
		return fmt.Errorf("running hf-mem: Hugging Face returned 403 Forbidden; request access to the selected model and verify your HF token")
	case strings.Contains(stderr, "404 Not Found"):
		return fmt.Errorf("running hf-mem: Hugging Face returned 404 Not Found; verify the selected model ID")
	case strings.Contains(stderr, "quant_method different than `fp8`") || strings.Contains(stderr, "quant_method different than `fp8`"):
		return fmt.Errorf("running hf-mem: this model needs an explicit --kv-cache-dtype (for example from the BKC preset); apply BKC first or add the kv-cache dtype manually")
	}

	lines := strings.Split(stderr, "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if line != "" {
			return fmt.Errorf("running hf-mem: %s", line)
		}
	}

	return fmt.Errorf("running hf-mem: %w", err)
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
