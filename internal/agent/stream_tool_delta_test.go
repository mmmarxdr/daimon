package agent

import (
	"context"
	"sync"
	"testing"
	"time"

	"daimon/internal/audit"
	"daimon/internal/config"
	"daimon/internal/notify"
	"daimon/internal/provider"
	"daimon/internal/skill"
)

// ---------------------------------------------------------------------------
// mockTelemetryEmitter captures EmitTelemetry calls for assertions.
// ---------------------------------------------------------------------------

type mockTelemetryEmitter struct {
	mu     sync.Mutex
	frames []map[string]any
}

func (m *mockTelemetryEmitter) EmitTelemetry(_ context.Context, _ string, frame map[string]any) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	// Deep copy the frame map so later mutations don't affect recorded frames.
	cp := make(map[string]any, len(frame))
	for k, v := range frame {
		cp[k] = v
	}
	m.frames = append(m.frames, cp)
	return nil
}

func (m *mockTelemetryEmitter) filterByType(t string) []map[string]any {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []map[string]any
	for _, f := range m.frames {
		if f["type"] == t {
			out = append(out, f)
		}
	}
	return out
}

// mockTelemetryChannel implements channel.Channel and channel.TelemetryEmitter.
type mockTelemetryChannel struct {
	mockChannel
	te *mockTelemetryEmitter
}

func (c *mockTelemetryChannel) EmitTelemetry(ctx context.Context, channelID string, frame map[string]any) error {
	return c.te.EmitTelemetry(ctx, channelID, frame)
}

// ---------------------------------------------------------------------------
// REQ-4.1 — monotonically increasing cumulative bytes across 3 deltas.
// ---------------------------------------------------------------------------

