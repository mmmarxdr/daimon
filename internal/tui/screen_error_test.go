package tui

// screen_error_test.go — STRICT TDD: tests written RED-first.
//
// Tests cover:
//   1. handleBusEvent with EventToolEnd + Meta["denied"]="true" on screenChat
//      → returned model: m.screen==screenError, m.errorToolName/errorReason set,
//        m.recentDenials contains the entry, m.footer.screen==screenError.
//   2. handleBusEvent with EventToolEnd + IsError but NO denied flag
//      → m.screen STAYS screenChat (not screenError); ToolLine in error state.
//   3. updateError esc → m.screen == m.prevScreen.
//   4. activePolicyPanel renders the mode; empty → "".
//   5. recentDenialsPanel: setDenials → renders tools; empty → "".
//   6. renderError shows the offending tool + reason.

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"daimon/internal/notify"
)

// ---------------------------------------------------------------------------
// Test 1: denial event → switch to screenError
// ---------------------------------------------------------------------------

// TestHandleBusEvent_DenialToolEnd_SwitchesToScreenError verifies that a
// EventToolEnd with Meta["denied"]=="true" transitions the model to screenError,
// captures errorToolName / errorReason, appends to recentDenials, and updates
// the footer.
func TestHandleBusEvent_DenialToolEnd_SwitchesToScreenError(t *testing.T) {
	m := newTestModel()
	m.screen = screenChat
	m.focus = focusEditor

	// Insert a ToolLine in toolRunning state so the EventToolEnd has something to
	// transition (findToolLineIdx looks up by callID).
	tl := &ToolLine{callID: "call-denial-1", name: "Bash", state: toolRunning, styles: m.styles}
	m.thread.append(tl)

	ev := notify.Event{
		Type:       notify.EventToolEnd,
		ToolCallID: "call-denial-1",
		ToolName:   "Bash",
		IsError:    true,
		Error:      "tool 'Bash' not allowed in mode 'plan'",
		Meta:       map[string]string{"denied": "true"},
	}

	next, _ := m.handleBusEvent(ev)
	nm := next.(Model)

	if nm.screen != screenError {
		t.Errorf("screen = %v, want screenError after denial event", nm.screen)
	}
	if nm.errorToolName != "Bash" {
		t.Errorf("errorToolName = %q, want %q", nm.errorToolName, "Bash")
	}
	if nm.errorReason != "tool 'Bash' not allowed in mode 'plan'" {
		t.Errorf("errorReason = %q, want the denial reason", nm.errorReason)
	}
	if len(nm.recentDenials) == 0 {
		t.Fatal("recentDenials is empty, want at least one entry")
	}
	if nm.recentDenials[0].tool != "Bash" {
		t.Errorf("recentDenials[0].tool = %q, want \"Bash\"", nm.recentDenials[0].tool)
	}
	if nm.footer.screen != screenError {
		t.Errorf("footer.screen = %v, want screenError", nm.footer.screen)
	}
}

// ---------------------------------------------------------------------------
// Test 2: runtime error (no denied flag) → stays on screenChat
// ---------------------------------------------------------------------------

// TestHandleBusEvent_RuntimeErrorToolEnd_StaysOnScreenChat verifies that a
// EventToolEnd with IsError==true but NO denied flag does NOT switch the screen.
// The ToolLine transitions to toolError state (existing behavior preserved).
func TestHandleBusEvent_RuntimeErrorToolEnd_StaysOnScreenChat(t *testing.T) {
	m := newTestModel()
	m.screen = screenChat
	m.focus = focusEditor

	tl := &ToolLine{callID: "call-runtime-1", name: "shell_exec", state: toolRunning, styles: m.styles}
	m.thread.append(tl)

	ev := notify.Event{
		Type:       notify.EventToolEnd,
		ToolCallID: "call-runtime-1",
		ToolName:   "shell_exec",
		IsError:    true,
		Error:      "runtime failure: exit status 1",
		// No Meta["denied"] key.
	}

	next, _ := m.handleBusEvent(ev)
	nm := next.(Model)

	if nm.screen != screenChat {
		t.Errorf("screen = %v, want screenChat for non-denial runtime error", nm.screen)
	}
	// The ToolLine must be in toolError state.
	idx := nm.thread.findToolLineIdx("call-runtime-1")
	if idx < 0 {
		t.Fatal("ToolLine call-runtime-1 not found in thread after EventToolEnd")
	}
	foundTL, ok := nm.thread.items[idx].(*ToolLine)
	if !ok {
		t.Fatal("thread item at found index is not *ToolLine")
	}
	if foundTL.state != toolError {
		t.Errorf("ToolLine state = %v, want toolError for runtime error", foundTL.state)
	}
}

// ---------------------------------------------------------------------------
// Test 3: updateError esc → returns to prevScreen
// ---------------------------------------------------------------------------

// TestUpdateError_Esc_ReturnsToPrevScreen verifies that pressing Esc on the
// error screen transitions back to the previous screen.
func TestUpdateError_Esc_ReturnsToPrevScreen(t *testing.T) {
	m := newTestModel()
	m.screen = screenError
	m.prevScreen = screenChat
	m.footer = footerHints{screen: screenError}

	msg := tea.KeyMsg{Type: tea.KeyEsc}
	next, _ := m.updateError(msg)
	nm := next.(Model)

	if nm.screen != screenChat {
		t.Errorf("screen = %v, want screenChat after esc on screenError", nm.screen)
	}
	if nm.footer.screen != screenChat {
		t.Errorf("footer.screen = %v, want screenChat after esc on screenError", nm.footer.screen)
	}
}

