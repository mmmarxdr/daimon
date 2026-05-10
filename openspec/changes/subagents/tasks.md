# Tasks: Subagents (Spawnable Specialized Agent Loops)

**Change**: `subagents`
**Date**: 2026-05-10
**Dependencies**:
- Proposal: `openspec/changes/subagents/proposal.md`
- Specs: `openspec/changes/subagents/specs/{subagents,agent-loop,output-store,config}/spec.md`
- Design: `openspec/changes/subagents/design.md`
**TDD**: Strict TDD enabled — every IMPL task MUST be preceded by a failing [TEST] task.
**Migration note**: v16 = conversations (parent_conv_id + status); v17 = cost_records. NOT v11/v12.

---

## Review Workload Forecast

| Field | Value |
|-------|-------|
| Estimated changed lines | ~2 100–2 600 (Phase 1: ~650, Phase 2: ~950, Phase 3: ~350, Phase 4: ~300) |
| 400-line budget risk | **High** — every phase alone exceeds 400 lines |
| Chained PRs recommended | **Yes** |
| Suggested split | PR1 = Phase 1 · PR2 = Phase 2 · PR3 = Phase 3+4 |
| Delivery strategy | ask-on-risk |
| Chain strategy | pending (orchestrator must ask user before apply) |

Decision needed before apply: Yes
Chained PRs recommended: Yes
Chain strategy: pending
400-line budget risk: High

### Suggested Work Units

| Unit | Goal | Likely PR | Notes |
|------|------|-----------|-------|
| 1 | Schema + store extensions + SubagentChannel + skill parsing | PR 1 — base: main | Fully additive; existing tests must stay green |
| 2 | SubagentManager + SubagentSpawnTool + agent.New() wiring + events | PR 2 — base: PR 1 | Needs PR 1 schema; all runtime logic |
| 3 | REST/WS visibility + polish (80% warn, batch_id, edge cases) | PR 3 — base: PR 2 | Purely additive on top of PR 2 |

---

## Phase 1 — Foundation

> Goal: schema in DB, store methods, SubagentChannel, skill parsing. No user-visible behavior. Existing tests stay green.

### 1A — Migration v16 (conversations)

- [ ] 1.1 [TEST] `internal/store/migration_v16_test.go` — table-driven: (a) apply on fresh DB: `parent_conv_id` + `status` columns exist; (b) apply on v15 DB with existing rows: `status='active'` + `parent_conv_id=NULL` on all existing rows; (c) `idx_conv_parent` partial index exists; (d) round-trip up+down leaves table valid at v15. Satisfies OUTPUT-STORE-REQ-5.
- [ ] 1.2 [IMPL] `internal/store/migration.go` — add `migrateV16()` with exact SQL from design §2.4.1 (two ALTERs + two CREATE INDEX + `UPDATE schema_version SET version=16`); register in `initSchemaVersioned` under `if version < 16`.
- [ ] 1.3 [IMPL] `internal/store/migration.go` — add boot-time orphan sweep inside `initSchemaVersioned` after the v16 gate: `UPDATE conversations SET status='cancelled' WHERE status='running' AND updated_at < datetime('now','-24 hours')`. Satisfies §3.4 compactor guard note.

### 1B — Migration v17 (cost_records)

- [ ] 1.4 [TEST] `internal/store/migration_v17_test.go` — table-driven: (a) apply on v16 DB with existing cost_records: `conv_id=session_id` on all rows; (b) `attribution_kind='self'` on all rows; (c) `parent_conv_id=NULL`; (d) `idx_cost_conv` + `idx_cost_parent_conv` indexes exist; (e) round-trip up+down leaves table valid at v16. Satisfies OUTPUT-STORE-REQ-6.
- [ ] 1.5 [IMPL] `internal/store/migration.go` — add `migrateV17()` with exact SQL from design §2.4.2; register in `initSchemaVersioned` under `if version < 17`.

### 1C — Store struct/interface extensions

