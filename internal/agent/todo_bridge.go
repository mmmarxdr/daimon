// Package agent — todo_bridge.go
//
// Provides the per-turn active-conv registry (AD-1 / D4 resolution) and the
// TodoToolDeps constructor that wires the registry into the tool layer without
// an import cycle.
//
// Design rationale:
//
//   - activeTurns holds a *store.Conversation for each in-flight turn so that
//     the todo tool callbacks can locate the live pointer and mutate Metadata
//     in place. The existing turn-end SaveConversation at loop.go:952 then
//     persists the mutation naturally — no change to the save site needed.
//
//   - Lock ordering: activeTurnsMu is INDEPENDENT of all other agent mutexes
//     (modeMu, providerMu, toolsMu). The mutex guards only the map; the caller
//     copies the pointer out and releases the lock before calling the store or
//     bus (AD-2 / lock-ordering comment).
//
//   - Bus nil-guard: all emit calls check a.bus != nil so unit tests without a
//     wired bus do not panic (known risk documented in PR3 task brief).
package agent

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"daimon/internal/notify"
	"daimon/internal/store"
	"daimon/internal/tool"
)

// registerActiveConv records conv in the per-turn registry so that the todo
// callbacks can locate the live *conv by ID during a turn.
// Call immediately after conv is resolved in processMessage; pair with
// defer unregisterActiveConv(conv.ID).
func (a *Agent) registerActiveConv(conv *store.Conversation) {
	a.activeTurnsMu.Lock()
	a.activeTurns[conv.ID] = conv
	a.activeTurnsMu.Unlock()
}

// unregisterActiveConv removes the conv from the registry when the turn ends.
func (a *Agent) unregisterActiveConv(id string) {
	a.activeTurnsMu.Lock()
	delete(a.activeTurns, id)
	a.activeTurnsMu.Unlock()
}

// lookupActiveConv returns the live *conv for id, or nil if not registered.
// The caller must NOT hold activeTurnsMu when calling this — the method
// acquires and releases the lock internally (copy-then-release, AD-2).
func (a *Agent) lookupActiveConv(id string) *store.Conversation {
	a.activeTurnsMu.Lock()
	conv := a.activeTurns[id]
	a.activeTurnsMu.Unlock()
	return conv
}

// TodoToolDeps constructs a tool.TodoToolDeps whose callbacks are backed by
// this agent's per-turn registry and store.
//
// Mutate callback (AD-1):
//  1. Lookup the live *conv in the registry.
//  2. If absent (e.g. cron context without an active turn), fall back to
//     loading from the store.
//  3. Decode conv.Metadata["daimon/todolist"] → TodoList (empty if missing).
//  4. Invoke the caller's mutate fn.
//  5. Re-encode and write back into conv.Metadata (preserving all other keys).
//  6. If conv came from the registry: do NOT call SaveConversation (the loop
//     owns the save at loop.go:952). If from the store fallback: call
//     SaveConversation explicitly.
//  7. On success, emit agent.todolist.changed via a.bus (nil-guarded).
//
// Read callback: lookup or load the conv, decode, return. No mutation.
//
// The caller (cmd/daimon/todo_wiring.go) invokes this method AFTER agent.New
// because the registry is initialised in New.
func (a *Agent) TodoToolDeps() tool.TodoToolDeps {
	return tool.TodoToolDeps{
		Mutate: a.todoMutate,
		Read:   a.todoRead,
	}
}

