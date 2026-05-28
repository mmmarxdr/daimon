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
	"github.com/charmbracelet/lipgloss"
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

// Render renders all items joined by newlines. A blank separator line is
// inserted before each user turn (every *MsgUser except the first rendered
// item) so consecutive turns get breathing room, matching the design's
// per-turn marginBottom — without splitting a daimon turn from the tool lines
// that follow it as sibling items.
func (t *thread) Render(width int) string {
	if len(t.items) == 0 {
		return ""
	}
	parts := make([]string, 0, len(t.items))
	for _, item := range t.items {
		s := item.Render(width)
		if s == "" {
			continue
		}
		if _, isUser := item.(*MsgUser); isUser && len(parts) > 0 {
			parts = append(parts, "")
		}
		parts = append(parts, s)
	}
	return strings.Join(parts, "\n")
}

// findToolLine returns the first ToolLine with the given callID, or nil.
// NOTE: callers that mutate the returned ToolLine MUST use findToolLineIdx
// and copy-on-write to avoid pointer-aliasing into the prior model's slice.
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

// findToolLineIdx returns the slice index of the first ToolLine with the given
// callID, or -1 if not found. Use this for copy-on-write mutations.
func (t *thread) findToolLineIdx(callID string) int {
	for i, item := range t.items {
		if tl, ok := item.(*ToolLine); ok {
			if tl.callID == callID {
				return i
			}
		}
	}
	return -1
}

// ---------------------------------------------------------------------------
// MsgUser — user turn bubble
// ---------------------------------------------------------------------------

// MsgUser renders a user-submitted message in the chat thread.
// Per tui.jsx MsgUser: a speaker header row ("▌ name · time") followed by the
// body indented under it. name defaults to "you" when empty; time is omitted
// from the header when empty.
type MsgUser struct {
	text   string
	name   string // speaker name; "" → "you"
	time   string // "HH:MM"; "" → header shows no "· time" segment
	styles tuiStyles
}

// bodyIndent is the column indent applied to message bodies so they sit under
// the speaker header (matches tui.jsx paddingLeft: 14 ≈ 2 terminal cols).
const bodyIndent = "  "

// Render implements threadItem. Renders the speaker header then the indented body.
func (m *MsgUser) Render(width int) string {
	name := m.name
	if name == "" {
		name = "you"
	}
	header := speakerHeader(m.styles,
		m.styles.inkSoft.Render(glyphUser),
		m.styles.ink.Bold(true).Render(name),
		"", m.time)

	return joinHeaderBody(header, m.text, width, m.styles.ink)
}

// speakerHeader builds a "<glyph> <name>[ <suffix>][ · <time>]" header line.
// suffix is an optional italic-muted word (e.g. "speaks" for daimon); empty
// suffix and empty time are each dropped so the header stays clean.
func speakerHeader(s tuiStyles, glyph, name, suffix, t string) string {
	parts := []string{glyph, name}
	if suffix != "" {
		parts = append(parts, s.dimLabel.Italic(true).Render(suffix))
	}
	if t != "" {
		parts = append(parts, s.inkFaint.Render("·"), s.dimLabel.Render(t))
	}
	return strings.Join(parts, " ")
}

// joinHeaderBody wraps body to the available width and indents each body line
// under the header by bodyIndent, rendering the body with bodyStyle.
func joinHeaderBody(header, body string, width int, bodyStyle lipgloss.Style) string {
	indentW := ansi.StringWidth(bodyIndent)
	inner := wrapText(body, width-indentW)
	out := []string{header}
	for _, line := range strings.Split(inner, "\n") {
		out = append(out, bodyIndent+bodyStyle.Render(line))
	}
	return strings.Join(out, "\n")
}

// ---------------------------------------------------------------------------
// MsgDaimon — assistant turn bubble
// ---------------------------------------------------------------------------

// MsgDaimon renders a completed assistant message in the chat thread.
// It appears on chat (02) and error (07) screens per the panel matrix.
// Per tui.jsx MsgDaimon: a "⫶ daimon speaks · time" header followed by the
// indented body. time is omitted from the header when empty.
type MsgDaimon struct {
	text   string
	time   string // "HH:MM"; "" → header shows no "· time" segment
	styles tuiStyles
}

// Render implements threadItem. Renders the speaker header then the indented body.
func (m *MsgDaimon) Render(width int) string {
	header := speakerHeader(m.styles,
		m.styles.accent.Render(glyphDaimon),
		m.styles.accent.Bold(true).Render("daimon"),
		"speaks", m.time)

	return joinHeaderBody(header, m.text, width, m.styles.ink)
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
// for this ToolLine after a 100ms delay, advancing the spinner frame on the
// next Update. The delay is essential — without it the pump spins at 100% CPU.
// Only meaningful when state == toolRunning.
func (tl *ToolLine) Tick() tea.Cmd {
	id := tl.callID
	return tea.Tick(100*time.Millisecond, func(time.Time) tea.Msg {
		return spinnerTickMsg{callID: id}
	})
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
	// When expanded==false and the name was truncated, show "▸ view" hint.
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

	line := stateStr + nameField
	lineW := ansi.StringWidth(line)
	if statsW > 0 {
		gap := width - lineW - statsW
		if gap < 1 {
			gap = 1
		}
		line += strings.Repeat(" ", gap) + tl.styles.dimLabel.Render(statsStr)
	}

	// Expand affordance: when the name was truncated and the tool is not
	// yet expanded, append a "▸ view" hint on its own line so the user
	// knows they can expand it.
	if wasTruncated && !tl.expanded {
		hint := tl.styles.dimLabel.Render("  " + glyphExpand + " view")
		hint = ansi.Truncate(hint, width, "")
		line += "\n" + hint
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
// Uses the pink accent (#d67b9e) per the component spec.
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
		// ansi.Truncate takes the first maxWidth VISUAL columns without cutting
		// mid-rune or mid-ANSI-escape sequence.
		segment := ansi.Truncate(line, maxWidth, "")
		out = append(out, segment)
		// Advance the remainder by the byte length of the consumed segment so
		// we never re-process the same bytes (safe: Truncate preserves UTF-8).
		line = line[len(segment):]
	}
	// FIX 2: ansi.Truncate may leave a residual ANSI reset sequence (\x1b[0m)
	// as the remainder, which has zero visible width. Skip it to avoid a
	// spurious zero-width trailing segment in the output.
	if line != "" && ansi.StringWidth(line) > 0 {
		out = append(out, line)
	}
	return out
}

// nowHHMM returns the current wall-clock time formatted as "HH:MM" for use in
// message speaker headers. Called from Update (never from Render) so it does
// not break render determinism; golden tests construct messages without a time.
func nowHHMM() string { return time.Now().Format("15:04") }

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
