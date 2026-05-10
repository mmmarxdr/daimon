# Subagents — Exploration Report

**Change**: `subagents`
**Date**: 2026-05-08
**Author**: sdd-explore sub-agent (Claude Sonnet 4.6)
**artifact_store**: hybrid

---

## Executive Summary

The codebase supports the subagent design philosophically but requires concrete work across 6 layers: store schema (2 new migrations), a `SubagentChannel` headless channel, a `SubagentManager` to own spawn lifecycle and budget enforcement, provider-per-spawn factory wiring, compactor guard for in-flight subs, and WS/notify extensions for real-time visibility. No existing interface needs breaking changes — all additions are additive. The biggest architectural risk is that `Agent` today couples tightly to a single `channel.Channel` and a single `provider.Provider`, both set at construction time. Spawning a subagent means calling `agent.New(...)` with a synthetic channel and a provider instantiated from the sub's profile config — `provider.NewFromConfig` already exists for this. The `sem` semaphore in `Agent.Run` gates ALL concurrent turns; subagents must run their own agent loop (own goroutine, own sem) — not share the parent's. Schema migrations are the critical-path foundation.

---

## 1. Agent Loop — Spawn Model

**Files**: `internal/agent/agent.go`, `internal/agent/loop.go`

`Agent.Run` creates a buffered `inbox` channel (cap 100), starts background workers, then enters a `for/select` loop reading from `inbox`. For each message, it acquires `a.sem` (buffered channel of size `maxConcurrent`, default 4) and spawns a goroutine to call `processMessage`. The semaphore drains completely on `ctx.Done()`.

**Key finding**: A subagent CANNOT share the parent's `Agent` instance or its `sem`. Each subagent must be a fully independent `agent.New(...)` instance with:
- Its own `inbox` (created inside `Run`)
- Its own `sem` (sized by subagent profile budget, e.g. `max_concurrent: 1`)
- Its own `context.Context` derived from a parent-managed cancellable ctx
- Its own `store.Conversation` (new conv, separate ID)

The spawn model is: `agent.New(subCfg, subLimits, ..., subProvider, store, subChannel, subTools, subSkills, 1)` — then `go subAgent.Run(subCtx)`. The `SubagentManager` owns the goroutine + ctx cancel + budget polling.

**How a subagent tool triggers the loop**: The synthetic spawn tool (e.g. `researcher`) executes inside the parent's `processMessage` goroutine. It calls `SubagentManager.Spawn(...)`, which starts the sub goroutine and (for async mode) returns immediately with a handle. For sync (`wait()`), it blocks with a timeout.

**EP-7 (spawn lifecycle API)** is directly mappable: `Spawn() → handle{ id, batch_id, wait(), cancel(), status(), subscribe() }`.

---

## 2. Skill System — `executable: true` Path

**Files**: `internal/skill/skill.go`, `internal/skill/loader.go`, `internal/tool/skill_loader.go`

`SkillContent` today has: `Name`, `Description`, `Prose`, `Autoload`. No `executable` flag, no `budget`, no `tools_allowlist`, no `system_prompt_addendum`, no `model`/`provider`.

`LoadSkills` returns `([]SkillContent, map[string]tool.Tool, []error)`. The `tool.Tool` entries are `SkillShellTool` (shell subprocess), NOT spawn handles.

**Changes needed**:
1. Extend `skill.SkillContent` / YAML frontmatter with: `executable: bool`, `model: string`, `provider: string`, `system_prompt_addendum: string`, `tools_allowlist: []string`, `budget: { max_cost_usd, max_turns, timeout_min }`.
2. Extend `skill.LoadSkills` to detect `executable: true` and produce a `SubagentToolDef` instead of (or in addition to) `SkillShellTool`.
3. In `agent.New(...)` wiring, executable skills register synthetic tool entries into `a.tools` that dispatch to `SubagentManager.Spawn`.

