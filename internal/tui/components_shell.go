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

// footerHint holds a single key+label pair for footer rendering.
// key is rendered with s.accent (e.g. "⇥"); label with s.dimLabel (e.g. "switch agent").
type footerHint struct {
	key   string
	label string
}

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
//  2. Left hints (key in accent, label in dimLabel) + flexible spacer +
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

// hintsForScreen returns the canonical []footerHint for each screen.
//
// Per-screen hint sets are sourced verbatim from the design's outer TUIFooter in
// docs/tui-design/daimon/project/tui-screens-a.jsx and tui-screens-b.jsx.
// Where the spec.md inferred a different set, the DESIGN SOURCE is canonical;
// differences are noted below.
//
// Note on deferred actions: some hint keys reference features not yet wired
// (e.g. ⌃P palette on welcome, ↵ open-detail on tools). The hint TEXT is
// rendered faithfully; the keybinding behavior is deferred to the appropriate
// phase. This is intentional per the Phase 1 scope boundary.
func (fh *footerHints) hintsForScreen() []footerHint {
	switch fh.screen {
	// Screen 01 — Welcome
	// Design source: tui-screens-a.jsx:103-108 (outer TUIFooter).
	// Delta from spec: spec had "⇥ /commands · ⌃R resume last · ⌃C exit"
	// (those are the INLINE nav hints in the center div, not the outer TUIScreen footer).
	// Canonical outer footer: / commands · ⇥ switch agent · ⌃P palette · ? help
	case screenWelcome:
		return []footerHint{
			{key: "/", label: "commands"},
			{key: "⇥", label: "switch agent"},
			{key: "⌃P", label: "palette"},
			{key: "?", label: "help"},
		}

	// Screen 02 — Chat
	// Design source: tui-screens-a.jsx:302-308 (outer TUIFooter).
	// Note: ⌃C/⌃R/⌃E/⌃S are design hints; wiring is phased. Text rendered as-is per Phase 1 scope.
	case screenChat:
		return []footerHint{
			{key: "⇥", label: "/commands"},
			{key: "⌃C", label: "interrupt"},
			{key: "⌃R", label: "retry turn"},
			{key: "⌃E", label: "edit last"},
			{key: "⌃S", label: "save session"},
		}

	// Screen 03 — Diff (Phase 3 screen — render hints per design, no new keybindings wired)
	// Design source: tui-screens-a.jsx:489-495 (outer TUIFooter).
	case screenDiff:
		return []footerHint{
			{key: "a/A", label: "apply / apply-all"},
			{key: "r", label: "reject hunk"},
			{key: "e", label: "open in $EDITOR"},
			{key: "n/p", label: "next/prev hunk"},
			{key: "q", label: "cancel patch"},
		}

	// Screen 04 — Slash palette
	// Design source: tui-screens-b.jsx:153-157 (outer TUIFooter).
	// Delta from spec: spec inferred hints from the inner palette overlay footer
	// (↑↓ select · ↵ run · esc close · ⇥ autocomplete). The outer screen TUIFooter
	// has only 3 entries: esc close palette · / search prefix · ? help
	case screenSlash:
		return []footerHint{
			{key: "esc", label: "close palette"},
			{key: "/", label: "search prefix"},
			{key: "?", label: "help"},
		}

	// Screen 05 — Tools & MCPs
	// Design source: tui-screens-b.jsx:319-325 (outer TUIFooter).
	// Delta from spec: spec had "↑↓ select · ↵ toggle · f filter · a add-MCP"
	// Canonical footer: space toggle enabled · ↵ open detail · a add MCP server · d remove · / filter
	case screenTools:
		return []footerHint{
			{key: "space", label: "toggle enabled"},
			{key: "↵", label: "open detail"},
			{key: "a", label: "add MCP server"},
			{key: "d", label: "remove"},
			{key: "/", label: "filter"},
		}

	// Screen 06 — Sessions browser
	// Design source: tui-screens-b.jsx:492-497 (outer TUIFooter).
	// Mostly matches spec except label differences: "↵ resume thread" vs "↵ open",
	// "n new from this" vs "n new", "m change model" vs "m model".
	case screenSessions:
		return []footerHint{
			{key: "↵", label: "resume thread"},
			{key: "n", label: "new from this"},
			{key: "d", label: "delete"},
			{key: "m", label: "change model"},
			{key: "/", label: "filter"},
		}

	// Screen 07 — Error / permission denied (Phase 3 screen — render hints per design)
	// Design source: tui-screens-b.jsx:682-687 (outer TUIFooter).
	case screenError:
		return []footerHint{
			{key: "a/A", label: "allow once / always"},
			{key: "d/D", label: "deny / never ask"},
			{key: "e", label: "edit path"},
			{key: "p", label: "open policy file"},
		}

	default:
		return []footerHint{
			{key: "⌃C", label: "quit"},
		}
	}
}

// renderHints returns the left-side hint string for the current screen,
// with keys in accent color and labels in dimLabel color, separated by two spaces.
func (fh *footerHints) renderHints(s tuiStyles) string {
	hints := fh.hintsForScreen()
	sep := s.dimLabel.Render("   ")

	parts := make([]string, 0, len(hints))
	for _, h := range hints {
		parts = append(parts, s.accent.Render(h.key)+s.dimLabel.Render(" "+h.label))
	}
	return strings.Join(parts, sep)
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
