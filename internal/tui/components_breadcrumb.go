package tui

// components_breadcrumb.go — chat session breadcrumb (Inc.2).
//
// Renders the session header row shown under the topbar on the chat screen,
// per tui-screens-a.jsx TUI_Chat:
//
//	~/chat/ <label> · <N> turns · tokens <Xk> in · <Yk> out … autosave · <ago>
//
// All fields are sourced from real runtime data accumulated in handleBusEvent
// from EventTokensUsage (turns, in/out tokens, last-turn timestamp). The design
// also shows an "iter N" segment; daimon has no honest session-level iteration
// source (Event.Iteration is the per-turn agentic-loop depth, which resets each
// turn), so that segment is intentionally omitted rather than fabricated.
//
// RULES: no IO in Render; width math via ansi.StringWidth / ansi.Truncate;
// all colors from tuiStyles.

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/charmbracelet/x/ansi"
)

// breadcrumb holds the chat session header data. Value type; updated via
// copy-on-write in Model.Update so the prior model is never mutated.
type breadcrumb struct {
	styles    tuiStyles
	label     string // session label (conv title or short id); "" → "untitled"
	turns     int    // completed turns this session
	tokensIn  int    // cumulative input tokens
	tokensOut int    // cumulative output tokens
	ago       string // pre-computed "autosave ago" string (e.g. "just now"); "" → no autosave segment
}

// hasData reports whether the breadcrumb has anything worth rendering.
func (b breadcrumb) hasData() bool {
	return b.turns > 0 || b.label != ""
}

// Render returns the one-line breadcrumb, or "" when there is no data yet
// (before the first turn). The output never exceeds width visible columns:
// the right-side "autosave · ago" segment is dropped first, then the left
// segment is truncated, so the most important fields survive a narrow rail.
func (b breadcrumb) Render(width int) string {
	if !b.hasData() || width <= 0 {
		return ""
	}

	label := b.label
	if label == "" {
		label = "untitled"
	}

	s := b.styles
	left := s.accent.Render("~/chat/") + " " + s.ink.Bold(true).Render(label) +
		" " + s.inkFaint.Render("·") + " " + s.dimLabel.Render(fmt.Sprintf("%d turns", b.turns)) +
		" " + s.inkFaint.Render("·") + " " + s.dimLabel.Render("tokens") +
		" " + s.inkSoft.Render(formatTokensK(b.tokensIn)+" in") +
		" " + s.inkFaint.Render("·") + " " + s.inkSoft.Render(formatTokensK(b.tokensOut)+" out")

	// The "ago" string is pre-computed in Update (see handleBusEvent) so Render
	// stays pure — no time.Now()/IO here.
	right := ""
	if b.ago != "" {
		right = s.dimLabel.Italic(true).Render("autosave · " + b.ago)
	}

	leftW := ansi.StringWidth(left)
	rightW := ansi.StringWidth(right)

	// Drop the right segment if it doesn't fit alongside the left. After this,
	// right is non-empty only when it fits, so gap is guaranteed ≥ 1 below.
	if right != "" && leftW+1+rightW > width {
		right = ""
	}

	if right == "" {
		return ansi.Truncate(left, width, "…")
	}

	gap := width - leftW - rightW
	return left + strings.Repeat(" ", gap) + right
}

// atoiSafe parses s as an int, returning 0 on any error (e.g. absent meta key).
func atoiSafe(s string) int {
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0
	}
	return n
}

// breadcrumbLabel derives a short session label from the event's conv_id (or
// the model's activeConvID fallback). The "conv_" prefix is stripped and the
// remainder shortened to 8 runes so the breadcrumb stays compact. Title-based
// labels (from the titler) are a future refinement; this is the honest default.
func breadcrumbLabel(convID, fallback string) string {
	id := convID
	if id == "" {
		id = fallback
	}
	id = strings.TrimPrefix(id, "conv_")
	if r := []rune(id); len(r) > 8 {
		id = string(r[:8])
	}
	return id
}

// formatTokensK formats a token count compactly: counts below 1000 render as
// the bare integer; 1000+ render as a one-decimal "k" value (e.g. 34210 →
// "34.2k"), matching the design's "34.2k in · 8.9k out".
func formatTokensK(n int) string {
	if n < 1000 {
		return fmt.Sprintf("%d", n)
	}
	return fmt.Sprintf("%.1fk", float64(n)/1000)
}
