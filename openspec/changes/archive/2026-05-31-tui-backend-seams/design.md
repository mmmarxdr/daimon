# Technical Design: tui-backend-seams

> Phase: design. Inputs: exploration.md (this change), proposal approach (per exploration
> "Recommended scope" + "Open questions"), and direct reads of `internal/notify/events.go`
> and `internal/agent/context_manager.go`. All other file:line anchors come from the
> exploration's verified evidence map and the task brief's verified facts.
>
> NOTE ON ARTIFACT STATE: at design time only `exploration.md` exists on disk for this
> change (no separate `proposal.md`). The exploration's "Recommended scope" + "Open
> questions for the proposal phase" sections ARE the approved approach this design
> finalizes. The four open questions (Q1–Q4) are answered below as binding decisions.

## Executive Summary

Add four small, additive, nil-bus-safe backend seams so the Phase-2 rail panels
(context-meter, telemetry, memory-peek) can render real data while preserving the
`View = pure(Model)` invariant: (1) a public `Agent.ContextWindowSize()` accessor read
once at TUI boot; (2) a cached `ContextManager.LastUsage()` breakdown surfaced as three
new typed `notify.Event` fields on `EventTokensUsage`; (3) a `subRecord.tokens`
accumulator published as a `"tokens"` Meta key on `EventSubagentCompleted`; (4) a new
`EventMemoryChanged` bare-signal event emitted after every `AppendMemory` via a bus
reference injected into `Curator`/`Consolidator`/`MemoryToolDeps`. Every new field is
zero/`omitempty` and degrades to a legacy-safe 0 when the strategy is `none`/`legacy`.

---

## 1. Context & Constraints

### What's blocked

- **context-meter** renders against a hardcoded `const contextLimit = 200_000`
  (rail_panels.go:353) and has no per-category breakdown (system / conversation / tools).
- **telemetry** has the aggregate panel wired but no per-subagent token rows.
- **memory-peek** is unscaffolded and has no bus event for memory mutations.

### Hard constraints (non-negotiable)

