package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"daimon/internal/config"
	"daimon/internal/content"
)

// --------------------------------------------------------------------------
// Helpers
// --------------------------------------------------------------------------

func newOpenAIStreamServer(t *testing.T, sseBody string) (*httptest.Server, config.ProviderConfig) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, sseBody)
	}))
	t.Cleanup(srv.Close)
	cfg := config.ProviderConfig{
		Type:    "openai",
		Model:   "gpt-4o",
		APIKey:  "test-key",
		BaseURL: srv.URL,
	}
	return srv, cfg
}

// --------------------------------------------------------------------------
// Interface assertion
// --------------------------------------------------------------------------

func TestOpenAIProvider_ImplementsStreamingProvider(t *testing.T) {
	// This is also enforced at compile time via var _ StreamingProvider = (*OpenAIProvider)(nil)
	// but this test makes the assertion explicit in the test suite.
	cfg := config.ProviderConfig{
		Type:    "openai",
		Model:   "gpt-4o",
		APIKey:  "test-key",
		BaseURL: "http://localhost:99999",
	}
	p, err := NewOpenAIProvider(cfg)
	if err != nil {
		t.Fatalf("NewOpenAIProvider: %v", err)
	}
	var _ StreamingProvider = p // compile-time checked; runtime assertion for clarity
}

// --------------------------------------------------------------------------
// Streaming tests
// --------------------------------------------------------------------------

func TestOpenAIProvider_ChatStreamTextOnly(t *testing.T) {
	// Construct an SSE stream with incremental text deltas.
	sseBody := "" +
		"data: {\"choices\":[{\"delta\":{\"content\":\"Hello\"},\"finish_reason\":null}]}\n\n" +
		"data: {\"choices\":[{\"delta\":{\"content\":\" world\"},\"finish_reason\":null}]}\n\n" +
		"data: {\"choices\":[{\"delta\":{},\"finish_reason\":\"stop\"}],\"usage\":{\"prompt_tokens\":5,\"completion_tokens\":2}}\n\n" +
		"data: [DONE]\n\n"

	_, cfg := newOpenAIStreamServer(t, sseBody)

	p, err := NewOpenAIProvider(cfg)
	if err != nil {
		t.Fatalf("NewOpenAIProvider: %v", err)
	}

	sr, err := p.ChatStream(context.Background(), ChatRequest{
		Messages: []ChatMessage{{Role: "user", Content: content.TextBlock("Say hello")}},
	})
	if err != nil {
		t.Fatalf("ChatStream: %v", err)
	}

	var textDeltas []string
	var gotDone bool
	var gotUsage bool

	for ev := range sr.Events {
		switch ev.Type {
		case StreamEventTextDelta:
			textDeltas = append(textDeltas, ev.Text)
		case StreamEventDone:
			gotDone = true
		case StreamEventUsage:
			gotUsage = true
			if ev.StopReason != "end_turn" {
				t.Errorf("expected stop reason 'end_turn', got %q", ev.StopReason)
			}
		case StreamEventError:
			t.Fatalf("unexpected stream error: %v", ev.Err)
		}
	}

	resp, err := sr.Response()
	if err != nil {
		t.Fatalf("Response(): %v", err)
	}

	if resp.Content != "Hello world" {
		t.Errorf("expected 'Hello world', got %q", resp.Content)
	}
	if len(textDeltas) != 2 {
		t.Errorf("expected 2 text deltas, got %d", len(textDeltas))
	}
	if !gotDone {
		t.Error("expected StreamEventDone")
	}
	if !gotUsage {
		t.Error("expected StreamEventUsage")
	}
}

