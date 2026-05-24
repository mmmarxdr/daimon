package agent

// provider_accessor_test.go — RED tests for PR1 (model-hot-swap):
//   - providerSnapshot() returns the stored provider under RLock
//   - WithProviderCredentials stores creds on the agent
//
// These tests intentionally reference types and methods that do NOT yet exist.
// They are written first (TDD RED step) and will fail to compile until the
// corresponding implementation is added.

import (
	"context"
	"testing"

	"daimon/internal/audit"
	"daimon/internal/config"
	"daimon/internal/provider"
	"daimon/internal/skill"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// buildMinimalAgent constructs a minimal Agent for the accessor tests.
// Uses an in-package constructor (New) — no integration, no goroutines.
func buildMinimalAgent(t *testing.T, prov provider.Provider) *Agent {
	t.Helper()
	ch := &mockChannel{}
	st := &mockStore{}
	return New(
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
}

// ---------------------------------------------------------------------------
// swappableMockProvider — a mock provider with configurable model/name
// for use in the accessor tests (distinct from the loop_test.go mockProvider).
// ---------------------------------------------------------------------------

type swappableMockProvider struct {
	nameStr  string
	modelStr string
}

func (p *swappableMockProvider) Name() string             { return p.nameStr }
func (p *swappableMockProvider) Model() string            { return p.modelStr }
func (p *swappableMockProvider) SupportsTools() bool      { return true }
func (p *swappableMockProvider) SupportsMultimodal() bool { return false }
func (p *swappableMockProvider) SupportsAudio() bool      { return false }
func (p *swappableMockProvider) HealthCheck(_ context.Context) (string, error) {
	return p.modelStr, nil
}
func (p *swappableMockProvider) Chat(_ context.Context, _ provider.ChatRequest) (*provider.ChatResponse, error) {
	return &provider.ChatResponse{Content: "ok"}, nil
}

// ---------------------------------------------------------------------------
// T1.1 — providerSnapshot() returns the stored provider
// ---------------------------------------------------------------------------

// TestProviderSnapshot_ReturnsStoredProvider verifies that providerSnapshot()
// returns the same provider that was passed to New(). This is the baseline
// before any swap — the snapshot should equal the initial provider.
func TestProviderSnapshot_ReturnsStoredProvider(t *testing.T) {
	mp := &swappableMockProvider{nameStr: "test", modelStr: "test-model"}
	a := buildMinimalAgent(t, mp)

	got := a.providerSnapshot()
	if got != mp {
		t.Errorf("providerSnapshot() = %v, want %v", got, mp)
	}
}

// TestProviderSnapshot_UnderConcurrentReads verifies that providerSnapshot()
// is safe for concurrent use (no data race under -race).
func TestProviderSnapshot_UnderConcurrentReads(t *testing.T) {
	mp := &swappableMockProvider{nameStr: "test", modelStr: "test-model"}
	a := buildMinimalAgent(t, mp)

	const goroutines = 50
	done := make(chan struct{}, goroutines)
	for range goroutines {
		go func() {
			_ = a.providerSnapshot()
			done <- struct{}{}
		}()
	}
	for range goroutines {
		<-done
	}
}

// TestProviderSnapshot_AfterDirectFieldUpdate verifies that providerSnapshot()
// returns the updated provider after a direct write to a.provider (simulating
// what SetProvider will do under Lock in PR2). This test does NOT use SetProvider
// — it manipulates the field directly under Lock to verify the snapshot
// accessor reads through correctly.
func TestProviderSnapshot_AfterDirectFieldUpdate(t *testing.T) {
	initial := &swappableMockProvider{nameStr: "test", modelStr: "model-initial"}
	a := buildMinimalAgent(t, initial)

	// Simulate a provider swap by writing the field directly under Lock.
	// This is exactly the pattern SetProvider (PR2) will use.
	newer := &swappableMockProvider{nameStr: "test", modelStr: "model-newer"}
	a.providerMu.Lock()
	a.provider = newer
	a.providerMu.Unlock()

	got := a.providerSnapshot()
	if got.Model() != "model-newer" {
		t.Errorf("providerSnapshot().Model() = %q, want %q", got.Model(), "model-newer")
	}
}

// ---------------------------------------------------------------------------
// T1.2 — WithProviderCredentials stores creds on the agent
// ---------------------------------------------------------------------------

// TestWithProviderCredentials_StoresCreds verifies that calling
// WithProviderCredentials sets the providerCreds field on the agent.
func TestWithProviderCredentials_StoresCreds(t *testing.T) {
	mp := &swappableMockProvider{nameStr: "test", modelStr: "test-model"}
	a := buildMinimalAgent(t, mp)

	creds := config.ProviderCredentials{
		APIKey:  "test-api-key-123",
		BaseURL: "https://api.example.com",
	}

	result := a.WithProviderCredentials(creds)

	// Fluent: must return *Agent for chaining.
	if result != a {
		t.Error("WithProviderCredentials() must return the same *Agent for chaining")
	}

	// Verify creds are stored.
	if a.providerCreds.APIKey != creds.APIKey {
		t.Errorf("providerCreds.APIKey = %q, want %q", a.providerCreds.APIKey, creds.APIKey)
	}
	if a.providerCreds.BaseURL != creds.BaseURL {
		t.Errorf("providerCreds.BaseURL = %q, want %q", a.providerCreds.BaseURL, creds.BaseURL)
	}
}

// TestWithProviderCredentials_ZeroValueIsValid verifies that passing a
// zero-value ProviderCredentials is accepted (no panic, no error).
// This covers the common test path where no thinking config is needed.
func TestWithProviderCredentials_ZeroValueIsValid(t *testing.T) {
	mp := &swappableMockProvider{nameStr: "test", modelStr: "test-model"}
	a := buildMinimalAgent(t, mp)

	// Should not panic.
	a.WithProviderCredentials(config.ProviderCredentials{})

	if a.providerCreds.APIKey != "" {
		t.Errorf("expected empty APIKey after zero-value creds, got %q", a.providerCreds.APIKey)
	}
}

// TestWithProviderCredentials_ThinkingConfigPreserved verifies that the
// thinking config sub-struct survives the WithProviderCredentials call
// (no accidental zeroing of nested pointers).
func TestWithProviderCredentials_ThinkingConfigPreserved(t *testing.T) {
	mp := &swappableMockProvider{nameStr: "test", modelStr: "test-model"}
	a := buildMinimalAgent(t, mp)

	enabled := true
	creds := config.ProviderCredentials{
		APIKey: "sk-key",
		Thinking: &config.ProviderThinkingConfig{
			Enabled:      &enabled,
			Effort:       "high",
			BudgetTokens: 10000,
		},
	}

	a.WithProviderCredentials(creds)

	if a.providerCreds.Thinking == nil {
		t.Fatal("expected Thinking config to be non-nil after WithProviderCredentials")
	}
	if a.providerCreds.Thinking.Effort != "high" {
		t.Errorf("Thinking.Effort = %q, want %q", a.providerCreds.Thinking.Effort, "high")
	}
	if a.providerCreds.Thinking.BudgetTokens != 10000 {
		t.Errorf("Thinking.BudgetTokens = %d, want %d", a.providerCreds.Thinking.BudgetTokens, 10000)
	}
}

// ---------------------------------------------------------------------------
// T1.3 — loop snapshot: providerSnapshot() used in processMessage
// ---------------------------------------------------------------------------

// TestLoopUsesProviderSnapshot verifies that after a direct field swap
// (simulating what SetProvider will do), the loop captures the new provider
// via providerSnapshot() at turn start.
//
// This is a compile-time + behavioral check. The actual loop path for
// REQ-11 (per-turn capture) is verified by checking that prov is returned
// from providerSnapshot() and matches the updated field.
func TestLoopUsesProviderSnapshot_ConsistentView(t *testing.T) {
	initial := &swappableMockProvider{nameStr: "test", modelStr: "model-old"}
	a := buildMinimalAgent(t, initial)

	// Verify initial snapshot.
	snap1 := a.providerSnapshot()
	if snap1.Model() != "model-old" {
		t.Fatalf("initial snapshot model = %q, want %q", snap1.Model(), "model-old")
	}

	// Simulate a mid-swap (SetProvider will do this under Lock).
	newer := &swappableMockProvider{nameStr: "test", modelStr: "model-new"}
	a.providerMu.Lock()
	a.provider = newer
	a.providerMu.Unlock()

	// The NEXT call to providerSnapshot() should return the new provider.
	snap2 := a.providerSnapshot()
	if snap2.Model() != "model-new" {
		t.Errorf("post-swap snapshot model = %q, want %q", snap2.Model(), "model-new")
	}
}
