# Exploration: tui-rail-panels (REFRESHED)

> Phase 2 of the daimon embedded-TUI design alignment.
> Original investigation: 2026-05-31 (pre-backend)
> **REFRESHED: 2026-05-31** — re-grounded against merged `tui-backend-seams` (main=ccffc6f).
> All four seams verified present in source. Previously-blocked panels are now unblocked.

---

## Intent

Wire real data into the three existing chat-rail panel skeletons (`todolistPanel`,
`contextMeterPanel`, `telemetryPanel`) and build the new `memoryPeekPanel`, using
the four backend seams delivered by `tui-backend-seams` (merged to main at ccffc6f).
All panels implement `Panel.Render(width, height int) string` — a pure function of
cached Model fields — mutated only in `Update` via `copyRailWith`. The
`View=pure(Model)` invariant must hold: no clock reads, no live-object access, no IO
in any `Render` path.

**Status change from original exploration**: all four panels are now unblocked. The
three seams that were missing (`ContextWindowSize`, `SysToks/MsgToks/ToolToks`,
`EventMemoryChanged`) are verified live on main. This exploration maps the exact
consumption wiring for each.

---

## Verified backend seams (all present on main=ccffc6f)

| Seam                                        | Location                                                         | Signature / shape                                                                                                                                | Notes                                                                               |
| ------------------------------------------- | ---------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------ | ----------------------------------------------------------------------------------- |
| `Agent.ContextWindowSize() int`             | `internal/agent/agent_accessors.go:56`                           | returns `a.contextMgr.MaxTokens()`; 0 when `contextMgr==nil` (nil=unknown, caller falls back)                                                    | Static: read ONCE at TUI boot, never changes                                        |
| `notify.Event.SysToks/MsgToks/ToolToks int` | `internal/notify/bus.go:34-36`                                   | populated on `EventTokensUsage` from `contextMgr.LastUsage()` (loop.go:1033-1048); zero when smart strategy did not run                          | REPLACE semantics: snapshot, not delta                                              |
| Per-subagent telemetry                      | `internal/agent/subagent_meta.go`, `subagent_manager.go:550-573` | `EventTokensUsage.Meta["subagent_id"]` (live; from `mergeSubagentMeta`); `EventSubagentCompleted.Meta["tokens"]` (authoritative total as string) | `mergeSubagentMeta` only injects subagent keys for subagent convs (ParentConvID≠"") |
| `notify.EventMemoryChanged`                 | `internal/notify/events.go:51`                                   | bare signal; `Meta["scope_id"]` + `Meta["entry_id"]`; emitted after `AppendMemory`                                                               | NOT in StreamingSkipSet; TUI must refetch via `store.SearchMemory`                  |

---

## Per-panel matrix

### Panel 1 — `todolistPanel`

**a. Current skeleton state**
`rail_panels.go:147-200` — fully implemented. Renders `tool.TodoList.Items` with status markers (✓ done, ● in_progress, ○ pending); `""` when empty. `setList(list tool.TodoList)` at line 158.

**b. Consumption wiring — already complete**
`EventTodolistChanged` → `fetchTodolist(ag, activeConvID)` cmd (rail_panels_cmd.go:27) → `todolistRefreshMsg` (rail_panels_cmd.go:18) → `Model.Update` case at model.go:288 → `copyRailWith` → `tp.setList(msg.list)` → `Render` reads `p.list.Items`.

**c. In `panelsFor(screenChat)`?** Yes: `panels.go:39`.

**d. COW pattern** — `copyRailWith` at model.go:289 produces a new rail; `*tp` value copy before `setList`; original model snapshot unaffected.

**e. Verdict: DONE. Zero new work.**
Rough LOC: 0.

---

### Panel 2 — `contextMeterPanel`

**a. Current skeleton state**
`rail_panels.go:321-377` — renders a `[███░░]` bar at `const contextLimit = 200_000` (line 353); `accumulate(ev)` reads only `ev.TokenCount` (cumulative output tokens — NOT real context fill). Has `hasData bool`; returns `""` until first event. Hardcoded `"%.1f%% of 200k"` label (line 369).

**b. Consumption wiring needed**

_Boot-time: real window limit_

