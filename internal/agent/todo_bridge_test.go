package agent

// todo_bridge_test.go — unit tests for the activeTurns registry and TodoToolDeps
// callbacks (tasks 3.1, 3.2, 3.4; REQ-5, REQ-6, REQ-7).
//
// Strict TDD: tests are written RED first, then the implementation makes them GREEN.
//
// Coverage:
//   - registerActiveConv / unregisterActiveConv / lookupActiveConv thread-safety
//   - TodoToolDeps().Mutate mutates the live *conv in place (D4 clobber-safety)
//   - TodoToolDeps().Read decodes an existing key correctly
//   - bus nil-guard: Mutate does not panic when a.bus is nil (unit test scenario)
//   - EventTodolistChanged emitted on Mutate success (via recordingBus)
//   - event meta: conv_id, action, item_count, item_id present
//   - Mutate on unknown convID falls back to a store load (store-fallback path)

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"sync"
	"testing"
	"time"

	"daimon/internal/audit"
	"daimon/internal/channel"
	"daimon/internal/config"
	"daimon/internal/content"
	"daimon/internal/notify"
	"daimon/internal/provider"
	"daimon/internal/skill"
	"daimon/internal/store"
	"daimon/internal/tool"
)

// ---------------------------------------------------------------------------
// 3.1 — registry thread-safety
// ---------------------------------------------------------------------------

// TestActiveTurnsRegistry_ConcurrentOps exercises registerActiveConv,
// lookupActiveConv, and unregisterActiveConv under concurrent load without
// data races. The three operations use separate sync phases to ensure ordering:
// all registers complete, then concurrent lookups, then all unregisters.
func TestActiveTurnsRegistry_ConcurrentOps(t *testing.T) {
	t.Parallel()
	ag := newMinimalAgent(t)

	const n = 50

	// Phase 1: concurrent registers (all must complete before lookups).
	var wgReg sync.WaitGroup
	wgReg.Add(n)
	for i := 0; i < n; i++ {
		id := strconv.Itoa(i)
		go func() {
			defer wgReg.Done()
			ag.registerActiveConv(&store.Conversation{ID: id})
		}()
	}
	wgReg.Wait()

	// Phase 2: concurrent lookups (all convs must be found).
	var wgLook sync.WaitGroup
	wgLook.Add(n)
	for i := 0; i < n; i++ {
		id := strconv.Itoa(i)
		go func() {
			defer wgLook.Done()
			if ag.lookupActiveConv(id) == nil {
				t.Errorf("lookupActiveConv(%s) = nil after register, expected non-nil", id)
			}
		}()
	}
	wgLook.Wait()

	// Phase 3: concurrent unregisters.
	var wgUnreg sync.WaitGroup
	wgUnreg.Add(n)
	for i := 0; i < n; i++ {
		id := strconv.Itoa(i)
		go func() {
			defer wgUnreg.Done()
			ag.unregisterActiveConv(id)
		}()
	}
	wgUnreg.Wait()

	// After all unregisters the registry must be empty.
	for i := 0; i < n; i++ {
		if got := ag.lookupActiveConv(strconv.Itoa(i)); got != nil {
			t.Errorf("lookupActiveConv(%d) = non-nil after unregister", i)
		}
	}
}

// TestActiveTurnsRegistry_RegisterLookupUnregister verifies the sequential
// contract: register → lookup returns the pointer → unregister → lookup returns nil.
func TestActiveTurnsRegistry_RegisterLookupUnregister(t *testing.T) {
	t.Parallel()
	ag := newMinimalAgent(t)

	conv := &store.Conversation{ID: "conv-rl-1"}
	ag.registerActiveConv(conv)

	got := ag.lookupActiveConv("conv-rl-1")
	if got == nil {
		t.Fatal("lookupActiveConv: expected non-nil after register, got nil")
	}
	if got != conv {
		t.Error("lookupActiveConv: returned a different pointer than registered")
	}

	ag.unregisterActiveConv("conv-rl-1")
	if ag.lookupActiveConv("conv-rl-1") != nil {
		t.Error("lookupActiveConv: expected nil after unregister, got non-nil")
	}
}

// ---------------------------------------------------------------------------
// 3.2 — TodoToolDeps callbacks
// ---------------------------------------------------------------------------

