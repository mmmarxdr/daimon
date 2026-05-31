package tui

// chat_design_test.go — INCREMENT 1 strict-TDD tests for chat screen design.
//
// Tasks covered:
//   A — Rail panels rendered as bordered boxes
//   B — Footer hints: top rule + right tagline + design hint set
//   C — Input bar: no double prompt, second row with chips + mode pill
//   D — TopBar: │ separators + colored segments + bottom rule
//   E — styles.go: tagline, modePill, ink style slots

import (
	"io"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"

	"daimon/internal/tool"
)

// ─── TASK A: Rail panels rendered as BORDERED BOXES ──────────────────────────

// TestRail_Panels_HaveBorderChars asserts that when panels have data, their
// output contains NormalBorder corner characters (┌ and └), meaning the panel
// content is wrapped in a bordered box.
func TestRail_Panels_HaveBorderChars(t *testing.T) {
	s := newTuiStyles()

	// telemetry panel with data
	tp := newTelemetryPanel(s)
	tp.hasData = true
	tp.totalIn = 1000
	tp.totalCost = 0.001
	got := tp.Render(32, 20)
	if got == "" {
		t.Fatal("telemetryPanel.Render with data: got empty string")
	}
	if !strings.Contains(got, "┌") {
		t.Errorf("telemetryPanel.Render: missing NormalBorder top-left corner '┌'\ngot:\n%s", got)
	}
	if !strings.Contains(got, "└") {
		t.Errorf("telemetryPanel.Render: missing NormalBorder bottom-left corner '└'\ngot:\n%s", got)
	}
}

// TestRail_TodolistPanel_HasBorderChars asserts bordered box rendering for todolist.
func TestRail_TodolistPanel_HasBorderChars(t *testing.T) {
	s := newTuiStyles()
	p := newTodolistPanel(s)
	p.list.Items = []tool.TodoItem{{ID: "1", Content: "do something", Status: "pending"}}
	got := p.Render(32, 20)
	if got == "" {
		t.Fatal("todolistPanel.Render with data: got empty string")
	}
	if !strings.Contains(got, "┌") {
		t.Errorf("todolistPanel.Render: missing NormalBorder corner '┌'\ngot:\n%s", got)
	}
}

// TestRail_ContextMeterPanel_HasBorderChars asserts bordered box for context meter.
func TestRail_ContextMeterPanel_HasBorderChars(t *testing.T) {
	s := newTuiStyles()
	p := newContextMeterPanel(s)
	p.tokenUsed = 5000
	p.hasData = true
	got := p.Render(32, 20)
	if got == "" {
		t.Fatal("contextMeterPanel.Render with data: got empty string")
	}
	if !strings.Contains(got, "┌") {
		t.Errorf("contextMeterPanel.Render: missing NormalBorder corner '┌'\ngot:\n%s", got)
	}
}

// TestRail_Panels_WithinRailWidth asserts that each panel's first line visible
// width does not exceed railWidth.
func TestRail_Panels_WithinRailWidth(t *testing.T) {
	s := newTuiStyles()

	tp := newTelemetryPanel(s)
	tp.hasData = true
	tp.totalIn = 12345

	got := tp.Render(railWidth, 20)
	for _, line := range strings.Split(got, "\n") {
		w := ansi.StringWidth(line)
		if w > railWidth {
			t.Errorf("telemetryPanel.Render(railWidth=%d): line visible width %d exceeds railWidth\nline: %q", railWidth, w, line)
		}
	}
}

// TestRail_PanelHeaderWithBadge_Format asserts the panelHeaderWithBadge helper
// produces a string containing the title (uppercased) and the badge text.
func TestRail_PanelHeaderWithBadge_Format(t *testing.T) {
	s := newTuiStyles()
	got := s.panelHeaderWithBadge("telemetry", "live")
	stripped := ansi.Strip(got)
	if !strings.Contains(stripped, "TELEMETRY") {
		t.Errorf("panelHeaderWithBadge: stripped output %q missing 'TELEMETRY'", stripped)
	}
	if !strings.Contains(stripped, "live") {
		t.Errorf("panelHeaderWithBadge: stripped output %q missing badge 'live'", stripped)
	}
}