1. **`View = pure(Model)`** — established by the archived `tui-render-architecture`
   change (mem #581). The TUI MUST NOT read live backend objects in `Render`. Every
   datum travels: backend compute → `bus.Emit` → `Subscribe` handler → `evCh` →
   `pumpEvents` tea.Cmd → `busEventMsg` → `handleBusEvent` → panel `accumulate` → cached
   Model field → `View` reads field only. Static values (window size) are read ONCE at
   construction.
2. **Additive only** — no breaking changes to `notify.Event`, `TokenUsage`, the `Tool`
   interface, or any public constructor. New fields are zero-value/`omitempty`. New Meta
   keys are absent-safe.
3. **Nil-bus-safe** — every emit site guards `if bus == nil { return }`. This mirrors
   the established pattern (`emitCompactionEvent` at context_manager.go:256–258, and the
   prior nil-bus fix). `EventBus.Emit` additionally checks `b.closed`.
4. **Strict TDD** — every production change is preceded by a failing table-test; runner
   is `make test` (`go test -timeout 300s ./...`), race via `make test-race`.

### Convention baseline (read directly from `events.go`)

- `notify.Event` already favors **typed fields for numeric per-turn metrics**
  (`TokenCount int`, `CostUSD float64`, `DurationMs int64`, `Iteration int`) and the
  `Meta map[string]string` for **string-keyed structured/extensible attribution**
  (`conv_id`, `subagent_id`, `input_tokens`, `output_tokens`, etc.). This split is the
  decisive precedent for Decisions 2 and 5 below.
- Adding a new event constant requires registering it in `KnownEventTypes`
  (events.go:66) and deciding whether it belongs in `StreamingSkipSet` (events.go:95).

---

## 2. Architecture Decisions

The seven decisions the proposal deferred, each: decision → rationale → concrete
signature/diff-in-prose → file:line anchor.

### ADR-1 — `ContextManager.LastUsage()`: cache the breakdown, don't re-estimate

**Decision.** Resolve open-question Q1 in favor of **Option B (cache)**, NOT Option A
(re-estimate at the emit site). Add a single cached `TokenUsage` field to
`ContextManager`, populated inside `Manage()` after the per-category breakdown is
computed, and expose it via:

```
func (cm *ContextManager) LastUsage() (TokenUsage, bool)
```

The `bool` is `hasBreakdown` — `false` until at least one `Manage()` call on the `smart`
strategy has computed a category breakdown; `true` thereafter. Consumers MUST treat
`false` as "no breakdown available → render from total only".

**Why cache over re-estimate.** The exploration's Q1 leaned toward Option A for
simplicity, but two facts (verified in the task brief and `context_manager.go`) make
caching strictly better:

1. `smartManage` (context_manager.go:189–192) ALREADY computes `sysToks`, `toolToks`,
   `msgToks` as locals every turn. Re-estimating at loop.go would duplicate
   `EstimateTokens(systemPrompt)` + `estimateToolDefTokens(toolDefs)` — and the loop
   emit site does NOT have `sysToks`/`toolToks` in scope (exploration §Datum-2 caveat),
   so Option A means re-deriving values the manager just threw away.
2. The `Usage()` method (context_manager.go:104) already returns a `TokenUsage` but
   `TokenUsage` lacks a `Tools` field — so even calling `Usage()` again would NOT give
   the tool breakdown. Caching the FULL breakdown is the only path that captures tools.

**Concrete change.** Three edits in `context_manager.go`:

1. Extend `TokenUsage` (context_manager.go:14) with a `Tools int` field — additive,
   existing `Usage()` callers get the zero value for free:
   ```
   type TokenUsage struct {
       SystemPrompt int
       Messages     int
       Tools        int   // NEW — tool-definition token estimate
       Total        int
       Max          int
       UsagePercent float64
   }
   ```
2. Add two fields to the `ContextManager` struct (context_manager.go:29–39), guarded by
   the EXISTING `cm.mu sync.Mutex`:
   ```
   lastUsage    TokenUsage // last computed per-category breakdown (smart path only)
   hasBreakdown bool       // true once smartManage has populated lastUsage
   ```
3. In `smartManage`, immediately after the three locals are computed
   (context_manager.go:189–193), populate the cache BEFORE the early-return branches
   (so the breakdown reflects the CURRENT window fill even on turns that do not
   compact):
   ```
   cm.lastUsage = TokenUsage{
       SystemPrompt: sysToks,
       Messages:     msgToks,
       Tools:        toolToks,
       Total:        total,
       Max:          max,
       UsagePercent: pctOf(total, max),
   }
   cm.hasBreakdown = true
   ```
   This sits inside `Manage()`'s `cm.mu.Lock()`/`defer cm.mu.Unlock()` (acquired at
   context_manager.go:137–138), so the write is already serialized.
4. Add the accessor (acquires the same mutex for the read):
   ```
   func (cm *ContextManager) LastUsage() (TokenUsage, bool) {
       cm.mu.Lock()
       defer cm.mu.Unlock()
       return cm.lastUsage, cm.hasBreakdown
   }
   ```

**Placement of the early-cache write.** It MUST go before the `if total < threshold {
return messages }` guard at context_manager.go:200, otherwise the breakdown only updates
on turns that breach the compaction threshold — i.e., the meter would freeze below
threshold. Write the cache first, then run the existing compaction decision unchanged.

**Concurrency verdict: SAFE.** See §4 — `ContextManager` is explicitly documented
"safe for concurrent use; mutable state is protected by a mutex" (context_manager.go:28)
and `Manage()` already takes `cm.mu` for its whole body. The new field writes happen
inside that lock; `LastUsage()` takes the same lock. No new lock, no atomic, no
goroutine-confinement assumption required.

### ADR-2 — `EventTokensUsage` category fields: typed struct fields, REPLACE semantics

**Decision.** Resolve Q4 in favor of **typed fields on `notify.Event`**, not Meta
strings. Add three fields (placed adjacent to the existing `TokenCount`/`CostUSD` block
in `notify/bus.go`):

```
SysToks  int `json:"sys_toks,omitempty"`
MsgToks  int `json:"msg_toks,omitempty"`
ToolToks int `json:"tool_toks,omitempty"`
```

**Why typed over Meta.** The `events.go`/`bus.go` convention (verified) uses typed
fields for numeric per-turn metrics (`TokenCount`, `CostUSD`, `DurationMs`) and Meta for
string attribution. These three are numeric window-fill metrics → typed fields are
consistent, avoid `strconv.Atoi` in the TUI accumulate path, and `omitempty` keeps them
out of JSON when zero (legacy/none paths).