func TestProcessStreamingCall_ToolDelta_MonotonicCumulativeBytes(t *testing.T) {
	te := &mockTelemetryEmitter{}
	ch := &mockTelemetryChannel{te: te}
	sp := &mockStreamingProvider{}
	ag := New(
		defaultCfg(), defaultLimits(), config.FilterConfig{},
		ch, sp, &mockStore{}, audit.NoopAuditor{},
		nil, nil, skill.SkillIndex{}, 4, true,
	)

	// 3 ToolCallDelta events for the same tool ID with cumulative input lengths
	// 10, 47, 120 (fragment lengths: 10, 37, 73).
	fragment1 := string(make([]byte, 10))
	fragment2 := string(make([]byte, 37))
	fragment3 := string(make([]byte, 73))

	events := []provider.StreamEvent{
		{Type: provider.StreamEventToolCallStart, ToolCallID: "tc-1", ToolName: "shell"},
		{Type: provider.StreamEventToolCallDelta, ToolCallID: "tc-1", ToolInput: fragment1},
		{Type: provider.StreamEventToolCallDelta, ToolCallID: "tc-1", ToolInput: fragment2},
		{Type: provider.StreamEventToolCallDelta, ToolCallID: "tc-1", ToolInput: fragment3},
		{Type: provider.StreamEventToolCallEnd},
		{Type: provider.StreamEventDone},
	}
	resp := &provider.ChatResponse{StopReason: "tool_use"}
	sp.streamFunc = func(ctx context.Context, req provider.ChatRequest) (*provider.StreamResult, error) {
		return scriptedStream(events, resp, nil), nil
	}

	_, _, err := ag.processStreamingCall(
		context.Background(), sp, nil, provider.ChatRequest{}, "ch-1", 0, time.Now(), te, nil,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	deltaFrames := te.filterByType(notify.EventToolDelta)
	if len(deltaFrames) != 3 {
		t.Fatalf("expected 3 tool.delta frames, got %d", len(deltaFrames))
	}

	wantCounts := []int{10, 47, 120}
	for i, frame := range deltaFrames {
		got, ok := frame["token_count"].(int)
		if !ok {
			t.Errorf("frame[%d] token_count is not int: %T = %v", i, frame["token_count"], frame["token_count"])
			continue
		}
		if got != wantCounts[i] {
			t.Errorf("frame[%d] token_count = %d, want %d", i, got, wantCounts[i])
		}
	}
}

// ---------------------------------------------------------------------------
// REQ-4.2 — two interleaved tool IDs: each frame carries the correct ID.
// ---------------------------------------------------------------------------

func TestProcessStreamingCall_ToolDelta_MultipleConcurrentCallsDisambiguatedByID(t *testing.T) {
	te := &mockTelemetryEmitter{}
	ch := &mockTelemetryChannel{te: te}
	sp := &mockStreamingProvider{}
	ag := New(
		defaultCfg(), defaultLimits(), config.FilterConfig{},
		ch, sp, &mockStore{}, audit.NoopAuditor{},
		nil, nil, skill.SkillIndex{}, 4, true,
	)

	frag5 := string(make([]byte, 5))
	frag7 := string(make([]byte, 7))

	events := []provider.StreamEvent{
		{Type: provider.StreamEventToolCallDelta, ToolCallID: "tc-1", ToolInput: frag5},
		{Type: provider.StreamEventToolCallDelta, ToolCallID: "tc-2", ToolInput: frag7},
		{Type: provider.StreamEventToolCallDelta, ToolCallID: "tc-1", ToolInput: frag5},
		{Type: provider.StreamEventDone},
	}
	resp := &provider.ChatResponse{StopReason: "tool_use"}
	sp.streamFunc = func(ctx context.Context, req provider.ChatRequest) (*provider.StreamResult, error) {
		return scriptedStream(events, resp, nil), nil
	}

	_, _, err := ag.processStreamingCall(
		context.Background(), sp, nil, provider.ChatRequest{}, "ch-1", 0, time.Now(), te, nil,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	deltaFrames := te.filterByType(notify.EventToolDelta)
	if len(deltaFrames) != 3 {
		t.Fatalf("expected 3 tool.delta frames, got %d", len(deltaFrames))
	}

	// Frame 0: tc-1 cumulative = 5
	assertToolDeltaFrame(t, deltaFrames[0], "tc-1", 5)
	// Frame 1: tc-2 cumulative = 7
	assertToolDeltaFrame(t, deltaFrames[1], "tc-2", 7)
	// Frame 2: tc-1 cumulative = 10 (5+5)
	assertToolDeltaFrame(t, deltaFrames[2], "tc-1", 10)
}

func assertToolDeltaFrame(t *testing.T, frame map[string]any, wantID string, wantCount int) {
	t.Helper()
	gotID, _ := frame["tool_call_id"].(string)
	if gotID != wantID {
		t.Errorf("tool_call_id = %q, want %q", gotID, wantID)
	}
	gotCount, ok := frame["token_count"].(int)
	if !ok {
		t.Errorf("token_count not int: %T = %v", frame["token_count"], frame["token_count"])
		return
	}
	if gotCount != wantCount {
		t.Errorf("token_count = %d, want %d", gotCount, wantCount)
	}
}

// ---------------------------------------------------------------------------
// REQ-4.3 — nil TelemetryEmitter: no panic.
// ---------------------------------------------------------------------------

func TestProcessStreamingCall_ToolDelta_NilTelemetry_NoPanic(t *testing.T) {
	sp := &mockStreamingProvider{}
	ch := &mockChannel{}
	ag := New(
		defaultCfg(), defaultLimits(), config.FilterConfig{},
		ch, sp, &mockStore{}, audit.NoopAuditor{},
		nil, nil, skill.SkillIndex{}, 4, true,
	)

	events := []provider.StreamEvent{
		{Type: provider.StreamEventToolCallDelta, ToolCallID: "tc-1", ToolInput: "abc"},
		{Type: provider.StreamEventDone},
	}
	resp := &provider.ChatResponse{StopReason: "tool_use"}
	sp.streamFunc = func(ctx context.Context, req provider.ChatRequest) (*provider.StreamResult, error) {
		return scriptedStream(events, resp, nil), nil
	}

	// te=nil — must not panic.
	_, _, err := ag.processStreamingCall(
		context.Background(), sp, nil, provider.ChatRequest{}, "ch-1", 0, time.Now(), nil, nil,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
