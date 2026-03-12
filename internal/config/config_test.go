package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLoadConfig_ValidYAML(t *testing.T) {
	yamlData := `
agent:
  name: "TestAgent"
  personality: "Test Personality"
  max_iterations: 5
  max_tokens_per_turn: 1024
  history_length: 10
  memory_results: 3
provider:
  type: "test_provider"
  model: "test-model"
  api_key: "test-key"
  base_url: "http://test.com"
  timeout: 30s
  max_retries: 1
channel:
  type: "test_channel"
  token: "test-token"
  allowed_users: ["user1", "user2"]
tools:
  shell:
    enabled: true
    allowed_commands: ["echo"]
    allow_all: false
    working_dir: "/tmp"
  file:
    enabled: true
    base_path: "/data"
    max_file_size: "2MB"
  http:
    enabled: true
    timeout: 10s
    max_response_size: "1MB"
    blocked_domains: ["evil.com"]
store:
  type: "test_store"
  path: "/store"
logging:
  level: "debug"
  format: "json"
  file: "log.txt"
limits:
  tool_timeout: 15s
  total_timeout: 60s
`

	tmpFile := createTempFile(t, yamlData)
	defer os.Remove(tmpFile)

	cfg, err := Load(tmpFile)
	if err != nil {
		t.Fatalf("Load expected no error, got: %v", err)
	}

	if cfg.Agent.Name != "TestAgent" {
		t.Errorf("Expected Agent.Name 'TestAgent', got %q", cfg.Agent.Name)
	}
	if cfg.Provider.Timeout != 30*time.Second {
		t.Errorf("Expected Provider.Timeout 30s, got %v", cfg.Provider.Timeout)
	}
	if len(cfg.Channel.AllowedUsers) != 2 {
		t.Errorf("Expected 2 allowed users, got %d", len(cfg.Channel.AllowedUsers))
	}
}

func TestLoadConfig_Defaults(t *testing.T) {
	yamlData := `
provider:
  api_key: "test-key"
agent:
  max_iterations: 10
`

	tmpFile := createTempFile(t, yamlData)
	defer os.Remove(tmpFile)

	cfg, err := Load(tmpFile)
	if err != nil {
		t.Fatalf("Load expected no error, got: %v", err)
	}

	if cfg.Agent.MaxIterations != 10 {
		t.Errorf("Expected Agent.MaxIterations default 10, got %d", cfg.Agent.MaxIterations)
	}
	if cfg.Agent.HistoryLength != 20 {
		t.Errorf("Expected Agent.HistoryLength default 20, got %d", cfg.Agent.HistoryLength)
	}
	if cfg.Agent.MemoryResults != 5 {
		t.Errorf("Expected Agent.MemoryResults default 5, got %d", cfg.Agent.MemoryResults)
	}
	if cfg.Agent.MaxTokensPerTurn != 4096 {
		t.Errorf("Expected Agent.MaxTokensPerTurn default 4096, got %d", cfg.Agent.MaxTokensPerTurn)
	}
	if cfg.Provider.Timeout != 60*time.Second {
		t.Errorf("Expected Provider.Timeout default 60s, got %v", cfg.Provider.Timeout)
	}
	if cfg.Provider.MaxRetries != 3 {
		t.Errorf("Expected Provider.MaxRetries default 3, got %d", cfg.Provider.MaxRetries)
	}
	if cfg.Tools.Shell.AllowAll != false {
		t.Errorf("Expected Tools.Shell.AllowAll default false, got %t", cfg.Tools.Shell.AllowAll)
	}
	if cfg.Tools.File.MaxFileSize != "1MB" {
		t.Errorf("Expected Tools.File.MaxFileSize default 1MB, got %q", cfg.Tools.File.MaxFileSize)
	}
	if cfg.Tools.HTTP.Timeout != 15*time.Second {
		t.Errorf("Expected Tools.HTTP.Timeout default 15s, got %v", cfg.Tools.HTTP.Timeout)
	}
	if cfg.Limits.ToolTimeout != 30*time.Second {
		t.Errorf("Expected Limits.ToolTimeout default 30s, got %v", cfg.Limits.ToolTimeout)
	}
	if cfg.Limits.TotalTimeout != 120*time.Second {
		t.Errorf("Expected Limits.TotalTimeout default 120s, got %v", cfg.Limits.TotalTimeout)
	}
	if cfg.Logging.Level != "info" {
		t.Errorf("Expected Logging.Level default info, got %q", cfg.Logging.Level)
	}
	if cfg.Store.Type != "file" {
		t.Errorf("Expected Store.Type default file, got %q", cfg.Store.Type)
	}
}

