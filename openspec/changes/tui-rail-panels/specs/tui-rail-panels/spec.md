# TUI Rail Panels Specification

## Purpose

Wires the embedded TUI's three existing chat-rail panel skeletons
(`contextMeterPanel`, `telemetryPanel`) to consume the backend seams
delivered by `tui-backend-seams`, and introduces a fourth new panel
(`memoryPeekPanel`). After this change the rail shows live, accurate data
instead of skeleton placeholders.

This is a **data-wiring change only**. Panel rendering remains a pure
function of cached `Model` fields. No visual restyle (borders, chips,
row heights) is in scope. No files outside `internal/tui` are modified.

Consumes `tui-backend-seams` (archived 2026-05-31). Does not modify the
backend seams spec or any backend source.

---

## Architectural invariants (cross-cutting — apply to all requirements)

These invariants must hold across every panel added or modified by this
change. They are testable and will be checked by the verify phase.

### Requirement TR-0-A: View is a pure function of Model

No panel `Render(width, height int) string` method MAY perform any of the
following while executing:

- read from a `store.Store`
- call a method on an `*agent.Agent` or any live object
- read the system clock
- perform any IO

A panel MUST read only its own cached struct fields.

Observable: calling `panel.Render(w, h)` twice with identical struct state
MUST return identical output.

#### Scenario: Render is deterministic

- GIVEN a panel whose struct fields have been set to a fixed state
- WHEN `Render(width, height)` is called twice in succession with the same
  arguments and no intervening field mutation
- THEN both calls return exactly the same string
- AND neither call panics

---

### Requirement TR-0-B: Panel mutations use copy-on-write

Every change to a rail panel's cached state MUST go through `copyRailWith`
in `rail.go`, producing a new `rail` value and leaving the previous `Model`
snapshot unaffected.

No panel field MUST be mutated directly on the `Model`'s embedded panel
pointer outside of a `copyRailWith` callback.

#### Scenario: COW leaves prior Model snapshot unaffected

- GIVEN a `Model` snapshot `m1` with `contextMeterPanel.tokenUsed == 0`
- WHEN an `EventTokensUsage` event is processed, producing a new model `m2`
  with `tokenUsed > 0`
- THEN `m1.rail` panels are unchanged (tokenUsed still 0)
- AND `m2.rail` panels reflect the updated value

---

### Requirement TR-0-C: Empty panel contributes zero height

A panel with no data MUST return `""` (empty string) from `Render`.
The rail stacks panels vertically; an empty string contributes zero rows
and does not overflow the column height.

#### Scenario: Empty panel renders empty

- GIVEN any panel struct in its zero/initial state (no events received)
- WHEN `Render(width, height)` is called
- THEN the return value is `""` (empty string)
- AND the call does not panic

---

### Requirement TR-0-D: ANSI width via x/ansi; centralized styles

All string-width calculations in Render paths MUST use `x/ansi` (not
`len(s)` or `utf8.RuneCountInString`). All style values (colors, borders)
MUST come from the centralized `tuiStyles` struct; no inline hex color
literals.

---

## PR-a: context-meter

### Requirement TR-1: Boot-time real context-window limit

`contextMeterPanel` MUST store a `limit int` field set once at TUI
construction. `run.go` MUST call `ag.ContextWindowSize()` after `newRail`
is constructed and wire the returned value into the panel via `copyRailWith`
(mirroring the existing `panelModelPicker` / `panelActivePolicy` boot
wiring in the same block).

When `ContextWindowSize()` returns `0`, the panel MUST fall back to the
internal heuristic value `200_000` and MUST suffix the percentage label
with ` est.` to signal the heuristic is in use. When a non-zero value is
returned the label MUST NOT include ` est.`.

The limit is static; it MUST NOT be re-read on subsequent turns.

#### Scenario: Non-zero limit stored and used

- GIVEN `ag.ContextWindowSize()` returns `128000`
- WHEN the TUI constructs `newRail` and runs the boot `copyRailWith` block
- THEN `contextMeterPanel.limit` equals `128000`
- AND a subsequent `Render` call uses `128000` as the meter's maximum
- AND the percentage label does NOT contain ` est.`

