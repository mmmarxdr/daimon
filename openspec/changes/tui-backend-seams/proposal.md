# Proposal: tui-backend-seams

> Additive backend seams that publish the data the Phase-2 rail panels need, via the
> notify bus, without breaking any existing Event consumer and without violating
> `View=pure(Model)`.
>
> Builds on: `openspec/changes/tui-backend-seams/exploration.md`
> Downstream consumer: `openspec/changes/tui-rail-panels` (the panels that read these seams)
> Phase: 2 (data seams). Phase 3 (session metadata, MCP status, approvals) is OUT.

---

## Intent

Three Phase-2 rail panels (`context-meter`, `telemetry`, `memory-peek`) are blocked or
degraded because the backend does not publish all the data they render:

1. **context-meter** renders against a hardcoded `200_000` heuristic and has no
   per-category breakdown (system / conversation / tools).
2. **telemetry** has no authoritative per-subagent token total at completion.
3. **memory-peek** does not exist and has no bus signal for in-process memory mutations.

This change adds the minimal **additive** seams so those panels can wire real data. The
non-negotiable constraint holds throughout: the embedded TUI never reads live backend
objects in `Render`. Every per-turn-changing datum reaches the TUI as
`notify.Bus` event → `tea.Msg` → cached `Model` field. The one static value
(context-window size) is read ONCE at TUI construction. All seams are nil-bus-safe and
introduce no breaking changes to existing `notify.Event` consumers.

---

## Scope

### In

- A `ContextWindowSize() int` accessor on `*Agent` (static, read once at TUI boot).
- A cached per-category context breakdown surfaced from `ContextManager` via a new
  `LastUsage() TokenUsage` method, with a `Tools int` field added to `TokenUsage`.
- Three category fields on `notify.Event` (`SysToks`, `MsgToks`, `ToolToks`),
  emitted on `EventTokensUsage`, carrying REPLACE-semantics window-fill snapshots.
- An authoritative `tokens` total on `EventSubagentCompleted` Meta, backed by a new
  `tokens int` accumulator on `subRecord`.
- A new `EventMemoryChanged` event constant + minimal Meta payload, emitted after a
  successful `AppendMemory` at the production write sites, with the bus injected into
  `Curator`, `Consolidator`, and `MemoryToolDeps`.

### Out (deferred to Phase 3 or to `tui-rail-panels` directly)

- **Session metadata** (per-conversation turns/cost/tokens/branch) — needs a store
  schema change; Phase 3.
- **MCP server status events** — separate concern; Phase 3.
- **Approval seam** — Phase 3.
- **Event timestamps on thread items** — zero backend work; lands directly in
  `tui-rail-panels`.
- **All TUI panel wiring / accumulators / scaffolding** — owned by `tui-rail-panels`;
  this change only exposes the data.

---

## Capabilities

### New capability: `tui-backend-seams`

The bus-published and accessor-exposed contract that lets a pure-Model TUI render
context-window fill by category, per-subagent token totals, and in-process memory
changes without reading live backend objects.

### Modified spec'd behavior

- `EventTokensUsage` (agent-stream-events / REQ-9.2) — gains three optional,
  zero-value-safe category fields. Existing typed fields (`TokenCount`, `CostUSD`) and
  Meta keys are unchanged.
- `EventSubagentCompleted` (subagent design §2.6) — gains one optional Meta key
  (`tokens`). Existing keys (`subagent_id`, `batch_id`, `skill`, `parent_conv_id`,
  `cost_usd`, `turns`) are unchanged.
- `notify` event-type set — gains `EventMemoryChanged`; registered in
  `KnownEventTypes` and added to `StreamingSkipSet` so it never triggers a user
  notification rule.

---

## Approach

Each decision below is resolved with rationale and concrete signatures.

### Decision 1 — Category-context seam: cache via `LastUsage()` (Option B), NOT re-estimate

**Resolved: Option B (cache the last-computed breakdown in `ContextManager`).**

The exploration left this open and leaned toward Option A (re-estimate at the emit
site). Grounding it in the real code reverses that recommendation:

