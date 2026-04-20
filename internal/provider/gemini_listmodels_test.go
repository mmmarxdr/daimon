package provider

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"daimon/internal/config"
)

// ---------------------------------------------------------------------------
// Phase 5 — Gemini ListModels: reasoning flag via capability map
// ---------------------------------------------------------------------------

// geminiModelsResponse builds a fake /v1beta/models response containing the given models.
func geminiModelsResponse(models []map[string]any) map[string]any {
	return map[string]any{
		"models": models,
	}
}

// geminiModelEntry builds a single model entry for the ListModels response.
func geminiModelEntry(name, displayName string, inputLimit, outputLimit int, methods []string) map[string]any {
	return map[string]any{
		"name":                       "models/" + name,
		"displayName":                displayName,
		"inputTokenLimit":            inputLimit,
		"outputTokenLimit":           outputLimit,
		"supportedGenerationMethods": methods,
	}
}

// TestGeminiListModels_ReasoningFlag — Task 5.1 (RED → GREEN)
// Req ADR 7: capable model in map → SupportedParameters includes "reasoning".
func TestGeminiListModels_ReasoningFlag(t *testing.T) {
	models := []map[string]any{
		geminiModelEntry("gemini-2.5-pro", "Gemini 2.5 Pro", 1048576, 8192, []string{"generateContent"}),
		geminiModelEntry("gemini-2.5-flash", "Gemini 2.5 Flash", 1048576, 8192, []string{"generateContent"}),
		geminiModelEntry("gemini-2.0-flash", "Gemini 2.0 Flash", 1048576, 8192, []string{"generateContent"}),
		geminiModelEntry("gemini-1.5-pro", "Gemini 1.5 Pro", 2097152, 8192, []string{"generateContent"}),
	}

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(geminiModelsResponse(models))
	}))
	defer ts.Close()

	p := NewGeminiProvider(config.ProviderConfig{
		APIKey:  "test-key",
		BaseURL: ts.URL,
	})

	result, err := p.ListModels(context.Background())
	if err != nil {
		t.Fatalf("ListModels: %v", err)
	}

	// Build a lookup by ID.
	byID := make(map[string]ModelInfo)
	for _, m := range result {
		byID[m.ID] = m
	}

	reasoningModels := []string{"gemini-2.5-pro", "gemini-2.5-flash"}
	for _, id := range reasoningModels {
		m, ok := byID[id]
		if !ok {
			t.Errorf("model %q not found in ListModels result", id)
			continue
		}
		var hasReasoning bool
		for _, p := range m.SupportedParameters {
			if p == "reasoning" {
				hasReasoning = true
				break
			}
		}
		if !hasReasoning {
			t.Errorf("model %q: expected SupportedParameters to include 'reasoning', got %v", id, m.SupportedParameters)
		}
	}

	nonReasoningModels := []string{"gemini-2.0-flash", "gemini-1.5-pro"}
	for _, id := range nonReasoningModels {
		m, ok := byID[id]
		if !ok {
			t.Errorf("model %q not found in ListModels result", id)
			continue
		}
		for _, p := range m.SupportedParameters {
			if p == "reasoning" {
				t.Errorf("model %q: expected NO 'reasoning' in SupportedParameters, got %v", id, m.SupportedParameters)
			}
		}
	}
}