**Consumer semantics: REPLACE, not accumulate.** This is the single most important
consumer contract (exploration Risk "Category tokens double-counting", MEDIUM). Unlike
`TokenCount` (a per-turn delta the telemetry panel SUMS), `SysToks`/`MsgToks`/`ToolToks`
describe the CURRENT total window fill at turn end. The context-meter panel MUST
overwrite its cached category fields on each `EventTokensUsage`, never add. State this
explicitly so the spec/tasks can assert it:

> On each `EventTokensUsage`, `contextMeterPanel.accumulate` sets
> `p.sysToks = ev.SysToks; p.msgToks = ev.MsgToks; p.toolToks = ev.ToolToks`
> (assignment, not `+=`).

### ADR-3 — Emit-site change at loop.go (~1016): populate from `LastUsage()`, nil-guarded

**Decision.** At the existing `EventTokensUsage` emit site (loop.go ~1016, the brief's
verified location), read the cached breakdown and populate the three new fields
additively. No other field on the existing event changes.

**Concrete change (diff-in-prose).** Immediately before the `a.bus.Emit(notify.Event{
Type: notify.EventTokensUsage, ... })` construction:

```
var sysT, msgT, toolT int
if a.contextMgr != nil {
    if u, ok := a.contextMgr.LastUsage(); ok {
        sysT, msgT, toolT = u.SystemPrompt, u.Messages, u.Tools
    }
}
```

Then add `SysToks: sysT, MsgToks: msgT, ToolToks: toolT` to the existing event literal.
When `contextMgr` is nil OR no breakdown has been computed (legacy/none strategy), all
three stay 0 and serialize away via `omitempty`.

> TASKS-PHASE CONFIRM: the exact emit-site literal and the receiver name (`a`) must be
> confirmed against the live loop.go ~1016 lines; the brief states `subagent_id` +
> input/output tokens are already in Meta there, so the literal exists and is additive.

**Subagent attribution is preserved for free.** Because child agents emit
`EventTokensUsage` through this SAME site (with `subagent_id` already merged into Meta
via `mergeSubagentMeta`, subagent_meta.go:24), the category fields will also appear on
child-tagged token events. The telemetry panel ignores them; the context-meter panel
keys on the ROOT agent's events. No special-casing needed.

### ADR-4 — `EventSubagentCompleted` tokens: accumulate in `subRecord`, publish in Meta

**Decision.** Resolve Q2 in favor of **Option A (backend accumulation)** — authoritative
final count is worth ~8 LOC. Add a `tokens int` accumulator to `subRecord`, increment it
where per-turn cost is already parsed, and publish it as a `"tokens"` Meta key on
`EventSubagentCompleted`.

**Accumulation source (confirmed exists).** `budgetMonitor` (subagent_manager.go:454)
already parses per-turn cost from the event Meta (`turnCost`). The exploration's Risk
table confirms `budgetMonitor` receives `EventTurnCompleted` and that
`Meta["input_tokens"]`/`Meta["output_tokens"]` are present on those events (matching
`totalInputTokens + totalOutputTokens` from loop.go:1010). So the accumulation signal
already flows into `budgetMonitor`; we add parsing of the two token Meta keys alongside
the existing cost parse.

**Concrete change (three edits in `subagent_manager.go`).**

1. `subRecord` struct (subagent_manager.go:36): add `tokens int` beside `cost float64`,
   `turns int`.
2. `budgetMonitor` (subagent_manager.go:454): after the existing `turnCost` parse, add
   ```
   if n, err := strconv.Atoi(ev.Meta["input_tokens"]); err == nil { rec.tokens += n }
   if n, err := strconv.Atoi(ev.Meta["output_tokens"]); err == nil { rec.tokens += n }
   ```
   (under the same lock that already protects `rec`).
3. `finalize` Meta build (subagent_manager.go:541): add
   `"tokens": strconv.Itoa(rec.tokens)` to the `EventSubagentCompleted` Meta map next to
   the existing `"cost_usd"` and `"turns"` keys.

**Why Meta (not a typed field) for subagent tokens.** This is per-SUBAGENT attribution
keyed by `subagent_id` already living in Meta — it belongs in the Meta bag with its
siblings (`cost_usd`, `turns`), consistent with the convention. Contrast ADR-2 where the
root window-fill metrics are typed because they are not attribution-scoped.

> TASKS-PHASE CONFIRM: exact Meta key names emitted on `EventTurnCompleted` from the
> child loop (`input_tokens`/`output_tokens`) and the lock discipline in `budgetMonitor`.