- [ ] 1.6 [TEST] `internal/store/store_structs_test.go` — `Conversation.ParentConvID` + `Status` JSON marshal/unmarshal round-trip; zero values produce `omitempty`-clean output. `CostRecord.ConvID` + `ParentConvID` + `AttributionKind` JSON round-trip.
- [ ] 1.7 [IMPL] `internal/store/store.go` — extend `Conversation` struct with `ParentConvID string json:"parent_conv_id,omitempty"` and `Status string json:"status,omitempty"`; extend `CostRecord` with `ConvID`, `ParentConvID`, `AttributionKind`. Add `CostSummary` struct. Satisfies OUTPUT-STORE-REQ-5/6.
- [ ] 1.8 [TEST] `internal/store/sqlitestore_subagent_test.go` — `ListChildConversations`: (a) returns 2 children ordered by `created_at`; (b) returns empty slice (not error) for parentless conv; (c) unknown parent returns empty slice. Satisfies OUTPUT-STORE-REQ-7.
- [ ] 1.9 [TEST] `internal/store/sqlitestore_subagent_test.go` — `CostSummaryForTree`: (a) root+2 children → `TotalUSD` sums all three; (b) root with no children → own cost only; (c) `ConversationCount` correct. Satisfies OUTPUT-STORE-REQ-8.
- [ ] 1.10 [TEST] `internal/store/sqlitestore_subagent_test.go` — `SetConversationStatus`: (a) `active→running` succeeds; (b) invalid value returns error; (c) non-existent convID returns error; (d) idempotent repeated call. Satisfies OUTPUT-STORE-REQ-9.
- [ ] 1.11 [IMPL] `internal/store/store.go` — add `ListChildConversations`, `SetConversationStatus` to `Store` interface; add `CostSummaryForTree` to `CostStore` interface. Add `ErrNotFound` sentinel if not already present.
- [ ] 1.12 [IMPL] `internal/store/sqlitestore.go` — implement `ListChildConversations` (SELECT by `parent_conv_id` ORDER BY `created_at`), `SetConversationStatus` (UPDATE + validate enum + ErrNotFound), `CostSummaryForTree` (recursive CTE from design §2.4.5). Update `SaveConversation` UPSERT + `LoadConversation` SELECT to include new columns. Update `RecordCost` INSERT to write `conv_id`, `parent_conv_id`, `attribution_kind`. Add FileStore stubs.
- [ ] 1.13 [TEST] `internal/store/compactor_status_guard_test.go` — `ListCompactableConversations`: (a) `status='running'` + old `updated_at` → NOT returned; (b) `status='completed'` + old `updated_at` → returned; (c) `status='active'` + old `updated_at` → returned (unchanged behavior). Satisfies OUTPUT-STORE-REQ-10.
- [ ] 1.14 [IMPL] `internal/store/sqlitestore.go` — add `AND status != 'running'` predicate to `ListCompactableConversations` query (~line 919 per design §2.4.3).

### 1D — Title generator guard

- [ ] 1.15 [TEST] `internal/agent/compactor_test.go` (or equivalent) — `shouldGenerateTitle` returns `false` when `conv.ParentConvID != ""`. Satisfies design §6 risk 5.
- [ ] 1.16 [IMPL] `internal/agent/compactor.go` (or wherever `shouldGenerateTitle` lives) — add guard: `if conv.ParentConvID != "" { return false }`. One-line change.

### 1E — SubagentChannel

- [ ] 1.17 [TEST] `internal/channel/subagent_test.go` — table-driven: (a) `NewSubagentChannel` + `Start` sets inbox; (b) `Deliver` pushes exactly one `IncomingMessage` with correct `ChannelID`; (c) `Deliver` before `Start` returns error; (d) `Send` appends to `output` and tracks `finalText`; (e) `Stop` is idempotent (second call no-ops); (f) `Outputs()` returns a defensive copy (mutation of returned slice does not affect internal state); (g) compile-time assertion `var _ Channel = (*SubagentChannel)(nil)` enforced.
- [ ] 1.18 [IMPL] `internal/channel/subagent.go` — implement `SubagentChannel` (~70 lines) per design §2.3: `Name`, `ID`, `Start`, `Deliver`, `Send`, `Stop`, `FinalAssistant`, `Outputs` methods.

### 1F — Skill schema parsing

