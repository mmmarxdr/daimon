# Exploration: tui-backend-seams

> Exposes backend data the Phase-2 rail panels need, additively, via the notify bus.
> Investigation date: 2026-05-31

## Intent

Three Phase-2 rail panels in `tui-rail-panels` are either blocked or degraded because
the backend does not publish all the data they need:

1. **context-meter** renders with a hardcoded 200k heuristic limit and has no
   per-category breakdown (system / memory / conversation / tools).
2. **telemetry** has no per-subagent token/cost rows yet; the aggregate panel is wired.
3. **memory-peek** is entirely unscaffolded and needs a bus event for memory mutations.

This change adds the minimal additive backend seams so those panels can wire real data.
The `View=pure(Model)` invariant is preserved throughout: no panel reads live objects in
Render; all data travels bus→tea.Msg→cached Model field.

---

## notify.Event + Bus contract today

### Event struct (internal/notify/bus.go:10–30)

| Field        | Type                | Purpose                                 |
| ------------ | ------------------- | --------------------------------------- |
| `Type`       | `string`            | Event type constant (see below)         |
| `Origin`     | `Origin`            | `"agent"` / `"cron"` / `"notification"` |
| `JobID`      | `string`            | Set for cron events                     |
| `JobPrompt`  | `string`            | Set for cron events                     |
| `ChannelID`  | `string`            | Conversation channel                    |
| `Text`       | `string`            | Human-readable summary                  |
| `Error`      | `string`            | Error string when applicable            |
| `Timestamp`  | `time.Time`         | Emission timestamp                      |
| `Meta`       | `map[string]string` | Structured string payload (extensible)  |
| `ToolCallID` | `string`            | Tool-lifecycle events                   |
| `ToolName`   | `string`            | Tool-lifecycle events                   |
| `Iteration`  | `int`               | Loop iteration counter                  |
| `TokenCount` | `int`               | Output token count (EventTokensUsage)   |
| `DurationMs` | `int64`             | Tool execution duration                 |
| `CostUSD`    | `float64`           | Turn cost in USD                        |
| `IsError`    | `bool`              | Error indicator                         |

### Event type constants (internal/notify/events.go)

| Constant                 | Value                        | Line | Note                                                                                                                                                                   |
| ------------------------ | ---------------------------- | ---- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `EventTurnStarted`       | `"agent.turn.started"`       | 26   |                                                                                                                                                                        |
| `EventTurnCompleted`     | `"agent.turn.completed"`     | 27   | Meta: `conv_id`, `input_tokens`, `output_tokens`                                                                                                                       |
| `EventContextCompacted`  | `"agent.context.compacted"`  | 28   |                                                                                                                                                                        |
| `EventSubagentSpawned`   | `"agent.subagent.spawned"`   | 37   | Meta: `subagent_id`, `batch_id`, `skill`, `parent_conv_id`                                                                                                             |
| `EventSubagentCompleted` | `"agent.subagent.completed"` | 38   | Meta: `subagent_id`, `batch_id`, `skill`, `parent_conv_id`, `cost_usd`, `turns`                                                                                        |
| `EventSubagentFailed`    | `"agent.subagent.failed"`    | 39   | Same + `reason`                                                                                                                                                        |
| `EventTodolistChanged`   | `"agent.todolist.changed"`   | 44   |                                                                                                                                                                        |
| `EventReasoningStart`    | `"agent.reasoning.start"`    | 49   |                                                                                                                                                                        |
| `EventReasoningEnd`      | `"agent.reasoning.end"`      | 50   |                                                                                                                                                                        |
| `EventToolStart`         | `"agent.tool.start"`         | 51   |                                                                                                                                                                        |
| `EventToolEnd`           | `"agent.tool.end"`           | 52   |                                                                                                                                                                        |
| `EventTokensUsage`       | `"agent.tokens.usage"`       | 53   | Typed fields: `TokenCount`, `CostUSD`; Meta: `conv_id`, `input_tokens`, `output_tokens`, `elapsed_ms` — plus subagent attribution keys when emitted from a child agent |

### Bus implementation (internal/notify/bus.go:56–204)

