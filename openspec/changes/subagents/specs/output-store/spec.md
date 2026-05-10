# Delta for Output Store (output-store)

## Overview

Adds two schema migrations (v16 and v17), three new `Store` interface methods, and a compactor query guard. Existing rows are backfilled with sensible defaults. All changes are additive and backward-compatible — existing code that does not read the new columns is unaffected.

---

## ADDED Requirements

### OUTPUT-STORE-REQ-5 — Migration v16: parent linkage and status on `conversations`

The system SHALL apply migration v16 which adds the following columns and index to the `conversations` table:

```sql
ALTER TABLE conversations ADD COLUMN parent_conv_id TEXT;
ALTER TABLE conversations ADD COLUMN status TEXT NOT NULL DEFAULT 'active';
CREATE INDEX idx_conv_parent ON conversations(parent_conv_id)
  WHERE parent_conv_id IS NOT NULL;
```

Existing rows backfill: `parent_conv_id` defaults to `NULL`; `status` defaults to `'active'` (handled by `DEFAULT` clause — no explicit UPDATE needed).

Valid `status` values in V1: `'active'`, `'running'`, `'completed'`, `'failed'`.

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

Valid `status` values: `"active"`, `"running"`, `"completed"`, `"failed"`. An invalid status value MUST return an error. A `convID` that does not exist MUST return an error.

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

---

## Backward Compatibility

- Existing code reading `store.Conversation` without `ParentConvID` or `Status` fields is unaffected — new struct fields have zero-value defaults (`""` and `""` respectively).
- `session_id` column on `cost_records` remains present and populated; `conv_id` is added alongside it. Queries using `session_id` continue to work.
- `attribution_kind` defaults to `"self"` on all existing rows — no semantic change to pre-existing cost records.
- The `idx_conv_parent` and `idx_cost_*` indexes are partial (conditional) — they do not affect reads/writes on rows with NULL values in the indexed columns.

---

## Acceptance Criteria

- [ ] Migration v16 adds `parent_conv_id` and `status` to `conversations` with correct defaults; round-trip tested.
- [ ] Migration v17 adds `conv_id`, `parent_conv_id`, `attribution_kind` to `cost_records`; backfills `conv_id = session_id`; round-trip tested.
- [ ] `ListChildConversations` returns direct children ordered by `created_at`; returns empty slice (not error) for parentless conversations.
- [ ] `CostSummaryForTree` aggregates principal + all direct children cost records.
- [ ] `SetConversationStatus` rejects invalid status strings and non-existent conv IDs.
- [ ] `ListCompactableConversations` excludes `status = 'running'` conversations; includes `status = 'completed'` and `status = 'active'`.
- [ ] All existing store tests remain green after migrations apply.
