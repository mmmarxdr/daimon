package tui

// rail_panels.go — concrete right-rail panel structs for PR2b and PR4b.
//
// Implements panel types for multiple screens:
//   - telemetryPanel   — tokens / cost / tool-call counts from EventTokensUsage / EventToolStart/End
//   - todolistPanel    — LLM-generated todo list from Agent.TodoListForConv
//   - contextMeterPanel — context usage from EventTokensUsage (token count used as proxy)
//   - modelPickerPanel  — active model display (read-only V1, sessions screen)
//   - toolDetailPanel   — selected tool detail (PR4a: tools screen)
//   - environmentPanel  — cwd / model / go / os / store (PR4b: welcome screen)
//   - resumeListPanel   — recent sessions (PR4b: welcome + sessions screens)
//
// Panel contract (AD-6 / REQ-7):
//   - Render(width, height int) string
//   - Panels with no data MUST return "" (zero height).
//   - No hex literals in Render; all styles from tuiStyles.
//   - ANSI width math via ansi.StringWidth / ansi.Truncate — never len() or byte-slice.
//
// Thread-safety: panel structs are mutated only in Update (via copy-on-write rail
// replacement) so no synchronization is needed inside Render.

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/x/ansi"

	"daimon/internal/notify"
	"daimon/internal/store"
	"daimon/internal/tool"
)

// ---------------------------------------------------------------------------
// telemetryPanel — tokens / cost / tool-call counts
// ---------------------------------------------------------------------------

// telemetryPanel accumulates EventTokensUsage and tool lifecycle events
// and renders a compact token/cost summary in the right rail.
type telemetryPanel struct {
	styles     tuiStyles
	totalIn    int     // cumulative token count from EventTokensUsage
	totalCost  float64 // cumulative cost in USD
	toolCalls  int     // total tool calls started (EventToolStart)
	toolErrors int     // tool calls that ended with IsError (EventToolEnd)
	hasData    bool    // true once at least one EventTokensUsage has been received
}

// newTelemetryPanel constructs a telemetryPanel with zero state.
func newTelemetryPanel(s tuiStyles) *telemetryPanel {
	return &telemetryPanel{styles: s}
}

// accumulate processes a single notify.Event and updates the panel state.
// Called from handleBusEvent in screen_chat.go — runs on the model's Update path
// (via copy-on-write rail replacement).
func (p *telemetryPanel) accumulate(ev notify.Event) {
	switch ev.Type {
	case notify.EventTokensUsage:
		p.totalIn += ev.TokenCount
		p.totalCost += ev.CostUSD
		p.hasData = true
	case notify.EventToolStart:
		p.toolCalls++
	case notify.EventToolEnd:
		if ev.IsError {
			p.toolErrors++
		}
	}
}

// Render implements Panel. Returns "" when no EventTokensUsage has been received.
func (p *telemetryPanel) Render(width, _ int) string {
	if !p.hasData {
		return ""
	}

	// Inner width: no manual margin is prepended; just clamp to a minimum.
	inner := width - 1
	if inner < 4 {
		inner = 4
	}

	header := p.styles.accent.Render("◈ telemetry")
	tokLine := fmt.Sprintf("tokens  %d", p.totalIn)
	costLine := fmt.Sprintf("cost    $%.4f", p.totalCost)
	toolLine := fmt.Sprintf("tools   %d", p.toolCalls)

	rows := []string{
		ansi.Truncate(header, inner, "…"),
		ansi.Truncate(p.styles.dimLabel.Render(tokLine), inner, "…"),
		ansi.Truncate(p.styles.dimLabel.Render(costLine), inner, "…"),
		ansi.Truncate(p.styles.dimLabel.Render(toolLine), inner, "…"),
	}

	// Render error count only when non-zero, using errStyle (no inline hex).
	if p.toolErrors > 0 {
		errLine := fmt.Sprintf("errors  %d", p.toolErrors)
		rows = append(rows, ansi.Truncate(p.styles.errStyle.Render(errLine), inner, "…"))
	}

	return strings.Join(rows, "\n")
}

// ---------------------------------------------------------------------------
// todolistPanel — LLM-generated todo list
// ---------------------------------------------------------------------------