- `EventBus.ch` — buffered channel (default cap 1024).
- `Emit` — non-blocking; drops + warns when full. **nil-safe guard in all callers**: `if a.bus != nil` / `if m.bus == nil { return }`.
- `Subscribe` — thin dispatcher; handlers must be non-blocking (<5 s watchdog).
- Circuit breaker: 1000 events/min sliding window.

### TUI subscription path (internal/tui/events.go:57–73, model.go:334–354)

```
bus.Subscribe(handler) → sends on evCh (cap 256)
  → pumpEvents tea.Cmd (events.go:39–43) — blocks on evCh
    → bubbletea runtime → Model.Update
      → case busEventMsg → handleBusEvent(ev) (screen_chat.go:148)
        → copyRailWith → panel.accumulate(ev)
          → Model.View() reads cached field only
```

---

## Per-datum seam table

### Datum 1 — Context-window real limit (context-meter accuracy)

**Consumer panel:** `contextMeterPanel` (internal/tui/rail_panels.go:321)  
**Current state:** hardcoded `const contextLimit = 200_000` at rail_panels.go:353  
**Source:** `agent.contextMgr.resolvedMaxToks` (internal/agent/agent.go:158 field,
internal/agent/context_manager.go:34 field, :98 `MaxTokens()` method already public)

**Exists internally?** YES — `ContextManager.MaxTokens()` already returns it.
`contextMgr` is unexported (`agent.go:158`), but `ContextManager.MaxTokens()` is a
public method. The agent just needs a thin public accessor.

**Minimal seam:**

```go
// internal/agent/agent_accessors.go (~3 new lines)
func (a *Agent) ContextWindowSize() int {
    if a.contextMgr == nil { return 0 }
    return a.contextMgr.MaxTokens()
}
```

Called once at TUI construction in `runTUIWithStdin` (run.go:52), passed as an `int`
to a new `newContextMeterPanel(s, limit int)` constructor overload, stored as
`p.contextLimit int`. No bus event needed — this is a static value resolved at
agent boot.

**Additive?** YES — new exported method + constructor param only.  
**Rough LOC:** ~5 (accessor 3 + constructor param + Render update 2)  
**Breaking?** No.

---

### Datum 2 — Per-category context breakdown (context-meter category bar)

**Consumer panel:** `contextMeterPanel` (phase-2 extended design)  
**Breakdown desired:** system-prompt tokens / conversation tokens / tool-def tokens

**Source:** `ContextManager.smartManage` (internal/agent/context_manager.go:182–232)
already computes `sysToks`, `toolToks`, `msgToks` as local variables — but they are
DROPPED after the compaction decision. `TokenUsage` struct (context_manager.go:14–20)
only carries `SystemPrompt` and `Messages` (no `Tools` field).

**Exists internally?** PARTIALLY — the three values are computed locally in
`smartManage`. They are not stored and not published.

**Minimal seam — two parts:**

1. **Extend `TokenUsage`** with a `Tools int` field
   (internal/agent/context_manager.go:14):

   ```go
   type TokenUsage struct {
       SystemPrompt int
       Messages     int
       Tools        int   // NEW — tool-definition token estimate
       Total        int
       Max          int
       UsagePercent float64
   }
   ```

   Update `smartManage` to populate it (already has `toolToks`).

2. **Add category fields to `EventTokensUsage`** (internal/notify/bus.go ~line 30):

   ```go
   // Category breakdown — omitempty; zero means not provided.
   SysToks  int `json:"sys_toks,omitempty"`
   MsgToks  int `json:"msg_toks,omitempty"`
   ToolToks int `json:"tool_toks,omitempty"`
   ```

   Emit them at the existing `EventTokensUsage` emit site (loop.go:1015–1028) by
   calling `a.contextMgr.Usage(sysToks, conv.Messages)` once more (the data is already
   available as local variables in `processMessage`).

   NOTE: `loop.go` already has `systemPrompt` and `toolDefs` in scope at the emit site;
   estimating their token cost is one `EstimateTokens(systemPrompt)` +
   `estimateToolDefTokens(toolDefs)` call — both are already present in `smartManage`.