- `ContextManager.Manage(ctx, systemPrompt, toolDefs, messages)` is called at
  `loop.go:351` inside `processMessage`. Its `smartManage` path computes `sysToks`,
  `toolToks`, `msgToks` as locals (`context_manager.go:189-191`).
- The `EventTokensUsage` emit site is at `loop.go:1015`, in turn-end code where
  `systemPrompt` and `toolDefs` are **no longer in scope** — only `conv`,
  `totalInputTokens`, `totalOutputTokens`, `turnCostUSD` are. Option A would force
  re-deriving the system prompt and tool definitions at turn-end purely to re-estimate
  them. That is MORE work and MORE duplication than the exploration assumed, not less.
- Correction to a verified fact: `cm.Usage(sysToks, messages)` does **not** compute
  tool tokens — its signature takes only system-prompt tokens + messages
  (`context_manager.go:104`). `TokenUsage` has no `Tools` field today. Only
  `smartManage` computes `toolToks`. So the breakdown is genuinely computed in exactly
  one place per turn, and that place is the natural cache point.

**Concrete seam:**

```go
// context_manager.go — TokenUsage gains a Tools field (additive)
type TokenUsage struct {
    SystemPrompt int
    Messages     int
    Tools        int     // NEW — tool-definition token estimate
    Total        int
    Max          int
    UsagePercent float64
}

// context_manager.go — new cached field on ContextManager (mutex-guarded like the rest)
type ContextManager struct {
    // ...existing fields...
    lastUsage TokenUsage // last per-category breakdown computed by Manage()
}

// context_manager.go — new accessor (additive, nil-safe at call site)
func (cm *ContextManager) LastUsage() TokenUsage {
    cm.mu.Lock()
    defer cm.mu.Unlock()
    return cm.lastUsage
}
```

`Manage` populates `cm.lastUsage` on EVERY strategy path before returning, so the cache
is never stale:

- `smartManage` already has `sysToks/toolToks/msgToks` — store them (including when it
  returns early below threshold).
- `legacyManage` and the `"none"` path compute `sysToks = EstimateTokens(systemPrompt)`,
  `toolToks = estimateToolDefTokens(toolDefs)`, `msgToks = EstimateMessagesTokens(...)`
  and store them. (~4 lines; these helpers already exist.)

At the emit site (`loop.go:1015`) the breakdown is read once and placed on the event:

```go
u := a.contextMgr.LastUsage() // nil-guarded: emit block is already inside `if a.bus != nil`
// add to the existing EventTokensUsage Event literal:
SysToks:  u.SystemPrompt,
MsgToks:  u.Messages,
ToolToks: u.Tools,
```

**REPLACE semantics (load-bearing note for the TUI consumer):** `SysToks`/`MsgToks`/
`ToolToks` are **window-fill snapshots** of the current turn, NOT per-turn deltas.
Contrast with `TokenCount` (output tokens) which IS a delta. The `context-meter` panel
MUST overwrite these three on each event, never sum them. This is the single most
important integration contract for `tui-rail-panels` and is restated under Risks.

Why not "memory" as a 4th category: daimon's in-process memory is not part of the
prompt token budget (it is injected as content, already counted inside `msgToks`/system
prompt as applicable). The three real categories the `ContextManager` actually measures
are system / messages / tools. The panel's "memory" slice, if desired, is a
memory-peek concern, not a context-window-fill category.

### Decision 2 — Per-subagent telemetry: do BOTH (TUI live-bucketing + authoritative final total)

**Resolved: (a) AND (b).** They serve different display moments and are not redundant.

- **(a) TUI buckets existing `EventTokensUsage` by `subagent_id`** — free. Verified
  fact: child agents already emit `EventTokensUsage` with `subagent_id` +
  `input_tokens`/`output_tokens` in Meta (via `mergeSubagentMeta`). This gives the
  telemetry panel a **live running total** while a subagent is active. Zero backend
  work; this is a `tui-rail-panels` accumulator.
