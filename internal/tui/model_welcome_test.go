package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

// TestUpdateWelcome_EnterWithText_TransitionsToChat verifies the welcome→chat
// transition: pressing Enter on the welcome screen with non-empty input must
// switch to the chat screen, append the typed text as a MsgUser, and return a
// submit Cmd (which produces promptSentMsg). This is the navigation that makes
// the chat screen reachable; without it `daimon tui` is stuck on welcome.
func TestUpdateWelcome_EnterWithText_TransitionsToChat(t *testing.T) {
	m := newTestModel() // screen=welcome, focus=focusEditor, inbox=nil (submit is instant)
	m.input.ti.SetValue("Write a reverse function")

	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	rm := next.(Model)

	if rm.screen != screenChat {
		t.Errorf("screen after Enter = %v, want chat", rm.screen)
	}
	if rm.focus != focusEditor {
		t.Errorf("focus after Enter = %v, want focusEditor", rm.focus)
	}
	if rm.footer.screen != screenChat {
		t.Errorf("footer.screen after Enter = %v, want chat", rm.footer.screen)
	}
	if len(rm.thread.items) != 1 {
		t.Fatalf("thread items = %d, want 1 (the submitted MsgUser)", len(rm.thread.items))
	}
	mu, ok := rm.thread.items[0].(*MsgUser)
	if !ok {
		t.Fatalf("first thread item = %T, want *MsgUser", rm.thread.items[0])
	}
	if mu.text != "Write a reverse function" {
		t.Errorf("MsgUser.text = %q, want %q", mu.text, "Write a reverse function")
	}
	if cmd == nil {
		t.Error("expected a submit Cmd, got nil")
	}
}

// TestUpdateWelcome_EnterEmpty_StaysOnWelcome verifies that pressing Enter with
// an empty input does NOT transition (no spurious chat screen, no empty MsgUser).
func TestUpdateWelcome_EnterEmpty_StaysOnWelcome(t *testing.T) {
	m := newTestModel() // empty input by default

	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	rm := next.(Model)

	if rm.screen != screenWelcome {
		t.Errorf("screen = %v, want welcome (empty input must not transition)", rm.screen)
	}
	if len(rm.thread.items) != 0 {
		t.Errorf("thread items = %d, want 0 (empty input must not append)", len(rm.thread.items))
	}
}

// ---------------------------------------------------------------------------
// PR 1c tests — Welcome ASCII logo + narrow-terminal fallback
// ---------------------------------------------------------------------------

// TestWelcomeCenter_ASCIILogoPresent verifies that the full welcome View()
// (through the real layout, which reserves the right rail) shows the ASCII logo
// block and the tagline once the terminal is wide enough that the center column
// clears the art width. The welcome screen carries a 32-col rail, so the logo
// only fits at terminal width >= railWidth(32) + artWidth(69) = 101 cols; we use 110.
// Driving m.View() — not renderWelcomeCenter directly — is what makes this test
// honest: it exercises the width the layout actually passes the center column.
// [Req: Welcome ASCII logo — present scenario]
func TestWelcomeCenter_ASCIILogoPresent(t *testing.T) {
	m := newTestModel()
	m.width = 110
	m.height = 40
	m.screen = screenWelcome
	m.mode = "build"

	stripped := ansi.Strip(m.View())
	if !strings.Contains(stripped, "▄▄▄▄▄") {
		t.Errorf("welcome View() at width=110 missing ASCII logo line '▄▄▄▄▄'\noutput:\n%s", stripped)
	}
	if strings.Contains(stripped, "⫶ daimon") {
		t.Errorf("welcome View() at width=110 fell back to '⫶ daimon' — logo should fit at 110 cols")
	}
	if !strings.Contains(stripped, "speak, and daimon listens.") {
		t.Errorf("welcome View() at width=110 missing tagline 'speak, and daimon listens.'\noutput:\n%s", stripped)
	}
}

// TestWelcomeCenter_NarrowTerminal_RealLayoutFallsBack verifies that at a typical
// 80-col terminal — where the 32-col rail leaves the center column too narrow for
// the art — the full View() degrades to the single-line "⫶ daimon" mark instead of
// wrapping the block. This is the companion to the present-scenario test above and
// guards the real (rail-aware) geometry, not the bare helper.
func TestWelcomeCenter_NarrowTerminal_RealLayoutFallsBack(t *testing.T) {
	m := newTestModel()
	m.width = 80
	m.height = 24
	m.screen = screenWelcome
	m.mode = "build"

	stripped := ansi.Strip(m.View())
	if strings.Contains(stripped, "▄▄▄▄▄") {
		t.Errorf("welcome View() at width=80 rendered ASCII art — center column is too narrow with the rail; want fallback")
	}
	if !strings.Contains(stripped, "⫶ daimon") {
		t.Errorf("welcome View() at width=80 missing '⫶ daimon' fallback\noutput:\n%s", stripped)
	}
}

// TestWelcomeCenter_NarrowTerminal_FallbackToSimpleLogo verifies that
// renderWelcomeCenter on a narrow terminal (width < art width) falls back to
// "⫶ daimon" rather than wrapping the multi-line ASCII art.
// [Req: Welcome ASCII logo — narrow terminal fallback]
func TestWelcomeCenter_NarrowTerminal_FallbackToSimpleLogo(t *testing.T) {
	m := newTestModel()

	// Use a width that is definitely narrower than the ~67-col logo.
	out := renderWelcomeCenter(m, 40, 19)
	stripped := ansi.Strip(out)

	if strings.Contains(stripped, "▄▄▄▄▄") {
		t.Errorf("renderWelcomeCenter(40) rendered ASCII art on narrow terminal — should fall back to ⫶ daimon")
	}
	if !strings.Contains(stripped, "⫶ daimon") {
		t.Errorf("renderWelcomeCenter(40) narrow fallback missing '⫶ daimon'\noutput:\n%s", stripped)
	}
}

// TestWelcomeCenter_LogoAbsentOnChatScreen verifies that the chat screen render
// does NOT contain the ASCII logo block. [Req: Logo absent in non-welcome screens]
func TestWelcomeCenter_LogoAbsentOnChatScreen(t *testing.T) {
	m := newTestModel()
	m.width = 80
	m.height = 24
	m.screen = screenChat

	view := m.View()
	stripped := ansi.Strip(view)

	if strings.Contains(stripped, "▄▄▄▄▄") {
		t.Errorf("chat screen View() contains ASCII logo line '▄▄▄▄▄' — must be absent on non-welcome screens")
	}
}
