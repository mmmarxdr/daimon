// Package tool provides LLM-callable builtin tools. This file defines the
// TodoItem/TodoList data model, the JSON envelope encode/decode helpers,
// and the dependency types used by the todo tool set (PR2).
package tool

import (
	"encoding/json"
	"fmt"
	"time"
)

// maxActiveTodos is the soft cap on active (non-terminal) todo items per
// conversation. Items with status "pending" or "in_progress" count against
// the cap; "completed" and "cancelled" items do not (AD-5).
const maxActiveTodos = 200

// IDGen is the function type used to generate todo item IDs.
// The default implementation uses crypto/rand; tests inject deterministic sequences.
type IDGen func() string

// TodoItem represents a single task in the per-conversation todo list (REQ-1).
// Position is 1-based and stable across status changes (AD-8 / Q5).
type TodoItem struct {
	// ID is a unique identifier with the "td_" prefix (e.g. "td_a1b2c3d4").
	ID string `json:"id"`

	// Content is the task description; non-empty is required at create time.
	Content string `json:"content"`

	// Status is one of "pending", "in_progress", "completed", "cancelled".
	Status string `json:"status"`

	// Position is 1-based and stable across status changes.
	Position int `json:"position"`

	// CreatedAt is the wall-clock time when the item was first created.
	CreatedAt time.Time `json:"created_at"`

	// UpdatedAt is the wall-clock time of the last mutation.
	UpdatedAt time.Time `json:"updated_at"`
}

// TodoList is the envelope stored as a JSON string under the
// "daimon/todolist" key in conv.Metadata (REQ-1, AD-7).
//
// Zero value: Version=0 (callers see Version=1 after decodeTodoList("")).
// The zero value of Items is nil, treated as empty.
type TodoList struct {
	// Version starts at 1 and provides a forward-compatibility hook (AD-7).
	Version int `json:"version"`

	// Items holds every todo entry ordered by position.
	Items []TodoItem `json:"items"`
}

// TodoMutator is the callback that tools use to atomically read-modify-write
// the live TodoList for a conversation. The agent layer provides the
// implementation (internal/agent/todo_bridge.go); tools must not call the
// store directly.
//
// Mutate decodes the current list, invokes fn with a pointer to it, then
// encodes and persists the result. It returns the final list so callers can
// inspect counts for event payloads.
type TodoMutator func(convID string, mutate func(list *TodoList) error) (TodoList, error)

// TodoToolDeps holds the callback dependencies for the todo tool set.
// Using callback functions avoids import cycles between internal/tool and
// internal/agent (mirrors MemoryToolDeps).
type TodoToolDeps struct {
	// Mutate atomically mutates and persists the TodoList for convID.
	// Nil-guarded by each tool's Execute method.
	Mutate TodoMutator

	// Read loads the current TodoList for convID without mutating it.
	// Used by todo_list (read-only path).
	Read func(convID string) (TodoList, error)

	// IDGen generates a new unique todo item ID.
	// Default: crypto/rand 4-byte hex with "td_" prefix.
	// Tests inject a deterministic counter.
	IDGen IDGen
}

// encodeTodoList serialises list to a JSON string suitable for storage
// in conv.Metadata["daimon/todolist"].
//
// The output is a JSON object (not double-encoded) — the Metadata value
// is already a string field in the store, so one encoding level suffices.
func encodeTodoList(list TodoList) (string, error) {
	b, err := json.Marshal(list)
	if err != nil {
		return "", fmt.Errorf("encodeTodoList: %w", err)
	}
	return string(b), nil
}

// decodeTodoList deserialises a JSON string produced by encodeTodoList.
// An empty or absent key (s == "") returns a default TodoList{Version:1}
// with no error, satisfying the zero-value-is-useful contract (AD-7, REQ-1).
func decodeTodoList(s string) (TodoList, error) {
	if s == "" {
		return TodoList{Version: 1}, nil
	}
	var list TodoList
	if err := json.Unmarshal([]byte(s), &list); err != nil {
		return TodoList{Version: 1}, fmt.Errorf("decodeTodoList: %w", err)
	}
	return list, nil
}
