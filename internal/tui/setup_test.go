package tui

// setup_test.go — tests for embedded first-run setup TUI (PR-B1 + PR-B2).
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
	cfg := buildSetupConfig("anthropic", "claude-sonnet-4-6", "sk-ant-123", "", ragSetup{})

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
	cfg := buildSetupConfig("ollama", "llama3.2", "", "http://localhost:11434", ragSetup{})

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
	embKeyIn := textinput.New()
	embKeyIn.EchoMode = textinput.EchoPassword
	return setupModel{
		styles:        s,
		step:          stepProvider,
		cfgPath:       cfgPath,
		providers:     config.KnownProviders,
		provIdx:       0,
		modelInput:    textinput.New(),
		keyInput:      textinput.New(),
		baseURLInput:  textinput.New(),
		ragProviders:  ragEmbeddingProviders,
		embModelInput: textinput.New(),
		embKeyInput:   embKeyIn,
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

// TestSetupModel_StepCredentials_ValidSubmitReturnsWriteCmd drives the full
// no-RAG path: credentials → stepRAGEnable → 'n' → writeConfigCmd.
// PR-B2 changed the direct-write from stepCredentials to go through stepRAGEnable;
// this test is updated to reflect the new flow.
func TestSetupModel_StepCredentials_ValidSubmitReturnsWriteCmd(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")

	m := newTestSetupModel(cfgPath)
	// Advance to credentials (anthropic)
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	nm := next.(setupModel)

	nm.modelInput.SetValue("claude-sonnet-4-6")
	nm.keyInput.SetValue("sk-ant-test-key")

	// Submit credentials → now goes to stepRAGEnable (not direct write).
	next2, _ := nm.Update(tea.KeyMsg{Type: tea.KeyEnter})
	nm2 := next2.(setupModel)
	if nm2.step != stepRAGEnable {
		t.Fatalf("expected stepRAGEnable after credentials submit, got %v", nm2.step)
	}

	// Press 'n' to skip RAG → write cmd issued.
	next3, cmd := nm2.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("n")})
	_ = next3
	if cmd == nil {
		t.Fatal("expected a non-nil Cmd after 'n' at stepRAGEnable")
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

// ─── PR-B2: buildSetupConfig RAG tests ────────────────────────────────────

// TestBuildSetupConfig_RAGEnabled asserts that when ragSetup.enabled is true,
// cfg.RAG is populated with the embedding fields.
func TestBuildSetupConfig_RAGEnabled(t *testing.T) {
	rag := ragSetup{
		enabled:  true,
		provider: "openai",
		model:    "text-embedding-3-small",
		apiKey:   "sk-x",
	}
	cfg := buildSetupConfig("anthropic", "claude-sonnet-4-6", "sk-ant-123", "", rag)

	if !cfg.RAG.Enabled {
		t.Error("cfg.RAG.Enabled = false, want true")
	}
	if !cfg.RAG.Embedding.Enabled {
		t.Error("cfg.RAG.Embedding.Enabled = false, want true")
	}
	if cfg.RAG.Embedding.Provider != "openai" {
		t.Errorf("cfg.RAG.Embedding.Provider = %q, want %q", cfg.RAG.Embedding.Provider, "openai")
	}
	if cfg.RAG.Embedding.Model != "text-embedding-3-small" {
		t.Errorf("cfg.RAG.Embedding.Model = %q, want %q", cfg.RAG.Embedding.Model, "text-embedding-3-small")
	}
	if cfg.RAG.Embedding.APIKey != "sk-x" {
		t.Errorf("cfg.RAG.Embedding.APIKey = %q, want %q", cfg.RAG.Embedding.APIKey, "sk-x")
	}
}

// TestBuildSetupConfig_RAGDisabled asserts that when ragSetup is zero,
// cfg.RAG stays at its zero value (Enabled==false).
func TestBuildSetupConfig_RAGDisabled(t *testing.T) {
	cfg := buildSetupConfig("anthropic", "claude-sonnet-4-6", "sk-ant-123", "", ragSetup{})

	if cfg.RAG.Enabled {
		t.Error("cfg.RAG.Enabled = true, want false (RAG not requested)")
	}
	if cfg.RAG.Embedding.Enabled {
		t.Error("cfg.RAG.Embedding.Enabled = true, want false")
	}
}

// ─── PR-B2: flow tests ─────────────────────────────────────────────────────

// TestSetupModel_CredentialsSubmit_GoesToRAGEnable asserts that after a
// valid stepCredentials submit the model transitions to stepRAGEnable (NOT
// directly to a write cmd).
func TestSetupModel_CredentialsSubmit_GoesToRAGEnable(t *testing.T) {
	m := newTestSetupModel("")
	// Advance to credentials (anthropic, index 0)
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	nm := next.(setupModel)

	nm.modelInput.SetValue("claude-sonnet-4-6")
	nm.keyInput.SetValue("sk-ant-test-key")

	next2, cmd := nm.Update(tea.KeyMsg{Type: tea.KeyEnter})
	nm2 := next2.(setupModel)

	if nm2.step != stepRAGEnable {
		t.Errorf("step = %v, want stepRAGEnable after valid credentials submit", nm2.step)
	}
	// cmd must be nil here — no write yet, just advancing step
	if cmd != nil {
		msg := cmd()
		if _, ok := msg.(setupWroteMsg); ok {
			t.Error("writeConfigCmd was issued before RAG enable step — must go to stepRAGEnable first")
		}
	}
}

// TestSetupModel_RAGEnable_N_GoesToWrite asserts that pressing 'n' at
// stepRAGEnable skips RAG and returns the write cmd (RAG disabled).
func TestSetupModel_RAGEnable_N_GoesToWrite(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")

	m := newTestSetupModel(cfgPath)
	m = advanceToRAGEnable(t, m)

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("n")})
	if cmd == nil {
		t.Fatal("expected a non-nil write cmd after 'n' at stepRAGEnable")
	}
	msg := cmd()
	wroteMsg, ok := msg.(setupWroteMsg)
	if !ok {
		t.Fatalf("cmd returned %T, want setupWroteMsg", msg)
	}
	if wroteMsg.err != nil {
		t.Fatalf("writeConfigCmd error: %v", wroteMsg.err)
	}
}

