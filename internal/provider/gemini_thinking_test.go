package provider

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"daimon/internal/config"
	"daimon/internal/content"
)

// ---------------------------------------------------------------------------
// Phase 3 — Gemini request builder: thinkingConfig injection
// ---------------------------------------------------------------------------

// captureBuildGeminiRequest builds a geminiRequest and marshals it, allowing
// assertions on the JSON shape.
func buildAndMarshal(t *testing.T, p *GeminiProvider, req ChatRequest) map[string]any {
	t.Helper()
	apiReq := p.buildGeminiRequest(context.Background(), req)
	b, err := json.Marshal(apiReq)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	var out map[string]any
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	return out
}

// hasThinkingConfig returns true if the generationConfig contains thinkingConfig.
func hasThinkingConfig(body map[string]any) bool {
	gc, ok := body["generationConfig"].(map[string]any)
	if !ok {
		return false
	}
	_, ok = gc["thinkingConfig"]
	return ok
}

// thinkingBudget extracts the thinkingBudget value from the JSON body.
func thinkingBudget(body map[string]any) (float64, bool) {
	gc, ok := body["generationConfig"].(map[string]any)
	if !ok {
		return 0, false
	}
	tc, ok := gc["thinkingConfig"].(map[string]any)
	if !ok {
		return 0, false
	}
	v, ok := tc["thinkingBudget"].(float64)
	return v, ok
}

// newGeminiProviderWithThinking constructs a GeminiProvider with a given thinking config.
func newGeminiProviderWithThinking(baseURL, model string, thinking *config.ProviderThinkingConfig) *GeminiProvider {
	p := NewGeminiProvider(config.ProviderConfig{
		APIKey:  "test-key",
		Model:   model,
		BaseURL: baseURL,
	})
	p.SetThinkingConfig(config.ProviderCredentials{
		Thinking: thinking,
	})
	return p
}

// TestBuildGeminiRequest_InjectsThinkingConfig — Task 3.1 (RED → GREEN)
// Req 2, Sc: Capable model + budget set → thinkingConfig injected.
func TestBuildGeminiRequest_InjectsThinkingConfig(t *testing.T) {
	budget := 8192
	p := newGeminiProviderWithThinking("http://dummy", "gemini-2.5-pro", &config.ProviderThinkingConfig{
		BudgetTokens: budget,
	})
	req := ChatRequest{
		Messages: []ChatMessage{{Role: "user", Content: content.TextBlock("hello")}},
	}

	body := buildAndMarshal(t, p, req)

	if !hasThinkingConfig(body) {
		t.Errorf("expected thinkingConfig in generationConfig, got body: %v", body["generationConfig"])
	}
	if bgt, ok := thinkingBudget(body); !ok || int(bgt) != budget {
		t.Errorf("thinkingBudget = %v, want %d", bgt, budget)
	}
}

// TestBuildGeminiRequest_SkipsNonCapable — Task 3.2 (RED → GREEN)
// Req 2/3: Non-capable model → thinkingConfig absent.
func TestBuildGeminiRequest_SkipsNonCapable(t *testing.T) {
	p := newGeminiProviderWithThinking("http://dummy", "gemini-1.5-pro", &config.ProviderThinkingConfig{
		BudgetTokens: 8192,
	})
	req := ChatRequest{
		Messages: []ChatMessage{{Role: "user", Content: content.TextBlock("hello")}},
	}

	body := buildAndMarshal(t, p, req)

	if hasThinkingConfig(body) {
		t.Errorf("expected NO thinkingConfig for non-capable model gemini-1.5-pro")
	}
}

// TestBuildGeminiRequest_DefaultBudgetMinusOne — Task 3.3 (RED → GREEN)
// Req 2, Sc: Capable model, no explicit budget → thinkingBudget == -1 (dynamic).
func TestBuildGeminiRequest_DefaultBudgetMinusOne(t *testing.T) {
	// Thinking block present but BudgetTokens == 0 (unset) → should default to -1.
	p := newGeminiProviderWithThinking("http://dummy", "gemini-2.5-flash", &config.ProviderThinkingConfig{
		// No BudgetTokens set — zero value means "use default"
	})
	req := ChatRequest{
		Messages: []ChatMessage{{Role: "user", Content: content.TextBlock("hello")}},
	}

	body := buildAndMarshal(t, p, req)

	if !hasThinkingConfig(body) {
		t.Errorf("expected thinkingConfig for capable model with no explicit budget")
	}
	bgt, ok := thinkingBudget(body)
	if !ok {
		t.Errorf("expected thinkingBudget to be present")
	}
	if int(bgt) != -1 {
		t.Errorf("thinkingBudget = %v, want -1 (dynamic)", bgt)
	}
}

