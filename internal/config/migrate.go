package config

import (
	"log"
	"log/slog"
)

// MigrateLegacyProviderPublic is the exported wrapper for migrateLegacyProvider.
// Use this when you have already unmarshaled a Config and need to apply migration
// outside of the Load function (e.g., in setup handlers that read and re-marshal).
func MigrateLegacyProviderPublic(cfg *Config) {
	migrateLegacyProvider(cfg)
}

// migrateLegacyProvider reshapes a v1 Config into v2 form in place. Idempotent.
//
// Trigger: cfg.Provider != nil && cfg.Provider.Type != "" &&
//
//	(cfg.Providers == nil || len(cfg.Providers) == 0)
//
// When triggered (pure v1 file):
//  1. Populate cfg.Providers[type] with APIKey + BaseURL from v1 block.
//  2. Set cfg.Models.Default from v1 Type + Model.
//  3. Move cfg.Provider.Fallback → cfg.Fallback if applicable (OQ-4).
//  4. Set cfg.Provider = nil so yaml.Marshal omits the legacy key via omitempty.
//
// Mixed v1+v2 (both provider: and providers: present):
// v2 wins — trigger is false because Providers is already non-empty.
// The legacy pointer is still nilled to prevent re-emission on the next save.
func migrateLegacyProvider(cfg *Config) {
	if cfg.Provider == nil {
		return
	}

	// Mixed case: v2 already populated. Nil the legacy pointer and exit.
	if len(cfg.Providers) > 0 {
		cfg.Provider = nil
		return
	}

	// Pure v1 case: trigger.
	if cfg.Provider.Type == "" {
		// Pointer is set but Type is empty — nothing to migrate.
		cfg.Provider = nil
		return
	}

	if cfg.Providers == nil {
		cfg.Providers = make(map[string]ProviderCredentials)
	}
	cfg.Providers[cfg.Provider.Type] = ProviderCredentials{
		APIKey:  cfg.Provider.APIKey,
		BaseURL: cfg.Provider.BaseURL,
	}
	cfg.Models.Default = ModelRef{
		Provider: cfg.Provider.Type,
		Model:    cfg.Provider.Model,
	}

	// OQ-4: migrate Fallback from Provider to Config top-level.
	if cfg.Provider.Fallback != nil && cfg.Fallback == nil {
		cfg.Fallback = cfg.Provider.Fallback
	}

	log.Printf("config: migrated legacy provider block into providers/models (v1→v2); file will be rewritten on next save")
	cfg.Provider = nil
}

// migrateThinkingConfig migrates legacy top-level thinking fields on each ProviderCredentials
// entry to the unified nested Thinking block. Idempotent.
//
// Rules (per ADR 6):
//  1. If unified Thinking block is present → it wins; legacy fields are ignored (no warn).
//  2. If only legacy fields are present → synthesise Thinking from them + emit slog.Warn once per provider.
//  3. If neither → leave Thinking nil (capability-map auto-activation applies at request time).
func migrateThinkingConfig(cfg *Config) {
	if len(cfg.Providers) == 0 {
		return
	}
	updated := make(map[string]ProviderCredentials, len(cfg.Providers))
	for name, creds := range cfg.Providers {
		hasLegacyEffort := creds.ThinkingEffort != ""
		hasLegacyBudget := creds.ThinkingBudgetTokens != nil
		hasLegacy := hasLegacyEffort || hasLegacyBudget

		if creds.Thinking != nil {
			// Unified block present — it wins. No migration, no warning.
			updated[name] = creds
			continue
		}

		if hasLegacy {
			// Emit deprecation warning.
			legacyKeys := make([]string, 0, 2)
			if hasLegacyEffort {
				legacyKeys = append(legacyKeys, "thinking_effort")
			}
			if hasLegacyBudget {
				legacyKeys = append(legacyKeys, "thinking_budget_tokens")
			}
			for _, k := range legacyKeys {
				slog.Warn("config: deprecated key detected; migrate to providers.<name>.thinking.*",
					"provider", name,
					"key", k,
					"deprecated", k,
				)
			}

			// Synthesise unified block.
			tc := &ProviderThinkingConfig{}
			if hasLegacyEffort {
				tc.Effort = creds.ThinkingEffort
			}
			if hasLegacyBudget {
				tc.BudgetTokens = *creds.ThinkingBudgetTokens
			}
			creds.Thinking = tc
		}

		updated[name] = creds
	}
	cfg.Providers = updated
}
