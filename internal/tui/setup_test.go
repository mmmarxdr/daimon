package tui

// setup_test.go — tests for embedded first-run setup TUI (PR-B1).
// Strict TDD: these tests must FAIL before setup.go exists.

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"daimon/internal/config"
)

// ─── buildSetupConfig table test ───────────────────────────────────────────

func TestBuildSetupConfig_Anthropic(t *testing.T) {
	cfg := buildSetupConfig("anthropic", "claude-sonnet-4-6", "sk-ant-123", "")

	// Provider credentials
	creds, ok := cfg.Providers["anthropic"]
	if !ok {
		t.Fatal("expected providers[anthropic] to exist")
	}
	if creds.APIKey != "sk-ant-123" {
		t.Errorf("APIKey = %q, want %q", creds.APIKey, "sk-ant-123")
	}
	if creds.BaseURL != "" {
		t.Errorf("BaseURL = %q, want empty", creds.BaseURL)
	}

	// Models
	if cfg.Models.Default.Provider != "anthropic" {
		t.Errorf("Models.Default.Provider = %q, want %q", cfg.Models.Default.Provider, "anthropic")
	}
	if cfg.Models.Default.Model != "claude-sonnet-4-6" {
		t.Errorf("Models.Default.Model = %q, want %q", cfg.Models.Default.Model, "claude-sonnet-4-6")
	}

	// Store must be sqlite (NOT "file" — that was the old wizard bug)
	if cfg.Store.Type != "sqlite" {
		t.Errorf("Store.Type = %q, want %q", cfg.Store.Type, "sqlite")
	}

	// Channel
	if cfg.Channel.Type != "cli" {
		t.Errorf("Channel.Type = %q, want %q", cfg.Channel.Type, "cli")
	}

	// Audit
	if cfg.Audit.Enabled == nil {
		t.Fatal("Audit.Enabled is nil, want *true")
	}
	if !*cfg.Audit.Enabled {
		t.Error("*Audit.Enabled = false, want true")
	}
	if cfg.Audit.Type != "sqlite" {
		t.Errorf("Audit.Type = %q, want %q", cfg.Audit.Type, "sqlite")
	}
}

func TestBuildSetupConfig_Ollama(t *testing.T) {
	cfg := buildSetupConfig("ollama", "llama3.2", "", "http://localhost:11434")

	creds, ok := cfg.Providers["ollama"]
	if !ok {
		t.Fatal("expected providers[ollama] to exist")
	}
	// ollama has no API key
	if creds.APIKey != "" {
		t.Errorf("APIKey = %q, want empty", creds.APIKey)
	}
	if creds.BaseURL != "http://localhost:11434" {
		t.Errorf("BaseURL = %q, want %q", creds.BaseURL, "http://localhost:11434")
	}

	if cfg.Models.Default.Provider != "ollama" {
		t.Errorf("Models.Default.Provider = %q, want %q", cfg.Models.Default.Provider, "ollama")
	}
	if cfg.Models.Default.Model != "llama3.2" {
		t.Errorf("Models.Default.Model = %q, want %q", cfg.Models.Default.Model, "llama3.2")
	}

	if cfg.Store.Type != "sqlite" {
		t.Errorf("Store.Type = %q, want %q", cfg.Store.Type, "sqlite")
	}
	if cfg.Channel.Type != "cli" {
		t.Errorf("Channel.Type = %q, want %q", cfg.Channel.Type, "cli")
	}
	if cfg.Audit.Enabled == nil || !*cfg.Audit.Enabled {
		t.Error("Audit.Enabled must be *true")
	}
}

// ─── setupModel Update tests ────────────────────────────────────────────────

func newTestSetupModel(cfgPath string) setupModel {
	s := newTuiStyles()
	providers := config.KnownProviders
	return setupModel{
		styles:       s,
		step:         stepProvider,
		cfgPath:      cfgPath,
		providers:    providers,
		provIdx:      0,
		modelInput:   textinput.New(),
		keyInput:     textinput.New(),
		baseURLInput: textinput.New(),
	}
}

