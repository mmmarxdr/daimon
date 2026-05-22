# Tasks: subagents-crud

**Change**: `subagents-crud`
**Date**: 2026-05-10
**Author**: sdd-tasks (Sonnet 4.6)
**Artifact store**: hybrid (engram topic `sdd/subagents-crud/tasks` + this file)
**Depends on**: proposal `sdd/subagents-crud/proposal`, spec deltas (subagents/output-store/config/agent-loop), design `sdd/subagents-crud/design`
**Strict TDD**: ACTIVE — every IMPL task MUST be preceded by a paired [TEST] task. Write the failing test first.
**Delivery strategy**: `ask-on-risk` — orchestrator MUST present Review Workload Forecast before launching apply.

---

## Review Workload Forecast

| Field | Value |
|-------|-------|
| Estimated changed lines (nominal) | ~1,360 LoC implementation; ~2,700–3,400 with tests |
| 400-line budget risk | High |
| Chained PRs recommended | Yes |
| Suggested split | PR1 = Phase 1 (standalone vs main); PR2 = Phase 2+3; PR3 = Phase 4+5 (small Phase 5 bundles with 4); PR4 = Phase 6 |
| Delivery strategy | ask-on-risk |
| Chain strategy | pending — orchestrator MUST ask user before apply |

Decision needed before apply: Yes
Chained PRs recommended: Yes
Chain strategy: pending
400-line budget risk: High

### Suggested Work Units

| Unit | Goal | Likely PR | Base branch | Notes |
|------|------|-----------|-------------|-------|
| 1 | Foundation: ConfigurableProvider + cancel endpoint | PR1 | main | Orthogonal; no schema deps. Ships standalone. |
| 2 | Schema + Store + REST CRUD + Hot-Reload | PR2 | PR1 branch | Phases 2+3 bundled (~550 LoC nominal; ~1,100 with tests) — may need to split further if risk is High after estimation |
| 3 | Loader unification + Budget reversal + Timeout fix | PR3 | PR2 branch | Phases 4+5 bundled (~280 LoC) |
| 4 | Curated catalog | PR4 | PR3 branch | Phase 6 (~300 LoC + templates) |

---

## Phase 1 — Foundation (~230 LoC, standalone PR vs main)

> Pre-tasks run FIRST to prevent compile breakage when the SubagentProvider interface is extended.

### Pre-Tasks (compile-guard)

- [x] 1.0 [TEST] Catalog existing mock implementations of `SubagentProvider` in `internal/web/handler_subagents_test.go` — identify every struct that must gain `CancelSubagent`. (REQ-17, REQ-18)
- [x] 1.1 [IMPL] Add `CancelSubagent(id string) error` stub (returns `nil`) to `MockSubagentProvider` in `internal/web/handler_subagents_test.go` so the file continues to compile after the interface is extended. (REQ-17)

### W4 — ConfigurableProvider Interface

- [x] 1.2 [TEST] In `internal/provider/provider_configurable_test.go` (NEW): assert all 5 concrete providers satisfy `ConfigurableProvider`; assert returned `ProviderConfig` is non-zero after a provider is constructed with real cfg values. (REQ-20)
- [x] 1.3 [IMPL] Declare `ConfigurableProvider interface { Provider; Config() config.ProviderConfig }` in `internal/provider/provider.go` (or `factory.go`). (REQ-20)
- [x] 1.4 [IMPL] Add `Config() config.ProviderConfig` to `AnthropicProvider` in its provider file — returns the existing `p.config` field. (REQ-20)
- [x] 1.5 [IMPL] Add `Config()` to `OpenRouterProvider` — returns existing `p.config` field. (REQ-20)
- [x] 1.6 [IMPL] Add `Config()` to `GeminiProvider` — returns existing `p.config` field. (REQ-20)
- [x] 1.7 [IMPL] Add `Config()` to `OllamaProvider` — delegates to embedded `*OpenAIProvider.Config()` via method promotion (inherits automatically once 1.8 is done; verify no additional code needed). (REQ-20)
- [x] 1.8 [IMPL] Add `config config.ProviderConfig` field to `OpenAIProvider` struct; assign it in `NewOpenAIProvider`; add `Config() config.ProviderConfig` returning `p.config`. (REQ-20; design §2.7 — OpenAI is the ONLY provider missing this field)
- [x] 1.9 [TEST] In `internal/agent/agent_test.go` or `internal/agent/child_agent_test.go`: verify `makeChildAgentFn` type-asserts parent to `ConfigurableProvider` → child inherits creds; verify non-ConfigurableProvider parent falls back gracefully (no panic). (REQ-20)

