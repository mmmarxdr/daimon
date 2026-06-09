package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"daimon/internal/config"
	"daimon/internal/content"
)

// --------------------------------------------------------------------------
// Helpers
// --------------------------------------------------------------------------

// newMiniMaxTestProvider creates a MiniMaxProvider pointing at the given base URL.
// api_key is required; we always supply a dummy key and a test base URL.
func newMiniMaxTestProvider(t *testing.T, baseURL string) *MiniMaxProvider {
	t.Helper()
	cfg := config.ProviderConfig{
		Type:    "minimax",
		Model:   "MiniMax-Text-01",
		APIKey:  "sk-cp-test",
		BaseURL: baseURL,
	}
	p, err := NewMiniMaxProvider(cfg)
	if err != nil {
		t.Fatalf("NewMiniMaxProvider: %v", err)
	}
	return p
}

// sseLines builds a raw SSE response body from a slice of delta content strings.
// Each string becomes one data frame. The last frame gets finish_reason:"stop"
// when includeFinishReason is true. Always appends a [DONE] sentinel.
func sseLines(deltas []string, includeFinishReason bool) []byte {
	var sb strings.Builder
	for i, d := range deltas {
		var finishReason *string
		if includeFinishReason && i == len(deltas)-1 {
			s := "stop"
			finishReason = &s
		}
		chunk := fmt.Sprintf(
			`{"choices":[{"delta":{"content":%s},"finish_reason":%s}]}`,
			jsonQuote(d),
			jsonNullOrQuote(finishReason),
		)
		sb.WriteString("data: ")
		sb.WriteString(chunk)
		sb.WriteString("\n\n")
	}
	sb.WriteString("data: [DONE]\n\n")
	return []byte(sb.String())
}

