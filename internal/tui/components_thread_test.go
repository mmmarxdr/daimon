package tui

import (
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
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

// TestMsgUser_Render_HeaderLine asserts the design speaker header:
// first line is "▌ <name> · <time>" and the body text appears on a LATER
// line (not the header), matching tui.jsx MsgUser (header row + indented body).
func TestMsgUser_Render_HeaderLine(t *testing.T) {
	s := newTuiStyles()
	m := &MsgUser{text: "hello there", name: "you", time: "14:32", styles: s}
	got := m.Render(80)
	header := strings.Split(got, "\n")[0]
	for _, want := range []string{glyphUser, "you", "·", "14:32"} {
		if !strings.Contains(header, want) {
			t.Errorf("MsgUser header missing %q\nheader: %q", want, header)
		}
	}
	if strings.Contains(header, "hello there") {
		t.Errorf("body text must not be on the header line: %q", header)
	}
	if !strings.Contains(got, "hello there") {
		t.Errorf("body text missing from render: %q", got)
	}
}

// TestMsgUser_Render_DefaultName verifies an empty name falls back to "you".
func TestMsgUser_Render_DefaultName(t *testing.T) {
	s := newTuiStyles()
	m := &MsgUser{text: "hi", styles: s}
	header := strings.Split(m.Render(80), "\n")[0]
	if !strings.Contains(header, "you") {
		t.Errorf("empty MsgUser name must default to 'you'\nheader: %q", header)
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

// TestMsgDaimon_Render_HeaderLine asserts the design speaker header:
// first line is "⫶ daimon speaks · <time>" and the body appears on a LATER line.
func TestMsgDaimon_Render_HeaderLine(t *testing.T) {
	s := newTuiStyles()
	m := &MsgDaimon{text: "the answer", time: "14:33", styles: s}
	got := m.Render(80)
	header := strings.Split(got, "\n")[0]
	for _, want := range []string{glyphDaimon, "daimon", "speaks", "14:33"} {
		if !strings.Contains(header, want) {
			t.Errorf("MsgDaimon header missing %q\nheader: %q", want, header)
		}
	}
	if strings.Contains(header, "the answer") {
		t.Errorf("body text must not be on the header line: %q", header)
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

// ---------------------------------------------------------------------------
// C1 — wrapLine must not corrupt UTF-8 or mid-ANSI-escape (RED → GREEN)
// ---------------------------------------------------------------------------

// TestWrapLine_MultibyteAndANSI verifies that wrapLine never cuts mid-rune
// (CJK, emoji) and never produces a line whose visible width exceeds maxWidth.
// Also verifies ANSI-styled strings are handled correctly.
func TestWrapLine_MultibyteAndANSI(t *testing.T) {
	cases := []struct {
		name     string
		input    string
		maxWidth int
	}{
		{
			name:     "CJK long string",
			input:    "日本語が長い文字列テスト",
			maxWidth: 8,
		},
		{
			name:     "emoji sequence",
			input:    "🎉🚀💡🔥🎯🏆✨🌟",
			maxWidth: 6,
		},
		{
			name:     "mixed ASCII and CJK",
			input:    "hello日本語world",
			maxWidth: 10,
		},
		{
			name:     "ANSI styled — lipgloss bold",
			input:    "\x1b[1msome bold text here and more\x1b[0m",
			maxWidth: 10,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			lines := wrapLine(tc.input, tc.maxWidth)
			if len(lines) == 0 {
				t.Fatal("wrapLine returned empty slice for non-empty input")
			}
			for i, line := range lines {
				// Every line must be valid UTF-8.
				if !utf8.ValidString(line) {
					t.Errorf("line %d is not valid UTF-8: %q", i, line)
				}
				// Every line must have visible width <= maxWidth.
				w := ansi.StringWidth(line)
				if w > tc.maxWidth {
					t.Errorf("line %d visual width %d > maxWidth %d: %q", i, w, tc.maxWidth, line)
				}
			}
		})
	}
}

// ---------------------------------------------------------------------------
// FIX 2 — wrapLine must not produce a spurious zero-width trailing segment
// ---------------------------------------------------------------------------

// TestWrapLine_ANSI_NoTrailingZeroWidthSegment verifies that wrapLine does not
// append a spurious zero-width trailing segment when ansi.Truncate leaves a
// residual ANSI reset sequence (\x1b[0m) as the remainder. All returned lines
// must have visible width > 0.
func TestWrapLine_ANSI_NoTrailingZeroWidthSegment(t *testing.T) {
	cases := []struct {
		name  string
		input string
		width int
	}{
		{
			name:  "bold styled wrapping string",
			input: "\x1b[1msome bold text that wraps at ten\x1b[0m",
			width: 10,
		},
		{
			name:  "red foreground styled",
			input: "\x1b[31mred text that is long enough to wrap here\x1b[0m",
			width: 12,
		},
		{
			name:  "nested styles",
			input: "\x1b[1;32mbright green bold text that should definitely wrap\x1b[0m",
			width: 15,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			lines := wrapLine(tc.input, tc.width)
			if len(lines) == 0 {
				t.Fatal("wrapLine returned empty slice for non-empty input")
			}
			for i, line := range lines {
				w := ansi.StringWidth(line)
				if w == 0 {
					t.Errorf("line %d has zero visible width (spurious trailing segment): %q", i, line)
				}
			}
		})
	}
}

// ---------------------------------------------------------------------------
// W3 — 'r' key must toggle the most-recent Reasoning (RED → GREEN)
// ---------------------------------------------------------------------------

// TestHandleChatKey_R_ExpandsReasoning verifies that pressing 'r' when focus
// is on the thread (focusMain) toggles the most-recent Reasoning on the
// RETURNED model (not via pointer aliasing).
func TestHandleChatKey_R_ExpandsReasoning(t *testing.T) {
	m := newTestModel()
	m.screen = screenChat
	m.focus = focusMain // FIX 3: 'r' only toggles when focus is on thread

	// Insert a collapsed Reasoning item.
	r := &Reasoning{text: "some reasoning", styles: m.styles}
	m.thread.append(r)

	// Press 'r'.
	m2, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
	got := m2.(Model)

	// Find the Reasoning in the returned model's thread.
	var foundReasoning *Reasoning
	for _, item := range got.thread.items {
		if rr, ok := item.(*Reasoning); ok {
			foundReasoning = rr
			break
		}
	}
	if foundReasoning == nil {
		t.Fatal("no Reasoning item found in returned model after 'r' key press")
	}
	if !foundReasoning.Expanded() {
		t.Error("Reasoning should be expanded after pressing 'r' with focusMain")
	}

	// Press 'r' again — should collapse.
	m3, _ := got.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
	got2 := m3.(Model)
	var foundReasoning2 *Reasoning
	for _, item := range got2.thread.items {
		if rr, ok := item.(*Reasoning); ok {
			foundReasoning2 = rr
			break
		}
	}
	if foundReasoning2 == nil {
		t.Fatal("no Reasoning item found in returned model after second 'r' key press")
	}
	if foundReasoning2.Expanded() {
		t.Error("Reasoning should be collapsed after pressing 'r' again (toggle)")
	}
}

// ---------------------------------------------------------------------------
// 1a.3 — MsgDaimon must render with ⫶ glyph, not δ
// ---------------------------------------------------------------------------

// TestMsgDaimon_Render_GlyphDaimon asserts that MsgDaimon uses the ⫶ glyph
// as the speaker prefix, not the legacy δ character.
func TestMsgDaimon_Render_GlyphDaimon(t *testing.T) {
	s := newTuiStyles()
	m := &MsgDaimon{text: "hello from daimon", styles: s}
	got := m.Render(80)

	if !strings.ContainsRune(got, '⫶') {
		t.Errorf("MsgDaimon.Render must contain ⫶ (U+2AF6), got: %q", got)
	}
	if strings.ContainsRune(got, 'δ') {
		t.Errorf("MsgDaimon.Render must NOT contain δ (legacy glyph), got: %q", got)
	}
}

// ---------------------------------------------------------------------------
// 1a.4 — MsgUser must render with ▌ glyph, not "you  "
// ---------------------------------------------------------------------------

// TestMsgUser_Render_GlyphUser asserts that MsgUser uses ▌ as the user-line
// prefix, not the legacy "you  " string.
func TestMsgUser_Render_GlyphUser(t *testing.T) {
	s := newTuiStyles()
	m := &MsgUser{text: "hello from user", styles: s}
	got := m.Render(80)

	if !strings.ContainsRune(got, '▌') {
		t.Errorf("MsgUser.Render must contain ▌ (U+258C), got: %q", got)
	}
	if strings.Contains(got, "you  ") {
		t.Errorf("MsgUser.Render must NOT contain legacy 'you  ' prefix, got: %q", got)
	}
}

// ---------------------------------------------------------------------------
// Inc.2 slice 3 — ToolLine argument column + honest affordances
// ---------------------------------------------------------------------------

// TestToolLine_Render_NameAndInput verifies the design column split: a bold
// tool name followed by a separate input (argument) column, name first.
func TestToolLine_Render_NameAndInput(t *testing.T) {
	s := newTuiStyles()
	tl := &ToolLine{
		name:   "read_file",
		input:  "/var/log/payments/2026-04-18.log",
		state:  toolDone,
		styles: s,
	}
	got := tl.Render(90)
	if !strings.Contains(got, "read_file") {
		t.Errorf("ToolLine must show the tool name\ngot: %q", got)
	}
	if !strings.Contains(got, "/var/log/payments/2026-04-18.log") {
		t.Errorf("ToolLine must show the input argument\ngot: %q", got)
	}
	if strings.Index(got, "read_file") > strings.Index(got, "/var/log") {
		t.Errorf("name must precede the input column\ngot: %q", got)
	}
}

// TestToolLine_Render_NoViewAffordance verifies the misleading name-truncation
// "▸ view" hint is gone: with no expandable tool output wired, no view
// affordance is shown regardless of name/input length.
func TestToolLine_Render_NoViewAffordance(t *testing.T) {
	s := newTuiStyles()
	tl := &ToolLine{
		name:   "a_very_long_tool_name_that_exceeds_the_width",
		input:  "some/very/long/argument/path/that/keeps/going/and/going",
		state:  toolDone,
		styles: s,
	}
	if got := tl.Render(40); strings.Contains(got, "view") {
		t.Errorf("ToolLine must not show a view affordance without output\ngot: %q", got)
	}
}

// TestToolInputSummary verifies raw JSON tool input is reduced to a clean
// argument (salient key first, then single-key value, then raw passthrough).
func TestToolInputSummary(t *testing.T) {
	cases := []struct{ in, want string }{
		{``, ``},
		{`{"path":"/var/log/x.log"}`, `/var/log/x.log`},
		{`{"pattern":"webhook timeout"}`, `webhook timeout`},
		{`{"command":"bun test"}`, `bun test`},
		{`{"foo":"bar"}`, `bar`},
		{`{"a":"1","b":"2"}`, `a=1 b=2`},
		{`not json`, `not json`},
	}
	for _, c := range cases {
		if got := toolInputSummary(c.in); got != c.want {
			t.Errorf("toolInputSummary(%q)=%q want %q", c.in, got, c.want)
		}
	}
}

// TestToolLine_Render_InputTruncated_WidthRespected verifies a long input is
// truncated so the row never exceeds the available width.
func TestToolLine_Render_InputTruncated_WidthRespected(t *testing.T) {
	s := newTuiStyles()
	tl := &ToolLine{
		name:   "grep",
		input:  strings.Repeat("x", 200),
		state:  toolDone,
		styles: s,
		stats:  toolStats{duration: 89 * time.Millisecond},
	}
	got := tl.Render(50)
	for i, line := range strings.Split(got, "\n") {
		if w := visibleWidth(line); w > 50 {
			t.Errorf("ToolLine.Render(50) line %d width=%d > 50: %q", i, w, line)
		}
	}
}

// ---------------------------------------------------------------------------
// FIX 3 — 'r' key must not steal from input bar when focusEditor is active
// ---------------------------------------------------------------------------

// TestHandleChatKey_R_FocusEditor_GoesToInput verifies that pressing 'r' when
// focus is on the editor (focusEditor) appends 'r' to the input bar and does
// NOT toggle any Reasoning item. After Esc (→ focusMain), 'r' DOES toggle.
func TestHandleChatKey_R_FocusEditor_GoesToInput(t *testing.T) {
	m := newTestModel()
	m.screen = screenChat
	m.focus = focusEditor
	// newTestModel already initializes input via newInputBar(); focus is set above.

	// Insert a collapsed Reasoning item so we can assert it is NOT toggled.
	r := &Reasoning{text: "some reasoning", styles: m.styles}
	m.thread.append(r)

	// Press 'r' — must NOT toggle reasoning.
	m2, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
	got := m2.(Model)

	// The Reasoning in the returned model must still be collapsed.
	var foundReasoning *Reasoning
	for _, item := range got.thread.items {
		if rr, ok := item.(*Reasoning); ok {
			foundReasoning = rr
			break
		}
	}
	if foundReasoning == nil {
		t.Fatal("Reasoning item missing from returned model")
	}
	if foundReasoning.Expanded() {
		t.Error("FIX 3: pressing 'r' with focusEditor must NOT expand Reasoning (input bar owns 'r')")
	}

	// The input bar must contain 'r'.
	if got.input.Value() != "r" {
		t.Errorf("FIX 3: pressing 'r' with focusEditor must append 'r' to input, got %q", got.input.Value())
	}

	// Now press Esc to switch to focusMain.
	m3, _ := got.Update(tea.KeyMsg{Type: tea.KeyEscape})
	got3 := m3.(Model)
	if got3.focus != focusMain {
		t.Errorf("Esc must switch focus to focusMain, got %v", got3.focus)
	}

	// Now 'r' must toggle reasoning.
	m4, _ := got3.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
	got4 := m4.(Model)
	var foundReasoning4 *Reasoning
	for _, item := range got4.thread.items {
		if rr, ok := item.(*Reasoning); ok {
			foundReasoning4 = rr
			break
		}
	}
	if foundReasoning4 == nil {
		t.Fatal("Reasoning item missing from model after Esc + 'r'")
	}
	if !foundReasoning4.Expanded() {
		t.Error("FIX 3: pressing 'r' with focusMain must expand Reasoning")
	}
}
