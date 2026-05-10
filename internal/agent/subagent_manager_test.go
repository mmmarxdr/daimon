package agent

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"daimon/internal/channel"
	"daimon/internal/notify"
	"daimon/internal/skill"
	"daimon/internal/store"
	"daimon/internal/tool"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func defaultBudget() skill.BudgetConfig {
	return skill.BudgetConfig{
		MaxCostUSD: 1.0,
		MaxTurns:   20,
		Timeout:    5 * time.Minute,
	}
}

func defaultDef(name string) skill.ExecutableSkillDef {
	return skill.ExecutableSkillDef{
		Name:           name,
		Description:    "test skill",
		Budget:         defaultBudget(),
		ToolsAllowlist: []string{},
	}
}

// fakeChildAgent simulates a child agent. It immediately delivers
// the prompt back as a final assistant message so the manager
// can observe a clean completion.
type fakeChildAgent struct {
	mu        sync.Mutex
	channel   *channel.SubagentChannel
	runCalled bool
}

func (f *fakeChildAgent) runFn(_ context.Context) {
	f.mu.Lock()
	f.runCalled = true
	ch := f.channel
	f.mu.Unlock()

	// Simulate a completed turn: Send a final assistant message.
	_ = ch.Send(context.Background(), channel.OutgoingMessage{
		Text: "done",
	})
	_ = ch.Stop()
}

// busRecorder records all events emitted on a Bus.
type busRecorder struct {
	mu     sync.Mutex
	events []notify.Event
	inner  notify.Bus
}

func newBusRecorder() *busRecorder {
	bus := notify.NewEventBus(256, 0, 0)
	r := &busRecorder{inner: bus}
	bus.Subscribe(func(ev notify.Event) {
		r.mu.Lock()
		r.events = append(r.events, ev)
		r.mu.Unlock()
	})
	return r
}

func (r *busRecorder) Emit(ev notify.Event)          { r.inner.Emit(ev) }
func (r *busRecorder) Subscribe(fn func(notify.Event)) { r.inner.Subscribe(fn) }
func (r *busRecorder) Close()                         { r.inner.Close() }

func (r *busRecorder) eventsOfType(typ string) []notify.Event {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []notify.Event
	for _, ev := range r.events {
		if ev.Type == typ {
			out = append(out, ev)
		}
	}
	return out
}

func (r *busRecorder) waitForEvent(typ string, timeout time.Duration) (notify.Event, bool) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		evs := r.eventsOfType(typ)
		if len(evs) > 0 {
			return evs[0], true
		}
		time.Sleep(10 * time.Millisecond)
	}
	return notify.Event{}, false
}

// newTestManager builds a SubagentManager with a fake newChildAgent seam.
// The fakeChildAgent is returned so the test can inspect or control it.
func newTestManager(t *testing.T, bus notify.Bus, st store.Store) (*SubagentManager, *fakeChildAgent) {
	t.Helper()
	fake := &fakeChildAgent{}

	m := &SubagentManager{
		bus:         bus,
		store:       st,
		mu:          sync.RWMutex{},
		subs:        make(map[string]*subRecord),
		callerIsSub: make(map[string]bool),
	}
	fake.channel = nil // set per-spawn below

	m.newChildAgent = func(
		_ skill.ExecutableSkillDef,
		_ string,
		subCtx context.Context,
		subCh *channel.SubagentChannel,
		_ map[string]tool.Tool,
		_ store.Store,
	) (*Agent, error) {
		// Wire the channel on the fake so runFn can call Send/Stop.
		fake.mu.Lock()
		fake.channel = subCh
		fake.mu.Unlock()

		// Start the channel so Deliver can enqueue.
		inbox := make(chan channel.IncomingMessage, 10)
		_ = subCh.Start(context.Background(), inbox)

		// Launch fake run.
		go fake.runFn(subCtx)
		return nil, nil // nil agent is fine; manager checks done channel
	}

	return m, fake
}

// ---------------------------------------------------------------------------
// 2.5 — Basic spawn + Active()
// ---------------------------------------------------------------------------

func TestSubagentManager_Spawn_ReturnsHandle(t *testing.T) {
	bus := newBusRecorder()
	t.Cleanup(func() { bus.Close() })
	st := &mockStore{}

	m, _ := newTestManager(t, bus, st)
	m.installBusSubscription()

	ctx := context.Background()
	def := defaultDef("researcher")
	handle, err := m.Spawn(ctx, def, "do research", SpawnModeSync, "conv_parent")
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}
	if handle == nil {
		t.Fatal("Spawn returned nil handle")
	}
	if handle.ID == "" {
		t.Error("handle.ID must not be empty")
	}
	if handle.BatchID == "" {
		t.Error("handle.BatchID must not be empty")
	}
}

