package tui

// rail.go — right-rail panel stub (PR2+ implements concrete panel types).
// PR1 declares the Panel interface, rail type, and railWidth constant so
// the layout math in renderLayout compiles.

// railWidth is the fixed column width of the right rail when panels are active.
const railWidth = 32

// Panel is the interface implemented by all right-rail panel structs.
// Panels with no data MUST return "" (zero height) so rail computes correctly.
type Panel interface {
	Render(width, height int) string
}

// rail manages the set of active panels for the current screen.
// PR2-5 populate concrete panel implementations; PR1 keeps this a stub.
type rail struct {
	panels map[panelID]Panel
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