- [ ] 1.19 [TEST] `internal/skill/parser_executable_test.go` — table-driven: (a) full frontmatter parses `Model`, `Provider`, `MaxCostUSD`, `MaxTurns`, `TimeoutMin`, `ToolsAllowlist`, `SystemAddendum`; (b) `budget: defaults` expands to 0.50/20/10; (c) `budget: random_value` → load error; (d) `executable: true` + no budget block → load error; (e) `version` defaults to 1 when absent; (f) `executable: false` → no `ExecutableSkillDef` produced; (g) `tools_allowlist: []` (empty) → valid, no filtering. Satisfies CONFIG-REQ-4, CONFIG-REQ-7, CONFIG-REQ-6.
- [ ] 1.20 [TEST] `internal/skill/loader_executable_test.go` — `LoadSkills`: (a) returns `ExecutableSkillDef` slice with one entry for `executable:true` skill; (b) non-executable skill in same load produces no def; (c) 4-return-value signature compiles; (d) existing call sites (update to 4-return) still produce same `SkillContent` slice as before. Satisfies CONFIG-REQ-8, CONFIG-REQ-4.
- [ ] 1.21 [IMPL] `internal/skill/skill.go` — extend `SkillContent` with new fields; add `BudgetFrontmatter` struct; add `ExecutableSkillDef` struct with `AgentConfig` + `ProviderCfg` methods per design §2.5.2.
- [ ] 1.22 [IMPL] `internal/skill/loader.go` — extend `frontmatter` struct for new YAML fields; implement `decodeBudget` helper; update `parseSkillContent` to produce `ExecutableSkillDef` for `executable:true`; update `LoadSkills` signature to return `([]SkillContent, map[string]tool.Tool, []ExecutableSkillDef, []error)`. Update `cmd/daimon` wiring for 4-return signature.

---

## Phase 2 — Core Runtime

> Goal: SubagentManager, SubagentSpawnTool, agent.New() wiring, lifecycle events. Spawn actually works end-to-end.

### 2A — Notify events

- [ ] 2.1 [TEST] `internal/notify/events_test.go` — (a) `EventSubagentSpawned/Completed/Failed` are distinct string constants; (b) all three are present in `KnownEventTypes`; (c) `notify.Event` with Meta map serializes all required fields (subagent_id, batch_id, skill, parent_conv_id, reason). Satisfies SUBAGENTS-REQ-10.
- [ ] 2.2 [IMPL] `internal/notify/events.go` — add 3 event constants + add to `KnownEventTypes` map per design §2.6.

### 2B — Provider model override regression

- [ ] 2.3 [TEST] `internal/provider/model_override_test.go` — `TestProviderModelOverride_AllTypes`: for each of anthropic, openai, openrouter, gemini, ollama: construct provider via `NewFromConfig` with `Model: "override-model"`; assert `provider.Model()` (or equivalent) returns `"override-model"`. Satisfies design §6 risk 6.
- [ ] 2.4 [IMPL] `internal/provider/` — fix any provider that silently ignores the `Model` field in `NewFromConfig`. (May be a no-op if all providers comply; test result determines scope.)

### 2C — SubagentManager

