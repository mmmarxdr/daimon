package agent

// loop_toolend_denial_test.go — STRICT TDD: tests written RED-first.
//
// Asserts that EventToolEnd carries:
//   - Error == result.Content when IsError (reason/message for consumers)
//   - Meta["denied"] == "true" ONLY for policy/mode denials (name-gate + arg-gate)
//   - Meta["denied"] absent for runtime errors (tool-not-found, tool crash, tool's own IsError)
//   - Error == "" and no "denied" key on success
//
// Design refs: loop.go AD-6 (name-gate ~L723), AD-5 (arg-gate ~L728).

import (
	"context"
	"encoding/json"
	"testing"

	"daimon/internal/audit"
	"daimon/internal/channel"
	"daimon/internal/config"
	"daimon/internal/content"
	"daimon/internal/notify"
	"daimon/internal/provider"
	"daimon/internal/skill"
	"daimon/internal/store"
	"daimon/internal/tool"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// denialTestAgent constructs an Agent in the given mode (plan/build/review)
// with the supplied tool map and a recording bus.
func denialTestAgent(t *testing.T, mode, convID string, prov provider.Provider, tools map[string]tool.Tool, rb *recordingBus) *Agent {
	t.Helper()
	st := &mockStore{
		conv: &store.Conversation{
			ID:        convID,
			ChannelID: convID + "-ch",
			Metadata:  map[string]string{"daimon/mode": mode},
		},
	}
	ch := &mockChannel{}
	ag := New(
		config.AgentConfig{
			MaxIterations:    5,
			MaxTokensPerTurn: 100,
			ContextMode:      config.ContextModeConfig{Mode: config.ContextModeOff},
		},
		defaultLimits(),
		config.FilterConfig{},
		ch,
		prov,
		st,
		audit.NoopAuditor{},
		tools,
		nil,
		skill.SkillIndex{},
		4,
		false,
	).withBus(rb)
	return ag
}

// singleToolCallProvider returns a provider that emits one tool call on the
// first Chat and a final "done" text on the second. rawInput is the tool input.
func singleToolCallProvider(toolName string, rawInput json.RawMessage) provider.Provider {
	return &mockProvider{
		responses: []provider.ChatResponse{
			{
				ToolCalls: []provider.ToolCall{
					{ID: "denial-tc-1", Name: toolName, Input: rawInput},
				},
			},
			{Content: "done"},
		},
	}
}

// captureToolEnd filters the recording bus for a single EventToolEnd for the
// given tool name and returns it. Fails the test if not found.
func captureToolEnd(t *testing.T, rb *recordingBus, toolName string) notify.Event {
	t.Helper()
	ends := rb.filterByType(notify.EventToolEnd)
	for _, ev := range ends {
		if ev.ToolName == toolName {
			return ev
		}
	}
	t.Fatalf("no EventToolEnd for tool %q; got events: %+v", toolName, ends)
	return notify.Event{}
}

// ---------------------------------------------------------------------------
// TC-1: Name-gate denial (plan mode blocks "Bash")
// Error == denial reason, Meta["denied"] == "true".
// ---------------------------------------------------------------------------

func TestToolEnd_NameGateDenial_HasErrorAndDeniedFlag(t *testing.T) {
	t.Parallel()

	rb := &recordingBus{}
	prov := singleToolCallProvider("Bash", json.RawMessage(`{}`))
	bash := &mockTool{name: "Bash"}
	ag := denialTestAgent(t, "plan", "conv-denial-name-1",
		prov,
		map[string]tool.Tool{"Bash": bash},
		rb,
	)

	ag.processMessage(context.Background(), channel.IncomingMessage{
		ChannelID: "conv-denial-name-1-ch",
		SenderID:  "u1",
		Content:   content.TextBlock("run bash"),
	})

	ev := captureToolEnd(t, rb, "Bash")

	if !ev.IsError {
		t.Error("TC-1: IsError must be true for name-gate denial")
	}
	wantError := "tool 'Bash' not allowed in mode 'plan'"
	if ev.Error != wantError {
		t.Errorf("TC-1: Error = %q, want %q", ev.Error, wantError)
	}
	if ev.Meta["denied"] != "true" {
		t.Errorf("TC-1: Meta[\"denied\"] = %q, want \"true\"", ev.Meta["denied"])
	}
}

// ---------------------------------------------------------------------------
// TC-2: Arg-gate denial (review mode blocks mutating git commit on shell_exec)
// Error == rejection reason, Meta["denied"] == "true".
// ---------------------------------------------------------------------------

func TestToolEnd_ArgGateDenial_HasErrorAndDeniedFlag(t *testing.T) {
	t.Parallel()

	rb := &recordingBus{}
	blockedCmd, _ := json.Marshal(map[string]string{"command": "git commit -m 'x'"})
	prov := singleToolCallProvider("shell_exec", json.RawMessage(blockedCmd))
	stub := &shellExecStub{}
	ag := denialTestAgent(t, "review", "conv-denial-arg-1",
		prov,
		map[string]tool.Tool{"shell_exec": stub},
		rb,
	)

	ag.processMessage(context.Background(), channel.IncomingMessage{
		ChannelID: "conv-denial-arg-1-ch",
		SenderID:  "u1",
		Content:   content.TextBlock("commit"),
	})

	ev := captureToolEnd(t, rb, "shell_exec")

	if !ev.IsError {
		t.Error("TC-2: IsError must be true for arg-gate denial")
	}
	if ev.Error != reviewShellRejectMsg {
		t.Errorf("TC-2: Error = %q, want %q", ev.Error, reviewShellRejectMsg)
	}
	if ev.Meta["denied"] != "true" {
		t.Errorf("TC-2: Meta[\"denied\"] = %q, want \"true\"", ev.Meta["denied"])
	}
	// Execute must NOT have been called.
	if stub.calls != 0 {
		t.Errorf("TC-2: stub.Execute should not be called for denied arg; got calls=%d", stub.calls)
	}
}

// ---------------------------------------------------------------------------
// TC-3: Runtime error — tool's own Execute returns IsError.
// IsError==true, Error==message, but Meta["denied"] must be ABSENT.
// ---------------------------------------------------------------------------

func TestToolEnd_RuntimeError_NodeniedFlag(t *testing.T) {
	t.Parallel()

	rb := &recordingBus{}
	prov := singleToolCallProvider("bad_tool", json.RawMessage(`{}`))
	ag := denialTestAgent(t, "build", "conv-denial-runtime-1",
		prov,
		map[string]tool.Tool{
			"bad_tool": &mockTool{
				name:   "bad_tool",
				result: tool.ToolResult{IsError: true, Content: "runtime failure"},
			},
		},
		rb,
	)

	ag.processMessage(context.Background(), channel.IncomingMessage{
		ChannelID: "conv-denial-runtime-1-ch",
		SenderID:  "u1",
		Content:   content.TextBlock("run bad tool"),
	})

	ev := captureToolEnd(t, rb, "bad_tool")

	if !ev.IsError {
		t.Error("TC-3: IsError must be true for runtime error")
	}
	if ev.Error == "" {
		t.Error("TC-3: Error must be non-empty for runtime error")
	}
	if ev.Meta["denied"] == "true" {
		t.Error("TC-3: Meta[\"denied\"] must NOT be set for runtime error")
	}
}

// ---------------------------------------------------------------------------
// TC-4: Tool-not-found (runtime error path, not denial).
// IsError==true, Error set, Meta["denied"] absent.
// ---------------------------------------------------------------------------

func TestToolEnd_ToolNotFound_NoDeniedFlag(t *testing.T) {
	t.Parallel()

	rb := &recordingBus{}
	prov := singleToolCallProvider("ghost_tool", json.RawMessage(`{}`))
	// ghost_tool is NOT in the tools map → "Tool ghost_tool not found" path.
	ag := denialTestAgent(t, "build", "conv-denial-notfound-1",
		prov,
		map[string]tool.Tool{}, // empty tools map
		rb,
	)

	ag.processMessage(context.Background(), channel.IncomingMessage{
		ChannelID: "conv-denial-notfound-1-ch",
		SenderID:  "u1",
		Content:   content.TextBlock("call ghost"),
	})

	ev := captureToolEnd(t, rb, "ghost_tool")

	if !ev.IsError {
		t.Error("TC-4: IsError must be true for tool-not-found")
	}
	if ev.Error == "" {
		t.Error("TC-4: Error must be non-empty for tool-not-found")
	}
	if ev.Meta["denied"] == "true" {
		t.Error("TC-4: Meta[\"denied\"] must NOT be set for tool-not-found (it is a runtime error, not a denial)")
	}
}

// ---------------------------------------------------------------------------
// TC-5: Successful tool call.
// IsError==false, Error=="", Meta["denied"] absent.
// ---------------------------------------------------------------------------

func TestToolEnd_Success_NoErrorNoDeniedFlag(t *testing.T) {
	t.Parallel()

	rb := &recordingBus{}
	prov := singleToolCallProvider("ok_tool", json.RawMessage(`{}`))
	ag := denialTestAgent(t, "build", "conv-denial-success-1",
		prov,
		map[string]tool.Tool{
			"ok_tool": &mockTool{name: "ok_tool", result: tool.ToolResult{Content: "all good"}},
		},
		rb,
	)

	ag.processMessage(context.Background(), channel.IncomingMessage{
		ChannelID: "conv-denial-success-1-ch",
		SenderID:  "u1",
		Content:   content.TextBlock("run ok"),
	})

	ev := captureToolEnd(t, rb, "ok_tool")

	if ev.IsError {
		t.Error("TC-5: IsError must be false for successful tool")
	}
	if ev.Error != "" {
		t.Errorf("TC-5: Error must be empty for success, got %q", ev.Error)
	}
	if ev.Meta["denied"] == "true" {
		t.Error("TC-5: Meta[\"denied\"] must NOT be set for successful tool")
	}
}
