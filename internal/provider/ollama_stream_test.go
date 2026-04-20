package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"daimon/internal/config"
	"daimon/internal/content"
)

// --------------------------------------------------------------------------
// helpers
// --------------------------------------------------------------------------

func newOllamaStreamProvider(t *testing.T, baseURL string, model string) *OllamaProvider {
	t.Helper()
	cfg := config.ProviderConfig{
		Type:    "ollama",
		Model:   model,
		BaseURL: baseURL + "/v1",
	}
	p, err := NewOllamaProvider(cfg)
	if err != nil {
		t.Fatalf("NewOllamaProvider: %v", err)
	}
	return p
}

// textMsg builds a user ChatMessage with a single text block.
func textMsg(text string) ChatMessage {
	return ChatMessage{
		Role:    "user",
		Content: content.Blocks{{Type: content.BlockText, Text: text}},
	}
}

// serveNDJSONFixture writes the contents of testdata/ollama_stream_reasoning.ndjson to w.
func serveNDJSONFixture(t *testing.T, w http.ResponseWriter) {
	t.Helper()
	data, err := os.ReadFile("testdata/ollama_stream_reasoning.ndjson")
	if err != nil {
		t.Fatalf("reading fixture: %v", err)
	}
	w.Header().Set("Content-Type", "application/x-ndjson")
	_, _ = w.Write(data)
}

// --------------------------------------------------------------------------
// Interface compliance
// --------------------------------------------------------------------------

func TestOllamaProvider_ImplementsStreamingProvider(t *testing.T) {
	var _ StreamingProvider = (*OllamaProvider)(nil)
}

// --------------------------------------------------------------------------
// Phase 7 — ChatStream sends to /api/chat
// --------------------------------------------------------------------------

func TestOllamaChatStream_ReasoningModel_UseNativePath(t *testing.T) {
	// A reasoning model must POST to /api/chat with think:true.
	var gotPath string
	var gotBody map[string]any

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Errorf("decoding request body: %v", err)
		}
		serveNDJSONFixture(t, w)
	}))
	defer ts.Close()

	p := newOllamaStreamProvider(t, ts.URL, "deepseek-r1:7b")
	sr, err := p.ChatStream(context.Background(), ChatRequest{
		Messages: []ChatMessage{textMsg("hello")},
	})
	if err != nil {
		t.Fatalf("ChatStream() error: %v", err)
	}
	collectStreamEvents(t, sr)

	if gotPath != "/api/chat" {
		t.Errorf("path = %q, want /api/chat", gotPath)
	}
	if think, ok := gotBody["think"].(bool); !ok || !think {
		t.Errorf("think field = %v (%T), want true (bool)", gotBody["think"], gotBody["think"])
	}
}

func TestOllamaChatStream_NonReasoningModel_NoThinkFlag(t *testing.T) {
	// A non-reasoning model must POST to /api/chat without think:true.
	var gotPath string
	var gotBody map[string]any

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Errorf("decoding request body: %v", err)
		}
		w.Header().Set("Content-Type", "application/x-ndjson")
		_, _ = fmt.Fprintln(w, `{"model":"llama3:latest","message":{"role":"assistant","content":"Hello!"},"done":false}`)
		_, _ = fmt.Fprintln(w, `{"model":"llama3:latest","message":{"role":"assistant","content":""},"done":true,"done_reason":"stop","prompt_eval_count":5,"eval_count":3}`)
	}))
	defer ts.Close()

	p := newOllamaStreamProvider(t, ts.URL, "llama3:latest")
	sr, err := p.ChatStream(context.Background(), ChatRequest{
		Messages: []ChatMessage{textMsg("hello")},
	})
	if err != nil {
		t.Fatalf("ChatStream() error: %v", err)
	}
	collectStreamEvents(t, sr)

	if gotPath != "/api/chat" {
		t.Errorf("path = %q, want /api/chat", gotPath)
	}
	// think must NOT be true for non-reasoning models.
	if think, exists := gotBody["think"]; exists && think == true {
		t.Errorf("think should not be true for non-reasoning model, got %v", think)
	}
}

