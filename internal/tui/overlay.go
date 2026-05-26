package tui

// overlay.go — dialog interface and overlayManager (AD-9).
// PR1 declares the full overlay infrastructure so the Model compiles and
// Update's overlay interception guard works. Concrete dialog types
// (CommandPalette, ApprovalPrompt, PermissionPrompt) land in PR3/PR5.

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

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

// placeOverlay composites box on top of base, centering box within a termW×termH
// terminal. base lines outside the box rectangle are preserved unchanged so the
// chat content remains visible around and behind the palette.
//
// Algorithm:
//  1. Split both strings into lines.
//  2. Compute the box dimensions from its widest line.
//  3. Center: xOff = (termW-boxW)/2, yOff = (termH-boxH)/2 (clamped ≥ 0).
//  4. Build an output of termH rows. For each row: if it falls within the box's
//     vertical range, splice the box line in at xOff, preserving base columns
//     outside the box. Base lines that don't exist are treated as empty strings.
//
// Width measurement uses ansi.StringWidth so ANSI escape sequences are ignored.
// Line splicing uses ansi.Truncate for the left segment; the right segment is
// computed via ansiSkipCols.
//
// If box is empty, base is returned unchanged.
func placeOverlay(base, box string, termW, termH int) string {
	if box == "" {
		return base
	}

	boxLines := strings.Split(box, "\n")
	baseLines := strings.Split(base, "\n")

	// Determine the box's visual width from its widest line.
	boxW := 0
	for _, l := range boxLines {
		if w := ansi.StringWidth(l); w > boxW {
			boxW = w
		}
	}
	boxH := len(boxLines)

	// Center offsets, clamped ≥ 0.
	xOff := (termW - boxW) / 2
	if xOff < 0 {
		xOff = 0
	}
	yOff := (termH - boxH) / 2
	if yOff < 0 {
		yOff = 0
	}

	// Output has termH rows so the box is always placed correctly even when
	// the rendered base has fewer lines than termH (e.g. in tests).
	outH := termH
	if len(baseLines) > outH {
		outH = len(baseLines)
	}

	result := make([]string, outH)
	for i := range result {
		// Fetch the base line for this row (empty string if base is shorter).
		baseLine := ""
		if i < len(baseLines) {
			baseLine = baseLines[i]
		}

		boxRow := i - yOff
		if boxRow < 0 || boxRow >= boxH {
			// Outside box vertical range — keep base line as-is.
			result[i] = baseLine
			continue
		}

		boxLine := boxLines[boxRow]

		// Left segment: first xOff visible columns of baseLine.
		left := ansi.Truncate(baseLine, xOff, "")

		// Pad left segment if baseLine is shorter than xOff.
		leftW := ansi.StringWidth(left)
		if leftW < xOff {
			left += strings.Repeat(" ", xOff-leftW)
		}

		// Right segment: base columns after (xOff + boxW).
		rightStart := xOff + boxW
		baseW := ansi.StringWidth(baseLine)
		right := ""
		if baseW > rightStart {
			right = ansiSkipCols(baseLine, rightStart)
		}

		result[i] = left + boxLine + right
	}

	return strings.Join(result, "\n")
}

// ansiSkipCols returns the suffix of s starting after the first n visible columns.
// It advances through the string rune-by-rune, tracking visual width via
// ansi.StringWidth on accumulated runes, and returns everything once n cols have
// been consumed. ANSI escape sequences consume 0 visual cols and are included in
// the prefix counting, so the returned suffix begins on a clean column boundary.
func ansiSkipCols(s string, n int) string {
	if n <= 0 {
		return s
	}
	// Walk rune-by-rune, tracking position in the string (byte index).
	// We accumulate a growing prefix and measure its width each iteration.
	// This is O(n²) but strings here are terminal-width (~80-200 chars) — acceptable.
	bytePos := 0
	visW := 0
	for bytePos < len(s) {
		// Measure the next rune's visual contribution.
		rem := s[bytePos:]
		// Find next rune's byte length.
		r, size := runeAt(rem)
		_ = r
		candidate := s[:bytePos+size]
		newW := ansi.StringWidth(candidate)
		if newW > n {
			// The next rune would exceed n — stop here.
			return s[bytePos:]
		}
		visW = newW
		bytePos += size
		if visW >= n {
			return s[bytePos:]
		}
	}
	return ""
}

// runeAt decodes the first UTF-8 rune from s, handling ANSI escape sequences
// as single units by advancing past the ESC sequence bytes.
// Returns the first rune and its byte length.
func runeAt(s string) (rune, int) {
	if len(s) == 0 {
		return 0, 0
	}
	// Fast path: plain ASCII.
	if s[0] != 0x1b {
		r := rune(s[0])
		if r < 0x80 {
			return r, 1
		}
		// Multi-byte UTF-8: use standard library.
		return decodeRune(s)
	}
	// ESC sequence: scan to end of sequence (letter terminator or end of string).
	// CSI: ESC '[' params... final (0x40-0x7e).
	// OSC: ESC ']' ... ST.
	// Simple ESC + one char (e.g. ESC m).
	if len(s) < 2 {
		return rune(s[0]), 1
	}
	switch s[1] {
	case '[':
		// CSI: read until a byte in 0x40-0x7e.
		i := 2
		for i < len(s) && (s[i] < 0x40 || s[i] > 0x7e) {
			i++
		}
		if i < len(s) {
			i++ // include terminator
		}
		return 0x1b, i
	default:
		return 0x1b, 2
	}
}

// decodeRune decodes the first rune from s using unicode/utf8.
func decodeRune(s string) (rune, int) {
	b0 := s[0]
	var n int
	switch {
	case b0&0xE0 == 0xC0:
		n = 2
	case b0&0xF0 == 0xE0:
		n = 3
	case b0&0xF8 == 0xF0:
		n = 4
	default:
		return rune(b0), 1
	}
	if len(s) < n {
		return rune(b0), 1
	}
	r := rune(b0 & (0xFF >> n))
	for i := 1; i < n; i++ {
		r = (r << 6) | rune(s[i]&0x3F)
	}
	return r, n
}