- At `run.go` construction (after `r := newRail(s)`), read `ag.ContextWindowSize()` once.
- If non-zero: call `copyRailWith` to replace `panelContextMeter` with a new panel constructed with `limit int` field set (add `setLimit(n int)` method or a `newContextMeterPanel(s, limit int)` constructor overload).
- `Render` uses `p.limit` with fallback: `if p.limit == 0 { p.limit = 200_000 }`.

_Per-turn: per-category fill (REPLACE semantics)_
The existing `accumulate` currently sums `ev.TokenCount` as a delta accumulator. This must be changed to REPLACE semantics: each `EventTokensUsage` is a snapshot.

- Add fields `sysToks, msgToks, toolToks int` to `contextMeterPanel`.
- Change `accumulate(ev)`: when `ev.SysToks+ev.MsgToks+ev.ToolToks > 0`, store them as the new snapshot value (overwrite, not add). Keep `p.tokenUsed` updated from `ev.SysToks+ev.MsgToks+ev.ToolToks` when the breakdown is available, else fall back to `ev.TokenCount`.
- `handleBusEvent` case `EventTokensUsage` already calls `cm.accumulate(ev)` via `copyRailWith` at `screen_chat.go:297-300` — no new wiring point needed, only the panel struct changes.
- `Render` shows breakdown rows when `sysToks > 0`: sys/msg/tool lines + total bar.

_Exact wiring chain (no new handleBusEvent case needed):_

```
loop.go emits EventTokensUsage{SysToks, MsgToks, ToolToks}
  → bus → evCh → pumpEvents → busEventMsg
    → handleBusEvent case EventTokensUsage (screen_chat.go:271)
      → copyRailWith → cm.accumulate(ev) [REPLACE snap]
        → Render reads p.sysToks/msgToks/toolToks + p.limit
```

**c. In `panelsFor(screenChat)`?** Yes: `panels.go:39`.

**d. COW pattern** — existing `copyRailWith` call at screen_chat.go:291 already handles this panel. Boot-time `setLimit` also goes through `copyRailWith` in run.go.

**e. Verdict: MODIFY. Needs ~35 LOC (limit field + setLimit + accumulate REPLACE semantics + updated Render with breakdown rows + run.go boot wiring).**

---

### Panel 3 — `telemetryPanel`

**a. Current skeleton state**
`rail_panels.go:76-139` — renders aggregate tokens/cost/tool-calls/errors. `accumulate(ev)` handles `EventTokensUsage` (adds `ev.TokenCount + ev.CostUSD`) and `EventToolStart`/`EventToolEnd` (counts). No per-tool breakdown, no per-subagent rows.

**b. Consumption wiring needed**

_Per-tool rows:_

- Add `toolStats map[string]toolStat` (struct: `calls int, errors int, durationMs int64`) to `telemetryPanel`.
- Extend `accumulate(ev)` for `EventToolStart`: bucket `ev.ToolName`. For `EventToolEnd`: bucket `ev.ToolName`, add `ev.DurationMs`, count errors.
- `Render` appends per-tool rows (cap at 5, show "+N more" if > 5).
- No new handleBusEvent case — existing `EventToolStart`/`EventToolEnd` cases at screen_chat.go:180-186 and 211-216 already call `tp.accumulate(ev)`.

_Per-subagent rows (live bucket + authoritative total):_

- Add `subagentStats map[string]subagentStat` (struct: `tokens int, done bool`) to `telemetryPanel`.
- Two new handleBusEvent cases:
  1. `EventTokensUsage` when `ev.Meta["subagent_id"] != ""`: bucket live token accumulation by subagent_id. Already inside the existing `EventTokensUsage` copyRailWith block at screen_chat.go:291 — add a second `if sa, ok := panels[panelTelemetry]...` branch (or share the same copyRailWith call).
  2. `EventSubagentCompleted` (NEW case): parse `ev.Meta["tokens"]` as int; overwrite bucket with authoritative total; set `done=true`. Also add `EventSubagentFailed` as no-op or mark failed.
- `Render` appends per-subagent rows (capped at 3, truncated ID).

_Exact wiring for the new subagent case:_

