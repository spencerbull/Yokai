// Package mcp implements a minimal Model Context Protocol server over stdio
// that proxies the yokai daemon's /agent/* read endpoints. It is intentionally
// thin: the daemon owns catalog/topology/recommend logic, this package only
// translates MCP tool calls into daemon HTTP requests. Any MCP-speaking agent
// (pi, opencode, hermes, openclaw, ...) can register this binary.
package mcp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/spencerbull/yokai/internal/config"
)

// ProtocolVersion is the MCP spec version this shim speaks.
const ProtocolVersion = "2024-11-05"

const (
	toolListCatalog    = "list_catalog"
	toolListTopology   = "list_topology"
	toolRecommendModel = "recommend_model"
	toolListCandidates = "list_candidates"
	toolProposeRecipe  = "propose_recipe"
	toolGetRecipe      = "get_recipe"
	toolValidateRecipe = "validate_recipe"
)

type rpcRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type rpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Result  any             `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type toolDefinition struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"inputSchema"`
}

// Server proxies /agent/* daemon endpoints over the MCP stdio transport.
type Server struct {
	baseURL string
	http    *http.Client
}

// New returns an MCP server bound to the local yokai daemon address.
func New() (*Server, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, fmt.Errorf("loading yokai config: %w", err)
	}
	addr := cfg.Daemon.Listen
	if addr == "" {
		addr = "127.0.0.1:7473"
	}
	return &Server{
		baseURL: "http://" + addr,
		http:    &http.Client{Timeout: 60 * time.Second},
	}, nil
}

// Serve reads JSON-RPC messages from stdin (one per line, 2024-11-05 stdio
// transport) and writes responses to stdout.
func (s *Server) Serve() error {
	scanner := bufio.NewScanner(os.Stdin)
	scanner.Buffer(make([]byte, 0, 1<<20), 8<<20)
	writer := bufio.NewWriter(os.Stdout)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		resp := s.handleLine(line)
		if resp == nil {
			continue // notification
		}
		payload, err := json.Marshal(resp)
		if err != nil {
			return err
		}
		if _, err := writer.Write(append(payload, '\n')); err != nil {
			return err
		}
		if err := writer.Flush(); err != nil {
			return err
		}
	}
	return scanner.Err()
}

func (s *Server) handleLine(line string) *rpcResponse {
	var req rpcRequest
	if err := json.Unmarshal([]byte(line), &req); err != nil {
		return &rpcResponse{JSONRPC: "2.0", Error: &rpcError{Code: -32700, Message: "parse error: " + err.Error()}}
	}
	// Notifications carry no id and expect no response.
	isNotification := len(req.ID) == 0 || string(req.ID) == "null"

	switch req.Method {
	case "initialize":
		return &rpcResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result: map[string]any{
				"protocolVersion": ProtocolVersion,
				"capabilities":    map[string]any{"tools": map[string]any{"listChanged": false}},
				"serverInfo":      map[string]any{"name": "yokai-mcp", "version": "0.1.0"},
			},
		}
	case "notifications/initialized", "notifications/cancelled", "notifications/progress":
		return nil
	case "ping":
		return &rpcResponse{JSONRPC: "2.0", ID: req.ID, Result: map[string]any{}}
	case "tools/list":
		return &rpcResponse{JSONRPC: "2.0", ID: req.ID, Result: map[string]any{"tools": s.toolDefinitions()}}
	case "tools/call":
		result, err := s.callTool(req)
		if err != nil {
			return &rpcResponse{JSONRPC: "2.0", ID: req.ID, Error: &rpcError{Code: -32602, Message: err.Error()}}
		}
		return &rpcResponse{JSONRPC: "2.0", ID: req.ID, Result: map[string]any{
			"content": []map[string]any{{"type": "text", "text": result}},
		}}
	default:
		if isNotification {
			return nil
		}
		return &rpcResponse{JSONRPC: "2.0", ID: req.ID, Error: &rpcError{Code: -32601, Message: "method not found: " + req.Method}}
	}
}