3. **TUI accumulate:** `contextMeterPanel.accumulate` reads `ev.SysToks`, `ev.MsgToks`,
   `ev.ToolToks` (zero-safe: if not provided, accumulates zero for that category).

**Additive?** YES for the Event struct and bus emit. The `TokenUsage` struct extension
is additive; `Usage()` callers get the new field for free.  
**Rough LOC:** ~20 (3 Event fields + TokenUsage field + Usage() update + loop.go emit
update + TUI accumulate extension)  
**Breaking?** No. All new fields are `omitempty` / zero-value safe.

**Caveat:** `loop.go` does not have direct access to `sysToks`/`toolToks` at the emit
site — they live inside `smartManage`. Two design options:

- (A) Re-estimate at the emit site (slight duplication, ~5 LOC extra).
- (B) Have `Manage()` or a new `LastUsage() TokenUsage` method return the last-computed
  breakdown. Option B is cleaner and worth the extra method.

This is an **open question for the proposal phase** (see below).

---

### Datum 3 — Per-subagent telemetry (telemetry panel subagent rows)

**Consumer panel:** `telemetryPanel` (internal/tui/rail_panels.go:74)  
**Data desired:** per-subagent name + tokens-in + tokens-out + cost + status

**Key finding:** `EventTokensUsage` already carries subagent attribution via
`mergeSubagentMeta` (internal/agent/subagent_meta.go:24). When a child agent emits
`EventTokensUsage`, the Meta map already contains:

- `subagent_id` — the handle ID
- `skill` — the skill name
- `parent_conv_id`
- `input_tokens`, `output_tokens` (string)

This means the TUI can accumulate per-subagent token rows by bucketing
`EventTokensUsage` events on `ev.Meta["subagent_id"]` — **no new backend event fields
are required for tokens**.

For cost per subagent: `ev.CostUSD` (typed field) is already on `EventTokensUsage`.

For final status (completed / failed): `EventSubagentCompleted` / `EventSubagentFailed`
already carry `cost_usd` and `turns` in Meta (subagent_manager.go:541–553), but NOT
token counts. Token totals are NOT tracked in `subRecord` (no `tokens` field — only
`cost float64` and `turns int`).

**What is needed:**

Option A (preferred): Add a `tokens int` accumulator field to `subRecord`
(subagent_manager.go:36) — increment it from `ev.Meta["output_tokens"]` inside
`budgetMonitor` (line 454, where `turnCost` is already parsed). Then include
`"tokens": strconv.Itoa(tokens)` in the `EventSubagentCompleted` Meta (finalize,
line 541). ~8 LOC.

Option B (zero backend work): The TUI accumulates tokens from the stream of tagged
`EventTokensUsage` events during the subagent's life. This works but means the panel
shows running totals while the subagent is live and stops accumulating on completion
without a final authoritative number.

Option A is cleaner and gives the telemetry panel a single authoritative source on
completion. Option B is zero backend cost.

**Exists internally?** Tokens: computed per-turn in child loop, available in
`EventTokensUsage.Meta`. Not stored in `subRecord`. Cost: stored in `subRecord.cost`,
published in `EventSubagentCompleted.Meta["cost_usd"]`.

**Minimal seam (Option A):**

- `subRecord`: add `tokens int` field.
- `budgetMonitor`: parse `ev.Meta["input_tokens"]` + `ev.Meta["output_tokens"]` and
  add to `rec.tokens`.
- `finalize`: add `"tokens"` to Meta before emitting `EventSubagentCompleted`.

**Additive?** YES — new field on internal struct, new Meta key. All nil/zero-safe.  
**Rough LOC:** ~8  
**TUI work (in tui-rail-panels):** ~40 LOC accumulator in telemetryPanel.

---

### Datum 4 — Memory-peek (memory-peek panel)

**Consumer panel:** `panelMemoryPeek` — does not exist yet.  
**Data desired:** recent in-process memory entries for the active session.

**Key findings from investigation:**

1. `store.MemoryEntry` (internal/store/store.go:61) is the in-process memory model.
   It is queryable via `store.SearchMemory()` — both `FileStore` and `SQLiteStore`
   implement it.

