# Tasks: tui-rail-panels

> Artifact store: openspec. Delivery strategy: ask-on-risk. Strict TDD: `make test`.
> All files under `internal/tui/`. No edits outside that directory.

---

## Review Workload Forecast

| Field                                     | Value                                |
| ----------------------------------------- | ------------------------------------ |
| Estimated changed lines (PR-a)            | ~150 (impl ~90, tests ~60)           |
| Estimated changed lines (PR-b)            | ~250 (impl ~140, tests ~110)         |
| Estimated changed lines (PR-c)            | ~270 (impl ~130, tests ~140)         |
| Total estimated                           | ~670 across three PRs                |
| 400-line budget risk (per-slice)          | Low / Low / Low                      |
| 400-line budget risk (combined single PR) | High                                 |
| Chained PRs recommended                   | Yes                                  |
| Suggested split                           | PR-a → PR-b → PR-c (stacked-to-main) |
| Delivery strategy                         | ask-on-risk                          |
| Chain strategy                            | stacked-to-main                      |

Decision needed before apply: Yes
Chained PRs recommended: Yes
Chain strategy: stacked-to-main
400-line budget risk: Low (per slice) / High (combined)

### Suggested Work Units

| Unit | Goal                                                       | Likely PR   | Notes                                         |
| ---- | ---------------------------------------------------------- | ----------- | --------------------------------------------- |
| PR-a | context-meter REPLACE + real limit + category rows         | PR 1 → main | base: main; green standalone                  |
| PR-b | telemetry per-tool + per-subagent + EventSubagentCompleted | PR 2 → main | base: main (structurally independent of PR-a) |
| PR-c | memory-peek panel end-to-end + contract test updates       | PR 3 → main | base: main; only PR touching 8 files          |

---

## PR-a: context-meter REPLACE semantics + real limit + category rows

Files touched: `rail_panels.go` (contextMeterPanel section ~:321–377), `run.go` (~:78–98), `rail_panels_test.go`.

### RED — failing tests first

- [x] **a.1** In `rail_panels_test.go`, add `TestContextMeter_REPLACE_SecondEventWins`: feed two `EventTokensUsage` events with non-zero `SysToks/MsgToks/ToolToks`; assert the second event's values win (spec scenario TR-2 "REPLACE semantics — second event wins"). Also assert `tokenUsed == sum of second event only`. Run `make test` → RED.

- [x] **a.2** In `rail_panels_test.go`, add `TestContextMeter_Legacy_Accumulates`: feed two `EventTokensUsage` events with all-zero category fields and non-zero `TokenCount`; assert `tokenUsed` accumulates both `TokenCount`s and `sysToks/msgToks/toolToks` stay 0 (spec scenario TR-2 "Legacy fallback — tokenUsed accumulates"). Run `make test` → RED.

- [x] **a.3** In `rail_panels_test.go`, add `TestContextMeter_SetLimit_NonZero`: construct panel, call `setLimit(128000)`; assert `limit == 128000`; call `Render` after feeding one smart-strategy event; assert output does NOT contain ` est.` (spec scenario TR-1 "Non-zero limit stored and used"). Run `make test` → RED.

- [x] **a.4** In `rail_panels_test.go`, add `TestContextMeter_SetLimit_ZeroFallback`: construct panel, call `setLimit(0)` (or skip call); assert `Render` output contains ` est.` (spec scenario TR-1 "Zero limit falls back to heuristic"). Run `make test` → RED.

- [x] **a.5** In `rail_panels_test.go`, add `TestContextMeter_Render_SmartStrategy_Golden`: fix panel state to `sysToks=1000, msgToks=2000, toolToks=500, limit=128000, hasData=true`; call `Render(32, 0)`; assert output contains `sys`, `msg`, `tool` rows AND a total bar AND NO ` est.` (spec scenario TR-3 "Smart strategy — sub-bars present"). Use `golden.RequireEqual` against `testdata/context_meter_with_categories.golden` — generate on first `-update` run. Run `make test` → RED.