func (s *Server) toolDefinitions() []toolDefinition {
	return []toolDefinition{
		{
			Name:        toolListCatalog,
			Description: "List the full Best-Known-Config recipe catalog: models, workloads, images, hardware affinity (target devices, min VRAM/GPU), quantizations, and use cases (coding, chat, reasoning, vision, ocr, translation, speech, rerank, embedding, guardrails).",
			InputSchema: map[string]any{"type": "object", "properties": map[string]any{}},
		},
		{
			Name:        toolListTopology,
			Description: "Describe the fleet: each device, its online status, GPU count, per-GPU VRAM, and the inference services currently running on it.",
			InputSchema: map[string]any{"type": "object", "properties": map[string]any{}},
		},
		{
			Name:        toolRecommendModel,
			Description: "Recommend recipes for a use case. Use cases include: coding, chat, reasoning, vision, ocr, translation, speech, rerank, embedding, guardrails. Optionally pass a device_id (from list_topology) to rank by hardware fit.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"use_case":  map[string]any{"type": "string", "description": "The task to serve, e.g. coding, chat, reasoning, vision."},
					"device_id": map[string]any{"type": "string", "description": "Optional device id to rank suggestions by hardware fit."},
					"limit":     map[string]any{"type": "integer", "description": "Max suggestions to return (default 10)."},
				},
				"required": []string{"use_case"},
			},
		},
		{
			Name:        toolListCandidates,
			Description: "List agent-researched candidate recipes currently in the store (tier=candidate) with their status, fingerprint, and provenance.",
			InputSchema: map[string]any{"type": "object", "properties": map[string]any{}},
		},
		{
			Name:        toolProposeRecipe,
			Description: "Propose a new candidate recipe from research (tier=candidate, status=proposed). Pass proposed_by (your agent name), config (model_id, workload, image [must be digest-pinned repo/img@sha256:...], min_vram_gb_per_gpu, min_gpu_count, quantization, target_devices, plugins), and provenance (source/source_url + research_note). Candidates are server-validated and never auto-deployed.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"proposed_by": map[string]any{"type": "string", "description": "Your agent/human identity."},
					"config":      map[string]any{"type": "object", "description": "Recipe config: model_id, workload, digest-pinned image, min_vram_gb_per_gpu, min_gpu_count, etc."},
					"provenance":  map[string]any{"type": "object", "description": "{agent, source, source_url, research_note, reported_on}."},
				},
				"required": []string{"proposed_by", "config", "provenance"},
			},
		},
		{
			Name:        toolGetRecipe,
			Description: "Fetch a single candidate recipe by id (from list_candidates).",
			InputSchema: map[string]any{
				"type":       "object",
				"properties": map[string]any{"recipe_id": map[string]any{"type": "string", "description": "Candidate recipe id."}},
				"required":   []string{"recipe_id"},
			},
		},
		{
			Name:        toolValidateRecipe,
			Description: "Dry-run a candidate recipe against a real device's live GPU metrics (hardware gate: VRAM/GPU count). Pass device_id from list_topology. If the gate passes against live metrics, the candidate is promoted to validated (evidence-based). No deployment is performed.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"recipe_id": map[string]any{"type": "string", "description": "Candidate recipe id."},
					"device_id": map[string]any{"type": "string", "description": "Device to validate against (live topology)."},
				},
				"required": []string{"recipe_id", "device_id"},
			},
		},
	}
}

