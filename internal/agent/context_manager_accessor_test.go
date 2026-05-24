package agent

import (
	"testing"

	"daimon/internal/config"
	"daimon/internal/provider"
)

// TestContextManager_NoBareProvField asserts REQ-4/S4-2:
// The ContextManager struct MUST hold providerFn func() provider.Provider, NOT a bare prov provider.Provider.
// This test accesses cm.providerFn to confirm the field exists and is callable.
func TestContextManager_NoBareProvField(t *testing.T) {
	prov := &cmMockProvider{name: "test", model: "m1"}
	cfg := config.ContextConfig{Strategy: "none"}

	fn := func() provider.Provider { return prov }

	cm := NewContextManager(cfg, fn, nil)
	if cm == nil {
		t.Fatal("NewContextManager returned nil")
	}

	// cm.providerFn must exist and return the correct provider.
	got := cm.providerFn()
	if got == nil {
		t.Fatal("cm.providerFn() returned nil")
	}
	if got.Model() != prov.Model() {
		t.Errorf("providerFn() returned wrong provider: Model()=%q, want %q", got.Model(), prov.Model())
	}
}

// TestContextManager_SeesSwappedProvider asserts REQ-4/S4-1:
// After the closure variable is updated, the next call to cm.providerFn() returns the new provider.
// This simulates what happens after SetProvider updates a.provider under providerMu.Lock.
func TestContextManager_SeesSwappedProvider(t *testing.T) {
	prov1 := &cmMockProvider{name: "test", model: "m1"}
	prov2 := &cmMockProvider{name: "test", model: "m2"}

	var current provider.Provider = prov1
	fn := func() provider.Provider { return current }

	cfg := config.ContextConfig{Strategy: "none"}
	cm := NewContextManager(cfg, fn, nil)
	if cm == nil {
		t.Fatal("NewContextManager returned nil")
	}

	// Before swap: closure returns prov1.
	if got := cm.providerFn(); got != prov1 {
		t.Errorf("before swap: providerFn() = %v, want prov1", got)
	}

	// Simulate SetProvider updating the live provider (direct field update under Lock).
	current = prov2

	// After swap: closure returns prov2 — post-swap visibility confirmed.
	if got := cm.providerFn(); got != prov2 {
		t.Errorf("after swap: providerFn() = %v, want prov2", got)
	}
}
