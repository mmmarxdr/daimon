# Tasks: tui-backend-seams

> Strict TDD active. Every seam: RED (failing test) → GREEN (impl) → REFACTOR.
> Scope: `internal/agent`, `internal/notify`, `internal/tool` only. Zero TUI files.

---

## Review Workload Forecast

| Field                   | Value                            |
| ----------------------- | -------------------------------- |
| Estimated changed lines | ~290–360 (impl ~160, tests ~140) |
| 400-line budget risk    | Medium                           |
| Chained PRs recommended | No                               |
| Suggested split         | Single PR                        |
| Delivery strategy       | ask-on-risk                      |
| Chain strategy          | pending                          |

Decision needed before apply: No
Chained PRs recommended: No
Chain strategy: pending
400-line budget risk: Medium

### Suggested Work Units

| Unit | Goal                           | Likely PR | Notes                                      |
| ---- | ------------------------------ | --------- | ------------------------------------------ |
| 1    | All four backend seams + tests | PR 1      | Self-contained; backend only; ~300-360 LOC |

---

## Seam 1 — ADR-1: `TokenUsage.Tools` + `ContextManager.LastUsage()`

**File:** `internal/agent/context_manager.go`
**Test file:** `internal/agent/context_manager_test.go`
**Spec scenarios:** "Smart strategy populates category fields", "Non-smart strategy emits zero", ADR-7 graceful degradation

### RED

- [ ] 1.1 In `context_manager_test.go`, add `TestContextManager_LastUsage_BeforeManage`: construct a `ContextManager` with `strategy="smart"`, call `LastUsage()` without calling `Manage()`, assert `(TokenUsage{}, false)`. Run `make test` — expect compile error (method absent).
- [ ] 1.2 Add `TestContextManager_LastUsage_AfterSmartManage` table: cases for below-threshold turn, above-threshold turn. Both must assert `hasBreakdown==true` AND `SystemPrompt > 0`, `Messages > 0`, `Tools >= 0`, `Total > 0`. KEY case: below-threshold turn still updates the cache (guards the "write before early return" invariant).
- [ ] 1.3 Add `TestContextManager_LastUsage_LegacyStrategy`: strategy `"legacy"` and `"none"` — after `Manage()`, `LastUsage()` returns `(TokenUsage{}, false)`.
- [ ] 1.4 Add `TestContextManager_LastUsage_Race`: spawn N goroutines alternating `Manage()` and `LastUsage()`; run `go test -race`. Asserts no data race.

### GREEN

- [ ] 1.5 In `context_manager.go`, add `Tools int` field to `TokenUsage` struct (line ~14) adjacent to existing fields.
- [ ] 1.6 Add `lastUsage TokenUsage` and `hasBreakdown bool` to `ContextManager` struct (line ~38), protected by existing `cm.mu`.
- [ ] 1.7 In `smartManage` (line ~189), insert cache write BEFORE the `if total < threshold { return messages }` guard (line ~200):
  ```go
  cm.lastUsage = TokenUsage{SystemPrompt: sysToks, Messages: msgToks, Tools: toolToks, Total: total, Max: max, UsagePercent: pctOf(total, max)}
  cm.hasBreakdown = true
  ```
- [ ] 1.8 Add `LastUsage()` accessor that acquires `cm.mu` and returns `(cm.lastUsage, cm.hasBreakdown)`.
- [ ] 1.9 Run `make test` — 1.1–1.4 must pass.

### REFACTOR

- [ ] 1.10 Confirm `pctOf(total, max)` helper exists or inline the expression; no new exported symbols needed.

---

## Seam 2 — ADR-2 + ADR-3: `EventTokensUsage` category fields + emit site

**Files:** `internal/notify/bus.go`, `internal/agent/loop.go`
**Test files:** `internal/notify/events_test.go`, `internal/agent/loop_tokens_usage_test.go`
**Spec scenarios:** "Smart strategy populates category fields", "Non-smart strategy emits zero for category fields", "Category fields do not affect existing EventTokensUsage consumers", "New Event fields are zero on unrelated event types"