- [ ] 2.5 [TEST] `internal/agent/subagent_manager_test.go` — table-driven using `newChildAgent` test seam: (a) `Spawn` returns a non-nil handle with `ID` + `BatchID`; (b) `Spawn` calls `newChildAgent` exactly once; (c) `Spawn` writes conv row with `parent_conv_id` + `status='running'`; (d) `Active()` returns the new record with `status='running'`; (e) `Cancel(id)` is idempotent (second call no error). Satisfies SUBAGENTS-REQ-2, REQ-3.
- [ ] 2.6 [TEST] `internal/agent/subagent_manager_test.go` — depth guard: `Spawn` from a caller convID that is itself a sub → returns `ErrSubagentDepthExceeded`; parent conv remains unchanged. Satisfies SUBAGENTS-REQ-9.
- [ ] 2.7 [TEST] `internal/agent/subagent_manager_test.go` — budget enforcement (table: cost cap, turn cap, timeout each): after budget monitor receives `EventTurnCompleted` carrying usage exceeding the limit, `subRecord.status` flips to `"failed"`, `rec.cancel()` called, `EventSubagentFailed{reason:"budget_exceeded"}` emitted, `rec.done` closed. Satisfies SUBAGENTS-REQ-4, REQ-5.
- [ ] 2.8 [TEST] `internal/agent/subagent_manager_test.go` — soft warning at 80%: on first turn that crosses 80% cost threshold, `injectSoftWarning` called exactly once; `softWarned` flag prevents second injection. Satisfies SUBAGENTS-REQ-5 (80% scenario).
- [ ] 2.9 [TEST] `internal/agent/subagent_cancel_cascade_test.go` — spawn 3 subs, cancel parent ctx, assert all three `rec.done` channels close within 1s and `EventSubagentFailed{reason:"cancelled"}` emitted for each. Satisfies SUBAGENTS-REQ-6.
- [ ] 2.10 [TEST] `internal/store/sqlitestore_subagent_test.go` — `CostSummaryForTree` integration: parent + 2 children cost records → rollup sums correctly; `ConversationCount=3`. Satisfies OUTPUT-STORE-REQ-8.
- [ ] 2.11 [IMPL] `internal/agent/subagent_manager.go` — implement `SubagentManager`, `subRecord`, `BudgetConfig`, `SubagentResult`, `SubagentStatus`, `SubagentHandle` per design §2.1. Includes: `Spawn`, `Cancel`, `Active`, `All`, `Get`, `budgetMonitor`, `finalize`, `injectSoftWarning`, `filterParentTools`, `installBusSubscription` (cap-8 + drop+warn per-rec channel fan-out). Error sentinels: `ErrSubagentDepthExceeded`.

### 2D — SubagentSpawnTool

- [ ] 2.12 [TEST] `internal/agent/subagent_tool_test.go` — table-driven: (a) `Name()` returns skill name; (b) `Schema()` returns valid JSON with `prompt` required, `mode` enum `["sync","async"]`; (c) sync mode blocks then returns `SubagentResult` JSON with `status`, `cost_usd`, `turns`; (d) async mode returns `{handle_id, batch_id, status:"running"}` immediately without blocking; (e) empty prompt → `ToolResult{IsError:true}`; (f) wait error propagates without panic. Satisfies SUBAGENTS-REQ-7, REQ-8.
- [ ] 2.13 [IMPL] `internal/agent/subagent_tool.go` — implement `SubagentSpawnTool` per design §2.2: `Name`, `Description`, `Schema`, `Execute` (sync + async paths). Satisfies SUBAGENTS-REQ-1, REQ-8.

### 2E — agent.New() wiring

- [ ] 2.14 [TEST] `internal/agent/agent_wiring_test.go` — (a) `agent.New` with 2 `ExecutableSkillDef` entries produces `a.tools["researcher"]` of type `*SubagentSpawnTool` + `a.tools["summarizer"]`; (b) `a.subMgr` is non-nil; (c) `agent.New` with empty `[]ExecutableSkillDef` produces no spawn tools + `a.subMgr` nil or no-op; (d) `tools_allowlist` cross-validation: unknown name in allowlist → logged warn, entry dropped from child tool map (non-fatal). Satisfies AGENT-LOOP-REQ-5, SUBAGENTS-REQ-1, SUBAGENTS-REQ-11 (two-phase).
- [ ] 2.15 [TEST] `internal/agent/agent_wiring_test.go` — principal `sem` unaffected while subagent goroutine runs (verify `sem` cap unchanged after spawn). Satisfies AGENT-LOOP-REQ-6.
- [ ] 2.16 [IMPL] `internal/agent/agent.go` — add `execSkills []skill.ExecutableSkillDef` parameter to `New`; add `subMgr *SubagentManager` field to `Agent` struct; wire `NewSubagentManager` + spawn tool registration + `filterKnownTools` (drop+warn) per design §2.5.4. Expose `SubagentManager()` accessor. Update `cmd/daimon` wiring.
- [ ] 2.17 [TEST] `internal/agent/subagent_integration_test.go` — end-to-end: load `testdata/skills/researcher.skill.md`, call `agent.New`, drive a turn that produces a tool call to `researcher`, assert: (a) child Agent ran; (b) parent received `<tool_result>` containing `SubagentResult{Status:"completed"}`; (c) `EventSubagentSpawned` then `EventSubagentCompleted` emitted in order; (d) child conv has `parent_conv_id=parent.ID` + `status='completed'`; (e) parent MCP tools NOT present in child's tool map (allowlist filtering). Satisfies SUBAGENTS-REQ-2,3,10,14.
- [ ] 2.18 [TEST] `internal/agent/subagent_budget_test.go` — load `testdata/skills/budget_low.skill.md` (max_cost_usd:0.0001); assert exactly one `EventSubagentFailed{reason:"budget_exceeded"}` and conv marked `'failed'`. Satisfies SUBAGENTS-REQ-5, REQ-13.
- [ ] 2.19 [IMPL] `testdata/skills/researcher.skill.md` — full executable skill fixture (model: stub provider, budget: defaults, tools_allowlist: [`shell_exec`]).
- [ ] 2.20 [IMPL] `testdata/skills/budget_low.skill.md` — budget `max_cost_usd:0.0001`, `max_turns:1`, `timeout_min:1`.
- [ ] 2.21 [IMPL] `testdata/skills/noallowlist.skill.md` — empty `tools_allowlist`; used to verify parent MCP tools not leaked.
- [ ] 2.22 [IMPL] `testdata/skills/nonexecutable.skill.md` — pure prose skill; sanity that loader still treats it as non-executable.

