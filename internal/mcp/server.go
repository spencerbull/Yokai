// Package mcp implements a minimal Model Context Protocol server over stdio
// that proxies the yokai daemon's /agent/* read endpoints. It is intentionally
// thin: the daemon owns catalog/topology/recommend logic, this package only
// translates MCP tool calls into daemon HTTP requests. Any MCP-speaking agent
// (pi, opencode, hermes, openclaw, ...) can register this binary.
package mcp

import (
	"bufio"
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
