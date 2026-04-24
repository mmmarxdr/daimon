package main

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"daimon/internal/audit"
	"daimon/internal/provider"
)

// wireRuntimePricing fetches the model list from a ModelLister-capable
// provider and registers a runtime price lookup with the audit package so
// EstimateCost can price models that aren't in the offline `modelPricing`
// map (most of OpenRouter's catalogue, for example).
//
// The fetch happens once at startup with a short timeout; if it fails the
// audit package falls back to the hardcoded map. The lookup map itself is
// refreshed every refreshInterval in the background so newly-released
// models become priced without restarting daimon.
//
// Returns a stop function that the caller defers to halt the refresh loop.
// Returns a no-op when the provider does not implement ModelLister.
func wireRuntimePricing(prov provider.Provider, refreshInterval time.Duration) func() {
	lister, ok := prov.(provider.ModelLister)
	if !ok {
		slog.Debug("runtime_pricing: provider does not implement ModelLister, using offline map only")
		return func() {}
	}

	type modelInfo struct {
		in, out       float64
		contextLength int
	}
	var (
		mu     sync.RWMutex
		models = map[string]modelInfo{}
	)

	refresh := func() int {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		list, err := lister.ListModels(ctx)
		if err != nil {
			slog.Warn("runtime_pricing: fetch failed", "error", err)
			return 0
		}
		next := make(map[string]modelInfo, len(list))
		for _, m := range list {
			// Keep entries with EITHER pricing or context length info — the
			// pricing lookup falls through to the hardcoded map for free
			// models, but context length is unique to the live fetch.
			if m.PromptCost == 0 && m.CompletionCost == 0 && m.ContextLength == 0 {
				continue
			}
			next[m.ID] = modelInfo{
				in:            m.PromptCost,
				out:           m.CompletionCost,
				contextLength: m.ContextLength,
			}
		}
		mu.Lock()
		models = next
		mu.Unlock()
		return len(next)
	}

	audit.SetPriceLookup(func(model string) (float64, float64, bool) {
		mu.RLock()
		defer mu.RUnlock()
		m, ok := models[model]
		if !ok || (m.in == 0 && m.out == 0) {
			return 0, 0, false
		}
		return m.in, m.out, true
	})

	audit.SetContextLengthLookup(func(model string) (int, bool) {
		mu.RLock()
		defer mu.RUnlock()
		m, ok := models[model]
		if !ok || m.contextLength <= 0 {
			return 0, false
		}
		return m.contextLength, true
	})

	if n := refresh(); n > 0 {
		slog.Info("runtime_pricing: loaded", "models_with_pricing", n)
	}

	if refreshInterval <= 0 {
		refreshInterval = 6 * time.Hour
	}
	stopCh := make(chan struct{})
	go func() {
		t := time.NewTicker(refreshInterval)
		defer t.Stop()
		for {
			select {
			case <-stopCh:
				return
			case <-t.C:
				_ = refresh()
			}
		}
	}()

	return func() {
		close(stopCh)
		audit.SetPriceLookup(nil)
		audit.SetContextLengthLookup(nil)
	}
}
