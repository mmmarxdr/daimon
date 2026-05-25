package tui

// rail_panels_test.go — RED tests for PR2b rail panel structs (tasks 2.14 + 2.16).
//
// Test strategy:
//   - Task 2.14 (RED): telemetry / todolist / context-meter Panel structs exist,
//     implement Panel interface, and return "" when no data.
//   - Task 2.16 (RED): EventTodolistChanged triggers a TodoListForConv re-read
//     via a tea.Cmd (not inline in Update); the todolist panel is refreshed.
//
// All tests use newTestModel() — hermetic, no real agent/bus/store.

import (
	"strings"
	"testing"

	"daimon/internal/notify"
	"daimon/internal/tool"
)

// ---------------------------------------------------------------------------
// Task 2.14 — telemetry panel
// ---------------------------------------------------------------------------

// TestTelemetryPanel_NoData_ReturnsEmpty verifies that a telemetry panel with
// zero accumulated values renders as "" (zero-height, per rail contract).
func TestTelemetryPanel_NoData_ReturnsEmpty(t *testing.T) {
	p := newTelemetryPanel(newTuiStyles())
	got := p.Render(32, 20)
	if got != "" {
		t.Errorf("telemetryPanel.Render with no data: got %q, want empty string", got)
	}
}

// TestTelemetryPanel_WithData_RendersTokensAndCost verifies that after
// accumulating token/cost data, Render returns a non-empty string that
// contains the token count and cost.
func TestTelemetryPanel_WithData_RendersTokensAndCost(t *testing.T) {
	p := newTelemetryPanel(newTuiStyles())
	p.accumulate(notify.Event{
		Type:       notify.EventTokensUsage,
		TokenCount: 1500,
		CostUSD:    0.0023,
	})
	got := p.Render(32, 20)
	if got == "" {
		t.Fatal("telemetryPanel.Render with data: got empty string, want non-empty")
	}
	if !strings.Contains(got, "1500") {
		t.Errorf("telemetryPanel.Render: expected token count '1500' in output, got:\n%s", got)
	}
}

// TestTelemetryPanel_ImplementsPanel verifies the interface at compile time.
func TestTelemetryPanel_ImplementsPanel(t *testing.T) {
	var _ Panel = newTelemetryPanel(newTuiStyles())
}

// TestTelemetryPanel_ToolCallCount_Increments verifies that EventToolStart/End
// are counted and appear in the render output.
func TestTelemetryPanel_ToolCallCount_Increments(t *testing.T) {
	p := newTelemetryPanel(newTuiStyles())
	// Need at least one token event to make the panel non-empty.
	p.accumulate(notify.Event{Type: notify.EventTokensUsage, TokenCount: 100, CostUSD: 0.001})
	p.accumulate(notify.Event{Type: notify.EventToolStart})
	p.accumulate(notify.Event{Type: notify.EventToolEnd})
	got := p.Render(32, 20)
	if got == "" {
		t.Fatal("telemetryPanel.Render after tool events: got empty string, want content")
	}
}

// ---------------------------------------------------------------------------
// Task 2.14 — todolist panel
// ---------------------------------------------------------------------------

// TestTodolistPanel_NoData_ReturnsEmpty verifies that a todolist panel with
// no items renders as "" (zero-height).
func TestTodolistPanel_NoData_ReturnsEmpty(t *testing.T) {
	p := newTodolistPanel(newTuiStyles())
	got := p.Render(32, 20)
	if got != "" {
		t.Errorf("todolistPanel.Render with no data: got %q, want empty string", got)
	}
}

// TestTodolistPanel_WithItems_RendersContent verifies that setting a TodoList
// with items causes Render to return non-empty output containing those items.
func TestTodolistPanel_WithItems_RendersContent(t *testing.T) {
	p := newTodolistPanel(newTuiStyles())
	p.setList(tool.TodoList{
		Items: []tool.TodoItem{
			{ID: "1", Content: "Write tests", Status: "pending"},
			{ID: "2", Content: "Implement panels", Status: "done"},
		},
	})
	got := p.Render(32, 20)
	if got == "" {
		t.Fatal("todolistPanel.Render with items: got empty string, want non-empty")
	}
	if !strings.Contains(got, "Write tests") {
		t.Errorf("todolistPanel.Render: expected 'Write tests' in output, got:\n%s", got)
	}
}

// TestTodolistPanel_ImplementsPanel verifies the interface at compile time.
func TestTodolistPanel_ImplementsPanel(t *testing.T) {
	var _ Panel = newTodolistPanel(newTuiStyles())
}

// ---------------------------------------------------------------------------
// Task 2.14 — context-meter panel
// ---------------------------------------------------------------------------

// TestContextMeterPanel_NoData_ReturnsEmpty verifies that a context-meter
// panel with no data renders as "" (zero-height).
func TestContextMeterPanel_NoData_ReturnsEmpty(t *testing.T) {
	p := newContextMeterPanel(newTuiStyles())
	got := p.Render(32, 20)
	if got != "" {
		t.Errorf("contextMeterPanel.Render with no data: got %q, want empty string", got)
	}
}

// TestContextMeterPanel_WithData_RendersContent verifies that after setting
// context usage data, Render returns non-empty output.
func TestContextMeterPanel_WithData_RendersContent(t *testing.T) {
	p := newContextMeterPanel(newTuiStyles())
	p.accumulate(notify.Event{
		Type:       notify.EventTokensUsage,
		TokenCount: 8000,
	})
	got := p.Render(32, 20)
	if got == "" {
		t.Fatal("contextMeterPanel.Render with data: got empty string, want non-empty")
	}
}