func (s *Server) callTool(req rpcRequest) (string, error) {
	var params struct {
		Name      string         `json:"name"`
		Arguments map[string]any `json:"arguments"`
	}
	if len(req.Params) > 0 {
		if err := json.Unmarshal(req.Params, &params); err != nil {
			return "", fmt.Errorf("invalid tool call params: %w", err)
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	switch params.Name {
	case toolListCatalog:
		body, err := s.get(ctx, "/agent/catalog")
		if err != nil {
			return "", err
		}
		return s.prettyCatalog(body)
	case toolListTopology:
		body, err := s.get(ctx, "/agent/topology")
		if err != nil {
			return "", err
		}
		return string(body), nil
	case toolRecommendModel:
		useCase, _ := params.Arguments["use_case"].(string)
		if strings.TrimSpace(useCase) == "" {
			return "", errors.New("use_case is required")
		}
		deviceID, _ := params.Arguments["device_id"].(string)
		limit := "10"
		if raw, ok := params.Arguments["limit"].(float64); ok && raw > 0 {
			limit = fmt.Sprintf("%d", int(raw))
		}
		q := fmt.Sprintf("/agent/recommend?use_case=%s&limit=%s", urlQueryEscape(useCase), limit)
		if deviceID != "" {
			q += "&device_id=" + urlQueryEscape(deviceID)
		}
		body, err := s.get(ctx, q)
		if err != nil {
			return "", err
		}
		return string(body), nil
	case toolListCandidates:
		body, err := s.get(ctx, "/agent/recipes")
		if err != nil {
			return "", err
		}
		return string(body), nil
	case toolProposeRecipe:
		payload := map[string]any{
			"proposed_by": params.Arguments["proposed_by"],
			"config":      params.Arguments["config"],
			"provenance":  params.Arguments["provenance"],
		}
		body, err := s.post(ctx, "/agent/recipe", payload)
		if err != nil {
			return "", err
		}
		return string(body), nil
	case toolGetRecipe:
		id, _ := params.Arguments["recipe_id"].(string)
		if strings.TrimSpace(id) == "" {
			return "", errors.New("recipe_id is required")
		}
		body, err := s.get(ctx, "/agent/recipe/"+urlQueryEscape(id))
		if err != nil {
			return "", err
		}
		return string(body), nil
	case toolValidateRecipe:
		id, _ := params.Arguments["recipe_id"].(string)
		deviceID, _ := params.Arguments["device_id"].(string)
		if strings.TrimSpace(id) == "" {
			return "", errors.New("recipe_id is required")
		}
		if strings.TrimSpace(deviceID) == "" {
			return "", errors.New("device_id is required")
		}
		body, err := s.post(ctx, "/agent/recipe/"+urlQueryEscape(id)+"/validate?device_id="+urlQueryEscape(deviceID), nil)
		if err != nil {
			return "", err
		}
		return string(body), nil
	default:
		return "", fmt.Errorf("unknown tool: %s", params.Name)
	}
}

func (s *Server) get(ctx context.Context, path string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.baseURL+path, nil)
	if err != nil {
		return nil, err
	}
	resp, err := s.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("yokai daemon unreachable (is it running?): %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("daemon error %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return body, nil
}

func (s *Server) post(ctx context.Context, path string, payload any) ([]byte, error) {
	var bodyReader io.Reader
	if payload != nil {
		buf, err := json.Marshal(payload)
		if err != nil {
			return nil, err
		}
		bodyReader = bytes.NewReader(buf)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.baseURL+path, bodyReader)
	if err != nil {
		return nil, err
	}
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := s.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("yokai daemon unreachable (is it running?): %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("daemon error %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return body, nil
}

// prettyCatalog turns the raw catalog response into a compact, readable text
// summary so agents don't burn 94 full records in one tool response.
func (s *Server) prettyCatalog(body []byte) (string, error) {
	var raw struct {
		Count    int               `json:"count"`
		UseCases []string          `json:"use_cases"`
		Catalog  []json.RawMessage `json:"catalog"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return string(body), nil
	}
	var b strings.Builder
	fmt.Fprintf(&b, "catalog: %d recipes; use cases: %s\n", raw.Count, strings.Join(raw.UseCases, ", "))
	for _, rmsg := range raw.Catalog {
		var r struct {
			ID       string   `json:"id"`
			Workload string   `json:"workload"`
			ModelID  string   `json:"model_id"`
			Quant    string   `json:"quantization"`
			Targets  []string `json:"target_devices"`
			MinVRAM  float64  `json:"min_vram_gb_per_gpu"`
			MinGPU   int      `json:"min_gpu_count"`
			UseCases []string `json:"use_cases"`
		}
		if err := json.Unmarshal(rmsg, &r); err != nil {
			continue
		}
		hardware := strings.Join(r.Targets, ",")
		if r.MinVRAM > 0 || r.MinGPU > 0 {
			hw := fmt.Sprintf("min %.0fGB", r.MinVRAM)
			if r.MinGPU > 1 {
				hw = fmt.Sprintf("%s x%dGPU", hw, r.MinGPU)
			}
			if hardware != "" {
				hardware += " | " + hw
			} else {
				hardware = hw
			}
		}
		fmt.Fprintf(&b, "  %s (%s)  workload=%s  quant=%s  use_cases=%s  hardware=[%s]\n",
			r.ID, r.ModelID, r.Workload, r.Quant, strings.Join(r.UseCases, ","), hardware)
	}
	return b.String(), nil
}

func urlQueryEscape(s string) string {
	return url.QueryEscape(s)
}