// --------------------------------------------------------------------------
// Phase 8 — NDJSON stream parser
// --------------------------------------------------------------------------

func TestOllamaStream_ThinkingFieldEmitsReasoningDelta(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/x-ndjson")
		_, _ = fmt.Fprintln(w, `{"model":"deepseek-r1:7b","message":{"role":"assistant","content":"","thinking":"Let me reason."},"done":false}`)
		_, _ = fmt.Fprintln(w, `{"model":"deepseek-r1:7b","message":{"role":"assistant","content":""},"done":true,"done_reason":"stop","prompt_eval_count":5,"eval_count":3}`)
	}))
	defer ts.Close()

	p := newOllamaStreamProvider(t, ts.URL, "deepseek-r1:7b")
	sr, err := p.ChatStream(context.Background(), ChatRequest{
		Messages: []ChatMessage{textMsg("go")},
	})
	if err != nil {
		t.Fatalf("ChatStream: %v", err)
	}
	events := collectStreamEvents(t, sr)

	var reasoningEvents []StreamEvent
	for _, ev := range events {
		if ev.Type == StreamEventReasoningDelta {
			reasoningEvents = append(reasoningEvents, ev)
		}
	}
	if len(reasoningEvents) == 0 {
		t.Fatal("expected at least one ReasoningDelta event, got none")
	}
	if !strings.Contains(reasoningEvents[0].Text, "Let me reason") {
		t.Errorf("ReasoningDelta text = %q, want to contain 'Let me reason'", reasoningEvents[0].Text)
	}
}

func TestOllamaStream_ContentFieldEmitsTextDelta(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/x-ndjson")
		_, _ = fmt.Fprintln(w, `{"model":"llama3:latest","message":{"role":"assistant","content":"Hello world"},"done":false}`)
		_, _ = fmt.Fprintln(w, `{"model":"llama3:latest","message":{"role":"assistant","content":""},"done":true,"done_reason":"stop","prompt_eval_count":5,"eval_count":3}`)
	}))
	defer ts.Close()

	p := newOllamaStreamProvider(t, ts.URL, "llama3:latest")
	sr, err := p.ChatStream(context.Background(), ChatRequest{
		Messages: []ChatMessage{textMsg("hi")},
	})
	if err != nil {
		t.Fatalf("ChatStream: %v", err)
	}
	events := collectStreamEvents(t, sr)

	var textEvents []StreamEvent
	for _, ev := range events {
		if ev.Type == StreamEventTextDelta {
			textEvents = append(textEvents, ev)
		}
	}
	if len(textEvents) == 0 {
		t.Fatal("expected at least one TextDelta event, got none")
	}
	if textEvents[0].Text != "Hello world" {
		t.Errorf("TextDelta text = %q, want 'Hello world'", textEvents[0].Text)
	}
}

func TestOllamaStream_BothFieldsSameLineEmitsBoth(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/x-ndjson")
		_, _ = fmt.Fprintln(w, `{"model":"deepseek-r1:7b","message":{"role":"assistant","content":"Answer.","thinking":"Reason."},"done":false}`)
		_, _ = fmt.Fprintln(w, `{"model":"deepseek-r1:7b","message":{"role":"assistant","content":""},"done":true,"done_reason":"stop","prompt_eval_count":5,"eval_count":3}`)
	}))
	defer ts.Close()

	p := newOllamaStreamProvider(t, ts.URL, "deepseek-r1:7b")
	sr, err := p.ChatStream(context.Background(), ChatRequest{
		Messages: []ChatMessage{textMsg("go")},
	})
	if err != nil {
		t.Fatalf("ChatStream: %v", err)
	}
	events := collectStreamEvents(t, sr)

	var hasReasoning, hasText bool
	for _, ev := range events {
		if ev.Type == StreamEventReasoningDelta {
			hasReasoning = true
		}
		if ev.Type == StreamEventTextDelta {
			hasText = true
		}
	}
	if !hasReasoning {
		t.Error("expected ReasoningDelta when thinking field is non-empty")
	}
	if !hasText {
		t.Error("expected TextDelta when content field is non-empty")
	}
}

