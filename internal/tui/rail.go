package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// rail.go — right-rail panel container (PR2b populates concrete panel types).
// PR1 declared the Panel interface and the rail type skeleton.
// PR2b (this PR) adds newRail() which wires the three chat-screen panels.
// tui-rail-height-clamp PR-a adds assignBudgets + two-pass Render.

// railWidth is the fixed column width of the right rail when panels are active.
const railWidth = 32

// Panel is the interface implemented by all right-rail panel structs.
// Panels with no data MUST return "" (zero height) so rail computes correctly.
type Panel interface {
	Render(width, height int) string
}

// rail manages the set of active panels for the current screen.
type rail struct {
	panels map[panelID]Panel
}

// newRail constructs a rail with the concrete panels wired for every screen
// that PR2b–PR3b delivers (chat: todolist, context-meter, telemetry;
// sessions: model-picker). panelResumeList for sessions is deferred to PR4
// (welcome screen owns it) — it is absent here; rail.Render skips missing IDs,
// satisfying the Panel contract (missing → zero-height).
func newRail(s tuiStyles) rail {
	return rail{
		panels: map[panelID]Panel{
			panelTodolist:      newTodolistPanel(s),
			panelContextMeter:  newContextMeterPanel(s),
			panelTelemetry:     newTelemetryPanel(s),
			panelMemoryPeek:    newMemoryPeekPanel(s),          // PR-c: memory entries; empty until first EventMemoryChanged
			panelModelPicker:   newModelPickerPanel(s, "", ""), // empty sentinel; RunTUI replaces this via copyRailWith
			panelToolDetail:    newToolDetailPanel(s),          // PR4a: tools screen
			panelActivePolicy:  newActivePolicyPanel(s, ""),    // PR5: error screen; RunTUI replaces with ag.CurrentMode()
			panelRecentDenials: newRecentDenialsPanel(s),       // PR5: error screen; populated by denial events
		},
	}
}

// Render renders all active panels for `screen` stacked vertically.
// Returns empty string when there are no panels (e.g. screenSlash).
//
// Two-pass budget distribution (design ADR-1 + ADR-3):
//
// Pass 1: render each panel at full height to measure its natural height.
//
//	Empty panels (Render → "") are excluded from the budget.
//
// Compute avail = height - (numPopulated - 1) to reserve inter-panel
// separator newlines, then call assignBudgets.
//
// Pass 2: re-render each populated panel at its assigned budget and join
// with newlines. Guarantees lipgloss.Height(result) <= height.
func (r *rail) Render(screen screenState, width, height int) string {
	ids := panelsFor(screen)
	if len(ids) == 0 {
		return ""
	}

	// Pass 1: measure natural heights; collect only populated panels.
	// Panels return their natural (unconstrained) height when called with height=0.
	// Since PR-b, todolistPanel caps itself at todolistMaxItems internally, so
	// natural heights are bounded without a rail-level cap.
	type entry struct {
		panel   Panel
		natural int
	}
	populated := make([]entry, 0, len(ids))
	for _, id := range ids {
		p, ok := r.panels[id]
		if !ok {
			continue
		}
		s := p.Render(width, 0) // 0 = full/unconstrained height
		if s == "" {
			continue
		}
		nat := lipgloss.Height(s)
		populated = append(populated, entry{panel: p, natural: nat})
	}
	if len(populated) == 0 {
		return ""
	}

	// Compute available rows: reserve one newline separator between each pair.
	avail := height - (len(populated) - 1)
	if avail < 0 {
		avail = 0
	}

	// Distribute budgets across populated panels.
	naturals := make([]int, len(populated))
	for i, e := range populated {
		naturals[i] = e.natural
	}
	budgets := assignBudgets(naturals, avail)

	// Pass 2: re-render each panel at its assigned budget.
	// Per ADR-2 (design §2): budget <= panelMinHeight-2 cannot hold a
	// bordered header (2 border rows consume the entire budget), so a
	// compliant panel returns "". Skip the render call to avoid wasted work.
	// Budgets of panelMinHeight-1 (== 3) still render: header + "+N more".
	const boxFloor = panelMinHeight - 2 // == 2; panels with budget <= this return ""
	parts := make([]string, 0, len(populated))
	for i, e := range populated {
		if budgets[i] <= boxFloor {
			// Budget cannot fit a bordered box — skip (panel would return "").
			continue
		}
		s := e.panel.Render(width, budgets[i])
		if s != "" {
			parts = append(parts, s)
		}
	}
	return strings.Join(parts, "\n")
}

// assignBudgets distributes avail rows across n panels using a deterministic
// forward-pass algorithm (design ADR-1 + ADR-3):
//
//  1. Pool starts at avail (clamped to 0 if negative).
//  2. For each panel left-to-right: give = min(ceil(pool/remaining), natural).
//  3. Surplus (natural < share) flows to later panels automatically.
//
// Invariants: len(result) == len(naturals), sum(result) <= avail, each >= 0.
// O(n), no maps — deterministic in slice order.
func assignBudgets(naturals []int, avail int) []int {
	n := len(naturals)
	budgets := make([]int, n)
	if n == 0 || avail <= 0 {
		return budgets
	}
	pool := avail
	for i, nat := range naturals {
		remaining := n - i
		// Ceiling division: ceil(pool / remaining).
		share := (pool + remaining - 1) / remaining
		give := share
		if nat < give {
			give = nat
		}
		budgets[i] = give
		pool -= give
	}
	return budgets
}

// HasPanels reports whether the given screen has any rail panels.
func HasPanels(screen screenState) bool {
	return len(panelsFor(screen)) > 0
}

// copyRailWith returns a new rail whose panel map is a shallow copy of r.panels
// with the mutations applied by fn. This enables copy-on-write panel updates
// in Model.Update (value-receiver): we never mutate the map of the prior model.
//
// fn receives the NEW (copied) map and may replace panel entries in it.
// The original r is not modified.
func copyRailWith(r rail, fn func(map[panelID]Panel)) rail {
	newPanels := make(map[panelID]Panel, len(r.panels))
	for k, v := range r.panels {
		newPanels[k] = v
	}
	fn(newPanels)
	return rail{panels: newPanels}
}