// todolistPanel renders the agent's current todo list for the active conversation.
// Data is refreshed via a tea.Cmd when EventTodolistChanged is received.
type todolistPanel struct {
	styles tuiStyles
	list   tool.TodoList
}

// newTodolistPanel constructs a todolistPanel with empty state.
func newTodolistPanel(s tuiStyles) *todolistPanel {
	return &todolistPanel{styles: s}
}

// setList replaces the current todo list. Called from handleTodolistRefresh.
func (p *todolistPanel) setList(list tool.TodoList) {
	p.list = list
}

// Render implements Panel. Returns "" when there are no todo items.
func (p *todolistPanel) Render(width, _ int) string {
	if len(p.list.Items) == 0 {
		return ""
	}

	inner := width - 1
	if inner < 4 {
		inner = 4
	}

	rows := []string{
		ansi.Truncate(p.styles.accent.Render("◈ todo"), inner, "…"),
	}
	for _, item := range p.list.Items {
		var marker string
		switch item.Status {
		case "done", "completed":
			marker = p.styles.accent.Render("✓")
		case "in_progress":
			marker = p.styles.amber.Render("●")
		default:
			marker = p.styles.dimLabel.Render("○")
		}
		line := marker + " " + item.Content
		rows = append(rows, ansi.Truncate(line, inner, "…"))
	}
	return strings.Join(rows, "\n")
}

// ---------------------------------------------------------------------------
// modelPickerPanel — current active model display (read-only V1)
// ---------------------------------------------------------------------------

// modelPickerPanel displays the active model and provider in the right rail
// for the sessions screen. V1 is read-only: switching models requires /model
// via the command palette (deferred). Renders "" when provider or model is empty.
type modelPickerPanel struct {
	styles   tuiStyles
	provider string // e.g. "anthropic"
	model    string // e.g. "claude-opus-4-5"
}

// newModelPickerPanel constructs a modelPickerPanel.
// provider and model are passed from cfg.Models.Default at construction time.
func newModelPickerPanel(s tuiStyles, provider, model string) *modelPickerPanel {
	return &modelPickerPanel{styles: s, provider: provider, model: model}
}

// Render implements Panel. Returns "" when provider or model is not configured.
func (p *modelPickerPanel) Render(width, _ int) string {
	if p.provider == "" || p.model == "" {
		return ""
	}

	inner := width - 1
	if inner < 4 {
		inner = 4
	}

	header := p.styles.accent.Render("◈ model")
	provLine := ansi.Truncate(p.styles.dimLabel.Render(p.provider), inner, "…")
	modelLine := ansi.Truncate(p.styles.dimLabel.Render(p.model), inner, "…")

	return strings.Join([]string{header, provLine, modelLine}, "\n")
}

// ---------------------------------------------------------------------------
// toolDetailPanel — selected tool detail (PR4a: tools screen)
// ---------------------------------------------------------------------------

// toolDetailPanel renders the detail view for the currently selected tool in
// the tools screen right rail. Renders "" when no tool has been set yet.
type toolDetailPanel struct {
	styles  tuiStyles
	name    string
	desc    string
	meta    tool.ToolMeta
	hasData bool
}

// newToolDetailPanel constructs a toolDetailPanel with empty state.
func newToolDetailPanel(s tuiStyles) *toolDetailPanel {
	return &toolDetailPanel{styles: s}
}

// setTool replaces the current tool detail. Called from updateToolDetailPanel
// via copyRailWith on navigation or toolsLoadedMsg.
func (p *toolDetailPanel) setTool(name, desc string, meta tool.ToolMeta) {
	p.name = name
	p.desc = desc
	p.meta = meta
	p.hasData = true
}

