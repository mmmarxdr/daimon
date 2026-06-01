# Proposal: tui-rail-panels

> Phase 2 of the daimon embedded-TUI design alignment.
> Consumes the `tui-backend-seams` contract (LIVE on main=ccffc6f).
> **TUI-only change.** No `internal/agent` / `internal/notify` source edits.

---

## Intent

Wire the embedded TUI's chat-rail panels to consume the four backend seams
that `tui-backend-seams` just published, so the rail shows live, accurate data
instead of skeleton placeholders:

- **context-meter** — render from the real context-window limit
  (`Agent.ContextWindowSize()`) and the per-category fill snapshot
  (`SysToks` / `MsgToks` / `ToolToks` on `EventTokensUsage`), with graceful
  fallback to the aggregate token count.
- **telemetry** — add per-tool rows (from existing `EventToolStart`/`End`)
  and per-subagent rows (live `EventTokensUsage[subagent_id]` bucket +
  authoritative `EventSubagentCompleted.Meta["tokens"]`).
- **memory-peek** — a NEW 4th chat-rail panel that mirrors the todolist
  cmd-refresh precedent, triggered by `EventMemoryChanged`.
- **todolist** — already fully wired; ZERO work.

This is a **data-wiring change, not a visual restyle.** Every panel remains a
pure `Render(width, height int) string` reading only CACHED `Model` fields.
Live backend data flows ONLY through `notify.Bus → tea.Msg → Update → Model
field`, and panel updates go through the copy-on-write `copyRailWith` pattern
(`rail.go:70`). No live reads in any `Render` path. Charm v1 only; ANSI width
via `x/ansi`; centralized `tuiStyles` (no inline hex).

---

## Scope

### In

- `contextMeterPanel` REPLACE-semantics rewrite + boot-time real limit
  (`rail_panels.go:321-377`, `run.go` boot).
- `telemetryPanel` per-tool accumulator + per-subagent rows + one new
  `handleBusEvent` case `EventSubagentCompleted` (`rail_panels.go:76-139`,
  `screen_chat.go`).
- NEW `memoryPeekPanel` panel: `panelID` constant, struct + `Render`, fetch
  cmd + msg, `handleBusEvent` case `EventMemoryChanged`, `Update` case,
  `newRail` entry, and the two contract-test updates.
- Unit tests (synthetic events → `handleBusEvent` → assert `Model` field) and
  golden render tests for each panel.

### Out (deferred to a later Phase-2 _visual_ change)

- Rail boxing/borders restyle; real per-panel rail height clamping.
- Input hint-chips; sessions search / columns / model-picker interactivity.
- Topbar per-slot colors; diff / approval screens.
- Thread-item event timestamps (called out in seams spec as a separate tiny
  TUI change; NOT bundled here).
- Any `internal/agent` / `internal/notify` edit. If a tiny accessor turns out
  unavoidable during apply, STOP and flag it — the design intentionally avoids
  adding backend surface just after `tui-backend-seams` archived.

---

## Capabilities

- **NEW capability `tui-rail-panels`** — the chat-rail data-consumption
  contract: which event populates which panel field, REPLACE-vs-accumulate
  semantics per field, and the pure-Render / COW invariants.
- **Consumes (does not modify) `tui-backend-seams`** — all four seams are read
  exactly as that spec publishes them; no spec change there.
- No other existing TUI spec is modified; the AD-6 panel-contract matrix is
  _extended_ (one new `screenChat` entry), not redefined.

---

## Approach — the six decisions

### Decision 1 — PR slicing (delivery_strategy = ask-on-risk, ≤400 lines each)

Confirmed: **three chained PRs**, each independently shippable and green. They
touch distinct panel structs, so they _could_ land in parallel, but PR-c
carries the contract-test churn, so the recommended chain is **a → b → c**.

