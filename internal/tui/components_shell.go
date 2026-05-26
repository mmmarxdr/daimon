package tui

// components_shell.go — persistent shell components present on every screen.
//
// TopBar (all 7 screens), FooterHints (all 7 screens), InputBar (welcome, chat, error).
//
// All Render functions receive width and tuiStyles — zero hex literals here.
// ANSI-safe width math via x/ansi.StringWidth / x/ansi.Truncate.

import (
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

// inputBarSentinel is a fixed placeholder string embedded in the input bar's
// rendered output. Tests use it to assert presence/absence of the bar.
const inputBarSentinel = "› "

// ---------------------------------------------------------------------------
// topBar
// ---------------------------------------------------------------------------

// topBar is the persistent top-chrome component rendered on all 7 screens.
// Slots: ⫶ daimon · cwd · branch · model · mode · cost · status
// Width is budgeted with ansi.StringWidth; slots are truncated with ansi.Truncate.
type topBar struct {
	brand  string
	cwd    string
	branch string
	model  string
	mode   string
	cost   string
	status string
}

// SetData populates all topBar slots. Call before the first Render.
func (tb *topBar) SetData(brand, cwd, branch, model, mode, cost, status string) {
	tb.brand = brand
	tb.cwd = cwd
	tb.branch = branch
	tb.model = model
	tb.mode = mode
	tb.cost = cost
	tb.status = status
}

// Render returns a single-line string of exactly `width` visible columns.
// Slots are separated by " · " and right-side slots are right-aligned.
func (tb *topBar) Render(width int, s tuiStyles) string {
	sep := " · "

	// Left group: brand + cwd + branch
	left := tb.brand
	if tb.cwd != "" {
		left += sep + tb.cwd
	}
	if tb.branch != "" {
		left += sep + tb.branch
	}

	// Right group: model + mode + cost + status
	right := tb.model
	if tb.mode != "" {
		right += sep + tb.mode
	}
	if tb.cost != "" {
		right += sep + tb.cost
	}
	if tb.status != "" {
		right += sep + tb.status
	}

	leftW := ansi.StringWidth(left)
	rightW := ansi.StringWidth(right)
	gap := width - leftW - rightW
	if gap < 1 {
		// Not enough room: truncate left to make space for right.
		maxLeft := width - rightW - 1
		if maxLeft < 0 {
			maxLeft = 0
		}
		left = ansi.Truncate(left, maxLeft, "…")
		gap = 1
	}

	line := s.accent.Render(left) + strings.Repeat(" ", gap) + s.dimLabel.Render(right)
	// Pad or truncate to exactly width visible columns.
	lineW := ansi.StringWidth(line)
	if lineW < width {
		line += strings.Repeat(" ", width-lineW)
	}
	return s.topBar.Render(ansi.Truncate(line, width, ""))
}

// ---------------------------------------------------------------------------
// footerHints
// ---------------------------------------------------------------------------

// footerHints is the persistent footer bar rendered on all 7 screens.
// Content is contextual to the active screen's keymap.
type footerHints struct {
	screen screenState
}

// SetScreen updates the active screen so Render returns the correct hints.
func (fh *footerHints) SetScreen(s screenState) {
	fh.screen = s
}

// Render returns a single-line footer hint string for the current screen.
func (fh *footerHints) Render(width int, s tuiStyles) string {
	hints := fh.hintsForScreen()
	return s.hint.Render(ansi.Truncate(hints, width, "…"))
}

func (fh *footerHints) hintsForScreen() string {
	switch fh.screen {
	case screenWelcome:
		return "enter: send  ctrl+c: quit  tab: sessions  ^t: tools"
	case screenChat:
		return "enter: send  /: commands  tab: sessions  ^t: tools  ctrl+c: quit"
	case screenDiff:
		return "↑↓: scroll hunks  q: back to chat  ctrl+c: quit"
	case screenSlash:
		return "↑↓: navigate  enter: run  esc: close  ctrl+c: quit"
	case screenTools:
		return "↑↓: navigate  esc: back  ctrl+c: quit"
	case screenSessions:
		return "↑↓: navigate  enter: resume  esc: back  ctrl+c: quit"
	case screenError:
		return "enter: continue  q: back to chat  ctrl+c: quit"
	default:
		return "ctrl+c: quit"
	}
}

// ---------------------------------------------------------------------------
// inputBar
// ---------------------------------------------------------------------------

// inputBar wraps bubbles/textinput to provide the prompt input bar.
// Shown on welcome (01), chat (02), and error (07) per the panel matrix.
// Hidden on diff (03), slash (04), tools (05), sessions (06).
type inputBar struct {
	ti textinput.Model
}

// newInputBar constructs a ready-to-use inputBar.
func newInputBar() inputBar {
	ti := textinput.New()
	ti.Placeholder = "message daimon…"
	ti.CharLimit = 4096
	ti.Focus()
	return inputBar{ti: ti}
}

// Update forwards tea.Msg to the underlying textinput model.
// Returns the updated inputBar and any tea.Cmd (e.g. blink).
func (ib inputBar) Update(msg tea.Msg) (inputBar, tea.Cmd) {
	var cmd tea.Cmd
	ib.ti, cmd = ib.ti.Update(msg)
	return ib, cmd
}

// Value returns the current text content of the input field.
func (ib inputBar) Value() string {
	return ib.ti.Value()
}

// Reset clears the input field.
func (ib *inputBar) Reset() {
	ib.ti.Reset()
}

// Focus gives keyboard focus to the input bar.
func (ib *inputBar) Focus() {
	ib.ti.Focus()
}

// Blur removes keyboard focus from the input bar.
func (ib *inputBar) Blur() {
	ib.ti.Blur()
}

// Render returns the rendered input bar capped to `width` visible columns.
// The sentinel string inputBarSentinel is always present so tests can detect it.
func (ib *inputBar) Render(width int, s tuiStyles) string {
	ib.ti.Width = width - 4 // subtract border + padding (2 each side)
	if ib.ti.Width < 1 {
		ib.ti.Width = 1
	}
	inner := inputBarSentinel + ib.ti.View()
	return s.inputBarStyle.Width(width - 2).Render(inner)
}