### REST Cancel Endpoint

- [x] 1.10 [TEST] In `internal/agent/agent_cancel_test.go` (NEW): table-driven — nil `subMgr` → returns nil; non-nil `subMgr` with running sub-1 → delegates to `subMgr.Cancel`. (REQ-18)
- [x] 1.11 [IMPL] Add `func (a *Agent) CancelSubagent(id string) error` in `internal/agent/agent.go` — nil-safe delegate to `a.subMgr.Cancel(id)`. (REQ-18)
- [x] 1.12 [IMPL] Extend `SubagentProvider` interface in `internal/web/server.go` with `CancelSubagent(id string) error`. (REQ-17)
- [x] 1.13 [TEST] In `internal/web/handler_subagents_cancel_test.go` (NEW): table-driven — 204 success; 404 unknown id; 200 already-finished (`{"already_finished": true}`); 500 internal error. (REQ-17)
- [x] 1.14 [IMPL] Add `handleSubagentCancel` in `internal/web/handler_subagents.go` using `r.PathValue("id")` (Go 1.22+). Maps: nil error → 204; "not found" error → 404; already-finished sentinel → 200 + JSON body; other errors → 500. (REQ-17; design §2.9)
- [x] 1.15 [IMPL] Register route `POST /api/subagents/{id}/cancel` in `internal/web/server.go` route table, wrapped with `requireOriginIfCrossOrigin`. (REQ-17)
- [x] 1.16 [TEST] Integration test in `internal/web/integration_test.go` or new file: spawn subagent → `POST /api/subagents/{id}/cancel` → assert `EventSubagentFailed{reason:"cancelled"}` on bus within 1 second. (REQ-17)

---

## Phase 2 — Schema + Store (~200 LoC)

> Requires Phase 1 merged (or rebased). Gates Phase 3.

### Migration v18

- [x] 2.1 [TEST] In `internal/store/migration_v18_test.go` (NEW): table-driven — apply on v17 DB → `user_skills` table exists, row count 0, `schema_version`=18, both indexes present; apply again (idempotent) → no error, no duplicates. (OUTPUT-STORE-REQ-11)
- [x] 2.2 [IMPL] Implement `migrateV18()` in `internal/store/migration.go` — single tx, `CREATE TABLE IF NOT EXISTS user_skills(...)` + 2 indexes + `schema_version=18`. Register in `RunMigrations` with `if version < 18`. (OUTPUT-STORE-REQ-11; design §2.2)

### UserSkill Struct and Sentinels

- [x] 2.3 [TEST] In `internal/store/userskill_test.go` (NEW): JSON marshal/unmarshal — nil `Budget` → SQL NULL round-trip → nil; non-nil `Budget` round-trips with all values; nil `ToolsAllowlist` → SQL NULL round-trip → nil; `[]string{}` → JSON `"[]"` round-trip → empty non-nil slice. (OUTPUT-STORE-REQ-12)
- [x] 2.4 [IMPL] Add `UserSkill` struct + `BudgetJSON` struct in `internal/store/store.go`. Helpers `encodeAllowlist`, `decodeAllowlist`, `encodeBudget`, `decodeBudget` using `sql.NullString`. (OUTPUT-STORE-REQ-12; design §2.1 and §2.3)
- [x] 2.5 [IMPL] Add sentinel error `ErrNameConflict` in `internal/store/store.go`. (OUTPUT-STORE-REQ-12; design §2.1)

### UserSkillStore Interface + sqlitestore Implementation

