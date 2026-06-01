package tui

// rail_panels_test.go — tests for PR2b rail panel structs (tasks 2.14–2.17).
//
// Test strategy:
//   - Task 2.14: telemetry / todolist / context-meter Panel structs exist,
//     implement Panel interface, and return "" when no data.
//   - Task 2.16: EventTodolistChanged triggers a TodoListForConv re-read
//     via a tea.Cmd (not inline in Update); the todolist panel is refreshed.
//   - Table-driven throughout; each behaviour variant is a named sub-test.
//
// All tests use newTestModel() — hermetic, no real agent/bus/store.

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/exp/golden"
	"github.com/muesli/termenv"

	"daimon/internal/notify"
	"daimon/internal/store"
	"daimon/internal/tool"
)

// ---------------------------------------------------------------------------
// collectMsgs — flattens tea.Batch messages recursively.
//
// handleBusEvent always wraps its cmds in tea.Batch (via tea.Batch(cmds...)).
// When executed, tea.Batch returns a tea.BatchMsg ([]tea.Cmd). We execute each
// inner cmd SYNCHRONOUSLY and collect all resulting tea.Msg values.
//
// Synchronous execution is leak-free by construction (no goroutine to strand).
// It is safe here because every cmd these tests produce is non-blocking:
// fetchTodolist(ag==nil) returns a no-op todolistRefreshMsg{} instantly, and
// the model's events channel is a CLOSED channel (see the test setup), so
// pumpEvents(events) reads the zero value immediately instead of blocking on a
// nil-channel receive. If a future test introduces a genuinely blocking cmd,
// give it a non-blocking channel rather than reintroducing a timeout goroutine.
// ---------------------------------------------------------------------------

func collectMsgs(cmd tea.Cmd) []tea.Msg {
	if cmd == nil {
		return nil
	}

	msg := cmd()
	if msg == nil {
		return nil
	}

	// Flatten a BatchMsg one level deep (tea.Batch wraps cmds, not msgs).
	if batch, ok := msg.(tea.BatchMsg); ok {
		var msgs []tea.Msg
		for _, inner := range batch {
			msgs = append(msgs, collectMsgs(inner)...)
		}
		return msgs
	}

	return []tea.Msg{msg}
}

// closedEventsChan returns a closed tea.Msg channel. handleBusEvent re-arms the
// pump via pumpEvents(m.events); a closed channel makes that cmd return the zero
// value immediately, so collectMsgs never blocks (and never leaks a goroutine).
func closedEventsChan() <-chan tea.Msg {
	ch := make(chan tea.Msg)
	close(ch)
	return ch
}