// ─── TASK B: Footer hints ────────────────────────────────────────────────────

// TestFooter_ChatScreen_ContainsDesignHints asserts the chat-screen footer
// matches the canonical design source (tui-screens-a.jsx:302-308).
// PR 1c updated the chat footer from old wired-only hints to the full design set.
func TestFooter_ChatScreen_ContainsDesignHints(t *testing.T) {
	s := newTuiStyles()
	fh := footerHints{}
	fh.SetScreen(screenChat)
	// Use width=160 so all 5 hints fit without truncation.
	rendered := fh.Render(160, s)

	// Design source: tui-screens-a.jsx:302-308 outer TUIFooter.
	tokens := []string{"/commands", "interrupt", "retry turn", "edit last", "save session"}
	for _, tok := range tokens {
		if !strings.Contains(rendered, tok) {
			t.Errorf("footerHints.Render(screenChat): missing hint token %q\nrendered: %q", tok, rendered)
		}
	}
}

// TestFooter_AllScreens_ContainTopRule asserts that every screen's footer
// contains a horizontal rule line (─ character).
func TestFooter_AllScreens_ContainTopRule(t *testing.T) {
	s := newTuiStyles()
	screens := []screenState{
		screenWelcome, screenChat, screenDiff, screenSlash,
		screenTools, screenSessions, screenError,
	}
	for _, sc := range screens {
		fh := footerHints{}
		fh.SetScreen(sc)
		rendered := fh.Render(80, s)
		if !strings.Contains(rendered, "─") {
			t.Errorf("footerHints.Render(screen=%v): missing top-rule character '─'\nrendered: %q", sc, rendered)
		}
	}
}

// TestFooter_AllScreens_ContainTagline asserts that every screen's footer
// contains the "daimon listens." tagline.
func TestFooter_AllScreens_ContainTagline(t *testing.T) {
	s := newTuiStyles()
	screens := []screenState{
		screenWelcome, screenChat, screenDiff, screenSlash,
		screenTools, screenSessions, screenError,
	}
	for _, sc := range screens {
		fh := footerHints{}
		fh.SetScreen(sc)
		rendered := fh.Render(120, s)
		if !strings.Contains(rendered, "daimon listens.") {
			t.Errorf("footerHints.Render(screen=%v): missing tagline 'daimon listens.'\nrendered: %q", sc, rendered)
		}
	}
}

// ─── TASK C: Input bar ───────────────────────────────────────────────────────

// TestInputBar_NoDoublePrompt asserts the input bar does NOT contain "> >"
// (the double-prompt caused by having both ti.Prompt and the sentinel).
func TestInputBar_NoDoublePrompt(t *testing.T) {
	s := newTuiStyles()
	ib := newInputBar()
	rendered := ib.Render(80, s)
	if strings.Contains(rendered, "> >") || strings.Contains(rendered, ">>") {
		t.Errorf("inputBar.Render: contains double prompt '>>':\n%s", rendered)
	}
	// The sentinel must still appear exactly once (styled › symbol).
	stripped := ansi.Strip(rendered)
	count := strings.Count(stripped, ">")
	if count > 1 {
		t.Errorf("inputBar.Render stripped output contains %d '>' chars, want at most 1\nstripped: %q", count, stripped)
	}
}

// TestInputBar_ContainsModeChip asserts the input second row contains the mode chip.
// Phase A: "⇥ mode" replaces the old "⇥ /commands" chip; commands are accessed via "/".
func TestInputBar_ContainsModeChip(t *testing.T) {
	s := newTuiStyles()
	ib := newInputBar()
	rendered := ib.Render(80, s)
	// The mode chip "⇥ mode" must be present.
	if !strings.Contains(rendered, "mode") {
		t.Errorf("inputBar.Render: missing 'mode' chip in second row\nrendered: %q", rendered)
	}
}

// TestInputBar_ContainsModePill asserts the input second row contains the mode pill text.
func TestInputBar_ContainsModePill(t *testing.T) {
	s := newTuiStyles()
	ib := newInputBar()
	rendered := ib.Render(80, s)
	// The mode pill must contain "BUILD MODE" or "MODE" as the structural placeholder.
	if !strings.Contains(rendered, "MODE") {
		t.Errorf("inputBar.Render: missing mode pill 'MODE' in second row\nrendered: %q", rendered)
	}
}

