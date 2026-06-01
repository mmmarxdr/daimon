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

// renderMoreRow returns the standard dim "+N more" overflow row, ANSI-truncated.
// Reuses the exact idiom from rail_panels.go:320-322 (telemetry overflow):
//
//	format: "  +%d more" (two leading spaces)
//	style:  dimLabel
//	truncation: ansi.Truncate(..., inner, "…")
//
// inner is the usable content width (width - 4 per wrapPanelBox convention).
func renderMoreRow(n, inner int, s tuiStyles) string {
	return ansi.Truncate(s.dimLabel.Render(fmt.Sprintf("  +%d more", n)), inner, "…")
}

// wrapPanelBox wraps the given content string in a bordered box using s.panelBorder.
//
// Width math (lipgloss v1.1.0 verified):
//
//	panelBorder.Width(N).Render(content) → outer box width = N + 2 (border only).
//	Padding(0,1) is included within N, so text content area = N - 2.
//	To get outer == `width`: N = width - 2.
//	Text content area = (width-2) - 2 = width - 4.
//
// Pre-truncate each content line to (width-5) to avoid lipgloss word-wrap at exactly
// content_area-1 columns (verified: lipgloss wraps when visible >= Width-1).
// Returns "" unchanged so callers can short-circuit on empty content.
func wrapPanelBox(content string, width int, s tuiStyles) string {
	if content == "" {
		return ""
	}
	// Width(N) → outer = N+2. So N = width-2 gives outer = width.
	lipglossW := width - 2
	if lipglossW < 1 {
		lipglossW = 1
	}
	// Text content area = lipglossW - 2 (padding each side).
	// Truncate to contentArea-1 to avoid lipgloss word-wrap edge case.
	truncW := lipglossW - 3 // = width - 5
	if truncW < 1 {
		truncW = 1
	}
	lines := strings.Split(content, "\n")
	for i, line := range lines {
		if ansi.StringWidth(line) > truncW {
			lines[i] = ansi.Truncate(line, truncW, "…")
		}
	}
	return s.panelBorder.Width(lipglossW).Render(strings.Join(lines, "\n"))
}

// ---------------------------------------------------------------------------
// telemetryPanel — tokens / cost / tool-call counts + per-tool + per-subagent
// ---------------------------------------------------------------------------

// toolStat accumulates call/error/duration statistics for a single named tool.
type toolStat struct {
	calls      int
	errors     int
	durationMs int64
}

// subagentStat tracks token usage and lifecycle state for a single subagent.
// done is set true on EventSubagentCompleted or EventSubagentFailed.
// failed is set true on EventSubagentFailed only.
type subagentStat struct {
	tokens int
	done   bool
	failed bool
}

// telemetryPanel accumulates EventTokensUsage and tool lifecycle events
// and renders a compact token/cost summary in the right rail.
type telemetryPanel struct {
	styles        tuiStyles
	totalIn       int                     // cumulative token count from EventTokensUsage
	totalCost     float64                 // cumulative cost in USD
	toolCalls     int                     // total tool calls started (EventToolStart)
	toolErrors    int                     // tool calls that ended with IsError (EventToolEnd)
	hasData       bool                    // true once at least one EventTokensUsage has been received
	toolStats     map[string]toolStat     // per-tool-name statistics
	subagentStats map[string]subagentStat // per-subagent_id statistics
	saOrder       []string                // first-seen insertion order for subagent IDs
}

