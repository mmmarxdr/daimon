// Package cost provides token cost calculation for supported LLM models.
//
// Pricing is stored as a compile-time map of model prefixes to per-token
// prices. Lookup uses longest-prefix matching so that model variants like
// "gpt-4o-2024-08-06" automatically resolve to the "gpt-4o" pricing entry.
package cost

import (
	"sort"
	"strings"
)

// ModelPricing holds per-token costs for a model family.
// Prices are in USD per token (e.g., 0.000015 means $0.015 per 1K tokens).
type ModelPricing struct {
	Input  float64 // USD per input token
	Output float64 // USD per output token
}

// pricing is the compile-time model pricing table.
// Keys are model name prefixes (e.g., "gpt-4o", "claude-sonnet-4-20250514").
var pricing = map[string]ModelPricing{
	// ── Anthropic ──
	"claude-opus-4-20250514":    {Input: 0.000015, Output: 0.000075},
	"claude-sonnet-4-20250514":  {Input: 0.000003, Output: 0.000015},
	"claude-3-5-haiku-20241022": {Input: 0.000001, Output: 0.000005},
	"claude-3-haiku-20240307":   {Input: 0.00000025, Output: 0.00000125},
	"claude-3-opus-20240229":    {Input: 0.000015, Output: 0.000075},
	"claude-3-sonnet-20240229":  {Input: 0.000003, Output: 0.000015},

	// ── OpenAI ──
	"gpt-4o":       {Input: 0.0000025, Output: 0.00001},
	"gpt-4o-mini":  {Input: 0.00000015, Output: 0.0000006},
	"gpt-4.1":      {Input: 0.000002, Output: 0.000008},
	"gpt-4.1-mini": {Input: 0.0000004, Output: 0.0000016},
	"gpt-4.1-nano": {Input: 0.0000001, Output: 0.0000004},
	"o3":           {Input: 0.00001, Output: 0.00004},
	"o3-mini":      {Input: 0.0000011, Output: 0.0000044},
	"o4-mini":      {Input: 0.0000011, Output: 0.0000044},

	// ── Google ──
	"gemini-2.5-pro":   {Input: 0.00000125, Output: 0.00001},
	"gemini-2.5-flash": {Input: 0.00000015, Output: 0.0000006},
	"gemini-2.0-flash": {Input: 0.0000001, Output: 0.0000004},

	// ── Deepseek ──
	"deepseek-r1":   {Input: 0.00000055, Output: 0.00000219},
	"deepseek-v3":   {Input: 0.00000027, Output: 0.0000011},
	"deepseek-v3.2": {Input: 0.0000003, Output: 0.00000085},

	// ── Qwen ──
	"qwen3-235b-a22b":  {Input: 0.0000004, Output: 0.0000012},
	"qwen3-30b-a3b":    {Input: 0.0000002, Output: 0.0000006},
	"qwen3-coder-480b": {Input: 0.0000006, Output: 0.0000024},
	"qwen3.6-plus":     {Input: 0.0000004, Output: 0.0000012},

	// ── Xiaomi ──
	"miimo-v2-pro": {Input: 0.0000004, Output: 0.0000012},
}

// sortedKeys is pre-computed longest-first for prefix matching.
var sortedKeys []string

func init() {
	sortedKeys = make([]string, 0, len(pricing))
	for k := range pricing {
		sortedKeys = append(sortedKeys, k)
	}
	sort.Slice(sortedKeys, func(i, j int) bool {
		return len(sortedKeys[i]) > len(sortedKeys[j])
	})
}

// Lookup returns the pricing for the given model name using longest-prefix
// matching. Returns the pricing and true if found; zero value and false otherwise.
func Lookup(model string) (ModelPricing, bool) {
	if p, ok := pricing[model]; ok {
		return p, true
	}
	for _, prefix := range sortedKeys {
		if strings.HasPrefix(model, prefix) {
			return pricing[prefix], true
		}
	}
	return ModelPricing{}, false
}

// All returns all registered model prefixes and their pricing.
// Useful for CLI display.
func All() map[string]ModelPricing {
	cp := make(map[string]ModelPricing, len(pricing))
	for k, v := range pricing {
		cp[k] = v
	}
	return cp
}
