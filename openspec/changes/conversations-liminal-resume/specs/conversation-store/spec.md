# Conversation Store Specification

## Purpose

Defines soft-delete semantics for conversations and introduces a cursor-paginated read API over conversation messages. Covers schema migration v14 and all query adjustments.

## ADDED Requirements

### Requirement: Schema migration v14 — `deleted_at`

The SQLite store MUST add a `deleted_at TIMESTAMP NULL` column to the `conversations` table and a partial index.

```sql
ALTER TABLE conversations ADD COLUMN deleted_at TIMESTAMP NULL;
CREATE INDEX idx_conversations_deleted_at
    ON conversations(deleted_at) WHERE deleted_at IS NOT NULL;
```

The migration MUST be idempotent and versioned as v14 in the existing `schema_version` pattern.

#### Scenario: Migration upgrades a v13 DB to v14

- GIVEN a SQLite DB at schema_version = 13
- WHEN the store runs migrations at startup
- THEN `deleted_at` column exists on `conversations`
- AND the partial index exists
- AND `schema_version` = 14

#### Scenario: Migration is idempotent

- GIVEN a DB already at v14
- WHEN the store starts again
- THEN no error; schema_version remains 14
- AND no ALTER TABLE is executed a second time

### Requirement: Soft delete via `DeleteConversation`

`DeleteConversation(ctx, id)` MUST perform `UPDATE conversations SET deleted_at = NOW() WHERE id = ? AND deleted_at IS NULL`.

#### Scenario: Deleting a live conv marks it as soft-deleted

- GIVEN a conv with `deleted_at IS NULL`
- WHEN `DeleteConversation(ctx, "conv_x")` is called
- THEN the row's `deleted_at` is set to the current time
- AND the row is NOT removed from the table

#### Scenario: Deleting an already-deleted conv is a no-op

- GIVEN a conv with `deleted_at = <some past time>`
- WHEN `DeleteConversation(ctx, "conv_x")` is called
- THEN `deleted_at` remains unchanged (the earliest delete time wins)
- AND no error

### Requirement: Soft-delete filter on reads

`LoadConversation`, `ListConversationsPaginated`, and any new message-level read MUST filter `WHERE deleted_at IS NULL` by default.

#### Scenario: LoadConversation skips soft-deleted convs

- GIVEN a conv with `deleted_at` set
- WHEN `LoadConversation(ctx, id)` is called
- THEN `store.ErrNotFound` is returned

#### Scenario: ListConversationsPaginated skips soft-deleted convs

- GIVEN 5 live convs and 3 soft-deleted
- WHEN `ListConversationsPaginated(ctx, "", 20, 0)` runs
- THEN only the 5 live convs are returned
- AND `total` = 5

### Requirement: Restore endpoint behavior

A new `RestoreConversation(ctx, id) error` method on the store MUST clear `deleted_at` for a previously soft-deleted conv. It MUST return `ErrNotFound` if the conv does not exist OR if the conv is already live.

#### Scenario: Restoring a soft-deleted conv

- GIVEN a conv with `deleted_at = <past>`
- WHEN `RestoreConversation(ctx, id)` is called
- THEN `deleted_at` is set to NULL
- AND subsequent `LoadConversation(ctx, id)` returns the conv

#### Scenario: Restoring a non-soft-deleted conv is an error

- GIVEN a live conv OR no conv at all
- WHEN `RestoreConversation` is called
- THEN `store.ErrNotFound` is returned (no partial update)

### Requirement: Paginated message read

A new `GetConversationMessages(ctx, id, beforeIndex int, limit int) ([]provider.ChatMessage, hasMore bool, oldestIndex int, err error)` MUST return a window of messages from a single conversation without materializing the whole JSON blob into memory if the row is large.

- `beforeIndex = -1` means "load the most recent window".
- `limit` is clamped to `[1, 200]`; default when 0 = 50.
- `oldestIndex` in the result is the index of the earliest message returned (useful as the next cursor).
- `hasMore` is true when `oldestIndex > 0`.

#### Scenario: Initial load returns last 50 of 200

- GIVEN a conv with 200 messages
- WHEN `GetConversationMessages(ctx, id, -1, 50)` is called
- THEN messages at indexes 150..199 are returned (50 items)
- AND `oldestIndex = 150`
- AND `hasMore = true`

#### Scenario: Paging upward

- GIVEN previous call returned `oldestIndex = 150`
- WHEN `GetConversationMessages(ctx, id, 150, 50)` is called
- THEN messages at indexes 100..149 are returned
- AND `oldestIndex = 100`
- AND `hasMore = true`

#### Scenario: Reaching the start

- WHEN `GetConversationMessages(ctx, id, 50, 100)` is called on the same 200-message conv
- THEN messages at indexes 0..49 are returned (50 items, not 100 — bounded by start)
- AND `oldestIndex = 0`
- AND `hasMore = false`

#### Scenario: Soft-deleted conv is not readable via messages endpoint

- GIVEN a conv with `deleted_at` set
- WHEN `GetConversationMessages` is called
- THEN `store.ErrNotFound` is returned

### Requirement: Title metadata

`conversations.metadata` JSON MAY include a `"title"` key (1–100 runes, newlines stripped). The store does NOT enforce validation at the DB layer; validation is the caller's responsibility (web handler / title generator).

#### Scenario: Title round-trips through Save/Load

- GIVEN a conv with `metadata["title"] = "A great chat"`
- WHEN `SaveConversation` then `LoadConversation`
- THEN the loaded conv's `metadata["title"]` equals `"A great chat"`

## Non-requirements

- The store does NOT automatically delete soft-deleted conversations. That is the pruner's responsibility (spec: `soft-delete-pruner`).
- The store does NOT index or search on title content. Search is a frontend concern in v1.
- `GetConversationMessages` does NOT support forward paging (from oldest toward newest) in v1; it is cursor-based going backward only.