### RED

- [ ] 2.1 In `internal/notify/events_test.go`, add `TestEvent_CategoryFields_OmitEmpty`: JSON-marshal an `Event` with zero `SysToks/MsgToks/ToolToks`; assert keys are absent. Non-zero values → keys present. Run `make test` — compile error (fields absent).
- [ ] 2.2 In `loop_tokens_usage_test.go`, add `TestEmitTokensUsage_PopulatesCategoryFields`: wire a fake `ContextManager` mock (or use a real one with `strategy="smart"`) whose `LastUsage()` returns a known breakdown `(sysToks=1500, msgToks=4200, toolToks=800, hasBreakdown=true)`; run a minimal agent turn; assert captured `EventTokensUsage` has `SysToks==1500`, `MsgToks==4200`, `ToolToks==800`.
- [ ] 2.3 Add `TestEmitTokensUsage_NilOrNoBreakdown_ZeroCategories`: nil `contextMgr` and `hasBreakdown==false` cases → all three fields are `0`.

### GREEN

- [ ] 2.4 In `internal/notify/bus.go`, add three fields to `Event` struct adjacent to `TokenCount`/`CostUSD`:
  ```go
  SysToks  int `json:"sys_toks,omitempty"`
  MsgToks  int `json:"msg_toks,omitempty"`
  ToolToks int `json:"tool_toks,omitempty"`
  ```
- [ ] 2.5 In `loop.go` at the `EventTokensUsage` emit site (~line 1015), insert nil-guarded `LastUsage()` read before the event literal:
  ```go
  var sysT, msgT, toolT int
  if a.contextMgr != nil {
      if u, ok := a.contextMgr.LastUsage(); ok {
          sysT, msgT, toolT = u.SystemPrompt, u.Messages, u.Tools
      }
  }
  ```
  Then add `SysToks: sysT, MsgToks: msgT, ToolToks: toolT` to the existing event literal.
- [ ] 2.6 Run `make test` — 2.1–2.3 must pass.

### REFACTOR

- [ ] 2.7 Grep for all `EventTokensUsage` subscribers (`internal/tui/screen_chat.go`, `rail_panels.go`, web `/ws/chat` handler) to confirm additive `omitempty` int fields don't break them. No code changes expected — this is a verification checkpoint, document result in commit message.

---

## Seam 3 — ADR-4: `subRecord.tokens` accumulator + `EventSubagentCompleted` Meta key

**File:** `internal/agent/subagent_manager.go`
**Test file:** `internal/agent/subagent_manager_test.go`
**Spec scenarios:** "Authoritative total on EventSubagentCompleted", "EventSubagentCompleted without tokens (zero-accumulation guard)"

### RED

- [ ] 3.1 In `subagent_manager_test.go`, add `TestSubagentTokens_Accumulate` table: feed `budgetMonitor` a sequence of `EventTurnCompleted` with varying `input_tokens`/`output_tokens` Meta (including missing/malformed keys). Assert `rec.tokens` equals running sum; malformed → no increment.
- [ ] 3.2 Add `TestSubagentCompleted_PublishesTokensMeta`: after `finalize`, the emitted `EventSubagentCompleted.Meta["tokens"]` equals the accumulated total; `strconv.Atoi` succeeds; zero-turn case emits `"0"` (not absent).

### GREEN

- [ ] 3.3 In `subRecord` struct (`subagent_manager.go` ~line 36), add `tokens int` beside `cost float64` and `turns int`.
- [ ] 3.4 In `budgetMonitor` (~line 456), inside the existing `rec.mu.Lock()` block after the `turnCost` parse, add:
  ```go
  if n, err := strconv.Atoi(ev.Meta["input_tokens"]); err == nil { rec.tokens += n }
  if n, err := strconv.Atoi(ev.Meta["output_tokens"]); err == nil { rec.tokens += n }
  ```
