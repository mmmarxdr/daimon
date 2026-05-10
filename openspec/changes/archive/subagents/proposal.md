# Proposal: Subagents (Spawnable Specialized Agent Loops)

**Change**: `subagents`
**Date**: 2026-05-10
**Author**: sdd-propose (Opus 4.7)
**artifact_store**: hybrid

---

## Why

Today the principal agent loop carries every concern — research, planning, execution, summarization — inside one conversation. As task complexity grows, the principal's context bloats with raw exploration output, tool results, and dead-end branches. There is no way to delegate a bounded sub-task ("research X and report back") to a specialized worker that uses its own model, its own tool subset, and its own budget, then returns a clean digest.

This change introduces **subagents**: declaratively-defined, spawnable agent loops triggered by synthetic tools auto-registered from `executable: true` skill files. Subagents enable parallel specialized work units (e.g., a `researcher` subagent burns 30 turns exploring on Haiku, returns a 200-token summary to the principal). The principal stays focused; the surface area of complex work expands without context bloat.

The exploration confirmed every required hook exists or is additively reachable — no breaking changes to `Agent`, `Provider`, `Tool`, or `Channel` interfaces. The architectural risk surface is small (6 packages, 2 migrations) and the value-per-line is high.

---

## What Changes

Capability-level changes (NOT file-level — see Affected Areas table for that):

- **Subagent spawn lifecycle**: synchronous `wait()` and asynchronous `handle.subscribe()` modes via a `SubagentManager` that owns goroutine + ctx + budget polling per spawn.
- **Per-spawn provider/model selection**: each subagent calls `provider.NewFromConfig` with its own profile config; principal's provider untouched.
- **Budget enforcement at turn-granularity**: cost USD, turn count, and timeout caps enforced after each `EventTurnCompleted`; soft warning at 80%, hard cancel at 100%.
- **Hierarchical cost attribution**: parent ↔ child rollup via `parent_conv_id` + `attribution_kind` columns; new `CostSummaryForTree(rootConvID)` store method.
- **Skill-driven subagent definitions**: `.skill.md` files gain an `executable: true` frontmatter flag; loader produces `ExecutableSkillDef` slice; agent.New() injects `SubagentSpawnTool` per definition.
- **Real-time visibility**: REST `GET /api/subagents/active` + WS event stream (`EventSubagentSpawned/Completed/Failed`) for the frontend panel.
- **Cancellation hierarchy**: parent ctx cancel cascades to all children; `SubagentManager.CancelSub(id)` cancels a single child without touching the parent.
- **Compactor guard**: new `status` column on `conversations` prevents the compactor from eating long-running subagent conversations.

---

## Scope

### In Scope (MVP / V1)

- Two store migrations: **v16** (`parent_conv_id`, `status` on `conversations` + index + compactor guard) and **v17** (`conv_id`, `parent_conv_id`, `attribution_kind` on `cost_records` + indexes + backfill).
- New `SubagentManager` owning spawn / lifecycle / budget / cancel.
- New `SubagentSpawnTool` (one per executable skill) implementing `tool.Tool`.
- New `SubagentChannel` headless channel (no Slack/web/CLI surface) — required by `agent.New()` signature.
- Skill schema additions: `executable`, `model`, `provider`, `system_prompt_addendum`, `tools_allowlist`, `budget { max_cost_usd, max_turns, timeout_min }`, `version` (default 1).
- `attribution_kind` always written as `"self"` in V1 (schema supports `"advisor_call"` / `"shared_resource"` for future).
- **Share-and-filter MCP model**: subagents inherit the parent's already-materialized MCP tools, filtered by `tools_allowlist`. No per-sub MCP subprocesses.
- **Turn-granularity budget enforcement** (post-turn check, not mid-turn).
- **3 lifecycle event hooks** wired to the existing `notify.Bus`: `on_spawn`, `on_complete`, `on_error`.
- **Recursive depth limit hard-coded to 1**: subagents cannot spawn subagents in V1.
- REST endpoint + WS event stream for active subagent visibility (backend only).
- `provider.NewFromConfig` reused as-is; no factory changes.