- [x] **a.6** In `rail_panels_test.go`, add `TestContextMeter_Render_LegacyStrategy_Golden`: fix panel state to `sysToks=0, tokenUsed=42000, limit=200000, hasData=true`; call `Render(32, 0)`; assert output does NOT contain `sys`/`msg`/`tool` rows AND contains ` est.` (spec scenario TR-3 "Legacy strategy — only aggregate bar"). Use `golden.RequireEqual` against `testdata/context_meter_fallback_aggregate.golden`. Run `make test` → RED.

- [x] **a.7** In `rail_panels_test.go`, add `TestContextMeter_Render_NoData_Empty`: zero-value panel, `Render(32, 0)`, assert `""` (spec scenario TR-3 "No data — empty render" + TR-0-C). Run `make test` → RED.

- [x] **a.8** In `rail_panels_test.go`, add `TestContextMeter_Render_Deterministic`: same fixed state called twice; assert both returns are equal (spec scenario TR-0-A "Render is deterministic"). Run `make test` → RED.

- [x] **a.9** In `run_test.go` (or `rail_panels_test.go`), add `TestContextMeter_BootWiring_RealLimit`: construct a model-like struct using `newRail`+`copyRailWith` with a known limit (e.g. `128000`); assert the `contextMeterPanel.limit == 128000` (spec scenario TR-1, boot-wiring path). Run `make test` → RED.

### GREEN — minimal implementation

- [x] **a.10** In `rail_panels.go` (~:321–338), extend `contextMeterPanel` struct: add fields `limit int`, `sysToks int`, `msgToks int`, `toolToks int`. Add `setLimit(n int)` method. Update `newContextMeterPanel` (no signature change needed — limit starts 0, heuristic applied in Render).

- [x] **a.11** In `rail_panels.go`, rewrite `contextMeterPanel.accumulate`: implement Branch A (REPLACE: `sysToks/msgToks/toolToks = ev.*`, `tokenUsed = sum`) and Branch B (legacy: `tokenUsed += ev.TokenCount`, categories unchanged). `hasData = true` in both.

- [x] **a.12** In `rail_panels.go`, add pure helper `humanK(n int) string` (e.g. `200000→"200k"`, `1500→"1.5k"`, `999→"999"`). No IO, no clock. Place near top of contextMeterPanel section.

- [x] **a.13** In `rail_panels.go`, rewrite `contextMeterPanel.Render`: resolve `limit` + `label` (`est.` suffix when `limit==0`), use heuristic `200_000` fallback; render aggregate bar + pct line always; when `sysToks > 0`, append three labeled count rows (`sys / msg / tool` + `humanK` counts, `dimLabel`-styled, ANSI-truncated to `inner`). ANSI width via `ansi.Truncate`, not `len`. Produce golden files with `-update` flag.

- [x] **a.14** In `run.go` (~:78–98), inside the existing `copyRailWith` block, read `ctxLimit := ag.ContextWindowSize()` once before the block; add a `copyRailWith` entry that value-copies the `contextMeterPanel` (`cp := *cm`) and calls `cp.setLimit(ctxLimit)`.

### REFACTOR

- [x] **a.15** Run `make test` → all PR-a tests GREEN. Confirm golden files are generated (`context_meter_with_categories.golden`, `context_meter_fallback_aggregate.golden`). Remove any temporary `t.Skip` lines. Verify `humanK` boundary values (999, 1000, 1500, 200000) are covered by at least one assertion inline in `TestContextMeter_Render_*` tests.

---

## PR-b: telemetry per-tool + per-subagent + EventSubagentCompleted

Files touched: `rail_panels.go` (telemetryPanel section ~:76–139), `screen_chat.go` (~:291 area), `rail_panels_test.go`.

### RED — failing tests first

- [x] **b.1** In `rail_panels_test.go`, add `TestTelemetry_ToolStats_CallsCountedOnStart`: send two `EventToolStart` events for `ToolName="bash"`; assert `toolStats["bash"].calls == 2` and `toolStats["bash"].errors == 0` (spec scenario TR-4 "Tool call counted on Start"). Run `make test` → RED.

- [x] **b.2** In `rail_panels_test.go`, add `TestTelemetry_ToolStats_ErrorAndDurationOnEnd`: send one `EventToolStart` + one `EventToolEnd{ToolName:"read_file", DurationMs:150, IsError:true}`; assert `toolStats["read_file"].errors == 1` and `toolStats["read_file"].durationMs == 150` (spec scenario TR-4 "Tool error and duration recorded on End"). Run `make test` → RED.

