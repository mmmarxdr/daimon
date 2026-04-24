package audit

import "sync"

// PriceLookup returns pricing for a model when known. Used by EstimateCost
// to query a runtime source (e.g. the OpenRouter model list with live
// pricing) before falling back to the hardcoded modelPricing map. Returns
// ok=false to defer to the fallback.
type PriceLookup func(model string) (inputPer1M, outputPer1M float64, ok bool)

// ContextLengthLookup returns the maximum context window (in tokens) for a
// model when known. Used by the metrics layer so the dashboard can show
// "current call vs model's true capacity" instead of "current call vs the
// agent's self-imposed soft cap" (which under-represents capacity for
// long-context models like deepseek-v4-pro at 1M tokens).
type ContextLengthLookup func(model string) (contextLength int, ok bool)

var (
	priceLookupMu   sync.RWMutex
	priceLookup     PriceLookup
	contextLookupMu sync.RWMutex
	contextLookup   ContextLengthLookup
)

// SetPriceLookup registers a runtime pricing source. Called by cmd/daimon at
// startup after the provider is constructed. Pass nil to clear. Goroutine-safe.
func SetPriceLookup(lookup PriceLookup) {
	priceLookupMu.Lock()
	priceLookup = lookup
	priceLookupMu.Unlock()
}

// SetContextLengthLookup registers a runtime context-window source. Same
// lifecycle as SetPriceLookup; pass nil to clear.
func SetContextLengthLookup(lookup ContextLengthLookup) {
	contextLookupMu.Lock()
	contextLookup = lookup
	contextLookupMu.Unlock()
}

// LookupContextLength returns the max context window for a model when known.
// Returns (0, false) when no lookup is registered or the model is unknown.
func LookupContextLength(model string) (int, bool) {
	contextLookupMu.RLock()
	lookup := contextLookup
	contextLookupMu.RUnlock()
	if lookup == nil {
		return 0, false
	}
	return lookup(model)
}

// resolvePricing returns (in, out) per 1M tokens. Tries the runtime lookup
// first, then the static modelPricing map, then (0,0,false) for unknowns.
func resolvePricing(model string) (inputPer1M, outputPer1M float64, ok bool) {
	priceLookupMu.RLock()
	lookup := priceLookup
	priceLookupMu.RUnlock()
	if lookup != nil {
		if in, out, found := lookup(model); found {
			return in, out, true
		}
	}
	if p, found := modelPricing[model]; found {
		return p.InputPer1M, p.OutputPer1M, true
	}
	return 0, 0, false
}

// modelPricing maps model IDs to per-1M-token pricing in USD.
// This is the offline fallback. Runtime PriceLookup (if registered) takes
// precedence so OpenRouter's hundreds of models work without listing them
// here. Keep this map for non-OpenRouter providers and offline use.
var modelPricing = map[string]struct{ InputPer1M, OutputPer1M float64 }{
	// Anthropic
	"claude-sonnet-4-20250514":   {3.0, 15.0},
	"claude-haiku-3-5-20241022":  {0.80, 4.0},
	"claude-opus-4-20250514":     {15.0, 75.0},
	"claude-opus-4-5":            {15.0, 75.0},
	"claude-sonnet-4-5":          {3.0, 15.0},
	"claude-haiku-3-5":           {0.80, 4.0},
	"claude-3-5-sonnet-20241022": {3.0, 15.0},
	"claude-3-5-haiku-20241022":  {0.80, 4.0},
	"claude-3-opus-20240229":     {15.0, 75.0},

	// OpenAI
	"gpt-4o":        {2.50, 10.0},
	"gpt-4o-mini":   {0.15, 0.60},
	"gpt-4-turbo":   {10.0, 30.0},
	"gpt-4":         {30.0, 60.0},
	"gpt-3.5-turbo": {0.50, 1.50},
	"o1":            {15.0, 60.0},
	"o1-mini":       {3.0, 12.0},
	"o3-mini":       {1.10, 4.40},

	// Google Gemini
	"gemini-2.0-flash":       {0.075, 0.30},
	"gemini-2.0-flash-lite":  {0.075, 0.30},
	"gemini-1.5-pro":         {1.25, 5.0},
	"gemini-1.5-flash":       {0.075, 0.30},
	"gemini-1.5-flash-8b":    {0.0375, 0.15},
	"gemini-2.5-pro-preview": {1.25, 10.0},

	// OpenRouter pass-through — Anthropic
	"anthropic/claude-haiku-4-5":         {0.80, 4.0},
	"anthropic/claude-sonnet-4-5":        {3.0, 15.0},
	"anthropic/claude-opus-4-5":          {15.0, 75.0},
	"anthropic/claude-sonnet-4-20250514": {3.0, 15.0},
	"anthropic/claude-opus-4-20250514":   {15.0, 75.0},
	"anthropic/claude-3.5-sonnet":        {3.0, 15.0},
	"anthropic/claude-3.5-haiku":         {0.80, 4.0},

	// OpenRouter pass-through — OpenAI
	"openai/gpt-4o":      {2.50, 10.0},
	"openai/gpt-4o-mini": {0.15, 0.60},
	"openai/o1":          {15.0, 60.0},
	"openai/o3-mini":     {1.10, 4.40},

	// OpenRouter pass-through — Google
	"google/gemini-2.0-flash-001":        {0.075, 0.30},
	"google/gemini-2.5-pro-preview":      {1.25, 10.0},
	"google/gemini-2.5-flash-preview":    {0.15, 0.60},

	// OpenRouter pass-through — Meta, Mistral, etc.
	"meta-llama/llama-3.1-8b-instruct":  {0.055, 0.055},
	"meta-llama/llama-3.1-70b-instruct": {0.40, 0.40},
	"meta-llama/llama-4-maverick":       {0.20, 0.60},
	"mistralai/mistral-7b-instruct":     {0.055, 0.055},
	"mistralai/mixtral-8x7b-instruct":   {0.24, 0.24},
	"deepseek/deepseek-chat-v3-0324":    {0.14, 0.28},
	"deepseek/deepseek-r1":              {0.55, 2.19},
}

// EstimateCost returns the estimated USD cost for the given model and token counts.
// Returns 0 for unknown models (treat as free rather than erroring).
func EstimateCost(model string, inputTokens, outputTokens int64) float64 {
	in, out, ok := resolvePricing(model)
	if !ok {
		return 0
	}
	return float64(inputTokens)/1_000_000*in + float64(outputTokens)/1_000_000*out
}

// EstimateCostSplit returns the input and output USD cost contributions
// separately for the given model. Used by callers that persist the split
// (e.g. store.CostRecord). Returns (0, 0) for unknown models.
func EstimateCostSplit(model string, inputTokens, outputTokens int64) (inCost, outCost float64) {
	in, out, ok := resolvePricing(model)
	if !ok {
		return 0, 0
	}
	return float64(inputTokens) / 1_000_000 * in,
		float64(outputTokens) / 1_000_000 * out
}