```
subagent_manager.go emits EventSubagentCompleted{Meta["tokens"]}
  → bus → evCh → pumpEvents → busEventMsg
    → handleBusEvent NEW case EventSubagentCompleted (screen_chat.go)
      → copyRailWith → tp.accumulate(ev) [parse tokens from Meta]
        → Render reads p.subagentStats
```

**c. In `panelsFor(screenChat)`?** Yes: `panels.go:39`.

**d. COW pattern** — existing `copyRailWith` calls for EventToolStart/EventToolEnd/EventTokensUsage. New EventSubagentCompleted case follows the same `copyRailWith` pattern.

**e. Verdict: MODIFY. Needs ~80 LOC (toolStats struct + per-tool accumulate + subagentStats struct + per-subagent accumulate + new EventSubagentCompleted case in handleBusEvent + updated Render with both breakdowns).**

---

### Panel 4 — `memoryPeekPanel`

**a. Current skeleton state**
Does NOT exist. No `panelMemoryPeek` constant in `panels.go`, no panel struct in `rail_panels.go`, no `panelsFor` entry, no `newRail` entry. Zero scaffolding.

**b. Consumption wiring needed — NEW PANEL, full build**

This panel mirrors the `todolistPanel` cmd-refresh pattern exactly, triggered by `EventMemoryChanged` instead of `EventTodolistChanged`.

_Step 1 — panelID constant (`panels.go`):_

```go
panelMemoryPeek panelID = "memory-peek" // chat
```

Add to `panelsFor(screenChat)`: `{panelTodolist, panelContextMeter, panelTelemetry, panelMemoryPeek}`.
**This change requires updating `model_test.go:TestPanelsFor_ContractMatrix`** (the frozen `screenChat` row).

_Step 2 — panel struct (`rail_panels.go`):_

```go
type memoryPeekPanel struct {
    styles  tuiStyles
    entries []store.MemoryEntry
}
func newMemoryPeekPanel(s tuiStyles) *memoryPeekPanel
func (p *memoryPeekPanel) setEntries(entries []store.MemoryEntry)
func (p *memoryPeekPanel) Render(width, _ int) string  // "" when len(entries)==0
```

`Render` shows the last N (cap=5) entries: `entry.Content` truncated per line, header "memory" badge.

_Step 3 — fetch cmd (`rail_panels_cmd.go`):_

```go
type memoryRefreshMsg struct { entries []store.MemoryEntry }

func fetchMemory(st store.Store, scopeID string) tea.Cmd {
    return func() tea.Msg {
        if st == nil || scopeID == "" { return memoryRefreshMsg{} }
        entries, _ := st.SearchMemory(context.Background(), scopeID, "", 5)
        return memoryRefreshMsg{entries: entries}
    }
}
```

`scopeID` = `ev.Meta["scope_id"]` from `EventMemoryChanged`. The `Model` already holds `store store.Store` (model.go:150) — no new injection needed.

_Step 4 — handleBusEvent new case (`screen_chat.go`):_

```go
case notify.EventMemoryChanged:
    cmds = append(cmds, fetchMemory(m.store, ev.Meta["scope_id"]))
```

Mirrors the `EventTodolistChanged` case at screen_chat.go:304-308 exactly.

_Step 5 — Update handler (`model.go`):_

```go
case memoryRefreshMsg:
    m.rail = copyRailWith(m.rail, func(panels map[panelID]Panel) {
        if mp, ok := panels[panelMemoryPeek].(*memoryPeekPanel); ok {
            cp := *mp
            cp.setEntries(msg.entries)
            panels[panelMemoryPeek] = &cp
        }
    })
    return m, nil
```

Mirrors the `todolistRefreshMsg` case at model.go:288-296 exactly.

_Step 6 — newRail wiring (`rail.go`):_
Add `panelMemoryPeek: newMemoryPeekPanel(s)` to `newRail`.

_Exact wiring chain:_

```
curator/consolidator/MemoryToolDeps emits EventMemoryChanged{Meta[scope_id, entry_id]}
  → bus → evCh → pumpEvents → busEventMsg
    → handleBusEvent NEW case EventMemoryChanged (screen_chat.go)
      → fetchMemory(m.store, ev.Meta["scope_id"]) tea.Cmd
        → st.SearchMemory(ctx, scopeID, "", 5) [goroutine]
          → memoryRefreshMsg{entries}
            → model.Update case memoryRefreshMsg
              → copyRailWith → mp.setEntries(entries)
                → Render reads p.entries
```