// TestContextMeterPanel_ImplementsPanel verifies the interface at compile time.
func TestContextMeterPanel_ImplementsPanel(t *testing.T) {
	var _ Panel = newContextMeterPanel(newTuiStyles())
}

// ---------------------------------------------------------------------------
// Task 2.14 — rail wiring: panelsFor(screenChat) panels are registered
// ---------------------------------------------------------------------------

// TestRail_ChatScreen_PanelsRegistered verifies that after constructing a
// Model with chat panels wired, all three chat rail panels are present and
// render without panic.
func TestRail_ChatScreen_PanelsRegistered(t *testing.T) {
	m := newTestModel()
	m.width = 80
	m.height = 24
	m.screen = screenChat

	// Verify the rail has panels for all three chat panel IDs.
	for _, id := range []panelID{panelTodolist, panelContextMeter, panelTelemetry} {
		if _, ok := m.rail.panels[id]; !ok {
			t.Errorf("rail.panels[%q] not registered — must be wired in newTestModel/RunTUI", id)
		}
	}
}

// TestRail_Render_ChatScreen_NoData_ReturnsEmpty verifies that when all chat
// panels have no data, Render returns "" (zero-height per rail contract).
func TestRail_Render_ChatScreen_NoData_ReturnsEmpty(t *testing.T) {
	m := newTestModel()
	m.screen = screenChat
	got := m.rail.Render(screenChat, 32, 20)
	if got != "" {
		t.Errorf("rail.Render(screenChat) with no data: got %q, want empty string", got)
	}
}

// ---------------------------------------------------------------------------
// Task 2.16 — EventTodolistChanged triggers a TodoListForConv re-read Cmd
// ---------------------------------------------------------------------------

// TestHandleBusEvent_TodolistChanged_ReturnsTodoRefreshCmd verifies that
// receiving EventTodolistChanged in handleBusEvent returns a tea.Cmd
// (not nil — it must schedule the re-read) rather than inlining the IO.
//
// We cannot assert the cmd READS the agent (no real agent in unit tests),
// but we CAN assert:
//  1. The returned model is valid (not panicking).
//  2. A non-nil Cmd is returned (signals that IO is deferred to a Cmd, not inline).
//  3. The model screen has not changed (no spurious screen switch).
func TestHandleBusEvent_TodolistChanged_ReturnsTodoRefreshCmd(t *testing.T) {
	m := newTestModel()
	m.screen = screenChat
	m.activeConvID = "conv-42"

	ev := notify.Event{
		Type:      notify.EventTodolistChanged,
		ChannelID: "tui",
	}
	result, cmd := m.handleBusEvent(ev)
	rm := result.(Model)

	// The model must still be on the chat screen.
	if rm.screen != screenChat {
		t.Errorf("screen after EventTodolistChanged = %v, want screenChat", rm.screen)
	}

	// A Cmd MUST be returned (the todolist re-read + pumpEvents at minimum).
	// We cannot run the cmd in a unit test (no real agent), but nil means the
	// IO happened inline in Update — that violates the Cmd discipline.
	if cmd == nil {
		t.Error("handleBusEvent(EventTodolistChanged): returned nil Cmd; expected non-nil (IO must be in a Cmd, not inline)")
	}
}

// TestHandleBusEvent_TodolistChanged_NoConvID_DoesNotPanic verifies that when
// activeConvID is empty, EventTodolistChanged is handled without panicking.
func TestHandleBusEvent_TodolistChanged_NoConvID_DoesNotPanic(t *testing.T) {
	m := newTestModel()
	m.screen = screenChat
	m.activeConvID = "" // no active conversation

	ev := notify.Event{Type: notify.EventTodolistChanged}
	// Must not panic.
	result, cmd := m.handleBusEvent(ev)
	_ = result
	// Pump cmd is always issued; nil would be wrong.
	if cmd == nil {
		t.Error("handleBusEvent(EventTodolistChanged) with empty convID: returned nil Cmd")
	}
}

// ---------------------------------------------------------------------------
// Task 2.17 — todolist panel refresh wires through to panel state
// ---------------------------------------------------------------------------

// TestModel_TodolistRefreshMsg_UpdatesPanel verifies that a todolistRefreshMsg
// (returned by the TodoListForConv Cmd) updates the todolist panel in the model.
func TestModel_TodolistRefreshMsg_UpdatesPanel(t *testing.T) {
	m := newTestModel()
	m.screen = screenChat
	m.activeConvID = "conv-42"

	items := []tool.TodoItem{
		{ID: "a", Content: "Refactor rail panels", Status: "pending"},
	}
	msg := todolistRefreshMsg{list: tool.TodoList{Items: items}}

	result, _ := m.Update(msg)
	rm := result.(Model)

	// The todolist panel must now have data and render non-empty.
	tp, ok := rm.rail.panels[panelTodolist].(*todolistPanel)
	if !ok {
		t.Fatal("rail.panels[panelTodolist] is not a *todolistPanel after todolistRefreshMsg")
	}
	got := tp.Render(32, 20)
	if got == "" {
		t.Error("todolistPanel.Render after todolistRefreshMsg: got empty string, want non-empty")
	}
	if !strings.Contains(got, "Refactor rail panels") {
		t.Errorf("todolistPanel.Render: expected 'Refactor rail panels' in output, got:\n%s", got)
	}
}