// todoMutate is the concrete TodoMutator implementation (AD-1, AD-2, AD-6).
func (a *Agent) todoMutate(convID string, mutate func(list *tool.TodoList) (string, error)) (tool.TodoList, error) {
	// Step 1 & 2: locate the live *conv (registry first, store fallback).
	conv, fromRegistry := a.resolveTodoConv(convID)
	if conv == nil {
		return tool.TodoList{}, fmt.Errorf("todoMutate: conversation %q not found", convID)
	}

	// Step 3: decode current list (empty key → empty list, AD-7).
	before, err := decodeTodoListFromMetadata(conv)
	if err != nil {
		return tool.TodoList{}, fmt.Errorf("todoMutate: decode: %w", err)
	}
	beforeCount := len(before.Items)

	// Work on a copy so we don't expose the partial mutation on error.
	after := before

	// Step 4: apply the mutation. Bail out without writing on error.
	// itemID is the ID of the affected item; "" signals no item was mutated
	// (e.g. not-found update), in which case no event should be emitted.
	itemID, err := mutate(&after)
	if err != nil {
		return tool.TodoList{}, err
	}

	// Step 5: encode and write back, preserving other keys.
	encoded, err := encodeTodoList(after)
	if err != nil {
		return tool.TodoList{}, fmt.Errorf("todoMutate: encode: %w", err)
	}
	if conv.Metadata == nil {
		conv.Metadata = make(map[string]string)
	}
	conv.Metadata[tool.TodoMetadataKey] = encoded

	// Step 6: only save explicitly for the store-fallback path.
	if !fromRegistry {
		if err := a.store.SaveConversation(context.Background(), *conv); err != nil {
			return tool.TodoList{}, fmt.Errorf("todoMutate: SaveConversation (fallback): %w", err)
		}
	}

	// Step 7: emit event only when an item was actually mutated (AD-6, REQ-3).
	// itemID == "" means the closure did not affect any item (e.g. not-found);
	// in that case no event is emitted.
	if itemID != "" {
		action := "update"
		if len(after.Items) > beforeCount {
			action = "create"
		}
		a.emitTodoChanged(convID, after, action, itemID)
	}

	return after, nil
}

// todoRead is the concrete Read callback for TodoToolDeps (AD-1, read-only path).
func (a *Agent) todoRead(convID string) (tool.TodoList, error) {
	conv, _ := a.resolveTodoConv(convID)
	if conv == nil {
		// No active turn and not in store — return empty list.
		return tool.TodoList{Version: 1}, nil
	}
	list, err := decodeTodoListFromMetadata(conv)
	if err != nil {
		return tool.TodoList{}, fmt.Errorf("todoRead: %w", err)
	}
	return list, nil
}

// resolveTodoConv looks up the live *conv in the registry first (preferred),
// then falls back to a synchronous store.LoadConversation. Returns (conv,
// fromRegistry): fromRegistry=true means the caller must NOT call SaveConversation.
func (a *Agent) resolveTodoConv(convID string) (*store.Conversation, bool) {
	if conv := a.lookupActiveConv(convID); conv != nil {
		return conv, true
	}
	// Store-fallback path: used by cron tasks or any caller without a live turn.
	conv, err := a.store.LoadConversation(context.Background(), convID)
	if err != nil {
		return nil, false
	}
	return conv, false
}

// decodeTodoListFromMetadata reads conv.Metadata["daimon/todolist"] and
// returns a TodoList. An absent or empty key returns TodoList{Version:1}.
func decodeTodoListFromMetadata(conv *store.Conversation) (tool.TodoList, error) {
	if conv.Metadata == nil {
		return tool.TodoList{Version: 1}, nil
	}
	raw := conv.Metadata[tool.TodoMetadataKey]
	if raw == "" {
		return tool.TodoList{Version: 1}, nil
	}
	return tool.DecodeTodoList(raw)
}

// encodeTodoList delegates to the exported helper from internal/tool.
func encodeTodoList(list tool.TodoList) (string, error) {
	return tool.EncodeTodoList(list)
}

// emitTodoChanged emits an agent.todolist.changed event on the bus (nil-guarded).
// action is "create" when items grew, "update" when an existing item changed (AD-6).
// itemID is the ID of the created or mutated item, supplied by the mutate closure
// so the bridge always reports the exact affected item rather than guessing.
func (a *Agent) emitTodoChanged(convID string, list tool.TodoList, action, itemID string) {
	if a.bus == nil {
		return
	}
	a.bus.Emit(notify.Event{
		Type:      notify.EventTodolistChanged,
		Origin:    notify.OriginAgent,
		Timestamp: time.Now(),
		Meta: map[string]string{
			"conv_id":    convID,
			"action":     action,
			"item_id":    itemID,
			"item_count": strconv.Itoa(len(list.Items)),
		},
	})
}
