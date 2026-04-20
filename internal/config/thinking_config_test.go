package config

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// ---------------------------------------------------------------------------
// Phase 1 — ProviderThinkingConfig: parse, defaults, legacy aliases, deprecation warn
// ---------------------------------------------------------------------------

// TestProviderThinkingConfig_ParseNested — Task 1.1 (RED → GREEN)
// Req 7, Sc: Config parsed correctly.
// budget_tokens: 8192 in YAML → Thinking.BudgetTokens == 8192
func TestProviderThinkingConfig_ParseNested(t *testing.T) {
	yamlData := `
providers:
  gemini:
    api_key: "test-key"
    thinking:
      budget_tokens: 8192
      effort: "high"
models:
  default:
    provider: gemini
    model: gemini-2.5-pro
`
	var cfg Config
	if err := yaml.Unmarshal([]byte(yamlData), &cfg); err != nil {
		t.Fatalf("yaml.Unmarshal: %v", err)
	}
	migrateThinkingConfig(&cfg)

	creds := cfg.Providers["gemini"]
	if creds.Thinking == nil {
		t.Fatal("expected Thinking to be non-nil")
	}
	if creds.Thinking.BudgetTokens != 8192 {
		t.Errorf("BudgetTokens = %d, want 8192", creds.Thinking.BudgetTokens)
	}
	if creds.Thinking.Effort != "high" {
		t.Errorf("Effort = %q, want %q", creds.Thinking.Effort, "high")
	}
}

// TestThinkingConfig_AbsentDefaults — Task 1.3 (RED → GREEN)
// Req 7, Sc: Absent block defaults to zero values.
// No thinking block → Thinking is nil (zero state: all sub-fields zero/empty)
func TestThinkingConfig_AbsentDefaults(t *testing.T) {
	yamlData := `
providers:
  gemini:
    api_key: "test-key"
models:
  default:
    provider: gemini
    model: gemini-2.5-pro
`
	var cfg Config
	if err := yaml.Unmarshal([]byte(yamlData), &cfg); err != nil {
		t.Fatalf("yaml.Unmarshal: %v", err)
	}
	migrateThinkingConfig(&cfg)

	creds := cfg.Providers["gemini"]
	// When absent, Thinking is nil — callers must treat nil as zero.
	if creds.Thinking != nil {
		if creds.Thinking.BudgetTokens != 0 {
			t.Errorf("BudgetTokens = %d, want 0", creds.Thinking.BudgetTokens)
		}
		if creds.Thinking.Effort != "" {
			t.Errorf("Effort = %q, want empty", creds.Thinking.Effort)
		}
		if creds.Thinking.Enabled != nil {
			t.Errorf("Enabled = %v, want nil", creds.Thinking.Enabled)
		}
	}
	// Whether nil or zero-value struct, both are acceptable and mean "not configured".
	// The test simply confirms no panic and no non-zero values.
}

// TestLegacyAliasThinkingEffort — Task 1.5 (RED → GREEN)
// Req 8, Sc: Legacy key still works.
// thinking_effort: "high" → Thinking.Effort == "high"
func TestLegacyAliasThinkingEffort(t *testing.T) {
	yamlData := `
providers:
  anthropic:
    api_key: "sk-ant-test"
    thinking_effort: "high"
models:
  default:
    provider: anthropic
    model: claude-opus-4-6
`
	var cfg Config
	if err := yaml.Unmarshal([]byte(yamlData), &cfg); err != nil {
		t.Fatalf("yaml.Unmarshal: %v", err)
	}
	migrateThinkingConfig(&cfg)

	creds := cfg.Providers["anthropic"]
	if creds.Thinking == nil {
		t.Fatal("expected Thinking to be non-nil after legacy migration")
	}
	if creds.Thinking.Effort != "high" {
		t.Errorf("Effort = %q, want %q", creds.Thinking.Effort, "high")
	}
}