### Out of Scope (explicitly deferred)

- **Sibling cancel** (one sub cancelling another).
- **Mid-turn budget gate** (would require provider streaming hooks).
- **API-level rate-limit coordination** across parallel subs sharing a key (per-instance 429 backoff covers MVP).
- **Per-sub MCP isolation** (own MCP subprocess per spawn).
- **Frontend subagent panel UI** — separate PR in `daimon-frontend` repo, consuming the new backend endpoints.
- **Recursive spawning** (subagent-spawning-subagent) — hard depth limit = 1.
- **Advisor-call attribution flow** (`attribution_kind = "advisor_call"`) — schema-ready, no runtime path.
- **Persistent in-memory budget tracking across restarts** — running totals are in-memory only; restart loses them but DB cost records remain.

---

## Capabilities

### New Capabilities

- `subagents`: full lifecycle of spawnable specialized agent loops — declarative skill-based definitions, spawn manager, budget enforcement, cost attribution, cancellation hierarchy, lifecycle events. **This becomes `openspec/specs/subagents/spec.md`.**

### Modified Capabilities

- `agent-loop`: extends `agent.New()` wiring to register `SubagentSpawnTool` entries from executable skills; documents that each subagent runs an independent `Agent` instance with its own `inbox` + `sem` + `ctx`. Behavior of the principal's loop unchanged.
- `output-store`: extends `Conversation` with `parent_conv_id` + `status`; extends `CostRecord` with `conv_id` + `parent_conv_id` + `attribution_kind`; adds `ListChildConversations`, `CostSummaryForTree`, `SetConversationStatus` to the `Store` interface; compactor query gains `AND status != 'running'` guard.
- `config`: adds optional skill frontmatter fields (`executable`, `model`, `provider`, `system_prompt_addendum`, `tools_allowlist`, `budget`, `version`) — all backward-compatible defaults.

---

## Approach

Four sequential phases. Each phase ships a testable end-to-end slice and gates the next.

**Phase 1 — Foundation** (no user-visible behavior). Migrations v16 + v17, store struct/interface extensions, `SubagentChannel` skeleton, skill frontmatter parsing (fields parsed but ignored at runtime). Compactor guard active immediately. Testable: existing tests still green; new store methods unit-tested; round-trip migration tested up + down.

**Phase 2 — Core Runtime**. `SubagentManager` (spawn / cancel / budget poll / status), `SubagentSpawnTool`, `agent.New()` wiring to register synthetic tools, `notify.Bus` event types. Testable: spawn a `researcher` subagent end-to-end via a test skill, verify budget cancel, verify cancel cascade, verify cost rollup query.

**Phase 3 — Visibility**. REST `GET /api/subagents/active`, WS subagent event stream. Testable: integration test asserts events emitted in correct order with correct payloads.

**Phase 4 — Polish**. 80% soft-warning injection, `batch_id` UUID per spawn group, expanded table-driven test coverage, edge cases (parent ctx cancel during sub spawn, budget exceeded mid-turn, provider 429 retry interaction with timeout).

Order rationale: schema must land first (everything reads parent_conv_id / status). Runtime can land independently of visibility. Visibility is purely additive on top of runtime events. Polish is non-blocking quality work.

---

## Extension Points (versioned)