2. There are FOUR production `AppendMemory` call sites:
   - `internal/tool/memory.go:179` — `save_memory` tool (agent-invoked)
   - `internal/agent/loop.go:616` — legacy fallback in turn loop
   - `internal/agent/loop.go:628` — legacy path (non-curator)
   - `internal/agent/curator.go:441` — Curator.Curate (smart path)
   - `internal/agent/consolidator.go:260` — background consolidator

3. **No bus event is emitted on any memory write.** The `Curator` struct has no `bus`
   field (curator.go:65–74). `MemoryToolDeps` (tool/memory.go:56) has no bus field.
   Neither `loop.go` nor `consolidator.go` emit a bus event after `AppendMemory`.

4. Engram is an **external MCP process** — daimon has zero visibility into engram's
   memory state. The `memory-peek` panel can only show daimon's own `store.MemoryEntry`
   entries, not engram observations.

**Minimal seam for an `EventMemoryChanged` bus event:**

A new constant:

```go
// internal/notify/events.go
EventMemoryChanged = "agent.memory.changed"  // emitted after AppendMemory succeeds
```

The payload can be minimal — just a signal that memory changed, with the scope and
entry ID so the TUI can schedule a `fetchRecentMemoriesCmd`:

```go
// Event fields used:
// Meta["scope_id"]  — memory scope
// Meta["entry_id"]  — new entry ID
// Meta["title"]     — truncated title (display convenience)
// Meta["cluster"]   — cluster bucket
```

**Injection point complexity:** Four production `AppendMemory` call sites need the bus.

- `save_memory` tool (tool/memory.go): `MemoryToolDeps` would need a `Bus notify.Bus`
  field (or a notify callback). The tool has no current bus access.
- `curator.go`: Curator would need a `bus notify.Bus` field injected at construction.
- `loop.go` legacy paths: `a.bus` is already available.
- `consolidator.go`: `Consolidator` would need a bus field.

This is a moderate injection effort — not a breaking change but affects 3 structs
(`MemoryToolDeps`, `Curator`, `Consolidator`). All are nil-guard safe.

**Is memory-peek in scope for tui-backend-seams?** Borderline — see Recommended Scope.

**Additive?** YES (new Event constant, new bus fields).  
**Rough LOC:** ~25 (event constant + Curator bus field + MemoryToolDeps bus field +
emit calls in 4 sites + nil guards). Panel scaffolding (~60 LOC) is TUI work, not
backend.  
**Engram boundary:** confirmed out of reach. memory-peek shows only daimon's own store.

---

### Datum 5 (lower priority) — Event timestamps on thread items

**Source:** `notify.Event.Timestamp` (bus.go:18) — already a `time.Time` on every bus
event.  
**Exists?** YES on every event. The TUI `busEventMsg` handler receives the full event.  
**Seam:** zero backend work. Pure TUI accumulator change — add `timestamp time.Time` to
`ToolLine` / `MsgDaimon` populated from `ev.Timestamp`.  
**LOC:** ~10 TUI only. NOT a backend seam.

---

### Datum 6 (lower priority) — Session metadata (turns/cost/tokens/branch)

**Consumer:** sessions panel (Phase 3 / WU-c rail).  
**Source:** `store.Conversation` (internal/store/store.go) — has `ID`, `Status`,
`CreatedAt`, `UpdatedAt`. Session token/cost summaries are NOT stored per-conversation
today (they are ephemeral per-turn in `EventTokensUsage`).  
**Seam complexity:** moderate — would need a per-conversation token/cost accumulator in
the store, or a new summary field written at turn end. Not additive-simple.  
**Verdict:** Defer to Phase 3 (sessions work). NOT in tui-backend-seams scope.

---

### Datum 7 (lower priority) — MCP server status (Screen 05)

**Consumer:** tools screen (screenTools, PR4a).  
**Source:** `internal/mcp/manager.go:45 Manager` — tracks `[]managedServer`. No bus
event for server connect/disconnect.  
**Seam:** would need a new `EventMCPServerStatus` constant and an emit in manager when
a server connects or fails. Moderate.  
**Verdict:** Defer to Phase 3. NOT in tui-backend-seams scope.

---

## Recommended scope for tui-backend-seams