func TestOllamaStream_DoneLineTerminates(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/x-ndjson")
		_, _ = fmt.Fprintln(w, `{"model":"llama3:latest","message":{"role":"assistant","content":"Hi"},"done":false}`)
		_, _ = fmt.Fprintln(w, `{"model":"llama3:latest","message":{"role":"assistant","content":""},"done":true,"done_reason":"stop","prompt_eval_count":8,"eval_count":2}`)
	}))
	defer ts.Close()

	p := newOllamaStreamProvider(t, ts.URL, "llama3:latest")
	sr, err := p.ChatStream(context.Background(), ChatRequest{
		Messages: []ChatMessage{textMsg("hi")},
	})
	if err != nil {
		t.Fatalf("ChatStream: %v", err)
	}
	events := collectStreamEvents(t, sr)

	var hasDone, hasUsage bool
	for _, ev := range events {
		if ev.Type == StreamEventDone {
			hasDone = true
		}
		if ev.Type == StreamEventUsage {
			hasUsage = true
			if ev.Usage == nil {
				t.Error("Usage event has nil Usage")
			}
		}
	}
	if !hasDone {
		t.Error("expected StreamEventDone after done:true line")
	}
	if !hasUsage {
		t.Error("expected StreamEventUsage before StreamEventDone")
	}
}

func TestOllamaChatStream_FixtureFile(t *testing.T) {
	// Reads the full fixture file and validates event types.
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		serveNDJSONFixture(t, w)
	}))
	defer ts.Close()

	p := newOllamaStreamProvider(t, ts.URL, "deepseek-r1:7b")
	sr, err := p.ChatStream(context.Background(), ChatRequest{
		Messages: []ChatMessage{textMsg("go")},
	})
	if err != nil {
		t.Fatalf("ChatStream: %v", err)
	}
	events := collectStreamEvents(t, sr)

	var reasoningCount, textCount, doneCount int
	for _, ev := range events {
		switch ev.Type {
		case StreamEventReasoningDelta:
			reasoningCount++
		case StreamEventTextDelta:
			textCount++
		case StreamEventDone:
			doneCount++
		}
	}
	if reasoningCount == 0 {
		t.Error("expected at least one ReasoningDelta from fixture")
	}
	if textCount == 0 {
		t.Error("expected at least one TextDelta from fixture")
	}
	if doneCount == 0 {
		t.Error("expected StreamEventDone from fixture")
	}
}

// --------------------------------------------------------------------------
// Phase 10 — tool_calls parsing in NDJSON stream
// --------------------------------------------------------------------------