- [x] 2.6 [TEST] `ListUserSkills`: empty DB → empty slice; populated DB → rows ordered by name ASC. (OUTPUT-STORE-REQ-12)
- [x] 2.7 [TEST] `GetUserSkill` by name: existing → returns matching row; missing → returns `ErrNotFound`. (OUTPUT-STORE-REQ-12)
- [x] 2.8 [TEST] `CreateUserSkill`: success + row matches input; UNIQUE name violation → `ErrNameConflict`. (OUTPUT-STORE-REQ-12)
- [x] 2.9 [TEST] `UpdateUserSkill`: existing row updated (`updated_at` advances, fields change); missing name → `ErrNotFound`. (OUTPUT-STORE-REQ-12)
- [x] 2.10 [TEST] `DeleteUserSkill`: removes row; subsequent `GetUserSkill` returns `ErrNotFound`. (OUTPUT-STORE-REQ-12)
- [x] 2.11 [TEST] Budget null round-trip: row stored with `Budget: nil` → retrieved `Budget` is nil (not zero-value struct). (OUTPUT-STORE-REQ-12)
- [x] 2.12 [TEST] `ToolsAllowlist` nil vs empty distinction: nil → SQL NULL → nil; `[]string{}` → `"[]"` → non-nil empty slice. (OUTPUT-STORE-REQ-12)
- [x] 2.13 [IMPL] Extend `Store` interface in `internal/store/store.go` with the 5 `UserSkillStore` methods. (OUTPUT-STORE-REQ-12)
- [x] 2.14 [IMPL] Implement all 5 methods in `internal/store/sqlitestore.go` — `QueryContext` for reads; `tx` wrapping for writes; SQLite UNIQUE error → `ErrNameConflict`; `sql.ErrNoRows` → `ErrNotFound`. (OUTPUT-STORE-REQ-12; design §2.3)
- [x] 2.15 [IMPL] Add no-op stubs for all 5 methods in `internal/store/filestore.go` so `FileStore` continues to satisfy `Store`. (OUTPUT-STORE-REQ-12)

### Mock Store Sweep

- [x] 2.16 [IMPL] Find all `type mockStore` (or equivalent) test fixtures across packages via `grep -r "mockStore\|MockStore" internal/`; add the 5 new method stubs to each. Confirm `go test ./...` compiles with no interface-satisfaction errors. (OUTPUT-STORE-REQ-12)

---

## Phase 3 — REST CRUD + Hot-Reload (~350 LoC)

> Requires Phase 2 merged. Gates Phase 4.

### Agent.ReplaceExecutableSkills

- [ ] 3.1 [TEST] `ReplaceExecutableSkills` removes old `*SubagentSpawnTool` entries and registers new defs; non-spawn tools in `a.tools` are untouched. (REQ-19)
- [ ] 3.2 [TEST] Lazy `subMgr` init: call with no prior `subMgr` and non-empty defs → `subMgr` is initialized. (REQ-19)
- [ ] 3.3 [TEST] Empty defs slice → all spawn tools removed; `subMgr` instance unchanged (not nilled out). (REQ-19)
- [ ] 3.4 [TEST] Unknown tool in `tools_allowlist` → dropped with `slog.Warn`, no error returned. (REQ-19; CONFIG-REQ-5 warn-not-block at hot-reload)
- [ ] 3.5 [IMPL] Add `func (a *Agent) ReplaceExecutableSkills(defs []skill.ExecutableSkillDef)` in `internal/agent/hot_reload.go` — (1) acquire `a.toolsMu.Lock`, (2) delete all `*SubagentSpawnTool` entries, (3) lazy-init `subMgr` if nil and `len(defs)>0`, (4) re-register with `filterKnownTools`. (REQ-19; design §2.5)

### AgentReloader Interface Extension

- [ ] 3.6 [IMPL] Extend `AgentReloader` interface in `internal/web/server.go` with `ReplaceExecutableSkills(defs []skill.ExecutableSkillDef)`. (REQ-19)
- [ ] 3.7 [IMPL] Find all test mocks implementing `AgentReloader` (via `grep -r "AgentReloader\|ReplaceSkills" internal/web/`); add `ReplaceExecutableSkills` stub to each mock. (REQ-19)

### REST CRUD Handlers

