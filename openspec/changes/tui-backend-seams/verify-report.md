# Verify Report: tui-backend-seams

**Date**: 2026-05-31
**Branch**: tui-backend-seams (HEAD cd70ad7)
**Verdict**: PASS WITH WARNINGS
**Status**: 1 WARNING, 2 SUGGESTIONS, 0 CRITICAL

---

## Test Execution Evidence

Command: `go test -race -count=1 ./internal/agent/ ./internal/notify/ ./internal/tool/`

```
ok  	daimon/internal/agent	20.212s
ok  	daimon/internal/notify	7.848s
ok  	daimon/internal/tool	7.137s
EXIT=0
```

All three scope packages pass with race detector. Pre-existing failure in `daimon/cmd/daimon TestRunTUICommand_MissingConfig` is out of scope (confirmed pre-existing).

---

## Backend-Only Boundary Check

`git diff --stat main...HEAD` — result:
- 18 source files changed; ZERO `internal/tui/` files.
- Only `internal/agent/`, `internal/notify/`, `internal/tool/`, and `openspec/` files modified.

**PASS** — additive-only, zero TUI source files touched.

---

## Spec Scenario Coverage Matrix

| # | Scenario | Test | Status |
|---|----------|------|--------|
| 1 | ContextWindowSize with smart context configured | `TestAgent_ContextWindowSize` case 2 (agent_accessors_test.go:103) | COVERED |
| 2 | ContextWindowSize with no context manager | `TestAgent_ContextWindowSize` case 1 (nil → 0) | COVERED |
| 3 | TUI falls back to heuristic on zero return | None — TUI not changed, `newContextMeterPanel` still hardcodes constant | DEFERRED (TUI-rail-panels PR) |
| 4 | Smart strategy populates category fields | `TestEmitTokensUsage_PopulatesCategoryFields` | COVERED (assertion weaker than spec example) |
| 5 | Non-smart strategy emits zero for category fields | `TestEmitTokensUsage_NilOrNoBreakdown_ZeroCategories` + `TestContextManager_LastUsage_LegacyStrategy` | COVERED |
| 6 | TUI context-meter degrades gracefully on zero category fields | None — pure TUI concern | DEFERRED (TUI-rail-panels PR) |
| 7 | Category fields do not affect existing EventTokensUsage consumers | `TestEvent_CategoryFields_OmitEmpty` (structural) | COVERED |
| 8 | Live per-subagent attribution via EventTokensUsage | None — zero new backend work (pre-existing behavior) | NOT REQUIRED |
| 9 | Authoritative total on EventSubagentCompleted | `TestSubagentCompleted_PublishesTokensMeta` case "three turns → 405" | COVERED, exact values asserted |
| 10 | EventSubagentCompleted without tokens (zero-accumulation guard) | `TestSubagentCompleted_PublishesTokensMeta` case "zero turns → 0" | COVERED |
| 11 | Curator write emits EventMemoryChanged | `TestCurator_EmitsMemoryChanged` (curator_test.go:614) | COVERED |
| 12 | save_memory tool emits EventMemoryChanged | `TestSaveMemoryTool_EmitsMemoryChanged` (memory_test.go:382) | COVERED |
| 13 | Nil bus does not panic on memory write | `TestCurator_NilBus_NoPanic` + `TestConsolidator_NilBus_NoPanic` | COVERED |
| 14 | Nil bus on MemoryToolDeps does not panic | `TestSaveMemoryTool_NilBus_NoPanic` | COVERED |
| 15 | Failed AppendMemory does not emit the event | `TestCurator_FailedAppendMemory_NoEvent` | COVERED |
| 16 | New Event fields are zero on unrelated event types | `TestEvent_NewFieldsZeroOnUnrelatedTypes` | COVERED |
| 17 | Nil EventBus at all new emit sites | `TestProcessMessage_TokensUsage_NilBus_NoPanic` + nil-bus curator/consolidator | COVERED |

**Coverage: 12/17 scenarios have real asserting tests. 3 are deferred TUI work. 1 is zero-work pre-existing. 1 uses a weaker assertion.**

---

## ADR Implementation Verification