func TestSetupModel_StepProvider_EnterAdvancesToCredentials(t *testing.T) {
	m := newTestSetupModel("")
	// Default provIdx=0 → "anthropic"

	msg := tea.KeyMsg{Type: tea.KeyEnter}
	next, _ := m.Update(msg)

	nm := next.(setupModel)
	if nm.step != stepCredentials {
		t.Errorf("step = %v, want stepCredentials", nm.step)
	}
	if nm.provider != "anthropic" {
		t.Errorf("provider = %q, want %q", nm.provider, "anthropic")
	}
	// modelInput should be prefilled with the default model
	if nm.modelInput.Value() == "" {
		t.Error("modelInput should be prefilled after advancing from stepProvider")
	}
}

func TestSetupModel_StepProvider_DownMovesIndex(t *testing.T) {
	m := newTestSetupModel("")
	msg := tea.KeyMsg{Type: tea.KeyDown}
	next, _ := m.Update(msg)
	nm := next.(setupModel)
	if nm.provIdx != 1 {
		t.Errorf("provIdx = %d, want 1", nm.provIdx)
	}
}

func TestSetupModel_StepCredentials_EmptyKeyBlocksSubmit(t *testing.T) {
	m := newTestSetupModel("")
	// Advance to stepCredentials first
	enterMsg := tea.KeyMsg{Type: tea.KeyEnter}
	next, _ := m.Update(enterMsg)
	nm := next.(setupModel)

	// Set model but leave key empty
	nm.modelInput.SetValue("claude-sonnet-4-6")
	nm.keyInput.SetValue("")

	// Try to submit
	next2, _ := nm.Update(tea.KeyMsg{Type: tea.KeyEnter})
	nm2 := next2.(setupModel)

	// Should NOT advance to stepDone
	if nm2.step == stepDone {
		t.Error("step advanced to stepDone with empty API key for non-ollama provider")
	}
	// The main assertion: step stays in stepCredentials (validation should block)
	if nm2.step != stepCredentials {
		t.Errorf("step = %v, want stepCredentials (validation should block)", nm2.step)
	}
}

func TestSetupModel_StepCredentials_ValidSubmitReturnsWriteCmd(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")

	m := newTestSetupModel(cfgPath)
	// Advance to credentials (anthropic)
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	nm := next.(setupModel)

	nm.modelInput.SetValue("claude-sonnet-4-6")
	nm.keyInput.SetValue("sk-ant-test-key")

	// Submit
	next2, cmd := nm.Update(tea.KeyMsg{Type: tea.KeyEnter})
	nm2 := next2.(setupModel)
	_ = nm2

	// A cmd must be returned (the write config cmd)
	if cmd == nil {
		t.Fatal("expected a non-nil Cmd from valid stepCredentials submit")
	}

	// Execute the cmd to get the message
	msg := cmd()
	wroteMsg, ok := msg.(setupWroteMsg)
	if !ok {
		t.Fatalf("cmd returned %T, want setupWroteMsg", msg)
	}
	if wroteMsg.err != nil {
		t.Fatalf("writeConfigCmd returned error: %v", wroteMsg.err)
	}
	if wroteMsg.path == "" {
		t.Error("setupWroteMsg.path is empty")
	}

	// Verify the file was actually written
	if _, err := os.Stat(cfgPath); err != nil {
		t.Errorf("config file not found at %s: %v", cfgPath, err)
	}

	// Reload and verify store.type
	loaded, err := config.Load(cfgPath)
	if err != nil {
		t.Fatalf("config.Load: %v", err)
	}
	if loaded.Store.Type != "sqlite" {
		t.Errorf("loaded Store.Type = %q, want sqlite", loaded.Store.Type)
	}
	if loaded.Models.Default.Provider != "anthropic" {
		t.Errorf("loaded Models.Default.Provider = %q, want anthropic", loaded.Models.Default.Provider)
	}
}

func TestSetupModel_CtrlC_SetsAborted(t *testing.T) {
	m := newTestSetupModel("")
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	nm := next.(setupModel)

	if !nm.aborted {
		t.Error("aborted should be true after ctrl+c")
	}
	if cmd == nil {
		t.Error("expected tea.Quit cmd from ctrl+c")
	}
	// Execute the cmd: tea.Quit returns tea.QuitMsg{}, not nil.
	msg := cmd()
	if _, ok := msg.(tea.QuitMsg); !ok {
		t.Errorf("ctrl+c cmd returned %T, want tea.QuitMsg", msg)
	}
}