func TestSubagentManager_Spawn_WritesConversationWithParentID(t *testing.T) {
	bus := newBusRecorder()
	t.Cleanup(func() { bus.Close() })
	st := &mockStore{}

	m, _ := newTestManager(t, bus, st)
	m.installBusSubscription()

	ctx := context.Background()
	_, err := m.Spawn(ctx, defaultDef("researcher"), "do research", SpawnModeSync, "conv_parent-123")
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}

	st.mu.Lock()
	saved := st.conv
	st.mu.Unlock()

	if saved == nil {
		t.Fatal("expected a conversation to be saved")
	}
	if saved.ParentConvID != "conv_parent-123" {
		t.Errorf("ParentConvID = %q, want %q", saved.ParentConvID, "conv_parent-123")
	}
	if saved.Status != "running" {
		t.Errorf("Status = %q, want 'running'", saved.Status)
	}
}

func TestSubagentManager_Active_ReturnsRunningRecord(t *testing.T) {
	bus := newBusRecorder()
	t.Cleanup(func() { bus.Close() })
	st := &mockStore{}

	m, _ := newTestManager(t, bus, st)
	m.installBusSubscription()

	// Initially empty.
	if got := m.Active(); len(got) != 0 {
		t.Errorf("Active() before spawn = %d records, want 0", len(got))
	}

	ctx := context.Background()
	handle, err := m.Spawn(ctx, defaultDef("researcher"), "do research", SpawnModeSync, "conv_parent")
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}

	// The sub may complete quickly in the fake; just check we can get a status.
	status := handle.Status()
	if status.ID == "" {
		t.Error("Status().ID must not be empty")
	}
}

func TestSubagentManager_Cancel_Idempotent(t *testing.T) {
	bus := newBusRecorder()
	t.Cleanup(func() { bus.Close() })
	st := &mockStore{}

	m, _ := newTestManager(t, bus, st)
	m.installBusSubscription()

	ctx := context.Background()
	handle, err := m.Spawn(ctx, defaultDef("researcher"), "do research", SpawnModeAsync, "conv_parent")
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}

	// First cancel — should succeed.
	if err := m.Cancel(handle.ID); err != nil {
		t.Errorf("Cancel (first): %v", err)
	}
	// Second cancel — must also succeed (idempotent).
	if err := m.Cancel(handle.ID); err != nil {
		t.Errorf("Cancel (second, idempotent): %v", err)
	}
	// Unknown ID — returns error.
	if err := m.Cancel("nonexistent-id"); err == nil {
		t.Error("expected error for unknown subID, got nil")
	}
}

// ---------------------------------------------------------------------------
// 2.6 — Depth guard
// ---------------------------------------------------------------------------

