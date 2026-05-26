package tui

// styles.go — centralized tuiStyles struct and constructor.
// Mirrors the dashStyles / newDashStyles pattern from dashboard.go.
//
// RULE: No hex color literals may appear in any Render function.
// ALL color references go through this struct. Render functions receive
// a tuiStyles value (or pointer to model.styles) and call s.accent.Render(...)
// etc. — never lipgloss.NewStyle().Foreground(lipgloss.Color("#...")) inline.

import "github.com/charmbracelet/lipgloss"

// Theme token values — sourced from components.md §Theme Tokens.
// Defined as package-level constants so tests can compare against them.
const (
	colorBG     = "#0e0f13" // terminal background (dark-only)
	colorAccent = "#5dbfa7" // phosphor teal — primary accent, ⫶ glyph, borders
	colorAmber  = "#ffb347" // mode badge, running tool state
	colorPink   = "#f48fb1" // subagent threads, branch indicators
)

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
	amber    lipgloss.Style // mode / running (#ffb347)
	pink     lipgloss.Style // subagents (#f48fb1)
	errStyle lipgloss.Style // error states — lipgloss terminal color 9 (matches existing errStyle)

	// interactive states
	activeTab   lipgloss.Style
	inactiveTab lipgloss.Style
	selected    lipgloss.Style

	// input bar
	inputBarStyle lipgloss.Style

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

	return tuiStyles{
		topBar: lipgloss.NewStyle().Background(bg).Foreground(lipgloss.Color("#cdd6f4")),
		footer: lipgloss.NewStyle().Faint(true).Italic(true),
		border: lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Padding(0, 1),

		label:    lipgloss.NewStyle().Bold(true),
		dimLabel: lipgloss.NewStyle().Faint(true),
		hint:     lipgloss.NewStyle().Faint(true).Italic(true),

		accent:   lipgloss.NewStyle().Foreground(accent),
		amber:    lipgloss.NewStyle().Foreground(amber),
		pink:     lipgloss.NewStyle().Foreground(pink),
		errStyle: lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Bold(true),

		activeTab:   lipgloss.NewStyle().Bold(true).Underline(true).Foreground(accent),
		inactiveTab: lipgloss.NewStyle().Faint(true),
		selected:    lipgloss.NewStyle().Bold(true).Foreground(accent),

		inputBarStyle: lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Padding(0, 1).BorderForeground(accent),

		dim: lipgloss.NewStyle().Faint(true),
		paletteBox: lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			Padding(0, 1).
			BorderForeground(accent),
	}
}
