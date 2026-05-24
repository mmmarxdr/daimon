package agent

// model_command_test.go — TDD RED tests for the /model slash command (PR5).
//
// Covers:
//   S2-1: /model registered as builtin at agent init
//   S2-2: cross-provider syntax (colon) → rejected with normative reply
//   S6-1: no-arg /model lists models, current marked with *
//   S6-2: no-arg /model fallback when provider has no ModelLister
//   S6-3: no-arg /model error from ListModels
//   S7-1: invalid model name → unknown model reply with available list
//   S7-2: valid model name proceeds to SetProvider
//   S7-3: optimistic path when no ModelLister
//   S9-1: IsDestructiveCommand("model") returns true
//   S11-2: /model in built-in section of grouped /help output

import (
	"context"
	"errors"
	"strings"
	"testing"

	"daimon/internal/audit"
	"daimon/internal/config"
	"daimon/internal/provider"
	"daimon/internal/skill"
)

// ---------------------------------------------------------------------------
// Test helpers
// ---------------------------------------------------------------------------

// buildAgentForModelCmd builds a minimal *Agent for /model command tests.
// prov is the initial provider. The agent's newProviderFn is overridden so
// SetProvider never makes real API calls.
func buildAgentForModelCmd(t *testing.T, prov provider.Provider) *Agent {
	t.Helper()
	ch := &mockChannel{}
	st := &mockStore{}
	a := New(
		config.AgentConfig{},
		defaultLimits(),
		config.FilterConfig{},
		ch,
		prov,
		st,
		audit.NoopAuditor{},
		nil,
		nil,
		skill.SkillIndex{},
		4,
		false,
	)
	// Override newProviderFn so SetProvider calls in cmdModel don't hit real APIs.
	a.newProviderFn = mockProviderFactory(prov)
	return a
}

// makeModelCC builds a CommandContext for /model command tests. args is the
// raw argument string after the command name (e.g. "" for no-arg, "m1" for swap).
func makeModelCC(a *Agent, cr *capturedReply, args string) CommandContext {
	return CommandContext{
		Ctx:       context.Background(),
		ChannelID: "chan:test",
		SenderID:  "user:test",
		Args:      args,
		Store:     &mockStore{},
		Config:    &config.AgentConfig{},
		Reply:     cr.reply,
		Registry:  a.commands,
	}
}

// mockSetProviderAgent wraps an *Agent and records SetProvider calls for
// scenario S7-2 (verify SetProvider is actually called on valid model).
// We accomplish this by checking the provider state after the call, using
// the mockProviderFactory which returns a provider with the requested model.

// ---------------------------------------------------------------------------
// S9-1: IsDestructiveCommand("model") returns true
// ---------------------------------------------------------------------------

func TestIsDestructiveCommand_Model_ReturnsTrue(t *testing.T) {
	if !IsDestructiveCommand("model") {
		t.Error("IsDestructiveCommand(\"model\") = false, want true")
	}
}

// ---------------------------------------------------------------------------
// S2-1: /model is registered as builtin at agent init
// ---------------------------------------------------------------------------

func TestCmdModel_S2_1_RegisteredAsBuiltin(t *testing.T) {
	prov := newConfigurableProviderWithModels("m1", []string{"m1", "m2"})
	a := buildAgentForModelCmd(t, prov)

	entry, ok := a.commands.commands["model"]
	if !ok {
		t.Fatal("command 'model' not found in registry")
	}
	if entry.source != SourceBuiltin {
		t.Errorf("command 'model' source = %q, want %q", entry.source, SourceBuiltin)
	}
}

// ---------------------------------------------------------------------------
// S2-2: /model rejects cross-provider syntax (contains ":")
// ---------------------------------------------------------------------------

func TestCmdModel_S2_2_CrossProviderRejected(t *testing.T) {
	prov := newConfigurableProviderWithModels("m1", []string{"m1", "m2"})
	a := buildAgentForModelCmd(t, prov)
	cr := &capturedReply{}
	cc := makeModelCC(a, cr, "anthropic:claude-haiku-4-5")

	if err := a.cmdModel(cc); err != nil {
		t.Fatalf("cmdModel returned error: %v", err)
	}
	if len(cr.messages) != 1 {
		t.Fatalf("expected 1 reply, got %d", len(cr.messages))
	}
	reply := cr.messages[0]
	if !strings.Contains(reply, "cross-provider swap is not supported") {
		t.Errorf("reply missing cross-provider rejection: %q", reply)
	}
	if !strings.Contains(reply, prov.Name()) {
		t.Errorf("reply missing current provider name %q: %q", prov.Name(), reply)
	}
}

// ---------------------------------------------------------------------------
// S6-1: No-arg /model lists models with current marked by *
// ---------------------------------------------------------------------------

