package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"microagent/internal/config"
)

// writeTestConfig creates a temporary config file and returns its path.
func writeTestConfig(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write test config: %v", err)
	}
	return path
}

const testConfigYAML = `
agent:
  name: "TestBot"
  personality: "helpful"
  max_iterations: 5
  max_tokens_per_turn: 2048
  history_length: 10
  memory_results: 3
provider:
  type: test_provider
  model: test-model-v1
  api_key: sk-secret-key-12345
  timeout: 30s
  max_retries: 2
  stream: true
channel:
  type: cli
store:
  type: file
  path: /tmp/test-microagent/data
logging:
  level: info
limits:
  tool_timeout: 15s
  total_timeout: 60s
`

func TestConfigShow_RedactsSecrets(t *testing.T) {
	path := writeTestConfig(t, testConfigYAML)

	// Load and show config — we test the redaction logic directly.
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	data, err := yaml.Marshal(cfg)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	output := redactSecrets(string(data))

	if strings.Contains(output, "sk-secret-key-12345") {
		t.Error("expected API key to be redacted, but it was present in output")
	}
	if !strings.Contains(output, "****") {
		t.Error("expected redacted placeholder '****' in output")
	}
	// Non-secret fields should still be present.
	if !strings.Contains(output, "test-model-v1") {
		t.Error("expected model name to be present in output")
	}
}

func TestConfigGet_ProviderModel(t *testing.T) {
	path := writeTestConfig(t, testConfigYAML)
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	val, err := getFieldByPath(cfg, "provider.model")
	if err != nil {
		t.Fatalf("get provider.model: %v", err)
	}
	if val != "test-model-v1" {
		t.Errorf("expected 'test-model-v1', got %q", val)
	}
}

func TestConfigGet_AgentName(t *testing.T) {
	path := writeTestConfig(t, testConfigYAML)
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	val, err := getFieldByPath(cfg, "agent.name")
	if err != nil {
		t.Fatalf("get agent.name: %v", err)
	}
	if val != "TestBot" {
		t.Errorf("expected 'TestBot', got %q", val)
	}
}

func TestConfigGet_ProviderStream(t *testing.T) {
	path := writeTestConfig(t, testConfigYAML)
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	val, err := getFieldByPath(cfg, "provider.stream")
	if err != nil {
		t.Fatalf("get provider.stream: %v", err)
	}
	if val != "true" {
		t.Errorf("expected 'true', got %q", val)
	}
}

func TestConfigGet_StoreType(t *testing.T) {
	path := writeTestConfig(t, testConfigYAML)
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	val, err := getFieldByPath(cfg, "store.type")
	if err != nil {
		t.Fatalf("get store.type: %v", err)
	}
	if val != "file" {
		t.Errorf("expected 'file', got %q", val)
	}
}

func TestConfigGet_ChannelType(t *testing.T) {
	path := writeTestConfig(t, testConfigYAML)
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	val, err := getFieldByPath(cfg, "channel.type")
	if err != nil {
		t.Fatalf("get channel.type: %v", err)
	}
	if val != "cli" {
		t.Errorf("expected 'cli', got %q", val)
	}
}

func TestConfigGet_UnknownPath(t *testing.T) {
	path := writeTestConfig(t, testConfigYAML)
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	_, err = getFieldByPath(cfg, "provider.nonexistent")
	if err == nil {
		t.Error("expected error for unknown path, got nil")
	}
	if !strings.Contains(err.Error(), "unknown config path") {
		t.Errorf("expected 'unknown config path' error, got %q", err.Error())
	}
}

func TestConfigSet_WritesValue(t *testing.T) {
	path := writeTestConfig(t, testConfigYAML)

	// Set via raw map approach.
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	var rawMap map[string]interface{}
	if err := yaml.Unmarshal(raw, &rawMap); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if err := setFieldInMap(rawMap, "provider.model", "new-model"); err != nil {
		t.Fatalf("set: %v", err)
	}

	data, err := yaml.Marshal(rawMap)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	// Verify the change by reloading.
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if cfg.Provider.Model != "new-model" {
		t.Errorf("expected 'new-model', got %q", cfg.Provider.Model)
	}
}

