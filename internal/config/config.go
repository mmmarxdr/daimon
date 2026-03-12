package config

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Agent    AgentConfig    `yaml:"agent"`
	Provider ProviderConfig `yaml:"provider"`
	Channel  ChannelConfig  `yaml:"channel"`
	Tools    ToolsConfig    `yaml:"tools"`
	Store    StoreConfig    `yaml:"store"`
	Logging  LoggingConfig  `yaml:"logging"`
	Limits   LimitsConfig   `yaml:"limits"`
}

type AgentConfig struct {
	Name             string `yaml:"name"`
	Personality      string `yaml:"personality"`
	MaxIterations    int    `yaml:"max_iterations"`
	MaxTokensPerTurn int    `yaml:"max_tokens_per_turn"`
	HistoryLength    int    `yaml:"history_length"`
	MemoryResults    int    `yaml:"memory_results"`
}

type ProviderConfig struct {
	Type       string        `yaml:"type"`
	Model      string        `yaml:"model"`
	APIKey     string        `yaml:"api_key"`
	BaseURL    string        `yaml:"base_url"`
	Timeout    time.Duration `yaml:"timeout"`
	MaxRetries int           `yaml:"max_retries"`
}

type ChannelConfig struct {
	Type         string   `yaml:"type"`
	Token        string   `yaml:"token"` // e.g. for telegram
	AllowedUsers []string `yaml:"allowed_users"`
}

type ToolsConfig struct {
	Shell ShellToolConfig `yaml:"shell"`
	File  FileToolConfig  `yaml:"file"`
	HTTP  HTTPToolConfig  `yaml:"http"`
}

type ShellToolConfig struct {
	Enabled         bool     `yaml:"enabled"`
	AllowedCommands []string `yaml:"allowed_commands"`
	AllowAll        bool     `yaml:"allow_all"`
	WorkingDir      string   `yaml:"working_dir"`
}

type FileToolConfig struct {
	Enabled     bool   `yaml:"enabled"`
	BasePath    string `yaml:"base_path"`
	MaxFileSize string `yaml:"max_file_size"`
}

type HTTPToolConfig struct {
	Enabled         bool          `yaml:"enabled"`
	Timeout         time.Duration `yaml:"timeout"`
	MaxResponseSize string        `yaml:"max_response_size"`
	BlockedDomains  []string      `yaml:"blocked_domains"`
}

type StoreConfig struct {
	Type string `yaml:"type"`
	Path string `yaml:"path"`
}

type LoggingConfig struct {
	Level  string `yaml:"level"`
	Format string `yaml:"format"`
	File   string `yaml:"file"`
}

type LimitsConfig struct {
	ToolTimeout  time.Duration `yaml:"tool_timeout"`
	TotalTimeout time.Duration `yaml:"total_timeout"`
}

func (c *Config) applyDefaults() {
	if c.Agent.MaxIterations < 0 {
		c.Agent.MaxIterations = 10
	}
	if c.Agent.HistoryLength == 0 {
		c.Agent.HistoryLength = 20
	}
	if c.Agent.MemoryResults == 0 {
		c.Agent.MemoryResults = 5
	}
	if c.Agent.MaxTokensPerTurn == 0 {
		c.Agent.MaxTokensPerTurn = 4096
	}
	if c.Provider.Timeout == 0 {
		c.Provider.Timeout = 60 * time.Second
	}
	if c.Provider.MaxRetries == 0 {
		c.Provider.MaxRetries = 3
	}
	if c.Tools.File.MaxFileSize == "" {
		c.Tools.File.MaxFileSize = "1MB"
	}
	if c.Tools.HTTP.Timeout == 0 {
		c.Tools.HTTP.Timeout = 15 * time.Second
	}
	if c.Limits.ToolTimeout == 0 {
		c.Limits.ToolTimeout = 30 * time.Second
	}
	if c.Limits.TotalTimeout == 0 {
		c.Limits.TotalTimeout = 120 * time.Second
	}
	if c.Logging.Level == "" {
		c.Logging.Level = "info"
	}
	if c.Store.Type == "" {
		c.Store.Type = "file"
	}
}

func expandTilde(path string) string {
	if strings.HasPrefix(path, "~") {
		home, err := os.UserHomeDir()
		if err == nil {
			return filepath.Join(home, path[1:])
		}
	}
	return path
}

func (c *Config) resolvePaths() {
	c.Store.Path = expandTilde(c.Store.Path)
	c.Tools.File.BasePath = expandTilde(c.Tools.File.BasePath)
	c.Tools.Shell.WorkingDir = expandTilde(c.Tools.Shell.WorkingDir)
}

func (c *Config) validate() error {
	if c.Provider.APIKey == "" {
		return fmt.Errorf("provider.api_key is required")
	}
	switch c.Provider.Type {
	case "anthropic", "openai", "ollama", "test", "test_provider", "":
		// valid
	default:
		return fmt.Errorf("unknown provider.type: %s", c.Provider.Type)
	}

	switch c.Channel.Type {
	case "cli", "telegram", "discord", "test_channel", "":
		// valid
	default:
		return fmt.Errorf("unknown channel.type: %s", c.Channel.Type)
	}

	if c.Agent.MaxIterations <= 0 {
		return fmt.Errorf("agent.max_iterations must be positive")
	}
	if c.Limits.ToolTimeout > c.Limits.TotalTimeout {
		return fmt.Errorf("limits.tool_timeout cannot be greater than limits.total_timeout")
	}

	return nil
}

func expandSafeEnv(s string) (string, error) {
	// os.ExpandEnv simply removes unresolvable chunks. We want to catch ones that are explicitly meant as variables but missing,
	// except if they are malformed like ${PARTIAL. A regex gives us more control.
	re := regexp.MustCompile(`\$\{([a-zA-Z_][a-zA-Z0-9_]*)\}`)
	var validationErr error

	expanded := re.ReplaceAllStringFunc(s, func(match string) string {
		varName := match[2 : len(match)-1]
		val, exists := os.LookupEnv(varName)
		if !exists {
			validationErr = fmt.Errorf("required environment variable %s is not set", varName)
			return match
		}
		return val
	})

	return expanded, validationErr
}

func FindConfigPath(override string) (string, error) {
	if override != "" {
		if _, err := os.Stat(override); err == nil {
			return override, nil
		}
		return "", fmt.Errorf("config file not found at %s", override)
	}

	home, err := os.UserHomeDir()
	if err == nil {
		homePath := filepath.Join(home, ".microagent/config.yaml")
		if _, err := os.Stat(homePath); err == nil {
			return homePath, nil
		}
	}

	localPath := "./config.yaml"
	if _, err := os.Stat(localPath); err == nil {
		return localPath, nil
	}

	return "", fmt.Errorf("no config file found")
}

func Load(path string) (*Config, error) {
	resolvedPath, err := FindConfigPath(path)
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(resolvedPath)
	if err != nil {
		return nil, fmt.Errorf("reading config file: %w", err)
	}

	expanded, err := expandSafeEnv(string(data))
	if err != nil {
		return nil, fmt.Errorf("expanding env vars: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal([]byte(expanded), &cfg); err != nil {
		return nil, fmt.Errorf("parsing config: %w", err)
	}

	cfg.applyDefaults()
	if err := cfg.validate(); err != nil {
		return nil, fmt.Errorf("config validation failed: %w", err)
	}
	cfg.resolvePaths()

	return &cfg, nil
}
