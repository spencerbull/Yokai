package hf

import (
	"testing"
)

func TestParseGGUFFilenameQuant(t *testing.T) {
	t.Parallel()

	cases := []struct {
		filename string
		quant    string
	}{
		{"Qwen3-27B-Instruct-Q4_K_M.gguf", "Q4_K_M"},
		{"qwen3-27b-instruct-q4_k_m.gguf", "Q4_K_M"},
		{"Qwen3-27B-Instruct.Q8_0.gguf", "Q8_0"},
		{"model-f16.gguf", "F16"},
		{"model-FP16.gguf", "F16"},
		{"model-bf16.gguf", "BF16"},
		{"model-IQ4_XS.gguf", "IQ4_XS"},
		{"Qwen3-27B-IQ2_M.gguf", "IQ2_M"},
		{"llama-3-Q4_K.gguf", "Q4_K"},
		{"llama-3-Q4_K_S.gguf", "Q4_K_S"},
		{"llama-3-Q6_K.gguf", "Q6_K"},
		{"model.gguf", ""},
		{"config.json", ""},
	}

	for _, tc := range cases {
		got := ParseGGUFFilename(tc.filename).Quantization
		if got != tc.quant {
			t.Errorf("%s: expected quant %q, got %q", tc.filename, tc.quant, got)
		}
	}
}

func TestParseGGUFFilenameShard(t *testing.T) {
	t.Parallel()

	cases := []struct {
		filename string
		index    int
		total    int
		base     string
	}{
		{"Qwen3-27B-Q4_K_M-00001-of-00003.gguf", 1, 3, "Qwen3-27B-Q4_K_M.gguf"},
		{"Qwen3-27B-Q4_K_M-00002-of-00003.gguf", 2, 3, "Qwen3-27B-Q4_K_M.gguf"},
		{"model-00003-OF-00003.gguf", 3, 3, "model.gguf"},
		{"model-Q4_K_M.gguf", 0, 0, "model-Q4_K_M.gguf"},
	}

	for _, tc := range cases {
		got := ParseGGUFFilename(tc.filename)
		if got.ShardIndex != tc.index || got.ShardTotal != tc.total {
			t.Errorf("%s: expected shard %d/%d, got %d/%d", tc.filename, tc.index, tc.total, got.ShardIndex, got.ShardTotal)
		}
		if got.Base != tc.base {
			t.Errorf("%s: expected base %q, got %q", tc.filename, tc.base, got.Base)
		}
	}
}

func TestGroupGGUFVariantsSingleFile(t *testing.T) {
	t.Parallel()

	files := []GGUFFile{
		{Filename: "Qwen3-27B-Q4_K_M.gguf", SizeMB: 16000},
		{Filename: "Qwen3-27B-Q8_0.gguf", SizeMB: 28000},
		{Filename: "Qwen3-27B-F16.gguf", SizeMB: 54000},
	}

	variants := GroupGGUFVariants(files)
	if len(variants) != 3 {
		t.Fatalf("expected 3 variants, got %d", len(variants))
	}

	index := make(map[string]GGUFVariant, len(variants))
	for _, v := range variants {
		index[v.Quantization] = v
	}

	q4 := index["Q4_K_M"]
	if q4.ShardCount != 1 || len(q4.Shards) != 1 || q4.Primary != "Qwen3-27B-Q4_K_M.gguf" {
		t.Errorf("Q4_K_M: unexpected variant %+v", q4)
	}
	if q4.TotalSizeMB != 16000 {
		t.Errorf("Q4_K_M: expected total 16000MB, got %d", q4.TotalSizeMB)
	}

	f16 := index["F16"]
	if f16.Primary != "Qwen3-27B-F16.gguf" || f16.TotalSizeMB != 54000 {
		t.Errorf("F16: unexpected variant %+v", f16)
	}
}

func TestGroupGGUFVariantsSharded(t *testing.T) {
	t.Parallel()

	// Shards arrive in non-sequential order; grouping should re-sort them.
	files := []GGUFFile{
		{Filename: "Qwen3-27B-Q4_K_M-00002-of-00003.gguf", SizeMB: 16000},
		{Filename: "Qwen3-27B-Q4_K_M-00001-of-00003.gguf", SizeMB: 16000},
		{Filename: "Qwen3-27B-Q4_K_M-00003-of-00003.gguf", SizeMB: 12000},
		{Filename: "Qwen3-27B-F16-00001-of-00002.gguf", SizeMB: 30000},
		{Filename: "Qwen3-27B-F16-00002-of-00002.gguf", SizeMB: 24000},
	}

	variants := GroupGGUFVariants(files)
	if len(variants) != 2 {
		t.Fatalf("expected 2 variants, got %d", len(variants))
	}

	index := make(map[string]GGUFVariant, len(variants))
	for _, v := range variants {
		index[v.Quantization] = v
	}

	q4 := index["Q4_K_M"]
	if q4.ShardCount != 3 {
		t.Errorf("Q4_K_M: expected shard count 3, got %d", q4.ShardCount)
	}
	if len(q4.Shards) != 3 {
		t.Errorf("Q4_K_M: expected 3 shards, got %d", len(q4.Shards))
	}
	if q4.Primary != "Qwen3-27B-Q4_K_M-00001-of-00003.gguf" {
		t.Errorf("Q4_K_M: expected primary to be first shard, got %q", q4.Primary)
	}
	if q4.TotalSizeMB != 44000 {
		t.Errorf("Q4_K_M: expected total 44000MB, got %d", q4.TotalSizeMB)
	}
	for i, shard := range q4.Shards {
		expected := []string{
			"Qwen3-27B-Q4_K_M-00001-of-00003.gguf",
			"Qwen3-27B-Q4_K_M-00002-of-00003.gguf",
			"Qwen3-27B-Q4_K_M-00003-of-00003.gguf",
		}[i]
		if shard.Filename != expected {
			t.Errorf("Q4_K_M: shard %d = %q, want %q", i, shard.Filename, expected)
		}
	}

	f16 := index["F16"]
	if f16.ShardCount != 2 || len(f16.Shards) != 2 {
		t.Errorf("F16: expected 2 shards, got shardCount=%d shards=%d", f16.ShardCount, len(f16.Shards))
	}
	if f16.Primary != "Qwen3-27B-F16-00001-of-00002.gguf" {
		t.Errorf("F16: expected primary to be first shard, got %q", f16.Primary)
	}
}

func TestGroupGGUFVariantsUnknownQuant(t *testing.T) {
	t.Parallel()

	files := []GGUFFile{
		{Filename: "model.gguf", SizeMB: 1024},
	}
	variants := GroupGGUFVariants(files)
	if len(variants) != 1 {
		t.Fatalf("expected 1 variant, got %d", len(variants))
	}
	if variants[0].Quantization != "unknown" {
		t.Errorf("expected unknown quant, got %q", variants[0].Quantization)
	}
	if variants[0].Primary != "model.gguf" {
		t.Errorf("expected primary model.gguf, got %q", variants[0].Primary)
	}
}