**Backward compatibility**: existing non-executable skills are unaffected — `Autoload`, prose injection, `load_skill` tool all unchanged. Only new YAML keys are added.

**EP-1 (profile schema versionable)**: Add `version: int` to frontmatter with default 1. Fields `advisor`, `collaboration` parsed but ignored if `version < 2/3`.

---

## 3. Tool System — Synthetic Tool Registration

**Files**: `internal/tool/tool.go`, `internal/agent/context.go`, `internal/agent/hot_reload.go`

`tool.Tool` interface: `Name()`, `Description()`, `Schema()`, `Execute(ctx, params)`. Clean — nothing to change.

`buildToolDefs()` rebuilds `[]provider.ToolDefinition` per turn by ranging over `a.tools` (map guarded by `toolsMu RWMutex`). This means synthetic subagent tools are automatically picked up if inserted into `a.tools` at startup or via hot-reload.

`hot_reload.go` already demonstrates dynamic `a.tools` mutation (MCP hot-add uses the same pattern: `a.toolsMu.Lock(); a.tools[name] = t; a.toolsMu.Unlock()`).

**EP-5 (synthetic tool injection)**: Straightforward. Each executable skill produces a `SubagentSpawnTool{name: "researcher", ...}` implementing `tool.Tool`. Inserted into `a.tools` at `agent.New()` time. Zero architectural change needed for the registry path.

**`tool/registry.go`** contains `BuildRegistry`/`mergeTools` for wiring tools from config. Subagent tools can be merged there or injected post-construction via `RegisterTool` (same pattern as MCP hot-add).

---

## 4. Provider System — Multi-Provider per Spawn

**Files**: `internal/provider/factory.go`, `internal/provider/provider.go`, `internal/provider/fallback.go`

`provider.NewFromConfig(cfg config.ProviderConfig) (Provider, error)` is the single factory. All provider types (anthropic, gemini, openrouter, openai, ollama) are supported.

**Key finding**: Each provider instance is stateless w.r.t. rate limiting — rate limiting is implemented per-instance via HTTP 429 backoff retry loops inside each provider's `Chat` method. There is NO shared rate-limit state across instances. A FallbackProvider (`fallback.go`) chains multiple providers.

**Multi-provider per spawn**: Straightforward — each `agent.New` call gets its own `provider.NewFromConfig(subProfile.ProviderConfig)`. If the subagent profile specifies `model: claude-haiku-4-5` and `provider: anthropic`, the factory constructs an independent Anthropic client. The parent's provider is untouched.

**Shared API key risk**: If 5 parallel subs use the same Anthropic API key, they will independently hit 429s and each retry independently. No coordination exists today. For MVP this is acceptable (retry with backoff handles it), but explicit concurrent request tracking would be needed for V2 budget enforcement at the API level.

**Rate limiting**: Currently per-instance, per-call, HTTP-level retry. No proactive quota tracking exists.

---

## 5. Store — Schema Gaps for Subagents

**Files**: `internal/store/migration.go`, `internal/store/sqlitestore.go`, `internal/store/store.go`

### Current `conversations` table (base schema):
```sql
CREATE TABLE IF NOT EXISTS conversations (
    id         TEXT PRIMARY KEY,
    channel_id TEXT NOT NULL,
    messages   TEXT NOT NULL,
    metadata   TEXT,
    created_at DATETIME NOT NULL,
    updated_at DATETIME NOT NULL
    -- also: compacted_at, compacted_summary (added in later migration)
    -- also: deleted_at (soft delete, added in migration)
);
```

**Missing**: `parent_conv_id TEXT` — needed for parent→child linkage, tree rollup, cascade cancel visibility.

### Current `cost_records` table (migration v10):
```sql
CREATE TABLE IF NOT EXISTS cost_records (
    id              TEXT PRIMARY KEY,
    session_id      TEXT NOT NULL,   -- this IS conv.ID by convention
    channel_id      TEXT NOT NULL,
    model           TEXT NOT NULL,
    ...
    created_at      DATETIME NOT NULL
);
```

