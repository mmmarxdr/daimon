# TUI Backend Seams Specification

## Purpose

Adds the minimal additive backend seams that the Phase-2 TUI rail panels
(`contextMeterPanel`, `telemetryPanel`, `panelMemoryPeek`) require to
display live, accurate data. Every seam is published via the notify bus
so the TUI can cache values in its `Model` without reading live objects
inside `View` (the `View=pure(Model)` invariant is enforced throughout).

All requirements are ADDITIVE only. No existing event field, bus contract,
struct, or consumer is changed in a breaking way. Existing consumers that
do not read new fields are unaffected because zero values are valid
fall-backs for every new field.

---

## ADDED Requirements

### Requirement: ContextWindowSize accessor on Agent

`internal/agent/agent_accessors.go` MUST expose:

```go
func (a *Agent) ContextWindowSize() int
```

- Returns `a.contextMgr.MaxTokens()` when `contextMgr` is non-nil.
- Returns `0` when `contextMgr` is nil.

This is a static value resolved at agent boot. It is read ONCE by the TUI
constructor (`runTUIWithStdin` / `newRail`) and passed as an `int` to
`newContextMeterPanel`. No bus event is emitted.

The returned `0` is the documented sentinel: the TUI MUST fall back to its
internal heuristic limit (200 000 tokens) when `ContextWindowSize()` returns
`0`.

#### Scenario: ContextWindowSize with smart context configured

- GIVEN an `Agent` whose `contextMgr` was initialised with a real
  `ContextManager` whose `resolvedMaxToks` is `200000`
- WHEN `agent.ContextWindowSize()` is called
- THEN it returns `200000`
- AND it does NOT panic

#### Scenario: ContextWindowSize with no context manager

- GIVEN an `Agent` constructed with `contextMgr == nil`
- WHEN `agent.ContextWindowSize()` is called
- THEN it returns `0`
- AND it does NOT panic

#### Scenario: TUI falls back to heuristic on zero return

- GIVEN `ContextWindowSize()` returns `0`
- WHEN `newContextMeterPanel` receives `0` as its `limit` parameter
- THEN the panel's internal `contextLimit` field is the heuristic value
  (`200000` or equivalent constant)
- AND the context-meter renders without error

---

### Requirement: Per-category context breakdown on EventTokensUsage

`notify.Event` MUST carry three new typed integer fields, populated only
when the smart context-management strategy is active:

| Field      | JSON key (omitempty) | Meaning                                               |
| ---------- | -------------------- | ----------------------------------------------------- |
| `SysToks`  | `sys_toks`           | System-prompt token estimate in current window        |
| `MsgToks`  | `msg_toks`           | Conversation-message token estimate in current window |
| `ToolToks` | `tool_toks`          | Tool-definition token estimate in current window      |

These fields carry REPLACE semantics: each `EventTokensUsage` event is a
snapshot of the current context-window fill, NOT a per-turn delta.
Consumers MUST overwrite (not accumulate) these values on each event.
This contrasts with `TokenCount` and `CostUSD`, which ARE per-turn deltas.

`TokenUsage` (internal/agent/context_manager.go) MUST gain a `Tools int`
field so the breakdown can be stored and propagated.

The emit site in `loop.go` MUST populate `SysToks`, `MsgToks`, `ToolToks`
from the computed category values whenever the smart strategy ran for that
turn. On non-smart paths (legacy strategy or strategy `"none"`) the three
fields MUST be `0`.

#### Scenario: Smart strategy populates category fields

- GIVEN an agent running the `"smart"` context-management strategy
- AND the most recent `ContextManager.Manage()` call computed
  `sysToks=1500`, `msgToks=4200`, `toolToks=800`
- WHEN an `EventTokensUsage` event is emitted at the end of that turn
- THEN `ev.SysToks == 1500`
- AND `ev.MsgToks == 4200`
- AND `ev.ToolToks == 800`
- AND `ev.TokenCount > 0` (the per-turn output-token count is unaffected)

#### Scenario: Non-smart strategy emits zero for category fields

- GIVEN an agent running the `"legacy"` or `"none"` context-management
  strategy
- WHEN an `EventTokensUsage` event is emitted
- THEN `ev.SysToks == 0`
- AND `ev.MsgToks == 0`
- AND `ev.ToolToks == 0`
- AND the event is otherwise valid (non-zero `TokenCount`, non-empty
  `ChannelID`)

#### Scenario: TUI context-meter degrades gracefully on zero category fields

- GIVEN a `contextMeterPanel` that has received an `EventTokensUsage` with
  `SysToks == 0`, `MsgToks == 0`, `ToolToks == 0`
  but a non-zero `TokenCount`