// TestSetupModel_RAGEnable_Y_GoesToRAGProvider asserts that pressing 'y' at
// stepRAGEnable transitions to stepRAGProvider.
func TestSetupModel_RAGEnable_Y_GoesToRAGProvider(t *testing.T) {
	m := newTestSetupModel("")
	m = advanceToRAGEnable(t, m)

	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("y")})
	nm := next.(setupModel)

	if nm.step != stepRAGProvider {
		t.Errorf("step = %v, want stepRAGProvider after 'y' at stepRAGEnable", nm.step)
	}
}

// TestSetupModel_RAGEnable_Enter_Default_GoesToWrite asserts that pressing
// Enter at stepRAGEnable with default selection (No / RAG disabled) goes to write.
func TestSetupModel_RAGEnable_Enter_Default_GoesToWrite(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")

	m := newTestSetupModel(cfgPath)
	m = advanceToRAGEnable(t, m)

	// Default is No (ragOptIdx == 0). Press Enter.
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("expected write cmd from Enter with default No at stepRAGEnable")
	}
	msg := cmd()
	if _, ok := msg.(setupWroteMsg); !ok {
		t.Fatalf("cmd returned %T, want setupWroteMsg", msg)
	}
}

// TestSetupModel_RAGEnable_Enter_Yes_GoesToProvider asserts that when
// ragOptIdx is toggled to 1 (Yes) and Enter is pressed, we go to stepRAGProvider.
func TestSetupModel_RAGEnable_Enter_Yes_GoesToProvider(t *testing.T) {
	m := newTestSetupModel("")
	m = advanceToRAGEnable(t, m)

	// Toggle to Yes with Down key.
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m = next.(setupModel)

	next2, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	nm := next2.(setupModel)

	if nm.step != stepRAGProvider {
		t.Errorf("step = %v, want stepRAGProvider after Enter on Yes", nm.step)
	}
}

// TestSetupModel_RAGEnable_Esc_BackToCredentials asserts that Esc at
// stepRAGEnable goes back to stepCredentials.
func TestSetupModel_RAGEnable_Esc_BackToCredentials(t *testing.T) {
	m := newTestSetupModel("")
	m = advanceToRAGEnable(t, m)

	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	nm := next.(setupModel)

	if nm.step != stepCredentials {
		t.Errorf("step = %v, want stepCredentials after Esc at stepRAGEnable", nm.step)
	}
}

// TestSetupModel_RAGProvider_EnterGoesToRAGCreds asserts that pressing Enter
// on a provider at stepRAGProvider advances to stepRAGCreds and prefills embModelInput.
func TestSetupModel_RAGProvider_EnterGoesToRAGCreds(t *testing.T) {
	m := newTestSetupModel("")
	m = advanceToRAGProvider(t, m)

	// Press Enter (first provider is openai by convention).
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	nm := next.(setupModel)

	if nm.step != stepRAGCreds {
		t.Errorf("step = %v, want stepRAGCreds after Enter at stepRAGProvider", nm.step)
	}
	// embModelInput should be prefilled with a sensible default.
	if nm.embModelInput.Value() == "" {
		t.Error("embModelInput should be prefilled with a default model after stepRAGProvider enter")
	}
}