#### Scenario: Zero limit falls back to heuristic

- GIVEN `ag.ContextWindowSize()` returns `0`
- WHEN the TUI constructs `newRail` and runs the boot `copyRailWith` block
- THEN `contextMeterPanel.limit` equals `200000` (the heuristic constant)
- AND a subsequent `Render` call labels the percentage with ` est.`
  (e.g. `"14.2% est."`)

---

### Requirement TR-2: Per-category context fill — REPLACE semantics

`contextMeterPanel.accumulate(ev notify.Event)` MUST implement two
branches based on whether the incoming `EventTokensUsage` event carries
per-category data:

**Branch A — smart strategy (category fields present):**
When `ev.SysToks + ev.MsgToks + ev.ToolToks > 0`:

- OVERWRITE (not accumulate) `p.sysToks`, `p.msgToks`, `p.toolToks` with
  the event values.
- Set `p.tokenUsed = ev.SysToks + ev.MsgToks + ev.ToolToks`.
- Set `p.hasData = true`.

**Branch B — legacy / none strategy (all category fields zero):**
When `ev.SysToks + ev.MsgToks + ev.ToolToks == 0`:

- ADD `ev.TokenCount` to `p.tokenUsed` (existing delta behavior).
- Leave `p.sysToks`, `p.msgToks`, `p.toolToks` unchanged (remain `0`).
- Set `p.hasData = true`.

No new `handleBusEvent` case is required; the existing
`EventTokensUsage` case in `screen_chat.go` already calls
`cm.accumulate(ev)` via `copyRailWith`.

#### Scenario: REPLACE semantics — second event wins

- GIVEN a `contextMeterPanel` that has processed one `EventTokensUsage`
  with `SysToks=1000, MsgToks=2000, ToolToks=500`
- WHEN a second `EventTokensUsage` arrives with
  `SysToks=1200, MsgToks=1800, ToolToks=400`
- THEN `p.sysToks == 1200`
- AND `p.msgToks == 1800`
- AND `p.toolToks == 400`
- AND `p.tokenUsed == 3400` (sum of second event only)
- AND the first event's values are NOT present anywhere in the panel state

#### Scenario: Legacy fallback — tokenUsed accumulates

- GIVEN a `contextMeterPanel` that has processed one `EventTokensUsage`
  with `SysToks=0, MsgToks=0, ToolToks=0, TokenCount=500`
- WHEN a second `EventTokensUsage` arrives with
  `SysToks=0, MsgToks=0, ToolToks=0, TokenCount=300`
- THEN `p.tokenUsed == 800` (accumulated delta)
- AND `p.sysToks == 0`
- AND `p.msgToks == 0`
- AND `p.toolToks == 0`

---

### Requirement TR-3: Context-meter Render — category sub-bars conditional

`contextMeterPanel.Render(width, _ int)` MUST produce different output
depending on whether per-category data is present:

**When `p.sysToks > 0`** (smart strategy active):

- MUST render a total fill bar scaled to `p.limit`.
- MUST render three category sub-bars (sys / msg / tool), each scaled to
  `p.limit`.
- Sub-bar labels MUST identify the category (e.g. `sys`, `msg`, `tool`).

**When `p.sysToks == 0`** (legacy / none strategy):

- MUST render ONLY the aggregate bar from `p.tokenUsed` against `p.limit`.
- MUST NOT render any category sub-bar rows.

**When `p.hasData == false`** (no event yet):

- MUST return `""` (per TR-0-C).

#### Scenario: Smart strategy — sub-bars present

- GIVEN `p.sysToks=1000, p.msgToks=2000, p.toolToks=500, p.limit=128000,
p.hasData=true`
- WHEN `Render(32, 0)` is called
- THEN the output contains sys, msg, and tool sub-bar lines
- AND the output contains a total-fill bar
- AND the output does NOT contain ` est.`
- AND the render does not panic

#### Scenario: Legacy strategy — only aggregate bar

- GIVEN `p.sysToks=0, p.msgToks=0, p.toolToks=0, p.tokenUsed=42000,
p.limit=200000, p.hasData=true`
- WHEN `Render(32, 0)` is called
- THEN the output contains a single aggregate bar
- AND the output does NOT contain sub-bar labels (no `sys`, `msg`, `tool`
  category lines)
