package agent

import (
	"context"
	"testing"
	"time"

	"daimon/internal/audit"
	"daimon/internal/config"
	"daimon/internal/provider"
	"daimon/internal/skill"
)

// TestProcessStreamingCall_StreamUsageTelemetry_Unchanged (REQ-9.1, REQ-19.3) —
// cements the existing contract: when a StreamEventUsage arrives, EmitTelemetry
// is called with frame type "stream_usage". This test must PASS immediately,
// confirming the pre-existing behaviour has not been regressed.
func TestProcessStreamingCall_StreamUsageTelemetry_Unchanged(t *testing.T) {
	te := &mockTelemetryEmitter{}
	ch := &mockTelemetryChannel{te: te}
	sp := &mockStreamingProvider{}
	ag := New(
		defaultCfg(), defaultLimits(), config.FilterConfig{},
		ch, sp, &mockStore{}, audit.NoopAuditor{},
		nil, nil, skill.SkillIndex{}, 4, true,
	)

	usage := &provider.UsageStats{InputTokens: 100, OutputTokens: 50}
	events := []provider.StreamEvent{
		{Type: provider.StreamEventTextDelta, Text: "hello"},
		{Type: provider.StreamEventUsage, Usage: usage},
		{Type: provider.StreamEventDone},
	}
	resp := &provider.ChatResponse{Content: "hello", StopReason: "end_turn", Usage: *usage}
	sp.streamFunc = func(ctx context.Context, req provider.ChatRequest) (*provider.StreamResult, error) {
		return scriptedStream(events, resp, nil), nil
	}

	_, _, err := ag.processStreamingCall(
		context.Background(), sp, nil, provider.ChatRequest{}, "ch-1", 0, time.Now(), te, nil,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	usageFrames := te.filterByType("stream_usage")
	if len(usageFrames) != 1 {
		t.Fatalf("expected 1 stream_usage telemetry frame, got %d", len(usageFrames))
	}

	frame := usageFrames[0]
	if frame["input_tokens"] != 100 {
		t.Errorf("input_tokens = %v, want 100", frame["input_tokens"])
	}
	if frame["output_tokens"] != 50 {
		t.Errorf("output_tokens = %v, want 50", frame["output_tokens"])
	}
	if _, ok := frame["elapsed_ms"]; !ok {
		t.Error("elapsed_ms key missing from stream_usage frame")
	}
}