- WHEN the panel renders
- THEN the aggregate token bar renders from `TokenCount` alone
- AND no per-category sub-bar is displayed (or all sub-bars show zero)
- AND the render does NOT panic

#### Scenario: Category fields do not affect existing EventTokensUsage consumers

- GIVEN an existing consumer that reads only `TokenCount`, `CostUSD`, and
  `Meta["input_tokens"]` / `Meta["output_tokens"]` from `EventTokensUsage`
- WHEN an event with non-zero `SysToks`/`MsgToks`/`ToolToks` is received
- THEN the consumer's behaviour is identical to pre-change
  (zero-value fields are transparent to consumers that ignore them)

---

### Requirement: Per-subagent telemetry seam

**Live attribution (zero new backend work):**  
`EventTokensUsage` events emitted by a child agent ALREADY carry
`Meta["subagent_id"]`, `Meta["skill"]`, `Meta["input_tokens"]`,
`Meta["output_tokens"]`, and `CostUSD` via `mergeSubagentMeta`. Consumers
MAY bucket these events by `Meta["subagent_id"]` to accumulate running
per-subagent totals.

**Authoritative final count (new backend work):**  
`EventSubagentCompleted` MUST include `Meta["tokens"]` — the total token
count accumulated over all turns of the subagent's life. This gives
consumers a single authoritative source when the subagent finishes, without
requiring them to re-sum the live stream.

To implement this, `subRecord` (internal/agent/subagent_manager.go) MUST
gain a `tokens int` field that is incremented inside `budgetMonitor`
from `ev.Meta["input_tokens"]` + `ev.Meta["output_tokens"]` on each
`EventTurnCompleted` (or `EventTokensUsage`) event received for that subagent.
`finalize` MUST include `"tokens": strconv.Itoa(rec.tokens)` in the
`EventSubagentCompleted` Meta map.

#### Scenario: Live per-subagent attribution via EventTokensUsage

- GIVEN a child agent running N turns each emitting `EventTokensUsage`
  with `Meta["subagent_id"] = "sa-1"`, `Meta["input_tokens"] = "100"`,
  `Meta["output_tokens"] = "50"` per turn
- WHEN a TUI consumer buckets events by `Meta["subagent_id"]`
- THEN after N turns the accumulated input tokens for `"sa-1"` is `N*100`
- AND the accumulated output tokens is `N*50`

#### Scenario: Authoritative total on EventSubagentCompleted

- GIVEN a child agent that ran 3 turns with token counts
  `[in=100/out=50]`, `[in=80/out=40]`, `[in=90/out=45]`
- WHEN the child agent completes and `EventSubagentCompleted` is emitted
- THEN `ev.Meta["tokens"]` is present
- AND `strconv.Atoi(ev.Meta["tokens"])` equals `405`
  (sum of all input + output: 100+50+80+40+90+45)
- AND `ev.Meta["cost_usd"]` is also present (pre-existing field, unaffected)

#### Scenario: EventSubagentCompleted without tokens (zero-accumulation guard)

- GIVEN a child agent that completed 0 turns (spawned then immediately
  cancelled before any `EventTurnCompleted`)
- WHEN `EventSubagentCompleted` is emitted
- THEN `ev.Meta["tokens"]` is `"0"` (not absent)
- AND the consumer can safely call `strconv.Atoi(ev.Meta["tokens"])`
  without error

---

### Requirement: EventMemoryChanged bus event

A new event type constant MUST be defined:

```go
// internal/notify/events.go
EventMemoryChanged = "agent.memory.changed"
```

This event MUST be emitted on the notify bus after every successful
`AppendMemory` call that goes through the following paths:

- `internal/agent/curator.go` (Curator.Curate smart path)
- `internal/agent/loop.go` legacy AppendMemory paths (lines 616, 628)
- `internal/tool/memory.go` save_memory tool

The `Consolidator` (`internal/agent/consolidator.go`) is a lower-priority
emit site; whether it emits this event is left to the design phase.

**Payload** (all fields in `Event.Meta`):

| Meta key   | Content                              |
| ---------- | ------------------------------------ |
| `scope_id` | Memory scope for the written entry   |
| `entry_id` | ID of the new entry (if available)   |
| `title`    | Truncated display title of the entry |
| `cluster`  | Cluster bucket (if applicable)       |

A raw `AppendMemory` call that does NOT go through any of the three paths
listed above (e.g. a future direct store call) is NOT guaranteed to emit
`EventMemoryChanged`. Consumers MUST treat this event as a best-effort
signal, not a guaranteed complete log of all memory writes.

**Struct injection requirements:**

- `Curator` MUST gain a `bus notify.Bus` field (nil-value safe).
- `MemoryToolDeps` MUST gain a `Bus notify.Bus` field (nil-value safe,
  additive — existing callers that do not set the field retain zero-value
  `nil` behavior without breaking).

