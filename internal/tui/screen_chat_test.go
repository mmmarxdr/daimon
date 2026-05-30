package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/exp/golden"
)

// TestModel_View_ChatScreen_Golden verifies the chat screen renders
// consistently at 80x24. Run with -update to regenerate the golden file.
//
// WU-c adaptation: the test drives a tea.WindowSizeMsg through Update before
// calling View() so the viewport is sized and thread content is pushed into it.
// Without this, m.viewport has zero dimensions and viewport.View() returns "".
// Task 2.12 (human-reviewed golden regen) is pending — the golden is expected
// to fail after WU-c changes the rendering path; see tasks.md §2.12.
func TestModel_View_ChatScreen_Golden(t *testing.T) {
	s := newTuiStyles()
	m := newTestModel()
	m.screen = screenChat
	m.focus = focusEditor
	m.topBar.SetData("⫶", "/home/user/project", "main", "claude-opus-4", "build", "$0.12", "ready")
	m.footer.SetScreen(screenChat)
	// WU-a: renderLayout reads m.mode (cached field), not the live modeAgent.
	// Set m.mode to match the value passed to SetData — golden output is unchanged.
	m.mode = "build"

	// WU-c (task 2.11): thread.styles must be set so the truncation marker
	// renders correctly and Render() has access to styles.
	m.thread.styles = s

	// Pre-populate with a realistic thread.
	m.thread.append(&MsgUser{text: "Write a Go function that reverses a slice.", styles: s})
	m.thread.append(&MsgDaimon{text: "Sure! Here's a generic reverse function:\n\n```go\nfunc Reverse[T any](s []T) {\n    for i, j := 0, len(s)-1; i < j; i, j = i+1, j-1 {\n        s[i], s[j] = s[j], s[i]\n    }\n}\n```", styles: s})
	tl := &ToolLine{callID: "call-1", name: "write_file", state: toolDone, styles: s}
	tl.stats.lines = 12
	tl.stats.tokens = 48
	m.thread.append(tl)

	// WU-c: drive WindowSizeMsg through Update so the viewport is sized and
	// thread content is pushed into it. Items were appended before the size msg,
	// so refreshThreadViewport in the WindowSizeMsg handler populates the content.
	upd, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = upd.(Model)

	got := m.View()
	golden.RequireEqual(t, []byte(got))
}