func TestConfigSet_BoolCoercion(t *testing.T) {
	path := writeTestConfig(t, testConfigYAML)

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	var rawMap map[string]interface{}
	if err := yaml.Unmarshal(raw, &rawMap); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if err := setFieldInMap(rawMap, "provider.stream", coerceValue("false")); err != nil {
		t.Fatalf("set: %v", err)
	}

	data, err := yaml.Marshal(rawMap)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if config.BoolVal(cfg.Provider.Stream) != false {
		t.Error("expected stream to be false after set")
	}
}

func TestConfigSet_IntCoercion(t *testing.T) {
	path := writeTestConfig(t, testConfigYAML)

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	var rawMap map[string]interface{}
	if err := yaml.Unmarshal(raw, &rawMap); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if err := setFieldInMap(rawMap, "agent.max_iterations", coerceValue("20")); err != nil {
		t.Fatalf("set: %v", err)
	}

	data, err := yaml.Marshal(rawMap)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if cfg.Agent.MaxIterations != 20 {
		t.Errorf("expected max_iterations=20, got %d", cfg.Agent.MaxIterations)
	}
}

func TestConfigValidate_ValidConfig(t *testing.T) {
	path := writeTestConfig(t, testConfigYAML)
	_, err := config.Load(path)
	if err != nil {
		t.Errorf("expected valid config, got error: %v", err)
	}
}

func TestConfigValidate_InvalidConfig(t *testing.T) {
	invalidYAML := `
provider:
  type: bogus_provider
  api_key: test
channel:
  type: cli
`
	path := writeTestConfig(t, invalidYAML)
	_, err := config.Load(path)
	if err == nil {
		t.Error("expected validation error for invalid config, got nil")
	}
}

func TestConfigPath(t *testing.T) {
	path := writeTestConfig(t, testConfigYAML)
	resolved, err := config.FindConfigPath(path)
	if err != nil {
		t.Fatalf("find config path: %v", err)
	}
	if resolved != path {
		t.Errorf("expected %q, got %q", path, resolved)
	}
}

func TestCoerceValue(t *testing.T) {
	tests := []struct {
		input    string
		expected interface{}
	}{
		{"true", true},
		{"false", false},
		{"42", 42},
		{"hello", "hello"},
		{"30s", "30s"},
		{"5m", "5m0s"},
	}

	for _, tt := range tests {
		got := coerceValue(tt.input)
		if got != tt.expected {
			t.Errorf("coerceValue(%q) = %v (%T), want %v (%T)", tt.input, got, got, tt.expected, tt.expected)
		}
	}
}

func TestRedactSecrets(t *testing.T) {
	input := `provider:
  type: anthropic
  api_key: sk-ant-very-secret
  model: claude-3
channel:
  type: telegram
  token: 12345:ABCDEF
store:
  encryption_key: deadbeef1234`

	output := redactSecrets(input)

	if strings.Contains(output, "sk-ant-very-secret") {
		t.Error("api_key not redacted")
	}
	if strings.Contains(output, "12345:ABCDEF") {
		t.Error("token not redacted")
	}
	if strings.Contains(output, "deadbeef1234") {
		t.Error("encryption_key not redacted")
	}
	if !strings.Contains(output, "anthropic") {
		t.Error("non-secret value was stripped")
	}
}

func TestStreamDefaultTrue(t *testing.T) {
	// Config without explicit stream field.
	yamlNoStream := `
provider:
  type: test_provider
  model: test
  api_key: test-key
channel:
  type: cli
`
	path := writeTestConfig(t, yamlNoStream)
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.Provider.Stream == nil {
		t.Fatal("expected stream to be non-nil after defaults")
	}
	if !*cfg.Provider.Stream {
		t.Error("expected stream to default to true")
	}
}

func TestStreamExplicitFalse(t *testing.T) {
	// Config with explicit stream: false.
	yamlStreamFalse := `
provider:
  type: test_provider
  model: test
  api_key: test-key
  stream: false
channel:
  type: cli
`
	path := writeTestConfig(t, yamlStreamFalse)
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.Provider.Stream == nil {
		t.Fatal("expected stream to be non-nil")
	}
	if *cfg.Provider.Stream {
		t.Error("expected stream to be false when explicitly set to false")
	}
}