**c. In `panelsFor(screenChat)`?** NOT YET. Adding it requires:

- `panels.go`: add `panelMemoryPeek` to `screenChat` slice.
- `model_test.go:TestPanelsFor_ContractMatrix`: update the `screenChat` row.
- `rail_panels_test.go`: update `TestRailWiring_ChatPanelsRegistered` (line 329) to include `panelMemoryPeek`.

**d. COW pattern** — `copyRailWith` in the `memoryRefreshMsg` handler follows the same pattern as `todolistRefreshMsg`. No mutation outside Update.

**e. Verdict: NEW PANEL. Needs ~100 LOC (panelID constant, panel struct, fetch cmd, handleBusEvent case, model.Update case, newRail entry, test updates).**

---

## Consumption-wiring canonical pattern

The authoritative event→msg→Model-field→panel pattern (evidence: todolist, the fully-wired precedent):

```
1. notify.Bus.Emit(Event)                          [agent loop / subagent_manager]
2.   → bus subscriber (events.go:57-73)            [thin non-blocking send to evCh]
3.       → evCh chan tea.Msg (cap 256)
4.           → pumpEvents tea.Cmd (events.go:39-43) [blocking receive, one Msg per call]
5.               → bubbletea runtime → Model.Update()
6.                   → case busEventMsg → handleBusEvent(ev) (screen_chat.go:148)
7.                       → EITHER: copyRailWith → panel copy → panel.accumulate(ev)
8.                       → OR:     issue tea.Cmd (fetchTodolist / fetchMemory) → delivers Refresh Msg
9.                           → Model.Update() case RefreshMsg → copyRailWith → panel.setXxx()
10.                              → Model.View() → rail.Render() → panel.Render()
11.                                   reads cached field only; NO live calls
```

**Precedent file:lines:**

- Steps 2-4: `internal/tui/events.go:39-73`
- Steps 5-6: `internal/tui/model.go:334-354` (global busEventMsg), `internal/tui/screen_chat.go:148-314`
- Step 7 (accumulate): `screen_chat.go:180-186` (EventToolStart), `291-301` (EventTokensUsage)
- Step 8-9 (cmd pattern): `screen_chat.go:304-308` (EventTodolistChanged→fetchTodolist), `model.go:286-296` (todolistRefreshMsg)
- Step 10: `internal/tui/rail.go:64-77` (copyRailWith)
- Step 11: `rail_panels.go:163-199` (todolistPanel.Render — pure field read)

**Accumulate vs. Cmd-refresh decision rule:**

- Use **accumulate** (steps 7) when the full data is carried in the event (EventTokensUsage, EventToolStart/End).
- Use **Cmd-refresh** (steps 8-9) when the event is a bare signal and the data must be fetched from a store (EventTodolistChanged → TodoListForConv; EventMemoryChanged → SearchMemory).

---

## PR-slice recommendation (chained, ≤400 lines each)

**PR-a: context-meter real data** (~80 LOC impl + ~60 LOC tests = ~140 lines)

- `contextMeterPanel`: add `limit, sysToks, msgToks, toolToks int` fields; change `accumulate` to REPLACE semantics; update `Render` with per-category rows and real limit.
- `run.go`: read `ag.ContextWindowSize()` at boot, wire into panel via `copyRailWith`.
- Tests: accumulate-REPLACE semantics, boot-limit wiring, golden Render.
- No handleBusEvent changes (existing `EventTokensUsage` case already calls accumulate).

**PR-b: telemetry per-tool and per-subagent rows** (~140 LOC impl + ~100 LOC tests = ~240 lines)

- `telemetryPanel`: add `toolStats map[string]toolStat`; extend `accumulate` for per-tool bucketing; add `subagentStats map[string]subagentStat`; extend `accumulate` for `EventSubagentCompleted`.
- `screen_chat.go`: new `case notify.EventSubagentCompleted` in `handleBusEvent`.
- Tests: per-tool accumulation, subagent authoritative-total overwrite, Render cap behavior.

**PR-c: memory-peek new panel** (~160 LOC impl + ~100 LOC tests = ~260 lines)