- [ ] 3.5 In `finalize` (~line 541), inside the `meta` map literal, capture `tokens` before `rec.mu.Unlock()` (alongside `cost`/`turns`), then add `"tokens": strconv.Itoa(tokens)` to the Meta map.
- [ ] 3.6 Run `make test` — 3.1–3.2 must pass.

### REFACTOR

- [ ] 3.7 Verify `tokens` is read under `rec.mu.Lock()` inside `finalize` consistently with how `cost` and `turns` are read (currently captured as locals at line ~509); match the same pattern.

---

## Seam 4 — ADR-5: `EventMemoryChanged` — registration, bus injection, emit sites

**Files:** `internal/notify/events.go`, `internal/agent/curator.go`, `internal/agent/consolidator.go`, `internal/tool/memory.go`, `internal/agent/loop.go`, `internal/agent/agent.go`
**Test files:** `internal/notify/events_test.go`, `internal/agent/curator_test.go`, `internal/agent/consolidator_test.go`, `internal/tool/memory_test.go`
**Spec scenarios:** "Curator write emits EventMemoryChanged", "save_memory tool emits EventMemoryChanged", "Nil bus does not panic on memory write", "Nil bus on MemoryToolDeps does not panic", "Failed AppendMemory does not emit the event"

### RED

- [ ] 4.1 In `internal/notify/events_test.go`, add `TestEventMemoryChanged_Registered`: assert `KnownEventTypes["agent.memory.changed"] == true` AND `StreamingSkipSet["agent.memory.changed"] == false`. Run `make test` — fails (constant absent).
- [ ] 4.2 In `curator_test.go`, add `TestCurator_EmitsMemoryChanged`: construct `Curator` with a fake recording bus via `SetBus`; call `Curate` with content that passes classification; assert exactly one `EventMemoryChanged` emitted with `Meta["scope_id"]` non-empty and `Meta["entry_id"]` non-empty.
- [ ] 4.3 Add `TestCurator_NilBus_NoPanic`: `Curator` with `bus==nil`; `Curate` succeeds; zero events emitted; no panic.
- [ ] 4.4 Add `TestCurator_FailedAppendMemory_NoEvent`: mock store returns error from `AppendMemory`; assert no `EventMemoryChanged` emitted.
- [ ] 4.5 In `consolidator_test.go`, add `TestConsolidator_EmitsMemoryChanged` and `TestConsolidator_NilBus_NoPanic` (mirror of curator tests).
- [ ] 4.6 In `internal/tool/memory_test.go`, add `TestSaveMemoryTool_EmitsMemoryChanged` (non-nil bus → one event, `scope_id` + `entry_id` present) and `TestSaveMemoryTool_NilBus_NoPanic`.

### GREEN

- [ ] 4.7 In `internal/notify/events.go`, add constant: `EventMemoryChanged = "agent.memory.changed"`. Register `EventMemoryChanged: true` in `KnownEventTypes`. Do NOT add to `StreamingSkipSet`.
- [ ] 4.8 Add `bus notify.Bus` field to `Curator` struct (`curator.go` ~line 65). Add `SetBus(b notify.Bus)` method on `*Curator`.
- [ ] 4.9 Wire `SetBus` in `agent.go`'s `WithBus` method (~line 478): after existing wiring lines, add `if a.curator != nil { a.curator.SetBus(bus) }`.
- [ ] 4.10 In `curator.go` after the successful `AppendMemory` call (~line 441) and before the `return nil`:
  ```go
  if c.bus != nil {
      c.bus.Emit(notify.Event{Type: notify.EventMemoryChanged, Origin: notify.OriginAgent,
          Meta: map[string]string{"scope_id": scope, "entry_id": entry.ID}})
  }
  ```
  (Also emit on the dedup-update path at ~line 421 if `AppendMemory`/`UpdateMemory` succeeds there — confirm the exact call site.)