| Slice                  | Scope                                                                        | Files                                                                                                                                | ~LOC | depends-on        |
| ---------------------- | ---------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------ | ---- | ----------------- |
| **PR-a** context-meter | REPLACE semantics + real limit + category sub-bars + boot wiring             | `rail_panels.go` (contextMeterPanel), `run.go`, `rail_panels_test.go`                                                                | ~140 | —                 |
| **PR-b** telemetry     | per-tool accumulator + per-subagent rows + new `EventSubagentCompleted` case | `rail_panels.go` (telemetryPanel), `screen_chat.go`, `rail_panels_test.go`                                                           | ~240 | PR-a (chain only) |
| **PR-c** memory-peek   | new panel end-to-end + 2 contract-test updates                               | `panels.go`, `rail_panels.go`, `rail_panels_cmd.go`, `screen_chat.go`, `model.go`, `rail.go`, `model_test.go`, `rail_panels_test.go` | ~260 | PR-b (chain only) |

Each PR is well under the 400-line budget. No `size:exception` needed.
Apply order is the chain order; verify after each.

### Decision 2 — context-meter REPLACE semantics + graceful fallback

**Current state** (`rail_panels.go:321-377`): `accumulate` ADDS `ev.TokenCount`
across turns (delta accumulator) against a hardcoded `const contextLimit =
200_000`; label hardcoded `"%.1f%% of 200k"`.

**Change:**

- Add fields to `contextMeterPanel`: `limit int`, `sysToks, msgToks, toolToks
int`. Keep `tokenUsed int` and `hasData bool`.
- **Boot-time real limit** — in `run.go` (after `r := newRail(s)`, inside the
  existing `copyRailWith` block at `run.go:78-98`), read
  `ag.ContextWindowSize()` ONCE and construct/replace the panel with the limit
  threaded in (add `newContextMeterPanel(s, limit int)` OR a `setLimit(n int)`
  method invoked through `copyRailWith`). This mirrors how `panelModelPicker`
  and `panelActivePolicy` are already replaced in that same block. Static value
  read once at construction — never re-read.
- **0-sentinel** — when `ContextWindowSize()` returns `0` (nil contextMgr), the
  panel keeps the `200_000` heuristic, and the percentage label is suffixed
  `est.` to signal it is a heuristic, not the real window.
- **REPLACE semantics in `accumulate`** — each `EventTokensUsage` is a
  _snapshot_, not a delta:
  - `if ev.SysToks+ev.MsgToks+ev.ToolToks > 0` → OVERWRITE
    `p.sysToks/msgToks/toolToks` with the event values, and set
    `p.tokenUsed = ev.SysToks+ev.MsgToks+ev.ToolToks` (current fill).
  - `else` (legacy / `none` strategy → category fields are `0`) → FALL BACK to
    the original behavior: `p.tokenUsed += ev.TokenCount` and leave category
    fields at `0` so no sub-bars render.
  - `p.hasData = true` in both branches.
