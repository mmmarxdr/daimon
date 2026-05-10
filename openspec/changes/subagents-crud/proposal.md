# Proposal: subagents-crud

**Change**: `subagents-crud`
**Date**: 2026-05-10
**Author**: sdd-propose (Opus 4.7)
**artifact_store**: hybrid (engram topic `sdd/subagents-crud/proposal` + this file)
**Builds on**: `subagents` (PR #4/#5/#6, archived 2026-05-10)

---

## 1. Why

The `subagents` change shipped a fully working spawnable agent runtime — but every executable skill must today be authored as a `.skill.md` file on disk, manually listed in `cfg.Skills`, and the daimon process restarted to pick up changes. There is no way for an end-user to create, edit, or delete an agent from the UI; there is no curated catalog of starter templates; and REQ-12 forces every spawn to declare a `budget` block, which contradicts the user's mental model of *"just let me launch an agent — I'll add limits if I want them"*.

This change closes those gaps:

- **User-driven creation**: a CRUD REST surface backed by SQLite (`user_skills`, migration v18) so the frontend can create/edit/delete agent definitions without disk access or restarts.
- **Out-of-the-box value**: a small curated catalog of templates shipped with the binary (via `//go:embed`), so a fresh install has spawnable agents on day one.
- **Mental-model alignment**: budget becomes OPTIONAL — `nil = unlimited` at runtime; the UI surfaces a warning when the user opts in to no-cap. This is an explicit reversal of REQ-12.
- **Quality-of-life foundation**: a Phase-1 standalone slice ships the W4 follow-up (`Provider.Config()` interface, so cross-provider subagents inherit credentials cleanly) plus the long-missing `POST /api/subagents/{id}/cancel` endpoint.

Success looks like: a brand-new daimon install boots with 2-5 curated agents available; the user opens the frontend, creates a custom `code-reviewer` skill via a form, immediately spawns it through the parent agent without any restart, and can cancel it mid-run from the UI.

---

## 2. What Changes

Capability-level changes (file-level lives in §8):

- User-defined agents are persisted in SQLite (`user_skills` table, migration v18) with REST CRUD over `/api/skills`.
- A curated catalog of agent templates ships with the daimon binary via `//go:embed`.
- Loader unification: filesystem (`cfg.Skills`) + DB (`user_skills`) + curated (`embed.FS`) merge into a single source for `agent.New` with explicit precedence (DB > FS > Curated).
- Hot-reload of executable skills: every CRUD write reloads the full skill set without restarting the agent.
- Optional budget: parser no longer rejects executable skills that omit `budget`; runtime treats nil as unlimited; UI is responsible for the warning.
- Provider credential inheritance: new `ConfigurableProvider` interface so child agents pick up the parent's `ProviderConfig` cleanly across all five providers.
- REST endpoint to cancel a running subagent: `POST /api/subagents/{id}/cancel`.
- Critical runtime fix: `Spawn` no longer cancels a subagent instantly when `Budget.Timeout == 0` (today `context.WithTimeout(ctx, 0)` expires on the next scheduler yield).

---

## 3. Scope (In)

- Migration v18 — `user_skills` table + indexes (schema in exploration §3).
- `store.UserSkill` struct + `store.UserSkillStore` interface + sqlitestore implementation with table-driven CRUD tests.
- `internal/skill/loader_unified.go` — `LoadSkillsUnified(fsPaths, dbSkills, curatedFS, shellCfg, limits)` wrapper around existing `LoadSkills`.
- REST endpoints: `GET/POST/PUT/DELETE /api/skills`, `GET /api/skills/{name}`, `POST /api/subagents/{id}/cancel`.
- `agent.Agent.ReplaceExecutableSkills(defs)` — hot-reload spawn tools under `toolsMu` (re-runs `LoadSkillsUnified` after each CRUD write).
- `agent.Agent.CancelSubagent(id)` — delegates to `subMgr.Cancel`, nil-safe.
- `AgentReloader` interface extension to declare `ReplaceExecutableSkills`.
- `SubagentProvider` interface extension to declare `CancelSubagent`.
- New interface `ConfigurableProvider { Config() config.ProviderConfig }` (additive, opt-in — recommended over forcing into `Provider` to minimize blast radius).
- Skill schema change: `budget` becomes OPTIONAL. Parser drops the hard-error; `BudgetFrontmatter{}` zero value maps to `BudgetConfig{}` and is honored as "no limits".
- `Spawn` Timeout==0 fix: branch on `def.Budget.Timeout > 0` → `context.WithTimeout` else `context.WithCancel`.
- 2-5 initial curated templates shipped under `internal/skill/curated/` (e.g., `researcher`, `summarizer`, `code-reviewer`).
- Embedded curated FS via `//go:embed curated/*.md` inside `loader_unified.go`.
- Backward compatibility: every existing FS skill in `cfg.Skills` continues to load unchanged.

---

## 4. Scope (Out)

Per the user's "todo lo demás se puede ver de hacer o no" — explicitly deferred:

- **Recursive subagents** (depth > 1) — confirmed exclusion; REQ-9 in the canonical spec stays in force.
- **Multi-spawn `batch_id` grouping** — today batch_id is 1:1 with subagent_id; multi-spawn semantics deferred.
- **Soft warning for turn/timeout caps** — today only cost has the 80% soft warn; defer parity.
- **Sibling cancel** (cancel-N-by-batch_id) — defer.
- **Mid-turn budget gate** — would require provider streaming hooks; defer.
- **Per-sub MCP isolation** — subagents continue to share the parent's MCP tool set.
- **`attribution_kind = "advisor_call"` runtime path** — schema supports it; runtime keeps writing `"self"`.
- **API-level rate-limit coordination** between parent and children — defer.
- **Skill versioning history** — `version int` column exists but no `user_skill_history` table or revision viewer in V1.
- **Tags / categories** for the curated catalog — defer.
- **Export / import** (`.skill.md` round-trip from DB) — defer.
- **Marketplace / remote registry** — `SkillsRegistryURL` infra exists, but UI/CRUD over it is a separate change.
- **Frontend UI itself** — lives in the separate `daimon-frontend` repo and ships in its own PR.

---

## 5. Capabilities

> Contract for `sdd-spec`. Research before drafting deltas: `openspec/specs/subagents/spec.md`, `openspec/specs/output-store/spec.md`, `openspec/specs/config/spec.md`, `openspec/specs/agent-loop/spec.md`.

### New Capabilities
- **None**. All work extends existing capabilities.

### Modified Capabilities
- `subagents` — REQ-12 REVERSED (budget OPTIONAL); REQ-15 ADDED `POST /api/subagents/{id}/cancel`; NEW REQ for `Agent.CancelSubagent` semantics; NEW REQ for `Agent.ReplaceExecutableSkills` hot-reload semantics; NEW REQ for `Spawn` Timeout==0 → `context.WithCancel` branch (prevents instant cancel).
- `output-store` — Migration v18 (`user_skills` table + indexes); NEW `UserSkillStore` interface and methods (`ListUserSkills`, `GetUserSkill`, `CreateUserSkill`, `UpdateUserSkill`, `DeleteUserSkill`).
- `config` — Skill schema: `budget` is OPTIONAL; nil maps to "unlimited"; parser validation drops the hard-error for executable-without-budget. (`source` is DB-only — does not appear in `.skill.md` schema.)
- `agent-loop` — `LoadSkills` is wrapped (not replaced) by `LoadSkillsUnified`; `cmd/daimon/main.go` and `cmd/daimon/web_cmd.go` switch callers; `UserSkillStore` is injected into `ServerDeps`.

---

## 6. Approach

Six phases. Phase 1 is **orthogonal** and can ship as a standalone PR straight to `main`. Phases 2-5 form the core dependency chain. Phase 6 is a polish layer that depends on Phase 4.

### Phase 1 — Foundation (~230 LoC, standalone PR, no schema deps)
- W4: introduce `ConfigurableProvider interface { Config() config.ProviderConfig }` (additive, opt-in — smaller change than hoisting into `Provider`). Implement on all 5 concrete providers. `makeChildAgentFn` consumes it via type-assertion with graceful fallback to parent inheritance.
- Cancel endpoint: extend `SubagentProvider` with `CancelSubagent(id) error`; add `Agent.CancelSubagent` (nil-safe delegate to `subMgr.Cancel`); add `handleSubagentCancel` in `handler_subagents.go`; register `POST /api/subagents/{id}/cancel`.
- **Pre-Phase-1 task**: update `handler_subagents_test.go` mock to satisfy the extended `SubagentProvider` interface.
- **Testable**: cancel endpoint integration test (200 on running, 404 on unknown); provider Config round-trip test for all 5 providers.
- **Gates**: nothing.

### Phase 2 — Schema + Store (~200 LoC)
- `internal/store/migration.go`: `migrateV18()` adds `user_skills` table + 2 indexes.
- `internal/store/store.go`: `UserSkill` struct + `BudgetJSON` helper + `UserSkillStore` interface.
- `internal/store/sqlitestore.go`: implementation of all 5 methods.
- Table-driven tests covering CRUD + JSON round-trip for `tools_allowlist` (nil/[]/values) and `budget` (nil/zero/full).
- **Testable**: store unit tests pass; migration round-trips on a real DB without data loss.
- **Gates**: Phase 3.

### Phase 3 — REST CRUD + Hot-Reload (~350 LoC)
- `internal/web/handler_skills.go` (NEW): GET-list (`?source=user|curated|all`), GET-by-name, POST, PUT, DELETE.
- Validation at write time: name regex `^[a-z][a-z0-9_-]*$` ≤ 64 chars; prose/description ≤ 8 KB; `tools_allowlist` cross-checked against `s.deps.Tools`; `budget` (if present) requires ≥1 positive field.
- 403 on PUT/DELETE when `source = "curated"`.
- Routes registered in `server.go` (mutating routes wrapped with `requireOriginIfCrossOrigin`).
- `internal/agent/hot_reload.go`: `ReplaceExecutableSkills(defs)` — acquires `toolsMu`, drops all `*SubagentSpawnTool` entries, re-registers with `filterKnownTools`, lazy-inits `subMgr` if needed.
- `AgentReloader` interface extension declares `ReplaceExecutableSkills`.
- After every CRUD write the handler re-runs `LoadSkillsUnified` (full re-merge) and calls `Agent.ReplaceExecutableSkills` + `Agent.ReplaceSkills`.
- **Testable**: full REST round-trip; hot-reload verified by spawning a freshly-created skill without restart.
- **Gates**: Phase 4.

### Phase 4 — Loader Unification (~200 LoC)
- `internal/skill/loader_unified.go`: wrapper described in exploration §7.
- Inject `UserSkillStore` into `cmd/daimon/main.go` and `cmd/daimon/web_cmd.go`.
- DB skills now load on boot; precedence (DB > FS > Curated) applied; collisions logged.
- **Testable**: boot loads merged set from FS+DB; explicit collision test (same-name FS + DB → DB wins).
- **Gates**: Phase 5.

### Phase 5 — Budget Reversal + Timeout==0 Fix (~80 LoC)
- `internal/skill/parser.go`: remove the `fm.Budget.IsZero()` hard-error block (lines 258-261).
- `internal/agent/subagent_manager.go`: `Spawn` branches on `def.Budget.Timeout > 0` → `context.WithTimeout` else `context.WithCancel`.
- `subagents` spec REQ-12 text + scenarios updated by the `sdd-spec` phase.
- **Testable**: regression test — unlimited-budget executable skill loads, spawns, runs to natural completion (no instant cancel).
- **Gates**: Phase 6.

### Phase 6 — Curated Catalog (~300 LoC + 2-5 templates)
- `internal/skill/curated/` (NEW dir) with 2-5 `.skill.md` templates.
- `//go:embed curated/*.md` in `loader_unified.go`.
- `LoadSkillsUnified` walks the embedded FS and emits curated `SkillContent` + `ExecutableSkillDef` entries with `source = "curated"`.
- A user can shadow any curated skill by creating a `user_skill` with the same name (DB wins).
- **Testable**: curated load on boot; shadow test (create same-name user_skill → DB wins).

**Order rationale**: Phase 1 is fully orthogonal — ships first, no schema. Phases 2 → 3 → 4 → 5 form the strict chain (schema before REST; REST before loader unification because hot-reload needs CRUD; loader unification before budget reversal because the reversal must hold across all three sources). Phase 6 depends on the loader (Phase 4) being unified.

---

## 7. Extension Points (versioned)

| EP | V1 behavior | Future versions |
|---|---|---|
| **EP-1** UserSkill schema versioning | `version int` defaults to 1; no per-skill history table. | V2: append-only `user_skill_history` table with full revision log. |
| **EP-2** Curated catalog source | `embed.FS` baked into the binary; updates ship with releases. | V2: pull from remote registry via existing `SkillsRegistryURL` infra. |
| **EP-3** Loader precedence | DB > FS > Curated, hard-coded in `LoadSkillsUnified`. | V2: per-skill `priority` field; user can promote a curated skill above their own. |
| **EP-4** Provider Config interface | New `ConfigurableProvider interface { Config() config.ProviderConfig }`; not all providers must implement it (graceful type-assertion). | V2: hoist into the base `Provider` interface once all 5 providers ship `Config()`. |
| **EP-5** Hot-reload scope | `ReplaceExecutableSkills` re-loads ALL sources after every CRUD op (coarse). | V2: incremental update — only the changed skill's tool entry. |
| **EP-6** Budget validation | Backend accepts nil = unlimited; UI is responsible for the warning. | V2: optional admin-side default cap that overrides nil at runtime; per-installation policy. |

---

## 8. Affected Areas

| Package / File | Action | Phase |
|---|---|---|
| `internal/store/migration.go` | Add `migrateV18()` | 2 |
| `internal/store/store.go` | `UserSkill` struct + `UserSkillStore` interface + `BudgetJSON` helper | 2 |
| `internal/store/sqlitestore.go` | Implement `UserSkillStore` methods | 2 |
| `internal/skill/parser.go` | Remove budget hard-error block (lines 258-261) | 5 |
| `internal/skill/loader_unified.go` (NEW) | `LoadSkillsUnified` wrapper + `//go:embed` curated FS | 4, 6 |
| `internal/skill/curated/` (NEW dir) | 2-5 embedded `.skill.md` templates | 6 |
| `internal/agent/subagent_manager.go` | Fix `Timeout==0` → `context.WithCancel` branch in `Spawn` | 5 |
| `internal/agent/hot_reload.go` | Add `ReplaceExecutableSkills(defs)` | 3 |
| `internal/agent/agent.go` | Add `CancelSubagent(id)`; (interface extensions live in `web/server.go`) | 1 |
| `internal/web/server.go` | Extend `SubagentProvider` (CancelSubagent) + `AgentReloader` (ReplaceExecutableSkills); register new routes | 1, 3 |
| `internal/web/handler_subagents.go` | Add `handleSubagentCancel` | 1 |
| `internal/web/handler_subagents_test.go` | Update mock to satisfy extended `SubagentProvider` | 1 |
| `internal/web/handler_skills.go` (NEW) | REST CRUD handlers | 3 |
| `internal/llm/provider.go` (or per-provider files) | New `ConfigurableProvider` interface; implement `Config()` on all 5 providers | 1 |
| `cmd/daimon/main.go` | Switch to `LoadSkillsUnified`; inject `UserSkillStore` into `ServerDeps` | 4 |
| `cmd/daimon/web_cmd.go` | Same as `main.go` | 4 |
| `openspec/specs/subagents/spec.md` | REQ-12 reversal + REQ-15 cancel addition + new REQs | 5 (spec phase) |
| `openspec/specs/output-store/spec.md` | Add migration v18 + `UserSkillStore` interface section | 2 (spec phase) |
| `openspec/specs/config/spec.md` | Mark `budget` optional in skill schema | 5 (spec phase) |
| `openspec/specs/agent-loop/spec.md` | Document `LoadSkillsUnified` wrapper | 4 (spec phase) |

---

## 9. Risks

| Risk | Severity | Mitigation |
|---|---|---|
| `context.WithTimeout(ctx, 0)` cancels every unlimited-budget subagent instantly. | **CRITICAL** | Phase 5 branches `Spawn` on `Timeout > 0` → `context.WithTimeout`, else `context.WithCancel`. Regression test in Phase 5. |
| `ReplaceExecutableSkills` naive remove-all wipes FS-loaded spawn tools at boot. | **HIGH** | After every CRUD write, re-run full `LoadSkillsUnified` (FS + DB + curated) and pass the merged exec-def slice to `ReplaceExecutableSkills`. Documented in Phase 3 acceptance test. |
| FS + DB name collision is non-deterministic at startup. | **HIGH** | `LoadSkillsUnified` enforces explicit DB > FS > Curated precedence; logs collisions at WARN. |
| Budget reversal contradicts archived `subagents` spec REQ-12. | **HIGH** | `sdd-spec` phase explicitly rewrites REQ-12 + scenarios; proposal flags reversal upfront; CHANGELOG note in apply phase. |
| `SubagentProvider` interface change (adds `CancelSubagent`) breaks existing test mocks. | **MEDIUM** | Phase 1 includes the `handler_subagents_test.go` mock update as a pre-task. |
| `ConfigurableProvider` adoption is partial in V1 (W4) — cross-provider skills may still fall back. | **MEDIUM** | Acceptance criteria require all 5 providers implement `Config()` in Phase 1; type-assertion fallback keeps the path safe even if one regresses. |
| Allowlist write-time validation rejects tools added later via MCP hot-add. | **LOW** | Warn (not block) when a referenced tool is unknown at write time; document as advisory. |
| Curated `embed.FS` adds to binary size. | **LOW** | ~40 KB for 10 templates; negligible. |

---

## 10. Phasing

Per §6. The chain Phase 2 → 3 → 4 → 5 → 6 is strict; Phase 1 is orthogonal and can land first. Each phase ends with a green test slice and gates the next.

---

## 11. Dependencies

- **External**: none.
- **Internal**: Phase 1 standalone (no deps); Phase 3 depends on Phase 2 (needs `UserSkillStore`); Phase 4 depends on Phase 3 (needs hot-reload + handler wiring); Phase 5 depends on Phase 4 (reversal must hold across all three loader sources); Phase 6 depends on Phase 4 (curated path lives inside `LoadSkillsUnified`).

---

## 12. Acceptance Criteria

- [ ] User can `POST /api/skills` with `{name, prose, executable: true, budget: null}` and receive HTTP 201.
- [ ] A freshly created `user_skill` is immediately spawnable via `SubagentSpawnTool` with no daimon restart.
- [ ] `GET /api/skills` returns merged set (curated + user); `?source=user|curated|all` filters correctly.
- [ ] PUT or DELETE on a `source = "curated"` skill returns HTTP 403.
- [ ] User can shadow a curated skill by creating a `user_skill` with the same name (DB wins, GET returns the user version).
- [ ] A subagent spawned with `budget = nil` runs to natural completion (no instant cancel from `Timeout == 0`).
- [ ] `POST /api/subagents/{id}/cancel` cancels a running subagent within 1 second; returns 200 on success, 404 on unknown ID.
- [ ] All 5 providers implement `Config()` and return a non-zero `ProviderConfig`.
- [ ] Migration v18 applies cleanly on a real DB; round-trips without data loss; idempotent on re-run.
- [ ] Existing `cfg.Skills` FS skills load unchanged after Phase 4.
- [ ] `tools_allowlist` validation rejects unknown tool names at CRUD write time with HTTP 422 listing the bad names.
- [ ] After REQ-12 reversal, an executable skill `.skill.md` with no `budget` block loads without error.
- [ ] Curated catalog ships ≥ 2 templates; they appear in `GET /api/skills?source=curated` on a fresh install with no `cfg.Skills` entries.

---

## 13. Rollback Plan

- **Phase 1**: pure code revert. The `ConfigurableProvider` interface is opt-in (additive type-assertion in `makeChildAgentFn`), so removing it is a clean revert. Cancel endpoint removal is a route + handler delete.
- **Phase 2**: migration v18 is additive and safe to leave in place even if Phases 3-6 are reverted (inert table). SQLite column drops are non-trivial; easier to keep the table than drop it.
- **Phase 3**: code revert removes the routes + handler. Without Phase 4, DB skills were never loaded at boot, so no runtime behavior leaks.
- **Phase 4**: revert restores `LoadSkills` callers in `main.go` and `web_cmd.go`. The injected `UserSkillStore` becomes unused.
- **Phase 5**: revert restores the parser hard-error and the `WithTimeout(ctx, 0)` line. ⚠️ Reverting Phase 5 alone (without Phases 1-4) would leave the original spec contract honored.
- **Phase 6**: removing the `internal/skill/curated/` directory is a code change; no state to clean.

Worst case across all phases: leave migration v18 applied (inert table), unmount routes, restore old loader callers. No data loss path.

---

## 14. Resolved Decisions

The following decisions are binding (resolved by the user this session):

1. **Catalog model**: curated (shipped templates) + user-created. Both visible from UI; both editable from CRUD only when `source = "user"`.
2. **Persistence**: SQLite `user_skills` table via migration v18. Precedence: **DB > FS > Curated**.
3. **Default budget**: **unlimited** (REVERSAL of REQ-12). Backend accepts skills without a `budget` block; UI is responsible for warning the user when they opt in.
4. **Scope**: full SDD cycle — propose → spec → design → tasks → apply → verify → archive — as 6 implementation phases.
5. **No recursive subagents**: depth > 1 stays out (REQ-9 in canonical spec stands).
6. **Phase 1 foundation**: W4 (provider `Config()` interface) + cancel endpoint ship together as the orthogonal first PR.

No new open questions surfaced during proposal drafting.
