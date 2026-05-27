package tui

// styles.go — centralized tuiStyles struct and constructor.
// Mirrors the dashStyles / newDashStyles pattern from dashboard.go.
//
// RULE: No hex color literals may appear in any Render function.
// ALL color references go through this struct. Render functions receive
// a tuiStyles value (or pointer to model.styles) and call s.accent.Render(...)
// etc. — never lipgloss.NewStyle().Foreground(lipgloss.Color("#...")) inline.

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// ---------------------------------------------------------------------------
// Design token constants — sourced verbatim from docs/tui-design/daimon/project/tui.jsx
// ---------------------------------------------------------------------------
// Every hex value below is a direct copy from the TUI object in tui.jsx.
// Do NOT change these without updating tui.jsx first.

const (
	// Background layers
	colorBG      = "#0e0f13" // terminal background (dark-only)
	colorBGElev  = "#15171d" // elevated surface
	colorBGDeep  = "#0a0b0f" // deep background
	colorBGPanel = "#11131a" // panel background

	// Ink (text) hierarchy
	colorInk      = "#eae5d8" // primary text (warm parchment)
	colorInkSoft  = "#c2bca9" // secondary text
	colorInkMuted = "#7a7465" // muted text
	colorInkFaint = "#4a4438" // faint text
	colorInkGhost = "#2c2a25" // ghost / placeholder text

	// Line (border / divider) hierarchy
	colorLine     = "#22242c" // default border / divider
	colorLineSoft = "#1a1c22" // soft border
	colorLineStr  = "#2e3038" // strong border

	// Accent and semantic colors
	colorAccent = "#5dbfa7" // phosphor teal — primary accent, ⫶ glyph, borders
	colorAmber  = "#e3b67a" // mode badge, running tool state (was #ffb347)
	colorRed    = "#e38775" // error / danger (was ANSI 9)
	colorGreen  = "#7aba8a" // success / ok
	colorPink   = "#d67b9e" // subagent threads, branch indicators (was #f48fb1)
)

// ---------------------------------------------------------------------------
// Deferred rgba tint approximations (for Phase 2 background fill slots)
// ---------------------------------------------------------------------------
// These rgba tokens are defined in tui.jsx but cannot be resolved at parse
// time (alpha blending requires runtime computation). Exact tints will be
// computed in Phase 2.
//
//   accentDim  rgba(93,191,167,0.14)  — exact tint computed in Phase 2
//   accentBg   rgba(93,191,167,0.07)  — exact tint computed in Phase 2
//   amberBg    rgba(227,182,122,0.10) — exact tint computed in Phase 2
//   redBg      rgba(227,135,117,0.10) — exact tint computed in Phase 2
//
// Phase 2: add accentDim, accentBg, amberBg, redBg fields to tuiStyles and
// initialize them with the correct blended values.

// ---------------------------------------------------------------------------
// Glyph constants — sourced from the design spec and components.md
// ---------------------------------------------------------------------------

const (
	glyphDaimon = "⫶" // U+2AF6 — daimon speaker prefix (was δ)
	glyphUser   = "▌" // U+258C — user-line prefix (was "you  ")
	glyphPrompt = "›" // U+203A — prompt indicator
	glyphExpand = "▸" // U+25B8 — expand affordance prefix
)

// ---------------------------------------------------------------------------
// tuiStyles struct
// ---------------------------------------------------------------------------

// tuiStyles holds all Lipgloss styles for the TUI.
// It is constructed once in RunTUI and threaded to all sub-components.
// Adding a new style slot here is the ONLY permitted way to introduce a color.
type tuiStyles struct {
	// chrome
	topBar lipgloss.Style
	footer lipgloss.Style
	border lipgloss.Style

	// text hierarchy
	label    lipgloss.Style
	dimLabel lipgloss.Style
	hint     lipgloss.Style // faint + italic (stage directions in FooterHints)

	// accents
	accent   lipgloss.Style // phosphor teal (#5dbfa7)
	amber    lipgloss.Style // mode / running (#e3b67a)
	pink     lipgloss.Style // subagents (#d67b9e)
	errStyle lipgloss.Style // error states — #e38775
	green    lipgloss.Style // success / ok (#7aba8a)
	inkSoft  lipgloss.Style // secondary text (#c2bca9)

	// interactive states
	activeTab   lipgloss.Style
	inactiveTab lipgloss.Style
	selected    lipgloss.Style

	// input bar
	inputBarStyle lipgloss.Style

	// panel border — square NormalBorder colored with colorLine.
	// Use for all rail/screen panel borders in Phase 1+.
	// Phase 1 introduces this slot and repoints existing borders to NormalBorder.
	panelBorder lipgloss.Style

	// overlay compositing
	dim lipgloss.Style // Faint overlay for the base behind modal dialogs
	// paletteBox is the centralized border style for the command palette.
	paletteBox lipgloss.Style
}