### Include (all consumed by Phase-2 panels, all additive, all small)

| Seam                                                                                            | Datum   | Backend file(s)                                        | Rough LOC |
| ----------------------------------------------------------------------------------------------- | ------- | ------------------------------------------------------ | --------- |
| `agent.ContextWindowSize()` accessor                                                            | Datum 1 | `agent_accessors.go`                                   | ~5        |
| `EventTokensUsage` category fields (`SysToks`, `MsgToks`, `ToolToks`) + `TokenUsage.Tools`      | Datum 2 | `bus.go`, `context_manager.go`, `loop.go`              | ~20       |
| `subRecord.tokens` + `EventSubagentCompleted` `"tokens"` Meta key                               | Datum 3 | `subagent_manager.go`                                  | ~8        |
| `EventMemoryChanged` constant + bus injection in Curator / save_memory tool / loop legacy paths | Datum 4 | `events.go`, `curator.go`, `tool/memory.go`, `loop.go` | ~25       |
| **Total backend LOC**                                                                           |         |                                                        | **~58**   |

Memory-peek (Datum 4) is included because the Phase-2 exploration explicitly listed it
as requiring `tui-backend-seams`; the injection work is moderate but clean and entirely
additive.

### Defer

- Session metadata accumulation (Datum 6) — Phase 3, store schema change.
- MCP server status events (Datum 7) — Phase 3, separate concern.
- Event timestamps on thread items (Datum 5) — zero backend work; pure TUI, goes
  straight into `tui-rail-panels`.

---

## Risks

| Risk                                                      | Severity | Mitigation                                                                                                                                                                                                                                                                       |
| --------------------------------------------------------- | -------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| **Bus nil-panic**                                         | LOW      | All four new emit sites already use or can use `if bus == nil { return }` guards. `EventBus.Emit` itself checks `b.closed`. The nil-bus fix (prior work) established the guard pattern; follow it.                                                                               |
| **Category tokens double-counting**                       | MEDIUM   | `SysToks` + `MsgToks` + `ToolToks` from `EventTokensUsage` accumulate per-turn. The context-meter panel must REPLACE (not accumulate) these on each event, since they reflect the current window fill, not a delta. (Contrast with `TokenCount` which IS a per-turn delta.)      |
| **Subagent token count methodology**                      | LOW      | `budgetMonitor` only receives `EventTurnCompleted` (not `EventTokensUsage`), so it accumulates from `Meta["input_tokens"] + output_tokens`. This matches `totalInputTokens + totalOutputTokens` from loop.go:1010. Small estimation drift is acceptable for a telemetry display. |
| **Memory bus injection scope**                            | MEDIUM   | Four production `AppendMemory` sites need bus access. Curator and Consolidator are agent-internal and easily injected. `MemoryToolDeps` is a public struct — adding `Bus notify.Bus` is additive and zero-value safe (nil = no event).                                           |
| **`EventTokensUsage` category fields on legacy strategy** | LOW      | `smartManage` computes the breakdown; `legacyManage` and strategy `"none"` do not. Emit zero for `SysToks`/`MsgToks`/`ToolToks` on non-smart paths — the panel's `hasData` flag fires from non-zero `TokenCount` regardless.                                                     |
| **`resolvedMaxToks == 0` when provider unknown**          | LOW      | Return `0` from `ContextWindowSize()`; TUI falls back to heuristic 200k. This is the documented sentinel.                                                                                                                                                                        |

---

## Open questions for the proposal phase

### Q1 — Category token source: re-estimate vs. cache (CRITICAL)

`loop.go`'s `EventTokensUsage` emit site does NOT have direct access to `sysToks` /
`toolToks` — those are computed inside `ContextManager.smartManage`. Two options:

- **Option A (re-estimate):** call `EstimateTokens(systemPrompt)` +
  `estimateToolDefTokens(toolDefs)` at the emit site. Both variables are in scope in
  `processMessage`. Slight duplication (~2 calls) but no API change to `ContextManager`.

- **Option B (cache via `Manage()`):** Add a `LastUsage() TokenUsage` method to
  `ContextManager` that returns the breakdown computed during the last `Manage()` call.
  Cleaner API, zero duplication, but adds mutable state to `ContextManager`.