func TestLoadConfig_EnvVarResolution(t *testing.T) {
	t.Setenv("TEST_API_KEY", "secret-from-env")

	tests := []struct {
		name     string
		yamlData string
		wantErr  bool
		checkAPI string
	}{
		{
			name: "resolves env var",
			yamlData: `
provider:
  type: "test"
  api_key: "${TEST_API_KEY}"
agent:
  max_iterations: 5
`,
			wantErr:  false,
			checkAPI: "secret-from-env",
		},
		{
			name: "undefined env var fails validation",
			yamlData: `
provider:
  type: "test"
  api_key: "${UNDEFINED_TEST_VAR}"
`,
			wantErr: true,
		},
		{
			name: "string literal maintained",
			yamlData: `
provider:
  type: "test"
  api_key: "direct-key"
agent:
  max_iterations: 5
`,
			wantErr:  false,
			checkAPI: "direct-key",
		},
		{
			name: "broken syntax literal",
			yamlData: `
provider:
  type: "test"
  api_key: "${PARTIAL"
agent:
  max_iterations: 5
`,
			wantErr:  false,
			checkAPI: "${PARTIAL",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tmpFile := createTempFile(t, tc.yamlData)
			defer os.Remove(tmpFile)

			cfg, err := Load(tmpFile)
			if tc.wantErr {
				if err == nil {
					t.Errorf("Expected error for %q, got nil", tc.name)
				}
			} else {
				if err != nil {
					t.Fatalf("Expected no error for %q, got: %v", tc.name, err)
				}
				if cfg.Provider.APIKey != tc.checkAPI {
					t.Errorf("Expected APIKey %q, got %q", tc.checkAPI, cfg.Provider.APIKey)
				}
			}
		})
	}
}

func TestLoadConfig_ValidationErrors(t *testing.T) {
	tests := []struct {
		name     string
		yamlData string
	}{
		{
			name: "empty api key",
			yamlData: `
provider:
  type: anthropic
  api_key: ""
`,
		},
		{
			name: "unknown provider type",
			yamlData: `
provider:
  api_key: "abc"
  type: "quantum_brain"
`,
		},
		{
			name: "unknown channel type",
			yamlData: `
provider:
  api_key: "abc"
channel:
  type: "telepathy"
`,
		},
		{
			name: "zero max iterations",
			yamlData: `
provider:
  api_key: "abc"
agent:
  max_iterations: 0
`,
		},
		{
			name: "tool timeout exceeds total",
			yamlData: `
provider:
  api_key: "abc"
limits:
  tool_timeout: 100s
  total_timeout: 50s
`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tmpFile := createTempFile(t, tc.yamlData)
			defer os.Remove(tmpFile)

			_, err := Load(tmpFile)
			if err == nil {
				t.Errorf("Expected validation error for %q, got nil", tc.name)
			}
		})
	}
}

func TestLoadConfig_TildeExpansion(t *testing.T) {
	homeDir, _ := os.UserHomeDir()

	yamlData := `
provider:
  api_key: "abc"
agent:
  max_iterations: 5
store:
  path: "~/.microagent/data"
tools:
  file:
    base_path: "~/workspace"
`

	tmpFile := createTempFile(t, yamlData)
	defer os.Remove(tmpFile)

	cfg, err := Load(tmpFile)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	expectedStorePath := filepath.Join(homeDir, ".microagent/data")
	if cfg.Store.Path != expectedStorePath {
		t.Errorf("Expected Store.Path %q, got %q", expectedStorePath, cfg.Store.Path)
	}

	expectedBasePath := filepath.Join(homeDir, "workspace")
	if cfg.Tools.File.BasePath != expectedBasePath {
		t.Errorf("Expected Tools.File.BasePath %q, got %q", expectedBasePath, cfg.Tools.File.BasePath)
	}

	// Verify path without tilde doesn't expand
	yamlDataNoTilde := `
provider:
  api_key: "abc"
agent:
  max_iterations: 5
store:
  path: "/absolute/path"
`
	tmpFile2 := createTempFile(t, yamlDataNoTilde)
	defer os.Remove(tmpFile2)

	cfg2, err := Load(tmpFile2)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if cfg2.Store.Path != "/absolute/path" {
		t.Errorf("Expected Store.Path '/absolute/path', got %q", cfg2.Store.Path)
	}
}