- [ ] 3.8 [TEST] `handleListSkills` in `internal/web/handler_skills_test.go` (NEW): `?source=user` returns only user rows; `?source=curated` returns only curated; `?source=all` / no param returns merged; response shape `{"skills": [...]}`. (CONFIG-REQ-9; AGENT-LOOP-REQ-8)
- [ ] 3.9 [TEST] `handleGetSkill`: existing name → 200 + skill body; missing name → 404. (AGENT-LOOP-REQ-8)
- [ ] 3.10 [TEST] `handleCreateSkill`: valid payload → 201 + `Location: /api/skills/{name}` header; name regex violation → 422; name UNIQUE conflict → 409; prose > 8 KB → 422; `tools_allowlist` with unknown tool → 422; malformed JSON → 400. (CONFIG-REQ-6; OUTPUT-STORE-REQ-12)
- [ ] 3.11 [TEST] `handleUpdateSkill`: `source=user` row → 200 + updated body; `source=curated` row → 403. (CONFIG-REQ-9)
- [ ] 3.12 [TEST] `handleDeleteSkill`: `source=user` row → 204; `source=curated` row → 403; missing name → 404. (CONFIG-REQ-9)
- [ ] 3.13 [TEST] `s.reloadSkills()` helper: re-runs `LoadSkillsUnified` + `Agent.ReplaceExecutableSkills` + `Agent.ReplaceSkills` in sequence; mock store + mock reloader verify each is called once per write. (AGENT-LOOP-REQ-8)
- [ ] 3.14 [TEST] Integration — allowlist TWO modes: (a) REST write with unknown tool → 422 hard error; (b) hot-reload with unknown tool in stored allowlist → warn-and-drop (no 422). (CONFIG-REQ-5; REQ-19)
- [ ] 3.15 [TEST] Integration: POST create skill → immediate spawn via `SubagentSpawnTool` succeeds (no restart). (AGENT-LOOP-REQ-8)
- [ ] 3.16 [TEST] Integration: DELETE skill → subsequent spawn attempt returns tool-not-found error. (AGENT-LOOP-REQ-8)
- [ ] 3.17 [IMPL] Create `internal/web/handler_skills.go` (NEW file). Implement `handleListSkills`. (CONFIG-REQ-9; AGENT-LOOP-REQ-8; design §2.8)
- [ ] 3.18 [IMPL] Implement `handleGetSkill` in `internal/web/handler_skills.go`. (AGENT-LOOP-REQ-8)
- [ ] 3.19 [IMPL] Implement `handleCreateSkill` with validation: name regex `^[a-z][a-z0-9_-]*$` ≤64 chars; prose ≤8 KB; `tools_allowlist` cross-check against `s.deps.Tools` (unknown → 422 at write time); budget if present requires ≥1 positive field. (CONFIG-REQ-6; CONFIG-REQ-9; OUTPUT-STORE-REQ-12; design §2.8)
- [ ] 3.20 [IMPL] Implement `handleUpdateSkill` with curated 403 guard. (CONFIG-REQ-9)
- [ ] 3.21 [IMPL] Implement `handleDeleteSkill` with curated 403 guard + missing 404. (CONFIG-REQ-9)
- [ ] 3.22 [IMPL] Implement `s.reloadSkills()` helper in `handler_skills.go` or `server.go`: `ListUserSkills` → `LoadSkillsUnified` → `ReplaceExecutableSkills` → `ReplaceSkills`. (AGENT-LOOP-REQ-8)
- [ ] 3.23 [IMPL] Register 5 skill routes in `internal/web/server.go` route table: `GET /api/skills`, `GET /api/skills/{name}`, `POST /api/skills`, `PUT /api/skills/{name}`, `DELETE /api/skills/{name}`. Mutating routes wrapped with `requireOriginIfCrossOrigin`. Also add `UserSkillStore` field to `ServerDeps`. (AGENT-LOOP-REQ-8; design §2.8)

---

## Phase 4 — Loader Unification (~200 LoC)

> Requires Phase 3 merged. Gates Phase 5 and Phase 6.

### LoadSkillsUnified

- [ ] 4.1 [TEST] In `internal/skill/loader_unified_test.go` (NEW): empty inputs (empty curatedFS, empty fsPaths, empty dbSkills) → empty slices, nil errors. (AGENT-LOOP-REQ-7)
- [ ] 4.2 [TEST] Source isolation: only curated → returns curated only; only FS → returns FS only; only DB → returns DB only. (AGENT-LOOP-REQ-7)
- [ ] 4.3 [TEST] Precedence — name collision: FS vs Curated → FS wins; DB vs FS → DB wins; DB vs Curated → DB wins. (AGENT-LOOP-REQ-7)
- [ ] 4.4 [TEST] Collision logging: name collision → `slog.Warn` emitted with `"name"`, `"winner"`, `"loser"` keys. (AGENT-LOOP-REQ-7)
- [ ] 4.5 [TEST] Tools map merges correctly: non-conflicting names from all three sources all appear in the returned `map[string]tool.Tool`. (AGENT-LOOP-REQ-7)
- [ ] 4.6 [TEST] Error aggregation: parse error in one source does NOT suppress results from other sources; error slice contains the parse error alongside valid results. (AGENT-LOOP-REQ-7)
- [ ] 4.7 [IMPL] Create `internal/skill/loader_unified.go` (NEW). Implement `LoadSkillsUnified(ctx, fsPaths, dbStore UserSkillStore, curatedFS embed.FS, shellCfg, limits)` — 3-pass merge (curated → fs → db) into `map[name]→entry`. Calls existing `LoadSkills` for FS pass. (AGENT-LOOP-REQ-7; design §2.4)
- [ ] 4.8 [IMPL] Add helper `userSkillToParts(us UserSkill) (SkillContent, *ExecutableSkillDef)` in `loader_unified.go` — converts `BudgetJSON.TimeoutMin` to `time.Duration` for `BudgetConfig.Timeout`. (AGENT-LOOP-REQ-7; design §2.4)