**Missing**: `conv_id TEXT` (explicit, not implicit via session_id), `parent_conv_id TEXT`, `attribution_kind TEXT` (EP-6, values: `"self"` | `"advisor_call"` | `"shared_resource"`).

### `store.Conversation` struct: No `ParentConvID` field.
### `store.CostRecord` struct: No `ConvID`, `ParentConvID`, `AttributionKind`.

**Required migrations** (two new versioned steps):

**Migration v11** — extend conversations:
```sql
ALTER TABLE conversations ADD COLUMN parent_conv_id TEXT;
ALTER TABLE conversations ADD COLUMN status TEXT NOT NULL DEFAULT 'active';
CREATE INDEX idx_conv_parent ON conversations(parent_conv_id) WHERE parent_conv_id IS NOT NULL;
```

**Migration v12** — extend cost_records:
```sql
ALTER TABLE cost_records ADD COLUMN conv_id TEXT;
ALTER TABLE cost_records ADD COLUMN parent_conv_id TEXT;
ALTER TABLE cost_records ADD COLUMN attribution_kind TEXT NOT NULL DEFAULT 'self';
UPDATE cost_records SET conv_id = session_id WHERE conv_id IS NULL;
CREATE INDEX idx_cost_conv ON cost_records(conv_id);
CREATE INDEX idx_cost_parent_conv ON cost_records(parent_conv_id) WHERE parent_conv_id IS NOT NULL;
```

**Store interface additions needed**:
- `ListChildConversations(ctx, parentConvID) ([]Conversation, error)`
- `CostSummaryForTree(ctx, rootConvID) (CostSummary, error)` — rolls up principal + all children
- `SetConversationStatus(ctx, convID, status string) error` — for in-flight tracking

---

## 6. Cost Store — Budget Enforcement & Attribution

**Files**: `internal/cost/cost.go`, `internal/store/store.go` (CostRecord)

`cost.ComputeCost(model, inputTokens, outputTokens)` returns `CostResult` — pure computation, no state. Cost is recorded to store by the agent loop turn-end path (`cs.RecordCost(ctx, store.CostRecord{SessionID: conv.ID, ...})`).

**Budget enforcement gap**: Today there is NO mid-turn cost check. Cost is recorded POST-TURN. Hard stops mid-task require a NEW mechanism:

**Option A (recommended)**: `SubagentManager` polls accumulated cost after each turn completion event (turn-level granularity). Uses `CostSummaryForTree` from new store method. At 80% → inject warning; at 100% → cancel subagent context.

**Option B** (aggressive): Estimate cost per-turn from token counts before saving, track in-memory running total per sub. More responsive but not persistent across restarts.

**EP-6 (`attribution_kind`)**: V1 always writes `"self"`. Schema already supports future `"advisor_call"` and `"shared_resource"` once those paths exist.

---

## 7. Compaction & Indexing Workers

**Files**: `internal/agent/compactor.go`, `internal/agent/indexing_worker.go`

### Compactor Risk — CRITICAL

`ListCompactableConversations` selects WHERE `updated_at < idleBefore AND compacted_at IS NULL AND deleted_at IS NULL`. A long-running subagent with `timeout: 30min` will have `updated_at` from spawn time — it WILL appear idle within the compaction window (default `IdleAfter` is configurable, typically minutes).

**Fix required**: Add `status` column to `conversations` (migration v11 above). Compactor adds `AND status != 'running'` to its query. `SubagentManager` sets `status = 'running'` at spawn and `status = 'completed'`/`'failed'` on finish.

### IndexingWorker Race

`IndexingWorker` has a single 256-item buffer per Agent. N parallel subagents each produce their own `IndexingWorker` (since each is a new `Agent` instance). No shared contention. However, all share the same SQLite DB — concurrent writes go through SQLite's WAL mode (presumed enabled) which serializes writers. Non-blocking drops (`slog.Warn`) apply per worker.

