package tui

// overlay_palette.go — commandPalette dialog (PR3a).
//
// commandPalette implements the dialog interface and renders a searchable
// slash command palette overlay. It is built AGENT-FREE so it is fully
// unit-testable with a static []agent.CommandInfo.
//
// Message flow:
//   tea.KeyMsg "/" in handleChatKey
//     → overlays.Push(newCommandPalette(ag.Commands(), styles))
//     → overlay-interception block in Model.Update calls HandleMsg
//     → HandleMsg returns (updatedPalette, cmd, consumed)
//     → cmd emits dispatchCommandMsg or popOverlayMsg
//     → Model.Update global type-switch handles them

import (
	"context"
	"strings"
	"unicode/utf8"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"daimon/internal/agent"
)

// ---------------------------------------------------------------------------
// Messages
// ---------------------------------------------------------------------------

// popOverlayMsg is emitted by commandPalette when the user presses Esc.
// Model.Update's global type-switch pops the top overlay on receipt.
type popOverlayMsg struct{}

// dispatchCommandMsg is emitted by commandPalette when a command is confirmed.
// Model.Update's global type-switch pops the overlay and launches runCommandCmd.
type dispatchCommandMsg struct {
	name             string
	allowDestructive bool
}

// commandResultMsg is returned by runCommandCmd when the agent completes
// (or fails) a command execution. Model.Update appends a MsgDaimon.
type commandResultMsg struct {
	reply string
	err   error
}

// ---------------------------------------------------------------------------
// commandPalette — the dialog implementation
// ---------------------------------------------------------------------------

// commandPalette is a value type implementing the dialog interface.
// All state mutations return a new copy (value semantics).
type commandPalette struct {
	all                   []agent.CommandInfo // full registered command list
	filtered              []agent.CommandInfo // subset matching current query
	query                 string
	selIdx                int
	confirmingDestructive bool
	styles                tuiStyles
}

// newCommandPalette constructs a commandPalette with the given command list.
// Pass an empty slice when the agent is nil (welcome screen / tests without agent).
func newCommandPalette(cmds []agent.CommandInfo, styles tuiStyles) commandPalette {
	p := commandPalette{
		all:    cmds,
		styles: styles,
	}
	p.filtered = p.applyFilter("")
	return p
}

// ID implements dialog.
func (p commandPalette) ID() string { return "command-palette" }

// HandleMsg implements dialog.
// Value receiver: every path returns the updated palette as `dialog`,
// so the overlayManager's Replace(next) call stores the new state.
func (p commandPalette) HandleMsg(msg tea.Msg) (dialog, tea.Cmd, bool) {
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		// Non-key messages: swallow (palette is modal).
		return p, nil, true
	}

	switch key.Type {
	case tea.KeyEsc:
		cmd := func() tea.Msg { return popOverlayMsg{} }
		return p, cmd, true

	case tea.KeyUp:
		p.selIdx--
		if p.selIdx < 0 {
			p.selIdx = 0
		}
		p.confirmingDestructive = false
		return p, nil, true

	case tea.KeyDown:
		p.selIdx++
		if max := len(p.filtered) - 1; p.selIdx > max {
			if max < 0 {
				p.selIdx = 0
			} else {
				p.selIdx = max
			}
		}
		p.confirmingDestructive = false
		return p, nil, true

	case tea.KeyCtrlP:
		p.selIdx--
		if p.selIdx < 0 {
			p.selIdx = 0
		}
		p.confirmingDestructive = false
		return p, nil, true

	case tea.KeyCtrlN:
		p.selIdx++
		if max := len(p.filtered) - 1; p.selIdx > max {
			if max < 0 {
				p.selIdx = 0
			} else {
				p.selIdx = max
			}
		}
		p.confirmingDestructive = false
		return p, nil, true

	case tea.KeyEnter:
		if len(p.filtered) == 0 {
			return p, nil, true // empty list — consumed no-op
		}
		sel := p.filtered[p.selIdx]
		if sel.Destructive && !p.confirmingDestructive {
			p.confirmingDestructive = true
			return p, nil, true
		}
		// Dispatch the command.
		name := sel.Name
		allow := sel.Destructive
		cmd := func() tea.Msg {
			return dispatchCommandMsg{name: name, allowDestructive: allow}
		}
		return p, cmd, true

	case tea.KeyBackspace:
		if len(p.query) > 0 {
			// Trim last rune (query is ASCII-safe in practice but use utf8 for correctness).
			_, size := utf8.DecodeLastRuneInString(p.query)
			p.query = p.query[:len(p.query)-size]
		}
		p.filtered = p.applyFilter(p.query)
		p.selIdx = 0
		p.confirmingDestructive = false
		return p, nil, true

	case tea.KeyRunes:
		p.query += string(key.Runes)
		p.filtered = p.applyFilter(p.query)
		p.selIdx = 0
		p.confirmingDestructive = false
		return p, nil, true

	default:
		// All other keys: swallow (modal).
		return p, nil, true
	}
}

// applyFilter returns the subset of p.all whose Name or Description contains
// query as a case-insensitive substring. Empty query returns all commands.
func (p commandPalette) applyFilter(query string) []agent.CommandInfo {
	if query == "" {
		out := make([]agent.CommandInfo, len(p.all))
		copy(out, p.all)
		return out
	}
	q := strings.ToLower(query)
	var out []agent.CommandInfo
	for _, cmd := range p.all {
		if strings.Contains(strings.ToLower(cmd.Name), q) ||
			strings.Contains(strings.ToLower(cmd.Description), q) {
			out = append(out, cmd)
		}
	}
	return out
}