// TestTodoToolDeps_Mutate_MutatesLiveConv verifies the Mutate callback
// locates the live *conv via the registry and mutates Metadata in place.
func TestTodoToolDeps_Mutate_MutatesLiveConv(t *testing.T) {
	t.Parallel()

	conv := &store.Conversation{
		ID:       "conv-mut-1",
		Metadata: map[string]string{},
	}
	ag := newMinimalAgent(t)
	ag.registerActiveConv(conv)
	defer ag.unregisterActiveConv(conv.ID)

	deps := ag.TodoToolDeps()

	list, err := deps.Mutate("conv-mut-1", func(l *tool.TodoList) error {
		l.Items = append(l.Items, tool.TodoItem{
			ID:      "td_00000001",
			Content: "write tests",
			Status:  "pending",
		})
		return nil
	})
	if err != nil {
		t.Fatalf("Mutate returned error: %v", err)
	}
	if len(list.Items) != 1 {
		t.Fatalf("expected 1 item in returned list, got %d", len(list.Items))
	}

	// Verify the live conv pointer was mutated.
	val, ok := conv.Metadata["daimon/todolist"]
	if !ok {
		t.Fatal("conv.Metadata missing 'daimon/todolist' after Mutate")
	}
	var decoded tool.TodoList
	if err := json.Unmarshal([]byte(val), &decoded); err != nil {
		t.Fatalf("could not decode persisted list: %v", err)
	}
	if len(decoded.Items) != 1 || decoded.Items[0].ID != "td_00000001" {
		t.Errorf("persisted list: unexpected content %+v", decoded)
	}
}

// TestTodoToolDeps_Mutate_OtherMetadataPreserved verifies REQ-5:
// Mutate does not clobber pre-existing metadata keys.
func TestTodoToolDeps_Mutate_OtherMetadataPreserved(t *testing.T) {
	t.Parallel()

	conv := &store.Conversation{
		ID:       "conv-meta-1",
		Metadata: map[string]string{"some/other-key": "value"},
	}
	ag := newMinimalAgent(t)
	ag.registerActiveConv(conv)
	defer ag.unregisterActiveConv(conv.ID)

	deps := ag.TodoToolDeps()
	_, err := deps.Mutate("conv-meta-1", func(l *tool.TodoList) error {
		l.Items = append(l.Items, tool.TodoItem{ID: "td_00000001", Content: "x", Status: "pending"})
		return nil
	})
	if err != nil {
		t.Fatalf("Mutate returned error: %v", err)
	}
	if got := conv.Metadata["some/other-key"]; got != "value" {
		t.Errorf("other metadata key clobbered: want 'value', got %q", got)
	}
}

// TestTodoToolDeps_Mutate_NilBus verifies the nil-guard: Mutate must not
// panic when a.bus is nil (unit test scenario — no notify bus wired).
func TestTodoToolDeps_Mutate_NilBus(t *testing.T) {
	t.Parallel()

	conv := &store.Conversation{
		ID:       "conv-nilbus-1",
		Metadata: map[string]string{},
	}
	ag := newMinimalAgent(t) // bus is nil in minimal agent
	ag.registerActiveConv(conv)
	defer ag.unregisterActiveConv(conv.ID)

	deps := ag.TodoToolDeps()

	// Must not panic.
	_, err := deps.Mutate("conv-nilbus-1", func(l *tool.TodoList) error {
		l.Items = append(l.Items, tool.TodoItem{ID: "td_00000001", Content: "x", Status: "pending"})
		return nil
	})
	if err != nil {
		t.Errorf("Mutate returned error when bus is nil: %v", err)
	}
}

// TestTodoToolDeps_Mutate_EmitsEvent verifies REQ-7: Mutate emits
// agent.todolist.changed with the correct Meta payload when bus is wired.
func TestTodoToolDeps_Mutate_EmitsEvent(t *testing.T) {
	t.Parallel()

	conv := &store.Conversation{
		ID:       "conv-event-1",
		Metadata: map[string]string{},
	}
	bus := &recordingBus{}
	ag := newMinimalAgentWithBus(t, bus)
	ag.registerActiveConv(conv)
	defer ag.unregisterActiveConv(conv.ID)

	deps := ag.TodoToolDeps()
	_, err := deps.Mutate("conv-event-1", func(l *tool.TodoList) error {
		l.Items = append(l.Items, tool.TodoItem{
			ID:      "td_aabbccdd",
			Content: "test",
			Status:  "pending",
		})
		return nil
	})
	if err != nil {
		t.Fatalf("Mutate error: %v", err)
	}

	events := bus.filterByType(notify.EventTodolistChanged)
	if len(events) != 1 {
		t.Fatalf("expected 1 %q event, got %d", notify.EventTodolistChanged, len(events))
	}
	ev := events[0]
	if ev.Origin != notify.OriginAgent {
		t.Errorf("origin = %q, want %q", ev.Origin, notify.OriginAgent)
	}
	if ev.Meta["conv_id"] != conv.ID {
		t.Errorf("meta.conv_id = %q, want %q", ev.Meta["conv_id"], conv.ID)
	}
	if ev.Meta["item_id"] != "td_aabbccdd" {
		t.Errorf("meta.item_id = %q, want 'td_aabbccdd'", ev.Meta["item_id"])
	}
	if ev.Meta["item_count"] != "1" {
		t.Errorf("meta.item_count = %q, want '1'", ev.Meta["item_count"])
	}
	// action is supplied by the mutate caller via the list (implicitly "create" for
	// new items). We verify the key exists; the exact value is set by the tool itself
	// which is tested in the tool unit tests.
	if _, ok := ev.Meta["action"]; !ok {
		t.Error("meta.action missing from event")
	}
}