func TestOllamaStream_ToolCallEmitsStartDeltaEnd(t *testing.T) {
	// An NDJSON line with message.tool_calls must emit ToolCallStart, ToolCallDelta,
	// and ToolCallEnd in that order, then terminate normally on done:true.
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/x-ndjson")
		// One chunk with a tool call.
		_, _ = fmt.Fprintln(w, `{"model":"llama3:latest","message":{"role":"assistant","content":"","tool_calls":[{"function":{"name":"get_weather","arguments":{"city":"NYC"}}}]},"done":false}`)
		// Done line.
		_, _ = fmt.Fprintln(w, `{"model":"llama3:latest","message":{"role":"assistant","content":""},"done":true,"done_reason":"stop","prompt_eval_count":10,"eval_count":5}`)
	}))
	defer ts.Close()

	p := newOllamaStreamProvider(t, ts.URL, "llama3:latest")
	sr, err := p.ChatStream(context.Background(), ChatRequest{
		Messages: []ChatMessage{textMsg("what's the weather?")},
	})
	if err != nil {
		t.Fatalf("ChatStream: %v", err)
	}
	events := collectStreamEvents(t, sr)

	// Collect tool call event types in order.
	var tcEvents []StreamEventType
	for _, ev := range events {
		switch ev.Type {
		case StreamEventToolCallStart, StreamEventToolCallDelta, StreamEventToolCallEnd:
			tcEvents = append(tcEvents, ev.Type)
		}
	}

	wantSeq := []StreamEventType{StreamEventToolCallStart, StreamEventToolCallDelta, StreamEventToolCallEnd}
	if len(tcEvents) != len(wantSeq) {
		t.Fatalf("tool call event sequence = %v, want %v", tcEvents, wantSeq)
	}
	for i, want := range wantSeq {
		if tcEvents[i] != want {
			t.Errorf("tcEvents[%d] = %v, want %v", i, tcEvents[i], want)
		}
	}

	// Validate ToolCallStart carries name and a generated ID.
	var startEv StreamEvent
	for _, ev := range events {
		if ev.Type == StreamEventToolCallStart {
			startEv = ev
			break
		}
	}
	if startEv.ToolName != "get_weather" {
		t.Errorf("ToolCallStart.ToolName = %q, want %q", startEv.ToolName, "get_weather")
	}
	if startEv.ToolCallID == "" {
		t.Error("ToolCallStart.ToolCallID must not be empty")
	}

	// Validate ToolCallDelta carries marshaled JSON arguments.
	var deltaEv StreamEvent
	for _, ev := range events {
		if ev.Type == StreamEventToolCallDelta {
			deltaEv = ev
			break
		}
	}
	if deltaEv.ToolInput == "" {
		t.Error("ToolCallDelta.ToolInput must not be empty")
	}
	// Must be valid JSON.
	var args map[string]any
	if err := json.Unmarshal([]byte(deltaEv.ToolInput), &args); err != nil {
		t.Errorf("ToolCallDelta.ToolInput is not valid JSON: %v — got %q", err, deltaEv.ToolInput)
	}
	if args["city"] != "NYC" {
		t.Errorf("args[city] = %v, want NYC", args["city"])
	}
}

func TestOllamaStream_ToolCallWithThinkingAndContent(t *testing.T) {
	// A line with tool_calls + thinking + content must emit all three event types:
	// ReasoningDelta, TextDelta, and the ToolCall sequence.
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/x-ndjson")
		_, _ = fmt.Fprintln(w, `{"model":"deepseek-r1:7b","message":{"role":"assistant","content":"Here you go.","thinking":"Let me check.","tool_calls":[{"function":{"name":"lookup","arguments":{"q":"test"}}}]},"done":false}`)
		_, _ = fmt.Fprintln(w, `{"model":"deepseek-r1:7b","message":{"role":"assistant","content":""},"done":true,"done_reason":"stop","prompt_eval_count":15,"eval_count":8}`)
	}))
	defer ts.Close()

	p := newOllamaStreamProvider(t, ts.URL, "deepseek-r1:7b")
	sr, err := p.ChatStream(context.Background(), ChatRequest{
		Messages: []ChatMessage{textMsg("lookup test")},
	})
	if err != nil {
		t.Fatalf("ChatStream: %v", err)
	}
	events := collectStreamEvents(t, sr)

	var hasReasoning, hasText, hasToolStart, hasToolDelta, hasToolEnd bool
	for _, ev := range events {
		switch ev.Type {
		case StreamEventReasoningDelta:
			hasReasoning = true
		case StreamEventTextDelta:
			hasText = true
		case StreamEventToolCallStart:
			hasToolStart = true
		case StreamEventToolCallDelta:
			hasToolDelta = true
		case StreamEventToolCallEnd:
			hasToolEnd = true
		}
	}

	if !hasReasoning {
		t.Error("expected ReasoningDelta event")
	}
	if !hasText {
		t.Error("expected TextDelta event")
	}
	if !hasToolStart {
		t.Error("expected ToolCallStart event")
	}
	if !hasToolDelta {
		t.Error("expected ToolCallDelta event")
	}
	if !hasToolEnd {
		t.Error("expected ToolCallEnd event")
	}
}
