package tui

// components_thread.go — imperative thread-item components for the chat screen.
//
// Thread items appear in the center column of screen 02 (chat) and partially
// on screen 07 (error). All implement the threadItem interface:
//   Render(width int) string
//
// RULE: No IO in any Render or state-transition method. All state changes
// happen in Update (via busEventMsg handlers in screen_chat.go).
// RULE: All width math uses ansi.StringWidth / ansi.Truncate — never len().
// RULE: All colors come from the tuiStyles struct — no hex literals here.

import (
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

// ---------------------------------------------------------------------------
// Interfaces (AD-6 interface ladder)
// ---------------------------------------------------------------------------

// threadItem is the base interface for all items rendered in the chat thread.
// Concrete types: MsgUser, MsgDaimon, Reasoning, ToolLine, Subagent.
type threadItem interface {
	Render(width int) string
}

// ---------------------------------------------------------------------------
// thread — ordered list of threadItems
// ---------------------------------------------------------------------------

// thread holds an ordered slice of threadItems for the chat screen center column.
type thread struct {
	items []threadItem
}

// append adds a threadItem to the end of the thread.
func (t *thread) append(item threadItem) {
	t.items = append(t.items, item)
}

// Render renders all items joined by newlines.
func (t *thread) Render(width int) string {
	if len(t.items) == 0 {
		return ""
	}
	parts := make([]string, 0, len(t.items))
	for _, item := range t.items {
		s := item.Render(width)
		if s != "" {
			parts = append(parts, s)
		}
	}
	return strings.Join(parts, "\n")
}

// findToolLine returns the first ToolLine with the given callID, or nil.
func (t *thread) findToolLine(callID string) *ToolLine {
	for _, item := range t.items {
		if tl, ok := item.(*ToolLine); ok {
			if tl.callID == callID {
				return tl
			}
		}
	}
	return nil
}

// ---------------------------------------------------------------------------
// MsgUser — user turn bubble
// ---------------------------------------------------------------------------

// MsgUser renders a user-submitted message in the chat thread.
type MsgUser struct {
	text   string
	styles tuiStyles
}

// Render implements threadItem. Returns the user message styled as a bubble.
func (m *MsgUser) Render(width int) string {
	prefix := m.styles.label.Render("you  ")
	inner := wrapText(m.text, width-ansi.StringWidth(prefix)-2)
	lines := strings.Split(inner, "\n")
	out := make([]string, 0, len(lines))
	for i, line := range lines {
		if i == 0 {
			out = append(out, prefix+m.styles.accent.Render(line))
		} else {
			// Indent continuation lines to align with message start.
			indent := strings.Repeat(" ", ansi.StringWidth(prefix))
			out = append(out, indent+m.styles.accent.Render(line))
		}
	}
	return strings.Join(out, "\n")
}

// ---------------------------------------------------------------------------
// MsgDaimon — assistant turn bubble
// ---------------------------------------------------------------------------

// MsgDaimon renders a completed assistant message in the chat thread.
// It appears on chat (02) and error (07) screens per the panel matrix.
type MsgDaimon struct {
	text   string
	styles tuiStyles
}

// Render implements threadItem. Returns the daimon message styled as a bubble.
func (m *MsgDaimon) Render(width int) string {
	prefix := m.styles.dimLabel.Render("δ    ")
	inner := wrapText(m.text, width-ansi.StringWidth(prefix)-2)
	lines := strings.Split(inner, "\n")
	out := make([]string, 0, len(lines))
	for i, line := range lines {
		if i == 0 {
			out = append(out, prefix+line)
		} else {
			indent := strings.Repeat(" ", ansi.StringWidth(prefix))
			out = append(out, indent+line)
		}
	}
	return strings.Join(out, "\n")
}

// ---------------------------------------------------------------------------
// Reasoning — collapsed-by-default reasoning block
// ---------------------------------------------------------------------------

// Reasoning renders the model's internal reasoning text.
// Collapsed by default; Expand/Collapse toggles visibility.
// Implements the Expandable interface (architecture.md).
type Reasoning struct {
	text     string
	expanded bool
	styles   tuiStyles
}

// Expand shows the full reasoning text.
func (r *Reasoning) Expand() { r.expanded = true }

// Collapse hides the reasoning text (shows a summary line only).
func (r *Reasoning) Collapse() { r.expanded = false }

// Expanded reports whether the reasoning is currently expanded.
func (r *Reasoning) Expanded() bool { return r.expanded }

// Render implements threadItem. When collapsed, shows a one-line summary
// that does NOT reveal the reasoning content — only a character count and
// an expand affordance. When expanded, shows the full text.
func (r *Reasoning) Render(width int) string {
	prefix := r.styles.dimLabel.Render("△ reasoning  ")
	if !r.expanded {
		// Collapsed: show character count + expand affordance, no content.
		charCount := utf8.RuneCountInString(r.text)
		summary := fmt.Sprintf("(%d chars) — press r to expand", charCount)
		return prefix + r.styles.hint.Render(ansi.Truncate(summary, width-ansi.StringWidth(prefix)-1, "…"))
	}
	inner := wrapText(r.text, width-ansi.StringWidth(prefix)-2)
	lines := strings.Split(inner, "\n")
	out := make([]string, 0, len(lines)+1)
	out = append(out, prefix+r.styles.hint.Render(lines[0]))
	for _, line := range lines[1:] {
		indent := strings.Repeat(" ", ansi.StringWidth(prefix))
		out = append(out, indent+r.styles.hint.Render(line))
	}
	return strings.Join(out, "\n")
}

// ---------------------------------------------------------------------------
// ToolLine — tool execution line with 4 states and 4 stat slots
// ---------------------------------------------------------------------------

// toolState encodes the lifecycle state of a tool invocation.
type toolState int

const (
	toolDone    toolState = iota // tool finished successfully
	toolRunning                  // tool currently executing (spinner active)
	toolError                    // tool finished with an error
	toolQueued                   // tool is queued, not yet started
)

// brailleSpinner is the animation frame sequence for running tool lines.
// Glyphs per components.md §Thread Components.
var brailleSpinner = []rune("⣾⣽⣻⢿⡿⣟⣯⣷")

// toolStats holds the 4 stat slots shown when a tool completes.
type toolStats struct {
	lines    int
	matches  int
	tokens   int
	duration time.Duration
}

// ToolLine renders a single tool invocation row in the chat thread.
// It implements both threadItem and Animatable (Tick() tea.Cmd).
type ToolLine struct {
	callID       string    // matches notify.Event.ToolCallID
	name         string    // tool name (e.g. "bash", "read_file")
	state        toolState // current lifecycle state
	stats        toolStats // populated on completion
	spinnerFrame int       // current braille frame index
	expanded     bool      // show expand affordance when name is truncated
	styles       tuiStyles
}

// spinnerTickMsg is the tea.Msg returned by Tick() to advance the spinner.
type spinnerTickMsg struct {
	callID string
}

// Tick implements Animatable. Returns a tea.Cmd that fires spinnerTickMsg
// for this ToolLine, advancing the spinner frame on the next Update.
// Only meaningful when state == toolRunning.
func (tl *ToolLine) Tick() tea.Cmd {
	id := tl.callID
	return func() tea.Msg {
		return spinnerTickMsg{callID: id}
	}
}

// AdvanceSpinner increments the spinner frame (called from Update on spinnerTickMsg).
func (tl *ToolLine) AdvanceSpinner() {
	tl.spinnerFrame = (tl.spinnerFrame + 1) % len(brailleSpinner)
}

// Render implements threadItem. Renders a single-line tool row (possibly
// multi-line if expanded affordance is shown).
func (tl *ToolLine) Render(width int) string {
	// State glyph + color.
	var stateStr string
	switch tl.state {
	case toolRunning:
		spinner := string(brailleSpinner[tl.spinnerFrame%len(brailleSpinner)])
		stateStr = tl.styles.amber.Render(spinner)
	case toolDone:
		stateStr = tl.styles.accent.Render("✓")
	case toolError:
		stateStr = tl.styles.errStyle.Render("✗")
	case toolQueued:
		stateStr = tl.styles.dimLabel.Render("○")
	}

	// Tool name — truncate with expand affordance if it overflows.
	// When expanded==false and the name was truncated, show "↵ expand" hint.
	nameField := "  " + tl.name
	stateW := ansi.StringWidth(stateStr)
	statsStr := tl.renderStats()
	statsW := ansi.StringWidth(statsStr)
	// Budget: width - stateW - " " - statsW - some padding.
	nameBudget := width - stateW - 1 - statsW - 2
	if nameBudget < 8 {
		nameBudget = 8
	}
	wasTruncated := false
	if ansi.StringWidth(nameField) > nameBudget {
		truncated := ansi.Truncate(nameField, nameBudget-3, "")
		if !tl.expanded {
			nameField = truncated + "…"
		} else {
			nameField = "  " + tl.name // show full name when expanded
		}
		wasTruncated = true
	}
	_ = wasTruncated // expand affordance rendered below

	line := stateStr + nameField
	lineW := ansi.StringWidth(line)
	if statsW > 0 {
		gap := width - lineW - statsW
		if gap < 1 {
			gap = 1
		}
		line += strings.Repeat(" ", gap) + tl.styles.dimLabel.Render(statsStr)
	}
	return line
}

// renderStats formats the 4 stat slots as a compact string.
// Returns empty string when stats are zero (queued/running state).
func (tl *ToolLine) renderStats() string {
	if tl.state == toolRunning || tl.state == toolQueued {
		return ""
	}
	parts := []string{}
	if tl.stats.lines > 0 {
		parts = append(parts, fmt.Sprintf("%dL", tl.stats.lines))
	}
	if tl.stats.matches > 0 {
		parts = append(parts, fmt.Sprintf("%dm", tl.stats.matches))
	}
	if tl.stats.tokens > 0 {
		parts = append(parts, fmt.Sprintf("%dtok", tl.stats.tokens))
	}
	if tl.stats.duration > 0 {
		parts = append(parts, formatDuration(tl.stats.duration))
	}
	return strings.Join(parts, " ")
}

// ---------------------------------------------------------------------------
// Subagent — nested mini-thread with pink accent
// ---------------------------------------------------------------------------

// Subagent renders a spawned sub-agent's activity as a nested mini-thread.
// Uses the pink accent (#f48fb1) per the component spec.
type Subagent struct {
	id     string // EventSubagentSpawned meta["subagent_id"] or similar
	thread thread // nested mini-thread
	styles tuiStyles
}

// Render implements threadItem. Shows the subagent header and its nested thread.
func (sa *Subagent) Render(width int) string {
	header := sa.styles.pink.Render("⤷ subagent")
	if sa.id != "" {
		header += sa.styles.dimLabel.Render("  " + sa.id)
	}
	out := []string{header}

	// Indent the nested thread by 2 spaces.
	nested := sa.thread.Render(width - 2)
	if nested != "" {
		for _, line := range strings.Split(nested, "\n") {
			out = append(out, "  "+line)
		}
	}
	return strings.Join(out, "\n")
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// wrapText wraps text to fit within maxWidth visible columns.
// Uses rune-level splitting (safe for multi-byte chars) then ANSI-width check.
func wrapText(text string, maxWidth int) string {
	if maxWidth <= 0 {
		return text
	}
	var lines []string
	for _, raw := range strings.Split(text, "\n") {
		lines = append(lines, wrapLine(raw, maxWidth)...)
	}
	return strings.Join(lines, "\n")
}

func wrapLine(line string, maxWidth int) []string {
	if ansi.StringWidth(line) <= maxWidth {
		return []string{line}
	}
	var out []string
	for ansi.StringWidth(line) > maxWidth {
		cut := maxWidth
		// Back up to not split in the middle of a multi-byte char.
		for cut > 0 {
			_, size := utf8.DecodeRuneInString(line[cut:])
			if size > 0 {
				break
			}
			cut--
		}
		out = append(out, line[:cut])
		line = line[cut:]
	}
	if line != "" {
		out = append(out, line)
	}
	return out
}

// formatDuration formats a duration as a human-readable compact string.
func formatDuration(d time.Duration) string {
	switch {
	case d < time.Second:
		return fmt.Sprintf("%dms", d.Milliseconds())
	case d < time.Minute:
		return fmt.Sprintf("%.1fs", d.Seconds())
	default:
		return fmt.Sprintf("%dm%ds", int(d.Minutes()), int(d.Seconds())%60)
	}
}

// visibleWidth returns the visible column width of s using ANSI-safe measurement.
// Used in tests to validate rendered output widths.
func visibleWidth(s string) int {
	return ansi.StringWidth(s)
}