// newTuiStyles constructs the canonical tuiStyles value.
// Called once by RunTUI and by newTestModel() in tests.
func newTuiStyles() tuiStyles {
	accent := lipgloss.Color(colorAccent)
	amber := lipgloss.Color(colorAmber)
	pink := lipgloss.Color(colorPink)
	bg := lipgloss.Color(colorBG)
	ink := lipgloss.Color(colorInk)
	inkMuted := lipgloss.Color(colorInkMuted)
	inkSoft := lipgloss.Color(colorInkSoft)
	red := lipgloss.Color(colorRed)
	green := lipgloss.Color(colorGreen)
	line := lipgloss.Color(colorLine)
	lineStrong := lipgloss.Color(colorLineStr)

	return tuiStyles{
		// topBar: background colorBG, foreground colorInk (warm parchment).
		// Was #cdd6f4 (Catppuccin) — corrected to design token colorInk.
		topBar: lipgloss.NewStyle().Background(bg).Foreground(ink),
		footer: lipgloss.NewStyle().Faint(true).Italic(true),
		// border: square NormalBorder colored with colorLine (was RoundedBorder).
		// NormalBorder has same thickness as RoundedBorder, so off-by-N math unchanged.
		border: lipgloss.NewStyle().Border(lipgloss.NormalBorder()).Padding(0, 1).BorderForeground(line),

		label: lipgloss.NewStyle().Bold(true),
		// dimLabel uses colorInkMuted as an explicit foreground rather than
		// Faint(true), which relies on terminal theme and varies across hosts.
		dimLabel: lipgloss.NewStyle().Foreground(inkMuted),
		hint:     lipgloss.NewStyle().Faint(true).Italic(true),

		accent:   lipgloss.NewStyle().Foreground(accent),
		amber:    lipgloss.NewStyle().Foreground(amber),
		pink:     lipgloss.NewStyle().Foreground(pink),
		errStyle: lipgloss.NewStyle().Foreground(red).Bold(true),
		green:    lipgloss.NewStyle().Foreground(green),
		inkSoft:  lipgloss.NewStyle().Foreground(inkSoft),

		activeTab:   lipgloss.NewStyle().Bold(true).Underline(true).Foreground(accent),
		inactiveTab: lipgloss.NewStyle().Faint(true),
		selected:    lipgloss.NewStyle().Bold(true).Foreground(accent),

		// inputBarStyle: square NormalBorder with colorLineStrong (design: lineStrong for input).
		// Was RoundedBorder with colorAccent. Off-by-N math unchanged (same border thickness).
		inputBarStyle: lipgloss.NewStyle().Border(lipgloss.NormalBorder()).Padding(0, 1).BorderForeground(lineStrong),

		// panelBorder: canonical square border for rail/screen panels.
		// NormalBorder (┌─┐│└─┘) + colorLine (dim border) + Padding(0,1).
		panelBorder: lipgloss.NewStyle().
			Border(lipgloss.NormalBorder()).
			Padding(0, 1).
			BorderForeground(line),

		dim: lipgloss.NewStyle().Faint(true),
		// paletteBox: NormalBorder (square) + accent. Command palette uses accent border
		// per design (tui-components.jsx:441: "1px solid ${TUI.accent}", Outline accent).
		// Do NOT change to line — this has regressed twice; see TestTuiStyles_PaletteBox_BorderIsAccent.
		paletteBox: lipgloss.NewStyle().
			Border(lipgloss.NormalBorder()).
			Padding(0, 1).
			BorderForeground(accent),
	}
}

// panelHeader returns the canonical "── TITLE" panel header string rendered with
// dimLabel style (muted). All panel types must call this instead of inlining
// accent.Render("◈ ...") directly.
//
// Example: s.panelHeader("telemetry") → "── TELEMETRY" (ANSI-colored with dimLabel).
func (s tuiStyles) panelHeader(title string) string {
	return s.dimLabel.Render("── " + strings.ToUpper(title))
}
