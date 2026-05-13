package agent

import (
	"context"
	"testing"

	"daimon/internal/config"
	"daimon/internal/provider"
	"daimon/internal/skill"
)

// ---------------------------------------------------------------------------
// Task 1.9 — providerConfigForSkill with ConfigurableProvider (REQ-20)
// ---------------------------------------------------------------------------

// testConfigurableProvider is a test double that satisfies both provider.Provider
// and provider.ConfigurableProvider. It returns a known config so tests can
// assert inheritance behavior.
type testConfigurableProvider struct {
	cfg config.ProviderConfig
}

func (c *testConfigurableProvider) Name() string              { return c.cfg.Type }
func (c *testConfigurableProvider) Model() string             { return c.cfg.Model }
func (c *testConfigurableProvider) SupportsTools() bool       { return true }
func (c *testConfigurableProvider) SupportsMultimodal() bool  { return false }
func (c *testConfigurableProvider) SupportsAudio() bool       { return false }
func (c *testConfigurableProvider) HealthCheck(_ context.Context) (string, error) {
	return c.cfg.Model, nil
}
func (c *testConfigurableProvider) Chat(_ context.Context, _ provider.ChatRequest) (*provider.ChatResponse, error) {
	return &provider.ChatResponse{}, nil
}
func (c *testConfigurableProvider) Config() config.ProviderConfig { return c.cfg }

// Ensure testConfigurableProvider satisfies provider.ConfigurableProvider at compile time.
var _ provider.ConfigurableProvider = (*testConfigurableProvider)(nil)
var _ provider.Provider = (*testConfigurableProvider)(nil)

// testNonConfigurableProvider satisfies provider.Provider but NOT ConfigurableProvider.
type testNonConfigurableProvider struct{}

func (n *testNonConfigurableProvider) Name() string              { return "nonconfigurable" }
func (n *testNonConfigurableProvider) Model() string             { return "model-x" }
func (n *testNonConfigurableProvider) SupportsTools() bool       { return false }
func (n *testNonConfigurableProvider) SupportsMultimodal() bool  { return false }
func (n *testNonConfigurableProvider) SupportsAudio() bool       { return false }
func (n *testNonConfigurableProvider) HealthCheck(_ context.Context) (string, error) {
	return "model-x", nil
}
func (n *testNonConfigurableProvider) Chat(_ context.Context, _ provider.ChatRequest) (*provider.ChatResponse, error) {
	return &provider.ChatResponse{}, nil
}

// Ensure testNonConfigurableProvider does NOT satisfy ConfigurableProvider.
// (Absence of Config() method is the proof — verified by the missing interface assertion.)
var _ provider.Provider = (*testNonConfigurableProvider)(nil)

// TestProviderConfigForSkill_InheritsFromConfigurableParent verifies that when
// the parent satisfies provider.ConfigurableProvider, the child inherits the
// parent's API key and other credential fields. (REQ-20)
func TestProviderConfigForSkill_InheritsFromConfigurableParent(t *testing.T) {
	parentCfg := config.ProviderConfig{
		Type:       "anthropic",
		APIKey:     "sk-parent-key",
		Model:      "claude-sonnet-4-6",
		Timeout:    60_000_000_000, // 60s as time.Duration nanoseconds
		MaxRetries: 2,
	}
	parent := &testConfigurableProvider{cfg: parentCfg}

	// Skill declares the same provider type but no credentials.
	def := skill.ExecutableSkillDef{
		Name:         "test-skill",
		ProviderName: "anthropic",
		Model:        "", // empty — should inherit parent model
	}

	got := providerConfigForSkill(def, parent)

	if got.APIKey != parentCfg.APIKey {
		t.Errorf("APIKey = %q, want %q (should inherit from parent)", got.APIKey, parentCfg.APIKey)
	}
	if got.Model != parentCfg.Model {
		t.Errorf("Model = %q, want %q (should inherit from parent when skill.Model is empty)", got.Model, parentCfg.Model)
	}
	if got.Timeout != parentCfg.Timeout {
		t.Errorf("Timeout = %v, want %v (should inherit from parent)", got.Timeout, parentCfg.Timeout)
	}
}

// TestProviderConfigForSkill_FallsBackGracefully verifies that when the parent
// does NOT satisfy ConfigurableProvider, the function returns a config derived
// from the skill def only — no panic. (REQ-20)
func TestProviderConfigForSkill_FallsBackGracefully(t *testing.T) {
	parent := &testNonConfigurableProvider{}

	def := skill.ExecutableSkillDef{
		Name:         "skill-x",
		ProviderName: "nonconfigurable",
		Model:        "model-x",
	}

	defer func() {
		if r := recover(); r != nil {
			t.Errorf("providerConfigForSkill panicked with non-ConfigurableProvider parent: %v", r)
		}
	}()

	got := providerConfigForSkill(def, parent)
	// No panic is the key assertion. Credentials will be empty — expected.
	if got.APIKey != "" {
		t.Errorf("APIKey = %q, want empty (no inheritance from non-ConfigurableProvider parent)", got.APIKey)
	}
	if got.Type != "nonconfigurable" {
		t.Errorf("Type = %q, want 'nonconfigurable'", got.Type)
	}
}
