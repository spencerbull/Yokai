package daemon

import (
	"testing"

	"github.com/spencerbull/yokai/internal/hf"
)

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