- [x] **b.3** In `rail_panels_test.go`, add `TestTelemetry_ToolStats_MultipleTools`: send three `EventToolStart` for three distinct names; assert `len(toolStats) == 3` and each has `calls == 1` (spec scenario TR-4 "Accumulation across multiple tools"). Run `make test` → RED.

- [x] **b.4** In `rail_panels_test.go`, add `TestTelemetry_SubagentLive_Accumulates`: send two `EventTokensUsage` events with `Meta["subagent_id"]="sa-abc"`, `input_tokens="100"`, `output_tokens="50"`; assert `subagentStats["sa-abc"].tokens == 300` and `done == false` (spec scenario TR-6 "Live accumulation from multiple EventTokensUsage events"). Run `make test` → RED.

- [x] **b.5** In `rail_panels_test.go`, add `TestTelemetry_AtoiSafe_BadMeta`: send `EventTokensUsage` with `subagent_id="sa-x"`, `input_tokens=""`, `output_tokens="abc"`; assert `subagentStats["sa-x"].tokens == 0` and no panic (spec scenario TR-6 "atoiSafe handles missing or non-numeric Meta values"). Run `make test` → RED.

- [x] **b.6** In `rail_panels_test.go`, add `TestTelemetry_SubagentCompleted_AuthoritativeWins`: set `subagentStats["sa-1"].tokens = 250` via live events; send `EventSubagentCompleted{Meta{"subagent_id":"sa-1","tokens":"405"}}`; assert `tokens == 405` and `done == true` (spec scenario TR-7 "Authoritative total overwrites live accumulation"). Run `make test` → RED.

- [x] **b.7** In `rail_panels_test.go`, add `TestTelemetry_SubagentCompleted_EmptyID_NoOp`: send `EventSubagentCompleted{Meta{"subagent_id":""}}` on empty panel; assert `subagentStats` remains empty, no panic (spec scenario TR-7 "empty subagent_id is no-op"). Run `make test` → RED.

- [x] **b.8** In `rail_panels_test.go`, add `TestTelemetry_SubagentCompleted_UnseenCreates`: empty panel, send `EventSubagentCompleted{Meta{"subagent_id":"sa-new","tokens":"120"}}`; assert `subagentStats["sa-new"].tokens == 120` and `done == true` (spec scenario TR-7 "creates bucket"). Run `make test` → RED.

- [x] **b.9** In `rail_panels_test.go`, add `TestTelemetry_SubagentFailed_MarkerSet`: send `EventSubagentFailed{Meta{"subagent_id":"sa-f"}}`; assert `subagentStats["sa-f"].done == true`, `failed == true`, and `tokens` is NOT read from Meta (spec scope boundary + design ADR-2 failed marker). Run `make test` → RED.

- [x] **b.10** In `rail_panels_test.go`, add `TestTelemetry_SubagentLive_LateEventIgnoredAfterDone`: send `EventSubagentCompleted` setting `done=true, tokens=405`; then send another `EventTokensUsage` with same `subagent_id`; assert `tokens` still `405`, not re-inflated (design Risk 5 `if !st.done` guard). Run `make test` → RED.

- [x] **b.11** In `rail_panels_test.go`, add **`TestTelemetry_COW_PriorSnapshotUnchangedAfterAccumulate`** (the mandatory COW map-clone test): obtain a `*telemetryPanel` from a `copyRailWith` call; fire a second event on the copy; assert the ORIGINAL panel's `toolStats` map is UNCHANGED (spec scenario TR-0-B + design Risk 5). This test MUST fail until the clone helpers are implemented. Run `make test` → RED.

- [x] **b.12** In `rail_panels_test.go`, add `TestTelemetry_HandleBusEvent_SubagentCompleted_UpdatesPanel`: drive `EventSubagentCompleted` through `handleBusEvent` on a test `Model`; assert `telemetryPanel.subagentStats` updated in the resulting model (spec scenario TR-7, non-vacuous via `handleBusEvent` path, mirrors `TestHandleBusEvent_TodolistChanged_*`). Run `make test` → RED.