---

## 8. Web / WS Layer

**Files**: `internal/web/handler_conversations.go`, `internal/channel/web.go`, `internal/notify/`

The current model: one `WebChannel` per agent, one WS connection per browser tab. Each WS connection gets a `connID` (channel_id). Messages push to `inbox` with `ConversationID` override.

**Subagent panel requirements** (from design notes):
- Real-time status + cost per active subagent
- No direct user↔subagent messaging
- Principal synthesizes and presents output

**What needs to change**:
1. **notify/events.go**: Add `EventSubagentSpawned`, `EventSubagentCompleted`, `EventSubagentFailed`, `EventSubagentProgress` event types. `SubagentManager` emits these to `a.bus`.
2. **web handler**: New WS endpoint or existing `handler_ws_metrics.go` extension that streams subagent events to the frontend. Frontend subscribes to a `subagent_panel` feed.
3. **API endpoint**: `GET /api/subagents/active` — returns live spawns with status, cost-so-far, turn count.

**Cost real-time**: The `handler_metrics.go` path already serves cost summaries. Extending it to include `parent_conv_id` tree rollup requires only new store methods.

---

## 9. Budget Enforcement — Primitives

**Current state**: No real-time cost tracking per conversation. `RecordCost` is called after each LLM response (post-hoc). No pre-turn cost gate.

**MVP mechanism** (turn-granularity enforcement):

```
SubagentManager {
    runningCost map[subID]float64  // updated after each turn via event
    budget      BudgetConfig        // from profile
}

After each EventTurnCompleted for a subagent:
  runningCost[id] += turn.cost
  if runningCost[id] >= budget.MaxCostUSD * 0.8:
    inject soft warning into next turn
  if runningCost[id] >= budget.MaxCostUSD:
    cancel(subCtx)
```

Turn count: increment counter in `SubagentManager` per `EventTurnCompleted`, cancel at `budget.MaxTurns`.

Timeout: `context.WithTimeout(parentCtx, budget.TimeoutMin)` at spawn time — natural Go cancellation.

---

## 10. Cancellation

**Files**: `internal/agent/agent.go` (`Run` ctx drain), `internal/agent/loop.go` (`formatToolError`)

The `agent.Run` ctx cancellation path drains the semaphore by filling it (waits for in-flight turns). `context.Canceled` propagates cleanly to tool executions via `formatToolError`.

**Hierarchy**:
- `parentCtx` → `subCtx (WithCancel or WithTimeout)` per subagent
- Parent cancelled: all subCtx cancelled automatically (child of parent)
- User cancel (UI): cancel root ctx → all children cascade
- Child cancel: `SubagentManager.CancelSub(id)` calls `subCancel()` — does NOT touch parent ctx
- Sibling cancel: not supported in MVP (correct per design)

**No new primitives needed** — Go's `context.WithCancel` / `context.WithTimeout` gives the full hierarchy.

---

## 11. MCP Integration

**Files**: `internal/mcp/manager.go`, `internal/agent/hot_reload.go`

`mcp.Manager` holds `[]managedServer` at boot. Hot-add MCP servers go into `a.mcpClients map[string]interface{Close() error}` on the Agent.

**Sharing model for subagents**: Each subagent should inherit the PARENT's MCP tool names (filtered by `tools_allowlist`). Options:

- **Share parent's MCP connections** (recommended for MVP): Pass the parent's `map[string]tool.Tool` (MCP tools already materialized) filtered by profile `tools_allowlist`. Subagent gets a copy of the subset. No new MCP client connections.
- **Own MCP clients**: Spawn new `connectStdio/connectHTTP` per sub. More isolated but more expensive and process-heavy.

**Decision for MVP**: share-and-filter. The `tools_allowlist` in the profile drives which parent tools are copied into the subagent's `a.tools` map.

---

## 12. Skill Registry vs. Subagent Profile Fusion

