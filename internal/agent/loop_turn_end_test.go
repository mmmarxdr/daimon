package agent

import (
	"context"
	"encoding/json"
	"math"
	"testing"
	"time"

	"daimon/internal/audit"
	"daimon/internal/channel"
	"daimon/internal/config"
	"daimon/internal/content"
	"daimon/internal/provider"
	"daimon/internal/skill"
	"daimon/internal/tool"
)

// TestProcessMessage_TurnEndReportsActualIterations — DAIM-22. With no iteration
// cap configured (MaxIterations=0, the default), the turn_end telemetry frame
// must report the ACTUAL number of iterations the turn ran, not the
// math.MaxInt32 sentinel the loop uses internally to mean "no hard cap". The
// frontend StatusBar renders this value verbatim as the "iter N" pill, so
// leaking the sentinel showed "iter 2147483647".
func TestProcessMessage_TurnEndReportsActualIterations(t *testing.T) {
	toolCall := provider.ChatResponse{
		ToolCalls: []provider.ToolCall{
			{ID: "t1", Name: "mock_tool", Input: json.RawMessage(`{}`)},
		},
	}
	// 2 tool iterations + 1 final text iteration = 3 iterations total.
	prov := &mockProvider{
		responses: []provider.ChatResponse{toolCall, toolCall, {Content: "done"}},
	}
	ch := &telemetryChannel{}
	st := &mockStore{}
	cfg := config.AgentConfig{MaxIterations: 0, MaxTokensPerTurn: 100}
	limits := config.LimitsConfig{TotalTimeout: 5 * time.Second, ToolTimeout: 1 * time.Second}

	mt := &mockTool{name: "mock_tool", result: tool.ToolResult{Content: "ok"}}
	ag := New(cfg, limits, config.FilterConfig{}, ch, prov, st, audit.NoopAuditor{}, map[string]tool.Tool{"mock_tool": mt}, nil, skill.SkillIndex{}, 4, false)

	ag.processMessage(context.Background(), channel.IncomingMessage{
		ChannelID: "test",
		Content:   content.TextBlock("go"),
	})

	ends := ch.framesByType("turn_end")
	if len(ends) != 1 {
		t.Fatalf("expected 1 turn_end frame, got %d (frames: %v)", len(ends), ch.frames)
	}
	got := ends[0]["iterations"]
	if got == math.MaxInt32 {
		t.Fatalf("turn_end leaked the no-cap sentinel math.MaxInt32 (2147483647) as iterations")
	}
	if got != 3 {
		t.Errorf("turn_end iterations=%v, want 3 (2 tool + 1 final text)", got)
	}
}