---

## Phase 3 — Visibility

> Goal: REST + WS endpoints exposing live subagent state. Additive on top of Phase 2 runtime.

### 3A — REST endpoint

- [ ] 3.1 [TEST] `internal/web/handler_subagents_test.go` — `GET /api/subagents/active`: (a) 2 live subs → response array with `id`, `skill`, `status:"running"`, `accumulated_cost_usd`, `turn_count`; (b) no active subs → `{"subagents":[]}`; (c) nil `subMgr` (no executable skills) → `{"subagents":[]}`; (d) response is valid JSON. Satisfies SUBAGENTS-REQ-15.
- [ ] 3.2 [IMPL] `internal/web/handler_subagents.go` — `handleSubagentsActive` + `handleSubagentByID` per design §2.7; register routes in `internal/web/server.go`.

### 3B — WS event stream

- [ ] 3.3 [TEST] `internal/web/handler_subagents_test.go` — WS feed: subscriber receives `subagent.spawned` frame then `subagent.completed` frame in correct order with correct payloads. Satisfies SUBAGENTS-REQ-10.
- [ ] 3.4 [TEST] `internal/web/handler_subagents_test.go` — bus fan-out: 10 WS subscribers; one slow consumer (blocked write) does not block others; warning logged for slow consumer. Satisfies design §6 risk 3 (cap-8 + drop+warn).
- [ ] 3.5 [IMPL] `internal/web/handler_subagents.go` — WS endpoint: subscribe to bus on connect, filter `Type` prefix `agent.subagent.`, write JSON frames; cap-8 + drop+warn for slow consumers per design §3.2.

---

## Phase 4 — Polish

> Goal: 80% soft warning injection, batch_id, edge cases, coverage expansion.

### 4A — 80% soft warning injection

- [ ] 4.1 [TEST] `internal/agent/subagent_manager_test.go` — subagent at 80% budget receives synthetic user message on next turn; confirm `softWarned=true` prevents second injection (covered in 2.8 but add scenario: warning text content correctness). Satisfies SUBAGENTS-REQ-5 80% scenario.
- [ ] 4.2 [IMPL] `internal/agent/subagent_manager.go` — implement `injectSoftWarning` body: construct an `IncomingMessage` with warning text and deliver via `rec.subChannel.Deliver` (or push to child inbox). Satisfies SUBAGENTS-REQ-5.

### 4B — batch_id UUID per spawn group

- [ ] 4.3 [TEST] `internal/agent/subagent_manager_test.go` — multiple spawns in one logical group share `batch_id` (V1: same as id; document V2 note); separate spawn calls get different IDs. Stored in `conversations.metadata`. Satisfies SUBAGENTS-REQ-8 (batch_id in metadata).
- [ ] 4.4 [IMPL] `internal/agent/subagent_manager.go` — ensure `batchID` stored in `conversations.metadata` JSON at spawn time (V1: `batchID = id`). Satisfies design EP-2.