- [x] **b.13** In `rail_panels_test.go`, add `TestTelemetry_Render_ToolRows_Golden`: set `toolStats` with 3 tools (counts: A=5, B=3, C=1) and `hasData=true`; call `Render(40, 0)`; assert output contains all 3 tool names in count-desc order and NO `+N more` (spec scenario TR-5 "Five or fewer tools — all shown"). Use `golden.RequireEqual` against `testdata/telemetry_with_tool_rows.golden`. Run `make test` → RED.

- [x] **b.14** In `rail_panels_test.go`, add `TestTelemetry_Render_ToolRows_Cap5_Overflow`: set 8 tools; `Render(40, 0)`; assert exactly 5 tool rows and `+3 more` line (spec scenario TR-5 "More than five tools — cap enforced"). Run `make test` → RED.

- [x] **b.15** In `rail_panels_test.go`, add `TestTelemetry_Render_SubagentRows_Golden`: set `subagentStats` with 2 entries and `saOrder`; call `Render(40, 0)`; assert both rows present (spec scenario TR-8 "Three or fewer subagents"). Use `golden.RequireEqual` against `testdata/telemetry_with_subagent_rows.golden`. Run `make test` → RED.

- [x] **b.16** In `rail_panels_test.go`, add `TestTelemetry_Render_SubagentRows_Cap3`: set 5 subagents; `Render(40, 0)`; assert exactly 3 subagent rows (spec scenario TR-8 "More than three subagents — cap enforced"). Run `make test` → RED.

### GREEN — minimal implementation

- [x] **b.17** In `rail_panels.go` (~:76–106), extend `telemetryPanel` struct: add `toolStats map[string]toolStat`, `subagentStats map[string]subagentStat`, `saOrder []string`. Add `toolStat` struct (`calls int`, `errors int`, `durationMs int64`). Add `subagentStat` struct (`tokens int`, `done bool`, `failed bool`).

- [x] **b.18** In `rail_panels.go`, add clone helpers `cloneToolStats(map[string]toolStat) map[string]toolStat` and `cloneSubagentStats(map[string]subagentStat) map[string]subagentStat` (return fresh map copies). Add `cloneSAOrder([]string) []string` (return fresh slice copy). These ensure COW correctness when `accumulate` runs on the shallow-copied `cp := *tp`.

- [x] **b.19** In `rail_panels.go`, rewrite `telemetryPanel.accumulate`: add `EventToolStart` (clone `toolStats`, increment `calls`), `EventToolEnd` (clone `toolStats`, accumulate `durationMs`/`errors`), `EventTokensUsage` live subagent branch (clone `subagentStats`+`saOrder`, accumulate `tokens` when `!st.done`), `EventSubagentCompleted` (clone maps, REPLACE `tokens`, set `done=true`), `EventSubagentFailed` (clone maps, set `done=true, failed=true`, do NOT read `Meta["tokens"]`). All clones happen BEFORE mutating.

- [x] **b.20** In `rail_panels.go`, rewrite `telemetryPanel.Render`: keep four aggregate lines; add per-tool rows (sort count-desc, name-asc, cap 5, `+N more` in `dimLabel`; error suffix in `errStyle`); add per-subagent rows (first-seen via `saOrder`, cap 3, `+N more`; status marker `✓`/`✗`/`●`; truncate IDs to 8 runes via `ansi.Truncate`). Produce golden files with `-update`.

- [x] **b.21** In `screen_chat.go` (~after `:302`), add new `handleBusEvent` case `notify.EventSubagentCompleted, notify.EventSubagentFailed`: `copyRailWith` → `cp := *tp` → `cp.accumulate(ev)` → `panels[panelTelemetry] = &cp`. Pattern mirrors existing `EventToolStart` block.

### REFACTOR

- [x] **b.22** Run `make test` → all PR-b tests GREEN. Confirm golden files generated (`telemetry_with_tool_rows.golden`, `telemetry_with_subagent_rows.golden`). Verify `TestTelemetry_COW_PriorSnapshotUnchangedAfterAccumulate` passes (the map is cloned, prior snapshot untouched). Remove any `t.Skip`.

