package agent

// memory_changed_wiring_test.go — integration test for the production bus-wiring
// order. Reproduces the confirmed critical bug where EventMemoryChanged is dead
// code for the smart-curation path because WithBus runs BEFORE WithCurator /
// WithConsolidator in cmd/daimon/main.go.
//
// Production ordering (main.go):
//
//	ag := agent.New(...).WithBus(notifyBus)   // line ~550 — bus wired FIRST
//	...
//	wireSmartMemory(ag, ...)                   // line ~586 — WithCurator/WithConsolidator LATER
//
// RED: tests MUST FAIL before the fix (bus never reaches curator/consolidator).
// GREEN: tests MUST PASS after WithCurator/WithConsolidator call SetBus(a.bus).

import (
	"context"
	"testing"

	"daimon/internal/audit"
	"daimon/internal/config"
	"daimon/internal/notify"
	"daimon/internal/provider"
	"daimon/internal/skill"
)

// TestMemoryChangedWiring_ProductionOrder_CuratorEmitsEvent mirrors the
// production call ordering: WithBus first, WithCurator later.
// Before the fix: curator.bus is nil → no event emitted → FAIL.
// After the fix: WithCurator propagates a.bus → event emitted → PASS.
func TestMemoryChangedWiring_ProductionOrder_CuratorEmitsEvent(t *testing.T) {
	rb := &captureBus{}

	// Build a minimal agent using the same helpers used by other agent tests.
	prov := &curatorMockProvider{
		response: classifyJSON(8, "fact", "topic", "title"),
	}
	ch := &mockChannel{}
	st := &curatorMockStore{}

	ag := New(
		config.AgentConfig{MaxIterations: 1},
		defaultLimits(),
		config.FilterConfig{},
		ch, prov, st,
		audit.NoopAuditor{},
		nil, nil, skill.SkillIndex{}, 4, false,
	)

	// PRODUCTION ORDER STEP 1: wire bus before curator exists (mirrors main.go:550).
	ag.WithBus(rb)

	// PRODUCTION ORDER STEP 2: build and attach curator after bus is set
	// (mirrors wireSmartMemory in cmd/daimon/memory_wiring.go:34).
	curator := NewCurator(
		curatorStaticProvFn(prov),
		st,
		nil, nil,
		enabledCurationCfg(),
		disabledDedupCfg(),
	)
	if curator == nil {
		t.Fatal("NewCurator returned nil — curation config must be enabled")
	}
	// BUG (pre-fix): WithCurator is a bare setter; it does NOT call
	// curator.SetBus(a.bus), so curator.bus stays nil forever.
	ag.WithCurator(curator)

	// Trigger a successful Curate call.
	userMsg := "Which language do you use for backend services?"
	response := "I always use Go for backend services because of its performance and simplicity. Python is reserved for data science tasks."
	if err := curator.Curate(context.Background(), "scope-prod", userMsg, response, "conv-prod"); err != nil {
		t.Fatalf("Curate: %v", err)
	}

	// Falsifiable assertion: EventMemoryChanged must have been emitted.
	// FAILS before fix (curator.bus is nil → emit skipped).
	// PASSES after fix (WithCurator propagates a.bus).
	memEvs := filterEvents(rb.events, notify.EventMemoryChanged)
	if len(memEvs) != 1 {
		t.Fatalf("production-order wiring: expected 1 EventMemoryChanged, got %d — "+
			"WithCurator must call curator.SetBus(a.bus) when a.bus != nil", len(memEvs))
	}
	ev := memEvs[0]
	if ev.Meta["scope_id"] == "" {
		t.Error("EventMemoryChanged.Meta[scope_id] must not be empty")
	}
	if ev.Meta["entry_id"] == "" {
		t.Error("EventMemoryChanged.Meta[entry_id] must not be empty")
	}
}

// TestMemoryChangedWiring_ProductionOrder_ConsolidatorEmitsEvent mirrors the
// same production ordering for the Consolidator path.
// Before the fix: consolidator.bus is nil → no event emitted → FAIL.
// After the fix: WithConsolidator propagates a.bus → event emitted → PASS.
func TestMemoryChangedWiring_ProductionOrder_ConsolidatorEmitsEvent(t *testing.T) {
	rb := &captureBus{}

	consolidatorProv := &callCountProvider{
		onCall: func(_ int) (*provider.ChatResponse, error) {
			return &provider.ChatResponse{Content: "Merged memory about the user."}, nil
		},
	}
	ch := &mockChannel{}
	st := makeConsolidatableStore("golang", 4) // >=3 entries triggers consolidation

	ag := New(
		config.AgentConfig{MaxIterations: 1},
		defaultLimits(),
		config.FilterConfig{},
		ch, consolidatorProv, st,
		audit.NoopAuditor{},
		nil, nil, skill.SkillIndex{}, 4, false,
	)

	// PRODUCTION ORDER STEP 1: bus first.
	ag.WithBus(rb)

	// PRODUCTION ORDER STEP 2: consolidator after bus.
	consolidator := makeConsolidator(consolidatorProv, st)
	if consolidator == nil {
		t.Fatal("makeConsolidator returned nil")
	}
	// BUG (pre-fix): bare setter — bus never reaches consolidator.
	ag.WithConsolidator(consolidator)

	// Trigger consolidation through the consolidator that was attached after WithBus.
	if _, _, err := consolidator.consolidateScope(context.Background(), "scope-prod"); err != nil {
		t.Fatalf("consolidateScope: %v", err)
	}

	// Falsifiable assertion: at least one EventMemoryChanged must have been emitted.
	memEvs := filterEvents(rb.events, notify.EventMemoryChanged)
	if len(memEvs) == 0 {
		t.Fatal("production-order wiring: expected at least 1 EventMemoryChanged from consolidator, got 0 — " +
			"WithConsolidator must call consolidator.SetBus(a.bus) when a.bus != nil")
	}
	ev := memEvs[0]
	if ev.Meta["scope_id"] == "" {
		t.Error("EventMemoryChanged.Meta[scope_id] must not be empty")
	}
	if ev.Meta["entry_id"] == "" {
		t.Error("EventMemoryChanged.Meta[entry_id] must not be empty")
	}
}
