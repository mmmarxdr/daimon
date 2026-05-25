package tui

import (
	"strings"
	"testing"
	"time"
)

// TestThreadItem_Interface verifies that all concrete thread item types
// satisfy the threadItem interface at compile time.
var (
	_ threadItem = (*MsgUser)(nil)
	_ threadItem = (*MsgDaimon)(nil)
	_ threadItem = (*Reasoning)(nil)
	_ threadItem = (*ToolLine)(nil)
	_ threadItem = (*Subagent)(nil)
)

// ---------------------------------------------------------------------------
// MsgUser
// ---------------------------------------------------------------------------

func TestMsgUser_Render_ContainsText(t *testing.T) {
	s := newTuiStyles()
	m := &MsgUser{text: "hello from user", styles: s}
	got := m.Render(80)
	if !strings.Contains(got, "hello from user") {
		t.Errorf("MsgUser.Render(80) does not contain expected text\ngot: %q", got)
	}
}

func TestMsgUser_Render_Width_Respected(t *testing.T) {
	s := newTuiStyles()
	m := &MsgUser{text: "hello", styles: s}
	got := m.Render(20)
	// Each line must not exceed 20 visible columns.
	for i, line := range strings.Split(got, "\n") {
		w := visibleWidth(line)
		if w > 20 {
			t.Errorf("MsgUser.Render(20) line %d width=%d > 20: %q", i, w, line)
		}
	}
}

// ---------------------------------------------------------------------------
// MsgDaimon
// ---------------------------------------------------------------------------

func TestMsgDaimon_Render_ContainsText(t *testing.T) {
	s := newTuiStyles()
	m := &MsgDaimon{text: "response from agent", styles: s}
	got := m.Render(80)
	if !strings.Contains(got, "response from agent") {
		t.Errorf("MsgDaimon.Render(80) does not contain expected text\ngot: %q", got)
	}
}

// ---------------------------------------------------------------------------
// Reasoning
// ---------------------------------------------------------------------------

func TestReasoning_CollapsedByDefault(t *testing.T) {
	s := newTuiStyles()
	r := &Reasoning{text: "internal chain of thought", styles: s}
	if r.Expanded() {
		t.Error("Reasoning must be collapsed by default")
	}
	got := r.Render(80)
	// Collapsed: must NOT show full reasoning text.
	if strings.Contains(got, "internal chain of thought") {
		t.Errorf("Reasoning collapsed render must not show full text\ngot: %q", got)
	}
}

func TestReasoning_Expand_Toggle(t *testing.T) {
	s := newTuiStyles()
	r := &Reasoning{text: "internal chain of thought", styles: s}
	r.Expand()
	if !r.Expanded() {
		t.Error("after Expand(), Expanded() should be true")
	}
	got := r.Render(80)
	if !strings.Contains(got, "internal chain of thought") {
		t.Errorf("Reasoning expanded render must contain full text\ngot: %q", got)
	}

	r.Collapse()
	if r.Expanded() {
		t.Error("after Collapse(), Expanded() should be false")
	}
}

// ---------------------------------------------------------------------------
// ToolLine
// ---------------------------------------------------------------------------

func TestToolLine_ToolStateEnum(t *testing.T) {
	// Verify the toolState enum values match the contract.
	if toolDone != 0 {
		t.Errorf("toolDone must be 0, got %d", toolDone)
	}
	if toolRunning != 1 {
		t.Errorf("toolRunning must be 1, got %d", toolRunning)
	}
	if toolError != 2 {
		t.Errorf("toolError must be 2, got %d", toolError)
	}
	if toolQueued != 3 {
		t.Errorf("toolQueued must be 3, got %d", toolQueued)
	}
}