func TestOpenAIProvider_ChatStreamToolCall(t *testing.T) {
	// Construct an SSE stream with a tool call.
	sseBody := "" +
		"data: {\"choices\":[{\"delta\":{\"tool_calls\":[{\"index\":0,\"id\":\"call_xyz\",\"type\":\"function\",\"function\":{\"name\":\"shell_exec\",\"arguments\":\"\"}}]},\"finish_reason\":null}]}\n\n" +
		"data: {\"choices\":[{\"delta\":{\"tool_calls\":[{\"index\":0,\"function\":{\"arguments\":\"{\\\"command\\\":\"}}]},\"finish_reason\":null}]}\n\n" +
		"data: {\"choices\":[{\"delta\":{\"tool_calls\":[{\"index\":0,\"function\":{\"arguments\":\"\\\"ls\\\"}\"}}]},\"finish_reason\":null}]}\n\n" +
		"data: {\"choices\":[{\"delta\":{},\"finish_reason\":\"tool_calls\"}]}\n\n" +
		"data: [DONE]\n\n"

	_, cfg := newOpenAIStreamServer(t, sseBody)

	p, err := NewOpenAIProvider(cfg)
	if err != nil {
		t.Fatalf("NewOpenAIProvider: %v", err)
	}

	sr, err := p.ChatStream(context.Background(), ChatRequest{
		Messages: []ChatMessage{{Role: "user", Content: content.TextBlock("Run ls")}},
	})
	if err != nil {
		t.Fatalf("ChatStream: %v", err)
	}

	var gotToolStart bool
	var gotToolEnd bool
	var toolInputAccum string

	for ev := range sr.Events {
		switch ev.Type {
		case StreamEventToolCallStart:
			gotToolStart = true
			if ev.ToolCallID != "call_xyz" {
				t.Errorf("unexpected tool call ID: %q", ev.ToolCallID)
			}
			if ev.ToolName != "shell_exec" {
				t.Errorf("unexpected tool name: %q", ev.ToolName)
			}
		case StreamEventToolCallDelta:
			toolInputAccum += ev.ToolInput
		case StreamEventToolCallEnd:
			gotToolEnd = true
		case StreamEventError:
			t.Fatalf("unexpected stream error: %v", ev.Err)
		}
	}

	resp, err := sr.Response()
	if err != nil {
		t.Fatalf("Response(): %v", err)
	}

	if !gotToolStart {
		t.Error("expected StreamEventToolCallStart")
	}
	if !gotToolEnd {
		t.Error("expected StreamEventToolCallEnd")
	}
	if len(resp.ToolCalls) != 1 {
		t.Fatalf("expected 1 tool call in response, got %d", len(resp.ToolCalls))
	}
	if resp.ToolCalls[0].Name != "shell_exec" {
		t.Errorf("unexpected tool name: %q", resp.ToolCalls[0].Name)
	}
	if resp.StopReason != "tool_use" {
		t.Errorf("unexpected stop reason: %q", resp.StopReason)
	}
}

func TestOpenAIProvider_ChatStreamHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"error":{"message":"unauthorized"}}`, http.StatusUnauthorized)
	}))
	defer srv.Close()

	cfg := config.ProviderConfig{
		Type:    "openai",
		Model:   "gpt-4o",
		APIKey:  "bad-key",
		BaseURL: srv.URL,
	}

	p, err := NewOpenAIProvider(cfg)
	if err != nil {
		t.Fatalf("NewOpenAIProvider: %v", err)
	}

	_, err = p.ChatStream(context.Background(), ChatRequest{
		Messages: []ChatMessage{{Role: "user", Content: content.TextBlock("Hi")}},
	})
	if err == nil {
		t.Fatal("expected error for 401 response")
	}
	if !errIs(err, ErrAuth) {
		t.Errorf("expected ErrAuth, got: %v", err)
	}
}

// TestOpenAIProvider_ChatStreamRequestsUsage asserts the streaming request opts
// into usage reporting via stream_options.include_usage. OpenAI-compatible
// endpoints (including MiniMax) omit the usage frame entirely unless this flag
// is set, which left chat telemetry showing "0 in / 0 out" tokens.
func TestOpenAIProvider_ChatStreamRequestsUsage(t *testing.T) {
	var capturedBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedBody, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
	}))
	defer srv.Close()

	cfg := config.ProviderConfig{Type: "openai", Model: "gpt-4o", APIKey: "test-key", BaseURL: srv.URL}
	p, err := NewOpenAIProvider(cfg)
	if err != nil {
		t.Fatalf("NewOpenAIProvider: %v", err)
	}

	sr, err := p.ChatStream(context.Background(), ChatRequest{
		Messages: []ChatMessage{{Role: "user", Content: content.TextBlock("Hi")}},
	})
	if err != nil {
		t.Fatalf("ChatStream: %v", err)
	}
	for range sr.Events { // drain to ensure the request completed
	}

	var body struct {
		Stream        bool `json:"stream"`
		StreamOptions *struct {
			IncludeUsage bool `json:"include_usage"`
		} `json:"stream_options"`
	}
	if err := json.Unmarshal(capturedBody, &body); err != nil {
		t.Fatalf("unmarshal request body: %v (body=%s)", err, capturedBody)
	}
	if !body.Stream {
		t.Error("expected stream:true in request body")
	}
	if body.StreamOptions == nil || !body.StreamOptions.IncludeUsage {
		t.Errorf("expected stream_options.include_usage:true, got %+v (body=%s)", body.StreamOptions, capturedBody)
	}
}