func jsonQuote(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

func jsonNullOrQuote(s *string) string {
	if s == nil {
		return "null"
	}
	b, _ := json.Marshal(*s)
	return string(b)
}

// --------------------------------------------------------------------------
// Task 2.3 — TestNewMiniMaxProvider (MM-1a/1b, ADR-1)
// --------------------------------------------------------------------------

func TestNewMiniMaxProvider(t *testing.T) {
	t.Run("valid config returns non-nil provider", func(t *testing.T) {
		cfg := config.ProviderConfig{
			Type:    "minimax",
			Model:   "MiniMax-Text-01",
			APIKey:  "sk-cp-test",
			BaseURL: "https://api.minimax.io/v1",
		}
		p, err := NewMiniMaxProvider(cfg)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if p == nil {
			t.Fatal("expected non-nil provider")
		}
	})

	t.Run("Name returns minimax", func(t *testing.T) {
		cfg := config.ProviderConfig{
			Type:    "minimax",
			Model:   "MiniMax-Text-01",
			APIKey:  "sk-cp-test",
			BaseURL: "https://api.minimax.io/v1",
		}
		p, err := NewMiniMaxProvider(cfg)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got := p.Name(); got != "minimax" {
			t.Errorf("Name() = %q, want %q", got, "minimax")
		}
	})

	t.Run("inner model is wired from config", func(t *testing.T) {
		cfg := config.ProviderConfig{
			Type:    "minimax",
			Model:   "MiniMax-Text-01",
			APIKey:  "sk-cp-test",
			BaseURL: "https://api.minimax.io/v1",
		}
		p, err := NewMiniMaxProvider(cfg)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got := p.Model(); got != "MiniMax-Text-01" {
			t.Errorf("Model() = %q, want %q", got, "MiniMax-Text-01")
		}
	})

	t.Run("empty api_key propagates error", func(t *testing.T) {
		// api_key is enforced by NewMiniMaxProvider itself (MM-1b), independent of
		// base_url — the minimax default base URL bypasses the openai-only guard.
		cfg := config.ProviderConfig{
			Type:   "minimax",
			Model:  "MiniMax-M2",
			APIKey: "",
		}
		_, err := NewMiniMaxProvider(cfg)
		if err == nil {
			t.Fatal("expected error for empty api_key, got nil")
		}
		if !strings.Contains(err.Error(), "api_key") {
			t.Errorf("expected error to mention api_key, got: %v", err)
		}
	})

	t.Run("empty base_url defaults to MiniMax endpoint", func(t *testing.T) {
		// MM-1a: a minimax config without base_url must target MiniMax, never
		// silently fall through to OpenAI's default endpoint.
		cfg := config.ProviderConfig{
			Type:   "minimax",
			Model:  "MiniMax-M2",
			APIKey: "sk-cp-test",
		}
		p, err := NewMiniMaxProvider(cfg)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got := p.OpenAIProvider.baseURL; got != miniMaxDefaultBaseURL {
			t.Errorf("baseURL = %q, want %q", got, miniMaxDefaultBaseURL)
		}
	})
}

// --------------------------------------------------------------------------
// Task 2.3 — TestMiniMaxProvider_InterfaceSatisfaction (ADR-1.1)
// --------------------------------------------------------------------------

func TestMiniMaxProvider_InterfaceSatisfaction(t *testing.T) {
	cfg := config.ProviderConfig{
		Type:    "minimax",
		Model:   "MiniMax-Text-01",
		APIKey:  "sk-cp-test",
		BaseURL: "https://api.minimax.io/v1",
	}
	p, err := NewMiniMaxProvider(cfg)
	if err != nil {
		t.Fatalf("NewMiniMaxProvider: %v", err)
	}

	t.Run("satisfies Provider", func(t *testing.T) {
		var _ Provider = p
		if p.Name() != "minimax" {
			t.Errorf("Provider.Name() = %q, want minimax", p.Name())
		}
	})

	t.Run("satisfies StreamingProvider", func(t *testing.T) {
		if _, ok := any(p).(StreamingProvider); !ok {
			t.Fatal("MiniMaxProvider does not satisfy StreamingProvider interface")
		}
	})

	t.Run("satisfies ModelLister", func(t *testing.T) {
		if _, ok := any(p).(ModelLister); !ok {
			t.Fatal("MiniMaxProvider does not satisfy ModelLister interface")
		}
	})

	t.Run("satisfies EmbeddingProvider", func(t *testing.T) {
		if _, ok := any(p).(EmbeddingProvider); !ok {
			t.Fatal("MiniMaxProvider does not satisfy EmbeddingProvider interface")
		}
	})
}

// --------------------------------------------------------------------------
// Task 2.5 — TestMiniMaxProvider_Chat_StripsThink (MM-2a/2c/MM-6a)
// --------------------------------------------------------------------------

func TestMiniMaxProvider_Chat_StripsThink(t *testing.T) {
	t.Run("think block stripped from response", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			c := "<think>cot</think>The answer"
			resp := openaiResponse{
				Choices: []openaiChoice{
					{
						Message: struct {
							Role      string           `json:"role"`
							Content   *string          `json:"content"`
							ToolCalls []openaiToolCall `json:"tool_calls,omitempty"`
						}{Role: "assistant", Content: &c},
						FinishReason: "stop",
					},
				},
				Usage: struct {
					PromptTokens     int `json:"prompt_tokens"`
					CompletionTokens int `json:"completion_tokens"`
				}{PromptTokens: 5, CompletionTokens: 10},
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(resp)
		}))
		defer ts.Close()

		p := newMiniMaxTestProvider(t, ts.URL)
		r, err := p.Chat(context.Background(), ChatRequest{
			Messages: []ChatMessage{{Role: "user", Content: content.TextBlock("hello")}},
		})
		if err != nil {
			t.Fatalf("Chat() error: %v", err)
		}
		if r.Content != "The answer" {
			t.Errorf("Content = %q, want %q", r.Content, "The answer")
		}
	})

	t.Run("tool calls pass through unchanged (MM-6a)", func(t *testing.T) {
		thinkContent := "<think>cot</think>"
		toolCallID := "tc-1"
		toolName := "search"
		toolArgs := `{"query":"foo"}`
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			resp := openaiResponse{
				Choices: []openaiChoice{
					{
						Message: struct {
							Role      string           `json:"role"`
							Content   *string          `json:"content"`
							ToolCalls []openaiToolCall `json:"tool_calls,omitempty"`
						}{
							Role:    "assistant",
							Content: &thinkContent,
							ToolCalls: []openaiToolCall{
								{
									ID:   toolCallID,
									Type: "function",
									Function: struct {
										Name      string `json:"name"`
										Arguments string `json:"arguments"`
									}{Name: toolName, Arguments: toolArgs},
								},
							},
						},
						FinishReason: "tool_calls",
					},
				},
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(resp)
		}))
		defer ts.Close()

		p := newMiniMaxTestProvider(t, ts.URL)
		r, err := p.Chat(context.Background(), ChatRequest{
			Messages: []ChatMessage{{Role: "user", Content: content.TextBlock("hello")}},
		})
		if err != nil {
			t.Fatalf("Chat() error: %v", err)
		}
		if r.Content != "" {
			t.Errorf("Content = %q, want empty (only think block)", r.Content)
		}
		if len(r.ToolCalls) != 1 {
			t.Fatalf("expected 1 tool call, got %d", len(r.ToolCalls))
		}
		if r.ToolCalls[0].Name != toolName {
			t.Errorf("ToolCall.Name = %q, want %q", r.ToolCalls[0].Name, toolName)
		}
	})

	t.Run("empty api_key construction fails (MM-1b)", func(t *testing.T) {
		cfg := config.ProviderConfig{
			Type:   "minimax",
			Model:  "MiniMax-Text-01",
			APIKey: "",
		}
		_, err := NewMiniMaxProvider(cfg)
		if err == nil {
			t.Fatal("expected construction error for empty api_key")
		}
	})
}