| EP | V1 behavior | Future versions can add |
|----|-------------|-------------------------|
| **EP-1** Profile schema versionable | Frontmatter `version: int` (default 1). V1 parses base fields. | V2 adds `advisor` block, V3 adds `collaboration` block; ignored if profile version < required. |
| **EP-2** `batch_id` UUID | Generated per spawn group, stored in `conversations.metadata`. | V2 surfaces batch grouping in the frontend panel and cost rollups. |
| **EP-3** Lifecycle event hooks | 3 hooks emitted on `notify.Bus`: `on_spawn`, `on_complete`, `on_error`. | V2 adds `on_progress`, `on_budget_warning`, `on_cancel_requested`. |
| **EP-4** Result schema | `{status, summary, artifacts, cost, errors, metadata}` returned by `wait()`. | V2 adds `tool_call_count`, `model_used`, `latency_ms` without breaking shape. |
| **EP-5** Synthetic tool injection | `SubagentSpawnTool` inserted into `a.tools` at `agent.New()`. | V2 adds hot-reload of executable skill changes via existing hot_reload path. |
| **EP-6** Cost `attribution_kind` | Schema present; V1 always writes `"self"`. | V2 introduces `"advisor_call"` flow; V3 adds `"shared_resource"` for MCP/tool cost attribution. |
| **EP-7** Spawn lifecycle API | `SubagentHandle { id, batch_id, wait(), cancel(), status(), subscribe() }`. | V2 adds `pause()`, `resume()`, `replay()`. |

---

## Affected Areas

| Area | Impact | Description |
|------|--------|-------------|
| `internal/store/migration.go` | Modified | Add migrations v16 (conversations) + v17 (cost_records) |
| `internal/store/store.go` | Modified | Extend Conversation + CostRecord structs; add 3 new Store methods |
| `internal/store/sqlitestore.go` | Modified | Implement ListChildConversations / CostSummaryForTree / SetConversationStatus |
| `internal/skill/skill.go` | Modified | Extend SkillContent with executable, model, provider, system_prompt_addendum, tools_allowlist, budget, version |
| `internal/skill/loader.go` | Modified | Parse new frontmatter; return ExecutableSkillDef slice |
| `internal/agent/subagent_manager.go` | New | Core spawn / lifecycle / budget / cancel machinery |
| `internal/agent/subagent_tool.go` | New | SubagentSpawnTool implementing tool.Tool |
| `internal/agent/agent.go` | Modified | Wire SubagentManager in New(); register synthetic tools |
| `internal/agent/compactor.go` | Modified | Add `AND status != 'running'` guard to compaction query |
| `internal/channel/subagent.go` | New | SubagentChannel headless implementation (~50 lines) |
| `internal/notify/events.go` | Modified | New constants: EventSubagentSpawned/Completed/Failed |
| `internal/web/handler_subagents.go` | New | REST + WS for active subagent panel feed |

---

## Risks

| Risk | Severity | Mitigation |
|------|----------|------------|
| Compactor eats live subagent conversations | HIGH | `status` column (migration v16) + WHERE guard in `ListCompactableConversations`. Phase 1 ships this first. |
| Missing `parent_conv_id` blocks all tree rollup features | HIGH | Migration v16 is critical-path foundation. No subagent code merges until it lands. |
| `cost_records.session_id` vs new `conv_id` naming creates query confusion | MEDIUM | v17 backfills `conv_id = session_id` on existing rows; both columns coexist; future cleanup migration documented as follow-up. |
| Share-and-filter MCP: misbehaving sub spams shared MCP server | MEDIUM (architectural debt) | Accepted for MVP. Documented in profile schema docs. V2 plan: per-sub MCP isolation. |
| N parallel subs share API key → independent 429 retries waste budget | MEDIUM | Per-instance backoff covers MVP. V2: shared rate-limit coordinator. Documented as known limitation. |
| `SubagentChannel` is brand-new code path — could hide bugs | LOW | ~50 lines, narrow interface, table-driven tests in Phase 1. |
| `IndexingWorker` per sub = N workers hitting SQLite WAL | LOW | WAL handles concurrent writers; 256-item buffer per worker. V2 may share a pool. |
| No persistent in-memory budget across restart | LOW | Cost records survive in DB; in-memory totals reset means a restarted sub gets full fresh budget. Acceptable since restart implies operator intervention. |
| Recursive subagent spawning loop | LOW | Hard depth limit = 1 in V1. Enforced in `SubagentManager.Spawn` (rejects spawn if caller is itself a subagent). |

---

## Phasing