// TestLegacyAlias_UnifiedTakesPrecedence — Task 1.6 (RED → GREEN)
// Req 8, Sc: Unified key takes precedence.
// Both thinking_effort: "low" and thinking.effort: "high" set → "high" wins.
func TestLegacyAlias_UnifiedTakesPrecedence(t *testing.T) {
	yamlData := `
providers:
  anthropic:
    api_key: "sk-ant-test"
    thinking_effort: "low"
    thinking:
      effort: "high"
models:
  default:
    provider: anthropic
    model: claude-opus-4-6
`
	var cfg Config
	if err := yaml.Unmarshal([]byte(yamlData), &cfg); err != nil {
		t.Fatalf("yaml.Unmarshal: %v", err)
	}
	migrateThinkingConfig(&cfg)

	creds := cfg.Providers["anthropic"]
	if creds.Thinking == nil {
		t.Fatal("expected Thinking to be non-nil")
	}
	if creds.Thinking.Effort != "high" {
		t.Errorf("Effort = %q, want 'high' (unified wins over legacy)", creds.Thinking.Effort)
	}
}

// TestLegacyAlias_EmitsDeprecationWarn — Task 1.7 (RED → GREEN)
// Req 9, Sc: Legacy key produces warn, not error.
// thinking_budget_tokens legacy key → WARN logged, app starts.
func TestLegacyAlias_EmitsDeprecationWarn(t *testing.T) {
	// Capture slog output via a custom handler.
	var buf bytes.Buffer
	handler := slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelWarn})
	oldDefault := slog.Default()
	slog.SetDefault(slog.New(handler))
	defer slog.SetDefault(oldDefault)

	yamlData := `
providers:
  anthropic:
    api_key: "sk-ant-test"
    thinking_budget_tokens: 4096
models:
  default:
    provider: anthropic
    model: claude-opus-4-6
`
	var cfg Config
	if err := yaml.Unmarshal([]byte(yamlData), &cfg); err != nil {
		t.Fatalf("yaml.Unmarshal: %v", err)
	}
	migrateThinkingConfig(&cfg)

	logOutput := buf.String()
	if !strings.Contains(logOutput, "deprecated") && !strings.Contains(logOutput, "thinking_budget_tokens") {
		t.Errorf("expected deprecation warning in log output, got: %q", logOutput)
	}

	// Confirm the value was migrated successfully (no error path).
	creds := cfg.Providers["anthropic"]
	if creds.Thinking == nil {
		t.Fatal("expected Thinking to be non-nil after legacy budget_tokens migration")
	}
	if creds.Thinking.BudgetTokens != 4096 {
		t.Errorf("BudgetTokens = %d, want 4096", creds.Thinking.BudgetTokens)
	}
}

// TestLegacyAlias_UnifiedKeyNoWarn — Req 9, Sc: Unified key produces no warning.
func TestLegacyAlias_UnifiedKeyNoWarn(t *testing.T) {
	var buf bytes.Buffer
	handler := slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelWarn})
	oldDefault := slog.Default()
	slog.SetDefault(slog.New(handler))
	defer slog.SetDefault(oldDefault)

	yamlData := `
providers:
  anthropic:
    api_key: "sk-ant-test"
    thinking:
      budget_tokens: 4096
models:
  default:
    provider: anthropic
    model: claude-opus-4-6
`
	var cfg Config
	if err := yaml.Unmarshal([]byte(yamlData), &cfg); err != nil {
		t.Fatalf("yaml.Unmarshal: %v", err)
	}
	migrateThinkingConfig(&cfg)

	logOutput := buf.String()
	if strings.Contains(logOutput, "deprecated") {
		t.Errorf("unexpected deprecation warning for unified key: %q", logOutput)
	}
}

// TestProviderThinkingConfig_EnabledPointer — Req 7 / ADR 6
// Enabled *bool correctly parsed: explicit false → not nil, points to false.
func TestProviderThinkingConfig_EnabledPointer(t *testing.T) {
	yamlData := `
providers:
  gemini:
    api_key: "test-key"
    thinking:
      enabled: false
      budget_tokens: 1024
models:
  default:
    provider: gemini
    model: gemini-2.5-pro
`
	var cfg Config
	if err := yaml.Unmarshal([]byte(yamlData), &cfg); err != nil {
		t.Fatalf("yaml.Unmarshal: %v", err)
	}

	creds := cfg.Providers["gemini"]
	if creds.Thinking == nil {
		t.Fatal("expected Thinking to be non-nil when block is present")
	}
	if creds.Thinking.Enabled == nil {
		t.Fatal("expected Enabled to be non-nil when explicitly set to false")
	}
	if *creds.Thinking.Enabled != false {
		t.Errorf("Enabled = %v, want false", *creds.Thinking.Enabled)
	}
}