// --------------------------------------------------------------------------
// Tasks 3.1–3.4 — TestMiniMaxProvider_ChatStream_* (MM-3a/3b/3c/4a/6b)
// --------------------------------------------------------------------------

func TestMiniMaxProvider_ChatStream_StripsThink(t *testing.T) {
	// SSE frames deliberately split <think> and </think> across chunks (MM-3b).
	deltas := []string{"<thi", "nk>cot</thi", "nk> answer"}

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		_, _ = w.Write(sseLines(deltas, true))
	}))
	defer ts.Close()

	p := newMiniMaxTestProvider(t, ts.URL)
	sr, err := p.ChatStream(context.Background(), ChatRequest{
		Messages: []ChatMessage{{Role: "user", Content: content.TextBlock("hello")}},
	})
	if err != nil {
		t.Fatalf("ChatStream() error: %v", err)
	}

	var collectedText, collectedReason string
	for ev := range sr.Events {
		switch ev.Type {
		case StreamEventTextDelta:
			collectedText += ev.Text
		case StreamEventReasoningDelta:
			collectedReason += ev.Text
		}
	}

	resp, err := sr.Response()
	if err != nil {
		t.Fatalf("Response() error: %v", err)
	}

	if collectedText != " answer" {
		t.Errorf("collected TextDelta = %q, want %q", collectedText, " answer")
	}
	if collectedReason != "cot" {
		t.Errorf("collected ReasoningDelta = %q, want %q", collectedReason, "cot")
	}
	if resp.Content != " answer" {
		t.Errorf("Response().Content = %q, want %q", resp.Content, " answer")
	}
}

func TestMiniMaxProvider_ChatStream_UnclosedThink(t *testing.T) {
	// MM-4a: stream ends with <think>partial cot and no closing tag.
	deltas := []string{"<think>partial cot"}

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write(sseLines(deltas, true))
	}))
	defer ts.Close()

	p := newMiniMaxTestProvider(t, ts.URL)
	sr, err := p.ChatStream(context.Background(), ChatRequest{
		Messages: []ChatMessage{{Role: "user", Content: content.TextBlock("hello")}},
	})
	if err != nil {
		t.Fatalf("ChatStream() error: %v", err)
	}

	var collectedReason string
	for ev := range sr.Events {
		if ev.Type == StreamEventReasoningDelta {
			collectedReason += ev.Text
		}
	}

	if collectedReason != "partial cot" {
		t.Errorf("collected ReasoningDelta = %q, want %q", collectedReason, "partial cot")
	}

	// Response() must return promptly — no goroutine hang.
	resp, err := sr.Response()
	if err != nil {
		t.Fatalf("Response() error: %v", err)
	}
	if resp.Content != "" {
		t.Errorf("Response().Content = %q, want empty", resp.Content)
	}
}

