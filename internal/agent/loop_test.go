package agent

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"microagent/internal/channel"
	"microagent/internal/config"
	"microagent/internal/provider"
	"microagent/internal/store"
	"microagent/internal/tool"
)

// ---------------------------------------------------------------------------
// Extended mock types
// ---------------------------------------------------------------------------

type mockProvider struct {
	responses []provider.ChatResponse
	errs      []error // parallel to responses; nil entry = no error for that call
	calls     int
}

func (m *mockProvider) Name() string        { return "mock" }
func (m *mockProvider) SupportsTools() bool { return true }
func (m *mockProvider) Chat(ctx context.Context, req provider.ChatRequest) (*provider.ChatResponse, error) {
	idx := m.calls
	m.calls++
	if idx < len(m.errs) && m.errs[idx] != nil {
		return nil, m.errs[idx]
	}
	if idx < len(m.responses) {
		resp := m.responses[idx]
		return &resp, nil
	}
	return &provider.ChatResponse{Content: "default"}, nil
}

type mockChannel struct {
	sent     []channel.OutgoingMessage
	stopErr  error
	messages []channel.IncomingMessage // pre-filled inbox for Run tests
}

func (m *mockChannel) Name() string { return "mock" }
func (m *mockChannel) Start(ctx context.Context, inbox chan<- channel.IncomingMessage) error {
	for _, msg := range m.messages {
		inbox <- msg
	}
	return nil
}
func (m *mockChannel) Send(ctx context.Context, msg channel.OutgoingMessage) error {
	m.sent = append(m.sent, msg)
	return nil
}
func (m *mockChannel) Stop() error { return m.stopErr }

type mockTool struct {
	name        string
	result      tool.ToolResult
	err         error
	shouldPanic bool
	calls       int
}

func (m *mockTool) Name() string            { return m.name }
func (m *mockTool) Description() string     { return "mock tool" }
func (m *mockTool) Schema() json.RawMessage { return json.RawMessage(`{}`) }
func (m *mockTool) Execute(ctx context.Context, params json.RawMessage) (tool.ToolResult, error) {
	m.calls++
	if m.shouldPanic {
		panic("test panic")
	}
	return m.result, m.err
}

type mockStore struct {
	conv         *store.Conversation // nil means "not found" → creates new
	loadErr      error
	saveErr      error
	memories     []store.MemoryEntry
	appendedMems []store.MemoryEntry
}

func (m *mockStore) SaveConversation(ctx context.Context, conv store.Conversation) error {
	if m.saveErr != nil {
		return m.saveErr
	}
	m.conv = &conv
	return nil
}
func (m *mockStore) LoadConversation(ctx context.Context, id string) (*store.Conversation, error) {
	if m.loadErr != nil {
		return nil, m.loadErr
	}
	if m.conv == nil {
		return nil, errors.New("not found")
	}
	return m.conv, nil
}
func (m *mockStore) ListConversations(ctx context.Context, channelID string, limit int) ([]store.Conversation, error) {
	return nil, nil
}
func (m *mockStore) AppendMemory(ctx context.Context, entry store.MemoryEntry) error {
	m.appendedMems = append(m.appendedMems, entry)
	return nil
}
func (m *mockStore) SearchMemory(ctx context.Context, query string, limit int) ([]store.MemoryEntry, error) {
	return m.memories, nil
}
func (m *mockStore) Close() error { return nil }

// ---------------------------------------------------------------------------
// Helper to build a default agent config.
// ---------------------------------------------------------------------------

func defaultCfg() config.AgentConfig {
	return config.AgentConfig{MaxIterations: 5, MaxTokensPerTurn: 100}
}

func defaultLimits() config.LimitsConfig {
	return config.LimitsConfig{TotalTimeout: 10 * time.Second, ToolTimeout: 2 * time.Second}
}

// ---------------------------------------------------------------------------
// Original test — preserved
// ---------------------------------------------------------------------------

