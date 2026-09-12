package daemon

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/spencerbull/yokai/internal/config"
	"github.com/spencerbull/yokai/internal/hf"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

func TestHandleHFModelsReturnsEmptyWhenSiblingPipelineFails(t *testing.T) {
	originalTransport := http.DefaultTransport
	var filters []string
	http.DefaultTransport = roundTripFunc(func(request *http.Request) (*http.Response, error) {
		filter := request.URL.Query().Get("filter")
		filters = append(filters, filter)
		status := http.StatusOK
		body := "[]"
		if filter == "image-text-to-text" {
			status = http.StatusServiceUnavailable
			body = `{"error":"temporarily unavailable"}`
		}
		return &http.Response{
			StatusCode: status,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(body)),
			Request:    request,
		}, nil
	})
	t.Cleanup(func() { http.DefaultTransport = originalTransport })

	daemon := &Daemon{cfg: config.DefaultConfig()}
	request := httptest.NewRequest(http.MethodGet, "/hf/models?query=qwen&workload=vllm", nil)
	response := httptest.NewRecorder()

	daemon.handleHFModels(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", response.Code, http.StatusOK, response.Body.String())
	}
	var body struct {
		Models []hf.Model `json:"models"`
	}
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.Models == nil {
		t.Fatal("models = null, want []")
	}
	if len(body.Models) != 0 {
		t.Fatalf("models = %+v, want empty", body.Models)
	}
	if len(filters) != 2 || filters[0] != "text-generation" || filters[1] != "image-text-to-text" {
		t.Fatalf("filters = %v, want both pipeline searches", filters)
	}
}

func TestMergeModelsByLikesDedupesAndSorts(t *testing.T) {
	text := []hf.Model{
		{ID: "a/llama", Likes: 10},
		{ID: "b/qwen", Likes: 5},
	}
	multimodal := []hf.Model{
		{ID: "c/vision", Likes: 99},
		{ID: "a/llama", Likes: 10}, // duplicate across pipelines
		{ID: "b/qwen", Likes: 5},
	}

	merged := mergeModelsByLikes([][]hf.Model{text, multimodal}, 30)
	if len(merged) != 3 {
		t.Fatalf("expected 3 deduped models, got %d", len(merged))
	}
	// Sorted by likes desc: c/vision(99), a/llama(10), b/qwen(5)
	if merged[0].ID != "c/vision" || merged[1].ID != "a/llama" || merged[2].ID != "b/qwen" {
		t.Fatalf("unexpected order or ids: %+v", merged)
	}
}

func TestMergeModelsByLikesCapsLimit(t *testing.T) {
	group := []hf.Model{}
	for i := 0; i < 10; i++ {
		group = append(group, hf.Model{ID: string(rune('a'+i)) + "/m", Likes: i})
	}
	merged := mergeModelsByLikes([][]hf.Model{group}, 3)
	if len(merged) != 3 {
		t.Fatalf("expected cap at 3, got %d", len(merged))
	}
}

func TestMergeModelsByLikesEmpty(t *testing.T) {
	merged := mergeModelsByLikes([][]hf.Model{{}, {}}, 30)
	if len(merged) != 0 {
		t.Fatalf("expected 0 models, got %d", len(merged))
	}
}

func TestMergeModelsBestEffort(t *testing.T) {
	errText := "boom"
	errTextGen := errors.New(errText)
	// Both pipelines succeed -> merged.
	got, err := mergeModelsBestEffort(
		[][]hf.Model{{{ID: "a/m", Likes: 1}}, {{ID: "b/v", Likes: 5}}},
		[]error{nil, nil}, 30)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if len(got) != 2 || got[0].ID != "b/v" {
		t.Fatalf("merged = %+v", got)
	}
	// One pipeline fails, the other succeeds -> degrade independently.
	got, err = mergeModelsBestEffort(
		[][]hf.Model{nil, {{ID: "b/v", Likes: 5}}},
		[]error{errTextGen, nil}, 30)
	if err != nil {
		t.Fatalf("should tolerate single-pipeline failure, got %v", err)
	}
	if len(got) != 1 || got[0].ID != "b/v" {
		t.Fatalf("expected the successful batch, got %+v", got)
	}
	// One success-empty pipeline + one failed pipeline -> success (empty result,
	// not a failure).
	got, err = mergeModelsBestEffort(
		[][]hf.Model{{}, nil},
		[]error{nil, errTextGen}, 30)
	if err != nil {
		t.Fatalf("successful empty pipeline should not surface the sibling failure, got %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("expected empty merged result, got %+v", got)
	}
	// Every pipeline fails -> return the first error.
	_, err = mergeModelsBestEffort(
		[][]hf.Model{nil, nil},
		[]error{errTextGen, errors.New("other")}, 30)
	if err == nil || err.Error() != errText {
		t.Fatalf("expected first error, got %v", err)
	}
	// No pipelines at all -> empty, no error.
	got, err = mergeModelsBestEffort([][]hf.Model{}, []error{}, 30)
	if err != nil || len(got) != 0 {
		t.Fatalf("empty case: got=%v err=%v", got, err)
	}
}
