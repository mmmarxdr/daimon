package agent

import (
	"testing"

	"daimon/internal/provider"
)

// TestEnricher_NoBareProviderField asserts REQ-3/S3-2:
// The Enricher struct MUST hold providerFn func() provider.Provider, NOT a bare provider.Provider.
// This test accesses e.providerFn to confirm the field exists and is callable.
func TestEnricher_NoBareProviderField(t *testing.T) {
	prov := &enrichMockProvider{response: "tag1,tag2"}
	st := &enrichMockStore{}
	cfg := enrichTestCfg()

	var captured provider.Provider = prov
	fn := func() provider.Provider { return captured }

	e := NewEnricher(fn, st, cfg)
	if e == nil {
		t.Fatal("NewEnricher returned nil with EnrichMemory=true")
	}

	// e.providerFn must exist and return the correct provider.
	got := e.providerFn()
	if got == nil {
		t.Fatal("e.providerFn() returned nil")
	}
	if got.Model() != prov.Model() {
		t.Errorf("providerFn() returned wrong provider: Model()=%q, want %q", got.Model(), prov.Model())
	}
}

// TestEnricher_SeesSwappedProvider asserts REQ-3/S3-1:
// After the closure variable is updated, the next call to e.providerFn() returns the new provider.
// This simulates what happens after SetProvider updates a.provider under providerMu.Lock.
func TestEnricher_SeesSwappedProvider(t *testing.T) {
	prov1 := &enrichMockProvider{response: "tag1"}
	prov2 := &enrichMockProvider{response: "tag2"}

	// Use a swappable pointer; the closure captures the pointer.
	var current provider.Provider = prov1
	fn := func() provider.Provider { return current }

	st := &enrichMockStore{}
	cfg := enrichTestCfg()

	e := NewEnricher(fn, st, cfg)
	if e == nil {
		t.Fatal("NewEnricher returned nil")
	}

	// Before swap: closure returns prov1.
	if got := e.providerFn(); got != prov1 {
		t.Errorf("before swap: providerFn() = %v, want prov1", got)
	}

	// Simulate SetProvider updating the live provider (direct field update under Lock).
	current = prov2

	// After swap: closure returns prov2 — post-swap visibility confirmed.
	if got := e.providerFn(); got != prov2 {
		t.Errorf("after swap: providerFn() = %v, want prov2", got)
	}
}