// TestInputBar_PlaceholderText asserts the updated placeholder text.
func TestInputBar_PlaceholderText(t *testing.T) {
	s := newTuiStyles()
	ib := newInputBar()
	// Placeholder is rendered when input is empty+focused.
	rendered := ib.Render(80, s)
	if !strings.Contains(rendered, "follow-up") {
		t.Errorf("inputBar.Render: placeholder does not contain 'follow-up'\nrendered: %q", rendered)
	}
}

// ─── TASK D: TopBar ──────────────────────────────────────────────────────────

// TestTopBar_ContainsPipeSeparators asserts the topbar uses │ separators.
func TestTopBar_ContainsPipeSeparators(t *testing.T) {
	s := newTuiStyles()
	tb := topBar{}
	tb.SetData("⫶", "~/projects/daimon", "main", "claude-3-5", "build", "$0.01", "ready")
	rendered := tb.Render(120, s)
	if !strings.Contains(rendered, "│") {
		t.Errorf("topBar.Render: missing '│' pipe separator\nrendered: %q", rendered)
	}
}

// TestTopBar_ContainsSegments asserts the topbar contains all design segments.
func TestTopBar_ContainsSegments(t *testing.T) {
	s := newTuiStyles()
	tb := topBar{}
	tb.SetData("⫶", "~/projects/daimon", "main", "claude-3-5", "build", "$0.01", "ready")
	rendered := tb.Render(120, s)

	tokens := []string{"⫶", "daimon", "main", "claude-3-5", "build", "$0.01", "ready"}
	for _, tok := range tokens {
		if !strings.Contains(rendered, tok) {
			t.Errorf("topBar.Render: missing segment %q\nrendered: %q", tok, rendered)
		}
	}
}

// TestTopBar_ContainsBottomRule asserts the topbar output contains a
// horizontal rule line (─) as its bottom border.
func TestTopBar_ContainsBottomRule(t *testing.T) {
	s := newTuiStyles()
	tb := topBar{}
	tb.SetData("⫶", "~/projects/daimon", "main", "claude-3-5", "build", "$0.01", "ready")
	rendered := tb.Render(80, s)
	if !strings.Contains(rendered, "─") {
		t.Errorf("topBar.Render: missing bottom-rule character '─'\nrendered: %q", rendered)
	}
}

// TestTopBar_BranchOmittedWhenEmpty asserts the topbar omits the branch segment
// gracefully when branch is empty.
func TestTopBar_BranchOmittedWhenEmpty(t *testing.T) {
	s := newTuiStyles()
	tb := topBar{}
	tb.SetData("⫶ daimon", "~/projects/daimon", "", "claude-3-5", "build", "$0.01", "ready")
	rendered := tb.Render(80, s)
	// Should not panic and should still contain other slots.
	if !strings.Contains(rendered, "daimon") {
		t.Errorf("topBar.Render with empty branch: missing 'daimon'\nrendered: %q", rendered)
	}
}

// ─── TASK E: styles.go slots ─────────────────────────────────────────────────

// TestTuiStyles_TaglineSlot asserts the tagline style slot exists, is italic,
// and uses the inkFaint color.
func TestTuiStyles_TaglineSlot(t *testing.T) {
	r := lipgloss.NewRenderer(io.Discard)
	r.SetColorProfile(termenv.TrueColor)

	s := newTuiStyles()

	// tagline must render non-empty.
	if s.tagline.Render("x") == "" {
		t.Fatal("tuiStyles.tagline renders to empty string — slot may be uninitialized")
	}

	// tagline must be italic.
	if !s.tagline.GetItalic() {
		t.Error("tuiStyles.tagline: expected italic=true")
	}

	// tagline foreground must be inkFaint (#4a4438 → R=73,G=68,B=56 via termenv float truncation).
	// Note: 0x4a/0xff * 255 = 73.999… → uint8 truncation gives 73, not 74.
	fg := s.tagline.GetForeground()
	if _, isNoColor := fg.(lipgloss.NoColor); isNoColor {
		t.Fatal("tuiStyles.tagline: foreground not set")
	}
	rendered := r.NewStyle().Foreground(fg).Render("x")
	const wantANSI = "38;2;73;68;56"
	if !strings.Contains(rendered, wantANSI) {
		t.Errorf("tuiStyles.tagline: foreground ANSI sequence %q does not contain expected inkFaint sequence %q", rendered, wantANSI)
	}
}