- **(b) Add an authoritative `tokens` total to `EventSubagentCompleted`** — small
  backend work. `budgetMonitor` already parses `turnCost`; have it also accumulate
  `input_tokens + output_tokens` into a new `subRecord.tokens int` field, and emit
  `"tokens": strconv.Itoa(rec.tokens)` in the `finalize` Meta. This gives the panel a
  **single authoritative final number** on completion, so the row freezes at a correct
  total instead of whatever the last live event happened to carry.

Rationale for both: (a) alone leaves the row showing a possibly-truncated live total at
completion (the completion event is not an `EventTokensUsage`, so the last bucketed
value may lag). (b) alone shows nothing until the subagent finishes. Together: live
feedback during the run, authoritative correction at the end. The backend cost of (b) is
~8 LOC and is purely additive (new internal field + new Meta key, zero-value safe).

**Concrete seam:**

```go
// subagent_manager.go — subRecord gains a tokens accumulator
type subRecord struct {
    // ...existing fields (cost float64, turns int, ...)...
    tokens int // NEW — cumulative input+output tokens across the subagent's turns
}

// subagent_manager.go — budgetMonitor (where turnCost is already parsed):
in, _  := strconv.Atoi(ev.Meta["input_tokens"])
out, _ := strconv.Atoi(ev.Meta["output_tokens"])
rec.tokens += in + out

// subagent_manager.go — finalize Meta build, before emitting EventSubagentCompleted:
"tokens": strconv.Itoa(rec.tokens),
```

### Decision 3 — Memory-change seam: IN scope, signal-plus-thin-payload, injected at 4 sites

> **⚠ SUPERSEDED IN PART by design ADR-5 (binding).** Two details below were corrected
> in the design phase after grounding against the real `notify` contract:
>
> 1. **Payload → BARE SIGNAL, not thin-payload.** `EventMemoryChanged` carries Meta
>    `scope_id` + `entry_id` ONLY; the TUI refetches via `store.SearchMemory` — mirroring
>    the proven `EventTodolistChanged` → `fetchTodolist` pattern (single source of truth,
>    no `title`/`cluster` payload to keep in sync).
> 2. **`StreamingSkipSet` → NOT added.** (Correcting the rationale: `StreamingSkipSet`
>    only makes the _rules engine_ skip notification-rule matching — `rules.go:58` — it
>    does NOT block bus delivery to the TUI's `handleBusEvent`. Proof: `EventTokensUsage`
>    IS in the skipset (`events.go:100`) yet the TUI consumes it for the context meter.)
>    The real reason: the skipset holds high-frequency streaming deltas (chunks/reasoning/
>    tool/tokens). `EventMemoryChanged` is a LOW-frequency lifecycle signal, so it follows
>    the `EventTodolistChanged` precedent (`events.go:43`, "Must NOT be in StreamingSkipSet"):
>    registered in `KnownEventTypes`, deliberately OUT of `StreamingSkipSet` so a user CAN
>    configure a notification rule for memory changes. With no default rule it stays silent.
>    Site count: curator + consolidator + memory-tool + the two legacy `loop.go` paths = 5
>    emit sites (design ADR-5). The Affected-Areas table below already lists all of them.

**Resolved: IN scope.** The Phase-2 exploration explicitly lists `memory-peek` as
requiring `tui-backend-seams`. Deferring it would leave the panel permanently blocked
and split a single cohesive "expose Phase-2 data" change across two PRs. The injection
cost is moderate but entirely additive — honest tradeoff stated below.

**Event:**

```go
// events.go — new constant; registered in KnownEventTypes AND StreamingSkipSet
EventMemoryChanged = "agent.memory.changed" // emitted after a successful AppendMemory
```

**Payload — thin payload, not signal-only, not full entry.** Carry just enough to
render a peek row without a store round-trip, but no large blobs:

```
Meta["scope_id"] — memory scope
Meta["entry_id"] — new entry ID
Meta["title"]    — truncated title (display)
Meta["cluster"]  — cluster bucket (display)
```

