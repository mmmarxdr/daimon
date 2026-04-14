package cost

import (
	"fmt"
	"math"
	"time"
)

// CostResult holds the computed USD cost for a single LLM call.
// This is pure computation output — the caller adds metadata (session, channel, model)
// to build a store.CostRecord for persistence.
type CostResult struct {
	InputCostUSD  float64
	OutputCostUSD float64
	TotalCostUSD  float64
	Timestamp     time.Time
	Ok            bool // false if model pricing was not found
}

// ComputeCost calculates the USD cost for a model call given token counts.
// Returns CostResult with Ok=false if the model is unknown.
func ComputeCost(model string, inputTokens, outputTokens int) CostResult {
	pricing, ok := Lookup(model)
	if !ok {
		return CostResult{}
	}

	inCost := float64(inputTokens) * pricing.Input
	outCost := float64(outputTokens) * pricing.Output
	total := inCost + outCost

	return CostResult{
		InputCostUSD:  roundUSD(inCost),
		OutputCostUSD: roundUSD(outCost),
		TotalCostUSD:  roundUSD(total),
		Timestamp:     time.Now().UTC(),
		Ok:            true,
	}
}

// FormatCost returns a human-readable cost string like "$0.0012".
func FormatCost(usd float64) string {
	return fmt.Sprintf("$%.4f", usd)
}

// roundUSD rounds to 6 decimal places (micro-dollars).
func roundUSD(v float64) float64 {
	return math.Round(v*1e6) / 1e6
}