### Wiring into Binary Entry Points

- [ ] 4.9 [IMPL] In `cmd/daimon/main.go`: switch from `skill.LoadSkills` to `skill.LoadSkillsUnified`; inject `UserSkillStore` from store into `ServerDeps`. (AGENT-LOOP-REQ-7)
- [ ] 4.10 [IMPL] In `cmd/daimon/web_cmd.go`: same switch + inject `UserSkillStore` into `ServerDeps`. (AGENT-LOOP-REQ-7)
- [ ] 4.11 [TEST] Integration: boot loads from FS + DB simultaneously; DB > FS precedence respected; existing FS skills appear unchanged in result. (AGENT-LOOP-REQ-7)

---

## Phase 5 — Budget Reversal + Timeout==0 Fix (~80 LoC)

> Requires Phase 4 merged (reversal must hold across all three loader sources). Bundles into PR3 with Phase 4.

### Spawn Timeout==0 Fix

- [ ] 5.1 [TEST] In `internal/agent/subagent_manager_test.go`: spawn with `Budget.Timeout == 0` → subagent ctx is NOT done within 100 ms of spawn (no instant cancel). (REQ-16; design §2.11 — exact file line `subagent_manager.go:233`)
- [ ] 5.2 [TEST] Spawn with `Budget.Timeout > 0` → existing timeout behavior unchanged (ctx done after timeout elapses). (REQ-16)
- [ ] 5.3 [IMPL] In `internal/agent/subagent_manager.go` at line 233: replace unconditional `context.WithTimeout(ctx, def.Budget.Timeout)` with branch — `if def.Budget.Timeout > 0 { WithTimeout } else { WithCancel }`. (REQ-16; design §2.11)

### Parser Budget Reversal

- [ ] 5.4 [TEST] In `internal/skill/parser_test.go`: skill file with `executable: true` and NO `budget` key → loads successfully; resulting `ExecutableSkillDef` has zero-value `Budget` (was: load error). (REQ-12 reversal; CONFIG-REQ-6)
- [ ] 5.5 [TEST] Skill file with `executable: true` and `budget: defaults` → unchanged: loads successfully and expands to `{0.50, 20, 10}`. (REQ-12)
- [ ] 5.6 [TEST] Skill file with explicit budget block → unchanged: loads successfully. (REQ-12)
- [ ] 5.7 [IMPL] In `internal/skill/parser.go` at lines 257-263: REMOVE the `if fm.Executable { if fm.Budget.IsZero() { errs = append(errs, ...) } }` block entirely. No replacement needed. (REQ-12; CONFIG-REQ-6; design §2.12)

### End-to-End Regression

- [ ] 5.8 [TEST] Integration: skill with no budget block → loads without error → spawns → subagent ctx lives past 500 ms → completes naturally (validates both parser change + Timeout==0 fix together). (REQ-12; REQ-16)

---

## Phase 6 — Curated Catalog (~300 LoC + 5 templates)

> Requires Phase 4 merged (LoadSkillsUnified must exist). Ships as PR4.

### Embedded FS

- [ ] 6.1 [IMPL] Create directory `internal/skill/curated/`. (design §2.10)
- [ ] 6.2 [IMPL] Create `internal/skill/curated_embed.go` (NEW) with `//go:embed curated/*.md` directive and exported `CuratedFS embed.FS`. (design §2.10)

### Initial 5 Templates

> Each template MUST include valid frontmatter (`executable: true`, `budget: defaults`) so a fresh install has working spawn tools out-of-the-box.

- [ ] 6.3 [IMPL] `internal/skill/curated/researcher.skill.md` — researcher persona; reads/searches; `budget: defaults`. (design §2.10)
- [ ] 6.4 [IMPL] `internal/skill/curated/summarizer.skill.md` — summarizes text/docs; `budget: defaults`. (design §2.10)
- [ ] 6.5 [IMPL] `internal/skill/curated/code-reviewer.skill.md` — code review persona; references `read_file` in allowlist; `budget: defaults`. (design §2.10)
- [ ] 6.6 [IMPL] `internal/skill/curated/email-drafter.skill.md` — email composition; `budget: defaults`. (design §2.10)
- [ ] 6.7 [IMPL] `internal/skill/curated/meeting-notes.skill.md` — meeting note extractor; `budget: defaults`. (design §2.10)