func TestLoadConfig_FilePriority(t *testing.T) {
	tmpDir := t.TempDir()

	// Create test configs
	flagConfig := filepath.Join(tmpDir, "flag.yaml")
	os.WriteFile(flagConfig, []byte("provider:\n  api_key: \"flag\"\nagent:\n  max_iterations: 5\n"), 0644)

	localConfig := filepath.Join(tmpDir, "config.yaml")
	os.WriteFile(localConfig, []byte("provider:\n  api_key: \"local\"\nagent:\n  max_iterations: 5\n"), 0644)

	homeConfigDir := filepath.Join(tmpDir, ".microagent")
	os.MkdirAll(homeConfigDir, 0755)
	homeConfig := filepath.Join(homeConfigDir, "config.yaml")
	os.WriteFile(homeConfig, []byte("provider:\n  api_key: \"home\"\nagent:\n  max_iterations: 5\n"), 0644)

	// Mock os.UserHomeDir temporarily by overriding the internal resolver var (we'll implement this hook in config.go later if needed, or just test logic)
	// For testing FilePriority logic per se, LoadAuto reads the rules.

	// Rule 1: Flag passed
	cfg, err := Load(flagConfig)
	if err != nil || cfg.Provider.APIKey != "flag" {
		t.Errorf("Expected to load flag config, got %v (err: %v)", cfg.Provider.APIKey, err)
	}

	// Rule 2 & 3: Find default paths. We will add a ResolvePath logic to test these cleanly in unit tests
}

func TestFindConfigPath_Override(t *testing.T) {
	t.Run("override path exists returns path", func(t *testing.T) {
		tmpDir := t.TempDir()
		cfgPath := filepath.Join(tmpDir, "my-config.yaml")
		if err := os.WriteFile(cfgPath, []byte(""), 0644); err != nil {
			t.Fatalf("setup: %v", err)
		}
		got, err := FindConfigPath(cfgPath)
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
		if got != cfgPath {
			t.Errorf("expected %q, got %q", cfgPath, got)
		}
	})

	t.Run("override path does not exist returns error", func(t *testing.T) {
		tmpDir := t.TempDir()
		nonexistent := filepath.Join(tmpDir, "nonexistent.yaml")
		_, err := FindConfigPath(nonexistent)
		if err == nil {
			t.Errorf("expected error for non-existent override path, got nil")
		}
	})
}

func TestFindConfigPath_NoOverride(t *testing.T) {
	t.Run("empty override exercises fallback logic without error if no files exist", func(t *testing.T) {
		// FindConfigPath("") tries ~/.microagent/config.yaml then ./config.yaml
		// Neither likely exists in a clean test environment; it should return an error about "no config file found"
		// We can't guarantee ./config.yaml doesn't exist in the working dir, so just verify the function doesn't panic
		// and returns either a valid path or an error.
		path, err := FindConfigPath("")
		if err != nil {
			// Expected when no config files are present — acceptable
			if !strings.Contains(err.Error(), "no config file found") {
				t.Errorf("expected 'no config file found' error, got: %v", err)
			}
		} else {
			// If a path is returned, it must be non-empty
			if path == "" {
				t.Errorf("expected non-empty path when no error returned")
			}
		}
	})
}

func TestLoad_InvalidYAML(t *testing.T) {
	invalidYAML := "not: valid: yaml: [unclosed"
	tmpFile := createTempFile(t, invalidYAML)
	defer os.Remove(tmpFile)

	_, err := Load(tmpFile)
	if err == nil {
		t.Fatal("expected error for invalid YAML, got nil")
	}
	if !strings.Contains(err.Error(), "parsing") && !strings.Contains(err.Error(), "yaml") {
		t.Errorf("expected error to mention 'parsing' or 'yaml', got: %v", err)
	}
}

func TestLoad_UnreadableFile(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("skipping permission test: running as root")
	}

	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "unreadable.yaml")
	if err := os.WriteFile(cfgPath, []byte("provider:\n  api_key: abc\n"), 0644); err != nil {
		t.Fatalf("setup: %v", err)
	}
	if err := os.Chmod(cfgPath, 0000); err != nil {
		t.Fatalf("chmod: %v", err)
	}

	_, err := Load(cfgPath)
	if err == nil {
		t.Fatal("expected error for unreadable file, got nil")
	}
	if !strings.Contains(err.Error(), "reading") {
		t.Errorf("expected error to mention 'reading', got: %v", err)
	}
}

func createTempFile(t *testing.T, content string) string {
	t.Helper()
	f, err := os.CreateTemp("", "microagent-config-*.yaml")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	if _, err := f.Write([]byte(strings.TrimSpace(content))); err != nil {
		t.Fatalf("failed to write to temp file: %v", err)
	}
	f.Close()
	return f.Name()
}
