package agent

// set_provider.go — SetProvider method + error sentinels for model-hot-swap PR2.
//
// Design references:
//   - AD-3: per-turn snapshot pattern (providerSnapshot already in agent.go)
//   - AD-4: lock ordering — cancels.mu first, then providerMu
//   - AD-5: thinking config re-apply after NewFromConfig
//   - AD-6: pre-validate via ModelLister with 5s timeout
//   - AD-7: atomic swap under providerMu.Lock(); no Close() on old provider
//   - AD-10: audit shape using Details map
//
// Spec coverage: REQ-1 (S1-1 to S1-5), REQ-5 (S5-1 to S5-3), REQ-8 (S8-1 to S8-3).

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"daimon/internal/audit"
	"daimon/internal/config"
	"daimon/internal/provider"
	"github.com/google/uuid"
)

// ---------------------------------------------------------------------------
// Error sentinels (REQ-5, design D6)
// ---------------------------------------------------------------------------

// ErrTurnInProgress is returned by SetProvider when a.cancels.Size() > 0,
// indicating at least one processMessage goroutine is currently running.
// The caller should use /cancel first and retry.
var ErrTurnInProgress = errors.New("turn in progress, retry in a moment (or /cancel first)")

// ErrInvalidModel is returned by SetProvider when the requested model name is
// not found in the provider's ModelLister.ListModels() response.
var ErrInvalidModel = errors.New("unknown model for this provider")

// ErrValidationTimeout is returned by SetProvider when the ModelLister call
// exceeds the 5-second validation timeout. We DO NOT fall through to an
// optimistic accept — we prefer explicit failure over silent typo acceptance.
var ErrValidationTimeout = errors.New("model validation timed out; retry or check provider availability")

// ErrProviderNotConfigurable is returned when the current provider does not
// implement provider.ConfigurableProvider — we cannot rebuild it without its
// config.
var ErrProviderNotConfigurable = errors.New("provider does not support hot-swap (ConfigurableProvider not implemented)")

// ---------------------------------------------------------------------------
// SetProvider
// ---------------------------------------------------------------------------

