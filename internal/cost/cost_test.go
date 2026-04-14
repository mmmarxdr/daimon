package cost

import (
	"math"
	"testing"
)

// TestComputeCost_KnownModel verifies cost computation for a known model.
func TestComputeCost_KnownModel(t *testing.T) {
	// gpt-4o: input=0.0000025, output=0.00001 per token
	// 1000 input tokens = 1000 * 0.0000025 = $0.0025
	// 500 output tokens  = 500  * 0.00001   = $0.005
	// total = $0.0075
	cr := ComputeCost("gpt-4o", 1000, 500)
	if !cr.Ok {
		t.Fatal("ComputeCost(gpt-4o): Ok should be true")
	}
	if cr.InputCostUSD != 0.0025 {
		t.Errorf("InputCostUSD = %g, want 0.0025", cr.InputCostUSD)
	}
	if cr.OutputCostUSD != 0.005 {
		t.Errorf("OutputCostUSD = %g, want 0.005", cr.OutputCostUSD)
	}
	if cr.TotalCostUSD != 0.0075 {
		t.Errorf("TotalCostUSD = %g, want 0.0075", cr.TotalCostUSD)
	}
	if cr.Timestamp.IsZero() {
		t.Error("Timestamp should be non-zero")
	}
}

// TestComputeCost_UnknownModel verifies that an unknown model returns Ok=false.
func TestComputeCost_UnknownModel(t *testing.T) {
	cr := ComputeCost("nonexistent-model", 1000, 500)
	if cr.Ok {
		t.Error("ComputeCost(unknown): Ok should be false")
	}
	if cr.TotalCostUSD != 0 {
		t.Errorf("ComputeCost(unknown): TotalCostUSD should be 0, got %g", cr.TotalCostUSD)
	}
}

// TestComputeCost_ZeroTokens verifies zero-token calls produce zero cost.
func TestComputeCost_ZeroTokens(t *testing.T) {
	cr := ComputeCost("gpt-4o", 0, 0)
	if !cr.Ok {
		t.Fatal("Ok should be true even with zero tokens")
	}
	if cr.InputCostUSD != 0 || cr.OutputCostUSD != 0 || cr.TotalCostUSD != 0 {
		t.Errorf("expected zero costs, got input=%g output=%g total=%g",
			cr.InputCostUSD, cr.OutputCostUSD, cr.TotalCostUSD)
	}
}

// TestComputeCost_PrefixMatch verifies cost computation works with model variant names.
func TestComputeCost_PrefixMatch(t *testing.T) {
	cr := ComputeCost("gpt-4o-2024-08-06", 1000, 500)
	if !cr.Ok {
		t.Fatal("ComputeCost(gpt-4o-2024-08-06): Ok should be true")
	}
	// Same as gpt-4o pricing
	if cr.TotalCostUSD != 0.0075 {
		t.Errorf("TotalCostUSD = %g, want 0.0075", cr.TotalCostUSD)
	}
}

// TestComputeCost_Rounding verifies micro-dollar rounding.
func TestComputeCost_Rounding(t *testing.T) {
	// claude-3-haiku-20240307: input=0.00000025, output=0.00000125
	// 10000 input = 10000 * 0.00000025 = $0.0025
	// 10000 output = 10000 * 0.00000125 = $0.0125
	cr := ComputeCost("claude-3-haiku-20240307", 10000, 10000)
	if !cr.Ok {
		t.Fatal("Ok should be true")
	}
	expected := 0.015
	if math.Abs(cr.TotalCostUSD-expected) > 1e-6 {
		t.Errorf("TotalCostUSD = %g, want %g", cr.TotalCostUSD, expected)
	}
}

// TestComputeCost_MultipleModels verifies cost across different model families.
func TestComputeCost_MultipleModels(t *testing.T) {
	tests := []struct {
		name      string
		model     string
		inTokens  int
		outTokens int
		wantTotal float64
	}{
		{name: "gpt-4o-mini", model: "gpt-4o-mini", inTokens: 10000, outTokens: 5000, wantTotal: 0.0045},
		{name: "deepseek-v3", model: "deepseek-v3", inTokens: 1000, outTokens: 1000, wantTotal: 0.00137},
		{name: "o3-mini", model: "o3-mini", inTokens: 500, outTokens: 200, wantTotal: 0.00143},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cr := ComputeCost(tc.model, tc.inTokens, tc.outTokens)
			if !cr.Ok {
				t.Fatalf("ComputeCost(%s): Ok should be true", tc.model)
			}
			if math.Abs(cr.TotalCostUSD-tc.wantTotal) > 1e-6 {
				t.Errorf("TotalCostUSD = %g, want %g", cr.TotalCostUSD, tc.wantTotal)
			}
		})
	}
}

// TestFormatCost verifies the human-readable cost string format.
func TestFormatCost(t *testing.T) {
	tests := []struct {
		name string
		usd  float64
		want string
	}{
		{name: "zero", usd: 0, want: "$0.0000"},
		{name: "small", usd: 0.0001, want: "$0.0001"},
		{name: "penny", usd: 0.01, want: "$0.0100"},
		{name: "dollar", usd: 1.50, want: "$1.5000"},
		{name: "fractional", usd: 0.0075, want: "$0.0075"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := FormatCost(tc.usd)
			if got != tc.want {
				t.Errorf("FormatCost(%g) = %q, want %q", tc.usd, got, tc.want)
			}
		})
	}
}
