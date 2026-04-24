# Soft-Delete Pruner Specification

## Purpose

Defines the background goroutine that physically removes conversations soft-deleted longer than the configured retention window. Runs in-process, integrates with server startup/shutdown.

## ADDED Requirements

### Requirement: Pruner lifecycle

A new type `ConversationPruner` MUST live in `internal/store/pruner.go` with:

- `NewConversationPruner(store, clock, retention, interval) *ConversationPruner`
- `Start(ctx context.Context)` — launches the ticker goroutine
- `Stop()` — cancels the internal context, waits for the goroutine to exit (bounded — no explicit timeout; caller's `ctx` for Start is the upper bound)

The pruner is constructed and started in `internal/web/server.go`'s `Start()` flow alongside other server-lifetime goroutines.

#### Scenario: Pruner starts and stops cleanly

- GIVEN a running daimon web server
- WHEN the pruner is started via `server.Start()`
- THEN exactly one pruner goroutine is running
- AND server `Stop()` causes the pruner to exit within 100ms

### Requirement: Prune execution

On each tick the pruner MUST execute:

```sql
DELETE FROM conversations
WHERE deleted_at IS NOT NULL
  AND deleted_at < (NOW() - retention_days days)
```

(Equivalent logic via the store's SQL driver.)

The operation MUST emit `slog.Info` with:
- `event`: "pruner_run"
- `deleted_count`: number of rows affected
- `duration_ms`: wall time of the DELETE

#### Scenario: Prune removes expired soft-deleted convs

- GIVEN 3 convs with `deleted_at = now - 31d` and retention=30d
- WHEN the pruner ticks
- THEN all 3 rows are DELETE'd physically
- AND `slog.Info "pruner_run" deleted_count=3` is emitted

#### Scenario: Prune leaves recent soft-deleted convs alone

- GIVEN a conv with `deleted_at = now - 5d` and retention=30d
- WHEN the pruner ticks
- THEN the row is NOT deleted

#### Scenario: Prune leaves live convs alone

- GIVEN 10 live convs (`deleted_at IS NULL`)
- WHEN the pruner ticks
- THEN no rows are touched

### Requirement: Configuration

The pruner MUST read its behavior from `config.Conversations.Prune`:

| Key | Default | Range | Meaning |
|---|---|---|---|
| `retention_days` | 30 | 1..3650 | Days after soft-delete before physical removal |
| `interval_hours` | 6 | 1..168 | Ticker period |
| `enabled` | `true` | — | Master off switch |

Invalid values (out of range) MUST be clamped to the nearest valid bound and a `slog.Warn` emitted at startup.

#### Scenario: Out-of-range retention is clamped

- GIVEN `conversations.prune.retention_days = 5000` in config
- WHEN the pruner starts
- THEN effective retention is 3650 days
- AND `slog.Warn` notes the clamp

#### Scenario: Pruner disabled via config

- GIVEN `conversations.prune.enabled = false`
- WHEN the server starts
- THEN no pruner goroutine is launched
- AND a single `slog.Info "conversation_pruner_disabled"` is emitted at startup

### Requirement: Clock injection for tests

The pruner MUST accept a `Clock` interface (with `Now() time.Time`) so that tests can drive time deterministically without `time.Sleep`.

#### Scenario: Test with injected clock

- GIVEN a `FakeClock` starting at `T0`
- AND the pruner is configured with `retention_days=7`, `interval_hours=1`
- WHEN `clock.Advance(8 * 24 * time.Hour)` is called
- AND a manual `pruner.Tick()` is invoked (exposed for tests)
- THEN convs with `deleted_at < T0 + 1 day` are pruned

### Requirement: Error handling

Any error from the DELETE (SQL driver error, DB locked, etc) MUST be logged via `slog.Error` with the error and convID scope, but MUST NOT crash the goroutine or propagate upward. The pruner continues ticking on the next cycle.

#### Scenario: DB error during prune does not kill the goroutine

- GIVEN the DB is locked when the pruner ticks
- WHEN DELETE fails with "database is locked"
- THEN `slog.Error` logs the failure
- AND the pruner goroutine is still running
- AND the next tick attempts the DELETE again

## Non-requirements

- The pruner does NOT run on startup (first tick happens one interval after Start).
  - EXCEPTION: a `--prune-now` admin command MAY be added as follow-up, out of scope for v1.
- The pruner does NOT notify users of imminent pruning. 30-day window is documented but no UI warning surfaces.
- The pruner does NOT cascade to related tables (memory entries scoped to the conv remain). If needed in future, a separate migration/design addresses it.
- The pruner does NOT support per-conv retention overrides (e.g., "keep this conv forever").
