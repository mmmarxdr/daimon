package tui

// overlay_help.go — helpOverlay dialog (Phase A / A2).
//
// helpOverlay implements the dialog interface and renders a bordered box
// listing key bindings. It is AGENT-FREE and fully unit-testable.
//
// Message flow:
//   tea.KeyMsg "?" in updateWelcome / handleChatKey
//     → overlays.Push(newHelpOverlay(styles))
//     → overlay-interception block in Model.Update calls HandleMsg
//     → Esc → popOverlayMsg → Model.Update pops the overlay

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

// helpOverlay is a value type implementing the dialog interface.
// All state mutations return a new copy (value semantics, same as commandPalette).
type helpOverlay struct {
	mode   string // current agent mode (empty = no agent)
	styles tuiStyles
}

// newHelpOverlay constructs a helpOverlay with current styles.
// mode is populated from the model's modeAgent at push time (may be "").
func newHelpOverlay(styles tuiStyles) helpOverlay {
	return helpOverlay{styles: styles}
}

// newHelpOverlayWithMode constructs a helpOverlay that shows the current mode.
func newHelpOverlayWithMode(styles tuiStyles, mode string) helpOverlay {
	return helpOverlay{styles: styles, mode: mode}
}

// ID implements dialog.
func (h helpOverlay) ID() string { return "help-overlay" }

// HandleMsg implements dialog. Only Esc and ctrl+c are handled; all others are
// swallowed (the help overlay is modal while open).
func (h helpOverlay) HandleMsg(msg tea.Msg) (dialog, tea.Cmd, bool) {
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		// Non-key: swallow (modal).
		return h, nil, true
	}

	switch key.Type {
	case tea.KeyEsc:
		cmd := func() tea.Msg { return popOverlayMsg{} }
		return h, cmd, true
	default:
		// All other keys: swallow (modal, user must press Esc).
		return h, nil, true
	}
}

// Render implements dialog. Draws a centered bordered box with key bindings.
// All widths use ansi.StringWidth; all colors come from tuiStyles.
func (h helpOverlay) Render(width, height int, styles tuiStyles) string {
	// Box dimensions: 50% of terminal width, capped at 60, min 40.
	boxWidth := width * 50 / 100
	if boxWidth > 60 {
		boxWidth = 60
	}
	if boxWidth < 40 {
		boxWidth = 40
	}
	// Inner content width (paletteBox: NormalBorder(2) + Padding(0,1)(2) = 4 cols).
	innerWidth := boxWidth - 4
	if innerWidth < 1 {
		innerWidth = 1
	}

	var sb strings.Builder

	// Title.
	title := styles.accent.Render("⫶ help")
	sb.WriteString(title)
	sb.WriteRune('\n')

	// Separator.
	sb.WriteString(styles.dimLabel.Render(strings.Repeat("─", innerWidth)))
	sb.WriteRune('\n')

	// Key bindings table.
	type binding struct {
		key   string
		label string
	}
	bindings := []binding{
		{"/", "open command palette"},
		{"⌃P", "open command palette"},
		{"⇥", "cycle mode (build→plan→review)"},
		{"⌃C", "quit"},
		{"esc", "close overlay / toggle focus"},
		{"enter", "send message"},
		{"⌃T", "tools screen"},
	}
	for _, b := range bindings {
		keyStr := styles.accent.Render(b.key)
		keyW := ansi.StringWidth(keyStr)
		sep := "  "
		descAvail := innerWidth - keyW - ansi.StringWidth(sep)
		if descAvail < 0 {
			descAvail = 0
		}
		descTrunc := ansi.Truncate(b.label, descAvail, "…")
		row := keyStr + sep + styles.dimLabel.Render(descTrunc)
		sb.WriteString(row)
		sb.WriteRune('\n')
	}

	// Mode line (when available).
	if h.mode != "" {
		sb.WriteString(styles.dimLabel.Render(strings.Repeat("─", innerWidth)))
		sb.WriteRune('\n')
		modeLine := styles.dimLabel.Render("mode ") + styles.amber.Render(h.mode)
		sb.WriteString(modeLine)
		sb.WriteRune('\n')
	}

	// Footer hint.
	hint := styles.hint.Render(ansi.Truncate("esc close", innerWidth, "…"))
	sb.WriteString(hint)

	content := sb.String()
	return styles.paletteBox.Width(boxWidth - 4).Render(content)
}