Rationale (resolves exploration Q3): a signal-only event forces the TUI to issue a
`fetchRecentMemoriesCmd` store read on every memory write; a full-entry payload bloats
the Meta string with content. A thin title+cluster payload renders the most-recent row
immediately AND lets the panel batch a deeper `fetchRecentMemoriesCmd` only when the
user focuses the panel. Best of both.

**Bus injection — honest cost.** This is the most cross-cutting part of the change. Four
production `AppendMemory` sites must publish; the bus reaches them differently:

| Site                                 | Struct           | Bus access today          | Injection                                                                |
| ------------------------------------ | ---------------- | ------------------------- | ------------------------------------------------------------------------ |
| `loop.go:616,628` (legacy paths)     | `Agent`          | `a.bus` already present   | none — emit directly, nil-guarded                                        |
| `curator.go:441` (`Curator.Curate`)  | `Curator`        | none (`curator.go:65-74`) | add `bus notify.Bus` field, wire at construction                         |
| `consolidator.go:260`                | `Consolidator`   | none                      | add `bus notify.Bus` field, wire at construction                         |
| `tool/memory.go:179` (`save_memory`) | `MemoryToolDeps` | none                      | add `Bus notify.Bus` field (public struct, additive, zero-value = no-op) |

Tradeoff: three structs gain a bus field. None is a breaking change — every field is
nil-zero-safe (`if bus != nil { bus.Emit(...) }`), and `MemoryToolDeps` is a public
struct so the new field is purely additive for external constructors. The wiring touches
construction sites for `Curator` and `Consolidator` (agent-internal, easy) and the
`save_memory` tool dependency assembly. This is the one MEDIUM-effort seam in the change;
it is called out explicitly so the spec/design phase budgets for it.

Engram boundary (confirmed, non-negotiable): engram is an EXTERNAL MCP process. daimon
has zero in-process visibility into engram's store. `EventMemoryChanged` reflects ONLY
daimon's own `store.MemoryEntry` writes. The `memory-peek` panel is, by construction, a
view of daimon's local memory store — never engram observations. This must be stated in
the panel's spec so users are not misled.

### Decision 4 — Context-window accessor (trivial, confirmed)

**Resolved: confirm as specified in the exploration.**

```go
// internal/agent/agent_accessors.go (~3 lines)
func (a *Agent) ContextWindowSize() int {
    if a.contextMgr == nil {
        return 0
    }
    return a.contextMgr.MaxTokens() // already public at context_manager.go:98
}
```

Read ONCE at TUI construction (`internal/tui/run.go:52` / rail construction
`internal/tui/rail.go:26`), passed as an `int` into the `contextMeterPanel`
constructor and stored as a static field. NO bus event — this value is fixed at agent
boot and never changes per turn, so it does not violate `View=pure(Model)` (static
construction-time reads are explicitly allowed). Sentinel: `0` means "unknown"; the
panel falls back to its `200_000` heuristic and may label it "(est.)".

### Decision 5 — Scope cut (confirmed)

**IN:** the four Phase-2 data seams above (Decisions 1-4). **DEFERRED:** session
metadata (Datum 6, needs store schema → Phase 3), MCP server status (Datum 7 → Phase 3),
approvals (Phase 3), and event timestamps on thread items (Datum 5, zero backend work →
folds into `tui-rail-panels`). `tui-rail-panels` is the named downstream consumer that
wires every seam this change exposes; no panel work happens here.

### Decision 6 — Event-shape philosophy: per-datum typed fields, NOT a new batched snapshot event

**Resolved: per-datum typed fields on existing events.**

- Category tokens → typed fields (`SysToks`/`MsgToks`/`ToolToks int`) on `notify.Event`,
  riding the existing `EventTokensUsage`. This matches the established `TokenCount` /
  `CostUSD` typed-field pattern (resolves exploration Q4), avoids string→int parsing in
  the TUI, and is additive (omitempty / zero-value safe).
- Subagent tokens → one Meta key on the existing `EventSubagentCompleted` (Meta is the
  established carrier for subagent attribution; a typed field there would be wasted on
  every non-subagent event).
- Memory change → a dedicated `EventMemoryChanged` event (genuinely new lifecycle
  signal with no existing host event).