### Loader Integration

- [ ] 6.8 [TEST] In `internal/skill/loader_unified_test.go`: `loadCurated(embed.FS)` parses all 5 templates; each yields well-formed `SkillContent` and `ExecutableSkillDef`; `source` field = `"curated"`. (AGENT-LOOP-REQ-7)
- [ ] 6.9 [TEST] Zero-value / empty `embed.FS` passed as curated source → returns empty slices, no error. (AGENT-LOOP-REQ-7)
- [ ] 6.10 [IMPL] Implement `loadCurated(fs embed.FS) ([]SkillContent, []ExecutableSkillDef, []error)` helper in `internal/skill/loader_unified.go` or `curated_embed.go`. Walks `"curated/"` dir; parses each `.skill.md`; emits NO `tool.Tool` entries (curated templates reference environment tools only). (AGENT-LOOP-REQ-7; design §2.10)
- [ ] 6.11 [IMPL] Integrate `loadCurated` into `LoadSkillsUnified` as the lowest-precedence pass (curated runs first; DB and FS can override). (AGENT-LOOP-REQ-7)

### Shadow Tests

- [ ] 6.12 [TEST] User creates `user_skill` with the same name as a curated skill → DB entry wins; `GET /api/skills/{name}` returns the user version with `source="user"`. (CONFIG-REQ-9; design §3.3)
- [ ] 6.13 [TEST] User deletes their `user_skill` → curated skill reappears in `GET /api/skills/{name}` response with `source="curated"`. (CONFIG-REQ-9; design §3.3)

---

## Deferred Items (Proposal §4 — explicitly out of scope)

- Recursive subagents (depth > 1) — REQ-9 stays in force
- Multi-spawn `batch_id` grouping
- Soft warning for turn/timeout caps (parity with cost 80% warn)
- Sibling cancel (cancel-N-by-batch_id)
- Mid-turn budget gate (requires provider streaming hooks)
- Per-sub MCP isolation
- `attribution_kind = "advisor_call"` runtime path
- API-level rate-limit coordination
- Skill versioning history (`user_skill_history` table)
- Tags / categories for the curated catalog
- Export / import (`.skill.md` round-trip from DB)
- Marketplace / remote registry (SkillsRegistryURL)
- Frontend UI (lives in daimon-frontend repo, ships separately)

---

## Test Coverage Summary

**Total test tasks**: 47
**Total impl tasks**: 41
**Total tasks**: 88

### REQ → Test Mapping

| Requirement | Test Tasks |
|---|---|
| REQ-12 reversal (optional budget) | 5.4, 5.5, 5.6, 5.8 |
| REQ-16 (Timeout==0 → WithCancel) | 5.1, 5.2, 5.8 |
| REQ-17 (POST /api/subagents/{id}/cancel) | 1.0, 1.13, 1.16 |
| REQ-18 (Agent.CancelSubagent nil-safe) | 1.10 |
| REQ-19 (Agent.ReplaceExecutableSkills) | 3.1, 3.2, 3.3, 3.4, 3.13, 3.14, 3.15, 3.16 |
| REQ-20 (ConfigurableProvider) | 1.2, 1.9 |
| OUTPUT-STORE-REQ-11 (migration v18) | 2.1 |
| OUTPUT-STORE-REQ-12 (UserSkillStore) | 2.3, 2.6, 2.7, 2.8, 2.9, 2.10, 2.11, 2.12 |
| CONFIG-REQ-4 (budget OPTIONAL in frontmatter) | 5.4, 5.5, 5.6 |
| CONFIG-REQ-6 (parser no hard-error) | 5.4, 5.7, 3.10 |
| CONFIG-REQ-9 (source metadata-only) | 3.8, 3.11, 3.12, 6.12, 6.13 |
| AGENT-LOOP-REQ-7 (LoadSkillsUnified) | 4.1, 4.2, 4.3, 4.4, 4.5, 4.6, 4.11, 6.8, 6.9 |
| AGENT-LOOP-REQ-8 (hot-reload after CRUD) | 3.13, 3.14, 3.15, 3.16 |