// TestBuildGeminiRequest_ExplicitDisabledSkips — Task 3.4 (RED → GREEN)
// Req 10: Capable model + Enabled=false → no thinkingConfig.
func TestBuildGeminiRequest_ExplicitDisabledSkips(t *testing.T) {
	disabled := false
	p := newGeminiProviderWithThinking("http://dummy", "gemini-2.5-pro", &config.ProviderThinkingConfig{
		Enabled:      &disabled,
		BudgetTokens: 4096,
	})
	req := ChatRequest{
		Messages: []ChatMessage{{Role: "user", Content: content.TextBlock("hello")}},
	}

	body := buildAndMarshal(t, p, req)

	if hasThinkingConfig(body) {
		t.Errorf("expected NO thinkingConfig when Enabled=false (explicit user opt-out)")
	}
}

// TestBuildGeminiRequest_AutoActivate_CapableNoConfig — Task 10.1 (RED → GREEN — included here for locality)
// Req 10: Capable model, thinking block absent → thinkingConfig injected with budget -1.
func TestBuildGeminiRequest_AutoActivate_CapableNoConfig(t *testing.T) {
	// No SetThinkingConfig called — simulates absent thinking block (creds.Thinking==nil).
	p := NewGeminiProvider(config.ProviderConfig{
		APIKey:  "test-key",
		Model:   "gemini-2.5-pro",
		BaseURL: "http://dummy",
	})
	req := ChatRequest{
		Messages: []ChatMessage{{Role: "user", Content: content.TextBlock("hello")}},
	}

	body := buildAndMarshal(t, p, req)

	if !hasThinkingConfig(body) {
		t.Errorf("expected thinkingConfig auto-activated for capable model with no config block")
	}
	bgt, ok := thinkingBudget(body)
	if !ok {
		t.Fatal("expected thinkingBudget to be present")
	}
	if int(bgt) != -1 {
		t.Errorf("thinkingBudget = %v, want -1 for auto-activation", bgt)
	}
}

// TestBuildGeminiRequest_AutoActivate_ExplicitDisabledOverrides — Task 10.2
// Req 10: Capable model + Enabled=false → no injection even with auto-activation.
func TestBuildGeminiRequest_AutoActivate_ExplicitDisabledOverrides(t *testing.T) {
	disabled := false
	p := newGeminiProviderWithThinking("http://dummy", "gemini-2.5-flash", &config.ProviderThinkingConfig{
		Enabled: &disabled,
	})
	req := ChatRequest{
		Messages: []ChatMessage{{Role: "user", Content: content.TextBlock("hello")}},
	}

	body := buildAndMarshal(t, p, req)

	if hasThinkingConfig(body) {
		t.Errorf("expected NO thinkingConfig when Enabled=false overrides capable model")
	}
}

// TestBuildGeminiRequest_ThinkingConfig_IncludeThoughtsTrue verifies includeThoughts:true is set.
func TestBuildGeminiRequest_ThinkingConfig_IncludeThoughtsTrue(t *testing.T) {
	p := newGeminiProviderWithThinking("http://dummy", "gemini-2.5-pro", &config.ProviderThinkingConfig{
		BudgetTokens: 4096,
	})
	req := ChatRequest{
		Messages: []ChatMessage{{Role: "user", Content: content.TextBlock("hello")}},
	}
	body := buildAndMarshal(t, p, req)

	gc, _ := body["generationConfig"].(map[string]any)
	tc, _ := gc["thinkingConfig"].(map[string]any)
	includeThoughts, _ := tc["includeThoughts"].(bool)
	if !includeThoughts {
		t.Errorf("expected includeThoughts=true in thinkingConfig, got: %v", tc)
	}
}

// TestBuildGeminiRequest_CapableModel_ViaHTTP verifies the JSON sent over the wire to the server.
func TestBuildGeminiRequest_CapableModel_ViaHTTP(t *testing.T) {
	var capturedBody map[string]any

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&capturedBody); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		resp := geminiOKResponse("ok")
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer ts.Close()

	budget := 2048
	p := NewGeminiProvider(config.ProviderConfig{
		APIKey:  "test-key",
		Model:   "gemini-2.5-flash",
		BaseURL: ts.URL,
	})
	p.SetThinkingConfig(config.ProviderCredentials{
		Thinking: &config.ProviderThinkingConfig{BudgetTokens: budget},
	})

	_, _ = p.Chat(context.Background(), ChatRequest{
		Messages: []ChatMessage{{Role: "user", Content: content.TextBlock("hello")}},
	})

	if !hasThinkingConfig(capturedBody) {
		t.Errorf("expected thinkingConfig in wire request, got: %v", capturedBody["generationConfig"])
	}
}
