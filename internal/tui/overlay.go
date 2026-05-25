package tui

// overlay.go — dialog interface and overlayManager (AD-9).
// PR1 declares the full overlay infrastructure so the Model compiles and
// Update's overlay interception guard works. Concrete dialog types
// (CommandPalette, ApprovalPrompt, PermissionPrompt) land in PR3/PR5.

import tea "github.com/charmbracelet/bubbletea"

// dialog is the interface implemented by all overlay dialogs.
// Dialogs are drawn LAST in View(), composited over the dimmed main layout.
//
//   - ID() identifies the dialog (used for deduplication).
//   - HandleMsg processes a tea.Msg; consumed=true means the message is fully
//     handled by the overlay and must NOT fall through to screen routing.
//   - Render draws the overlay at the given dimensions using the provided styles.
type dialog interface {
	ID() string
	HandleMsg(msg tea.Msg) (dialog, tea.Cmd, bool) // (next state, cmd, consumed)
	Render(width, height int, styles tuiStyles) string
}

// overlayManager is a LIFO stack of active dialog overlays.
// Push/Pop/Active/Top follow the architecture.md spec.
type overlayManager struct {
	stack []dialog
}

// Push adds a dialog to the top of the stack.
func (o *overlayManager) Push(d dialog) {
	o.stack = append(o.stack, d)
}

// Pop removes the top dialog. No-op on empty stack.
func (o *overlayManager) Pop() {
	if len(o.stack) == 0 {
		return
	}
	o.stack = o.stack[:len(o.stack)-1]
}

// Active reports whether any dialog is currently on the stack.
func (o *overlayManager) Active() bool {
	return len(o.stack) > 0
}

// Top returns the topmost dialog. Panics on empty stack — call Active() first.
func (o *overlayManager) Top() dialog {
	return o.stack[len(o.stack)-1]
}

// Replace swaps the top dialog with next. Used by Update after HandleMsg.
func (o *overlayManager) Replace(next dialog) {
	if len(o.stack) == 0 {
		return
	}
	o.stack[len(o.stack)-1] = next
}

// Render draws all active overlays, top-most last (drawn on top of everything).
// Returns empty string when no overlays are active.
func (o *overlayManager) Render(width, height int, styles tuiStyles) string {
	if len(o.stack) == 0 {
		return ""
	}
	// Only the top overlay is rendered (single-active model in V1).
	return o.stack[len(o.stack)-1].Render(width, height, styles)
}
