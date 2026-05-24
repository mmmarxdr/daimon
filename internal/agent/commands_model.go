package agent

// commands_model.go — /model slash command handler (model-hot-swap PR5).
//
// Spec coverage: REQ-2 (S2-1, S2-2), REQ-6 (S6-1..S6-3), REQ-7 (S7-1..S7-3),
//   REQ-9 (S9-1..S9-3), REQ-11 (S11-2).
//
// /model (no args) — lists current model and all available models for the
//   current provider, with the active model prefixed by "* ".
// /model <model-name> — swaps the active model by calling Agent.SetProvider.
//   Pre-validates via ModelLister when available (REQ-7). Maps the four error
//   sentinels to user-readable replies (REQ-7).

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"daimon/internal/provider"
)

// cmdModel implements the /model built-in slash command.
//
// No args:  list current model + available models (via ModelLister if supported).
// One arg:  call SetProvider with the requested model name.
//
// Argument grammar (REQ-2):
//   - Empty → list mode (S6-1..S6-3).
//   - Contains ":" → cross-provider attempt; rejected immediately (S2-2).
//   - Otherwise → model name for swap (S7-1..S7-3).
//
// Error sentinel mapping (REQ-7, REQ-5):
//
//	ErrTurnInProgress            → "A turn is currently in progress…"
//	ErrInvalidModel              → "unknown model <name>…" (should have been
//	                                caught by pre-validation, but map it anyway)
//	ErrValidationTimeout         → surfaces raw error with context
//	ErrProviderNotConfigurable   → "failed to set model: …"
func (a *Agent) cmdModel(cc CommandContext) error {
	arg := strings.TrimSpace(cc.Args)

	if arg == "" {
		return a.cmdModelList(cc)
	}
	return a.cmdModelSwap(cc, arg)
}

// cmdModelList handles /model with no arguments: lists available models and
// marks the current model with "* ". Output format (O-3 normative):
//
//	Current model: <name> (provider: <name>)
//
//	Available models:
//	  * <current-model>
//	    <other-model>
//	    ...
//	Usage: /model <model-name>
//
// Falls back to a brief one-liner when the provider does not implement ModelLister.
func (a *Agent) cmdModelList(cc CommandContext) error {
	prov := a.providerSnapshot()

	lister, ok := prov.(provider.ModelLister)
	if !ok {
		// Provider does not support listing (S6-2).
		cc.Reply(fmt.Sprintf("current model: %q (listing not supported by provider %q)",
			prov.Model(), prov.Name()))
		return nil
	}

	listCtx, cancel := context.WithCancel(cc.Ctx)
	defer cancel()

	models, err := lister.ListModels(listCtx)
	if err != nil {
		// ListModels error (S6-3).
		cc.Reply(fmt.Sprintf("could not list models for provider %q: %v", prov.Name(), err))
		return nil
	}

	currentModel := prov.Model()

	var sb strings.Builder
	// Header line (REQ-6 normative).
	sb.WriteString(fmt.Sprintf("Current model: %s (provider: %s)\n\n", currentModel, prov.Name()))
	sb.WriteString("Available models:\n")
	for _, m := range models {
		if m.ID == currentModel {
			sb.WriteString(fmt.Sprintf("  * %s\n", m.ID))
		} else {
			sb.WriteString(fmt.Sprintf("    %s\n", m.ID))
		}
	}
	sb.WriteString("Usage: /model <model-name>")

	cc.Reply(sb.String())
	return nil
}

// cmdModelSwap handles /model <model-name>: pre-validates the model name then
// calls Agent.SetProvider. Maps error sentinels to user-readable messages.
func (a *Agent) cmdModelSwap(cc CommandContext, modelName string) error {
	// Cross-provider syntax guard (S2-2, REQ-2).
	if strings.Contains(modelName, ":") {
		prov := a.providerSnapshot()
		cc.Reply(fmt.Sprintf(
			"cross-provider swap is not supported; current provider is %q. Specify model name only: /model <model-name>",
			prov.Name(),
		))
		return nil
	}

	// REQ-7: Pre-validate via ModelLister when available.
	// If the model is clearly invalid, reply without calling SetProvider.
	prov := a.providerSnapshot()
	if lister, ok := prov.(provider.ModelLister); ok {
		listCtx, cancel := context.WithCancel(cc.Ctx)
		models, listErr := lister.ListModels(listCtx)
		cancel()

		if listErr == nil {
			// List succeeded — pre-validate.
			if !modelInList(modelName, models) {
				available := modelIDList(models)
				cc.Reply(fmt.Sprintf("unknown model %q for provider %q. Available: %s",
					modelName, prov.Name(), available))
				return nil
			}
			// Model found — proceed to SetProvider (S7-2).
		}
		// listErr != nil: proceed optimistically (REQ-7: "do NOT block the swap attempt
		// on a failed listing call").
	}

	// Call SetProvider and map error sentinels to user-readable replies.
	err := a.SetProvider(cc.Ctx, modelName)
	if err == nil {
		cc.Reply(fmt.Sprintf("Model set to %q.", modelName))
		return nil
	}

	switch {
	case errors.Is(err, ErrTurnInProgress):
		cc.Reply("A turn is currently in progress. Try again in a moment, or use /cancel first.")
	case errors.Is(err, ErrInvalidModel):
		// Should have been caught by pre-validation, but surface gracefully.
		cc.Reply(fmt.Sprintf("unknown model %q — use /model with no args to list available models.", modelName))
	case errors.Is(err, ErrValidationTimeout):
		cc.Reply(fmt.Sprintf("model validation timed out: %v", err))
	default:
		// ErrProviderNotConfigurable + any NewFromConfig error.
		cc.Reply(fmt.Sprintf("failed to set model: %v", err))
	}

	return nil
}
