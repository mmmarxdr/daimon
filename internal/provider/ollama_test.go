package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"daimon/internal/config"
)

func newOllamaTestProvider(t *testing.T) *OllamaProvider {
	t.Helper()
	cfg := config.ProviderConfig{
		Type:    "ollama",
		Model:   "llama3.2",
		BaseURL: "http://localhost:11434/v1",
		// api_key intentionally empty — Ollama does not require one
	}
	p, err := NewOllamaProvider(cfg)
	if err != nil {
		t.Fatalf("NewOllamaProvider: %v", err)
	}
	return p
}

func TestOllamaProvider_Capabilities(t *testing.T) {
	p := newOllamaTestProvider(t)

	if got := p.SupportsMultimodal(); got != false {
		t.Errorf("SupportsMultimodal() = %v, want false", got)
	}
	if got := p.SupportsAudio(); got != false {
		t.Errorf("SupportsAudio() = %v, want false", got)
	}
	if got := p.Name(); got != "ollama" {
		t.Errorf("Name() = %q, want %q", got, "ollama")
	}
}

func TestOllamaProvider_SupportsToolsDelegates(t *testing.T) {
	p := newOllamaTestProvider(t)
	// SupportsTools() must delegate to the embedded OpenAIProvider (returns true).
	if got := p.SupportsTools(); got != true {
		t.Errorf("SupportsTools() = %v, want true (delegated from OpenAIProvider)", got)
	}
}

// --------------------------------------------------------------------------
// Phase 4.1 — OllamaProvider.ListModels()
// --------------------------------------------------------------------------

func newOllamaProviderWithBaseURL(t *testing.T, baseURL string) *OllamaProvider {
	t.Helper()
	cfg := config.ProviderConfig{
		Type:    "ollama",
		Model:   "llama3:latest",
		BaseURL: baseURL + "/v1", // OpenAI-compat path for Chat
	}
	p, err := NewOllamaProvider(cfg)
	if err != nil {
		t.Fatalf("NewOllamaProvider: %v", err)
	}
	return p
}

func TestOllamaProvider_ListModels_Success(t *testing.T) {
	// /api/tags returns two models; assert mapping to ModelInfo
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/tags" {
			t.Errorf("unexpected path %q, want /api/tags", r.URL.Path)
		}
		resp := map[string]any{
			"models": []any{
				map[string]any{"name": "llama3:latest"},
				map[string]any{"name": "mistral:7b"},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer ts.Close()

	p := newOllamaProviderWithBaseURL(t, ts.URL)
	models, err := p.ListModels(context.Background())
	if err != nil {
		t.Fatalf("ListModels() error: %v", err)
	}

	if len(models) != 2 {
		t.Fatalf("expected 2 models, got %d: %v", len(models), models)
	}

	wantModels := []ModelInfo{
		{ID: "llama3:latest", Name: "llama3:latest", Free: true},
		{ID: "mistral:7b", Name: "mistral:7b", Free: true},
	}
	for i, want := range wantModels {
		got := models[i]
		if got.ID != want.ID {
			t.Errorf("models[%d].ID = %q, want %q", i, got.ID, want.ID)
		}
		if got.Name != want.Name {
			t.Errorf("models[%d].Name = %q, want %q", i, got.Name, want.Name)
		}
		if got.Free != want.Free {
			t.Errorf("models[%d].Free = %v, want %v", i, got.Free, want.Free)
		}
	}
}

func TestOllamaProvider_ListModels_ServerError(t *testing.T) {
	// Non-200 response returns a non-nil error, no panic
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "internal error", http.StatusInternalServerError)
	}))
	defer ts.Close()

	p := newOllamaProviderWithBaseURL(t, ts.URL)
	models, err := p.ListModels(context.Background())
	if err == nil {
		t.Fatalf("expected error from non-200 response, got models: %v", models)
	}
	if models != nil {
		t.Errorf("expected nil models on error, got %v", models)
	}
}

