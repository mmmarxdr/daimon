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

// chatViewportSize returns the content-area width and height for the chat
// screen's viewport. It mirrors the chrome-reservation math in renderLayout
// so the viewport window matches the slot renderChat occupies.
//
// This is the single source of truth for layout math shared by renderLayout
// and the WindowSizeMsg handler in Model.Update (task 2.6 / design §C.5).
func chatViewportSize(m Model) (vw, vh int) {
	hasRail := HasPanels(m.screen)
	hasInput := inputBarScreens[m.screen]

	topBarHeight := 2
	footerHeight := 2
	inputHeight := 0
	if hasInput {
		inputHeight = 4
	}
	vh = m.height - topBarHeight - footerHeight - inputHeight
	if vh < 0 {
		vh = 0
	}

	vw = m.width
	if hasRail {
		rWidth := railWidth
		if rWidth >= m.width {
			rWidth = m.width / 3
		}
		vw = m.width - rWidth
	}
	return vw, vh
}

// enterChatViewport recomputes the viewport dimensions for the chat screen
// geometry and resets content. Call this AFTER m.screen has been set to
// screenChat so chatViewportSize reads the correct screen state.
//
// This helper is the single call site for chat-entry viewport resets, keeping
// chatViewportSize as the sole source of truth for layout math (design §C.5).
// It mirrors the WindowSizeMsg handler logic so the viewport is always correctly
// sized when entering chat from a screen with different chrome geometry
// (e.g. tools or sessions, which have no input bar and therefore different height).
func (m Model) enterChatViewport() Model {
	vw, vh := chatViewportSize(m)
	m.viewport.Width = vw
	m.viewport.Height = vh
	m.viewport.SetContent("")
	m.viewport.GotoTop()
	return m.refreshThreadViewport()
}

// renderLayout composes the full TUI view for the given model state.
// It is called from Model.View() and is the only place where vertical/horizontal
// joining happens. Screen-specific center content is delegated to renderCenter.
func renderLayout(m Model) string {
	// 1. Determine dimensions.
	hasRail := HasPanels(m.screen)
	hasInput := inputBarScreens[m.screen]

	// 2. Reserve rows for chrome (matches chatViewportSize math).
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
	// WU-a: read the cached mode field — never call the live agent from View.
	// m.mode is set at construction and updated in cycleMode / commandResultMsg(mode).
	// Tests that set neither m.mode nor m.topBar.mode get "" → "BUILD" default from
	// the rendering components; the golden output is unchanged (zero value == build).
	currentMode := m.mode

	// TopBar — set the mode slot from the cached currentMode (m.mode), then render.
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

// welcomeLogo is the ASCII δ block art from docs/tui-design/daimon/project/tui-screens-a.jsx:9–17.
// Stored verbatim; rendered in s.accent and centered as ONE block (a single shared
// left pad keyed to the widest line) so the pyramid keeps a common left edge.
// The art is a pyramid: most lines are 68 cols, lines 3–4 are 69; artWidth is the
// MAX (69). On narrow terminals (width < artWidth) fall back to the "⫶ daimon" mark.
//
// Delta from old tagline: previous code used "your embedded AI agent" (dimLabel).
// Design source uses "speak, and daimon listens." as the tagline in hint/italic style.
var welcomeLogo = []string{
	`       ▄▄▄▄▄                                                        `,
	`     ▄█▀   ▀█▄    ▐█▌                                               `,
	`    █▀       ▀█   ▐█▌  ┌───────────────────────────────────┐        `,
	`   █▀  ▄▄▄    █   ▐█▌  │  daimon · v0.4.2 · MIT             │        `,
	`   █  ▐█ █▌   █   ▐█▌  │  agent runtime, on your hardware   │        `,
	`   █▄  ▀▀▀    █   ▐█▌  └───────────────────────────────────┘        `,
	`    █▄       ▄█   ▐█▌                                               `,
	`     ▀█▄▄▄▄▄█▀     ▀                                                `,
}

// renderWelcomeCenter renders the welcome screen center column.
// At width >= artWidth: renders the ASCII δ logo block (accent color) + tagline (hint style).
// At width < artWidth: falls back to the single-line "⫶ daimon" mark to prevent art wrapping.
func renderWelcomeCenter(m Model, width, height int) string {
	s := m.styles

	// Measure the widest line — the art is a pyramid whose lines differ by a
	// column, so the block width is the MAX, not line[0] (which is the narrowest).
	artWidth := 0
	for _, artLine := range welcomeLogo {
		if w := ansi.StringWidth(artLine); w > artWidth {
			artWidth = w
		}
	}

	if width >= artWidth {
		// Full logo path: 8 art lines + blank + tagline = blockHeight 10.
		blockHeight := len(welcomeLogo) + 1 + 1 // art + blank + tagline
		padTop := (height - blockHeight) / 2
		if padTop < 0 {
			padTop = 0
		}

		// Center the art as a single block: one shared left pad keyed to the
		// widest line, so every line keeps a common left edge. Per-line centering
		// would shift the 69-col lines vs the 68-col lines and break the shape;
		// the design renders the art as one `pre` block (tui-screens-a.jsx:28-32).
		leftPad := (width - artWidth) / 2
		if leftPad < 0 {
			leftPad = 0
		}
		pad := strings.Repeat(" ", leftPad)

		lines := make([]string, 0, padTop+blockHeight)
		for i := 0; i < padTop; i++ {
			lines = append(lines, "")
		}
		for _, artLine := range welcomeLogo {
			lines = append(lines, pad+s.accent.Render(artLine))
		}
		// Blank separator.
		lines = append(lines, "")
		// Tagline: italic hint style — design source uses "speak, and daimon listens."
		lines = append(lines, centerText(s.hint.Render("speak, and daimon listens."), width))
		return strings.Join(lines, "\n")
	}

	// Narrow terminal fallback: single-line "⫶ daimon" mark.
	fallbackLogo := s.accent.Render("⫶ daimon")
	padTop := height / 3
	if padTop < 0 {
		padTop = 0
	}
	lines := make([]string, 0, padTop+2)
	for i := 0; i < padTop; i++ {
		lines = append(lines, "")
	}
	lines = append(lines, centerText(fallbackLogo, width))
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
