package provider

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"daimon/internal/config"
	"daimon/internal/content"
)

// ---------------------------------------------------------------------------
// Phase 4 — Gemini stream parser: thought part → StreamEventReasoningDelta
// ---------------------------------------------------------------------------

// geminiThoughtChunk builds a Gemini SSE chunk with a thought part.
func geminiThoughtChunk(text string, promptTokens, candidateTokens int) string {
	return geminiSSEFrame(
		`{"candidates":[{"content":{"parts":[{"thought":true,"text":"` + text + `"}],"role":"model"},"index":0}],"usageMetadata":{"promptTokenCount":` +
			itoa(promptTokens) + `,"candidatesTokenCount":` + itoa(candidateTokens) + `}}`,
	)
}

// geminiMixedThoughtAndText builds a chunk with both a thought part and a text part.
func geminiMixedThoughtAndText(thoughtText, textContent string, promptTokens, candidateTokens int) string {
	return geminiSSEFrame(
		`{"candidates":[{"content":{"parts":[{"thought":true,"text":"` + thoughtText + `"},{"text":"` + textContent + `"}],"role":"model"},"index":0}],"usageMetadata":{"promptTokenCount":` +
			itoa(promptTokens) + `,"candidatesTokenCount":` + itoa(candidateTokens) + `}}`,
	)
}

// geminiEmptyThoughtChunk builds a chunk with an empty thought part + STOP.
func geminiEmptyThoughtChunkWithStop(promptTokens, candidateTokens int) string {
	return geminiSSEFrame(
		`{"candidates":[{"content":{"parts":[{"thought":true,"text":""}],"role":"model"},"index":0,"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":` +
			itoa(promptTokens) + `,"candidatesTokenCount":` + itoa(candidateTokens) + `}}`,
	)
}

// itoa is a minimal int-to-string helper for test SSE payload building.
func itoa(n int) string {
	if n < 0 {
		return "-" + itoa(-n)
	}
	if n < 10 {
		return string(rune('0' + n))
	}
	return itoa(n/10) + string(rune('0'+n%10))
}

// newThinkingGeminiProvider creates a Gemini provider with thinking enabled (budget 8192).
func newThinkingGeminiProvider(serverURL string) *GeminiProvider {
	p := NewGeminiProvider(config.ProviderConfig{
		APIKey:  "test-key",
		Model:   "gemini-2.5-pro",
		BaseURL: serverURL,
	})
	p.SetThinkingConfig(config.ProviderCredentials{
		Thinking: &config.ProviderThinkingConfig{BudgetTokens: 8192},
	})
	return p
}

// collectStreamEvents drains a StreamResult and returns all events.
func collectStreamEvents(t *testing.T, sr *StreamResult) []StreamEvent {
	t.Helper()
	var events []StreamEvent
	for ev := range sr.Events {
		events = append(events, ev)
		if ev.Type == StreamEventError {
			t.Logf("stream error: %v", ev.Err)
		}
	}
	return events
}

// serveSSE starts an httptest.Server that returns the given SSE payload.
func serveSSE(payload string) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(payload))
	}))
}

// TestGeminiStream_ThoughtPartEmitsReasoningDelta — Task 4.2 (RED → GREEN)
// Req 1, Sc 1: thought=true part → StreamEventReasoningDelta.
func TestGeminiStream_ThoughtPartEmitsReasoningDelta(t *testing.T) {
	payload := strings.Join([]string{
		geminiThoughtChunk("Let me think...", 10, 5),
		geminiTextChunkWithStop("Answer.", "STOP", 10, 10),
	}, "")

	ts := serveSSE(payload)
	defer ts.Close()

	prov := newThinkingGeminiProvider(ts.URL)
	sr, err := prov.ChatStream(context.Background(), ChatRequest{
		Messages: []ChatMessage{{Role: "user", Content: content.TextBlock("hi")}},
	})
	if err != nil {
		t.Fatalf("ChatStream: %v", err)
	}

	events := collectStreamEvents(t, sr)

	var reasoningDeltas []string
	for _, ev := range events {
		if ev.Type == StreamEventReasoningDelta {
			reasoningDeltas = append(reasoningDeltas, ev.Text)
		}
	}

	if len(reasoningDeltas) == 0 {
		t.Fatalf("expected at least one StreamEventReasoningDelta, got events: %v", events)
	}
	if reasoningDeltas[0] != "Let me think..." {
		t.Errorf("ReasoningDelta text = %q, want %q", reasoningDeltas[0], "Let me think...")
	}
}

// TestGeminiStream_TextPartEmitsTextDelta — Task 4.3 (RED → GREEN)
// Req 1, Sc 2: non-thought part → StreamEventTextDelta.
func TestGeminiStream_TextPartEmitsTextDelta(t *testing.T) {
	payload := strings.Join([]string{
		geminiThoughtChunk("Thinking...", 10, 5),
		geminiTextChunkWithStop("The answer is 42.", "STOP", 10, 10),
	}, "")

	ts := serveSSE(payload)
	defer ts.Close()

	prov := newThinkingGeminiProvider(ts.URL)
	sr, err := prov.ChatStream(context.Background(), ChatRequest{
		Messages: []ChatMessage{{Role: "user", Content: content.TextBlock("what is the answer")}},
	})
	if err != nil {
		t.Fatalf("ChatStream: %v", err)
	}

	events := collectStreamEvents(t, sr)

	var textDeltas []string
	for _, ev := range events {
		if ev.Type == StreamEventTextDelta {
			textDeltas = append(textDeltas, ev.Text)
		}
	}

	if len(textDeltas) == 0 {
		t.Fatalf("expected at least one StreamEventTextDelta, got events: %v", events)
	}
	if textDeltas[0] != "The answer is 42." {
		t.Errorf("TextDelta text = %q, want %q", textDeltas[0], "The answer is 42.")
	}
}

