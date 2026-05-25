package tui

import (
	"testing"

	"github.com/charmbracelet/x/exp/golden"
)

// TestModel_View_ChatScreen_Golden verifies the chat screen renders
// consistently at 80x24. Run with -update to regenerate the golden file.
func TestModel_View_ChatScreen_Golden(t *testing.T) {
	s := newTuiStyles()
	m := newTestModel()
	m.width = 80
	m.height = 24
	m.screen = screenChat
	m.focus = focusEditor
	m.topBar.SetData("⫶", "/home/user/project", "main", "claude-opus-4", "build", "$0.12", "ready")
	m.footer.SetScreen(screenChat)

	// Pre-populate with a realistic thread.
	m.thread.append(&MsgUser{text: "Write a Go function that reverses a slice.", styles: s})
	m.thread.append(&MsgDaimon{text: "Sure! Here's a generic reverse function:\n\n```go\nfunc Reverse[T any](s []T) {\n    for i, j := 0, len(s)-1; i < j; i, j = i+1, j-1 {\n        s[i], s[j] = s[j], s[i]\n    }\n}\n```", styles: s})
	tl := &ToolLine{callID: "call-1", name: "write_file", state: toolDone, styles: s}
	tl.stats.lines = 12
	tl.stats.tokens = 48
	m.thread.append(tl)

	got := m.View()
	golden.RequireEqual(t, []byte(got))
}
