package tui

// setup_test.go — tests for embedded first-run setup TUI (PR-B1).
// Strict TDD: these tests must FAIL before setup.go exists.

import (
	"errors"
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

// ─── Fix 1: ollama Tab phantom field ───────────────────────────────────────

// TestSetupModel_OllamaTab_FieldIdxNeverExceedsOne drives provider=ollama to
// stepCredentials then presses Tab 10 times, asserting that fieldIdx never
// exceeds 1 (the valid field range for ollama: 0=model, 1=baseURL).
// Before the fix this MUST FAIL because fieldCount was 3 for ollama, making
// fieldIdx reach 2 — a dead state with no focused input.
func TestSetupModel_OllamaTab_FieldIdxNeverExceedsOne(t *testing.T) {
	m := newTestSetupModel("")

	// Navigate to ollama: it is the last provider; press Down until we reach it.
	// Use KeyRunes "j" to move down until provider == "ollama".
	for m.provider != "ollama" {
		next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
		nm := next.(setupModel)
		if nm.provIdx == m.provIdx {
			// We hit the bottom without finding ollama — fail fast.
			t.Fatalf("could not navigate to ollama; providers: %v", m.providers)
		}
		m = nm
		if m.provIdx == len(m.providers)-1 {
			break // at the bottom; enter selects it
		}
	}

	// Select ollama with Enter.
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(setupModel)
	if m.step != stepCredentials {
		t.Fatalf("expected stepCredentials after Enter on ollama, got %v", m.step)
	}
	if m.provider != "ollama" {
		t.Fatalf("expected provider=ollama, got %q", m.provider)
	}

	// Press Tab 10 times; fieldIdx must never exceed 1.
	for i := 0; i < 10; i++ {
		next2, _ := m.Update(tea.KeyMsg{Type: tea.KeyTab})
		m = next2.(setupModel)
		if m.fieldIdx > 1 {
			t.Errorf("Tab press %d: fieldIdx=%d exceeds max valid index 1 (ollama has 2 fields)", i+1, m.fieldIdx)
		}
		// Also assert the focused field is never the hidden keyInput.
		// After focusField, keyInput must be blurred when ollama.
		if m.keyInput.Focused() {
			t.Errorf("Tab press %d: keyInput is focused for ollama (it is a hidden field)", i+1)
		}
		// Positive focus: every reachable index must focus a VISIBLE field, so a
		// regression that blurs everything at index 1 (the original dead-state bug)
		// is caught. ollama: 0=model, 1=baseURL.
		switch m.fieldIdx {
		case 0:
			if !m.modelInput.Focused() {
				t.Errorf("Tab press %d: fieldIdx=0 but modelInput not focused", i+1)
			}
		case 1:
			if !m.baseURLInput.Focused() {
				t.Errorf("Tab press %d: fieldIdx=1 (ollama) but baseURLInput not focused", i+1)
			}
		}
	}
}

// ─── Fix 3: writeErr takes precedence over aborted ─────────────────────────

// TestResolveSetupError_WriteErrBeatsAborted asserts that when both writeErr
// and aborted are set, resolveSetupError returns the writeErr — not the abort
// sentinel. This tests the error-precedence logic extracted from RunSetupTUI.
func TestResolveSetupError_WriteErrBeatsAborted(t *testing.T) {
	writeErr := errors.New("disk full")

	final := setupModel{
		aborted:  true,
		writeErr: writeErr,
	}

	got := resolveSetupError(final)
	if got == nil {
		t.Fatal("resolveSetupError returned nil, want writeErr")
	}
	if !errors.Is(got, writeErr) {
		t.Errorf("resolveSetupError returned %v, want an error wrapping %v", got, writeErr)
	}
	if errors.Is(got, errSetupAborted) {
		t.Error("resolveSetupError returned the abort sentinel, writeErr should take precedence")
	}
}

// TestResolveSetupError_AbortedOnly asserts that when only aborted is set,
// resolveSetupError returns errSetupAborted.
func TestResolveSetupError_AbortedOnly(t *testing.T) {
	final := setupModel{aborted: true}
	got := resolveSetupError(final)
	if !errors.Is(got, errSetupAborted) {
		t.Errorf("resolveSetupError returned %v, want errSetupAborted", got)
	}
}

// TestResolveSetupError_Success asserts that when writtenPath is set and no
// error, resolveSetupError returns nil.
func TestResolveSetupError_Success(t *testing.T) {
	final := setupModel{writtenPath: "/tmp/config.yaml"}
	got := resolveSetupError(final)
	if got != nil {
		t.Errorf("resolveSetupError returned %v, want nil", got)
	}
}

// ─── Fix 4: non-vacuous gate tests ─────────────────────────────────────────

// TestSetupModel_StepCredentials_EmptyModelBlocksSubmit asserts that an empty
// model field (with non-empty key, non-ollama provider) blocks submission:
// step stays at stepCredentials and no cmd is returned.
func TestSetupModel_StepCredentials_EmptyModelBlocksSubmit(t *testing.T) {
	m := newTestSetupModel("")
	// Advance to stepCredentials (anthropic, provIdx=0)
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	nm := next.(setupModel)

	// Empty model, non-empty key
	nm.modelInput.SetValue("")
	nm.keyInput.SetValue("sk-ant-some-key")

	next2, cmd := nm.Update(tea.KeyMsg{Type: tea.KeyEnter})
	nm2 := next2.(setupModel)

	if nm2.step == stepDone {
		t.Error("step advanced to stepDone with empty model — gate failed")
	}
	if nm2.step != stepCredentials {
		t.Errorf("step = %v, want stepCredentials (empty model should block)", nm2.step)
	}
	if cmd != nil {
		t.Errorf("cmd = %v, want nil when empty model blocks submit", cmd)
	}
}

// TestSetupModel_StepCredentials_EmptyKeyBlocksSubmit_CmdNil strengthens the
// existing empty-key gate test by asserting cmd==nil (not just step check).
// Without this, the gate could be silently removed and a write cmd returned.
func TestSetupModel_StepCredentials_EmptyKeyBlocksSubmit_CmdNil(t *testing.T) {
	m := newTestSetupModel("")
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	nm := next.(setupModel)

	nm.modelInput.SetValue("claude-sonnet-4-6")
	nm.keyInput.SetValue("")

	_, cmd := nm.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd != nil {
		t.Errorf("cmd = %v, want nil when empty key blocks submit (write cmd must not be issued)", cmd)
	}
}

// TestSetupModel_StepCredentials_ValidSubmit_StepNotDoneSynchronously asserts
// that a valid submit does NOT advance step to stepDone synchronously — step
// only changes via the async setupWroteMsg, so immediately after Update the
// step must still be stepCredentials.
func TestSetupModel_StepCredentials_ValidSubmit_StepNotDoneSynchronously(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")

	m := newTestSetupModel(cfgPath)
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	nm := next.(setupModel)

	nm.modelInput.SetValue("claude-sonnet-4-6")
	nm.keyInput.SetValue("sk-ant-test-key")

	next2, _ := nm.Update(tea.KeyMsg{Type: tea.KeyEnter})
	nm2 := next2.(setupModel)

	if nm2.step == stepDone {
		t.Error("step advanced to stepDone synchronously — it must only change via async setupWroteMsg")
	}
}