// cloneToolStats returns a fresh deep copy of the toolStats map.
// Called inside accumulate before mutating the map to preserve COW invariant.
func cloneToolStats(src map[string]toolStat) map[string]toolStat {
	if src == nil {
		return make(map[string]toolStat)
	}
	dst := make(map[string]toolStat, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}

// cloneSubagentStats returns a fresh deep copy of the subagentStats map.
// Called inside accumulate before mutating the map to preserve COW invariant.
func cloneSubagentStats(src map[string]subagentStat) map[string]subagentStat {
	if src == nil {
		return make(map[string]subagentStat)
	}
	dst := make(map[string]subagentStat, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}

// cloneSAOrder returns a fresh deep copy of the saOrder slice.
// Called inside accumulate before appending to preserve COW invariant.
func cloneSAOrder(src []string) []string {
	dst := make([]string, len(src))
	copy(dst, src)
	return dst
}

// newTelemetryPanel constructs a telemetryPanel with zero state.
func newTelemetryPanel(s tuiStyles) *telemetryPanel {
	return &telemetryPanel{styles: s}
}

// accumulate processes a single notify.Event and updates the panel state.
// Called from handleBusEvent in screen_chat.go — runs on the model's Update path
// (via copy-on-write rail replacement).
//
// COW discipline: all map and slice mutations clone the container FIRST using
// clone helpers above. This ensures a shallow-copied telemetryPanel (cp := *tp)
// never aliases the original panel's maps/slices.
func (p *telemetryPanel) accumulate(ev notify.Event) {
	switch ev.Type {
	case notify.EventTokensUsage:
		p.totalIn += ev.TokenCount
		p.totalCost += ev.CostUSD
		p.hasData = true

		// Live subagent accumulation: only when subagent_id is present.
		id := ev.Meta["subagent_id"]
		if id != "" {
			// Check first-sight BEFORE cloning (clone creates same key set).
			_, alreadyKnown := p.subagentStats[id]
			p.subagentStats = cloneSubagentStats(p.subagentStats)
			st := p.subagentStats[id]
			// Guard: ignore late events after Completed/Failed (done flag).
			if !st.done {
				tokens := atoiSafe(ev.Meta["input_tokens"]) + atoiSafe(ev.Meta["output_tokens"])
				st.tokens += tokens
				p.subagentStats[id] = st
				// Register insertion order on first sight.
				if !alreadyKnown {
					p.saOrder = cloneSAOrder(p.saOrder)
					p.saOrder = append(p.saOrder, id)
				}
			}
			// If done: no mutation needed — leave st as-is (clone already copied it).
		}

	case notify.EventToolStart:
		p.toolCalls++
		p.toolStats = cloneToolStats(p.toolStats)
		st := p.toolStats[ev.ToolName]
		st.calls++
		p.toolStats[ev.ToolName] = st

	case notify.EventToolEnd:
		if ev.IsError {
			p.toolErrors++
		}
		p.toolStats = cloneToolStats(p.toolStats)
		st := p.toolStats[ev.ToolName]
		st.durationMs += ev.DurationMs
		if ev.IsError {
			st.errors++
		}
		p.toolStats[ev.ToolName] = st

	case notify.EventSubagentCompleted:
		id := ev.Meta["subagent_id"]
		if id == "" {
			return
		}
		p.subagentStats = cloneSubagentStats(p.subagentStats)
		// Register in saOrder on first sight.
		if _, exists := p.subagentStats[id]; !exists {
			p.saOrder = cloneSAOrder(p.saOrder)
			p.saOrder = append(p.saOrder, id)
		}
		st := p.subagentStats[id]
		st.tokens = atoiSafe(ev.Meta["tokens"]) // REPLACE (authoritative)
		st.done = true
		p.subagentStats[id] = st

	case notify.EventSubagentFailed:
		id := ev.Meta["subagent_id"]
		if id == "" {
			return
		}
		p.subagentStats = cloneSubagentStats(p.subagentStats)
		// Register in saOrder on first sight.
		if _, exists := p.subagentStats[id]; !exists {
			p.saOrder = cloneSAOrder(p.saOrder)
			p.saOrder = append(p.saOrder, id)
		}
		st := p.subagentStats[id]
		// Failed: set markers only — do NOT read Meta["tokens"].
		st.done = true
		st.failed = true
		p.subagentStats[id] = st
	}
}

// Render implements Panel. Returns "" when no EventTokensUsage has been received.
//
// Height contract (design ADR-2):
//
//	budget := height (0 = natural/unconstrained)
//	  budget <= 2  → return ""
//	  budget == 3  → header + renderMoreRow (0 data rows)
//	  budget >= 4  → header + as many assembled rows as fit (tail truncated) + renderMoreRow if cut
//	The already-assembled rows slice (aggregate first, tool rows, subagent rows) is
//	truncated from the tail so the aggregate block survives first.
func (p *telemetryPanel) Render(width, height int) string {
	if !p.hasData {
		return ""
	}

	// Inner width: panelBorder overhead = 4 (2 border + 2 padding).
	inner := width - 4
	if inner < 4 {
		inner = 4
	}

	headerRow := p.styles.panelHeaderWithBadge("telemetry", "live")
	tokLine := fmt.Sprintf("tokens  %d", p.totalIn)
	costLine := fmt.Sprintf("cost    $%.4f", p.totalCost)
	toolLine := fmt.Sprintf("tools   %d", p.toolCalls)

	dataRows := []string{
		ansi.Truncate(p.styles.dimLabel.Render(tokLine), inner, "…"),
		ansi.Truncate(p.styles.dimLabel.Render(costLine), inner, "…"),
		ansi.Truncate(p.styles.dimLabel.Render(toolLine), inner, "…"),
	}

	// Render error count only when non-zero, using errStyle (no inline hex).
	if p.toolErrors > 0 {
		errLine := fmt.Sprintf("errors  %d", p.toolErrors)
		dataRows = append(dataRows, ansi.Truncate(p.styles.errStyle.Render(errLine), inner, "…"))
	}

	// Per-tool rows: sort by count desc, name asc, cap 5, "+N more" overflow.
	dataRows = append(dataRows, p.renderToolRows(inner)...)

	// Per-subagent rows: first-seen order, cap 3.
	dataRows = append(dataRows, p.renderSubagentRows(inner)...)

	// Apply height budget gating (ADR-2). height==0 means natural.
	if height > 0 {
		if height <= 2 {
			return ""
		}
		contentRowBudget := height - 2 - 1 // border(2) + header(1)
		if contentRowBudget == 0 {
			// budget==3: header + +N more only.
			rows := []string{
				ansi.Truncate(headerRow, inner, "…"),
				renderMoreRow(len(dataRows), inner, p.styles),
			}
			return wrapPanelBox(strings.Join(rows, "\n"), width, p.styles)
		}
		// budget>=4: show as many data rows as fit, tail-truncated.
		maxShow := contentRowBudget - 1 // reserve 1 for +N more if cutting
		if maxShow < 0 {
			maxShow = 0
		}
		shown := dataRows
		var hidden int
		if len(dataRows) > contentRowBudget {
			// Truncate tail; reserve last slot for +N more.
			shown = dataRows[:maxShow]
			hidden = len(dataRows) - maxShow
		}
		rows := make([]string, 0, 1+len(shown)+1)
		rows = append(rows, ansi.Truncate(headerRow, inner, "…"))
		rows = append(rows, shown...)
		if hidden > 0 {
			rows = append(rows, renderMoreRow(hidden, inner, p.styles))
		}
		return wrapPanelBox(strings.Join(rows, "\n"), width, p.styles)
	}

	// Natural render (height==0): show all rows.
	rows := make([]string, 0, 1+len(dataRows))
	rows = append(rows, ansi.Truncate(headerRow, inner, "…"))
	rows = append(rows, dataRows...)
	return wrapPanelBox(strings.Join(rows, "\n"), width, p.styles)
}

// renderToolRows returns rendered per-tool stat lines for inclusion in Render.
// Sort: count desc, name asc. Cap: 5. Overflow: "+N more" in dimLabel.
func (p *telemetryPanel) renderToolRows(inner int) []string {
	if len(p.toolStats) == 0 {
		return nil
	}

	// Collect and sort tool names.
	type entry struct {
		name  string
		calls int
	}
	entries := make([]entry, 0, len(p.toolStats))
	for name, st := range p.toolStats {
		entries = append(entries, entry{name: name, calls: st.calls})
	}
	// Sort: count desc, name asc (stable: avoid golden flakiness).
	for i := 1; i < len(entries); i++ {
		for j := i; j > 0; j-- {
			a, b := entries[j-1], entries[j]
			if a.calls < b.calls || (a.calls == b.calls && a.name > b.name) {
				entries[j-1], entries[j] = entries[j], entries[j-1]
			} else {
				break
			}
		}
	}

	const cap5 = 5
	visible := entries
	overflow := 0
	if len(entries) > cap5 {
		visible = entries[:cap5]
		overflow = len(entries) - cap5
	}

	var rows []string
	for _, e := range visible {
		st := p.toolStats[e.name]
		// Truncate name to 8 runes.
		displayName := ansi.Truncate(e.name, 8, "")
		line := fmt.Sprintf("  %-8s %3d", displayName, e.calls)
		if st.errors > 0 {
			errMark := p.styles.errStyle.Render(fmt.Sprintf(" ✗%d", st.errors))
			rows = append(rows, ansi.Truncate(p.styles.dimLabel.Render(line)+errMark, inner, "…"))
		} else {
			rows = append(rows, ansi.Truncate(p.styles.dimLabel.Render(line), inner, "…"))
		}
	}
	if overflow > 0 {
		overflowLine := fmt.Sprintf("  +%d more", overflow)
		rows = append(rows, ansi.Truncate(p.styles.dimLabel.Render(overflowLine), inner, "…"))
	}
	return rows
}

// renderSubagentRows returns rendered per-subagent stat lines for Render.
// Order: first-seen (saOrder). Cap: 3. Status markers: ✓/✗/●.
func (p *telemetryPanel) renderSubagentRows(inner int) []string {
	if len(p.subagentStats) == 0 {
		return nil
	}

	const cap3 = 3
	order := p.saOrder
	if len(order) > cap3 {
		order = order[:cap3]
	}

	var rows []string
	for _, id := range order {
		st, ok := p.subagentStats[id]
		if !ok {
			continue
		}
		// Marker: ✓ done+ok, ✗ failed, ● live.
		var marker string
		switch {
		case st.done && st.failed:
			marker = p.styles.errStyle.Render("✗")
		case st.done:
			marker = p.styles.accent.Render("✓")
		default:
			marker = p.styles.amber.Render("●")
		}
		// Truncate ID to 8 runes.
		shortID := ansi.Truncate(id, 8, "")
		tokStr := humanK(st.tokens)
		line := fmt.Sprintf(" %-8s %s", shortID, tokStr)
		rows = append(rows, ansi.Truncate(marker+p.styles.dimLabel.Render(line), inner, "…"))
	}
	return rows
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
//
// Height contract (design ADR-2, ADR-4):
//
//	budget := height (0 = natural/unconstrained)
//	Step 1: cap items to todolistMaxItems (=10) — height-independent.
//	Step 2: gate on contentRowBudget = budget - 2 (border) - 1 (header).
//	  budget <= 2  → return ""
//	  budget == 3  → header + renderMoreRow(hidden, inner, styles)
//	  budget >= 4  → header + min(contentRowBudget-1, len) data rows + renderMoreRow if cut
//	Reconciled single +N more: N = totalItems - shownItems.
func (p *todolistPanel) Render(width, height int) string {
	if len(p.list.Items) == 0 {
		return ""
	}

	// Inner width: panelBorder overhead = 4 (2 border + 2 padding).
	inner := width - 4
	if inner < 4 {
		inner = 4
	}

	totalItems := len(p.list.Items)

	// Step 1: apply hard cap (height-independent, ADR-4).
	items := p.list.Items
	if len(items) > todolistMaxItems {
		items = items[:todolistMaxItems]
	}

	// Badge uses the original total for accurate "X/Y" ratio.
	done := 0
	for _, item := range p.list.Items {
		if item.Status == "done" || item.Status == "completed" {
			done++
		}
	}
	badge := fmt.Sprintf("%d/%d · auto", done, totalItems)
	headerRow := ansi.Truncate(p.styles.panelHeaderWithBadgeWidth("todo", badge, inner), inner, "…")

	// Step 2: apply height budget gating (ADR-2).
	// height==0 means "natural" — no truncation.
	if height > 0 {
		if height <= 2 {
			return ""
		}
		contentRowBudget := height - 2 - 1 // border(2) + header(1)
		if contentRowBudget == 0 {
			// budget==3: header + +N more only.
			hidden := totalItems // all items hidden
			rows := []string{headerRow, renderMoreRow(hidden, inner, p.styles)}
			return wrapPanelBox(strings.Join(rows, "\n"), width, p.styles)
		}
		// budget>=4: show up to contentRowBudget-1 data rows (reserve 1 for +N more if cut).
		maxShow := contentRowBudget - 1
		if maxShow < 0 {
			maxShow = 0
		}
		if len(items) <= contentRowBudget {
			// All capped items fit — no height clamp needed.
			maxShow = len(items)
		}
		shownItems := maxShow
		if shownItems > len(items) {
			shownItems = len(items)
		}

		rows := []string{headerRow}
		for _, item := range items[:shownItems] {
			var marker string
			switch item.Status {
			case "done", "completed":
				marker = p.styles.accent.Render("✓")
			case "in_progress":
				marker = p.styles.amber.Render("●")
			default:
				marker = p.styles.dimLabel.Render("○")
			}
			rows = append(rows, ansi.Truncate(marker+" "+item.Content, inner, "…"))
		}
		// Single reconciled +N more (covers both cap and clamp hidden items).
		hidden := totalItems - shownItems
		if hidden > 0 {
			rows = append(rows, renderMoreRow(hidden, inner, p.styles))
		}
		return wrapPanelBox(strings.Join(rows, "\n"), width, p.styles)
	}

	// Natural render (height==0): apply cap but no height truncation.
	rows := []string{headerRow}
	for _, item := range items {
		var marker string
		switch item.Status {
		case "done", "completed":
			marker = p.styles.accent.Render("✓")
		case "in_progress":
			marker = p.styles.amber.Render("●")
		default:
			marker = p.styles.dimLabel.Render("○")
		}
		rows = append(rows, ansi.Truncate(marker+" "+item.Content, inner, "…"))
	}
	// If cap was applied, show +N more for cap overflow.
	capHidden := totalItems - len(items)
	if capHidden > 0 {
		rows = append(rows, renderMoreRow(capHidden, inner, p.styles))
	}
	return wrapPanelBox(strings.Join(rows, "\n"), width, p.styles)
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

	inner := width - 4
	if inner < 4 {
		inner = 4
	}

	header := p.styles.panelHeader("model")
	provLine := ansi.Truncate(p.styles.dimLabel.Render(p.provider), inner, "…")
	modelLine := ansi.Truncate(p.styles.dimLabel.Render(p.model), inner, "…")

	return wrapPanelBox(strings.Join([]string{header, provLine, modelLine}, "\n"), width, p.styles)
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

	inner := width - 4
	if inner < 4 {
		inner = 4
	}

	header := p.styles.panelHeader("tool detail")
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

	return wrapPanelBox(strings.Join(rows, "\n"), width, p.styles)
}

// ---------------------------------------------------------------------------
// contextMeterPanel — context window usage
// ---------------------------------------------------------------------------

// humanK formats an integer as a compact human-readable string.
// Rules (deterministic — no locale, no rounding surprises):
//
//	n < 1000  → "%d"        (e.g. 999  → "999")
//	n < 10000 → "%.1fk"    (e.g. 1000 → "1.0k", 1500 → "1.5k")
//	else      → "%dk"      (e.g. 200000 → "200k")
func humanK(n int) string {
	switch {
	case n < 1000:
		return fmt.Sprintf("%d", n)
	case n < 10000:
		return fmt.Sprintf("%.1fk", float64(n)/1000)
	default:
		return fmt.Sprintf("%dk", n/1000)
	}
}

// contextMeterPanel renders a visual indicator of how much of the context
// window is consumed. Data source: EventTokensUsage per-category fields
// (REPLACE semantics — each event is a snapshot of current window fill).
// Falls back to aggregate TokenCount accumulation when category fields are 0.
//
// The real context-window limit is threaded once at boot via setLimit.
// Zero limit triggers a 200k heuristic estimate (suffix " est.").
type contextMeterPanel struct {
	styles    tuiStyles
	limit     int // real window from ContextWindowSize(); 0 ⇒ heuristic fallback
	tokenUsed int // current fill: sum of categories (REPLACE) or TokenCount (accumulate)
	sysToks   int // REPLACE per EventTokensUsage snapshot (0 in legacy/none mode)
	msgToks   int // REPLACE
	toolToks  int // REPLACE
	hasData   bool
}

// newContextMeterPanel constructs a contextMeterPanel with zero state.
func newContextMeterPanel(s tuiStyles) *contextMeterPanel {
	return &contextMeterPanel{styles: s}
}

// setLimit threads the real context-window size once at boot (via copyRailWith in run.go).
func (p *contextMeterPanel) setLimit(n int) { p.limit = n }

// accumulate processes EventTokensUsage events.
//
// Branch A (smart strategy): SysToks+MsgToks+ToolToks > 0 → REPLACE snapshot.
// Branch B (legacy/none):    all-zero categories → tokenUsed += TokenCount (delta).
//
// Subagent guard: events carrying a non-empty Meta["subagent_id"] are dropped
// without mutation. Every subagent emits EventTokensUsage on the shared bus
// with its own category snapshot; letting those through would clobber the
// main conversation's window fill via the REPLACE branch. Per-subagent tokens
// are the telemetry panel's responsibility (see telemetryPanel.accumulate).
func (p *contextMeterPanel) accumulate(ev notify.Event) {
	if ev.Type != notify.EventTokensUsage {
		return
	}
	// Drop subagent snapshots — they would overwrite the top-level window fill.
	if ev.Meta["subagent_id"] != "" {
		return
	}
	if ev.SysToks+ev.MsgToks+ev.ToolToks > 0 {
		// REPLACE — each EventTokensUsage is a snapshot of current window fill.
		p.sysToks = ev.SysToks
		p.msgToks = ev.MsgToks
		p.toolToks = ev.ToolToks
		p.tokenUsed = ev.SysToks + ev.MsgToks + ev.ToolToks
	} else {
		// Legacy / none strategy: no breakdown. Accumulate aggregate delta.
		// Category fields stay 0 so Render hides sub-bars.
		p.tokenUsed += ev.TokenCount
	}
	p.hasData = true
}

// Render implements Panel. Returns "" until at least one token event is received.
//
// Height contract (design ADR-2):
//
//	budget := height (0 = natural/unconstrained)
//	  budget <= 2  → return ""
//	  budget == 3  → header + renderMoreRow (0 data rows)
//	  budget >= 4  → header + bar + pct (always) + category rows tail-first to budget + renderMoreRow if cut
//	The bar is NEVER split — if budget can't fit bar+pct, this is the ==3 case.
func (p *contextMeterPanel) Render(width, height int) string {
	if !p.hasData {
		return ""
	}

	// Inner width: panelBorder overhead = 4 (2 border + 2 padding).
	inner := width - 4
	if inner < 8 {
		inner = 8
	}

	// Resolve limit and label. Zero limit → heuristic 200k with " est." sentinel.
	limit := p.limit
	label := "of " + humanK(limit)
	if limit == 0 {
		limit = 200_000
		label = "of 200k est."
	}

	pct := float64(p.tokenUsed) / float64(limit)
	if pct > 1.0 {
		pct = 1.0
	}

	// Render a simple bar using fill characters.
	// barLine = "[" + bar + "]" has length barWidth+2.
	// wrapPanelBox truncates content lines to truncW = width-5.
	// So barWidth+2 must be ≤ width-5 → barWidth ≤ width-7.
	// (At width=32: width-7 = 25 = inner-3, so wide goldens are unaffected.)
	barWidth := width - 7 // budget = width-5 (wrapPanelBox truncW), minus 2 for brackets
	if barWidth < 2 {
		barWidth = 2
	}
	filled := int(pct * float64(barWidth))
	bar := strings.Repeat("█", filled) + strings.Repeat("░", barWidth-filled)

	headerRow := ansi.Truncate(p.styles.panelHeader("context"), inner, "…")
	barRow := ansi.Truncate("["+bar+"]", inner, "…")
	pctRow := ansi.Truncate(p.styles.dimLabel.Render(fmt.Sprintf("%.1f%% %s", pct*100, label)), inner, "…")

	// Build category rows (smart strategy only).
	var categoryRows []string
	if p.sysToks > 0 {
		sysLine := fmt.Sprintf("sys   %s", humanK(p.sysToks))
		msgLine := fmt.Sprintf("msg   %s", humanK(p.msgToks))
		toolLine := fmt.Sprintf("tool  %s", humanK(p.toolToks))
		categoryRows = []string{
			ansi.Truncate(p.styles.dimLabel.Render(sysLine), inner, "…"),
			ansi.Truncate(p.styles.dimLabel.Render(msgLine), inner, "…"),
			ansi.Truncate(p.styles.dimLabel.Render(toolLine), inner, "…"),
		}
	}
	// Total natural data rows: bar + pct + category.
	totalDataRows := 2 + len(categoryRows)

	// Apply height budget gating (ADR-2). height==0 means natural.
	if height > 0 {
		if height <= 2 {
			return ""
		}
		contentRowBudget := height - 2 - 1 // border(2) + header(1)
		if contentRowBudget == 0 {
			// budget==3: header + +N more only.
			rows := []string{headerRow, renderMoreRow(totalDataRows, inner, p.styles)}
			return wrapPanelBox(strings.Join(rows, "\n"), width, p.styles)
		}
		// budget>=4: bar+pct always shown (they are the panel's core), then
		// category rows tail-first to fit remaining budget.
		// contentRowBudget rows available; reserve 1 for +N more if cutting.
		rows := []string{headerRow, barRow, pctRow}
		shownData := 2 // bar + pct always shown
		remaining := contentRowBudget - 2
		if remaining < 0 {
			remaining = 0
		}
		// Show as many category rows as fit (tail truncated).
		catToShow := len(categoryRows)
		if catToShow > remaining {
			catToShow = remaining
			if catToShow > 0 {
				// Reserve 1 for +N more when cutting.
				catToShow--
			}
		}
		rows = append(rows, categoryRows[:catToShow]...)
		shownData += catToShow
		hidden := totalDataRows - shownData
		if hidden > 0 {
			rows = append(rows, renderMoreRow(hidden, inner, p.styles))
		}
		return wrapPanelBox(strings.Join(rows, "\n"), width, p.styles)
	}

	// Natural render (height==0): show all rows.
	rows := []string{headerRow, barRow, pctRow}
	rows = append(rows, categoryRows...)
	return wrapPanelBox(strings.Join(rows, "\n"), width, p.styles)
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

	inner := width - 4
	if inner < 8 {
		inner = 8
	}

	header := p.styles.panelHeader("environment")

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

	return wrapPanelBox(strings.Join(rows, "\n"), width, p.styles)
}

// ---------------------------------------------------------------------------
// activePolicyPanel — current active mode (PR5: error screen)
// ---------------------------------------------------------------------------

// activePolicyPanel shows the active mode name ("plan", "build", or "review")
// so the user understands which policy gate triggered the denial.
// Renders "" when mode is empty (sentinel for "not configured" or tests).
type activePolicyPanel struct {
	styles tuiStyles
	mode   string // active mode name; "" renders ""
}

// newActivePolicyPanel constructs an activePolicyPanel with the given mode.
// mode is passed from ag.CurrentMode() at construction/registration time
// in run.go. An empty string renders "" (matches the ""-when-empty contract).
func newActivePolicyPanel(s tuiStyles, mode string) *activePolicyPanel {
	return &activePolicyPanel{styles: s, mode: mode}
}

// setMode updates the active mode name. Called at denial time (via copyRailWith)
// so the panel always reflects the mode that actually triggered the denial,
// not the (potentially stale) mode captured at startup. Mirrors the pattern
// of toolDetailPanel.setTool / todolistPanel.setList.
func (p *activePolicyPanel) setMode(mode string) {
	p.mode = mode
}

// Render implements Panel. Returns "" when mode is empty.
func (p *activePolicyPanel) Render(width, _ int) string {
	if p.mode == "" {
		return ""
	}

	inner := width - 4
	if inner < 4 {
		inner = 4
	}

	header := p.styles.panelHeader("active policy")
	modeLine := ansi.Truncate(p.styles.amber.Render("mode: "+p.mode), inner, "…")
	noteLine := ansi.Truncate(p.styles.dimLabel.Render("tool gates enforced"), inner, "…")

	return wrapPanelBox(strings.Join([]string{header, modeLine, noteLine}, "\n"), width, p.styles)
}

// ---------------------------------------------------------------------------
// recentDenialsPanel — list of recently denied tool calls (PR5: error screen)
// ---------------------------------------------------------------------------

// recentDenialsPanel renders the most-recent denied tool calls in the right
// rail so the user can see the history of blocked actions this session.
// Renders "" when no denials have been recorded.
type recentDenialsPanel struct {
	styles  tuiStyles
	denials []denialEntry // copy-on-write snapshot; "" when empty
}

// newRecentDenialsPanel constructs an empty recentDenialsPanel.
func newRecentDenialsPanel(s tuiStyles) *recentDenialsPanel {
	return &recentDenialsPanel{styles: s}
}

// setDenials replaces the current denial list. Called from copyRailWith in
// the EventToolEnd denial handler (screen_chat.go handleBusEvent).
func (p *recentDenialsPanel) setDenials(denials []denialEntry) {
	p.denials = denials
}

// Render implements Panel. Returns "" when there are no denials.
func (p *recentDenialsPanel) Render(width, _ int) string {
	if len(p.denials) == 0 {
		return ""
	}

	inner := width - 4
	if inner < 4 {
		inner = 4
	}

	header := p.styles.panelHeader("recent denials")
	rows := []string{ansi.Truncate(header, inner, "…")}

	for _, d := range p.denials {
		// Tool name (bold/label style).
		toolLine := ansi.Truncate(p.styles.errStyle.Render(d.tool), inner, "…")
		rows = append(rows, toolLine)

		// Reason truncated to fit width (ANSI-safe: ansi.Truncate measures visible
		// columns and never splits a multi-byte rune or ANSI escape sequence).
		if d.reason != "" {
			const maxReason = 40
			truncReason := d.reason
			if ansi.StringWidth(truncReason) > maxReason {
				truncReason = ansi.Truncate(truncReason, maxReason, "…")
			}
			reasonLine := ansi.Truncate(p.styles.dimLabel.Render(" "+truncReason), inner, "…")
			rows = append(rows, reasonLine)
		}
	}

	return wrapPanelBox(strings.Join(rows, "\n"), width, p.styles)
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
	ago      []string // WU-b: pre-computed "ago" strings parallel to sessions
}

// newResumeListPanel constructs an empty resumeListPanel.
func newResumeListPanel(s tuiStyles) *resumeListPanel {
	return &resumeListPanel{styles: s}
}

// setSessions updates the panel's session list and pre-computes the "ago"
// strings so Render never calls relativeTime (i.e. time.Since) from the
// View path. Called from copyRailWith in the global sessionsLoadedMsg handler.
func (p *resumeListPanel) setSessions(convs []store.Conversation) {
	p.sessions = convs
	p.ago = make([]string, len(convs))
	for i, c := range convs {
		p.ago[i] = relativeTime(c.UpdatedAt) // clock read happens here, in Update
	}
}

// Render implements Panel. Returns "" when there are no sessions.
func (p *resumeListPanel) Render(width, _ int) string {
	if len(p.sessions) == 0 {
		return ""
	}

	inner := width - 4
	if inner < 8 {
		inner = 8
	}

	header := p.styles.panelHeader("recent sessions")
	rows := []string{ansi.Truncate(header, inner, "…")}

	// Cap at 5 most-recent sessions.
	const maxSessions = 5
	convs := p.sessions
	if len(convs) > maxSessions {
		convs = convs[:maxSessions]
	}

	for i, conv := range convs {
		// Short ID: first 8 runes — rune-safe, no byte-slicing.
		shortID := conv.ID
		if len([]rune(shortID)) > 8 {
			shortID = string([]rune(shortID)[:8])
		}

		// WU-b: read pre-computed ago string; never call relativeTime from Render.
		// Guard i < len(p.ago) defensively — slices are always parallel but this
		// protects against a future divergence without a visible breakage.
		ago := ""
		if i < len(p.ago) {
			ago = p.ago[i]
		}

		// Title from metadata or fallback.
		title := conv.Metadata["title"]
		if title == "" {
			title = "(untitled)"
		}

		// Format: "shortID  ago  title" — mirrors renderSessions row format.
		line := fmt.Sprintf("%-8s  %-8s  %s", shortID, ago, title)
		rows = append(rows, p.styles.dimLabel.Render(ansi.Truncate(line, inner, "…")))
	}

	return wrapPanelBox(strings.Join(rows, "\n"), width, p.styles)
}

// ---------------------------------------------------------------------------
// memoryPeekPanel — recent memory entries (PR-c)
// ---------------------------------------------------------------------------

// memoryPeekPanel renders the most-recent memory entries for the active scope.
// Data is refreshed via a tea.Cmd when EventMemoryChanged is received.
// Mirrors the todolistPanel cmd-refresh precedent exactly.
//
// The panel starts empty (entries == nil). Before the first EventMemoryChanged
// the panel renders "" (zero height, per TR-0-C / TR-14).
type memoryPeekPanel struct {
	styles  tuiStyles
	entries []store.MemoryEntry
}

// newMemoryPeekPanel constructs a memoryPeekPanel with empty state.
func newMemoryPeekPanel(s tuiStyles) *memoryPeekPanel {
	return &memoryPeekPanel{styles: s}
}

// setEntries replaces the current memory entries. Called from the
// memoryRefreshMsg Update case via copyRailWith.
func (p *memoryPeekPanel) setEntries(entries []store.MemoryEntry) {
	p.entries = entries
}

// Render implements Panel. Returns "" when there are no entries (TR-0-C, TR-14).
// When entries are present, renders a "memory" header badge followed by up to 5
// entry rows. Each row shows Title; falls back to Content when Title is empty.
// All lines are ANSI-truncated to inner width (width-4 per wrapPanelBox convention).
//
// Height contract (design ADR-2):
//
//	budget := height (0 = natural/unconstrained)
//	  budget <= 2  → return ""
//	  budget == 3  → header + renderMoreRow
//	  budget >= 4  → header + entry rows (tail-truncated to budget) + renderMoreRow if cut
//	maxRows=5 cap is applied before height truncation; height truncation is applied second.
func (p *memoryPeekPanel) Render(width, height int) string {
	if len(p.entries) == 0 {
		return ""
	}

	inner := width - 4
	if inner < 4 {
		inner = 4
	}

	headerRow := ansi.Truncate(p.styles.panelHeader("memory"), inner, "…")

	// Apply existing maxRows=5 cap (pre-height-truncation).
	const maxRows = 5
	entries := p.entries
	if len(entries) > maxRows {
		entries = entries[:maxRows]
	}
	totalEntries := len(p.entries) // original count for +N more reconciliation

	// Build all entry rows (post-cap).
	dataRows := make([]string, 0, len(entries))
	for _, e := range entries {
		text := e.Title
		if text == "" {
			text = e.Content // fallback to Content when Title is empty
		}
		dataRows = append(dataRows, ansi.Truncate(p.styles.dimLabel.Render("• "+text), inner, "…"))
	}

	// Apply height budget gating (ADR-2). height==0 means natural.
	if height > 0 {
		if height <= 2 {
			return ""
		}
		contentRowBudget := height - 2 - 1 // border(2) + header(1)
		if contentRowBudget == 0 {
			// budget==3: header + +N more only.
			rows := []string{headerRow, renderMoreRow(totalEntries, inner, p.styles)}
			return wrapPanelBox(strings.Join(rows, "\n"), width, p.styles)
		}
		// budget>=4: show as many entry rows as fit, tail-truncated.
		maxShow := contentRowBudget - 1 // reserve 1 for +N more if cutting
		if maxShow < 0 {
			maxShow = 0
		}
		shown := dataRows
		var hidden int
		if len(dataRows) > contentRowBudget {
			shown = dataRows[:maxShow]
			hidden = totalEntries - maxShow
		} else if totalEntries > len(dataRows) {
			// Cap was applied; the remaining are cap-hidden.
			hidden = totalEntries - len(dataRows)
		}
		rows := make([]string, 0, 1+len(shown)+1)
		rows = append(rows, headerRow)
		rows = append(rows, shown...)
		if hidden > 0 {
			rows = append(rows, renderMoreRow(hidden, inner, p.styles))
		}
		return wrapPanelBox(strings.Join(rows, "\n"), width, p.styles)
	}

	// Natural render (height==0): show capped rows, no height truncation.
	rows := make([]string, 0, 1+len(dataRows))
	rows = append(rows, headerRow)
	rows = append(rows, dataRows...)
	return wrapPanelBox(strings.Join(rows, "\n"), width, p.styles)
}
