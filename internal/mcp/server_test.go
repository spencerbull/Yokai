package mcp

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

func newStubServer(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/agent/catalog", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"count":     2,
			"use_cases": []string{"chat", "coding"},
			"catalog": []map[string]any{
				{"id": "a", "workload": "vllm", "model_id": "M1", "use_cases": []string{"coding"}},
				{"id": "b", "workload": "vllm", "model_id": "M2", "use_cases": []string{"chat"}},
			},
		})
	})
	mux.HandleFunc("/agent/recommend", func(w http.ResponseWriter, r *http.Request) {
		uc := r.URL.Query().Get("use_case")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"use_case": uc,
			"total":    1,
			"options":  []map[string]any{{"id": "a", "model_id": "M1", "use_cases": []string{"coding"}}},
		})
	})
	mux.HandleFunc("/agent/topology", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"devices": []map[string]any{}})
	})
	return httptest.NewServer(mux)
}

func TestServeHandshakeAndToolsList(t *testing.T) {
	srv := newStubServer(t)
	defer srv.Close()
	s := &Server{baseURL: srv.URL, http: &http.Client{Timeout: 5 * time.Second}}

	initResp := s.handleLine(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05"}}`)
	if initResp == nil {
		t.Fatalf("initialize returned nil")
	}
	var result struct {
		ProtocolVersion string `json:"protocolVersion"`
		Capabilities    struct {
			Tools map[string]any `json:"tools"`
		} `json:"capabilities"`
	}
	raw, _ := json.Marshal(initResp.Result)
	if err := json.Unmarshal(raw, &result); err != nil {
		t.Fatalf("decode initialize result: %v", err)
	}
	if result.ProtocolVersion == "" {
		t.Fatalf("expected protocolVersion")
	}
	if result.Capabilities.Tools == nil {
		t.Fatalf("expected tools capability")
	}

	listResp := s.handleLine(`{"jsonrpc":"2.0","id":2,"method":"tools/list"}`)
	var list struct {
		Tools []toolDefinition `json:"tools"`
	}
	raw, _ = json.Marshal(listResp.Result)
	if err := json.Unmarshal(raw, &list); err != nil {
		t.Fatalf("decode tools/list: %v", err)
	}
	names := map[string]bool{}
	for _, tool := range list.Tools {
		names[tool.Name] = true
	}
	for _, want := range []string{toolListCatalog, toolListTopology, toolRecommendModel} {
		if !names[want] {
			t.Fatalf("missing tool %s", want)
		}
	}
}

func TestCallToolCatalog(t *testing.T) {
	srv := newStubServer(t)
	defer srv.Close()
	s := &Server{baseURL: srv.URL, http: &http.Client{Timeout: 5 * time.Second}}

	out, err := s.callTool(rpcRequest{Params: json.RawMessage(`{"name":"list_catalog","arguments":{}}`)})
	if err != nil {
		t.Fatalf("callTool catalog: %v", err)
	}
	if !strings.Contains(out, "catalog: 2 recipes") || !strings.Contains(out, "use cases: chat, coding") {
		t.Fatalf("unexpected catalog text: %s", out)
	}
}

func TestCallToolRecommend(t *testing.T) {
	srv := newStubServer(t)
	defer srv.Close()
	s := &Server{baseURL: srv.URL, http: &http.Client{Timeout: 5 * time.Second}}

	out, err := s.callTool(rpcRequest{Params: json.RawMessage(`{"name":"recommend_model","arguments":{"use_case":"coding"}}`)})
	if err != nil {
		t.Fatalf("callTool recommend: %v", err)
	}
	if !strings.Contains(out, `"use_case":"coding"`) {
		t.Fatalf("expected coding recommendation, got: %s", out)
	}

	// Missing required use_case should error.
	if _, err := s.callTool(rpcRequest{Params: json.RawMessage(`{"name":"recommend_model","arguments":{}}`)}); err == nil {
		t.Fatalf("expected error for missing use_case")
	}
}

func TestServeConcurrentClients(t *testing.T) {
	srv := newStubServer(t)
	defer srv.Close()
	s := &Server{baseURL: srv.URL, http: &http.Client{Timeout: 5 * time.Second}}

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if resp := s.handleLine(`{"jsonrpc":"2.0","id":1,"method":"ping"}`); resp == nil {
				t.Error("ping returned nil")
			}
		}()
	}
	wg.Wait()
}