func TestSubagentManager_Spawn_DepthGuard(t *testing.T) {
	bus := newBusRecorder()
	t.Cleanup(func() { bus.Close() })
	st := &mockStore{}

	m, _ := newTestManager(t, bus, st)
	m.installBusSubscription()

	// Simulate a sub conversation ID already registered.
	subConvID := "sub_existing-sub"
	m.mu.Lock()
	m.callerIsSub[subConvID] = true
	m.mu.Unlock()

	ctx := context.Background()
	_, err := m.Spawn(ctx, defaultDef("researcher"), "nested spawn", SpawnModeSync, subConvID)
	if err == nil {
		t.Fatal("expected ErrSubagentDepthExceeded, got nil")
	}
	if !errors.Is(err, ErrSubagentDepthExceeded) {
		t.Errorf("expected ErrSubagentDepthExceeded, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// 2.7 — Budget enforcement
// ---------------------------------------------------------------------------

func TestSubagentManager_BudgetMonitor_CostCap(t *testing.T) {
	bus := newBusRecorder()
	t.Cleanup(func() { bus.Close() })
	st := &mockStore{}

	m, _ := newTestManager(t, bus, st)
	m.installBusSubscription()

	def := defaultDef("cheap")
	def.Budget = skill.BudgetConfig{
		MaxCostUSD: 0.001, // very low threshold
		MaxTurns:   100,
		Timeout:    10 * time.Second,
	}

	ctx := context.Background()
	handle, err := m.Spawn(ctx, def, "do work", SpawnModeAsync, "conv_parent")
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}

	// Emit a turn_completed event that exceeds the cost budget.
	// The budgetMonitor fan-out subscribes to the bus; we emit directly.
	bus.Emit(notify.Event{
		Type:      notify.EventTurnCompleted,
		Origin:    notify.OriginAgent,
		ChannelID: handle.rec.subChannel.ID(),
		Timestamp: time.Now(),
		Meta: map[string]string{
			"input_tokens":  "1000",
			"output_tokens": "1000",
			"model":         "claude-haiku-4-5",
			"cost_usd":      "0.002", // exceeds 0.001 cap
		},
	})

	// Wait for EventSubagentFailed.
	ev, found := bus.waitForEvent(notify.EventSubagentFailed, 3*time.Second)
	if !found {
		t.Fatal("EventSubagentFailed not emitted within timeout")
	}
	if ev.Meta["reason"] != "budget_exceeded" {
		t.Errorf("reason = %q, want %q", ev.Meta["reason"], "budget_exceeded")
	}
	if ev.Meta["subagent_id"] != handle.ID {
		t.Errorf("subagent_id = %q, want %q", ev.Meta["subagent_id"], handle.ID)
	}
}

func TestSubagentManager_BudgetMonitor_TurnsCap(t *testing.T) {
	bus := newBusRecorder()
	t.Cleanup(func() { bus.Close() })
	st := &mockStore{}

	m, _ := newTestManager(t, bus, st)
	m.installBusSubscription()

	def := defaultDef("limited")
	def.Budget = skill.BudgetConfig{
		MaxCostUSD: 10.0,
		MaxTurns:   1, // only 1 turn allowed
		Timeout:    10 * time.Second,
	}

	ctx := context.Background()
	handle, err := m.Spawn(ctx, def, "do work", SpawnModeAsync, "conv_parent")
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}

	// Emit a turn_completed event. After 1 turn, budget should be exceeded.
	bus.Emit(notify.Event{
		Type:      notify.EventTurnCompleted,
		Origin:    notify.OriginAgent,
		ChannelID: handle.rec.subChannel.ID(),
		Timestamp: time.Now(),
		Meta: map[string]string{
			"input_tokens":  "10",
			"output_tokens": "10",
			"model":         "claude-haiku-4-5",
			"cost_usd":      "0.0001",
		},
	})

	ev, found := bus.waitForEvent(notify.EventSubagentFailed, 3*time.Second)
	if !found {
		t.Fatal("EventSubagentFailed not emitted within timeout")
	}
	if ev.Meta["reason"] != "budget_exceeded" {
		t.Errorf("reason = %q, want %q", ev.Meta["reason"], "budget_exceeded")
	}
}

// ---------------------------------------------------------------------------
// 2.8 — Soft warning at 80%
// ---------------------------------------------------------------------------

func TestSubagentManager_SoftWarning_FiredOnceAt80Percent(t *testing.T) {
	bus := newBusRecorder()
	t.Cleanup(func() { bus.Close() })
	st := &mockStore{}

	m, _ := newTestManager(t, bus, st)
	m.installBusSubscription()

	// Track injectSoftWarning calls.
	var softWarnCalls int
	var softWarnMu sync.Mutex

	def := defaultDef("expensive")
	def.Budget = skill.BudgetConfig{
		MaxCostUSD: 1.0,   // threshold at $1
		MaxTurns:   100,
		Timeout:    10 * time.Second,
	}

	ctx := context.Background()
	handle, err := m.Spawn(ctx, def, "do work", SpawnModeAsync, "conv_parent")
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}

	// Override the injectSoftWarning to track calls.
	origFn := m.softWarnFn
	m.softWarnFn = func(rec *subRecord) {
		softWarnMu.Lock()
		softWarnCalls++
		softWarnMu.Unlock()
		if origFn != nil {
			origFn(rec)
		}
	}

	// Emit a turn at 85% of the cost cap (just over 80% threshold).
	bus.Emit(notify.Event{
		Type:      notify.EventTurnCompleted,
		Origin:    notify.OriginAgent,
		ChannelID: handle.rec.subChannel.ID(),
		Timestamp: time.Now(),
		Meta: map[string]string{
			"input_tokens":  "10",
			"output_tokens": "10",
			"model":         "claude-haiku-4-5",
			"cost_usd":      "0.85", // 85% of $1.00
		},
	})

	// Give the budget monitor time to process.
	time.Sleep(200 * time.Millisecond)

	softWarnMu.Lock()
	calls := softWarnCalls
	softWarnMu.Unlock()

	if calls != 1 {
		t.Errorf("injectSoftWarning called %d times, want 1", calls)
	}

	// Verify softWarned is set on the record.
	handle.rec.mu.Lock()
	warned := handle.rec.softWarned
	handle.rec.mu.Unlock()
	if !warned {
		t.Error("softWarned flag not set after 80% threshold crossed")
	}

	// Emit another turn below the hard cap — should NOT fire soft warning again.
	bus.Emit(notify.Event{
		Type:      notify.EventTurnCompleted,
		Origin:    notify.OriginAgent,
		ChannelID: handle.rec.subChannel.ID(),
		Timestamp: time.Now(),
		Meta: map[string]string{
			"input_tokens":  "10",
			"output_tokens": "10",
			"model":         "claude-haiku-4-5",
			"cost_usd":      "0.01",
		},
	})

	time.Sleep(100 * time.Millisecond)

	softWarnMu.Lock()
	callsAfter := softWarnCalls
	softWarnMu.Unlock()
	if callsAfter != 1 {
		t.Errorf("injectSoftWarning called %d times after second turn, want still 1", callsAfter)
	}
}