// TestTodoToolDeps_Mutate_MutatorError verifies that if the mutate fn
// returns an error, Mutate propagates it and does NOT write Metadata.
func TestTodoToolDeps_Mutate_MutatorError(t *testing.T) {
	t.Parallel()

	conv := &store.Conversation{
		ID:       "conv-errmut-1",
		Metadata: map[string]string{},
	}
	ag := newMinimalAgent(t)
	ag.registerActiveConv(conv)
	defer ag.unregisterActiveConv(conv.ID)

	deps := ag.TodoToolDeps()
	sentinel := errors.New("mutate fn failed")
	_, err := deps.Mutate("conv-errmut-1", func(l *tool.TodoList) error {
		return sentinel
	})
	if !errors.Is(err, sentinel) {
		t.Errorf("expected sentinel error, got %v", err)
	}
	if _, ok := conv.Metadata["daimon/todolist"]; ok {
		t.Error("Metadata was written despite mutate fn error")
	}
}

// TestTodoToolDeps_Read_DecodesExistingKey verifies the Read callback
// decodes a pre-existing "daimon/todolist" metadata value.
func TestTodoToolDeps_Read_DecodesExistingKey(t *testing.T) {
	t.Parallel()

	pre := tool.TodoList{
		Version: 1,
		Items: []tool.TodoItem{
			{ID: "td_11223344", Content: "read me", Status: "pending", Position: 1,
				CreatedAt: time.Now(), UpdatedAt: time.Now()},
		},
	}
	encoded, _ := json.Marshal(pre)
	conv := &store.Conversation{
		ID:       "conv-read-1",
		Metadata: map[string]string{"daimon/todolist": string(encoded)},
	}
	ag := newMinimalAgent(t)
	ag.registerActiveConv(conv)
	defer ag.unregisterActiveConv(conv.ID)

	deps := ag.TodoToolDeps()
	list, err := deps.Read("conv-read-1")
	if err != nil {
		t.Fatalf("Read error: %v", err)
	}
	if len(list.Items) != 1 || list.Items[0].ID != "td_11223344" {
		t.Errorf("Read: unexpected list %+v", list)
	}
}

// TestTodoToolDeps_Read_EmptyKey verifies Read returns an empty TodoList
// when the metadata key is absent (zero-value contract, REQ-1).
func TestTodoToolDeps_Read_EmptyKey(t *testing.T) {
	t.Parallel()

	conv := &store.Conversation{
		ID:       "conv-read-empty-1",
		Metadata: map[string]string{},
	}
	ag := newMinimalAgent(t)
	ag.registerActiveConv(conv)
	defer ag.unregisterActiveConv(conv.ID)

	deps := ag.TodoToolDeps()
	list, err := deps.Read("conv-read-empty-1")
	if err != nil {
		t.Fatalf("Read error: %v", err)
	}
	if len(list.Items) != 0 {
		t.Errorf("expected empty list, got %d items", len(list.Items))
	}
	if list.Version != 1 {
		t.Errorf("expected Version=1, got %d", list.Version)
	}
}

// TestTodoToolDeps_Mutate_StoreFallback verifies that when convID is NOT in
// the active registry (e.g. cron context), Mutate falls back to loading from
// the store, mutates, and calls SaveConversation.
func TestTodoToolDeps_Mutate_StoreFallback(t *testing.T) {
	t.Parallel()

	stored := &store.Conversation{
		ID:       "conv-fallback-1",
		Metadata: map[string]string{},
	}
	st := &mockStore{conv: stored}
	ag := newMinimalAgentWithStore(t, st)
	// Do NOT register the conv — force the store-fallback path.

	deps := ag.TodoToolDeps()
	_, err := deps.Mutate("conv-fallback-1", func(l *tool.TodoList) error {
		l.Items = append(l.Items, tool.TodoItem{ID: "td_fallback1", Content: "fb", Status: "pending"})
		return nil
	})
	if err != nil {
		t.Fatalf("Mutate (store-fallback) error: %v", err)
	}

	// Store should have been called with the mutated conversation.
	st.mu.Lock()
	saved := st.conv
	st.mu.Unlock()
	if saved == nil {
		t.Fatal("store.SaveConversation not called on fallback path")
	}
	if _, ok := saved.Metadata["daimon/todolist"]; !ok {
		t.Error("saved conv missing 'daimon/todolist' key after store-fallback Mutate")
	}
}