**Current skill consumers**: `agent.New(skills []skill.SkillContent, skillIndex skill.SkillIndex)` injects skills into system prompt. `skill.LoadSkills` produces tools for non-executable skill shell tools.

**Fusion approach** (executable flag in same skill file):

- A `.skill.md` file with `executable: true` in frontmatter is BOTH a behavioral document (its prose is the subagent's system_prompt_addendum) AND a spawn-point definition.
- Non-executable skills: behavior unchanged.
- `LoadSkills` returns an additional `[]ExecutableSkillDef` for wiring.

**Backward-compat impact**: Zero — existing skill files have no `executable` key, default is `false`.

**Consumer that breaks**: None. The skill loader returns an extra slice; callers that don't consume it are unaffected.

---

## Architectural Mapping — Packages to Touch

| Package | Action | Reason |
|---------|--------|--------|
| `internal/store/migration.go` | Add v11, v12 | parent_conv_id, status, attribution_kind |
| `internal/store/store.go` | Extend structs + interfaces | Conversation.ParentConvID, CostRecord.ConvID/AttributionKind, new methods |
| `internal/store/sqlitestore.go` | Implement new methods | ListChildConversations, CostSummaryForTree, SetConversationStatus |
| `internal/skill/skill.go` | Extend SkillContent | executable, budget, model, provider, tools_allowlist, system_prompt_addendum |
| `internal/skill/loader.go` | Parse new fields | Return ExecutableSkillDef slice |
| `internal/agent/subagent_manager.go` | NEW | Core spawn/lifecycle/budget/cancel machinery |
| `internal/agent/subagent_tool.go` | NEW | SubagentSpawnTool implements tool.Tool |
| `internal/agent/agent.go` | Minor | Wire SubagentManager in New(), pass to spawn tools |
| `internal/notify/events.go` | Extend | New event type constants for subagent lifecycle |
| `internal/web/handler_subagents.go` | NEW | Active subagent REST + WS streaming |
| `internal/cost/cost.go` | No change | Pure computation, fine as-is |
| `internal/provider/` | No change | NewFromConfig already factory-ready |
| `internal/channel/` | Add SubagentChannel | Headless channel for subagent loops |
| `internal/mcp/` | No change | Sharing model via tools_allowlist filter |

### New Packages / Files
- `internal/agent/subagent_manager.go` — `SubagentManager`, `SubagentHandle`, `BudgetConfig`, spawn/cancel/status/wait
- `internal/agent/subagent_tool.go` — `SubagentSpawnTool` (one per executable skill)
- `internal/channel/subagent.go` — `SubagentChannel` (headless channel; `Start` stores inbox, `Stop` is no-op, `Send` routes to output collector)
- `internal/web/handler_subagents.go` — REST + WS for panel

---

## Extension Points Validation

| EP | Viable? | Notes |
|----|---------|-------|
| EP-1 Profile schema versionable | ✅ | Add `version` to SkillContent frontmatter. Fields ignored if version < requirement. |
| EP-2 batch_id UUID | ✅ | UUID field on SubagentHandle, stored in conversation metadata. |
| EP-3 Event hooks | ✅ | notify.Bus already exists. Add on_spawn/on_complete/on_error constants. V1 implements these 3. |
| EP-4 Result schema | ✅ | Output struct `{status, summary, artifacts, cost, errors, metadata}` defined in subagent_manager.go. |
| EP-5 Synthetic tool injection | ✅ | `buildToolDefs` rebuilds per-turn from `a.tools` RWMutex map. Insert SubagentSpawnTool at New() time. |
| EP-6 Cost attribution_kind | ✅ blocked by schema | Requires migration v12. V1 writes "self" always. |
| EP-7 Spawn lifecycle API | ✅ | `SubagentHandle` struct is exactly this API. |

---

## Obstacles — What the Plan Doesn't Account For

1. **Headless channel required**: `agent.New` requires a `channel.Channel`. Subagents have no Slack/web/CLI channel. A `SubagentChannel` implementation is needed — ~50 lines. Not mentioned in the plan.

2. **`CompactedAt` race for running subs**: Compactor WILL compact a long-running subagent if `updated_at` is old. Requires `status` column + compactor guard. Not in original plan.

3. **`CostRecord.session_id` vs `conv_id` naming confusion**: `session_id` is used in the code to hold `conv.ID` (a conversation UUID). The field name is misleading and will confuse future rollup queries. Migration should add explicit `conv_id` for clarity.

4. **No proactive API-level rate limit coordination**: N parallel subs with same key will independently retry. For MVP acceptable; surfaced for V2 planning.

5. **`IndexingWorker` per sub**: Each sub Agent creates its own `IndexingWorker`. With 5 concurrent subs, that's 5 workers hitting SQLite WAL concurrently. WAL handles this, but with bursts the 256-item buffers may drop silently. Consider raising buffer or sharing a pool in V2.

6. **MCP subprocess per hot-add**: If subs each launched their own MCP clients they'd spawn N subprocess copies. The share-and-filter model avoids this — document the constraint explicitly in the profile schema.

---

## Risks (from Code, Not Plan)

| Risk | Severity | Mitigation |
|------|----------|------------|
| Compactor eats live subagent convs | HIGH | `status` column + compactor WHERE guard (migration v11) |
| No `parent_conv_id` = no tree rollup | HIGH | migration v11 critical path |
| Cost CostRecord schema doesn't support attribution | MEDIUM | migration v12 |
| sem contention (parent vs subs) | LOW | Each sub has own Agent+sem — no sharing by design |
| Provider 429 N parallel subs | LOW | Per-instance backoff handles it for MVP |
| SubagentChannel missing | MEDIUM | New ~50-line file, clear interface to implement |
| `Conversation.metadata` as catch-all | LOW | Store batch_id, sub name etc. in metadata map — works but not queryable |

---

## Recommendations for Propose — Implementation Order

### Phase 1 — Foundation (no behavior yet)
1. **Migration v11**: `parent_conv_id`, `status` on `conversations`. Compactor guard.
2. **Migration v12**: `conv_id`, `parent_conv_id`, `attribution_kind` on `cost_records`.
3. **Extend store structs and interfaces**: `Conversation.ParentConvID`, `Conversation.Status`, `CostRecord.*`, new Store methods.
4. **`SubagentChannel`**: headless channel implementation.
5. **Extend `skill.SkillContent`**: new frontmatter fields parsed but ignored at runtime until Phase 2.

### Phase 2 — Core Runtime
6. **`SubagentManager`**: spawn, cancel, budget enforcement (turn-granularity), status tracking. EP-7 handle API.
7. **`SubagentSpawnTool`**: implements `tool.Tool`, dispatches to manager.
8. **Wire at `agent.New()`**: executable skills → SubagentSpawnTools inserted into `a.tools`.
9. **notify events**: `EventSubagentSpawned/Completed/Failed`.

### Phase 3 — Visibility
10. **`handler_subagents.go`**: REST endpoint for active subs + WS streaming.
11. **Frontend**: subagent panel (separate PR, daimon-frontend repo).
12. **Cost rollup**: `CostSummaryForTree` surfaced via API.

### Phase 4 — Polish
13. **Soft warnings**: 80% budget injection into next turn.
14. **batch_id**: UUID per spawn group (EP-2).
15. **Test coverage**: table-driven tests for SubagentManager lifecycle, budget enforcement, cancel cascade.

---

## Scope Estimate

| Category | Count |
|----------|-------|
| New files | 4–5 |
| Modified packages | 7 |
| New migration versions | 2 |
| New event types | 3 |
| New API endpoints | 1–2 |
| New tests required | ~15 table-driven |

**Total**: medium-large change. Recommend 4 SDD apply batches aligned to the 4 phases above.