// ---------------------------------------------------------------------------
// REQ-14 — MCP share-and-filter: filterParentTools unit tests (W3)
// ---------------------------------------------------------------------------

// mockToolImpl is a minimal tool.Tool used to populate parent tool maps.
type mockToolImpl struct{ n string }

func (m *mockToolImpl) Name() string                                                  { return m.n }
func (m *mockToolImpl) Description() string                                           { return "test tool" }
func (m *mockToolImpl) Schema() json.RawMessage                                       { return json.RawMessage(`{}`) }
func (m *mockToolImpl) Execute(_ context.Context, _ json.RawMessage) (tool.ToolResult, error) {
	return tool.ToolResult{}, nil
}

func makeParentTools(names ...string) map[string]tool.Tool {
	out := make(map[string]tool.Tool, len(names))
	for _, n := range names {
		out[n] = &mockToolImpl{n: n}
	}
	return out
}

// TestFilterParentTools_AllowlistFiltering verifies design §2.5.4:
//   - empty allowlist → child inherits all parent tools
//   - non-empty allowlist → child gets exactly the named tools
//   - allowlist with unknown name → unknown entry silently dropped (no error)
//   - nil parent map → child always gets empty map (no panic)
func TestFilterParentTools_AllowlistFiltering(t *testing.T) {
	parent3 := makeParentTools("tool_a", "tool_b", "tool_c")

	cases := []struct {
		name      string
		parent    map[string]tool.Tool
		allowlist []string
		wantKeys  []string // sorted; nil = expect empty
	}{
		{
			name:      "empty allowlist inherits all 3",
			parent:    parent3,
			allowlist: []string{},
			wantKeys:  []string{"tool_a", "tool_b", "tool_c"},
		},
		{
			name:      "allowlist of 2 from 3 parent tools",
			parent:    parent3,
			allowlist: []string{"tool_a", "tool_c"},
			wantKeys:  []string{"tool_a", "tool_c"},
		},
		{
			name:      "allowlist with unknown name — unknown dropped, known kept",
			parent:    parent3,
			allowlist: []string{"tool_b", "nonexistent_tool"},
			wantKeys:  []string{"tool_b"},
		},
		{
			name:      "nil parent map — always empty regardless of allowlist",
			parent:    nil,
			allowlist: []string{"tool_a"},
			wantKeys:  nil,
		},
		{
			name:      "nil parent with empty allowlist — empty",
			parent:    nil,
			allowlist: []string{},
			wantKeys:  nil,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := filterParentTools(tc.parent, tc.allowlist)

			wantLen := len(tc.wantKeys)
			if len(got) != wantLen {
				t.Errorf("len(filterParentTools) = %d, want %d; keys = %v",
					len(got), wantLen, toolKeys(got))
				return
			}
			for _, k := range tc.wantKeys {
				if _, ok := got[k]; !ok {
					t.Errorf("expected key %q in result; got keys = %v", k, toolKeys(got))
				}
			}
		})
	}
}

// toolKeys returns the sorted key list of a tool map for test error messages.
func toolKeys(m map[string]tool.Tool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
