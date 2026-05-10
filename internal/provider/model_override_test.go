package provider_test

import (
	"testing"

	"daimon/internal/config"
	"daimon/internal/provider"
)

// TestProviderModelOverride_AllTypes verifies that every provider type honours
// the Model field passed via ProviderConfig to NewFromConfig. This guards
// against providers silently ignoring the model override (design risk §6 risk 6).
//
// If a provider returns a different model string, SubagentManager's budget
// accounting will use the wrong cost table — making this a correctness gate,
// not a style check.
func TestProviderModelOverride_AllTypes(t *testing.T) {
	const overrideModel = "override-model"

	tests := []struct {
		name    string
		cfg     config.ProviderConfig
		wantErr bool
	}{
		{
			name: "anthropic",
			cfg: config.ProviderConfig{
				Type:   "anthropic",
				Model:  overrideModel,
				APIKey: "test-key",
			},
		},
		{
			name: "openai",
			cfg: config.ProviderConfig{
				Type:   "openai",
				Model:  overrideModel,
				APIKey: "test-key",
			},
		},
		{
			name: "openrouter",
			cfg: config.ProviderConfig{
				Type:   "openrouter",
				Model:  overrideModel,
				APIKey: "test-key",
			},
		},
		{
			name: "gemini",
			cfg: config.ProviderConfig{
				Type:   "gemini",
				Model:  overrideModel,
				APIKey: "test-key",
			},
		},
		{
			name: "ollama",
			cfg: config.ProviderConfig{
				Type:    "ollama",
				Model:   overrideModel,
				BaseURL: "http://localhost:11434",
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p, err := provider.NewFromConfig(tc.cfg)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("NewFromConfig: unexpected error: %v", err)
			}
			if got := p.Model(); got != overrideModel {
				t.Errorf("Model() = %q, want %q — provider %q silently ignores Model override", got, overrideModel, tc.name)
			}
		})
	}
}