### ADR-1 — `ContextManager.LastUsage()` cache

- `TokenUsage.Tools int` field added: context_manager.go:17. CONFIRMED.
- `lastUsage TokenUsage` + `hasBreakdown bool` fields on `ContextManager`: context_manager.go:41–43. CONFIRMED.
- Cache write in `smartManage` BEFORE the `threshold` early-return guard: context_manager.go:213–226. CONFIRMED.
- `LastUsage()` acquires `cm.mu.Lock()` for the read: context_manager.go:111–114. CONFIRMED.
- `pctOf` inlined as anonymous closure: context_manager.go:218–224. CONFIRMED (no new exported symbol).
- Below-threshold cache update guarded by test: `TestContextManager_LastUsage_AfterSmartManage` case "below-threshold turn still updates cache". CONFIRMED.
- Legacy/none never sets `hasBreakdown`: `TestContextManager_LastUsage_LegacyStrategy`. CONFIRMED.
- Race test: `TestContextManager_LastUsage_Race` under -race. CONFIRMED.

**ADR-1: COMPLIANT**

### ADR-2 — `SysToks/MsgToks/ToolToks` on `notify.Event`

- Three `int` fields with `json:"...,omitempty"` tags: bus.go:34–36. CONFIRMED.
- REPLACE semantics documented in bus.go comment. CONFIRMED.
- Existing TUI consumers (telemetryPanel, screen_chat, breadcrumb) read only `TokenCount`/`CostUSD`/`Meta` — unaffected by new zero-value fields. CONFIRMED via code inspection.
- `TestEvent_CategoryFields_OmitEmpty` asserts omitempty for both zero and non-zero cases. CONFIRMED.

**ADR-2: COMPLIANT**

### ADR-3 — Emit site in loop.go

- `LastUsage()` read is nil-guarded before the event literal: loop.go:1033–1038. CONFIRMED.
- The emit at loop.go:1039 is inside the `if a.bus != nil` block (lines 1018–1056). CONFIRMED (nil-bus-safe).
- `SysToks: sysT, MsgToks: msgT, ToolToks: toolT` added to the event literal: loop.go:1046–1048. CONFIRMED.

**ADR-3: COMPLIANT**

### ADR-4 — `subRecord.tokens` accumulator

- `tokens int` field on `subRecord`: subagent_manager.go:50. CONFIRMED.
- Accumulation in `budgetMonitor` inside `rec.mu.Lock()`: subagent_manager.go:461–465. CONFIRMED (same lock as `cost` and `turns`).
- `tokens` captured in `finalize` under `rec.mu.Lock()` at line 518. CONFIRMED.
- `"tokens": strconv.Itoa(tokens)` in the Meta map: subagent_manager.go:557. CONFIRMED.
- Zero-turn emits `"0"` not absent: `TestSubagentCompleted_PublishesTokensMeta` confirms. CONFIRMED.

**ADR-4: COMPLIANT**

### ADR-5 — `EventMemoryChanged` bare signal

- Constant `EventMemoryChanged = "agent.memory.changed"` in events.go:51. CONFIRMED.
- In `KnownEventTypes`: events.go:90. CONFIRMED.
- NOT in `StreamingSkipSet`: events.go:105–111 (only streaming types there). CONFIRMED.
- `Curator.SetBus(b notify.Bus)`: curator.go:81. CONFIRMED.
- `Consolidator.SetBus(b notify.Bus)`: consolidator.go:42. CONFIRMED.
- `MemoryToolDeps.Bus notify.Bus`: tool/memory.go:61. CONFIRMED.
- `WithBus` propagates to curator and consolidator: agent.go:487–491. CONFIRMED.
- All 5 emit sites present: curator.go:453, consolidator.go:273, memory.go:186, loop.go:618, loop.go:642. CONFIRMED.
- All emit sites nil-bus-guarded. CONFIRMED.

**Meta payload divergence from spec: spec requires `title` + `cluster` in Meta. Design ADR-5 explicitly overrode to bare signal (`scope_id` + `entry_id` only). Implementation follows ADR-5. See WARNING-1 below.**

**ADR-5: COMPLIANT with design; DIVERGES from spec payload definition.**