func TestAgentLoop(t *testing.T) {
	prov := &mockProvider{
		responses: []provider.ChatResponse{
			{
				ToolCalls: []provider.ToolCall{
					{ID: "t1", Name: "mock_tool", Input: json.RawMessage(`{}`)},
				},
			},
			{
				Content: "final response",
			},
		},
	}
	ch := &mockChannel{}
	st := &mockStore{}

	ag := New(defaultCfg(), defaultLimits(), ch, prov, st, map[string]tool.Tool{
		"mock_tool": &mockTool{name: "mock_tool", result: tool.ToolResult{Content: "mock result"}},
	})

	ag.processMessage(context.Background(), channel.IncomingMessage{ChannelID: "test", Text: "hello"})

	if len(ch.sent) != 1 {
		t.Fatalf("expected 1 final message, got %d", len(ch.sent))
	}
	if ch.sent[0].Text != "final response" {
		t.Errorf("unexpected output: %s", ch.sent[0].Text)
	}
	if len(st.conv.Messages) != 4 {
		t.Errorf("expected 4 messages in history, got %d", len(st.conv.Messages))
	}
}

// ---------------------------------------------------------------------------
// TestAgent_Run_ProcessesMessages
// ---------------------------------------------------------------------------

func TestAgent_Run_ProcessesMessages(t *testing.T) {
	prov := &mockProvider{
		responses: []provider.ChatResponse{
			{Content: "hi there"},
		},
	}
	// Pre-fill inbox with one message via mockChannel.messages
	ch := &mockChannel{
		messages: []channel.IncomingMessage{
			{ChannelID: "test", Text: "hello"},
		},
	}
	st := &mockStore{}

	ag := New(defaultCfg(), defaultLimits(), ch, prov, st, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	runDone := make(chan error, 1)
	go func() {
		runDone <- ag.Run(ctx)
	}()

	// Wait until the provider is called, then cancel.
	deadline := time.After(3 * time.Second)
	for {
		if prov.calls >= 1 {
			cancel()
			break
		}
		select {
		case <-deadline:
			t.Fatal("provider.Chat was never called within 3s")
		case <-time.After(10 * time.Millisecond):
		}
	}

	select {
	case err := <-runDone:
		if err != context.Canceled && err != context.DeadlineExceeded {
			t.Errorf("Run returned unexpected error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not return after context cancel")
	}

	if len(ch.sent) == 0 {
		t.Error("expected channel.Send to be called with provider response")
	} else if ch.sent[0].Text != "hi there" {
		t.Errorf("expected 'hi there', got %q", ch.sent[0].Text)
	}
}

// ---------------------------------------------------------------------------
// TestAgent_Shutdown
// ---------------------------------------------------------------------------

func TestAgent_Shutdown_NilError(t *testing.T) {
	ch := &mockChannel{stopErr: nil}
	ag := New(defaultCfg(), defaultLimits(), ch, &mockProvider{}, &mockStore{}, nil)
	if err := ag.Shutdown(); err != nil {
		t.Errorf("expected nil, got %v", err)
	}
}

func TestAgent_Shutdown_PropagatesError(t *testing.T) {
	stopErr := errors.New("stop failed")
	ch := &mockChannel{stopErr: stopErr}
	ag := New(defaultCfg(), defaultLimits(), ch, &mockProvider{}, &mockStore{}, nil)
	if err := ag.Shutdown(); !errors.Is(err, stopErr) {
		t.Errorf("expected stopErr, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// TestBuildContext_*
// ---------------------------------------------------------------------------

func TestBuildContext_NoMemories(t *testing.T) {
	ag := New(defaultCfg(), defaultLimits(), &mockChannel{}, &mockProvider{}, &mockStore{}, nil)
	conv := &store.Conversation{}
	req := ag.buildContext(conv, []store.MemoryEntry{})
	if strings.Contains(req.SystemPrompt, "## Relevant Context:") {
		t.Error("system prompt should NOT contain '## Relevant Context:' when no memories")
	}
}

func TestBuildContext_WithMemories(t *testing.T) {
	ag := New(defaultCfg(), defaultLimits(), &mockChannel{}, &mockProvider{}, &mockStore{}, nil)
	conv := &store.Conversation{}
	memories := []store.MemoryEntry{
		{Content: "User likes Go"},
		{Content: "Prefers short answers"},
	}
	req := ag.buildContext(conv, memories)
	if !strings.Contains(req.SystemPrompt, "## Relevant Context:") {
		t.Error("system prompt should contain '## Relevant Context:'")
	}
	if !strings.Contains(req.SystemPrompt, "User likes Go") {
		t.Error("system prompt should contain first memory content")
	}
	if !strings.Contains(req.SystemPrompt, "Prefers short answers") {
		t.Error("system prompt should contain second memory content")
	}
}

func TestBuildContext_ToolsIncluded(t *testing.T) {
	toolA := &mockTool{name: "tool_a"}
	toolB := &mockTool{name: "tool_b"}

	ag := New(defaultCfg(), defaultLimits(), &mockChannel{}, &mockProvider{}, &mockStore{},
		map[string]tool.Tool{"tool_a": toolA, "tool_b": toolB})

	conv := &store.Conversation{}
	req := ag.buildContext(conv, nil)

	if len(req.Tools) != 2 {
		t.Errorf("expected 2 tools in ChatRequest, got %d", len(req.Tools))
	}
	names := map[string]bool{}
	for _, td := range req.Tools {
		names[td.Name] = true
	}
	if !names["tool_a"] || !names["tool_b"] {
		t.Errorf("tools missing from ChatRequest: %v", names)
	}
}

func TestBuildContext_NoTools(t *testing.T) {
	ag := New(defaultCfg(), defaultLimits(), &mockChannel{}, &mockProvider{}, &mockStore{}, nil)
	conv := &store.Conversation{}
	req := ag.buildContext(conv, nil)
	if req.Tools == nil {
		t.Error("Tools slice should not be nil even with no tools registered")
	}
	if len(req.Tools) != 0 {
		t.Errorf("expected 0 tools, got %d", len(req.Tools))
	}
}

// ---------------------------------------------------------------------------
// TestProcessMessage_MaxIterations
// ---------------------------------------------------------------------------

func TestProcessMessage_MaxIterations(t *testing.T) {
	// Provider always returns a tool_use call — loop should hit max iterations.
	toolCall := provider.ChatResponse{
		ToolCalls: []provider.ToolCall{
			{ID: "t1", Name: "mock_tool", Input: json.RawMessage(`{}`)},
		},
	}
	prov := &mockProvider{
		responses: []provider.ChatResponse{
			toolCall, toolCall, toolCall, toolCall, toolCall,
			toolCall, toolCall, toolCall, toolCall, toolCall,
		},
	}
	ch := &mockChannel{}
	st := &mockStore{}
	cfg := config.AgentConfig{MaxIterations: 2, MaxTokensPerTurn: 100}
	limits := config.LimitsConfig{TotalTimeout: 5 * time.Second, ToolTimeout: 1 * time.Second}

	mt := &mockTool{name: "mock_tool", result: tool.ToolResult{Content: "result"}}
	ag := New(cfg, limits, ch, prov, st, map[string]tool.Tool{"mock_tool": mt})

	ag.processMessage(context.Background(), channel.IncomingMessage{ChannelID: "test", Text: "go"})

	found := false
	for _, msg := range ch.sent {
		if strings.Contains(msg.Text, "iteration limit") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected 'iteration limit' message in sent messages; got: %v", ch.sent)
	}
}

// ---------------------------------------------------------------------------
// TestProcessMessage_UnknownTool
// ---------------------------------------------------------------------------

func TestProcessMessage_UnknownTool(t *testing.T) {
	prov := &mockProvider{
		responses: []provider.ChatResponse{
			{
				ToolCalls: []provider.ToolCall{
					{ID: "t1", Name: "unknown_tool", Input: json.RawMessage(`{}`)},
				},
			},
			{Content: "done"},
		},
	}
	ch := &mockChannel{}
	st := &mockStore{}

	ag := New(defaultCfg(), defaultLimits(), ch, prov, st, map[string]tool.Tool{})
	ag.processMessage(context.Background(), channel.IncomingMessage{ChannelID: "test", Text: "hello"})

	// The conversation should have a tool-role message with "not found"
	if st.conv == nil {
		t.Fatal("no conversation saved")
	}
	foundNotFound := false
	for _, msg := range st.conv.Messages {
		if msg.Role == "tool" && strings.Contains(msg.Content, "not found") {
			foundNotFound = true
			break
		}
	}
	if !foundNotFound {
		t.Errorf("expected tool result message containing 'not found'; messages: %v", st.conv.Messages)
	}
}

// ---------------------------------------------------------------------------
// TestProcessMessage_ToolGoError
// ---------------------------------------------------------------------------

func TestProcessMessage_ToolGoError(t *testing.T) {
	goErr := errors.New("disk full")
	mt := &mockTool{name: "err_tool", err: goErr}

	prov := &mockProvider{
		responses: []provider.ChatResponse{
			{
				ToolCalls: []provider.ToolCall{
					{ID: "t1", Name: "err_tool", Input: json.RawMessage(`{}`)},
				},
			},
			{Content: "done"},
		},
	}
	ch := &mockChannel{}
	st := &mockStore{}

	ag := New(defaultCfg(), defaultLimits(), ch, prov, st, map[string]tool.Tool{"err_tool": mt})
	ag.processMessage(context.Background(), channel.IncomingMessage{ChannelID: "test", Text: "hello"})

	if st.conv == nil {
		t.Fatal("no conversation saved")
	}
	foundErr := false
	for _, msg := range st.conv.Messages {
		if msg.Role == "tool" && strings.Contains(msg.Content, "disk full") {
			foundErr = true
			break
		}
	}
	if !foundErr {
		t.Errorf("expected tool result with 'disk full'; messages: %v", st.conv.Messages)
	}
}

// ---------------------------------------------------------------------------
// TestProcessMessage_ToolPanic
// ---------------------------------------------------------------------------

func TestProcessMessage_ToolPanic(t *testing.T) {
	// This test verifies that a panicking tool does NOT crash the process.
	mt := &mockTool{name: "panic_tool", shouldPanic: true}

	prov := &mockProvider{
		responses: []provider.ChatResponse{
			{
				ToolCalls: []provider.ToolCall{
					{ID: "t1", Name: "panic_tool", Input: json.RawMessage(`{}`)},
				},
			},
			{Content: "recovered"},
		},
	}
	ch := &mockChannel{}
	st := &mockStore{}

	ag := New(defaultCfg(), defaultLimits(), ch, prov, st, map[string]tool.Tool{"panic_tool": mt})

	// Should NOT panic
	ag.processMessage(context.Background(), channel.IncomingMessage{ChannelID: "test", Text: "go"})

	if st.conv == nil {
		t.Fatal("no conversation saved")
	}
	foundCrash := false
	for _, msg := range st.conv.Messages {
		if msg.Role == "tool" && (strings.Contains(msg.Content, "crashed") || strings.Contains(msg.Content, "test panic")) {
			foundCrash = true
			break
		}
	}
	if !foundCrash {
		t.Errorf("expected tool result containing panic info; messages: %v", st.conv.Messages)
	}
}

// ---------------------------------------------------------------------------
// TestProcessMessage_MultipleToolCalls
// ---------------------------------------------------------------------------

func TestProcessMessage_MultipleToolCalls(t *testing.T) {
	toolA := &mockTool{name: "tool_a", result: tool.ToolResult{Content: "a result"}}
	toolB := &mockTool{name: "tool_b", result: tool.ToolResult{Content: "b result"}}

	prov := &mockProvider{
		responses: []provider.ChatResponse{
			{
				ToolCalls: []provider.ToolCall{
					{ID: "t1", Name: "tool_a", Input: json.RawMessage(`{}`)},
					{ID: "t2", Name: "tool_b", Input: json.RawMessage(`{}`)},
				},
			},
			{Content: "done"},
		},
	}
	ch := &mockChannel{}
	st := &mockStore{}

	ag := New(defaultCfg(), defaultLimits(), ch, prov, st, map[string]tool.Tool{
		"tool_a": toolA,
		"tool_b": toolB,
	})
	ag.processMessage(context.Background(), channel.IncomingMessage{ChannelID: "test", Text: "hello"})

	if toolA.calls != 1 {
		t.Errorf("tool_a expected 1 call, got %d", toolA.calls)
	}
	if toolB.calls != 1 {
		t.Errorf("tool_b expected 1 call, got %d", toolB.calls)
	}
}

// ---------------------------------------------------------------------------
// TestProcessMessage_ProviderError
// ---------------------------------------------------------------------------

func TestProcessMessage_ProviderError(t *testing.T) {
	provErr := errors.New("api down")
	prov := &mockProvider{
		errs: []error{provErr},
	}
	ch := &mockChannel{}
	st := &mockStore{}

	ag := New(defaultCfg(), defaultLimits(), ch, prov, st, nil)

	// Should not panic
	ag.processMessage(context.Background(), channel.IncomingMessage{ChannelID: "test", Text: "hello"})

	found := false
	for _, msg := range ch.sent {
		if strings.Contains(msg.Text, "api down") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected error message containing 'api down' sent to channel; got: %v", ch.sent)
	}
}

// ---------------------------------------------------------------------------
// TestProcessMessage_ExistingHistory
// ---------------------------------------------------------------------------

func TestProcessMessage_ExistingHistory(t *testing.T) {
	existingConv := &store.Conversation{
		ID:        "conv_test",
		ChannelID: "test",
		Messages: []provider.ChatMessage{
			{Role: "user", Content: "first message"},
			{Role: "assistant", Content: "first reply"},
		},
	}

	prov := &mockProvider{
		responses: []provider.ChatResponse{
			{Content: "response"},
		},
	}
	ch := &mockChannel{}
	st := &mockStore{conv: existingConv}

	var capturedReq provider.ChatRequest
	origProv := prov
	_ = origProv

	// Wrap provider to capture the request
	capturingProv := &capturingProvider{inner: prov}

	ag := New(defaultCfg(), defaultLimits(), ch, capturingProv, st, nil)
	ag.processMessage(context.Background(), channel.IncomingMessage{ChannelID: "test", Text: "new message"})

	capturedReq = capturingProv.lastReq

	// The ChatRequest should contain the 2 existing messages plus the new user message
	if len(capturedReq.Messages) < 3 {
		t.Errorf("expected at least 3 messages in ChatRequest (2 existing + 1 new), got %d", len(capturedReq.Messages))
	}
	if capturedReq.Messages[0].Content != "first message" {
		t.Errorf("expected first message to be 'first message', got %q", capturedReq.Messages[0].Content)
	}
	if capturedReq.Messages[len(capturedReq.Messages)-1].Content != "new message" {
		t.Errorf("expected last message to be 'new message', got %q", capturedReq.Messages[len(capturedReq.Messages)-1].Content)
	}
}

// capturingProvider wraps a mockProvider and captures the last ChatRequest.
type capturingProvider struct {
	inner   *mockProvider
	lastReq provider.ChatRequest
}

func (c *capturingProvider) Name() string        { return "capturing" }
func (c *capturingProvider) SupportsTools() bool { return true }
func (c *capturingProvider) Chat(ctx context.Context, req provider.ChatRequest) (*provider.ChatResponse, error) {
	c.lastReq = req
	return c.inner.Chat(ctx, req)
}

// ---------------------------------------------------------------------------
// TestProcessMessage_AppendMemoryCalledOnFinalResponse
// ---------------------------------------------------------------------------

func TestProcessMessage_AppendMemoryCalledOnFinalResponse(t *testing.T) {
	prov := &mockProvider{
		responses: []provider.ChatResponse{
			{Content: "here is my answer"},
		},
	}
	ch := &mockChannel{}
	st := &mockStore{}

	ag := New(defaultCfg(), defaultLimits(), ch, prov, st, nil)
	ag.processMessage(context.Background(), channel.IncomingMessage{ChannelID: "test", Text: "hello"})

	if len(st.appendedMems) != 1 {
		t.Fatalf("expected 1 memory entry appended, got %d", len(st.appendedMems))
	}
	mem := st.appendedMems[0]
	if mem.Content != "here is my answer" {
		t.Errorf("expected memory content 'here is my answer', got %q", mem.Content)
	}
	if mem.Source != "conv_test" {
		t.Errorf("expected memory source 'conv_test', got %q", mem.Source)
	}
	if mem.ID == "" {
		t.Error("expected memory ID to be non-empty")
	}
}

func TestProcessMessage_NoMemoryOnEmptyResponse(t *testing.T) {
	prov := &mockProvider{
		responses: []provider.ChatResponse{
			{Content: ""},
		},
	}
	ch := &mockChannel{}
	st := &mockStore{}

	ag := New(defaultCfg(), defaultLimits(), ch, prov, st, nil)
	ag.processMessage(context.Background(), channel.IncomingMessage{ChannelID: "test", Text: "hello"})

	if len(st.appendedMems) != 0 {
		t.Errorf("expected 0 memory entries for empty response, got %d", len(st.appendedMems))
	}
}