- AND the output contains ` est.` in the percentage label
  (because limit==200000 heuristic)
- AND the render does not panic

#### Scenario: No data — empty render

- GIVEN a freshly constructed `contextMeterPanel` with `p.hasData=false`
- WHEN `Render(32, 0)` is called
- THEN the return value is `""`

---

## PR-b: telemetry

### Requirement TR-4: Per-tool row accumulation

`telemetryPanel` MUST maintain a `toolStats map[string]toolStat` field
where `toolStat` holds:

- `calls int` — incremented once per `EventToolStart` for that tool name.
- `errors int` — incremented once per `EventToolEnd` where `ev.IsError`.
- `durationMs int64` — accumulated from `ev.DurationMs` on `EventToolEnd`.

`accumulate(ev)` MUST update the appropriate bucket when the event type is
`EventToolStart` or `EventToolEnd`, keyed by `ev.ToolName`.

No new `handleBusEvent` case is required; the existing `EventToolStart`
and `EventToolEnd` cases in `screen_chat.go` already route through
`tp.accumulate(ev)`.

Accumulation is over the full session; buckets are never reset within a
session.

#### Scenario: Tool call counted on Start

- GIVEN a `telemetryPanel` with no prior events
- WHEN two `EventToolStart` events arrive for `ToolName="bash"`
- THEN `toolStats["bash"].calls == 2`
- AND `toolStats["bash"].errors == 0`

#### Scenario: Tool error and duration recorded on End

- GIVEN `telemetryPanel` has received one `EventToolStart` for `ToolName="read_file"`
- WHEN `EventToolEnd` arrives with `ToolName="read_file"`, `DurationMs=150`,
  `IsError=true`
- THEN `toolStats["read_file"].errors == 1`
- AND `toolStats["read_file"].durationMs == 150`

#### Scenario: Accumulation across multiple tools

- GIVEN three distinct tool names each fired once
- WHEN all three `EventToolStart` events are processed
- THEN `len(toolStats) == 3`
- AND each bucket has `calls == 1`

---

### Requirement TR-5: Per-tool Render cap at 5 with overflow summary

`telemetryPanel.Render` MUST display per-tool rows sorted by insertion
(first-seen) order. When `len(toolStats) <= 5`, all rows are shown. When
`len(toolStats) > 5`, exactly 5 rows are shown plus one summary line
`"+N more"` where N is the number of hidden tools. No scroll is provided.

#### Scenario: Five or fewer tools — all shown, no overflow line

- GIVEN `toolStats` with exactly 3 tools (A, B, C) each having 1 call
- WHEN `Render(32, 0)` is called
- THEN the output contains 3 tool-name rows
- AND the output does NOT contain "+N more"

#### Scenario: More than five tools — cap enforced

- GIVEN `toolStats` with 8 tools (A through H) each having 1 call
- WHEN `Render(32, 0)` is called
- THEN the output contains exactly 5 tool-name rows (the first 5 seen)
- AND the output contains "+3 more"

---

### Requirement TR-6: Per-subagent live token accumulation

`telemetryPanel` MUST maintain a `subagentStats map[string]subagentStat`
field where `subagentStat` holds:

- `tokens int` — running total, accumulated from live events.
- `done bool` — set when the authoritative final count arrives.

When processing an `EventTokensUsage` event where
`ev.Meta["subagent_id"] != ""`, the accumulate logic MUST add
`atoiSafe(ev.Meta["input_tokens"]) + atoiSafe(ev.Meta["output_tokens"])`
to the bucket keyed by `ev.Meta["subagent_id"]`.

This accumulation rides the existing `EventTokensUsage` `copyRailWith`
block in `screen_chat.go`; no new `handleBusEvent` case is required for
this path.

`atoiSafe` MUST return `0` for empty or non-numeric strings (no panic).

#### Scenario: Live accumulation from multiple EventTokensUsage events

- GIVEN a `telemetryPanel` receiving two `EventTokensUsage` events with
  `Meta["subagent_id"]="sa-abc"`, `Meta["input_tokens"]="100"`,
  `Meta["output_tokens"]="50"` each