Rejected: a single new batched `EventTUISnapshot`. It would (a) duplicate data already
on `EventTokensUsage`/`EventSubagentCompleted`, (b) force a new emit cadence decision,
and (c) give the TUI a second, competing source of truth — directly at odds with the
"one datum, one event, REPLACE on arrival" model that keeps `View=pure(Model)` simple.
Riding existing events keeps each emit nil-guarded by the guard that already wraps it.

---

## Affected Areas

| File                                                                      | Change                                                                                                                                                     | Datum |
| ------------------------------------------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------- | ----- |
| `internal/notify/events.go`                                               | Add `EventMemoryChanged` const; register in `KnownEventTypes` + `StreamingSkipSet`                                                                         | 3     |
| `internal/notify/bus.go`                                                  | Add `SysToks`, `MsgToks`, `ToolToks int` (omitempty) to `Event`                                                                                            | 1     |
| `internal/agent/context_manager.go`                                       | Add `Tools` to `TokenUsage`; add `lastUsage` field + `LastUsage()` method; populate breakdown on all `Manage` paths                                        | 1     |
| `internal/agent/loop.go`                                                  | Emit `SysToks/MsgToks/ToolToks` via `LastUsage()` on `EventTokensUsage` (~line 1015); emit `EventMemoryChanged` at legacy `AppendMemory` paths (~616, 628) | 1, 3  |
| `internal/agent/agent_accessors.go`                                       | Add `ContextWindowSize() int`                                                                                                                              | 4     |
| `internal/agent/subagent_manager.go`                                      | Add `subRecord.tokens`; accumulate in `budgetMonitor` (~454); emit `"tokens"` in `finalize` Meta (~541)                                                    | 2     |
| `internal/agent/curator.go`                                               | Add `bus notify.Bus` field; emit `EventMemoryChanged` after `AppendMemory` (~441)                                                                          | 3     |
| `internal/agent/consolidator.go`                                          | Add `bus notify.Bus` field; emit `EventMemoryChanged` after `AppendMemory` (~260)                                                                          | 3     |
| `internal/tool/memory.go`                                                 | Add `Bus notify.Bus` to `MemoryToolDeps`; emit `EventMemoryChanged` after `AppendMemory` (~179)                                                            | 3     |
| construction/wiring sites for `Curator`, `Consolidator`, `MemoryToolDeps` | Pass the bus in                                                                                                                                            | 3     |

No TUI files are touched by this change (they belong to `tui-rail-panels`).

---

## Risks

| Risk                                                      | Severity | Mitigation                                                                                                                                                                                                                                     |
| --------------------------------------------------------- | -------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| **Category tokens double-counting**                       | MEDIUM   | `SysToks/MsgToks/ToolToks` are window-fill snapshots, NOT deltas. The consumer MUST REPLACE on each event, never accumulate. Spec this in `tui-rail-panels`. (Contrast `TokenCount` = delta.)                                                  |
| **Cross-cutting Curator/Consolidator/tool bus injection** | MEDIUM   | Three structs gain a `bus notify.Bus` field. All emits are `if bus != nil` guarded; nil = no-op. `MemoryToolDeps` is public → additive zero-value field. Wiring is mechanical at known construction sites.                                     |
| **Bus nil-panic at new emit sites**                       | LOW      | Every new emit follows the established guard (`if a.bus != nil` / `if cm.bus == nil { return }`); `EventBus.Emit` also checks `b.closed`.                                                                                                      |
| **Compute-vs-store staleness of `LastUsage()`**           | LOW      | `Manage` writes `lastUsage` on EVERY strategy path (smart/legacy/none) before returning, so the cache always reflects the current turn. Mutex-guarded read/write (same lock as the rest of `ContextManager`).                                  |
| **Category fields zero on non-smart strategies**          | LOW      | Legacy/none populate the breakdown too (helpers already exist); even if zero, the panel's `hasData` flag fires from non-zero `TokenCount`.                                                                                                     |
| **`resolvedMaxToks == 0` when provider unknown**          | LOW      | `ContextWindowSize()` returns `0`; TUI falls back to the 200k heuristic (documented sentinel).                                                                                                                                                 |
| **Subagent token drift**                                  | LOW      | `budgetMonitor` accumulates from Meta `input_tokens + output_tokens`, matching loop.go's `totalInputTokens + totalOutputTokens`. Estimation drift is acceptable for a telemetry display; the final `tokens` Meta is authoritative for the row. |