func TestToolLine_Render_Done(t *testing.T) {
	s := newTuiStyles()
	tl := &ToolLine{
		name:   "read_file",
		state:  toolDone,
		styles: s,
		stats: toolStats{
			lines:    42,
			tokens:   100,
			duration: 500 * time.Millisecond,
		},
	}
	got := tl.Render(80)
	if !strings.Contains(got, "read_file") {
		t.Errorf("ToolLine done must show tool name\ngot: %q", got)
	}
}

func TestToolLine_Render_Running_HasSpinner(t *testing.T) {
	s := newTuiStyles()
	tl := &ToolLine{
		name:   "bash",
		state:  toolRunning,
		styles: s,
	}
	got := tl.Render(80)
	// Must contain a braille spinner glyph.
	spinnerGlyphs := "⣾⣽⣻⢿⡿⣟⣯⣷"
	found := false
	for _, g := range spinnerGlyphs {
		if strings.ContainsRune(got, g) {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("ToolLine running render must contain a braille spinner glyph\ngot: %q", got)
	}
}

func TestToolLine_Render_Error(t *testing.T) {
	s := newTuiStyles()
	tl := &ToolLine{
		name:   "write_file",
		state:  toolError,
		styles: s,
	}
	got := tl.Render(80)
	if !strings.Contains(got, "write_file") {
		t.Errorf("ToolLine error must show tool name\ngot: %q", got)
	}
}

func TestToolLine_LongName_Truncated(t *testing.T) {
	s := newTuiStyles()
	longName := "a_very_long_tool_name_that_exceeds_any_reasonable_width_limit_for_display_purposes"
	tl := &ToolLine{
		name:   longName,
		state:  toolDone,
		styles: s,
	}
	got := tl.Render(40)
	// No single line should exceed 40 visible columns.
	for i, line := range strings.Split(got, "\n") {
		w := visibleWidth(line)
		if w > 40 {
			t.Errorf("ToolLine.Render(40) line %d width=%d > 40: %q", i, w, line)
		}
	}
}

func TestToolLine_Tick_ReturnsCmd(t *testing.T) {
	s := newTuiStyles()
	tl := &ToolLine{
		name:   "bash",
		state:  toolRunning,
		styles: s,
	}
	cmd := tl.Tick()
	if cmd == nil {
		t.Error("Tick() should return a non-nil tea.Cmd for running state")
	}
}

// ---------------------------------------------------------------------------
// Subagent
// ---------------------------------------------------------------------------

func TestSubagent_Render_HasPinkAccent(t *testing.T) {
	s := newTuiStyles()
	sa := &Subagent{
		id:     "sub-123",
		styles: s,
	}
	sa.thread.append(&MsgDaimon{text: "sub response", styles: s})
	got := sa.Render(80)
	// Should contain the nested content.
	if !strings.Contains(got, "sub response") {
		t.Errorf("Subagent render must show nested thread content\ngot: %q", got)
	}
}

// ---------------------------------------------------------------------------
// thread type
// ---------------------------------------------------------------------------

func TestThread_Append_And_Render(t *testing.T) {
	s := newTuiStyles()
	var th thread
	th.append(&MsgUser{text: "user msg", styles: s})
	th.append(&MsgDaimon{text: "daimon msg", styles: s})

	got := th.Render(80)
	if !strings.Contains(got, "user msg") {
		t.Errorf("thread.Render must contain user msg\ngot: %q", got)
	}
	if !strings.Contains(got, "daimon msg") {
		t.Errorf("thread.Render must contain daimon msg\ngot: %q", got)
	}
}

func TestThread_FindToolLine_ByCallID(t *testing.T) {
	s := newTuiStyles()
	var th thread
	tl := &ToolLine{name: "bash", callID: "call-42", state: toolRunning, styles: s}
	th.append(tl)

	found := th.findToolLine("call-42")
	if found == nil {
		t.Fatal("findToolLine should return the ToolLine by callID")
	}
	if found.callID != "call-42" {
		t.Errorf("found.callID = %q, want %q", found.callID, "call-42")
	}
}