// Render implements Panel. Returns "" when no tool has been set.
func (p *toolDetailPanel) Render(width, _ int) string {
	if !p.hasData {
		return ""
	}

	inner := width - 1
	if inner < 4 {
		inner = 4
	}

	header := p.styles.accent.Render("◈ tool detail")
	nameLine := ansi.Truncate(p.styles.label.Render(p.name), inner, "…")

	// Description: truncate to a single visible line.
	const maxDesc = 80
	descText := p.desc
	if ansi.StringWidth(descText) > maxDesc {
		descText = ansi.Truncate(descText, maxDesc, "…")
	}
	descLine := ansi.Truncate(p.styles.dimLabel.Render(descText), inner, "…")

	riskLine := ansi.Truncate(p.styles.dimLabel.Render("risk:       "+string(p.meta.Risk)), inner, "…")
	catLine := ansi.Truncate(p.styles.dimLabel.Render("category:   "+string(p.meta.Category)), inner, "…")
	permLine := ansi.Truncate(p.styles.dimLabel.Render("permission: "+string(p.meta.Permission)), inner, "…")
	srcLine := ansi.Truncate(p.styles.dimLabel.Render("source:     "+string(p.meta.Source)), inner, "…")

	rows := []string{
		ansi.Truncate(header, inner, "…"),
		nameLine,
		"",
		descLine,
		"",
		riskLine,
		catLine,
		permLine,
		srcLine,
	}

	return strings.Join(rows, "\n")
}

// ---------------------------------------------------------------------------
// contextMeterPanel — context window usage
// ---------------------------------------------------------------------------

// contextMeterPanel renders a visual indicator of how much of the context
// window is consumed. In V1 the data source is EventTokensUsage.TokenCount —
// cumulative output tokens accumulated across ALL turns in this session (NOT
// the live context-window fill, which is not exposed by the backend in V1).
// A 200k heuristic (Claude 3/4 flagship) is used to compute the percentage bar.
//
// If there is no live source for context-window limits, the panel still shows
// the raw token count so it is always useful without a backend change.
type contextMeterPanel struct {
	styles    tuiStyles
	tokenUsed int // cumulative output tokens from all EventTokensUsage events this session
	hasData   bool
}

// newContextMeterPanel constructs a contextMeterPanel with zero state.
func newContextMeterPanel(s tuiStyles) *contextMeterPanel {
	return &contextMeterPanel{styles: s}
}

// accumulate processes EventTokensUsage events.
func (p *contextMeterPanel) accumulate(ev notify.Event) {
	if ev.Type == notify.EventTokensUsage {
		p.tokenUsed += ev.TokenCount
		p.hasData = true
	}
}

// Render implements Panel. Returns "" until at least one token event is received.
func (p *contextMeterPanel) Render(width, _ int) string {
	if !p.hasData {
		return ""
	}

	inner := width - 1
	if inner < 8 {
		inner = 8
	}

	// Heuristic context limit (Claude 3/4 flagship: 200k tokens).
	const contextLimit = 200_000
	pct := float64(p.tokenUsed) / float64(contextLimit)
	if pct > 1.0 {
		pct = 1.0
	}

	// Render a simple bar using fill characters.
	barWidth := inner - 2 // leave room for brackets
	if barWidth < 2 {
		barWidth = 2
	}
	filled := int(pct * float64(barWidth))
	bar := strings.Repeat("█", filled) + strings.Repeat("░", barWidth-filled)

	header := p.styles.accent.Render("◈ context")
	barLine := "[" + bar + "]"
	pctLine := fmt.Sprintf("%.1f%% of 200k", pct*100)

	rows := []string{
		ansi.Truncate(header, inner, "…"),
		ansi.Truncate(barLine, inner, "…"),
		ansi.Truncate(p.styles.dimLabel.Render(pctLine), inner, "…"),
	}
	return strings.Join(rows, "\n")
}

// ---------------------------------------------------------------------------
// environmentPanel — cwd / model / go / os / store (PR4b: welcome screen)
// ---------------------------------------------------------------------------

// environmentPanel renders a compact key/value block for the welcome screen
// showing the runtime environment: cwd, active model, Go version, OS/arch,
// and store type. Renders "" only when ALL fields are empty.
type environmentPanel struct {
	styles    tuiStyles
	cwd       string // working directory
	model     string // "provider/model" string
	goVersion string // runtime.Version()
	osArch    string // "GOOS/GOARCH"
	storeType string // cfg.Store.Type
}

