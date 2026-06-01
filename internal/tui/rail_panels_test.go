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