- WHEN both events are processed
- THEN `subagentStats["sa-abc"].tokens == 300`
- AND `subagentStats["sa-abc"].done == false`

#### Scenario: atoiSafe handles missing or non-numeric Meta values

- GIVEN an `EventTokensUsage` event with `Meta["subagent_id"]="sa-x"`,
  `Meta["input_tokens"]=""`, `Meta["output_tokens"]="abc"`
- WHEN the event is processed
- THEN `subagentStats["sa-x"].tokens == 0`
- AND the call does NOT panic

---

### Requirement TR-7: Authoritative subagent total on EventSubagentCompleted

A NEW `handleBusEvent` case MUST be added for `notify.EventSubagentCompleted`
in `screen_chat.go`. This case MUST follow the existing `copyRailWith`
pattern.

When `EventSubagentCompleted` arrives, the telemetry panel's accumulate
logic MUST:

- Parse `ev.Meta["tokens"]` via `atoiSafe`.
- OVERWRITE `subagentStats[id].tokens` with the parsed value (REPLACE, not
  add).
- Set `subagentStats[id].done = true`.

The subagent ID MUST be read from `ev.Meta["subagent_id"]`. If
`ev.Meta["subagent_id"]` is empty, the event MUST be a no-op.

`EventSubagentFailed` is a no-op in V1; the bucket is left as-is (live
accumulated tokens remain, `done` stays `false`). The design phase MAY
add a `failed bool` marker to `subagentStat` but this is not required for
the spec.

#### Scenario: Authoritative total overwrites live accumulation

- GIVEN `subagentStats["sa-1"].tokens == 250` (from live accumulation)
  and `subagentStats["sa-1"].done == false`
- WHEN `EventSubagentCompleted` arrives with
  `Meta["subagent_id"]="sa-1"`, `Meta["tokens"]="405"`
- THEN `subagentStats["sa-1"].tokens == 405`
- AND `subagentStats["sa-1"].done == true`

#### Scenario: EventSubagentCompleted with empty subagent_id is no-op

- GIVEN a `telemetryPanel` with no prior subagentStats
- WHEN `EventSubagentCompleted` arrives with `Meta["subagent_id"]=""`
- THEN `subagentStats` remains empty
- AND no panic occurs

#### Scenario: EventSubagentCompleted for unseen subagent creates bucket

- GIVEN a `telemetryPanel` with empty `subagentStats`
- WHEN `EventSubagentCompleted` arrives with
  `Meta["subagent_id"]="sa-new"`, `Meta["tokens"]="120"`
- THEN `subagentStats["sa-new"].tokens == 120`
- AND `subagentStats["sa-new"].done == true`

---

### Requirement TR-8: Per-subagent Render cap at 3 with truncated IDs

`telemetryPanel.Render` MUST display per-subagent rows capped at 3.
Subagent IDs MUST be truncated to fit the panel width using ANSI-safe
truncation. When more than 3 subagents are present, the remaining rows
are silently omitted (no overflow summary line for subagents — only tool
rows get the "+N more" line). The design phase MAY change this to
"+N more" if preferred; the cap of 3 is fixed in this spec.

#### Scenario: Three or fewer subagents — all shown

- GIVEN `subagentStats` with 2 subagents
- WHEN `Render(32, 0)` is called
- THEN the output contains 2 subagent rows
- AND no fourth subagent row is present

#### Scenario: More than three subagents — cap enforced

- GIVEN `subagentStats` with 5 subagents
- WHEN `Render(32, 0)` is called
- THEN the output contains exactly 3 subagent rows
- AND no panic occurs

---

## PR-c: memory-peek

### Requirement TR-9: panelMemoryPeek panel ID registered in panelsFor(screenChat)

`panels.go` MUST define the constant:

```go
panelMemoryPeek panelID = "memory-peek"
```

`panelsFor(screenChat)` MUST return:

```go
[]panelID{panelTodolist, panelContextMeter, panelTelemetry, panelMemoryPeek}
```

`panelMemoryPeek` MUST be the last entry (bottom panel in the rail).