// newEnvironmentPanel constructs an environmentPanel with the given values.
func newEnvironmentPanel(s tuiStyles, cwd, model, goVersion, osArch, storeType string) *environmentPanel {
	return &environmentPanel{
		styles:    s,
		cwd:       cwd,
		model:     model,
		goVersion: goVersion,
		osArch:    osArch,
		storeType: storeType,
	}
}

// Render implements Panel. Returns "" when all fields are empty.
func (p *environmentPanel) Render(width, _ int) string {
	if p.cwd == "" && p.model == "" && p.goVersion == "" && p.osArch == "" && p.storeType == "" {
		return ""
	}

	inner := width - 1
	if inner < 8 {
		inner = 8
	}

	header := p.styles.accent.Render("◈ environment")

	// Each key/value row: key rendered with dimLabel, value truncated to fit.
	// cwd can be very long — truncate with ansi.Truncate before rendering.
	const keyWidth = 7 // "store  " — widest key

	renderRow := func(key, value string) string {
		if value == "" {
			return ""
		}
		// Reserve space for the key prefix so value doesn't overflow inner.
		valWidth := inner - keyWidth - 2
		if valWidth < 4 {
			valWidth = 4
		}
		truncatedVal := ansi.Truncate(value, valWidth, "…")
		label := fmt.Sprintf("%-*s", keyWidth, key)
		line := p.styles.dimLabel.Render(label) + " " + truncatedVal
		return ansi.Truncate(line, inner, "…")
	}

	rows := []string{ansi.Truncate(header, inner, "…")}
	if row := renderRow("cwd", p.cwd); row != "" {
		rows = append(rows, row)
	}
	if row := renderRow("model", p.model); row != "" {
		rows = append(rows, row)
	}
	if row := renderRow("go", p.goVersion); row != "" {
		rows = append(rows, row)
	}
	if row := renderRow("os", p.osArch); row != "" {
		rows = append(rows, row)
	}
	if row := renderRow("store", p.storeType); row != "" {
		rows = append(rows, row)
	}

	return strings.Join(rows, "\n")
}

// ---------------------------------------------------------------------------
// resumeListPanel — recent sessions (PR4b: welcome + sessions screens)
// ---------------------------------------------------------------------------

// resumeListPanel renders the most-recent N (capped at 5) conversations in the
// right rail. Provides a quick resume-list on both the welcome and sessions
// screens. Data is set via setSessions after sessionsLoadedMsg arrives globally.
// Renders "" when no sessions are loaded.
type resumeListPanel struct {
	styles   tuiStyles
	sessions []store.Conversation
}

// newResumeListPanel constructs an empty resumeListPanel.
func newResumeListPanel(s tuiStyles) *resumeListPanel {
	return &resumeListPanel{styles: s}
}

// setSessions updates the panel's session list. Called from copyRailWith in
// the global sessionsLoadedMsg handler (model.go Update).
func (p *resumeListPanel) setSessions(convs []store.Conversation) {
	p.sessions = convs
}

// Render implements Panel. Returns "" when there are no sessions.
func (p *resumeListPanel) Render(width, _ int) string {
	if len(p.sessions) == 0 {
		return ""
	}

	inner := width - 1
	if inner < 8 {
		inner = 8
	}

	header := p.styles.accent.Render("◈ recent sessions")
	rows := []string{ansi.Truncate(header, inner, "…")}

	// Cap at 5 most-recent sessions.
	const maxSessions = 5
	convs := p.sessions
	if len(convs) > maxSessions {
		convs = convs[:maxSessions]
	}

	for _, conv := range convs {
		// Short ID: first 8 runes — rune-safe, no byte-slicing.
		shortID := conv.ID
		if len([]rune(shortID)) > 8 {
			shortID = string([]rune(shortID)[:8])
		}

		// Relative updated-ago time.
		ago := relativeTime(conv.UpdatedAt)

		// Title from metadata or fallback.
		title := conv.Metadata["title"]
		if title == "" {
			title = "(untitled)"
		}

		// Format: "shortID  ago  title" — mirrors renderSessions row format.
		line := fmt.Sprintf("%-8s  %-8s  %s", shortID, ago, title)
		rows = append(rows, p.styles.dimLabel.Render(ansi.Truncate(line, inner, "…")))
	}

	return strings.Join(rows, "\n")
}
