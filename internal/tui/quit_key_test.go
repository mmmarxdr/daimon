package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// TestQuitKey_Q_WithActiveOverlay_DoesNotQuit verifies that 'q' does NOT quit
// when the command palette (or any overlay) is active.
func TestQuitKey_Q_WithActiveOverlay_DoesNotQuit(t *testing.T) {
	m := newTestModel()
	m.screen = screenChat
	m.focus = focusMain // navigation focus — 'q' would quit without overlay
	m.overlays.Push(newCommandPalette(testCmds, newTuiStyles()))

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})

	if cmd != nil {
		// Check whether the returned cmd produces a tea.Quit signal.
		msg := cmd()
		if _, isQuit := msg.(tea.QuitMsg); isQuit {
			t.Error("'q' with active overlay must NOT quit; overlay should intercept it")
		}
	}
	// Also verify the overlay is still active (was not popped by a non-esc key from outside).
}

// TestQuitKey_Q_WithFocusEditor_DoesNotQuit verifies that 'q' does NOT quit
// when the editor has focus (user is typing).
func TestQuitKey_Q_WithFocusEditor_DoesNotQuit(t *testing.T) {
	m := newTestModel()
	m.screen = screenChat
	m.focus = focusEditor

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})

	// cmd should NOT be tea.Quit.
	if cmd != nil {
		msg := cmd()
		if _, isQuit := msg.(tea.QuitMsg); isQuit {
			t.Error("'q' with focusEditor must NOT quit (user is typing)")
		}
	}
}

// TestQuitKey_Q_WithFocusMain_NoOverlay_Quits verifies that 'q' DOES quit
// in navigation context (focusMain, no active overlay).
func TestQuitKey_Q_WithFocusMain_NoOverlay_Quits(t *testing.T) {
	m := newTestModel()
	m.screen = screenChat
	m.focus = focusMain // navigation focus

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})

	if cmd == nil {
		t.Fatal("'q' in navigation context (focusMain, no overlay) should return tea.Quit cmd")
	}
	msg := cmd()
	if _, isQuit := msg.(tea.QuitMsg); !isQuit {
		t.Errorf("'q' in navigation context: cmd returned %T, want tea.QuitMsg", msg)
	}
}

// TestQuitKey_CtrlC_AlwaysQuits verifies that ctrl+c always quits regardless
// of overlay or focus state.
func TestQuitKey_CtrlC_AlwaysQuits(t *testing.T) {
	cases := []struct {
		name  string
		setup func(m *Model)
	}{
		{
			name: "with_active_overlay",
			setup: func(m *Model) {
				m.overlays.Push(newCommandPalette(testCmds, newTuiStyles()))
				m.focus = focusEditor
			},
		},
		{
			name: "with_focus_editor",
			setup: func(m *Model) {
				m.focus = focusEditor
			},
		},
		{
			name: "with_focus_main",
			setup: func(m *Model) {
				m.focus = focusMain
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := newTestModel()
			m.screen = screenChat
			tc.setup(&m)

			_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})

			if cmd == nil {
				t.Fatal("ctrl+c must always return a non-nil tea.Quit cmd")
			}
			msg := cmd()
			if _, isQuit := msg.(tea.QuitMsg); !isQuit {
				t.Errorf("ctrl+c: cmd returned %T, want tea.QuitMsg", msg)
			}
		})
	}
}

// TestQuitKey_Q_WithFocusNone_DoesNotQuit verifies that 'q' does NOT quit
// when focus is focusNone (undefined focus, safe default: treat as editor).
func TestQuitKey_Q_WithFocusNone_DoesNotQuit(t *testing.T) {
	m := newTestModel()
	m.screen = screenChat
	m.focus = focusNone // undefined focus

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})

	if cmd != nil {
		msg := cmd()
		if _, isQuit := msg.(tea.QuitMsg); isQuit {
			t.Error("'q' with focusNone must NOT quit")
		}
	}
}
