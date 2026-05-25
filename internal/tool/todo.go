// Package tool provides LLM-callable builtin tools. This file defines the
// TodoItem/TodoList data model, the JSON envelope encode/decode helpers,
// the dependency types, and the three LLM-callable todo tools (PR2).
package tool

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

// maxActiveTodos is the soft cap on active (non-terminal) todo items per
// conversation. Items with status "pending" or "in_progress" count against
// the cap; "completed" and "cancelled" items do not (AD-5).
const maxActiveTodos = 200

// TodoMetadataKey is the conv.Metadata key under which the JSON-encoded
// TodoList is stored. Exported so the agent bridge can read/write the key
// without duplicating the constant.
const TodoMetadataKey = "daimon/todolist"

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

// EncodeTodoList is the exported counterpart of encodeTodoList.
// Used by the agent bridge (internal/agent/todo_bridge.go) to persist the list
// without duplicating the encoding logic.
func EncodeTodoList(list TodoList) (string, error) {
	return encodeTodoList(list)
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

// DecodeTodoList is the exported counterpart of decodeTodoList.
// Used by the agent bridge (internal/agent/todo_bridge.go).
func DecodeTodoList(s string) (TodoList, error) {
	return decodeTodoList(s)
}

// validStatuses is the set of permitted status enum values (AD-8).
var validStatuses = map[string]bool{
	"pending":     true,
	"in_progress": true,
	"completed":   true,
	"cancelled":   true,
}

// errCancelledTerminal is returned by the todo_update mutate callback when a
// status transition out of "cancelled" is attempted. cancelled is terminal
// (AD-8). It is a typed sentinel so Execute can discriminate it with errors.Is
// rather than matching the error message text.
var errCancelledTerminal = errors.New("cannot transition from cancelled: item is terminal")

// defaultIDGen generates a unique todo item ID using crypto/rand.
// Format: "td_" + 8 lowercase hex characters (32 bits, AD-4).
func defaultIDGen() string {
	b := make([]byte, 4)
	if _, err := rand.Read(b); err != nil {
		// Fallback: use time-based pseudo-random bytes. This path is
		// unreachable in practice; crypto/rand fails only on broken OSes.
		return fmt.Sprintf("td_%08x", time.Now().UnixNano()&0xFFFFFFFF)
	}
	return "td_" + hex.EncodeToString(b)
}

// generateID calls idGen (or defaultIDGen if nil) and retries up to 3 times
// to avoid collisions within the existing list (AD-4, belt-and-suspenders).
func generateID(idGen IDGen, items []TodoItem) string {
	gen := idGen
	if gen == nil {
		gen = defaultIDGen
	}
	for range 3 {
		id := gen()
		exists := false
		for _, it := range items {
			if it.ID == id {
				exists = true
				break
			}
		}
		if !exists {
			return id
		}
	}
	// After 3 collisions (astronomically unlikely), fall back to crypto/rand directly.
	return defaultIDGen()
}

// countActiveItems returns the number of items with status pending or in_progress.
// Terminal items (completed, cancelled) do not count against the cap (AD-5).
func countActiveItems(items []TodoItem) int {
	n := 0
	for _, it := range items {
		if it.Status == "pending" || it.Status == "in_progress" {
			n++
		}
	}
	return n
}

// BuildTodoTools constructs the three LLM-callable todo tools and returns them
// keyed by name. The returned map is ready for MergeTools. deps provides the
// Mutate/Read/IDGen callbacks that decouple the tool layer from the agent layer
// (mirrors BuildMemoryTools, avoids import cycles).
func BuildTodoTools(deps TodoToolDeps) map[string]Tool {
	m := make(map[string]Tool)
	m["todo_create"] = &todoCreateTool{deps: deps}
	m["todo_update"] = &todoUpdateTool{deps: deps}
	m["todo_list"] = &todoListTool{deps: deps}
	return m
}

// ---------------------------------------------------------------------------
// todo_create
// ---------------------------------------------------------------------------

// todoCreateTool implements the todo_create LLM-callable tool (REQ-2, AD-4/5/8).
type todoCreateTool struct {
	deps TodoToolDeps
}

func (t *todoCreateTool) Name() string { return "todo_create" }

// Description returns the tool's human-readable description for the LLM.
func (t *todoCreateTool) Description() string {
	return "Create a new todo item in the conversation's todo list. Use to track tasks, sub-goals, or action items."
}

// Schema returns the JSON Schema for todo_create parameters (AD-7).
func (t *todoCreateTool) Schema() json.RawMessage {
	return json.RawMessage(`{
  "type": "object",
  "required": ["content"],
  "properties": {
    "content": {
      "type": "string",
      "description": "The task description. Must be non-empty."
    },
    "position": {
      "type": "integer",
      "minimum": 1,
      "description": "Optional 1-based insert position. Omit to append at the end."
    }
  }
}`)
}

type todoCreateParams struct {
	Content  string `json:"content"`
	Position *int   `json:"position,omitempty"`
}

// Execute runs todo_create: validates params, checks the active-item cap,
// generates an ID, inserts/appends the item, and calls deps.Mutate (AD-4/5/8).
// The Go error return is reserved for infrastructure panics; all user-visible
// errors are returned as ToolResult{IsError:true} (executeWithRecover contract).
func (t *todoCreateTool) Execute(ctx context.Context, params json.RawMessage) (ToolResult, error) {
	convID := ConvIDFromContext(ctx)
	if convID == "" {
		return ToolResult{IsError: true, Content: "no conversation context"}, nil
	}

	// Nil-guard: Mutate is required for create.
	if t.deps.Mutate == nil {
		return ToolResult{IsError: true, Content: "todo_create: mutator not configured"}, nil
	}

	var input todoCreateParams
	if err := json.Unmarshal(params, &input); err != nil {
		return ToolResult{IsError: true, Content: fmt.Sprintf("invalid parameters: %v", err)}, nil
	}

	if strings.TrimSpace(input.Content) == "" {
		return ToolResult{IsError: true, Content: "content cannot be empty"}, nil
	}

	var createdID string
	var createdPos int

	finalList, err := t.deps.Mutate(convID, func(list *TodoList) error {
		// AD-5: soft cap on active (non-terminal) items.
		if countActiveItems(list.Items) >= maxActiveTodos {
			return fmt.Errorf("todo list full: max %d active items (complete or cancel some first)", maxActiveTodos)
		}

		id := generateID(t.deps.IDGen, list.Items)
		now := time.Now().UTC()

		// Determine target position (append or insert, AD-8).
		targetPos := len(list.Items) + 1 // default: append
		if input.Position != nil && *input.Position >= 1 && *input.Position <= len(list.Items)+1 {
			targetPos = *input.Position
		}

		// Shift items at or after targetPos up by 1 (insert semantics).
		for i := range list.Items {
			if list.Items[i].Position >= targetPos {
				list.Items[i].Position++
			}
		}

		newItem := TodoItem{
			ID:        id,
			Content:   input.Content,
			Status:    "pending",
			Position:  targetPos,
			CreatedAt: now,
			UpdatedAt: now,
		}
		list.Items = append(list.Items, newItem)

		// Keep items sorted by position for deterministic output.
		sort.Slice(list.Items, func(a, b int) bool {
			return list.Items[a].Position < list.Items[b].Position
		})

		createdID = id
		createdPos = targetPos
		return nil
	})
	if err != nil {
		return ToolResult{IsError: true, Content: err.Error()}, nil
	}

	_ = finalList // available for callers that need the updated list count
	return ToolResult{
		Content: fmt.Sprintf("Created todo %s (pos %d): %s", createdID, createdPos, input.Content),
	}, nil
}

// ---------------------------------------------------------------------------
// todo_update
// ---------------------------------------------------------------------------

// todoUpdateTool implements the todo_update LLM-callable tool (REQ-3, AD-8).
type todoUpdateTool struct {
	deps TodoToolDeps
}

func (t *todoUpdateTool) Name() string { return "todo_update" }

// Description returns the tool's human-readable description for the LLM.
func (t *todoUpdateTool) Description() string {
	return "Update an existing todo item's content or status. Use the todo ID from todo_list results."
}

// Schema returns the JSON Schema for todo_update parameters (AD-7).
func (t *todoUpdateTool) Schema() json.RawMessage {
	return json.RawMessage(`{
  "type": "object",
  "required": ["id"],
  "properties": {
    "id": {
      "type": "string",
      "description": "The todo item ID to update (from todo_list results)"
    },
    "content": {
      "type": "string",
      "description": "New task description"
    },
    "status": {
      "type": "string",
      "enum": ["pending", "in_progress", "completed", "cancelled"],
      "description": "New status for the item"
    }
  }
}`)
}

type todoUpdateParams struct {
	ID      string  `json:"id"`
	Content *string `json:"content,omitempty"`
	Status  *string `json:"status,omitempty"`
}

// Execute runs todo_update: locates an item by ID, validates the mutation,
// applies fields, and persists via deps.Mutate.
func (t *todoUpdateTool) Execute(ctx context.Context, params json.RawMessage) (ToolResult, error) {
	convID := ConvIDFromContext(ctx)
	if convID == "" {
		return ToolResult{IsError: true, Content: "no conversation context"}, nil
	}

	// Nil-guard: Mutate is required for update.
	if t.deps.Mutate == nil {
		return ToolResult{IsError: true, Content: "todo_update: mutator not configured"}, nil
	}

	var input todoUpdateParams
	if err := json.Unmarshal(params, &input); err != nil {
		return ToolResult{IsError: true, Content: fmt.Sprintf("invalid parameters: %v", err)}, nil
	}

	// At least one field besides id must be present (REQ-3).
	if input.Content == nil && input.Status == nil {
		return ToolResult{IsError: true, Content: "at least one of 'content' or 'status' must be provided"}, nil
	}

	// Validate the requested status before any mutation.
	if input.Status != nil && !validStatuses[*input.Status] {
		return ToolResult{IsError: true, Content: fmt.Sprintf("invalid status %q: must be one of pending, in_progress, completed, cancelled", *input.Status)}, nil
	}

	// notFound is a sentinel for the mutate callback to communicate that the
	// item was not found without returning an error (not-found is non-fatal).
	notFound := false

	_, err := t.deps.Mutate(convID, func(list *TodoList) error {
		var target *TodoItem
		for i := range list.Items {
			if list.Items[i].ID == input.ID {
				target = &list.Items[i]
				break
			}
		}
		if target == nil {
			notFound = true
			return nil
		}

		// AD-8: cancelled is terminal — no transition out of cancelled is permitted.
		if target.Status == "cancelled" && input.Status != nil {
			return fmt.Errorf("%w (%s)", errCancelledTerminal, input.ID)
		}

		if input.Content != nil {
			target.Content = *input.Content
		}
		if input.Status != nil {
			target.Status = *input.Status
		}
		target.UpdatedAt = time.Now().UTC()
		return nil
	})

	switch {
	case errors.Is(err, errCancelledTerminal):
		return ToolResult{IsError: true, Content: err.Error()}, nil
	case err != nil:
		return ToolResult{IsError: true, Content: fmt.Sprintf("todo_update failed: %v", err)}, nil
	case notFound:
		// Not-found is informational, not an error (mirrors update_memory.go:343, REQ-3).
		return ToolResult{Content: fmt.Sprintf("Todo not found: %s", input.ID)}, nil
	}

	return ToolResult{Content: fmt.Sprintf("Updated %s", input.ID)}, nil
}

// ---------------------------------------------------------------------------
// todo_list
// ---------------------------------------------------------------------------

// todoListTool implements the todo_list LLM-callable tool (REQ-4).
type todoListTool struct {
	deps TodoToolDeps
}

func (t *todoListTool) Name() string { return "todo_list" }

// Description returns the tool's human-readable description for the LLM.
func (t *todoListTool) Description() string {
	return "List todo items for the current conversation. Optionally filter by status. This tool never mutates the list and never emits events."
}

// Schema returns the JSON Schema for todo_list parameters (AD-7).
func (t *todoListTool) Schema() json.RawMessage {
	return json.RawMessage(`{
  "type": "object",
  "properties": {
    "status": {
      "type": "string",
      "enum": ["pending", "in_progress", "completed", "cancelled"],
      "description": "Optional status filter. Omit to list all items."
    }
  }
}`)
}

type todoListParams struct {
	Status *string `json:"status,omitempty"`
}

// Execute runs todo_list: loads the list, applies an optional status filter,
// and returns a numbered text list ordered by position (REQ-4, AD-8).
// It NEVER calls deps.Mutate and NEVER emits an event.
func (t *todoListTool) Execute(ctx context.Context, params json.RawMessage) (ToolResult, error) {
	convID := ConvIDFromContext(ctx)
	if convID == "" {
		return ToolResult{IsError: true, Content: "no conversation context"}, nil
	}

	// Nil-guard: Read is required.
	if t.deps.Read == nil {
		return ToolResult{IsError: true, Content: "todo_list: read dependency not configured"}, nil
	}

	var input todoListParams
	if err := json.Unmarshal(params, &input); err != nil {
		return ToolResult{IsError: true, Content: fmt.Sprintf("invalid parameters: %v", err)}, nil
	}

	// Validate status filter if provided.
	if input.Status != nil && !validStatuses[*input.Status] {
		return ToolResult{IsError: true, Content: fmt.Sprintf("invalid status filter %q: must be one of pending, in_progress, completed, cancelled", *input.Status)}, nil
	}

	list, err := t.deps.Read(convID)
	if err != nil {
		return ToolResult{IsError: true, Content: fmt.Sprintf("failed to read todos: %v", err)}, nil
	}

	// Filter and sort by position.
	items := list.Items
	if input.Status != nil {
		filtered := items[:0:0]
		for _, it := range items {
			if it.Status == *input.Status {
				filtered = append(filtered, it)
			}
		}
		items = filtered
	}

	sort.Slice(items, func(a, b int) bool {
		return items[a].Position < items[b].Position
	})

	if len(items) == 0 {
		return ToolResult{Content: "No todos."}, nil
	}

	// Format: "N. [status] content (id: td_xxxx)" (REQ-4).
	var sb strings.Builder
	for i, it := range items {
		fmt.Fprintf(&sb, "%d. [%s] %s (id: %s)\n", i+1, it.Status, it.Content, it.ID)
	}

	return ToolResult{Content: strings.TrimRight(sb.String(), "\n")}, nil
}
