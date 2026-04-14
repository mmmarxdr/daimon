package cost

import (
	"testing"
)

// TestLookup_DirectMatch verifies that an exact model name returns the correct pricing.
func TestLookup_DirectMatch(t *testing.T) {
	tests := []struct {
		name      string
		model     string
		wantInput float64
	}{
		{name: "gpt-4o", model: "gpt-4o", wantInput: 0.0000025},
		{name: "gpt-4o-mini", model: "gpt-4o-mini", wantInput: 0.00000015},
		{name: "claude-opus-4-20250514", model: "claude-opus-4-20250514", wantInput: 0.000015},
		{name: "claude-sonnet-4-20250514", model: "claude-sonnet-4-20250514", wantInput: 0.000003},
		{name: "deepseek-v3", model: "deepseek-v3", wantInput: 0.00000027},
		{name: "qwen3.6-plus", model: "qwen3.6-plus", wantInput: 0.0000004},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p, ok := Lookup(tc.model)
			if !ok {
				t.Fatalf("Lookup(%q): not found", tc.model)
			}
			if p.Input != tc.wantInput {
				t.Errorf("Lookup(%q).Input = %g, want %g", tc.model, p.Input, tc.wantInput)
			}
		})
	}
}

// TestLookup_PrefixMatch verifies that model variant names match by prefix.
func TestLookup_PrefixMatch(t *testing.T) {
	tests := []struct {
		name      string
		model     string
		wantInput float64
	}{
		{name: "gpt-4o-dated", model: "gpt-4o-2024-08-06", wantInput: 0.0000025},
		{name: "gpt-4o-mini-dated", model: "gpt-4o-mini-2024-07-18", wantInput: 0.00000015},
		{name: "o3-variant", model: "o3-2025-04-16", wantInput: 0.00001},
		{name: "gemini-flash-variant", model: "gemini-2.5-flash-preview-05-20", wantInput: 0.00000015},
		{name: "deepseek-v3.2-variant", model: "deepseek-v3.2-chat", wantInput: 0.0000003},
		{name: "qwen3-235b-variant", model: "qwen3-235b-a22b-instruct", wantInput: 0.0000004},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p, ok := Lookup(tc.model)
			if !ok {
				t.Fatalf("Lookup(%q): not found", tc.model)
			}
			if p.Input != tc.wantInput {
				t.Errorf("Lookup(%q).Input = %g, want %g", tc.model, p.Input, tc.wantInput)
			}
		})
	}
}

// TestLookup_UnknownModel verifies that an unknown model returns (zero, false).
func TestLookup_UnknownModel(t *testing.T) {
	tests := []string{
		"unknown-model-123",
		"",
		"llama-99b",
		"gpt5", // no prefix match for "gpt5"
	}

	for _, model := range tests {
		t.Run(model, func(t *testing.T) {
			p, ok := Lookup(model)
			if ok {
				t.Errorf("Lookup(%q) should not be found, got %+v", model, p)
			}
			if p.Input != 0 || p.Output != 0 {
				t.Errorf("Lookup(%q) should return zero pricing, got %+v", model, p)
			}
		})
	}
}

// TestLookup_LongestPrefixWins verifies that when multiple prefixes could match,
// the longest prefix is used.
func TestLookup_LongestPrefixWins(t *testing.T) {
	// "deepseek-v3" and "deepseek-v3.2" both exist.
	// "deepseek-v3.2-chat" should match "deepseek-v3.2" (longer), not "deepseek-v3".
	p, ok := Lookup("deepseek-v3.2-chat")
	if !ok {
		t.Fatal("Lookup(deepseek-v3.2-chat) not found")
	}
	// deepseek-v3.2: Input=0.0000003 vs deepseek-v3: Input=0.00000027
	if p.Input != 0.0000003 {
		t.Errorf("Lookup(deepseek-v3.2-chat).Input = %g, want 3e-7 (deepseek-v3.2 pricing)", p.Input)
	}
}

// TestAll_ReturnsNonEmpty verifies that All() returns a populated map.
func TestAll_ReturnsNonEmpty(t *testing.T) {
	models := All()
	if len(models) == 0 {
		t.Fatal("All() returned empty map")
	}
	if _, ok := models["gpt-4o"]; !ok {
		t.Error("All() missing gpt-4o")
	}
}

// TestAll_ReturnsCopy verifies that All() returns an independent copy.
func TestAll_ReturnsCopy(t *testing.T) {
	m1 := All()
	m1["test-injected"] = ModelPricing{Input: 99, Output: 99}
	m2 := All()
	if _, ok := m2["test-injected"]; ok {
		t.Error("All() returned shared map — mutation leaked")
	}
}
