package agent

import (
	"testing"

	"daimon/internal/provider"
)

// TestCurator_NoBareProvField asserts REQ-D2/S7-2:
// The Curator struct MUST hold providerFn func() provider.Provider, NOT a bare prov provider.Provider.
// This test accesses c.providerFn to confirm the field exists and is callable.
func TestCurator_NoBareProvField(t *testing.T) {
	prov := &curatorMockProvider{response: `{"memorable_fact":"","importance":0,"type":"skip","cluster":"general","topic":"","title":""}`}
	st := &curatorMockStore{}
	cfg := enabledCurationCfg()

	var captured provider.Provider = prov
	fn := func() provider.Provider { return captured }

	c := NewCurator(fn, st, nil, nil, cfg, disabledDedupCfg())
	if c == nil {
		t.Fatal("NewCurator returned nil with Enabled=true")
	}

	// c.providerFn must exist and return the correct provider.
	got := c.providerFn()
	if got == nil {
		t.Fatal("c.providerFn() returned nil")
	}
	if got.Model() != prov.Model() {
		t.Errorf("providerFn() returned wrong provider: Model()=%q, want %q", got.Model(), prov.Model())
	}
}

// TestCurator_SeesSwappedProvider asserts REQ-D2/S7-1:
// After the closure variable is updated, the next call to c.providerFn() returns the new provider.
// This simulates what happens after SetProvider updates a.provider under providerMu.Lock.
func TestCurator_SeesSwappedProvider(t *testing.T) {
	prov1 := &curatorMockProvider{response: `{"memorable_fact":"fact1","importance":8,"type":"fact","cluster":"general","topic":"t","title":"title1"}`}
	prov2 := &curatorMockProvider{response: `{"memorable_fact":"fact2","importance":8,"type":"fact","cluster":"general","topic":"t","title":"title2"}`}

	// Use a swappable pointer; the closure captures the pointer.
	var current provider.Provider = prov1
	fn := func() provider.Provider { return current }

	st := &curatorMockStore{}
	cfg := enabledCurationCfg()

	c := NewCurator(fn, st, nil, nil, cfg, disabledDedupCfg())
	if c == nil {
		t.Fatal("NewCurator returned nil")
	}

	// Before swap: closure returns prov1.
	if got := c.providerFn(); got != prov1 {
		t.Errorf("before swap: providerFn() = %v, want prov1", got)
	}

	// Simulate SetProvider updating the live provider (direct field update under Lock).
	current = prov2

	// After swap: closure returns prov2 — post-swap visibility confirmed.
	if got := c.providerFn(); got != prov2 {
		t.Errorf("after swap: providerFn() = %v, want prov2", got)
	}
}