- [ ] 4.11 Add `bus notify.Bus` field to `Consolidator` struct (`consolidator.go` ~line 26). Add `SetBus(b notify.Bus)` method. Wire in `WithBus` (~line 486 in agent.go). Emit nil-guarded `EventMemoryChanged` after `AppendMemory` succeeds at consolidator.go ~line 260.
- [ ] 4.12 Add `Bus notify.Bus` field to `MemoryToolDeps` struct (`tool/memory.go` ~line 56). In `saveMemoryTool.Execute`, after the successful `AppendMemory` call (~line 179):
  ```go
  if t.deps.Bus != nil {
      t.deps.Bus.Emit(notify.Event{Type: notify.EventMemoryChanged, Origin: notify.OriginAgent,
          Meta: map[string]string{"scope_id": scope, "entry_id": entry.ID}})
  }
  ```
- [ ] 4.13 In `loop.go`, add nil-guarded `EventMemoryChanged` emit after the fallback `AppendMemory` at ~line 616 and after the legacy-path `AppendMemory` at ~line 628. Both use `a.bus`, `scope` (in scope), and `entry.ID` (in scope).
- [ ] 4.14 Run `make test` — 4.1–4.6 must pass.

### REFACTOR

- [ ] 4.15 Confirm the Curator dedup-update path (~line 421 in curator.go) also emits `EventMemoryChanged` when it writes via `UpdateMemory`; if it does not call `AppendMemory`, no emit is needed there (spec says "after every successful `AppendMemory` call" — update path is out of scope unless it calls AppendMemory).

---

## Seam 5 — ADR-6: `Agent.ContextWindowSize()` accessor

**Files:** `internal/agent/agent_accessors.go` (NEW), `internal/agent/agent_accessors_test.go` (NEW)
**Spec scenarios:** "ContextWindowSize with smart context configured", "ContextWindowSize with no context manager", "TUI falls back to heuristic on zero return"

### RED

- [ ] 5.1 Create `internal/agent/agent_accessors_test.go`. Add `TestAgent_ContextWindowSize` table:
  - case 1: nil `contextMgr` → returns `0`, no panic.
  - case 2: real `ContextManager` with `resolvedMaxToks=200000` → returns `200000`.
    Run `make test` — compile error (file/method absent).

### GREEN

- [ ] 5.2 Create `internal/agent/agent_accessors.go` with:

  ```go
  package agent

  func (a *Agent) ContextWindowSize() int {
      if a.contextMgr == nil {
          return 0
      }
      return a.contextMgr.MaxTokens()
  }
  ```

- [ ] 5.3 Run `make test` — 5.1 must pass.

### REFACTOR

- [ ] 5.4 No refactor needed. `MaxTokens()` already exists at `context_manager.go:98`.

---

## Seam 6 — ADR-7: Cross-cutting nil-bus safety + zero-value invariants

**Spec scenarios:** "Nil EventBus at all new emit sites", "New Event fields are zero on unrelated event types"

### RED

- [ ] 6.1 In `internal/notify/events_test.go`, add `TestEvent_NewFieldsZeroOnUnrelatedTypes`: construct `EventTurnStarted` and `EventToolEnd` events; assert `SysToks==0`, `MsgToks==0`, `ToolToks==0` (Go zero value — compile-time property, verified at runtime).

### GREEN

- [ ] 6.2 No production code needed — zero-value is guaranteed by Go. This task is the test-only guard.

### REFACTOR

- [ ] 6.3 Run full suite with race detector: `go test -race ./internal/agent/... ./internal/notify/... ./internal/tool/...`. All seam tests must pass cleanly.

---

## Implementation Order

Sequential within each seam (RED → GREEN); seams are independent and can proceed in this order: Seam 1 → Seam 2 (depends on Seam 1 for `LastUsage()`) → Seam 3 (independent) → Seam 5 (independent) → Seam 4 (largest, depends on notify constant from task 4.7) → Seam 6 (final race check).

**Total tasks: 35**
