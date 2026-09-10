package bkc

import (
	"sort"
	"strings"
)

// Use cases are the "what are you trying to do" axis for a recipe. They let an
// external agent (or the TUI) answer "I need a fast coding model for my two
// 4090s" against the catalog instead of knowing a model id up front.
const (
	UseCaseCoding      = "coding"      // code completion / agentic coding
	UseCaseChat        = "chat"        // general instruct / conversation
	UseCaseReasoning   = "reasoning"   // extended thinking / chain-of-thought
	UseCaseVision      = "vision"      // multimodal image input
	UseCaseOCR         = "ocr"         // document / text extraction
	UseCaseTranslation = "translation" // translation-focused
	UseCaseSpeech      = "speech"      // ASR / omni audio input
	UseCaseRerank      = "rerank"      // retrieval reranking
	UseCaseEmbedding   = "embedding"   // dense embeddings
	UseCaseGuardrails  = "guardrails"  // moderation / output guard
)

// useCasesByModel curates which tasks each catalog model is best suited for,
// keyed by the exact ModelID (matched case-insensitively). Unknown models fall
// back to downstream heuristics in UseCasesForModel, so future recipes get a
// sensible default without editing this map.
var useCasesByModel = map[string][]string{
	"amd/gpt-oss-120b-w-mxfp4-a-fp8":                      {UseCaseCoding, UseCaseChat, UseCaseReasoning},
	"arcee-ai/trinity-large-thinking":                     {UseCaseReasoning, UseCaseChat},
	"baidu/ernie-4.5-21b-a3b-pt":                          {UseCaseChat, UseCaseReasoning},
	"baidu/ernie-4.5-300b-a47b-pt":                        {UseCaseChat, UseCaseReasoning},
	"baidu/ernie-4.5-vl-28b-a3b-pt":                       {UseCaseVision, UseCaseChat},
	"baidu/ernie-4.5-vl-424b-a47b-pt":                     {UseCaseVision, UseCaseChat},
	"bytedance-seed/seed-oss-36b-instruct":                {UseCaseCoding, UseCaseChat},
	"deepseek-ai/deepseek-ocr":                            {UseCaseOCR},
	"deepseek-ai/deepseek-ocr-2":                          {UseCaseOCR},
	"deepseek-ai/deepseek-r1-0528":                        {UseCaseReasoning, UseCaseChat},
	"deepseek-ai/deepseek-v3.1":                           {UseCaseCoding, UseCaseChat, UseCaseReasoning},
	"deepseek-ai/deepseek-v3.2":                           {UseCaseCoding, UseCaseChat},
	"deepseek-ai/deepseek-v3.2-exp":                       {UseCaseCoding, UseCaseChat, UseCaseReasoning},
	"google/gemma-4-26b-a4b-it":                           {UseCaseChat, UseCaseCoding},
	"google/gemma-4-31b-it":                               {UseCaseChat, UseCaseCoding},
	"google/gemma-4-e2b-it":                               {UseCaseChat},
	"google/gemma-4-e4b-it":                               {UseCaseChat},
	"google/translategemma-27b-it":                        {UseCaseTranslation},
	"inclusionai/ring-1t-fp8":                             {UseCaseReasoning, UseCaseChat, UseCaseCoding},
	"internlm/intern-s1":                                  {UseCaseCoding, UseCaseChat},
	"internlm/intern-s1-fp8":                              {UseCaseCoding, UseCaseChat},
	"jinaai/jina-reranker-m0":                             {UseCaseRerank, UseCaseEmbedding},
	"meta-llama/llama-3.1-8b-instruct":                    {UseCaseChat, UseCaseCoding},
	"meta-llama/llama-3.3-70b-instruct":                   {UseCaseChat, UseCaseCoding},
	"microsoft/phi-4-mini-instruct":                       {UseCaseChat, UseCaseCoding, UseCaseReasoning},
	"minimaxai/minimax-m2.7":                              {UseCaseChat, UseCaseCoding, UseCaseReasoning},
	"mistralai/ministral-3-14b-instruct-2512":             {UseCaseChat, UseCaseCoding},
	"mistralai/ministral-3-14b-reasoning-2512":            {UseCaseReasoning, UseCaseChat},
	"mistralai/ministral-3-8b-reasoning-2512":             {UseCaseReasoning, UseCaseChat},
	"mistralai/mistral-large-3-675b-instruct-2512":        {UseCaseChat, UseCaseCoding},
	"mistralai/mistral-large-3-675b-instruct-2512-nvfp4":  {UseCaseChat, UseCaseCoding},
	"moonshotai/kimi-k2.5":                                {UseCaseCoding, UseCaseChat},
	"moonshotai/kimi-k2-instruct":                         {UseCaseCoding, UseCaseChat},
	"moonshotai/kimi-k2-thinking":                         {UseCaseReasoning, UseCaseChat},
	"moonshotai/kimi-linear-48b-a3b-instruct":             {UseCaseCoding, UseCaseChat},
	"nvidia/gemma-4-26b-a4b-nvfp4":                        {UseCaseChat, UseCaseCoding},
	"nvidia/kimi-k2.5-nvfp4":                              {UseCaseCoding, UseCaseChat},
	"nvidia/llama-3.3-70b-instruct-fp4":                   {UseCaseChat, UseCaseCoding},
	"nvidia/llama-3.3-70b-instruct-fp8":                   {UseCaseChat, UseCaseCoding},
	"nvidia/llama-4-scout-17b-16e-instruct-fp4":           {UseCaseVision, UseCaseChat},
	"nvidia/llama-4-scout-17b-16e-instruct-fp8":           {UseCaseVision, UseCaseChat},
	"nvidia/nemotron-3-nano-omni-30b-a3b-reasoning-nvfp4": {UseCaseReasoning, UseCaseChat, UseCaseSpeech, UseCaseVision},
	"nvidia/nvidia-nemotron-3-nano-30b-a3b-bf16":          {UseCaseChat, UseCaseSpeech},
	"nvidia/nvidia-nemotron-3-nano-30b-a3b-fp8":           {UseCaseChat, UseCaseSpeech},
	"nvidia/nvidia-nemotron-3-super-120b-a12b-nvfp4":      {UseCaseChat, UseCaseSpeech, UseCaseCoding},
	"nvidia/nvidia-nemotron-nano-12b-v2-vl-bf16":          {UseCaseVision, UseCaseChat},
	"nvidia/nvidia-nemotron-nano-12b-v2-vl-fp8":           {UseCaseVision, UseCaseChat},
	"nvidia/nvidia-nemotron-nano-12b-v2-vl-nvfp4-qad":     {UseCaseVision, UseCaseChat},
	"nvidia/qwen3.5-397b-a17b-nvfp4":                      {UseCaseCoding, UseCaseChat, UseCaseReasoning},
	"openai/gpt-oss-120b":                                 {UseCaseCoding, UseCaseChat, UseCaseReasoning},
	"openai/gpt-oss-20b":                                  {UseCaseCoding, UseCaseChat, UseCaseReasoning},
	"opengvlab/internvl3_5-8b":                            {UseCaseVision, UseCaseChat},
	"paddlepaddle/paddleocr-vl":                           {UseCaseOCR, UseCaseVision},
	"qwen/qwen2.5-vl-72b-instruct":                        {UseCaseVision, UseCaseChat},
	"qwen/qwen2.5-vl-7b-instruct":                         {UseCaseVision, UseCaseChat},
	"qwen/qwen3-235b-a22b":                                {UseCaseCoding, UseCaseChat, UseCaseReasoning},
	"qwen/qwen3-235b-a22b-fp8":                            {UseCaseCoding, UseCaseChat, UseCaseReasoning},
	"qwen/qwen3-30b-a3b":                                  {UseCaseChat, UseCaseCoding, UseCaseReasoning},
	"qwen/qwen3.5-397b-a17b-fp8":                          {UseCaseCoding, UseCaseChat, UseCaseReasoning},
	"qwen/qwen3.6-35b-a3b":                                {UseCaseChat, UseCaseCoding, UseCaseReasoning},
	"qwen/qwen3-asr-1.7b":                                 {UseCaseSpeech},
	"qwen/qwen3-coder-480b-a35b-instruct":                 {UseCaseCoding},
	"qwen/qwen3guard-gen-0.6b":                            {UseCaseGuardrails},
	"qwen/qwen3-next-80b-a3b-instruct":                    {UseCaseCoding, UseCaseChat},
	"qwen/qwen3-next-80b-a3b-instruct-fp8":                {UseCaseCoding, UseCaseChat},
	"qwen/qwen3-vl-235b-a22b-instruct":                    {UseCaseVision, UseCaseChat, UseCaseCoding},
	"qwen/qwen3-vl-235b-a22b-instruct-fp8":                {UseCaseVision, UseCaseChat, UseCaseCoding},
	"radixark/qwen3.8-27b-nvfp4":                          {UseCaseCoding, UseCaseChat, UseCaseReasoning},
	"radixark/qwen3.8-27b-nvfp4-bf16-lmhead":              {UseCaseCoding, UseCaseChat, UseCaseReasoning},
	"sakamakismile/qwen3.6-27b-nvfp4":                     {UseCaseCoding, UseCaseChat, UseCaseReasoning},
	"sakamakismile/qwen3.6-27b-text-nvfp4-mtp":            {UseCaseCoding, UseCaseChat, UseCaseReasoning},
	"stepfun-ai/step-3.5-flash":                           {UseCaseChat, UseCaseCoding, UseCaseReasoning},
	"tencent/hunyuan-a13b-instruct":                       {UseCaseChat, UseCaseCoding},
	"tencent/hunyuanocr":                                  {UseCaseOCR},
	"xiaomimimo/mimo-v2-flash":                            {UseCaseChat, UseCaseCoding, UseCaseReasoning},
	"zai-org/glm-4.5-air-fp8":                             {UseCaseChat, UseCaseCoding, UseCaseReasoning},
	"zai-org/glm-4.5v-fp8":                                {UseCaseVision, UseCaseChat},
	"zai-org/glm-4.7-fp8":                                 {UseCaseChat, UseCaseCoding, UseCaseReasoning},
	"zai-org/glm-5.1-fp8":                                 {UseCaseChat, UseCaseCoding, UseCaseReasoning},
	"zai-org/glm-asr-nano-2512":                           {UseCaseSpeech},
	"zai-org/glm-ocr":                                     {UseCaseOCR},
	"zai-org/glyph":                                       {UseCaseOCR},
}