func TestCmdModel_S6_1_NoArgListsModels_CurrentMarked(t *testing.T) {
	prov := newConfigurableProviderWithModels("m2", []string{"m1", "m2", "m3"})
	a := buildAgentForModelCmd(t, prov)
	cr := &capturedReply{}
	cc := makeModelCC(a, cr, "")

	if err := a.cmdModel(cc); err != nil {
		t.Fatalf("cmdModel returned error: %v", err)
	}
	if len(cr.messages) != 1 {
		t.Fatalf("expected 1 reply, got %d", len(cr.messages))
	}
	reply := cr.messages[0]

	// Current model must be marked with *
	if !strings.Contains(reply, "* m2") {
		t.Errorf("reply missing '* m2' (current model marker): %q", reply)
	}
	// Other models prefixed with spaces (not *)
	if !strings.Contains(reply, "  m1") {
		t.Errorf("reply missing '  m1' (non-current model): %q", reply)
	}
	if !strings.Contains(reply, "  m3") {
		t.Errorf("reply missing '  m3' (non-current model): %q", reply)
	}
	// Header must contain provider name
	if !strings.Contains(reply, prov.Name()) {
		t.Errorf("reply missing provider name %q: %q", prov.Name(), reply)
	}
}

// ---------------------------------------------------------------------------
// S6-2: No-arg /model fallback when provider has no ModelLister
// ---------------------------------------------------------------------------

func TestCmdModel_S6_2_NoArgFallback_NoModelLister(t *testing.T) {
	prov := &mockNonConfigurableProvider{}
	a := buildAgentForModelCmd(t, prov)
	cr := &capturedReply{}
	cc := makeModelCC(a, cr, "")

	if err := a.cmdModel(cc); err != nil {
		t.Fatalf("cmdModel returned error: %v", err)
	}
	if len(cr.messages) != 1 {
		t.Fatalf("expected 1 reply, got %d", len(cr.messages))
	}
	reply := cr.messages[0]
	if !strings.Contains(reply, "listing not supported by provider") {
		t.Errorf("reply missing fallback message: %q", reply)
	}
}

// ---------------------------------------------------------------------------
// S6-3: No-arg /model — ListModels returns error
// ---------------------------------------------------------------------------

func TestCmdModel_S6_3_NoArgListModelsError(t *testing.T) {
	prov := &mockConfigurableProvider{
		nameStr:  "mock-provider",
		modelStr: "m1",
		cfg:      config.ProviderConfig{Type: "mock-provider", Model: "m1"},
		models:   nil,
		listErr:  errors.New("network failure"),
	}
	a := buildAgentForModelCmd(t, prov)
	cr := &capturedReply{}
	cc := makeModelCC(a, cr, "")

	if err := a.cmdModel(cc); err != nil {
		t.Fatalf("cmdModel returned error: %v", err)
	}
	if len(cr.messages) != 1 {
		t.Fatalf("expected 1 reply, got %d", len(cr.messages))
	}
	reply := cr.messages[0]
	if !strings.Contains(reply, "could not list models for provider") {
		t.Errorf("reply missing list error message: %q", reply)
	}
}

// ---------------------------------------------------------------------------
// S7-1: Invalid model name → rejected with available list
// ---------------------------------------------------------------------------

func TestCmdModel_S7_1_InvalidModel_RejectedWithAvailableList(t *testing.T) {
	prov := newConfigurableProviderWithModels("m1", []string{"m1", "m2"})
	a := buildAgentForModelCmd(t, prov)
	cr := &capturedReply{}
	cc := makeModelCC(a, cr, "bad-model")

	if err := a.cmdModel(cc); err != nil {
		t.Fatalf("cmdModel returned error: %v", err)
	}
	if len(cr.messages) != 1 {
		t.Fatalf("expected 1 reply, got %d", len(cr.messages))
	}
	reply := cr.messages[0]
	if !strings.Contains(reply, "unknown model") {
		t.Errorf("reply missing 'unknown model': %q", reply)
	}
	if !strings.Contains(reply, "bad-model") {
		t.Errorf("reply missing the requested bad model name: %q", reply)
	}
	if !strings.Contains(reply, "m1") {
		t.Errorf("reply missing available model 'm1': %q", reply)
	}
	if !strings.Contains(reply, "m2") {
		t.Errorf("reply missing available model 'm2': %q", reply)
	}
	// Verify provider was NOT swapped.
	if a.providerSnapshot().Model() != "m1" {
		t.Errorf("provider was swapped on invalid model; got %q, want %q", a.providerSnapshot().Model(), "m1")
	}
}

// ---------------------------------------------------------------------------
// S7-2: Valid model name proceeds to SetProvider
// ---------------------------------------------------------------------------