`rail.go`'s `newRail` function MUST register `panelMemoryPeek` →
`newMemoryPeekPanel(s)`.

The frozen contract tests `TestPanelsFor_ContractMatrix` (`model_test.go`)
and `TestRailWiring_ChatPanelsRegistered` (`rail_panels_test.go`) MUST be
updated to reflect the 4-panel `screenChat` row. These are deliberate
break-then-fix steps in the PR-c task list.

#### Scenario: panelsFor returns 4 entries for screenChat

- GIVEN the registered `panelsFor` mapping
- WHEN `panelsFor(screenChat)` is called
- THEN the returned slice has length 4
- AND the slice contains `panelMemoryPeek`
- AND `panelMemoryPeek` is the last element

#### Scenario: newRail registers memory-peek panel

- GIVEN `newRail(s tuiStyles)` is called
- WHEN the returned rail's panel map is inspected
- THEN `panels[panelMemoryPeek]` is non-nil
- AND it is of type `*memoryPeekPanel`

---

### Requirement TR-10: memoryPeekPanel struct and Render

`rail_panels.go` MUST define:

```go
type memoryPeekPanel struct {
    styles  tuiStyles
    entries []store.MemoryEntry
}
func newMemoryPeekPanel(s tuiStyles) *memoryPeekPanel
func (p *memoryPeekPanel) setEntries(entries []store.MemoryEntry)
func (p *memoryPeekPanel) Render(width, _ int) string
```

`Render` MUST:

- Return `""` when `len(p.entries) == 0` (per TR-0-C).
- When entries are present, render a header badge (e.g. `memory`) followed
  by up to 5 entry rows.
- Each row MUST display the entry's `Title` field. When `Title` is empty,
  the row MUST fall back to a truncated prefix of `Content`.
- Each row MUST be ANSI-truncated to `width` using `x/ansi`.
- Render MUST NOT call `store.SearchMemory` or any store method.

#### Scenario: Empty entries — Render returns ""

- GIVEN `p.entries` is nil or empty
- WHEN `Render(32, 0)` is called
- THEN the return value is `""`
- AND no panic occurs

#### Scenario: Populated entries — rows rendered

- GIVEN `p.entries` contains 3 `store.MemoryEntry` values each with a
  non-empty `Title`
- WHEN `Render(32, 0)` is called
- THEN the output contains all 3 entry titles
- AND the output contains the `memory` header badge
- AND no panic occurs

#### Scenario: Entries cap at 5

- GIVEN `p.entries` contains 8 `store.MemoryEntry` values
- WHEN `Render(32, 0)` is called
- THEN at most 5 entry rows are rendered
- AND no panic occurs

#### Scenario: Empty Title falls back to Content

- GIVEN `p.entries` contains one entry with `Title=""` and
  `Content="some content text"`
- WHEN `Render(32, 0)` is called
- THEN the output contains a truncated prefix of `"some content text"`
- AND the output does NOT contain an empty title row

---

### Requirement TR-11: fetchMemory cmd and memoryRefreshMsg

`rail_panels_cmd.go` MUST define:

```go
type memoryRefreshMsg struct { entries []store.MemoryEntry }

func fetchMemory(st store.Store, scopeID string) tea.Cmd
```

`fetchMemory` MUST:

- Return a `tea.Cmd` that, when executed, calls
  `st.SearchMemory(ctx, scopeID, "", 5)`.
- When `st == nil` OR `scopeID == ""`, return a cmd that immediately
  produces `memoryRefreshMsg{}` (empty entries) without calling
  `SearchMemory`.
- Ignore `SearchMemory` errors silently (return empty entries on error).
- Never panic regardless of input values.

`SearchMemory` MUST be called from within the `tea.Cmd` goroutine, NOT
from `handleBusEvent` or `Update` directly.

#### Scenario: Nil store produces empty msg without panic

- GIVEN `fetchMemory(nil, "scope-123")` is called
- WHEN the returned `tea.Cmd` is executed
- THEN a `memoryRefreshMsg{}` with empty entries is produced
- AND no panic occurs

#### Scenario: Empty scopeID produces empty msg without panic

