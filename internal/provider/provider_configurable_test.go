package provider_test

import (
	"testing"
	"time"

	"daimon/internal/config"
	"daimon/internal/provider"
)

// TestConfigurableProvider verifies that all 5 concrete providers satisfy the
// ConfigurableProvider interface and that Config() returns a non-zero ProviderConfig
// matching the values used at construction. (REQ-20, task 1.2)
func TestConfigurableProvider_AllProvidersSatisfyInterface(t *testing.T) {
	cfg := config.ProviderConfig{
		Type:       "test",
		Model:      "test-model",
		APIKey:     "test-key",
		BaseURL:    "https://example.com",
		Timeout:    30 * time.Second,
		MaxRetries: 3,
	}

	t.Run("AnthropicProvider", func(t *testing.T) {
		p := provider.NewAnthropicProvider(cfg)
		var cp provider.ConfigurableProvider = p
		got := cp.Config()
		if got.APIKey != cfg.APIKey {
			t.Errorf("Config().APIKey = %q, want %q", got.APIKey, cfg.APIKey)
		}
		if got.Model != cfg.Model {
			t.Errorf("Config().Model = %q, want %q", got.Model, cfg.Model)
		}
	})

	t.Run("GeminiProvider", func(t *testing.T) {
		p := provider.NewGeminiProvider(cfg)
		var cp provider.ConfigurableProvider = p
		got := cp.Config()
		if got.APIKey != cfg.APIKey {
			t.Errorf("Config().APIKey = %q, want %q", got.APIKey, cfg.APIKey)
		}
	})

	t.Run("OpenRouterProvider", func(t *testing.T) {
		p := provider.NewOpenRouterProvider(cfg)
		var cp provider.ConfigurableProvider = p
		got := cp.Config()
		if got.APIKey != cfg.APIKey {
			t.Errorf("Config().APIKey = %q, want %q", got.APIKey, cfg.APIKey)
		}
	})

	t.Run("OpenAIProvider", func(t *testing.T) {
		openAICfg := config.ProviderConfig{
			Type:       "openai",
			Model:      "gpt-4o",
			APIKey:     "sk-test-key",
			BaseURL:    "https://api.openai.com/v1",
			Timeout:    30 * time.Second,
			MaxRetries: 2,
		}
		p, err := provider.NewOpenAIProvider(openAICfg)
		if err != nil {
			t.Fatalf("NewOpenAIProvider: %v", err)
		}
		var cp provider.ConfigurableProvider = p
		got := cp.Config()
		if got.APIKey != openAICfg.APIKey {
			t.Errorf("Config().APIKey = %q, want %q", got.APIKey, openAICfg.APIKey)
		}
		if got.Model != openAICfg.Model {
			t.Errorf("Config().Model = %q, want %q", got.Model, openAICfg.Model)
		}
	})

	t.Run("OllamaProvider", func(t *testing.T) {
		ollamaCfg := config.ProviderConfig{
			Type:    "ollama",
			Model:   "llama3",
			BaseURL: "http://localhost:11434/v1",
		}
		p, err := provider.NewOllamaProvider(ollamaCfg)
		if err != nil {
			t.Fatalf("NewOllamaProvider: %v", err)
		}
		var cp provider.ConfigurableProvider = p
		got := cp.Config()
		if got.Model != ollamaCfg.Model {
			t.Errorf("Config().Model = %q, want %q", got.Model, ollamaCfg.Model)
		}
	})
}

// TestConfigurableProvider_NonZeroAfterConstruction verifies that Config()
// returns non-zero values for all fields populated at construction time. (REQ-20)
func TestConfigurableProvider_NonZeroAfterConstruction(t *testing.T) {
	cfg := config.ProviderConfig{
		Type:    "anthropic",
		Model:   "claude-3-5-sonnet-20241022",
		APIKey:  "sk-ant-xxx",
		Timeout: 45 * time.Second,
	}
	p := provider.NewAnthropicProvider(cfg)
	got := p.Config()
	if got.Model == "" {
		t.Error("Config().Model is empty; expected non-zero after construction")
	}
	if got.APIKey == "" {
		t.Error("Config().APIKey is empty; expected non-zero after construction")
	}
	if got.Timeout == 0 {
		t.Error("Config().Timeout is zero; expected non-zero after construction")
	}
}