### 4C — Edge cases

- [ ] 4.5 [TEST] `internal/agent/subagent_integration_test.go` — race: parent ctx cancelled between conv insert and child `Run` start → `EventSubagentFailed{reason:"cancelled_during_spawn"}` emitted; no goroutine leak (use `goleak` or equivalent). Satisfies design §3.1 race note.
- [ ] 4.6 [TEST] `internal/agent/subagent_budget_test.go` — provider 429 retry interaction with timeout: sub with `timeout_min:1` and a provider that returns 429 + backoff > 60s → timeout wins; `EventSubagentFailed{reason:"budget_exceeded"}` (timeout) not a provider_error. Satisfies proposal §Phase 4 scope.
- [ ] 4.7 [TEST] `internal/agent/subagent_integration_test.go` — tool_result injection shape (REQ-8): `SubagentResult` JSON in parent conv includes `subagent_id` + `batch_id` in metadata. Satisfies SUBAGENTS-REQ-8.

---

## Deferred Items (NOT in this change)

- Sibling cancel (one sub cancelling another sub).
- Mid-turn budget gate (requires provider streaming hooks).
- API-level rate-limit coordination across parallel subs sharing a key.
- Per-sub MCP isolation (own MCP subprocess per spawn).
- Frontend subagent panel UI — separate PR in `daimon-frontend`.
- Recursive spawning (depth > 1).
- `attribution_kind = "advisor_call"` runtime (schema present; no runtime path).
- Persistent in-memory budget tracking across restarts.
- Hot-reload of executable skill changes (EP-5 V2).
- `SubagentHandle.pause() / resume() / replay()` (EP-7 V2).
- `on_progress` / `on_budget_warning` lifecycle events (EP-3 V2).
- V2 batch grouping in cost rollups and frontend panel (EP-2).

---

## Test Coverage Summary

**Total test tasks**: 29 [TEST] tasks
**Total impl tasks**: 24 [IMPL] tasks
**Ratio**: ~1.2 tests per impl task

### REQ → Test Task Mapping

| REQ | Test Task(s) |
|-----|-------------|
| SUBAGENTS-REQ-1 | 1.19, 1.20, 2.14 |
| SUBAGENTS-REQ-2 | 2.5, 2.15, 2.17 |
| SUBAGENTS-REQ-3 | 2.5, 2.17 |
| SUBAGENTS-REQ-4 | 2.7 |
| SUBAGENTS-REQ-5 | 2.7, 2.8, 4.1 |
| SUBAGENTS-REQ-6 | 2.9 |
| SUBAGENTS-REQ-7 | 2.12 |
| SUBAGENTS-REQ-8 | 2.12, 4.3, 4.7 |
| SUBAGENTS-REQ-9 | 2.6 |
| SUBAGENTS-REQ-10 | 2.1, 3.3, 2.17 |
| SUBAGENTS-REQ-11 | 1.19, 2.14 |
| SUBAGENTS-REQ-12 | 1.19, 1.20 |
| SUBAGENTS-REQ-13 | 2.18 |
| SUBAGENTS-REQ-14 | 2.17, 2.14 |
| SUBAGENTS-REQ-15 | 3.1 |
| AGENT-LOOP-REQ-5 | 2.14 |
| AGENT-LOOP-REQ-6 | 2.15, 2.9 |
| OUTPUT-STORE-REQ-5 | 1.1, 1.2, 1.6, 1.7 |
| OUTPUT-STORE-REQ-6 | 1.4, 1.5, 1.6, 1.7 |
| OUTPUT-STORE-REQ-7 | 1.8 |
| OUTPUT-STORE-REQ-8 | 1.9, 2.10 |
| OUTPUT-STORE-REQ-9 | 1.10 |
| OUTPUT-STORE-REQ-10 | 1.13 |
| CONFIG-REQ-4 | 1.19, 1.20 |
| CONFIG-REQ-5 | 1.19 |
| CONFIG-REQ-6 | 1.19 |
| CONFIG-REQ-7 | 1.19 |
| CONFIG-REQ-8 | 1.20 |
