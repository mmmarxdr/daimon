package tui

// layout.go — layout math and the main renderLayout function.
//
// Layout hierarchy (top → bottom):
//   TopBar (1 row)
//   main area (flex height) split horizontally:
//     center column (width - railWidth when rail present)
//     right rail (railWidth cols, only when panelsFor(screen) is non-empty)
//   InputBar (shown on welcome, chat, error — per matrix)
//   FooterHints (1 row)
//
// Off-by-N rule (AD-10): every bordered box subtracts border+padding before
// passing width to children. A NormalBorder+Padding(0,1) steals 4 columns.
// ALL width math uses ansi.StringWidth — never len() or byte slicing.

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// inputBarScreens is the set of screens where the InputBar is shown.
// Authoritative — matches components.md §Matrix (welcome, chat, error).
var inputBarScreens = map[screenState]bool{
	screenWelcome: true,
	screenChat:    true,
	screenError:   true,
}

// renderLayout composes the full TUI view for the given model state.
// It is called from Model.View() and is the only place where vertical/horizontal
// joining happens. Screen-specific center content is delegated to renderCenter.
func renderLayout(m Model) string {
	// 1. Determine dimensions.
	hasRail := HasPanels(m.screen)
	hasInput := inputBarScreens[m.screen]

	// 2. Reserve rows for chrome.
	// topBar now renders 2 lines: content row + bottom border rule.
	topBarHeight := 2
	// footer now renders 2 lines: top border rule + hints row.
	footerHeight := 2
	inputHeight := 0
	if hasInput {
		inputHeight = 4 // NormalBorder top + 2 content rows (input + chips) + NormalBorder bottom
	}
	centerHeight := m.height - topBarHeight - footerHeight - inputHeight
	if centerHeight < 0 {
		centerHeight = 0
	}

	// 3. Horizontal split of center area.
	centerWidth := m.width
	rWidth := 0
	if hasRail {
		rWidth = railWidth
		if rWidth >= m.width {
			rWidth = m.width / 3
		}
		centerWidth = m.width - rWidth
	}

	// 4. Render each zone.
	// Resolve the current mode from modeAgent (live) or topBar.mode (startup snapshot).
	// modeAgent is nil in tests that don't inject an agent; fall back to topBar.mode.
	currentMode := m.topBar.mode
	if m.modeAgent != nil {
		currentMode = m.modeAgent.CurrentMode()
	}

	// TopBar — update the mode slot dynamically from the live modeAgent, then render.
	tb := m.topBar
	tb.mode = currentMode
	topRendered := tb.Render(m.width, m.styles)

	// Center content.
	center := renderCenter(m, centerWidth, centerHeight)

	// Rail.
	railRendered := ""
	if hasRail {
		railRendered = m.rail.Render(m.screen, rWidth, centerHeight)
	}

	// Compose center + rail horizontally.
	mainRow := center
	if hasRail && railRendered != "" {
		mainRow = lipgloss.JoinHorizontal(lipgloss.Top, center, railRendered)
	}

	// InputBar — pass the live mode so the mode pill reflects the current mode.
	inputRendered := ""
	if hasInput {
		ib := m.input
		inputRendered = ib.RenderWithMode(m.width, m.styles, currentMode)
	}

	// FooterHints.
	fh := m.footer
	footerRendered := fh.Render(m.width, m.styles)

	// 5. Stack vertically.
	parts := []string{topRendered, mainRow}
	if inputRendered != "" {
		parts = append(parts, inputRendered)
	}
	parts = append(parts, footerRendered)

	return strings.Join(parts, "\n")
}

// renderCenter returns the center-column content for the current screen.
// PR2 wires chat; PR3b wires sessions; other screens return a placeholder.
func renderCenter(m Model, width, height int) string {
	switch m.screen {
	case screenWelcome:
		return renderWelcomeCenter(m, width, height)
	case screenChat:
		return renderChat(m, width, height)
	case screenSessions:
		return renderSessions(m, width, height)
	case screenTools:
		return renderTools(m, width, height) // PR4a
	case screenError:
		return renderError(m, width, height) // PR5
	default:
		// Stub: later PRs replace these with screen-specific render functions.
		return renderCenterPlaceholder(m.screen, width, height)
	}
}

// renderWelcomeCenter renders the welcome screen center column:
// the ⫶ daimon logo centered, with no thread content.
func renderWelcomeCenter(m Model, width, height int) string {
	logo := m.styles.accent.Render("⫶ daimon")
	// Center the logo horizontally and vertically.
	padTop := height / 3
	if padTop < 0 {
		padTop = 0
	}
	lines := make([]string, 0, padTop+3)
	for i := 0; i < padTop; i++ {
		lines = append(lines, "")
	}
	lines = append(lines, centerText(logo, width))
	lines = append(lines, centerText(m.styles.dimLabel.Render("your embedded AI agent"), width))
	lines = append(lines, "")
	return strings.Join(lines, "\n")
}

// renderCenterPlaceholder returns a minimal placeholder for unimplemented screens.
func renderCenterPlaceholder(s screenState, width, _ int) string {
	label := "[" + s.String() + " — coming in a later PR]"
	return centerText(label, width)
}

// centerText centers `s` within `width` columns using space padding.
// Uses ansi.StringWidth so ANSI-escaped strings (e.g. lipgloss-rendered
// colored text) are measured by their visible column width, not raw rune count.
func centerText(s string, width int) string {
	slen := ansi.StringWidth(s)
	if slen >= width {
		return s
	}
	pad := (width - slen) / 2
	return strings.Repeat(" ", pad) + s
}
