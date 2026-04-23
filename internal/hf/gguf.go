package hf

import (
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// GGUFVariant groups one or more GGUF shards that share the same quantization.
type GGUFVariant struct {
	// Quantization is the canonical quant label (e.g. "Q4_K_M", "F16"), or
	// "unknown" if one could not be detected from the filenames.
	Quantization string `json:"quantization"`
	// Shards lists all files that make up this variant, sorted by shard index
	// when a shard suffix is present. For single-file variants this holds a
	// single entry.
	Shards []GGUFFile `json:"shards"`
	// ShardCount is the total number of shards discovered. For well-formed
	// `<N>-of-<M>` suffixes this equals M; otherwise it is len(Shards).
	ShardCount int `json:"shard_count"`
	// TotalSizeMB sums the sizes of every shard in this variant.
	TotalSizeMB int64 `json:"total_size_mb"`
	// Primary is the filename the runtime (llama.cpp / vLLM) should be pointed
	// at. For multi-shard variants this is the first shard (`00001-of-000NN`).
	Primary string `json:"primary"`
}

// shardSuffixPattern matches the standard llama.cpp multi-file suffix:
//
//	name-00001-of-00003.gguf
//
// Captures: 1=index, 2=total
var shardSuffixPattern = regexp.MustCompile(`(?i)-(\d{2,6})-of-(\d{2,6})\.gguf$`)

// quantizationPatterns enumerates known GGUF quantization labels. The order is
// longest-match-first so "Q4_K_M" wins over "Q4_K".
var quantizationPatterns = []string{
	// IQ (importance-matrix) quants
	"IQ1_S", "IQ1_M",
	"IQ2_XXS", "IQ2_XS", "IQ2_S", "IQ2_M",
	"IQ3_XXS", "IQ3_XS", "IQ3_S", "IQ3_M",
	"IQ4_XS", "IQ4_NL",
	// K-quants
	"Q2_K_S", "Q2_K",
	"Q3_K_XL", "Q3_K_L", "Q3_K_M", "Q3_K_S", "Q3_K",
	"Q4_K_XL", "Q4_K_M", "Q4_K_S", "Q4_K", "Q4_0", "Q4_1",
	"Q5_K_M", "Q5_K_S", "Q5_K", "Q5_0", "Q5_1",
	"Q6_K_XL", "Q6_K",
	"Q8_K", "Q8_0",
	// Ternary quants
	"TQ1_0", "TQ2_0",
	// Float formats
	"BF16", "FP16", "F16", "FP32", "F32",
}

var quantTokenPattern = buildQuantTokenPattern()

func buildQuantTokenPattern() *regexp.Regexp {
	// Match the quant label delimited by a non-alphanumeric boundary on either
	// side (hyphen, underscore, dot, or start/end). We allow `_` in the
	// boundary because real filenames use it (e.g. `_Q4_K_M`), so the boundary
	// before the label can be any non-letter.
	escaped := make([]string, 0, len(quantizationPatterns))
	for _, p := range quantizationPatterns {
		escaped = append(escaped, regexp.QuoteMeta(p))
	}
	expr := `(?i)(?:^|[^A-Za-z0-9])(` + strings.Join(escaped, "|") + `)(?:[^A-Za-z0-9]|$)`
	return regexp.MustCompile(expr)
}

// ParsedGGUFName holds the components extracted from a GGUF filename.
type ParsedGGUFName struct {
	Quantization string // canonical quant label, or "" if none matched
	ShardIndex   int    // 1-based shard index, or 0 if not sharded
	ShardTotal   int    // total shard count, or 0 if not sharded
	Base         string // filename with the shard suffix removed; used as the
	// grouping key so sibling shards land in the same variant.
}

// ParseGGUFFilename extracts the quantization and shard info from a GGUF
// filename. The filename may include directory components; only the basename
// is considered.
func ParseGGUFFilename(filename string) ParsedGGUFName {
	base := filename
	if idx := strings.LastIndex(base, "/"); idx >= 0 {
		base = base[idx+1:]
	}

	parsed := ParsedGGUFName{Base: base}

	if match := shardSuffixPattern.FindStringSubmatchIndex(base); match != nil {
		idx, _ := strconv.Atoi(base[match[2]:match[3]])
		total, _ := strconv.Atoi(base[match[4]:match[5]])
		parsed.ShardIndex = idx
		parsed.ShardTotal = total
		// Strip the shard suffix so siblings group together. Preserve the
		// `.gguf` extension on the base key.
		parsed.Base = base[:match[0]] + ".gguf"
	}

	// Run the quant match against the full original name (including shard
	// suffix) since the quant label usually sits before the shard suffix.
	if match := quantTokenPattern.FindStringSubmatch(base); len(match) == 2 {
		parsed.Quantization = canonicalizeQuant(match[1])
	}

	return parsed
}

// canonicalizeQuant normalizes a matched quant token to its canonical upper
// case form. F16/FP16 are treated as the same variant (FP16 is commonly used
// by unsloth; F16 by llama.cpp). Likewise F32/FP32.
func canonicalizeQuant(token string) string {
	upper := strings.ToUpper(token)
	switch upper {
	case "FP16":
		return "F16"
	case "FP32":
		return "F32"
	}
	return upper
}

// GroupGGUFVariants groups a flat list of GGUF files into variants, collapsing
// shard siblings together. The returned list is sorted by quantization label.
func GroupGGUFVariants(files []GGUFFile) []GGUFVariant {
	type groupKey struct {
		quant string
		base  string
	}

	groups := make(map[groupKey]*GGUFVariant)
	order := make([]groupKey, 0)

	for _, f := range files {
		parsed := ParseGGUFFilename(f.Filename)
		quant := parsed.Quantization
		if quant == "" {
			quant = "unknown"
		}
		key := groupKey{quant: quant, base: parsed.Base}

		variant, ok := groups[key]
		if !ok {
			variant = &GGUFVariant{Quantization: quant}
			groups[key] = variant
			order = append(order, key)
		}
		variant.Shards = append(variant.Shards, f)
		variant.TotalSizeMB += f.SizeMB
		if parsed.ShardTotal > variant.ShardCount {
			variant.ShardCount = parsed.ShardTotal
		}
	}

	result := make([]GGUFVariant, 0, len(order))
	for _, key := range order {
		variant := groups[key]
		sort.SliceStable(variant.Shards, func(i, j int) bool {
			a := ParseGGUFFilename(variant.Shards[i].Filename)
			b := ParseGGUFFilename(variant.Shards[j].Filename)
			if a.ShardIndex != b.ShardIndex {
				return a.ShardIndex < b.ShardIndex
			}
			return variant.Shards[i].Filename < variant.Shards[j].Filename
		})
		if variant.ShardCount == 0 {
			variant.ShardCount = len(variant.Shards)
		}
		if len(variant.Shards) > 0 {
			variant.Primary = variant.Shards[0].Filename
		}
		result = append(result, *variant)
	}

	sort.SliceStable(result, func(i, j int) bool {
		return result[i].Quantization < result[j].Quantization
	})

	return result
}