// UseCasesForModel returns the use cases a model serves, using the curated map
// and falling back to name heuristics for models that aren't curated yet.
func UseCasesForModel(modelID string) []string {
	key := strings.ToLower(strings.TrimSpace(modelID))
	if ucs, ok := useCasesByModel[key]; ok && len(ucs) > 0 {
		return dedupe(ucs)
	}
	// Heuristic fallback for uncurated / future models.
	lower := key
	ucs := []string{UseCaseChat}
	if strings.Contains(lower, "coder") || strings.Contains(lower, "gpt-oss") || strings.Contains(lower, "deepseek-v") || strings.Contains(lower, "kimi") || strings.HasPrefix(lower, "qwen3") {
		ucs = append(ucs, UseCaseCoding)
	}
	if strings.Contains(lower, "think") || strings.Contains(lower, "reason") || strings.Contains(lower, "r1") || strings.Contains(lower, "-s1") {
		ucs = append(ucs, UseCaseReasoning)
	}
	if strings.Contains(lower, "vl") || strings.Contains(lower, "vision") || strings.Contains(lower, "omni") || strings.Contains(lower, "internvl") {
		ucs = append(ucs, UseCaseVision)
	}
	if strings.Contains(lower, "ocr") {
		ucs = append(ucs, UseCaseOCR)
	}
	if strings.Contains(lower, "asr") || strings.Contains(lower, "audio") || strings.Contains(lower, "speech") {
		ucs = append(ucs, UseCaseSpeech)
	}
	if strings.Contains(lower, "rerank") {
		ucs = append(ucs, UseCaseRerank)
	}
	if strings.Contains(lower, "embed") {
		ucs = append(ucs, UseCaseEmbedding)
	}
	return dedupe(ucs)
}

// UseCases returns the use cases for the receiving config's model.
func (c Config) UseCases() []string {
	return UseCasesForModel(c.ModelID)
}

// HasUseCase reports whether the config serves the given use case.
func (c Config) HasUseCase(useCase string) bool {
	uc := strings.ToLower(strings.TrimSpace(useCase))
	for _, candidate := range c.UseCases() {
		if candidate == uc {
			return true
		}
	}
	return false
}

// AllUseCases returns every use case referenced by the catalog, sorted.
func AllUseCases() []string {
	set := map[string]bool{}
	for _, cfg := range catalog {
		for _, uc := range cfg.UseCases() {
			set[uc] = true
		}
	}
	out := make([]string, 0, len(set))
	for uc := range set {
		out = append(out, uc)
	}
	sort.Strings(out)
	return out
}

func dedupe(in []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(in))
	for _, v := range in {
		if !seen[v] {
			seen[v] = true
			out = append(out, v)
		}
	}
	return out
}
