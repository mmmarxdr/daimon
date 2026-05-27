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

// Render returns a two-line string:
//
//	Line 1: topBar content with │ separators and colored segments.
//	Line 2: a full-width horizontal rule of ─ in the line/dimLabel color.
//
// Design segments:
//
//	LEFT:  ⫶(accent) daimon(ink) │ path/(inkMuted)+project(inkSoft) · branch(pink) │ model(inkMuted) model(ink) · mode(inkMuted) mode(amber)
//	RIGHT: cost(inkMuted) · ●(accent) status(inkMuted)
func (tb *topBar) Render(width int, s tuiStyles) string {
	pipe := s.inkFaint.Render(" │ ")
	dot := s.inkFaint.Render(" · ")

	// Brand segment: ⫶ (accent) + " daimon" (ink). The brand field holds only
	// the glyph; the "daimon" wordmark is fixed by the design.
	brand := s.accent.Render(tb.brand) + s.ink.Render(" daimon")

	// CWD segment: split into leading path (inkMuted) + final segment (inkSoft).
	cwdSeg := ""
	if tb.cwd != "" {
		// Split at last "/" to get leading path + project name.
		lastSlash := strings.LastIndex(tb.cwd, "/")
		if lastSlash >= 0 && lastSlash < len(tb.cwd)-1 {
			cwdSeg = s.dimLabel.Render(tb.cwd[:lastSlash+1]) + s.inkSoft.Render(tb.cwd[lastSlash+1:])
		} else {
			cwdSeg = s.inkSoft.Render(tb.cwd)
		}
	}

	// Branch segment (only when non-empty).
	branchSeg := ""
	if tb.branch != "" {
		branchSeg = s.pink.Render(tb.branch)
	}

	// Model+mode segment.
	modelSeg := s.dimLabel.Render("model ") + s.ink.Render(tb.model)
	modeSeg := s.dimLabel.Render("mode ") + s.amber.Render(tb.mode)

	// Compose left side.
	left := brand
	if cwdSeg != "" {
		left += pipe + cwdSeg
		if branchSeg != "" {
			left += dot + branchSeg
		}
	}
	left += pipe + modelSeg
	if tb.mode != "" {
		left += dot + modeSeg
	}

	// Right side: cost · ● status.
	right := ""
	if tb.cost != "" {
		right += s.dimLabel.Render(tb.cost)
	}
	if tb.status != "" {
		if right != "" {
			right += dot
		}
		right += s.accent.Render("●") + s.dimLabel.Render(" "+tb.status)
	}

	// Compose the topbar line with right-align spacer.
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

	line1 := left + strings.Repeat(" ", gap) + right
	// Pad/truncate to exactly width.
	line1W := ansi.StringWidth(line1)
	if line1W < width {
		line1 += strings.Repeat(" ", width-line1W)
	}
	line1 = ansi.Truncate(line1, width, "")
	line1 = s.topBar.Render(line1)

	// Line 2: bottom border rule of ─ in dimLabel (colorLine token).
	line2 := s.dimLabel.Render(strings.Repeat("─", width))

	return line1 + "\n" + line2
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

// Render returns a multi-line footer string for the current screen.
// Layout (two rows):
//  1. A full-width horizontal rule of ─ in the line token.
//  2. Left hints (symbol in accent, label in inkMuted) + flexible spacer +
//     right-aligned italic tagline "daimon listens." in inkFaint.
func (fh *footerHints) Render(width int, s tuiStyles) string {
	// Row 1: horizontal rule of ─ characters in colorLine style.
	rule := s.dimLabel.Render(strings.Repeat("─", width))

	// Row 2: hints left + spacer + tagline right.
	hintsStr := fh.renderHints(s)
	taglineStr := s.tagline.Render("daimon listens.")

	hintsW := ansi.StringWidth(hintsStr)
	taglineW := ansi.StringWidth(taglineStr)
	gap := width - hintsW - taglineW
	if gap < 1 {
		gap = 1
	}
	row2 := hintsStr + strings.Repeat(" ", gap) + taglineStr
	// Pad/truncate row2 to exactly width.
	row2W := ansi.StringWidth(row2)
	if row2W < width {
		row2 += strings.Repeat(" ", width-row2W)
	} else if row2W > width {
		row2 = ansi.Truncate(row2, width, "")
	}

	return rule + "\n" + row2
}

// renderHints returns the left-side hint string for the current screen,
// with symbols in accent color and labels in inkMuted.
func (fh *footerHints) renderHints(s tuiStyles) string {
	type hint struct {
		sym   string
		label string
	}

	renderHint := func(sym, label string) string {
		return s.accent.Render(sym) + s.dimLabel.Render(" "+label)
	}
	sep := s.dimLabel.Render("   ")

	switch fh.screen {
	case screenChat:
		// Hints list ONLY keys that handleChatKey actually wires (enter, r, tab→
		// mode, ⌃t→tools, esc, ⌃p+/→palette, ?→help) plus the global ⌃C quit.
		// retry/edit/save are design hints but not yet implemented — do NOT
		// advertise them until they work.
		hints := []hint{
			{"/", "commands"},
			{"⇥", "mode"},
			{"⌃T", "tools"},
			{"?", "help"},
			{"⌃C", "quit"},
		}
		parts := make([]string, len(hints))
		for i, h := range hints {
			parts[i] = renderHint(h.sym, h.label)
		}
		return strings.Join(parts, sep)
	case screenWelcome:
		return renderHint("enter", "send") + sep +
			renderHint("⌃C", "quit") + sep +
			renderHint("⇥", "mode") + sep +
			renderHint("/", "commands") + sep +
			renderHint("^t", "tools")
	case screenDiff:
		return renderHint("↑↓", "scroll hunks") + sep +
			renderHint("q", "back") + sep +
			renderHint("⌃C", "quit")
	case screenSlash:
		return renderHint("↑↓", "navigate") + sep +
			renderHint("enter", "run") + sep +
			renderHint("esc", "close") + sep +
			renderHint("⌃C", "quit")
	case screenTools:
		return renderHint("↑↓", "navigate") + sep +
			renderHint("esc", "back") + sep +
			renderHint("⌃C", "quit")
	case screenSessions:
		return renderHint("↑↓", "navigate") + sep +
			renderHint("enter", "resume") + sep +
			renderHint("esc", "back") + sep +
			renderHint("⌃C", "quit")
	case screenError:
		return renderHint("esc", "back to chat") + sep +
			renderHint("⌃C", "quit")
	default:
		return renderHint("⌃C", "quit")
	}
}

// hintsForScreen returns the plain text hint string for the current screen.
// Kept for backward compatibility with any callers that expect a plain string.
func (fh *footerHints) hintsForScreen() string {
	switch fh.screen {
	case screenWelcome:
		return "enter: send  ctrl+c: quit  tab: mode  /: commands  ^t: tools"
	case screenChat:
		return "⇥ /commands   ⌃C interrupt   ⌃R retry turn   ⌃E edit last   ⌃S save session"
	case screenDiff:
		return "↑↓: scroll hunks  q: back to chat  ctrl+c: quit"
	case screenSlash:
		return "↑↓: navigate  enter: run  esc: close  ctrl+c: quit"
	case screenTools:
		return "↑↓: navigate  esc: back  ctrl+c: quit"
	case screenSessions:
		return "↑↓: navigate  enter: resume  esc: back  ctrl+c: quit"
	case screenError:
		return "esc: back to chat  ctrl+c: quit"
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
	ti.Placeholder = "add a follow-up, or ⌃C to interrupt…"
	ti.CharLimit = 4096
	// Set Prompt to "" so the textinput does not render its own "> " prompt;
	// the sentinel "› " is prepended manually in Render to avoid double-prompt.
	ti.Prompt = ""
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
// Mode pill defaults to "[BUILD MODE]" — use RenderWithMode to supply the
// current agent mode dynamically.
//
// Layout (inside the inputBarStyle border+padding box):
//
//	Row 1: › (accent sentinel) + textinput view
//	Row 2: ⇥ /commands  @ mention file  # add to memory  ⌃R retry  <spacer>  <MODE>  ⇧⇥ switch
func (ib *inputBar) Render(width int, s tuiStyles) string {
	return ib.RenderWithMode(width, s, "")
}

// RenderWithMode renders the input bar with the given mode name in the mode pill.
// Empty mode defaults to "BUILD MODE". Mode names are upper-cased for display.
func (ib *inputBar) RenderWithMode(width int, s tuiStyles, mode string) string {
	// Width math (lipgloss v1.1.0): inputBarStyle.Width(N).Render → outer = N+2.
	// Padding(0,1) is inside N, so text content area = N-2.
	// To get outer = width: N = width-2.  Content area = width-4.
	lipglossW := width - 2
	if lipglossW < 1 {
		lipglossW = 1
	}
	contentW := lipglossW - 2 // text area inside padding
	if contentW < 1 {
		contentW = 1
	}
	// Textinput width: content area minus the "› " sentinel (2 visible chars).
	ib.ti.Width = contentW - 2
	if ib.ti.Width < 1 {
		ib.ti.Width = 1
	}

	// Row 1: accent sentinel + input view.
	row1 := s.accent.Render(inputBarSentinel) + ib.ti.View()

	// Row 2: chips left + spacer + mode pill + switch hint right.
	sep := s.dimLabel.Render("    ")
	chip := func(sym, label string) string {
		return s.accent.Render(sym) + s.dimLabel.Render(" "+label)
	}
	chipsLeft := chip("⇥", "mode") + sep +
		chip("@", "mention file") + sep +
		chip("#", "add to memory") + sep +
		chip("⌃R", "retry")

	// Render the mode pill as inline bracketed text (single line, amber).
	// A lipgloss bordered box would produce 3 lines; inline brackets keep it on one line.
	modeName := strings.ToUpper(mode)
	if modeName == "" {
		modeName = "BUILD"
	}
	modePillStr := s.amber.Render("[" + modeName + " MODE]")
	switchHint := chip("/", "commands")

	chipsLeftW := ansi.StringWidth(chipsLeft)
	modeW := ansi.StringWidth(modePillStr)
	switchW := ansi.StringWidth(switchHint)
	// gap between chips and mode pill + switch hint: at least 1 space.
	gap := contentW - chipsLeftW - modeW - 1 - switchW
	if gap < 1 {
		gap = 1
	}
	row2 := chipsLeft + strings.Repeat(" ", gap) + modePillStr + " " + switchHint

	// Pre-truncate to contentW-1 to avoid lipgloss word-wrap edge case (wraps at Width-1).
	truncW := contentW - 1
	if truncW < 1 {
		truncW = 1
	}
	if ansi.StringWidth(row1) > truncW {
		row1 = ansi.Truncate(row1, truncW, "…")
	}
	if ansi.StringWidth(row2) > truncW {
		row2 = ansi.Truncate(row2, truncW, "…")
	}

	content := row1 + "\n" + row2
	return s.inputBarStyle.Width(lipglossW).Render(content)
}