**Engram boundary:** `EventMemoryChanged` reflects ONLY `daimon`'s own
`store.MemoryEntry` writes. Engram (external MCP process) memory writes
are NOT observable and MUST NOT be assumed to emit this event.

#### Scenario: Curator write emits EventMemoryChanged

- GIVEN a `Curator` with a non-nil `bus` field
- WHEN `Curator.Curate` calls `AppendMemory` successfully
- THEN one `EventMemoryChanged` event is emitted on the bus
- AND `ev.Meta["scope_id"]` is non-empty
- AND `ev.Meta["title"]` is non-empty

#### Scenario: save_memory tool emits EventMemoryChanged

- GIVEN the save_memory tool invoked with `MemoryToolDeps.Bus` set to a
  non-nil bus
- WHEN the tool's `AppendMemory` call succeeds
- THEN one `EventMemoryChanged` event is emitted on the bus
- AND `ev.Meta["scope_id"]` and `ev.Meta["entry_id"]` are present

#### Scenario: Nil bus does not panic on memory write

- GIVEN a `Curator` with `bus == nil`
- WHEN `Curator.Curate` calls `AppendMemory` successfully
- THEN no `EventMemoryChanged` is emitted
- AND no panic occurs

#### Scenario: Nil bus on MemoryToolDeps does not panic

- GIVEN the save_memory tool invoked with `MemoryToolDeps.Bus == nil`
- WHEN the tool's `AppendMemory` call succeeds
- THEN no panic occurs
- AND the tool's return value is unaffected

#### Scenario: Failed AppendMemory does not emit the event

- GIVEN `AppendMemory` returns a non-nil error (e.g. store write failure)
- WHEN the Curator or tool handles the error
- THEN `EventMemoryChanged` is NOT emitted
  (event signals success only)

---

### Requirement: Additive and nil-bus safety cross-cutting invariants

All requirements above share these cross-cutting constraints:

1. **Additive only**: No existing `notify.Event` field, `EventBus` method,
   event type constant, or consumer interface changes its type or is removed.
   New fields on `notify.Event` MUST use `omitempty` JSON tags so that
   existing serialised event payloads remain valid.

2. **Nil-bus safety**: Every new `bus.Emit(...)` call site MUST be guarded
   by `if bus == nil { return }` or equivalent before calling `Emit`.
   `EventBus.Emit` itself is nil-safe per the existing contract; the guard
   is belt-and-suspenders and is REQUIRED at every new call site.

3. **View=pure(Model) consumer contract**: These seams exist so the TUI can
   cache backend data into its `Model` via `bus→tea.Msg→Model.Update`.
   No TUI `View` function MAY read a live agent object directly. The spec
   says nothing about how the TUI caches values — that is a TUI concern —
   but the backend MUST publish all required data via the bus so that
   caching is possible.

4. **Zero-value fall-back**: Every new typed field on `notify.Event`
   (`SysToks`, `MsgToks`, `ToolToks`) MUST default to `0` when not
   populated. Consumers that check `> 0` before using a field are
   considered compliant.

#### Scenario: New Event fields are zero on unrelated event types

- GIVEN an `EventTurnStarted` or `EventToolEnd` event (i.e. any event that
  is NOT `EventTokensUsage`)
- WHEN a consumer reads `ev.SysToks`, `ev.MsgToks`, `ev.ToolToks`
- THEN all three are `0` (Go zero value)
- AND no extra fields appear in a JSON-marshalled event that were absent
  before (omitempty ensures this)

#### Scenario: Nil EventBus at all new emit sites

- GIVEN any code path that would emit `EventMemoryChanged`,
  `EventTokensUsage` with category fields, or `EventSubagentCompleted`
  with the tokens Meta key
- WHEN the relevant `Bus` field or `a.bus` is `nil`
- THEN the code path completes without panic
- AND no event is emitted

---

## Non-requirements (scope boundary)

- Session metadata accumulation (turns/cost/tokens per conversation in the
  store) — Phase 3, requires store schema change.
- MCP server connect/disconnect events (`EventMCPServerStatus`) — Phase 3,
  separate concern.
- Event timestamps on thread items — zero backend work; pure TUI change,
  goes directly into `tui-rail-panels`.
- Engram (external MCP) memory observation — outside daimon's process
  boundary; not observable.
- `panelMemoryPeek` TUI scaffolding — TUI work, belongs in
  `tui-rail-panels`, not this change.
- `Consolidator` as a mandatory `EventMemoryChanged` emit site — deferred
  to design phase decision.
- Any behaviour change to existing event fields, bus capacity, or circuit
  breaker thresholds.