// Render implements dialog.
// Draws a centered box with title, query line, and filtered command list.
// All width math uses ansi.StringWidth / ansi.Truncate — never len().
// All colors come from tuiStyles — no inline hex literals.
func (p commandPalette) Render(width, height int, styles tuiStyles) string {
	// Box dimensions: 60% of terminal width, capped at 72, min 40.
	boxWidth := width * 60 / 100
	if boxWidth > 72 {
		boxWidth = 72
	}
	if boxWidth < 40 {
		boxWidth = 40
	}
	// Inner content width (box has 1-col padding each side).
	innerWidth := boxWidth - 4 // RoundedBorder(2) + Padding(0,1)(2)
	if innerWidth < 1 {
		innerWidth = 1
	}

	var sb strings.Builder

	// Title line.
	title := styles.accent.Render("⫶ commands")
	sb.WriteString(title)
	sb.WriteRune('\n')

	// Query line: "> <query>█"  (caret marks the cursor position)
	queryLine := "> " + p.query + "█"
	queryLine = ansi.Truncate(queryLine, innerWidth, "…")
	sb.WriteString(styles.dimLabel.Render(queryLine))
	sb.WriteRune('\n')

	// Separator
	sb.WriteString(styles.dimLabel.Render(strings.Repeat("─", innerWidth)))
	sb.WriteRune('\n')

	// Confirming-destructive prompt (shown instead of / after the list).
	if p.confirmingDestructive && p.selIdx < len(p.filtered) {
		sb.WriteString(styles.amber.Render("⚠ destructive — enter to confirm, esc to cancel"))
		sb.WriteRune('\n')
	}

	// Command list.
	maxRows := 10
	if len(p.filtered) < maxRows {
		maxRows = len(p.filtered)
	}
	start := 0
	// Scroll window: keep selIdx visible.
	if p.selIdx >= maxRows {
		start = p.selIdx - maxRows + 1
	}
	for i := start; i < start+maxRows && i < len(p.filtered); i++ {
		cmd := p.filtered[i]

		// Name column (left), description (right, dim+truncated).
		nameStr := cmd.Name
		descStr := cmd.Description

		// Destructive marker.
		marker := ""
		if cmd.Destructive {
			marker = styles.amber.Render("⚠ ")
		}

		// Build the row: marker + name + spaces + desc
		nameRendered := marker + styles.label.Render(nameStr)
		nameW := ansi.StringWidth(nameRendered)
		sep := "  "
		descAvail := innerWidth - nameW - ansi.StringWidth(sep)
		if descAvail < 0 {
			descAvail = 0
		}
		descTrunc := ansi.Truncate(descStr, descAvail, "…")
		row := nameRendered + sep + styles.dimLabel.Render(descTrunc)

		if i == p.selIdx {
			// Highlight selected row.
			row = styles.selected.Render(nameStr)
			if cmd.Destructive {
				row = styles.amber.Render("⚠ ") + styles.selected.Render(nameStr)
			}
			rowW := ansi.StringWidth(row)
			descAvail2 := innerWidth - rowW - ansi.StringWidth(sep)
			if descAvail2 < 0 {
				descAvail2 = 0
			}
			descTrunc2 := ansi.Truncate(descStr, descAvail2, "…")
			row += sep + styles.dimLabel.Render(descTrunc2)
		}

		sb.WriteString(row)
		sb.WriteRune('\n')
	}

	if len(p.filtered) == 0 {
		sb.WriteString(styles.dimLabel.Render("no matching commands"))
		sb.WriteRune('\n')
	}

	// Hints footer.
	hints := "↑/↓ navigate  enter select  esc close"
	sb.WriteString(styles.hint.Render(ansi.Truncate(hints, innerWidth, "…")))

	content := sb.String()

	// Wrap in the centralized palette border box (styles.paletteBox).
	// Width sets the inner content width (border+padding steal 4 cols).
	// Return the raw bordered box; Model.View() calls placeOverlay to composite
	// it over the dimmed base at the correct center position.
	// lipgloss.Place is NOT called here — the caller (placeOverlay) handles centering.
	return styles.paletteBox.Width(boxWidth - 4).Render(content)
}

// ---------------------------------------------------------------------------
// runCommandCmd — IO in a tea.Cmd closure (Cmd discipline)
// ---------------------------------------------------------------------------

// runCommandCmd returns a tea.Cmd that calls ag.RunCommand in a goroutine
// and delivers the result as a commandResultMsg. No mutation in the closure.
// V1 limitation: context.Background() is used — there is no cancellation on TUI exit.
func runCommandCmd(ag *agent.Agent, name, args string, allowDestructive bool) tea.Cmd {
	return func() tea.Msg {
		res, err := ag.RunCommand(context.Background(), agent.RunCommandRequest{
			Name:             name,
			Args:             args,
			ChannelID:        "tui",
			SenderID:         "local_user",
			AllowDestructive: allowDestructive,
		})
		if err != nil {
			return commandResultMsg{err: err}
		}
		return commandResultMsg{reply: res.Reply}
	}
}