---

## Decisions (resolved)

- [x] **D1 — Category-context seam:** Option B — cache `LastUsage() TokenUsage` in
      `ContextManager`; add `Tools` to `TokenUsage`; emit `SysToks/MsgToks/ToolToks` typed
      fields on `EventTokensUsage` with REPLACE semantics. (Option A rejected: systemPrompt/
      toolDefs are out of scope at the emit site.)
- [x] **D2 — Per-subagent telemetry:** BOTH — TUI buckets existing `EventTokensUsage`
      `subagent_id` Meta for live totals, AND add authoritative `tokens` Meta to
      `EventSubagentCompleted` (backed by `subRecord.tokens`) for the final number.
- [x] **D3 — Memory-change seam:** IN scope — new `EventMemoryChanged` with thin
      `scope_id`/`entry_id`/`title`/`cluster` Meta payload; inject `bus` into `Curator`,
      `Consolidator`, `MemoryToolDeps`; emit at all 4 `AppendMemory` sites. Engram store is
      out of reach by design.
- [x] **D4 — Context-window accessor:** confirm `agent.ContextWindowSize() int`, read
      ONCE at TUI boot; `0` = unknown sentinel.
- [x] **D5 — Scope cut:** IN = D1-D4; DEFERRED = session metadata, MCP status, approvals
      (Phase 3) + thread-item timestamps (→ `tui-rail-panels`).
- [x] **D6 — Event shape:** per-datum typed fields / Meta keys on existing events; new
      dedicated event only for the genuinely-new memory lifecycle signal. No batched
      snapshot event.

---

## Rollback

Every seam is additive and independently revertible:

- Revert the `notify.Event`/`TokenUsage`/`LastUsage` additions → `EventTokensUsage`
  reverts to `TokenCount`+`CostUSD` only; consumers ignoring the new fields are
  unaffected.
- Revert `subRecord.tokens` + Meta key → `EventSubagentCompleted` reverts to its prior
  Meta set; the `"tokens"` key simply disappears (consumers read it with a zero default).
- Revert `EventMemoryChanged` + bus injection → memory writes stop emitting; the
  `memory-peek` panel shows empty (its construction is nil-bus-safe). No other subsystem
  depends on the event.
- Revert `ContextWindowSize()` → the panel falls back to its heuristic.

No data migration, no persisted schema change, no removal of any existing field. A full
revert restores the exact prior bus contract.

---

## Success Criteria

1. `agent.ContextWindowSize()` returns the resolved window size (or `0` when unknown) and
   is read exactly once at TUI construction.
2. `EventTokensUsage` carries `SysToks`/`MsgToks`/`ToolToks` reflecting the current
   turn's window fill (REPLACE semantics), populated on smart/legacy/none strategies.
3. `EventSubagentCompleted.Meta["tokens"]` carries an authoritative cumulative
   input+output token total for the subagent; live per-subagent totals are also derivable
   from tagged `EventTokensUsage` events.
4. `EventMemoryChanged` is emitted after every successful production `AppendMemory`
   (4 sites), carries `scope_id`/`entry_id`/`title`/`cluster`, is registered in
   `KnownEventTypes` + `StreamingSkipSet`, and never triggers a user notification rule.
5. All new emits are nil-bus-safe; with `bus == nil` no panic occurs on any path.
6. No existing `notify.Event` consumer, subagent Meta reader, or test breaks: every
   addition is omitempty / zero-value safe (verified by `make test`).
7. No TUI file is modified by this change.

---

## SDD session config

- artifact_store: `openspec`
- exec mode: `Automatic`
- delivery_strategy: `ask-on-risk`
- strict TDD: ON — test runner `make test`
- UI stack: Charm v1 only