- GIVEN `fetchMemory(someStore, "")` is called
- WHEN the returned `tea.Cmd` is executed
- THEN a `memoryRefreshMsg{}` with empty entries is produced
- AND `SearchMemory` is NOT called on `someStore`
- AND no panic occurs

#### Scenario: Valid inputs — entries returned

- GIVEN a fake `store.Store` whose `SearchMemory(ctx, "scope-1", "", 5)`
  returns 3 entries and nil error
- WHEN `fetchMemory(fakeStore, "scope-1")` is called and the cmd executed
- THEN the resulting `memoryRefreshMsg.entries` has length 3

---

### Requirement TR-12: EventMemoryChanged wires to fetchMemory

`screen_chat.go`'s `handleBusEvent` MUST contain a new case:

```go
case notify.EventMemoryChanged:
    cmds = append(cmds, fetchMemory(m.store, ev.Meta["scope_id"]))
```

This case MUST mirror the `EventTodolistChanged` case in placement and
pattern. The `scopeID` used for the fetch MUST come from
`ev.Meta["scope_id"]` (the scope at the point of the memory write).

#### Scenario: EventMemoryChanged triggers fetchMemory cmd

- GIVEN a `Model` with a non-nil `store` and a subscribed bus
- WHEN `EventMemoryChanged` arrives with `Meta["scope_id"]="scope-abc"`
- THEN `handleBusEvent` appends a `fetchMemory(m.store, "scope-abc")` cmd
  to its returned commands
- AND no other model state is mutated directly by this case

#### Scenario: EventMemoryChanged with empty scope_id — no-op

- GIVEN an `EventMemoryChanged` event where `Meta["scope_id"]` is absent
  or `""`
- WHEN the event is processed
- THEN `fetchMemory` is called with `scopeID=""` — which is safe per TR-11
- AND no panic occurs

---

### Requirement TR-13: memoryRefreshMsg updates panel via copyRailWith

`model.go`'s `Update` MUST contain a new case:

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

This MUST mirror the `todolistRefreshMsg` case in pattern. The panel
update MUST use a value copy (`cp := *mp`) before mutation so that no
prior `Model` snapshot is mutated (COW invariant from TR-0-B).

#### Scenario: memoryRefreshMsg populates panel entries

- GIVEN a `Model` with an empty `memoryPeekPanel`
- WHEN a `memoryRefreshMsg{entries: []store.MemoryEntry{{Title:"t1"},...}}`
  is processed through `Update`
- THEN the new model's `memoryPeekPanel.entries` reflects the 3 entries
- AND the prior model snapshot's panel remains empty

#### Scenario: memoryRefreshMsg with empty entries clears prior entries

- GIVEN a `Model` whose `memoryPeekPanel` has 3 cached entries
- WHEN `memoryRefreshMsg{entries: nil}` is processed through `Update`
- THEN the new model's `memoryPeekPanel.entries` is empty
- AND `Render` subsequently returns `""`

---

### Requirement TR-14: Initial state — panel empty until first EventMemoryChanged

`memoryPeekPanel` MUST start with `entries == nil`. Before any
`EventMemoryChanged` event is received, the panel MUST render as `""`
(per TR-0-C). The panel MUST NOT pre-fetch on boot or on
`EventTurnStarted`. This matches the todolist's "empty until first change"
behavior.

#### Scenario: Panel empty at session start

- GIVEN a freshly created `Model` (no events processed)
- WHEN `memoryPeekPanel.Render(32, 0)` is called
- THEN the return value is `""`

#### Scenario: Panel populated after first EventMemoryChanged cycle

- GIVEN a `Model` with an empty `memoryPeekPanel`
- WHEN `EventMemoryChanged` is received, `fetchMemory` executes and
  returns 2 entries, and `memoryRefreshMsg` is processed by `Update`
- THEN `memoryPeekPanel.Render(32, 0)` returns a non-empty string
- AND the output contains the entry titles

---

## Contract test requirements

### Requirement TR-15: Contract tests updated for 4-panel screenChat

`model_test.go`'s `TestPanelsFor_ContractMatrix` MUST be updated so the
`screenChat` row expects 4 panels:
`[panelTodolist, panelContextMeter, panelTelemetry, panelMemoryPeek]`.