// TestSetupModel_RAGCreds_EmptyKey_Blocks asserts that submitting with an
// empty embKeyInput at stepRAGCreds does not issue a write cmd.
func TestSetupModel_RAGCreds_EmptyKey_Blocks(t *testing.T) {
	m := newTestSetupModel("")
	m = advanceToRAGCreds(t, m)

	// Leave embKeyInput empty; embModelInput may be prefilled.
	m.embKeyInput.SetValue("")

	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	nm := next.(setupModel)
	_ = nm

	if cmd != nil {
		msg := cmd()
		if _, ok := msg.(setupWroteMsg); ok {
			t.Error("writeConfigCmd issued with empty embKey — gate must block")
		}
	}
}

// TestSetupModel_RAGCreds_WithKey_IssuesWriteCmd asserts that a valid
// embKeyInput at stepRAGCreds issues the write cmd.
func TestSetupModel_RAGCreds_WithKey_IssuesWriteCmd(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")

	m := newTestSetupModel(cfgPath)
	m = advanceToRAGCreds(t, m)

	m.embKeyInput.SetValue("emb-secret-key")

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("expected write cmd from valid RAG creds submit")
	}
	msg := cmd()
	wroteMsg, ok := msg.(setupWroteMsg)
	if !ok {
		t.Fatalf("cmd returned %T, want setupWroteMsg", msg)
	}
	if wroteMsg.err != nil {
		t.Fatalf("writeConfigCmd error: %v", wroteMsg.err)
	}
}

// ─── PR-B2: RAG creds field-cycling consistency ────────────────────────────

// TestSetupModel_RAGCreds_Tab_FieldIdxNeverExceedsOne asserts that Tab
// cycling at stepRAGCreds keeps fieldIdx in {0,1} and always focuses a
// visible field (mirrors the PR-B1 ollama tab test).
func TestSetupModel_RAGCreds_Tab_FieldIdxNeverExceedsOne(t *testing.T) {
	m := newTestSetupModel("")
	m = advanceToRAGCreds(t, m)

	for i := 0; i < 10; i++ {
		next, _ := m.Update(tea.KeyMsg{Type: tea.KeyTab})
		m = next.(setupModel)
		if m.fieldIdx > 1 {
			t.Errorf("Tab press %d: fieldIdx=%d exceeds max valid index 1", i+1, m.fieldIdx)
		}
		switch m.fieldIdx {
		case 0:
			if !m.embModelInput.Focused() {
				t.Errorf("Tab press %d: fieldIdx=0 but embModelInput not focused", i+1)
			}
		case 1:
			if !m.embKeyInput.Focused() {
				t.Errorf("Tab press %d: fieldIdx=1 but embKeyInput not focused", i+1)
			}
		}
	}
}

// ─── PR-B2: helpers ────────────────────────────────────────────────────────

// advanceToRAGEnable drives the model from stepProvider through valid
// stepCredentials to reach stepRAGEnable.
func advanceToRAGEnable(t *testing.T, m setupModel) setupModel {
	t.Helper()
	// Select provider (anthropic, idx 0).
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(setupModel)
	if m.step != stepCredentials {
		t.Fatalf("expected stepCredentials, got %v", m.step)
	}
	m.modelInput.SetValue("claude-sonnet-4-6")
	m.keyInput.SetValue("sk-ant-key")
	next2, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next2.(setupModel)
	if m.step != stepRAGEnable {
		t.Fatalf("expected stepRAGEnable, got %v (advanceToRAGEnable helper)", m.step)
	}
	return m
}

// advanceToRAGProvider drives the model to stepRAGProvider.
func advanceToRAGProvider(t *testing.T, m setupModel) setupModel {
	t.Helper()
	m = advanceToRAGEnable(t, m)
	// Select Yes (Down then Enter, or just 'y').
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("y")})
	m = next.(setupModel)
	if m.step != stepRAGProvider {
		t.Fatalf("expected stepRAGProvider, got %v (advanceToRAGProvider helper)", m.step)
	}
	return m
}

// advanceToRAGCreds drives the model to stepRAGCreds (openai embedding provider).
func advanceToRAGCreds(t *testing.T, m setupModel) setupModel {
	t.Helper()
	m = advanceToRAGProvider(t, m)
	// Press Enter to select first provider (openai).
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(setupModel)
	if m.step != stepRAGCreds {
		t.Fatalf("expected stepRAGCreds, got %v (advanceToRAGCreds helper)", m.step)
	}
	return m
}
