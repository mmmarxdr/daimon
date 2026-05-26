package tui

// screen_error.go — permission-denied screen handler (screen 07, PR5).
//
// The error screen is triggered ONLY by policy/mode DENIAL events
// (EventToolEnd with Meta["denied"]=="true"). Runtime errors (tool crash,
// tool-not-found) are NOT denials — they stay in the chat thread via the
// existing ToolLine toolError state.
//
// V1 scope: render-only. No blocking approve/reject seam exists yet.
// TODO(post-V1): PermissionPrompt overlay when an approval seam exists.
//
// RULE: No IO in Update. All side effects return tea.Cmd.

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

// updateError is the screenError Update handler. Called from Model.Update
// when m.screen == screenError.
//
// V1 keys:
//   - esc → return to prevScreen (the chat screen that triggered the denial)
//
// No other keys are handled in V1 — the screen is read-only.
func (m Model) updateError(msg tea.Msg) (tea.Model, tea.Cmd) {
	if key, ok := msg.(tea.KeyMsg); ok {
		switch key.String() {
		case "esc":
			m.screen = m.prevScreen
			m.footer = footerHints{screen: m.prevScreen}
			return m, nil
		}
	}
	return m, nil
}

// renderError renders the center column for the error screen.
//
// It shows the offending tool name and the denial reason, styled with the
// errStyle (amber/red) to signal this is an access-control boundary — NOT a
// crash. When errorToolName is empty (shouldn't normally render) a neutral
// placeholder is returned.
func renderError(m Model, width, height int) string {
	if m.errorToolName == "" {
		msg := m.styles.dimLabel.Render("no active denial — press esc to return")
		return centerText(msg, width)
	}

	inner := width
	if inner < 8 {
		inner = 8
	}

	var rows []string

	// Header — amber to signal mode/policy context (not a crash).
	header := m.styles.amber.Render("⊘ permission denied")
	rows = append(rows, ansi.Truncate(header, inner, "…"))
	rows = append(rows, "")

	// Offending tool line.
	toolLabel := m.styles.label.Render("tool:   ")
	toolValue := m.styles.errStyle.Render(m.errorToolName)
	toolLine := ansi.Truncate(toolLabel+toolValue, inner, "…")
	rows = append(rows, toolLine)

	// Reason — may be multi-word; truncate to a single visible line.
	reason := m.errorReason
	if reason != "" {
		reasonLabel := m.styles.label.Render("reason: ")
		// Reserve space for "reason: " prefix (8 visible chars).
		const prefixWidth = 8
		valWidth := inner - prefixWidth
		if valWidth < 4 {
			valWidth = 4
		}
		truncatedReason := ansi.Truncate(reason, valWidth, "…")
		reasonLine := ansi.Truncate(reasonLabel+m.styles.dimLabel.Render(truncatedReason), inner, "…")
		rows = append(rows, reasonLine)
	}

	rows = append(rows, "")
	hint := m.styles.dimLabel.Render("press esc to return to chat")
	rows = append(rows, ansi.Truncate(hint, inner, "…"))

	// Pad to height to avoid layout gaps.
	for len(rows) < height {
		rows = append(rows, "")
	}

	return strings.Join(rows, "\n")
}