func TestOllamaProvider_ListModels_ConnectionRefused(t *testing.T) {
	// Unreachable server returns error, no panic
	p := newOllamaProviderWithBaseURL(t, fmt.Sprintf("http://127.0.0.1:%d", 19999))
	models, err := p.ListModels(context.Background())
	if err == nil {
		t.Fatalf("expected error from connection refused, got models: %v", models)
	}
	if models != nil {
		t.Errorf("expected nil models on error, got %v", models)
	}
}

func TestOllamaProvider_ListModels_EmptyResponse(t *testing.T) {
	// Empty models array returns empty slice, no error
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"models":[]}`))
	}))
	defer ts.Close()

	p := newOllamaProviderWithBaseURL(t, ts.URL)
	models, err := p.ListModels(context.Background())
	if err != nil {
		t.Fatalf("ListModels() error: %v", err)
	}
	if len(models) != 0 {
		t.Errorf("expected 0 models for empty response, got %d", len(models))
	}
}

// --------------------------------------------------------------------------
// Phase 6 — isOllamaReasoningModel heuristic
// --------------------------------------------------------------------------

func TestIsOllamaReasoningModel(t *testing.T) {
	cases := []struct {
		name string
		want bool
	}{
		// Positive — well-known reasoning models
		{"deepseek-r1:7b", true},
		{"deepseek-r1:70b", true},
		{"deepseek-r1:latest", true},
		{"qwq:32b", true},
		{"qwq:latest", true},
		{"qwen3:14b-thinking", true},
		{"marco-o1", true},
		{"marco-o1:latest", true},
		{"reflection:latest", true},
		{"reflection:70b", true},
		// Negative — standard models
		{"llama3:latest", false},
		{"llama3.2:3b", false},
		{"mistral:7b", false},
		{"mistral:latest", false},
		{"gemma3:4b", false},
		{"phi4:latest", false},
		// Negative — qwen without "thinking" suffix
		{"qwen3:14b", false},
		{"qwen2.5:7b", false},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			got := isOllamaReasoningModel(tc.name)
			if got != tc.want {
				t.Errorf("isOllamaReasoningModel(%q) = %v, want %v", tc.name, got, tc.want)
			}
		})
	}
}

// --------------------------------------------------------------------------
// Phase 9 — Ollama ListModels reasoning flag
// --------------------------------------------------------------------------

func TestOllamaListModels_ReasoningFlag(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		resp := map[string]any{
			"models": []any{
				map[string]any{"name": "deepseek-r1:7b"},
				map[string]any{"name": "llama3:latest"},
				map[string]any{"name": "qwq:32b"},
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer ts.Close()

	p := newOllamaProviderWithBaseURL(t, ts.URL)
	models, err := p.ListModels(context.Background())
	if err != nil {
		t.Fatalf("ListModels() error: %v", err)
	}
	if len(models) != 3 {
		t.Fatalf("expected 3 models, got %d", len(models))
	}

	for _, m := range models {
		wantReasoning := isOllamaReasoningModel(m.ID)
		hasReasoning := false
		for _, p := range m.SupportedParameters {
			if p == "reasoning" {
				hasReasoning = true
				break
			}
		}
		if hasReasoning != wantReasoning {
			t.Errorf("model %q: SupportedParameters has reasoning=%v, want %v", m.ID, hasReasoning, wantReasoning)
		}
	}
}

func TestOllamaListModels_NonMatchingModel_NoFlag(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		resp := map[string]any{
			"models": []any{
				map[string]any{"name": "llama3:latest"},
				map[string]any{"name": "mistral:7b"},
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer ts.Close()

	p := newOllamaProviderWithBaseURL(t, ts.URL)
	models, err := p.ListModels(context.Background())
	if err != nil {
		t.Fatalf("ListModels() error: %v", err)
	}
	for _, m := range models {
		if len(m.SupportedParameters) != 0 {
			t.Errorf("model %q: expected empty SupportedParameters, got %v", m.ID, m.SupportedParameters)
		}
	}
}