// ---------------------------------------------------------------------------
// Test 4: activePolicyPanel renders the mode; empty → ""
// ---------------------------------------------------------------------------

func TestActivePolicyPanel_RenderWithMode(t *testing.T) {
	s := newTuiStyles()
	p := newActivePolicyPanel(s, "build")
	got := p.Render(40, 10)
	if got == "" {
		t.Error("activePolicyPanel.Render() = \"\", want non-empty when mode is set")
	}
	if !strings.Contains(got, "build") {
		t.Errorf("activePolicyPanel.Render() = %q, want to contain \"build\"", got)
	}
}

func TestActivePolicyPanel_RenderEmptyMode(t *testing.T) {
	s := newTuiStyles()
	p := newActivePolicyPanel(s, "")
	got := p.Render(40, 10)
	if got != "" {
		t.Errorf("activePolicyPanel.Render() = %q, want \"\" when mode is empty", got)
	}
}

// ---------------------------------------------------------------------------
// Test 5: recentDenialsPanel setDenials → renders tools; empty → ""
// ---------------------------------------------------------------------------

func TestRecentDenialsPanel_Empty(t *testing.T) {
	s := newTuiStyles()
	p := newRecentDenialsPanel(s)
	got := p.Render(40, 10)
	if got != "" {
		t.Errorf("recentDenialsPanel.Render() = %q, want \"\" when empty", got)
	}
}

func TestRecentDenialsPanel_SetDenialsRenders(t *testing.T) {
	s := newTuiStyles()
	p := newRecentDenialsPanel(s)
	p.setDenials([]denialEntry{
		{tool: "Bash", reason: "not allowed in plan mode"},
		{tool: "write_file", reason: "blocked by policy"},
	})
	got := p.Render(40, 10)
	if got == "" {
		t.Error("recentDenialsPanel.Render() = \"\", want non-empty after setDenials")
	}
	if !strings.Contains(got, "Bash") {
		t.Errorf("recentDenialsPanel.Render() does not contain \"Bash\", got: %q", got)
	}
	if !strings.Contains(got, "write_file") {
		t.Errorf("recentDenialsPanel.Render() does not contain \"write_file\", got: %q", got)
	}
}

// ---------------------------------------------------------------------------
// Test 6: renderError shows the offending tool + reason
// ---------------------------------------------------------------------------

func TestRenderError_ShowsToolAndReason(t *testing.T) {
	m := newTestModel()
	m.width = 80
	m.height = 24
	m.screen = screenError
	m.errorToolName = "Bash"
	m.errorReason = "tool 'Bash' not allowed in mode 'plan'"

	got := renderError(m, 60, 20)
	if got == "" {
		t.Error("renderError() = \"\", want non-empty when errorToolName is set")
	}
	if !strings.Contains(got, "Bash") {
		t.Errorf("renderError() does not contain \"Bash\", got: %q", got)
	}
	if !strings.Contains(got, "not allowed in mode") {
		t.Errorf("renderError() does not contain the denial reason, got: %q", got)
	}
}

func TestRenderError_EmptyToolName(t *testing.T) {
	m := newTestModel()
	m.width = 80
	m.height = 24
	m.screen = screenError
	m.errorToolName = ""
	m.errorReason = ""

	// Should not panic; should return neutral placeholder.
	got := renderError(m, 60, 20)
	_ = got // no assertion on content; just confirm no panic
}

// ---------------------------------------------------------------------------
// Test 7: recentDenials copy-on-write — append must not alias prior model
// ---------------------------------------------------------------------------

func TestHandleBusEvent_DenialToolEnd_CopyOnWrite(t *testing.T) {
	m := newTestModel()
	m.screen = screenChat

	tl1 := &ToolLine{callID: "call-cow-1", name: "Bash", state: toolRunning, styles: m.styles}
	m.thread.append(tl1)

	ev1 := notify.Event{
		Type:       notify.EventToolEnd,
		ToolCallID: "call-cow-1",
		ToolName:   "Bash",
		IsError:    true,
		Error:      "denied reason 1",
		Meta:       map[string]string{"denied": "true"},
	}
	next1, _ := m.handleBusEvent(ev1)
	nm1 := next1.(Model)

	// Now feed a second denial without re-inserting a ToolLine (tool already done).
	nm1.screen = screenChat
	tl2 := &ToolLine{callID: "call-cow-2", name: "write_file", state: toolRunning, styles: m.styles}
	nm1.thread.append(tl2)

	ev2 := notify.Event{
		Type:       notify.EventToolEnd,
		ToolCallID: "call-cow-2",
		ToolName:   "write_file",
		IsError:    true,
		Error:      "denied reason 2",
		Meta:       map[string]string{"denied": "true"},
	}
	next2, _ := nm1.handleBusEvent(ev2)
	nm2 := next2.(Model)

	// nm1 should still have exactly 1 denial; nm2 should have 2.
	if len(nm1.recentDenials) != 1 {
		t.Errorf("nm1.recentDenials len = %d, want 1 (copy-on-write violated)", len(nm1.recentDenials))
	}
	if len(nm2.recentDenials) != 2 {
		t.Errorf("nm2.recentDenials len = %d, want 2", len(nm2.recentDenials))
	}
}