// SetProvider swaps the active LLM model within the current provider type.
// It is session-only: no write to config.yaml; process restart restores the
// original model.
//
// Thread-safety: safe for concurrent callers. Two simultaneous calls serialize
// via providerMu.Lock(). Last-write-wins semantics (see design AD-4).
//
// Lock ordering (per AD-4):
//
//	cancels.mu (inside Size()) → providerMu.RLock (snapshot) → [validation off-lock] → providerMu.Lock (swap)
//
// TODO(provider-lifecycle): known connection-pool drain gap on swap; providers
// have no Close(); future change to add Close() to Provider interface.
// See sdd/model-hot-swap change #3.
func (a *Agent) SetProvider(ctx context.Context, modelName string) error {
	// Step 1: Validate non-empty model name (fast, no locks needed).
	if modelName == "" {
		return fmt.Errorf("model-hot-swap: model name must not be empty")
	}

	// Step 2: Reject if any turn is in-flight (AD-3, REQ-5).
	// cancels.Size() takes its own internal lock (cancels.mu) and releases it.
	// We check BEFORE acquiring providerMu to avoid holding providerMu during
	// the registry check (which would invert the AD-4 lock ordering).
	if a.cancels.Size() > 0 {
		a.emitSwapAudit(ctx, "", modelName, "rejected_turn_in_progress")
		return ErrTurnInProgress
	}

	// Step 3: Snapshot current provider config under RLock.
	// We release RLock before the long-running validation step (AD-4 insight:
	// holding RLock during network I/O would block SetProvider's write-lock
	// acquisition and starve processMessage reads unnecessarily).
	a.providerMu.RLock()
	oldProv := a.provider
	a.providerMu.RUnlock()

	oldModel := oldProv.Model()

	// Step 4: Verify provider supports hot-swap (implements ConfigurableProvider).
	cfgProv, ok := oldProv.(provider.ConfigurableProvider)
	if !ok {
		return fmt.Errorf("model-hot-swap: provider %q does not support hot-swap (ConfigurableProvider not implemented)", oldProv.Name())
	}

	// Step 5: Build new config (clone old, replace model only — D1: model-within-provider only).
	oldCfg := cfgProv.Config()
	newCfg := oldCfg
	newCfg.Model = modelName

	// Step 6: Pre-validate via ModelLister (AD-6) with 5s timeout.
	// If provider does not implement ModelLister, fall through (optimistic).
	if lister, isLister := oldProv.(provider.ModelLister); isLister {
		validationCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		models, listErr := lister.ListModels(validationCtx)
		cancel()

		if listErr != nil {
			if errors.Is(listErr, context.DeadlineExceeded) {
				a.emitSwapAudit(ctx, oldModel, modelName, "validation_timeout")
				return ErrValidationTimeout
			}
			// Any other ListModels error: fall through optimistically.
			// The first Chat call will surface the API error if the model is invalid.
			slog.Warn("set_provider: ListModels failed, proceeding optimistically", "error", listErr)
		} else {
			// Check if the requested model is in the returned list.
			if !modelInList(modelName, models) {
				a.emitSwapAudit(ctx, oldModel, modelName, "rejected_invalid_model")
				available := modelIDList(models)
				return fmt.Errorf("%w %q for provider %q; available: %s", ErrInvalidModel, modelName, oldProv.Name(), available)
			}
		}
	}

	// Step 7: Build new provider (off-lock — may allocate, no agent lock held).
	// Uses a.newProviderFn which defaults to provider.NewFromConfig; overridable
	// in tests to avoid real API calls.
	newProv, err := a.newProviderFn(newCfg)
	if err != nil {
		a.emitSwapAudit(ctx, oldModel, modelName, "build_failed")
		return fmt.Errorf("model-hot-swap: failed to build provider with model %q: %w", modelName, err)
	}

	// Step 8: Re-apply thinking config (AD-5).
	// provider.NewFromConfig does NOT wire thinking config — registry.go does it
	// post-construction. We replicate that pattern using the stored providerCreds.
	applyThinkingConfig(newProv, a.providerCreds)

	// Step 9: Atomic swap under write lock (holds lock only for one assignment).
	a.providerMu.Lock()
	a.provider = newProv
	a.providerMu.Unlock()

	// Step 10: Emit success audit.
	a.emitSwapAudit(ctx, oldModel, modelName, "ok")

	return nil
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// applyThinkingConfig re-applies thinking configuration to a freshly-built
// provider. Mirrors provider/registry.go:52-65 which does the same after
// NewFromConfig at startup. Three providers support SetThinkingConfig:
// AnthropicProvider, GeminiProvider, OllamaProvider.
//
// Type assertions against concrete types are intentional (per design R2 note):
// if a provider ever wraps behind another interface layer, these asserts go
// silent — that is a future concern, not PR2's problem. Tests with mocks verify
// the calling pattern.
func applyThinkingConfig(prov provider.Provider, creds config.ProviderCredentials) {
	// Only bother if there's a thinking config to apply.
	if creds.Thinking == nil {
		return
	}
	switch p := prov.(type) {
	case *provider.AnthropicProvider:
		p.SetThinkingConfig(creds)
	case *provider.GeminiProvider:
		p.SetThinkingConfig(creds)
	case *provider.OllamaProvider:
		p.SetThinkingConfig(creds)
	}
}

// emitSwapAudit emits a model_swap audit event. Errors are logged but do NOT
// affect the SetProvider return value (audit is best-effort, per F2 in design).
func (a *Agent) emitSwapAudit(ctx context.Context, oldModel, newModel, outcome string) {
	ev := audit.AuditEvent{
		ID:        uuid.New().String(),
		EventType: "command",
		Timestamp: time.Now(),
		Model:     newModel,
		Details: map[string]string{
			"action":    "model_swap",
			"old_model": oldModel,
			"new_model": newModel,
			"outcome":   outcome,
		},
	}
	if err := a.auditorFn().Emit(ctx, ev); err != nil {
		slog.Warn("model_swap audit emit failed", "error", err, "outcome", outcome)
	}
}

// modelInList reports whether modelName equals any ModelInfo.ID in models.
func modelInList(modelName string, models []provider.ModelInfo) bool {
	for _, m := range models {
		if m.ID == modelName {
			return true
		}
	}
	return false
}

// modelIDList returns a comma-separated string of model IDs.
func modelIDList(models []provider.ModelInfo) string {
	if len(models) == 0 {
		return "(none)"
	}
	ids := make([]byte, 0, len(models)*20)
	for i, m := range models {
		if i > 0 {
			ids = append(ids, ',', ' ')
		}
		ids = append(ids, m.ID...)
	}
	return string(ids)
}
