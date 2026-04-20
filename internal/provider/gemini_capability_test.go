package provider

import "testing"

// ---------------------------------------------------------------------------
// Phase 2 — Gemini capability map + type tests
// ---------------------------------------------------------------------------

// TestGeminiCapabilityFor_ExactMatch — Task 2.1 (RED → GREEN)
// Req 3: gemini-2.5-pro → hit; gpt-4o → miss.
func TestGeminiCapabilityFor_ExactMatch(t *testing.T) {
	tests := []struct {
		model   string
		wantHit bool
	}{
		{"gemini-2.5-pro", true},
		{"gemini-2.5-flash", true},
		{"gemini-2.5-flash-lite", true},
		{"gpt-4o", false},
		{"claude-opus-4-6", false},
		{"gemini-1.5-pro", false},
		{"gemini-2.0-flash", false},
	}

	for _, tc := range tests {
		cap, ok := geminiCapabilityFor(tc.model)
		if ok != tc.wantHit {
			t.Errorf("geminiCapabilityFor(%q): hit=%v, want %v", tc.model, ok, tc.wantHit)
		}
		if tc.wantHit {
			if !cap.SupportsThinking {
				t.Errorf("geminiCapabilityFor(%q): SupportsThinking=false, want true", tc.model)
			}
			if !cap.IncludeThoughts {
				t.Errorf("geminiCapabilityFor(%q): IncludeThoughts=false, want true", tc.model)
			}
		}
	}
}

// TestGeminiCapabilityFor_PrefixFallback — Task 2.2 (RED → GREEN)
// gemini-2.5-pro-preview-0506 → hit via HasPrefix fallback for preview/exp variants.
func TestGeminiCapabilityFor_PrefixFallback(t *testing.T) {
	tests := []struct {
		model   string
		wantHit bool
	}{
		{"gemini-2.5-pro-preview-0506", true},
		{"gemini-2.5-flash-preview-04-17", true},
		{"gemini-2.5-flash-exp-0827", true},
		{"gemini-2.5-flash-lite-preview-06-17", true},
		{"gemini-1.5-pro-001", false}, // 1.5 not in capability map
		{"gemini-2.0-flash-001", false},
	}

	for _, tc := range tests {
		_, ok := geminiCapabilityFor(tc.model)
		if ok != tc.wantHit {
			t.Errorf("geminiCapabilityFor(%q): hit=%v, want %v (prefix fallback)", tc.model, ok, tc.wantHit)
		}
	}
}