func TestCmdModel_S7_2_ValidModel_ProceedsToSetProvider(t *testing.T) {
	prov := newConfigurableProviderWithModels("m1", []string{"m1", "m2"})
	a := buildAgentForModelCmd(t, prov)
	cr := &capturedReply{}
	cc := makeModelCC(a, cr, "m2")

	if err := a.cmdModel(cc); err != nil {
		t.Fatalf("cmdModel returned error: %v", err)
	}
	// Verify provider was swapped to m2.
	if a.providerSnapshot().Model() != "m2" {
		t.Errorf("after valid swap: provider.Model() = %q, want %q", a.providerSnapshot().Model(), "m2")
	}
}

// ---------------------------------------------------------------------------
// S7-3: Optimistic path when no ModelLister — SetProvider is called
// ---------------------------------------------------------------------------

func TestCmdModel_S7_3_OptimisticPath_NoModelLister(t *testing.T) {
	// Use a non-configurable provider so the cross-provider check passes but
	// no ModelLister is available. SetProvider will return ErrProviderNotConfigurable
	// (since the provider is not configurable). The command should surface that error.
	prov := &mockNonConfigurableProvider{}
	a := buildAgentForModelCmd(t, prov)
	cr := &capturedReply{}
	cc := makeModelCC(a, cr, "any-model")

	if err := a.cmdModel(cc); err != nil {
		t.Fatalf("cmdModel returned error: %v", err)
	}
	// Since the provider doesn't support hot-swap, SetProvider returns an error.
	// The command should reply with "failed to set model: ..."
	if len(cr.messages) != 1 {
		t.Fatalf("expected 1 reply, got %d", len(cr.messages))
	}
	reply := cr.messages[0]
	if !strings.Contains(reply, "failed to set model") {
		t.Errorf("reply missing SetProvider error message: %q", reply)
	}
}

// ---------------------------------------------------------------------------
// ErrTurnInProgress — /model reply maps sentinel correctly
// ---------------------------------------------------------------------------

func TestCmdModel_ErrTurnInProgress_MappedToUserMessage(t *testing.T) {
	prov := newConfigurableProviderWithModels("m1", []string{"m1", "m2"})
	a := buildAgentForModelCmd(t, prov)

	// Simulate a turn in progress by registering a cancel entry.
	key := cancelKey{ChannelID: "chan:test", SenderID: "user:test"}
	if err := a.cancels.Register(key, func() {}); err != nil {
		t.Fatalf("failed to register cancel entry: %v", err)
	}
	t.Cleanup(func() { a.cancels.Cancel(key) })

	cr := &capturedReply{}
	cc := makeModelCC(a, cr, "m2")

	if err := a.cmdModel(cc); err != nil {
		t.Fatalf("cmdModel returned error: %v", err)
	}
	if len(cr.messages) != 1 {
		t.Fatalf("expected 1 reply, got %d", len(cr.messages))
	}
	reply := cr.messages[0]
	if !strings.Contains(reply, "turn is currently in progress") {
		t.Errorf("reply missing turn-in-progress message: %q", reply)
	}
}

// ---------------------------------------------------------------------------
// S11-2: /model appears in Built-in section of grouped /help output
// ---------------------------------------------------------------------------

func TestCmdModel_AppearsInHelpBuiltinSection(t *testing.T) {
	prov := newConfigurableProviderWithModels("m1", []string{"m1"})
	a := buildAgentForModelCmd(t, prov)
	cr := &capturedReply{}
	cc := CommandContext{
		Ctx:      context.Background(),
		Reply:    cr.reply,
		Registry: a.commands,
	}

	if err := cmdHelp(cc); err != nil {
		t.Fatalf("cmdHelp returned error: %v", err)
	}
	if len(cr.messages) != 1 {
		t.Fatalf("expected 1 reply, got %d", len(cr.messages))
	}
	reply := cr.messages[0]

	if !strings.Contains(reply, "Built-in commands:") {
		t.Fatalf("reply missing 'Built-in commands:' section: %q", reply)
	}
	if !strings.Contains(reply, "/model") {
		t.Fatalf("reply missing '/model' in output: %q", reply)
	}

	// Verify /model is in the Built-in section (appears BEFORE "Cron commands:" if present,
	// or just exists in the Built-in section if cron section is absent).
	builtinIdx := strings.Index(reply, "Built-in commands:")
	modelIdx := strings.Index(reply, "/model")
	if modelIdx < builtinIdx {
		t.Errorf("/model (pos %d) appears before 'Built-in commands:' section (pos %d)", modelIdx, builtinIdx)
	}

	// If there's a Cron section, /model must appear before it.
	if cronIdx := strings.Index(reply, "Cron commands:"); cronIdx >= 0 {
		if modelIdx > cronIdx {
			t.Errorf("/model (pos %d) appears in or after Cron section (pos %d)", modelIdx, cronIdx)
		}
	}
}