`rail_panels_test.go`'s `TestRailWiring_ChatPanelsRegistered` MUST be
updated to assert that `panelMemoryPeek` is present in the rail map.

Both tests MUST remain red until the `panels.go` / `rail.go` changes are
applied (deliberate break-then-fix order in PR-c).

#### Scenario: TestPanelsFor_ContractMatrix passes after PR-c

- GIVEN `panelsFor(screenChat)` returning 4 panels
- WHEN `TestPanelsFor_ContractMatrix` runs
- THEN the test passes (no unexpected/missing panel IDs for screenChat)

#### Scenario: TestRailWiring_ChatPanelsRegistered passes after PR-c

- GIVEN `newRail(s)` registering `panelMemoryPeek`
- WHEN `TestRailWiring_ChatPanelsRegistered` runs
- THEN the test passes (panel present in map, non-nil)

---

## Test strategy

Strict TDD (make test) applies. For each requirement, a failing test MUST
be written before the implementation is added. Test categories:

| Category                        | Mechanism                                                          | Covers                               |
| ------------------------------- | ------------------------------------------------------------------ | ------------------------------------ |
| Synthetic-event unit tests      | Feed `notify.Event` through `handleBusEvent`, assert `Model` field | TR-2, TR-4, TR-6, TR-7, TR-12, TR-13 |
| Boot-wiring unit test           | Construct TUI with known `ContextWindowSize` return, inspect panel | TR-1                                 |
| Golden render tests             | `panel.Render` against `.golden` files in testdata                 | TR-3, TR-5, TR-8, TR-10              |
| Cmd unit tests                  | Execute `fetchMemory` cmd with fake store                          | TR-11                                |
| Contract tests (break-then-fix) | Updated frozen matrix and wiring tests                             | TR-9, TR-15                          |
| COW unit test                   | Two model snapshots after one event, compare fields                | TR-0-B                               |
| Determinism unit test           | Double-render with fixed state                                     | TR-0-A                               |

Golden files to create:

- `context_meter_with_categories.golden`
- `context_meter_fallback_aggregate.golden`
- `telemetry_with_tool_rows.golden`
- `telemetry_with_subagent_rows.golden`
- `memory_peek_populated.golden`

---

## Non-requirements (scope boundary)

- Rail boxing / borders restyle — deferred to a later visual change.
- Real per-panel rail height clamping — out of scope (visual change).
- Input hint-chips; sessions search / columns / model-picker interactivity.
- Thread-item event timestamps — separate tiny TUI change.
- `EventSubagentFailed` beyond no-op — design phase may add `failed bool`.
- Per-subagent row sort order other than insertion order — design phase may
  specify tokens-desc, but insertion order is the V1 default.
- Category sub-bar visual format (stacked mini-bars vs. segmented bar) —
  design phase decides; golden tests will lock it in.
- Memory-peek boot pre-fetch — deferred; panel is empty until first write.
- Any file outside `internal/tui` modified.
- Any `internal/agent` / `internal/notify` source edit.

---

## Open items for the design phase

The following questions are under-specified in the proposal and MUST be
pinned down in the design artifact before tasks are generated:

1. **Category sub-bar visual format** — three stacked mini-bars (one line
   each: `sys ███░░ 23%`) vs. one segmented composite bar
   (`sys|msg|tool ████░░░ 52%`). Golden tests will lock in the chosen
   format.

2. **Per-subagent row sort order** — insertion order (spec default) vs.
   tokens-desc. Insertion order is deterministic for golden tests; design
   phase may switch to tokens-desc.

3. **EventSubagentFailed treatment** — no-op (current spec) vs. a `failed
bool` marker on `subagentStat` rendered with a visual indicator. No-op
   is safe; design phase decides if a marker is needed for V1.

4. **Memory-peek row content — Title vs. Content field** — spec requires
   `Title`-first with `Content` fallback. Design phase must confirm this
   matches the `store.MemoryEntry` struct field names and decide on
   truncation length.

5. **ANSI truncation function** — the spec says `wrapPanelBox` for memory
   rows (from proposal). Design phase must confirm this is the right helper
   or specify `ansi.Truncate` directly.