---

## PR-c: memory-peek panel end-to-end + contract test updates

Files touched: `panels.go`, `rail_panels.go`, `rail_panels_cmd.go`, `screen_chat.go`, `model.go`, `rail.go`, `model_test.go`, `rail_panels_test.go`.

### RED — failing tests first (deliberate break-then-fix for contract tests)

- [ ] **c.1** In `model_test.go` (~:19–22), update `TestPanelsFor_ContractMatrix` `screenChat` row to expect `[]panelID{panelTodolist, panelContextMeter, panelTelemetry, panelMemoryPeek}` (spec scenario TR-15). Run `make test` → RED (`panelMemoryPeek` undefined).

- [ ] **c.2** In `rail_panels_test.go` (~:329), update `TestRail_ChatScreen_PanelsRegistered` to include `panelMemoryPeek` in the panel-ID loop (spec scenario TR-15). Run `make test` → RED.

- [ ] **c.3** In `rail_panels_test.go`, add `TestMemoryPeek_Render_Empty`: zero-value `memoryPeekPanel`, `Render(32, 0)`, assert `""` (spec scenario TR-10 "Empty entries — Render returns """). Run `make test` → RED (`memoryPeekPanel` undefined).

- [ ] **c.4** In `rail_panels_test.go`, add `TestMemoryPeek_Render_PopulatedTitles_Golden`: panel with 3 entries (non-empty `Title`); `Render(32, 0)`; assert output contains all 3 titles and `memory` header badge (spec scenario TR-10 "Populated entries — rows rendered"). Use `golden.RequireEqual` against `testdata/memory_peek_populated.golden`. Run `make test` → RED.

- [ ] **c.5** In `rail_panels_test.go`, add `TestMemoryPeek_Render_Cap5`: 8 entries; `Render(32, 0)`; assert at most 5 entry rows (spec scenario TR-10 "Entries cap at 5"). Run `make test` → RED.

- [ ] **c.6** In `rail_panels_test.go`, add `TestMemoryPeek_Render_TitleFallback_Content`: one entry `Title=""`, `Content="some content text"`; `Render(32, 0)`; assert output contains a prefix of `"some content text"` and NOT an empty row (spec scenario TR-10 "Empty Title falls back to Content"). Run `make test` → RED.

- [ ] **c.7** In `rail_panels_test.go`, add `TestFetchMemory_NilStore_NoOp`: call `fetchMemory(nil, "scope-123")`; execute returned cmd; assert `memoryRefreshMsg{}` with empty entries, no panic (spec scenario TR-11 "Nil store produces empty msg"). Run `make test` → RED (`fetchMemory` undefined).

- [ ] **c.8** In `rail_panels_test.go`, add `TestFetchMemory_EmptyScopeID_NoOp`: call `fetchMemory(someStore, "")`; execute cmd; assert empty msg, `SearchMemory` NOT called (spec scenario TR-11 "Empty scopeID produces empty msg"). Run `make test` → RED.

- [ ] **c.9** In `rail_panels_test.go`, add `TestFetchMemory_ValidInputs_ReturnsEntries`: fake `store.Store` whose `SearchMemory(ctx,"scope-1","",5)` returns 3 entries; execute cmd; assert `memoryRefreshMsg.entries` has length 3 (spec scenario TR-11 "Valid inputs — entries returned"). Run `make test` → RED.

- [ ] **c.10** In `rail_panels_test.go`, add `TestHandleBusEvent_MemoryChanged_ReturnsFetchCmd`: drive `EventMemoryChanged{Meta{"scope_id":"scope-abc"}}` through `handleBusEvent`; execute batch; assert `memoryRefreshMsg` appears among messages (spec scenario TR-12 "EventMemoryChanged triggers fetchMemory cmd"). Run `make test` → RED.

- [ ] **c.11** In `rail_panels_test.go`, add `TestMemoryRefreshMsg_Update_SetsEntries`: drive `memoryRefreshMsg{entries: []store.MemoryEntry{{Title:"t1"},{Title:"t2"}}}` through `Update`; assert new model's `memoryPeekPanel.entries` has 2 entries; assert prior model snapshot's panel remains empty (spec scenario TR-13 "memoryRefreshMsg populates panel entries", COW). Run `make test` → RED.

