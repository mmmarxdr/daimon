# Delta for Output Store (output-store)

## Overview

Fixes two bugs in `sqlitestore.go`: `SearchOutputs` LIKE fallback does not escape `%` and `_` metacharacters (M3), and `IndexOutput` accepts empty required fields without returning an error (M4).

## MODIFIED Requirements

### Requirement: SearchOutput Tool

The system SHALL provide a `search_output` tool that queries indexed outputs via FTS5.

(Previously: LIKE fallback path used unescaped user input — a query containing `%` or `_` would match more rows than intended. No SQL injection risk (params bound), but semantically incorrect results.)

#### Scenario: Search returns matching outputs (existing — unchanged)

- GIVEN indexed outputs from previous tool executions
- WHEN search_output is called with query "git status"
- THEN results include outputs matching "git status" ranked by relevance
- AND each result includes command, timestamp, and truncated content preview

#### Scenario: Search with no results (existing — unchanged)

- GIVEN indexed outputs
- WHEN search_output is called with query "nonexistent_term_xyz"
- THEN results array is empty

#### Scenario: Search limit respected (existing — unchanged)

- GIVEN 50 indexed outputs
- WHEN search_output is called with limit=5
- THEN at most 5 results are returned

#### Scenario: LIKE query containing percent literal

- GIVEN indexed output with content "CPU at 50% usage"
- AND the FTS query builder returns empty (no keywords in "50%")
- WHEN search_output is called with query "50%"
- THEN only the entry containing "50%" is returned
- AND the query does NOT match all rows (percent is treated as literal)

#### Scenario: LIKE query containing underscore literal

- GIVEN indexed outputs with commands "run_test" and "runXtest"
- AND the FTS query builder returns empty
- WHEN search_output is called with query "run_test"
- THEN only "run_test" matches
- AND "runXtest" does NOT match

### Requirement: Index Entry Schema

Each indexed output MUST include: id, tool_name, command, content, timestamp, truncated bool, exit_code. Fields `id`, `tool_name`, and `content` are REQUIRED — `IndexOutput` MUST return an error if any of them is empty.

(Previously: `IndexOutput` accepted empty values, inserting rows that violate the implicit contract; callers had no way to detect the programming error.)

#### Scenario: All fields populated (existing — unchanged)

- GIVEN shell_exec output with truncated=true
- WHEN indexed
- THEN entry has all required fields populated
- AND truncated field is true
- AND exit_code reflects actual process exit code

#### Scenario: IndexOutput rejects empty ID

- GIVEN a `ToolOutput` with `ID = ""`
- WHEN `IndexOutput` is called
- THEN it returns a non-nil error containing "ID"
- AND no row is inserted into the store

#### Scenario: IndexOutput rejects empty ToolName

- GIVEN a `ToolOutput` with `ToolName = ""`
- WHEN `IndexOutput` is called
- THEN it returns a non-nil error containing "ToolName"
- AND no row is inserted into the store

#### Scenario: IndexOutput rejects empty Content

- GIVEN a `ToolOutput` with `Content = ""`
- WHEN `IndexOutput` is called
- THEN it returns a non-nil error containing "Content"
- AND no row is inserted into the store

#### Scenario: IndexOutput succeeds with all required fields set

- GIVEN a `ToolOutput` with valid ID, ToolName, and Content
- WHEN `IndexOutput` is called
- THEN it returns nil
- AND the row is retrievable via `SearchOutputs`

### Requirement: Store Cleanup

The system SHOULD support configurable TTL-based cleanup of indexed outputs to prevent unbounded growth.

(Previously: unchanged — carried forward intact)

#### Scenario: Old entries cleaned up

- GIVEN indexed outputs older than retention period
- WHEN cleanup runs
- THEN entries older than TTL are removed

---

## ADDED Requirements (from subagents change)

### OUTPUT-STORE-REQ-5 — Migration v16: parent linkage and status on `conversations`

The system SHALL apply migration v16 which adds the following columns and index to the `conversations` table:

```sql
ALTER TABLE conversations ADD COLUMN parent_conv_id TEXT;
ALTER TABLE conversations ADD COLUMN status TEXT NOT NULL DEFAULT 'active';
CREATE INDEX idx_conv_parent ON conversations(parent_conv_id)
  WHERE parent_conv_id IS NOT NULL;
```

Existing rows backfill: `parent_conv_id` defaults to `NULL`; `status` defaults to `'active'` (handled by `DEFAULT` clause — no explicit UPDATE needed).

Valid `status` values in V1: `'active'`, `'running'`, `'completed'`, `'failed'`, `'cancelled'`. (`'cancelled'` is set by the boot-time orphan sweep and by explicit subagent cancellation.)

#### Scenario: migration v16 applies cleanly on existing DB

- GIVEN a database at migration version 10 with existing conversation rows
- WHEN migration v16 runs
- THEN the `conversations` table has `parent_conv_id` and `status` columns
- AND all existing rows have `status = 'active'` and `parent_conv_id = NULL`
- AND `idx_conv_parent` index exists

#### Scenario: migration v16 round-trip (up + down)

- GIVEN a database with migration v16 applied
- WHEN the down migration is run
- THEN the added columns and index are removed
- AND the table is valid at version 10

---

### OUTPUT-STORE-REQ-6 — Migration v17: attribution columns on `cost_records`

The system SHALL apply migration v17 which adds the following columns and indexes to the `cost_records` table:

```sql
ALTER TABLE cost_records ADD COLUMN conv_id TEXT;
ALTER TABLE cost_records ADD COLUMN parent_conv_id TEXT;
ALTER TABLE cost_records ADD COLUMN attribution_kind TEXT NOT NULL DEFAULT 'self';
UPDATE cost_records SET conv_id = session_id WHERE conv_id IS NULL;
CREATE INDEX idx_cost_conv ON cost_records(conv_id);
CREATE INDEX idx_cost_parent_conv ON cost_records(parent_conv_id)
  WHERE parent_conv_id IS NOT NULL;
```

