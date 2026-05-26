package tui

// rail.go — right-rail panel container (PR2b populates concrete panel types).
// PR1 declared the Panel interface and the rail type skeleton.
// PR2b (this PR) adds newRail() which wires the three chat-screen panels.

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
			panelModelPicker:   newModelPickerPanel(s, "", ""), // empty sentinel; RunTUI replaces this via copyRailWith
			panelToolDetail:    newToolDetailPanel(s),          // PR4a: tools screen
			panelActivePolicy:  newActivePolicyPanel(s, ""),    // PR5: error screen; RunTUI replaces with ag.CurrentMode()
			panelRecentDenials: newRecentDenialsPanel(s),       // PR5: error screen; populated by denial events
		},
	}
}

// Render renders all active panels for `screen` stacked vertically.
// Returns empty string when there are no panels (e.g. screenSlash).
func (r *rail) Render(screen screenState, width, height int) string {
	ids := panelsFor(screen)
	if len(ids) == 0 {
		return ""
	}
	out := ""
	for _, id := range ids {
		if p, ok := r.panels[id]; ok {
			s := p.Render(width, height)
			if s != "" {
				out += s + "\n"
			}
		}
	}
	return out
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
