package tui

// rail_panels.go — concrete right-rail panel structs for PR2b.
//
// Implements three Panel types for the chat screen (tasks 2.14–2.17):
//   - telemetryPanel   — tokens / cost / tool-call counts from EventTokensUsage / EventToolStart/End
//   - todolistPanel    — LLM-generated todo list from Agent.TodoListForConv
//   - contextMeterPanel — context usage from EventTokensUsage (token count used as proxy)
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

	// Inner width accounts for a 1-char left margin we manually add.
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
// contextMeterPanel — context window usage
// ---------------------------------------------------------------------------

// contextMeterPanel renders a visual indicator of how much of the context
// window is consumed. In V1 the data source is EventTokensUsage.TokenCount —
// the total tokens accumulated this turn. A model-limit heuristic (200k for
// Claude models) is used to compute the percentage bar.
//
// If there is no live source for context-window limits, the panel still shows
// the raw token count so it is always useful without a backend change.
type contextMeterPanel struct {
	styles    tuiStyles
	tokenUsed int // total tokens from EventTokensUsage accumulation
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