Existing rows backfill: `conv_id` is set to `session_id`'s value (since `session_id` stores the conversation UUID by convention); `parent_conv_id` defaults to `NULL`; `attribution_kind` defaults to `'self'`.

#### Scenario: migration v17 applies and backfills conv_id

- GIVEN a database at migration v16 with existing cost_records (session_id populated)
- WHEN migration v17 runs
- THEN all existing cost_records have `conv_id = session_id`
- AND `attribution_kind = 'self'` on all existing rows
- AND `parent_conv_id = NULL` on all existing rows
- AND both new indexes exist

#### Scenario: migration v17 round-trip (up + down)

- GIVEN a database with migration v17 applied
- WHEN the down migration is run
- THEN the added columns and indexes are removed
- AND the table is valid at version 11

---

### OUTPUT-STORE-REQ-7 — New Store interface method: `ListChildConversations`

The `Store` interface SHALL expose:

```go
ListChildConversations(ctx context.Context, parentConvID string) ([]Conversation, error)
```

Returns all conversations whose `parent_conv_id` equals `parentConvID`, ordered by `created_at` ascending. Returns an empty slice (not an error) when no children exist.

#### Scenario: lists direct children of a parent conversation

- GIVEN principal conversation `"parent-abc"` has spawned two subagents (conv IDs `"child-1"`, `"child-2"`)
- WHEN `ListChildConversations(ctx, "parent-abc")` is called
- THEN the result contains exactly two `Conversation` entries with IDs `"child-1"` and `"child-2"`
- AND they are ordered by `created_at` ascending

#### Scenario: no children returns empty slice, not error

- GIVEN a conversation with no children
- WHEN `ListChildConversations(ctx, "lone-conv")` is called
- THEN the result is an empty slice
- AND error is nil

---

### OUTPUT-STORE-REQ-8 — New Store interface method: `CostSummaryForTree`

The `Store` interface SHALL expose:

```go
CostSummaryForTree(ctx context.Context, rootConvID string) (CostSummary, error)
```

`CostSummary` SHALL contain at minimum: `TotalUSD float64`, `TotalInputTokens int64`, `TotalOutputTokens int64`, `ConversationCount int`. The query MUST aggregate cost records for both the root conversation and all its direct children (one level). In V1 the tree depth is 1 (principal + direct subagents only, no recursive CTE needed).

#### Scenario: cost summary includes principal and all children

- GIVEN principal conv `"parent-abc"` with $0.10 in cost records
- AND child conv `"child-1"` with $0.05 in cost records
- AND child conv `"child-2"` with $0.03 in cost records
- WHEN `CostSummaryForTree(ctx, "parent-abc")` is called
- THEN `TotalUSD` equals 0.18 (within floating-point tolerance)
- AND `ConversationCount` equals 3

#### Scenario: root conv with no children returns own cost

- GIVEN a conversation `"solo-conv"` with $0.07 in cost records and no children
- WHEN `CostSummaryForTree(ctx, "solo-conv")` is called
- THEN `TotalUSD` equals 0.07
- AND `ConversationCount` equals 1

---

### OUTPUT-STORE-REQ-9 — New Store interface method: `SetConversationStatus`

The `Store` interface SHALL expose:

```go
SetConversationStatus(ctx context.Context, convID string, status string) error
```

Valid `status` values: `"active"`, `"running"`, `"completed"`, `"failed"`, `"cancelled"`. An invalid status value MUST return an error. A `convID` that does not exist MUST return an error.

#### Scenario: status updated to running on spawn

- GIVEN an existing conversation `"child-1"` with `status = "active"`
- WHEN `SetConversationStatus(ctx, "child-1", "running")` is called
- THEN the database row has `status = "running"`
- AND error is nil

#### Scenario: invalid status value returns error

- GIVEN an existing conversation
- WHEN `SetConversationStatus(ctx, id, "unknown_status")` is called
- THEN error is non-nil and references the invalid value
- AND the row is NOT updated

#### Scenario: non-existent conv returns error

- GIVEN no conversation with ID `"ghost-id"` exists
- WHEN `SetConversationStatus(ctx, "ghost-id", "completed")` is called
- THEN error is non-nil

---

### OUTPUT-STORE-REQ-10 — Compactor guard: skip `status = 'running'` conversations

The `ListCompactableConversations` query SHALL include an additional predicate `AND status != 'running'`. This prevents the compactor from selecting long-running subagent conversations that appear idle (i.e., `updated_at` is old) but are actively executing.

(Previously: the query had no status filter — any conversation with `updated_at < idleBefore AND compacted_at IS NULL AND deleted_at IS NULL` was eligible for compaction.)

#### Scenario: running subagent conversation excluded from compaction

- GIVEN a subagent conversation with `status = 'running'` and `updated_at` older than `idleBefore`
- WHEN `ListCompactableConversations` is called
- THEN the subagent conversation is NOT in the result set

#### Scenario: completed subagent conversation eligible for compaction

- GIVEN a subagent conversation with `status = 'completed'` and `updated_at` older than `idleBefore`
- WHEN `ListCompactableConversations` is called
- THEN the subagent conversation IS in the result set (eligible for normal compaction)

#### Scenario: active (principal) conversation compaction unaffected

- GIVEN a principal conversation with `status = 'active'` and `updated_at` older than `idleBefore`
- WHEN `ListCompactableConversations` is called
- THEN the principal conversation IS in the result set (behavior unchanged)