func TestMiniMaxProvider_ChatStream_ContentOnlyThink(t *testing.T) {
	// MM-3c: stream is entirely a think block — zero TextDelta events.
	deltas := []string{"<think>reasoning only</think>"}

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write(sseLines(deltas, true))
	}))
	defer ts.Close()

	p := newMiniMaxTestProvider(t, ts.URL)
	sr, err := p.ChatStream(context.Background(), ChatRequest{
		Messages: []ChatMessage{{Role: "user", Content: content.TextBlock("hello")}},
	})
	if err != nil {
		t.Fatalf("ChatStream() error: %v", err)
	}

	var gotTextDelta bool
	for ev := range sr.Events {
		if ev.Type == StreamEventTextDelta {
			gotTextDelta = true
		}
	}

	if gotTextDelta {
		t.Error("expected no TextDelta events for content-only think stream")
	}

	resp, err := sr.Response()
	if err != nil {
		t.Fatalf("Response() error: %v", err)
	}
	if resp.Content != "" {
		t.Errorf("Response().Content = %q, want empty", resp.Content)
	}
}

func TestMiniMaxProvider_ChatStream_ToolCallAfterThink(t *testing.T) {
	// MM-6b: tool call deltas after </think> must pass through unmodified.
	toolCallID := "tc-1"
	toolName := "search"
	toolArgs := `{"q":"foo"}`

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")

		frames := []string{
			// Frame 1: think block (no text answer).
			`data: {"choices":[{"delta":{"content":"<think>cot</think>"},"finish_reason":null}]}` + "\n\n",
			// Frame 2: tool call start (index 0, id + name).
			fmt.Sprintf(`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":%s,"type":"function","function":{"name":%s,"arguments":""}}]},"finish_reason":null}]}`+"\n\n",
				jsonQuote(toolCallID), jsonQuote(toolName)),
			// Frame 3: tool call arguments delta.
			fmt.Sprintf(`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":%s}}]},"finish_reason":null}]}`+"\n\n",
				jsonQuote(toolArgs)),
			// Frame 4: finish_reason=tool_calls.
			`data: {"choices":[{"delta":{},"finish_reason":"tool_calls"}]}` + "\n\n",
			"data: [DONE]\n\n",
		}
		for _, f := range frames {
			_, _ = fmt.Fprint(w, f)
		}
	}))
	defer ts.Close()

	p := newMiniMaxTestProvider(t, ts.URL)
	sr, err := p.ChatStream(context.Background(), ChatRequest{
		Messages: []ChatMessage{{Role: "user", Content: content.TextBlock("hello")}},
	})
	if err != nil {
		t.Fatalf("ChatStream() error: %v", err)
	}

	var gotToolStart, gotToolDelta, gotToolEnd bool
	var receivedToolName string
	for ev := range sr.Events {
		switch ev.Type {
		case StreamEventToolCallStart:
			gotToolStart = true
			receivedToolName = ev.ToolName
		case StreamEventToolCallDelta:
			gotToolDelta = true
		case StreamEventToolCallEnd:
			gotToolEnd = true
		}
	}

	if !gotToolStart {
		t.Error("expected ToolCallStart event, got none")
	}
	if receivedToolName != toolName {
		t.Errorf("ToolCallStart.ToolName = %q, want %q", receivedToolName, toolName)
	}
	if !gotToolDelta {
		t.Error("expected ToolCallDelta event, got none")
	}
	if !gotToolEnd {
		t.Error("expected ToolCallEnd event, got none")
	}
}