- **Render** — when `p.sysToks > 0`, show the total bar against `p.limit` plus
  three category sub-bars (sys / msg / tool). When category fields are `0`,
  render ONLY the aggregate bar from `p.tokenUsed` (today's behavior), hiding
  sub-bars. The bar/percentage use `p.limit` (fallback `200_000`).
- **No new `handleBusEvent` case** — the existing `EventTokensUsage` block
  (`screen_chat.go:291-302`) already calls `cm.accumulate(ev)` via
  `copyRailWith`. Only the struct + `run.go` boot change.

_Resolved sub-question (exploration Q1):_ the panel shows **current fill**, not
a session high-watermark. REPLACE semantics means the value drops after a
compaction (`EventContextCompacted`) — that is correct and desirable, because
the bar must reflect what is actually in the window now. No watermark in V1.

### Decision 3 — telemetry sourcing (per-tool + per-subagent)

**Current state** (`rail_panels.go:76-139`): aggregate `totalIn`, `totalCost`,
`toolCalls`, `toolErrors` only.

**Per-tool rows (ACCUMULATE — event carries the data):**

- Add `toolStats map[string]toolStat` where
  `toolStat{calls int; errors int; durationMs int64}`.
- Extend `accumulate`: on `EventToolStart` bucket `ev.ToolName` (`calls++`);
  on `EventToolEnd` bucket `ev.ToolName` (`+= ev.DurationMs`, `errors++` when
  `ev.IsError`). These accumulate over the session.
- No new wiring point — existing `EventToolStart`/`End` cases
  (`screen_chat.go:180-186`, `211-217`) already call `tp.accumulate(ev)`.

**Per-subagent rows (live ACCUMULATE bucket + authoritative REPLACE total):**

- Add `subagentStats map[string]subagentStat` where
  `subagentStat{tokens int; done bool}`.
- **Live bucket (ACCUMULATE):** on `EventTokensUsage` where
  `ev.Meta["subagent_id"] != ""`, accumulate
  `atoiSafe(ev.Meta["input_tokens"]) + atoiSafe(ev.Meta["output_tokens"])`
  into that subagent's bucket. This rides the EXISTING `EventTokensUsage`
  `copyRailWith` block at `screen_chat.go:291-302` — the same `accumulate(ev)`
  call already runs for telemetry; the new branch lives inside `accumulate`.
- **Authoritative total (REPLACE):** on a **NEW `handleBusEvent` case
  `EventSubagentCompleted`**, parse `ev.Meta["tokens"]` (guaranteed present,
  `"0"` minimum per the seams spec) and OVERWRITE the bucket with that
  authoritative value, set `done = true`. The new case follows the existing
  `copyRailWith` pattern. (`EventSubagentFailed` is a no-op in V1, or marks the
  row failed — design phase decides the marker; functionally a no-op is safe.)
- Both `EventSubagentCompleted` (`events.go:38`) and `EventTokensUsage` are in
  `KnownEventTypes` and travel on the bus → reach `handleBusEvent`. Verified.

**Render caps/ordering:** per-tool rows capped at **5**, with a `"+N more"`
summary line when exceeded (the rail has no scroll — `View=pure` — so a hard
cap is the only option). Per-subagent rows capped at **3** with truncated IDs.
Ordering: insertion/first-seen order is acceptable for V1 (deterministic for
golden tests); design phase MAY sort by tokens desc.

### Decision 4 — memory-peek (NEW panel) + scopeID resolution

The panel does not exist (no `panelMemoryPeek`, no struct, no `panelsFor`
entry, no `newRail` entry). Full build mirroring the todolist precedent:

1. **`panels.go`** — add `panelMemoryPeek panelID = "memory-peek"`; append it
   to `panelsFor(screenChat)` → `{panelTodolist, panelContextMeter,
panelTelemetry, panelMemoryPeek}` (placed LAST so it is the bottom-most
   panel — see height risk).
2. **`rail_panels.go`** — `type memoryPeekPanel struct { styles tuiStyles;
entries []store.MemoryEntry }`, `newMemoryPeekPanel(s tuiStyles)`,
   `setEntries([]store.MemoryEntry)`, `Render(width, _ int) string` returning
   `""` when `len(entries)==0`. Render shows a `memory` header badge + the last
   N (cap **5**) entries' `Title` (fallback truncated `Content`), ANSI-truncated
   per line via `wrapPanelBox`.
3. **`rail_panels_cmd.go`** — `memoryRefreshMsg{ entries []store.MemoryEntry }`
   and `fetchMemory(st store.Store, scopeID string) tea.Cmd` calling
   `st.SearchMemory(ctx, scopeID, "", 5)` (signature confirmed at
   `store.go` / `filestore.go:235`); no-op zero msg when `st==nil` or
   `scopeID==""`. `Model.store` already exists (`model.go:150`).
4. **`screen_chat.go` handleBusEvent** — NEW case
   `case notify.EventMemoryChanged: cmds = append(cmds, fetchMemory(m.store,
ev.Meta["scope_id"]))`. Mirrors the `EventTodolistChanged` case
   (`screen_chat.go:304-308`).
5. **`model.go` Update** — NEW `case memoryRefreshMsg:` using `copyRailWith` to
   set entries on `panelMemoryPeek`, mirroring `todolistRefreshMsg`
   (`model.go:288-296`).
6. **`rail.go` newRail** — add `panelMemoryPeek: newMemoryPeekPanel(s)`.
7. **Contract tests** — update `TestPanelsFor_ContractMatrix` (`model_test.go`,
   the frozen `screenChat` row) and `TestRailWiring_ChatPanelsRegistered`
   (`rail_panels_test.go:317`). These are _deliberate break-then-fix_ steps.

**scopeID question — RESOLVED: option (a) — start empty, populate on first
`EventMemoryChanged`.**

- Rationale: SIMPLEST, PURE, and requires **no backend change** (option (b),
  `Agent.ScopeID()`, would add backend surface immediately after
  `tui-backend-seams` archived — discouraged by the change's constraint).
  Option (c) deriving from `activeConvID` is unreliable: a memory scope is NOT
  the conversation ID (`MemoryEntry.ScopeID` is distinct from
  `MemoryEntry.Source`, which IS the conversation ID — `store.go:63,69`), so
  `activeConvID` would query the wrong scope.
- `EventMemoryChanged.Meta["scope_id"]` carries the exact scope at write time —
  it is the authoritative source for both the trigger AND the fetch scope.
- **Accepted trade-off:** the panel shows nothing until the first memory write
  of the session. Existing memories are NOT pre-fetched at boot. This is
  acceptable for V1 (matches the todolist's "empty until first change"
  behavior) and naturally defers the 4th panel's height cost (see risks).

### Decision 5 — Scope cut (data-wiring, not restyle)

Explicitly stated above (Scope/Out). This change wires DATA into existing
panel skeletons and adds one new data panel. It does NOT restyle borders,
add chips, or touch other screens. The visual restyle is a separate later
Phase-2 change. No `internal/agent` / `internal/notify` edits.

### Decision 6 — Test strategy (strict TDD, `make test`)

- **Data caching is unit-testable** by feeding synthetic `notify.Event`s
  through `handleBusEvent` and asserting the resulting `Model` rail-panel
  field — exactly how `TestRailWiring_*` and the todolist tests already work
  (`rail_panels_test.go`). For PR-c, the cmd path (`EventMemoryChanged →
memoryRefreshMsg`) is tested by driving the message through `Update` with a
  fake `store.Store` returning canned entries.
- **Golden render tests** for each panel's `Render` output, using the existing
  golden-test infrastructure in `internal/tui` (`go-testing` patterns:
  teatest / golden files). New golden files: context-meter with categories,
  context-meter fallback (no categories), telemetry with tool+subagent rows,
  memory-peek populated.
- Strict TDD: write the failing test FIRST for each behavior, then implement.

---

## Affected Areas

| File                              | Change                                                                      |
| --------------------------------- | --------------------------------------------------------------------------- |
| `internal/tui/rail_panels.go`     | contextMeterPanel + telemetryPanel rewrites; new memoryPeekPanel            |
| `internal/tui/rail_panels_cmd.go` | new `memoryRefreshMsg` + `fetchMemory`                                      |
| `internal/tui/screen_chat.go`     | new `EventSubagentCompleted` + `EventMemoryChanged` cases in handleBusEvent |
| `internal/tui/model.go`           | new `memoryRefreshMsg` case in Update                                       |
| `internal/tui/panels.go`          | new `panelMemoryPeek` const + `panelsFor(screenChat)` entry                 |
| `internal/tui/rail.go`            | `newRail` memory-peek entry                                                 |
| `internal/tui/run.go`             | boot-time `ContextWindowSize()` read → context-meter limit                  |
| `internal/tui/*_test.go`          | contract-matrix + wiring tests; new panel unit + golden tests               |

No source files outside `internal/tui` are modified.

---

## Risks

1. **4th-panel height overflow (real).** `rail.Render` (`rail.go:42-57`) stacks
   every populated panel box with `out += s + "\n"` and NEVER clamps to the
   `height` param; panels ignore the height arg entirely; `layout.go:138`
   `JoinHorizontal(lipgloss.Top, …)` expands the row to the tallest column.
   On a short terminal, 4 fully-populated panels can push the main row past
   `centerHeight` and overflow. **Mitigation:** each chat panel returns `""`
   until it has data, and memory-peek (option a) stays empty until the first
   memory write — so the 4th panel adds zero height until then. Real rail
   height clamping is OUT of scope (visual change). Saved to engram (id 603).
2. **REPLACE-not-accumulate traps.** context-meter category fields and the
   subagent authoritative total MUST overwrite, not add. Mixing the two (e.g.
   `+=` on `SysToks`) silently inflates the bar. Unit tests assert REPLACE by
   sending two events and checking the second value wins.
3. **Legacy/none strategy → zero category fields.** If the fallback branch is
   missed, the context bar shows `0%` whenever the smart strategy did not run.
   The explicit `if sum > 0 … else TokenCount` branch (Decision 2) is mandatory
   and golden-tested.
4. **pure-Model violations.** No `Render` may read the store, the agent, or the
   clock. `fetchMemory` does the `SearchMemory` IO inside a `tea.Cmd`
   goroutine; `Render` only reads `p.entries`. Reviewed against the todolist
   precedent.
5. **COW correctness.** Every panel mutation MUST go through `copyRailWith` with
   a value-copy of the panel (`cp := *p`) before `setXxx`, so prior `Model`
   snapshots are never mutated (windowed-viewport history depends on this).
6. **Contract-test break-then-fix.** Adding `panelMemoryPeek` deliberately
   breaks `TestPanelsFor_ContractMatrix` and `TestRailWiring_ChatPanelsRegistered`;
   both updates are explicit tasks, not accidents.

---

## Decisions (resolved checklist)

- [x] PR slicing: 3 chained PRs (a context-meter ~140, b telemetry ~240,
      c memory-peek ~260), chain order a→b→c, each ≤400 and green.
- [x] context-meter: REPLACE category snapshot; fallback to `TokenCount`
      aggregate (hide sub-bars) when categories are 0; real limit from
      `ContextWindowSize()` read once in `run.go`; 0 → keep 200k labeled `est.`;
      show current fill (no watermark).
- [x] telemetry: per-tool ACCUMULATE (cap 5 + "+N more"); per-subagent live
      ACCUMULATE bucket by `Meta["subagent_id"]` + authoritative REPLACE on new
      `EventSubagentCompleted` case (cap 3).
- [x] memory-peek: NEW `panelMemoryPeek`, placed last in `panelsFor(screenChat)`;
      `EventMemoryChanged → fetchMemory(store, scope_id) → memoryRefreshMsg →
    copyRailWith`; scopeID from `ev.Meta["scope_id"]` (option a — empty until
      first write, no backend change).
- [x] scope cut: data-wiring only; no restyle; no agent/notify edits.
- [x] test strategy: synthetic-event unit tests through handleBusEvent +
      golden render tests; strict TDD.

**Left to spec/design:**

- Exact `EventSubagentFailed` treatment (no-op vs. failed marker) and
  per-subagent row sort order (insertion vs. tokens-desc).
- Exact category sub-bar visual format (3 stacked mini-bars vs. one segmented
  bar) — golden-test-driven in design.
- Whether memory-peek shows `Title` or truncated `Content` per row (design;
  `Title` preferred, `Content` fallback).

---

## Rollback

Each PR is an isolated, additive TUI change reverting cleanly:

- PR-a revert → context-meter returns to the delta-accumulator + 200k hardcode.
- PR-b revert → telemetry returns to aggregate-only; the
  `EventSubagentCompleted` case is removed (event is harmlessly ignored).
- PR-c revert → `panelMemoryPeek` removed from `panels.go`/`newRail`; the
  `EventMemoryChanged` case becomes a no-op fall-through (event ignored); the
  two contract tests revert to the 3-panel `screenChat` row.

No backend, store, or bus state is touched, so any revert is a pure code revert
with no migration.

---

## Success Criteria

- context-meter renders the real window limit and per-category sub-bars under
  the smart strategy, and gracefully renders the aggregate-only bar (no panic,
  no `0%`) under legacy/none.
- telemetry shows per-tool rows (≤5 + "+N more") and per-subagent rows (≤3)
  with the authoritative total after `EventSubagentCompleted`.
- memory-peek appears as the 4th chat-rail panel and populates from the first
  `EventMemoryChanged` of the session; empty (zero-height) before that.
- All four panels obey `View=pure(Model)`: no live reads in `Render`, all
  mutations via `copyRailWith`.
- `make test` green, including updated contract tests and new golden files.
- No file outside `internal/tui` is modified.

---

## SDD session config

- **artifact_store:** openspec
- **exec:** Automatic
- **delivery_strategy:** ask-on-risk
- **strict TDD:** enabled (`make test`)
- **Charm:** v1 only (bubbletea/lipgloss v1, `x/ansi` width math)