// TestTuiStyles_ModePillSlot asserts the modePill style slot exists and uses
// the amber color.
func TestTuiStyles_ModePillSlot(t *testing.T) {
	r := lipgloss.NewRenderer(io.Discard)
	r.SetColorProfile(termenv.TrueColor)

	s := newTuiStyles()

	// modePill must render non-empty.
	if s.modePill.Render("x") == "" {
		t.Fatal("tuiStyles.modePill renders to empty string — slot may be uninitialized")
	}

	// modePill foreground must be amber (#e3b67a → R=227,G=182,B=122 → 38;2;227;182;121).
	// Note: 0x7a → uint8(121.999…) = 121 via Go truncation.
	fg := s.modePill.GetForeground()
	if _, isNoColor := fg.(lipgloss.NoColor); isNoColor {
		t.Fatal("tuiStyles.modePill: foreground not set")
	}
	rendered := r.NewStyle().Foreground(fg).Render("x")
	const wantANSI = "38;2;227;182;121"
	if !strings.Contains(rendered, wantANSI) {
		t.Errorf("tuiStyles.modePill: foreground ANSI sequence %q does not contain expected amber sequence %q", rendered, wantANSI)
	}
}

// TestTuiStyles_InkSlot asserts that an ink (primary text) style slot exists.
func TestTuiStyles_InkSlot(t *testing.T) {
	r := lipgloss.NewRenderer(io.Discard)
	r.SetColorProfile(termenv.TrueColor)

	s := newTuiStyles()

	if s.ink.Render("x") == "" {
		t.Fatal("tuiStyles.ink renders to empty string — slot may be uninitialized")
	}

	// ink foreground must be colorInk (#eae5d8 → R=234,G=229,B=216 → 38;2;234;229;216).
	fg := s.ink.GetForeground()
	if _, isNoColor := fg.(lipgloss.NoColor); isNoColor {
		t.Fatal("tuiStyles.ink: foreground not set")
	}
	rendered := r.NewStyle().Foreground(fg).Render("x")
	const wantANSI = "38;2;234;229;216"
	if !strings.Contains(rendered, wantANSI) {
		t.Errorf("tuiStyles.ink: foreground ANSI %q missing expected sequence %q", rendered, wantANSI)
	}
}

// TestTuiStyles_InkFaintSlot asserts that the inkFaint style slot exists with
// the correct color.
func TestTuiStyles_InkFaintSlot(t *testing.T) {
	r := lipgloss.NewRenderer(io.Discard)
	r.SetColorProfile(termenv.TrueColor)

	s := newTuiStyles()

	if s.inkFaint.Render("x") == "" {
		t.Fatal("tuiStyles.inkFaint renders to empty string — slot may be uninitialized")
	}
	// inkFaint (#4a4438 → 38;2;73;68;56 via termenv float truncation).
	fg := s.inkFaint.GetForeground()
	rendered := r.NewStyle().Foreground(fg).Render("x")
	const wantANSI = "38;2;73;68;56"
	if !strings.Contains(rendered, wantANSI) {
		t.Errorf("tuiStyles.inkFaint: foreground ANSI %q missing expected sequence %q", rendered, wantANSI)
	}
}

// TestTuiStyles_InkMutedSlot asserts that the inkMuted style slot exists (via dimLabel alias or own slot).
func TestTuiStyles_InkMutedSlot(t *testing.T) {
	s := newTuiStyles()
	// inkMuted is exposed as dimLabel in existing code; we check dimLabel remains correct.
	if s.dimLabel.Render("x") == "" {
		t.Fatal("tuiStyles.dimLabel renders to empty string")
	}
}