The proposal must choose one. Recommendation: Option A for simplicity (the estimates
are already called internally; duplication is 2 lines not 2 systems).

### Q2 — Per-subagent tokens: Option A (backend accumulation) vs. Option B (TUI accumulation)

Datum 3 showed both paths work. Option A (8 LOC backend) gives an authoritative final
count on completion. Option B (zero backend) gives only running totals. The proposal
should commit to one. Recommendation: Option A — authoritative final number is worth 8
LOC.

### Q3 — Memory event granularity: signal vs. payload

Should `EventMemoryChanged` carry a full `store.MemoryEntry` summary (title, cluster,
importance) in Meta, or just `scope_id` + `entry_id` and let the TUI do a
`fetchRecentMemoriesCmd` round-trip to the store?

Trade-off: full payload = zero TUI store read (but large Meta string); signal only =
one extra store read per event (cheap SQLite query). Recommendation: carry title +
cluster + importance in Meta (3 short strings), skip a store round-trip.

### Q4 — `EventTokensUsage` category fields: new typed fields on `notify.Event` vs. Meta strings

Adding `SysToks int`, `MsgToks int`, `ToolToks int` as typed fields on `notify.Event`
is the clean pattern (matches `TokenCount`, `CostUSD`). Alternatively, pack them into
Meta as strings (zero `notify.Event` struct change). The proposal must decide.
Recommendation: typed fields — consistent with existing `TokenCount`/`CostUSD` pattern,
no string→int parsing in TUI.

---

## Evidence map (file:line references)

| Symbol / location                        | Significance                                                         |
| ---------------------------------------- | -------------------------------------------------------------------- |
| `internal/notify/bus.go:10–30`           | `notify.Event` full field list                                       |
| `internal/notify/events.go:19–101`       | All event type constants + KnownEventTypes + StreamingSkipSet        |
| `internal/agent/context_manager.go:14`   | `TokenUsage` struct — missing `Tools int`                            |
| `internal/agent/context_manager.go:98`   | `MaxTokens()` already public                                         |
| `internal/agent/context_manager.go:182`  | `smartManage` — computes `sysToks`, `toolToks`, `msgToks` locally    |
| `internal/agent/agent.go:158`            | `contextMgr *ContextManager` (unexported field)                      |
| `internal/agent/agent_accessors.go:1`    | Where `ContextWindowSize()` goes                                     |
| `internal/agent/loop.go:1015`            | `EventTokensUsage` emit site — add category fields here              |
| `internal/agent/subagent_manager.go:36`  | `subRecord` — needs `tokens int` field                               |
| `internal/agent/subagent_manager.go:454` | `budgetMonitor` — where to accumulate tokens from Meta               |
| `internal/agent/subagent_manager.go:541` | `finalize` Meta build — where to add `"tokens"` key                  |
| `internal/agent/curator.go:65`           | `Curator` struct — needs `bus notify.Bus` field                      |
| `internal/tool/memory.go:56`             | `MemoryToolDeps` — needs `Bus notify.Bus` field                      |
| `internal/tool/memory.go:179`            | `save_memory` `AppendMemory` call — emit `EventMemoryChanged` here   |
| `internal/agent/loop.go:616,628`         | Legacy `AppendMemory` paths — emit `EventMemoryChanged` here         |
| `internal/agent/curator.go:441`          | Curator `AppendMemory` — emit `EventMemoryChanged` here              |
| `internal/agent/consolidator.go:260`     | Consolidator `AppendMemory` — emit `EventMemoryChanged` here         |
| `internal/tui/rail_panels.go:321`        | `contextMeterPanel` — consumer of Datum 1 + 2                        |
| `internal/tui/rail_panels.go:74`         | `telemetryPanel` — consumer of Datum 3                               |
| `internal/tui/rail.go:26`                | `newRail` — construction site for `ContextWindowSize()` call         |
| `internal/tui/run.go:52`                 | `runTUIWithStdin` — where `ag.ContextWindowSize()` is called at boot |
| `internal/tui/screen_chat.go:271`        | `EventTokensUsage` TUI handler — extend for category fields          |