**Phase 1 — Foundation**. Ships: migrations v16 + v17, store extensions, compactor guard, `SubagentChannel`, skill schema parsing. Why first: every other phase depends on schema. Testable end-to-end: existing test suite green; new store methods + migration round-trip tested.

**Phase 2 — Core Runtime**. Ships: `SubagentManager`, `SubagentSpawnTool`, `agent.New()` wiring, lifecycle events. Why second: needs schema to record parent_conv_id + attribution. Testable end-to-end: spawn a real `researcher` subagent from a real principal conversation; verify budget cancel, cancel cascade, cost rollup, lifecycle event emission.

**Phase 3 — Visibility**. Ships: REST `/api/subagents/active`, WS event stream. Why third: needs runtime events to exist. Testable end-to-end: integration test asserts API + WS contracts.

**Phase 4 — Polish**. Ships: 80% soft warning, `batch_id` UUID, broad test coverage. Why last: non-blocking quality gates. Testable end-to-end: edge-case suite passes.

---

## Dependencies

- **External**: none.
- **Internal**: Phase 1 (migrations + store extensions) blocks Phases 2–4. Frontend panel work is a separate PR in `daimon-frontend` and is not blocked by this change beyond Phase 3 backend endpoints landing.

---

## Rollback Plan

Each phase is independently revertable.

- **Phase 1 (migrations)**: migrations v16 + v17 are additive — column drops are non-trivial in SQLite but data integrity is preserved if columns are simply ignored. Rollback path: revert code; new columns become inert. If the `status` column must be removed, write reverse migration v16_down (table rebuild). Compactor guard reverts cleanly.
- **Phases 2–4**: pure code revert. No state migration needed beyond Phase 1.
- **Skill files** with `executable: true` are inert if the runtime does not register subagent tools — backward compatible.
- **Cost records** written with `attribution_kind = "self"` remain readable by code unaware of the column.

Worst case: revert all four phases, leave migrations v16 + v17 applied (inert columns). DB stays consistent; no user-visible regression.

---

## Success Criteria

- [ ] Spawning a `researcher` subagent does NOT block the principal's `for/select` loop.
- [ ] Compactor leaves subagent conversations untouched while `status = 'running'`; compacts them normally once status flips to `completed` / `failed`.
- [ ] `CostSummaryForTree(rootConvID)` returns parent + all children rollup with correct sum.
- [ ] Killing the parent ctx cascades cancellation to all live children within 1 second.
- [ ] Budget exceeded → subagent cancelled, parent receives `EventSubagentFailed{reason: "budget_exceeded"}`.
- [ ] Subagent attempting to spawn another subagent is rejected with a clear error.
- [ ] `GET /api/subagents/active` returns live spawns with status, accumulated cost, turn count.
- [ ] Existing skill files (without `executable: true`) load unchanged with no warnings.
- [ ] Migration v16 + v17 round-trip on a real DB without data loss.

---

## Resolved Decisions

Resolved by user on 2026-05-10 before spec phase:

1. **`tools_allowlist` matching semantics**: **exact names only** in V1. List is `[]string` of fully-qualified tool names (e.g. `["read_file", "mcp.github.search_code"]`). Validated at skill load time; unknown names → load error. Globs deferred to V2.

2. **Default budget caps**: **opt-in defaults**. Profile MUST declare a `budget` block. The block accepts either explicit values (`max_cost_usd`, `max_turns`, `timeout_min`) or the literal `budget: defaults` shortcut, which expands to floor: `max_cost_usd: 0.50`, `max_turns: 20`, `timeout_min: 10`. Profile with no `budget` key at all → load error (forces explicit intent). Logged at spawn time for visibility.

3. **Subagent output persistence shape**: **synthetic `tool_result` message** injected into the parent's conversation. The principal's next turn naturally reads the digest. Attribution is preserved via the new `parent_conv_id` link on the child conversation; the tool_result message itself records `subagent_id` + `batch_id` in its metadata for traceability.