### ADR-6 — `Agent.ContextWindowSize()` accessor

- New file `internal/agent/agent_accessors.go` with correct nil-guard: lines 56–61. CONFIRMED.
- Nil contextMgr → 0. Non-nil → `contextMgr.MaxTokens()`. CONFIRMED.
- `TestAgent_ContextWindowSize` table test covers both cases. CONFIRMED.

**ADR-6: COMPLIANT**

### ADR-7 — Graceful degradation

- legacy/none → `LastUsage()` returns `(TokenUsage{}, false)` → all three toks = 0. CONFIRMED.
- `ContextWindowSize()` nil → 0. CONFIRMED.
- `subRecord.tokens` on zero-turn completion → `"0"` in Meta. CONFIRMED.
- `EventMemoryChanged` not emitted on nil bus. CONFIRMED.

**ADR-7: COMPLIANT**

---

## Issues

### WARNING-1: EventMemoryChanged Meta payload diverges from spec definition

**Spec requirement** (spec.md lines 201–211) defines 4 Meta keys:
```
scope_id, entry_id, title, cluster
```
**Implementation** (and all tests) only emit `scope_id` and `entry_id`. Design ADR-5 explicitly chose a bare signal, citing `EventTodolistChanged` pattern, single source of truth, and simplicity at multi-site emit.

**Impact**: Consumers expecting `title`/`cluster` in Meta will find them absent. If the Phase-2 TUI `panelMemoryPeek` was designed against the spec's payload (expecting `title` in the event for display without a refetch), it would need to refetch instead. The design intended this as a deliberate tradeoff; a refetch is cheap and authoritative.

**Disposition**: This is a real spec/implementation divergence in the public event contract. It is intentional per ADR-5 and well-reasoned. Recommend either (a) updating spec.md to match the bare-signal decision, or (b) accepting the deviation with a comment in spec.md that ADR-5 overrode the payload definition.

### SUGGESTION-1: TestEmitTokensUsage_PopulatesCategoryFields uses weak assertions

**Spec scenario** says: "THEN `ev.SysToks == 1500`, `ev.MsgToks == 4200`, `ev.ToolToks == 800`", implying a test with KNOWN breakdown values.

**Actual test** uses a real ContextManager and only asserts `>= 0` and "at least SysToks or MsgToks > 0". The wire is proven connected but the exact round-trip fidelity (smartManage computes values → LastUsage() returns them → event carries them) is not asserted with exact values. Design test strategy specified a "fake ContextManager mock whose LastUsage() returns a known breakdown".

**Consequence**: A bug where the values were accidentally 1/10th the correct size would pass the test. Not a regression risk in practice since the implementation is straightforward, but the test doesn't fully exercise the spec's exact scenario.

**Recommendation**: Enhance test to use a fixed-size provider mock whose EstimateTokens behavior is deterministic and assert exact values, or add a unit test that directly calls the emit path with a known LastUsage().

### SUGGESTION-2: TestConsolidator_EmitsMemoryChanged does not assert entry_id

The curator test asserts both `scope_id` and `entry_id`. The consolidator test only asserts `scope_id`. For symmetry and completeness, `entry_id` should be asserted in the consolidator test as well.

---

## Task Completion Verification

All 35 tasks marked `[x]` in tasks.md match the implementation:
- Seam 1 (ADR-1): 10 tasks — DONE
- Seam 2 (ADR-2/3): 7 tasks — DONE
- Seam 3 (ADR-4): 7 tasks — DONE
- Seam 4 (ADR-5): 15 tasks — DONE (14 + 1 refactor)
- Seam 5 (ADR-6): 4 tasks — DONE
- Seam 6 (ADR-7): 3 tasks — DONE

---

## Summary

PASS WITH WARNINGS. All backend seams are implemented correctly. Tests are green with race detector. Zero TUI files touched. All 7 ADRs are implemented as specified. The one real issue is the spec payload for `EventMemoryChanged` specifying `title`/`cluster` Meta keys that the design explicitly removed — this is a spec-document drift, not an implementation bug. The spec should be updated to reflect ADR-5's bare-signal decision.