- [ ] **c.12** In `rail_panels_test.go`, add `TestMemoryRefreshMsg_Update_ClearsPrior`: model with 3 entries in panel; drive `memoryRefreshMsg{entries:nil}`; assert new model's panel entries is empty, Render returns `""` (spec scenario TR-13 "clears prior entries"). Run `make test` → RED.

### GREEN — minimal implementation

- [ ] **c.13** In `panels.go`: add const `panelMemoryPeek panelID = "memory-peek"`. Update `panelsFor(screenChat)` to return `[]panelID{panelTodolist, panelContextMeter, panelTelemetry, panelMemoryPeek}` (spec TR-9). This fixes c.1 + c.2 RED.

- [ ] **c.14** In `rail_panels.go`, add `memoryPeekPanel` struct (`styles tuiStyles`, `entries []store.MemoryEntry`), `newMemoryPeekPanel(s tuiStyles) *memoryPeekPanel`, `setEntries([]store.MemoryEntry)`, and `Render(width, _ int) string` (empty → `""`; else header badge `"memory"` + up to 5 rows Title-first/Content-fallback, `ansi.Truncate` per row, `wrapPanelBox`). `store` already imported at `:30`. Produce golden with `-update`.

- [ ] **c.15** In `rail_panels_cmd.go`, add `memoryRefreshMsg{entries []store.MemoryEntry}` and `fetchMemory(st store.Store, scopeID string) tea.Cmd`. Mirror `fetchTodolist` exactly: nil/empty → return `memoryRefreshMsg{}` without calling `SearchMemory`; otherwise call `st.SearchMemory(context.Background(), scopeID, "", 5)`, silently ignore error. Add imports: `"context"` and `"daimon/internal/store"`.

- [ ] **c.16** In `screen_chat.go`, add `handleBusEvent` case `notify.EventMemoryChanged`: `cmds = append(cmds, fetchMemory(m.store, ev.Meta["scope_id"]))`. Place after the `EventTodolistChanged` case (~:304–308).

- [ ] **c.17** In `model.go`, add `Update` case `case memoryRefreshMsg:`: `copyRailWith` → value-copy `mp := *mp` → `cp.setEntries(msg.entries)` → `panels[panelMemoryPeek] = &cp` → `return m, nil`. Mirror `todolistRefreshMsg` case (~:288–296).

- [ ] **c.18** In `rail.go` (`newRail` function ~:26–38), add `panelMemoryPeek: newMemoryPeekPanel(s)` to the panel map.

### REFACTOR

- [ ] **c.19** Run `make test` → all PR-c tests GREEN including the two previously-RED contract tests (c.1, c.2). Confirm `memory_peek_populated.golden` generated. Run `make test` without `-update` to assert golden stability. Check `TestModel_View_ChatScreen_Golden`: if diff shows only the empty memory-peek addition, regenerate it with `-update` (design Risk 7). Verify no file outside `internal/tui/` was modified (`git diff --name-only`).

---

## Cross-cutting notes (apply-time reminders)

- **`humanK` determinism** (design Risk 4): define fixed rules (`n < 1000 → "%d"`, `n < 10000 → "%.1fk"`, else `"%dk"`). Test boundary values 999, 1000, 1500, 200000 inline in a.5 or a.15 assertions.
- **COW map discipline** (design Risk 5): `cloneToolStats` / `cloneSubagentStats` / `cloneSAOrder` MUST be called inside `accumulate` on the copy BEFORE any map/slice mutation. Verified by b.11.
- **`EventSubagentFailed` Meta** (design Risk 6): the Failed branch MUST NOT call `atoiSafe(ev.Meta["tokens"])`; it only sets `done=true, failed=true`. Verified by b.9.
- **Rail height deferral** (ADR-4): do NOT add a height clamp. No `rail.Render` changes.
- **Imports in `rail_panels_cmd.go`** (design Risk 2): add `"context"` and `"daimon/internal/store"` in c.15; `"daimon/internal/agent"` and `"daimon/internal/tool"` remain (used by `fetchTodolist`).
