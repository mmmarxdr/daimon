package agent

// commands_mode.go — /mode slash command handler (mode-system PR3).
//
// Spec coverage: REQ-1, REQ-2, REQ-3, REQ-10, REQ-12.
//
// /mode (no args) — lists all known modes, marking the current mode with "* ".
// /mode <name>   — swaps the active mode by calling Agent.SetMode.
//   Maps ErrInvalidMode and ErrTurnInProgress to exact user-readable replies
//   per AD-11. On success: emits a best-effort telemetry frame per REQ-12, AD-7.

import (
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"daimon/internal/channel"
)

// modeDescriptions maps mode names to their one-line description used in the
// /mode no-args list. These are the AD-11 normative per-line descriptions.
var modeDescriptions = map[string]string{
	"plan":   "read-only analysis and proposals",
	"build":  "default; all tools available",
	"review": "diff and audit; read-only execution",
}

// cmdMode implements the /mode built-in slash command.
//
// No args:  list all modes and mark the current mode with "* ".
// One arg:  call SetMode with the requested mode name.
func (a *Agent) cmdMode(cc CommandContext) error {
	arg := strings.TrimSpace(cc.Args)
	if arg == "" {
		return a.cmdModeList(cc)
	}
	return a.cmdModeSwap(cc, arg)
}

// cmdModeList handles /mode with no arguments. Output format (AD-11 normative):
//
//	Available modes:
//	  plan   — read-only analysis and proposals
//	* build  — default; all tools available (current)
//	  review — diff and audit; read-only execution
//
//	Use /mode <name> to switch.
//
// The "* " prefix is applied to the active mode; two-space indent for all others.
func (a *Agent) cmdModeList(cc CommandContext) error {
	current := a.modeSnapshot().Name

	var sb strings.Builder
	sb.WriteString("Available modes:\n")
	for _, name := range ModeNames() {
		desc := modeDescriptions[name]
		if name == current {
			sb.WriteString(fmt.Sprintf("* %-6s — %s (current)\n", name, desc))
		} else {
			sb.WriteString(fmt.Sprintf("  %-6s — %s\n", name, desc))
		}
	}
	sb.WriteString("\nUse /mode <name> to switch.")
	sb.WriteString("\nNote: memory and RAG remain active in all modes.")

	cc.Reply(sb.String())
	return nil
}

// cmdModeSwap handles /mode <name>: validates and calls Agent.SetMode.
// Maps ErrInvalidMode and ErrTurnInProgress to exact reply strings per AD-11.
// On success: emits best-effort telemetry frame (REQ-12, AD-7).
func (a *Agent) cmdModeSwap(cc CommandContext, name string) error {
	if err := a.SetMode(cc.Ctx, cc.ChannelID, cc.SenderID, name); err != nil {
		switch {
		case errors.Is(err, ErrTurnInProgress):
			// AD-11 exact string for ErrTurnInProgress
			cc.Reply("A turn is currently in progress. Try again in a moment, or use /cancel first.")
		case errors.Is(err, ErrInvalidMode):
			// AD-11 exact string for ErrInvalidMode
			cc.Reply(fmt.Sprintf("unknown mode %q. Use /mode with no args to list available modes.", name))
		default:
			cc.Reply(fmt.Sprintf("failed to set mode: %v", err))
		}
		return nil
	}

	// Best-effort telemetry emit (AD-7 / REQ-12).
	// Mirror of model-hot-swap's audit emit pattern — failure is logged but does
	// NOT undo the swap or block the success reply.
	if te, ok := a.channel.(channel.TelemetryEmitter); ok {
		if emitErr := te.EmitTelemetry(cc.Ctx, cc.ChannelID, map[string]any{
			"type": "mode.changed",
			"mode": name,
		}); emitErr != nil {
			slog.Warn("mode.changed telemetry emit failed", "error", emitErr, "mode", name)
		}
	}

	cc.Reply(fmt.Sprintf("Mode set to %q.", name))
	return nil
}
