package agent

import (
	"context"
	"testing"
	"time"

	"daimon/internal/audit"
	"daimon/internal/config"
	"daimon/internal/notify"
	"daimon/internal/provider"
	"daimon/internal/skill"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// newAgentWithBus creates a minimal Agent wired to the given recording bus.
func newAgentWithBus(bus notify.Bus) *Agent {
	sp := &mockStreamingProvider{}
	ch := &mockStreamChannel{}
	return New(
		defaultCfg(), defaultLimits(), config.FilterConfig{},
		ch, sp, &mockStore{}, audit.NoopAuditor{},
		nil, nil, skill.SkillIndex{}, 4, true,
	).withBus(bus)
}

// withBus is a test-only method that sets the bus on an Agent and returns it.
// It mirrors the pattern established by agent.WithBus (used in production wiring).
func (a *Agent) withBus(bus notify.Bus) *Agent {
	a.bus = bus
	return a
}

// reasoningStream builds a StreamResult from a sequence of events. It is a
// convenience wrapper over scriptedStream that also injects a Done event at
// the end (unless the caller already added one).
func reasoningStream(events []provider.StreamEvent, resp *provider.ChatResponse) *provider.StreamResult {
	// Ensure the final event is a Done so processStreamingCall can finalize.
	if len(events) == 0 || events[len(events)-1].Type != provider.StreamEventDone {
		events = append(events, provider.StreamEvent{Type: provider.StreamEventDone})
	}
	if resp == nil {
		resp = &provider.ChatResponse{StopReason: "end_turn"}
	}
	return scriptedStream(events, resp, nil)
}

// ---------------------------------------------------------------------------
// REQ-5.1, REQ-7.1 — single reasoning span: exactly 1 start + 1 end.
// ---------------------------------------------------------------------------

func TestProcessStreamingCall_ReasoningStartEnd_Single(t *testing.T) {
	rb := &recordingBus{}
	ag := newAgentWithBus(rb)

	// 15 consecutive ReasoningDeltas then Done.
	events := make([]provider.StreamEvent, 0, 16)
	for i := 0; i < 15; i++ {
		events = append(events, provider.StreamEvent{
			Type: provider.StreamEventReasoningDelta, Text: "think",
		})
	}

	sp := &mockStreamingProvider{
		streamFunc: func(ctx context.Context, req provider.ChatRequest) (*provider.StreamResult, error) {
			return reasoningStream(events, nil), nil
		},
	}

	_, _, err := ag.processStreamingCall(
		context.Background(), sp, nil, provider.ChatRequest{}, "ch-1", 0, time.Now(), nil, nil,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	starts := rb.filterByType(notify.EventReasoningStart)
	ends := rb.filterByType(notify.EventReasoningEnd)

	if len(starts) != 1 {
		t.Errorf("expected 1 agent.reasoning.start, got %d", len(starts))
	}
	if len(ends) != 1 {
		t.Errorf("expected 1 agent.reasoning.end, got %d", len(ends))
	}

	// DurationMs is >= 0: in tests the span may start and end within the same
	// millisecond so we only verify the field is present (non-negative).
	if len(ends) == 1 && ends[0].DurationMs < 0 {
		t.Errorf("agent.reasoning.end DurationMs should be >= 0, got %d", ends[0].DurationMs)
	}
}

// ---------------------------------------------------------------------------
// REQ-5.2, REQ-7.3 — no reasoning → no start/end emitted.
// ---------------------------------------------------------------------------

func TestProcessStreamingCall_NoReasoning_NoBracketEmitted(t *testing.T) {
	rb := &recordingBus{}
	ag := newAgentWithBus(rb)

	events := []provider.StreamEvent{
		{Type: provider.StreamEventTextDelta, Text: "hello"},
		{Type: provider.StreamEventTextDelta, Text: " world"},
	}
	sp := &mockStreamingProvider{
		streamFunc: func(ctx context.Context, req provider.ChatRequest) (*provider.StreamResult, error) {
			return reasoningStream(events, &provider.ChatResponse{Content: "hello world", StopReason: "end_turn"}), nil
		},
	}

	_, _, err := ag.processStreamingCall(
		context.Background(), sp, nil, provider.ChatRequest{}, "ch-1", 0, time.Now(), nil, nil,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if n := len(rb.filterByType(notify.EventReasoningStart)); n != 0 {
		t.Errorf("expected 0 agent.reasoning.start, got %d", n)
	}
	if n := len(rb.filterByType(notify.EventReasoningEnd)); n != 0 {
		t.Errorf("expected 0 agent.reasoning.end, got %d", n)
	}
}

// ---------------------------------------------------------------------------
// REQ-5.3 — interleaved R×5, T×10, R×3 → 2 starts + 2 ends.
// ---------------------------------------------------------------------------

func TestProcessStreamingCall_InterleavedReasoningText_TwoSpans(t *testing.T) {
	rb := &recordingBus{}
	ag := newAgentWithBus(rb)

	events := []provider.StreamEvent{}
	for i := 0; i < 5; i++ {
		events = append(events, provider.StreamEvent{Type: provider.StreamEventReasoningDelta, Text: "r"})
	}
	for i := 0; i < 10; i++ {
		events = append(events, provider.StreamEvent{Type: provider.StreamEventTextDelta, Text: "t"})
	}
	for i := 0; i < 3; i++ {
		events = append(events, provider.StreamEvent{Type: provider.StreamEventReasoningDelta, Text: "r2"})
	}

	sp := &mockStreamingProvider{
		streamFunc: func(ctx context.Context, req provider.ChatRequest) (*provider.StreamResult, error) {
			return reasoningStream(events, nil), nil
		},
	}

	_, _, err := ag.processStreamingCall(
		context.Background(), sp, &mockStreamChannel{}, provider.ChatRequest{}, "ch-1", 0, time.Now(), nil, nil,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	starts := rb.filterByType(notify.EventReasoningStart)
	ends := rb.filterByType(notify.EventReasoningEnd)

	if len(starts) != 2 {
		t.Errorf("expected 2 agent.reasoning.start, got %d", len(starts))
	}
	if len(ends) != 2 {
		t.Errorf("expected 2 agent.reasoning.end, got %d", len(ends))
	}
}

// ---------------------------------------------------------------------------
// REQ-7.2 — reasoning-only response: end emitted at Done.
// ---------------------------------------------------------------------------

func TestProcessStreamingCall_ReasoningOnly_EndAtDone(t *testing.T) {
	rb := &recordingBus{}
	ag := newAgentWithBus(rb)

	events := []provider.StreamEvent{}
	for i := 0; i < 5; i++ {
		events = append(events, provider.StreamEvent{Type: provider.StreamEventReasoningDelta, Text: "r"})
	}

	sp := &mockStreamingProvider{
		streamFunc: func(ctx context.Context, req provider.ChatRequest) (*provider.StreamResult, error) {
			return reasoningStream(events, nil), nil
		},
	}

	_, _, err := ag.processStreamingCall(
		context.Background(), sp, &mockStreamChannel{}, provider.ChatRequest{}, "ch-1", 0, time.Now(), nil, nil,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	ends := rb.filterByType(notify.EventReasoningEnd)
	if len(ends) != 1 {
		t.Errorf("expected 1 agent.reasoning.end at Done, got %d", len(ends))
	}
}

// ---------------------------------------------------------------------------
// REQ-6.1 — WriteReasoning still called on existing path.
// ---------------------------------------------------------------------------

func TestProcessStreamingCall_ReasoningDelta_StillForwardsToWriter(t *testing.T) {
	rb := &recordingBus{}
	ag := newAgentWithBus(rb)

	events := []provider.StreamEvent{
		{Type: provider.StreamEventReasoningDelta, Text: "think1"},
		{Type: provider.StreamEventReasoningDelta, Text: "think2"},
	}
	sp := &mockStreamingProvider{
		streamFunc: func(ctx context.Context, req provider.ChatRequest) (*provider.StreamResult, error) {
			return reasoningStream(events, nil), nil
		},
	}

	sCh := &mockStreamChannel{}
	_, _, err := ag.processStreamingCall(
		context.Background(), sp, sCh, provider.ChatRequest{}, "ch-1", 0, time.Now(), nil, nil,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if sCh.writer == nil {
		t.Fatal("expected stream writer to be opened for reasoning")
	}
	got := sCh.writer.getReasoning()
	if len(got) != 2 || got[0] != "think1" || got[1] != "think2" {
		t.Errorf("WriteReasoning calls: got %v, want [think1 think2]", got)
	}
}

// ---------------------------------------------------------------------------
// REQ-14 — nil bus: no panic.
// ---------------------------------------------------------------------------

func TestProcessStreamingCall_NilBus_NoReasoningEmits_NoPanic(t *testing.T) {
	// Agent with no bus (ag.bus == nil).
	sp := &mockStreamingProvider{}
	ch := &mockStreamChannel{}
	ag := New(
		defaultCfg(), defaultLimits(), config.FilterConfig{},
		ch, sp, &mockStore{}, audit.NoopAuditor{},
		nil, nil, skill.SkillIndex{}, 4, true,
	)
	// ag.bus is nil — no withBus call.

	events := []provider.StreamEvent{
		{Type: provider.StreamEventReasoningDelta, Text: "r1"},
		{Type: provider.StreamEventReasoningDelta, Text: "r2"},
	}
	sp.streamFunc = func(ctx context.Context, req provider.ChatRequest) (*provider.StreamResult, error) {
		return reasoningStream(events, nil), nil
	}

	// Must not panic.
	_, _, err := ag.processStreamingCall(
		context.Background(), sp, ch, provider.ChatRequest{}, "ch-1", 0, time.Now(), nil, nil,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// ---------------------------------------------------------------------------
// REQ-8 invariant — TextDelta MUST NOT emit agent.message.chunk on the bus.
// The text-delta path is interface-only (StreamWriter.WriteChunk).
// Symmetric guard alongside the tool.delta / reasoning.delta invariants.
// ---------------------------------------------------------------------------

func TestProcessStreamingCall_TextDelta_NoBusMessageChunk(t *testing.T) {
	rb := &recordingBus{}
	ag := newAgentWithBus(rb)

	events := []provider.StreamEvent{
		{Type: provider.StreamEventTextDelta, Text: "hello"},
		{Type: provider.StreamEventTextDelta, Text: " world"},
		{Type: provider.StreamEventTextDelta, Text: "!"},
	}
	sp := &mockStreamingProvider{
		streamFunc: func(ctx context.Context, req provider.ChatRequest) (*provider.StreamResult, error) {
			return reasoningStream(events, &provider.ChatResponse{Content: "hello world!", StopReason: "end_turn"}), nil
		},
	}

	_, _, err := ag.processStreamingCall(
		context.Background(), sp, nil, provider.ChatRequest{}, "ch-1", 0, time.Now(), nil, nil,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if n := len(rb.filterByType(notify.EventMessageChunk)); n != 0 {
		t.Errorf("agent.message.chunk MUST be interface-only (StreamWriter.WriteChunk); got %d bus events", n)
	}
	// Also guard the sibling interface-only deltas in the same sweep.
	if n := len(rb.filterByType(notify.EventReasoningDelta)); n != 0 {
		t.Errorf("agent.reasoning.delta MUST be interface-only; got %d bus events", n)
	}
	if n := len(rb.filterByType(notify.EventToolDelta)); n != 0 {
		t.Errorf("agent.tool.delta MUST be interface-only; got %d bus events", n)
	}
}