// hasTodolistRefreshMsg returns true if any of msgs is a todolistRefreshMsg.
func hasTodolistRefreshMsg(msgs []tea.Msg) bool {
	for _, m := range msgs {
		if _, ok := m.(todolistRefreshMsg); ok {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// Task 2.14 — "no data → returns empty" table (three panels, one table)
// ---------------------------------------------------------------------------

func TestPanels_NoData_ReturnsEmpty(t *testing.T) {
	s := newTuiStyles()
	tests := []struct {
		name   string
		render func() string
	}{
		{
			name: "telemetryPanel",
			render: func() string {
				return newTelemetryPanel(s).Render(32, 20)
			},
		},
		{
			name: "todolistPanel",
			render: func() string {
				return newTodolistPanel(s).Render(32, 20)
			},
		},
		{
			name: "contextMeterPanel",
			render: func() string {
				return newContextMeterPanel(s).Render(32, 20)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.render()
			if got != "" {
				t.Errorf("%s.Render with no data: got %q, want empty string", tt.name, got)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Task 2.14 — interface compliance (compile-time)
// ---------------------------------------------------------------------------

func TestPanels_ImplementPanel(t *testing.T) {
	s := newTuiStyles()
	var _ Panel = newTelemetryPanel(s)
	var _ Panel = newTodolistPanel(s)
	var _ Panel = newContextMeterPanel(s)
}

// ---------------------------------------------------------------------------
// Task 2.14 — telemetry panel render variants (table-driven)
// ---------------------------------------------------------------------------

func TestTelemetryPanel_Render(t *testing.T) {
	tests := []struct {
		name        string
		setup       func(p *telemetryPanel)
		wantEmpty   bool
		wantContain []string
		wantAbsent  []string
	}{
		{
			name:      "no data returns empty",
			setup:     func(_ *telemetryPanel) {},
			wantEmpty: true,
		},
		{
			name: "tokens and cost appear after EventTokensUsage",
			setup: func(p *telemetryPanel) {
				p.accumulate(notify.Event{
					Type:       notify.EventTokensUsage,
					TokenCount: 1500,
					CostUSD:    0.0023,
				})
			},
			wantContain: []string{"1500", "0.0023"},
		},
		{
			name: "tool calls counted; no error line when zero errors",
			setup: func(p *telemetryPanel) {
				p.accumulate(notify.Event{Type: notify.EventTokensUsage, TokenCount: 100, CostUSD: 0.001})
				p.accumulate(notify.Event{Type: notify.EventToolStart})
				p.accumulate(notify.Event{Type: notify.EventToolEnd}) // no error
			},
			wantContain: []string{"1"},      // tools line has count
			wantAbsent:  []string{"errors"}, // errors line must NOT appear when toolErrors == 0
		},
		{
			name: "error count line appears when toolErrors > 0",
			setup: func(p *telemetryPanel) {
				p.accumulate(notify.Event{Type: notify.EventTokensUsage, TokenCount: 200, CostUSD: 0.002})
				p.accumulate(notify.Event{Type: notify.EventToolStart})
				p.accumulate(notify.Event{Type: notify.EventToolEnd, IsError: true})
			},
			wantContain: []string{"errors", "1"},
		},
		{
			name: "multiple errors counted correctly",
			setup: func(p *telemetryPanel) {
				p.accumulate(notify.Event{Type: notify.EventTokensUsage, TokenCount: 300, CostUSD: 0.003})
				for i := 0; i < 3; i++ {
					p.accumulate(notify.Event{Type: notify.EventToolStart})
					p.accumulate(notify.Event{Type: notify.EventToolEnd, IsError: true})
				}
			},
			wantContain: []string{"errors", "3"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := newTelemetryPanel(newTuiStyles())
			tt.setup(p)
			got := p.Render(40, 20)

			if tt.wantEmpty {
				if got != "" {
					t.Errorf("Render: got %q, want empty string", got)
				}
				return
			}
			if got == "" {
				t.Fatal("Render: got empty string, want non-empty")
			}
			for _, want := range tt.wantContain {
				if !strings.Contains(got, want) {
					t.Errorf("Render: expected %q in output:\n%s", want, got)
				}
			}
			for _, absent := range tt.wantAbsent {
				if strings.Contains(got, absent) {
					t.Errorf("Render: expected %q to be absent from output:\n%s", absent, got)
				}
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Task 2.14 — todolist panel: all four status markers (table-driven)
// ---------------------------------------------------------------------------

func TestTodolistPanel_StatusMarkers(t *testing.T) {
	tests := []struct {
		name        string
		status      string
		wantMarkers []string // at least one of these characters must appear in the item line
	}{
		{
			name:        "done shows check mark",
			status:      "done",
			wantMarkers: []string{"✓"},
		},
		{
			name:        "completed shows check mark",
			status:      "completed",
			wantMarkers: []string{"✓"},
		},
		{
			name:        "in_progress shows filled circle",
			status:      "in_progress",
			wantMarkers: []string{"●"},
		},
		{
			name:        "pending shows empty circle",
			status:      "pending",
			wantMarkers: []string{"○"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := newTodolistPanel(newTuiStyles())
			p.setList(tool.TodoList{
				Items: []tool.TodoItem{
					{ID: "1", Content: "test item", Status: tt.status},
				},
			})
			got := p.Render(40, 20)
			if got == "" {
				t.Fatal("Render: got empty string, want content")
			}
			found := false
			for _, marker := range tt.wantMarkers {
				if strings.Contains(got, marker) {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("status=%q: expected one of %v in output:\n%s", tt.status, tt.wantMarkers, got)
			}
		})
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

// ---------------------------------------------------------------------------
// Task 2.14 — context-meter panel
// ---------------------------------------------------------------------------

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

	for _, id := range []panelID{panelTodolist, panelContextMeter, panelTelemetry, panelMemoryPeek} {
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
// Task 2.14 — cost line assertion (was missing from the token+cost test)
// ---------------------------------------------------------------------------

func TestTelemetryPanel_WithData_RendersTokensAndCost(t *testing.T) {
	p := newTelemetryPanel(newTuiStyles())
	p.accumulate(notify.Event{
		Type:       notify.EventTokensUsage,
		TokenCount: 1500,
		CostUSD:    0.0023,
	})
	got := p.Render(40, 20)
	if got == "" {
		t.Fatal("telemetryPanel.Render with data: got empty string, want non-empty")
	}
	if !strings.Contains(got, "1500") {
		t.Errorf("telemetryPanel.Render: expected token count '1500' in output:\n%s", got)
	}
	if !strings.Contains(got, "0.0023") {
		t.Errorf("telemetryPanel.Render: expected cost '0.0023' in output:\n%s", got)
	}
}

// ---------------------------------------------------------------------------
// Task 2.16 — EventTodolistChanged triggers a TodoListForConv re-read Cmd
//
// These tests are NON-VACUOUS: they execute the returned Cmd and assert that
// a todolistRefreshMsg is produced among the resulting messages. If the
// EventTodolistChanged case is removed from handleBusEvent, fetchTodolist is
// never appended to cmds, so no todolistRefreshMsg can appear in the batch —
// causing both tests to FAIL (the guard is real).
//
// The model's events channel is set to a CLOSED channel so the pump cmd that
// handleBusEvent always re-arms (pumpEvents(m.events)) returns immediately
// instead of blocking on a nil-channel receive. fetchTodolist with nil agent
// returns todolistRefreshMsg{} synchronously, so both paths are instant and
// collectMsgs executes synchronously without stranding any goroutine.
// ---------------------------------------------------------------------------

func TestHandleBusEvent_TodolistChanged_ReturnsTodoRefreshCmd(t *testing.T) {
	m := newTestModel()
	m.screen = screenChat
	m.events = closedEventsChan() // pump cmd returns immediately, never blocks
	m.activeConvID = "conv-42"    // non-empty, so fetchTodolist runs (ag==nil → no-op msg)

	ev := notify.Event{
		Type:      notify.EventTodolistChanged,
		ChannelID: "tui",
	}
	result, cmd := m.handleBusEvent(ev)
	rm := result.(Model)

	// Model must still be on the chat screen — no spurious screen switch.
	if rm.screen != screenChat {
		t.Errorf("screen after EventTodolistChanged = %v, want screenChat", rm.screen)
	}

	// Execute the returned Cmd and collect all messages it produces.
	msgs := collectMsgs(cmd)

	// A todolistRefreshMsg MUST appear — it is produced by fetchTodolist.
	// If the EventTodolistChanged case is deleted, fetchTodolist is never
	// scheduled and no todolistRefreshMsg can appear in the batch.
	if !hasTodolistRefreshMsg(msgs) {
		t.Errorf("handleBusEvent(EventTodolistChanged): no todolistRefreshMsg in batch messages %T %v; "+
			"the EventTodolistChanged case may be missing from handleBusEvent", msgs, msgs)
	}
}

func TestHandleBusEvent_TodolistChanged_NoConvID_DoesNotPanic(t *testing.T) {
	m := newTestModel()
	m.screen = screenChat
	m.events = closedEventsChan() // pump cmd returns immediately, never blocks
	m.activeConvID = ""           // empty convID → fetchTodolist returns no-op todolistRefreshMsg{}

	ev := notify.Event{Type: notify.EventTodolistChanged}
	// Must not panic.
	_, cmd := m.handleBusEvent(ev)

	// Execute the returned Cmd and collect messages.
	msgs := collectMsgs(cmd)

	// Even with an empty convID, fetchTodolist still returns a todolistRefreshMsg{}
	// (zero-value, no-op). The EventTodolistChanged case must still produce it.
	if !hasTodolistRefreshMsg(msgs) {
		t.Errorf("handleBusEvent(EventTodolistChanged) with empty convID: " +
			"no todolistRefreshMsg in batch; EventTodolistChanged case may be missing")
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

	tp, ok := rm.rail.panels[panelTodolist].(*todolistPanel)
	if !ok {
		t.Fatal("rail.panels[panelTodolist] is not a *todolistPanel after todolistRefreshMsg")
	}
	got := tp.Render(32, 20)
	if got == "" {
		t.Error("todolistPanel.Render after todolistRefreshMsg: got empty string, want non-empty")
	}
	if !strings.Contains(got, "Refactor rail panels") {
		t.Errorf("todolistPanel.Render: expected 'Refactor rail panels' in output:\n%s", got)
	}
}

// ---------------------------------------------------------------------------
// Task 1.7 — WU-b: resumeListPanel pre-computed ago (RED → GREEN with WU-b)
// ---------------------------------------------------------------------------

// ---------------------------------------------------------------------------
// PR-a tasks a.1–a.9 — context-meter RED tests
// ---------------------------------------------------------------------------

// a.1: REPLACE semantics — second event wins (spec scenario TR-2).
func TestContextMeter_REPLACE_SecondEventWins(t *testing.T) {
	p := newContextMeterPanel(newTuiStyles())

	// First event: non-zero category fields.
	p.accumulate(notify.Event{
		Type:     notify.EventTokensUsage,
		SysToks:  100,
		MsgToks:  200,
		ToolToks: 50,
	})

	// Second event: different values — must replace, not accumulate.
	p.accumulate(notify.Event{
		Type:     notify.EventTokensUsage,
		SysToks:  10,
		MsgToks:  20,
		ToolToks: 5,
	})

	if p.sysToks != 10 {
		t.Errorf("sysToks = %d, want 10 (second event wins)", p.sysToks)
	}
	if p.msgToks != 20 {
		t.Errorf("msgToks = %d, want 20 (second event wins)", p.msgToks)
	}
	if p.toolToks != 5 {
		t.Errorf("toolToks = %d, want 5 (second event wins)", p.toolToks)
	}
	wantUsed := 10 + 20 + 5
	if p.tokenUsed != wantUsed {
		t.Errorf("tokenUsed = %d, want %d (sum of second event only)", p.tokenUsed, wantUsed)
	}
}

// a.2: Legacy fallback — tokenUsed accumulates (spec scenario TR-2).
func TestContextMeter_Legacy_Accumulates(t *testing.T) {
	p := newContextMeterPanel(newTuiStyles())

	// Two events with all-zero category fields and non-zero TokenCount.
	p.accumulate(notify.Event{
		Type:       notify.EventTokensUsage,
		TokenCount: 1000,
	})
	p.accumulate(notify.Event{
		Type:       notify.EventTokensUsage,
		TokenCount: 500,
	})

	// tokenUsed must be cumulative.
	if p.tokenUsed != 1500 {
		t.Errorf("tokenUsed = %d, want 1500 (accumulated both TokenCounts)", p.tokenUsed)
	}
	// Category fields must stay zero.
	if p.sysToks != 0 || p.msgToks != 0 || p.toolToks != 0 {
		t.Errorf("category fields = (%d, %d, %d), want all 0 in legacy mode",
			p.sysToks, p.msgToks, p.toolToks)
	}
}

// a.3: Non-zero limit stored and used (spec scenario TR-1).
func TestContextMeter_SetLimit_NonZero(t *testing.T) {
	p := newContextMeterPanel(newTuiStyles())
	p.setLimit(128000)

	if p.limit != 128000 {
		t.Errorf("limit = %d, want 128000", p.limit)
	}

	// Feed a smart-strategy event so Render has data.
	p.accumulate(notify.Event{
		Type:     notify.EventTokensUsage,
		SysToks:  1000,
		MsgToks:  2000,
		ToolToks: 500,
	})

	got := p.Render(32, 0)
	if strings.Contains(got, " est.") {
		t.Errorf("Render with non-zero limit should NOT contain ' est.', got:\n%s", got)
	}
}

// a.4: Zero limit falls back to heuristic — " est." suffix present (spec scenario TR-1).
func TestContextMeter_SetLimit_ZeroFallback(t *testing.T) {
	p := newContextMeterPanel(newTuiStyles())
	// No setLimit call — limit stays 0.
	p.accumulate(notify.Event{
		Type:       notify.EventTokensUsage,
		TokenCount: 1000,
	})

	got := p.Render(32, 0)
	if !strings.Contains(got, " est.") {
		t.Errorf("Render with zero limit should contain ' est.', got:\n%s", got)
	}
}

// a.5: Smart strategy render — category rows present (spec scenario TR-3).
// State: sysToks=1000, msgToks=2000, toolToks=500, limit=128000.
// Golden: testdata/TestContextMeter_Render_SmartStrategy_Golden.golden
func TestContextMeter_Render_SmartStrategy_Golden(t *testing.T) {
	// Force a deterministic TrueColor profile so ANSI output is stable
	// regardless of the terminal environment (no-TTY vs CLICOLOR_FORCE=1).
	prev := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() { lipgloss.SetColorProfile(prev) })

	p := newContextMeterPanel(newTuiStyles())
	p.setLimit(128000)
	p.accumulate(notify.Event{
		Type:     notify.EventTokensUsage,
		SysToks:  1000,
		MsgToks:  2000,
		ToolToks: 500,
	})
	got := p.Render(32, 0)

	if got == "" {
		t.Fatal("Render: got empty string, want non-empty")
	}
	// Must contain category rows.
	for _, want := range []string{"sys", "msg", "tool"} {
		if !strings.Contains(got, want) {
			t.Errorf("Render: expected %q in output (smart strategy category rows):\n%s", want, got)
		}
	}
	// Must NOT contain " est." (limit is non-zero).
	if strings.Contains(got, " est.") {
		t.Errorf("Render: must NOT contain ' est.' when limit is set:\n%s", got)
	}
	// Bar must have a closing bracket (FIX A: ensures ] is not truncated away).
	if !strings.Contains(got, "]") {
		t.Error("Render: bar must render a closing ]")
	}

	// humanK boundary assertions (design Risk 4 — determinism).
	// 999 → "999", 1000 → "1.0k", 1500 → "1.5k", 200000 → "200k"
	humanKTests := []struct {
		n    int
		want string
	}{
		{999, "999"},
		{1000, "1.0k"},
		{1500, "1.5k"},
		{200000, "200k"},
	}
	for _, tt := range humanKTests {
		gotK := humanK(tt.n)
		if gotK != tt.want {
			t.Errorf("humanK(%d) = %q, want %q", tt.n, gotK, tt.want)
		}
	}

	// Golden pin: regenerate with -update, then rerun without to confirm stability.
	golden.RequireEqual(t, []byte(got))
}

// a.6: Legacy strategy render — only aggregate bar, " est." present (spec scenario TR-3).
// State: tokenUsed=42000, limit=0 (heuristic), sysToks=0.
// Golden: testdata/TestContextMeter_Render_LegacyStrategy_Golden.golden
func TestContextMeter_Render_LegacyStrategy_Golden(t *testing.T) {
	// Force a deterministic TrueColor profile so ANSI output is stable
	// regardless of the terminal environment (no-TTY vs CLICOLOR_FORCE=1).
	prev := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() { lipgloss.SetColorProfile(prev) })

	p := newContextMeterPanel(newTuiStyles())
	// No setLimit — limit stays 0 → heuristic, " est." suffix.
	p.accumulate(notify.Event{
		Type:       notify.EventTokensUsage,
		TokenCount: 42000,
	})
	got := p.Render(32, 0)

	if got == "" {
		t.Fatal("Render: got empty string, want non-empty")
	}
	// Must NOT contain category rows (sysToks == 0).
	for _, absent := range []string{"sys", "msg", "tool"} {
		if strings.Contains(got, absent) {
			t.Errorf("Render (legacy): must NOT contain %q row:\n%s", absent, got)
		}
	}
	// Must contain " est." (limit is 0 → heuristic).
	if !strings.Contains(got, " est.") {
		t.Errorf("Render (legacy): must contain ' est.':\n%s", got)
	}
	// Bar must have a closing bracket (FIX A: ensures ] is not truncated away).
	if !strings.Contains(got, "]") {
		t.Error("Render (legacy): bar must render a closing ]")
	}

	// Golden pin.
	golden.RequireEqual(t, []byte(got))
}

// a.7: No data — empty render (spec scenario TR-3 "No data", TR-0-C).
func TestContextMeter_Render_NoData_Empty(t *testing.T) {
	p := newContextMeterPanel(newTuiStyles())
	got := p.Render(32, 0)
	if got != "" {
		t.Errorf("Render with no data: got %q, want empty string", got)
	}
}

// a.8: Render is deterministic (spec scenario TR-0-A).
func TestContextMeter_Render_Deterministic(t *testing.T) {
	p := newContextMeterPanel(newTuiStyles())
	p.setLimit(128000)
	p.accumulate(notify.Event{
		Type:     notify.EventTokensUsage,
		SysToks:  1000,
		MsgToks:  2000,
		ToolToks: 500,
	})

	got1 := p.Render(32, 0)
	got2 := p.Render(32, 0)
	if got1 != got2 {
		t.Errorf("Render is not deterministic:\nfirst:  %q\nsecond: %q", got1, got2)
	}
}

// a.9: Boot-wiring — setLimit via copyRailWith sets the correct limit (spec scenario TR-1).
func TestContextMeter_BootWiring_RealLimit(t *testing.T) {
	s := newTuiStyles()
	r := newRail(s)

	const wantLimit = 128000
	r = copyRailWith(r, func(panels map[panelID]Panel) {
		if cm, ok := panels[panelContextMeter].(*contextMeterPanel); ok {
			cp := *cm
			cp.setLimit(wantLimit)
			panels[panelContextMeter] = &cp
		}
	})

	cm, ok := r.panels[panelContextMeter].(*contextMeterPanel)
	if !ok {
		t.Fatal("rail.panels[panelContextMeter] is not *contextMeterPanel")
	}
	if cm.limit != wantLimit {
		t.Errorf("contextMeterPanel.limit = %d, want %d", cm.limit, wantLimit)
	}
}

// ---------------------------------------------------------------------------
// Task 1.7 — WU-b: resumeListPanel pre-computed ago (RED → GREEN with WU-b)
// ---------------------------------------------------------------------------

// ---------------------------------------------------------------------------
// Judgment-day fix: context-meter must ignore subagent EventTokensUsage events.
// ---------------------------------------------------------------------------

// TestContextMeter_IgnoresSubagentEvents verifies that accumulate silently
// drops any EventTokensUsage whose Meta["subagent_id"] is non-empty.
//
// Rationale: every subagent emits EventTokensUsage on the SHARED bus carrying
// its own SysToks/MsgToks/ToolToks. Without this guard, the REPLACE branch
// would overwrite the main conversation's window fill with a subagent's values
// during multi-agent runs. The telemetry panel handles per-subagent tokens;
// the context meter tracks ONLY the top-level conversation's window.
func TestContextMeter_IgnoresSubagentEvents(t *testing.T) {
	p := newContextMeterPanel(newTuiStyles())

	// Seed a top-level snapshot (no subagent_id — main conversation).
	p.accumulate(notify.Event{
		Type:     notify.EventTokensUsage,
		SysToks:  1000,
		MsgToks:  2000,
		ToolToks: 500,
	})
	// tokenUsed must reflect the top-level event.
	if p.tokenUsed != 3500 {
		t.Fatalf("pre-guard: tokenUsed = %d, want 3500", p.tokenUsed)
	}

	// Subagent smart-strategy event: should be ignored entirely.
	p.accumulate(notify.Event{
		Type:     notify.EventTokensUsage,
		SysToks:  9000,
		MsgToks:  9000,
		ToolToks: 9000,
		Meta:     map[string]string{"subagent_id": "sa-x"},
	})

	if p.tokenUsed != 3500 {
		t.Errorf("tokenUsed = %d after subagent event, want 3500 (subagent event must be ignored)", p.tokenUsed)
	}
	if p.sysToks != 1000 {
		t.Errorf("sysToks = %d, want 1000 (subagent event must not clobber top-level snapshot)", p.sysToks)
	}
	if p.msgToks != 2000 {
		t.Errorf("msgToks = %d, want 2000", p.msgToks)
	}
	if p.toolToks != 500 {
		t.Errorf("toolToks = %d, want 500", p.toolToks)
	}

	// Also verify the legacy/aggregate path: a subagent event with only
	// TokenCount set (and subagent_id present) must NOT inflate tokenUsed.
	pLegacy := newContextMeterPanel(newTuiStyles())
	// Seed with a top-level legacy event.
	pLegacy.accumulate(notify.Event{
		Type:       notify.EventTokensUsage,
		TokenCount: 5000,
	})
	// Subagent legacy event — must be ignored.
	pLegacy.accumulate(notify.Event{
		Type:       notify.EventTokensUsage,
		TokenCount: 9000,
		Meta:       map[string]string{"subagent_id": "sa-y"},
	})
	if pLegacy.tokenUsed != 5000 {
		t.Errorf("legacy path: tokenUsed = %d after subagent event, want 5000 (subagent event must be ignored)",
			pLegacy.tokenUsed)
	}
}

// ---------------------------------------------------------------------------
// PR-b tasks b.1–b.16 — telemetry RED tests
// ---------------------------------------------------------------------------

// b.1: Tool call counted on EventToolStart (spec scenario TR-4).
func TestTelemetry_ToolStats_CallsCountedOnStart(t *testing.T) {
	p := newTelemetryPanel(newTuiStyles())

	p.accumulate(notify.Event{Type: notify.EventToolStart, ToolName: "bash"})
	p.accumulate(notify.Event{Type: notify.EventToolStart, ToolName: "bash"})

	if p.toolStats == nil {
		t.Fatal("toolStats is nil after EventToolStart")
	}
	stat, ok := p.toolStats["bash"]
	if !ok {
		t.Fatal("toolStats[\"bash\"] not found after two EventToolStart events")
	}
	if stat.calls != 2 {
		t.Errorf("toolStats[\"bash\"].calls = %d, want 2", stat.calls)
	}
	if stat.errors != 0 {
		t.Errorf("toolStats[\"bash\"].errors = %d, want 0", stat.errors)
	}
}

// b.2: Tool error and duration recorded on EventToolEnd (spec scenario TR-4).
func TestTelemetry_ToolStats_ErrorAndDurationOnEnd(t *testing.T) {
	p := newTelemetryPanel(newTuiStyles())

	p.accumulate(notify.Event{Type: notify.EventToolStart, ToolName: "read_file"})
	p.accumulate(notify.Event{Type: notify.EventToolEnd, ToolName: "read_file", DurationMs: 150, IsError: true})

	if p.toolStats == nil {
		t.Fatal("toolStats is nil after events")
	}
	stat, ok := p.toolStats["read_file"]
	if !ok {
		t.Fatal("toolStats[\"read_file\"] not found")
	}
	if stat.errors != 1 {
		t.Errorf("toolStats[\"read_file\"].errors = %d, want 1", stat.errors)
	}
	if stat.durationMs != 150 {
		t.Errorf("toolStats[\"read_file\"].durationMs = %d, want 150", stat.durationMs)
	}
}

// b.3: Accumulation across multiple distinct tool names (spec scenario TR-4).
func TestTelemetry_ToolStats_MultipleTools(t *testing.T) {
	p := newTelemetryPanel(newTuiStyles())

	p.accumulate(notify.Event{Type: notify.EventToolStart, ToolName: "bash"})
	p.accumulate(notify.Event{Type: notify.EventToolStart, ToolName: "read_file"})
	p.accumulate(notify.Event{Type: notify.EventToolStart, ToolName: "write_file"})

	if len(p.toolStats) != 3 {
		t.Errorf("len(toolStats) = %d, want 3", len(p.toolStats))
	}
	for _, name := range []string{"bash", "read_file", "write_file"} {
		stat, ok := p.toolStats[name]
		if !ok {
			t.Errorf("toolStats[%q] not found", name)
			continue
		}
		if stat.calls != 1 {
			t.Errorf("toolStats[%q].calls = %d, want 1", name, stat.calls)
		}
	}
}

// b.4: Live accumulation from multiple EventTokensUsage events with subagent_id
// (spec scenario TR-6).
func TestTelemetry_SubagentLive_Accumulates(t *testing.T) {
	p := newTelemetryPanel(newTuiStyles())

	p.accumulate(notify.Event{
		Type: notify.EventTokensUsage,
		Meta: map[string]string{"subagent_id": "sa-abc", "input_tokens": "100", "output_tokens": "50"},
	})
	p.accumulate(notify.Event{
		Type: notify.EventTokensUsage,
		Meta: map[string]string{"subagent_id": "sa-abc", "input_tokens": "100", "output_tokens": "50"},
	})

	if p.subagentStats == nil {
		t.Fatal("subagentStats is nil after EventTokensUsage with subagent_id")
	}
	st, ok := p.subagentStats["sa-abc"]
	if !ok {
		t.Fatal("subagentStats[\"sa-abc\"] not found")
	}
	if st.tokens != 300 {
		t.Errorf("subagentStats[\"sa-abc\"].tokens = %d, want 300", st.tokens)
	}
	if st.done {
		t.Error("subagentStats[\"sa-abc\"].done = true, want false")
	}
}

// b.5: atoiSafe handles missing or non-numeric Meta values (spec scenario TR-6).
func TestTelemetry_AtoiSafe_BadMeta(t *testing.T) {
	p := newTelemetryPanel(newTuiStyles())

	p.accumulate(notify.Event{
		Type: notify.EventTokensUsage,
		Meta: map[string]string{"subagent_id": "sa-x", "input_tokens": "", "output_tokens": "abc"},
	})

	if p.subagentStats == nil {
		t.Fatal("subagentStats is nil after EventTokensUsage")
	}
	st, ok := p.subagentStats["sa-x"]
	if !ok {
		t.Fatal("subagentStats[\"sa-x\"] not found")
	}
	if st.tokens != 0 {
		t.Errorf("subagentStats[\"sa-x\"].tokens = %d, want 0 (bad meta should not panic)", st.tokens)
	}
}

// b.6: Authoritative total from EventSubagentCompleted overwrites live
// accumulation (spec scenario TR-7).
func TestTelemetry_SubagentCompleted_AuthoritativeWins(t *testing.T) {
	p := newTelemetryPanel(newTuiStyles())

	// Live accumulate 250 tokens first.
	p.accumulate(notify.Event{
		Type: notify.EventTokensUsage,
		Meta: map[string]string{"subagent_id": "sa-1", "input_tokens": "150", "output_tokens": "100"},
	})

	// Authoritative Completed event: must replace, not add.
	p.accumulate(notify.Event{
		Type: notify.EventSubagentCompleted,
		Meta: map[string]string{"subagent_id": "sa-1", "tokens": "405"},
	})

	if p.subagentStats == nil {
		t.Fatal("subagentStats is nil")
	}
	st, ok := p.subagentStats["sa-1"]
	if !ok {
		t.Fatal("subagentStats[\"sa-1\"] not found")
	}
	if st.tokens != 405 {
		t.Errorf("subagentStats[\"sa-1\"].tokens = %d, want 405 (authoritative wins)", st.tokens)
	}
	if !st.done {
		t.Error("subagentStats[\"sa-1\"].done = false, want true after Completed")
	}
}

// b.7: Empty subagent_id in EventSubagentCompleted is a no-op (spec scenario TR-7).
func TestTelemetry_SubagentCompleted_EmptyID_NoOp(t *testing.T) {
	p := newTelemetryPanel(newTuiStyles())

	p.accumulate(notify.Event{
		Type: notify.EventSubagentCompleted,
		Meta: map[string]string{"subagent_id": ""},
	})

	if len(p.subagentStats) != 0 {
		t.Errorf("subagentStats should be empty after empty subagent_id, got %d entries", len(p.subagentStats))
	}
}

// b.8: EventSubagentCompleted creates a bucket for previously unseen subagent
// (spec scenario TR-7 "creates bucket").
func TestTelemetry_SubagentCompleted_UnseenCreates(t *testing.T) {
	p := newTelemetryPanel(newTuiStyles())

	p.accumulate(notify.Event{
		Type: notify.EventSubagentCompleted,
		Meta: map[string]string{"subagent_id": "sa-new", "tokens": "120"},
	})

	if p.subagentStats == nil {
		t.Fatal("subagentStats is nil")
	}
	st, ok := p.subagentStats["sa-new"]
	if !ok {
		t.Fatal("subagentStats[\"sa-new\"] not found after Completed on unseen id")
	}
	if st.tokens != 120 {
		t.Errorf("subagentStats[\"sa-new\"].tokens = %d, want 120", st.tokens)
	}
	if !st.done {
		t.Error("subagentStats[\"sa-new\"].done = false, want true")
	}
}

// b.9: EventSubagentFailed sets done+failed markers, does NOT read tokens from
// Meta (spec scope boundary + design ADR-2).
func TestTelemetry_SubagentFailed_MarkerSet(t *testing.T) {
	p := newTelemetryPanel(newTuiStyles())

	// Ensure no tokens are read from Meta (design: Failed has no guaranteed tokens key).
	p.accumulate(notify.Event{
		Type: notify.EventSubagentFailed,
		Meta: map[string]string{"subagent_id": "sa-f", "tokens": "9999"},
	})

	if p.subagentStats == nil {
		t.Fatal("subagentStats is nil")
	}
	st, ok := p.subagentStats["sa-f"]
	if !ok {
		t.Fatal("subagentStats[\"sa-f\"] not found after EventSubagentFailed")
	}
	if !st.done {
		t.Error("subagentStats[\"sa-f\"].done = false, want true")
	}
	if !st.failed {
		t.Error("subagentStats[\"sa-f\"].failed = false, want true")
	}
	// tokens MUST NOT be read from Meta["tokens"] for Failed events.
	if st.tokens != 0 {
		t.Errorf("subagentStats[\"sa-f\"].tokens = %d, want 0 (Failed must not read Meta[\"tokens\"])", st.tokens)
	}
}

// b.10: Late EventTokensUsage after EventSubagentCompleted must not re-inflate
// tokens (design Risk 5 `if !st.done` guard).
func TestTelemetry_SubagentLive_LateEventIgnoredAfterDone(t *testing.T) {
	p := newTelemetryPanel(newTuiStyles())

	// Mark done via Completed.
	p.accumulate(notify.Event{
		Type: notify.EventSubagentCompleted,
		Meta: map[string]string{"subagent_id": "sa-done", "tokens": "405"},
	})

	// Late live event — must be ignored.
	p.accumulate(notify.Event{
		Type: notify.EventTokensUsage,
		Meta: map[string]string{"subagent_id": "sa-done", "input_tokens": "500", "output_tokens": "500"},
	})

	st := p.subagentStats["sa-done"]
	if st.tokens != 405 {
		t.Errorf("tokens = %d after late live event, want 405 (done guard failed)", st.tokens)
	}
}

// b.11: COW discipline — prior snapshot must be unchanged after accumulate on copy
// (spec scenario TR-0-B + design Risk 5 — the mandatory COW map-clone test).
// This test MUST fail until cloneToolStats/cloneSubagentStats/cloneSAOrder exist.
func TestTelemetry_COW_PriorSnapshotUnchangedAfterAccumulate(t *testing.T) {
	s := newTuiStyles()
	r := newRail(s)

	// Seed the panel with one tool event so toolStats is non-nil.
	r = copyRailWith(r, func(panels map[panelID]Panel) {
		if tp, ok := panels[panelTelemetry].(*telemetryPanel); ok {
			cp := *tp
			cp.accumulate(notify.Event{Type: notify.EventToolStart, ToolName: "bash"})
			panels[panelTelemetry] = &cp
		}
	})

	// Capture the "before" snapshot.
	original, ok := r.panels[panelTelemetry].(*telemetryPanel)
	if !ok {
		t.Fatal("panelTelemetry not *telemetryPanel")
	}
	beforeCalls := original.toolStats["bash"].calls

	// Apply a second accumulate via copyRailWith (this is the COW path).
	r2 := copyRailWith(r, func(panels map[panelID]Panel) {
		if tp, ok := panels[panelTelemetry].(*telemetryPanel); ok {
			cp := *tp
			cp.accumulate(notify.Event{Type: notify.EventToolStart, ToolName: "bash"})
			panels[panelTelemetry] = &cp
		}
	})

	// Verify the new panel shows the increment.
	newTp, ok := r2.panels[panelTelemetry].(*telemetryPanel)
	if !ok {
		t.Fatal("r2 panelTelemetry not *telemetryPanel")
	}
	if newTp.toolStats["bash"].calls != beforeCalls+1 {
		t.Errorf("new panel toolStats[\"bash\"].calls = %d, want %d", newTp.toolStats["bash"].calls, beforeCalls+1)
	}

	// The ORIGINAL panel must not have been mutated (COW invariant).
	if original.toolStats["bash"].calls != beforeCalls {
		t.Errorf("ORIGINAL panel toolStats[\"bash\"].calls = %d, want %d (COW violation — map was shared)",
			original.toolStats["bash"].calls, beforeCalls)
	}
}

// b.12: EventSubagentCompleted through handleBusEvent updates telemetryPanel
// (spec scenario TR-7, non-vacuous via handleBusEvent path).
func TestTelemetry_HandleBusEvent_SubagentCompleted_UpdatesPanel(t *testing.T) {
	m := newTestModel()
	m.screen = screenChat
	m.events = closedEventsChan()

	ev := notify.Event{
		Type: notify.EventSubagentCompleted,
		Meta: map[string]string{"subagent_id": "sa-hbe", "tokens": "777"},
	}
	result, _ := m.handleBusEvent(ev)
	rm := result.(Model)

	tp, ok := rm.rail.panels[panelTelemetry].(*telemetryPanel)
	if !ok {
		t.Fatal("panelTelemetry not *telemetryPanel after handleBusEvent")
	}
	if tp.subagentStats == nil {
		t.Fatal("subagentStats is nil after EventSubagentCompleted via handleBusEvent")
	}
	st, found := tp.subagentStats["sa-hbe"]
	if !found {
		t.Fatal("subagentStats[\"sa-hbe\"] not found")
	}
	if st.tokens != 777 {
		t.Errorf("subagentStats[\"sa-hbe\"].tokens = %d, want 777", st.tokens)
	}
	if !st.done {
		t.Error("subagentStats[\"sa-hbe\"].done = false, want true")
	}
}

// b.13: Golden test — tool rows rendered in count-desc order (spec scenario TR-5).
// Golden: testdata/TestTelemetry_Render_ToolRows_Golden.golden
func TestTelemetry_Render_ToolRows_Golden(t *testing.T) {
	prev := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() { lipgloss.SetColorProfile(prev) })

	p := newTelemetryPanel(newTuiStyles())
	// Force known state: toolStats with 3 tools, counts A=5, B=3, C=1.
	// Use EventToolStart to build stats so accumulate handles cloning.
	for i := 0; i < 5; i++ {
		p.accumulate(notify.Event{Type: notify.EventToolStart, ToolName: "tool_a"})
	}
	for i := 0; i < 3; i++ {
		p.accumulate(notify.Event{Type: notify.EventToolStart, ToolName: "tool_b"})
	}
	p.accumulate(notify.Event{Type: notify.EventToolStart, ToolName: "tool_c"})
	// Need hasData = true (toolStats alone does not set it; token event does).
	p.accumulate(notify.Event{Type: notify.EventTokensUsage, TokenCount: 100})

	got := p.Render(40, 0)
	if got == "" {
		t.Fatal("Render: got empty string, want non-empty")
	}
	for _, name := range []string{"tool_a", "tool_b", "tool_c"} {
		if !strings.Contains(got, name) {
			t.Errorf("Render: expected %q in output (≤5 tools → all shown):\n%s", name, got)
		}
	}
	// Must NOT have "+N more" — only 3 tools.
	if strings.Contains(got, "more") {
		t.Errorf("Render: must NOT contain 'more' for 3 tools:\n%s", got)
	}

	golden.RequireEqual(t, []byte(got))
}

// b.14: Cap at 5 tools — "+N more" line appears (spec scenario TR-5).
func TestTelemetry_Render_ToolRows_Cap5_Overflow(t *testing.T) {
	p := newTelemetryPanel(newTuiStyles())

	for i := 0; i < 8; i++ {
		name := fmt.Sprintf("tool_%d", i)
		p.accumulate(notify.Event{Type: notify.EventToolStart, ToolName: name})
	}
	p.accumulate(notify.Event{Type: notify.EventTokensUsage, TokenCount: 100})

	got := p.Render(40, 0)
	if got == "" {
		t.Fatal("Render: got empty string")
	}

	// Count rendered tool rows (lines containing "tool_").
	toolRowCount := 0
	for _, line := range strings.Split(got, "\n") {
		if strings.Contains(line, "tool_") {
			toolRowCount++
		}
	}
	if toolRowCount != 5 {
		t.Errorf("Render: got %d tool rows, want exactly 5 (cap enforced)", toolRowCount)
	}
	if !strings.Contains(got, "+3 more") {
		t.Errorf("Render: expected '+3 more' for 8-5=3 overflow tools:\n%s", got)
	}
}

// b.15: Golden test — subagent rows rendered (spec scenario TR-8).
// Golden: testdata/TestTelemetry_Render_SubagentRows_Golden.golden
func TestTelemetry_Render_SubagentRows_Golden(t *testing.T) {
	prev := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() { lipgloss.SetColorProfile(prev) })

	p := newTelemetryPanel(newTuiStyles())
	// Seed two subagents via Completed (authoritative) so saOrder is populated.
	p.accumulate(notify.Event{
		Type: notify.EventSubagentCompleted,
		Meta: map[string]string{"subagent_id": "sa-alpha-1", "tokens": "200"},
	})
	p.accumulate(notify.Event{
		Type: notify.EventSubagentCompleted,
		Meta: map[string]string{"subagent_id": "sa-beta-2", "tokens": "150"},
	})
	// hasData needed for Render.
	p.accumulate(notify.Event{Type: notify.EventTokensUsage, TokenCount: 100})

	got := p.Render(40, 0)
	if got == "" {
		t.Fatal("Render: got empty string, want non-empty")
	}
	// Both IDs must appear (truncated to 8 runes → "sa-alpha-" → "sa-alpha" or similar).
	// Use first 8 rune chars of each ID.
	for _, id := range []string{"sa-alpha", "sa-beta-"} {
		if !strings.Contains(got, id[:8]) {
			t.Errorf("Render: expected subagent ID prefix %q in output:\n%s", id[:8], got)
		}
	}

	golden.RequireEqual(t, []byte(got))
}

// b.16: Cap at 3 subagent rows (spec scenario TR-8).
func TestTelemetry_Render_SubagentRows_Cap3(t *testing.T) {
	p := newTelemetryPanel(newTuiStyles())

	// Seed 5 subagents.
	for i := 0; i < 5; i++ {
		id := fmt.Sprintf("sa-sub-%d", i)
		p.accumulate(notify.Event{
			Type: notify.EventSubagentCompleted,
			Meta: map[string]string{"subagent_id": id, "tokens": "100"},
		})
	}
	p.accumulate(notify.Event{Type: notify.EventTokensUsage, TokenCount: 100})

	got := p.Render(40, 0)
	if got == "" {
		t.Fatal("Render: got empty string")
	}

	// Count subagent rows (lines containing "sa-sub-").
	saRowCount := 0
	for _, line := range strings.Split(got, "\n") {
		if strings.Contains(line, "sa-sub") {
			saRowCount++
		}
	}
	if saRowCount != 3 {
		t.Errorf("Render: got %d subagent rows, want exactly 3 (cap enforced)", saRowCount)
	}
	// Subagent rows use silent truncation (no "+N more" overflow line, per
	// spec TR-8 / design ADR-2) — unlike tool rows. Guard against a
	// regression that adds one.
	if strings.Contains(got, "more") {
		t.Errorf("Render: subagent rows must NOT show a '+N more' overflow line:\n%s", got)
	}
}

// ---------------------------------------------------------------------------
// PR-c tasks c.3–c.12 — memory-peek panel RED tests
// ---------------------------------------------------------------------------

// c.3: Empty entries — Render returns "" (spec scenario TR-10 "Empty entries").
func TestMemoryPeek_Render_Empty(t *testing.T) {
	p := newMemoryPeekPanel(newTuiStyles())
	got := p.Render(32, 0)
	if got != "" {
		t.Errorf("memoryPeekPanel.Render with no entries: got %q, want empty string", got)
	}
}

// c.4: Populated entries — rows rendered with titles and "memory" badge
// (spec scenario TR-10 "Populated entries — rows rendered").
// Golden: testdata/TestMemoryPeek_Render_PopulatedTitles_Golden.golden
func TestMemoryPeek_Render_PopulatedTitles_Golden(t *testing.T) {
	prev := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() { lipgloss.SetColorProfile(prev) })

	p := newMemoryPeekPanel(newTuiStyles())
	p.setEntries([]store.MemoryEntry{
		{Title: "Memory entry one"},
		{Title: "Memory entry two"},
		{Title: "Memory entry three"},
	})

	got := p.Render(32, 0)
	if got == "" {
		t.Fatal("Render: got empty string, want non-empty")
	}

	// All three titles must appear.
	for _, title := range []string{"Memory entry one", "Memory entry two", "Memory entry three"} {
		if !strings.Contains(got, title) {
			t.Errorf("Render: expected %q in output:\n%s", title, got)
		}
	}
	// The "MEMORY" badge must appear in the header (panelHeader uppercases the label).
	if !strings.Contains(got, "MEMORY") {
		t.Errorf("Render: expected 'MEMORY' header badge in output:\n%s", got)
	}

	golden.RequireEqual(t, []byte(got))
}

// c.5: Entries cap at 5 (spec scenario TR-10 "Entries cap at 5").
func TestMemoryPeek_Render_Cap5(t *testing.T) {
	p := newMemoryPeekPanel(newTuiStyles())
	entries := make([]store.MemoryEntry, 8)
	for i := range entries {
		entries[i] = store.MemoryEntry{Title: fmt.Sprintf("entry-%d", i)}
	}
	p.setEntries(entries)

	got := p.Render(32, 0)
	if got == "" {
		t.Fatal("Render: got empty string")
	}

	// Count rendered entry rows (lines containing "entry-").
	entryRowCount := 0
	for _, line := range strings.Split(got, "\n") {
		if strings.Contains(line, "entry-") {
			entryRowCount++
		}
	}
	if entryRowCount > 5 {
		t.Errorf("Render: got %d entry rows, want at most 5 (cap enforced)", entryRowCount)
	}
}

// c.6: Empty Title falls back to Content (spec scenario TR-10 "Empty Title falls back to Content").
func TestMemoryPeek_Render_TitleFallback_Content(t *testing.T) {
	p := newMemoryPeekPanel(newTuiStyles())
	p.setEntries([]store.MemoryEntry{
		{Title: "", Content: "some content text"},
	})

	got := p.Render(32, 0)
	if got == "" {
		t.Fatal("Render: got empty string, want non-empty")
	}
	if !strings.Contains(got, "some content") {
		t.Errorf("Render: expected 'some content' (fallback Content) in output:\n%s", got)
	}
}

// c.7: fetchMemory with nil store returns empty msg without panic (spec scenario TR-11).
func TestFetchMemory_NilStore_NoOp(t *testing.T) {
	cmd := fetchMemory(nil, "scope-123")
	if cmd == nil {
		t.Fatal("fetchMemory returned nil cmd, want non-nil")
	}
	msg := cmd()
	rm, ok := msg.(memoryRefreshMsg)
	if !ok {
		t.Fatalf("fetchMemory(nil, ...) returned %T, want memoryRefreshMsg", msg)
	}
	if len(rm.entries) != 0 {
		t.Errorf("entries = %d, want 0 (nil store no-op)", len(rm.entries))
	}
}

// c.8: fetchMemory with empty scopeID returns empty msg, SearchMemory NOT called
// (spec scenario TR-11 "Empty scopeID produces empty msg without panic").
func TestFetchMemory_EmptyScopeID_NoOp(t *testing.T) {
	// Use a fake store that panics if SearchMemory is called.
	fakeStore := &panicOnSearchStore{}
	cmd := fetchMemory(fakeStore, "")
	if cmd == nil {
		t.Fatal("fetchMemory returned nil cmd")
	}
	msg := cmd()
	rm, ok := msg.(memoryRefreshMsg)
	if !ok {
		t.Fatalf("fetchMemory(store, \"\") returned %T, want memoryRefreshMsg", msg)
	}
	if len(rm.entries) != 0 {
		t.Errorf("entries = %d, want 0 (empty scopeID no-op)", len(rm.entries))
	}
}

// panicOnSearchStore is a fake store.Store that panics if SearchMemory is called.
// Used to verify fetchMemory does NOT call SearchMemory for empty scopeID / nil store.
// Embeds noopStore to satisfy the full store.Store interface.
type panicOnSearchStore struct{ noopStore }

func (s *panicOnSearchStore) SearchMemory(_ context.Context, _, _ string, _ int) ([]store.MemoryEntry, error) {
	panic("SearchMemory was called when it should not have been")
}

// c.9: fetchMemory with valid inputs returns entries from store (spec scenario TR-11).
func TestFetchMemory_ValidInputs_ReturnsEntries(t *testing.T) {
	fakeStore := &fakeMemoryStore{
		entries: []store.MemoryEntry{
			{Title: "entry-1"},
			{Title: "entry-2"},
			{Title: "entry-3"},
		},
	}
	cmd := fetchMemory(fakeStore, "scope-1")
	if cmd == nil {
		t.Fatal("fetchMemory returned nil cmd")
	}
	msg := cmd()
	rm, ok := msg.(memoryRefreshMsg)
	if !ok {
		t.Fatalf("fetchMemory(fakeStore, \"scope-1\") returned %T, want memoryRefreshMsg", msg)
	}
	if len(rm.entries) != 3 {
		t.Errorf("entries len = %d, want 3", len(rm.entries))
	}
}

// fakeMemoryStore implements store.Store: SearchMemory returns canned entries;
// all other methods delegate to noopStore.
type fakeMemoryStore struct {
	noopStore
	entries []store.MemoryEntry
}

func (s *fakeMemoryStore) SearchMemory(_ context.Context, _, _ string, _ int) ([]store.MemoryEntry, error) {
	return s.entries, nil
}

// noopStore is a no-op implementation of store.Store used as an embed base for
// fake stores in tests. All methods return zero values.
type noopStore struct{}

func (noopStore) SaveConversation(_ context.Context, _ store.Conversation) error { return nil }
func (noopStore) LoadConversation(_ context.Context, _ string) (*store.Conversation, error) {
	return nil, nil
}
func (noopStore) ListConversations(_ context.Context, _ string, _ int) ([]store.Conversation, error) {
	return nil, nil
}
func (noopStore) AppendMemory(_ context.Context, _ string, _ store.MemoryEntry) error { return nil }
func (noopStore) SearchMemory(_ context.Context, _, _ string, _ int) ([]store.MemoryEntry, error) {
	return nil, nil
}
func (noopStore) UpdateMemory(_ context.Context, _ string, _ store.MemoryEntry) error { return nil }
func (noopStore) ListChildConversations(_ context.Context, _ string) ([]store.Conversation, error) {
	return nil, nil
}
func (noopStore) SetConversationStatus(_ context.Context, _, _ string) error  { return nil }
func (noopStore) ListUserSkills(_ context.Context) ([]store.UserSkill, error) { return nil, nil }
func (noopStore) GetUserSkill(_ context.Context, _ string) (store.UserSkill, error) {
	return store.UserSkill{}, nil
}
func (noopStore) CreateUserSkill(_ context.Context, _ store.UserSkill) (store.UserSkill, error) {
	return store.UserSkill{}, nil
}
func (noopStore) UpdateUserSkill(_ context.Context, _ store.UserSkill) (store.UserSkill, error) {
	return store.UserSkill{}, nil
}
func (noopStore) DeleteUserSkill(_ context.Context, _ string) error { return nil }
func (noopStore) Close() error                                      { return nil }

// c.10: EventMemoryChanged via handleBusEvent returns a fetchMemory cmd that
// produces a memoryRefreshMsg (spec scenario TR-12).
func TestHandleBusEvent_MemoryChanged_ReturnsFetchCmd(t *testing.T) {
	m := newTestModel()
	m.screen = screenChat
	m.events = closedEventsChan()

	ev := notify.Event{
		Type: notify.EventMemoryChanged,
		Meta: map[string]string{"scope_id": "scope-abc"},
	}
	_, cmd := m.handleBusEvent(ev)

	// Execute the batch and collect all messages.
	msgs := collectMsgs(cmd)

	// A memoryRefreshMsg must appear among the batch messages.
	found := false
	for _, msg := range msgs {
		if _, ok := msg.(memoryRefreshMsg); ok {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("handleBusEvent(EventMemoryChanged): no memoryRefreshMsg in batch messages %T %v; "+
			"the EventMemoryChanged case may be missing from handleBusEvent", msgs, msgs)
	}
}

// c.11: memoryRefreshMsg through Update sets entries and prior model snapshot unchanged
// (spec scenario TR-13 "memoryRefreshMsg populates panel entries", COW).
func TestMemoryRefreshMsg_Update_SetsEntries(t *testing.T) {
	m := newTestModel()
	m.screen = screenChat

	// Capture original panel pointer before Update.
	origPanel, ok := m.rail.panels[panelMemoryPeek].(*memoryPeekPanel)
	if !ok {
		t.Fatal("panelMemoryPeek not *memoryPeekPanel in initial model")
	}
	origEntriesLen := len(origPanel.entries)

	msg := memoryRefreshMsg{entries: []store.MemoryEntry{
		{Title: "t1"},
		{Title: "t2"},
	}}

	result, _ := m.Update(msg)
	rm := result.(Model)

	// New model must have 2 entries.
	newPanel, ok := rm.rail.panels[panelMemoryPeek].(*memoryPeekPanel)
	if !ok {
		t.Fatal("panelMemoryPeek not *memoryPeekPanel after Update")
	}
	if len(newPanel.entries) != 2 {
		t.Errorf("new model memoryPeekPanel.entries len = %d, want 2", len(newPanel.entries))
	}

	// Prior model snapshot must remain unchanged (COW invariant).
	if len(origPanel.entries) != origEntriesLen {
		t.Errorf("ORIGINAL panel entries changed from %d to %d (COW violation)",
			origEntriesLen, len(origPanel.entries))
	}
}

// c.12: memoryRefreshMsg with nil entries clears prior entries (spec scenario TR-13).
func TestMemoryRefreshMsg_Update_ClearsPrior(t *testing.T) {
	m := newTestModel()
	m.screen = screenChat

	// Seed 3 entries into the panel via Update.
	seed := memoryRefreshMsg{entries: []store.MemoryEntry{
		{Title: "a"},
		{Title: "b"},
		{Title: "c"},
	}}
	m2, _ := m.Update(seed)
	m = m2.(Model)

	// Now send empty msg — must clear.
	clear := memoryRefreshMsg{entries: nil}
	result, _ := m.Update(clear)
	rm := result.(Model)

	newPanel, ok := rm.rail.panels[panelMemoryPeek].(*memoryPeekPanel)
	if !ok {
		t.Fatal("panelMemoryPeek not *memoryPeekPanel after clear Update")
	}
	if len(newPanel.entries) != 0 {
		t.Errorf("entries len = %d after nil msg, want 0", len(newPanel.entries))
	}
	got := newPanel.Render(32, 0)
	if got != "" {
		t.Errorf("Render after clear: got %q, want empty string", got)
	}
}

// ---------------------------------------------------------------------------
// tui-rail-height-clamp PR-a tests — RED phase (a.1–a.7)
// ---------------------------------------------------------------------------

// a.1: assignBudgets worked example h=12, naturals [9,8,7,5] → budgets [3,2,2,2]
// (spec scenario TR-HC-1 "Separator reservation — worked example at h=12").
// Design §5: n=4, avail=12-3=9; forward pass → [3,2,2,2], sum=9.
func TestAssignBudgets_WorkedExample_h12(t *testing.T) {
	avail := 9 // height=12, n=4 → avail=12-(4-1)=9

	budgets := assignBudgets([]int{9, 8, 7, 5}, avail)

	want := []int{3, 2, 2, 2}
	if len(budgets) != len(want) {
		t.Fatalf("assignBudgets: got %v (len %d), want %v (len %d)", budgets, len(budgets), want, len(want))
	}
	for i, g := range budgets {
		if g != want[i] {
			t.Errorf("budgets[%d] = %d, want %d (full budgets: %v)", i, g, want[i], budgets)
		}
	}
}

// a.2: assignBudgets surplus reflow — under-full panels donate to later ones.
// naturals=[3,2,10], avail=10 → [3,2,5] (panels 0+1 take only their natural).
func TestAssignBudgets_SurplusReflow(t *testing.T) {
	avail := 10

	budgets := assignBudgets([]int{3, 2, 10}, avail)

	if len(budgets) != 3 {
		t.Fatalf("assignBudgets: got len %d, want 3; budgets: %v", len(budgets), budgets)
	}
	if budgets[0] != 3 {
		t.Errorf("budgets[0] = %d, want 3 (panel 0 takes its natural 3)", budgets[0])
	}
	if budgets[1] != 2 {
		t.Errorf("budgets[1] = %d, want 2 (panel 1 takes its natural 2)", budgets[1])
	}
	if budgets[2] != 5 {
		t.Errorf("budgets[2] = %d, want 5 (all remaining rows flow to panel 2)", budgets[2])
	}
}

// a.3: assignBudgets invariant — sum(budgets) <= avail for all inputs.
func TestAssignBudgets_SumNeverExceedsAvail(t *testing.T) {
	tests := []struct {
		name     string
		naturals []int
		avail    int
	}{
		{"single panel", []int{20}, 15},
		{"single panel avail=0", []int{5}, 0},
		{"single panel n=1", []int{3}, 8},
		{"four panels equal", []int{9, 9, 9, 9}, 20},
		{"four panels surplus", []int{2, 2, 2, 2}, 20},
		{"four panels h12 worked example", []int{9, 8, 7, 5}, 9},
		{"two panels avail tight", []int{10, 10}, 3},
		{"avail zero 4 panels", []int{5, 5, 5, 5}, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			budgets := assignBudgets(tt.naturals, tt.avail)
			if len(budgets) != len(tt.naturals) {
				t.Fatalf("len(budgets)=%d, want %d", len(budgets), len(tt.naturals))
			}
			sum := 0
			for _, b := range budgets {
				sum += b
			}
			if sum > tt.avail {
				t.Errorf("sum(budgets)=%d > avail=%d; budgets=%v", sum, tt.avail, budgets)
			}
		})
	}
}

// a.4: rail.Render height guarantee for screenChat — lipgloss.Height(result) <= h.
func TestRailRender_HeightGuarantee_ChatScreen(t *testing.T) {
	prev := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() { lipgloss.SetColorProfile(prev) })

	s := newTuiStyles()
	r := newRail(s)

	// Populate all four screenChat panels with minimal data.
	r = copyRailWith(r, func(panels map[panelID]Panel) {
		// todolist: 6 items
		if p, ok := panels[panelTodolist].(*todolistPanel); ok {
			cp := *p
			items := make([]tool.TodoItem, 6)
			for i := range items {
				items[i] = tool.TodoItem{ID: fmt.Sprintf("%d", i), Content: fmt.Sprintf("item %d", i), Status: "pending"}
			}
			cp.setList(tool.TodoList{Items: items})
			panels[panelTodolist] = &cp
		}
		// context-meter: smart strategy
		if p, ok := panels[panelContextMeter].(*contextMeterPanel); ok {
			cp := *p
			cp.setLimit(128000)
			cp.accumulate(notify.Event{Type: notify.EventTokensUsage, SysToks: 1000, MsgToks: 2000, ToolToks: 500})
			panels[panelContextMeter] = &cp
		}
		// telemetry: some token + tool data
		if p, ok := panels[panelTelemetry].(*telemetryPanel); ok {
			cp := *p
			cp.accumulate(notify.Event{Type: notify.EventTokensUsage, TokenCount: 5000, CostUSD: 0.01})
			cp.accumulate(notify.Event{Type: notify.EventToolStart, ToolName: "bash"})
			cp.accumulate(notify.Event{Type: notify.EventToolStart, ToolName: "read_file"})
			panels[panelTelemetry] = &cp
		}
		// memory-peek: 2 entries
		if p, ok := panels[panelMemoryPeek].(*memoryPeekPanel); ok {
			cp := *p
			cp.setEntries([]store.MemoryEntry{{Title: "entry-1"}, {Title: "entry-2"}})
			panels[panelMemoryPeek] = &cp
		}
	})

	for _, h := range []int{8, 12, 24} {
		t.Run(fmt.Sprintf("h=%d", h), func(t *testing.T) {
			result := r.Render(screenChat, 32, h)
			got := lipgloss.Height(result)
			if got > h {
				t.Errorf("h=%d: lipgloss.Height(result)=%d > h; result:\n%s", h, got, result)
			}
		})
	}
}

// a.5: rail.Render height guarantee for screenDiff.
func TestRailRender_HeightGuarantee_DiffScreen(t *testing.T) {
	prev := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() { lipgloss.SetColorProfile(prev) })

	s := newTuiStyles()
	r := newRail(s)

	// Populate telemetry (the one panel screenDiff always has data for).
	r = copyRailWith(r, func(panels map[panelID]Panel) {
		if p, ok := panels[panelTelemetry].(*telemetryPanel); ok {
			cp := *p
			cp.accumulate(notify.Event{Type: notify.EventTokensUsage, TokenCount: 3000, CostUSD: 0.005})
			panels[panelTelemetry] = &cp
		}
	})

	for _, h := range []int{8, 12} {
		t.Run(fmt.Sprintf("h=%d", h), func(t *testing.T) {
			result := r.Render(screenDiff, 32, h)
			got := lipgloss.Height(result)
			if got > h {
				t.Errorf("h=%d: lipgloss.Height(result)=%d > h; result:\n%s", h, got, result)
			}
		})
	}
}

// a.6: Empty panel excluded from budget distribution (n=2 when B is empty).
func TestRailRender_EmptyPanel_ExcludedFromBudget(t *testing.T) {
	prev := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() { lipgloss.SetColorProfile(prev) })

	s := newTuiStyles()
	r := newRail(s)

	// Populate todolist (A) and memory-peek (C); leave context-meter (B) and telemetry empty.
	r = copyRailWith(r, func(panels map[panelID]Panel) {
		if p, ok := panels[panelTodolist].(*todolistPanel); ok {
			cp := *p
			cp.setList(tool.TodoList{Items: []tool.TodoItem{
				{ID: "1", Content: "task 1", Status: "pending"},
			}})
			panels[panelTodolist] = &cp
		}
		if p, ok := panels[panelMemoryPeek].(*memoryPeekPanel); ok {
			cp := *p
			cp.setEntries([]store.MemoryEntry{{Title: "mem entry"}})
			panels[panelMemoryPeek] = &cp
		}
	})

	h := 20
	result := r.Render(screenChat, 32, h)
	// With n=2 populated panels, avail = h - 1 = 19.
	// The result must not exceed h.
	got := lipgloss.Height(result)
	if got > h {
		t.Errorf("lipgloss.Height(result)=%d > h=%d; result:\n%s", got, h, result)
	}
	// Context-meter (B) must contribute no rows: check result doesn't have "context" header.
	if strings.Contains(result, "CONTEXT") {
		t.Errorf("empty context-meter must not appear in output; result:\n%s", result)
	}
}

// a.7: renderMoreRow — exact format "  +N more", dimLabel style, ANSI-truncated.
func TestRenderMoreRow_ExactFormat(t *testing.T) {
	prev := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() { lipgloss.SetColorProfile(prev) })

	s := newTuiStyles()
	got := renderMoreRow(6, 28, s)

	// Must contain the exact "+ more" text.
	if !strings.Contains(got, "+6 more") {
		t.Errorf("renderMoreRow(6, 28, s): expected '+6 more' in output, got: %q", got)
	}
	// Must contain the two leading spaces (before the "+").
	if !strings.Contains(got, "  +6 more") {
		t.Errorf("renderMoreRow(6, 28, s): expected '  +6 more' (two leading spaces), got: %q", got)
	}
}

// ---------------------------------------------------------------------------

// TestResumeListPanel_PrecomputedAgo verifies that after setSessions, the panel's
// ago slice is populated and that Render reads from it rather than calling
// relativeTime live. The test uses a fixed "ago" value injected via the
// sessionsLoadedMsg → setSessions path and checks the rendered output contains
// that exact string (not a freshly-computed one that might differ).
func TestResumeListPanel_PrecomputedAgo(t *testing.T) {
	s := newTuiStyles()
	p := newResumeListPanel(s)

	convs := []store.Conversation{
		{
			ID:        "abc12345",
			Status:    "active",
			UpdatedAt: time.Now().Add(-7 * time.Minute),
			Metadata:  map[string]string{"title": "Test conv"},
		},
	}
	p.setSessions(convs)

	// Verify ago slice is populated.
	if len(p.ago) != len(convs) {
		t.Fatalf("p.ago len = %d, want %d", len(p.ago), len(convs))
	}
	if p.ago[0] == "" {
		t.Fatal("p.ago[0] is empty after setSessions, want non-empty ago string")
	}

	// Capture the pre-computed ago value.
	precomputed := p.ago[0]

	// Render must contain the pre-computed string, not a re-computed one.
	out := p.Render(40, 20)
	if !strings.Contains(out, precomputed) {
		t.Errorf("Render output does not contain pre-computed ago %q\noutput:\n%s", precomputed, out)
	}

	// Override p.ago with a sentinel to confirm Render reads from the field.
	p.ago[0] = "SENTINEL"
	out2 := p.Render(40, 20)
	if !strings.Contains(out2, "SENTINEL") {
		t.Errorf("Render output does not contain overridden ago sentinel %q — Render is not reading p.ago\noutput:\n%s", "SENTINEL", out2)
	}
}