// ---------------------------------------------------------------------------
// 3.4 — D4 regression test: processMessage with todo_create tool call
// ---------------------------------------------------------------------------

// todoCreateProvider is a provider that:
//   - Call 0: issues a todo_create tool call.
//   - Call 1: returns "done" (terminal).
//
// It is structurally identical to executionGateTestProvider in
// loop_mode_integration_test.go but wired for todo_create specifically.
type todoCreateProvider struct {
	mu        sync.Mutex
	callCount int
}

func (p *todoCreateProvider) Name() string                                  { return "todo-test" }
func (p *todoCreateProvider) Model() string                                 { return "todo-model" }
func (p *todoCreateProvider) SupportsTools() bool                           { return true }
func (p *todoCreateProvider) SupportsMultimodal() bool                      { return false }
func (p *todoCreateProvider) SupportsAudio() bool                           { return false }
func (p *todoCreateProvider) HealthCheck(_ context.Context) (string, error) { return "ok", nil }

func (p *todoCreateProvider) Chat(_ context.Context, req provider.ChatRequest) (*provider.ChatResponse, error) {
	p.mu.Lock()
	call := p.callCount
	p.callCount++
	p.mu.Unlock()

	if call == 0 {
		return &provider.ChatResponse{
			ToolCalls: []provider.ToolCall{
				{
					ID:    "tc-todo-1",
					Name:  "todo_create",
					Input: json.RawMessage(`{"content":"D4 regression item"}`),
				},
			},
		}, nil
	}
	return &provider.ChatResponse{Content: "done"}, nil
}

// TestD4_TodoCreate_SurvivesTurnEnd is the D4 regression test (task 3.4).
//
// It drives processMessage with a todoCreateProvider that issues one
// todo_create call during the turn. After the turn completes, the test
// asserts that conv.Metadata["daimon/todolist"] in the store contains the
// created item (i.e. the end-of-turn SaveConversation preserved the mutation).
func TestD4_TodoCreate_SurvivesTurnEnd(t *testing.T) {
	t.Parallel()

	convID := "conv-d4-1"
	st := &mockStore{
		conv: &store.Conversation{
			ID:        convID,
			ChannelID: "d4-ch",
			Metadata:  map[string]string{},
		},
	}

	prov := &todoCreateProvider{}
	ch := &mockChannel{}

	ag := New(
		config.AgentConfig{MaxIterations: 5, MaxTokensPerTurn: 100},
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

	// Wire todo tools with the agent's own deps.
	todoTools := tool.BuildTodoTools(ag.TodoToolDeps())
	ag.toolsMu.Lock()
	for name, t := range todoTools {
		ag.tools[name] = t
	}
	ag.toolsMu.Unlock()

	ag.processMessage(context.Background(), channel.IncomingMessage{
		ChannelID: "d4-ch",
		SenderID:  "u1",
		Content:   content.TextBlock("create a todo"),
	})

	// After the turn, the store must contain the created item.
	st.mu.Lock()
	saved := st.conv
	st.mu.Unlock()
	if saved == nil {
		t.Fatal("store has no saved conversation after turn")
	}
	raw, ok := saved.Metadata["daimon/todolist"]
	if !ok {
		t.Fatal("conv.Metadata missing 'daimon/todolist' after turn — D4 regression!")
	}
	var list tool.TodoList
	if err := json.Unmarshal([]byte(raw), &list); err != nil {
		t.Fatalf("could not decode saved list: %v", err)
	}
	if len(list.Items) == 0 {
		t.Fatal("todo list is empty after turn — D4 regression: todo_create did not persist!")
	}
	if list.Items[0].Content != "D4 regression item" {
		t.Errorf("unexpected item content: %q", list.Items[0].Content)
	}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// newMinimalAgent builds the smallest possible Agent for bridge unit tests.
// bus is nil (use newMinimalAgentWithBus when you need event assertions).
func newMinimalAgent(t *testing.T) *Agent {
	t.Helper()
	return newMinimalAgentWithStore(t, &mockStore{})
}

func newMinimalAgentWithStore(t *testing.T, st store.Store) *Agent {
	t.Helper()
	return New(
		config.AgentConfig{MaxIterations: 5, MaxTokensPerTurn: 100},
		defaultLimits(),
		config.FilterConfig{},
		&mockChannel{},
		&mockProvider{},
		st,
		audit.NoopAuditor{},
		nil,
		nil,
		skill.SkillIndex{},
		4,
		false,
	)
}

func newMinimalAgentWithBus(t *testing.T, bus notify.Bus) *Agent {
	t.Helper()
	ag := newMinimalAgent(t)
	ag.bus = bus
	return ag
}