- New `panelMemoryPeek` constant in `panels.go`; update `panelsFor(screenChat)`.
- New `memoryPeekPanel` struct + Render in `rail_panels.go`.
- New `fetchMemory` cmd + `memoryRefreshMsg` in `rail_panels_cmd.go`.
- New `case notify.EventMemoryChanged` in `screen_chat.go:handleBusEvent`.
- New `case memoryRefreshMsg` in `model.go:Update`.
- `newRail` entry in `rail.go`.
- Test updates: `TestPanelsFor_ContractMatrix` (model_test.go), `TestRailWiring_ChatPanelsRegistered` (rail_panels_test.go); new panel tests.

**Todolist: already done — zero work.**

**Total estimated across all three PRs: ~640 lines.** Each individual PR is well within the ≤400-line budget. Chain order: PR-a → PR-b → PR-c (independent; can also land in parallel since they touch distinct panel structs, but PR-c has the most test-contract impact).

---

## Open questions for the proposal phase

1. **context-meter REPLACE semantics migration** — The current `accumulate` for `contextMeterPanel` ADDS `ev.TokenCount` across turns (delta model). Switching to REPLACE semantics (snapshot the latest `SysToks+MsgToks+ToolToks`) changes the displayed value from "cumulative output tokens" to "current context fill". The bar becomes significantly more accurate but the displayed number will drop between turns when a compaction fires (`EventContextCompacted`). Should the panel show the current fill OR a max-seen-this-session high watermark?

2. **context-meter fallback when `SysToks/MsgToks/ToolToks` are all zero** — The category fields are zero when the legacy/none context strategy runs (loop.go:1033: `if a.contextMgr != nil`). In that case the panel must fall back to the original `ev.TokenCount` delta accumulator. The `accumulate` method needs a branch: if category fields are all 0, accumulate `ev.TokenCount` as before; else REPLACE with the category snapshot. Proposal phase must specify this clearly.

3. **telemetry per-tool cap display** — Cap at 5 visible tool rows with a "+N more" line, or show all and truncate the panel height? The rail has no scrolling (View=pure), so a hard cap with a summary line is the only option. Decide the cap number and the summary format.

4. **memory-peek `scopeID` source** — `EventMemoryChanged.Meta["scope_id"]` is the scope at the point of the `AppendMemory` call. The TUI `Model` does not currently track the agent's scope ID directly — it uses `activeConvID` for the todolist. `SearchMemory(ctx, scopeID, "", 5)` requires the correct scope. Where does the TUI get the scope for the initial fetch (boot) vs. subsequent refreshes (EventMemoryChanged carries it)? This is the single biggest open design question (see below).

5. **memory-peek initial boot fetch** — Unlike the todolist (which waits for `EventTodolistChanged` before first fetch), should the memory panel pre-fetch on `EventTurnStarted` or on the first `EventMemoryChanged`? Starting with an empty panel until the first memory write is fine for V1 but may surprise users with existing memories. Decide in proposal.

6. **`panelsFor(screenChat)` order with 4 panels** — Adding `panelMemoryPeek` to the chat rail makes 4 panels. At `railWidth=32`, four stacked bordered panels may overflow available screen height on short terminals. The proposal should specify the panel order and whether any panel is conditionally excluded below a height threshold.

7. **`model_test.go:TestPanelsFor_ContractMatrix`** — This is a frozen contract test (AD-6). Updating the `screenChat` row to include `panelMemoryPeek` is required but is a deliberate break-then-fix. Proposal should call this out as an explicit task with the "update contract test" step.

---

## Summary: what changed from the original exploration

| Panel         | Was (pre-seams)                            | Now (post-seams)                                                   |
| ------------- | ------------------------------------------ | ------------------------------------------------------------------ |
| todolist      | DONE                                       | DONE (unchanged)                                                   |
| context-meter | PARTIAL (hardcoded 200k, no categories)    | UNBLOCKED (ContextWindowSize + SysToks/MsgToks/ToolToks available) |
| telemetry     | PARTIAL (aggregate done, subagent blocked) | UNBLOCKED (EventSubagentCompleted.Meta["tokens"] available)        |
| memory-peek   | BLOCKED (no EventMemoryChanged)            | UNBLOCKED (EventMemoryChanged + store.SearchMemory path confirmed) |