// TestGeminiStream_MixedPartsOrder — Task 4.4 (RED → GREEN)
// Req 1, Sc 3: chunk with both thought + text parts → ReasoningDelta THEN TextDelta, in order.
func TestGeminiStream_MixedPartsOrder(t *testing.T) {
	payload := strings.Join([]string{
		geminiMixedThoughtAndText("Thinking hard.", "Response text.", 10, 15),
		geminiTextChunkWithStop("", "STOP", 10, 15),
	}, "")

	ts := serveSSE(payload)
	defer ts.Close()

	prov := newThinkingGeminiProvider(ts.URL)
	sr, err := prov.ChatStream(context.Background(), ChatRequest{
		Messages: []ChatMessage{{Role: "user", Content: content.TextBlock("test mixed")}},
	})
	if err != nil {
		t.Fatalf("ChatStream: %v", err)
	}

	events := collectStreamEvents(t, sr)

	// Find first ReasoningDelta and first TextDelta and verify ordering.
	var firstReasoning, firstText int = -1, -1
	for i, ev := range events {
		if ev.Type == StreamEventReasoningDelta && firstReasoning == -1 {
			firstReasoning = i
		}
		if ev.Type == StreamEventTextDelta && firstText == -1 {
			firstText = i
		}
	}

	if firstReasoning == -1 {
		t.Fatal("expected StreamEventReasoningDelta in events")
	}
	if firstText == -1 {
		t.Fatal("expected StreamEventTextDelta in events")
	}
	if firstReasoning >= firstText {
		t.Errorf("expected ReasoningDelta (idx %d) before TextDelta (idx %d)", firstReasoning, firstText)
	}
}

// TestGeminiStream_EmptyThoughtSkipped — Task 4.5 (RED → GREEN)
// Req 1, Sc 4: empty thought part → no ReasoningDelta emitted.
func TestGeminiStream_EmptyThoughtSkipped(t *testing.T) {
	payload := geminiEmptyThoughtChunkWithStop(10, 10)

	ts := serveSSE(payload)
	defer ts.Close()

	prov := newThinkingGeminiProvider(ts.URL)
	sr, err := prov.ChatStream(context.Background(), ChatRequest{
		Messages: []ChatMessage{{Role: "user", Content: content.TextBlock("test empty thought")}},
	})
	if err != nil {
		t.Fatalf("ChatStream: %v", err)
	}

	events := collectStreamEvents(t, sr)

	for _, ev := range events {
		if ev.Type == StreamEventReasoningDelta {
			t.Errorf("expected no StreamEventReasoningDelta for empty thought, got ev: %+v", ev)
		}
	}
}

// TestGeminiStream_ThoughtNotAccumulatedInContent verifies reasoning text is NOT
// accumulated into ChatResponse.Content (mirrors OpenRouter behavior).
func TestGeminiStream_ThoughtNotAccumulatedInContent(t *testing.T) {
	payload := strings.Join([]string{
		geminiThoughtChunk("Internal reasoning.", 10, 5),
		geminiTextChunkWithStop("Final answer.", "STOP", 10, 10),
	}, "")

	ts := serveSSE(payload)
	defer ts.Close()

	prov := newThinkingGeminiProvider(ts.URL)
	sr, err := prov.ChatStream(context.Background(), ChatRequest{
		Messages: []ChatMessage{{Role: "user", Content: content.TextBlock("test")}},
	})
	if err != nil {
		t.Fatalf("ChatStream: %v", err)
	}

	for range sr.Events {
	}

	resp, err := sr.Response()
	if err != nil {
		t.Fatalf("Response(): %v", err)
	}
	if resp.Content != "Final answer." {
		t.Errorf("Content = %q, want 'Final answer.' (reasoning should NOT be in content)", resp.Content)
	}
	if strings.Contains(resp.Content, "Internal reasoning.") {
		t.Errorf("Content must NOT contain reasoning text: %q", resp.Content)
	}
}

// TestGeminiStream_Fixture_ThoughtAndText reads the golden SSE fixture and verifies events.
func TestGeminiStream_Fixture_ThoughtAndText(t *testing.T) {
	fixtureData, err := os.ReadFile("testdata/gemini_stream_thought.sse")
	if err != nil {
		t.Fatalf("reading fixture: %v", err)
	}

	ts := serveSSE(string(fixtureData))
	defer ts.Close()

	prov := newThinkingGeminiProvider(ts.URL)
	sr, err := prov.ChatStream(context.Background(), ChatRequest{
		Messages: []ChatMessage{{Role: "user", Content: content.TextBlock("fixture test")}},
	})
	if err != nil {
		t.Fatalf("ChatStream: %v", err)
	}

	events := collectStreamEvents(t, sr)

	var gotReasoning, gotText bool
	for _, ev := range events {
		if ev.Type == StreamEventReasoningDelta {
			gotReasoning = true
		}
		if ev.Type == StreamEventTextDelta {
			gotText = true
		}
	}

	if !gotReasoning {
		t.Error("expected at least one ReasoningDelta from fixture")
	}
	if !gotText {
		t.Error("expected at least one TextDelta from fixture")
	}
}