### ADR-5 — `EventMemoryChanged`: BARE SIGNAL (TUI refetches), bus injected into writers

**Decision.** Resolve Q3 in favor of a **bare signal**, NOT a payload — overriding the
exploration's tentative "carry title+cluster+importance" recommendation. Define:

```
// internal/notify/events.go
EventMemoryChanged = "agent.memory.changed" // emitted after AppendMemory succeeds
```

The event carries minimal Meta for routing/debug only — `scope_id` and `entry_id` — and
NO display payload. The TUI treats it exactly like `EventTodolistChanged`: on receipt it
schedules a `fetchRecentMemoriesCmd` round-trip to `store.SearchMemory`, which returns
authoritative, ordered, deduplicated `store.MemoryEntry` rows.

**Why bare signal over payload (overriding exploration Q3).** Three reasons:

1. **Mirror the proven pattern.** `EventTodolistChanged` is a bare signal →
   `fetchTodolist` → `todolistRefreshMsg` → `copyRailWith` → `setList`
   (exploration, mem #586: "fully wired"). memory-peek should be symmetric, not a
   bespoke payload path. Consistency lowers TUI complexity and review surface.
2. **Single source of truth.** A payload in Meta can drift from the store (ordering,
   importance re-ranking, consolidation merges). A refetch always reflects the store's
   real current top-N. The panel shows daimon's OWN curated store — exactly what a
   refetch returns.
3. **Multiple emit sites stay trivial.** Five `AppendMemory` call sites must emit. A
   bare signal means each site emits the same two-key Meta with no need to marshal a
   `store.MemoryEntry` summary at each site (some sites don't have the full entry handy).

The cost is one cheap SQLite/file `SearchMemory` per memory write — debounced naturally
by how rarely memory writes happen (curation/consolidation, not per-token). Acceptable.

**Bus injection (the moderate part).** Five production `AppendMemory` sites
(exploration §Datum-4): two already have `a.bus` (loop.go:616, 628), three need a bus
reference:

- **Curator** (curator.go:65–74): add `bus notify.Bus` field. The construction site and
  the `AppendMemory` call (curator.go:441) get a nil-guarded emit. Confirm the bus is
  threaded in at construction — see the established `SetBus` pattern other agent
  components use (agent.go ~475–481, per brief). RECOMMENDATION: inject via constructor
  param if Curator has one; otherwise add a `SetBus(b notify.Bus)` method mirroring the
  existing pattern, called at the same wiring site agent.go uses for its other
  components. Either way the field defaults nil → no emit, fully safe.
- **Consolidator** (consolidator.go:260): same — add `bus notify.Bus` field +
  nil-guarded emit after `AppendMemory`.
- **MemoryToolDeps** (tool/memory.go:56): add `Bus notify.Bus` field (exported, since
  the struct is public). Nil = no event. Emit at tool/memory.go:179 after
  `AppendMemory` succeeds.

**Emit shape (identical at all five sites):**

```
if bus != nil {
    bus.Emit(notify.Event{
        Type:   notify.EventMemoryChanged,
        Origin: notify.OriginAgent,
        Meta:   map[string]string{"scope_id": scopeID, "entry_id": entryID},
    })
}
```

**Registration.** Add `EventMemoryChanged: true` to `KnownEventTypes` (events.go:66).
Do NOT add it to `StreamingSkipSet` — memory changes are legitimate notification-rule
candidates (unlike high-frequency streaming boundaries), and skipping is only for the
`agent.reasoning.*`/`agent.tool.*`/`agent.tokens.usage` set.

> TASKS-PHASE CONFIRM: Curator's construction site and whether it takes a constructor
> bus or needs `SetBus`; the exact variable names for `scopeID`/`entryID` available at
> each of the five emit sites (the consolidator/curator paths may name them differently).

### ADR-6 — `ContextWindowSize()` accessor: new `agent_accessors.go`, single boot read

**Decision.** Add the public accessor in a NEW file `internal/agent/agent_accessors.go`
(keeps thin public read-shims grouped; avoids touching the large `agent.go`):

```
func (a *Agent) ContextWindowSize() int {
    if a.contextMgr == nil {
        return 0
    }
    return a.contextMgr.MaxTokens()
}
```

`MaxTokens()` is already public (context_manager.go:98) and returns `resolvedMaxToks`
(fixed at construction). `0` is the documented sentinel for "provider unknown" → TUI
falls back to its heuristic.

**Single read site.** Called ONCE at TUI construction in `runTUIWithStdin` (run.go:52,
per exploration evidence map), passed as an `int` into the context-meter panel
constructor and stored as `p.contextLimit int`. NO bus event — this is a static
boot-time value, fully consistent with "static values read once at construction".

### ADR-7 — Graceful degradation contract (the legacy/disabled story)

**Decision.** Define the exact zero-behavior so spec/tasks/tests can assert it:

| Seam                                 | Value on legacy / `none` / nil              | Downstream panel behavior                                       |
| ------------------------------------ | ------------------------------------------- | --------------------------------------------------------------- |
| `ContextWindowSize()`                | `0` (provider unknown or nil mgr)           | TUI uses heuristic 200k fallback                                |
| `SysToks`/`MsgToks`/`ToolToks`       | `0` (no breakdown; `LastUsage()` → `false`) | Render bar from `TokenCount`/total only; HIDE category sub-bars |
| `subRecord.tokens` → `"tokens"` Meta | absent / `"0"`                              | Telemetry row shows `—` for tokens, still shows cost/turns      |
| `EventMemoryChanged`                 | not emitted (nil bus)                       | Panel shows last successful fetch; never panics                 |

**Contract statement (for tests):**

> When the context strategy is `legacy` or `none`, `ContextManager.LastUsage()` returns
> `(TokenUsage{}, false)` and the emitted `EventTokensUsage` has
> `SysToks==MsgToks==ToolToks==0`. A consumer MUST detect the all-zero category triple
> (equivalently, `hasBreakdown==false` upstream) and render the meter from the total
> only, hiding per-category segmentation. No new field ever causes a panic when zero.

---

## 3. Data Flow Diagram

```
DATUM 1 — context window size (static, no bus)
  Agent.contextMgr.resolvedMaxToks
    └─ Agent.ContextWindowSize()           [agent_accessors.go]
        └─ runTUIWithStdin (boot, once)    [run.go:52]
            └─ newContextMeterPanel(limit) → p.contextLimit
                └─ View() reads p.contextLimit   (pure)

DATUM 2 — per-category breakdown (per-turn, bus)
  smartManage: sysToks/msgToks/toolToks     [context_manager.go:189-193]
    └─ cm.lastUsage = {...}; cm.hasBreakdown = true   (under cm.mu)
        └─ loop emit site reads LastUsage()  [loop.go ~1016]
            └─ bus.Emit(EventTokensUsage{SysToks,MsgToks,ToolToks})
                └─ Subscribe → evCh → pumpEvents → busEventMsg
                    └─ handleBusEvent → contextMeterPanel.accumulate (REPLACE)
                        └─ p.sysToks/p.msgToks/p.toolToks
                            └─ View() reads fields   (pure)

DATUM 3 — per-subagent tokens (on completion, bus Meta)
  child loop EventTurnCompleted Meta[input_tokens,output_tokens]
    └─ budgetMonitor: rec.tokens += in+out   [subagent_manager.go:454]
        └─ finalize: Meta["tokens"]          [subagent_manager.go:541]
            └─ bus.Emit(EventSubagentCompleted)
                └─ ... → telemetryPanel.accumulate (per subagent_id)
                    └─ View() reads row   (pure)

DATUM 4 — memory changed (on write, bus signal)
  AppendMemory success @ {curator:441, consolidator:260, memory.go:179, loop:616,628}
    └─ if bus != nil: bus.Emit(EventMemoryChanged{Meta:scope_id,entry_id})
        └─ ... → busEventMsg → schedule fetchRecentMemoriesCmd
            └─ store.SearchMemory → memoryRefreshMsg
                └─ panelMemoryPeek cached rows
                    └─ View() reads rows   (pure)
```

---

## 4. Concurrency & Safety Analysis

### LastUsage cache — VERDICT: SAFE under existing mutex

- `ContextManager` is documented "safe for concurrent use; mutable state is protected by
  a mutex" (context_manager.go:28).
- `Manage()` acquires `cm.mu.Lock()` / `defer cm.mu.Unlock()` for its ENTIRE body
  (context_manager.go:137–138), and `smartManage` runs inside that lock. The new
  `cm.lastUsage`/`cm.hasBreakdown` writes therefore happen under `cm.mu` with zero extra
  work.
- `LastUsage()` acquires `cm.mu` for the read and returns a VALUE copy of `TokenUsage`
  (a flat struct of ints/float) — no shared pointer escapes the lock, so the caller
  cannot race on it after release.
- Caller goroutine: `LastUsage()` is invoked from the agent loop's emit site
  (loop.go ~1016). Even if that runs on a different goroutine than `Manage()`, the shared
  mutex serializes them. No goroutine-confinement assumption is needed, and no atomic is
  required.
- WHY NOT a separate lock: introducing a second mutex would risk lock-ordering bugs
  against `cm.mu`; reusing `cm.mu` is correct and the read is O(1).

### subRecord.tokens — SAFE if it reuses the existing subRecord lock

`subRecord` is mutated from `budgetMonitor` (a bus handler goroutine) and read in
`finalize`. The existing `cost`/`turns` accumulation already runs under whatever lock
guards `subRecord`; `tokens += n` MUST sit under that same lock. No new synchronization.
TASKS-PHASE: confirm the lock that currently guards `rec.cost`/`rec.turns` and place the
token increment inside it.

### Bus emit safety (all new sites)

- Every emit is `if bus != nil { bus.Emit(...) }`. `EventBus.Emit` is non-blocking and
  drops+warns when full; the circuit breaker (1000/min) protects against floods. Memory
  writes are low-frequency, so `EventMemoryChanged` cannot trip the breaker in practice.
- `EventTokensUsage` already fires once per turn; adding three int fields changes
  nothing about its frequency.

### View purity

No seam adds a live-object read in any `View`/`Render`. Datum 1 is read once at boot;
Datums 2–4 are cached into Model fields by `accumulate`/refetch before `View` runs.

---

## 5. Test Strategy (strict TDD, `make test`)

Each seam is unit-testable with table tests in its own package. Failing test precedes
production code.

### internal/agent (context_manager_test.go)

- `TestContextManager_LastUsage_BeforeManage` → `(TokenUsage{}, false)`.
- `TestContextManager_LastUsage_AfterSmartManage` table: cases (below-threshold turn,
  above-threshold turn, post-compaction) all assert `hasBreakdown==true` and that
  `SystemPrompt/Messages/Tools/Total` match the estimator outputs for the fixture
  messages. KEY: assert the cache updates even on the below-threshold path (guards the
  ADR-1 "write before the early return" requirement).
- `TestContextManager_LastUsage_LegacyStrategy` → strategy `legacy`/`none` never sets
  `hasBreakdown`; returns `(TokenUsage{}, false)`.
- Concurrency: `TestContextManager_LastUsage_Race` — spawn N goroutines alternating
  `Manage()` and `LastUsage()`; run under `make test-race`. Asserts no data race.

### internal/notify (events_test.go)

- `TestEventMemoryChanged_Registered` → present in `KnownEventTypes`, ABSENT from
  `StreamingSkipSet`.
- `TestEvent_CategoryFields_OmitEmpty` → JSON-marshal an `Event` with zero
  `SysToks/MsgToks/ToolToks` and assert the keys are omitted; non-zero → present.

### internal/agent — emit site (loop_test.go or a focused emit test)

- `TestEmitTokensUsage_PopulatesCategoryFields` — fake bus + `ContextManager` whose
  `LastUsage()` returns a known breakdown; assert the captured `EventTokensUsage` carries
  matching `SysToks/MsgToks/ToolToks`.
- `TestEmitTokensUsage_NilOrNoBreakdown_ZeroCategories` — nil mgr / `hasBreakdown==false`
  → all three are 0.

### internal/agent — subagent (subagent_manager_test.go)

- `TestSubagentTokens_Accumulate` table: feed budgetMonitor a sequence of
  `EventTurnCompleted` with `input_tokens`/`output_tokens` Meta; assert `rec.tokens`
  equals the running sum, including malformed/absent Meta (parse error → no increment).
- `TestSubagentCompleted_PublishesTokensMeta` → after finalize, the emitted
  `EventSubagentCompleted` Meta has `"tokens"` equal to the accumulated total.

### internal/agent + internal/tool — memory emit

- `TestCurator_EmitsMemoryChanged` — Curator with a fake bus; after `Curate`→
  `AppendMemory`, assert one `EventMemoryChanged` with `scope_id`/`entry_id` Meta.
- `TestCurator_NilBus_NoPanic` — Curator with nil bus; `Curate` succeeds, no emit.
- Mirror for `TestConsolidator_EmitsMemoryChanged` and
  `TestSaveMemoryTool_EmitsMemoryChanged` (MemoryToolDeps with fake bus).

### internal/agent — accessor

- `TestAgent_ContextWindowSize` table: nil contextMgr → 0; wired mgr → `MaxTokens()`
  passthrough.

---

## 6. Alternatives Considered / Rejected

| Alternative                                                                   | Rejected because                                                                                                                                                                                                                                                        |
| ----------------------------------------------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| **Q1-A: re-estimate sys/tool tokens at the loop emit site**                   | The emit site lacks `sysToks`/`toolToks` in scope and `Usage()` has no `Tools` field; re-estimating duplicates work `smartManage` already did AND still can't produce the tool breakdown without extending `TokenUsage`. Caching is cleaner and the only complete path. |
| **Category fields as Meta strings**                                           | Breaks the typed-numeric convention (`TokenCount`/`CostUSD`); forces `strconv` in the TUI accumulate hot-ish path; loses `omitempty` ergonomics.                                                                                                                        |
| **Q2-B: TUI accumulates subagent tokens from live `EventTokensUsage` stream** | Gives only running totals, no authoritative final number, and stops mid-flight on completion. 8 LOC backend buys a single trustworthy source.                                                                                                                           |
| **Q3 payload: carry title/cluster/importance in `EventMemoryChanged` Meta**   | Drifts from the store's real ordering/ranking; bloats Meta; diverges from the proven `EventTodolistChanged` bare-signal pattern; some emit sites lack the full entry. Refetch is cheap and authoritative.                                                               |
| **Accumulate (not replace) category fields in the meter**                     | Would double/triple-count window fill every turn — the values are absolute, not deltas. REPLACE is mandatory.                                                                                                                                                           |
| **Add `ContextWindowSize()` to agent.go directly**                            | Larger file churn; a dedicated `agent_accessors.go` groups thin read-shims and minimizes diff surface.                                                                                                                                                                  |
| **A second mutex for the LastUsage cache**                                    | Risks lock-ordering bugs vs `cm.mu`; the existing mutex already covers the write path for free.                                                                                                                                                                         |

---

## 7. Open Risks (for the tasks phase to handle)

1. **Live-line confirmation — PARTIALLY RESOLVED.** The `EventTokensUsage` emit site
   was re-read directly: loop.go:1001–1029 confirms the literal, the `a` receiver, the
   `if a.bus != nil` guard, and `mergeSubagentMeta` carrying `subagent_id` +
   `input_tokens`/`output_tokens`. ADR-3's diff drops in cleanly. STILL to confirm at
   tasks time: the `budgetMonitor`/`finalize` lock + Meta-key handling in
   subagent_manager.go (~454/~541), and the Curator construction/SetBus site
   (curator.go + agent.go ~475–481).
2. **Subagent token Meta-key parity — RESOLVED.** Direct read confirms BOTH
   `EventTurnCompleted` (loop.go:1008–1012) AND `EventTokensUsage` (loop.go:1022–1027)
   carry `input_tokens`/`output_tokens` in Meta via `mergeSubagentMeta` for child
   agents. So `budgetMonitor` can accumulate from whichever of the two it already
   subscribes to — the keys exist on both. Tasks only need to confirm which event
   `budgetMonitor` consumes and parse those two keys there.

   NOTE: `internal/agent/loop_tokens_usage_test.go` already exists with a
   `filterByType(notify.EventTokensUsage)` recording-bus helper (lines 58, 151) — the
   ADR-2/ADR-3 test harness is in place, lowering TDD cost.

3. **Curator bus threading (MEDIUM).** Whether Curator takes a constructor bus or needs
   a new `SetBus` mirroring the agent.go pattern determines a small wiring task. Pick the
   existing pattern; do not invent a new injection style.
4. **`smartManage` early-cache placement (MEDIUM, correctness).** The cache write MUST
   precede the `total < threshold` early return (context_manager.go:200) or the meter
   freezes below threshold. The race test + the below-threshold table case guard this.
5. **Hidden `EventTokensUsage` consumers (LOW).** Adding typed fields is additive, but
   tasks should grep for all `EventTokensUsage` subscribers (web `/ws/chat`, telemetry)
   to confirm none break on the new fields. They are `omitempty` ints — backward
   compatible by construction — but a quick consumer census is cheap insurance.
